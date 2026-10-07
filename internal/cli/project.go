package cli

import (
	"fmt"
	"io"
	"slices"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/pipelineconfig"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/repo"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/scaffold"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/stage"
)

func newProjectCmd() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "project",
		Short: "List the projects and what each chart declares",
		Long: `List this repository's projects and what each chart declares.

The list is the repository itself: the chart paths ` + "`" + pipelineconfig.FileName + "`" + ` names,
and the directories under ` + "`" + `projects/` + "`" + `. Nothing records it separately.

It says what each chart has, and nothing about what it lacks. A chart with a
SemanticLayer and no Agent may be finished or unfinished, and the files cannot
tell you which.

A project is the unit of deployment: one Helm chart, one namespace per
environment. An onboarding usually starts before the split is known, so projects
are added as the engagement discovers them.

A chart with fewer kinds than another is not necessarily less complete. Charts
are built in whatever order the engagement finds the answers, so this lists
only what is declared.

It also does not say what is deployed. Whether a finished chart is waiting for
its first tag or has been live for a month cannot be read from the files.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFormat(format); err != nil {
				return err
			}
			state, err := loadState()
			if err != nil {
				return err
			}
			if format == formatJSON {
				return writeJSON(cmd.OutOrStdout(), projectReport(state))
			}
			printProjects(cmd.OutOrStdout(), state)
			return nil
		},
	}

	cmd.Flags().StringVar(&format, formatFlag, formatText, formatUsage)
	cmd.AddCommand(newProjectAddCmd())

	return cmd
}

// projectJSON is one project's chart, as a record rather than as a column.
//
// projectJSON is one project's chart, as a record rather than as a column.
//
// It reports what the chart HAS. It used to also report what it was missing,
// measured against a recorded "shape" - and both the field and the judgement
// are gone: the record was an intent this tool cannot verify.
type projectJSON struct {
	Slug  string         `json:"slug"`
	Kinds map[string]int `json:"kinds"`
}

func projectReport(state stage.State) []projectJSON {
	out := []projectJSON{}
	for _, p := range state.Projects {
		out = append(out, projectJSON{Slug: p.Slug, Kinds: p.Kinds})
	}
	return out
}

// printProjects reports what each chart declares.
func printProjects(out io.Writer, state stage.State) {
	if len(state.Projects) == 0 {
		fmt.Fprintf(out, "No projects yet. A project is one chart, deployed by one or more releases:\n\n    asgard-cli project add <slug>\n")
		return
	}

	fmt.Fprintf(out, "Projects:\n\n")
	for _, p := range state.Projects {
		fmt.Fprintf(out, "  %-20s %s\n", p.Slug, p.Summary())
	}
	fmt.Fprintln(out)
}

func newProjectAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <slug>",
		Short: "Write a project's chart skeleton",
		Long: `Write a project's chart skeleton under ` + "`" + `projects/<slug>/chart/app` + "`" + `.

A project is one Helm chart. Which releases deploy it, and to which platform
project, is declared in ` + "`" + pipelineconfig.FileName + "`" + `. This command writes the chart
only; declaring its releases is a separate step.

One chart usually carries one release per environment. ` + "`" + `<slug>-dev` + "`" + ` and
` + "`" + `<slug>-prod` + "`" + ` name the same chart directory here and differ by their trigger
pattern; what separates them at deploy time is that each is bound to a different
platform project, and a platform project is what decides the namespace. A single
release is the POC shape.

The slug ends up in the names of the objects the chart renders, so keep it
short: Kubernetes caps a name at 63 characters and names derived from this
inherit its length.

Nothing records the project anywhere else. It exists because the directory
exists and the declaration names its chart.

Existing files are left alone, so this is safe to re-run.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			if err := repo.ValidateSlug("project", slug); err != nil {
				return err
			}

			root, err := loadRepo()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			projects, err := repo.Projects(root)
			if err != nil {
				return err
			}
			if !slices.Contains(projects, slug) {
				projects = append(projects, slug)
			}

			written, err := scaffold.Write(root, projects, false)
			if err != nil {
				return fmt.Errorf("write the chart skeleton for %q: %w", slug, err)
			}
			var created int
			for _, r := range written {
				if r.Status == scaffold.Created {
					created++
				}
			}
			fmt.Fprintf(out, "Wrote %d file(s) under projects/%s/chart.\n", created, slug)

			// Said in the plural, at the moment the decision is made.
			//
			// Every prompt around this used to be singular — "a release", "that
			// release", "the project" — which reads as a 1:1:1 chart-to-release-
			// to-platform-project mapping, and an agent onboarding a repository
			// takes the prompts literally. One release is right only for a
			// throwaway POC; the cost of finding that out later is that the
			// platform project, its namespace and the release name are already
			// the ones production uses.
			fmt.Fprintf(out, "\nDeclare its releases in %s, and create each one on the platform bound to\n"+
				"the platform project whose namespace it deploys into. A chart that no release\n"+
				"names deploys nowhere, and nothing here will say so.\n",
				pipelineconfig.FileName)
			fmt.Fprintf(out, "\nUsually one release per environment, not one release. %s-dev and\n"+
				"%s-prod share this chart directory and differ by on.pattern, each bound to a\n"+
				"DIFFERENT platform project, which is what gives them different namespaces.\n"+
				"One release is right for a POC nobody will maintain, and for nothing else.\n",
				slug, slug)
			// Naming the requirement without naming the command is how the
			// other half of this went wrong: an output that says what must be
			// true and not what does it leaves the reader to find the command,
			// and `project add` here and `pipeline project create` there are
			// close enough in name to look like the same thing already done.
			fmt.Fprintf(out, "\nThe platform project is not this one - `project add` wrote a chart in this\nrepository, and a platform project is a division of the workspace that owns a\nnamespace. Each release needs one, and they must not be the same one:\n\n")
			fmt.Fprintf(out, "    asgard-cli pipeline projects                     what the workspace already has\n")
			fmt.Fprintf(out, "    asgard-cli pipeline project create %s-dev\n", slug)
			fmt.Fprintf(out, "    asgard-cli pipeline project create %s-prod\n", slug)
			fmt.Fprintf(out, "    asgard-cli pipeline release create %s-dev --project <id>\n", slug)
			fmt.Fprintf(out, "    asgard-cli pipeline release create %s-prod --project <id>\n", slug)
			fmt.Fprintf(out, "\nAn existing platform project can serve one of them; what cannot happen is both\nreleases on the same one, because then both deploy into the same namespace and\nprod is whatever was tagged last.\n")
			return nil
		},
	}

	return cmd
}
