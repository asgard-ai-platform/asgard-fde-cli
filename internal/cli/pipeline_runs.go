package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

// runPollInterval is how often a watched run is re-read.
//
// The platform has no streaming endpoint, so this is the whole mechanism. Two
// seconds is paced against what is being watched: a plan takes a few seconds
// and an apply takes minutes, and a step that started is visible within one
// tick either way.
const runPollInterval = 2 * time.Second

// appearTimeout is how long `watch --commit` waits for a webhook-created run to
// exist at all.
//
// A push reaches the platform in about three seconds, so this is generous
// enough to absorb a slow delivery and short enough that a push that produced
// no run - the pattern did not match, the release was never created - is
// reported rather than waited on forever. The remedy for that case is the
// pipeline's deliveries, which say why, and the timeout message says so.
const appearTimeout = 90 * time.Second

func newPipelineRunsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "runs",
		Short: "Read and review the pipeline's runs",
		Long: `Read and review the pipeline's runs.

A run walks Checkout, Lint, Variables, Render & Dry-run, Review and Apply, and
everything that decides whether a change is deployable happens in them, on the
platform, against the real cluster's CRDs. This is where the answer comes back.

    asgard-cli pipeline runs list
    asgard-cli pipeline runs watch --release dev --ref <tag>
    asgard-cli pipeline runs get <run-id>
    asgard-cli pipeline runs log <run-id> lint
    asgard-cli pipeline runs approve <run-id>

Use ` + "`watch`" + ` after a push: it waits for the run the
push created, follows its steps, and ends by printing the plan report or the
reason it failed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newRunsListCmd(),
		newRunsGetCmd(),
		newRunsLogCmd(),
		newRunsWatchCmd(),
		newRunsReviewCmd("approve"),
		newRunsReviewCmd("reject"),
		newRunsCancelCmd(),
	)
	return cmd
}

func newRunsListCmd() *cobra.Command {
	var (
		f       pipelineFlags
		release string
		states  []string
		size    int
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List runs, newest first",
		Long: `List runs of this pipeline, newest first.

Rows carry no steps and no plan report; ` + "`runs get`" + ` fetches those for one. They do
carry the failure reason, so why a run ended badly is visible without opening
it.

    asgard-cli pipeline runs list --release dev
    asgard-cli pipeline runs list --state awaiting_review`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pc, err := f.context(cmd)
			if err != nil {
				return err
			}
			p, err := resolvePipeline(cmd.Context(), pc, f.pipeline)
			if err != nil {
				return err
			}
			filter := platform.RunFilter{PipelineID: p.PipelineId, States: states, Size: size}
			if release != "" {
				rel, err := resolveRelease(cmd.Context(), pc, p, release)
				if err != nil {
					return err
				}
				filter.ReleaseID = rel.ReleaseId
			}
			runs, err := pc.Client.ListRuns(cmd.Context(), filter)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if f.format == formatJSON {
				return writeJSON(out, runs)
			}
			if len(runs) == 0 {
				fmt.Fprintf(out, "No runs.\n")
				return nil
			}
			for _, r := range runs {
				fmt.Fprintf(out, "#%-5d %-22s %-16s %-18s %-24s %s\n",
					r.Number, r.ReleaseName, r.State, r.Trigger, r.Ref, r.RunId)
				if r.ErrorMessage != "" {
					fmt.Fprintf(out, "       %s\n", r.ErrorMessage)
				}
			}
			return nil
		},
	}
	f.register(cmd, true)
	cmd.Flags().StringVar(&release, "release", "", "only runs of this release, by name")
	cmd.Flags().StringSliceVar(&states, "state", nil, "only runs in these states, repeatable or comma-separated")
	cmd.Flags().IntVar(&size, "size", 20, "how many rows to fetch")
	return cmd
}

func newRunsGetCmd() *cobra.Command {
	var (
		f            pipelineFlags
		versionBumps bool
	)

	cmd := &cobra.Command{
		Use:   "get <run-id>",
		Short: "Show one run with its steps and plan report",
		Long: `Show one run: its steps, and the plan report a reviewer reads.

