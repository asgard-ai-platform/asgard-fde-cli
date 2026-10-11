package gate

import (
	"fmt"
	"sort"
	"strings"
)

const (
	publishedLabel = annotationPrefix + "agent-published"

	// minSampleQuestions is what a published Agent needs. Unpublished ones are
	// exempt: they are not going to be given work yet.
	minSampleQuestions = 2
)

// AgentSplit checks the "one system, one Agent" split.
//
// None of these break helm lint or apply. They make the orchestrator route
// wrongly, or let an agent's search space grow back:
//
//	R1b  an Agent cannot have no capability at all - a layer, a Toolset or a
//	     SkillSet, at least one - so that deleting one by accident does not pass
//	     quietly. **SkillSet was missing from that list and the rule was wrong
//	     about a running deployment**: every subagent of a flow-agent supervisor
//	     mounts skills and nothing else, and nine of them were told they had no
//	     capability at all.
//	R4   no Agent sets allowedCubes, which keeps the standing decision that a
//	     bound layer is queryable in full.
//	R7   a published Agent has at least two sampleQuestions. Published is the
//	     agent-published label, and that label is the only gate on whether a
//	     caller includes the Agent in agent_hub.agent_names.
//	R11  a SemanticLayer that no Agent binds is reported. An observation, not a
//	     verdict: the render cannot tell "deliberately unbound" from "somebody
//	     has not finished the read path", and neither can this tool.
//	R13  no Agent lists the same semantic layer twice in its own semanticLayers.
//	     A duplicate entry has no reading in which it was meant: the second one
//	     grants nothing the first did not, and it is what a copied line looks
//	     like when the name inside it was not changed.
//
// The numbering is inherited from the requirements document that first wrote
// these rules down for one deployment, which is why it has gaps; R13 is new
// here and continues it rather than reusing a retired number.
//
//	R1 is gone. It refused an Agent that bound more than one semantic layer, and
//	an Agent that bound a layer some other Agent had already bound. **Neither
//	half is enforced by anything else in the chain**: the CRD declares
//	`semanticLayers` as a plain array with no maxItems, and the platform's own
//	rule list checks that each name resolves to a real SemanticLayer and nothing
//	more. Run over a 12-industry demo chart set - agents modelled one per
//	business role, roles sharing the systems they read the way they do in a
//	company - it produced 121 failures across 11 of its 12 charts, and every one
//	of them was the shape somebody meant. The first half at least had a
//	rationale in the material (an agent mounting two layers has the search space
//	the split exists to shrink - `internal/corpus/usecase/agent-hub.md`); the
//	second half had none anywhere. **That rationale is still the advice and it
//	is not a verdict a render can reach**, so it stays in the extract and is not
//	a rule here. What survives of R1 is R13, which is the half of "bound twice"
//	that can only be a mistake.
//	R10 is gone. It refused an Agent binding a layer recorded as "OLAP-only" in
//	`.asgard-config.json`, and the recording was done by a flag on `verify`. **A rule
//	that needs a per-customer exemption list to work is not a rule.** It also
//	wrote a product use case - Data Insight, read through Mimir - into a config
//	field of a tool that cannot know what a customer is building. See
//	asgard-odin-pm docs/decisions/2026-09-05-asgard-cli-config-surface.md.
//	R12 is gone. It required prompt.task and prompt.format to be byte-identical
//	across every Agent in one render, on the premise - measured at the time, on
//	the deployments there were - that those two fields are wholly a shared
//	scaffold and the specialisation lives in persona and context. **A chart set
//	that interleaves instead breaks the premise**: one agent per business role,
//	writing task and format as a shared skeleton with the role's own substance
//	inside it. On that shape the rule was simultaneously always-red and blind -
//	22 failures across 11 of 12 charts that no edit could clear short of
//	redesigning 64 prompts, and an edit to a genuinely shared line in one agent
//	would not have changed the output, which was already failing.
//
//	**Nothing replaced it, and the reason is that the replacement was tried.**
//	The obvious one is to compare only the lines every Agent shares and report
//	drift in those. Measured on the same 12 charts, "a line present in every
//	Agent but one" hits 14 times, and all 14 are deliberate: a read-only role
//	whose capability line says (read) where the others say (read + write). A
//	rule cannot tell that from a copy somebody edited in one place, so there is
//	no version of this check that does not cry wolf on a correct chart.
//
//	It was also not applying where it was supposed to. Subagents of a flow-agent
//	supervisor are excluded, and the exclusion reads SandboxBlueprint
//	`spec.agents.value` - a reference deployment declares its five subagents
//	through `spec.agents.expression` instead, so all five counted as hub Agents
//	and R12 failed that deployment too. Finding that the exemption silently did
//	not apply to the shape it was written for is the other half of why this is a
//	deletion rather than a repair.
//
//	`internal/corpus/usecase/agent-hub.md` keeps the advice - copy a shared
//	block whole, and change every copy in one edit - as advice.
//
// Zero Agents is legal and passes: a pure Flow Agent project keeps its prompt in
// a Workflow and its capability in a SandboxBlueprint, so its chart has no Agent
// CR at all. The summary says "0 agent(s)" so that a project that lost its
// Agents by accident is visible to a reviewer.
//
// Every check here is per Agent, so none of them needs to know which Agents are
// a supervisor's subagents. The helper that read that out of a SandboxBlueprint
// existed for R12 and went with it.
func AgentSplit(docs []Doc, opts Options) Result {
	ix := newIndex(docs)
	agents := ix.of("Agent")

	var problems []string
	errf := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}
	var warnings []string
	warnf := func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	}

	boundBy := map[string]string{} // layer -> the first Agent that bound it
	published := 0
	var allLayers []string

	for _, a := range agents {
		managed := mapOf(a.Spec["managed"])
		layers := digList(managed, "semanticLayers")

		// A SkillSet is a capability source too, and leaving it out made this
		// rule wrong about nine Agents in a deployment that is running: every
		// subagent of a flow-agent supervisor mounts skills and nothing else,
		// and the message said it had "no source of capability at all" while it
		// had one. Found by running the gate over the reference deployments
		// rather than over a chart this tool generated.
		if len(layers) == 0 &&
			len(strList(managed["toolsetNames"])) == 0 &&
			len(strList(managed["skillSetNames"])) == 0 {
			errf("R1b %s: has no semanticLayers, no toolsetNames and no skillSetNames, so this Agent has no source of capability at all", a.Name)
		}

		// R13 is per Agent, so what counts as "already" resets here. Two Agents
		// binding one layer is a shape the platform deploys and a chart set can
		// mean - roles that share a system read it through the same layer - and
		// refusing it was half of R1. One Agent binding it twice is the same
		// line copied and not edited, and nothing else in the chain reports it:
		// the CRD's array has no uniqueness rule.
		seen := map[string]bool{}

		for _, item := range layers {
			layer := mapOf(item)
			name := digStr(layer, "name")
			if name == "" {
				name = "<unnamed>"
			}
			if seen[name] {
				errf("R13 %s: lists %s twice in its own semanticLayers; the second entry grants nothing the first did not",
					a.Name, name)
			}
			seen[name] = true

			// The first Agent to bind a layer is what R11 reads, and it is only
			// ever asked whether the layer is bound at all.
			if _, ok := boundBy[name]; !ok {
				boundBy[name] = a.Name
				allLayers = append(allLayers, name)
			}

			if len(digList(layer, "allowedCubes")) > 0 {
				errf("R4 %s: %s sets allowedCubes, against the standing decision that a bound layer is queryable in full; limit tables with the DataConnector's database user's grants instead",
					a.Name, name)
			}
		}

		if strings.EqualFold(a.Labels[publishedLabel], "true") {
			published++
			if n := len(digList(managed, "sampleQuestions")); n < minSampleQuestions {
				errf("R7 %s: published but has %d sampleQuestions, and needs at least %d (set %s to \"false\" for an unverified agent rather than leaving the questions empty)",
					a.Name, n, minSampleQuestions, publishedLabel)
			}
		}
	}

	// R11. A layer nobody binds may be finished and correct, or a read path
	// somebody has not wired yet, and **the render cannot tell them apart**.
	//
	// So this states the fact and stops. It used to end with a remedy - record
	// the layer as "OLAP-only" and R10 will refuse to let anyone bind it later -
	// which required a per-customer exemption list and put a product use case
	// into a config field. Both are gone. What is left is worth saying because a
	// reader who finds an unbound layer months later tends to bind it as a
	// tidy-up, and that can hand an agent a second path into a database; but
	// whether that matters here is a judgement this tool does not have.
	for _, d := range ix.of("SemanticLayer") {
		if boundBy[d.Name] != "" || mentionedElsewhere(docs, d) {
			continue
		}
		warnf("R11 %s: no Agent binds it and nothing else in the render mentions it", d.Name)
	}

	sort.Strings(problems)
	sort.Strings(warnings)
	sort.Strings(allLayers)

	// Each bound layer once, not once per binding. The list was one entry per
	// binding while a layer could only have one, and a chart where four role
	// agents read the same ERP layer printed its name four times - which reads
	// as four layers to somebody scanning the summary for how wide the read
	// path is.
	summary := fmt.Sprintf("%d agent(s) (%d published)", len(agents), published)
	if len(allLayers) > 0 {
		summary += ", layers: " + strings.Join(allLayers, ", ")
	}
	return Result{Problems: problems, Warnings: warnings, Summary: summary}
}

