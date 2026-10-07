package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/auth"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

// inSandboxSession reports whether the identity comes from the Workbench
// sandbox's session file rather than from a sign-in: sandbox mode, and no
// ASGARD_TOKEN overriding it.
func inSandboxSession() bool {
	return auth.SandboxMode() && os.Getenv(auth.EnvToken) == ""
}

// sandboxIdentityNote is what login and logout say in the Workbench sandbox,
// where neither has anything to do.
const sandboxIdentityNote = `This is the Workbench assistant's sandbox: the platform supplies the member's
identity on every turn (the session file ` + auth.DefaultSessionFile + `), so there is nothing to
sign in to or out of. Every command already acts as the member, in the workspace
this conversation is in. "asgard-cli whoami" shows who that is.`

// profileFlag is the name of the flag that selects a platform environment.
const profileFlag = "profile"

// addProfileFlag registers --profile on a command that talks to the platform.
//
// It is deliberately not a persistent flag on the root: the knowledge commands
// work with no network and no session, and a flag in their help implies a
// choice that changes nothing about what they answer.
func addProfileFlag(cmd *cobra.Command, target *string) {
	// No backquotes in a flag's usage string: cobra reads the first
	// backquoted word as the placeholder to print after the flag name.
	cmd.Flags().StringVar(target, profileFlag, "",
		fmt.Sprintf("which platform to act on; defaults to %s, then %q. \"asgard-cli profile list\" shows them",
			auth.EnvProfile, auth.DefaultProfileName))
}

