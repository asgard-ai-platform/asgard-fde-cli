package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

// What a Trigger and a context index share: each run is an invocation, and
// each invocation is a conversation the agent may still be having, or may
// have stopped to ask a question in.

// invocationSource is one thing whose runs are invocations: a Trigger, or a
// SourceSet's context index.
type invocationSource struct {
	// What names it in messages - "Trigger daily-report".
	What string
	// list reads one page of its invocations, newest first.
	list func(context.Context, platform.InvocationQuery) ([]platform.Invocation, platform.Paging, error)
	// fire starts one run now.
	fire func(context.Context) error
	// runsCmd and logsCmd are the commands that read what fire started, for
	// the messages that send somebody there. logsCmd and chatCmd take the
	// invocation id.
	runsCmd string
	logsCmd func(invocation string) string
	chatCmd func(invocation string) string
}

// invocationFlags are the flags of the commands that list invocations.
type invocationFlags struct {
	status string
	limit  int64
}

func (f *invocationFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.status, "status", "",
		"only the invocations in this state: running, succeeded or failed; all of them by default")
	cmd.Flags().Int64Var(&f.limit, "limit", 20, "how many of the newest invocations to list, at most 100")
}

func (f *invocationFlags) query() (platform.InvocationQuery, error) {
	switch f.status {
	case "", platform.InvocationRunning, platform.InvocationSucceeded, platform.InvocationFailed:
	default:
		return platform.InvocationQuery{}, fmt.Errorf("--status is one of running, succeeded, failed; got %q", f.status)
	}
	if f.limit < 1 || f.limit > 100 {
		return platform.InvocationQuery{}, fmt.Errorf("--limit is between 1 and 100, which is what the platform pages by")
	}
	return platform.InvocationQuery{Status: f.status, Size: f.limit}, nil
}

// listInvocations prints a page of invocations.
func listInvocations(cmd *cobra.Command, format string, src invocationSource, q platform.InvocationQuery) error {
	runs, paging, err := src.list(cmd.Context(), q)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if format == formatJSON {
		if runs == nil {
			runs = []platform.Invocation{}
		}
		return writeJSON(out, runs)
	}
	if len(runs) == 0 {
		fmt.Fprintf(out, "%s has no invocations", src.What)
		if q.Status != "" {
			fmt.Fprintf(out, " in state %s", q.Status)
		}
		fmt.Fprintln(out, ".")
		return nil
	}
	fmt.Fprintf(out, "%s, newest first", src.What)
	if paging.Total > int64(len(runs)) {
		fmt.Fprintf(out, " (%d of %d)", len(runs), paging.Total)
	}
	fmt.Fprintf(out, ":\n\n")
	writeInvocations(out, runs)
	return nil
}

func writeInvocations(out io.Writer, runs []platform.Invocation) {
	fmt.Fprintf(out, "%-36s %-10s %-24s %-17s %-9s %s\n", "INVOCATION", "STATUS", "CONVERSATION", "INVOKED", "DURATION", "TITLE")
	for _, r := range runs {
		title := "-"
		if r.Channel != nil && r.Channel.Title != "" {
			title = r.Channel.Title
		}
		fmt.Fprintf(out, "%-36s %-10s %-24s %-17s %-9s %s\n", r.InvocationID, r.Status, conversationState(r),
			r.InvokedAt.Local().Format("2006-01-02 15:04"), invocationDuration(r), title)
		if r.ErrorMessage != nil && *r.ErrorMessage != "" {
			fmt.Fprintf(out, "    error: %s\n", firstLine(*r.ErrorMessage))
		}
	}
}

// conversationState is the run state and the agent's verdict together,
// because either alone misreads: IDLE says nothing about whether the agent
// finished or asked, and the verdict is empty until it gives one.
func conversationState(r platform.Invocation) string {
	if r.Channel == nil {
		return "gone"
	}
	if r.Channel.ConversationStatus == "" {
		return r.Channel.RunState
	}
	return r.Channel.RunState + " " + r.Channel.ConversationStatus
}

func invocationDuration(r platform.Invocation) string {
	if r.CompletedAt == nil {
		return "-"
	}
	return r.CompletedAt.Sub(r.InvokedAt).Round(time.Second).String()
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i] + " ..."
	}
	return s
}

// fireResult is what a fire reports, in both formats.
type fireResult struct {
	What       string               `json:"what"`
	Invocation *platform.Invocation `json:"invocation,omitempty"`
	// TimedOut is set when --wait ran out before the new invocation finished.
	TimedOut bool `json:"timed_out,omitempty"`
}

