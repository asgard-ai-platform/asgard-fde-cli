package gate

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// The processor contract, extracted from `ProcessorDefinitions` in asgard-core
// `internal/constants.go` with a go/ast walk on 2026-09-03, and cross-checked
// against the CRDs. `.agents/skills/asgard-platform/wiki/processors.md` is the
// prose version and carries the same table.
//
// **The read marker names asgard-core, which is where the definitions are.**
// It said asgard-kube, which is what the extraction was checked against and
// not what it came from - so the one upstream whose movement would invalidate
// this table was the one the marker did not name.
//
// Moved to `623ceb50` on 2026-09-11 after seven upstream commits, none of
// which touched the definitions: `ProcessorDefinitions` is lines 1390-2064 of
// `internal/constants.go` and every hunk in that diff starts at 645 or at
// 2097 and beyond. The table's source is byte-identical, so the marker moves
// without the table changing - which is the only case where that is honest.
//
// It is a pinned copy, and it is incomplete: a processor with dynamic config
// takes keys no definition names. Treat it as the set of keys the definitions
// know about, never as the set a chart may use.
const processorDefsRead = "asgard-core 478cf5d6, against asgard-kube 3da0365"

// processorDef holds only what a rule reads. It carried five more fields -
// optional keys, defaults, whether extra keys are allowed, and the declared
// outputs - and they were removed on 2026-09-03 after the outputs proved
// **wrong**: `ProcessorDefinitions` lists no relationships for `listen-message`
// and Success only for `http-request`, while the documentation gives
// `http-request` a Failure branch producing `prevError` and four production
// charts route one. A table that is partly wrong invites the next rule to be
// built on the wrong part, which is exactly what happened here.
type processorDef struct {
	// mustSet is required with no default. Leaving one out is broken against
	// every version of the platform, which is why W2 fails on it.
	mustSet []string
}

var processorDefs = map[string]processorDef{
	"execute-script":                {mustSet: []string{"engine", "script"}},
	"generate-embedding":            {mustSet: []string{"embeddingModel", "input", "resultField"}},
	"http-request":                  {mustSet: []string{"url", "method"}},
	"listen-message":                {mustSet: nil},
	"llm-completion":                {mustSet: []string{"completionModel", "prompt", "outputSchema"}},
	"llm-query-database":            {mustSet: []string{"semanticLayer", "query", "resultField", "completionModel", "maxTokens"}},
	"push-message":                  {mustSet: nil},
	"query-database":                {mustSet: []string{"dataConnector", "sql", "resultField"}},
	"retrieve-knowledge":            {mustSet: []string{"knowledgeBases", "textQuery", "similarityThreshold", "resultField"}},
	"router":                        {mustSet: nil},
	"stream-llm-completion-message": {mustSet: []string{"completionModel"}},
	"update-context":                {mustSet: nil},
	"validate-payload":              {mustSet: nil},
}

