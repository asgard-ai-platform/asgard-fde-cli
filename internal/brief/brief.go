// Package brief answers a different question from `next`.
//
// `next` answers "where am I", derived from the repository, and it does that
// well. The question it cannot answer is "the thing I am about to do - where
// will I get it wrong", and that is the question with the expensive answers,
// for three reasons.
//
// The riskiest activity leaves no trace in a repository. Talking to a customer
// changes no file, so nothing state-derived can prepare anybody for it. The
// walk is arranged by position, and meetings happen at every position - an
// engagement at the entry-point stage still has a meeting tomorrow. And the
// material is arranged for reading at the point of use, which is right, except
// where what you need to know precedes where it is filed.
//
// The knowledge was already here and already marked: every stage names the
// decision that gets answered wrong, every extract carries an Unchecked block,
// and platform-unknowns is a page of nothing else. What was missing was a way
// in that is addressed by intent rather than by position.
package brief

import (
	"fmt"
	"sort"
	"strings"

	"testing/fstest"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/kb"
)

// Item is one thing that gets got wrong, and where the right version lives.
type Item struct {
	Subject string
	Wrong   string
	Right   string
	// Where is what owns the right version: a path relative to the directory
	// this briefing lands in, or **a command to run**, where the answer is a
	// fact about this workspace rather than a rule anybody wrote down - which
	// pipeline the workspace has cannot be a page. `audit-material --links`
	// resolves the first and `--commands` the second, so both are checked, and
	// a row whose right version is spread over two documents names the second
	// inside `Right` rather than taking a second field nothing resolves.
	Where string
}

// Activity is something an FDE is about to do.
type Activity struct {
	Name string
	// Description is the row this briefing gets in the landed index: what a
	// reader would come to it FOR, as against the title, which says only its
	// subject. The index used to show the title, and a title does not tell a
	// reader whether this is the document they need before the thing they are
	// about to do - "write-chart" is every chart ever authored, and the
	// briefing is the decisions that were made the obvious way and reversed.
	//
	// It is metadata for the index and is not rendered into the document, which
	// opens on its own title and `When` line.
	Description string
	When        string
	Lead        string
	Items       []Item
	Close       string
}