The report's parts are the lint findings, the variable changes, the resource
changes as a diff per CR, and the server-side dry run. A run that
failed also carries the reason and which step produced it, so the first thing to
read is the top, not the log.

A tag moves the version labels a chart stamps on every resource, so most
updates of a release are nothing else. The platform marks those "version bump"
and they are folded into one line under resources, with the two versions;
--version-bumps lists them one by one. Every other update is a change of
content and is always listed. --format json carries every row either way.

Exit code is 1 when the run failed, so a script can branch on it.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, err := f.context(cmd)
			if err != nil {
				return err
			}
			run, err := pc.Client.GetRun(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if f.format == formatJSON {
				if err := writeJSON(out, run); err != nil {
					return err
				}
			} else {
				printRun(out, run, versionBumps)
			}
			if platform.Failed(run.State) {
				return ErrSilent
			}
			return nil
		},
	}
	f.register(cmd, false)
	cmd.Flags().BoolVar(&versionBumps, versionBumpsFlag, false, versionBumpsUsage)
	return cmd
}

const (
	versionBumpsFlag  = "version-bumps"
	versionBumpsUsage = "list every update that only moves the version, instead of folding them into one line"
)

func newRunsLogCmd() *cobra.Command {
	var f pipelineFlags

	cmd := &cobra.Command{
		Use:   "log <run-id> <step>",
		Short: "Print one step's captured log",
		Long: `Print one step's captured log.

The step is one of: ` + platform.StepList() + `.

A failed step's reason is the last line beginning "✗ ". Reading the first line
instead gets the step's opening banner, which is not the reason it failed.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			runID, step := args[0], args[1]
			if !platform.ValidStep(step) {
				return fmt.Errorf("no step %q; one of %s", step, platform.StepList())
			}
			pc, err := f.context(cmd)
			if err != nil {
				return err
			}
			log, err := pc.Client.GetStepLog(cmd.Context(), runID, step)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if f.format == formatJSON {
				return writeJSON(out, log)
			}
			for _, line := range log.Lines {
				fmt.Fprintln(out, line)
			}
			if log.Truncated {
				fmt.Fprintf(cmd.ErrOrStderr(), "\n(the log hit the platform's cap; the tail above is what was kept)\n")
			}
			return nil
		},
	}
	f.register(cmd, false)
	return cmd
}

func newRunsWatchCmd() *cobra.Command {
	var (
		f       pipelineFlags
		release string
		ref     string
		commit  string
		runID   string
		appear  time.Duration

		versionBumps bool
	)

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Follow a run until it finishes, then print what it decided",
		Long: `Follow a run until it reaches a state that will not change again, then print the
plan report or the reason it failed.

Run this after a push. The platform decides whether a change
is deployable - it renders the chart, checks every CR against the cluster's own
CRDs, and reports back - and this is how that answer arrives without opening a
browser.

    git push origin dev-0.0.4
    asgard-cli pipeline runs watch --release internal-dev --ref dev-0.0.4

--ref waits for the run the push created, which does not exist yet when the
command starts: a push reaches the platform in a few seconds. Waiting for it to
appear and then following it is one command.

--ref is the tag or branch you pushed, which is what a run is indexed by.
--commit takes a SHA instead; prefer --ref, for two reasons: an annotated tag
is its own git object, so
"git rev-parse <tag>" is not the commit, and "git rev-parse HEAD" is not the tag
target unless you tagged HEAD; and the run summaries carry no commit, so
matching on one costs a request per run per poll.

--release alone follows that release's newest run. --run follows one by id.

The plan report is printed as "pipeline runs get" prints it: updates that only
move the version are folded into one line, and --version-bumps lists them.

If no run appears, this says which of two cases it is. When there are runs it
lists them - the push was received, and the search was for the wrong name. When
there are none the push matched nothing, and the pipeline's deliveries say why.

Exit codes: 0 when the run succeeded or is waiting for review; 1 when it failed,
was rejected, expired, was superseded or cancelled. Waiting for review is not a
failure: the plan is good and a person has to approve it.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pc, err := f.context(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			msg := cmd.ErrOrStderr()

			var run *platform.Run
			switch {
			case runID != "":
				run, err = pc.Client.GetRun(ctx, runID)
			default:
				run, err = findRunToWatch(ctx, pc, f.pipeline, release, ref, commit, appear, msg)
			}
			if err != nil {
				return err
			}

			final, err := followRun(ctx, pc, run, msg)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if f.format == formatJSON {
				if err := writeJSON(out, final); err != nil {
					return err
				}
			} else {
				printRun(out, final, versionBumps)
			}
			if platform.Failed(final.State) {
				return ErrSilent
			}
			return nil
		},
	}
	f.register(cmd, true)
	cmd.Flags().StringVar(&release, "release", "", "release name whose run to follow")
	cmd.Flags().StringVar(&ref, "ref", "", "wait for the run of this tag or branch, which a push may not have created yet")
	cmd.Flags().StringVar(&commit, "commit", "", "wait for the run of this commit SHA, which a push may not have created yet")
	cmd.Flags().StringVar(&runID, "run", "", "follow this run id, instead of finding one")
	cmd.Flags().DurationVar(&appear, "appear-timeout", appearTimeout, "how long to wait for a run of --commit to appear")
	cmd.Flags().BoolVar(&versionBumps, versionBumpsFlag, false, versionBumpsUsage)
	return cmd
}

