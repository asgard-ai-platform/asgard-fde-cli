package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/auth"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

// pipeline connect in the Workbench assistant's sandbox (asgard-odin-pm
// decision 2026-09-30-workbench-sandbox-fde-browser-choice FDE-18).
//
// On a desktop the command opens the browser, prints the URL and waits, in one
// process. In the sandbox the agent sees a command's output only when it ends,
// so a URL printed before a five-minute wait would reach the member five
// minutes late. So there it is two steps: `pipeline connect` prints the link
// and ends, the agent gives the link to the member - for their own browser,
// which is signed in to GitHub already - and `pipeline connect --continue` waits
// for the connection to appear, exactly as the desktop's wait does. When the
// flow needs a second page (installing the app on an account nobody has
// installed it on), --continue ends again with that link, and the next
// --continue picks up from there.

// errConnectPending ends a step that handed the member a link to open.
var errConnectPending = errors.New("waiting for the member to open a link")

// errSandboxConnectNeedsAccount is connect's first step without an account to
// connect and no origin remote to take one from. No link is started: one that
// ends in "which account?" after the member has clicked it is a link wasted.
var errSandboxConnectNeedsAccount = errors.New("which GitHub account to connect is not known yet, so no link was started.\n" +
	"Ask the member which GitHub organisation (or personal account) to connect - the account the app is\n" +
	"installed on, not the person who will authorize - then run\n" +
	"    asgard-cli pipeline connect --account <login>\n" +
	"An organisation that does not have the app yet is fine: the flow installs it there.")

// connectPending is what the two steps share: which flow, and what the
// workspace held before it started. It is kept beside the Workbench session
// file, 0600 - the state is a bearer for the install flow.
type connectPending struct {
	Workspace string `json:"workspace"`
	// Stage is "attach" (authorize, and connect an installation the member
	// reaches) or "install" (install the app on an account first).
	Stage     string    `json:"stage"`
	State     string    `json:"state"`
	Before    []string  `json:"before"`
	Account   string    `json:"account,omitempty"`
	StartedAt time.Time `json:"started_at"`
}

func connectPendingPath() string {
	return filepath.Join(filepath.Dir(auth.SessionFilePath()), "pipeline-connect.json")
}

func saveConnectPending(p *connectPending) error {
	path := connectPendingPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(p, "", "  ")
	return os.WriteFile(path, b, 0o600)
}

func loadConnectPending() (*connectPending, error) {
	b, err := os.ReadFile(connectPendingPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("no connection is being set up; `asgard-cli pipeline connect` starts one")
	}
	if err != nil {
		return nil, err
	}
	var p connectPending
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("the pending connection %s is unreadable: %w", connectPendingPath(), err)
	}
	return &p, nil
}

func idsOf(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out
}

