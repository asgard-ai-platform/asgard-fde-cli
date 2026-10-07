// Package generate writes CR skeletons into a project's chart.
//
// A reference document tells an agent what a CR should look like; this writes
// one that already is. The difference matters for the parts that fail silently:
// a missing display annotation shows up as a nameless resource in the UI, a
// workflow without its set labels is invisible there, and a field renamed
// upstream still lints clean under its old name. None of those are caught by
// lint, by CRD validation, or by a server dry-run.
package generate

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"text/template"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/chart"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/repo"
)

//go:embed templates
var templates embed.FS

// File is one file a Kind writes.
type File struct {
	Dir      string // directory under the chart's templates/
	Template string // template file name
	Suffix   string // appended to the CR name, for a multi-file kind
}

// Kind is one thing that can be generated.
type Kind struct {
	Name    string // what the user types
	Summary string
	Prefix  string // CR name prefix
	Files   []File
	Needs   []string
	Extract string // which usecase extract covers it

	// Wiki is the platform wiki page that says what this thing IS, as opposed
	// to how it is assembled. An extract assumes the reader already knows the
	// platform has this shape; the wiki page is where that comes from, so the
	// two are read in that order.
	Wiki string

	// AlsoRead are extracts that cover the mechanism rather than the shape.
	// Without these a reader is told how the shape is arranged and not how the
	// pieces inside it pass values to each other, which is where the silent
	// failures are.
	AlsoRead []string
	After    []string // what has to be true before this can pass the gate
	Values   string   // template for the values.yaml keys this CR reads
}