// findRunToWatch locates the run a watch should follow.
func findRunToWatch(
	ctx context.Context,
	pc *platformContext,
	pipelineRef, release, ref, commit string,
	appear time.Duration,
	msg io.Writer,
) (*platform.Run, error) {
	p, err := resolvePipeline(ctx, pc, pipelineRef)
	if err != nil {
		return nil, err
	}
	filter := platform.RunFilter{PipelineID: p.PipelineId, Size: 50}
	if release != "" {
		rel, err := resolveRelease(ctx, pc, p, release)
		if err != nil {
			return nil, err
		}
		filter.ReleaseID = rel.ReleaseId
	}

	if ref == "" && commit == "" {
		runs, err := pc.Client.ListRuns(ctx, filter)
		if err != nil {
			return nil, err
		}
		if len(runs) == 0 {
			return nil, fmt.Errorf("no runs to follow; --ref waits for one that a push is about to create")
		}
		return pc.Client.GetRun(ctx, runs[0].RunId)
	}

	waitingFor := ref
	if waitingFor == "" {
		waitingFor = "commit " + short(commit)
	}
	fmt.Fprintf(msg, "Waiting for the run of %s...\n", waitingFor)

	deadline := time.Now().Add(appear)
	for {
		runs, err := pc.Client.ListRuns(ctx, filter)
		if err != nil {
			return nil, err
		}
		for _, r := range runs {
			// --ref matches the summary, which already carries the ref.
			// --commit cannot: the summary has no commit, so every row costs a
			// GetRun on every poll - about 1500 calls to conclude nothing at
			// the default timeout. That is the second reason --ref exists.
			if ref != "" {
				if r.Ref == ref {
					return pc.Client.GetRun(ctx, r.RunId)
				}
				continue
			}
			full, err := pc.Client.GetRun(ctx, r.RunId)
			if err != nil {
				return nil, err
			}
			if strings.HasPrefix(full.CommitSha, commit) || strings.HasPrefix(commit, full.CommitSha) {
				return full, nil
			}
		}
		if time.Now().After(deadline) {
			return nil, noRunAppeared(waitingFor, commit, release, appear, runs)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(runPollInterval):
		}
	}
}

