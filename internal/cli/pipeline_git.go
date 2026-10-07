package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/auth"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

// git in the Workbench assistant's sandbox. Nobody there has a GitHub login,
// so git's credential for github.com is a token the workspace's GitHub
// Connection signs - the GitHub App installation - for exactly the one
// repository being fetched or pushed, fresh each time and never written to
// disk. The token never reaches the conversation: git asks this CLI for it
// and hands it straight to GitHub.

// errNotInSandbox is what the sandbox-only commands say anywhere else.
var errNotInSandbox = errors.New("this command is for the Workbench assistant's sandbox (" + auth.EnvSandboxMode +
	"=true), where nobody has a GitHub login. On your own machine git uses your own GitHub credentials; " +
	"use those, not the pipeline's GitHub App")

// gitCredentialUser is the username GitHub expects with an installation
// token.
const gitCredentialUser = "x-access-token"

// noreplyDomain is the email domain of a commit author with no address of
// their own.
const noreplyDomain = "users.noreply.asgard-ai.com"

func newPipelineGitAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "git-auth",
		Short: "Make git use the workspace's GitHub Connection (Workbench sandbox only)",
		Long: `Configure git in the Workbench assistant's sandbox, once per sandbox: github.com
credentials come from this CLI, and commits are authored as the member.

It writes to the user's global git config:

    credential.https://github.com.helper       reset, then this CLI's
                                                "pipeline git-credential"
    credential.https://github.com.useHttpPath  true, so the helper knows the repository
    user.name, user.email                       the member's, from the Workbench session

The helper list is reset first so this CLI is the only source of a github.com
credential: a helper configured elsewhere would answer with another identity.
It warns when a url.<base>.insteadOf rule or ~/.netrc would send github.com
traffic around the helper, and leaves both unchanged, because this command did
not write them.

Fetching needs a workspace member; pushing needs workspace administration:
the push credential writes to the repository as the GitHub App, and a push to a
Deployment's branch or tag starts a Run. A member's push is refused by GitHub.

Only in the Workbench sandbox (` + auth.EnvSandboxMode + `=true); anywhere else it refuses.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !auth.SandboxMode() {
				return errNotInSandbox
			}
			s, err := auth.LoadSandboxSession()
			if err != nil {
				return err
			}
			self, err := os.Executable()
			if err != nil {
				return fmt.Errorf("find this binary: %w", err)
			}
			name, email := gitIdentity(s)
			helper := "!" + shellQuote(self) + " pipeline git-credential"
			ctx := cmd.Context()
			steps := [][]string{
				{"config", "--global", "--replace-all", "credential.https://github.com.helper", ""},
				{"config", "--global", "--add", "credential.https://github.com.helper", helper},
				{"config", "--global", "credential.https://github.com.useHttpPath", "true"},
				{"config", "--global", "user.name", name},
				{"config", "--global", "user.email", email},
			}
			for _, args := range steps {
				if out, err := exec.CommandContext(ctx, "git", args...).CombinedOutput(); err != nil {
					return fmt.Errorf("git %s: %v: %s", strings.Join(args[:len(args)-1], " "), err, strings.TrimSpace(string(out)))
				}
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "git now asks this CLI for github.com credentials, and commits as %s <%s>.\n", name, email)
			for _, w := range gitBypasses(ctx) {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
			}
			fmt.Fprintf(out, "Fetching works for any workspace member; pushing needs workspace administration.\n")
			return nil
		},
	}
	return cmd
}

// gitIdentity is the commit author: the member, from the session.
func gitIdentity(s *auth.SandboxSession) (name, email string) {
	if s.User != nil {
		name, email = s.User.DisplayName, s.User.Email
		if email == "" && s.User.ID != "" {
			email = s.User.ID + "@" + noreplyDomain
		}
	}
	if name == "" {
		name = "Asgard Workbench"
	}
	if email == "" {
		email = "workbench@" + noreplyDomain
	}
	return name, email
}

// gitBypasses names configuration that would send github.com traffic around
// the credential helper.
func gitBypasses(ctx context.Context) []string {
	var out []string
	raw, _ := exec.CommandContext(ctx, "git", "config", "--global", "--get-regexp", `^url\..*\.insteadof$`).Output()
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.Contains(strings.ToLower(line), "github.com") {
			out = append(out, "git rewrites github.com URLs ("+line+"); a rewritten URL does not use this CLI's credential")
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		if b, err := os.ReadFile(filepath.Join(home, ".netrc")); err == nil && strings.Contains(string(b), "github.com") {
			out = append(out, "~/.netrc has a github.com entry; git may use it instead of this CLI's credential")
		}
	}
	return out
}

// shellQuote quotes s for the shell git runs a "!" helper through.
func shellQuote(s string) string {
	if !strings.ContainsAny(s, " '\"\\$`!*?[]{}()<>|&;#~") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func newPipelineGitCredentialCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "git-credential <get|store|erase>",
		Short:  "git's credential helper for the Workbench sandbox",
		Hidden: true,
		Long: `git's credential helper for github.com in the Workbench assistant's sandbox,
set up by "asgard-cli pipeline git-auth". Not for running by hand: git calls it,
with the credential protocol on stdin.

For "get" on an https github.com URL it asks the platform for a token that
reaches exactly that repository - a push token when the member may push, a read
token otherwise - and answers username x-access-token with the token as the
password. Nothing is cached or written anywhere. "store" and "erase" do nothing.
Anything that is not github.com over https gets no answer, so git goes on to
whatever else it has.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "get" {
				// store / erase: the token is minted fresh every time, so
				// there is nothing to keep and nothing to forget.
				_, _ = io.Copy(io.Discard, cmd.InOrStdin())
				return nil
			}
			if !auth.SandboxMode() {
				return errNotInSandbox
			}
			in, err := readCredentialRequest(cmd.InOrStdin())
			if err != nil {
				return err
			}
			if in["protocol"] != "https" || !strings.EqualFold(in["host"], "github.com") {
				return nil
			}
			repo := repositoryFromPath(in["path"])
			if repo == "" {
				// Without useHttpPath git does not say which repository, and
				// a token for "every repository" is exactly what this does not
				// hand out.
				return errors.New("git did not say which repository; run `asgard-cli pipeline git-auth`, which sets credential.useHttpPath")
			}
			pc, err := resolveContext(cmd, contextOptions{NeedWorkspace: true})
			if err != nil {
				return err
			}
			tok, err := mintForGit(cmd, pc, repo)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "username=%s\npassword=%s\n", gitCredentialUser, tok.Token)
			return nil
		},
	}
	return cmd
}

// mintForGit asks for a push token and settles for a read token.
//
// git's credential protocol does not say whether the credential is for a
// fetch or a push, so the helper cannot ask for what the operation needs. It
// asks for the most the member may have: a push token needs workspace
// administration, and a member refused one (a 403 from the platform's own
// permissions, which carries no error code) gets a read token instead - enough
// to fetch, and GitHub refuses the push. A GitHub App that was never granted
// contents:write is the same fallback, said out loud.
func mintForGit(cmd *cobra.Command, pc *platformContext, repo string) (*platform.RepositoryToken, error) {
	ctx := cmd.Context()
	tok, err := pc.Client.MintRepositoryToken(ctx, repo, true)
	if err == nil {
		return tok, nil
	}
	var apiErr *platform.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusForbidden {
		return nil, err
	}
	if apiErr.ErrorCode == platform.IacAppPermissionNotGranted {
		fmt.Fprintf(cmd.ErrOrStderr(), "asgard-cli: the GitHub App cannot push to %s (it was not granted contents:write); "+
			"fetching still works. An owner of the GitHub organization has to accept the App's permissions.\n", repo)
	} else {
		fmt.Fprintf(cmd.ErrOrStderr(), "asgard-cli: pushing to %s needs workspace administration; "+
			"this credential can fetch only, so a push is refused by GitHub.\n", repo)
	}
	return pc.Client.MintRepositoryToken(ctx, repo, false)
}

// readCredentialRequest reads git's key=value lines up to a blank line or
// the end of input.
func readCredentialRequest(r io.Reader) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			break
		}
		k, v, ok := strings.Cut(line, "=")
		if ok {
			out[k] = v
		}
	}
	return out, sc.Err()
}

// repositoryFromPath turns git's path ("owner/name.git") into "owner/name",
// or "" when it is not that shape.
func repositoryFromPath(p string) string {
	p = strings.TrimSuffix(strings.Trim(p, "/"), ".git")
	owner, name, ok := strings.Cut(p, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return ""
	}
	return owner + "/" + name
}

// ---- repo create

func newPipelineRepoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo",
		Short: "Repositories under a GitHub Connection",
		Long: `Repositories under one of the workspace's GitHub Connections.

    asgard-cli pipeline repo create <name> --org <organization>`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newPipelineRepoCreateCmd())
	return cmd
}

func newPipelineRepoCreateCmd() *cobra.Command {
	var (
		f           pipelineFlags
		org         string
		connection  string
		description string
		public      bool
	)
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an empty repository under a connection's GitHub organization",
		Long: `Create an empty repository - no initial commit - under the GitHub organization
