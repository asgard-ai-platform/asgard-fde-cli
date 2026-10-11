// Package size estimates what an engagement is made of, before any of it is
// written.
//
// It exists because there was a gap between the interview and the proposal that
// every FDE crossed by hand, with different arithmetic. "How many agents, how
// many projects" is the first question a proposal is asked and the basis of a
// quote, and nothing here could answer it: the stage prompts say how the split
// is decided, the extracts say what one shape contains, and nothing added them
// up.
//
// The numbers are counted off deployments in production rather than reasoned
// out. Where a shape's base differs from what somebody would guess - and the
// flow-agent shapes differ sharply, because they contain no Agent at all - the
// deployment is what settles it.
package size

import (
	"fmt"
	"sort"
	"strings"
)

// Shape is one deployment shape and what it costs before anything is added.
type Shape struct {
	Name string
	// Audience is who reaches it, which is what selects the shape.
	Audience string
	// Base is the CRs the shape needs whatever it reads or writes.
	Base map[string]int
	// SeenIn records the deployment the base was counted off, so a reader can
	// check it rather than trusting it.
	SeenIn string
	// Note is what a reader would otherwise get wrong.
	Note string
}

// Shapes are the entry points an engagement chooses between. They are the
// shapes an interview's audience question reaches; the rest of `asgard-cli
// usecase` describes parts rather than whole deployments.
var Shapes = []Shape{
	{
		Name:     "flow-agent-single",
		Audience: "public, anonymous visitors - one job",
		Base: map[string]int{
			"BotProvider":      1,
			"Workflow":         1,
			"SandboxBlueprint": 1,
		},
		SeenIn: "a shopping assistant whose whole chart is 8 CRs",
		Note: "**No `Agent` CR at all.** The prompt lives on a Workflow processor and\n" +
			"the capabilities on the blueprint. Asked \"how many agents\", the\n" +
			"intuitive answer is 1 and the correct one is 0 - and a quote built on\n" +
			"the intuitive answer is priced for work that does not exist.",
	},
	{
		Name:     "flow-agent-supervisor",
		Audience: "public, anonymous visitors - several specialisms",
		Base: map[string]int{
			"BotProvider":      1,
			"Workflow":         1,
			"SandboxBlueprint": 1,
		},
		SeenIn: "the same shape with subagents mounted on the blueprint",
		Note: "Still no `Agent` CR. Subagents are Managed Agents mounted by the\n" +
			"blueprint, so add one `Agent` per specialism with `--agents`.",
	},
	{
		Name:     "agent-hub",
		Audience: "internal, authenticated callers",
		Base: map[string]int{
			"BotProvider":      1,
			"Workflow":         1,
			"SandboxBlueprint": 1,
		},
		SeenIn: "a deployment with 9 Agents over 2 SemanticLayers, 20 CRs in total",
		Note: "One `Agent` per specialism the customer names - `--agents`. This is\n" +
			"the only shape where the agent count is the obvious one.",
	},
	{
		Name:     "mimir-dashboard",
		Audience: "internal - people who watch numbers rather than ask questions",
		Base:     map[string]int{},
		// **Not a whole deployment.** Every chart read for this set has an
		// entry point somewhere; what was counted is the read surface on its
		// own - layers with no Agent bound to them - which is what a Mimir
		// deliverable is. Naming a deployment here read as "that chart has no
		// Agent", and the finance one has three and a supervisor.
		SeenIn: "the read surface of a finance deployment: 3 SemanticLayers with no Agent bound to them",
		Note: "**No entry point and no agent.** The customer reaches it through the\n" +
			"product, so the chart is the read surface and nothing else. Views and\n" +
			"Dashboards are built by them, in Mimir, and are not chart work at all -\n" +
			"which is the part of the estimate people forget to say out loud.",
	},
}

