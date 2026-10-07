// Package stage holds the guidance for the decisions an onboarding makes, and
// reads the repository the guidance is rendered against.
//
// It reports no position. What it once did - derive one stage from the earliest
// missing CR kind and call that where the onboarding stood - is gone, and so is
// the mechanism that replaced it, which raised the same rungs from conditions
// instead of from a counter. Guidance is reached by name or by subject; what a
// chart still lacks is arithmetic against its declared shape, and Gaps is that.
package stage

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/chart"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/kb"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/repo"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/work"
)

//go:embed prompts
var prompts embed.FS

// Name identifies one step of an onboarding.
type Name string

const (
	Projects    Name = "projects"
	DataSources Name = "data-sources"
	ReadPath    Name = "read-path"
	EntryPoint  Name = "entry-point"
	Knowledge   Name = "knowledge"
	Verify      Name = "verify"
	Deploy      Name = "deploy"
	Enhance     Name = "enhance"
	Idle        Name = "idle"

	Requirements Name = "requirements"
)

// Stage is one piece of guidance, reached by name.
//
// It carried a Number until the walk was removed. Nothing read it once "stage 4
// of 9" stopped being printed, and a field nobody reads is a position waiting
// to be reintroduced - the order below is the order they are listed in, and
// that is the whole of it.
//
// Title is filled in from the document's own "# " heading at startup, not
// written here. It was written here, in a table beside the filename, which made
// the Go source a second home for a fact the document should carry - and the
// two could disagree with nothing to notice.
type Stage struct {
	Name    Name
	Title   string
	promptF string
}

// init reads each document's title out of the document.
//
// Panicking is right: the prompts are embedded, so a missing heading is a
// build-time mistake that would otherwise ship as an empty column in every
// listing.
func init() {
	for i := range Stages {
		Stages[i].Title = titleOf(Stages[i].promptF)
	}
	IdleStage.Title = titleOf(IdleStage.promptF)
	RequirementsStage.Title = titleOf(RequirementsStage.promptF)
	Readable = append(append([]Stage{}, Stages...), RequirementsStage, IdleStage)
}

func titleOf(file string) string {
	data, err := prompts.ReadFile("prompts/" + file)
	if err != nil {
		panic("read prompt " + file + ": " + err.Error())
	}
	d := kb.Parse(strings.TrimSuffix(file, ".md"), data)
	if d.Title == "" {
		panic("prompt " + file + " has no `# ` heading; it is where the title lives")
	}
	return d.Title
}

// Stages lists every stage in order.
var Stages = []Stage{
	{Name: Projects, promptF: "02-projects.md"},
	{Name: DataSources, promptF: "03-data-sources.md"},
	{Name: ReadPath, promptF: "04-read-path.md"},
	{Name: EntryPoint, promptF: "05-entry-point.md"},
	{Name: Knowledge, promptF: "06-knowledge.md"},
	{Name: Verify, promptF: "07-verify.md"},
	{Name: Deploy, promptF: "08-deploy.md"},
	// Enhance is the one nothing can raise from the files: "the onboarding is
	// finished" is not something the repo can tell you - a deployed repo and one
	// waiting for its first tag look the same on disk. Read it with
	// `asgard-cli guide enhance` once the repo is live.
	{Name: Enhance, promptF: "09-enhance.md"},
}

// IdleStage covers the state between two pieces of work: no request, no task,
// and no chart still missing what its shape asks for. It is not a step of an
// onboarding, and nothing puts a reader here - the only question it can ask is
// what the customer wants next.
var IdleStage = Stage{Name: Idle, promptF: "10-idle.md"}

// RequirementsStage is the interview that turns what a customer said into a
// request. A conversation leaves no trace on disk until somebody writes it
// down, so no state of the files can tell a reader they need it. It is worth
// reading again at every later request rather than only the first.
var RequirementsStage = Stage{Name: Requirements, promptF: "11-requirements.md"}

