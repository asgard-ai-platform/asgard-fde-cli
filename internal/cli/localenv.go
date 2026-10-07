package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/auth"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/localenv"
)

func newLocalEnvCmd() *cobra.Command {
	var (
		focus     []string
		noBrowser bool
		terminal  bool
		timeout   time.Duration
		wait      bool
		waitFor   time.Duration
		serveBG   bool
	)

	cmd := &cobra.Command{
		Use:   "local-env",
		Short: "Fill in this repository's design-time credentials, without typing them at an agent",
		Long: `Open a form for the ` + "`.env`" + ` at the repository root, so that whoever holds a
credential can type it in themselves.

A coding agent must never ask anybody to give it a password, in the
conversation or as "paste it and I will remove it after". A credential that has
been through a transcript has to be treated as disclosed. Asking a person who
may not be an engineer to open a dotfile, find the right line and mind the
whitespace usually fails as well.

The agent writes the keys it needs, with the values left empty, and this
opens a form to fill them in:

    asgard-cli local-env
    asgard-cli local-env --focus UOF_DB_HOST,UOF_DB_PASSWORD

It serves one page on 127.0.0.1 on a random port, opens a browser at it, and
closes as soon as the form is saved. The URL carries a one-time token, the
server answers to no other host name, and the page is served under a policy
that lets it talk to nothing but the process that served it.

What this command reports is a list of key names. It never prints a value,
on save, in an error or in the summary: the values are written to the file and
nowhere else.

` + "`--focus`" + ` highlights the keys you are waiting for. It does not hide the
others, because the person filling this in may know about a second database
nobody has mentioned yet. They can add keys too, so the agent should re-read
` + "`.env`" + ` afterwards rather than assume it got back exactly what it asked for.

There are three kinds of credential, and this command handles only the first:

    design time         this .env, on this machine       the customer, or you
    pipeline variables  the platform, per release        asgard-cli pipeline variables set
    runtime secret      a Kubernetes Secret in a cluster the platform provisions it

The value is often the same string, because it is the same database, but each
is set differently. Do not read one to obtain another, such as a cluster Secret
to get a design-time password: that puts a production credential somewhere
nobody can withdraw it from.

With no browser, such as in a container or on a locked-down server, the URL is printed for
you to open, over an SSH port forward if that is what it takes. Where even that
is not possible, ` + "`--terminal`" + ` asks for each value at the prompt instead, and
does not echo the secrets.

In the Workbench assistant's sandbox it takes two steps, because the agent reads
a command's output only when the command ends, and one command runs at most ten
minutes:

    asgard-cli local-env --focus UOF_DB_PASSWORD   serves the form in the background,
                                                   opens it in the sandbox's browser,
                                                   and ends
    (the agent calls open_sandbox_browser, so the member takes over that browser)
    asgard-cli local-env --wait                    waits for the save; after a few
                                                   minutes says it is still waiting,
                                                   and is run again

The member fills the form in the sandbox's browser, because the page listens on
the sandbox's 127.0.0.1 and nothing else can reach it. --terminal has no terminal
to ask at there.`,
		Args:    cobra.NoArgs,
		GroupID: groupBuild,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := envRoot()
			if err != nil {
				return err
			}
			if wait {
				if !auth.SandboxMode() {
					return errors.New("--wait is the second step of the Workbench sandbox's form; on a desktop local-env itself waits")
				}
				return waitLocalEnvInSandbox(cmd, root, waitFor)
			}
			if terminal && auth.SandboxMode() {
				return errors.New("--terminal needs a terminal, and the Workbench sandbox has none; run local-env without it and the form opens in the sandbox's browser")
			}
			file, err := localenv.Load(root)
			if err != nil {
				return err
			}

			errOut := cmd.ErrOrStderr()
			if err := ensureIgnored(root); err != nil {
				fmt.Fprintf(errOut, "warning: %v\n", err)
			}

			focus = splitFocus(focus)
			if missing := localenv.MissingFocus(file, focus); len(missing) > 0 {
				// A typo in --focus would otherwise highlight nothing and look
				// like the form simply had nothing to point at.
				fmt.Fprintf(errOut, "warning: --focus names %s that %s not in %s yet, so nothing is highlighted for %s\n",
					plural(len(missing), "key"), isAre(len(missing)), localenv.FileName,
					strings.Join(missing, ", "))
			}

			opts := localenv.Options{
				File: file, Focus: focus, Timeout: timeout, NoBrowser: noBrowser,
			}
			if serveBG {
				return serveLocalEnvInBackground(cmd, root, opts)
			}
			if auth.SandboxMode() {
				return startLocalEnvInSandbox(cmd, root, focus, timeout)
			}

			var res localenv.Result
			if terminal {
				res, err = localenv.Terminal(opts, os.Stdin, errOut)
			} else {
				res, err = localenv.Serve(cmd.Context(), opts, errOut)
			}
			if err != nil {
				return err
			}

			// stdout, and key names only: this is what an agent reads back.
			res.Report(cmd.OutOrStdout())
			return nil
		},
	}

	f := cmd.Flags()
	f.StringSliceVar(&focus, "focus", nil,
		"highlight these keys in the form; it never hides the rest")
	f.BoolVar(&noBrowser, "no-browser", false,
		"do not open a browser; the URL is printed either way")
	f.BoolVar(&terminal, "terminal", false,
		"ask for each value at the prompt, for a machine that cannot reach a browser at all")
	f.DurationVar(&timeout, "timeout", 15*time.Minute,
		"give up if nothing is saved by then, so a tab holding credentials does not outlive the task")
	f.BoolVar(&wait, "wait", false,
		"in the Workbench sandbox: wait for the form opened by the previous local-env to be saved")
	f.DurationVar(&waitFor, "wait-for", localEnvWaitDefault,
		"with --wait: how long this call waits before saying it is still waiting; keep it under the agent's command limit")
	f.BoolVar(&serveBG, localEnvServeFlag, false, "internal: the Workbench sandbox's background form server")
	_ = f.MarkHidden(localEnvServeFlag)

	return cmd
}

