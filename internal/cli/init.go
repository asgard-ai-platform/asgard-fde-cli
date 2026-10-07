package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/auth"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/gitrepo"
)

// `asgard-cli init` is the one command in this tool written for a person.
//
// **Everything else here is written for a coding agent**, or for an FDE who
// already has one. This one runs before there is one, and that is the whole
// reason it is different: interactive, and the only command that reads stdin.
//
// It used to require a workspace id and a pipeline id, on the assumption that
// an agent would have read the skills and know what those are and how to get
// them. **That assumption was circular.** The material that teaches an agent
// what a workspace is - AGENTS.md, CLAUDE.md, .agents/skills/ - is what this
// command writes. Before it runs, the directory is empty.
//
// So it does the least that gets an agent into service, and nothing else. It
// touches no network and needs no account: the skeleton is available before the
// platform is. Signing in, choosing a workspace, creating a pipeline and
// fetching the reference material all come after, guided by the agent this
// wrote the material for.
//
// See asgard-odin-pm docs/decisions/2026-09-07-asgard-cli-init-is-a-human-command.md.
func newInitCmd() *cobra.Command {
	var (
		force bool
		yes   bool
		noGit bool
	)

	cmd := &cobra.Command{
		Use:         "init",
		Annotations: touchesNoNetwork(),
		Short:       "Write the repository skeleton here, so a coding agent can take over",
		Long: `Write the Asgard repository skeleton into this directory.

    mkdir acme-asgard-kube && cd acme-asgard-kube
    asgard-cli init

This is the command a person runs, and the only one that asks questions.
Everything else in this tool is written for a coding agent working in a
repository that already exists - and until this has run, that repository does
not. There is no AGENTS.md, no CLAUDE.md and no .agents/skills/, so an agent
opened in an empty directory knows nothing about Asgard at all.

It touches no network and needs no account. The skeleton is a fact about
this tool, not about any platform, so it can be written on a plane, before a
workspace exists, or before anybody has signed in.

What it writes: the platform contract (AGENTS.md), the four-layer docs model,
the SDD rules, the design-time skills, an empty declaration and a chart skeleton
per project. What it deliberately does not write is the customer's own knowledge
- which systems exist, how the work splits, what the CRs look like. That is what
the onboarding produces.

It does not connect this checkout to the platform. Signing in, choosing a
workspace, creating a pipeline and fetching the reference material that
describes the server all come after, guided by the coding agent this command
just set up. When it is done, open the directory in your agent and say so; the closing message has the
words.

Run it again whenever this CLI has moved on or a project was added: existing
files are left alone and reported as skipped.

For the files this CLI ships - AGENTS.md and the design-time skills - it keeps a record of which version of itself wrote
each, in ` + "`.asgard-scaffold.json`" + `, so it can say which of four things a
difference is rather than guessing. ` + "`behind`" + `: this CLI has moved on and nobody
here touched the file. ` + "`edited`" + `: somebody here did, and ` + "`--force`" + ` would discard
it. ` + "`ahead`" + `: a NEWER CLI wrote this repository, and ` + "`--force`" + ` will not hand it
this binary's older copy. ` + "`retired`" + `: an older CLI shipped the file and this one
does not, which nothing else in a repository records.

` + "`--force`" + ` takes the newer shipped material, discarding local edits to the
skeleton; files this tool writes into - the deployment declaration, the
indexes, the open-questions table, the living spec - are preserved either way
and reported. The declaration decides which releases exist and what each takes,
which a scaffold cannot know, so ` + "`--force`" + ` does not replace a hand-written one
with the shape it generates from the directories under ` + "`projects/`" + `.

In a file with a managed region, ` + "`--force`" + ` replaces the region and nothing
else: the scaffolded AGENTS.md tells its reader that the half above the marker
is theirs and is never overwritten.

With ` + "`--yes`" + `, or when stdin is not a terminal, it asks nothing. That is the
form for a re-run from an agent or from CI.`,
		Args:    cobra.NoArgs,
		GroupID: groupBuild,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()

			root, err := scaffoldRoot()
			if err != nil {
				return err
			}
			if err := refuseObviouslyWrongRoot(root); err != nil {
				return err
			}

			// A terminal is the test for whether there is somebody to ask. An
			// agent's tool call and a CI step both fail it, and both have
			// already decided - which is what --yes says explicitly.
			interactive := !yes && term.IsTerminal(int(os.Stdin.Fd()))
			in := bufio.NewReader(os.Stdin)

			if interactive {
				fmt.Fprintf(out, "This writes the Asgard repository skeleton into\n\n    %s\n\n", root)
				fmt.Fprintf(out, "and the repository will be called %s, after that directory.\n\n", filepath.Base(root))
				ok, err := confirm(in, out, "Write it here?", true)
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintf(out, "\nNothing written. Change directory to where the repository should be\n"+
						"and run `asgard-cli init` there.\n")
					return nil
				}
				fmt.Fprintln(out)
			}

			if err := ensureGit(cmd, root, interactive, noGit, in, out); err != nil {
				return err
			}

			if err := runScaffold(cmd, root, force); err != nil {
				return err
			}

			noteNoOrigin(cmd, root, out)
			fmt.Fprint(out, handoff)
			return nil
		},
	}

	f := cmd.Flags()
	f.BoolVarP(&force, "force", "f", false,
		"overwrite skeleton files that already exist, taking material this CLI has changed since; "+
			"it will not replace a shipped file a newer CLI wrote, which would be a downgrade")
	f.BoolVarP(&yes, "yes", "y", false, "ask nothing; the form for a re-run from an agent or CI")
	f.BoolVar(&noGit, "no-git", false, "do not offer to run git init, even in an empty directory")
	return cmd
}