// Readable is every piece of guidance a reader can ask for by name. Filled in
// by init, after the titles are read.
var Readable []Stage

// Find returns the stage with the given name.
func Find(name string) (Stage, bool) {
	for _, s := range Readable {
		if string(s.Name) == name {
			return s, true
		}
	}
	return Stage{}, false
}

// State is what the repository looks like right now.
type State struct {
	Projects  []ProjectState
	Requests  []work.Request
	Tasks     []work.Task
	Questions []work.Question

	// References is how many files of customer material have been filed. It is
	// here so the interview prompt can tell the difference between an interview
	// that has not happened and one that happened and left nothing behind - the
	// second is the failure worth naming, and it looks identical from the
	// records alone.
	References int

	// SpecSlug is the living-spec directory this repository actually uses,
	// which an engagement may have renamed. It is read from disk rather than
	// assumed, because a prompt naming a path that is not there sends a reader
	// looking for it. Empty outside a repository, where `repo.SpecSlug` - the
	// default a fresh one gets - is what a prompt should say.
	SpecSlug string
}

// InFlight reports whether the repository records any work not yet done. It is
// what separates "the onboarding is between two pieces of work" from "somebody
// is halfway through one".
func (s State) InFlight() bool {
	return len(work.ActiveRequests(s.Requests)) > 0 || len(work.ActiveTasks(s.Tasks)) > 0
}

// ProjectState is what one project's chart contains.
type ProjectState struct {
	Slug  string
	Kinds map[string]int
}

// Has reports whether the project's chart declares any CR of these kinds.
func (p ProjectState) Has(kinds ...string) bool {
	for _, kind := range kinds {
		if p.Kinds[kind] > 0 {
			return true
		}
	}
	return false
}

// NeedsEntryPoint reports whether this project is still missing the thing that
// reaches it.
//
// It used to be qualified by the project's declared shape, so that a finished
// mimir-dashboard - which has no entry point by design - was not listed as
// missing one. That shape was a record of intent this tool has no way to check
// and no business judging, and it is gone; what is left is the plain question,
// and a project that deliberately has no entry point will answer yes to it.
// **That is a prompt naming a project, not a gate failing one.**
func (p ProjectState) NeedsEntryPoint() bool {
	return !p.Has("Agent", "BotProvider")
}

// summaryOrder is the order CR kinds are reported in. It is fixed so that the
// same repository prints the same line twice, which a map range would not do.
var summaryOrder = []string{
	"DataConnector", "SemanticLayer", "Toolset", "Workflow", "Agent",
	"BotProvider", "SandboxBlueprint", "SkillSet", "SourceSet", "Syncer",
	"Trigger", "Plugin", "CompletionModel",
}

// Summary names the CR kinds this project's chart declares. Anything not in the
// known order is listed after them, sorted, so a kind added to the platform
// shows up rather than vanishing.
func (p ProjectState) Summary() string {
	if len(p.Kinds) == 0 {
		return "chart is empty"
	}

	seen := map[string]bool{}
	var parts []string
	add := func(kind string) {
		if n := p.Kinds[kind]; n > 1 {
			parts = append(parts, fmt.Sprintf("%s x%d", kind, n))
		} else if n == 1 {
			parts = append(parts, kind)
		}
	}
	for _, kind := range summaryOrder {
		seen[kind] = true
		add(kind)
	}

	var rest []string
	for kind := range p.Kinds {
		if !seen[kind] {
			rest = append(rest, kind)
		}
	}
	sort.Strings(rest)
	for _, kind := range rest {
		add(kind)
	}
	return strings.Join(parts, ", ")
}

