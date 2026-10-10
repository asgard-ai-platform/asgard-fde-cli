package gate

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// displayNameAnnotation maps a kind to the annotation the Platform UI reads its
// name from. Without it the CR applies cleanly and shows up nameless in the UI,
// so nothing before this gate can catch it.
//
// Agent is not required to carry a description: that comes from
// spec.managed.description.
var displayNameAnnotation = map[string]string{
	"Agent":            "agent-name",
	"SemanticLayer":    "semantic-layer-name",
	"DataConnector":    "data-connector-name",
	"SkillSet":         "skill-set-name",
	"SourceSet":        "source-set-name",
	"Syncer":           "syncer-name",
	"Toolset":          "toolset-name",
	"Workflow":         "workflow-name",
	"KnowledgeBase":    "knowledge-base-name",
	"Loader":           "loader-name",
	"BotProvider":      "bot-provider-name",
	"Trigger":          "trigger-name",
	"CompletionModel":  "completion-model-name",
	"Plugin":           "plugin-name",
	"SandboxBlueprint": "sandbox-blueprint-name",
}

// workflowSetTypes are the legal values of the workflow-set-type label, per
// asgard-workflow-service internal/shared.go. `trigger` is the newest of the
// three, so an older CR is not a reliable thing to copy from.
var workflowSetTypes = map[string]bool{
	"bot":             true,
	"automation_tool": true,
	"trigger":         true,
}

// workflowSetLabels is the set a Workflow needs to be visible at all. The list
// endpoint always queries with main-workflow-set=true, and the detail endpoint
// locates a set by workflow-set-id. A Workflow missing these is entirely legal -
// lint, CRD validation and a server dry run all pass - and the only symptom is
// that the Platform UI has nothing to list.
var workflowSetLabels = []string{
	"main-workflow-set",
	"workflow-set-id",
	"workflow-key",
	"workflow-set-type",
}

const (
	managedByKey       = "asgard-ai.com/managed-by"
	managedBySkillSet  = "skill-set"
	annotationPrefix   = "asgard-ai.com/"
	workflowSetIDLabel = annotationPrefix + "workflow-set-id"
)

// Xref checks that every reference between rendered CRs resolves, and that the
// metadata the Platform UI depends on is present.
func Xref(docs []Doc, opts Options) Result {
	ix := newIndex(docs)
	x := &xref{index: ix, opts: opts}

	x.buildLookups()
	x.checkNoDuplicateNames()
	for _, d := range ix.docs {
		x.checkDisplayName(d)
		x.checkKind(d)
	}
	x.checkOneMainPerSet()

	sort.Strings(x.problems)
	sort.Strings(x.warnings)
	return Result{Problems: x.problems, Warnings: x.warnings, Summary: ix.summary()}
}

type xref struct {
	*index
	opts     Options
	problems []string

	// warnings are for a shape the platform accepts and that costs something
	// else - UI presentation, a name the console cannot show. A problem is what
	// the apiserver refuses or what breaks when used, and the two were the same
	// list until four running deployments turned up in it.
	warnings []string

	// entriesByWorkflow is what makes a two-part reference checkable: an
	// entrypoint is (workflow, entry), and a wrong entry is as fatal as a wrong
	// workflow while apply still succeeds.
	entriesByWorkflow map[string]map[string]bool
	labelsByWorkflow  map[string]map[string]string
	labelsBySourceSet map[string]map[string]string

	// syncerDestsBySourceSet is the fact a SkillSet's searchPaths are checked
	// against. A SourceSet has no member registry any more, so where a Syncer
	// puts content is the only declaration that content is there.
	syncerDestsBySourceSet map[string]map[string]bool

	// skillSetsBySourceSet finds a SourceSet shared by two SkillSets, which
	// leaves the UI unable to show either one's git configuration.
	skillSetsBySourceSet map[string][]string

	// searchPathsBySkillSet tells the shared-monorepo shape from an accident.
	// Every SkillSet sharing a store slices it with its own searchPaths in the
	// four deployments that do this; one sharing it and slicing nothing is the
	// mistake the rule was written for.
	searchPathsBySkillSet map[string][]string

	// bundledSkillSets are the SkillSets a Plugin carries. They are exempt from
	// the one-SourceSet-each rule: a chart of bundles shares one skill store on
	// purpose, because the skills live in one repository and a store per bundle
	// would clone it per bundle. A deployment with 29 Plugins has exactly one
	// store. Those SkillSets are the Plugin's implementation rather than
	// something a person picks in the UI, so the UI cost the rule protects
	// against is one the shape accepts knowingly.
	bundledSkillSets map[string]bool
}