// handoff is the last thing a person sees, and it is the whole handover.
//
// **The sentence is printed rather than remembered.** Somebody is looking at
// this terminal at the moment they need it, and a sentence that ships with the
// binary cannot go stale against the binary. It is a safety net rather than the
// mechanism: CLAUDE.md is `@AGENTS.md`, so an agent opened here has read what
// state this checkout is in before the first word is typed at it.
const handoff = `
Now open this directory in your coding agent and say:

    Connect this repo to the Asgard platform
    幫我把這個 repo 接上 Asgard 平台

It will sign you in, find the workspace, set up the pipeline and fetch the
reference material for the server you deploy to - asking you for what only you
can answer. None of that has happened yet, and ` + "`asgard-cli gate`" + ` says so at
any point.

`

// refuseObviouslyWrongRoot stops the one mistake that is expensive to undo.
//
// Forty files written into a home directory, or into the root of an unrelated
// repository, is not a mistake anybody makes on purpose - it is what happens
// when a command is run one directory up from where it was meant. A prompt
// catches it for a person; this catches it for --yes and for a non-terminal,
// which is where nobody is watching.
func refuseObviouslyWrongRoot(root string) error {
	home, err := os.UserHomeDir()
	if err == nil && filepath.Clean(root) == filepath.Clean(home) {
		return fmt.Errorf(
			"refusing to write the skeleton into your home directory (%s)\n\n"+
				"    mkdir <customer>-asgard-kube && cd <customer>-asgard-kube\n"+
				"    asgard-cli init", home)
	}
	if filepath.Dir(root) == root {
		return fmt.Errorf("refusing to write the skeleton into the filesystem root (%s)", root)
	}
	// In the Workbench assistant's sandbox, the working directory holds
	// every repository the member works on, side by side. A skeleton there
	// would make it one repository that swallows all the others - a
	// declaration is found by walking up, so every repository below would
	// read it as its own.
	if auth.SandboxMode() && filepath.ToSlash(filepath.Clean(root)) == sandboxWorkDir {
		return fmt.Errorf(
			"refusing to write the skeleton into %s itself: in the Workbench sandbox it holds every repository side by side\n\n"+
				"    mkdir %s/<customer>-asgard-kube && cd %s/<customer>-asgard-kube\n"+
				"    asgard-cli init", sandboxWorkDir, sandboxWorkDir, sandboxWorkDir)
	}
	return nil
}

