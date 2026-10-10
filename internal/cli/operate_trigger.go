package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

func newOperateTriggerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trigger",
		Short: "Run a Trigger now, and read what its invocations did",
		Long: `Run a Trigger now, and read what its invocations did.

    asgard-cli operate trigger fire <name> --release <r> [--wait 5m]
    asgard-cli operate trigger runs <name> --release <r>
    asgard-cli operate trigger logs <name> <invocation> --release <r>

A Trigger says one sentence to an agent on its schedule, and each time it does
is an invocation: a conversation the agent may finish, fail, or stop part way
through to ask a question. "fire" is the schedule's run brought forward, which
is how a Trigger is tested without waiting for its cron.

An agent that stops to ask ends its invocation as succeeded, the same as one
that finished. "runs" prints the agent's own verdict beside the status, and
NEEDS_INPUT there means a question is waiting.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newOperateTriggerFireCmd(), newOperateTriggerRunsCmd(), newOperateTriggerLogsCmd())
	return cmd
}

// triggerSource is a Trigger's invocations, once its name is checked.
func triggerSource(pc *platformContext, f *operateFlags, scope *operateScope, name string) (invocationSource, error) {
	if _, err := scope.object("Trigger", name); err != nil {
		return invocationSource{}, err
	}
	project := scope.ProjectID
	return invocationSource{
		What: "Trigger " + name,
		list: func(ctx context.Context, q platform.InvocationQuery) ([]platform.Invocation, platform.Paging, error) {
			return pc.Client.TriggerInvocations(ctx, project, name, q)
		},
		fire:    func(ctx context.Context) error { return pc.Client.FireTrigger(ctx, project, name) },
		runsCmd: fmt.Sprintf("asgard-cli operate trigger runs %s %s", name, f.scopeFlag()),
		logsCmd: func(inv string) string {
			return fmt.Sprintf("asgard-cli operate trigger logs %s %s %s", name, inv, f.scopeFlag())
		},
	}, nil
}

func newOperateTriggerFireCmd() *cobra.Command {
	var (
		f    operateFlags
		wait time.Duration
	)
	cmd := &cobra.Command{
		Use:   "fire <trigger>",
		Short: "Start one invocation of a Trigger now",
		Long: `Start one invocation of a Trigger now, as its schedule would.

    asgard-cli operate trigger fire daily-report --release internal-dev --wait 5m

The platform answers with no invocation id, so without --wait this returns as
soon as the run is accepted. With --wait it watches the invocations for one
that was not there before, and returns when both the run and the conversation
have stopped: zero for succeeded, non-zero for failed and when the wait runs
out first. A succeeded invocation whose agent stopped to ask a question says so.

A fire is a Job that hands the sentence to the agent and ends within seconds;
the conversation then runs on its own. So a fire is refused only during those
seconds, and a fire while the previous conversation is still going starts a
second one beside it. Read "runs" first when that matters. Only a Trigger with
a schedule can be fired. Needs workflow/put in the project. In the
Workbench sandbox a command is cut off after about ten minutes, so keep --wait
under that and read the rest with "asgard-cli operate trigger runs".`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, scope, err := f.resolve(cmd)
			if err != nil {
				return err
			}
			src, err := triggerSource(pc, &f, scope, args[0])
			if err != nil {
				return err
			}
			return fireInvocation(cmd, pc, f.format, wait, src)
		},
	}
	f.register(cmd)
	cmd.Flags().DurationVar(&wait, "wait", 0,
		"how long to wait for the invocation to finish, e.g. 5m; 0, the default, returns once the run is accepted")
	return cmd
}

func newOperateTriggerRunsCmd() *cobra.Command {
	var (
		f  operateFlags
		lf invocationFlags
	)
	cmd := &cobra.Command{
		Use:   "runs <trigger>",
		Short: "List a Trigger's invocations, newest first",
		Long: `List a Trigger's invocations, newest first.

    asgard-cli operate trigger runs daily-report --release internal-dev --status failed

Each row is one invocation: its id, the run's status, the conversation's run
state with the agent's verdict, when it was invoked, how long it took and the
title the agent gave the conversation. "gone" means the conversation was never
created or has been reaped. A failed invocation's error follows on its own line.

--format json prints the platform's array as it is. Needs project-resource/read
in the project.`,
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
			src, err := triggerSource(pc, &f, scope, args[0])
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

func newOperateTriggerLogsCmd() *cobra.Command {
	var (
		f     operateFlags
		limit int64
	)
	cmd := &cobra.Command{
		Use:   "logs <trigger> <invocation>",
		Short: "Print one invocation's log",
		Long: `Print one invocation's log, oldest line first.

    asgard-cli operate trigger logs daily-report 6f1c... --release internal-dev

The invocation id is the first column of "asgard-cli operate trigger runs". The
log is the conversation as it completed: the sentence the Trigger sent, the
agent's thinking and replies, and each tool call. Text prints each line's
message, indented under its time; --format json keeps the frame each line came
from, which names the tool and carries its result. Needs project-resource/read
in the project.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 0 {
				return fmt.Errorf("--limit cannot be negative")
			}
			pc, scope, err := f.resolve(cmd)
			if err != nil {
				return err
			}
			if _, err := scope.object("Trigger", args[0]); err != nil {
				return err
			}
			logs, err := pc.Client.InvocationLogs(cmd.Context(), scope.ProjectID, args[0], args[1], limit)
			if err != nil {
				return err
			}
			return printInvocationLogs(cmd.OutOrStdout(), f.format, logs, "invocation "+args[1])
		},
	}
	f.register(cmd)
	cmd.Flags().Int64Var(&limit, "limit", 0, "at most this many lines; 0, the default, leaves it to the platform")
	return cmd
}
