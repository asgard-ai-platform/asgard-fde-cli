package gate

import (
	"strings"
	"testing"
)

func workflowDoc(ptype string, configs ...map[string]any) Doc {
	list := make([]any, len(configs))
	for i, c := range configs {
		list[i] = c
	}
	return Doc{Kind: "Workflow", Name: "wf-x", Spec: map[string]any{
		"processors": []any{map[string]any{"name": "p", "type": ptype, "configs": list}},
	}}
}

// W4 warns rather than fails: narrowing by table name is legitimate when the
// names line up. The cases that must stay silent are a layer with no
// allowedCubes, an empty allowedTables, and a list built by an expression
// nothing here can evaluate.
func TestW4WarnsOnTableNarrowingOnProcessors(t *testing.T) {
	model := map[string]any{"name": "completionModel", "value": "preset-default"}
	prompt := map[string]any{"name": "prompt", "value": "p"}
	schema := map[string]any{"name": "outputSchema", "value": "{}"}
	q := []map[string]any{
		{"name": "dataConnector", "value": "dc"},
		{"name": "sql", "value": "select 1"},
		{"name": "resultField", "value": "rows"},
	}
	for _, c := range []struct {
		name   string
		doc    Doc
		expect string
	}{
		{"a layer with no allowedCubes", workflowDoc("llm-completion", model, prompt, schema,
			map[string]any{"name": "semanticLayers", "expression": `[{"name": "sl-erp", "allowQuery": true, "allowWrite": false}]`}), ""},
		{"a layer narrowed by allowedCubes", workflowDoc("llm-completion", model, prompt, schema,
			map[string]any{"name": "semanticLayers", "expression": `[{"name": "sl-erp", "allowQuery": true, "allowWrite": false, "allowedCubes": ["orders"]}]`}),
			"W4 Workflow/wf-x processor \"p\" (llm-completion): sets allowedCubes on sl-erp"},
		{"the deprecated single-layer key", workflowDoc("stream-llm-completion-message", model,
			map[string]any{"name": "semanticLayer.allowedCubes", "value": "orders,items"}),
			"sets semanticLayer.allowedCubes"},
		{"a list no JSON parser can read", workflowDoc("llm-completion", model, prompt, schema,
			map[string]any{"name": "semanticLayers", "expression": `layers(ctx)`}), ""},
		{"allowedTables on query-database", workflowDoc("query-database", append(q,
			map[string]any{"name": "allowedTables", "value": "public.orders"})...), "sets allowedTables"},
		{"an empty allowedTables", workflowDoc("query-database", append(q,
			map[string]any{"name": "allowedTables", "value": ""})...), ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			res := Processors([]Doc{c.doc}, Options{})
			if p := strings.Join(res.Problems, "\n"); strings.Contains(p, "W4") {
				t.Errorf("W4 failed the render; it is a warning:\n%s", p)
			}
			got := strings.Join(res.Warnings, "\n")
			if c.expect == "" && strings.Contains(got, "W4") {
				t.Errorf("warned on a processor that narrows nothing:\n%s", got)
			}
			if c.expect != "" && !strings.Contains(got, c.expect) {
				t.Errorf("want %q in:\n%s", c.expect, got)
			}
		})
	}
}
