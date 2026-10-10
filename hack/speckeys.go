package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"text/template/parse"

	"github.com/asgard-ai-platform/asgard-fde-cli/hack/internal/src"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/generate"
	"gopkg.in/yaml.v3"
)

func init() {
	register("spec-key-gap", check{
		Needs: "the clones, helm and a built binary",
		What:  "**how much of a production chart `add` never writes** - the number behind \"the chart half is the least finished\", rendered on both sides rather than quoted, with `add` run across the flag combinations it supports rather than one per kind. A key it does not write is in one of four states - written, named in a commented skeleton, absent on purpose with the document that says why, or nowhere - and only the last is a gap: --missing prints it and lists the others, --shown lists the skeletons. Fails if a decision's document is gone or has stopped naming its key",
		Run:   runSpecKeyGap,
	})
}

// **What `add` can write is not one run per kind.** Several templates branch on
// a flag - `<<if .Private>>`, `<<if .Supervisor>>`, the `--db-class` blocks
// - and a key that exists only inside one of those branches is still a key
// `add` writes. One combination per kind counted every one of them as never
// written: 48 of the 165 keys this check reported as missing were already
// generated, which is the expensive direction to be wrong in, because the
// number is what somebody reads before implementing one of them.
//
// So the combinations are **derived from the generator**, in three steps, and
// nothing below lists a flag that a new branch would have to be added to:
//
//  1. every flag `add` registers, and the Options field each one sets, read
//     off the cobra calls in internal/cli/add.go;
//  2. per kind, the Options fields its own templates **branch** on, taken from
//     the template's parse tree. A field that is only interpolated -
//     `<<.DisplayName>>` - changes a value and never a key path, so it is not a
//     combination;
//  3. for a flag whose vocabulary is closed, the vocabulary itself, read from
//     `generate` - so a DataConnector class added upstream is probed here
//     without an edit.
//
// **A flag nobody would type in a real chart still counts as written.** What
// this check measures is what `add` can produce, not what an FDE usually asks
// for: a key behind `--db-class netsuite` is one an FDE gets by typing that, and
// listing it as missing would be telling somebody to implement what exists.

// requiredArgs is what `add` refuses to run at all without, so that a variant
// meant to exercise the other branch of a flag fails loudly rather than
// quietly. Two sources: `requiredFlags` in internal/cli/add.go, and `Resolve`
// in internal/generate, which is what rejects a plugin with no store.
//
// Everything else a kind takes is enumerated, so this names only what would
// otherwise turn a variant into an error.
var requiredArgs = map[string][]string{
	"semanticlayer": {"connector"},
	"httptool":      {"toolset"},
	"querytool":     {"connector", "toolset"},
	"skillset":      {"repo"},
	"plugin":        {"connector"},
}

// sampleValue is what to pass to a flag that takes one. The value decides which
// name appears inside a key's value and never which keys are written, so one
// placeholder per flag covers every kind that takes it - a plugin's store is a
// `ss-` SourceSet rather than a `dc-` DataConnector, and the rendered key paths
// are identical either way.
var sampleValue = map[string]string{
	"connector": "dc-probe",
	"layer":     "sl-probe",
	"layers":    "sl-probe",
	"toolset":   "ts-probe",
	"repo":      "https://github.com/example/skills",
}

// enumDomain is the closed vocabulary of a flag, where it has one. The values
// are the generator's own; only the two flag names are written here, because
// nothing in the generator declares which of its flags are enumerated.
func enumDomain(flag string) []string {
	switch flag {
	case "db-class":
		return generate.DBClasses()
	case "bot-class":
		return generate.BotClasses
	}
	return nil
}

// structureField is the exception to step 2 above, and the reason it is
// written down rather than derived: a field that is only interpolated normally
// changes a value inside a key, but `<<.DBSpec>>` interpolates **a whole spec
// block**, so each `--db-class` value is a different set of keys with no branch
// anywhere in the template. Nothing in a parse tree distinguishes a
// field that carries a scalar from one that carries YAML.
//
// `DBNote` is the same flag's note, and is a branch; naming it here keeps both
// halves of the class attributed to the flag that decides them.
var structureField = map[string]string{
	"DBSpec": "db-class",
	"DBNote": "db-class",
}

// stateSeed names the template fields that are **what the chart already
// contains** rather than anything typed, and what has to be in the chart for
// each to be true. A kind that branches on one is probed twice: once in an
// empty project and once in a project seeded with these.
//
// Both branches matter. An Agent in a chart with no SkillSet writes a comment
// where one with a SkillSet writes `managed.skillSetNames`, and a chart is only
// ever in one of those two states at a time.
var stateSeed = map[string][]string{
	"SkillSets":     {"add", "skillset", "seed", "--repo", "https://github.com/example/skills"},
	"ToolsetExists": {"add", "httptool", "seedtool", "--toolset", "ts-probe"},
}

