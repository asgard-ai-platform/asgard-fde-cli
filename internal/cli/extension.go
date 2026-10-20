package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/extension"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/gitrepo"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/repo"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/skills"
)

func newExtensionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "extension",
		Short: "Take an Extension into this repository, or check one you are writing",
		Long: `Take an Extension into this repository, or check one you are writing.

An Extension is design-time material for one external system (Google Drive,
NetSuite, Intercom) or one body of domain knowledge. It lands in
` + "`.agents/skills/<id>/`" + ` beside the skills ` + "`asgard-cli init`" + ` writes, and the
agent reads its SKILL.md to choose a route and write the CRs. It is not a
runtime skill: nothing here touches ` + "`assets/skills/`" + `.

Every id starts with ` + "`" + extension.Prefix + "`" + `. What the repository holds is
recorded in ` + "`" + extension.LockName + "`" + ` at its root, which is committed.

    asgard-cli extension get ../asgard-ext-foo    copy an Extension from a path
    asgard-cli extension update asgard-ext-foo    copy it again after it changed
    asgard-cli extension gate                     check the Extension in this directory
    asgard-cli extension lint                     its first step, on its own

This build copies from a path on this machine, which is how an author tests an
Extension before it is published. Fetching a published one by name comes with
the registry.`,
	}
	cmd.AddCommand(newExtensionGetCmd(), newExtensionUpdateCmd(), newExtensionGateCmd(), newExtensionLintCmd())
	return cmd
}

func newExtensionGetCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "get <path>",
		Short: "Copy an Extension from a path on this machine into this repository",
		Long: `Copy an Extension from a path on this machine into this repository.

    asgard-cli extension get ../asgard-ext-foo

This is for an author testing an Extension that is not published yet: write it
in its own repository, keep a separate FDE repository beside it for testing,
and run this there. Test it with a coding agent session other than the one that
wrote it.

It copies, it does not link. Only what an Extension publishes is copied -
README.md, SKILL.md, CHANGELOG.md and routes/, brakes/, setup/, knowledge/,
examples/, scripts/, verify/, assets/ - so the test repository holds what a
customer's would, and the session testing it cannot edit the Extension's own
repository.

The copy lands in the skills directory this repository already uses
(` + "`.agents/skills/`" + `, or ` + "`.claude/skills/`" + ` where that holds the material), under
the Extension's id. ` + "`" + extension.LockName + "`" + ` records it with ` + "`source: local`" + `,
the path it came from, the source's commit and whether it had uncommitted
changes. ` + "`asgard-cli extension update <id>`" + ` copies it again.

The Extension is linted first. Problems are printed and the copy goes ahead:
an Extension being written is often not complete yet.

It fails, and writes nothing, when the destination holds files asgard-cli did
not write or that were edited since. ` + "`--force`" + ` replaces them.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isLocalPath(args[0]) {
				return fmt.Errorf("%q is not a path; this build copies an Extension from a path on this machine (start it with ./, ../ or /), and fetching a published one by name comes with the registry", args[0])
			}
			root, err := fdeRepoRoot()
			if err != nil {
				return err
			}
			return copyExtension(cmd, root, args[0], force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "replace files in the destination that were edited or not written by asgard-cli")
	return cmd
}

func newExtensionUpdateCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Copy an Extension again from the path it was copied from",
		Long: `Copy an Extension again from the path it was copied from.

    asgard-cli extension update asgard-ext-foo

For an Extension taken with ` + "`asgard-cli extension get <path>`" + `: it reads the
path from ` + "`" + extension.LockName + "`" + ` and copies again, so an author who changed the
Extension can test the change. Files the Extension no longer has are removed.