// Activities are the ones with a known way to go wrong. The test for adding one
// is whether somebody has actually got it wrong, not whether it is important.
var Activities = []Activity{
	{
		Name:        "customer-meeting",
		Description: "what gets oversold and undersold in the room - the one network shape, the approval gate, what the platform cannot send, and what must never reach a slide",
		When:        "before any conversation with the customer, at any stage",
		Lead: `The intuitive answers here are wrong in both directions, and the two
mistakes cost different things. Underselling - "it cannot tell who they are",
"we would have to build an approval step", "only the systems you connect to
it" - gives away scope, and the customer cannot tell caution from a wrong
answer. Overselling - a ticket number treated as authentication, a notification
the platform cannot send - is a promise that cannot be built. Read every entry
before the meeting, not only the ones that match your topic, because you cannot
predict which direction you will get wrong.

This stage's output is what you say. Other stages produce files, which can be
edited and re-rendered; a wrong sentence in a meeting ends up in the
customer's notes.`,
		Items: []Item{
			{
				"how we reach a system inside their network",
				`"a VPN, an allowlist or a jump host - whichever suits you"`,
				"One shape only: they add our outbound addresses to their allowlist. Asgard is hosted and the agent runs in a sandbox in that cloud, so there is nothing of ours to place on their network. If you offer options, the customer picks one that does not apply. Ask who will make the firewall change, for the follow-up list, and do not hand out the addresses in the meeting",
				"../wiki/operations.md",
			},
			{
				"whether an anonymous channel knows who is asking",
				`"it cannot tell who they are, so identity has to wait"`,
				"An anonymous channel can answer \"where is MY order\". The caller supplies identity server-side every turn; only the model supplies nothing. LINE's webhook carries a userId, which `../guide/requirements.md` covers rather than this pointer. Deferring identity gives away something you already have",
				"../guide/read-path.md",
			},
			{
				"how many agents",
				`"one agent, then"`,
				"A flow-agent shape renders zero `Agent` CRs - the prompt is on a Workflow processor and the capabilities on the blueprint. A quote that assumes one Agent is priced for work that does not exist",
				"asgard-cli size flow-agent-single",
			},
			{
				"how an anonymous caller is identified",
				`"they can give us their ticket number and we look it up"`,
				"A ticket number is not authentication. Ticket numbers are usually sequential, so a lookup keyed on one alone lets anybody enumerate other people's cases. Any self-service query on a public channel has to say what identifies the person, and a number they type does not count",
				"../guide/requirements.md",
			},
			{
				"how a write is tested",
				`"during the test, shall we really create the ticket or just draft it?"`,
				"Ask whether they have a test environment first. The question under Said assumes they only have production, and it gives away the strongest version of the first delivery: writing into a test system proves the fields, the validation rules and the status codes, and a mock proves none of them. A mock is the fallback when there is no test environment - or when they will not let us write to production, which is their call",
				"../usecase/write-path.md",
			},
			{
				"whether it can do things, not only answer",
				`"we would have to build an approval step for that"`,
				"The approval gate is built into the platform. A write stops, names the tool, and nothing runs until a person allows it. It is not custom work. What is approved is the tool rather than the exact content",
				"../usecase/write-path.md",
			},
			{
				"whether it can email or message people",
				`"sure, we can have it send you a summary"`,
				"The platform sends nothing: no SMTP, no mail toolset, nothing in the core. It can call an endpoint of theirs that sends mail, if they have one. Ask whether they have one before promising a notification",
				"../wiki/integration.md",
			},
			{
				"what the agent can reach",
				`"only the systems you connect to it"`,
				"That is incorrect. Every sandbox carries the coding CLI's own tools - web search and fetch among them - and no Toolset or blueprint setting removes them. The only control is an instruction in the prompt",
				"../wiki/tools.md",
			},
			{
				"whether they can use their own model or their own key",
				`"yes, we can point it at your account"`,
				"Only on Odin. Sindri and Mimir use the platform's designated models and the LLM cannot be swapped there. So the answer depends on which product the capability lands in - which is question 2b, decided before anyone thinks about billing. A customer with a model contract or a rule about where inference happens needs this while the shape is still open",
				"../wiki/fehu.md",
			},
			{
				"whether they can have a specific model",
				`"of course, we will configure that one"`,
				"They can, and they should know the cost: a builtin tier is a logical model backed by several providers with automatic failover, and a custom `CompletionModel` is one provider, one key, one point of failure. A concrete reason - compliance, an existing contract, a model they tested against - justifies it. A preference usually does not",
				"../wiki/settings.md",
			},
			{
				"what the limits are",
				`"30 steps and 3 minutes per request is the limit"`,
				"The time is a default, raised by contacting sales or service@asgard-ai.com. The steps are not a quota at all: a step is one hand-off between processors, however many tools one of them calls, and the ceiling is the chart's own `maxUnsupervisedSteps`, 30 unless the chart says otherwise - so say what a step is and that it is set per deployment",
				"../wiki/integration.md",
			},
		},
		Close: `A slide is not an agenda and not a tracking list. "Who is
responsible for this?" is fine to ask in the room and should not be printed:
asking it shows you will follow up, while printing it reads as managing their
organisation. To decide, ask whether the answer is something we chase after
the meeting (keep it off the slide) or something we build (it can go on). Six
of one deck's twenty corrections were this, more than any other kind, and three
of those were copied out of this tool's own material.

Ask what they have already read before describing anything. The
product site tells them Odin is for non-technical staff, gives them a vocabulary
- Basic Function, Template - that maps to nothing here, and says Mimir simulates
the future. ` + "`../wiki/what-they-read.md`" + ` has the three, and it costs
one sentence to find out which of them you are correcting.

Anything on
` + "`../wiki/platform-unknowns.md`" + ` that this engagement touches is ours to
resolve before the meeting. Do not raise it with the customer as something we
do not know about our own product.

Never on their screen: implementation nouns, another customer, coordinates -
theirs *or* our own outbound addresses - and this repository's own bookkeeping,
question numbers included. The ` + "`proposal-deck`" + ` skill has the rest.`,
	},
	{
		Name:        "connect",
		Description: "the questions binding a checkout has to ask a person - workspace, account, repository, pipeline, platform project - which of them `origin` answers on its own, and why a list of one is not a default",
		When:        "before binding a fresh checkout to the platform, and before adding a second account or pipeline to one",
		Lead: `Every item below is a question for a person, and for each one the tool
will appear to have an answer already. The risk here is a question that was
never asked.

A list with one entry reads as a default whatever the text next to it says. An
agent bound the only pipeline in a workspace because its name resembled the
directory, when the person wanted a new pipeline against a different
repository. Read this as a checklist and get an answer for each line.

One item is not a question when the checkout has already answered it, and the
item says so: the remote called origin decides the provider account and the
repository, and both commands that need them derive them from it. Do not ask a
question whose answer is already on disk. An FDE was once given three options
that their own origin remote had already settled, and that extra question
displaced the one that needed asking.`,
		Items: []Item{
			{
				"which workspace",
				"the one whose name looks like the customer's",
				"Always ask. Nothing in the checkout says which workspace it deploys into - it is one of the two facts a repository cannot supply about itself, which is why `.asgard-cli.yaml` records it. An account can reach fifteen, and several will be plausible",
				"asgard-cli workspace list",
			},
			{
				"which provider account",
				"the one the workspace already has a connection to",
				"The account whose repositories this engagement is about, which is often not the one connected months ago for something else. A workspace holds one connection per account and may hold several; `pipeline connect --account` names the one you mean, and defaults to the owner of this checkout's origin remote when there is one",
				"asgard-cli pipeline connections",
			},
			{
				"the app is not installed on that account yet",
				"pick an account from the list you already reach",
				"Install the app; a person does that on GitHub. `pipeline connect --account <account>` prints the install URL for it - the app's slug is per-platform and a wrong slug is indistinguishable from a private app, so it comes from the platform rather than being guessed. On an organisation it may need one of its admins. That is a wait, not a failure: say so and stop",
				"asgard-cli pipeline connect --help",
			},
			{
				"which repository",
				"the one this directory is named after",
				"Look at `origin` before asking anything. When it is set, do not ask: `pipeline connect` and `pipeline create` both derive the account and the repository from it, and both say on stderr what they took. When there is none - the ordinary state of a fresh `init` - ask for the repository's remote URL, which is what the pipeline binds and the only thing to ask: one URL answers the account, the name and whether the repository exists. Do not split it into separate questions; a directory name is not a repository, and the name a pipeline binds is the one this engagement carries afterwards. Then set it and push, because the pipeline reads its declaration off the default branch and a repository with no commits fails its first config sync. `pipeline repos` lists what the installation actually grants - a repository missing from it was not granted, which is fixed on GitHub and not here",
				"asgard-cli pipeline repos",
			},
			{
				"a new pipeline, or one the workspace already has",
				"there is one pipeline and its name resembles the directory, so that is it",
				"This question has been answered wrong before. A workspace's existing pipeline may be for an entirely different repository, and one repository may carry several pipelines as long as their config paths differ. `pipeline use` and `pipeline create` are equally normal, and which one applies is not derivable from anything the tool can see",
				"asgard-cli pipeline list",
			},
			{
				"which platform project a release deploys into",
				"a workspace with no projects means something is broken",
				"An empty workspace is the normal start. A release deploys into a platform project, and the project decides the namespace, so nothing deploys until one exists. `pipeline project create <name>` makes one with the default environment that `release create` requires - and it consumes account quota, so a refusal there is a subscription limit rather than a bad name",
				"asgard-cli pipeline projects",
			},
			{
				"how many releases the chart needs",
				"one release, because the chart is one chart",
				"Usually one per environment. `<slug>-dev` and `<slug>-prod` name the same chart directory, differ by `on.pattern`, and are each created against a different platform project, which gives them different namespaces. One release is the shape for a POC nobody will maintain; if that is the answer, record it as a decision",
				"../guide/projects.md",
			},
		},
		Close: `None of these commands prompts. They are run by an agent following a
skill, not by somebody at a terminal - ` + "`init`" + ` is the one command in this tool with
a person in front of it - so the tool cannot ask these questions and the agent
has to. This list is those questions.

` + "`asgard-cli gate`" + ` says which of these have been recorded, at any point.`,
	},
	{
		Name:        "write-chart",
		Description: "the decisions that look obvious and were reversed - the public read surface, the anonymous entry point, `allowWrite` defaulting to true, and what a green render does not cover",
		When:        "before authoring or editing CRs",
		Lead: `Three decisions in this repository's history were made the obvious way,
built, and reversed. The same obvious choice is tempting each time.`,
		Items: []Item{
			{
				"the read surface for a public audience",
				"a SemanticLayer, like everything else",
				"Five zero-parameter query tools. A bound layer is arbitrary SQL over every cube in it and the exposed surface grows every time one is added - `../guide/read-path.md` has why narrowing it with `allowedCubes` is refused. A fixed tool ensures that no user input reaches SQL and widening it takes a CR change and a review",
				"../usecase/fixed-query-tools.md",
			},
			{
				"the entry point for an anonymous visitor",
				"the platform's agent hub, like everything else",
				"Its own BotProvider -> Workflow -> SandboxBlueprint. An anonymous visitor cannot authenticate to the hub, and `BotProvider.entrypoint` takes a Workflow, never an Agent",
				"../usecase/flow-agent-single.md",
			},
			{
				"where unstructured knowledge goes",
				"a KnowledgeBase with Loaders and a retrieval workflow",
				"A SourceSet Drive with `contextIndex`. Both are live; the Drive is the recommendation for new work",
				"../usecase/knowledge-drive.md",
			},
			{
				"a write inside a scheduled run",
				"omitting `allowWrite` because it is read-only anyway",
				"The definitions give `allowWrite` a default of true and `allowQuery` a default of false - the safe field is off by default and the dangerous one is on. So configuring a processor by adding only what you want produces a write path, and the rendered chart does not show it. Write `allowWrite: false` out on every entry that has a layer, and note that `query-database` carries the same pair",
				"../wiki/processors.md",
			},
			{
				"what an Expression may use",
				"ECMA5 only, because the documentation says ECMA5",
				"`ECMA5` is only the name of `execute-script`'s Engine value; the script body and every Expression run in the same VM. Expressions are ordinary JavaScript: `prevBlobs.map(b => b.blobId).join(',')` evaluates in a shipped chart, and `const` gets in through an IIFE, which is the form this tool generates. A bare declaration has nowhere to go only because the field holds one expression. Writing an Expression defensively costs nothing; refusing a shape because of a limit that does not exist is the mistake to avoid",
				"../wiki/processors.md",
			},
			{
				"a SandboxBlueprint's subagents",
				"it deployed, so the blueprint is right",
				"Exactly one of `baseAgentName` or `aliasName`, and no CRD checks it - the shape rides inside a JSON string where CEL cannot see it, so the controller enforces it at evaluation time. Both set, or neither, deploys green and fails the first time somebody talks to the agent. Check this rule by hand on every blueprint",
				"../wiki/crd-rules.md",
			},
			{
				"whether the chart is enough",
				"a green render means it is done",
				"A Workflow needs its `project-environment-id` label or the editor opens blank - not an Asgard CR, so nothing in the gate mentions it. The node-position `ConfigMap` beside it in an older chart is not needed for this: the platform lays the graph out itself now, and hand-written positions go stale against a graph anybody edits",
				"../wiki/workflow.md",
			},
		},
		Close: `` + "`asgard-cli check`" + ` and ` + "`asgard-cli verify`" + ` catch the shape. None of the
above is a shape problem, which is why they are here.`,
	},
	{
		Name:        "handover",
		Description: "a green deploy the customer cannot see - the per-resource permission step, the order to walk them through, and which screenshots have to be cropped",
		When:        "before telling anyone it is live, and before a training session",
		Lead: `A green deploy does not mean the customer can use it. One more step is
needed, and it is not in this repository.`,
		Items: []Item{
			{
				"why the customer says there is nothing there",
				"a chart problem, so look at the chart",
				"Building a resource does not make it visible. Somebody opens the Management Console, finds the product's page, uses *Manage Accounts in* and adds the people - per resource, because permissions do not inherit. Nothing in a chart, in `check`, in `verify` or in CD can see that it was skipped",
				"../wiki/console.md",
			},
			{
				"what to walk them through",
				"the chart, because that is what we built",
				"The order from a credential to an agent somebody can talk to - and there is no Sindri step in it, because every Managed Agent is published there already",
				"../wiki/setup-path.md",
			},
			{
				"which screens to show",
				"whatever is in the documentation",
				"Two of the recommended images are marked CROP FIRST, for different reasons: one dialog names a toolset by its `ts-` prefix, and one card view has the whole Odin console navigation down its left side. Open every one - nothing records when any was captured",
				"../wiki/screenshots.md",
			},
		},
		Close: `A handover deck may show the console; a proposal may not. They are
different documents - ` + "`proposal-deck`" + ` in ` + "`.agents/skills/`" + ` has the table.`,
	},
}