// flagRegistration reads a cobra flag registration: the variable it binds, and
// the flag name. `--layers` binds a local rather than an Options field, which is
// why the `opts.` is optional and the match is made case-insensitively against
// the field a template names.
var flagRegistration = regexp.MustCompile(`cmd\.Flags\(\)\.(String|StringSlice|Bool)Var\(&(?:opts\.)?(\w+), "([\w-]+)"`)

type addFlag struct {
	name   string // as typed, without the dashes
	isBool bool
}

// addFlags maps an Options field, lowercased, to the flag that sets it.
func addFlags(root string) (map[string]addFlag, error) {
	path := filepath.Join(root, "internal/cli/add.go")
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]addFlag{}
	for _, m := range flagRegistration.FindAllStringSubmatch(string(body), -1) {
		out[strings.ToLower(m[2])] = addFlag{name: m[3], isBool: m[1] == "Bool"}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no flag registration found in %s, so every combination below would be the default one", path)
	}
	return out, nil
}

// fieldsUsed returns the fields a template branches on - the pipelines of its
// if, range and with actions - and, separately, every field it names at all.
// The first is what changes which keys are written; the second is only read for
// structureField below.
//
// It reads the parse tree rather than matching text, because `<<if eq .BotClass
// "generic">>` and `<<if .Write>>POST<<else>>GET<<end>>` are both branches and
// neither is the shape a regular expression over `<<if .X>>` catches.
func fieldsUsed(path string) (branch, all map[string]bool, err error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	tree := parse.New(filepath.Base(path))
	// The generator registers its own functions; SkipFuncCheck means a
	// function added there is not a parse error here.
	tree.Mode = parse.SkipFuncCheck
	if _, err := tree.Parse(string(body), "<<", ">>", map[string]*parse.Tree{}); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	branch, all = map[string]bool{}, map[string]bool{}
	var pipe func(*parse.PipeNode, map[string]bool)
	pipe = func(p *parse.PipeNode, into map[string]bool) {
		if p == nil {
			return
		}
		for _, cmd := range p.Cmds {
			for _, arg := range cmd.Args {
				switch a := arg.(type) {
				case *parse.FieldNode:
					if len(a.Ident) > 0 {
						into[a.Ident[0]] = true
						all[a.Ident[0]] = true
					}
				case *parse.PipeNode:
					pipe(a, into)
				}
			}
		}
	}
	var walk func(parse.Node)
	walk = func(n parse.Node) {
		switch t := n.(type) {
		case *parse.ListNode:
			if t == nil {
				return
			}
			for _, c := range t.Nodes {
				walk(c)
			}
		case *parse.ActionNode:
			pipe(t.Pipe, all)
		case *parse.IfNode:
			pipe(t.Pipe, branch)
			walk(t.List)
			walk(t.ElseList)
		case *parse.RangeNode:
			pipe(t.Pipe, branch)
			walk(t.List)
			walk(t.ElseList)
		case *parse.WithNode:
			pipe(t.Pipe, branch)
			walk(t.List)
			walk(t.ElseList)
		}
	}
	walk(tree.Root)
	return branch, all, nil
}

// probe is one scratch project: what to create in it before anything else, and
// the flag combinations to run in it.
//
// One project per kind rather than one for everything, because the chart's own
// contents decide two of the branches and one kind's leftovers would settle
// another kind's: an Agent added to a chart holding one SemanticLayer mounts it
// without being asked, and `add agent` refuses outright once there are two.
type probe struct {
	kind  string
	slug  string
	seeds [][]string // whole `add` argv, minus --project
	runs  [][]string // flag arguments, one entry per run
}