func (x *xref) errf(format string, args ...any) {
	x.problems = append(x.problems, fmt.Sprintf(format, args...))
}

func (x *xref) warnf(format string, args ...any) {
	x.warnings = append(x.warnings, fmt.Sprintf(format, args...))
}

func (x *xref) buildLookups() {
	x.entriesByWorkflow = map[string]map[string]bool{}
	x.labelsByWorkflow = map[string]map[string]string{}
	x.labelsBySourceSet = map[string]map[string]string{}
	x.syncerDestsBySourceSet = map[string]map[string]bool{}
	x.skillSetsBySourceSet = map[string][]string{}
	x.searchPathsBySkillSet = map[string][]string{}
	x.bundledSkillSets = map[string]bool{}

	for _, d := range x.docs {
		switch d.Kind {
		case "Workflow":
			entries := map[string]bool{}
			for _, e := range digList(d.Spec, "entries") {
				if name := digStr(mapOf(e), "name"); name != "" {
					entries[name] = true
				}
			}
			x.entriesByWorkflow[d.Name] = entries
			x.labelsByWorkflow[d.Name] = d.Labels

		case "SourceSet":
			x.labelsBySourceSet[d.Name] = d.Labels

		case "Syncer":
			ss := digStr(d.Spec, "sourceSetName")
			dest := digStr(d.Spec, "destinationPath")
			if ss != "" && dest != "" {
				if x.syncerDestsBySourceSet[ss] == nil {
					x.syncerDestsBySourceSet[ss] = map[string]bool{}
				}
				x.syncerDestsBySourceSet[ss][dest] = true
			}

		case "SkillSet":
			if ref := digStr(d.Spec, "sourceSetName"); ref != "" {
				x.skillSetsBySourceSet[ref] = append(x.skillSetsBySourceSet[ref], d.Name)
			}
			x.searchPathsBySkillSet[d.Name] = strList(d.Spec["searchPaths"])

		case "Plugin":
			for _, e := range digList(d.Spec, "skillSets") {
				if name := digStr(mapOf(e), "name"); name != "" {
					x.bundledSkillSets[name] = true
				}
			}
		}
	}
}

func (x *xref) checkDisplayName(d Doc) {
	want, ok := displayNameAnnotation[d.Kind]
	if !ok {
		return
	}
	// **A SkillSet a Plugin bundles is not shown in the UI at all**, so it has
	// no name to be missing. `skill-set.md` records that as the shape rather
	// than as an omission: those SkillSets carry no `skill-set-name` and no
	// `managed-by`, because they are implementation detail of the bundle. The
	// shared-SourceSet rules already exempt them; this one did not, and the
	// deployment with 29 Plugins was told 29 times to add a name nothing would
	// ever display. Found the first time that chart was rendered through the
	// gate at all.
	if d.Kind == "SkillSet" && x.bundledSkillSets[d.Name] {
		return
	}
	if d.Annotations[annotationPrefix+want] == "" {
		x.errf("%s/%s: missing annotation %s%s, so the Platform UI shows it with no name",
			d.Kind, d.Name, annotationPrefix, want)
	}
}

// checkEntrypoint verifies a (workflow, entry) pair. The entry half is the one
// that gets missed: a wrong entry name is a dead end that apply accepts.
func (x *xref) checkEntrypoint(where, workflow, entry string) {
	if workflow == "" {
		return
	}
	if !x.has("Workflow", workflow) {
		x.errf("%s: no Workflow/%s%s", where, workflow, x.fix("Workflow", workflow))
		return
	}
	if entry != "" && !x.entriesByWorkflow[workflow][entry] {
		x.errf("%s: Workflow/%s has no entry %q (it has: %s)",
			where, workflow, entry, sorted(x.entriesByWorkflow[workflow]))
	}
}