// noRunAppeared explains a timeout with what the search actually found.
//
// The old message asserted two causes - no pattern matched, or the release was
// never created - and pointed at the one command that contradicts it. Both were
// false the one time it fired: the run existed and had already planned
// successfully. The loop is holding that run when it gives up, because it lists
// the release's runs on every poll, and it threw the list away.
//
// Trusting the message would have sent somebody after an uncreated release or a
// broken pattern, and the next step in that hunt is re-pushing the tag or
// recreating the release - one of them destructive against a deploy that is
// already waiting for review.
func noRunAppeared(waitingFor, commit, release string, appear time.Duration, found []*platform.RunSummary) error {
	if len(found) == 0 {
		where := "no release's pattern matched the ref, or the one it matched was never created on the platform"
		if release != "" {
			where = "nothing has been pushed that matches " + release + "'s pattern"
		}
		return fmt.Errorf(
			"no run for %s appeared within %s, and there are no runs here at all.\n"+
				"So the push matched nothing: %s. A run that was not\n"+
				"created leaves no record of its own, so the reason is in the pipeline's deliveries:\n\n"+
				"    asgard-cli pipeline deliveries", waitingFor, appear, where)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "no run for %s appeared within %s, but there are runs here:\n\n", waitingFor, appear)
	for i, r := range found {
		if i == 5 {
			fmt.Fprintf(&b, "    ... and %d more\n", len(found)-i)
			break
		}
		fmt.Fprintf(&b, "    #%-4d %-20s %-16s %s %s\n", r.Number, r.ReleaseName, r.State, r.Trigger, r.Ref)
	}
	b.WriteString("\nSo the push was received. Follow one of those by id with --run, or by name with\n")
	b.WriteString("--ref <tag-or-branch> - which is what a run is indexed by.\n")
	if commit != "" {
		b.WriteString("\nAgainst an older platform an ANNOTATED tag recorded the tag object rather than\n")
		b.WriteString("the commit, and then no --commit value can match it. --ref can.\n")
	}
	b.WriteString("\nIf none of those is the push you are waiting for, the deliveries say what arrived:\n\n")
	b.WriteString("    asgard-cli pipeline deliveries")
	return errors.New(b.String())
}

// followRun polls a run to a terminal state, reporting each step as it moves.
//
// awaiting_review stops the wait: it is terminal from this command's point of
// view, because what happens next is a person deciding, and a CLI that waited
// on that would wait for hours.
func followRun(ctx context.Context, pc *platformContext, run *platform.Run, msg io.Writer) (*platform.Run, error) {
	fmt.Fprintf(msg, "Run #%d of %s, %s %s (%s)\n",
		run.Number, run.ReleaseName, run.Trigger, run.Ref, short(run.CommitSha))

	reported := map[string]string{}
	for {
		for _, s := range run.Steps {
			if reported[s.Name] == s.State {
				continue
			}
			reported[s.Name] = s.State
			switch s.State {
			case "running":
				fmt.Fprintf(msg, "  %-16s running\n", s.Name)
			case "done":
				fmt.Fprintf(msg, "  %-16s done   %s\n", s.Name, duration(s.DurationMs))
			case "failed":
				fmt.Fprintf(msg, "  %-16s FAILED %s\n", s.Name, duration(s.DurationMs))
			case "waiting":
				fmt.Fprintf(msg, "  %-16s waiting for review\n", s.Name)
			case "skipped":
				fmt.Fprintf(msg, "  %-16s skipped\n", s.Name)
			}
		}

		if platform.Terminal(run.State) || run.State == platform.RunAwaitingReview {
			return run, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(runPollInterval):
		}

		next, err := pc.Client.GetRun(ctx, run.RunId)
		if err != nil {
			return nil, err
		}
		run = next
	}
}

