// Package cli assembles the asgard-cli command tree.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/check"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/stage"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/version"
)

// NewRootCmd builds the root command. Every call returns a fresh tree so tests
// cannot interfere with one another.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "asgard-cli",
		Short: "Command line tool for Asgard FDE",
		Long: `asgard-cli is what an agent asks about integrating with Asgard, so that
an FDE can walk into a customer's room with it.

BEFORE A MEETING, read the one for what you are about to do:

    .agents/skills/asgard-platform/
      needs/<shape>.md           what they have to give us before we start
      brief/customer-meeting.md  what gets said wrong to a customer
      brief/connect.md           before binding a checkout to the platform
      brief/write-chart.md       before touching a chart
      brief/handover.md          before telling anyone it is live

    asgard-cli guide requirements   the interview, and what to ask for

Talking to a customer changes no file, so nothing derived from the repository
can prepare anybody for it. Read the brief instead: every entry under brief/ is
there because somebody has got it wrong before.

ASKING - what the platform has, which CR a UI name maps to, how one shape is
assembled field by field, and where each has been got wrong before. This is
files, not a command. "asgard-cli init" writes all of it into the
repository, and you read it with cat and grep:

    .agents/skills/asgard-platform/
      index.md    the map, and what is deliberately not there
      aliases.md  what a customer said -> what to search for
      wiki/       what the platform has
      usecase/    how one deployment shape is assembled, field by field
      needs/      what to get from the customer before it can be built
      brief/      what this activity gets wrong
      guide/      which decision to make now

    grep -ril "<term>" .agents/skills/asgard-platform/

Read aliases.md first if the question did not arrive in English. The
material is English and a customer conversation usually is not, so a term
taken from what somebody said may match nothing, which looks the same as a
subject the material does not cover.

With no repository, "asgard-cli init" in an empty directory is enough. It
needs no account and touches no network. The question gets asked in a meeting,
before there is a directory, so that is the first thing to run:

    mkdir -p /tmp/asgard && cd /tmp/asgard && asgard-cli init

One of those stays a command, because it reads the repository you are in as
well as the material:

    asgard-cli guide <name>    one decision, against what this repo has

BUILDING - a chart of Asgard custom resources per project, each deployed to its
own namespace:

    asgard-cli init          onboard a repository: skeleton, binding, material
    asgard-cli project       every chart: its shape and what it declares
    asgard-cli question      what nobody has answered yet, and who each is with
    asgard-cli request       what the customer asked for and is not done
    asgard-cli task          the task specs that are open
    asgard-cli add <kind>    a CR skeleton, wired to what the chart declares
    asgard-cli check         the structure; "verify" is the rendered chart

"project", "question", "request" and "task" each read a file in the
customer's repository back to you, and each takes ` + "`--format json`" + `.
None of them says where the engagement is, and no command does: an
onboarding is not linear, so the repository cannot support a single position
derived from the earliest missing CR kind. What answers "what now" is those
records, plus guidance by name with "asgard-cli guide <name>" or
by grep over the material.

FILING - three places, decided by who has to act on it: the workspace's
Workbench for what the customer's side has to see, answer or supply, or what
is wrong in what is live ("asgard-cli workbench"); this repository's records
for what the next builder needs ("question", "request", "task"); upstream for
a gap in this tool ("asgard-cli issue-report"), which goes to the maintainers
and never carries the customer's content. "asgard-cli workbench --help" has
the table.

Work arrives as a request: one thing the customer wants that the agent cannot do
today. "asgard-cli request add" opens one, and every status the engagement keeps
lives in the customer's repository, never in this tool, so the agent that opens
that repo next can read where the work stands.

With no repository yet, "asgard-cli init" writes one: the skeleton, and nothing
else. It is the one command here written for a person rather than for an agent,
because it runs before there is an agent. The material init writes is what
teaches an agent what a workspace is. Connecting the checkout to a platform
comes after, guided by that agent.

Which workspace and which pipeline are the only two facts a repository cannot
supply about itself, so they are the only two it records. Neither is ever
guessed, even from a list of one.

Run "asgard-cli <command> --help" for details on an individual command.`,
		Version: version.Get().String(),

		// On failure print just the error, not a full page of usage; main is the
		// single place that prints it and picks the exit code.
		SilenceUsage:  true,
		SilenceErrors: true,

		// With no subcommand, show help rather than silently succeeding.
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.SetVersionTemplate("{{.Name}} {{.Version}}\n")

	// --template-dir is persistent because a prompt is read by `guide`,
	// `audit-material` and `init` alike, and an override that applied to only
	// one of them would make the three disagree about what the material says.
	var templateDir string
	cmd.PersistentFlags().StringVar(&templateDir, "template-dir", "",
		"read stage prompts from this directory instead of the embedded copies, per file; for iterating on prompt text")
	// Started here and read in PersistentPostRun, so the question costs the
	// command nothing: see startUpdateCheck.
	var updates updateCheck
	cmd.PersistentPreRunE = func(c *cobra.Command, _ []string) error {
		updates = startUpdateCheck(c)
		if templateDir == "" {
			stage.SetOverrideDir("")
			return nil
		}
		info, err := os.Stat(templateDir)
		if err != nil {
			return fmt.Errorf("--template-dir %s: %w", templateDir, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("--template-dir %s is not a directory", templateDir)
		}
		stage.SetOverrideDir(templateDir)
		// Say it on every run. A prompt that is not the released one is the
		// first thing to suspect when guidance looks wrong, and an override
		// nobody can see is worse than no override.
		fmt.Fprintf(c.ErrOrStderr(), "reading stage prompts from %s where they exist, embedded otherwise\n", templateDir)
		return nil
	}

	// Said at the end of whatever was run, because it is the only moment an
	// agent is looking. Its reference material going stale has no symptom - the
	// CR it writes is wrong in a way that reads fine - so the report has to
	// ride on something already being run rather than wait to be asked for.
	//
	// PersistentPostRun rather than a call in each command: every command that
	// reaches the platform is a chance to say it, and one that has to be added
	// per command is one that gets forgotten on the command somebody adds next.
	cmd.PersistentPostRun = func(c *cobra.Command, _ []string) {
		warnIfBehind(c)
		// The same argument, for the half of the material that comes from this
		// binary rather than from a platform - and this one needs no platform
		// to have been reached, so unlike the line above it is not silent on
		// the commands that never leave the machine.
		warnIfShippedStale(c)
		// And the same argument once more for the binary itself. Everything
		// above is about material this binary put in a repository; this one is
		// about the binary, which nothing in a repository can be behind.
		warnIfNewerRelease(c, updates)
	}

	// The command tree, grouped. **The grouping is the source, not a rendering
	// of it**: a command is added by naming the group it belongs to, so adding
	// one without deciding where it goes does not compile. A flat list of
	// thirty commands is what this help used to be, and it is what made four
	// commands that compose look like four commands that compete.
	// Insertion order, not alphabetical. Within a group the first entry is
	// where somebody starts - `gate` before the checks it composes,
	// `login` before what needs a session - and alphabetical ordering put
	// `check` and `doctor` in front of `gate`, which is the opposite of what
	// the grouping was for.
	cobra.EnableCommandSorting = false

	cmd.AddGroup(
		&cobra.Group{ID: groupAsk, Title: "Ask - the platform itself is files under .agents/skills/; these are the rest:"},
		&cobra.Group{ID: groupBuild, Title: "Build - write the repository and the CRs in it:"},
		&cobra.Group{ID: groupCheck, Title: "Check - everything this machine can check:"},
		&cobra.Group{ID: groupDeploy, Title: "Deploy - the platform, and what it knows:"},
	)

	addTo(cmd, groupAsk,
		newGuideCmd(),
		newSizeCmd(),
		newLinksCmd(),
		newIssueCmd(),
	)
	addTo(cmd, groupBuild,
		newInitCmd(),
		newProjectCmd(),
		newAddCmd(),
		newLocalEnvCmd(),
		newQuestionCmd(),
		newRequestCmd(),
		newTaskCmd(),
		newDecisionCmd(),
		newReferenceCmd(),
	)
	addTo(cmd, groupCheck,
		newGateCmd(),
		newCheckCmd(),
		newRenderCmd(),
		newVerifyCmd(),
		newDoctorCmd(),
	)
	addTo(cmd, groupDeploy,
		newLoginCmd(),
		newLogoutCmd(),
		newWhoamiCmd(),
		newProfileCmd(),
		newWorkspaceCmd(),
		newWorkbenchCmd(),
		newAuditLogCmd(),
		newPipelineCmd(),
		newSkillCmd(),
	)

	// Ungrouped, and they belong there. `version` and `update` answer a
	// question about the binary rather than about an engagement, and
	// `audit-material` is hidden - its reader edits this material, and the
	// help belongs to whoever is onboarding a customer.
	cmd.AddCommand(
		newVersionCmd(),
		newUpdateCmd(),
		newAuditCmd(),
	)

	// What this build answers to, handed to `check` so it can report a command
	// name in a customer repository that no longer exists. The list has to come
	// from the tree rather than from a constant: a constant is a second copy
	// that goes stale exactly when a command is renamed, which is the failure
	// this exists to catch.
	check.SetKnownCommands(commandNames(cmd))
	check.SetReplacements(replacements)

	return cmd
}