// remedy names the command that creates a missing CR, per kind. A gate that
// says only what is wrong leaves an agent working from this output alone to
// guess, and the guess is usually to delete the reference rather than create
// what it points at.
var remedy = map[string]string{
	"Agent":            "asgard-cli add agent %s --project %s",
	"DataConnector":    "asgard-cli add dataconnector %s --project %s --db-class postgres|mssql",
	"KnowledgeBase":    "asgard-cli add knowledgedrive %s --project %s",
	"Plugin":           "asgard-cli add plugin %s --project %s",
	"SandboxBlueprint": "asgard-cli add flowagent %s --project %s",
	"SemanticLayer":    "asgard-cli add semanticlayer %s --project %s --connector dc-<name>",
	"SkillSet":         "asgard-cli add skillset %s --project %s --repo <git url>",
	"SourceSet":        "asgard-cli add knowledgedrive %s --project %s",
	"Toolset":          "asgard-cli add querytool %s --project %s",
	"Workflow":         "asgard-cli add flowagent %s --project %s",
}

// ref checks a single reference and reports it against the field it came from.
func (x *xref) ref(where, kind, name string) {
	if name == "" || x.has(kind, name) {
		return
	}
	x.errf("%s: no %s/%s%s", where, kind, name, x.fix(kind, name))
}

// fix appends the command that would create the missing CR, with the prefix
// trimmed off the name because `add` takes the bare form.
func (x *xref) fix(kind, name string) string {
	tmpl, ok := remedy[kind]
	if !ok {
		return ""
	}
	bare := name
	if i := strings.Index(bare, "-"); i > 0 && i <= 4 {
		bare = bare[i+1:]
	}
	return fmt.Sprintf(" - create it with `"+tmpl+"`, or drop the reference", bare, x.opts.ProjectOr())
}

func (x *xref) checkKind(d Doc) {
	switch d.Kind {
	case "Agent":
		x.checkAgent(d)
	case "Toolset":
		for _, t := range digList(d.Spec, "tools") {
			ep := mapOf(dig(mapOf(t), "entrypoint"))
			x.checkEntrypoint(fmt.Sprintf("Toolset/%s.tools.entrypoint", d.Name),
				digStr(ep, "workflow"), digStr(ep, "entry"))
		}
	case "Trigger":
		x.checkTrigger(d)
	case "SemanticLayer":
		x.ref(fmt.Sprintf("SemanticLayer/%s.dataConnectorName", d.Name),
			"DataConnector", digStr(d.Spec, "dataConnectorName"))
		// A SemanticLayer may also bind Toolsets. This rule was missing once,
		// and a layer carried a Toolset that did not exist while the gate
		// stayed green.
		for _, ref := range strList(d.Spec["toolsetNames"]) {
			x.ref(fmt.Sprintf("SemanticLayer/%s.toolsetNames", d.Name), "Toolset", ref)
		}
		x.checkSemanticLayer(d)
	case "SkillSet":
		x.checkSkillSet(d)
	case "Syncer":
		x.checkSyncer(d)
	case "Workflow":
		x.checkWorkflow(d)
	case "SandboxBlueprint":
		x.checkSandboxBlueprint(d)
	case "BotProvider":
		ep := mapOf(dig(d.Spec, "entrypoint"))
		x.checkEntrypoint(fmt.Sprintf("BotProvider/%s.entrypoint", d.Name),
			digStr(ep, "workflow"), digStr(ep, "entry"))
	case "Loader":
		x.ref(fmt.Sprintf("Loader/%s.knowledgeBaseName", d.Name),
			"KnowledgeBase", digStr(d.Spec, "knowledgeBaseName"))
		x.ref(fmt.Sprintf("Loader/%s.database.dataConnectorName", d.Name),
			"DataConnector", digStr(d.Spec, "database", "dataConnectorName"))
	}
}