// addMatrix plans every run, from the generator rather than from a list.
func addMatrix(root string) ([]probe, error) {
	flags, err := addFlags(root)
	if err != nil {
		return nil, err
	}
	var out []probe
	for _, k := range generate.Kinds {
		branch, used := map[string]bool{}, map[string]bool{}
		for _, f := range k.Files {
			b, a, err := fieldsUsed(filepath.Join(root, "internal/generate/templates", f.Template))
			if err != nil {
				return nil, err
			}
			for name := range b {
				branch[name] = true
			}
			for name := range a {
				used[name] = true
			}
		}

		required := toSet(requiredArgs[k.Name])
		var base []string
		for _, name := range requiredArgs[k.Name] {
			arg, err := flagArgs(addFlag{name: name}, k.Name)
			if err != nil {
				return nil, err
			}
			base = append(base, arg...)
		}

		// What to vary: the flags this kind's own templates react to.
		combine := map[string]bool{}
		var seeds [][]string
		for _, field := range sortedKeys(used) {
			if flag, ok := structureField[field]; ok {
				combine[flag] = true
				continue
			}
			// A field that is only interpolated puts a name inside a value and
			// leaves the key paths alone, so it is not worth a run.
			if !branch[field] {
				continue
			}
			if seed, ok := stateSeed[field]; ok {
				seeds = append(seeds, seed)
				continue
			}
			f, ok := flags[strings.ToLower(field)]
			if !ok {
				// **The one thing that would quietly shrink this side again.**
				// A template that grows a branch on something no flag sets, and
				// that is not chart state, is a branch no run here ever takes.
				return nil, fmt.Errorf("a %s template branches on .%s, which no `add` flag sets\n"+
					"and which is neither chart state nor a structure block. Add it to stateSeed\n"+
					"or structureField in hack/speckeys.go, or the keys behind that branch will be\n"+
					"reported as never written", k.Name, field)
			}
			combine[f.name] = true
		}

		var toggles, refs, enums [][]string
		for _, name := range sortedKeys(combine) {
			if required[name] {
				continue
			}
			if domain := enumDomain(name); domain != nil {
				for _, v := range domain {
					enums = append(enums, []string{"--" + name, v})
				}
				continue
			}
			f, ok := flagByName(flags, name)
			if !ok {
				return nil, fmt.Errorf("%s needs --%s and `add` no longer registers it", k.Name, name)
			}
			arg, err := flagArgs(f, k.Name)
			if err != nil {
				return nil, err
			}
			if f.isBool {
				toggles = append(toggles, arg)
			} else {
				refs = append(refs, arg)
			}
		}

		// Not the cartesian product: the base, each optional flag on its own,
		// and all of them at once, once per value of an enumerated flag. That
		// covers every key behind one flag and every key behind the whole set;
		// a key needing exactly two of them and not the rest would be missed,
		// and no template has one.
		var all []string
		for _, a := range append(append([][]string{}, toggles...), refs...) {
			all = append(all, a...)
		}
		var runs [][]string
		add := func(args ...[]string) {
			var run []string
			run = append(run, base...)
			for _, a := range args {
				run = append(run, a...)
			}
			runs = append(runs, run)
		}
		add()
		for _, e := range enums {
			add(e)
			if len(all) > 0 {
				add(e, all)
			}
		}
		if len(enums) == 0 && len(all) > 0 {
			add(all)
		}
		for _, a := range append(append([][]string{}, toggles...), refs...) {
			add(a)
		}
		runs = dedupeRuns(runs)

		out = append(out, probe{kind: k.Name, slug: k.Name, runs: runs})
		if len(seeds) > 0 {
			out = append(out, probe{kind: k.Name, slug: k.Name + "-seeded", seeds: seeds, runs: runs})
		}
	}
	return out, nil
}

// flagByName finds a registration by the flag as typed.
func flagByName(flags map[string]addFlag, name string) (addFlag, bool) {
	for _, f := range flags {
		if f.name == name {
			return f, true
		}
	}
	return addFlag{}, false
}

// flagArgs is how one flag is typed, with its placeholder where it takes one.
func flagArgs(f addFlag, kind string) ([]string, error) {
	if f.isBool {
		return []string{"--" + f.name}, nil
	}
	v, ok := sampleValue[f.name]
	if !ok {
		return nil, fmt.Errorf("--%s takes a value and hack/speckeys.go has no placeholder for it,\n"+
			"so %s cannot be probed with it", f.name, kind)
	}
	return []string{"--" + f.name, v}, nil
}

func dedupeRuns(runs [][]string) [][]string {
	seen := map[string]bool{}
	var out [][]string
	for _, r := range runs {
		k := strings.Join(r, "\x00")
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, r)
	}
	return out
}

// Values the platform injects per run, which helm cannot know. `asgard-cli
// render` supplies them; this renders with helm directly so that both sides are
// rendered the same way, so they are supplied here.
const injected = `asgard:
  projectEnvironmentId: probe-env
  projectId: probe-project
  appSecretName: probe-secret
  namespace: probe-ns
`

// The claim, not the count. APPROACH.md states the judgement - that the chart
// half is the least finished, because `add` writes a starting point rather than
// a chart - and this holds whether that is still true. The numbers are printed
// by the run above rather than written in prose, where they would go stale.
var specKeyClaim = regexp.MustCompile(`the least finished`)

