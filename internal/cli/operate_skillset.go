package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

// The group a SkillSet's sync commands are listed under.
const operateGroupSync = "sync"

func newOperateSkillSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill-set",
		Short: "Run the Syncer that fills a SkillSet, and read its files",
		Long: `Run the Syncer that fills a SkillSet, read what its runs did, and read its
files.

    asgard-cli operate skill-set sync <name> --release <r> [--wait 5m]
    asgard-cli operate skill-set executions <name> --release <r>
    asgard-cli operate skill-set ls <name> [path] --release <r>

A SkillSet's SourceSet and Syncer carry asgard-ai.com/managed-by: skill-set,
and the platform hides both from its drive routes: their runs are reached
through the SkillSet. The platform finds the Syncer itself, as the one with
that label whose SourceSet is the SkillSet's, and refuses a SkillSet that has
none.

Every run rebuilds the whole skill tree. A run cut off part way leaves a
partial tree that the agent answers from without an error, and the next run
that finishes repairs it.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddGroup(&cobra.Group{ID: operateGroupSync, Title: "Sync - the Syncer that fills it:"})
	addTo(cmd, operateGroupSync, newOperateSkillSetSyncCmd(), newOperateSkillSetExecutionsCmd())
	addVolumeCmds(cmd, skillSetFiles)
	return cmd
}

func newOperateSkillSetSyncCmd() *cobra.Command {
	var (
		f    operateFlags
		wait time.Duration
	)
	cmd := &cobra.Command{
		Use:   "sync <skill-set>",
		Short: "Start one run of the Syncer that fills a SkillSet",
		Long: `Start one run of the Syncer that fills a SkillSet, outside its schedule.

    asgard-cli operate skill-set sync support-skills --release internal-dev --wait 5m

It behaves as "asgard-cli operate syncer sync" does: no run id comes back, so
--wait watches the history for a run that was not there before, and the exit
code is zero only when that run Succeeded within the wait.

Needs skill-set/put in the project.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, scope, err := f.resolve(cmd)
			if err != nil {
				return err
			}
			name := args[0]
			if _, err := scope.object("SkillSet", name); err != nil {
				return err
			}
			list := func(ctx context.Context) ([]platform.SyncerExecution, error) {
				return pc.Client.SkillSetSyncExecutions(ctx, scope.ProjectID, name)
			}
			trigger := func(ctx context.Context) error {
				return pc.Client.TriggerSkillSetSync(ctx, scope.ProjectID, name)
			}
			next := fmt.Sprintf("asgard-cli operate skill-set executions %s %s", name, f.scopeFlag())
			return runSync(cmd, pc, &f, scope, wait, syncResult{Kind: "SkillSet", Name: name}, list, trigger, next)
		},
	}
	f.register(cmd)
	cmd.Flags().DurationVar(&wait, "wait", 0,
		"how long to wait for the run to finish, e.g. 5m; 0, the default, returns once the run is accepted")
	return cmd
}

func newOperateSkillSetExecutionsCmd() *cobra.Command {
	var f operateFlags
	cmd := &cobra.Command{
		Use:   "executions <skill-set>",
		Short: "List the runs of the Syncer that fills a SkillSet",
		Long: `List the runs of the Syncer that fills a SkillSet, newest first.

    asgard-cli operate skill-set executions support-skills --release internal-dev

The same history "asgard-cli operate syncer executions" prints for a drive's
Syncer, reached through the SkillSet because the platform hides its Syncer.
--format json prints the platform's array as it is. Needs project-resource/read
in the project.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, scope, err := f.resolve(cmd)
			if err != nil {
				return err
			}
			name := args[0]
			if _, err := scope.object("SkillSet", name); err != nil {
				return err
			}
			runs, err := pc.Client.SkillSetSyncExecutions(cmd.Context(), scope.ProjectID, name)
			if err != nil {
				return err
			}
			return printRuns(cmd.OutOrStdout(), f.format, runs, fmt.Sprintf("SkillSet %s in %s", name, scope.Where))
		},
	}
	f.register(cmd)
	return cmd
}
