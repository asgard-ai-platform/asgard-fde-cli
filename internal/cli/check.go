package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/check"
)

func newCheckCmd() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "check [project ...]",
		Short: "Verify the repository's structural invariants",
		Long: `Verify the repository's structural invariants - the ones a chart render cannot
see, and that otherwise surface at deploy time or when the next person picks the
repo up:

  - the root README's project table matches the directories under projects/
  - .asgard-pipeline.yaml parses, names no release twice, and every release it
    declares points at a chart directory that has a Chart.yaml
  - runtime skills under assets/skills/ carry name and description frontmatter,
    with the name matching the directory
  - the SDD entry points under requirements/ are present
  - the docs/ spec layer is intact: required files, the living spec's module
    index matching the files on disk, dated filenames, and every relative link
    inside docs/ resolving
  - every "asgard-cli <command>" this repository's own documents name is a
    command this build has. "scaffold" never overwrites a file it has already
    written, because an FDE edits them, so a command renamed in the tool leaves
    every repository already scaffolded pointing at the old name, and no other
    check reports it

Naming projects limits the project-scoped checks to those; the repo-wide checks
always run. Exits non-zero when anything fails.

This is the ` + "`repo`" + ` step of ` + "`asgard-cli gate`" + `, and the one step that needs
no helm. Run ` + "`gate`" + ` after changing anything - it runs this and the rest in
order, and reports a step it could not run as skipped rather than passed. Run
this one alone while you are fixing what it found.

--format json emits the findings as records, with errors and warnings in
separate arrays. In text, a warning and an error differ only in the word at the
left margin, and only an error fails, so an agent should read the JSON.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFormat(format); err != nil {
				return err
			}
			root, err := loadRepo()
			if err != nil {
				return err
			}

			report, err := check.Run(root, args...)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if format == formatJSON {
				if err := writeJSON(out, checkReport(report)); err != nil {
					return err
				}
				if !report.OK() {
					// Silently, because the report is the output and a second
					// copy of it on stderr is not machine-readable either.
					return ErrSilent
				}
				return nil
			}
			for _, f := range report.Warnings() {
				fmt.Fprintf(out, "warn   %s\n", f.Message)
			}
			for _, f := range report.Errors() {
				fmt.Fprintf(out, "error  %s\n", f.Message)
			}

			scope := "whole repo"
			if len(report.Scope) > 0 {
				scope = fmt.Sprintf("%d project(s): %v", len(report.Scope), report.Scope)
			}

			if !report.OK() {
				return fmt.Errorf("%d problem(s) in %s", len(report.Errors()), scope)
			}
			fmt.Fprintf(out, "ok  structure is consistent (%s)\n", scope)
			return nil
		},
	}

	cmd.Flags().StringVar(&format, formatFlag, formatText, formatUsage)

	return cmd
}

// checkJSON is what `check --format json` emits.
//
// Errors and warnings are separate arrays rather than one list with a level
// field, because only one of them fails the gate and a caller that has to read
// a string to find out which will eventually read it wrong.
type checkJSON struct {
	OK       bool     `json:"ok"`
	Scope    []string `json:"scope"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

func checkReport(r check.Report) checkJSON {
	out := checkJSON{
		OK:       r.OK(),
		Scope:    r.Scope,
		Errors:   []string{},
		Warnings: []string{},
	}
	if out.Scope == nil {
		out.Scope = []string{}
	}
	for _, f := range r.Errors() {
		out.Errors = append(out.Errors, f.Message)
	}
	for _, f := range r.Warnings() {
		out.Warnings = append(out.Warnings, f.Message)
	}
	return out
}