// Kinds lists everything that can be generated, in the order they are usually
// created: a connector before a layer, a layer before an agent.
var Kinds = []Kind{
	{
		Name: "dataconnector", Summary: "connection to one database",
		Prefix: "dc-", Files: []File{{Dir: "data_connector", Template: "dataconnector.yaml.tmpl"}},
		Needs:   []string{"--db-class <one of nine; see asgard-cli add --help>"},
		Extract: "semantic-layer",
		Wiki:    "settings",
		// The values block is per class: salesforce has no port and no user,
		// athena has neither host nor database. dbValues renders it from the
		// CRD's own field list rather than from a template that assumed one
		// shape for all nine.
		Values: "<<.DBValues>>",
	},
	{
		Name: "semanticlayer", Summary: "read surface over one system, for an internal audience",
		Prefix: "sl-", Files: []File{{Dir: "semantic_layer", Template: "semanticlayer.yaml.tmpl"}},
		Needs:   []string{"--connector dc-<name>"},
		Extract: "semantic-layer",
		Wiki:    "semantic-model",
		After: []string{
			"Introspect the real database before filling in cubes. Do not guess a",
			"  schema: load .agents/skills/semantic-layer-modeling/ and query it.",
		},
		Values: `
# Reasoning depth for semantic layers. Omitting the field is NOT the same as
# medium: it sends no effort at all and the model's own default applies.
defaultSemanticLayerEffort: "medium"
`,
	},
	{
		Name: "agent", Summary: "one specialist, reached through the platform's agent hub",
		Prefix: "ag-", Files: []File{{Dir: "agent", Template: "agent.yaml.tmpl"}},
		Needs:   []string{"--layer sl-<name> (optional)"},
		Extract: "agent-hub",
		Wiki:    "agents",
		After: []string{
			"a project heading for a deploy needs at least one Syncer, and a SkillSet",
			"  brings one. The platform's apply step fires the Syncers of the release",
			"  that carry asgard-ai.com/auto-fire-on-rollout and waits for them, so with",
			"  none there is nothing after the dry run that proves the platform accepted",
			"  any of it, and a succeeded run only means helm returned:",
			"    asgard-cli add skillset base --repo <git url>",
			"  Then reference it from skillSetNames. Nothing is referenced by default,",
			"  because a name that does not exist is a dangling reference.",
			"prompt.task and prompt.format are the shared block: copy them from a",
			"  sibling agent rather than writing fresh ones, and change every copy in",
			"  one edit. Nothing checks this - a line edited in one agent and not the",
			"  others is invisible to the gate and to a reviewer reading one file.",
		},
	},
	{
		Name: "httptool", Summary: "a tool that calls an external HTTP API, plus its Toolset",
		Prefix: "wf-", Files: []File{{Dir: "tool", Template: "httptool.yaml.tmpl"}},
		Needs:    []string{"--toolset ts-<name>", "--write for a gated write path"},
		Extract:  "external-api",
		Wiki:     "api",
		AlsoRead: []string{"api-oauth", "workflow-chain"},
		After: []string{
			"Measure the request and response against the real API. The body field",
			"  names are ours until someone checks them against theirs.",
			"Add the auth header only once its key is declared AND set - a secretKeyRef",
			"  to a key nothing injects deploys fine and fails on the first call. The",
			"  lines above name the keys this wrote; declaring them is the half that",
			"  gets missed, and no local check can see it.",
			"tooling.description carries what decides a choice between tools. What is",
			"  true of the system rather than of this call goes in the skill, once,",
			"  and a skill written for this subject before the tool existed has to",
			"  lose whatever the tool now hides:",
			"    .agents/skills/asgard-platform/wiki/tool-description-and-skill.md",
		},
		Values: `
# <<.DisplayName>>
<<.ValuesKey>>:
  endpoint: ""
`,
	},
	{
		Name: "querytool", Summary: "a zero-parameter database query tool, plus its Toolset",
		Prefix: "wf-", Files: []File{{Dir: "tool", Template: "querytool.yaml.tmpl"}},
		Needs:    []string{"--connector dc-<name>", "--toolset ts-<name>"},
		Extract:  "fixed-query-tools",
		Wiki:     "workflow",
		AlsoRead: []string{"workflow-chain"},
		After: []string{
			"tooling.description is per tool: what this one returns, and which sibling",
			"  it is confused with. What is true of several of them belongs in the",
			"  agent's skill instead, where it is written and corrected once:",
			"    .agents/skills/asgard-platform/wiki/tool-description-and-skill.md",
		},
	},
	{
		Name: "skillset", Summary: "SkillSet with its own SourceSet and git Syncer",
		Prefix: "sk-", Files: []File{{Dir: "skill_set", Template: "skillset.yaml.tmpl"}},
		Needs:   []string{"--repo <git url>", "--private if it needs a PAT"},
		Extract: "skill-set",
		Wiki:    "tools",
		After: []string{
			"searchPaths must name one directory per skill. A parent directory",
			"  resolves to nothing, and no check catches it - the symptom is an agent",
			"  with fewer skills than expected.",
			"The agent reads this skill and the tooling.description of every tool it",
			"  has in one context, so the two have to divide the subject: anything",
			"  true across several tools belongs in the skill, and a skill still",
			"  describing what a tool now hides tells the model to parse a field that",
			"  never reaches it. Nothing checks the pair:",
			"    .agents/skills/asgard-platform/wiki/tool-description-and-skill.md",
		},
	},
	{
		Name: "trigger", Summary: "scheduled run, with its entrypoint Workflow",
		Prefix: "tr-", Files: []File{{Dir: "trigger", Template: "trigger.yaml.tmpl"}},
		Needs:    []string{"--layers sl-<name> (repeatable; --layer, singular, is a different flag this kind ignores)"},
		Extract:  "trigger",
		Wiki:     "automation",
		AlsoRead: []string{"workflow-chain"},
		After: []string{
			"The Trigger must read .Values.asgard.projectEnvironmentId for its",
			"  project-environment-id label. The platform injects that value on every",
			"  run, so this is a line in the template rather than a value to fetch -",
			"  and the skeleton already has it. `asgard-cli verify` warns if it goes",
			"  missing rather than failing: a Trigger with no label fires correctly on",
			"  a cluster while its editor opens as a blank canvas, which is the worst",
			"  failure shape there is - working, and uneditable.",
		},
		Values: `
# <<.DisplayName>>
triggers:
  <<.ValuesKey>>:
    # Ordinary five-field cron, or an @descriptor: the value is copied
    # verbatim into the derived CronJob, so the grammar is batch/v1's. The
    # platform parses it on write, in an admission webhook.
    schedule: "0 9 * * *"
    timeZone: "Asia/Taipei"
    # Suspended until the run has been exercised once by hand.
    suspend: "true"
`,
	},
	{
		Name: "knowledgedrive", Summary: "a SourceSet Drive with a knowledge graph, for documents",
		Prefix:  "ss-",
		Files:   []File{{Dir: "source_set", Template: "knowledgedrive.yaml.tmpl"}},
		Needs:   []string{"--connector dc-<name> for the database Syncer"},
		Extract: "knowledge-drive",
		Wiki:    "knowledge",
		After: []string{
			"Mount it read-only from the blueprint, and tell the agent in its prompt",
			"  to query the graph first and then read only the files it points at -",
			"  otherwise it crawls the whole Drive.",
			"Documents nobody can sync have to be uploaded after deploy, and the",
			"  index has to run once. Until then the knowledge answers are poor: put",
			"  it in the chart README as a post-deploy step with an owner.",
		},
		Values: `
# <<.DisplayName>>
<<.ValuesKey>>:
  timeZone: "Asia/Taipei"
  dbSync:
    # Ordinary five-field cron, or an @descriptor: the value is copied
    # verbatim into the derived CronJob, so the grammar is batch/v1's. The
    # platform parses it on write, in an admission webhook.
    schedule: "0 9 * * *"
    suspend: "false"
    batchSize: 1000
  contextIndex:
    schedule: "0 10 * * *"
    suspend: "false"
`,
	},
	{
		Name: "plugin", Summary: "a capability bundle a blueprint loads by name, and can pick per request",
		Prefix:  "pg-",
		Files:   []File{{Dir: "plugin", Template: "plugin.yaml.tmpl"}},
		Needs:   []string{"--connector ss-<name> for the skill store (defaults to ss-skill-repos)"},
		Extract: "plugin",
		Wiki:    "tools",
		After: []string{
			"Load it from a blueprint: pluginNames is a comma-separated string, or",
			"  an expression that computes the list from the caller's payload.",
		},
	},
	{
		Name: "flowagent", Summary: "a self-hosted entry point: BotProvider, Workflow and SandboxBlueprint",
		Prefix: "bp-",
		Files: []File{
			{Dir: "", Template: "flowagent-botprovider.yaml.tmpl", Suffix: "bot_provider"},
			{Dir: "", Template: "flowagent-workflow.yaml.tmpl", Suffix: "workflow"},
			{Dir: "", Template: "flowagent-blueprint.yaml.tmpl", Suffix: "sandbox_blueprint"},
		},
		Needs:    []string{"--public for an anonymous audience", "--toolset / --layer for its capabilities", "--bot-class generic|line|telegram|discord|slack (defaults to generic)"},
		Extract:  "flow-agent-single",
		Wiki:     "agents",
		AlsoRead: []string{"workflow-chain", "chat-channel", "per-turn-credentials"},
		After: []string{
			"a project heading for a deploy needs at least one Syncer, and a SkillSet",
			"  brings one. The platform's apply step fires the Syncers of the release",
			"  that carry asgard-ai.com/auto-fire-on-rollout and waits for them, so with",
			"  none there is nothing after the dry run that proves the platform accepted",
			"  any of it, and a succeeded run only means helm returned:",
			"    asgard-cli add skillset base --repo <git url>",
			"  Then reference it from skillSetNames. Nothing is referenced by default,",
			"  because a name that does not exist is a dangling reference.",
			"An anonymous entry point has no auth to protect it, so the protection is",
			"  on the capability side: read-only chain, zero-parameter tools, read-only",
			"  mounts. That argument stops holding the moment a parameterised or",
			"  write-capable tool is added.",
			"The prompt lives on the workflow's processor. Several specialists means",
			"  --supervisor, which writes the four-processor conversation loop three",
			"  deployments share edge for edge, and then subagents on the blueprint -",
			"  see .agents/skills/asgard-platform/usecase/flow-agent-supervisor.md.",
		},
		Values: `
# <<.DisplayName>>
botProviders:
  <<.ValuesKey>>:
    # Flip to take the public endpoint down without deleting anything.
    disabled: false
`,
	},
}