// checkSemanticLayer resolves the names a layer uses inside its own spec: each
// join's cube and dimensions, and each cube's primaryKeyDimensions. A join names
// a cube by `cubes[].name`, not by `sqlTable`, which is how asgard-core matches
// it. The CRD only checks that a join's two dimension lists are the same length,
// so a dry run accepts a join to a cube the layer does not declare, and at
// runtime the model is handed a relationship it cannot traverse - or, under an
// Agent's allowedCubes, the join is dropped - with no error either way.
//
// Cubes are often generated and joins hand-written, so regenerating the cubes,
// or capping how many dimensions a cube keeps, breaks a join without touching
// it.
func (x *xref) checkSemanticLayer(d Doc) {
	dims := map[string]map[string]bool{}
	for _, c := range digList(d.Spec, "cubes") {
		cube := mapOf(c)
		name := digStr(cube, "name")
		if name == "" {
			continue
		}
		set := map[string]bool{}
		for _, dim := range digList(cube, "dimensions") {
			if n := digStr(mapOf(dim), "name"); n != "" {
				set[n] = true
			}
		}
		dims[name] = set
	}

	for _, c := range digList(d.Spec, "cubes") {
		cube := mapOf(c)
		name := digStr(cube, "name")
		for _, pk := range strList(cube["primaryKeyDimensions"]) {
			if !dims[name][pk] {
				x.errf("SemanticLayer/%s.cubes[%s].primaryKeyDimensions: cube %s has no dimension %q (it has: %s) - add the dimension or drop it from primaryKeyDimensions",
					d.Name, name, name, pk, sorted(dims[name]))
			}
		}
	}

	for _, j := range digList(d.Spec, "joins") {
		join := mapOf(j)
		for _, side := range []string{"from", "to"} {
			ref := mapOf(join[side])
			cube := digStr(ref, "cube")
			if cube == "" {
				continue
			}
			where := fmt.Sprintf("SemanticLayer/%s.joins[%s].%s", d.Name, digStr(join, "name"), side)
			declared, ok := dims[cube]
			if !ok {
				x.errf("%s.cube: no cube named %q in this layer - declare the cube in cubes[] or drop the join", where, cube)
				continue
			}
			for _, dim := range strList(ref["dimensions"]) {
				if !declared[dim] {
					x.errf("%s.dimensions: cube %s has no dimension %q (it has: %s) - declare the dimension on the cube or drop the join",
						where, cube, dim, sorted(declared))
				}
			}
		}
	}
}

func (x *xref) checkAgent(d Doc) {
	managed := mapOf(d.Spec["managed"])
	if managed == nil {
		return
	}
	for _, ref := range strList(managed["toolsetNames"]) {
		x.ref(fmt.Sprintf("Agent/%s.toolsetNames", d.Name), "Toolset", ref)
	}
	for _, ref := range strList(managed["skillSetNames"]) {
		x.ref(fmt.Sprintf("Agent/%s.skillSetNames", d.Name), "SkillSet", ref)
	}
	for _, sl := range digList(managed, "semanticLayers") {
		x.ref(fmt.Sprintf("Agent/%s.semanticLayers", d.Name),
			"SemanticLayer", digStr(mapOf(sl), "name"))
	}
	// **The Agent's mounts are a real list where the blueprint's are a JSON
	// string**, which is the whole reason this was missed: the blueprint form
	// has been resolved since this check was written and the Agent form reads
	// so differently that it looked like a different field. It is not - it
	// names a SourceSet the same way and fails the same way, silently, with the
	// agent simply unable to see the files it was given.
	for _, m := range digList(managed, "sourceSetMounts") {
		x.ref(fmt.Sprintf("Agent/%s.managed.sourceSetMounts[].sourceSetName", d.Name),
			"SourceSet", digStr(mapOf(m), "sourceSetName"))
	}
}

func (x *xref) checkTrigger(d Doc) {
	ep := mapOf(dig(d.Spec, "entrypoint"))
	workflow := digStr(ep, "workflow")
	// A wrong entrypoint means the Trigger fires on schedule and fails every
	// time, while apply and lint stay green: only the CronJob log shows it.
	x.checkEntrypoint(fmt.Sprintf("Trigger/%s.entrypoint", d.Name), workflow, digStr(ep, "entry"))

	// The front end opens a Trigger's editor using these two labels on the
	// Trigger itself. Creating a Trigger from the UI copies them off the
	// entrypoint workflow; a hand-written CR has nobody to copy them.
	setID := d.Labels[workflowSetIDLabel]
	if setID == "" {
		x.errf("Trigger/%s: missing label %s, so its editor opens as a blank canvas",
			d.Name, workflowSetIDLabel)
	} else if want := x.labelsByWorkflow[workflow][workflowSetIDLabel]; want != "" && setID != want {
		x.errf("Trigger/%s: workflow-set-id %q does not match its entrypoint Workflow/%s, which has %q",
			d.Name, setID, workflow, want)
	}
	// project-environment-id is deliberately not checked here. It is empty
	// because the template does not read .Values.asgard.projectEnvironmentId,
	// which the platform injects on every run - so it is
	// empty through the whole middle of an onboarding, by design. Erring on it
	// failed `verify` for every freshly scaffolded project that added a
	// Trigger, and the message sent the reader to look at a label the template
	// already writes.
	//
	// Deployability warns about the same condition, for every kind it affects
	// rather than for Trigger alone, and names the cause and the fix. One
	// condition, one message.
}

