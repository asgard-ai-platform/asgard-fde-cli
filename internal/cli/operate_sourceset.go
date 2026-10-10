package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

func newOperateSourceSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "source-set",
		Short: "Refresh a SourceSet's context index now, and read what its refreshes did",
		Long: `Refresh a SourceSet's context index now, and read what its refreshes did.

    asgard-cli operate source-set reindex <name> --release <r> [--wait 15m]
    asgard-cli operate source-set index-runs <name> --release <r>
    asgard-cli operate source-set index-logs <name> <invocation> --release <r>

A SourceSet with a contextIndex gets a Trigger the platform derives for it, and
each refresh of the index is one invocation of that Trigger: an indexing agent
reads the drive and writes the graph, in a conversation it may finish, fail,
or stop to ask a question in. The Console calls these "Reindex Now" and the
refresh list. The schedule and the prompt are the chart's.

The graph is not useful until a refresh has run, and the first one runs on the
schedule unless somebody starts it. Run "reindex" after the Syncers that fill
the drive have finished, or the index describes a drive that is still being
written.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newOperateReindexCmd(), newOperateIndexRunsCmd(), newOperateIndexLogsCmd())
	return cmd
}

// contextIndexSource is a SourceSet's context-index refreshes, once the
// SourceSet is checked to have one.
func contextIndexSource(pc *platformContext, f *operateFlags, scope *operateScope, name string) (invocationSource, error) {
	cr, err := scope.object("SourceSet", name)
	if err != nil {
		return invocationSource{}, err
	}
	if cr != nil {
		if _, ok := cr.Spec["contextIndex"]; !ok {
			return invocationSource{}, fmt.Errorf("SourceSet %s in %s has no contextIndex, so there is no index to refresh", name, scope.Where)
		}
	}
	project := scope.ProjectID
	return invocationSource{
		What: "the context index of SourceSet " + name,
		list: func(ctx context.Context, q platform.InvocationQuery) ([]platform.Invocation, platform.Paging, error) {
			return pc.Client.ContextIndexInvocations(ctx, project, name, q)
		},
		fire:    func(ctx context.Context) error { return pc.Client.ReindexContextIndex(ctx, project, name) },
		runsCmd: fmt.Sprintf("asgard-cli operate source-set index-runs %s %s", name, f.scopeFlag()),
		logsCmd: func(inv string) string {
			return fmt.Sprintf("asgard-cli operate source-set index-logs %s %s %s", name, inv, f.scopeFlag())
		},
	}, nil
}

func newOperateReindexCmd() *cobra.Command {
	var (
		f    operateFlags
		wait time.Duration
	)
	cmd := &cobra.Command{
		Use:   "reindex <source-set>",
		Short: "Start one refresh of a SourceSet's context index now",
		Long: `Start one refresh of a SourceSet's context index now, as its schedule would.

    asgard-cli operate source-set reindex kb --release internal-dev --wait 15m

It behaves as "asgard-cli operate trigger fire" does: no invocation id comes
back, so --wait watches the refreshes for one that was not there before, and
the exit code is zero only when it succeeded within the wait. A refresh reads
the whole drive and can take a long time on a large one.

The platform accepts a reindex while a refresh is still running, and the two
then write the same graph at once. Check "index-runs" for a running one first.

Needs source-set/put in the project.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, scope, err := f.resolve(cmd)
			if err != nil {
				return err
			}
			src, err := contextIndexSource(pc, &f, scope, args[0])
			if err != nil {
				return err
			}
			return fireInvocation(cmd, pc, f.format, wait, src)
		},
	}
	f.register(cmd)
	cmd.Flags().DurationVar(&wait, "wait", 0,
		"how long to wait for the refresh to finish, e.g. 15m; 0, the default, returns once the run is accepted")
	return cmd
}

func newOperateIndexRunsCmd() *cobra.Command {
	var (
		f  operateFlags
		lf invocationFlags
	)
	cmd := &cobra.Command{
		Use:   "index-runs <source-set>",
		Short: "List a SourceSet's context-index refreshes, newest first",
		Long: `List a SourceSet's context-index refreshes, newest first: the scheduled ones
and the ones "reindex" started.

    asgard-cli operate source-set index-runs kb --release internal-dev

The columns are those of "asgard-cli operate trigger runs". --format json prints
the platform's array as it is; each entry's trigger_id is the Trigger the
platform derived for the index. Needs source-set/put in the project, reading
included.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q, err := lf.query()
			if err != nil {
				return err
			}
			pc, scope, err := f.resolve(cmd)
			if err != nil {
				return err
			}
			src, err := contextIndexSource(pc, &f, scope, args[0])
			if err != nil {
				return err
			}
			return listInvocations(cmd, f.format, src, q)
		},
	}
	f.register(cmd)
	lf.register(cmd)
	return cmd
}

func newOperateIndexLogsCmd() *cobra.Command {
	var (
		f     operateFlags
		limit int64
	)
	cmd := &cobra.Command{
		Use:   "index-logs <source-set> <invocation>",
		Short: "Print one context-index refresh's log",
		Long: `Print one context-index refresh's log, oldest line first.

    asgard-cli operate source-set index-logs kb 6f1c... --release internal-dev

The invocation id is the first column of "asgard-cli operate source-set
index-runs". The log is read through the Trigger the platform derived for the
index, which the refresh list names. Needs source-set/put and
project-resource/read in the project.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 0 {
				return fmt.Errorf("--limit cannot be negative")
			}
			pc, scope, err := f.resolve(cmd)
			if err != nil {
				return err
			}
			src, err := contextIndexSource(pc, &f, scope, args[0])
			if err != nil {
				return err
			}
			// The derived Trigger's name is whatever the platform reports on
			// a refresh; this does not rebuild it from the SourceSet's name.
			runs, _, err := src.list(cmd.Context(), platform.InvocationQuery{Size: 1})
			if err != nil {
				return err
			}
			if len(runs) == 0 {
				return fmt.Errorf("%s has no refreshes, so there is no log to read", src.What)
			}
			logs, err := pc.Client.InvocationLogs(cmd.Context(), scope.ProjectID, runs[0].TriggerID, args[1], limit)
			if err != nil {
				return err
			}
			return printInvocationLogs(cmd.OutOrStdout(), f.format, logs, "refresh "+args[1])
		},
	}
	f.register(cmd)
	cmd.Flags().Int64Var(&limit, "limit", 0, "at most this many lines; 0, the default, leaves it to the platform")
	return cmd
}