// Inspect reads the repository at root and reports its state.
func Inspect(root string) (State, error) {
	state := State{SpecSlug: repo.SpecSlugIn(root)}

	projects, err := repo.Projects(root)
	if err != nil {
		return state, err
	}
	for _, project := range projects {
		ps := ProjectState{Slug: project, Kinds: map[string]int{}}

		refs, err := chart.Scan(root, project)
		if err != nil {
			return state, err
		}
		ps.Kinds = chart.Counts(refs)

		state.Projects = append(state.Projects, ps)
	}

	// The three records the repository keeps of its own work. They are read
	// rather than derived: a stage is a fact about files, a status is a fact
	// somebody wrote down, and neither can stand in for the other.
	requests, err := work.ReadRequests(root)
	if err != nil {
		return state, err
	}
	tasks, err := work.ReadTasks(root)
	if err != nil {
		return state, err
	}
	questions, err := work.ReadQuestions(root)
	if err != nil {
		return state, err
	}
	state.Requests, state.Tasks, state.Questions = requests, tasks, questions

	filed, err := work.FiledReferences(root)
	if err != nil {
		return state, err
	}
	state.References = filed

	return state, nil
}

// projectStages is what a chart needs before it is finished, and which piece of
// guidance covers each.
//
// It used to be a ladder: one walk across every project at once, returning the
// earliest gap as "the stage you are at". That was the wrong shape twice over -
// it made a position out of what is really a set of conditions, and it could
// only ever report one of them. The entries are unordered as far as anything
// here is concerned; a chart needs all of them that its shape asks for, and the
// order they get built in is the engagement's business.
var projectStages = []struct {
	stage Name
	kinds []string

	// entryPoint marks the rung that asks what reaches the chart. It is the one
	// rung a shape can be complete without, which is why it is flagged rather
	// than assumed to be last - see wants.
	entryPoint bool
}{
	{DataSources, []string{"DataConnector"}, false},
	{ReadPath, []string{"SemanticLayer", "Toolset"}, false},
	{EntryPoint, []string{"Agent", "BotProvider"}, true},
}

// wants reports whether a rung applies to this project.
//
// Only the entry-point rung is ever skipped, and only for a shape that declares
// it has none. A mimir-dashboard chart is a SemanticLayer and nothing else: the
// customer reaches it through Data Insight, so no Agent, Toolset, BotProvider
// or entry point is written at all, and `guide read-path` spends a
// section saying so and naming what goes wrong when somebody adds one anyway -
// it hands agents deliberately restricted to an API a second path into the
// database, and no gate catches it.
//
// Before this, the ladder asked every project for an Agent or a BotProvider.
// A finished mimir-dashboard chart could not satisfy that and never can, so

// Gap is what one project's chart still lacks for the shape it is being built

// Gaps reports what each project is missing.
//
// It names CR kinds rather than a stage, which is the same information without
// the claim that they happen in an order. A repository with three projects used
// to report one stage for all of them, and an engagement whose first project
// was live and whose second had just started was told the whole repository was
// at the second one's step.
//
// **A project with no declared shape is skipped**, and that is the difference
// between arithmetic and a guess. Against a declared shape, "this shape asks for
// X and X is absent" is subtraction. With no shape there is nothing to subtract
// from, and answering anyway means picking a set of kinds every chart is assumed

// Data is what a stage prompt is rendered with.
type Data struct {
	Projects   []ProjectState
	Requests   []work.Request
	References int
	// Questions is how many are recorded. It is here so this stage reads the
	// same signals `check` does: material filed with questions written against
	// it has been read, and prompting for a record then contradicts the command
	// that says so.
	Questions int
	RepoName  string
	SpecSlug  string
	Stage     Stage
}

// InterviewRequests are the open requests whose target project is not decided
// yet. They are what `--stage projects` is about, and a prompt read out of
// order has none.
func (d Data) InterviewRequests() []work.Request {
	var out []work.Request
	for _, r := range d.Requests {
		if r.Project == "" {
			out = append(out, r)
		}
	}
	return out
}