It fails, and writes nothing, when a file in the destination was edited here
or was not written by asgard-cli; ` + "`--force`" + ` replaces them. It also fails when
the recorded path does not exist on this machine - in a clone of the test
repository, say - and then changes nothing.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := fdeRepoRoot()
			if err != nil {
				return err
			}
			lock, err := extension.ReadLock(root)
			if err != nil {
				return err
			}
			key, ok := lock.Find(args[0])
			if !ok {
				return fmt.Errorf("%s records no Extension %q", extension.LockName, args[0])
			}
			entry := lock.Extensions[key]
			if entry.Source != extension.SourceLocal || entry.Path == "" {
				return fmt.Errorf("%s did not come from a path on this machine; updating a published Extension comes with the registry", key)
			}
			src := entry.Path
			if !filepath.IsAbs(src) {
				src = filepath.Join(root, src)
			}
			if _, err := os.Stat(src); err != nil {
				return fmt.Errorf("%s was copied from %s, which does not exist here; nothing changed", key, entry.Path)
			}
			return copyExtension(cmd, root, src, force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "replace files in the destination that were edited or not written by asgard-cli")
	return cmd
}

// copyExtension copies the Extension at src into the repository at root and
// records it.
func copyExtension(cmd *cobra.Command, root, src string, force bool) error {
	out := cmd.OutOrStdout()
	src, err := filepath.Abs(src)
	if err != nil {
		return err
	}

	findings, meta := extension.Lint(src)
	if meta == nil || !extension.ValidID(meta.ID) {
		printFindings(out, findings)
		return fmt.Errorf("%s has no usable id in README.md, so there is nowhere to put it", src)
	}
	if len(findings) > 0 {
		fmt.Fprintf(out, "%s has problems; copying it anyway, run `asgard-cli extension lint` there for the list:\n", meta.ID)
		printFindings(out, findings)
		fmt.Fprintln(out)
	}

	files, err := extension.Collect(src)
	if err != nil {
		return err
	}

	lock, err := extension.ReadLock(root)
	if err != nil {
		return err
	}
	key := meta.ID
	if existing, ok := lock.Find(meta.ID); ok && existing != meta.ID {
		return fmt.Errorf("%s already holds %s from a registry; a local copy would take the same directory", extension.LockName, existing)
	}
	skillsRoot := skills.Root(root, "")
	dst := filepath.Join(skillsRoot, meta.ID)

	conflicts, err := extension.Conflicts(dst, lock.Extensions[key].Files)
	if err != nil {
		return err
	}
	if len(conflicts) > 0 && !force {
		for _, c := range conflicts {
			fmt.Fprintf(cmd.ErrOrStderr(), "- %s: %s\n", c.Path, c.Reason)
		}
		return fmt.Errorf("%s holds files this copy would replace; nothing was written. --force replaces them", rel(root, dst))
	}

	digests, err := extension.Replace(dst, files)
	if err != nil {
		return err
	}

	entry := lock.Extensions[key]
	entry.Source = extension.SourceLocal
	entry.Path = src
	if r, err := filepath.Rel(root, src); err == nil {
		entry.Path = filepath.ToSlash(r)
	}
	entry.Commit, entry.Dirty = sourceState(cmd.Context(), src)
	entry.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	entry.Files = digests
	lock.Extensions[key] = entry
	if err := lock.Write(root); err != nil {
		return err
	}

	fmt.Fprintf(out, "copied %s (%d files) into %s\n", meta.ID, len(digests), rel(root, dst))
	switch {
	case entry.Commit == "":
		fmt.Fprintf(out, "  from %s, which is not a git repository\n", entry.Path)
	case entry.Dirty:
		fmt.Fprintf(out, "  from %s at %s, with uncommitted changes\n", entry.Path, short(entry.Commit))
	default:
		fmt.Fprintf(out, "  from %s at %s\n", entry.Path, short(entry.Commit))
	}
	fmt.Fprintf(out, "recorded in %s; `asgard-cli extension update %s` copies it again\n", extension.LockName, meta.ID)
	return nil
}

