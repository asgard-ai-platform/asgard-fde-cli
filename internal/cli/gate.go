package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/auth"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/binding"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/check"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/gate"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/gitrepo"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/pipelineconfig"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/render"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/repo"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/scaffold"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/skills"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/tool"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/version"
)

// `asgard-cli gate` is the one command to run after changing anything.
//
// **It exists because a checklist in prose is not a gate.** Everything it runs
// was already here - `check`, `helm lint`, `render`, `verify`, and now the
// freshness of the reference material - and the only thing that assembled them
// was an AGENTS.md section, a stage prompt and whatever an agent happened to
// remember. Every one of those is a place the list can go stale, and one of
// them did: the scaffolded gate described four steps, the fourth of which ran a
// python script that no longer existed.
//
// An agent working in a compiled language does not have this problem. Whatever
// it changed, it knows to run the build, and the build is one command whose
// definition lives with the code. This is that command.
//
// **What it deliberately does not do is reproduce the platform's checks.** A
// CR's admission is decided by an apiserver, and no client is ever given
// credentials for one; a second copy of those rules here would disagree with
// the server the first time either changed, while still missing the ones that
// matter most. The last line of output says so and names the plan.

type stepStatus string

const (
	stepPass stepStatus = "pass"
	stepFail stepStatus = "fail"
	stepWarn stepStatus = "warn"
	stepSkip stepStatus = "skip"
)

// stepResult is one step's outcome.
//
// A skip is not a pass, and the distinction is the whole reason this is a
// struct rather than a boolean: "kubectl was not installed so the cluster check
// did not run" was reported as green for long enough to reach a customer, in
// the gate this replaces.
type stepResult struct {
	Name    string     `json:"name"`
	Status  stepStatus `json:"status"`
	Summary string     `json:"summary"`
	Details []string   `json:"details,omitempty"`
	// Remedy is the command that fixes it, when one command does.
	Remedy string `json:"remedy,omitempty"`
}