// Names lists the activities for an error message.
func Names() []string {
	out := make([]string, 0, len(Activities))
	for _, a := range Activities {
		out = append(out, a.Name)
	}
	sort.Strings(out)
	return out
}

// ── Landing ───────────────────────────────────────────────────────────────
//
// `asgard-cli init` writes these into a customer repository as `brief/`, one
// file per activity.
//
// **A brief is read by name before an activity rather than found by a word**,
// so landing it wins less than landing `needs` does - and it is worth being
// honest about which of the two the grep argument actually carries. What earns
// it a place is that two of the four are read from inside the repository:
// `write-chart` before touching a CR, `handover` before telling anyone it is
// live. And a sentence like "what must never appear on a customer's screen" is
// one somebody greps for without knowing a brief exists.

// provenance is the same on every brief: each entry is here because somebody
// got it wrong, and each names where the right version lives.
const provenance = `
**Checked:** every entry above is here because it actually happened, and each
names the document that carries the right version.

**Unchecked:** whether the list is complete, which only the next thing somebody gets wrong can show, so an activity with few entries is not a safe one.
`

// Document renders one activity as the markdown that lands at
// `brief/<name>.md`.
func (a Activity) Document() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Before %s\n\n%s\n\n%s\n", a.Name, a.When, a.Lead)
	for _, it := range a.Items {
		fmt.Fprintf(&b, "\n## %s\n\n**Said:** %s\n\n**True:** %s\n\nRead %s.\n",
			it.Subject, it.Wrong, it.Right, "`"+it.Where+"`")
	}
	fmt.Fprintf(&b, "\n%s\n", a.Close)
	b.WriteString(provenance)
	return b.String()
}

// Documents renders every activity, for the export and for the audit that
// resolves the pointers in them.
func Documents() []struct{ Name, Description, Body string } {
	out := make([]struct{ Name, Description, Body string }, 0, len(Activities))
	for _, a := range Activities {
		out = append(out, struct{ Name, Description, Body string }{a.Name, a.Description, a.Document()})
	}
	return out
}

// corpus is the rendered documents behind one `kb.Corpus`, for the same reason
// as in internal/needs: every body of material is read the same way, and these
// have no files of their own.
var corpus = func() kb.Corpus {
	files := fstest.MapFS{}
	for _, d := range Documents() {
		files["brief/"+d.Name+".md"] = &fstest.MapFile{Data: []byte(d.Body)}
	}
	return kb.Corpus{
		FS:      files,
		Dir:     "brief",
		Noun:    "brief",
		Command: "ls .agents/skills/asgard-platform/brief/",
	}
}()

// List returns every brief as a document.
func List() ([]kb.Doc, error) { return corpus.List() }
