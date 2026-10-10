package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

func newOperateSyncerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "syncer",
		Short: "Run a Syncer now, and read what its runs did",
		Long: `Run a Syncer now, and read what its runs did.

    asgard-cli operate syncer sync <name> --release <r> [--wait 5m]
    asgard-cli operate syncer executions <name> --release <r>

A Syncer runs on its schedule, and on a deploy when it carries
asgard-ai.com/auto-fire-on-rollout. "sync" is the third way, for the times
neither fits: the member asked for a sync now, or the Syncer is deliberately
not fired on deploy because its first run outlasts the apply step's wait.

A Syncer that fills a SkillSet is run through the SkillSet instead:
"asgard-cli operate skill-set sync". Both commands here say so when handed one.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newOperateSyncerSyncCmd(), newOperateSyncerExecutionsCmd())
	return cmd
}

// syncTarget is a Syncer located: the SourceSet the drive routes need.
type syncTarget struct {
	Name      string
	SourceSet string
}

// locateSyncer finds the SourceSet a Syncer fills, from the live CR when the
// scope is a release and from the platform otherwise, and refuses one a
// SkillSet owns: the drive routes answer 404 for it, which would read as a
// wrong name.
func locateSyncer(ctx context.Context, pc *platformContext, f *operateFlags, scope *operateScope, name, verb string) (*syncTarget, error) {
	var sourceSet, managedBy string
	cr, err := scope.object("Syncer", name)
	if err != nil {
		return nil, err
	}
	if cr != nil {
		sourceSet, managedBy = cr.specString("sourceSetName"), cr.Metadata.Labels[platform.ManagedByLabel]
	} else {
		s, err := pc.Client.GetSyncer(ctx, scope.ProjectID, name)
		if err != nil {
			return nil, err
		}
		sourceSet, managedBy = s.SourceSetID, s.ManagedBy()
	}
	if managedBy == platform.ManagedBySkillSet {
		skillSet := scope.skillSetFor(sourceSet)
		if skillSet == "" {
			skillSet = "<skill-set>"
		}
		return nil, fmt.Errorf("Syncer %s fills a SkillSet (it is labelled %s: %s), and its runs go through the SkillSet:\n\n"+
			"    asgard-cli operate skill-set %s %s %s",
			name, platform.ManagedByLabel, platform.ManagedBySkillSet, verb, skillSet, f.scopeFlag())
	}
	if sourceSet == "" {
		return nil, fmt.Errorf("Syncer %s in %s names no SourceSet, so there is no drive to run it in", name, scope.Where)
	}
	return &syncTarget{Name: name, SourceSet: sourceSet}, nil
}

func newOperateSyncerSyncCmd() *cobra.Command {
	var (
		f    operateFlags
		wait time.Duration
	)
	cmd := &cobra.Command{
		Use:   "sync <syncer>",
		Short: "Start one run of a Syncer now",
		Long: `Start one run of a Syncer now, outside its schedule.

    asgard-cli operate syncer sync kb-docs --release internal-dev
    asgard-cli operate syncer sync kb-docs --release internal-dev --wait 10m

The platform answers with no run id, so without --wait this returns as soon as
the run is accepted. With --wait it watches the Syncer's history for a run
that was not there before, and returns when that run has finished: zero for
Succeeded, non-zero for Failed, and non-zero when the wait runs out first. The
platform keeps no log of a Syncer's run - its outcome and the Job's name are
all there is.

It is refused while a run is already going, and for a Syncer the platform has
not provisioned yet. It does not wait for a deploy: a deploy that changes a
Syncer's spec replaces its CronJob and deletes a Job made from it, so do not
start one while a pipeline run that changes this Syncer is applying.

Needs source-set/put in the project. In the Workbench sandbox a command is cut
off after about ten minutes, so keep --wait under that and read the rest with
"asgard-cli operate syncer executions".`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, scope, err := f.resolve(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			t, err := locateSyncer(ctx, pc, &f, scope, args[0], "sync")
			if err != nil {
				return err
			}
			list := func(ctx context.Context) ([]platform.SyncerExecution, error) {
				return pc.Client.SyncerExecutions(ctx, scope.ProjectID, t.SourceSet, t.Name)
			}
			trigger := func(ctx context.Context) error {
				return pc.Client.TriggerSyncer(ctx, scope.ProjectID, t.SourceSet, t.Name)
			}
			res := syncResult{Kind: "Syncer", Name: t.Name, SourceSet: t.SourceSet}
			next := fmt.Sprintf("asgard-cli operate syncer executions %s %s", t.Name, f.scopeFlag())
			return runSync(cmd, pc, &f, scope, wait, res, list, trigger, next)
		},
	}
	f.register(cmd)
	cmd.Flags().DurationVar(&wait, "wait", 0,
		"how long to wait for the run to finish, e.g. 10m; 0, the default, returns once the run is accepted")
	return cmd
}

func newOperateSyncerExecutionsCmd() *cobra.Command {
	var f operateFlags
	cmd := &cobra.Command{
		Use:   "executions <syncer>",
		Short: "List the runs a Syncer's status records",
		Long: `List the runs a Syncer's own status records, newest first.

    asgard-cli operate syncer executions kb-docs --release internal-dev

Each is a Kubernetes Job: its name, Running, Succeeded or Failed, when it
started and how long it took. Every run is here whoever started it - the
schedule, a deploy's fire, or "sync" - and only the most recent ones: the
Syncer's status keeps a short history, not every run there ever was.

This is the answer to "did the first sync run", which a succeeded pipeline run
does not give: the apply step's log says what the deploy fired, not what the
run did.

