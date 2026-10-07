package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/auth"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/selfupdate"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/version"
)

func newVersionCmd() *cobra.Command {
	var asJSON, check bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version and build information",
		Long: `Print version and build information.

The version, commit and build date come from the ldflags GoReleaser sets when it
builds a release. A binary from a plain go build or go install reports "dev" and
falls back to the module and VCS metadata the Go toolchain embeds.

Use --json when a script needs to read the values.

    asgard-cli version --check     ask whether a newer release is published

Every command that can reach a network asks the same question, at most once
every ` + selfupdate.IntervalText() + `, and says one line when the answer is yes. The answer is
recorded beside the profiles rather than in any repository, the question is
asked beside the command rather than in front of it, and a run that does not get
an answer inside its own short leash drops it rather than waiting. A command
whose help says it touches no network never asks. --check asks now regardless,
and is the only way to hear that nothing is newer.

Failure is silent: no network, a rate limit or a proxy answering with HTML
does not change what this command does. A warning about a failed version check
would print on every run in an environment where the check never succeeds.

Set ` + selfupdate.EnvDisable + ` to stop the background one. It is off already wherever
stderr is not a terminal, so a CI log and a piped stderr get nothing.

This command does not replace the binary. Overwriting it in place can leave a
package manager's database out of step, fails on a running .exe, which is
locked, and usually needs root for /usr/local/bin. It prints the command
instead.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			info := version.Get()
			out := cmd.OutOrStdout()

			if asJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(info)
			}

			fmt.Fprintf(out, "asgard-cli %s\n", info.String())

			if check {
				home, err := auth.Home()
				if err != nil {
					return err
				}
				r := selfupdate.Check(cmd.Context(), home, info.Version, true)
				switch {
				case r.Latest == "":
					fmt.Fprintf(out, "\nCould not reach the release list, so whether a newer one exists is unknown.\n")
				case r.Newer:
					fmt.Fprintf(out, "\nasgard-cli %s is published. To take it:\n    %s\n", r.Latest, upgradeCommand())
				default:
					fmt.Fprintf(out, "\nThe newest release is %s.\n", r.Latest)
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&check, "check", false,
		"ask whether a newer release is published; the answer is otherwise cached for "+selfupdate.IntervalText())
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")

	return cmd
}
