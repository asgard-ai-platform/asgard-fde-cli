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

// W5 and W6 warn and never fail: validate-payload still runs, and a sql
// template that reads only our own values is correct.
func TestW5AndW6Warn(t *testing.T) {
	q := []map[string]any{
		{"name": "dataConnector", "value": "dc"},
		{"name": "resultField", "value": "rows"},
	}
	for _, c := range []struct {
		name   string
		doc    Doc
		expect string
	}{
		{"a validate-payload node", workflowDoc("validate-payload", map[string]any{"name": "schema", "value": "{}"}), "W5"},
		{"sql built by a template", workflowDoc("query-database", append(q,
			map[string]any{"name": "sql", "template": "select * from t where id = '{{prevPayload.id}}'"})...), "W6"},
		{"sql built by an expression", workflowDoc("query-database", append(q,
			map[string]any{"name": "sql", "expression": "'select * from t where id = ' + prevPayload.id"})...), "W6"},
		{"sql as a value with a bound argument", workflowDoc("query-database", append(q,
			map[string]any{"name": "sql", "value": "select * from t where id = $1"},
			map[string]any{"name": "sql.args.1.type", "value": "string"},
			map[string]any{"name": "sql.args.1.value", "expression": "prevPayload.id"})...), ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			res := Processors([]Doc{c.doc}, Options{})
			for _, p := range res.Problems {
				if strings.HasPrefix(p, "W5") || strings.HasPrefix(p, "W6") {
					t.Errorf("failed the render; it is a warning:\n%s", p)
				}
			}
			got := strings.Join(res.Warnings, "\n")
			if c.expect == "" && (strings.Contains(got, "W5") || strings.Contains(got, "W6")) {
				t.Errorf("warned on correct material:\n%s", got)
			}
			if c.expect != "" && !strings.Contains(got, c.expect) {
				t.Errorf("want %s in:\n%s", c.expect, got)
			}
		})
	}
}

func branchDoc(procs []any, rels ...[3]string) Doc {
	list := make([]any, len(rels))
	for i, r := range rels {
		list[i] = map[string]any{
			"from": map[string]any{"processor": r[0], "relationName": r[1]},
			"to":   map[string]any{"processor": r[2]},
		}
	}
	return Doc{Kind: "Workflow", Name: "wf-x", Spec: map[string]any{"processors": procs, "relationships": list}}
}

func proc(name, ptype string, configs ...map[string]any) map[string]any {
	list := make([]any, len(configs))
	for i, c := range configs {
		list[i] = c
	}
	return map[string]any{"name": name, "type": ptype, "configs": list}
}

// W7 and W8 are the two branch mistakes an Extension's examples shipped. The
// silent cases matter as much: a failure branch off an http-request that reads
// prevError, and a success branch that reads httpResponse, are both right.
func TestBranchRules(t *testing.T) {
	call := proc("call", "http-request",
		map[string]any{"name": "url", "value": "https://example.com"},
		map[string]any{"name": "method", "value": "GET"})
	input := proc("input", "update-context", map[string]any{"name": "id", "expression": "prevPayload.id"})
	readsStatus := proc("err", "push-message", map[string]any{"name": "payload", "expression": "({ status: httpResponse && httpResponse.status })"})
	readsError := proc("err", "push-message", map[string]any{"name": "payload", "expression": "({ error: String(prevError) })"})
	ok := proc("ok", "push-message", map[string]any{"name": "payload", "expression": "httpResponse.json"})
	similar := proc("err", "push-message", map[string]any{"name": "payload", "expression": "({ at: lastHttpResponseAt })"})

	for _, c := range []struct {
		name   string
		doc    Doc
		expect []string
	}{
		{"failure off update-context", branchDoc([]any{input, call, readsError},
			[3]string{"input", "success", "call"}, [3]string{"input", "failure", "err"}, [3]string{"call", "failure", "err"}),
			[]string{`W7 Workflow/wf-x processor "input": a failure relationship leaves an update-context`}},
		{"httpResponse on a failure branch", branchDoc([]any{input, call, readsStatus},
			[3]string{"input", "success", "call"}, [3]string{"call", "failure", "err"}),
			[]string{`W8 Workflow/wf-x processor "err" reads httpResponse on the failure branch of "call"`}},
		{"prevError on the failure branch, httpResponse on success", branchDoc([]any{input, call, ok, readsError},
			[3]string{"input", "success", "call"}, [3]string{"call", "success", "ok"}, [3]string{"call", "failure", "err"}),
			nil},
		{"a word that only contains httpResponse", branchDoc([]any{call, similar},
			[3]string{"call", "failure", "err"}),
			nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := Processors([]Doc{c.doc}, Options{})
			if len(got.Warnings) != len(c.expect) {
				t.Fatalf("warnings = %q, want %d", got.Warnings, len(c.expect))
			}
			for i, want := range c.expect {
				if !strings.HasPrefix(got.Warnings[i], want) {
					t.Errorf("warning %d = %q, want prefix %q", i, got.Warnings[i], want)
				}
			}
			if len(got.Problems) != 0 {
				t.Errorf("problems = %q, want none: both rules warn", got.Problems)
			}
		})
	}
}