// RequestID names the request a prompt should refer to in the commands it
// prints: the first one still waiting on its project, otherwise the first open
// one, otherwise a placeholder, so that reading a stage out of order still
// produces a command somebody can adapt rather than a broken one.
func (d Data) RequestID() string {
	if waiting := d.InterviewRequests(); len(waiting) > 0 {
		return waiting[0].ID
	}
	if len(d.Requests) > 0 {
		return d.Requests[0].ID
	}
	return "REQ-xxx"
}

// Prompt renders the stage's guidance.
// overrideDir, when set, is searched for a prompt file before the embedded copy.
//
// Prompts ship inside the binary and are versioned with it, which is the right
// default: an engagement gets the prompts that were released, and a fix reaches
// every engagement in one release rather than in whichever repo remembered to
// copy it. This exists for the case that default makes painful - iterating on
// prompt wording against a live customer, where the loop would otherwise be
// edit, build, install, run.
//
// It is per-file and not all-or-nothing. A directory holding one prompt
// overrides that one and leaves the other eleven embedded, so an experiment
// cannot silently freeze the rest at whatever was copied out.
var overrideDir string

// SetOverrideDir points prompt reads at a directory. An empty string restores
// the embedded prompts.
func SetOverrideDir(dir string) { overrideDir = dir }

// readPrompt returns a prompt, preferring the override directory.
//
// A file that is present but unreadable is an error rather than a silent
// fallback: someone who passed --template-dir wants that directory, and quietly
// using the embedded copy instead is how an experiment appears to do nothing.
func readPrompt(name string) ([]byte, error) {
	if overrideDir != "" {
		path := filepath.Join(overrideDir, name)
		content, err := os.ReadFile(path)
		if err == nil {
			return content, nil
		}
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
	}
	return prompts.ReadFile("prompts/" + name)
}

func (s Stage) Prompt(state State) (string, error) {
	content, err := readPrompt(s.promptF)
	if err != nil {
		return "", fmt.Errorf("read prompt %s: %w", s.promptF, err)
	}

	// **Printed, so the frontmatter comes off.** `Static` feeds the landed
	// `guide/<name>.md`, which keeps it for the same reason a wiki page does -
	// it is what the index is rendered from - so this cannot be done in
	// `readPrompt`, which both go through.
	tmpl, err := template.New(s.promptF).Delims("<<", ">>").Parse(kb.Body(string(content)))
	if err != nil {
		return "", fmt.Errorf("parse prompt %s: %w", s.promptF, err)
	}

	var out strings.Builder
	err = tmpl.Execute(&out, Data{
		Projects:   state.Projects,
		Requests:   work.ActiveRequests(state.Requests),
		References: state.References,
		Questions:  len(state.Questions),
		SpecSlug:   specSlugOf(state),
		Stage:      s,
	})
	if err != nil {
		return "", fmt.Errorf("render prompt %s: %w", s.promptF, err)
	}
	return out.String(), nil
}

func (s Stage) String() string {
	// Switch on the name, not the number: the stages outside the sequence shared the
	// number -1, so matching on that labelled the requirements interview
	// "nothing in flight" - the opposite of what a reader is being told at
	// that point. The numbers are gone; matching on the name is why it stayed
	// correct when they went.
	switch s.Name {
	case Idle:
		return "nothing in flight"
	case Requirements:
		return "the interview"
	}
	return fmt.Sprintf("%s: %s", s.Name, s.Title)
}

// Raw returns a stage's prompt as written, before rendering. It is what search
// reads: the template directives are noise to a reader, but the prose around
// them is the material, and rendering would need a repository to render against.
func (s Stage) Raw() (string, error) {
	content, err := readPrompt(s.promptF)
	if err != nil {
		return "", fmt.Errorf("read prompt %s: %w", s.promptF, err)
	}
	// The audits read this to list what a prompt tells somebody to do, and an
	// imperative is never in the frontmatter.
	return kb.Body(string(content)), nil
}

// List returns every piece of guidance a reader can ask for by name. Callers
// that want the parsed documents rather than the Stage values want Docs.
func List() []Stage { return Readable }