func (x *xref) checkSkillSet(d Doc) {
	ref := digStr(d.Spec, "sourceSetName")
	if ref == "" {
		return
	}
	if !x.has("SourceSet", ref) {
		x.errf("SkillSet/%s.sourceSetName: no SourceSet/%s%s", d.Name, ref, x.fix("SourceSet", ref))
		return
	}

	// A searchPath outside every Syncer destination resolves to zero skills,
	// silently. (The other silent failure - listing a parent directory when the
	// platform reads one searchPath as one skill directory - cannot be seen
	// from the CR at all.)
	dests := x.syncerDestsBySourceSet[ref]
	for _, path := range strList(d.Spec["searchPaths"]) {
		if len(dests) == 0 {
			continue
		}
		under := false
		for dest := range dests {
			if strings.HasPrefix(path, dest) {
				under = true
				break
			}
		}
		if !under {
			x.errf("SkillSet/%s.searchPaths[%q] is not under any Syncer destinationPath feeding SourceSet/%s (they are: %s)",
				d.Name, path, ref, sorted(dests))
		}
	}

	// One SkillSet, one SourceSet, one Syncer. The platform's
	// POST /v1/skill-set/from-git creates the three together and binds them,
	// and the UI shows them that way; sharing a SourceSet leaves the UI unable
	// to find a given skill set's git configuration.
	// A SkillSet a Plugin bundles is exempt from both rules below: the shared
	// store is the point of the shape, and it carries no managed-by label
	// because the platform is not meant to present it as a skill set at all.
	if x.bundledSkillSets[d.Name] {
		return
	}

	// Count only the SkillSets a person is meant to pick in the UI. A Plugin's
	// bundled ones share the store on purpose and are filtered out on both
	// sides: they neither trigger the rule nor make another SkillSet trigger it.
	var owners []string
	for _, o := range x.skillSetsBySourceSet[ref] {
		if !x.bundledSkillSets[o] {
			owners = append(owners, o)
		}
	}
	// **A shared store sliced by searchPaths is a shape four deployments use,
	// and the platform accepts it.** The exemption above is keyed on a Plugin
	// bundling the SkillSet, which is where the shape was first seen - and none
	// of the four has a Plugin at all. What the shape actually costs is UI
	// presentation, not acceptance: the apiserver takes it, nothing breaks at
	// runtime, and the UI cannot find a given skill set's git configuration.
	//
	// So it warns rather than fails. `errf` here means the platform refuses it
	// or it breaks when used; this is neither. The 1:1:1 case with a missing
	// label still fails, because there the label is simply absent rather than
	// deliberately so - `ss-sk-base` and `ss-sk-internal` both carry it.
	shared := len(owners) > 1
	if shared {
		for _, o := range owners {
			if len(x.searchPathsBySkillSet[o]) == 0 {
				shared = false // one sharer slices nothing: an accident, not the shape
				break
			}
		}
	}

	if len(owners) > 1 {
		sortedOwners := append([]string(nil), owners...)
		sort.Strings(sortedOwners)
		// Reported by the first owner only, or the same problem is reported
		// once per owner.
		if d.Name == sortedOwners[0] {
			if shared {
				// **Whether this chart has a Plugin decides which sentence is
				// true**, and the chart is right here - so it is read rather
				// than a count of somebody else's deployments being quoted.
				// The quoted one said four deployments run this shape and none
				// has a Plugin: six do, and the one with 29 Plugins is the
				// exemption itself, which is the opposite of what it told a
				// reader holding a Plugin bundle.
				cost := "If that cost was not decided on purpose, each SkillSet wants its own SourceSet"
				if x.countsByKind["Plugin"] > 0 {
					cost = "This chart declares Plugins, and a Plugin bundle is the one shape that earns this: " +
						"its SkillSets are implementation detail of the bundle rather than something a person picks in the UI, " +
						"so the presentation cost is one they never had. Nothing else here earns it"
				}
				x.warnf("SourceSet/%s is sliced by %d SkillSets (%s), each with its own searchPaths. That is the shared-skills-monorepo shape and the platform accepts it - "+
					"what it costs is the UI, which cannot find a given skill set's git configuration, and `asgard-ai.com/skill-set-name` with it. "+
					"`.agents/skills/asgard-platform/usecase/skill-set.md` records the one exemption. %s",
					ref, len(owners), strings.Join(sortedOwners, ", "), cost)
			} else {
				x.errf("SourceSet/%s is used by %d SkillSets (%s) and at least one declares no searchPaths, so it is not the monorepo shape; each SkillSet needs its own, or the Platform UI cannot find its git configuration",
					ref, len(owners), strings.Join(sortedOwners, ", "))
			}
		}
	}
	if x.labelsBySourceSet[ref][managedByKey] != managedBySkillSet {
		if shared {
			// Said once, by the first owner: the label is on the SourceSet, so
			// every sharer would otherwise report the same missing label.
			sortedOwners := append([]string(nil), owners...)
			sort.Strings(sortedOwners)
			if d.Name == sortedOwners[0] {
				x.warnf("SourceSet/%s carries no %s=%s, which follows from being shared: the platform is not meant to present a sliced store as one skill set's own. Deliberate in this shape, and worth knowing rather than fixing",
					ref, managedByKey, managedBySkillSet)
			}
		} else {
			x.errf("SourceSet/%s is SkillSet/%s's own source, so it needs label %s=%s (the front end uses it to recognise the pairing)",
				ref, d.Name, managedByKey, managedBySkillSet)
		}
	}
}

