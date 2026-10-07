package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/asgard-ai-platform/asgard-fde-cli/hack/internal/src"
	"gopkg.in/yaml.v3"
)

func init() {
	register("extract-crs", check{
		Needs: "this repository",
		What:  "pull the CR skeletons out of the extracts, as documents a schema check can read",
		Run:   runExtractCRs,
	})
	register("validate-crs", check{
		Needs: "$ASGARD_KUBE",
		What:  "validate CR documents against the CRDs - required fields, pruned fields, enums, patterns, maxItems and the ExactlyOneOf rules. With no argument it checks the extracts' own skeletons, which are what somebody copies by hand",
		Run:   runValidateCRs,
	})
}

// Two sentinels mean "not visible from an extract": one for a value the chart
// supplies, one for a value the reader fills in. The validator knows them and
// reports no pattern or enum failure against either, because what the chart
// really supplies cannot be seen from the extract.
const (
	helmSentinel  = "__HELM__"
	placeSentinel = "__PLACEHOLDER__"
)

var helmAction = regexp.MustCompile(`\{\{.*?\}\}`)
var placeholder = regexp.MustCompile(`(?:^|[^|])(<[^<>\n]*>)`)
var yamlFence = regexp.MustCompile("(?s)```ya?ml\n(.*?)```")

// defuse turns a chart fragment written for a person to copy into something a
// parser can read. The skeletons carry Helm actions and <placeholder> text, and
// neither is YAML.
func defuse(block string) string {
	var out []string
	// **Each injected entry gets its own key.** Two consecutive injecting
	// actions used to become two `helmInjected` keys in one map, which a
	// tolerant parser accepts and a strict one refuses - the tolerant one was
	// hiding a duplicate that meant nothing either way.
	injected := 0
	for _, line := range strings.Split(block, "\n") {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(s, "{{") && strings.HasSuffix(s, "}}") {
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			control := false
			for _, p := range []string{"{{- if", "{{ if", "{{- end", "{{ end",
				"{{- range", "{{ range", "{{- with", "{{ with"} {
				if strings.HasPrefix(s, p) {
					control = true
				}
			}
			// A control action becomes a comment; an injecting one becomes a map
			// entry, which is enough for the parser and required by nothing.
			if control {
				out = append(out, indent+"# "+s)
			} else {
				injected++
				out = append(out, fmt.Sprintf(`%shelmInjected%d: "x"`, indent, injected))
			}
			continue
		}
		line = helmAction.ReplaceAllString(line, helmSentinel)
		line = placeholder.ReplaceAllStringFunc(line, func(m string) string {
			if strings.HasPrefix(m, "<") {
				return placeSentinel
			}
			return m[:1] + placeSentinel
		})
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func runExtractCRs(args []string) error {
	out := ".out/extracts.ndjson"
	if len(args) > 0 {
		out = args[0]
	}
	root, err := src.Root()
	if err != nil {
		return err
	}
	paths, err := filepath.Glob(filepath.Join(root, "internal/corpus/usecase/*.md"))
	if err != nil {
		return err
	}
	sort.Strings(paths)

	type found struct {
		from string
		doc  map[string]any
	}
	var docs []found
	blocks, bad := 0, 0
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		for _, m := range yamlFence.FindAllStringSubmatch(string(data), -1) {
			if !strings.Contains(m[1], "apiVersion: asgard-ai.com") {
				continue
			}
			blocks++
			dec := yaml.NewDecoder(strings.NewReader(defuse(m[1])))
			failed := false
			for {
				var doc map[string]any
				if err := dec.Decode(&doc); err != nil {
					if err.Error() != "EOF" {
						fmt.Fprintf(os.Stderr, "PARSE FAIL %s: %s\n", rel, trim(err.Error(), 100))
						failed = true
					}
					break
				}
				if doc == nil {
					continue
				}
				if v, _ := doc["apiVersion"].(string); strings.HasPrefix(v, "asgard-ai.com") {
					docs = append(docs, found{rel, normaliseYAML(doc)})
				}
			}
			if failed {
				bad++
			}
		}
	}
	fmt.Fprintf(os.Stderr, "blocks: %d  documents: %d  unparseable: %d\n", blocks, len(docs), bad)

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	srcf, err := os.Create(out + ".src")
	if err != nil {
		return err
	}
	defer srcf.Close()
	for _, d := range docs {
		line, err := json.Marshal(d.doc)
		if err != nil {
			return err
		}
		fmt.Fprintln(f, string(line))
		meta, _ := d.doc["metadata"].(map[string]any)
		fmt.Fprintf(srcf, "%v/%v\t%s\n", d.doc["kind"], meta["name"], d.from)
	}
	if bad > 0 {
		return errFailed
	}
	return nil
}

// normaliseYAML turns yaml.v3's map[string]any tree into one json.Marshal can
// write - it already uses string keys, but nested maps decoded from an `any`
// come back as map[string]any only at the top level.
func normaliseYAML(v any) map[string]any {
	out, _ := normalise(v).(map[string]any)
	return out
}

func normalise(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = normalise(e)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[fmt.Sprint(k)] = normalise(e)
		}
		return out
	case []any:
		for i, e := range t {
			t[i] = normalise(e)
		}
		return t
	default:
		return v
	}
}