func newLoginCmd() *cobra.Command {
	var (
		profile   string
		noBrowser bool
		format    string
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in to the Asgard platform",
		Long: `Sign in to the Asgard platform, so that the pipeline commands can act as you.

It opens a browser at the platform's sign-in page, waits for it to come back on a
loopback port, and stores the session under this user account - never inside a
customer repository. The flow is OAuth 2.0 authorization code with PKCE and the
CLI ships no client secret, so a release contains no secret to extract.

The session lasts 24 hours and renews itself for 30 days without asking again;
after that, or once it is revoked, the next command says to run this one.

    asgard-cli login                     sign in to the hosted platform
    asgard-cli login --profile onprem    sign in to an installation you configured
    asgard-cli login --no-browser        do not open a browser; the URL is
                                         printed either way

With nothing configured, the only platform is the hosted one. That is the
profile named ` + "`default`" + `, and every command falls back to it.
` + "`prod`" + ` and ` + "`dev`" + ` are not built-in names. Configure a development platform like
any other installation, with ` + "`asgard-cli profile set`" + `. Passing either name
reports this instead of signing you in.

A session is held per profile, so signing in to one leaves the others alone and
several can be held at once.

When nothing names a profile, the profile is ` + "`default`" + `; no file records a
different default, because a preference stored on one machine is not shared by
two people running the same command. Name the profile per command with
--profile, or once per shell:

    export ASGARD_PROFILE=onprem

An exported variable is visible in the shell that set it, where a file under
the user's config directory is not, so it is harder to forget that a command
will reach a customer's platform rather than a test one.

IN THE WORKBENCH ASSISTANT'S SANDBOX (` + auth.EnvSandboxMode + `=true) there is nothing to sign in
to: the platform supplies the member's identity, and this command says so and
exits.

WITH NO BROWSER - CI, a container, an agent sandbox - do not use this command.
Set ASGARD_TOKEN to an access token instead: it bypasses the store completely,
reading nothing from disk and writing nothing to it. Over SSH, --no-browser plus
an "ssh -L" forward of the printed port works, because the browser has to reach
the loopback address this process is listening on.

It fails when the browser never comes back (five minutes), when the redirect does
not carry the state value this run generated - which means it was not this run's
- and when the platform refuses the sign-in, which it reports in full.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := checkFormat(format); err != nil {
				return err
			}
			if inSandboxSession() {
				fmt.Fprintln(cmd.OutOrStdout(), sandboxIdentityNote)
				return nil
			}
			r, err := auth.ResolveWithOrigin(profile)
			if err != nil {
				return err
			}
			p := r.Profile

			// Progress goes to stderr so that a --format json run's stdout is
			// the answer and nothing else.
			cred, info, err := auth.Login(cmd.Context(), auth.LoginOptions{
				Profile:     p,
				ProfileFrom: r.NameFrom,
				NoBrowser:   noBrowser,
				Out:         cmd.ErrOrStderr(),
			})
			if err != nil {
				return err
			}
			if err := auth.SaveCredential(p, cred); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if format == formatJSON {
				return writeJSON(out, map[string]any{
					"profile":      p.Name,
					"issuer":       p.Issuer,
					"platform_api": p.PlatformAPI,
					"subject":      info.Sub,
					"email":        info.Email,
					"name":         info.Who(),
					"expiresAt":    cred.ExpiresAt,
				})
			}

			fmt.Fprintf(out, "Signed in to %s as %s", p.Name, info.Who())
			if info.Email != "" && info.Email != info.Who() {
				fmt.Fprintf(out, " <%s>", info.Email)
			}
			fmt.Fprintf(out, ".\nThe session expires %s and renews itself until then.\n",
				cred.ExpiresAt.Local().Format("2006-01-02 15:04"))
			// Said whenever the profile was chosen rather than defaulted,
			// because nothing on disk remembers it for the next command.
			if p.Name != auth.DefaultProfileName {
				fmt.Fprintf(out, "\nEvery command still defaults to %s. Say %s per command,\nor `export %s=%s` once for this shell.\n",
					auth.DefaultProfileName, profileArgFor(p.Name), auth.EnvProfile, p.Name)
			}
			return nil
		},
	}

	addProfileFlag(cmd, &profile)
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false,
		"do not open a browser; the sign-in URL is printed either way, and the loopback port still has to be reachable from wherever it is opened")
	cmd.Flags().StringVar(&format, formatFlag, formatText, formatUsage)

	return cmd
}

func newLogoutCmd() *cobra.Command {
	var (
		profile string
		all     bool
	)

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Forget the stored session",
		Long: `Forget the stored session for a profile.

It deletes this machine's copy of the tokens and nothing else: the session is not
revoked at the platform, and any other machine holding one keeps it. Signing out
of a profile that has no session is not an error - running this twice reports the
same thing both times.

    asgard-cli logout                    forget the hosted platform's session
    asgard-cli logout --profile onprem   forget one installation's
    asgard-cli logout --all              forget every profile's

--all does not take --profile, because it means every one of them.

In the Workbench assistant's sandbox there is no stored session, and this says
so.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			if inSandboxSession() {
				fmt.Fprintln(out, sandboxIdentityNote)
				return nil
			}

			if all {
				if profile != "" {
					return errors.New("--all forgets every profile, so it cannot be combined with --" + profileFlag)
				}
				n, err := auth.DeleteAllCredentials()
				if err != nil {
					return err
				}
				if n == 0 {
					fmt.Fprintln(out, "No stored sessions.")
					return nil
				}
				fmt.Fprintf(out, "Forgot %d stored session(s).\n", n)
				return nil
			}

			r, err := auth.ResolveWithOrigin(profile)
			if err != nil {
				return err
			}
			p := r.Profile
			had, err := auth.DeleteCredential(p.Name)
			if err != nil {
				return err
			}
			if !had {
				fmt.Fprintf(out, "No stored session for %s.\n", p.Name)
				return nil
			}
			// Which installation's session, not just which local label: the
			// profile in effect can be one nobody remembers setting, and this
			// is the command whose effect is "you are no longer signed in to
			// something" - worth naming the something.
			fmt.Fprintf(out, "Forgot the session for %s  (profile %s, from %s).\n",
				p.PlatformAPI, p.Name, r.NameFrom)
			fmt.Fprintf(out, "It is not revoked at the platform.\n")
			return nil
		},
	}

	addProfileFlag(cmd, &profile)
	cmd.Flags().BoolVar(&all, "all", false, "forget every profile's session, not just one")

	return cmd
}