// fieldName is what a CRD field is called: lowerCamelCase, no separators. A
// segment that is not one is a name somebody chose.
var fieldName = regexp.MustCompile(`^[a-z][a-zA-Z0-9]*$`)

// specKeys returns every dotted key path under `spec`, with list indices
// collapsed.
//
// Collapsing indices is what makes the two sides comparable: `processors.0` and
// `processors.7` are the same key, and counting them apart would say the gap
// closes as a chart grows.
func specKeys(text string) map[string]bool {
	out := map[string]bool{}
	var walk func(node any, path string)
	walk = func(node any, path string) {
		switch t := node.(type) {
		case map[string]any:
			for k, v := range t {
				// **A map key is data, not a field**, and counting one as a
				// spec key asks `add` to write a name somebody chose. An
				// indexer called `idx-00001` and a repository path inside
				// `members` were both being reported as keys the generator
				// never writes - which is true and unfixable, because the
				// generator cannot know what a customer will call theirs.
				//
				// Collapsed rather than dropped, because what hangs off the
				// map IS schema: `docx.indexers.<key>.chunkSize` is a field
				// and `docx.indexers.idx-00001.chunkSize` is one chart's.
				seg := k
				if !fieldName.MatchString(k) {
					seg = "<key>"
				}
				child := seg
				if path != "" {
					child = path + "." + seg
				}
				out[child] = true
				walk(v, child)
			}
		case []any:
			for _, e := range t {
				walk(e, path)
			}
		}
	}
	// **One document at a time, and a failure skips only that one.** A
	// streaming decoder stops at the first document it cannot parse, which
	// silently abandons every document after it - here that lost two keys from
	// a chart whose later documents were fine, and the gap looked smaller than
	// it is.
	for _, doc := range strings.Split(text, "\n---") {
		var parsed map[string]any
		if err := yaml.Unmarshal([]byte(doc), &parsed); err != nil {
			continue
		}
		if spec, ok := parsed["spec"]; ok {
			walk(spec, "")
		}
	}
	return out
}

// referenceValues is what a reference chart is rendered with: its
// per-environment values file when it still has one, and the values the
// platform injects in every case. A chart deployed by the platform pipeline has
// no per-environment file and requires `.Values.asgard.*`, so rendering it
// without these gives nothing.
func referenceValues(holder string) ([]string, error) {
	var values []string
	for _, n := range []string{"values-prod.yaml", "values-dev.yaml"} {
		if _, err := os.Stat(filepath.Join(holder, n)); err == nil {
			values = append(values, filepath.Join(holder, n))
			break
		}
	}
	f, err := injectedFile()
	if err != nil {
		return nil, err
	}
	return append(values, f), nil
}

var injectedOnce struct {
	sync.Once
	path string
	err  error
}

// injectedFile writes the injected values once per process and returns where.
func injectedFile() (string, error) {
	injectedOnce.Do(func() {
		f, err := os.CreateTemp("", "asgard-injected-*.yaml")
		if err != nil {
			injectedOnce.err = err
			return
		}
		defer f.Close()
		if _, err := f.WriteString(injected); err != nil {
			injectedOnce.err = err
			return
		}
		injectedOnce.path = f.Name()
	})
	return injectedOnce.path, injectedOnce.err
}

func helmRender(chart string, values []string) string {
	args := []string{"template", "probe", chart}
	for _, v := range values {
		args = append(args, "-f", v)
	}
	out, _ := exec.Command("helm", args...).Output()
	return string(out)
}

// production returns each reference chart's spec keys.
//
// **Only the deployments `source/SOURCES.md` declares.** `$ASGARD_DEPLOYMENTS`
// is somebody's projects directory: it also holds scratch repositories this
// tool scaffolded, and those pass by construction - one showed 0 keys not
// written, which would make the gap look like it had closed.
func production(base, root string) (map[string]map[string]bool, error) {
	doc, err := sourcesDoc(root)
	if err != nil {
		return nil, err
	}
	known := toSet(referenceDeployments(doc))
	out := map[string]map[string]bool{}
	_ = filepath.Walk(base, func(p string, fi os.FileInfo, err error) error {
		// A clone is a checkout directly under base; one further down is a
		// worktree or a clone inside a clone, and its charts are not this
		// deployment's.
		if err == nil && p != base && filepath.Dir(p) != base && nestedCheckout(fi, p) {
			return filepath.SkipDir
		}
		if err != nil || !fi.IsDir() || filepath.Base(p) != "app" {
			return nil
		}
		if filepath.Base(filepath.Dir(p)) != "chart" {
			return nil
		}
		rel, err := filepath.Rel(base, p)
		if err != nil || !known[strings.Split(rel, string(filepath.Separator))[0]] {
			return nil
		}
		holder := filepath.Dir(p)
		values, err := referenceValues(holder)
		if err != nil {
			return err
		}
		text := helmRender(p, values)
		if strings.TrimSpace(text) == "" {
			return nil
		}
		label, _ := filepath.Rel(base, holder)
		out[label] = specKeys(text)
		return nil
	})
	return out, nil
}