// The groups the top-level help is organised into.
//
// They are the questions somebody arrives with, in the order they arrive:
// what is this platform, how do I write the repository, is what I wrote sound,
// and get it deployed. A command that fits none of them is a command whose
// place in the tool has not been decided.
//
// **The first group is nearly empty and its heading says why.** Most of the
// answer to "what is this platform" is files rather than commands, so what is
// left under Ask is the ones that are not: guidance read against this
// repository, a count taken off production, and the way back when the files
// have no answer.
const (
	groupAsk    = "ask"
	groupBuild  = "build"
	groupCheck  = "check"
	groupDeploy = "deploy"
)

// addTo registers commands into a group.
//
// It exists so the grouping cannot be forgotten: cobra panics on a GroupID
// naming a group the parent does not have, and a command added through
// AddCommand instead of this lands in "Additional Commands" where the next
// reader will see it and ask why.
func addTo(parent *cobra.Command, group string, children ...*cobra.Command) {
	for _, c := range children {
		c.GroupID = group
		parent.AddCommand(c)
	}
}

// replacements says what to type instead of a command this tool removed.
//
// **Add a row here when you rename or remove one.** Every repository already
// scaffolded carries the old name in files `scaffold` will never overwrite, and
// `asgard-cli check` reports them - but a reader told only that a command is
// gone has to find out what replaced it, and the first one to hit this had to
// ask a maintainer. That is the answer living in a conversation instead of in
// the binary.
//
// **It is also the closed set `audit-material --commands` sweeps for.** A bare
// name in the material - guidance "reached by subject through find" - claims a
// command exists without writing `asgard-cli` in front of it, so the
// invocation check cannot see it. Looking for arbitrary bare words would fail
// the build over English; looking only for names in this map cannot. So a row
// missing here is two failures, not one: a customer told nothing, and a sweep
// that stops looking.
var replacements = map[string]string{
	"find": "The material is files now. `asgard-cli init` writes it into " +
		"`.agents/skills/asgard-platform/` and `grep -ril \"<term>\" .agents/skills/asgard-platform/` is the way in - " +
		"read `aliases.md` there first if the question did not arrive in English",
	"wiki":    "`cat .agents/skills/asgard-platform/wiki/<name>.md`, or grep the directory. `asgard-cli init` writes it, and needs no account and no network",
	"usecase": "`cat .agents/skills/asgard-platform/usecase/<shape>.md`, or grep the directory",
	"brief":   "`cat .agents/skills/asgard-platform/brief/<activity>.md` - they are `customer-meeting`, `connect`, `write-chart` and `handover`",
	"needs":   "`cat .agents/skills/asgard-platform/needs/<shape>.md`, one file per deployment shape",
	"reading": "Gone with the reading list. What to read before an activity is `.agents/skills/asgard-platform/brief/<activity>.md`; " +
		"what a document points at is in the document",
	"scaffold": "Folded into `asgard-cli init`, which writes the skeleton and the material together. " +
		"`--force` there is what re-takes a file this CLI owns",
	"project shape": "Gone with `.asgard-config.json`. It recorded what a chart was being built to be, which is a claim about intent that nothing can verify - " +
		"say it in the chart, next to whatever makes the project unusual, where the next reader is already looking. `asgard-cli size` still lists the shapes",
	"next": "It derived a position from the earliest missing CR kind and there is no replacement for that, deliberately - an onboarding is not linear. " +
		"Where it meant \"what is still open\", `asgard-cli question`; where it meant \"what does each chart declare\", `asgard-cli project`; " +
		"`next --stage <name>` is `asgard-cli guide <name>`, and `next --list` is `asgard-cli guide` with no argument",
	"status": "Where it meant \"what is still open\", `asgard-cli question`, `asgard-cli request` and `asgard-cli task`; " +
		"where it meant \"what does each chart declare and still lack\", `asgard-cli project`. It also named the guidance the " +
		"repository's shape made relevant, and nothing replaces that: read one with `asgard-cli guide <name>` or grep the material in `.agents/skills/asgard-platform/`",
}

// commandNames returns every name and alias in the tree, one level deep.
//
// One level is deliberate. `asgard-cli request add` names the command
// `request`, and whether `add` is one of its subcommands is a different
// question - a wrong subcommand is a typo, a wrong command is a rename nobody
// was told about.
func commandNames(root *cobra.Command) []string {
	var out []string
	for _, c := range root.Commands() {
		out = append(out, c.Name())
		out = append(out, c.Aliases...)
	}
	return out
}
