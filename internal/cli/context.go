package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/auth"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/binding"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/gitrepo"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

// workspaceFlag is the name of the flag that names a workspace.
const workspaceFlag = "workspace"

// WorkspaceSource says where a resolved workspace id came from, so that a
// command can print it. It is not decoration: a command that acted in the wrong
// workspace is the failure this resolution order exists to make visible, and
// "which workspace, and why that one" is one line.
type WorkspaceSource string

const (
	fromFlag    WorkspaceSource = "--workspace"
	fromEnv     WorkspaceSource = auth.EnvWorkspace
	fromSandbox WorkspaceSource = "the Workbench session"
	fromBinding WorkspaceSource = binding.FileName
)

// platformContext is what every command that talks to the platform needs: a
// session, the workspace to act in, and - when there is one - the repository
// the command was run inside.
type platformContext struct {
	Session   *auth.Session
	Client    *platform.Client
	Workspace string
	// WorkspaceSource is why that workspace, for the line a command prints.
	WorkspaceSource WorkspaceSource
	// RepoHost is the host the origin remote points at, lowercased and without
	// a port. Empty when there is no remote or it is not a shape we read.
	//
	// Kept apart from RepoFullName because a name and the provider it lives on
	// are different facts: "acme/app" on gitlab.com is not an account on
	// GitHub, and anything that treats it as one is guessing.
	RepoHost string
	// RepoFullName is the origin remote as "owner/name"; empty outside a
	// checkout, or when the remote is a shape this does not recognise.
	RepoFullName string
	// RepoRoot is the checkout's top level; empty outside one.
	RepoRoot string
	// Binding is `.asgard-cli.yaml` if this checkout has one, nil otherwise.
	Binding *binding.File
}

// contextOptions are the per-command overrides of the resolution.
type contextOptions struct {
	Profile   string
	Workspace string
	// NeedWorkspace is false for the handful of calls that take none - listing
	// workspaces being the one that has to work before any is chosen.
	NeedWorkspace bool
}

// resolveContext produces everything a platform command needs, or an error that
// says what to do about it.
//
// The workspace is looked for in this order, and the first answer wins:
//
//  1. --workspace
//  2. ASGARD_WORKSPACE
//  3. in the Workbench sandbox, the workspace the member is talking to the
//     assistant in (the session file)
//  4. the checkout's `.asgard-cli.yaml`
//
// The sandbox's workspace ranks above the checkout's binding: the member
// asked the assistant in one workspace, and a repository cloned into /work
// that was bound to another is not a reason to act there. When the two
// disagree the command says so.
//
// The order puts the two explicit forms above the committed file on purpose.
// A file that says a customer's workspace and a flag that says a test one
// disagree in only one safe direction: acting on the test workspace when the
// customer's was meant costs a confusing error, and the reverse deploys to a
// customer.
//
// **Three steps is the whole list, and it used to be five.** A per-machine
// default recorded by `workspace use --default` sat below the file, and below
// that, the only workspace the account could reach when there was one. Both are
// gone, for the same reason in two forms: an answer nobody typed and nobody can
// see. The machine default was invisible on the machine that had it and absent
// on every other, so the same command in the same checkout did different things
// for two people; the single candidate stopped being single the day a customer
// opened a second workspace. Every one of the three left is either on the
// command line or in a committed file.
func resolveContext(cmd *cobra.Command, opts contextOptions) (*platformContext, error) {
	ctx := cmd.Context()

	session, err := auth.Resolve(ctx, opts.Profile)
	if err != nil {
		return nil, err
	}

	pc := &platformContext{Session: session}
	pc.RepoRoot, pc.RepoFullName, pc.RepoHost = locateRepo(ctx)
	pc.Binding = loadBinding()

	if !opts.NeedWorkspace {
		pc.Client = platform.New(session, "")
		return pc, nil
	}

	ws, source, err := resolveWorkspace(ctx, session, pc, opts.Workspace)
	if err != nil {
		return nil, err
	}
	if source == fromSandbox && pc.Binding != nil && pc.Binding.Workspace != "" && pc.Binding.Workspace != ws {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"note: this checkout's %s names workspace %s; acting in %s, the workspace this Workbench conversation is in. Pass --workspace to act in the other one\n",
			binding.FileName, pc.Binding.Workspace, ws)
	}
	pc.Workspace, pc.WorkspaceSource = ws, source
	pc.Client = platform.New(session, ws)
	return pc, nil
}

// locateRepo reports the checkout the command was run in, or two empty strings.
// Not being in one is normal - half this tool answers questions in a meeting -
// so nothing here is an error.
func locateRepo(ctx context.Context) (root, fullName, host string) {
	dir, err := os.Getwd()
	if err != nil {
		return "", "", ""
	}
	root, err = gitrepo.Root(ctx, dir)
	if err != nil {
		return "", "", ""
	}
	host = gitrepo.OriginHost(ctx, root)
	fullName, err = gitrepo.OriginFullName(ctx, root)
	if err != nil {
		return root, "", host
	}
	return root, fullName, host
}

// loadBinding reads `.asgard-cli.yaml` for the working directory, or nil.
//
// A missing one is the normal state of a repository nobody has bound yet, and a
// malformed one must not stop a command that was given --workspace anyway, so
// neither is an error here. A malformed one is reported when it is the thing
// being relied on.
func loadBinding() *binding.File {
	dir, err := os.Getwd()
	if err != nil {
		return nil
	}
	f, err := binding.LoadFrom(dir)
	if err != nil {
		return nil
	}
	return f
}