A Failed run with no duration is one whose Job disappeared before it finished.
The usual cause is a deploy that changed the Syncer's spec, which replaces its
CronJob and deletes the Job with it.

--format json prints the platform's array as it is, in the platform's order.
Needs project-resource/read in the project.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, scope, err := f.resolve(cmd)
			if err != nil {
				return err
			}
			t, err := locateSyncer(cmd.Context(), pc, &f, scope, args[0], "executions")
			if err != nil {
				return err
			}
			runs, err := pc.Client.SyncerExecutions(cmd.Context(), scope.ProjectID, t.SourceSet, t.Name)
			if err != nil {
				return err
			}
			return printRuns(cmd.OutOrStdout(), f.format, runs,
				fmt.Sprintf("Syncer %s (drive %s) in %s", t.Name, t.SourceSet, scope.Where))
		},
	}
	f.register(cmd)
	return cmd
}

// syncResult is what `sync` reports, in both formats.
type syncResult struct {
	Kind      string                    `json:"kind"`
	Name      string                    `json:"name"`
	SourceSet string                    `json:"source_set,omitempty"`
	Run       *platform.SyncerExecution `json:"run,omitempty"`
	// TimedOut is set when --wait ran out before a new run finished.
	TimedOut bool `json:"timed_out,omitempty"`
}

// runSync is `sync` for anything whose runs are a Syncer's: trigger, then
// optionally wait for the run that was not there before.
func runSync(cmd *cobra.Command, pc *platformContext, f *operateFlags, scope *operateScope, wait time.Duration,
	res syncResult, list func(context.Context) ([]platform.SyncerExecution, error),
	trigger func(context.Context) error, next string) error {
	ctx := cmd.Context()
	if wait < 0 {
		return fmt.Errorf("--wait cannot be negative")
	}
	var before []platform.SyncerExecution
	if wait > 0 {
		// Read before the trigger, so the run it starts is the one not here.
		var err error
		if before, err = list(ctx); err != nil {
			return err
		}
	}

	actingOn(cmd, pc.Session)
	if err := trigger(ctx); err != nil {
		// A run already going is the one refusal with a next step: the run
		// somebody wanted is probably the one in progress.
		var apiErr *platform.APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusConflict {
			return fmt.Errorf("%w\nA run is already going, and a Syncer runs one at a time. See where it is:\n\n    %s", err, next)
		}
		return err
	}
	out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()
	what := res.Kind + " " + res.Name
	if res.SourceSet != "" {
		what += " (drive " + res.SourceSet + ")"
	}

	if wait == 0 {
		if f.format == formatJSON {
			return writeJSON(out, res)
		}
		fmt.Fprintf(out, "Started a run of %s in %s.\nThe platform answers with no run id; its runs:\n\n    %s\n", what, scope.Where, next)
		return nil
	}

	fmt.Fprintf(errOut, "started a run of %s; waiting up to %s for it to finish\n", what, wait)
	waitCtx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	got, err := waitForNewRun(waitCtx, before, list, func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(runPollInterval):
			return nil
		}
	})
	if err != nil {
		return err
	}
	res.Run, res.TimedOut = got.Run, got.TimedOut
	failed := got.TimedOut || got.Run == nil || got.Run.Status == platform.SyncerRunFailed

	if f.format == formatJSON {
		if err := writeJSON(out, res); err != nil {
			return err
		}
		if failed {
			return ErrSilent
		}
		return nil
	}
	switch {
	case got.Run == nil:
		return fmt.Errorf("no new run of %s appeared within %s. The trigger was accepted; read the history later:\n\n    %s", what, wait, next)
	case got.TimedOut:
		return fmt.Errorf("run %s of %s is still %s after %s; read where it is later:\n\n    %s",
			got.Run.Name, what, got.Run.Status, wait, next)
	case got.Run.Status == platform.SyncerRunFailed:
		return fmt.Errorf("run %s of %s failed after %s. The platform keeps no log of a Syncer's run; "+
			"the Job's name is what there is to report", got.Run.Name, what, runDuration(*got.Run))
	}
	fmt.Fprintf(out, "Run %s of %s %s in %s.\n", got.Run.Name, what, got.Run.Status, runDuration(*got.Run))
	return nil
}

// printRuns writes a run history, newest first in text and as given in json.
func printRuns(out io.Writer, format string, runs []platform.SyncerExecution, what string) error {
	if format == formatJSON {
		if runs == nil {
			runs = []platform.SyncerExecution{}
		}
		return writeJSON(out, runs)
	}
	if len(runs) == 0 {
		fmt.Fprintf(out, "%s has no runs recorded.\n", what)
		return nil
	}
	sorted := append([]platform.SyncerExecution(nil), runs...)
	sort.SliceStable(sorted, func(i, j int) bool { return startOf(sorted[i]) > startOf(sorted[j]) })
	fmt.Fprintf(out, "%s:\n\n", what)
	fmt.Fprintf(out, "%-44s %-10s %-17s %s\n", "JOB", "STATUS", "STARTED", "DURATION")
	for _, r := range sorted {
		started := "-"
		if r.StartTimestamp != nil {
			started = time.Unix(*r.StartTimestamp, 0).Local().Format("2006-01-02 15:04")
		}
		fmt.Fprintf(out, "%-44s %-10s %-17s %s\n", r.Name, r.Status, started, runDuration(r))
	}
	return nil
}

// runDuration is how long a run took, or "-" when it has not finished or
// never recorded a start.
func runDuration(r platform.SyncerExecution) string {
	if r.StartTimestamp == nil || r.CompletionTimestamp == nil {
		return "-"
	}
	return (time.Duration(*r.CompletionTimestamp-*r.StartTimestamp) * time.Second).String()
}
