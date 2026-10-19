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
		Short: "File a source - a customer document or a reading off a running system - with where it came from",
		Long: `File a source, with where it came from: a document the customer hands over,
or a reading off a running system - a query and its result, a CR dumped from
the cluster, an API response.

` + "`references/`" + ` is the raw sources the engagement's pages are written from, read by
humans and spec-writing agents. It is not what
the running agent reads: nothing on the platform mounts or selects it. Rules the
agent applies at run time belong in a skill under ` + "`assets/skills/`" + `; pages it
answers from belong under ` + "`assets/drives/<drive>/`" + `, for a knowledge Drive.

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
		Short: "Copy a document or a reading into references/ and record its provenance",
		Long: `Copy a document or a reading into ` + "`references/`" + ` and record its provenance.

    asgard-cli reference add ~/Downloads/rma.xlsx \
      --what "the RMA status codes, as their support team uses them" \
      --from "their support team" --dated 2024-11 --as erp/rma-status.xlsx

    asgard-cli reference add .out/rma-status-codes.txt \
      --what "the RMA status codes the ERP actually stores" \
      --from "the ERP database, SELECT DISTINCT status FROM rma" \
      --dated 2026-10-19 --as erp/rma-status-codes.txt

A reading is saved to a file first and filed the same way, so that a claim
written from it points at something on disk rather than at a sentence saying it
was checked. --from names the system and the query or command, and --dated is
the day it was read. Take out credentials and personal data before filing, and
keep only the part that supports the claim: ` + "`references/`" + ` is committed, and a
git Syncer on this repository without includePaths copies it onto the platform.

The document is copied byte-identical and never rewritten. The provenance
goes in ` + "`" + work.ReferenceIndex + "`" + ` instead of a header pasted into their file, so that
when they send a second version you can diff it against the filed one. For the
same reason, filing over an existing name is refused: a customer's second version
is a different document, and the two together are how anyone sees what changed.

--dated is the document's own date, not today, or for a reading the day it was
read. It decides whether the material is stale. If the document carries no date, record that it has none:
material a customer wrote for their own staff describes the system they believe
they have, and a stale page looks the same as a current one.

--what describes the document in a sentence. "the RMA status codes, as their
support team uses them" tells a later reader more than "rma.xlsx" does, and is
what they use to decide whether to open it.

Every row starts ` + "`verified: no`" + ` and is meant to be edited by hand once you have
held a claim against the running system. Leave it at no until then, so the
reader knows which rows to trust. A reading is the running system as of its
date, so set its row by hand once it is filed. Checking a customer document
against the system is itself a reading: file it, and name it in the document's
row.

Filing material is not reading it. ` + "`asgard-cli check`" + ` says so: material in
` + "`references/`" + ` with no question and no request recorded against it is a warning,
because the interview is what turns a document into a requirement.

Filing it here does not give it to the deployed agent: nothing mounts
` + "`references/`" + `. When the agent is meant to answer from this material,
suggest a knowledge Drive to the person you are working with. When they do not
know how to build one, start with pages written from this file under
` + "`assets/drives/<drive>/`" + `, an ` + "`index.md`" + ` per folder, and no Context
Index - ` + "`assets/drives/README.md`" + ` and the section "When unsure: a written
wiki, with no index" in
` + "`.agents/skills/asgard-platform/usecase/knowledge-drive.md`" + `.`,
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
	f.StringVar(&ref.From, "from", "", "who supplied it, a role rather than a person; for a reading, the system and the query or command")
	f.StringVar(&ref.Dated, "dated", "", "the document's own date (YYYY-MM-DD or YYYY-MM), not today; for a reading, the day it was read")
	f.StringVar(&ref.Path, "as", "", "name to file it under, relative to references/ (defaults to the file's own name)")
	return cmd
}