// commented collects the spec keys the templates NAME in a comment without
// writing them.
//
// **A key taught where somebody meets it is not a missing key**, and counting
// it as one is the same measurement error as running one flag combination per
// kind: the number says work is owed where the work is done. A commented
// skeleton is often the better answer - `sourceSetMounts` and the blueprint's
// `agents` are choices, so a generator that guesses is worse than one that
// shows the shape and leaves the decision - and a render cannot see a comment.
//
// So a key is in one of four states rather than two: written, named where it
// is used, absent on purpose with the document that says why, and nowhere.
// Only the last is a gap.
//
// **Matched on the field name, and that is as far as it goes.** A comment line
// that reads as a YAML key - `#`, optional indent, a key, a colon - and nothing
// else; prose in a comment happens to contain colons, so anything with a space
// before the colon or a capital in the key is not one.
//
// **Reconstructing the full path was tried and is wrong here**, because a
// skeleton in this repository is written ABOVE the key it belongs to rather
// than inside it - the measure entry sits over `measures: []`, after
// `dimensions:` - so walking the enclosing structure attributes it to the
// previous sibling. What the comment reliably says is which field it names, and
// that is what this holds.
//
// The limit that leaves: a field name shown in one template counts as shown for
// every kind. That mattered exactly once, for the fields under a CR kind `add`
// does not generate at all, and it is the decided set that answers those - a
// decision covers what is under it.
var commentedKey = regexp.MustCompile(`(?m)^\s*#\s{0,6}-?\s*([a-z][a-zA-Z0-9_]*):`)

func commented(root string) (map[string]bool, error) {
	out := map[string]bool{}
	paths, err := filepath.Glob(filepath.Join(root, "internal/generate/templates/*.tmpl"))
	if err != nil {
		return nil, err
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		for _, m := range commentedKey.FindAllStringSubmatch(string(data), -1) {
			out[m[1]] = true
		}
	}
	return out, nil
}

func ours(binary, root string) (map[string]bool, int, error) {
	matrix, err := addMatrix(root)
	if err != nil {
		return nil, 0, err
	}
	tmp, err := os.MkdirTemp("", "asgard-keys-")
	if err != nil {
		return nil, 0, err
	}
	defer os.RemoveAll(tmp)
	if err := exec.Command("git", "-C", tmp, "init", "-q", ".").Run(); err != nil {
		return nil, 0, err
	}
	if _, stderr, err := runCLI(binary, tmp, "init"); err != nil {
		return nil, 0, fmt.Errorf("`init` failed in a scratch repository:\n%s", stderr)
	}
	vals := filepath.Join(tmp, "injected.yaml")
	if err := os.WriteFile(vals, []byte(injected), 0o644); err != nil {
		return nil, 0, err
	}

	keys := map[string]bool{}
	runs := 0
	for _, p := range matrix {
		if _, stderr, err := runCLI(binary, tmp, "project", "add", p.slug); err != nil {
			return nil, 0, fmt.Errorf("`project add %s` failed:\n%s", p.slug, stderr)
		}
		for _, seed := range p.seeds {
			argv := append(append([]string{}, seed...), "--project", p.slug)
			if _, stderr, err := runCLI(binary, tmp, argv...); err != nil {
				return nil, 0, fmt.Errorf("seeding %s with `%s` failed, so the branch it exists to\n"+
					"reach would be reported as never written:\n%s",
					p.slug, strings.Join(argv, " "), trim(stderr, 400))
			}
		}
		for i, flags := range p.runs {
			name := fmt.Sprintf("%s-%d", p.kind, i)
			argv := append([]string{"add", p.kind, name, "--project", p.slug}, flags...)
			if _, stderr, err := runCLI(binary, tmp, argv...); err != nil {
				return nil, 0, fmt.Errorf("`%s` failed, so the `add` side would be short whatever\n"+
					"that combination writes:\n%s\n"+
					"If the kind grew a flag it refuses to run without, add it to requiredArgs",
					strings.Join(argv, " "), trim(stderr, 400))
			}
			runs++
		}
		text := helmRender(filepath.Join(tmp, "projects", p.slug, "chart/app"), []string{vals})
		if strings.TrimSpace(text) == "" {
			return nil, 0, fmt.Errorf("the scaffolded chart for %s rendered empty, so its keys are missing\n"+
				"from the `add` side and the gap would look wider than it is", p.slug)
		}
		for k := range specKeys(text) {
			keys[k] = true
		}
	}
	return keys, runs, nil
}

