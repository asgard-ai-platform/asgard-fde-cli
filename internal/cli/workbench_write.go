package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

// ---- comment

func newWorkbenchCommentCmd() *cobra.Command {
	var (
		f      workbenchFlags
		body   bodyFlags
		status string
	)
	cmd := &cobra.Command{
		Use:   "comment <ISS-N>",
		Short: "Post a comment on an issue",
		Long: `Post a comment on an issue, as the member's assistant: the platform
authorizes it as the signed-in account and the timeline marks it
"via Asgard AI". The body is markdown, from --body or --body-file (- reads
stdin); one of the two is required.

    asgard-cli workbench comment ISS-12 --body "Asked the customer; waiting on their DBA."
    asgard-cli workbench comment ISS-12 --body-file draft.md --status done

--status moves the issue in the same write ("comment and mark as done"): the
comment and the move land together or not at all.

When the member wants to read a comment before it is posted, write it to a
file first, let them read it, and post that file once they say so. There is no
draft on the platform.

A comment cannot be deleted by this command or through the assistant; only the
member can. On a locked issue only workspace admins may comment.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			number, err := parseIssueNumber(args[0])
			if err != nil {
				return err
			}
			text, ok, err := body.read(cmd)
			if err != nil {
				return err
			}
			if !ok || strings.TrimSpace(text) == "" {
				return errors.New("a comment needs a body: pass --body or --body-file")
			}
			if cmd.Flags().Changed("status") {
				if status, err = normalizeEnum("status", status, workbenchStatuses); err != nil {
					return err
				}
			}
			pc, err := f.context(cmd)
			if err != nil {
				return err
			}
			actingOn(cmd, pc.Session)
			c, err := pc.Client.CreateWorkbenchComment(cmd.Context(), number, text, status)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if f.format == formatJSON {
				return writeJSON(out, c)
			}
			fmt.Fprintf(out, "Commented on %s in workspace %s", issueRef(number), pc.Workspace)
			if status != "" {
				fmt.Fprintf(out, ", and moved it to %s", enumLabel(status, workbenchStatuses))
			}
			fmt.Fprintln(out, ".")
			return nil
		},
	}
	f.register(cmd)
	cmd.Flags().StringVar(&body.body, "body", "", "the comment, in markdown")
	cmd.Flags().StringVar(&body.file, "body-file", "", "read the comment from this file; - reads stdin")
	cmd.Flags().StringVar(&status, "status", "", "also move the issue to this status, in the same write (backlog, to_do, ready, in_progress, pending_fix, in_review or done)")
	return cmd
}

// ---- attach

// datedPattern is the shape --dated takes.
var datedPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func newWorkbenchAttachCmd() *cobra.Command {
	var (
		f                             workbenchFlags
		what, from, dated, supersedes string
		name                          string
	)
	cmd := &cobra.Command{
		Use:   "attach <ISS-N> <file>",
		Short: "Attach a file to an issue",
		Long: `Attach a file to an issue, byte for byte, as the member's assistant.

    asgard-cli workbench attach ISS-12 /work/.blobs/<blobId>/minutes.pdf \
        --what "kickoff meeting minutes" --from "customer PM" --dated 2026-09-29

--what (what the file is), --from (who gave it - a role, not a person's name)
and --dated (the document's own date, YYYY-MM-DD) are required: they are what
"asgard-cli workbench pull" writes next to the file when it lands in a
repository's references/. Without them nobody can tell where the file came
from, and it cannot be used as evidence.

Attaching a file to an issue does not make what it says true; the issue's
fields and comments hold what is established.

A newer version of a file already attached is a new upload with --supersedes
naming the earlier attachment's id (from "asgard-cli workbench show"); the
earlier one stays, so the two can be compared. Never delete the old one to
replace it; this command cannot delete anything.

In the Workbench assistant's sandbox, a file the member dropped into the chat
is already at /work/.blobs/<blobId>/<name>; attach it from there. At most
25 MiB per file.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			number, err := parseIssueNumber(args[0])
			if err != nil {
				return err
			}
			for flag, v := range map[string]string{"what": what, "from": from, "dated": dated} {
				if strings.TrimSpace(v) == "" {
					return fmt.Errorf("--%s is required: an attachment says what it is, who gave it and when it is from", flag)
				}
			}
			if !datedPattern.MatchString(dated) {
				return fmt.Errorf("--dated %q is not a date; write YYYY-MM-DD", dated)
			}
			path := args[1]
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			if info.IsDir() {
				return fmt.Errorf("%s is a directory; attach one file at a time", path)
			}
			if name == "" {
				name = filepath.Base(path)
			}
			pc, err := f.context(cmd)
			if err != nil {
				return err
			}
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			actingOn(cmd, pc.Session)
			a, err := pc.Client.UploadWorkbenchAttachment(cmd.Context(), number, name, file, platform.WorkbenchAttachmentUpload{
				What: what, From: from, Dated: dated, Supersedes: supersedes,
			})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if f.format == formatJSON {
				return writeJSON(out, a)
			}
			fmt.Fprintf(out, "Attached %s to %s in workspace %s (attachment %s, sha256 %s).\n",
				a.OriginalFilename, issueRef(number), pc.Workspace, a.ID, a.SHA256)
			return nil
		},
	}
	f.register(cmd)
	fl := cmd.Flags()
	fl.StringVar(&what, "what", "", "what the file is (required)")
	fl.StringVar(&from, "from", "", "who gave it, as a role rather than a person's name (required)")
	fl.StringVar(&dated, "dated", "", "the document's own date, YYYY-MM-DD (required)")
	fl.StringVar(&supersedes, "supersedes", "", "the id of an earlier attachment of this issue that this one is a newer version of")
	fl.StringVar(&name, "name", "", "the filename to record; defaults to the file's own name")
	return cmd
}