func resolveWorkspace(
	ctx context.Context,
	session *auth.Session,
	pc *platformContext,
	flag string,
) (string, WorkspaceSource, error) {
	if flag != "" {
		return flag, fromFlag, nil
	}
	if env := os.Getenv(auth.EnvWorkspace); env != "" {
		return env, fromEnv, nil
	}
	if session.SandboxWorkspace != "" {
		return session.SandboxWorkspace, fromSandbox, nil
	}
	if pc.Binding != nil && pc.Binding.Workspace != "" {
		return pc.Binding.Workspace, fromBinding, nil
	}

	// Nothing recorded, so nothing is decided. The error lists what there is
	// rather than sending somebody off to look for it - and it lists one
	// candidate the same way it lists five.
	workspaces, err := platform.New(session, "").ListWorkspaces(ctx)
	if err != nil {
		return "", "", err
	}
	if len(workspaces) == 0 {
		return "", "", fmt.Errorf("the %s platform reports no workspaces for this account", session.Profile.Name)
	}
	return "", "", &needWorkspaceError{
		Profile:    session.Profile.Name,
		InRepo:     pc.RepoRoot != "",
		Workspaces: workspaces,
	}
}

// needWorkspaceError is the "which of these?" that a run with nothing recorded
// produces.
//
// **It is produced for one candidate as well as for five.** That is the whole
// of the rule: the answer is a choice somebody makes, and a list that happens
// to be short today is not a choice having been made.
type needWorkspaceError struct {
	Profile    string
	InRepo     bool
	Workspaces []platform.Workspace
}

func (e *needWorkspaceError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "no workspace chosen for the %s platform, and this account can reach %s:\n",
		e.Profile, plural(len(e.Workspaces), "workspace"))
	for _, w := range e.Workspaces {
		fmt.Fprintf(&b, "  %-22s %s\n", w.ID, w.Name)
	}
	if e.InRepo {
		fmt.Fprintf(&b, "\nRecord one in %s, which is committed so nobody has to choose again:\n\n", binding.FileName)
		fmt.Fprintf(&b, "    asgard-cli workspace use <id>%s\n", profileArgFor(e.Profile))
		fmt.Fprintf(&b, "\nNo workspace is chosen for you, even when the account can reach only one:\nsomebody has to decide which workspace a repository deploys into.\n")
	} else {
		// Nothing is recorded outside a checkout. A machine-wide default
		// existed and is gone: it was invisible where it was set and absent
		// everywhere else, which is how two people running the same command
		// got different answers.
		fmt.Fprintf(&b, "\nThis is not a checkout with a declaration, so there is nothing to write a\nbinding beside. Name one for this run, or for this shell:\n\n")
		fmt.Fprintf(&b, "    asgard-cli <command> --workspace <id>%s\n", profileArgFor(e.Profile))
		fmt.Fprintf(&b, "    export %s=<id>\n", auth.EnvWorkspace)
	}
	return b.String()
}

// plural writes "1 workspace" and "3 workspaces", so that a message which now
// fires for a single candidate does not read as though something is wrong.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// profileArgFor is profileArg for a bare profile name.
func profileArgFor(name string) string {
	if name == auth.DefaultProfileName {
		return ""
	}
	return " --profile " + name
}

// errNoDeclarationToBind is what `workspace use` reports where there is no
// declaration to write a binding beside.
var errNoDeclarationToBind = errors.New(
	"no .asgard-pipeline.yaml at or above this directory, so there is nothing for a binding to belong to.\n" +
		"`asgard-cli init` writes one. To act in a workspace without recording it, pass --workspace or " +
		"export " + auth.EnvWorkspace)

// actingOn writes the one line every command that changes something prints
// before it acts.
//
// **It is a receipt, not a reminder, and a receipt has no conditions.** The
// alternative considered was printing it only when the profile was chosen
// implicitly - inherited from the environment, or defaulted - on the reasoning
// that a name typed on the command line cannot be a surprise. That was refused
// for the same reason a skipped gate step is not a pass: once somebody types
// `--profile` out of habit the line disappears, and its absence carries no
// information at all. You cannot tell "this is safe" from "this build does not
// have that yet".
//
// **The URL is the identity; the name is a local label.** Two people's `dev`
// can point at different installations, and one of them can be production, so a
// line naming only the profile would be naming the one part that is nobody
// else's fact. The name is there because it is what you pass to change the
// answer, and the origin is there because it is where you go to look.
//
// stderr, so that `--format json` keeps a parseable stdout.
func actingOn(cmd *cobra.Command, session *auth.Session) {
	fmt.Fprintf(cmd.ErrOrStderr(), "acting on %s  (profile %s, from %s)\n",
		session.Profile.PlatformAPI, session.Profile.Name, session.ProfileFrom)
}

// actingLocally is actingOn for the commands that write this checkout's binding
// rather than changing the platform.
//
// It exists separately because "acting on <api url>" would be a lie - nothing
// is sent anywhere - while the platform is still the fact worth printing: an id
// recorded against the wrong one is accepted here and fails several commands
// later, somewhere that does not mention profiles.
//
// **It takes the flag rather than a session, because these commands have no
// session and should not acquire one.** `workspace use` says so in its own
// help: a check would put a network call in the middle of recording a choice.
// ResolveWithOrigin reads profiles.json and the environment and touches nothing
// else, so this stays as offline as the command it decorates.
func actingLocally(cmd *cobra.Command, profileFlag, what string) {
	r, err := auth.ResolveWithOrigin(profileFlag)
	if err != nil {
		// The command itself reports a profile that cannot be resolved. A
		// receipt is not the place to raise it a second time.
		return
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "recording %s for %s  (profile %s, from %s)\n",
		what, r.PlatformAPI, r.Name, r.NameFrom)
}
