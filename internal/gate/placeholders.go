package gate

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// placeholder is what `asgard-cli add` leaves where a person has to answer.
//
// **It is deliberately a bare word.** The generator writes "TODO" and nothing
// around it, because a field that looks decided but never was is worse than an
// empty one - so the same word is what survives into the render when nobody
// answered.
var placeholder = regexp.MustCompile(`\bTODO\b`)

// reachesAReader names the fields a placeholder is worst in, and says who meets
// it. Everything else is reported as a count; these are named one by one.
//
// **A chart full of TODOs renders, lints, applies and deploys green.** That is
// by design at every other step - the apiserver has no opinion about the string
// "TODO" - so this is the only place between `add` and a tag where anybody says
// they are still there.
var reachesAReader = map[string]map[string]string{
	"Agent": {
		"managed.description":     "the agent reads this to decide when this capability's guidance applies",
		"managed.sampleQuestions": "the customer sees these in Sindri, before anything the agent says",
		"managed.prompt.persona":  "the agent answers in the register its prompt is written in",
		"managed.prompt.task":     "the agent answers in the register its prompt is written in",
	},
	"SemanticLayer": {
		"instruction": "the model reads this to decide what to query",
		"summary":     "how the model chooses BETWEEN layers - the tool that lists them returns the name and this, and nothing else",
	},
	"SkillSet": {
		"searchPaths": "a path the sync never writes loads no skills at all, and nothing else reports it",
	},
}

// **A flow agent has no Agent CR, so its prompt is on a processor** - and that
// prompt is the whole product. A named field cannot find it, because a
// Workflow's prompt is a `configs` entry inside whichever processor happens to
// be the completion one, so it is looked for by walking the graph.
var promptProcessors = map[string]bool{
	"stream-llm-completion-message": true,
	"llm-completion":                true,
}

// namedConfigs are the other processor configs worth naming, and what each
// costs unanswered. `sql` is the one that looks harmless: the generator writes
// `select 1`, the CR applies, the tool answers every question with the same
// row, and nothing else in this gate reads SQL.
var namedConfigs = map[string]map[string]string{
	"query-database": {"sql": "the generator writes `select 1`, which applies cleanly and answers every question with the same row"},
	"http-request":   {"url": "a request to an unanswered URL fails at call time rather than at deploy"},
}

// Placeholders reports the generator's TODOs that are still in the render.
//
// **Warnings, never failures.** A chart is full of them through the whole
// middle of an onboarding, and failing there would leave the gate red for days
// - which trains people to ignore it, the same reasoning Deployability is built
// on. What this is for is the last read before a tag.
func Placeholders(docs []Doc, opts Options) Result {
	if len(docs) == 0 {
		return Result{Summary: "nothing rendered yet"}
	}

	var warnings []string
	total := 0
	byKind := map[string]int{}

	for _, d := range docs {
		n := countPlaceholders(d.Spec) + countPlaceholders(map[string]any{
			"annotations": toAny(d.Annotations),
		})
		if n == 0 {
			continue
		}
		total += n
		byKind[d.Kind] += n

		if d.Kind == "Workflow" {
			for _, p := range workflowPrompts(d) {
				warnings = append(warnings, fmt.Sprintf(
					"Workflow/%s: processor %q has prompt still TODO - the agent reads it on every turn, "+
						"and where the blueprint lists no Agent it is the whole of what the agent is told", d.Name, p))
			}
			for _, f := range workflowConfigs(d) {
				warnings = append(warnings, fmt.Sprintf("Workflow/%s: %s", d.Name, f))
			}
			for _, e := range workflowTooling(d) {
				warnings = append(warnings, fmt.Sprintf(
					"Workflow/%s: entry %q has tooling.description still TODO - the model reads it to "+
						"decide whether to call this tool, and reads nothing else about it", d.Name, e))
			}
		}

		named := reachesAReader[d.Kind]
		var hit []string
		for path, why := range named {
			if placeholder.MatchString(fmt.Sprint(digAny(d.Spec, strings.Split(path, ".")...))) {
				hit = append(hit, fmt.Sprintf("%s (%s)", path, why))
			}
		}
		sort.Strings(hit)
		for _, h := range hit {
			warnings = append(warnings, fmt.Sprintf("%s/%s: %s is still TODO - %s", d.Kind, d.Name, strings.SplitN(h, " (", 2)[0], strings.TrimSuffix(strings.SplitN(h, " (", 2)[1], ")")))
		}
	}

	if total == 0 {
		return Result{Summary: "no generator TODO left in the render"}
	}
	kinds := make([]string, 0, len(byKind))
	for _, k := range sortedKindKeys(byKind) {
		kinds = append(kinds, fmt.Sprintf("%s=%d", k, byKind[k]))
	}
	warnings = append(warnings, fmt.Sprintf(
		"%d TODO(s) remain across the render (%s). `asgard-cli add` writes them where somebody has to answer, "+
			"and nothing between here and a tag mentions them again: helm renders the word, the apiserver accepts it, "+
			"and the run succeeds", total, strings.Join(kinds, ", ")))
	return Result{Warnings: warnings, Summary: fmt.Sprintf("%d TODO(s)", total)}
}