// decided is a key `add` deliberately does not write, and the document that
// says so.
//
// **The fourth state, and the reason it is a list rather than a parse.** Three
// states are derivable - a render shows what is written, a template's comments
// show what is named, and the difference is what nobody meets. The fourth is a
// judgement: this key belongs to a shape an engagement should not reach for, so
// a generator for it would make the wrong thing the easy thing. No program
// recovers that from a CRD.
//
// **What is checked is the pointer, not the judgement.** Each row names the
// document that carries the decision, and the check fails when that document is
// gone or has stopped naming the key - the same contract `needs` and `brief`
// hold for the document owning each of their claims. So the judgement is
// written where a reader meets it and this file only records where.
//
// **A row is the judgement, not an enumeration.** `decidedFor` walks up, so a
// decision covers every field under it - listing them would be a hand-kept copy
// of the CRD, which is the shape this repository removes everywhere else.
//
// **The alternative was worse.** A commented skeleton would move these out of
// `--missing` and is an invitation to use the thing the material tells you not
// to; manufacturing one to lower a number is a checker holding a copy.
var decided = map[string]string{
	// The Neko editor sidecar, which is the browser sidecar's opposite: a
	// screen a PERSON drives, into a sandbox somebody is debugging.
	"editorServer": "internal/corpus/usecase/browser-operation.md",

	// The KnowledgeBase shape, whose four CRs `add` deliberately does not
	// write: `../wiki/knowledge.md` recommends a Drive with a Context Index for
	// new work, so a generator here would make the older shape the easy one.
	// `asgard-cli add knowledgedrive` is the route.
	"knowledgeBaseClass": "internal/corpus/usecase/knowledge-base.md",
	"knowledgeBaseName":  "internal/corpus/usecase/knowledge-base.md",
	"loaderClass":        "internal/corpus/usecase/knowledge-base.md",
	"sourceClass":        "internal/corpus/usecase/knowledge-base.md",
	"asgardBaseline":     "internal/corpus/usecase/knowledge-base.md",
	"deletedIndexerKeys": "internal/corpus/usecase/knowledge-base.md",
	"docx":               "internal/corpus/usecase/knowledge-base.md",

	// The CompletionModel class blocks. `add` has no `completionmodel` kind and
	// `../wiki/settings.md` carries why: the common case writes no CR at all,
	// and the uncommon one is a contract with a provider - which model id,
	// whose key, who pays for the tokens - that no skeleton can guess.
	"completionModelClass": "internal/corpus/wiki/settings.md",
	"anthropic":            "internal/corpus/wiki/settings.md",
	"gemini":               "internal/corpus/wiki/settings.md",
	"openaiChat":           "internal/corpus/wiki/settings.md",

	// A Workflow's label maps are open in the CRD and the runtime reads none of
	// their keys. The entry's `default` is the dangerous one: it reads as "the
	// entry to start at", when the entry is the BotProvider's
	// `entrypoint.entry` and a workflow that starts in the wrong place is not
	// something any check reports.
	"entries.labels.default": "internal/corpus/usecase/workflow-chain.md",
	"relationships.labels":   "internal/corpus/usecase/workflow-chain.md",

	// The pre-rename SourceSet/Syncer pair, and the two halves are not in the
	// same state: `members` is gone from the SourceSet, so a chart setting it
	// loses the field in silence, while the Syncer's `destinationMemberKey` is
	// deprecated and still applies. Writing either into a new chart is the
	// mistake `../usecase/skill-set.md` exists to stop, and `gate` warns on the
	// one that still resolves.
	"members":              "internal/corpus/usecase/skill-set.md",
	"destinationMemberKey": "internal/corpus/usecase/skill-set.md",
}

// decidedFor returns the document deciding this key absent, or the nearest
// decision above it.
//
// **A decision covers what is under it.** `add` writes no CompletionModel at
// all, so `anthropic` being absent on purpose settles
// `anthropic.apiKey.valueFrom.secretKeyRef.key` too - and listing every field
// of a CR nobody generates would be a copy of the CRD kept by hand, which is
// the shape this repository removes everywhere else. It also closes the way the
// third state used to leak: a generic leaf like `key` or `name` is commented in
// some template somewhere, so without this those fields were being reported as
// shown in a skeleton that belongs to another kind entirely.
func decidedFor(key string) string {
	for k := key; ; {
		if doc := decided[k]; doc != "" {
			return doc
		}
		i := strings.LastIndex(k, ".")
		if i < 0 {
			return ""
		}
		k = k[:i]
	}
}

