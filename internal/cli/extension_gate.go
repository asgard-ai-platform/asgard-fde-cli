package cli

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/extension"
)

// `asgard-cli extension gate` is an Extension repository's build step, as
// `asgard-cli gate` is an FDE repository's.
//
// **It exists because the examples had no check at all.** `extension lint`
// looks at their shape and leaves whether they are right to a test FDE
// repository, and an author's agent that transcribed four Workflows into one
// shipped a failure branch off `update-context` and a status read from
// `httpResponse` on a failure branch - both stated in the platform material, both
// green in `gate`. The checks that look at one CR at a time need no chart, so
// they run here, on the examples as written.
//
// **A command an agent does not know about is never run**, so the way it gets
// run is the way `asgard-cli gate` does: it is one command whose name says what
// it is, the repository's AGENTS.md names it with when to run it, every command
// an author runs around it ends by naming it, and `asgard-cli gate` run here by
// habit says this is the one.

type extensionStep struct {
	Name     string              `json:"name"`
	Status   stepStatus          `json:"status"`
	Summary  string              `json:"summary"`
	Findings []extension.Finding `json:"findings"`
}

func newExtensionGateCmd() *cobra.Command {
	var formatOut string
	cmd := &cobra.Command{
		Use:   "gate [dir]",
		Short: "Everything this machine can check about an Extension, in one command",
		Long: `Everything this machine can check about an Extension, in one command.

    asgard-cli extension gate             the Extension in this directory
    asgard-cli extension gate ../asgard-ext-foo
    asgard-cli extension gate --format json

Run it after changing anything in an Extension repository, and before every
commit and tag. It is the repository's build step: there is no compiler for
teaching material and CRs, and this is the nearest thing to one. A red step
stops the work.

It needs no account, no network and no helm. What it runs, in order:

  lint      what ` + "`asgard-cli extension lint`" + ` checks: the index in README.md,
            SKILL.md's name, one document under routes/ per declared route,
            and the shape of examples/ - a README per set, each set named
            after a route and referred to from one, the EXAMPLE banner
  examples  every CR under examples/, held against the checks ` + "`asgard-cli gate`" + `
            runs on one CR at a time: processor types and their required
            configs, branches a processor never takes or a failure branch
            that reads httpResponse, the CRDs' enums, patterns, bounds and
            one-of rules. Template actions and <placeholders> are stood in for,
            and nothing is reported about a stand-in

What it cannot check is everything that needs the rest of a chart -
references to CRs the FDE repository already has, Secret names, the
declaration's keys - and anything only a run shows. Those are found by copying
the Extension into a test FDE repository with ` + "`asgard-cli extension get`" + `,
transcribing an example into its chart and running ` + "`asgard-cli gate`" + ` there,
then deploying it and asking the agent with ` + "`asgard-cli operate chat send`" + `.

Exit status is 1 when a step fails. Warnings do not fail it.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFormat(formatOut); err != nil {
				return err
			}
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}

			lintFindings, meta := extension.Lint(dir)
			exFindings, crs := extension.CheckExamples(dir)
			steps := []extensionStep{
				newExtensionStep("lint", "index, routes, setup and the shape of examples/", lintFindings),
				newExtensionStep("examples", fmt.Sprintf("%d CR(s) under examples/", crs), exFindings),
			}
			ok := true
			for _, s := range steps {
				if s.Status == stepFail {
					ok = false
				}
			}

			if formatOut == formatJSON {
				id := ""
				if meta != nil {
					id = meta.ID
				}
				if err := writeJSON(cmd.OutOrStdout(), map[string]any{"id": id, "ok": ok, "steps": steps}); err != nil {
					return err
				}
				if !ok {
					return ErrSilent
				}
				return nil
			}

			printExtensionSteps(cmd.OutOrStdout(), steps, dir)
			if !ok {
				return ErrSilent
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&formatOut, formatFlag, formatText, formatUsage)
	return cmd
}

func newExtensionStep(name, summary string, findings []extension.Finding) extensionStep {
	if findings == nil {
		findings = []extension.Finding{}
	}
	s := extensionStep{Name: name, Status: stepPass, Summary: summary, Findings: findings}
	errs, warns := count(findings)
	switch {
	case errs > 0:
		s.Status = stepFail
		s.Summary = fmt.Sprintf("%s: %d error(s), %d warning(s)", summary, errs, warns)
	case warns > 0:
		s.Status = stepWarn
		s.Summary = fmt.Sprintf("%s: %d warning(s)", summary, warns)
	}
	return s
}

func printExtensionSteps(out io.Writer, steps []extensionStep, dir string) {
	failed, warned := 0, 0
	for _, s := range steps {
		fmt.Fprintf(out, "%-9s %-5s %s\n", s.Name, s.Status, s.Summary)
		// The path on a line of its own: an example's path is most of a
		// terminal's width, and wrapped together with the message it pushed
		// the level onto a line by itself.
		for _, f := range sortedFindings(s.Findings) {
			fmt.Fprintf(out, "  %s %s\n", f.Level, f.Path)
			fmt.Fprintf(out, "    %s\n", wrapAt(f.Message, 74, 4))
		}
		if s.Status == stepFail {
			failed++
		}
		_, w := count(s.Findings)
		warned += w
	}

	fmt.Fprintln(out)
	if failed > 0 {
		fmt.Fprintf(out, "%d of %d step(s) failed. Fix them and run it again.\n", failed, len(steps))
		return
	}
	fmt.Fprintf(out, "All %d steps passed.\n", len(steps))
	if warned > 0 {
		// A warning in an FDE repository is one chart's business. One in an
		// example is copied into every chart whose agent reads it.
		fmt.Fprintf(out, "%d warning(s) did not fail it, but an example is copied by every FDE Agent that reads it:\n"+
			"fix each one, or say in that set's README why it stays.\n", warned)
	}
	// Said every time, for the same reason `asgard-cli gate` names the plan: a
	// green gate is when somebody is most likely to think the work is done, and
	// an example is only right once a real chart built from it runs.
	// The path is absolute because the command runs somewhere else.
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	fmt.Fprintf(out, "This is what can be checked without a chart. In a test FDE repository:\n"+
		"    asgard-cli extension get %s && asgard-cli gate\n", dir)
}
