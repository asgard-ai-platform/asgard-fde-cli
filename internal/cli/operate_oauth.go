package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/browser"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

func newOperateOAuthCredentialCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "oauth-credential",
		Short: "Grant an OAuthCredential, and read whether it holds a token",
		Long: `Grant an OAuthCredential, and read whether it holds a token.

    asgard-cli operate oauth-credential authorize <name> --release <r>
    asgard-cli operate oauth-credential status <name> --release <r> [--wait 10m]

A chart can declare an OAuthCredential, but not the grant behind it: a token
exists only once a person has signed in to the service - a Google Drive, a
OneDrive, a Dropbox - and consented. Until then the credential sits in PENDING,
and a Syncer or a sandbox that reads it fails at runtime while every check
before that passes. So a release that deploys one has a step after the deploy,
and this is it.

"authorize" asks the platform for a link and puts it in front of the person who
signs in. That person has to be the account whose data the Syncer is meant to
read, which is not necessarily whoever runs this command - say whose account
it is when handing the link over.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newOperateOAuthAuthorizeCmd(), newOperateOAuthStatusCmd())
	return cmd
}

func newOperateOAuthAuthorizeCmd() *cobra.Command {
	var (
		f         operateFlags
		wait      time.Duration
		noBrowser bool
	)
	cmd := &cobra.Command{
		Use:   "authorize <oauth-credential>",
		Short: "Start a grant, and hand over the link that completes it",
		Long: `Start a grant of an OAuthCredential, and hand over the link that completes it.

    asgard-cli operate oauth-credential authorize gdrive-sales --release internal-dev --wait 10m

The link opens the service's own sign-in and consent page. It works once and
expires about fifteen minutes after it is issued; running this again issues a
new one. It carries no session, so it can be opened in any browser, by the
person whose account it is.

Where the link goes depends on where this runs:

    on a desktop                 the desktop's browser opens it; --no-browser
                                 prints it instead
    in the Workbench sandbox     it is printed for the member, to open in their
                                 own browser, and nothing waits

--wait is honoured only when this machine's browser opened the link, because
otherwise the link has to reach somebody before anything can happen, and a
command that is still waiting has not shown it to anyone. It waits for a new
token: READY on a Secret that was not there before, so re-granting a
credential that is already READY waits for the new grant rather than
returning at once. Otherwise follow up with
"asgard-cli operate oauth-credential status <name> --wait 10m".

A grant replaces the token, and with it whose data the Syncer reads. The
platform does not check the account. Needs a member of the project.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if wait < 0 {
				return fmt.Errorf("--wait cannot be negative")
			}
			pc, scope, err := f.resolve(cmd)
			if err != nil {
				return err
			}
			name := args[0]
			if _, err := scope.object("OAuthCredential", name); err != nil {
				return err
			}
			ctx := cmd.Context()
			before, err := pc.Client.GetOAuthCredential(ctx, scope.ProjectID, name)
			if err != nil {
				return err
			}

			actingOn(cmd, pc.Session)
			link, err := pc.Client.AuthorizeOAuthCredential(ctx, scope.ProjectID, name)
			if err != nil {
				return err
			}
			handoff, openErr := browser.Present(ctx, link, noBrowser)
			out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()
			status := fmt.Sprintf("asgard-cli operate oauth-credential status %s %s --wait 10m", name, f.scopeFlag())

			res := authorizeResult{Name: name, PhaseBefore: before.Status.Phase, AuthorizationURL: link, Opened: handoff == browser.OpenedLocally}
			if handoff != browser.OpenedLocally || wait == 0 {
				if f.format == formatJSON {
					return writeJSON(out, res)
				}
				switch handoff {
				case browser.OpenedLocally:
					fmt.Fprintf(out, "Opened the grant for OAuthCredential %s in this machine's browser.\n", name)
				case browser.ForTheMember:
					fmt.Fprintf(out, "Give the member this link, and say whose account has to sign in. It works once, for about fifteen minutes:\n\n    %s\n", link)
				default:
					if openErr != nil {
						fmt.Fprintf(errOut, "could not open a browser: %v\n", openErr)
					}
					fmt.Fprintf(out, "Open this link, signed in as the account whose data it is for. It works once, for about fifteen minutes:\n\n    %s\n", link)
				}
				fmt.Fprintf(out, "\nIt was %s before this. Once the sign-in is done:\n\n    %s\n", phaseOrNone(before.Status.Phase), status)
				return nil
			}

			fmt.Fprintf(errOut, "opened the grant for OAuthCredential %s in this machine's browser; waiting up to %s for a new token\n", name, wait)
			waitCtx, cancel := context.WithTimeout(ctx, wait)
			defer cancel()
			after, timedOut, err := waitForOAuth(waitCtx, func(ctx context.Context) (*platform.OAuthCredential, error) {
				return pc.Client.GetOAuthCredential(ctx, scope.ProjectID, name)
			}, func(c *platform.OAuthCredential) bool { return newGrant(before, c) }, pollSleep)
			if err != nil {
				return err
			}
			res.Credential, res.TimedOut = after, timedOut
			return reportOAuth(cmd, f.format, res, timedOut, after, wait,
				"the browser page reported an error, nothing was recorded; run authorize again for a new link")
		},
	}
	f.register(cmd)
	cmd.Flags().DurationVar(&wait, "wait", 0,
		"how long to wait for the grant to be completed, e.g. 10m; only when this machine's browser opened the link. 0, the default, returns at once")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "print the link instead of opening this machine's browser")
	return cmd
}