func newWhoamiCmd() *cobra.Command {
	var (
		profile string
		format  string
		local   bool
	)

	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Report who the stored session belongs to",
		Long: `Report who the stored session belongs to, and against which platform.

By default it asks the platform rather than reading the file: the same endpoint
the platform's own IAM calls to verify a bearer token, so what it reports is the
session the API would see. A token that has been revoked reads as signed in on
disk and is rejected by every call; asking the platform tells those two apart.

    asgard-cli whoami                    ask the platform the active profile names
    asgard-cli whoami --profile <name>   ask a particular profile's platform
    asgard-cli whoami --local            report what is stored, without a call

--local answers offline, from what was recorded at sign-in. It says whether the
access token has expired, which is not the same question as whether the session
is still good.

With ASGARD_TOKEN set, that token is the session: nothing is read from the store,
and --local has nothing recorded to report, so it says only which profile and
where the token came from.

In the Workbench assistant's sandbox the session is the member's, from the
session file the platform writes every turn: it reports that member and the
workspace of the conversation, and without --local confirms the platform still
accepts the token.

It exits non-zero when there is no session, when it has expired past renewing,
and when the platform rejects it.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := checkFormat(format); err != nil {
				return err
			}
			session, err := auth.Resolve(cmd.Context(), profile)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			if session.Source == auth.SourceSandbox {
				// No sign-in happened here, so the issuer's userinfo is the
				// wrong question - the session file already names the member.
				// What is worth asking is whether the platform still takes
				// the token.
				checked := false
				if !local {
					if _, err := platform.New(session, "").ListWorkspaces(cmd.Context()); err != nil {
						return fmt.Errorf("the platform did not accept the Workbench session: %w", err)
					}
					checked = true
				}
				return reportWhoami(out, format, session, auth.Userinfo{
					Sub:         session.Subject,
					Email:       session.Email,
					DisplayName: session.Name,
				}, checked)
			}
			if local {
				return reportWhoami(out, format, session, auth.Userinfo{
					Sub:         session.Subject,
					Email:       session.Email,
					DisplayName: session.Name,
				}, false)
			}

			info, err := auth.FetchUserinfo(cmd.Context(), session.Profile, session.Token)
			if err != nil {
				return fmt.Errorf("the %s platform did not accept the session: %w; run `asgard-cli login%s`",
					session.Profile.Name, err, profileArg(session.Profile.Name))
			}
			return reportWhoami(out, format, session, info, true)
		},
	}

	addProfileFlag(cmd, &profile)
	cmd.Flags().StringVar(&format, formatFlag, formatText, formatUsage)
	cmd.Flags().BoolVar(&local, "local", false,
		"report what is stored without calling the platform; says whether the token has expired, not whether it still works")

	return cmd
}

// profileArg renders the --profile argument to suggest, empty for the default
// so the advice reads as the command somebody would actually type.
func profileArg(name string) string {
	if name == auth.DefaultProfileName {
		return ""
	}
	return " --profile " + name
}

func reportWhoami(out io.Writer, format string, s *auth.Session, info auth.Userinfo, checked bool) error {
	who := info.Who()

	if format == formatJSON {
		return writeJSON(out, map[string]any{
			"profile":      s.Profile.Name,
			"issuer":       s.Profile.Issuer,
			"platform_api": s.Profile.PlatformAPI,
			"source":       string(s.Source),
			"workspace":    s.SandboxWorkspace,
			"subject":      info.Sub,
			"email":        info.Email,
			"name":         who,
			"confirmed":    checked,
		})
	}

	fmt.Fprintf(out, "%-9s %s\n", "profile", s.Profile.Name)
	fmt.Fprintf(out, "%-9s %s\n", "platform", s.Profile.PlatformAPI)
	fmt.Fprintf(out, "%-9s %s\n", "user", who)
	if info.Email != "" && info.Email != who {
		fmt.Fprintf(out, "%-9s %s\n", "email", info.Email)
	}
	if s.Source == auth.SourceEnv {
		fmt.Fprintf(out, "%-9s %s\n", "token", "from "+auth.EnvToken+", not the credential store")
	}
	if s.Source == auth.SourceSandbox {
		fmt.Fprintf(out, "%-9s %s\n", "workspace", s.SandboxWorkspace)
		fmt.Fprintf(out, "%-9s %s\n", "token", "from the Workbench session file "+auth.SessionFilePath())
		if !checked {
			fmt.Fprintf(out, "\nRead from the session file; the platform was not asked. Drop --local to\nconfirm it still accepts the session.\n")
		}
		return nil
	}
	if !checked {
		fmt.Fprintf(out, "\nRead from the credential store; the platform was not asked. Drop --local to\nconfirm the session is still accepted.\n")
	}
	return nil
}