// corpus is the guidance as a body of material, on the same terms as the wiki
// and the extracts.
//
// The files are `prompts/04-read-path.md` and the document is called
// `read-path`, so Docs resolves the names; everything else is the default,
// because each prompt now opens with its own "# " heading and paragraph.
var corpus = kb.Corpus{
	FS:      prompts,
	Dir:     "prompts",
	Docs:    promptRefs,
	Noun:    "stage",
	Command: "asgard-cli guide",
}

func promptRefs() ([]kb.Ref, error) {
	out := make([]kb.Ref, 0, len(Readable))
	for _, s := range Readable {
		out = append(out, kb.Ref{Name: string(s.Name), Path: "prompts/" + s.promptF})
	}
	return out, nil
}

// Docs returns every piece of guidance as a kb.Doc, so a caller listing the
// whole corpus does not have to special-case this part of it.
func Docs() ([]kb.Doc, error) { return corpus.List() }

// ── Landing ───────────────────────────────────────────────────────────────

// stateAction matches a template action that renders this repository's own
// state. The inline substitutions are not among them: a request id or a spec
// slug is a value in an example, and an example with a placeholder in it is
// still an example.
var stateAction = regexp.MustCompile(`<<(if|with|range|else|end|printf)\b`)

// inlineValues are the substitutions that become placeholders rather than
// disappearing.
var inlineValues = strings.NewReplacer(
	"<<.RequestID>>", "<request-id>",
	"<<.SpecSlug>>", "<spec-slug>",
	"<<.Slug>>", "<slug>",
	"<<.ID>>", "<id>",
	"<<.Title>>", "<title>",
	"<<.Summary>>", "<summary>",
	"<<.References>>", "<n>",
)

// Static returns the half of a stage's guidance that does not depend on this
// repository, as the markdown that lands at `guide/<name>.md`.
//
// **A guide is the one part of the material that is not a document but a view.**
// It renders what the repository currently has - which projects exist, what is
// still open - into the guidance, which is why it could not simply be written
// out: verbatim it ships template syntax, and rendered it commits one moment of
// a repository's state into a file that then goes stale against the directory
// next door rather than against the binary, where nothing would detect it.
//
// So the split is by paragraph, and it is small: 10 paragraphs of 542 across
// the ten stages. A paragraph carrying a state action goes; everything else
// stays. Placeholders are substituted first, so a paragraph is not lost for
// containing `<<.RequestID>>` in an example.
//
// **Three sentences had to be reworded in the source rather than dropped**,
// because a state claim is not always a template action: "Projects exist but no
// DataConnector does" is prose, and true only of the repository the command was
// run in. They now say which repository the stage is for, which reads correctly
// in both places.
func (s Stage) Static() (string, error) {
	body, err := readPrompt(s.promptF)
	if err != nil {
		return "", err
	}
	text := inlineValues.Replace(string(body))
	var kept []string
	for _, p := range strings.Split(text, "\n\n") {
		if stateAction.MatchString(p) {
			continue
		}
		kept = append(kept, p)
	}
	return strings.Join(kept, "\n\n"), nil
}

// StaticDocuments renders every stage's static half, for the export and for the
// audit that resolves the pointers in them.
func StaticDocuments() ([]struct{ Name, Body string }, error) {
	var out []struct{ Name, Body string }
	for _, s := range List() {
		body, err := s.Static()
		if err != nil {
			return nil, err
		}
		out = append(out, struct{ Name, Body string }{string(s.Name), body})
	}
	return out, nil
}

// specSlugOf is the living-spec directory a prompt should name: the one this
// repository has, or the default when there is no repository to read - `guide`
// answers outside one, and a prompt with an empty path in it is worse than a
// prompt naming the directory a fresh repository gets.
func specSlugOf(state State) string {
	if state.SpecSlug == "" {
		return repo.SpecSlug
	}
	return state.SpecSlug
}
