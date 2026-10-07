// Package needs answers what a scenario has to be obtained from the customer
// before it can be built.
//
// **It is a view over material that already existed and could not be reached.**
// Every row below is written down somewhere - the interview's coordinates
// question, the per-channel credential table, the four-shape ladder for a system
// nobody here has integrated - and reaching it meant reading a 1200-line
// interview, a wiki page and three extracts and assembling the answer. The
// expensive failure it exists for is not choosing the wrong shape: it is week
// three, when the allowlist somebody never asked for turns out to need a ticket,
// an approval and a window.
//
// So no row states a fact of its own. Each carries `From`, the document that
// owns it, and `audit-material --links` resolves those the way it resolves every
// other pointer here - which is what stops this becoming a fifth place the same
// fact is written.
package needs

import (
	"fmt"
	"sort"
	"strings"

	"testing/fstest"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/kb"
)

// Item is one thing to obtain, and where the material says so.
type Item struct {
	// Ask is what to ask for, in the shape to ask for it. Specific on purpose:
	// offering options invites the other side to pick one that does not apply,
	// and the week it takes to find that out is the week being saved.
	Ask string `json:"ask"`

	// Why is what it changes, so somebody can drop it when it does not apply.
	Why string `json:"why"`

	// From is the document that owns this - a path, relative to the directory
	// this shape lands in, so it resolves both in the corpus tree and in a
	// repository. A pointer, not a restatement.
	From string `json:"from"`
}

// Shape is a deployment shape and what it needs from the customer.
type Shape struct {
	Name string `json:"shape"`
	What string `json:"what"`

	// Description is the index row: what somebody would come to this document
	// FOR, as against the title, which says only which shape it is about.
	//
	// It exists because the landed index showed the title, and a title does not
	// tell a reader whether this is the document they need - "external-api"
	// names the subject and says nothing about the test environment, the rate
	// limit or the mail credential that are the reasons to open it.
	//
	// It is metadata for the index and is not rendered into Document(): a
	// reader who has arrived is past the question it answers.
	Description string `json:"description"`

	Items []Item `json:"needs"`
}