func newRunsReviewCmd(action string) *cobra.Command {
	var (
		f       pipelineFlags
		comment string
	)

	verb := "Approve a run and start its apply"
	long := `Approve a run waiting for review and start its apply.

Apply uses the chart and values captured when the plan ran, not the repository
as it is now, so what was reviewed is what is applied: a tag moved, a repository
switched or a file edited since then changes nothing about this run.`
	if action == "reject" {
		verb = "Reject a run waiting for review"
		long = `Reject a run waiting for review.

Nothing is applied and the run is finished. Leave a --comment: it is the only
record of why, and the next run does not inherit it.`
	}

	cmd := &cobra.Command{
		Use:   action + " <run-id>",
		Short: verb,
		Long:  long + "\n\nOnly a run in awaiting_review can be reviewed, and it needs workspace\nadministration - editing variables is open to members, sending them to the\ncluster is not.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, err := f.context(cmd)
			if err != nil {
				return err
			}
			actingOn(cmd, pc.Session)
			var run *platform.Run
			if action == "approve" {
				run, err = pc.Client.ApproveRun(cmd.Context(), args[0], comment)
			} else {
				run, err = pc.Client.RejectRun(cmd.Context(), args[0], comment)
			}
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if f.format == formatJSON {
				return writeJSON(out, run)
			}
			fmt.Fprintf(out, "Run #%d is now %s.\n", run.Number, run.State)
			if action == "approve" {
				fmt.Fprintf(out, "Follow it: asgard-cli pipeline runs watch --run %s\n", run.RunId)
			}
			return nil
		},
	}
	f.register(cmd, false)
	cmd.Flags().StringVar(&comment, "comment", "", "a note recorded with the decision")
	return cmd
}