// HasEntryPoint reports whether a chart of this shape is reached by something
// the chart itself declares.
//
// It is read off Base rather than stored, because Base was counted off a
// deployment and a second field would be a second place to be wrong. Three of
// the four shapes carry a BotProvider; mimir-dashboard carries nothing at all,
// because the customer reaches it through the product.
//
// `internal/stage` uses this to decide whether a project is still missing an
// entry point or has already finished. Before it did, the ladder asked every
// project for an Agent or a BotProvider, so a mimir-dashboard project could
// never be complete - and the guidance told the reader to build the CR that
// `guide read-path` spends a section explaining they must not build.
func (s Shape) HasEntryPoint() bool { return s.Base["BotProvider"] > 0 }

// Authenticated reports whether this shape's callers can authenticate, which is
// what decides the read surface. `asgard-cli guide requirements` asks it
// as question 2 and calls it the decision the whole interview exists to reach.
func (s Shape) Authenticated() bool {
	return !strings.HasPrefix(s.Audience, "public")
}

// Find returns a shape by name.
func Find(name string) (Shape, bool) {
	for _, s := range Shapes {
		if s.Name == name {
			return s, true
		}
	}
	return Shape{}, false
}

// Names lists the shapes for an error message.
func Names() []string {
	out := make([]string, 0, len(Shapes))
	for _, s := range Shapes {
		out = append(out, s.Name)
	}
	sort.Strings(out)
	return out
}

// Inputs are what an interview establishes, and what the count varies with.
type Inputs struct {
	Databases int // systems read through a SemanticLayer
	APIs      int // systems reached over HTTP
	Queries   int // fixed query tools, when a layer is not the right surface
	Writes    int // actions with a side effect, each gated
	Knowledge int // document sources - manuals, FAQs, a site
	Agents    int // specialisms, for the shapes that carry Agents
	Consoles  int // systems with no database and no API
	Schedules int // scheduled runs
}

// Estimate is what a shape plus those inputs adds up to.
type Estimate struct {
	Shape    Shape
	Inputs   Inputs
	CRs      map[string]int
	Total    int
	Warnings []string
}

// Of computes the estimate.
func Of(s Shape, in Inputs) Estimate {
	crs := map[string]int{}
	for k, v := range s.Base {
		crs[k] += v
	}

	// A database is a connector plus a read surface over it - but which read
	// surface depends on the audience, and this is one of the three decisions
	// this engagement reversed after building it the obvious way.
	//
	// An anonymous caller does not get a SemanticLayer. A layer without
	// allowedCubes is arbitrary SQL over every cube, and the exposed surface
	// grows by itself every time a table is added. Anonymous audiences get
	// fixed query tools instead - Workflows, counted below - so a database
	// contributes a connector and nothing else here.
	crs["DataConnector"] += in.Databases
	if s.Authenticated() {
		crs["SemanticLayer"] += in.Databases
	}

	// An API is reached by a Workflow, and the Workflows a caller may reach are
	// grouped into one Toolset. Same for fixed query tools.
	crs["Workflow"] += in.APIs + in.Queries
	if in.APIs+in.Queries > 0 {
		crs["Toolset"]++
	}

	// A write is its own Workflow and its own Toolset: the gate is a property
	// of the set, so a read tool sharing one is gated too.
	crs["Workflow"] += in.Writes
	crs["Toolset"] += in.Writes

	// Documents are a store and something that fills it.
	crs["SourceSet"] += in.Knowledge
	crs["Syncer"] += in.Knowledge

	crs["Agent"] += in.Agents
	crs["Trigger"] += in.Schedules
	crs["Workflow"] += in.Schedules

	// Operating a web console is a Workflow plus the skill describing the UI,
	// and the skill is the cost - see the warning below.
	crs["Workflow"] += in.Consoles
	if in.Consoles > 0 {
		crs["SourceSet"] += in.Consoles
		crs["Syncer"] += in.Consoles
	}

	// **A Workflow needed a ConfigMap of node positions and no longer does.** The
	// platform lays the graph out itself, so a chart written now writes none and
	// an estimate that adds one per Workflow is high by exactly that many. The
	// older deployments still carry theirs; reading one is not a reason to write
	// one. `.agents/skills/asgard-platform/wiki/workflow.md` owns the change.

	total := 0
	for k, v := range crs {
		if v == 0 {
			delete(crs, k)
			continue
		}
		total += v
	}

	e := Estimate{Shape: s, Inputs: in, CRs: crs, Total: total}
	e.Warnings = warnings(s, in)
	return e
}

