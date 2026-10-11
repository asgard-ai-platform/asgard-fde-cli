package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template/parse"

	"github.com/asgard-ai-platform/asgard-fde-cli/hack/internal/src"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/generate"
	"gopkg.in/yaml.v3"
)

func init() {
	register("spec-key-gap", check{
		Needs: "$ASGARD_KUBE, helm and a built binary",
		What:  "**how much of the CRD contract `add` never writes** - the claim behind \"the chart half is the least finished\", measured per kind against the CRDs at $ASGARD_KUBE rather than against any deployment, with `add` run across the flag combinations it supports. A field it does not write is in one of four states - written, named in a commented skeleton, absent on purpose with the document that says why, or nowhere - and only the last is owed: --missing prints it and lists the others, --shown lists the skeletons. Fails if `add` writes a field the CRD does not define, or if a decision's document is gone, has stopped naming its field, is about a field the CRD no longer has, or covers a field `add` now writes",
		Run:   runSpecKeyGap,
	})
}

// **What `add` can write is not one run per kind.** Several templates branch on
// a flag - `<<if .Private>>`, `<<if .Supervisor>>`, the `--db-class` blocks
// - and a key that exists only inside one of those branches is still a key
// `add` writes. One combination per kind counts every one of them as never
// written, which is the expensive direction to be wrong in, because the
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
// render` supplies them; this renders with helm directly, so they are supplied
// here.
const injected = `asgard:
  projectEnvironmentId: probe-env
  projectId: probe-project
  appSecretName: probe-secret
  namespace: probe-ns
`

// The claim, not the count. APPROACH.md states the judgement - that the chart
// half is the least finished, because `add` writes a starting point rather than
// a chart - and this holds whether that is still true. The numbers are printed
// by the run below rather than written in prose, where they would go stale.
var specKeyClaim = regexp.MustCompile(`the least finished`)

// A field is written `Kind.path`: the CR kind, then the dotted path under
// `spec`, with list indices collapsed and a map's own keys as `<key>`.
//
// The kind is part of the name because the contract is per kind. `name` under
// one kind and `name` under another are different fields, and merging them
// would let a key one generator writes cover a kind no generator writes.
func field(kind, path string) string {
	if path == "" {
		return kind
	}
	return kind + "." + path
}

func joinPath(path, seg string) string {
	if path == "" {
		return seg
	}
	return path + "." + seg
}

// contract returns every spec field each CRD defines, by kind. The CRD is the
// denominator: what the platform accepts, rather than what one deployment
// happened to set.
//
// A map whose keys are the customer's to choose - `additionalProperties` - is
// one field, `<key>`, because a generator cannot know what a customer will call
// theirs.
func contract(crds []crd) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, c := range crds {
		keys := map[string]bool{}
		var walk func(node map[string]any, path string)
		walk = func(node map[string]any, path string) {
			if props, ok := node["properties"].(map[string]any); ok {
				for name, v := range props {
					child := joinPath(path, name)
					keys[child] = true
					if s, ok := v.(map[string]any); ok {
						walk(s, child)
					}
				}
			}
			if items, ok := node["items"].(map[string]any); ok {
				walk(items, path)
			}
			if ap, ok := node["additionalProperties"].(map[string]any); ok {
				child := joinPath(path, "<key>")
				keys[child] = true
				walk(ap, child)
			}
		}
		walk(c.spec(), "")
		out[c.Spec.Names.Kind] = keys
	}
	return out
}

// openSchema reports whether a schema node takes keys it does not name, so a
// key under it is neither a field of the contract nor a mistake.
func openSchema(node map[string]any) bool {
	if node["x-kubernetes-preserve-unknown-fields"] == true {
		return true
	}
	ap, ok := node["additionalProperties"].(bool)
	return ok && ap
}

// written walks one rendered spec alongside its kind's schema. A key the schema
// has is recorded; a key it does not have, under a node that is not open, is
// returned as unknown, because the apiserver refuses it.
//
// Walking the schema rather than guessing from the key's spelling is what tells
// a map key from a field: `labels.default` is a customer's key under a map,
// and `processors.config` is a field, and both are lowerCamelCase.
func written(node any, schema map[string]any, path string, keys map[string]bool, unknown map[string]bool) {
	if schema == nil {
		return
	}
	switch t := node.(type) {
	case map[string]any:
		props, _ := schema["properties"].(map[string]any)
		ap, _ := schema["additionalProperties"].(map[string]any)
		for k, v := range t {
			if s, ok := props[k].(map[string]any); ok {
				child := joinPath(path, k)
				keys[child] = true
				written(v, s, child, keys, unknown)
				continue
			}
			if ap != nil {
				child := joinPath(path, "<key>")
				keys[child] = true
				written(v, ap, child, keys, unknown)
				continue
			}
			if !openSchema(schema) {
				unknown[joinPath(path, k)] = true
			}
		}
	case []any:
		items, _ := schema["items"].(map[string]any)
		for _, e := range t {
			written(e, items, path, keys, unknown)
		}
	}
}

