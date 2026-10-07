package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/asgard-ai-platform/asgard-fde-cli/hack/internal/src"
)

func init() {
	register("processors", check{
		Needs: "$ASGARD_CORE and $ASGARD_DOCS",
		What:  "**wiki/processors.md's three tables against the two repositories they distil** - outputs, required keys and defaults against asgard-core's ProcessorDefinitions; the palette's author and platform keys against asgard-docs' per-page metadata; that a processor accepting dynamic config says what its keys are for; and that every `processor/<name>` in prose resolves against the frontmatter rather than a file name. --dump prints upstream",
		Run:   runProcessors,
	})
}

const coreFile = "internal/constants.go"
const kubeTypes = "pkg/apis/asgard/v1alpha1/types.go"
const paletteDir = "content-generator/services/developer-reference/docs/processor"

// How many there are. Stated so that a definition appearing or vanishing is a
// failure rather than a quietly shorter table - the page says thirteen in four
// places, and a fourteenth processor is the single most consequential thing
// that can happen to it.
const expectedProcessors = 13

// A `const`/`var` line binding a name to a string literal. The config keys, the
// relation keys and the processor types are all this shape, in two repositories.
var declLine = regexp.MustCompile(`(?m)^\s*(?:var\s+|const\s+)?([A-Za-z0-9_]+)\s+(?:[A-Za-z0-9_.]+\s+)?=\s*"([^"]*)"\s*$`)
var numericLit = regexp.MustCompile(`^(?:int32|int64|float32|float64)\((-?[0-9.]+)\)$`)
var fieldLine = regexp.MustCompile(`^\s*([A-Za-z][A-Za-z0-9_]*):\s*(.*?),?\s*$`)

type procConfig struct {
	Name       string
	Required   bool
	Default    any
	HasDefault bool
}

type procDef struct {
	Type                string
	Configs             []procConfig
	Dynamic             bool
	DynamicRelationship bool
	Relationships       []string
}

// goConstants is every string constant either repository binds, by name.
//
// asgard-core's literal names asgard-kube's processor types through the
// `v1alpha1.` qualifier and its own keys bare, so both spellings are stored.
func goConstants(core, kube string) map[string]string {
	out := map[string]string{}
	for _, s := range []struct{ src, prefix string }{{core, ""}, {kube, "v1alpha1."}} {
		for _, m := range declLine.FindAllStringSubmatch(s.src, -1) {
			out[s.prefix+m[1]] = m[2]
			if s.prefix != "" {
				out[m[1]] = m[2]
			}
		}
	}
	return out
}

// braceLiteral returns the body of a brace-delimited literal, by depth rather
// than by pattern. An earlier pattern-based extraction of this same literal
// attributed one processor's fields to the next.
func braceLiteral(src, decl string) (string, error) {
	at := strings.Index(src, decl)
	if at < 0 {
		return "", fmt.Errorf("%s: %s is not there", coreFile, decl)
	}
	start := strings.Index(src[at:], "{")
	if start < 0 {
		return "", fmt.Errorf("%s: %s has no literal after it", coreFile, decl)
	}
	start += at
	depth := 0
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start+1 : i], nil
			}
		}
	}
	return "", fmt.Errorf("%s: %s is not closed, so this cannot be walked", coreFile, decl)
}

// sliceElements returns the top-level `{...}` elements of a slice literal.
func sliceElements(body string) []string {
	var out []string
	depth, start := 0, 0
	for i, ch := range body {
		switch ch {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 {
				out = append(out, body[start+1:i])
			}
		}
	}
	return out
}

// literalField is the value of a top-level `Name:` field inside one literal.
//
// Depth-tracked, so a `Name:` belonging to a nested `ConfigDefinition` is not
// read as the outer `ProcessorDefinition`'s.
func literalField(text, name string) (string, bool) {
	depth := 0
	for _, line := range strings.Split(text, "\n") {
		if m := fieldLine.FindStringSubmatch(line); depth == 0 && m != nil && m[1] == name {
			return strings.TrimSuffix(m[2], ","), true
		}
		depth += strings.Count(line, "{") + strings.Count(line, "[") -
			strings.Count(line, "}") - strings.Count(line, "]")
	}
	return "", false
}