// warnings are the things that make a number conditional. A proposal that
// states one number where one of these is unresolved has quoted a guess.
func warnings(s Shape, in Inputs) []string {
	var out []string

	if in.Consoles > 0 {
		out = append(out, fmt.Sprintf(
			"%d system(s) with only a web console. **The CR count is not the cost here.**\n"+
				"Browser operation needs every page and every dialog the menu does not show\n"+
				"written down first - one existing case runs to 93 pages. Quote this\n"+
				"separately or not at all, and say so.", in.Consoles))
	}
	if in.Writes > 0 && in.Schedules > 0 {
		out = append(out, "A write and a schedule in the same estimate. **A scheduled run cannot\n"+
			"approve anything** - nobody is there - so these cannot share a Toolset and\n"+
			"one of them is not what the customer thinks it is.")
	}
	if s.Name == "mimir-dashboard" && in.Agents > 0 {
		out = append(out, "Agents counted against a dashboard shape. Those are a second delivery\n"+
			"over the same model, not part of this one - estimate them separately.")
	}
	if in.Databases > 0 && !s.Authenticated() && in.Queries == 0 {
		out = append(out, "A database and an anonymous audience, with no query tools counted.\n"+
			"Anonymous callers do not get a SemanticLayer - a layer without allowedCubes is\n"+
			"arbitrary SQL over every cube - so the read surface is a fixed set of query\n"+
			"tools, and how many there are is a design decision nobody has made yet.")
	}
	if in.Databases == 0 && in.APIs == 0 && in.Consoles == 0 && s.Name != "mimir-dashboard" {
		out = append(out, "No system is being read. Either the interview has not reached\n"+
			"question 3 yet, or this is a knowledge-only deployment - which is a real\n"+
			"shape, and a much smaller one.")
	}
	return out
}

// Plain renders the estimate the way it may be said to a customer: no CR kinds,
// no counts of things they did not ask for.
//
// It exists because the number is asked for by whoever is writing the proposal,
// and the proposal may not carry any of the vocabulary above. Handing over only
// the CR table means somebody translates it under time pressure, and what they
// reach for is the word in front of them.
func (e Estimate) Plain() []string {
	var out []string
	in := e.Inputs

	switch e.Shape.Name {
	case "mimir-dashboard":
		out = append(out, "a set of dashboards your own people build and change, over the data below")
	case "agent-hub":
		out = append(out, "one place your staff sign in to and ask")
	default:
		out = append(out, "one way in for the people outside")
	}

	if n := in.Databases + in.APIs + in.Consoles; n > 0 {
		out = append(out, fmt.Sprintf("%d of your systems connected, read-only", n))
	}
	if n := in.Queries + in.APIs; n > 0 {
		out = append(out, fmt.Sprintf("%d kinds of question it can answer", n))
	}
	if in.Knowledge > 0 {
		out = append(out, fmt.Sprintf("%d source(s) of your own documents it reads from", in.Knowledge))
	}
	if in.Writes > 0 {
		out = append(out, fmt.Sprintf("%d action(s) it can take - each one stopping for a person to approve", in.Writes))
	}
	if in.Schedules > 0 {
		out = append(out, fmt.Sprintf("%d thing(s) that run on a schedule, reporting only", in.Schedules))
	}
	if in.Agents > 0 {
		out = append(out, fmt.Sprintf("%d specialisms, each answering for its own area", in.Agents))
	}
	return out
}