// Processors checks every Workflow's processors against the contract above.
//
//	W1  the type is one the CRD enum allows. A warning: the
//	    enum can gain a value, and a chart using a new one is right while this
//	    copy is stale.
//	W2  every required key with no default is set. **This fails.** A required
//	    key with no fallback is broken against every version of the platform,
//	    and the scaffold itself shipped a Workflow whose only LLM processor had
//	    no completionModel - it rendered, it verified, and it could not run.
//
//	W4  a processor that narrows a layer with `allowedCubes`, or a query with
//	    `allowedTables`, is reported. A warning: the field is legitimate where
//	    every query names each table exactly as the allowed list does, and the
//	    point is that it is set on purpose. The platform enforces both by
//	    matching the table names parsed from the SQL against the allowed names
//	    as strings, so a correct query that writes a table without its schema
//	    is refused; the database user's grants are the sturdier control. A
//	    `semanticLayers` list built by an expression this cannot parse is not
//	    read, because what it evaluates to is not in the chart.
//	W5  a `validate-payload` processor is reported. A warning: it still runs,
//	    and it is deprecated - the entry's `inputSchema` and `files` are
//	    checked before any processor and replace it.
//	W6  a `query-database` whose `sql` is an expression or a template is
//	    reported. A warning: building the statement at run time is how request
//	    data ends up in SQL, and a value bound with `sql.args.<n>.*` cannot
//	    change the statement. It is correct when everything the template reads
//	    is ours rather than the caller's, which this cannot tell.
//
// There is no W3, a rule flagging a key the definitions do not declare: the
// definitions are a subset of what the runtime accepts, not the config
// contract, so such a rule would call correct charts wrong.
//
// The counterpart to W2 is what it deliberately does not check: a required key
// that *does* have a default is left alone, because omitting it is legal. That
// is where a layer's `allowWrite` lives, defaulting to true, and no gate can
// tell a deliberate omission from a forgotten one - which is why the templates
// write it out explicitly instead.
func Processors(docs []Doc, opts Options) Result {
	var problems, warnings []string
	errf := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}
	warnf := func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	}

	seen := 0
	for _, d := range docs {
		if d.Kind != "Workflow" {
			continue
		}

		// W3. **`relationships` is what runs the chain**, and a Workflow that
		// declares several processors and wires none of them runs exactly one:
		// the entry's `handlingProcessor`, and then it stops.
		//
		// Nothing else sees it. A Workflow with no relationships is a legal
		// CR, so helm lint, CRD validation, the server-side dry run and every
		// other rule here are green, the run succeeds and the resource reads
		// `ok` - while a tool that was written to call an API reads its
		// arguments into context and returns nothing. An agent handed silence
		// does not report a failure; it answers from memory.
		//
		// `asgard-cli add httptool` generated three processors and no
		// relationships for as long as it existed, and `usecase/workflow-chain.md`
		// told its reader the generators wire them.
		//
		// One processor needs none, which is the Trigger and the single-step
		// tool. A reachability walk would be the stronger rule and is not this
		// one: an unreachable processor can be a branch somebody has not
		// finished, and refusing that would fire on work in progress.
		procs := digList(d.Spec, "processors")
		if len(procs) > 1 && len(digList(d.Spec, "relationships")) == 0 {
			names := make([]string, 0, len(procs))
			for _, raw := range procs {
				if n := digStr(mapOf(raw), "name"); n != "" {
					names = append(names, n)
				}
			}
			errf("W3 Workflow/%s: %d processors (%s) and no relationships, so only the entry's handlingProcessor runs and the rest are dead. "+
				"Nothing downstream reports this - the CR is legal, the run succeeds, and the tool returns nothing",
				d.Name, len(procs), strings.Join(names, ", "))
		}

		for _, raw := range procs {
			p := mapOf(raw)
			pname := digStr(p, "name")
			ptype := digStr(p, "type")
			if ptype == "" {
				continue
			}
			seen++

			def, known := processorDefs[ptype]
			if !known {
				// **The number is counted, not typed.** It is the length of the
				// pinned copy this build carries, and a copy that gains a type
				// while the sentence says the old figure tells the reader the
				// wrong thing about the thing the warning is complaining about.
				warnf("W1 Workflow/%s processor %q: type %q is not one of the %d this build knows (read %s). Either it is a typo, or the platform has added one and this copy is stale",
					d.Name, pname, ptype, len(processorDefs), processorDefsRead)
				continue
			}

			set := map[string]bool{}
			for _, c := range digList(p, "configs") {
				if n := digStr(mapOf(c), "name"); n != "" {
					set[n] = true
				}
			}

			for _, key := range def.mustSet {
				if !set[key] {
					errf("W2 Workflow/%s processor %q (%s): %s is required and has no default, so the processor cannot run without it",
						d.Name, pname, ptype, key)
				}
			}

			if ptype == "validate-payload" {
				warnf("W5 Workflow/%s processor %q: validate-payload is deprecated. Put the schema in the entry's inputSchema and any file requirements in its files; the platform checks both before the first processor runs",
					d.Name, pname)
			}

			for _, raw := range digList(p, "configs") {
				c := mapOf(raw)
				if ptype == "query-database" && digStr(c, "name") == "sql" && (digStr(c, "expression") != "" || digStr(c, "template") != "") {
					warnf("W6 Workflow/%s processor %q: sql is built at run time, by an expression or a template. Request data written into it runs as SQL; write sql as a value with placeholders and bind each value with sql.args.<n>.type and sql.args.<n>.value",
						d.Name, pname)
				}
				if what := narrows(ptype, c); what != "" {
					warnf("W4 Workflow/%s processor %q (%s): %s. The platform matches the table names in the SQL against it as strings, so a correct query that leaves out the schema is refused. Keep it only if every query names each table exactly as listed; otherwise prefer the DataConnector's database user's grants",
						d.Name, pname, ptype, what)
				}
			}

		}

		branchRules(d, procs, warnf)
	}

	sort.Strings(problems)
	sort.Strings(warnings)
	return Result{Problems: problems, Warnings: warnings, Summary: fmt.Sprintf("%d processor(s)", seen)}
}

// httpResponseRef finds the context variable an http-request writes, as a
// word rather than a substring, so `lastHttpResponseAt` does not match.
var httpResponseRef = regexp.MustCompile(`\bhttpResponse\b`)

