package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// immutableFields returns every property carrying `self == oldSelf`, by kind.
//
// **The rule count and the property count are not the same number**, because
// one kind can carry the rule at two paths - which is why `wiki/crd-rules.md`
// states one more rule than properties and says so.
func immutableFields(crds []crd) map[string][]string {
	out := map[string][]string{}
	for _, c := range crds {
		seen := map[string]bool{}
		walkProps(c.spec(), "", func(path string, schema map[string]any) {
			rules, ok := schema["x-kubernetes-validations"].([]any)
			if !ok {
				return
			}
			for _, r := range rules {
				m, ok := r.(map[string]any)
				if !ok {
					continue
				}
				if strings.TrimSpace(fmt.Sprint(m["rule"])) == "self == oldSelf" {
					seen[path] = true
				}
			}
		})
		if len(seen) > 0 {
			out[c.Spec.Names.Kind] = sortedKeys(seen)
		}
	}
	return out
}

// checkImmutable holds `wiki/crd-rules.md`'s account of what is chosen once
// against the CRDs.
//
// Nothing offline can say a chart will be refused at apply - that is the
// apiserver's, evaluated on write - but **which fields carry the rule decides a
// plan before anything is applied**, and a field nobody wrote down is one an
// FDE meets after the tag is pushed.
func checkImmutable(root string, crds []crd) []string {
	page, err := os.ReadFile(filepath.Join(root, "internal/corpus/wiki/crd-rules.md"))
	if err != nil {
		return []string{err.Error()}
	}
	text := string(page)
	byKind := immutableFields(crds)

	// **Pairs, not distinct paths.** `bot.botProviderName` is immutable on the
	// Loader and on the Syncer, and those are two fields somebody can be
	// refused on - counting the path once gives fewer than there are.
	pairs, classes := 0, map[string]bool{}
	for _, fields := range byKind {
		pairs += len(fields)
		for _, f := range fields {
			if strings.HasSuffix(f, "Class") {
				classes[f] = true
			}
		}
	}

	var out []string
	for _, want := range []struct {
		re    *regexp.Regexp
		value int
		what  string
	}{
		{regexp.MustCompile(`(\d+) properties across\s*\n?\s*twelve kinds`), pairs, "immutable properties"},
		{regexp.MustCompile(`(?i)the Syncer carries (\d+) of the \d+`), len(byKind["Syncer"]), "Syncer immutable fields"},
		{regexp.MustCompile(`(?i)the Syncer carries \d+ of the (\d+)`), pairs, "immutable properties"},
	} {
		m := want.re.FindStringSubmatch(text)
		if m == nil {
			out = append(out, fmt.Sprintf(
				"internal/corpus/wiki/crd-rules.md states no %s the way this check reads it, so %d is going unchecked",
				want.what, want.value))
			continue
		}
		if atoi(m[1]) != want.value {
			out = append(out, fmt.Sprintf("internal/corpus/wiki/crd-rules.md says %s %s, and the CRDs have %d",
				m[1], want.what, want.value))
		}
	}

	var unnamed []string
	for _, c := range sortedKeys(classes) {
		if !strings.Contains(text, "`"+c+"`") {
			unnamed = append(unnamed, c)
		}
	}
	if len(unnamed) > 0 {
		out = append(out, "crd-rules.md does not name every immutable class field: missing "+strings.Join(unnamed, ", "))
	}
	if len(byKind) != 12 {
		out = append(out, fmt.Sprintf("%d kinds carry an immutable field and the page says twelve", len(byKind)))
	}

	// The Syncer's list is written out in full, so every one of its fields has
	// to be somewhere in the corpus - that is what an FDE reads before
	// believing a Syncer can be edited.
	corpus, err := shipped(root)
	if err != nil {
		return append(out, err.Error())
	}
	for _, f := range byKind["Syncer"] {
		if !named(corpus, f) && !named(corpus, leaf(f)) {
			out = append(out, fmt.Sprintf("Syncer.%s is immutable and no page in the corpus names it", f))
		}
	}
	return out
}