// Docs is the product documentation page for each CR kind, so an estimate can
// hand over the reading rather than sending somebody back through the wiki's
// source blocks to reassemble it. A customer asking for the shape usually asks
// for the documentation in the same sentence.
//
// A kind with no entry has no page, and that is worth saying rather than
// leaving blank - see Undocumented.
var Docs = map[string]string{
	"DataConnector":    "https://docs.asgard-ai.com/docs/product-suite/odin/features/settings/data-source",
	"SemanticLayer":    "https://docs.asgard-ai.com/docs/product-suite/odin/features/data-insight-semantic-model",
	"SourceSet":        "https://docs.asgard-ai.com/docs/product-suite/odin/features/drive",
	"Syncer":           "https://docs.asgard-ai.com/docs/product-suite/odin/features/drive",
	"SkillSet":         "https://docs.asgard-ai.com/docs/product-suite/odin/features/skillsets",
	"Agent":            "https://docs.asgard-ai.com/docs/product-suite/odin/features/agent-hub-managed-agent",
	"SandboxBlueprint": "https://docs.asgard-ai.com/docs/product-suite/odin/features/agent-hub-flow-agent",
	"Workflow":         "https://docs.asgard-ai.com/docs/product-suite/odin/features/mcp-servers",
	"Trigger":          "https://docs.asgard-ai.com/docs/product-suite/odin/features/automation-trigger",
	"Plugin":           "https://docs.asgard-ai.com/docs/product-suite/odin/features/plugins",
	"CompletionModel":  "https://docs.asgard-ai.com/docs/product-suite/odin/features/settings/completion-model",
}

// notACR are the counted things the platform does not define. They are still
// files somebody writes and a deploy needs, so leaving them out of an estimate
// makes it wrong by exactly their number.
// **Nothing is in here today, and the entry that was is why it exists.** A
// Workflow used to need a ConfigMap of node positions - not an Asgard CR, in no
// CRD, and so the thing an estimate forgot. The platform lays graphs out itself
// now, so counting one per Workflow makes an estimate wrong in the other
// direction. The map stays because the shape recurs: a file somebody writes
// that a deploy needs and no CRD describes.
var NotACR = map[string]string{}

// Undocumented names the parts with no product documentation page at all, and
// why each absence matters. A customer's own test plan usually asks for
// technical documentation by name, so "there is no page for this" is an answer
// somebody needs before the meeting rather than during it.
var Undocumented = map[string]string{
	"Toolset": "the approval gate. `.agents/skills/asgard-platform/usecase/write-path.md` calls it the shape the\n" +
		"platform is built around, and it has **no product documentation page** -\n" +
		"not under Sindri, not under Odin. A proposal can show the dialog as a\n" +
		"screenshot (`.agents/skills/asgard-platform/wiki/screenshots.md`, and crop it first) and has\n" +
		"nothing to link. This is the most commonly asked-about mechanism here, and\n" +
		"the one hardest to explain in words.",
	"BotProvider": "the channel. Which page applies depends on the channel, and for LINE\n" +
		"the platform's own integration page is thin - `.agents/skills/asgard-platform/wiki/integration.md`.",
}

// Sorted returns the CR counts in a fixed order.
func (e Estimate) Sorted() []string {
	keys := make([]string, 0, len(e.CRs))
	for k := range e.CRs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// String is the shape's one-line summary for a listing.
func (s Shape) String() string {
	base := 0
	for _, v := range s.Base {
		base += v
	}
	return fmt.Sprintf("%-24s %s", s.Name, s.Audience)
}

// BaseSummary describes the base CRs in one line.
func (s Shape) BaseSummary() string {
	if len(s.Base) == 0 {
		return "no entry point at all"
	}
	keys := make([]string, 0, len(s.Base))
	for k := range s.Base {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s×%d", k, s.Base[k]))
	}
	return strings.Join(parts, ", ")
}