// mapLeaf is the segment a comment would name. A `<key>` segment is a name
// somebody chooses, so what a skeleton can show is the map it hangs off -
// `labels:` with `display_name:` under it, never the customer's own key.
func mapLeaf(k string) string {
	k = strings.TrimSuffix(k, ".<key>")
	if i := strings.LastIndex(k, "."); i >= 0 {
		return k[i+1:]
	}
	return k
}

// evidence is what a document has to contain for it to still be carrying a
// decision about this key: the dotted key, then each shorter dotted suffix,
// down to the bare field name. Any one of them is enough. `<key>` segments are
// dropped first, because a customer's own map key is not something any document
// can name.
func evidence(key string) []string {
	var seg []string
	for _, s := range strings.Split(key, ".") {
		if s != "<key>" {
			seg = append(seg, s)
		}
	}
	var forms []string
	for i := range seg {
		forms = append(forms, strings.Join(seg[i:], "."))
	}
	return forms
}

// names reports whether text names a field, as opposed to happening to contain
// the word.
//
// **The difference is the whole check.** `workingDirectory` appears in a
// document for exactly one reason, but `default`, `labels` and `members` are
// ordinary English, and a check satisfied by a bare occurrence passes on a page
// whose decision has been deleted - a green check over material nobody
// maintains, which is what this file exists to avoid.
//
// **And a check that fires on correct material is worse than none**, which
// rules out demanding the dotted path: a document showing a shape writes
// `editorServer:` with `enabled:` under it, and that is the right way to show
// it. So what counts is the form a field is written in and prose is not - the
// word carrying a colon after it, closing a backtick, or sitting after a dot in
// a path. "that is the default, and it can be raised" is none of the three;
// `default` in backticks, `enabled:` and `spec.deletedIndexerKeys` are each one.
//
// A dotted form is matched plainly: a dot between two field names does not
// arrive by accident.
func names(text, form string) bool {
	if strings.Contains(form, ".") {
		return strings.Contains(text, form)
	}
	return regexp.MustCompile(`(?:^|[^A-Za-z0-9_])` + regexp.QuoteMeta(form) + "[:`]" +
		`|\.` + regexp.QuoteMeta(form) + `\b`).MatchString(text)
}

// checkDecided reports a decision whose document is gone or no longer names the
// key. A pointer nobody maintains is how a decision quietly becomes an absence.
func checkDecided(root string) []string {
	var bad []string
	for key, doc := range decided {
		data, err := os.ReadFile(filepath.Join(root, doc))
		if err != nil {
			bad = append(bad, fmt.Sprintf("%s is decided absent and %s is not there", key, doc))
			continue
		}
		found := false
		forms := evidence(key)
		for _, f := range forms {
			if names(string(data), f) {
				found = true
				break
			}
		}
		if !found {
			bad = append(bad, fmt.Sprintf("%s is decided absent and %s names none of %s",
				key, doc, strings.Join(forms, ", ")))
		}
	}
	sort.Strings(bad)
	return bad
}

