package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/auth"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/selfupdate"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/version"
)

// backgroundLeash is how long the whole check gets when nobody asked for it.
//
// **It is shorter than the one `--check` gets**, and the difference is who is
// waiting. Somebody who typed `version --check` is waiting for this answer, so
// `selfupdate.Check` gives its own request room; everybody else is waiting for
// the command they ran, and a question they did not ask may not hold it up.
//
// **It is a deadline rather than a budget**, measured from before the command
// body runs. So the command's own time counts against it, and what is left
// when there is finally something to print is usually nothing - which is the
// intended shape: the check either answered while the work happened or it is
// dropped.
const backgroundLeash = 1500 * time.Millisecond

// updateCheck is a check in flight: where the answer arrives, and when this
// stops waiting for it. A zero value is "nothing was asked".
type updateCheck struct {
	result   <-chan selfupdate.Result
	deadline time.Time
}

// startUpdateCheck asks, beside the command rather than in front of it, whether
// a newer release is published. Its zero value is "nothing was asked".
//
// **It runs while the command runs.** A check that ran afterwards would add its
// own latency to every command that triggers one; started here, it has the
// whole command to answer in, and on the runs where the record is still fresh
// it answers off the disk with no call at all.
func startUpdateCheck(cmd *cobra.Command) updateCheck {
	// **Nothing is said where nobody can act on it.** This is a nag rather
	// than a result: in CI it is a line in a log that nobody upgrades from,
	// and on a piped stderr it is a line in somebody's parser. `actingOn`
	// prints unconditionally for the opposite reason - that one is a fact
	// about what the command just did to a platform.
	//
	// It is tested here and not inside `wantsUpdateCheck` because `go test`
	// never has a terminal, so a rule that read the file descriptor would make
	// every other rule in that function unreachable from a test.
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		return updateCheck{}
	}
	if !wantsUpdateCheck(cmd) {
		return updateCheck{}
	}
	home, err := auth.Home()
	if err != nil {
		return updateCheck{}
	}
	running := version.Get().Version

	ch := make(chan selfupdate.Result, 1)
	deadline := time.Now().Add(backgroundLeash)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	go func() {
		defer cancel()
		ch <- selfupdate.Check(ctx, home, running, false)
	}()
	return updateCheck{result: ch, deadline: deadline}
}

// wantsUpdateCheck says whether this run is one to ask on.
func wantsUpdateCheck(cmd *cobra.Command) bool {
	if os.Getenv(selfupdate.EnvDisable) != "" {
		return false
	}
	// `version --check` asks this question itself, with its own leash and its
	// own wording, and a second answer underneath it reads as a second
	// question. Completion runs are a shell's, not a person's.
	for c := cmd; c != nil; c = c.Parent() {
		if isOffline(c) {
			return false
		}
		switch c.Name() {
		case "version", "help", "completion", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
			return false
		}
	}
	return true
}

// offlineAnnotation marks a command that touches no network, so the background
// check does not ask on it either.
//
// **A request whose failure is silent is still a request.** It goes out from
// whatever network the command is run on, and the half that answers in a
// meeting - `init`, `size`, `guide` - is run on the customer's. So the mark is
// on the command rather than in a list here: the command is where its help
// makes the promise, and `TestAnOfflineClaimIsKept` holds the two together.
const offlineAnnotation = "asgard-cli/offline"

// touchesNoNetwork returns the annotation that marks a command as touching no network.
func touchesNoNetwork() map[string]string {
	return map[string]string{offlineAnnotation: "true"}
}

// isOffline says whether c is marked as touching no network.
func isOffline(c *cobra.Command) bool {
	return c.Annotations[offlineAnnotation] == "true"
}

// warnIfNewerRelease prints the one line, if the answer arrived and is yes.
//
// **Silence covers three different things and that is deliberate**: the check
// was skipped, it did not finish inside its leash, or there is nothing newer.
// None of the three is something the person who ran `asgard-cli render` needs
// to hear about, and a version check that reports its own failures is noise on
// every run of every machine that will never reach GitHub.
func warnIfNewerRelease(cmd *cobra.Command, uc updateCheck) {
	if uc.result == nil {
		return
	}
	var res selfupdate.Result
	select {
	case res = <-uc.result:
	case <-time.After(time.Until(uc.deadline)):
		// The record is written by the goroutine whether or not anybody is
		// still listening, so the next run reads an answer rather than asking
		// again - which is the whole point of there being a record.
		return
	}
	if !res.Newer {
		return
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "\nasgard-cli %s is published; this is %s\n    %s\n",
		res.Latest, res.Running, upgradeCommand())
}