// renderedFields returns the fields a rendered chart sets, by kind, and the
// ones it sets that the contract does not have. A document whose kind has no
// CRD - a Secret, a ConfigMap - is not the platform's contract and is skipped.
//
// **One document at a time, and a failure skips only that one.** A streaming
// decoder stops at the first document it cannot parse, which silently
// abandons every document after it.
func renderedFields(text string, schemas map[string]map[string]any, keys, unknown map[string]map[string]bool) {
	for _, doc := range strings.Split(text, "\n---") {
		var parsed map[string]any
		if err := yaml.Unmarshal([]byte(doc), &parsed); err != nil {
			continue
		}
		kind, _ := parsed["kind"].(string)
		schema, ok := schemas[kind]
		if !ok {
			continue
		}
		if keys[kind] == nil {
			keys[kind], unknown[kind] = map[string]bool{}, map[string]bool{}
		}
		if spec, ok := parsed["spec"]; ok {
			written(spec, schema, "", keys[kind], unknown[kind])
		}
	}
}

func helmRender(chart string, values []string) string {
	args := []string{"template", "probe", chart}
	for _, v := range values {
		args = append(args, "-f", v)
	}
	out, _ := exec.Command("helm", args...).Output()
	return string(out)
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

// ours runs every planned `add` in a scratch repository and returns the
// fields the rendered charts set, by kind, with the ones the contract does not
// have.
func ours(binary, root string, schemas map[string]map[string]any) (keys, unknown map[string]map[string]bool, runs int, err error) {
	matrix, err := addMatrix(root)
	if err != nil {
		return nil, nil, 0, err
	}
	tmp, err := os.MkdirTemp("", "asgard-keys-")
	if err != nil {
		return nil, nil, 0, err
	}
	defer os.RemoveAll(tmp)
	if err := exec.Command("git", "-C", tmp, "init", "-q", ".").Run(); err != nil {
		return nil, nil, 0, err
	}
	if _, stderr, err := runCLI(binary, tmp, "init"); err != nil {
		return nil, nil, 0, fmt.Errorf("`init` failed in a scratch repository:\n%s", stderr)
	}
	vals := filepath.Join(tmp, "injected.yaml")
	if err := os.WriteFile(vals, []byte(injected), 0o644); err != nil {
		return nil, nil, 0, err
	}

	keys, unknown = map[string]map[string]bool{}, map[string]map[string]bool{}
	for _, p := range matrix {
		if _, stderr, err := runCLI(binary, tmp, "project", "add", p.slug); err != nil {
			return nil, nil, 0, fmt.Errorf("`project add %s` failed:\n%s", p.slug, stderr)
		}
		for _, seed := range p.seeds {
			argv := append(append([]string{}, seed...), "--project", p.slug)
			if _, stderr, err := runCLI(binary, tmp, argv...); err != nil {
				return nil, nil, 0, fmt.Errorf("seeding %s with `%s` failed, so the branch it exists to\n"+
					"reach would be reported as never written:\n%s",
					p.slug, strings.Join(argv, " "), trim(stderr, 400))
			}
		}
		for i, flags := range p.runs {
			name := fmt.Sprintf("%s-%d", p.kind, i)
			argv := append([]string{"add", p.kind, name, "--project", p.slug}, flags...)
			if _, stderr, err := runCLI(binary, tmp, argv...); err != nil {
				return nil, nil, 0, fmt.Errorf("`%s` failed, so the `add` side would be short whatever\n"+
					"that combination writes:\n%s\n"+
					"If the kind grew a flag it refuses to run without, add it to requiredArgs",
					strings.Join(argv, " "), trim(stderr, 400))
			}
			runs++
		}
		text := helmRender(filepath.Join(tmp, "projects", p.slug, "chart/app"), []string{vals})
		if strings.TrimSpace(text) == "" {
			return nil, nil, 0, fmt.Errorf("the scaffolded chart for %s rendered empty, so its keys are missing\n"+
				"from the `add` side and the gap would look wider than it is", p.slug)
		}
		renderedFields(text, schemas, keys, unknown)
	}
	return keys, unknown, runs, nil
}

// decided is a field `add` deliberately does not write, or a kind it
// deliberately does not generate, and the document that says so.
//
// **The fourth state, and the reason it is a list rather than a parse.** Three
// states are derivable - a render shows what is written, a template's comments
// show what is named, and the difference is what nobody meets. The fourth is a
// judgement: this field belongs to a shape an engagement should not reach for,
// so a generator for it would make the wrong thing the easy thing. No program
// recovers that from a CRD.
//
// **What is checked is the pointer, not the judgement.** Each row names the
// document that carries the decision, and the check fails when that document is
// gone or has stopped naming the field, or when the contract no longer has the
// field at all - the same contract `needs` and `brief` hold for the document
// owning each of their claims.
//
// **A row is the judgement, not an enumeration.** `decidedFor` walks up, so a
// decision covers every field under it, and a row naming a kind covers the
// whole kind.
//
// **Do not answer a row with a commented skeleton.** That would move it out of
// `--missing` by inviting somebody to use the thing the material tells them not
// to.
var decided = map[string]string{
	// Internal kinds, the runtime object a blueprint produces, and the Indexer
	// a Source owns per indexer key: no chart writes one.
	"Sandbox":               "internal/corpus/wiki/index.md",
	"Indexer":               "internal/corpus/wiki/index.md",
	"ImageGenerationModel":  "internal/corpus/wiki/index.md",
	"TranscriptionModel":    "internal/corpus/wiki/index.md",
	"SourceSetEditorServer": "internal/corpus/wiki/index.md",

	// The Neko editor sidecar, which is the browser sidecar's opposite: a
	// screen a PERSON drives, into a sandbox somebody is debugging.
	"SandboxBlueprint.editorServer": "internal/corpus/usecase/browser-operation.md",

	// The KnowledgeBase shape, whose CRs `add` deliberately does not write:
	// `../wiki/knowledge.md` recommends a Drive with a Context Index for new
	// work, so a generator here would make the older shape the easy one.
	"KnowledgeBase": "internal/corpus/usecase/knowledge-base.md",
	"Loader":        "internal/corpus/usecase/knowledge-base.md",
	"Source":        "internal/corpus/usecase/knowledge-base.md",

	// `add` has no `completionmodel` kind and `../wiki/settings.md` carries
	// why: the common case writes no CR at all, and the uncommon one is a
	// contract with a provider - which model id, whose key, who pays for the
	// tokens - that no skeleton can guess.
	"CompletionModel": "internal/corpus/wiki/settings.md",

	// A relationship's label map is open in the CRD and the runtime reads none
	// of its keys.
	"Workflow.relationships.labels": "internal/corpus/usecase/workflow-chain.md",

	// Deprecated and still applied. Writing it into a new chart is the mistake
	// `../usecase/skill-set.md` exists to stop, and `gate` warns on it.
	"Syncer.destinationMemberKey": "internal/corpus/usecase/skill-set.md",
}

// decidedFor returns the document deciding this field absent, or the nearest
// decision above it, up to its kind.
//
// **A decision covers what is under it.** `add` writes no CompletionModel at
// all, so the kind being absent on purpose settles
// `anthropic.apiKey.valueFrom.secretKeyRef.key` too - and listing every field
// of a CR nobody generates would be a copy of the CRD kept by hand. It also
// keeps the third state honest: a generic leaf like `key` or `name` is
// commented in some template somewhere, so without this a field of a kind
// nobody generates would be reported as shown in a skeleton that belongs to
// another kind entirely.
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
// decision: for a kind, the kind; for a field, the dotted path under `spec`,
// then each shorter dotted suffix, down to the bare field name. Any one of them
// is enough. The kind is not asked of a field's document, because a page names
// `editorServer:` in the shape it shows rather than as `SandboxBlueprint`'s.
// `<key>` segments are dropped, because a customer's own map key is not
// something any document can name.
func evidence(key string) []string {
	parts := strings.Split(key, ".")
	if len(parts) == 1 {
		return parts
	}
	var seg []string
	for _, s := range parts[1:] {
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
// field, and - when the contract is given - one about a kind or field the
// contract no longer has. A pointer nobody maintains is how a decision quietly
// becomes an absence, and a decision about a field the platform removed is a
// row nobody can act on.
func checkDecided(root string, fields map[string]map[string]bool) []string {
	var bad []string
	for key, doc := range decided {
		if fields != nil {
			kind, path, _ := strings.Cut(key, ".")
			if have, ok := fields[kind]; !ok {
				bad = append(bad, fmt.Sprintf("%s is decided absent and asgard-kube has no %s CRD", key, kind))
				continue
			} else if path != "" && !have[path] {
				bad = append(bad, fmt.Sprintf("%s is decided absent and the %s CRD no longer has %s", key, kind, path))
				continue
			}
		}
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

// state is where one contract field stands.
type state int

const (
	isWritten state = iota
	isShown         // named in a commented skeleton
	isDecided       // absent on purpose, with the document that says why
	isNowhere
)

func classify(kind, path string, mine map[string]map[string]bool, named map[string]bool) state {
	switch {
	case mine[kind][path]:
		return isWritten
	case decidedFor(field(kind, path)) != "":
		return isDecided
	case mine[kind] != nil && named[mapLeaf(path)]:
		return isShown
	}
	return isNowhere
}

func runSpecKeyGap(args []string) error {
	if _, err := exec.LookPath("helm"); err != nil {
		return fmt.Errorf("helm is not on PATH, and what `add` writes has to be rendered")
	}
	root, err := src.Root()
	if err != nil {
		return err
	}
	kube, err := src.Resolve("kube")
	if err != nil {
		return err
	}
	crds, err := loadCRDs(filepath.Join(kube, "crd"))
	if err != nil {
		return err
	}
	binary := filepath.Join(root, ".out/asgard-cli")
	if _, err := os.Stat(binary); err != nil {
		return fmt.Errorf("no binary at %s\n  go build -o .out/asgard-cli ./cmd/asgard-cli", binary)
	}

	fields := contract(crds)
	schemas := map[string]map[string]any{}
	for _, c := range crds {
		schemas[c.Spec.Names.Kind] = c.spec()
	}
	if bad := checkDecided(root, fields); len(bad) > 0 {
		for _, b := range bad {
			fmt.Printf("adrift  %s\n", b)
		}
		fmt.Println("\nA decision is only as good as the document carrying it and the field it")
		fmt.Println("is about. Fix the pointer or drop the row.")
		return errFailed
	}
	mine, unknown, runs, err := ours(binary, root, schemas)
	if err != nil {
		return err
	}
	named, err := commented(root)
	if err != nil {
		return err
	}
	fmt.Printf("asgard-kube %s\n\n", src.Commit(kube))

	// A decision about a field `add` now writes in full is a reason kept for
	// something that no longer happens.
	var moot []string
	for key := range decided {
		kind, path, _ := strings.Cut(key, ".")
		if path == "" || mine[kind] == nil {
			continue
		}
		all := true
		for f := range fields[kind] {
			if (f == path || strings.HasPrefix(f, path+".")) && !mine[kind][f] {
				all = false
				break
			}
		}
		if all {
			moot = append(moot, fmt.Sprintf("%s is decided absent and `add` writes all of it", key))
		}
	}
	if len(moot) > 0 {
		sort.Strings(moot)
		for _, b := range moot {
			fmt.Printf("adrift  %s\n", b)
		}
		fmt.Println("\nDrop the row, or stop writing the field.")
		return errFailed
	}

	// A field `add` writes that the contract does not have is refused by the
	// apiserver, whatever this check is measuring, so it fails first.
	var refused []string
	for kind, ks := range unknown {
		for k := range ks {
			refused = append(refused, field(kind, k))
		}
	}
	if len(refused) > 0 {
		sort.Strings(refused)
		for _, k := range refused {
			fmt.Printf("  not in the CRD  %s\n", k)
		}
		fmt.Printf("\n`add` writes %d field(s) the CRD does not define, and the apiserver refuses\n", len(refused))
		fmt.Println("a CR carrying one. Fix the template, or pull asgard-kube if it is behind.")
		return errFailed
	}

	kinds := sortedKeys(fields)
	var kindsNowhere, kindsDecided []string
	tally := map[string][4]int{}
	byState := map[state][]string{}
	for _, kind := range kinds {
		if mine[kind] == nil {
			if decidedFor(kind) != "" {
				kindsDecided = append(kindsDecided, kind)
			} else {
				kindsNowhere = append(kindsNowhere, kind)
			}
			continue
		}
		var t [4]int
		for path := range fields[kind] {
			s := classify(kind, path, mine, named)
			t[s]++
			if s != isWritten {
				byState[s] = append(byState[s], field(kind, path))
			}
		}
		tally[kind] = t
	}
	for _, s := range []state{isShown, isDecided, isNowhere} {
		sort.Strings(byState[s])
	}

	if len(args) > 0 && args[0] == "--missing" {
		for _, k := range kindsNowhere {
			fmt.Printf("  %s (the whole kind)\n", k)
		}
		for _, k := range byState[isNowhere] {
			fmt.Printf("  %s\n", k)
		}
		if len(kindsNowhere)+len(byState[isNowhere]) == 0 {
			fmt.Println("Every field the contract defines has a home: written by `add`, named in")
			fmt.Println("a commented skeleton where somebody meets it, or absent on purpose with")
			fmt.Println("the document that says why.")
		} else {
			fmt.Printf("\n%d kind(s) and %d field(s) the CRDs define that `add` neither writes nor names,\n",
				len(kindsNowhere), len(byState[isNowhere]))
			fmt.Println("and that no document decides against.")
		}
		// By decision rather than by field: a row covers what is under it, and
		// listing each field again would be the CRD.
		covers := map[string]int{}
		for _, k := range byState[isDecided] {
			for d := k; ; {
				if decided[d] != "" {
					covers[d]++
					break
				}
				d = d[:strings.LastIndex(d, ".")]
			}
		}
		if len(kindsDecided)+len(covers) > 0 {
			fmt.Printf("\n%d kind(s) and %d field(s) are absent on purpose, by the document that says why:\n",
				len(kindsDecided), len(byState[isDecided]))
			for _, k := range kindsDecided {
				fmt.Printf("  %-46s %s\n", k+" (the whole kind)", decided[k])
			}
			for _, k := range sortedKeys(covers) {
				fmt.Printf("  %-46s %s\n", fmt.Sprintf("%s (%d field(s))", k, covers[k]), decided[k])
			}
			fmt.Println("A generator for one of these would make the shape the material tells")
			fmt.Println("you not to reach for into the easy one.")
		}
		if n := len(byState[isShown]); n > 0 {
			fmt.Printf("\n%d more are named in a commented skeleton, where somebody meets them\n", n)
			fmt.Println("at the moment they would write one. `--shown` lists them.")
		}
		return nil
	}
	if len(args) > 0 && args[0] == "--shown" {
		for _, k := range byState[isShown] {
			fmt.Printf("  %s\n", k)
		}
		fmt.Printf("\n%d field(s) `add` names in a comment rather than writing.\n", len(byState[isShown]))
		return nil
	}

	fmt.Printf("  %-22s %7s %8s %6s %8s %8s\n", "kind", "fields", "written", "shown", "decided", "nowhere")
	var sum [4]int
	total := 0
	for _, kind := range kinds {
		t, ok := tally[kind]
		if !ok {
			continue
		}
		n := len(fields[kind])
		fmt.Printf("  %-22s %7d %8d %6d %8d %8d\n", kind, n, t[isWritten], t[isShown], t[isDecided], t[isNowhere])
		for i := range sum {
			sum[i] += t[i]
		}
		total += n
	}
	fmt.Printf("  %-22s %7d %8d %6d %8d %8d\n", "every kind `add` writes", total, sum[isWritten], sum[isShown], sum[isDecided], sum[isNowhere])
	fmt.Printf("\nfrom %d `add` run(s). Kinds `add` never writes: %d decided against, %d not.\n",
		runs, len(kindsDecided), len(kindsNowhere))

	approach, err := os.ReadFile(filepath.Join(root, "APPROACH.md"))
	if err != nil {
		return err
	}
	if !specKeyClaim.Match(approach) {
		fmt.Println("\nAPPROACH.md no longer says the chart half is the least finished, so this")
		fmt.Println("measures nothing. Either the claim is back, or this check goes with it.")
		return errFailed
	}
	if sum[isWritten] == total {
		fmt.Println("\n`add` now writes every field of every kind it generates. APPROACH.md still")
		fmt.Println("says the chart half is the least finished, and that is what is wrong now -")
		fmt.Println("rewrite the paragraph rather than this check.")
		return errFailed
	}
	fmt.Println("\nAPPROACH.md states that as a judgement and carries no number. `--missing`")
	fmt.Println("lists what is owed: the fields no template writes or names and no document")
	fmt.Println("decides against. `--shown` lists the skeletons.")
	return nil
}