// resolveValue turns one Go expression into a value, or fails.
//
// An unresolved identifier means the literal has grown a shape this walk does
// not understand, which is exactly when its output must not be trusted.
func resolveValue(raw string, ok bool, consts map[string]string) (any, error) {
	if !ok {
		return nil, nil
	}
	raw = strings.TrimLeft(strings.TrimSuffix(strings.TrimSpace(raw), ","), "&")
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, `"`) {
		return strings.Trim(raw, `"`), nil
	}
	switch raw {
	case "nil":
		return nil, nil
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	if v, has := consts[raw]; has {
		return v, nil
	}
	if m := numericLit.FindStringSubmatch(raw); m != nil {
		if strings.Contains(m[1], ".") {
			f, _ := strconv.ParseFloat(m[1], 64)
			return f, nil
		}
		n, _ := strconv.Atoi(m[1])
		return n, nil
	}
	return nil, fmt.Errorf("%s: cannot resolve %q to a value. The literal has grown a shape "+
		"this walk does not understand; do not trust the page until this is taught it", coreFile, raw)
}

func definitions(coreDir, kubeDir string) ([]procDef, error) {
	coreSrc, err := os.ReadFile(filepath.Join(coreDir, coreFile))
	if err != nil {
		return nil, err
	}
	kubeSrc, err := os.ReadFile(filepath.Join(kubeDir, kubeTypes))
	if err != nil {
		return nil, err
	}
	consts := goConstants(string(coreSrc), string(kubeSrc))
	body, err := braceLiteral(string(coreSrc), "var ProcessorDefinitions = []ProcessorDefinition{")
	if err != nil {
		return nil, err
	}

	var out []procDef
	for _, element := range sliceElements(body) {
		var d procDef
		t, ok := literalField(element, "Type")
		v, err := resolveValue(t, ok, consts)
		if err != nil {
			return nil, err
		}
		d.Type, _ = v.(string)

		if at := strings.Index(element, "StaticConfigs:"); at >= 0 {
			cfgBody, err := braceLiteral(element[at:], "StaticConfigs:")
			if err != nil {
				return nil, err
			}
			for _, one := range sliceElements(cfgBody) {
				name, ok := literalField(one, "Name")
				nv, err := resolveValue(name, ok, consts)
				if err != nil {
					return nil, err
				}
				req, ok := literalField(one, "IsRequired")
				rv, err := resolveValue(req, ok, consts)
				if err != nil {
					return nil, err
				}
				raw, hasDefault := literalField(one, "DefaultValue")
				dv, err := resolveValue(raw, hasDefault, consts)
				if err != nil {
					return nil, err
				}
				s, _ := nv.(string)
				b, _ := rv.(bool)
				d.Configs = append(d.Configs, procConfig{
					Name: s, Required: b, Default: dv,
					HasDefault: hasDefault && strings.TrimSpace(raw) != "nil",
				})
			}
		}
		if m := regexp.MustCompile(`StaticRelationships:\s*\[\]v1alpha1\.RelationKey\{([^}]*)\}`).
			FindStringSubmatch(element); m != nil {
			for _, part := range strings.Split(m[1], ",") {
				if strings.TrimSpace(part) == "" {
					continue
				}
				rv, err := resolveValue(part, true, consts)
				if err != nil {
					return nil, err
				}
				s, _ := rv.(string)
				d.Relationships = append(d.Relationships, s)
			}
		}
		dyn, _ := literalField(element, "AllowDynamicConfig")
		d.Dynamic = dyn == "true"
		dynRel, _ := literalField(element, "AllowDynamicRelationship")
		d.DynamicRelationship = dynRel == "true"
		out = append(out, d)
	}
	return out, nil
}

type paletteEntry struct {
	Page     string
	Author   []string
	Platform []string
	Dynamic  bool
	Present  bool
	Scope    []string
}

// palette is the editor palette per processor type, from asgard-docs' metadata.
//
// Second hand on purpose: asgard-docs reads it out of `asgard-ai-platform-web`,
// which nothing here has a clone of, and records what it found beside each page.
func readPalette(docs string) (map[string][]paletteEntry, error) {
	paths, err := filepath.Glob(filepath.Join(docs, paletteDir, "*/metadata.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	out := map[string][]paletteEntry{}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var meta struct {
			CRDType        string   `json:"crdType"`
			AuthorConfigs  []string `json:"authorConfigs"`
			PlatformSetKey []string `json:"platformSetKeys"`
			EditorPalette  struct {
				Present          bool     `json:"present"`
				WorkflowSetTypes []string `json:"workflowSetTypes"`
				DynamicConfig    bool     `json:"dynamicConfig"`
			} `json:"editorPalette"`
		}
		if err := json.Unmarshal(data, &meta); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if meta.CRDType == "" {
			continue
		}
		out[meta.CRDType] = append(out[meta.CRDType], paletteEntry{
			Page:     filepath.Base(filepath.Dir(p)),
			Author:   meta.AuthorConfigs,
			Platform: meta.PlatformSetKey,
			Dynamic:  meta.EditorPalette.DynamicConfig,
			Present:  meta.EditorPalette.Present,
			Scope:    meta.EditorPalette.WorkflowSetTypes,
		})
	}
	return out, nil
}