of one of the workspace's connections, as the GitHub App. Private unless
--public. It prints the clone URL.

    asgard-cli pipeline repo create acme-assistant --org acme
    asgard-cli pipeline repo create acme-assistant --connection <id> --description "..."

--org picks the connection by its GitHub account; --connection names it by id.
One of the two is required ("asgard-cli pipeline connections" lists them).

It needs workspace administration, and it works only for an organization: a
GitHub App cannot create a repository under a person's account, so there the
person creates it on GitHub and connects it. A name already taken is an error.
It is never retried: a create whose answer was lost has still happened,
so look before trying again.

In the Workbench sandbox, "asgard-cli pipeline git-auth" then lets git push to it.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if (org == "") == (connection == "") {
				return errors.New("name the organization with --org, or the connection with --connection - one of the two")
			}
			pc, err := f.context(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			id := connection
			if org != "" {
				if id, err = connectionForAccount(ctx, pc, org); err != nil {
					return err
				}
			}
			actingOn(cmd, pc.Session)
			created, err := pc.Client.CreateRepository(ctx, id, args[0], description, public)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if f.format == formatJSON {
				return writeJSON(out, created)
			}
			visibility := "private"
			if !created.Repository.Private {
				visibility = "public"
			}
			fmt.Fprintf(out, "Created %s (%s, empty).\nClone it with: git clone %s\n",
				created.Repository.FullName, visibility, created.CloneURL)
			return nil
		},
	}
	f.register(cmd, false)
	cmd.Flags().StringVar(&org, "org", "", "the GitHub organization, which picks the connection whose account it is")
	cmd.Flags().StringVar(&connection, "connection", "", "the connection id, instead of --org")
	cmd.Flags().StringVar(&description, "description", "", "the repository's description")
	cmd.Flags().BoolVar(&public, "public", false, "make it public; it is private otherwise")
	return cmd
}

// connectionForAccount finds the workspace's connection on a GitHub account.
func connectionForAccount(ctx context.Context, pc *platformContext, account string) (string, error) {
	conns, err := pc.Client.ListConnections(ctx)
	if err != nil {
		return "", err
	}
	for _, c := range conns {
		if strings.EqualFold(c.AccountLogin, account) {
			return c.ConnectionId, nil
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "workspace %s has no connection on GitHub account %q", pc.Workspace, account)
	if len(conns) > 0 {
		b.WriteString("; it has:\n")
		for _, c := range conns {
			fmt.Fprintf(&b, "  %-22s %-24s %s\n", c.ConnectionId, c.AccountLogin, c.Status)
		}
	} else {
		b.WriteString("; `asgard-cli pipeline connect` creates one")
	}
	return "", errors.New(b.String())
}
