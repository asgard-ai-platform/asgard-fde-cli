package cli

import (
	"errors"
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/tool"
)

func newDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check the external tools the acceptance gate needs",
		Long: `Check the external tools the acceptance gate needs, and say how to install any
that are missing.

asgard-cli does the repository work on its own. helm is needed to render a chart
and for the gate's lint step, and kubectl to dry-run against the cluster; python3
is only for the db-query skill, which reads a customer's source systems at design
time, so a missing one is reported without failing. An engagement that never
connects to a database never needs it.

Lint a chart only through the gate's lint step. A bare
` + "`helm lint <chart>`" + ` has no reserved asgard values file, so it fails on every
chart that labels anything - see ` + "`asgard-cli gate --help`" + `.

The install line is worked out for the machine it runs on - including which Linux
distribution, because neither helm nor kubectl is in the Debian or Ubuntu default
repositories, and an apt install of either fails on an unmet dependency.

Exits non-zero when a required tool is missing, so it works as a CI preflight.

It reports and does not install. asgard-cli is distributed as a tar.gz, a zip
and through "go install", none of which carry dependency metadata, so this
prints the command and you run it.

` + "`asgard-cli gate`" + ` checks helm as its ` + "`tools`" + ` step and stops there, because
that is the only tool a check needs. This lists every tool, optional ones
included, and says how to install what is missing on the machine you are on.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "platform  %s/%s\n\n", runtime.GOOS, runtime.GOARCH)

			var missing []tool.Tool
			for _, t := range tool.All {
				label := t.Name
				if t.Optional {
					label += " (optional)"
				}

				version, err := t.Version(cmd.Context())
				switch {
				case err == nil:
					fmt.Fprintf(out, "ok    %-20s %s\n", label, version)
					if note := versionNote(t, version); note != "" {
						fmt.Fprintf(out, "      %-20s %s\n", "", note)
					}

				case errors.As(err, new(*tool.ErrMissing)):
					fmt.Fprintf(out, "MISSING %-18s %s\n", label, t.Purpose)
					missing = append(missing, t)

				default:
					// Found but not runnable: a broken install, or a version
					// flag that changed. Report it rather than claiming it is
					// fine, and do not treat it as missing - the install hint
					// would be the wrong advice.
					fmt.Fprintf(out, "warn  %-20s found, but reporting its version failed: %v\n", label, err)
				}
			}

			if len(missing) == 0 {
				fmt.Fprintf(out, "\nEverything the gate needs is here.\n")
				return nil
			}

			fmt.Fprintln(out)
			var required []string
			for _, t := range missing {
				fmt.Fprintf(out, "%s\n\n", &tool.ErrMissing{Tool: t})
				if !t.Optional {
					required = append(required, t.Name)
				}
			}
			if len(required) > 0 {
				return fmt.Errorf("%d required tool(s) missing: %v", len(required), required)
			}
			return nil
		},
	}

	return cmd
}

// versionNote reports a version that is installed and working but not the one
// the generated repository was written against.
func versionNote(t tool.Tool, version string) string {
	if t.Name != tool.Helm.Name {
		return ""
	}
	// Homebrew and scoop both ship Helm 4 now, so a fresh install gets it while
	// the scaffolded gate scripts and skills were written against 3. `template`
	// and `lint` are what asgard-cli uses and both still exist, which is why
	// this is a note and not a failure - but a chart that renders differently
	// under 4 is worth knowing about before the difference shows up in CD,
	// where CI decides its own helm version.
	if tool.Major(version) >= 4 {
		return "note: the generated repo's gate and skills were written for Helm 3. " +
			"render and lint still work; check CI uses the same major."
	}
	return ""
}