func (x *xref) checkSyncer(d Doc) {
	sourceSet := digStr(d.Spec, "sourceSetName")
	x.ref(fmt.Sprintf("Syncer/%s.sourceSetName", d.Name), "SourceSet", sourceSet)

	// destinationPath and statePath are relative paths inside the volume, and
	// these are the CRD's own CEL rules. Catching them here reads better than
	// having the apiserver reject the upgrade.
	// The pre-rename spelling is `destinationMemberKey`, and the CRD still
	// accepts it - "kept only so pre-rename Syncer objects stay readable during
	// the transition". Three Syncers in the reference deployments are on it,
	// and calling that "missing destinationPath" was wrong twice over: the
	// field is not missing, and the object is not refused. It is a migration to
	// name, and the same path rules apply to whichever spelling is in use.
	dest := digStr(d.Spec, "destinationPath")
	if dest == "" {
		if old := digStr(d.Spec, "destinationMemberKey"); old != "" {
			dest = old
			x.warnf("Syncer/%s sets destinationMemberKey=%q, the pre-rename spelling. The CRD still accepts it - kept so pre-rename objects stay readable - and new objects should set destinationPath. Both are immutable once set, so this is a new Syncer rather than an edit",
				d.Name, old)
		}
	}
	if dest == "" {
		x.errf("Syncer/%s: sets neither destinationPath nor the deprecated destinationMemberKey, and the CRD requires one of them", d.Name)
	} else if why := badRelPath(dest); why != "" {
		x.errf("Syncer/%s.destinationPath=%q: %s", d.Name, dest, why)
	} else if !strings.HasSuffix(dest, "/") {
		x.errf("Syncer/%s.destinationPath=%q: the destination is a directory and must end in /", d.Name, dest)
	}

	state := digStr(d.Spec, "statePath")
	if state == "" {
		if old := digStr(d.Spec, "stateMemberKey"); old != "" {
			state = old
			x.warnf("Syncer/%s sets stateMemberKey=%q, the pre-rename spelling of statePath, which the CRD still accepts. New objects should set statePath",
				d.Name, old)
		}
	}
	if state != "" {
		if why := badRelPath(state); why != "" {
			x.errf("Syncer/%s.statePath=%q: %s", d.Name, state, why)
		} else if strings.HasSuffix(state, "/") {
			x.errf("Syncer/%s.statePath=%q: the state path is a file and must not end in /", d.Name, state)
		}
	}

	// Only a database-class Syncer has a DataConnector.
	x.ref(fmt.Sprintf("Syncer/%s.database.dataConnectorName", d.Name),
		"DataConnector", digStr(d.Spec, "database", "dataConnectorName"))

	// A Syncer feeding a SkillSet's own SourceSet carries the same label;
	// without it, it appears in the general Syncer list although it belongs to
	// that SkillSet.
	if owners, ok := x.skillSetsBySourceSet[sourceSet]; ok {
		// A store sliced by several SkillSets is not any one of their own, so
		// its Syncers do not carry that SkillSet's label either - the same
		// shape as on the SourceSet itself, and the same four deployments.
		shared := len(owners) > 1
		for _, o := range owners {
			if len(x.searchPathsBySkillSet[o]) == 0 {
				shared = false
				break
			}
		}
		if shared {
			if d.Labels[managedByKey] != managedBySkillSet {
				sortedOwners := append([]string(nil), owners...)
				sort.Strings(sortedOwners)
				x.warnf("Syncer/%s feeds SourceSet/%s, which %d SkillSets slice (%s), so it carries no %s=%s and shows up in the general Syncer list. That follows from the shared-monorepo shape rather than being an omission - `.agents/skills/asgard-platform/usecase/skill-set.md` has what the shape costs",
					d.Name, sourceSet, len(owners), strings.Join(sortedOwners, ", "), managedByKey, managedBySkillSet)
			}
			return
		}
		if d.Labels[managedByKey] != managedBySkillSet {
			sortedOwners := append([]string(nil), owners...)
			sort.Strings(sortedOwners)
			x.errf("Syncer/%s feeds SkillSet/%s's own SourceSet/%s, so it needs label %s=%s (or it shows up in the general Syncer list)",
				d.Name, strings.Join(sortedOwners, ", "), sourceSet, managedByKey, managedBySkillSet)
		}
	}
}