// sandboxWorkDir is the Workbench sandbox's working directory.
const sandboxWorkDir = "/work"

// ensureGit offers to make this a git repository.
//
// It matters more than it looks. `.env` holds the customer's design-time
// credentials and is kept out of git by a .gitignore line - `asgard-cli check`
// fails when that line is missing, and `asgard-cli local-env` adds it. All of
// that assumes a git repository to be ignored by.
//
// Not offered without a terminal: running `git init` in somebody's directory
// unattended is a change nobody asked for.
func ensureGit(cmd *cobra.Command, root string, interactive, noGit bool, in *bufio.Reader, out interface{ Write([]byte) (int, error) }) error {
	if noGit {
		return nil
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
		return nil
	}
	if !interactive {
		fmt.Fprintf(out, "note: not a git repository. `git init` here before committing; `.env` is\n"+
			"      kept out of git by a .gitignore line, and that needs a repository.\n\n")
		return nil
	}

	fmt.Fprintf(out, "This is not a git repository yet. The skeleton expects one: the customer's\n"+
		"design-time credentials live in a .env that a .gitignore line keeps out of git.\n\n")
	ok, err := confirm(in, out, "Run `git init` here?", true)
	if err != nil {
		return err
	}
	fmt.Fprintln(out)
	if !ok {
		return nil
	}

	git := exec.CommandContext(cmd.Context(), "git", "init")
	git.Dir = root
	git.Stdout, git.Stderr = out, cmd.ErrOrStderr()
	if err := git.Run(); err != nil {
		// Not fatal: the skeleton is still worth writing, and `git init` is one
		// command the person can run themselves.
		fmt.Fprintf(out, "git init failed (%v); run it yourself before committing.\n\n", err)
	}
	return nil
}

// noteNoOrigin closes the loop between this command and the connect that
// follows it.
//
// **This command ends one step before the question the next one opens with.**
// The connect derives the provider account and the repository from `origin`,
// so a checkout that has none turns a derivation into a question - and it is
// asked of whoever is standing here now, by an agent, minutes later. Saying it
// while they are still at the terminal is the difference between an answer
// they have ready and one they meet cold.
//
// Only inside a checkout. Outside one, `ensureGit` has already said the larger
// thing, and a second note about a remote on top of "this is not a git
// repository" is noise.
func noteNoOrigin(cmd *cobra.Command, root string, out interface{ Write([]byte) (int, error) }) {
	ctx := cmd.Context()
	if _, err := gitrepo.Root(ctx, root); err != nil {
		return
	}
	if _, err := gitrepo.OriginURL(ctx, root); err == nil {
		return
	}
	fmt.Fprintf(out, "\nnote: there is no `origin` remote here yet. Connecting derives the provider\n"+
		"      account and the repository from it, so the first thing you will be asked\n"+
		"      for is this repository's remote URL.\n")
}

// confirm asks a yes/no question, with def taken on a bare Enter.
func confirm(in *bufio.Reader, out interface{ Write([]byte) (int, error) }, question string, def bool) (bool, error) {
	suffix := "[Y/n]"
	if !def {
		suffix = "[y/N]"
	}
	for {
		fmt.Fprintf(out, "%s %s ", question, suffix)
		line, err := in.ReadString('\n')
		answer := strings.ToLower(strings.TrimSpace(line))
		switch answer {
		case "":
			// EOF with nothing typed is not an answer; treat it the way a
			// non-terminal is treated rather than silently taking the default.
			if err != nil {
				fmt.Fprintln(out)
				return def, nil
			}
			return def, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		if err != nil {
			return def, nil
		}
		fmt.Fprintf(out, "  please answer y or n.\n")
	}
}