var shapes = []Shape{{
	Name:        "semantic-layer",
	What:        "a database we can read, with an agent asking questions of it",
	Description: "the connection and the account, whether it is read-only, the outbound addresses that have to go on their allowlist, a description per cube",
	Items: []Item{
		{Ask: "host, port, database or schema, and the account name", Why: "there is no connection without them", From: "../guide/requirements.md"},
		{Ask: "is that account read-only? Ask explicitly", Why: "the one offered first usually is not, and finding out later means going back for a second credential", From: "../guide/requirements.md"},
		{Ask: "that Asgard's four outbound addresses go on their allowlist - ask their network team for exactly that, not for \"a VPN, an allowlist or a jump host\"", Why: "Asgard is hosted and the agent runs in the platform's own cloud; there is nothing of ours to put on their network. In most companies this change needs a ticket, an approval and a maintenance window, so discovering it late delays the project by weeks", From: "../wiki/operations.md"},
		{Ask: "whether the allowlist change can be done, and roughly when", Why: "a date changes the plan. Do not ask who approves it - a name changes nothing we build", From: "../guide/requirements.md"},
		{Ask: "a description of every cube, dimension and measure, in the customer's own words", Why: "the CRD requires a description on each, and it is what the model matches on - not the column name", From: "../usecase/semantic-layer.md"},
	},
}, {
	Name:        "external-api",
	What:        "a system with an HTTP API rather than a database",
	Description: "the base URL and its credential, whether a test environment exists, the rate limit, and why mail is an HTTP API rather than SMTP",
	Items: []Item{
		{Ask: "the base URL, the auth scheme, and a credential for it", Why: "endpoints and non-secret settings become chart values; a token is a secret", From: "../usecase/external-api.md"},
		{Ask: "whether there is a test environment, before designing a mock", Why: "writing into a real test environment proves the fields, the validation rules and the status codes; a mock proves none of them", From: "../usecase/write-path.md"},
		{Ask: "if it is production-only, whether they permit testing against it", Why: "in the meeting, not assumed here - the answer decides whether the first delivery can be proved at all", From: "../wiki/taiwan-channels.md"},
		{Ask: "the rate limit", Why: "it decides whether a Syncer can backfill at all", From: "../guide/requirements.md"},
		{Ask: "if the system is email — an HTTP mail API and a key for it, plus a sender address already verified with that provider. Not SMTP credentials", Why: "the platform's only outbound call is HTTPS, so a username, a password and an SMTP host cannot be used - and that is what gets handed over when you ask for mail access. The verification is their IT's to do, on their schedule, and an unverified sender is refused outright", From: "../wiki/integration.md"},
	},
}, {
	Name:        "chat-channel",
	What:        "the agent reached from a chat platform the customer's users already use",
	Description: "which channel, asked before anything else because the field is immutable; the credential pair each platform actually takes; the LINE step that gates the rest",
	Items: []Item{
		{Ask: "which channel, asked together with who is on the other end", Why: "`botProviderClass` is immutable once created, so changing it later is a new BotProvider rather than an edit", From: "../guide/requirements.md"},
		{Ask: "LINE: that Messaging API is enabled on the Official Account, before anything else", Why: "it is their step in their console, and it gates every other LINE question. The Channel Secret and Channel Access Token do not exist until it is done", From: "../wiki/integration.md"},
		{Ask: "LINE: Channel Secret and Channel Access Token — and somebody who can paste a Webhook URL back into the LINE Developers Console and enable Use webhook", Why: "LINE is the only two-way setup: Asgard produces a URL that has to go back. The rest only take credentials inward", From: "../wiki/integration.md"},
		{Ask: "Slack: an app-level token and a bot token - the `xapp-` and `xoxb-` pair, not a Client ID", Why: "those are different credentials, and the Client ID is often asked for by mistake. The client id, client secret, signing secret and scopes are what the platform's own UI flow installs an OAuth app with; a chart's `spec.slack` requires `appToken` and `botToken` and neither of those four. Ask for the client pair as well only if the engagement is going through the UI", From: "../wiki/integration.md"},
		{Ask: "Discord: the Bot Token, and the bot authorised and invited to the server in their Developer Portal", Why: "the invitation is their step in their console rather than ours, and none of it is the token - a chart with the right `botToken` still has nowhere to speak", From: "../wiki/integration.md"},
		{Ask: "Telegram: the Bot Token from BotFather. The second field is ours, not theirs", Why: "`spec.telegram` requires `webhookSecretToken` beside the bot token and no documentation page mentions it - it is a secret we choose, so it is not something to ask for, but a CR without it is refused", From: "../wiki/integration.md"},
		{Ask: "whether anything sits between the channel and us", Why: "an existing bot, a middleware, a support desk already on that channel - it changes the entry point", From: "../guide/requirements.md"},
	},
}, {
	Name:        "knowledge-drive",
	What:        "documents the agent reads - manuals, FAQs, pages",
	Description: "the documents or the place they live, who keeps them current, how often they change",
	Items: []Item{
		{Ask: "the documents themselves, or the place they live and access to it", Why: "a Drive syncs from somewhere; without the source there is nothing to index", From: "../usecase/knowledge-drive.md"},
		{Ask: "who keeps them current, and how often they change", Why: "each Syncer carries its own `schedule` and the index its own cron, so the answer sets both - and with incremental sync the same record lands in several dated partitions, where the graph treats a stale one as a fact unless somebody says which copy wins", From: "../usecase/knowledge-drive.md"},
	},
}, {
	Name:        "write-path",
	What:        "the agent doing something rather than answering",
	Description: "whether a test environment exists, and who is on the other end when the approval gate stops",
	Items: []Item{
		{Ask: "whether there is a test environment, first", Why: "choosing a mock before asking loses the strongest version of the first delivery", From: "../usecase/write-path.md"},
		{Ask: "who is on the other end when the gate stops for approval", Why: "on a public channel the person approving is the visitor, not staff", From: "../wiki/tools.md"},
	},
}, {
	Name:        "browser-operation",
	What:        "a system with no database we can read and no API",
	Description: "a back-office login, whether a non-production one exists, how many pages and operations actually matter",
	Items: []Item{
		{Ask: "a login to the back office, and whether a non-production one exists", Why: "the whole shape is driving their UI; there is nothing else to reach", From: "../usecase/browser-operation.md"},
		{Ask: "how many pages and operations actually matter", Why: "SHOPLINE is what this costs: 88 menu-level page entry points mapped before the first useful call, and a second map for everything below them. One back-office-only system among four sets the cost of the whole item", From: "../wiki/taiwan-channels.md"},
	},
}, {
	Name:        "skill-set",
	What:        "skills the deployed agent loads at runtime",
	Description: "the git repository the skills live in, and a token for it if it is private",
	Items: []Item{
		{Ask: "a git repository, and a token for it if it is private", Why: "a SkillSet syncs from a repo; a private one needs a PAT the platform can hold", From: "../usecase/skill-set.md"},
	},
}}

