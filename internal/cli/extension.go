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

    asgard-cli extension search google drive          find one on this platform
    asgard-cli extension show asgard-ext-google-drive  one in full, and its versions
    asgard-cli extension get asgard-ext-google-drive   bring it into this repository
    asgard-cli extension status                        what this repository holds
    asgard-cli extension update asgard-ext-google-drive
    asgard-cli extension record asgard-ext-google-drive --route <route>

Published Extensions come through the platform, which relays its registry, so
no git access to an Extension's repository is needed here.

For an author:

    asgard-cli extension get ../asgard-ext-foo    copy one from a path on this machine
    asgard-cli extension gate                     check the Extension in this directory
    asgard-cli extension lint                     its first step, on its own`,
	}
	cmd.AddCommand(newExtensionSearchCmd(), newExtensionShowCmd(), newExtensionGetCmd(), newExtensionStatusCmd(),
		newExtensionUpdateCmd(), newExtensionRecordCmd(), newExtensionGateCmd(), newExtensionLintCmd())
	return cmd
}

func newExtensionGetCmd() *cobra.Command {
	var (
		force   bool
		profile string
	)
	cmd := &cobra.Command{
		Use:   "get <id-or-path>[@<version>] | <local path>",
		Short: "Bring an Extension into this repository, from the platform or from a path",
		Long: `Bring an Extension into this repository, from the platform or from a path.

    asgard-cli extension get asgard-ext-google-drive
    asgard-cli extension get asgard-ext-google-drive@v1.3.0-rc.1
    asgard-cli extension get github.com/asgard-ai-platform/asgard-ext-netsuite
    asgard-cli extension get ../asgard-ext-foo

By id or full path it downloads from the platform. Without a version it takes
the latest release; a pre-release (v1.3.0-rc.1) only when named, or when the
Extension has no release at all. The download is checked against the digest the
platform sends, and only what an Extension publishes is written - README.md,
SKILL.md, CHANGELOG.md and routes/, brakes/, setup/, knowledge/, examples/,
scripts/, verify/, assets/ - into the skills directory this repository already
uses (` + "`.agents/skills/`" + `, or ` + "`.claude/skills/`" + `), under the Extension's id.
` + "`" + extension.LockName + "`" + ` records the full path, the version, its commit and a digest
of every file; commit both.

The Extension's ` + "`requires`" + ` is checked against this repository and this
asgard-cli, and a mismatch only warns: the copy goes ahead.

A path starting with ./, ../ or / is a local copy instead, which is for an
author testing an Extension that is not published yet: write it
in its own repository, keep a separate FDE repository beside it for testing,
and run this there. Test it with a coding agent session other than the one that
wrote it.

A local copy copies, it does not link. Only what an Extension publishes is
copied, so the test repository holds what a customer's would, and the session
testing it cannot edit the Extension's own repository.

The copy lands where a fetched one does. ` + "`" + extension.LockName + "`" + ` records it with
` + "`source: local`" + `,
the path it came from, the source's commit and whether it had uncommitted
changes. ` + "`asgard-cli extension update <id>`" + ` copies it again.

The local Extension is linted first. Problems are printed and the copy goes
ahead: an Extension being written is often not complete yet.

Either way it fails, and writes nothing, when the destination holds files
asgard-cli did not write or that were edited since. ` + "`--force`" + ` replaces them.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := fdeRepoRoot()
			if err != nil {
				return err
			}
			if isLocalPath(args[0]) {
				return copyExtension(cmd, root, args[0], force)
			}
			ext, ver, err := splitExtensionRef(args[0])
			if err != nil {
				return err
			}
			pc, err := resolveContext(cmd, contextOptions{Profile: profile})
			if err != nil {
				return err
			}
			return fetchExtension(cmd, root, pc, ext, ver, force)
		},
	}
	addProfileFlag(cmd, &profile)
	cmd.Flags().BoolVar(&force, "force", false, "replace files in the destination that were edited or not written by asgard-cli")
	return cmd
}

func newExtensionUpdateCmd() *cobra.Command {
	var (
		force   bool
		profile string
	)
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Move an Extension to its latest version, or copy a local one again",
		Long: `Move an Extension to its latest version, or copy a local one again.

    asgard-cli extension update asgard-ext-google-drive

For an Extension fetched from the platform it asks for the latest version and,
when that is newer than the one recorded in ` + "`" + extension.LockName + "`" + `, fetches it
the way ` + "`asgard-cli extension get`" + ` does. The routes recorded for it are kept;
read their route documents again after the update. To take a particular
version, older or a pre-release, ` + "`asgard-cli extension get <id>@<version>`" + `.

For one taken with ` + "`asgard-cli extension get <path>`" + `: it reads the
path from ` + "`" + extension.LockName + "`" + ` and copies again, so an author who changed the
Extension can test the change. It fails when the recorded path does not exist
on this machine - in a clone of the test repository, say - and then changes
nothing.

Files the Extension no longer has are removed. It fails, and writes nothing,
when a file in the destination was edited here or was not written by
asgard-cli; ` + "`--force`" + ` replaces them.`,
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
			if entry.Source != extension.SourceLocal {
				return updateFromPlatform(cmd, root, key, entry, profile, force)
			}
			if entry.Path == "" {
				return fmt.Errorf("%s is committed here (source: local) with no path to copy it from, so there is nothing to update it from", key)
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
	addProfileFlag(cmd, &profile)
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
		return fmt.Errorf("%s already holds %s, fetched from the platform; a local copy would take the same directory", extension.LockName, existing)
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
	entry.Path = recordedPath(root, src)
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

It needs no account and no network. It is the only check of an Extension's
content: the registry takes a version in when the tag is semver, README.md's
frontmatter parses and the id matches the repository, and judges nothing else.
So run it before every tag. It checks the index in README.md, SKILL.md's name,
that routes/ holds exactly one document per declared route, the logo, and what
the FDE Agent relies on: an id starting with ` + "`" + extension.Prefix + "`" + `, no route
using an OAuth client Asgard registers, brakes from the catalogue this
asgard-cli carries, and every example saying in its first line that it is one.

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

// recordedPath is how the lock spells where a local copy came from: relative
// when the source sits beside the repository or inside it, so the record reads
// the same on every machine that keeps the two side by side, and absolute
// otherwise, where a chain of ../ would only describe this machine's layout.
func recordedPath(root, src string) string {
	r, err := filepath.Rel(root, src)
	if err != nil {
		return src
	}
	r = filepath.ToSlash(r)
	if strings.HasPrefix(r, "../..") {
		return src
	}
	return r
}