// badRelPath applies the CRD's relative-path rules.
func badRelPath(v string) string {
	if strings.HasPrefix(v, "/") {
		return "must not start with / (there is no root inside the volume)"
	}
	segments := strings.Split(strings.TrimSuffix(v, "/"), "/")
	for _, s := range segments {
		switch s {
		case "":
			return "must not contain consecutive slashes"
		case ".", "..":
			return "must not contain a . or .. component"
		}
	}
	return ""
}

func (x *xref) checkWorkflow(d Doc) {
	for _, key := range workflowSetLabels {
		if d.Labels[annotationPrefix+key] == "" {
			x.errf("Workflow/%s: missing label %s%s, so it belongs to no workflow set and the Platform UI has nothing to list",
				d.Name, annotationPrefix, key)
		}
	}
	if t := d.Labels[annotationPrefix+"workflow-set-type"]; t != "" && !workflowSetTypes[t] {
		legal := make([]string, 0, len(workflowSetTypes))
		for k := range workflowSetTypes {
			legal = append(legal, k)
		}
		sort.Strings(legal)
		x.errf("Workflow/%s.workflow-set-type=%q is not legal (%s)", d.Name, t, strings.Join(legal, ", "))
	}
	if d.Annotations[annotationPrefix+"workflow-set-name"] == "" {
		x.errf("Workflow/%s: missing annotation %sworkflow-set-name, so the set shows with no name in the Platform UI",
			d.Name, annotationPrefix)
	}

	// Node display names, default entries, relationship ids and the canvas
	// ConfigMap are deliberately not checked since 2026-08-31: the platform
	// falls back for all of that canvas metadata (asgard-workflow-service
	// #336-#340) and the chart no longer carries it.

	for _, exit := range digList(d.Spec, "exits") {
		e := mapOf(exit)
		hw := mapOf(e["handlingWorkflow"])
		x.checkEntrypoint(fmt.Sprintf("Workflow/%s.exits[%s].handlingWorkflow", d.Name, digStr(e, "name")),
			digStr(hw, "name"), digStr(hw, "entry"))
	}

	for _, p := range digList(d.Spec, "processors") {
		proc := mapOf(p)
		for _, c := range digList(proc, "configs") {
			cfg := mapOf(c)
			// Only static literals are checked: an expression or a template is
			// not known until runtime.
			if digStr(cfg, "name") == "sandboxBlueprint" {
				if v := digStr(cfg, "value"); v != "" {
					x.ref(fmt.Sprintf("Workflow/%s.processors[%s].configs[sandboxBlueprint]",
						d.Name, digStr(proc, "name")), "SandboxBlueprint", v)
				}
			}
		}
	}
}