var exactlyOne = regexp.MustCompile(`exactly one of the fields in \[([^\]]+)\]`)

func isSentinel(v any) bool {
	s, ok := v.(string)
	return ok && (strings.Contains(s, helmSentinel) || strings.Contains(s, placeSentinel))
}

// celExactlyOne reports an ExactlyOneOf rule the document breaks. The rule's
// own message names the fields, which is the only place they are listed.
func celExactlyOne(node map[string]any, obj any, path string, errs *[]string) {
	rules, _ := node["x-kubernetes-validations"].([]any)
	m, isMap := obj.(map[string]any)
	if !isMap {
		return
	}
	for _, r := range rules {
		rm, ok := r.(map[string]any)
		if !ok {
			continue
		}
		msg := fmt.Sprint(rm["message"])
		g := exactlyOne.FindStringSubmatch(msg)
		if g == nil {
			continue
		}
		var present []string
		for _, n := range strings.Fields(g[1]) {
			if _, has := m[n]; has {
				present = append(present, n)
			}
		}
		if len(present) != 1 {
			shown := "none"
			if len(present) > 0 {
				shown = "['" + strings.Join(present, "', '") + "']"
			}
			*errs = append(*errs, fmt.Sprintf("%s: %s (present: %s)", path, msg, shown))
		}
	}
}

func checkSchema(node map[string]any, obj any, path string, errs *[]string) {
	if node == nil {
		return
	}
	switch fmt.Sprint(node["type"]) {
	case "object":
		m, ok := obj.(map[string]any)
		if !ok {
			return
		}
		celExactlyOne(node, obj, path, errs)
		props, _ := node["properties"].(map[string]any)
		required, _ := node["required"].([]any)
		for _, r := range required {
			if _, has := m[fmt.Sprint(r)]; !has {
				*errs = append(*errs, fmt.Sprintf("%s.%v: REQUIRED but missing", path, r))
			}
		}
		ap := node["additionalProperties"]
		apMap, apIsMap := ap.(map[string]any)
		if len(props) > 0 {
			for _, k := range sortedAnyKeys(m) {
				child, has := props[k].(map[string]any)
				switch {
				case has:
					checkSchema(child, m[k], path+"."+k, errs)
				case apIsMap:
					checkSchema(apMap, m[k], path+"."+k, errs)
				case ap != true:
					*errs = append(*errs, fmt.Sprintf(
						"%s.%s: UNKNOWN field (pruned silently by the apiserver)", path, k))
				}
			}
		} else if apIsMap {
			for _, k := range sortedAnyKeys(m) {
				checkSchema(apMap, m[k], path+"."+k, errs)
			}
		}
	case "array":
		list, ok := obj.([]any)
		if !ok {
			return
		}
		celExactlyOne(node, obj, path, errs)
		items, _ := node["items"].(map[string]any)
		if mi, ok := node["maxItems"]; ok {
			if n := atoi(fmt.Sprint(mi)); n > 0 && len(list) > n {
				*errs = append(*errs, fmt.Sprintf("%s: %d items exceeds maxItems=%d", path, len(list), n))
			}
		}
		for i, v := range list {
			checkSchema(items, v, fmt.Sprintf("%s[%d]", path, i), errs)
		}
	default:
		if obj == nil || isSentinel(obj) {
			return
		}
		if raw, ok := node["enum"].([]any); ok {
			found := false
			for _, e := range raw {
				if fmt.Sprint(e) == fmt.Sprint(obj) {
					found = true
				}
			}
			if !found {
				*errs = append(*errs, fmt.Sprintf("%s: %s not in enum %v", path, pyRepr(obj), raw))
			}
		}
		if p, ok := node["pattern"].(string); ok {
			if s, ok := obj.(string); ok {
				re, err := regexp.Compile(p)
				if err == nil && !re.MatchString(s) {
					*errs = append(*errs, fmt.Sprintf("%s: %s fails pattern %s", path, pyRepr(obj), p))
				}
			}
		}
		if ml, ok := node["minLength"]; ok {
			if s, ok := obj.(string); ok {
				if n := atoi(fmt.Sprint(ml)); n > 0 && len(s) < n {
					*errs = append(*errs, fmt.Sprintf("%s: shorter than minLength=%d", path, n))
				}
			}
		}
	}
}

func sortedAnyKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// runValidateCRs checks documents against the CRDs.
//
// **Pull asgard-kube first** - validating against an old clone proves nothing.
// None of what this checks is done by `helm lint`, by `asgard-cli check`, or by
// a server-side dry-run: a dry-run is worse than silent, because it drops a
// field it does not recognise and reports success.
func runValidateCRs(args []string) error {
	// **With no argument, check the extracts.** They are what somebody copies by
	// hand, so they are the case worth defaulting to - and `pass` lists this as
	// a step, which a usage error would make unrunnable as listed. Pass an
	// ndjson path to check a rendered chart instead.
	docPath := ""
	if len(args) > 0 {
		docPath = args[0]
	} else {
		root, err := src.Root()
		if err != nil {
			return err
		}
		docPath = filepath.Join(root, ".out/extracts.ndjson")
		if err := runExtractCRs([]string{docPath}); err != nil {
			return err
		}
	}
	crdDir := ""
	if len(args) > 1 {
		crdDir = args[1]
	} else {
		kube, err := src.Resolve("kube")
		if err != nil {
			return err
		}
		crdDir = filepath.Join(kube, "crd")
	}
	crds, err := loadCRDs(crdDir)
	if err != nil {
		return err
	}
	schemas := map[string]map[string]any{}
	for _, c := range crds {
		schemas[c.Spec.Names.Kind] = c.Spec.Versions[0].Schema.OpenAPIV3Schema
	}

	data, err := os.ReadFile(docPath)
	if err != nil {
		return err
	}
	total := 0
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(line), &doc); err != nil {
			return fmt.Errorf("%s: %w", docPath, err)
		}
		kind := fmt.Sprint(doc["kind"])
		meta, _ := doc["metadata"].(map[string]any)
		name := fmt.Sprint(meta["name"])
		schema, ok := schemas[kind]
		if !ok {
			fmt.Printf("\n### %s/%s: NO CRD for this kind\n", kind, name)
			continue
		}
		var errs []string
		props, _ := schema["properties"].(map[string]any)
		spec, hasSpec := doc["spec"]
		if hasSpec {
			node, _ := props["spec"].(map[string]any)
			checkSchema(node, spec, "spec", &errs)
		} else {
			for _, r := range schema["required"].([]any) {
				if fmt.Sprint(r) == "spec" {
					errs = append(errs, "spec: REQUIRED but missing")
				}
			}
		}
		if len(errs) > 0 {
			total += len(errs)
			fmt.Printf("\n### %s/%s\n", kind, name)
			for _, e := range errs {
				fmt.Println("   ", e)
			}
		}
	}
	fmt.Printf("\n==== %d schema violation(s) ====\n", total)
	return nil
}