// mentionedElsewhere reports whether any document other than the layer itself
// names it.
//
// An Agent binds a layer in a structured field, but a Flow Agent project has no
// Agent CR at all: its layer is mounted on an LLM processor, and a processor
// config's value is a string. In the charts that value is an expression holding
// a JSON array - `[{"name": "sl-x", "allowQuery": true, ...}]` - so there is no
// field to dig for, and asking "which field references a layer" gets a
// different answer for every processor.
//
// So the test is textual and deliberately broad: a layer that no other document
// mentions anywhere is one nothing can be using. Broad in the safe direction -
// it can only suppress the warning, never raise a false one, and a name that
// appears in an unrelated comment is a name somebody chose to write down.
func mentionedElsewhere(docs []Doc, layer Doc) bool {
	for _, d := range docs {
		if d.Kind == layer.Kind && d.Name == layer.Name {
			continue
		}
		if mentions(d.Spec, layer.Name) {
			return true
		}
		for _, m := range []map[string]string{d.Labels, d.Annotations} {
			for _, v := range m {
				if strings.Contains(v, layer.Name) {
					return true
				}
			}
		}
	}
	return false
}

// mentions walks a decoded document looking for the name in any string.
func mentions(v any, name string) bool {
	switch t := v.(type) {
	case string:
		return strings.Contains(t, name)
	case map[string]any:
		for _, e := range t {
			if mentions(e, name) {
				return true
			}
		}
	case []any:
		for _, e := range t {
			if mentions(e, name) {
				return true
			}
		}
	}
	return false
}