// Shapes returns every shape, sorted by name.
func Shapes() []Shape {
	out := append([]Shape(nil), shapes...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Names lists the shapes that have a dependency list.
func Names() []string {
	out := make([]string, 0, len(shapes))
	for _, s := range shapes {
		out = append(out, s.Name)
	}
	sort.Strings(out)
	return out
}

// ── Landing ───────────────────────────────────────────────────────────────
//
// `asgard-cli init` writes these shapes into a customer repository as files,
// one per shape, so that an FDE's agent reaches a row by grepping for the word
// the customer used - `allowlist`, `read-only`, `test environment` - rather
// than by knowing this command exists.
//
// **One file per shape rather than one file for all seven**, because a grep hit
// then carries which shape it belongs to. A row is only actionable with that:
// "ask whether there is a test environment" means a different conversation for
// a write path than for an external API.

// Document renders one shape as the markdown that lands at `needs/<name>.md`.
func (s Shape) Document() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s: what to get from the customer\n\n", s.Name)
	fmt.Fprintf(&b, "**%s**\n\n", s.What)
	b.WriteString(intro)
	for _, i := range s.Items {
		fmt.Fprintf(&b, "\n## %s\n\n%s\n\nStated in %s.\n", i.Ask, i.Why, "`"+i.From+"`")
	}
	b.WriteString(provenance)
	return b.String()
}

// provenance is the same on every shape, because it is the same claim: the
// checking is per row rather than per document. Without it these read as
// material nobody has held against anything, which `audit-material
// --unverified` would say and would be the wrong shape of true.
const provenance = `
**Checked:** each row above names the document that owns its claim, and
` + "`asgard-cli audit-material --links`" + ` resolves those. That is the whole of the
checking: a row is as good as the document it cites.

**Unchecked:** whether the list is complete for a shape, which only a finished engagement of that shape can show.
`

// intro is on every shape rather than in one file they all point at: a reader
// arrives here by grepping for a word in one row, and the rule that governs how
// to ask is worth more at that moment than a pointer to it.
const intro = `The customer provides these; we do not design them. Ask for exactly the
thing named. If you offer options, the customer may pick one that does not
apply, and it can take a week to find that out.
`

// Documents renders every shape, for the export and for the audit that resolves
// the pointers in them.
//
// Description travels beside the body rather than inside it, because the index
// that lands beside these documents is rendered from what landed: a caller
// holding the file has to be able to say what the file is for without parsing
// it back out of the markdown.
func Documents() []struct{ Name, Description, Body string } {
	out := make([]struct{ Name, Description, Body string }, 0, len(shapes))
	for _, s := range Shapes() {
		out = append(out, struct{ Name, Description, Body string }{s.Name, s.Description, s.Document()})
	}
	return out
}

// corpus is the rendered documents behind one `kb.Corpus`, so the audits and
// the landing read these the way they read every other body of material. They
// have no files - each is rendered from the shapes above - so the FS is built
// from them.
var corpus = func() kb.Corpus {
	files := fstest.MapFS{}
	for _, d := range Documents() {
		files["needs/"+d.Name+".md"] = &fstest.MapFile{Data: []byte(d.Body)}
	}
	return kb.Corpus{
		FS:      files,
		Dir:     "needs",
		Noun:    "shape",
		Command: "ls .agents/skills/asgard-platform/needs/",
	}
}()

// List returns every shape as a document.
func List() ([]kb.Doc, error) { return corpus.List() }