func newRunsCancelCmd() *cobra.Command {
	var f pipelineFlags

	cmd := &cobra.Command{
		Use:   "cancel <run-id>",
		Short: "Cancel a queued, planning or applying run",
		Long: `Cancel a run.

The resulting state depends on when it is cancelled. Cancelling before apply is ` + "`cancelled`" + `:
planning only fetches, renders and dry-runs, and has touched nothing. Cancelling
during apply is ` + "`apply_failed`" + `, with a fixed reason saying so - the secret may
already be written, the helm upgrade may already have run, and nothing is rolled
back. It is not called cancelled because the platform does not undo anything.

Cancelling during apply is how to recover from an apply stuck on the cluster - a
syncer job that never ends, a rollout waiting on something that never becomes
ready. It releases the release's single active-run slot immediately, so the next
run does not queue behind the stuck one.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, err := f.context(cmd)
			if err != nil {
				return err
			}
			actingOn(cmd, pc.Session)
			run, err := pc.Client.CancelRun(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if f.format == formatJSON {
				return writeJSON(out, run)
			}
			fmt.Fprintf(out, "Run #%d is now %s.\n", run.Number, run.State)
			if run.ErrorMessage != "" {
				fmt.Fprintf(out, "%s\n", run.ErrorMessage)
			}
			return nil
		},
	}
	f.register(cmd, false)
	return cmd
}

// printRun renders a run for a reader: the header, the steps, then whichever of
// the report's parts have anything in them. Updates that only move the version
// are folded into one line unless versionBumps asks for each of them.
func printRun(out io.Writer, r *platform.Run, versionBumps bool) {
	fmt.Fprintf(out, "run          #%d  %s\n", r.Number, r.State)
	fmt.Fprintf(out, "release      %s\n", r.ReleaseName)
	fmt.Fprintf(out, "trigger      %s %s\n", r.Trigger, r.Ref)
	fmt.Fprintf(out, "commit       %s  %s\n", short(r.CommitSha), r.RepoFullName)
	fmt.Fprintf(out, "started by   %s\n", r.Actor)
	if r.ErrorMessage != "" {
		fmt.Fprintf(out, "failed at    %s\n", r.FailedStep)
		fmt.Fprintf(out, "reason       %s\n", r.ErrorMessage)
	}

	if len(r.Steps) > 0 {
		fmt.Fprintf(out, "\nsteps\n")
		for _, s := range r.Steps {
			fmt.Fprintf(out, "  %-16s %-10s %s\n", s.Name, s.State, duration(s.DurationMs))
		}
	}

	rep := r.Report
	if rep == nil {
		return
	}
	if rep.NoChanges {
		fmt.Fprintf(out, "\nplan: no changes\n")
	} else if versionOnly(rep) {
		fmt.Fprintf(out, "\nplan: only version labels change%s; review is still required\n", versionSpan(rep, " (%s)"))
	}
	if len(rep.Lint) > 0 {
		fmt.Fprintf(out, "\nlint\n")
		for _, l := range rep.Lint {
			where := l.Location
			if where != "" {
				where = "  " + where
			}
			fmt.Fprintf(out, "  %-8s %-26s %s%s\n", l.Level, l.Rule, l.Message, where)
		}
	}
	if len(rep.Variables) > 0 {
		fmt.Fprintf(out, "\nvariables\n")
		for _, v := range rep.Variables {
			fmt.Fprintf(out, "  %-12s %-40s %s\n", v.Kind, v.Key, v.Change)
		}
	}
	if len(rep.Resources) > 0 {
		fmt.Fprintf(out, "\nresources\n")
		bumps := 0
		for _, res := range rep.Resources {
			if res.VersionBump() {
				bumps++
				if !versionBumps {
					continue
				}
			}
			fmt.Fprintf(out, "  %-12s %-24s %-40s\n", resourceChangeLabel(res), res.Kind, res.Name)
		}
		if bumps > 0 && !versionBumps {
			noun := "resources"
			if bumps == 1 {
				noun = "resource"
			}
			fmt.Fprintf(out, "  %-12s %d %s, version labels only%s  (--%s lists them)\n",
				versionBumpLabel, bumps, noun, versionSpan(rep, "  %s"), versionBumpsFlag)
		}
	}
	if rep.DryRun != nil && !rep.DryRun.Ok {
		fmt.Fprintf(out, "\ndry run failed\n")
		if rep.DryRun.Summary != "" {
			fmt.Fprintf(out, "  %s\n", rep.DryRun.Summary)
		}
		for _, line := range rep.DryRun.Output {
			fmt.Fprintf(out, "  %s\n", line)
		}
	}
	if r.State == platform.RunAwaitingReview {
		fmt.Fprintf(out, "\nWaiting for review. Apply uses the chart and values captured at plan time.\n")
		fmt.Fprintf(out, "    asgard-cli pipeline runs approve %s\n", r.RunId)
	}
}

const versionBumpLabel = "version bump"

// resourceChangeLabel is what the change column says: an update that only
// moves the version reads "version bump", everything else its change.
func resourceChangeLabel(d *platform.ResourceDiff) string {
	if d.VersionBump() {
		return versionBumpLabel
	}
	return d.Change
}

// versionOnly reports whether the only thing the plan would change is version
// labels: at least one version bump, every other resource unchanged, and no
// variable changed. Such a run still waits for review.
func versionOnly(rep *platform.PlanReport) bool {
	bumps := 0
	for _, res := range rep.Resources {
		switch {
		case res.VersionBump():
			bumps++
		case res.Change != "unchanged":
			return false
		}
	}
	for _, v := range rep.Variables {
		if v.Change != "unchanged" {
			return false
		}
	}
	return bumps > 0
}

// versionSpan formats "<previous> -> <this>" through layout, or returns ""
// when the report does not name both versions.
func versionSpan(rep *platform.PlanReport, layout string) string {
	if rep.PreviousAppVersion == "" || rep.AppVersion == "" {
		return ""
	}
	return fmt.Sprintf(layout, rep.PreviousAppVersion+" -> "+rep.AppVersion)
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func duration(ms int64) string {
	if ms <= 0 {
		return ""
	}
	d := time.Duration(ms) * time.Millisecond
	if d < time.Second {
		return d.String()
	}
	return d.Round(100 * time.Millisecond).String()
}