// docURLs is every URL path the processor documentation answers at.
//
// **A page's URL is its `slug:` frontmatter, not its file name**, and eight of
// the sixteen processor pages differ. A path written from a file listing 404s,
// which is a mistake this check exists because somebody made - twice, in
// opposite directions, in the same table.
func docURLs(docs string) (map[string]bool, error) {
	paths, err := filepath.Glob(filepath.Join(docs, "docs/developer-reference/processor/*.md*"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	out := map[string]bool{}
	for _, p := range paths {
		base := filepath.Base(p)
		name := base[:strings.LastIndex(base, ".")]
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		head := string(data)
		if len(head) > 1500 {
			head = head[:1500]
		}
		if m := slugFront.FindStringSubmatch(head); m != nil {
			url := strings.TrimPrefix(strings.TrimLeft(strings.TrimSpace(m[1]), "/"), "docs/")
			out[url] = true
		} else {
			out["developer-reference/processor/"+name] = true
		}
	}
	return out, nil
}

// tableRows returns the table under one heading, keyed by the `type` in its
// first cell.
func tableRows(page, heading string) map[string][]string {
	i := strings.Index(page, heading)
	if i < 0 {
		return nil
	}
	out := map[string][]string{}
	firstCell := regexp.MustCompile("^`([a-z-]+)`$")
	for _, line := range strings.Split(page[i:], "\n") {
		if !strings.HasPrefix(line, "|") {
			if len(out) > 0 {
				break
			}
			continue
		}
		var cells []string
		for _, c := range strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|") {
			cells = append(cells, strings.TrimSpace(c))
		}
		if m := firstCell.FindStringSubmatch(cells[0]); m != nil {
			out[m[1]] = cells[1:]
		}
	}
	return out
}

var cellKey = regexp.MustCompile("`([A-Za-z][A-Za-z0-9_.-]*)`")
var cellDefault = regexp.MustCompile("`([A-Za-z][A-Za-z0-9_.]*)`\\s*=(\\S+)")

func cellKeys(cell string) []string {
	var out []string
	for _, m := range cellKey.FindAllStringSubmatch(cell, -1) {
		out = append(out, m[1])
	}
	return out
}

func cellDefaults(cell string) map[string]string {
	out := map[string]string{}
	for _, m := range cellDefault.FindAllStringSubmatch(cell, -1) {
		out[m[1]] = m[2]
	}
	return out
}

func shownDefault(v any) string {
	switch t := v.(type) {
	case bool:
		if t {
			return "true"
		}
		return "false"
	case string:
		if t == "" {
			return `""`
		}
		return t
	case nil:
		return "None"
	default:
		return fmt.Sprint(v)
	}
}

func runProcessors(args []string) error {
	root, err := src.Root()
	if err != nil {
		return err
	}
	core, err := src.Resolve("core")
	if err != nil {
		return err
	}
	kube, err := src.Resolve("kube")
	if err != nil {
		return err
	}
	docs, err := src.Resolve("docs")
	if err != nil {
		return err
	}
	list, err := definitions(core, kube)
	if err != nil {
		return err
	}
	defs := map[string]procDef{}
	for _, d := range list {
		defs[d.Type] = d
	}
	pal, err := readPalette(docs)
	if err != nil {
		return err
	}

	if len(args) > 0 && args[0] == "--dump" {
		for _, name := range sortedKeys(defs) {
			d := defs[name]
			rels := strings.Join(d.Relationships, ",")
			if rels == "" {
				rels = "none"
			}
			fmt.Printf("%s  outputs=%s  dynamic=%s  dynamicRelationship=%s\n",
				name, rels, pyBool(d.Dynamic), pyBool(d.DynamicRelationship))
			for _, c := range d.Configs {
				mark := "optional"
				if c.Required {
					mark = "required"
				}
				shown := ""
				if c.HasDefault {
					shown = " = " + pyRepr(c.Default)
				}
				fmt.Printf("    %-38s %s%s\n", c.Name, mark, shown)
			}
			for _, row := range pal[name] {
				fmt.Printf("    palette(%s): author=%s platform=%s dynamic=%s scope=%s\n",
					row.Page, joinOr(row.Author, "none"), joinOr(row.Platform, "none"),
					pyBool(row.Dynamic), joinCommaOr(row.Scope, "not in the palette"))
			}
		}
		return nil
	}

	data, err := os.ReadFile(filepath.Join(root, "internal/corpus/wiki/processors.md"))
	if err != nil {
		return err
	}
	page := string(data)
	var bad []string
	add := func(format string, a ...any) { bad = append(bad, fmt.Sprintf(format, a...)) }

	if len(defs) != expectedProcessors {
		add("asgard-core declares %d processors and the page is written for %d. "+
			"Every count on that page, and the CRD enum, has to be re-read.",
			len(defs), expectedProcessors)
	}

	// ── the definitions table ────────────────────────────────────────────
	table := tableRows(page, "| processor | outputs | extra keys | required keys, with any default |")
	names := toSet(sortedKeys(defs))
	for n := range table {
		names[n] = true
	}
	for _, name := range sortedKeys(names) {
		cells, inTable := table[name]
		d, inDefs := defs[name]
		switch {
		case !inTable:
			add("%s is in asgard-core and has no row in the definitions table", name)
			continue
		case !inDefs:
			add("the definitions table has a row for %s, which asgard-core does not declare", name)
			continue
		}
		outputs, extra, required := cells[0], cells[1], cells[2]

		want := map[string]string{"success": "Success", "failure": "Failure", "else": "Else"}
		said := map[string]bool{}
		for _, w := range []string{"Success", "Failure", "Else"} {
			if strings.Contains(outputs, w) {
				said[w] = true
			}
		}
		has := map[string]bool{}
		for _, r := range d.Relationships {
			if w, ok := want[r]; ok {
				has[w] = true
			}
		}
		if !sameSet(said, has) {
			add("%s outputs: the page says %s, asgard-core declares %s",
				name, joinPlusOr(sortedKeys(said), "none"), joinPlusOr(sortedKeys(has), "none"))
		}
		if d.Dynamic != strings.Contains(extra, "yes") {
			add("%s extra keys: the page says %s, asgard-core has AllowDynamicConfig=%s",
				name, pyRepr(extra), pyBool(d.Dynamic))
		}

		wantReq, saidReq := map[string]bool{}, toSet(cellKeys(required))
		for _, c := range d.Configs {
			if c.Required {
				wantReq[c.Name] = true
			}
		}
		if !sameSet(wantReq, saidReq) {
			missing := diffSet(wantReq, saidReq)
			extraK := diffSet(saidReq, wantReq)
			msg := ""
			if len(missing) > 0 {
				msg += "omits " + strings.Join(missing, ", ")
			}
			if len(missing) > 0 && len(extraK) > 0 {
				msg += " and "
			}
			if len(extraK) > 0 {
				msg += "claims " + strings.Join(extraK, ", ")
			}
			add("%s required keys: the page %s", name, msg)
		}

		// `=x` says the key is required AND has a default, which is the
		// distinction the page's `=` legend exists for.
		wantDef := map[string]any{}
		for _, c := range d.Configs {
			if c.Required && c.HasDefault {
				wantDef[c.Name] = c.Default
			}
		}
		saidDef := cellDefaults(required)
		for _, k := range sortedKeys(wantDef) {
			shown := shownDefault(wantDef[k])
			got, ok := saidDef[k]
			switch {
			case !ok:
				add("%s `%s` has a default of %s in asgard-core and the page marks none", name, k, shown)
			case strings.Trim(got, `"`) != strings.Trim(shown, `"`):
				add("%s `%s`: the page says =%s, asgard-core says %s", name, k, got, shown)
			}
		}
		for _, k := range sortedKeys(saidDef) {
			if _, ok := wantDef[k]; !ok {
				add("%s `%s`: the page marks a default asgard-core does not give it", name, k)
			}
		}
	}

	// ── the palette table ────────────────────────────────────────────────
	ptable := tableRows(page, "| type | author sets | platform sets | dynamic | scope |")
	for _, name := range sortedKeys(pal) {
		entries := pal[name]
		cells, ok := ptable[name]
		if !ok {
			add("%s is in the asgard-docs palette metadata and has no row in the palette table", name)
			continue
		}
		authorCell, platformCell, dynamicCell, scopeCell := cells[0], cells[1], cells[2], cells[3]

		// One CRD type can own two pages - `push-message` is reached as a bot
		// reply and as an Automation Tool's response - and the palette entry is
		// the same for both. Any of them may answer.
		agree := false
		for _, e := range entries {
			if e.Dynamic == strings.Contains(dynamicCell, "yes") {
				agree = true
			}
		}
		if !agree {
			add("%s dynamic: the palette table says %s, asgard-docs records dynamicConfig=%s",
				name, pyRepr(dynamicCell), pyBool(entries[0].Dynamic))
		}
		for _, e := range entries {
			if !e.Present && !strings.Contains(scopeCell, "not in the palette") {
				add("%s is absent from the palette per asgard-docs, and the page's scope cell says %s",
					name, pyRepr(scopeCell))
			}
			for _, s := range e.Scope {
				if !strings.Contains(scopeCell, s) {
					add("%s scope: asgard-docs says %s, which the page's scope cell %s does not name",
						name, pyRepr(s), pyRepr(scopeCell))
				}
			}
		}

		// Checked per key in one direction: a key asgard-docs calls the
		// platform's must not sit in the author column, because that is the
		// error that gets a chart to write a key it does not own. A row written
		// as a delta off the row above carries no key list of its own.
		if !strings.Contains(authorCell, "the same") {
			saidAuthor, saidPlatform := toSet(cellKeys(authorCell)), toSet(cellKeys(platformCell))
			for _, e := range entries {
				for _, k := range e.Platform {
					if saidAuthor[k] {
						add("%s `%s`: the page has it as the author's, asgard-docs records it as the platform's", name, k)
					}
					if !saidPlatform[k] && platformCell != "-" {
						add("%s `%s`: asgard-docs records it as a platform key and the page's platform cell does not name it", name, k)
					}
				}
				for _, k := range e.Author {
					if !saidAuthor[k] && !strings.Contains(authorCell, "one per branch") && authorCell != "*none*" {
						add("%s `%s`: asgard-docs records it as an author key and the page's author cell does not name it", name, k)
					}
				}
			}
		}
	}

	// ── the documentation paths this page writes ─────────────────────────
	//
	// Written as `processor/<name>` in prose rather than as a link, so
	// `audit-material --urls` never fetches them and nothing else would notice.
	live, err := docURLs(docs)
	if err != nil {
		return err
	}
	for _, m := range regexp.MustCompile("`(processor/[a-z0-9-]+)`").FindAllStringSubmatchIndex(page, -1) {
		named := page[m[2]:m[3]]
		// The one deliberate exception is the bare directory, which the page
		// cites in a sentence saying it 404s.
		if named == "processor" || live["developer-reference/"+named] {
			continue
		}
		line := strings.Count(page[:m[0]], "\n") + 1
		add("internal/corpus/wiki/processors.md:%d writes `%s`, which asgard-docs serves no page at", line, named)
	}

	// ── the extra-key table ──────────────────────────────────────────────
	//
	// What a dynamic key MEANS is not in the definitions - it is in the loop
	// that reads it. So this checks only what the definitions can answer: that
	// every processor accepting dynamic config has a row saying what its keys
	// are for.
	etable := tableRows(page, "| processor | an extra key is | the shape |")
	for _, name := range sortedKeys(defs) {
		if defs[name].Dynamic && etable[name] == nil {
			add("%s accepts dynamic config and has no row saying what an extra key means there", name)
		}
	}
	for _, name := range sortedKeys(etable) {
		if d, ok := defs[name]; ok && !d.Dynamic {
			add("the extra-key table has a row for %s, which asgard-core does not accept dynamic config on", name)
		}
	}

	fmt.Printf("processors: %d definitions, %d rows in the definitions table, %d in the palette table, %d in the extra-key table\n",
		len(defs), len(table), len(ptable), len(etable))
	fmt.Printf("  asgard-core %s, asgard-docs %s, asgard-kube %s\n",
		orQuestion(src.Commit(core)), orQuestion(src.Commit(docs)), orQuestion(src.Commit(kube)))
	for _, b := range bad {
		fmt.Printf("  %s\n", b)
	}
	fmt.Printf("%d disagreement(s)\n", len(bad))
	if len(bad) > 0 {
		return errFailed
	}
	return nil
}

// The dump is compared against the Python it replaces, so it prints the way
// Python does.
func pyBool(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

func pyRepr(v any) string {
	switch t := v.(type) {
	case string:
		return "'" + t + "'"
	case bool:
		return pyBool(t)
	case nil:
		return "None"
	default:
		return fmt.Sprint(v)
	}
}

func joinOr(list []string, empty string) string {
	if len(list) == 0 {
		return empty
	}
	return strings.Join(list, " ")
}

func joinCommaOr(list []string, empty string) string {
	if len(list) == 0 {
		return empty
	}
	return strings.Join(list, ",")
}

func joinPlusOr(list []string, empty string) string {
	if len(list) == 0 {
		return empty
	}
	return strings.Join(list, " + ")
}

func diffSet(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func orQuestion(s string) string {
	if s == "" {
		return "?"
	}
	return s
}