// Find returns the kind by name.
func Find(name string) (Kind, bool) {
	for _, k := range Kinds {
		if k.Name == name {
			return k, true
		}
	}
	return Kind{}, false
}

// Names lists every kind, for error messages.
func Names() string {
	names := make([]string, len(Kinds))
	for i, k := range Kinds {
		names[i] = k.Name
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// Options are the per-kind inputs.
type Options struct {
	Project     string
	Name        string // without the prefix
	DisplayName string
	Connector   string
	Layer       string
	Layers      []string
	Toolset     string
	Repo        string
	Private     bool
	Write       bool
	Public      bool

	// Supervisor writes the four-processor conversation loop instead of the
	// single-processor shape: the one three deployments share edge for edge.
	// Its specialists are subagents on the blueprint.
	Supervisor bool

	DBClass string
	Force   bool

	// SkillSets the project's chart already declares, filled in by Resolve. A
	// skeleton references only what exists: a name that does not is a dangling
	// reference the gate rejects, written by the tool itself.
	SkillSets []string

	// ToolsetExists reports that --toolset names a Toolset the chart already
	// has, so a template that would otherwise emit its own copy must not.
	ToolsetExists bool

	// BotClass is the BotProvider's channel. It defaults to generic, and it is
	// **immutable after creation** on the platform side, so getting it wrong
	// means a new CR rather than an edit.
	BotClass string
}

// Data is what a template renders with.
type Data struct {
	Options
	Chart     string // the chart's helper prefix
	CRName    string // prefixed
	ValuesKey string // the name as a Helm values key
	SpecSlug  string

	// DBSpec and DBValues are the DataConnector's class block and the values
	// keys it reads, rendered from the CRD's field list for that class. They
	// are not a template because the nine classes share almost nothing: one
	// template with conditionals in it would have to encode every difference
	// and would still have assumed a shape.
	DBSpec   string
	DBValues string

	// DBNote is what the class needs that its shape cannot say - Oracle taking
	// serviceName or sid and never both, HANA having no design-time driver.
	DBNote string
}

// valuesKey turns a CR name into something addressable in a values file.
// Helm cannot reach a key containing a hyphen with dot notation, so a name like
// daily-report has to become dailyReport - otherwise the chart fails to parse,
// and only at helm lint, well after it looked fine.
func valuesKey(name string) string {
	parts := strings.Split(name, "-")
	for i := 1; i < len(parts); i++ {
		if parts[i] != "" {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// Result reports what was written.
// SecretKeysWritten names every Secret and ConfigMap key the files just written
// reference, so that `add` can say what has to be declared before any of it
// works.
//
// This replaces a per-class list (dbSecretKeys) that was computed on every run
// and read by nothing, with two comments explaining that it existed so nobody
// would have to derive the names from the template. Somebody derived them from
// the template anyway - by running `strings` on this binary. Reading the files
// that were actually written covers every kind, including the next one added,
// and cannot disagree with them.
//
// It anchors on the ref block rather than matching `key:` anywhere: `key` is
// also an Asgard field name on several specs, and matching it loosely reports
// identifiers that have nothing to do with a Secret.
func SecretKeysWritten(results []Result) (secretKeys, configKeys []string, err error) {
	for _, r := range results {
		if r.Values || !r.Created {
			continue
		}
		body, err := os.ReadFile(r.Path)
		if err != nil {
			return nil, nil, err
		}
		sec, cfg := refKeys(string(body))
		secretKeys = append(secretKeys, sec...)
		configKeys = append(configKeys, cfg...)
	}
	return dedupeSorted(secretKeys), dedupeSorted(configKeys), nil
}

// refKeys reads the `key:` that belongs to each secretKeyRef / configMapKeyRef
// block, and nothing else.
func refKeys(body string) (secret, config []string) {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		var into *[]string
		switch trimmed {
		case "secretKeyRef:":
			into = &secret
		case "configMapKeyRef:":
			into = &config
		default:
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		// The block ends at the first line indented no further than the ref
		// itself, so a `key` belonging to the next field is never picked up.
		var key string
		for _, next := range lines[i+1:] {
			if strings.TrimSpace(next) == "" {
				continue
			}
			if len(next)-len(strings.TrimLeft(next, " ")) <= indent {
				break
			}
			if v, ok := strings.CutPrefix(strings.TrimSpace(next), "key:"); ok {
				if v = strings.TrimSpace(v); v != "" && !strings.Contains(v, "{{") {
					key = v
				}
			}
		}
		if key != "" {
			*into = append(*into, key)
		}
	}
	return secret, config
}

func dedupeSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

type Result struct {
	Path    string
	Created bool
	Values  bool // an addition to values.yaml rather than a new CR
}

// Write renders the kind into the project's chart. A kind may produce several
// files: some shapes are a chain of CRs that have no life apart from each other.
// BotClassGeneric is the default channel: an HTTP API for your own front end.
const BotClassGeneric = "generic"

// BotClasses is the whole vocabulary of spec.botProviderClass.
var BotClasses = []string{BotClassGeneric, "line", "telegram", "discord", "slack"}

func validBotClass(s string) bool {
	return slices.Contains(BotClasses, s)
}

// botClassNote says what choosing a channel costs, at the moment of choosing.
// Each of these is invisible in the generated YAML.
func botClassNote(class string) string {
	switch class {
	case "line":
		return "botProviderClass line: one LINE official account cannot host two bots, so replacing an existing one is a cutover - deploy with disabled: true first. " +
			"Both of its keys come from the LINE Developers console for that channel, so somebody outside this repo has to fetch them. Read `.agents/skills/asgard-platform/usecase/chat-channel.md`"
	case "telegram":
		return "botProviderClass telegram: both of its keys come from BotFather, so somebody outside this repo has to fetch them before the first deploy. Read `.agents/skills/asgard-platform/usecase/chat-channel.md`"
	case "discord", "slack":
		return "botProviderClass " + class + ": the operator creates a Connector Pod for this class, because it holds an outbound WebSocket. " +
			"Read `.agents/skills/asgard-platform/usecase/chat-channel.md`"
	default:
		return ""
	}
}

// Resolve fills in the references a kind needs from what the project's chart
// already declares, and returns what it decided so the caller can say so.
//
// It exists because the generator used to write a skeleton that could not pass
// the gate: an Agent hardcoded a SkillSet nothing creates, and ignored the
// SemanticLayer sitting in the same chart. Both are dangling references the
// moment the file is written, and an agent working from the CLI's output alone
// has no way to know that - it followed the instructions and the gate went red.
func Resolve(root string, kind Kind, opts Options) (Options, []string, error) {
	refs, err := chart.Scan(root, opts.Project)
	if err != nil {
		return opts, nil, err
	}

	var notes []string
	layers := chart.NamesOf(refs, "SemanticLayer")

	if kind.Name == "agent" && opts.Layer == "" {
		switch len(layers) {
		case 0:
			// Legal: an Agent may read through Toolsets instead. The skeleton
			// says so, and the gate rejects one with neither.
		case 1:
			opts.Layer = layers[0]
			notes = append(notes, fmt.Sprintf("mounted the chart's only SemanticLayer, %s", opts.Layer))
		default:
			return opts, nil, fmt.Errorf("this chart has %d semantic layers (%s); one agent takes at most one, so name it with --layer",
				len(layers), strings.Join(layers, ", "))
		}
	}

	if kind.Name == "flowagent" {
		if opts.BotClass == "" {
			opts.BotClass = BotClassGeneric
		}
		if !validBotClass(opts.BotClass) {
			return opts, nil, fmt.Errorf("unknown bot class %q; one of: %s",
				opts.BotClass, strings.Join(BotClasses, ", "))
		}
		if note := botClassNote(opts.BotClass); note != "" {
			notes = append(notes, note)
		}
	}

	// A Plugin's skill store is a decision, not something to guess. Defaulting
	// to a name nobody created writes a dangling reference; picking whichever
	// SourceSet happens to exist picks a knowledge Drive about half the time,
	// and the gate then rejects it for a reason that reads as unrelated.
	if kind.Name == "plugin" && opts.Connector == "" {
		return opts, nil, fmt.Errorf(
			"a plugin's SkillSet needs a skill store, and which one is a decision:\n\n"+
				"    asgard-cli add plugin %s --project %s --connector ss-<store>\n\n"+
				"Every plugin in a chart shares ONE store - the skills live in one\n"+
				"repository, so a SourceSet per bundle would clone it per bundle. It is\n"+
				"not a knowledge Drive: that holds documents an agent reads, this holds\n"+
				"skills it loads. If the chart has no store yet, create it with a git\n"+
				"Syncer first. See `.agents/skills/asgard-platform/usecase/plugin.md`",
			opts.Name, opts.Project)
	}

	opts.SkillSets = chart.NamesOf(refs, "SkillSet")

	// A fixed-query Toolset normally holds several tools, so the second and
	// later ones must not re-emit the set. Writing it twice renders two CRs with
	// one name, and whichever applies last takes the other's tools with it.
	if opts.Toolset != "" {
		for _, name := range chart.NamesOf(refs, "Toolset") {
			if name == opts.Toolset {
				opts.ToolsetExists = true
				notes = append(notes, fmt.Sprintf(
					"%s already exists, so only the tool was written; add its entry to that Toolset's tools list", opts.Toolset))
				break
			}
		}
	}

	return opts, notes, nil
}

func Write(root string, kind Kind, opts Options) ([]Result, error) {
	projects, err := repo.Projects(root)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(projects, opts.Project) {
		return nil, fmt.Errorf("no project %q in this repository; it has: %s",
			opts.Project, strings.Join(projects, ", "))
	}
	// Every reference in this CLI's own guidance is written prefixed - "--connector
	// dc-<name>", "--layer sl-<name>" - so the prefixed form is the natural thing
	// to type as the name too. Prefixing it again produced dc-dc-erp in
	// metadata.name, which lints clean and is only found by the xref gate, or by
	// nobody. Accept either form.
	opts.Name = strings.TrimPrefix(opts.Name, kind.Prefix)
	if err := repo.ValidateSlug("name", opts.Name); err != nil {
		return nil, err
	}

	// Defaulted here as well as in Resolve: an empty class renders a CR with no
	// channel and no credentials, and botProviderClass cannot be edited after it
	// is applied.
	if opts.BotClass == "" {
		opts.BotClass = BotClassGeneric
	}

	data := Data{
		Options:   opts,
		Chart:     opts.Project,
		CRName:    kind.Prefix + opts.Name,
		ValuesKey: valuesKey(opts.Name),
		SpecSlug:  repo.SpecSlugIn(root),
	}
	if data.DisplayName == "" {
		data.DisplayName = data.CRName
	}
	if kind.Name == "dataconnector" {
		data.DBSpec = dbSpec(opts.DBClass, data.ValuesKey, data.Chart)
		data.DBValues = dbValues(opts.DBClass, data.DisplayName, data.ValuesKey)
		data.DBNote = dbNote(opts.DBClass)
	}

	templatesDir := filepath.Join(root, "projects", opts.Project, "chart", "app", "templates")

	// Resolve every path before writing any of them, so a kind that would
	// clobber something does not leave half its files behind.
	targets := make([]string, len(kind.Files))
	for i, f := range kind.Files {
		dir, name := f.Dir, data.CRName+".yaml"
		if f.Suffix != "" {
			// A multi-file kind gets its own directory, named for the chain.
			dir = filepath.Join(f.Dir, opts.Name)
			name = f.Suffix + ".yaml"
		}
		targets[i] = filepath.Join(templatesDir, dir, name)

		if _, err := os.Stat(targets[i]); err == nil && !opts.Force {
			return nil, fmt.Errorf("%s already exists; pass --force to overwrite",
				mustRel(root, targets[i]))
		}
	}

	var results []Result
	for i, f := range kind.Files {
		content, err := render(f.Template, data)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(targets[i]), 0o755); err != nil {
			return nil, fmt.Errorf("create directory: %w", err)
		}
		if err := os.WriteFile(targets[i], content, 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", targets[i], err)
		}
		results = append(results, Result{Path: mustRel(root, targets[i]), Created: true})
	}

	if kind.Values != "" {
		added, err := appendValues(templatesDir, kind, data)
		if err != nil {
			return nil, err
		}
		if added != "" {
			results = append(results, Result{Path: mustRel(root, added), Created: true, Values: true})
		}
	}

	return results, nil
}

// appendValues adds the keys this CR reads to the chart's values.yaml, unless
// they are declared already. values.yaml has to default every .Values.* a
// template reads: the lint step of `asgard-cli gate` is what proves it, and
// without it a missing default is masked whenever an env file is overlaid, then
// nil-pointers for anyone running plain helm template.
func appendValues(templatesDir string, kind Kind, data Data) (string, error) {
	chartDir := filepath.Dir(templatesDir)
	path := filepath.Join(chartDir, "values.yaml")

	existing, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read %s: %w", path, err)
	}

	snippet, err := renderInline(kind.Values, data)
	if err != nil {
		return "", err
	}

	// A snippet either introduces its own top-level key, or adds an entry under
	// one that several CRs share. Both have to work, and only the first used to.
	//
	// The bug that came of it: `botProviders:` exists after the first flow
	// agent, so the second one's whole snippet was skipped as "already there",
	// and its template then read `.Values.botProviders.<name>.disabled` off a
	// map with no such entry. Bare `helm lint` catches it - which is exactly
	// what the gate's lint step is for - but only after the file is written.
	top, nested := splitSnippet(string(snippet))
	if top == "" {
		return "", nil
	}

	text := string(existing)
	if !strings.Contains("\n"+text, "\n"+top+":") {
		// New block: append the snippet whole.
		if err := os.WriteFile(path, []byte(text+string(snippet)), 0o644); err != nil {
			return "", fmt.Errorf("write %s: %w", path, err)
		}
		return path, nil
	}

	// The block exists. Add this CR's entry under it, unless it is already there.
	if len(nested) == 0 {
		return "", nil
	}
	entry := strings.TrimSpace(nested[0])
	if strings.Contains(text, "\n  "+entry) {
		return "", nil
	}

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line != top+":" {
			continue
		}
		updated := append([]string{}, lines[:i+1]...)
		updated = append(updated, nested...)
		updated = append(updated, lines[i+1:]...)
		if err := os.WriteFile(path, []byte(strings.Join(updated, "\n")), 0o644); err != nil {
			return "", fmt.Errorf("write %s: %w", path, err)
		}
		return path, nil
	}
	return "", nil
}

// splitSnippet separates a values snippet's top-level key from the indented
// lines under it, dropping the leading comment - a comment naming one CR does
// not belong above a block that several share.
func splitSnippet(snippet string) (top string, nested []string) {
	for _, line := range strings.Split(snippet, "\n") {
		switch {
		case strings.TrimSpace(line) == "":
			continue
		case top == "" && strings.HasPrefix(line, "#"):
			continue
		case top == "":
			key, _, found := strings.Cut(line, ":")
			if !found || key == "" || strings.HasPrefix(line, " ") {
				return "", nil
			}
			top = key
		default:
			nested = append(nested, line)
		}
	}
	return top, nested
}

func renderInline(text string, data Data) ([]byte, error) {
	tmpl, err := template.New("values").Delims("<<", ">>").Funcs(funcs).Parse(text)
	if err != nil {
		return nil, fmt.Errorf("parse values snippet: %w", err)
	}
	var out strings.Builder
	if err := tmpl.Execute(&out, data); err != nil {
		return nil, fmt.Errorf("render values snippet: %w", err)
	}
	return []byte(out.String()), nil
}

// funcs are what a skeleton template may call. A SandboxBlueprint's name lists
// are comma-separated strings in one field, not YAML lists, so joining is not a
// convenience here - it is the field's format.
var funcs = template.FuncMap{
	"join": func(items []string, sep string) string { return strings.Join(items, sep) },
}

func render(name string, data Data) ([]byte, error) {
	raw, err := templates.ReadFile("templates/" + name)
	if err != nil {
		return nil, fmt.Errorf("read template %s: %w", name, err)
	}

	// << >> because the output is a Helm template full of {{ }}.
	tmpl, err := template.New(name).Delims("<<", ">>").Funcs(funcs).Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("parse template %s: %w", name, err)
	}

	var out strings.Builder
	if err := tmpl.Execute(&out, data); err != nil {
		return nil, fmt.Errorf("render %s: %w", name, err)
	}
	return []byte(out.String()), nil
}

func mustRel(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}

// TemplateBodies returns every embedded CR template, keyed by file name.
//
// It exists for `asgard-cli audit-material`, which has to be able to see the
// templates as well as the prose: a platform field that gets renamed is taught
// in three places - a template that writes it, an extract that explains it and
// a stage prompt that mentions it - and a sweep that reads only the prose finds
// two of the three.
func TemplateBodies() (map[string]string, error) {
	entries, err := templates.ReadDir("templates")
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		raw, err := templates.ReadFile("templates/" + e.Name())
		if err != nil {
			return nil, err
		}
		out[e.Name()] = string(raw)
	}
	return out, nil
}

// ValuesBlocks returns each kind's values snippet, which is template text that
// lives in this file rather than in templates/ and would otherwise be invisible
// to the same sweep.
func ValuesBlocks() map[string]string {
	out := make(map[string]string, len(Kinds))
	for _, k := range Kinds {
		if strings.TrimSpace(k.Values) != "" {
			out[k.Name+" (values)"] = k.Values
		}
	}
	return out
}