func newGateCmd() *cobra.Command {
	var (
		format  string
		offline bool
		profile string
	)

	cmd := &cobra.Command{
		Use:   "gate [release ...]",
		Short: "Everything this machine can check, in one command",
		Long: `Everything this machine can check about this repository, in one command.

    asgard-cli gate                  every release the declaration names
    asgard-cli gate internal-dev     one of them
    asgard-cli gate --offline        skip the steps that need the platform
    asgard-cli gate --format json    one record per step, for an agent

The first line says which platform this run's verdict is about, and how that
profile came to be the one in effect. Some of the steps ask a platform, so check
that line before trusting the verdict. With ` + "`--offline`" + `, or when not signed in, the
line says so rather than being left out.

Run it after changing anything under a chart or ` + "`" + pipelineconfig.FileName + "`" + `. It is
this repository's build step: one command that runs ` + "`check`" + `, ` + "`helm lint`" + `,
` + "`render`" + `, ` + "`verify`" + ` and the check on the reference material's freshness.

What it runs, in order:

  tools    helm is on PATH. Without it the chart steps cannot run, and
           they are reported as skipped rather than passed
  repo     the structural invariants a chart render cannot see
  shipped  whether AGENTS.md and the design-time skills are still what this
           binary carries. It is the one freshness check that is always
           answerable: it compares against the binary rather than a platform,
           so no session and no --offline can skip it. Some of its states
           warn rather than fail - a shipped file somebody here changed on
           purpose, and material written by a NEWER CLI than the one running,
           which is a fact about the install rather than about the repository
  binding  whether .asgard-cli.yaml names a workspace and a pipeline that the
           platform still has. It is the step that catches a half-bound
           checkout - ` + "`workspace use`" + ` clears the pipeline line, and this goes
           red rather than waiting for whichever command somebody runs next.
           The verdict is platform facts only, and no git remote is compared
           to any of them. The one git fact reported is the origin remote of
           a checkout with no binding at all, because that is what
           ` + "`pipeline connect`" + ` and ` + "`pipeline create`" + ` derive their account
           and repository from - and an agent that cannot see whether there
           is one asks for what the tool would have derived.
           Needs a session; --offline skips the half that asks
  skills   whether the reference material here still describes the server this
           repository deploys to. Needs a session; --offline skips it
  lint     helm lint on each chart, with the reserved asgard block and nothing
           else. That is what proves values.yaml declares a default for every
           .Values.* the chart itself owns; overlay an environment file and a
           missing default is masked until somebody runs plain helm template.
           Linting with no values file at all fails on
           every chart that reads .Values.asgard.*, which a chart must not
           declare and the platform always injects
  render   each release renders, with placeholder platform values
  verify   the rendered CRs against each other: dangling references between
           CRs and inside a SemanticLayer (a join's cube and dimensions,
           primaryKeyDimensions), both halves of every entrypoint, missing
           display annotations, the workflow-set labels, the agent-split
           invariants

A skip is not a pass, and the two are printed differently.

This command is step 1 of the deploy loop, the local step:

    1. locally      asgard-cli gate                 <- documented here
    2. push         a tag or branch matching a release's trigger
    3. the plan     the platform renders it for real and checks it
    4. review       approve or reject
    5. apply

Steps 2 to 5 belong to the platform, and the platform describes them: the
` + "`asgard-cr-verification`" + ` skill that ` + "`asgard-cli skill update`" + ` fetches lists the
run steps, every rule code and what each means. It says nothing about step 1,
because a server cannot know which version of this binary somebody installed -
an on-prem customer's CLI can be several releases from the platform's in either
direction. This help describes what step 1 checks.

Do not run helm by hand here. The platform injects a reserved ` + "`asgard`" + ` block
into every render, and a chart must not declare it in its own values.yaml - so
` + "`helm lint <chart>`" + ` with no -f fails on every chart that reads
` + "`.Values.asgard.projectEnvironmentId`" + `, which is every chart that labels
anything. That failure does not mean the chart is broken. This
supplies that one file and nothing else, which is why the lint step still
proves that values.yaml defaults everything the chart itself owns.

It does not reproduce the platform's checks. Whether a CR
is admitted is decided by an apiserver, and no client is ever given credentials
for one - so a copy of those rules here would drift from the server the first
time either changed, while still missing the two that matter most: a field the
CRD silently prunes, and a rejection only the apiserver can produce. A green
gate means "worth pushing", not "this will deploy". The plan decides:

    asgard-cli pipeline runs watch --release <name> --ref <tag>

Exits non-zero if any step failed.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFormat(format); err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			// The declaration is what anchors a Pipeline repository, and it is
			// the one file every one of them has.
			root, err := repoRoot()
			if err != nil {
				return err
			}
			releases, err := releasesToVerify(root, args)
			if err != nil {
				return err
			}

			steps := []stepResult{gateTools()}
			helmReady := steps[0].Status == stepPass

			steps = append(steps, gateRepo(root, args))
			steps = append(steps, gateShipped(root))
			steps = append(steps, gateBinding(cmd, root, profile, offline))
			steps = append(steps, gateSkills(cmd, profile, offline))
			steps = append(steps, gateCharts(cmd, root, releases, helmReady)...)

			against := gateAgainst(profile, offline)

			if format == formatJSON {
				if err := writeJSON(out, map[string]any{
					"ok":       stepsOK(steps),
					"platform": against,
					"steps":    steps,
				}); err != nil {
					return err
				}
				if !stepsOK(steps) {
					return ErrSilent
				}
				return nil
			}
			// Before the steps, because a verdict is only a verdict about
			// something: some of these steps ask a platform, and reading their
			// answer against the wrong one is the failure this line exists for.
			fmt.Fprintf(out, "against   %s\n\n", against)
			printSteps(out, steps)
			if !stepsOK(steps) {
				return ErrSilent
			}
			return nil
		},
	}

	addProfileFlag(cmd, &profile)
	cmd.Flags().StringVar(&format, formatFlag, formatText, formatUsage)
	cmd.Flags().BoolVar(&offline, "offline", false, "skip the steps that need the platform: binding and skills")
	return cmd
}

// gateAgainst is the one line at the top saying what this run's verdict is a
// verdict about.
//
// **`--offline` and "not signed in" get a sentence rather than silence.** The
// platform steps report themselves as skipped, but the header is where
// somebody looks to know whether the run means anything about a platform at
// all, and an empty header there would read as "no platform involved" rather
// than "the platform was not asked".
func gateAgainst(profile string, offline bool) string {
	if offline {
		return "nothing - --offline, so no platform was asked"
	}
	r, err := auth.ResolveWithOrigin(profile)
	if err != nil {
		return "no resolvable profile, so no platform was asked"
	}
	return fmt.Sprintf("%s  (profile %s, from %s)", r.PlatformAPI, r.Name, r.NameFrom)
}

func stepsOK(steps []stepResult) bool {
	for _, s := range steps {
		if s.Status == stepFail {
			return false
		}
	}
	return true
}

func printSteps(out io.Writer, steps []stepResult) {
	failed, skipped := 0, 0
	for _, s := range steps {
		fmt.Fprintf(out, "%-9s %-5s %s\n", s.Name, s.Status, s.Summary)
		for _, d := range s.Details {
			fmt.Fprintf(out, "  %s\n", wrapAt(d, 76, 2))
		}
		if s.Remedy != "" {
			// Wrapped like the details above it. A remedy is usually one
			// command and fits, but the one that names a branch of the connect
			// is a sequence, and a sequence that runs off the terminal is read
			// as far as the edge and no further.
			//
			// **A newline in a remedy is the author saying it is already
			// broken**, and every line after the first is printed as it was
			// written. `wrapAt` reflows on `strings.Fields`, so a line it
			// touches loses its indent and can be split mid-command - the
			// install one-liner came out with its `| sh` alone on the next
			// line, which is not a command anybody can copy.
			lines := strings.Split(s.Remedy, "\n")
			fmt.Fprintf(out, "  -> %s\n", wrapAt(lines[0], 73, 5))
			for _, l := range lines[1:] {
				fmt.Fprintf(out, "     %s\n", strings.TrimLeft(l, " "))
			}
		}
		switch s.Status {
		case stepFail:
			failed++
		case stepSkip:
			skipped++
		}
	}

	fmt.Fprintln(out)
	switch {
	case failed > 0:
		fmt.Fprintf(out, "%d of %d step(s) failed.\n", failed, len(steps))
	case skipped > 0:
		fmt.Fprintf(out, "Nothing failed, and %d step(s) did not run. A skip is not a pass.\n", skipped)
	default:
		fmt.Fprintf(out, "All %d steps passed.\n", len(steps))
	}
	// Said every time, because a green local gate is the moment somebody is
	// most likely to believe the work is finished.
	fmt.Fprintf(out, "This is what a machine with no cluster can check. The plan checks the rest:\n"+
		"    asgard-cli pipeline runs watch --release <name> --ref <tag>\n")
}

// ── the steps ────────────────────────────────────────────────────────────

// newerWriters lists the versions this repository's record names that are not
// the running one, newest first.
//
// **Read off disk, so it costs nothing and works on a plane.** The record says
// which CLI version wrote each shipped file, so a repository somebody has run a
// newer binary in already carries the answer - the one case where this tool can
// say a newer release exists without asking anybody.
func newerWriters(root string) []string {
	writers, err := scaffold.Writers(root)
	if err != nil {
		return nil
	}
	running := version.Get().Version
	var out []string
	for _, w := range writers {
		if w != "" && w != running {
			out = append(out, w)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out
}

func gateTools() stepResult {
	if _, err := tool.Helm.Path(); err != nil {
		var missing *tool.ErrMissing
		remedy := "asgard-cli doctor"
		if errors.As(err, &missing) {
			remedy = missing.Tool.InstallHint()
		}
		return stepResult{
			Name:    "tools",
			Status:  stepFail,
			Summary: "helm is not on PATH, so nothing that touches a chart can run",
			Remedy:  remedy,
		}
	}
	return stepResult{Name: "tools", Status: stepPass, Summary: "helm is on PATH"}
}

func gateRepo(root string, only []string) stepResult {
	// AGENTS.md is what `scaffold` always writes, and it is the marker `stage`
	// already uses for "this repository has a skeleton". Without one there is
	// no skeleton to hold the repository against - the docs layers, the README
	// project table, the requirements indexes are all things `scaffold` writes
	// - and reporting their absence as six failures would be reporting that a
	// repository is not something it never claimed to be.
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err != nil {
		return stepResult{
			Name:    "repo",
			Status:  stepSkip,
			Summary: "no AGENTS.md, so this repository has no skeleton to check",
			Remedy:  "asgard-cli init",
		}
	}
	report, err := check.Run(root, only...)
	if err != nil {
		return stepResult{Name: "repo", Status: stepFail, Summary: err.Error()}
	}
	res := stepResult{Name: "repo", Status: stepPass}
	for _, w := range report.Warnings() {
		res.Details = append(res.Details, "warn  "+w.Message)
	}
	errs := report.Errors()
	for _, e := range errs {
		res.Details = append(res.Details, "FAIL  "+e.Message)
	}
	switch {
	case len(errs) > 0:
		res.Status = stepFail
		res.Summary = fmt.Sprintf("%d problem(s) in the repository's structure", len(errs))
	case len(report.Warnings()) > 0:
		res.Status = stepWarn
		res.Summary = fmt.Sprintf("%d warning(s)", len(report.Warnings()))
	default:
		res.Summary = "structure and declaration are consistent"
	}
	return res
}

// gateBinding checks that this checkout names a workspace and a pipeline the
// platform still has.
//
// **It is here because a half-bound checkout has no other symptom.**
// `workspace use <other>` clears the pipeline line, deliberately - a pipeline
// belongs to one workspace - and if the agent that ran it does not go on to
// `pipeline use`, nothing is wrong until somebody runs a pipeline command,
// which might be `runs approve`. This is a command an agent already runs after
// every change, so the half state surfaces at the next edit instead.
//
// **The verdict is platform facts and nothing else.** It does not compare the
// pipeline's repository to a git remote: a checkout may have several remotes,
// and which one is called `origin` is not this tool's business. The gap that
// leaves - a repository copied wholesale within one workspace - is named in the
// binding file's own header.
//
// **The one git fact it reports decides nothing**, and it is reported only
// where there is no binding to reach a verdict about. What comes next there is
// `pipeline connect` and `pipeline create`, both of which derive their account
// and their repository from `origin` - so an agent that cannot see whether
// there is one asks for what the tool would have derived, or, the expensive
// half, asks which repository this is and binds the pipeline to a guess.
func gateBinding(cmd *cobra.Command, root, profile string, offline bool) stepResult {
	res := stepResult{Name: "binding"}

	f, err := binding.LoadFrom(root)
	switch {
	case errors.Is(err, binding.ErrNotFound):
		// Nothing recorded is the ordinary state of a repository nobody has
		// bound yet, and the same reasoning as the repo step applies: this is
		// not a repository failing to be something it never claimed to be.
		res.Status = stepSkip
		res.Summary = "no " + binding.FileName + ", so this checkout is not bound to a pipeline yet"
		// One string per detail, not pre-broken lines: this step wraps its own
		// details, and pre-broken ones come out broken in a different place.
		detail, remedy := gateOrigin(cmd.Context(), root)
		if detail != "" {
			res.Details = append(res.Details, detail)
		}
		res.Remedy = remedy
		res.Details = append(res.Details,
			"`asgard-cli init` writes the skeleton and does not connect it to the platform, so it needs no "+
				"account. Connect it as a separate step; `asgard-cli pipeline create` makes the pipeline "+
				"when there is none yet.")
		return res
	case err != nil:
		res.Status = stepFail
		res.Summary = err.Error()
		return res
	}

	// The local half runs offline and runs first: a missing field is a fact
	// about the file, and asking the platform about it would be asking the
	// wrong question.
	if missing := f.Missing(); len(missing) > 0 {
		res.Status = stepFail
		res.Summary = fmt.Sprintf("%s records no %s", binding.FileName, strings.Join(missing, " and no "))
		if len(missing) == 1 && missing[0] == "pipeline" {
			res.Details = append(res.Details,
				"a workspace is recorded and a pipeline is not, which is what `asgard-cli workspace use` leaves behind")
			res.Remedy = "asgard-cli pipeline list, then asgard-cli pipeline use <id>"
			return res
		}
		res.Remedy = "asgard-cli workspace use <id>, then asgard-cli pipeline use <id>"
		return res
	}

	if offline {
		res.Status = stepSkip
		res.Summary = "--offline, so the platform was not asked whether these still exist"
		return res
	}

	pc, err := resolveContext(cmd, contextOptions{Profile: profile})
	if err != nil {
		res.Status = stepSkip
		res.Summary = "no session, so the platform was not asked (`asgard-cli login`, or --offline to say so on purpose)"
		return res
	}

	// The recorded workspace, not the resolved one. --workspace and
	// ASGARD_WORKSPACE outrank the file everywhere else on purpose, and here
	// the file is the thing being checked.
	workspaces, err := platform.New(pc.Session, "").ListWorkspaces(cmd.Context())
	if err != nil {
		res.Status = stepSkip
		res.Summary = fmt.Sprintf("the platform did not answer: %v", err)
		return res
	}
	wsName := ""
	found := false
	for _, w := range workspaces {
		if w.ID == f.Workspace {
			wsName, found = w.Name, true
			break
		}
	}
	if !found {
		res.Status = stepFail
		res.Summary = fmt.Sprintf("workspace %s is not one this account can reach on %s", f.Workspace, pc.Session.Profile.Name)
		for _, w := range workspaces {
			res.Details = append(res.Details, fmt.Sprintf("      %-22s %s", w.ID, w.Name))
		}
		res.Remedy = "asgard-cli workspace list"
		return res
	}

	pipelines, err := platform.New(pc.Session, f.Workspace).ListPipelines(cmd.Context())
	if err != nil {
		res.Status = stepFail
		res.Summary = fmt.Sprintf("workspace %s exists, and its pipelines could not be listed: %v", f.Workspace, err)
		return res
	}
	for _, p := range pipelines {
		if p.PipelineId == f.Pipeline {
			res.Status = stepPass
			res.Summary = fmt.Sprintf("%s -> %s (%s), pipeline %s", binding.FileName, f.Workspace, wsName, p.Name)
			return res
		}
	}

	res.Status = stepFail
	res.Summary = fmt.Sprintf("pipeline %s is not in workspace %s (%s)", f.Pipeline, f.Workspace, wsName)
	for _, p := range pipelines {
		res.Details = append(res.Details, fmt.Sprintf("      %-22s %-20s %s", p.PipelineId, p.Name, p.RepoFullName))
	}
	res.Remedy = "asgard-cli pipeline use <id>"
	return res
}

// gateOrigin is the first branch of connecting a checkout, reported at the one
// moment an agent is about to take it: `origin`, or the absence of one.
//
// **Both branches are common and they lead to different sessions.** A
// repository handed over already wired needs no question asked - `pipeline
// connect` and `pipeline create` derive their account and their repository
// from the remote and say so. A fresh `asgard-cli init` has nothing, and the
// question that fills the gap is **one**: the remote URL, which answers the
// account, the repository name and whether it exists at once. Asking those
// separately gets one of them guessed, and the name a pipeline binds is the
// one the engagement carries afterwards.
//
// It returns the remedy as well as the detail, because the remedy is the half
// that changes: with no remote, signing in is not the next thing to do.
//
// Everything here degrades the way `gitrepo` does. No git, no checkout, or a
// remote in a shape this does not read all come back as no detail at all and
// the ordinary remedy, because a gate that guesses about somebody's checkout
// is worse than one that says nothing about it.
func gateOrigin(ctx context.Context, root string) (detail, remedy string) {
	const connect = "asgard-cli login, then workspace use <id> and pipeline use <id>"

	// Not a checkout at all is a different sentence, and `asgard-cli init`
	// already says it while somebody is still at the terminal. Telling a
	// directory that is not a repository to add a remote to it is a step out
	// of order.
	if _, err := gitrepo.Root(ctx, root); err != nil {
		return "", connect
	}
	remoteURL, err := gitrepo.OriginURL(ctx, root)
	if errors.Is(err, gitrepo.ErrNoOrigin) {
		return "no `origin` remote, so `pipeline connect` and `pipeline create` have nothing to " +
				"derive an account or a repository from. Ask for this repository's remote URL, " +
				"then set it and push. Push, not only add: the pipeline reads " +
				"its declaration off the default branch, and a repository with no commits fails its " +
				"first config sync.",
			"git remote add origin <url> and push, then " + connect
	}
	if err != nil {
		return "", connect
	}
	// **The URL itself is never printed**, only what was read out of it. A
	// remote may carry a token in its userinfo - `https://x:<token>@host/o/n`
	// is an ordinary thing to find in a checkout - and gate output is pasted
	// into issues. `FullName` and `RemoteHost` both drop it.
	host := gitrepo.RemoteHost(remoteURL)
	full, ok := gitrepo.FullName(remoteURL)
	if !ok {
		where := "this checkout's origin remote"
		if host != "" {
			where = "origin, on " + host + ","
		}
		return where + " is not a shape this reads as owner/name, so `pipeline connect` " +
			"and `pipeline create` need --account and --repo given.", connect
	}
	return fmt.Sprintf("origin is %s on %s, and `pipeline connect` and `pipeline create` derive --account "+
		"and --repo from it. Neither has to be asked for or passed.", full, host), connect
}

// gateShipped asks whether the material this CLI ships into a repository is
// still what this binary carries: AGENTS.md and the design-time skills.
//
// **It is the freshness step that cannot be turned off.** `skills` below needs
// a session and skips on --offline, because the version it compares against is
// the platform's. This one compares against the binary it is part of, so it
// needs no network, no session and no binding - and it is answerable in exactly
// the situations where the other is not.
//
// **It matters more, not less, once this binary can update itself.** The
// platform's half moves when somebody publishes; this half moves when the
// binary is replaced, which with a self-update happens between two commands and
// is not something anybody ran. Until this step existed the only thing that
// ever reported it was a re-run of `asgard-cli init` - a command documented as
// the one written for a person, which nothing re-runs on a schedule.
//
// **Some states warn rather than fail.** `edited` is somebody here having
// changed a shipped file, which the scaffolded AGENTS.md invites by shipping a
// project list of TODO rows, and a gate that goes red on an engagement
// answering one is a checker crying wolf - which this material has caused once
// already. `ahead` is this binary being older than the material, which is a
// fact about the install rather than a defect in the repository, and the
// repository is what this command returns a verdict on. A warn is neither
// hidden nor counted against the exit code.
func gateShipped(root string) stepResult {
	res := stepResult{Name: "shipped"}

	projects, err := repo.Projects(root)
	if err != nil {
		res.Status = stepFail
		res.Summary = err.Error()
		return res
	}
	results, err := scaffold.InspectShipped(root, projects)
	if err != nil {
		res.Status = stepFail
		res.Summary = err.Error()
		return res
	}

	by := map[scaffold.Status][]string{}
	for _, r := range results {
		if r.Status != scaffold.Skipped {
			by[r.Status] = append(by[r.Status], r.Path)
		}
	}
	for _, st := range []scaffold.Status{
		scaffold.Missing, scaffold.Retired, scaffold.Behind, scaffold.Stale,
		scaffold.Edited, scaffold.Ahead, scaffold.Updated,
	} {
		for _, path := range by[st] {
			// No column padding: printSteps sends every detail through wrapAt,
			// which splits on strings.Fields and so collapses any run of
			// spaces. One word then one path is what survives.
			res.Details = append(res.Details, fmt.Sprintf("%s %s", st, path))
		}
	}

	// Ordered by what the reader has to do about it, so the summary names the
	// worst thing rather than the first.
	switch {
	case len(by[scaffold.Missing]) > 0:
		res.Status = stepFail
		res.Summary = fmt.Sprintf("%d shipped file(s) are not here, so an agent working here reads no copy of them",
			len(by[scaffold.Missing]))
		res.Remedy = "asgard-cli init"
	case len(by[scaffold.Retired]) > 0:
		res.Status = stepFail
		res.Summary = fmt.Sprintf("%d file(s) were shipped here by an asgard-cli that carried them and this one does not",
			len(by[scaffold.Retired]))
		res.Remedy = "read them and delete them; nothing compares them against anything any more"
	case len(by[scaffold.Behind]) > 0:
		res.Status = stepFail
		res.Summary = fmt.Sprintf("%d file(s) are older than what this CLI carries, and nobody here has touched them",
			len(by[scaffold.Behind]))
		res.Remedy = "asgard-cli init --force"
	case len(by[scaffold.Stale]) > 0:
		res.Status = stepFail
		res.Summary = fmt.Sprintf("%d file(s) differ from what this CLI carries, and which way round is not knowable",
			len(by[scaffold.Stale]))
		res.Remedy = "asgard-cli init --force, after reading the diff"
	}
	if res.Status == stepFail {
		return res
	}

	switch {
	case len(by[scaffold.Ahead]) > 0:
		res.Status = stepWarn
		res.Summary = fmt.Sprintf("%d file(s) here were written by a NEWER asgard-cli than this one (%s)",
			len(by[scaffold.Ahead]), version.Get().Version)
		// **Name the version and give the command.** This is the one place that
		// knows a newer binary exists without asking anything over a network -
		// the record says which version wrote each file - and a remedy saying
		// "upgrade" without saying to what, or how, leaves the reader to find
		// both.
		res.Remedy = "upgrade asgard-cli; the repository is fine and this binary is behind it"
		if ahead := newerWriters(root); len(ahead) > 0 {
			res.Remedy = fmt.Sprintf(
				"this repository was written by %s; the repository is fine and this binary is behind it:\n    %s",
				strings.Join(ahead, ", "), upgradeCommand())
		}
	case len(by[scaffold.Edited]) > 0:
		res.Status = stepWarn
		res.Summary = fmt.Sprintf("%d shipped file(s) were changed here; the rest is what this CLI carries",
			len(by[scaffold.Edited]))
	default:
		res.Status = stepPass
		writers, err := scaffold.Writers(root)
		switch {
		case err != nil:
			res.Summary = "matches this CLI"
		case len(writers) == 0:
			res.Summary = "matches this CLI, which has not recorded writing any of it"
		default:
			res.Summary = fmt.Sprintf("matches this CLI, written by %s", strings.Join(writers, ", "))
		}
	}
	return res
}

// gateSkills asks whether the reference material in this repository still
// describes the server it deploys to.
//
// **It is a step of the gate rather than a reminder**, because material going
// stale has no symptom: the CR an agent writes against it is wrong in a way
// that reads perfectly, applies cleanly, and fails at run time. That is the
// same class as the checks below it, and it belongs in the same command.
//
// Not having a session is a skip, not a failure. A gate that only works online
// is a gate that fails on a plane, and every other step here works offline.
func gateSkills(cmd *cobra.Command, profile string, offline bool) stepResult {
	res := stepResult{Name: "skills", Remedy: "asgard-cli skill update"}

	// **Two of this step's facts need no platform, so they are established
	// before --offline is honoured.** Which directory the material is read
	// from, and whether git will carry it, are answerable from the checkout
	// alone - and a repository holding two copies is the state where an agent
	// is reading stale material right now, which is not a thing to defer to
	// whenever somebody next runs this online.
	root, repoRoot, err := skillRoot(cmd, "")
	if err != nil {
		res.Status = stepSkip
		res.Summary = err.Error()
		return res
	}
	if others := skills.Elsewhere(repoRoot, root); len(others) > 0 {
		res.Status = stepFail
		res.Summary = fmt.Sprintf("%d other directory(s) here also hold fetched material, and an agent's runtime reads them too",
			len(others))
		for _, o := range others {
			res.Details = append(res.Details,
				fmt.Sprintf("%s version %s, %d file(s), not the directory in use", o.Dir, o.Version, o.Files))
		}
		res.Remedy = "read the copy that is not in use and delete it, or `asgard-cli skill update --dir <it>`"
		return res
	}
	// Appended to whatever this step concludes rather than deciding it: the
	// material can be perfectly current and still be in a directory a clone
	// will not receive, and those are two different things to fix.
	ignoredNote := skillsDirIgnoredNote(cmd, repoRoot, root)

	res = gateSkillsAgainstPlatform(cmd, profile, offline, root)
	res.Details = appendNote(res.Details, ignoredNote)
	return res
}

// gateSkillsAgainstPlatform is the half of the skills step that needs the
// platform: which version it serves, against which version is here.
//
// Split out so the note about the directory can be appended to whatever this
// concludes. It was a `defer` on the caller for one commit, which did nothing:
// the return value is not named, so `return res` copies the struct before the
// deferred append reaches it.
func gateSkillsAgainstPlatform(cmd *cobra.Command, profile string, offline bool, root string) stepResult {
	res := stepResult{Name: "skills", Remedy: "asgard-cli skill update"}
	if offline {
		res.Status = stepSkip
		res.Summary = "--offline, so the platform was not asked"
		res.Remedy = ""
		return res
	}
	stamp, err := skills.ReadStamp(root)
	if err != nil {
		res.Status = stepFail
		res.Summary = err.Error()
		return res
	}

	pc, err := resolveContext(cmd, contextOptions{Profile: profile})
	if err != nil {
		// Not logged in is the ordinary state of a CI runner and of an agent
		// sandbox, and neither is a reason to call the repository broken.
		res.Status = stepSkip
		res.Summary = "no session, so the platform was not asked (`asgard-cli login`, or --offline to say so on purpose)"
		res.Remedy = ""
		return res
	}
	remote, err := pc.Client.DocsVersionOnly(cmd.Context())
	if err != nil {
		res.Status = stepSkip
		res.Summary = fmt.Sprintf("the platform did not answer: %v", err)
		res.Remedy = ""
		return res
	}

	if stamp == nil {
		res.Status = stepFail
		res.Summary = fmt.Sprintf("nothing fetched here, so an agent has no statement of what %s accepts", pc.Session.Profile.Name)
		return res
	}
	if stamp.Version != remote.Version {
		res.Status = stepFail
		if skills.Behind(stamp.Version, remote.Version) {
			res.Summary = fmt.Sprintf("behind: the platform has published version %s and this repository holds %s",
				remote.Version, stamp.Version)
			return res
		}
		res.Summary = fmt.Sprintf("this repository holds version %s and %s serves %s",
			stamp.Version, pc.Session.Profile.Name, remote.Version)
		return res
	}
	if moved := skills.MovedSources(stamp, sourceDigests(remote.Sources)); len(moved) > 0 {
		res.Status = stepFail
		res.Summary = fmt.Sprintf("same version, different material: %s moved since this was fetched", strings.Join(moved, ", "))
		return res
	}

	res.Status = stepPass
	res.Summary = fmt.Sprintf("version %s, matching %s", stamp.Version, pc.Session.Profile.Name)
	res.Remedy = ""
	return res
}

// skillsDirIgnoredNote says when the directory the material lands in is
// excluded from this checkout by an ignore rule.
//
// **The files are on disk so that a clone has them without a fetch.** An
// ignore rule takes that away while every version number still reads as
// current, and `.claude/` is a line a great many repositories already carry -
// which is reachable here because `.claude/skills` is one of the two
// directories the material can land in. It is a note rather than a verdict:
// nothing about the material is wrong, and what has to change is a line in
// somebody's `.gitignore` rather than anything this tool wrote.
//
// Empty when git cannot be asked, which is a checkout with no git, no
// repository, or a git that answered unexpectedly. See gitrepo.Ignored.
func skillsDirIgnoredNote(cmd *cobra.Command, repoRoot, root string) string {
	ignored, known := gitrepo.Ignored(cmd.Context(), repoRoot, root)
	if !known || !ignored {
		return ""
	}
	rel := root
	if r, err := filepath.Rel(repoRoot, root); err == nil {
		rel = r
	}
	return fmt.Sprintf("an ignore rule excludes %s, so a fresh clone of this repository will not have any of it", rel)
}

func appendNote(details []string, note string) []string {
	if note == "" {
		return details
	}
	return append(details, note)
}

// gateCharts runs lint, render and verify over the releases, and reports them
// as three steps rather than one: they fail for different reasons and are fixed
// in different places, and a single "charts" line would hide which.
func gateCharts(cmd *cobra.Command, root string, releases []string, helmReady bool) []stepResult {
	lint := stepResult{Name: "lint", Status: stepPass}
	rendered := stepResult{Name: "render", Status: stepPass}
	verified := stepResult{Name: "verify", Status: stepPass}

	if !helmReady {
		for _, s := range []*stepResult{&lint, &rendered, &verified} {
			s.Status = stepSkip
			s.Summary = "helm is not on PATH"
		}
		return []stepResult{lint, rendered, verified}
	}
	if len(releases) == 0 {
		for _, s := range []*stepResult{&lint, &rendered, &verified} {
			s.Status = stepSkip
			s.Summary = pipelineconfig.FileName + " declares no releases yet"
		}
		return []stepResult{lint, rendered, verified}
	}

	decl, err := pipelineconfig.LoadFromRepo(root, "")
	if err != nil {
		lint.Status, lint.Summary = stepFail, err.Error()
		return []stepResult{lint, rendered, verified}
	}

	charts, renders, resources, problems := 0, 0, 0, 0
	warnings := &warningSet{}
	for _, name := range releases {
		release, ok := decl.Release(name)
		if !ok {
			continue
		}

		// With the reserved `asgard` block and NOTHING else.
		//
		// The instruction this replaces said to lint bare, with no values file
		// at all, because that is the only form that proves values.yaml
		// declares a default for every `.Values.*` a template reads - overlay
		// an environment file and a missing default is masked until somebody
		// runs plain `helm template`, at which point it is a nil pointer in
		// somebody else's terminal.
		//
		// **That instruction stopped being right when the platform started
		// injecting values.** A chart must not declare the `asgard` block in
		// its own values.yaml - the platform overwrites it, and declaring it is
		// a warning on every plan - so a bare lint fails on every chart that
		// reads `.Values.asgard.projectEnvironmentId`, which is every chart
		// that labels anything. Verified on a real repository: four of four
		// charts, all for that reason and no other.
		//
		// Supplying that one file and no other keeps the property intact for
		// everything the chart does own.
		chart := filepath.Join(root, filepath.FromSlash(release.Chart))
		if output, err := runHelmLint(cmd, chart, name); err != nil {
			lint.Status = stepFail
			lint.Details = append(lint.Details, fmt.Sprintf("FAIL  %s: %s", name, output))
		} else {
			charts++
		}

		var buf bytes.Buffer
		if _, err := render.Run(cmd.Context(), render.Options{Root: root, Release: name}, &buf, io.Discard); err != nil {
			rendered.Status = stepFail
			rendered.Details = append(rendered.Details, fmt.Sprintf("FAIL  %s: %v", name, err))
			continue
		}
		renders++

		docs, err := gate.Read(&buf)
		if err != nil {
			verified.Status = stepFail
			verified.Details = append(verified.Details, fmt.Sprintf("FAIL  %s: %v", name, err))
			continue
		}
		resources += len(docs)

		for _, r := range gates(docs, gate.Options{Project: projectOfRelease(root, name)}) {
			for _, p := range r.Problems {
				verified.Status = stepFail
				verified.Details = append(verified.Details, fmt.Sprintf("FAIL  %s: %s", name, p))
				problems++
			}
			// A warning is truncated to one line and a failure is not.
			// Several of these rules explain themselves in a paragraph -
			// correctly, because the judgement is the point - and a command
			// somebody runs after every edit cannot print six paragraphs of
			// advice about something that is not blocking them. The summary
			// says where the full text is.
			for _, w := range r.Warnings {
				warnings.add(w, name)
			}
		}
	}

	lint.Summary = fmt.Sprintf("%d of %d chart(s) declare a default for every value they read", charts, len(releases))
	rendered.Summary = fmt.Sprintf("%d of %d release(s) rendered", renders, len(releases))

	lines := warnings.lines()

	switch {
	case verified.Status == stepFail:
		verified.Details = append(verified.Details, lines...)
		verified.Summary = fmt.Sprintf("%d problem(s) across %d resource(s)", problems, resources)
	case renders == 0:
		// Nothing rendered, so nothing was checked. Reporting that as a pass
		// or as a warning would both be claims about resources that do not
		// exist.
		verified.Status = stepSkip
		verified.Summary = "nothing rendered, so nothing was checked"
	case len(lines) > 0:
		verified.Status = stepWarn
		verified.Details = lines
		verified.Summary = fmt.Sprintf("%d resource(s), %d warning(s); `asgard-cli verify` prints them in full", resources, len(lines))
	default:
		verified.Summary = fmt.Sprintf("%d resource(s) reference each other correctly", resources)
	}
	return []stepResult{lint, rendered, verified}
}

// warningSet collapses the same warning raised against several releases.
//
// Two releases of one chart produce identical findings, and a repository with
// four releases printed the same six sentences twice. The finding is one fact
// about one chart; which releases it reached is a suffix, not a second copy.
type warningSet struct {
	order    []string
	releases map[string][]string
}

func (w *warningSet) add(text, release string) {
	if w.releases == nil {
		w.releases = map[string][]string{}
	}
	if _, seen := w.releases[text]; !seen {
		w.order = append(w.order, text)
	}
	if release != "" && !slices.Contains(w.releases[text], release) {
		w.releases[text] = append(w.releases[text], release)
	}
}

func (w *warningSet) lines() []string {
	out := make([]string, 0, len(w.order))
	for _, text := range w.order {
		line := "warn  " + truncate(text, 110)
		if where := w.releases[text]; len(where) > 0 {
			line += "  [" + strings.Join(where, ", ") + "]"
		}
		out = append(out, line)
	}
	return out
}

// runHelmLint returns helm's own output on failure, because that output names
// the template and the line and this cannot improve on it.
func runHelmLint(cmd *cobra.Command, chart, release string) (string, error) {
	if _, err := os.Stat(filepath.Join(chart, "Chart.yaml")); err != nil {
		return "no Chart.yaml in " + chart, err
	}
	values, err := render.AsgardValuesFile(release, "")
	if err != nil {
		return err.Error(), err
	}
	defer os.Remove(values)

	helm, err := tool.Helm.Command(cmd.Context(), "lint", chart, "-f", values)
	if err != nil {
		return err.Error(), err
	}
	var buf bytes.Buffer
	helm.Stdout = &buf
	helm.Stderr = &buf
	if err := helm.Run(); err != nil {
		return firstProblem(buf.String()), err
	}
	return "", nil
}

// firstProblem picks what helm meant out of the banner it prints around it.
//
// helm splits one failure across three lines - the file and position, then the
// expression, then the reason - and the reason is the only one that says what
// is wrong. Taking the first line alone reported "templates/: <file>:28:52",
// which names a location and no problem.
func firstProblem(output string) string {
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[ERROR]") && !strings.HasPrefix(line, "Error:") {
			continue
		}
		if strings.HasPrefix(line, "Error:") && strings.Contains(line, "chart(s) failed") {
			// helm's own tally, printed after the real finding.
			continue
		}
		parts := []string{line}
		for _, next := range lines[i+1:] {
			next = strings.TrimSpace(next)
			if next == "" || strings.HasPrefix(next, "[") || strings.HasPrefix(next, "Error:") {
				break
			}
			parts = append(parts, next)
		}
		return strings.Join(parts, " ")
	}
	return strings.TrimSpace(output)
}