func setOf(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// handLinkToMember is what the agent is told to do with a link in the
// sandbox: give it to the member, for their own browser.
func handLinkToMember(w io.Writer, purpose, url string, expires *time.Time) {
	fmt.Fprintf(w, "Give the member this link, as a link in your reply, to open in THEIR OWN\n"+
		"browser - not the sandbox's, which is not signed in to GitHub:\n\n    %s\n\n"+
		"There they %s.\n", url, purpose)
	if expires != nil {
		fmt.Fprintf(w, "The link expires %s.\n", expiryLabel(*expires))
	}
	fmt.Fprintf(w, "\nThen run `asgard-cli pipeline connect --continue`. It notices the connection by\n"+
		"itself, so do not ask the member to say they are done. If it gives up, say so\n"+
		"and stop; run it again once the member is back.\n")
}

// startConnectInSandbox is the first step: the attach flow's link, and the
// snapshot the wait compares against.
func startConnectInSandbox(cmd *cobra.Command, pc *platformContext, before map[string]bool, account string) error {
	install, err := pc.Client.BeginGitHubAttach(cmd.Context())
	if err != nil {
		return err
	}
	if err := saveConnectPending(&connectPending{
		Workspace: pc.Workspace, Stage: "attach", State: install.State,
		Before: idsOf(before), Account: account, StartedAt: time.Now(),
	}); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Connecting GitHub to workspace %s.\n\n", pc.Workspace)
	handLinkToMember(cmd.OutOrStdout(), "authorize the app, so GitHub can say which accounts they can connect", install.InstallUrl, install.ExpiresAt)
	return nil
}

// continueConnectInSandbox is --continue: pick the flow up where the last step
// left it.
func continueConnectInSandbox(cmd *cobra.Command, pc *platformContext, format string, wait time.Duration, account string) error {
	p, err := loadConnectPending()
	if err != nil {
		return err
	}
	// --account here replaces the one the first step recorded. That is the
	// way out of a flow that started without one, and the error that says so
	// names exactly this command - it used to be ignored, so following the
	// instruction failed the same way and the member had to authorize again.
	if account != "" && !strings.EqualFold(account, p.Account) {
		p.Account = account
		if err := saveConnectPending(p); err != nil {
			return err
		}
	}
	if p.Workspace != pc.Workspace {
		return fmt.Errorf("the connection being set up is for workspace %s, not %s; pass --workspace %s, or start over with `asgard-cli pipeline connect`",
			p.Workspace, pc.Workspace, p.Workspace)
	}
	before := setOf(p.Before)
	fmt.Fprintf(cmd.ErrOrStderr(), "Waiting for the connection to appear in workspace %s...\n", pc.Workspace)
	created, choices, insufficient, err := waitForConnection(cmd.Context(), pc, before, p.State, wait)
	if err != nil {
		// The flow may still finish; the pending state stays for the next --continue.
		return fmt.Errorf("%w\nThe link stays valid until it expires; once the member has finished on GitHub, run `asgard-cli pipeline connect --continue` again", err)
	}
	switch p.Stage {
	case "install":
		switch {
		case choices != nil:
			created, err = attachInstalled(cmd, pc, choices, p.Account)
		case created == nil:
			err = errors.New("the app was installed, but the account that authorized reaches no installation of it.\n" +
				"Authorize as an account with repository access to the one it was installed on")
		case p.Account != "" && !strings.EqualFold(created.AccountLogin, p.Account):
			err = fmt.Errorf("the app was installed on %q rather than the %q that was asked for, and %q is what this workspace is now connected to",
				created.AccountLogin, p.Account, created.AccountLogin)
		}
	default:
		switch {
		case choices != nil:
			created, err = attachNamedAccount(cmd, pc, choices, p.Account, before, true, wait)
		case insufficient:
			fmt.Fprintf(cmd.ErrOrStderr(), "\nThe member reaches no installation of this app yet, so it has to be installed first.\n")
			created, err = installFirst(cmd, pc, before, p.Account, true, wait)
		}
	}
	if errors.Is(err, errConnectPending) {
		return nil
	}
	if err != nil {
		return err
	}
	_ = os.Remove(connectPendingPath())
	return printConnected(cmd, format, created)
}

// installInSandbox is installFirst's sandbox half: hand the member the install
// link, record where the flow is, and end the step.
func installInSandbox(cmd *cobra.Command, pc *platformContext, before map[string]bool, account string) error {
	install, err := pc.Client.BeginGitHubInstall(cmd.Context())
	if err != nil {
		return err
	}
	if err := saveConnectPending(&connectPending{
		Workspace: pc.Workspace, Stage: "install", State: install.State,
		Before: idsOf(before), Account: account, StartedAt: time.Now(),
	}); err != nil {
		return err
	}
	where := "the account this engagement is about"
	if account != "" {
		where = account
	}
	out := cmd.OutOrStdout()
	fmt.Fprintln(out)
	handLinkToMember(out, "install the app on "+where+
		". Installing on an organisation may need one of its admins to approve it; the flow then waits for them. If so, say who has to do it",
		install.InstallUrl, install.ExpiresAt)
	return errConnectPending
}

func printConnected(cmd *cobra.Command, format string, created *platform.VcsConnection) error {
	out := cmd.OutOrStdout()
	if format == formatJSON {
		return writeJSON(out, created)
	}
	fmt.Fprintf(out, "Connected %s (%s), installation %s, connection %s.\n",
		created.AccountLogin, created.AccountType, created.InstallationId, created.ConnectionId)
	fmt.Fprintf(out, "\n`asgard-cli pipeline repos --connection %s` lists what it can reach; a\nrepository missing from that list is one the installation was not granted,\nwhich is changed on the provider rather than here.\n", created.ConnectionId)
	return nil
}