func runSpecKeyGap(args []string) error {
	for _, tool := range []string{"helm"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%s is not on PATH, and both sides have to be rendered", tool)
		}
	}
	root, err := src.Root()
	if err != nil {
		return err
	}
	base, err := src.Resolve("deployments")
	if err != nil {
		return err
	}
	binary := os.Getenv("ASGARD_CLI")
	if binary == "" {
		binary = filepath.Join(root, ".out/asgard-cli")
	}
	if _, err := os.Stat(binary); err != nil {
		return fmt.Errorf("no binary at %s\n  go build -o .out/asgard-cli ./cmd/asgard-cli\n"+
			"  Set $ASGARD_CLI to use another one.", binary)
	}
	binary, _ = filepath.Abs(binary)

	prod, err := production(base, root)
	if err != nil {
		return err
	}
	if len(prod) == 0 {
		return fmt.Errorf("no reference chart under %s rendered, so there is nothing to measure", base)
	}
	mine, runs, err := ours(binary, root)
	if err != nil {
		return err
	}

	every := map[string]bool{}
	widest, widestN := "", -1
	for label, keys := range prod {
		for k := range keys {
			every[k] = true
		}
		if len(keys) > widestN {
			widest, widestN = label, len(keys)
		}
	}

	named, err := commented(root)
	if err != nil {
		return err
	}
	if bad := checkDecided(root); len(bad) > 0 {
		for _, b := range bad {
			fmt.Printf("adrift  %s\n", b)
		}
		fmt.Println("\nA decision is only as good as the document carrying it. Fix the pointer")
		fmt.Println("or drop the row - a key nobody decided about is a key nobody meets.")
		return errFailed
	}

	if len(args) > 0 && args[0] == "--missing" {
		var missing, shown, absent []string
		for k := range every {
			switch {
			case mine[k]:
			case decidedFor(k) != "":
				absent = append(absent, k)
			case named[mapLeaf(k)]:
				shown = append(shown, k)
			default:
				missing = append(missing, k)
			}
		}
		sort.Strings(absent)
		sort.Strings(missing)
		sort.Strings(shown)
		for _, k := range missing {
			fmt.Printf("  %s\n", k)
		}
		if len(missing) == 0 {
			fmt.Println("Every spec key a production chart uses has a home: written by `add`,")
			fmt.Println("named in a commented skeleton where somebody meets it, or absent on")
			fmt.Println("purpose with the document that says why. A key with none is listed here.")
		} else {
			fmt.Printf("\n%d key(s) production uses and `add` neither writes nor names.\n", len(missing))
		}
		if len(absent) > 0 {
			fmt.Printf("\n%d are absent on purpose, each with the document that says why:\n", len(absent))
			for _, k := range absent {
				fmt.Printf("  %-46s %s\n", k, decidedFor(k))
			}
			fmt.Println("A generator for one of these would make the shape the material tells")
			fmt.Println("you not to reach for into the easy one.")
		}
		if len(shown) > 0 {
			fmt.Printf("\n%d more are named in a commented skeleton, where somebody meets them\n", len(shown))
			fmt.Println("at the moment they would write one. That is not a gap: a key that is a")
			fmt.Println("choice is better shown than guessed, and a render cannot see a comment.")
			fmt.Println("`--shown` lists them.")
		}
		return nil
	}
	if len(args) > 0 && args[0] == "--shown" {
		var shown []string
		for k := range every {
			if !mine[k] && decidedFor(k) == "" && named[mapLeaf(k)] {
				shown = append(shown, k)
			}
		}
		sort.Strings(shown)
		for _, k := range shown {
			fmt.Printf("  %s\n", k)
		}
		fmt.Printf("\n%d key(s) `add` names in a comment rather than writing.\n", len(shown))
		return nil
	}

	labels := sortedKeys(prod)
	// Widest first, and ties by name so the order is the same on every run.
	sort.Slice(labels, func(i, j int) bool {
		if len(prod[labels[i]]) != len(prod[labels[j]]) {
			return len(prod[labels[i]]) > len(prod[labels[j]])
		}
		return labels[i] < labels[j]
	})
	for _, label := range labels {
		fmt.Printf("  %-44s%4d spec key(s), %4d not written\n",
			label, len(prod[label]), countMissing(prod[label], mine))
	}
	fmt.Printf("\n  %-44s%4d spec key(s), %4d not written\n",
		"every reference chart together", len(every), countMissing(every, mine))
	fmt.Printf("  %-44s%4d spec key(s), from %3d `add` run(s)\n", "what `add` writes", len(mine), runs)

	task, err := os.ReadFile(filepath.Join(root, "APPROACH.md"))
	if err != nil {
		return err
	}
	m := specKeyClaim.FindStringSubmatch(string(task))
	if m == nil {
		fmt.Println("\nAPPROACH.md no longer says the chart half is the least finished, so this")
		fmt.Println("measures nothing. Either the claim is back, or this check goes with it.")
		return errFailed
	}
	wantKeys, wantMissing := len(prod[widest]), countMissing(prod[widest], mine)
	if wantMissing == 0 {
		fmt.Printf("\n`add` now writes every one of the widest chart's %d spec keys.\n", wantKeys)
		fmt.Println("APPROACH.md still says the chart half is the least finished, and that is what is")
		fmt.Println("wrong now - rewrite the paragraph rather than this check.")
		return errFailed
	}
	fmt.Printf("\nThe widest reference chart uses %d spec keys and `add` does not write %d.\n",
		wantKeys, wantMissing)
	fmt.Println("APPROACH.md states that as a judgement and carries no number, which is why")
	fmt.Println("neither can go stale. `--missing` splits those by what is owed on them:")
	fmt.Println("nowhere at all, named in a commented skeleton, or absent on purpose with")
	fmt.Println("the document that says why. `--shown` lists the skeletons.")
	return nil
}

func countMissing(from, have map[string]bool) int {
	n := 0
	for k := range from {
		if !have[k] {
			n++
		}
	}
	return n
}
