package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/work"
)

// workbenchRefDir is where pulled attachments land, under references/.
const workbenchRefDir = "workbench"

func newWorkbenchPullCmd() *cobra.Command {
	var (
		f        workbenchFlags
		issues   []string
		pipeline string
	)
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "File an issue's or a pipeline's attachments into references/",
		Long: `Download the attachments that still exist on the named issues, or on every issue
about one pipeline, into this repository's references/, and record each in
` + "`references/_index.md`" + ` with the what, from and dated its uploader gave it.

    asgard-cli workbench pull --issue ISS-12
    asgard-cli workbench pull --pipeline support-bot

Each lands at references/workbench/ISS-<n>/<attachment id>/<original name>,
byte-identical, and its SHA-256 is checked against what the platform
recorded before anything is written - a file that does not match is not filed.
The attachment id is in the path because a customer's second version usually
has the first one's name, and both are kept so that they can be diffed.

Running it again is safe. An attachment already filed with the same bytes is
skipped; one whose path is taken by different bytes is reported and left alone,
because the filed copy is evidence and is never overwritten. An attachment
deleted on the platform is not pulled, and one pulled before it was deleted
stays where it is.

A filed attachment has not been read yet, and its content is not established
fact. It becomes a requirement when somebody reads it and records a question or
a request.
Every row starts verified: no, the same as "asgard-cli reference add".

It writes into the checkout, so it has to be run inside one that
"asgard-cli init" wrote. Exactly one of --issue and --pipeline is required.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if (len(issues) == 0) == (pipeline == "") {
				return fmt.Errorf("name what to pull: --issue ISS-N (repeatable) or --pipeline <name or id>, one of the two")
			}
			root, err := loadRepo()
			if err != nil {
				return err
			}
			pc, err := f.context(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()

			var numbers []int64
			for _, v := range splitList(issues) {
				n, err := parseIssueNumber(v)
				if err != nil {
					return err
				}
				numbers = append(numbers, n)
			}
			var pipelines []string
			if pipeline != "" {
				// Every issue about the pipeline, in every status: an
				// attachment on a done issue is still material somebody filed.
				id, err := newWorkbenchNames(ctx, pc).pipelineID("pipeline", pipeline, false)
				if err != nil {
					return err
				}
				pipelines = []string{id}
			}
			attachments, err := pc.Client.ListWorkspaceAttachments(ctx, pipelines, numbers)
			if err != nil {
				return err
			}

			var results []pullResult
			for _, a := range attachments {
				results = append(results, pullOne(cmd, pc.Client, root, a))
			}

			out := cmd.OutOrStdout()
			failed := 0
			for _, r := range results {
				if r.Outcome == pullFailed || r.Outcome == pullConflict {
					failed++
				}
			}
			if f.format == formatJSON {
				if results == nil {
					results = []pullResult{}
				}
				if err := writeJSON(out, map[string]any{"attachments": results}); err != nil {
					return err
				}
				if failed > 0 {
					return ErrSilent
				}
				return nil
			}
			if len(results) == 0 {
				fmt.Fprintf(out, "No attachments to pull.\n")
				return nil
			}
			for _, r := range results {
				fmt.Fprintf(out, "%-9s %s", r.Outcome, r.Path)
				if r.Detail != "" {
					fmt.Fprintf(out, "\n          %s", r.Detail)
				}
				fmt.Fprintln(out)
			}
			fmt.Fprintf(out, "\nThese are filed but not read yet: read each one and record what it leaves open with \"asgard-cli question add\".\n")
			if failed > 0 {
				return fmt.Errorf("%s not filed; see above", plural(failed, "attachment"))
			}
			return nil
		},
	}
	f.register(cmd)
	cmd.Flags().StringSliceVar(&issues, "issue", nil, "an issue whose attachments to pull (ISS-N); repeat or comma-separate for several")
	cmd.Flags().StringVar(&pipeline, "pipeline", "", "pull the attachments of every issue about this pipeline, by name or id; never taken from the checkout's binding")
	return cmd
}

type pullOutcome string

const (
	pullFiled    pullOutcome = "filed"
	pullSkipped  pullOutcome = "unchanged"
	pullConflict pullOutcome = "conflict"
	pullFailed   pullOutcome = "failed"
)

type pullResult struct {
	Issue      string      `json:"issue"`
	Attachment string      `json:"attachment_id"`
	Path       string      `json:"path"`
	SHA256     string      `json:"sha256"`
	Outcome    pullOutcome `json:"outcome"`
	Detail     string      `json:"detail,omitempty"`
}

// pullOne files one attachment. It never overwrites and never files bytes
// that did not verify.
func pullOne(cmd *cobra.Command, c *platform.Client, root string, a *platform.WorkbenchAttachment) pullResult {
	name := safeFileName(a.OriginalFilename)
	rel := path.Join(workbenchRefDir, issueRef(a.IssueNumber), safeFileName(a.ID), name)
	r := pullResult{Issue: issueRef(a.IssueNumber), Attachment: a.ID, Path: path.Join(work.ReferenceDir, rel), SHA256: a.SHA256}
	dest := filepath.Join(root, work.ReferenceDir, filepath.FromSlash(rel))

	if existing, err := fileSHA256(dest); err == nil {
		if strings.EqualFold(existing, a.SHA256) {
			r.Outcome = pullSkipped
			return r
		}
		r.Outcome, r.Detail = pullConflict, fmt.Sprintf("already holds different bytes (sha256 %s); the filed copy is left as it is", existing)
		return r
	}

	tmp, err := os.CreateTemp("", "asgard-attachment-*")
	if err != nil {
		r.Outcome, r.Detail = pullFailed, err.Error()
		return r
	}
	defer os.Remove(tmp.Name())
	sum, err := c.DownloadWorkbenchAttachment(cmd.Context(), a, tmp)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		r.Outcome, r.Detail = pullFailed, err.Error()
		if errors.Is(err, platform.ErrChecksumMismatch) {
			r.Detail += "; nothing was filed"
		}
		return r
	}
	r.SHA256 = sum

	ref := work.Reference{Path: rel, What: a.What, From: a.From, Dated: a.Dated, Filed: today()}
	if _, err := work.AddReference(root, tmp.Name(), ref, io.Discard); err != nil {
		r.Outcome, r.Detail = pullFailed, err.Error()
		return r
	}
	r.Outcome = pullFiled
	return r
}

// safeFileName keeps a name the platform returned from leaving the directory
// it is meant for.
func safeFileName(name string) string {
	name = strings.NewReplacer("/", "_", "\\", "_").Replace(strings.TrimSpace(name))
	if name == "" || name == "." || name == ".." {
		return "attachment"
	}
	return name
}

func fileSHA256(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
