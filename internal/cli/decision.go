package cli

import (
	"fmt"
	"strings"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/repo"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/work"
)

func newDecisionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "decision",
		Short: "Write a dated decision record",
		Long: `Write a dated decision record.

A decision record answers "why is it this way". It is a snapshot, and it is
immutable: changing your mind means writing a new one, not editing the old one.
The current behaviour lives in the living spec instead, which is why the two are
separate directories.

Write it when the decision is made. A record written afterwards can restate what
was built but not why the rejected option was rejected, and that reason cannot be
reconstructed later.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newDecisionAddCmd())

	return cmd
}

func newDecisionAddCmd() *cobra.Command {
	var (
		slug   string
		module string
	)

	cmd := &cobra.Command{
		Use:   "add <topic>",
		Short: "Write a dated decision record and link it from the living spec",
		Long: `Write a dated decision record and link it from the living spec.

It copies the repository's own ` + "`" + work.DecisionTmpl + "`" + `, fills in the topic and
today's date, names the file ` + "`" + work.DecisionDir + `/YYYY-MM-DD-<slug>.md` + "`" + `, and adds a
row to the living spec's traceability table. The date is the day the decision was
made, which is why it is stamped rather than typed.

    asgard-cli decision add "the website reads through fixed query tools" \
      --module architecture.md

The template is read from the repository, not from this CLI, so an engagement
that changed it keeps its version.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			topic := args[0]

			root, err := loadRepo()
			if err != nil {
				return err
			}

			if slug == "" {
				slug = work.Slugify(topic)
			}
			if slug == "" && module != "" {
				// A topic written wholly in Chinese slugifies to nothing, and a
				// decision has no ID to fall back on. The module it changes is
				// ASCII by construction and says something true about the
				// record - better than a number, which would name nothing.
				// Most topics never reach here: a real one usually carries a CR
				// name or a product term, and "官網改用 fixed query tools"
				// already slugifies to fixed-query-tools.
				slug = work.Slugify(strings.TrimSuffix(module, ".md"))
				if slug != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "Named it after the module it changes, %s, because the topic is not in ASCII.\nPass --slug to choose a better name; the heading keeps the topic as written.\n\n", slug)
				}
			}

			// **The slug is a fact on disk.** An engagement may rename
			// `docs/spec/<slug>/`, and the constant is only the default for a
			// repository that has none yet - using it in a repository that
			// renamed the directory writes the record and links it from a path
			// that is not there, which reports as an error at the end of a
			// command that otherwise succeeded.
			specSlug := repo.SpecSlugIn(root)
			path, linkErr := work.AddDecision(root, topic, slug, specSlug, module, today())
			if path == "" {
				return linkErr
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Wrote %s\n", path)
			if linkErr != nil {
				fmt.Fprintf(out, "\n%v\n\nAdd the row by hand, or re-run `asgard-cli init`.\n", linkErr)
				return nil
			}
			fmt.Fprintf(out, "Linked from docs/spec/%s/README.md\n", specSlug)
			fmt.Fprintf(out, `
Fill in why, including why the option you rejected was rejected. Then apply the
delta to the living spec module the record names; otherwise the next reader
implements against a spec that is wrong.
`)
			return nil
		},
	}

	cmd.Flags().StringVar(&slug, "slug", "", "short name for the file (defaults to one derived from the topic; required when the topic has no ASCII)")
	cmd.Flags().StringVar(&module, "module", "", "living spec module this changes, e.g. architecture.md (defaults to empty)")

	return cmd
}
