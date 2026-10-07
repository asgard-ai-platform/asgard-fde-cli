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
)

// The constraint keys internal/gate pins. A property carrying any of them is
// one the table may have an entry for.
var constraintKeys = []string{"pattern", "minLength", "maxLength", "minimum", "maximum", "minItems", "maxItems"}

func init() {
	register("tables", check{
		Needs: "$ASGARD_KUBE",
		What:  "the pinned enum and constraint tables against the CRDs; **every CEL-rule count this repository states** - 231 enforced against 79 markers, two numbers easy to write for each other; **every immutable field** - that the page names all eleven class fields and states the Syncer's count and the total; and **every required field of a per-class block**, which is what an FDE asks a customer for",
		Run:   runTables,
	})
}

// runTables holds internal/gate's pinned copies of the platform contract, and
// every count this repository states about it, against the generated CRDs.
//
// **The Go types are not the contract; the generated CRDs are**, and the two
// are not the same document - `status` carries three values in the Asgard types
// and six in the CRD, because Kubernetes' own condition schema uses that field
// name, and the wrong three sat in the enum table for a day. Everything here
// reads `crd/`.
func runTables(args []string) error {
	dir := ""
	if len(args) > 0 {
		dir = args[0]
	} else {
		kube, err := src.Resolve("kube")
		if err != nil {
			return err
		}
		dir = filepath.Join(kube, "crd")
	}
	crds, err := loadCRDs(dir)
	if err != nil {
		return err
	}
	root, err := src.Root()
	if err != nil {
		return err
	}

	enums := map[string]map[string]bool{}
	cons := map[string]map[string]bool{}
	for _, c := range crds {
		walkProps(c.Spec.Versions[0].Schema.OpenAPIV3Schema, "", func(path string, schema map[string]any) {
			name := leaf(path)
			if raw, ok := schema["enum"].([]any); ok {
				if enums[name] == nil {
					enums[name] = map[string]bool{}
				}
				for _, v := range raw {
					enums[name][fmt.Sprint(v)] = true
				}
			}
			shape := map[string]any{}
			for _, k := range constraintKeys {
				if v, ok := schema[k]; ok {
					shape[k] = v
				}
			}
			if len(shape) > 0 {
				b, _ := json.Marshal(shape)
				if cons[name] == nil {
					cons[name] = map[string]bool{}
				}
				cons[name][string(b)] = true
			}
		})
	}

	var problems []string

	// The enum table: every value, both directions.
	mine, err := pinnedEnums(root)
	if err != nil {
		return err
	}
	for _, name := range sortedKeys(mine) {
		got, ok := enums[name]
		if !ok {
			problems = append(problems, fmt.Sprintf("enum %s: not an enum in any CRD", name))
			continue
		}
		if !sameSet(mine[name], got) {
			problems = append(problems, fmt.Sprintf("enum %s: table %v != CRD %v",
				name, setList(mine[name]), setList(got)))
		}
	}

	// The constraint table: a field with more than one shape upstream cannot
	// have one entry here, whatever that entry says.
	fields, err := pinnedConstraints(root)
	if err != nil {
		return err
	}
	for _, name := range fields {
		shapes, ok := cons[name]
		if !ok {
			problems = append(problems, fmt.Sprintf("constraint %s: no CRD property constrains it", name))
			continue
		}
		if len(shapes) > 1 {
			problems = append(problems, fmt.Sprintf(
				"constraint %s: %d different constraint sets in the CRDs, so one table entry cannot be right",
				name, len(shapes)))
		}
	}

	problems = append(problems, checkImmutable(root, crds)...)
	problems = append(problems, checkRequiredBlocks(root, crds)...)
	problems = append(problems, checkCEL(root, crds, dir)...)
	problems = append(problems, checkStatusKinds(root, crds)...)
	problems = append(problems, checkDisplayAnnotations(root)...)

	for _, p := range problems {
		fmt.Printf("  %s\n", p)
	}
	fmt.Printf("\n%d enum field(s), %d constrained field(s), %d disagreement(s)\n",
		len(mine), len(fields), len(problems))
	if len(problems) > 0 {
		return errFailed
	}
	return nil
}

var enumRow = regexp.MustCompile(`(?m)^\t"(\w+)":\s*\{([^}]*)\}`)
var constraintRow = regexp.MustCompile(`(?m)^\t"(\w+)":`)

func pinnedEnums(root string) (map[string]map[string]bool, error) {
	data, err := os.ReadFile(filepath.Join(root, "internal/gate/enums.go"))
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]bool{}
	for _, m := range enumRow.FindAllStringSubmatch(string(data), -1) {
		values := map[string]bool{}
		for _, v := range strings.Split(m[2], ",") {
			v = strings.TrimSpace(strings.Trim(strings.TrimSpace(v), `"`))
			if v != "" {
				values[v] = true
			}
		}
		out[m[1]] = values
	}
	return out, nil
}

func pinnedConstraints(root string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(root, "internal/gate/constraints.go"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, m := range constraintRow.FindAllStringSubmatch(string(data), -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out, nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func setList(m map[string]bool) []string { return sortedKeys(m) }

func sameSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}