// fireInvocation is `fire` and `reindex`: start one run, and with a wait,
// follow the invocation that was not there before until it has finished.
func fireInvocation(cmd *cobra.Command, pc *platformContext, format string, wait time.Duration, src invocationSource) error {
	ctx := cmd.Context()
	if wait < 0 {
		return fmt.Errorf("--wait cannot be negative")
	}
	firstPage := platform.InvocationQuery{Size: 100}
	var before []platform.Invocation
	if wait > 0 {
		var err error
		if before, _, err = src.list(ctx, firstPage); err != nil {
			return err
		}
	}

	actingOn(cmd, pc.Session)
	if err := src.fire(ctx); err != nil {
		var apiErr *platform.APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusConflict {
			return fmt.Errorf("%w\nThe previous run is still being dispatched, which takes a few seconds. See where it is:\n\n    %s", err, src.runsCmd)
		}
		return err
	}
	out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()
	res := fireResult{What: src.What}

	if wait == 0 {
		if format == formatJSON {
			return writeJSON(out, res)
		}
		fmt.Fprintf(out, "Started a run of %s.\nThe platform answers with no invocation id; the newest one is it once it appears:\n\n    %s\n", src.What, src.runsCmd)
		return nil
	}

	fmt.Fprintf(errOut, "started a run of %s; waiting up to %s for it to finish\n", src.What, wait)
	waitCtx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	got, timedOut, err := waitForNewInvocation(waitCtx, before, func(ctx context.Context) ([]platform.Invocation, error) {
		runs, _, err := src.list(ctx, firstPage)
		return runs, err
	}, pollSleep)
	if err != nil {
		return err
	}
	res.Invocation, res.TimedOut = got, timedOut
	failed := timedOut || got == nil || got.Status == platform.InvocationFailed

	if format == formatJSON {
		if err := writeJSON(out, res); err != nil {
			return err
		}
		if failed {
			return ErrSilent
		}
		return nil
	}
	switch {
	case got == nil:
		return fmt.Errorf("no new invocation of %s appeared within %s. The run was accepted; read the list later:\n\n    %s", src.What, wait, src.runsCmd)
	case timedOut:
		return fmt.Errorf("invocation %s of %s is still %s after %s; read where it is later:\n\n    %s",
			got.InvocationID, src.What, conversationState(*got), wait, src.runsCmd)
	case got.Status == platform.InvocationFailed:
		msg := ""
		if got.ErrorMessage != nil {
			msg = ": " + firstLine(*got.ErrorMessage)
		}
		return fmt.Errorf("invocation %s of %s failed%s. Its log:\n\n    %s", got.InvocationID, src.What, msg, src.logsCmd(got.InvocationID))
	}
	writeInvocations(out, []platform.Invocation{*got})
	if got.Channel != nil && got.Channel.ConversationStatus == platform.ChannelNeedsInput {
		fmt.Fprintf(out, "\nThe agent stopped to ask a question instead of finishing. The run counts as succeeded, and the\n"+
			"question is waiting in the conversation. Read it, and answer it:\n\n    %s\n", src.chatCmd(got.InvocationID))
	}
	return nil
}

// waitForNewInvocation polls until an invocation that was not in before has
// finished, or ctx ends. It reports the newest new invocation and whether the
// wait ran out first.
//
// The fire answers with no id, so the new invocation is the one not there
// before; one the schedule started in the same window cannot be told apart,
// which the platform makes unavoidable.
func waitForNewInvocation(ctx context.Context, before []platform.Invocation,
	list func(context.Context) ([]platform.Invocation, error), sleep func(context.Context) error) (*platform.Invocation, bool, error) {
	seen := make(map[string]bool, len(before))
	for _, r := range before {
		seen[r.InvocationID] = true
	}
	var latest *platform.Invocation
	for {
		runs, err := list(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return latest, true, nil
			}
			return nil, false, err
		}
		latest = nil
		for i := range runs {
			if !seen[runs[i].InvocationID] && (latest == nil || runs[i].InvokedAt.After(latest.InvokedAt)) {
				r := runs[i]
				latest = &r
			}
		}
		if latest != nil && latest.Finished() {
			return latest, false, nil
		}
		if err := sleep(ctx); err != nil {
			return latest, true, nil
		}
	}
}

// pollSleep waits one poll interval, or until ctx ends.
func pollSleep(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(runPollInterval):
		return nil
	}
}

// printInvocationLogs writes an invocation's log, one line per entry.
func printInvocationLogs(out io.Writer, format string, logs []platform.InvocationLog, what string) error {
	if format == formatJSON {
		if logs == nil {
			logs = []platform.InvocationLog{}
		}
		return writeJSON(out, logs)
	}
	if len(logs) == 0 {
		fmt.Fprintf(out, "%s has no log lines.\n", what)
		return nil
	}
	for _, l := range logs {
		if l.Entry == nil {
			continue
		}
		lines := strings.Split(strings.TrimRight(l.Entry.Message, "\n"), "\n")
		fmt.Fprintf(out, "%s %-5s %s\n", l.Entry.Timestamp.Local().Format("2006-01-02 15:04:05"), l.Entry.Level, lines[0])
		for _, line := range lines[1:] {
			if line == "" {
				fmt.Fprintln(out)
				continue
			}
			fmt.Fprintf(out, "    %s\n", line)
		}
	}
	return nil
}
