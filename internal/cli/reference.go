package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/work"
)

func newReferenceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reference",
		Short: "File the customer's own material, with where it came from",
		Long: `File the customer's own material, with where it came from.

` + "`references/`" + ` is background for humans and spec-writing agents. It is not what
the running agent reads - domain knowledge the agent needs at run time belongs in
a skill, because a skill is synced into the platform and this directory is not.

Every engagement files documents. This command gives them one provenance
table and one directory instead of a different one per engagement.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newReferenceAddCmd())
	return cmd
}

func newReferenceAddCmd() *cobra.Command {
	var ref work.Reference

	cmd := &cobra.Command{
		Use:   "add <file>",
		Short: "Copy a document into references/ and record its provenance",
		Long: `Copy a document into ` + "`references/`" + ` and record its provenance.

    asgard-cli reference add ~/Downloads/rma.xlsx \
      --what "the RMA status codes, as their support team uses them" \
      --from "their support team" --dated 2024-11 --as erp/rma-status.xlsx

The document is copied byte-identical and never rewritten. The provenance
goes in ` + "`" + work.ReferenceIndex + "`" + ` instead of a header pasted into their file, so that
when they send a second version you can diff it against the filed one. For the
same reason, filing over an existing name is refused: a customer's second version
is a different document, and the two together are how anyone sees what changed.

--dated is the document's own date, not today. It decides whether the
material is stale. If the document carries no date, record that it has none:
material a customer wrote for their own staff describes the system they believe
they have, and a stale page looks the same as a current one.

--what describes the document in a sentence. "the RMA status codes, as their
support team uses them" tells a later reader more than "rma.xlsx" does, and is
what they use to decide whether to open it.

Every row starts ` + "`verified: no`" + ` and is meant to be edited by hand once you have
held a claim against the running system. Leave it at no until then, so the
reader knows which rows to trust.

Filing material is not reading it. ` + "`asgard-cli check`" + ` says so: material in
` + "`references/`" + ` with no question and no request recorded against it is a warning,
because the interview is what turns a document into a requirement.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := loadRepo()
			if err != nil {
				return err
			}
			ref.Filed = today()

			out := cmd.OutOrStdout()
			filed, err := work.AddReference(root, args[0], ref, out)
			if err != nil {
				return err
			}

			fmt.Fprintf(out, "Filed %s\n", filepath.ToSlash(filepath.Join(work.ReferenceDir, filed.Path)))
			fmt.Fprintf(out, "Recorded in %s\n", filepath.ToSlash(work.ReferenceIndex))

			var missing []string
			if filed.What == "" {
				missing = append(missing, "--what")
			}
			if filed.From == "" {
				missing = append(missing, "--from")
			}
			if filed.Dated == "" {
				missing = append(missing, "--dated")
			}
			if len(missing) > 0 {
				fmt.Fprintf(out, "\nThe row is missing %v. Fill them in now: whoever handed you\nthis document is the only person who knows, and may not be available later.\n", missing)
			}

			fmt.Fprintf(out, `
Filing is not reading. What turns this into a requirement is the interview:

    asgard-cli question add "<what it left open>" --ask "<who>"
    asgard-cli guide requirements
`)
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVar(&ref.What, "what", "", "what the document is, in your words - not its file name")
	f.StringVar(&ref.From, "from", "", "who supplied it; a role outlives a person")
	f.StringVar(&ref.Dated, "dated", "", "the document's own date (YYYY-MM-DD or YYYY-MM), not today")
	f.StringVar(&ref.Path, "as", "", "name to file it under, relative to references/ (defaults to the file's own name)")
	return cmd
}