// checkRequiredBlocks reports a required field of a per-class block that no
// page names.
//
// **These are the fields an FDE asks a customer for**, and a block whose second
// field nobody wrote down sends somebody to a meeting with half the ask:
// `BotProvider.spec.telegram` requires `webhookSecretToken` beside `botToken`,
// no documentation page mentions it, and this material listed "the Bot Token".
//
// Only the classed blocks - a spec property named by the class enum - because
// those are what a class chooses between, and a reader has to be told which
// fields come with the class they picked.
func checkRequiredBlocks(root string, crds []crd) []string {
	corpus, err := shipped(root)
	if err != nil {
		return []string{err.Error()}
	}
	var out []string
	for _, c := range crds {
		spec := c.spec()
		props, _ := spec["properties"].(map[string]any)
		if props == nil {
			continue
		}
		var classEnum map[string]bool
		for name, raw := range props {
			if !strings.HasSuffix(name, "Class") {
				continue
			}
			schema, _ := raw.(map[string]any)
			values, _ := schema["enum"].([]any)
			classEnum = map[string]bool{}
			for _, v := range values {
				classEnum[fmt.Sprint(v)] = true
			}
			break
		}
		if classEnum == nil {
			continue
		}
		for _, name := range sortedKeys(props) {
			if !classEnum[name] {
				continue
			}
			block, _ := props[name].(map[string]any)
			if block == nil || fmt.Sprint(block["type"]) != "object" {
				continue
			}
			required, _ := block["required"].([]any)
			for _, r := range required {
				field := fmt.Sprint(r)
				// **On a word boundary.** A plain substring test passes on
				// `region` because some page says "regional", which is a false
				// pass in a check whose whole job is to notice an absence.
				if !named(corpus, field) {
					out = append(out, fmt.Sprintf("%s.spec.%s.%s is required and no page in the corpus names it",
						c.Spec.Names.Kind, name, field))
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// checkDisplayAnnotations holds every place that lists the required display
// annotations against the one list the binary enforces.
//
// **`asgard-cli verify` fails a CR without one, and the list is in the gate.**
// The table shipped into every customer repository named eleven kinds and the
// gate enforces fifteen, so four kinds - KnowledgeBase, Loader, CompletionModel
// and Plugin - could fail a gate the repository's own AGENTS.md said nothing
// about. That is a hand-written copy of a list a program owns, which is the
// shape this repository has removed four times already.
func checkDisplayAnnotations(root string) []string {
	src, err := os.ReadFile(filepath.Join(root, "internal/gate/xref.go"))
	if err != nil {
		return []string{err.Error()}
	}
	block := regexp.MustCompile(`(?s)displayNameAnnotation = map\[string\]string\{(.*?)\n\}`).FindStringSubmatch(string(src))
	if block == nil {
		return []string{"internal/gate/xref.go no longer declares displayNameAnnotation, so nothing says which annotations are enforced"}
	}
	var keys []string
	for _, m := range regexp.MustCompile(`"[A-Za-z]+":\s*"([a-z-]+)"`).FindAllStringSubmatch(block[1], -1) {
		keys = append(keys, m[1])
	}
	sort.Strings(keys)

	// Where a reader meets the list. Each states it in its own form - a table
	// in the scaffolded AGENTS.md, a sentence in the extract - so what is held
	// is the set of keys named, not the wording.
	files := []string{
		"internal/corpus/usecase/conventions.md",
		"internal/scaffold/templates/AGENTS.md.tmpl",
	}
	var out []string
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			out = append(out, err.Error())
			continue
		}
		text := string(data)
		var missing []string
		for _, k := range keys {
			if !strings.Contains(text, k) {
				missing = append(missing, k)
			}
		}
		if len(missing) > 0 {
			out = append(out, fmt.Sprintf("%s lists the required display annotations and never names %s, which `verify` fails a CR for",
				f, strings.Join(missing, ", ")))
		}
	}
	return out
}

// checkStatusKinds holds the statusless-kind claim against the CRDs.
//
// **`pipeline manifest --status` turns on which kinds have a status to give**,
// and the help screen is the only place that list exists: it tells a reader
// whether an empty status block is the schema or a reconciler that has not run.
// It said half the kinds had none and named Agent, SemanticLayer, Plugin,
// SkillSet and SandboxBlueprint among them; all five declare one, and seven of
// the twenty-four kinds actually do not.
//
// A hand-written list of kinds is the shape that drifts - so it is held here,
// by name and by count, against the schemas themselves.
//
// **The inverse is not checked, and that is deliberate.** A paragraph naming
// kinds on both sides - "the seven are X, Y and Z; everything else has one,
// Agent and SemanticLayer included" - reads perfectly and cannot be told from
// the wrong version by any pattern over prose. A first version of this check
// reported exactly that correct sentence. What catches the original defect is
// the shape of the claim instead: it said "half the kinds", no pattern matched
// it, and the last branch below fails a claim written in a form nothing can
// check.
func checkStatusKinds(root string, crds []crd) []string {
	var without []string
	for _, c := range crds {
		props, _ := c.Spec.Versions[0].Schema.OpenAPIV3Schema["properties"].(map[string]any)
		if _, ok := props["status"]; !ok {
			without = append(without, c.Spec.Names.Kind)
		}
	}
	sort.Strings(without)

	bodies, err := celClaimFiles(root)
	if err != nil {
		return []string{err.Error()}
	}
	ws := func(p string) *regexp.Regexp { return regexp.MustCompile(strings.ReplaceAll(p, " ", `\s+`)) }
	claim := ws(`([\w-]+) of the ([\w-]+) kinds declare no status at all`)
	// The same number stated on its own, as provenance for a reading: "held
	// against the 24 kinds in asgard-kube". A kind added upstream makes it
	// wrong, and nothing was looking at it.
	total := ws(`the ([\w-]+) kinds in asgard-kube`)

	var out []string
	seen := 0
	for _, b := range bodies {
		for _, m := range total.FindAllStringSubmatch(b.text, -1) {
			if numberWord(m[1]) != len(crds) {
				out = append(out, fmt.Sprintf("%s says asgard-kube has %s kinds, and it has %d",
					b.name, m[1], len(crds)))
			}
		}
		for _, loc := range claim.FindAllStringIndex(b.text, -1) {
			m := claim.FindStringSubmatch(b.text[loc[0]:loc[1]])
			seen++
			// **The paragraph, not the file.** A whole-file search passes on a
			// kind the list dropped whenever any other sentence happens to
			// name it - and the paragraph after this one names DataConnector,
			// so removing it from the list changed nothing.
			para := b.text[loc[0]:]
			if i := strings.Index(para, "\n\n"); i >= 0 {
				para = para[:i]
			}
			if numberWord(m[1]) != len(without) {
				out = append(out, fmt.Sprintf("%s says %s kinds declare no status, and the CRDs have %d: %s",
					b.name, m[1], len(without), strings.Join(without, ", ")))
			}
			if numberWord(m[2]) != len(crds) {
				out = append(out, fmt.Sprintf("%s says there are %s kinds, and asgard-kube has %d",
					b.name, m[2], len(crds)))
			}
			// **Every kind it names has to be one of them.** The count being
			// right is not the claim a reader acts on; the names are.
			for _, kind := range without {
				if !strings.Contains(para, kind) {
					out = append(out, fmt.Sprintf("%s states the statusless kinds and does not name %s, which declares none",
						b.name, kind))
				}
			}
		}
	}
	if seen == 0 {
		out = append(out, fmt.Sprintf(
			"no statusless-kind claim matches, so %d kind(s) without a status schema are going unchecked",
			len(without)))
	}
	return out
}

// numberWord reads the small number words this material writes out, and digits.
func numberWord(s string) int {
	words := map[string]int{
		"zero": 0, "one": 1, "two": 2, "three": 3, "four": 4, "five": 5,
		"six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10,
		"eleven": 11, "twelve": 12, "thirteen": 13, "fourteen": 14,
		"fifteen": 15, "sixteen": 16, "seventeen": 17, "eighteen": 18,
		"nineteen": 19, "twenty": 20, "twenty-one": 21, "twenty-two": 22,
		"twenty-three": 23, "twenty-four": 24, "twenty-five": 25,
	}
	if n, ok := words[strings.ToLower(s)]; ok {
		return n
	}
	return atoi(s)
}

// checkCEL holds every CEL-rule count this repository states.
//
// **Two numbers that are easy to write for each other**: 79 is the
// `XValidation` markers in asgard-kube's Go types, 231 is what the generator
// emits from them, because one marker on a struct several kinds embed lands in
// every CRD that embeds it. This material had the marker count written down as
// the CRDs' own, in the one page whose subject is what the CRDs enforce.
func checkCEL(root string, crds []crd, crdDir string) []string {
	types := filepath.Join(filepath.Dir(crdDir), "pkg/apis/asgard/v1alpha1/types.go")
	data, err := os.ReadFile(types)
	if err != nil {
		return []string{fmt.Sprintf("  (the CEL counts need the Go types beside %s; skipped)", crdDir)}
	}
	src := string(data)

	counts := map[string]int{
		"markers":         len(regexp.MustCompile(`XValidation:rule=`).FindAllString(src, -1)),
		"oldself_markers": len(regexp.MustCompile("XValidation:rule=`?\"?self == oldSelf").FindAllString(src, -1)),
	}
	distinct := map[string]bool{}
	for _, c := range crds {
		// **Every node, not only the named properties.** A rule can sit on the
		// object itself rather than on one of its fields - ToolsetSpec's three
		// are the shape - and counting only properties said 219 where the
		// generator emits 231.
		walkAll(c.Spec.Versions[0].Schema.OpenAPIV3Schema, func(node map[string]any) {
			rules, ok := node["x-kubernetes-validations"].([]any)
			if !ok {
				return
			}
			for _, r := range rules {
				m, ok := r.(map[string]any)
				if !ok {
					continue
				}
				text := strings.TrimSpace(fmt.Sprint(m["rule"]))
				counts["rules"]++
				distinct[text] = true
				if text == "self == oldSelf" {
					counts["oldself_rules"]++
				}
			}
		})
	}
	counts["distinct"] = len(distinct)

	// **Whitespace-tolerant, because these documents are hard-wrapped.** A
	// pattern written with a literal space stops matching the moment a rewrap
	// puts a newline inside the phrase, and a claim that matches nothing is
	// reported only when NO claim anywhere matches - so one file drifting out
	// is silent. APPROACH.md was.
	ws := func(p string) *regexp.Regexp { return regexp.MustCompile(strings.ReplaceAll(p, " ", `\s+`)) }
	claims := []struct {
		re    *regexp.Regexp
		names []string
	}{
		{ws(`(\d+) CEL rules written and (\d+) enforced`), []string{"markers", "rules"}},
		{ws(`(\d+) of the CRDs' (\d+) enforced CEL rules`), []string{"oldself_rules", "rules"}},
		{ws(`(\d+) of the enforced rules are exactly ` + "`" + `self == oldSelf` + "`"), []string{"oldself_rules"}},
		{ws(`(\d+) of the (\d+) ` + "`" + `XValidation` + "`" + ` markers`), []string{"oldself_markers", "markers"}},
		{ws(`(\d+) rule instances, (\d+) of them distinct`), []string{"rules", "distinct"}},
	}

	bodies, err := celClaimFiles(root)
	if err != nil {
		return []string{err.Error()}
	}
	var out []string
	seen := 0
	for _, claim := range claims {
		for _, b := range bodies {
			for _, m := range claim.re.FindAllStringSubmatch(b.text, -1) {
				seen++
				for i, name := range claim.names {
					if atoi(m[i+1]) != counts[name] {
						out = append(out, fmt.Sprintf("%s says %s for %s, and asgard-kube has %d",
							b.name, m[i+1], name, counts[name]))
					}
				}
			}
		}
	}
	if seen == 0 {
		out = append(out, fmt.Sprintf(
			"no CEL-rule claim matches any pattern, so %d enforced rules and %d markers are going unchecked",
			counts["rules"], counts["markers"]))
	}
	return out
}

type body struct{ name, text string }

func celClaimFiles(root string) ([]body, error) {
	var out []body
	// **Everything that states one.** APPROACH.md was not in this list and
	// carried the marker count as the CRDs' own for as long as the page whose
	// subject it is did; `internal/cli` was not in it either, and `verify`'s
	// own help screen - which is where an FDE meets these numbers - called 79
	// markers "79 rules", the exact confusion this check exists for.
	for _, pattern := range []string{"internal/corpus/*/*.md", "internal/gate/*.go",
		"internal/cli/*.go", "TASK.md", "AGENTS.md", "APPROACH.md"} {
		paths, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil {
			return nil, err
		}
		sort.Strings(paths)
		for _, p := range paths {
			data, err := os.ReadFile(p)
			if err != nil {
				return nil, err
			}
			rel, _ := filepath.Rel(root, p)
			out = append(out, body{rel, unquoteBackticks(string(data))})
		}
	}
	return out, nil
}

// unquoteBackticks turns Go's way of putting a backtick inside a raw string
// back into the backtick a reader sees.
//
// **A help screen is prose, and this check reads it as source.** A claim
// written as a sentence quoting a field name is stored as a raw string broken
// around a concatenated backtick, so a pattern looking for the rendered
// sentence
// matches nothing - and a claim that matches nothing is reported only when NO
// claim anywhere matches. The first version of this check read `internal/cli`
// and still could not see the one wrong number in it.
func unquoteBackticks(s string) string {
	return goTick.ReplaceAllString(s, "\x60")
}

// goTick matches the two spellings, spaced and not. Written with \x60 rather
// than the character itself so that this file does not contain the sequence it
// is looking for.
var goTick = regexp.MustCompile("\x60\\s*\\+\\s*\"\x60\"\\s*\\+\\s*\x60")

// shipped is everything that reaches a reader: the corpus, what the generator
// writes, the scaffolded templates, and the Go-held bodies.
//
// **Not the prose alone.** A per-class field can be taught by the generator
// that writes it - `athena.outputLocation` is in `internal/generate/dbclass.go`
// and in the db-query skill's connector reference - and reading the wiki alone
// reported ten fields as unnamed that a chart author meets by running `add`.
func shipped(root string) (string, error) {
	var b strings.Builder
	for _, pattern := range []string{
		"internal/corpus/*/*.md", "internal/corpus/*.md",
		"internal/generate/*.go", "internal/generate/templates/*.tmpl",
		"internal/scaffold/templates/**/*.md", "internal/scaffold/templates/*.md",
		"internal/needs/*.go", "internal/brief/*.go", "internal/stage/prompts/*.md",
	} {
		paths, _ := filepath.Glob(filepath.Join(root, pattern))
		for _, p := range paths {
			data, err := os.ReadFile(p)
			if err != nil {
				return "", err
			}
			b.Write(data)
			b.WriteString("\n")
		}
	}
	// The scaffolded skills nest deeper than one glob reaches.
	_ = filepath.Walk(filepath.Join(root, "internal/scaffold/templates"), func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err == nil {
			b.Write(data)
			b.WriteString("\n")
		}
		return nil
	})
	return b.String(), nil
}

var wordCache = map[string]*regexp.Regexp{}

// named reports whether the text names this field on a word boundary.
func named(text, field string) bool {
	re, ok := wordCache[field]
	if !ok {
		re = regexp.MustCompile(`\b` + regexp.QuoteMeta(field) + `\b`)
		wordCache[field] = re
	}
	return re.MatchString(text)
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}