func newOperateOAuthStatusCmd() *cobra.Command {
	var (
		f    operateFlags
		wait time.Duration
	)
	cmd := &cobra.Command{
		Use:   "status <oauth-credential>",
		Short: "Whether an OAuthCredential holds a usable token",
		Long: `Whether an OAuthCredential holds a usable token, and until when.

    asgard-cli operate oauth-credential status gdrive-sales --release internal-dev
    asgard-cli operate oauth-credential status gdrive-sales --release internal-dev --wait 10m

The phase is the platform's:

    PENDING      no token yet - nobody has completed a grant - or a new
                 token is being taken up
    READY        a token is held, until the expiry printed
    REFRESHING   the platform is renewing it before it expires
    EXPIRED      it expired and was not renewed; grant it again
    FAILED       the message says why; usually a grant again fixes it

--wait waits until the phase is READY, FAILED or EXPIRED. On a credential that
is already READY it returns at once, so after re-granting one that was READY,
"authorize --wait" on a desktop is what tells the new token from the old.
Exit code: zero for READY, non-zero for anything else. --format json prints the
platform's object as it is.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if wait < 0 {
				return fmt.Errorf("--wait cannot be negative")
			}
			pc, scope, err := f.resolve(cmd)
			if err != nil {
				return err
			}
			name := args[0]
			if _, err := scope.object("OAuthCredential", name); err != nil {
				return err
			}
			get := func(ctx context.Context) (*platform.OAuthCredential, error) {
				return pc.Client.GetOAuthCredential(ctx, scope.ProjectID, name)
			}
			cred, timedOut := (*platform.OAuthCredential)(nil), false
			if wait > 0 {
				waitCtx, cancel := context.WithTimeout(cmd.Context(), wait)
				defer cancel()
				cred, timedOut, err = waitForOAuth(waitCtx, get, settled, pollSleep)
			} else {
				cred, err = get(cmd.Context())
			}
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if f.format == formatJSON {
				if err := writeJSON(out, cred); err != nil {
					return err
				}
				if cred == nil || cred.Status.Phase != platform.OAuthPhaseReady {
					return ErrSilent
				}
				return nil
			}
			if cred == nil {
				return fmt.Errorf("OAuthCredential %s could not be read within %s", name, wait)
			}
			writeOAuth(out, cred)
			if cred.Status.Phase == platform.OAuthPhaseReady {
				return nil
			}
			again := fmt.Sprintf("asgard-cli operate oauth-credential authorize %s %s", name, f.scopeFlag())
			if timedOut {
				return fmt.Errorf("OAuthCredential %s is still %s after %s. If nobody has completed the grant, a new link:\n\n    %s",
					name, phaseOrNone(cred.Status.Phase), wait, again)
			}
			return fmt.Errorf("OAuthCredential %s is %s, so whatever reads it fails. A grant:\n\n    %s",
				name, phaseOrNone(cred.Status.Phase), again)
		},
	}
	f.register(cmd)
	cmd.Flags().DurationVar(&wait, "wait", 0,
		"how long to wait for the phase to settle on READY, FAILED or EXPIRED, e.g. 10m; 0, the default, reads it once")
	return cmd
}

// authorizeResult is what `authorize` reports in json.
type authorizeResult struct {
	Name             string `json:"name"`
	PhaseBefore      string `json:"phase_before"`
	AuthorizationURL string `json:"authorization_url"`
	// Opened is whether this machine's browser was asked to open the link.
	Opened     bool                      `json:"opened"`
	Credential *platform.OAuthCredential `json:"credential,omitempty"`
	TimedOut   bool                      `json:"timed_out,omitempty"`
}

// newGrant reports whether after holds a token a grant made since before was
// read, or has failed since then.
//
// A grant writes a new Secret, so a new token is READY on a Secret version
// that was not there before. A failure is a FAILED that was not already the
// state, or that says something new.
func newGrant(before, after *platform.OAuthCredential) bool {
	switch after.Status.Phase {
	case platform.OAuthPhaseReady:
		return after.Status.SyncedSecretVersion != before.Status.SyncedSecretVersion || before.Status.Phase != platform.OAuthPhaseReady
	case platform.OAuthPhaseFailed:
		return before.Status.Phase != platform.OAuthPhaseFailed || after.Status.Message != before.Status.Message
	}
	return false
}

// settled reports whether a credential is in a phase that will not change
// without somebody acting.
func settled(c *platform.OAuthCredential) bool {
	switch c.Status.Phase {
	case platform.OAuthPhaseReady, platform.OAuthPhaseFailed, platform.OAuthPhaseExpired:
		return true
	}
	return false
}

// waitForOAuth polls a credential until done says so or ctx ends, and reports
// the last read and whether the wait ran out.
func waitForOAuth(ctx context.Context, get func(context.Context) (*platform.OAuthCredential, error),
	done func(*platform.OAuthCredential) bool, sleep func(context.Context) error) (*platform.OAuthCredential, bool, error) {
	var last *platform.OAuthCredential
	for {
		c, err := get(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return last, true, nil
			}
			return nil, false, err
		}
		last = c
		if done(c) {
			return c, false, nil
		}
		if err := sleep(ctx); err != nil {
			return last, true, nil
		}
	}
}

// reportOAuth finishes `authorize --wait`.
func reportOAuth(cmd *cobra.Command, format string, res authorizeResult, timedOut bool,
	cred *platform.OAuthCredential, wait time.Duration, timeoutHint string) error {
	ready := !timedOut && cred != nil && cred.Status.Phase == platform.OAuthPhaseReady
	if ready {
		// The grant changed what the platform holds, though this process did
		// not make the call that changed it.
		platform.NoteSideEffect()
	}
	out := cmd.OutOrStdout()
	if format == formatJSON {
		if err := writeJSON(out, res); err != nil {
			return err
		}
		if !ready {
			return ErrSilent
		}
		return nil
	}
	if timedOut {
		return fmt.Errorf("no new token for OAuthCredential %s within %s. If %s", res.Name, wait, timeoutHint)
	}
	writeOAuth(out, cred)
	if !ready {
		return fmt.Errorf("the grant for OAuthCredential %s failed: %s", res.Name, cred.Status.Message)
	}
	return nil
}

func writeOAuth(out io.Writer, c *platform.OAuthCredential) {
	fmt.Fprintf(out, "%-14s %s\n", "credential", c.ID)
	if c.Name != "" && c.Name != c.ID {
		fmt.Fprintf(out, "%-14s %s\n", "name", c.Name)
	}
	fmt.Fprintf(out, "%-14s %s\n", "provider", c.ProviderID)
	fmt.Fprintf(out, "%-14s %s\n", "phase", phaseOrNone(c.Status.Phase))
	if c.Status.Message != "" {
		fmt.Fprintf(out, "%-14s %s\n", "message", c.Status.Message)
	}
	if c.Status.ExpiresAt != "" {
		fmt.Fprintf(out, "%-14s %s\n", "token expires", c.Status.ExpiresAt)
	}
	if c.Status.LastUpdatedAt != "" {
		fmt.Fprintf(out, "%-14s %s\n", "updated", c.Status.LastUpdatedAt)
	}
	renew := "no - it has to be granted again when it expires"
	if c.RefreshToken.AutoRefresh {
		renew = fmt.Sprintf("yes, %ds before it expires", c.RefreshToken.AheadSeconds)
	}
	fmt.Fprintf(out, "%-14s %s\n", "auto-renew", renew)
}

// phaseOrNone names a phase, or says there is none yet - a credential the
// reconciler has not looked at reports an empty one.
func phaseOrNone(phase string) string {
	if phase == "" {
		return "without a phase yet"
	}
	return phase
}