// envRoot is the repository the .env belongs to.
//
// The declaration is what marks a repository, the same rule `render` uses. A
// .env somewhere above or below it would be a second answer to where the
// credentials live.
func envRoot() (string, error) {
	root, err := repoRoot()
	if err == nil {
		return root, nil
	}
	dir, wdErr := os.Getwd()
	if wdErr != nil {
		return "", err
	}
	return "", fmt.Errorf("%w\n  local-env edits the %s beside it, and %s is not in one",
		err, localenv.FileName, dir)
}

// ensureIgnored keeps .env out of git.
//
// It is a warning rather than an error, and it appends rather than asking:
// somebody is about to type a customer's password into this file, and a
// .gitignore that does not mention it is one `git add -A` away from putting
// that password in a repository's history, where removing it is not a delete.
func ensureIgnored(root string) error {
	path := filepath.Join(root, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		switch strings.TrimSpace(line) {
		case ".env", "/.env", "*.env", ".env*":
			return nil
		}
	}
	body := string(data)
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	body += "\n# Design-time credentials. Added by `asgard-cli local-env`.\n.env\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return fmt.Errorf("%s did not cover %s, and adding it failed: %w",
			path, localenv.FileName, err)
	}
	return fmt.Errorf("%s did not cover %s; added it", path, localenv.FileName)
}

// splitFocus accepts both --focus A,B and --focus A --focus B, and drops the
// empty strings a trailing comma leaves behind.
func splitFocus(in []string) []string {
	var out []string
	for _, item := range in {
		for _, k := range strings.Split(item, ",") {
			if k = strings.TrimSpace(k); k != "" {
				out = append(out, k)
			}
		}
	}
	return out
}

func isAre(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}