func (x *xref) checkSandboxBlueprint(d Doc) {
	// Every top-level field here is a ValueExprTemplate: the name lists are a
	// comma-separated string in `value`, and the mount lists are a JSON string
	// in `value`. Only static values are checked. (These were once iterated as
	// YAML lists; nothing used them then, so nothing broke.)
	names := func(field string) []string {
		raw := digStr(d.Spec, field, "value")
		var out []string
		for part := range strings.SplitSeq(raw, ",") {
			if s := strings.TrimSpace(part); s != "" {
				out = append(out, s)
			}
		}
		return out
	}

	for _, ref := range names("toolsetNames") {
		x.ref(fmt.Sprintf("SandboxBlueprint/%s.toolsetNames", d.Name), "Toolset", ref)
	}
	for _, ref := range names("skillSetNames") {
		x.ref(fmt.Sprintf("SandboxBlueprint/%s.skillSetNames", d.Name), "SkillSet", ref)
	}
	for _, ref := range names("pluginNames") {
		x.ref(fmt.Sprintf("SandboxBlueprint/%s.pluginNames", d.Name), "Plugin", ref)
	}

	if raw := digStr(d.Spec, "sourceSetMounts", "value"); raw != "" {
		var mounts []struct {
			SourceSetName string `json:"sourceSetName"`
		}
		if err := json.Unmarshal([]byte(raw), &mounts); err != nil {
			x.errf("SandboxBlueprint/%s.sourceSetMounts is not valid JSON: %v", d.Name, err)
		}
		for _, m := range mounts {
			x.ref(fmt.Sprintf("SandboxBlueprint/%s.sourceSetMounts[].sourceSetName", d.Name),
				"SourceSet", m.SourceSetName)
		}
	}

	// spec.agents is a JSON string holding an array of SandboxBlueprintAgent -
	// that is how the CRD defines it - so baseAgentName is only visible after
	// parsing the string. This is the joint of the Flow Agent chain
	// (BotProvider -> Workflow -> SandboxBlueprint -> Agent), and a typo here
	// fails as badly as a wrong entrypoint: lint passes, the CRD passes because
	// agents is just a string, and at runtime a subagent is silently absent so
	// the agent merely looks like it cannot call its tools.
	if raw := digStr(d.Spec, "agents", "value"); raw != "" {
		var agents []struct {
			BaseAgentName string `json:"baseAgentName"`
		}
		if err := json.Unmarshal([]byte(raw), &agents); err != nil {
			x.errf("SandboxBlueprint/%s.agents is not valid JSON: %v", d.Name, err)
		}
		for _, a := range agents {
			x.ref(fmt.Sprintf("SandboxBlueprint/%s.agents[].baseAgentName", d.Name),
				"Agent", a.BaseAgentName)
		}
	}
}

// checkNoDuplicateNames rejects two CRs of the same kind and name in one render.
//
// Kubernetes has no way to hold both: whichever applies second replaces the
// first, so the content of one of them is silently gone. Nothing else catches
// it - helm renders both happily, and apply reports success twice.
//
// The way this happens in practice is a generator writing a shared CR into more
// than one file: several fixed query tools belonging to one Toolset each emitted
// their own copy of that Toolset, and the surviving one carried a single tool.
func (x *xref) checkNoDuplicateNames() {
	seen := map[string]int{}
	for _, d := range x.docs {
		seen[d.Kind+"/"+d.Name]++
	}

	keys := make([]string, 0, len(seen))
	for k, n := range seen {
		if n > 1 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	for _, k := range keys {
		x.errf("%s is declared %d times in this chart; Kubernetes keeps only the one that applies last, so the others are silently discarded", k, seen[k])
	}
}

// checkOneMainPerSet requires exactly one main per workflow set. Zero means the
// list endpoint cannot see the set at all; two means the front end picks with a
// find() that takes the first, so which one wins depends on list order.
func (x *xref) checkOneMainPerSet() {
	mains := map[string][]string{}
	for name, labels := range x.labelsByWorkflow {
		setID := labels[workflowSetIDLabel]
		if setID == "" {
			continue
		}
		if _, ok := mains[setID]; !ok {
			mains[setID] = nil
		}
		if labels[annotationPrefix+"main-workflow-set"] == "true" {
			mains[setID] = append(mains[setID], name)
		}
	}

	setIDs := make([]string, 0, len(mains))
	for id := range mains {
		setIDs = append(setIDs, id)
	}
	sort.Strings(setIDs)

	for _, id := range setIDs {
		found := mains[id]
		if len(found) == 1 {
			continue
		}
		sort.Strings(found)
		detail := ""
		if len(found) > 0 {
			detail = ": " + strings.Join(found, ", ")
		}
		x.errf("workflow-set %q has %d workflows with main-workflow-set=true, and needs exactly 1%s",
			id, len(found), detail)
	}
}