// workflowPrompts names every completion processor whose prompt is still a
// placeholder.
func workflowPrompts(d Doc) []string {
	var out []string
	procs, _ := d.Spec["processors"].([]any)
	for _, p := range procs {
		m, ok := p.(map[string]any)
		if !ok || !promptProcessors[fmt.Sprint(m["type"])] {
			continue
		}
		configs, _ := m["configs"].([]any)
		for _, c := range configs {
			cm, ok := c.(map[string]any)
			if !ok || fmt.Sprint(cm["name"]) != "prompt" {
				continue
			}
			// **All three forms.** A config value is a Literal, an
			// Expression or a Template and the CRD enforces exactly one, so
			// reading `value` alone missed the supervisor's prompt - which is
			// written as a Template and is the whole of what that shape does.
			for _, form := range []string{"value", "expression", "template"} {
				if placeholder.MatchString(fmt.Sprint(cm[form])) {
					out = append(out, fmt.Sprint(m["name"]))
					break
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// workflowConfigs names the other processor configs in namedConfigs that are
// still placeholders.
func workflowConfigs(d Doc) []string {
	var out []string
	procs, _ := d.Spec["processors"].([]any)
	for _, p := range procs {
		m, ok := p.(map[string]any)
		if !ok {
			continue
		}
		named := namedConfigs[fmt.Sprint(m["type"])]
		if named == nil {
			continue
		}
		configs, _ := m["configs"].([]any)
		for _, c := range configs {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			why, wanted := named[fmt.Sprint(cm["name"])]
			if !wanted {
				continue
			}
			for _, form := range []string{"value", "expression", "template"} {
				if placeholder.MatchString(fmt.Sprint(cm[form])) {
					out = append(out, fmt.Sprintf("processor %q has %s still TODO - %s",
						fmt.Sprint(m["name"]), fmt.Sprint(cm["name"]), why))
					break
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// workflowTooling names every entry whose tool description a model would read
// as the word TODO.
func workflowTooling(d Doc) []string {
	var out []string
	entries, _ := d.Spec["entries"].([]any)
	for _, e := range entries {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		tooling, ok := m["tooling"].(map[string]any)
		if !ok {
			continue
		}
		if placeholder.MatchString(fmt.Sprint(tooling["description"])) {
			out = append(out, fmt.Sprint(m["name"]))
		}
	}
	sort.Strings(out)
	return out
}

func sortedKindKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func toAny(m map[string]string) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

// countPlaceholders walks any decoded YAML and counts the strings carrying one.
func countPlaceholders(node any) int {
	switch v := node.(type) {
	case string:
		return len(placeholder.FindAllString(v, -1))
	case map[string]any:
		n := 0
		for _, e := range v {
			n += countPlaceholders(e)
		}
		return n
	case []any:
		n := 0
		for _, e := range v {
			n += countPlaceholders(e)
		}
		return n
	}
	return 0
}

// digAny follows a dotted path and returns whatever is there, list or scalar.
func digAny(m map[string]any, path ...string) any {
	var cur any = m
	for _, p := range path {
		node, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = node[p]
	}
	return cur
}