func newExtensionLintCmd() *cobra.Command {
	var formatOut string
	cmd := &cobra.Command{
		Use:   "lint [dir]",
		Short: "Check an Extension before it is published",
		Long: `Check an Extension before it is published.

    asgard-cli extension lint             the Extension in this directory
    asgard-cli extension lint ../asgard-ext-foo

It needs no account and no network. It checks what the registry checks before
it offers a version - the index in README.md, SKILL.md's name, that routes/
holds exactly one document per declared route - and what the FDE Agent relies
on: an id starting with ` + "`" + extension.Prefix + "`" + `, no route using an OAuth client
Asgard registers, and every example saying in its first line that it is one.

Examples are pseudo charts, one folder per set of CRs under examples/, and
they are not rendered: placeholders like <chart> and <fde-repo-git-url> stand
for what only the FDE repository knows. What is checked is their shape - an
examples/README.md, a README in each set, each set named after a route and
referenced from a route document, the EXAMPLE banner on every file.

It is the first step of ` + "`asgard-cli extension gate`" + `, which also holds every CR
under examples/ against the checks that need no chart. Run that one.

Exit status is 1 when anything is an error. Warnings do not fail it.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFormat(formatOut); err != nil {
				return err
			}
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			findings, meta := extension.Lint(dir)
			failed := extension.Failed(findings)

			if formatOut == formatJSON {
				id := ""
				if meta != nil {
					id = meta.ID
				}
				if findings == nil {
					findings = []extension.Finding{}
				}
				if err := writeJSON(cmd.OutOrStdout(), map[string]any{"id": id, "ok": !failed, "findings": findings}); err != nil {
					return err
				}
				if failed {
					return ErrSilent
				}
				return nil
			}

			out := cmd.OutOrStdout()
			printFindings(out, findings)
			errs, warns := count(findings)
			if failed {
				fmt.Fprintf(out, "\n%d error(s), %d warning(s)\n", errs, warns)
				return ErrSilent
			}
			if warns > 0 {
				fmt.Fprintf(out, "ok, %d warning(s)\n", warns)
			} else {
				fmt.Fprintln(out, "ok")
			}
			// A lint that passes is when an author stops, and the CRs under
			// examples/ are not what it looked at.
			fmt.Fprintln(out, "That was the structure. `asgard-cli extension gate` also checks the CRs under examples/.")
			return nil
		},
	}
	cmd.Flags().StringVar(&formatOut, formatFlag, formatText, formatUsage)
	return cmd
}

// fdeRepoRoot is the FDE repository the current directory is in.
func fdeRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	root := repo.Root(cwd)
	if root == "" {
		return "", errors.New("not inside an FDE repository (no .asgard-pipeline.yaml here or above); run this in the repository that takes the Extension, after `asgard-cli init`")
	}
	return root, nil
}

// sourceState is the commit a local source is at, and whether it has
// uncommitted changes. Both are empty when the source is not a git repository.
func sourceState(ctx context.Context, dir string) (string, bool) {
	sha, err := gitrepo.HeadSHA(ctx, dir)
	if err != nil {
		return "", false
	}
	dirty, err := gitrepo.Dirty(ctx, dir)
	if err != nil {
		return sha, false
	}
	return sha, dirty
}

func isLocalPath(arg string) bool {
	return strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, "../") || filepath.IsAbs(arg) || arg == "." || arg == ".."
}

func printFindings(out io.Writer, findings []extension.Finding) {
	for _, f := range sortedFindings(findings) {
		fmt.Fprintf(out, "%-7s %s: %s\n", f.Level, f.Path, f.Message)
	}
}

// sortedFindings puts the errors first and keeps the order within each level.
func sortedFindings(findings []extension.Finding) []extension.Finding {
	sorted := append([]extension.Finding(nil), findings...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Level == extension.Error && sorted[j].Level != extension.Error
	})
	return sorted
}

func count(findings []extension.Finding) (errs, warns int) {
	for _, f := range findings {
		if f.Level == extension.Error {
			errs++
		} else {
			warns++
		}
	}
	return errs, warns
}

func rel(root, path string) string {
	if r, err := filepath.Rel(root, path); err == nil {
		return filepath.ToSlash(r)
	}
	return path
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