// branchRules holds a Workflow's failure relationships against the two facts
// about them that the material states and a chart keeps getting wrong.
//
// **Both are warnings, and neither comes from a table of outputs.** The table
// processorDef used to carry was removed because `StaticRelationships` is wrong
// in both directions (see processorDef), and the catalogue `skill update`
// fetches is generated from it, so it still says `http-request` has no Failure.
// What these rules rest on is narrower: the two places where the written
// documentation, the code and production charts agree. A platform that later
// gives `update-context` a Failure makes W7 wrong, which is why it warns.
//
// They were written after an Extension's examples shipped both mistakes in four
// Workflows. A coding agent transcribing those examples into a chart reproduced
// them, `gate` was green, and the agent that caught them did so by reading
// `wiki/processors.md` - which is not a step anybody can rely on.
func branchRules(d Doc, procs []any, warnf func(string, ...any)) {
	types := map[string]string{}
	byName := map[string]map[string]any{}
	for _, raw := range procs {
		p := mapOf(raw)
		name := digStr(p, "name")
		types[name] = digStr(p, "type")
		byName[name] = p
	}

	for _, raw := range digList(d.Spec, "relationships") {
		r := mapOf(raw)
		if digStr(r, "from", "relationName") != "failure" {
			continue
		}
		from := digStr(r, "from", "processor")
		to := digStr(r, "to", "processor")

		switch types[from] {
		case "update-context":
			// W7. `update-context` only ever emits Success: `wiki/processors.md`
			// says it is the one processor documented that way, and the fetched
			// catalogue agrees. The branch is dead, and the design that wanted
			// it - validate the arguments, throw, report the error - does not
			// work either: a processor with no Failure that throws stops the
			// run, and a tool that returns nothing is read by the model as an
			// empty answer.
			warnf("W7 Workflow/%s processor %q: a failure relationship leaves an update-context, which only ever emits success, so it never runs. "+
				"An expression that throws there stops the run and the tool returns nothing. Check the arguments in the entry's inputSchema, which the platform validates before the first processor, and branch with a router",
				d.Name, from)
		case "http-request":
			// W8. A failed http-request does not touch `httpResponse`: it is
			// still the last successful call's, or absent when there was none
			// (`usecase/api-oauth.md`, against asgard-core's http_request.go).
			// A failure branch that reads its status reports the previous
			// call's - in an OAuth chain, the token response's 200.
			if p := byName[to]; p != nil && readsHTTPResponse(p) {
				warnf("W8 Workflow/%s processor %q reads httpResponse on the failure branch of %q. A failed http-request leaves httpResponse as the last successful call's, or absent; read prevError there",
					d.Name, to, from)
			}
		}
	}
}

// readsHTTPResponse reports whether any config of p mentions httpResponse.
func readsHTTPResponse(p map[string]any) bool {
	for _, raw := range digList(p, "configs") {
		c := mapOf(raw)
		for _, field := range []string{"value", "expression", "template"} {
			if httpResponseRef.MatchString(digStr(c, field)) {
				return true
			}
		}
	}
	return false
}

// narrows says what a config entry narrows by table name, or "" when it does
// not. Only a value this can read is judged: a `semanticLayers` expression that
// is not a JSON literal is left alone.
func narrows(ptype string, c map[string]any) string {
	name := digStr(c, "name")
	set := digStr(c, "value") != "" || digStr(c, "expression") != "" || digStr(c, "template") != ""
	switch {
	case ptype == "query-database" && name == "allowedTables" && set:
		return "sets allowedTables"
	case ptype != "llm-completion" && ptype != "stream-llm-completion-message":
		return ""
	case name == "semanticLayer.allowedCubes" && set:
		return "sets semanticLayer.allowedCubes"
	case name != "semanticLayers":
		return ""
	}
	text := digStr(c, "value")
	if text == "" {
		text = digStr(c, "expression")
	}
	var layers []map[string]any
	if json.Unmarshal([]byte(text), &layers) != nil {
		return ""
	}
	var named []string
	for _, l := range layers {
		if cubes, ok := l["allowedCubes"].([]any); ok && len(cubes) > 0 {
			n, _ := l["name"].(string)
			named = append(named, n)
		}
	}
	if len(named) == 0 {
		return ""
	}
	return "sets allowedCubes on " + strings.Join(named, ", ")
}

// Tool is one entry exposed to a model as a callable tool.
type Tool struct {
	Workflow    string
	Entry       string
	Name        string
	Description string
}

// Tools returns every tool a render exposes, in workflow order.
//
// There is no rule attached to this and there should not be. `tooling.description`
// is the single field where a wrong value makes a model pick the wrong tool, and
// what makes one wrong is that it does not distinguish itself from the tool
// beside it - which is a property of the set, not of any entry. No check can
// read that; a person reading all of them at once can, and until now there was
// nowhere that put them together.
func Tools(docs []Doc) []Tool {
	var out []Tool
	for _, d := range docs {
		if d.Kind != "Workflow" {
			continue
		}
		for _, raw := range digList(d.Spec, "entries") {
			e := mapOf(raw)
			t := mapOf(e["tooling"])
			if len(t) == 0 {
				continue
			}
			out = append(out, Tool{
				Workflow:    d.Name,
				Entry:       digStr(e, "name"),
				Name:        digStr(t, "name"),
				Description: digStr(t, "description"),
			})
		}
	}
	return out
}
