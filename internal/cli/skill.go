package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/gitrepo"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/pipelineconfig"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/repo"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/scaffold"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/skills"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/version"
)

func newSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Fetch the reference material for the Asgard server this repository deploys to",
		Long: `Fetch the reference material for the Asgard server this repository deploys to.

These are design-time skills, under ` + "`.agents/skills/`" + `, read by the coding
agent while it authors a chart. They are not the runtime skills under
` + "`assets/skills/`" + ` that a Syncer feeds to a deployed agent - nothing here touches
those, and the bare word means the runtime ones everywhere else in this
material. The glossary has the pair.

An agent writing an Asgard CR needs facts that move with the platform: which
Workflow processors exist, what each config key is called, which are required,
what they default to. Those are not in this binary and must not be.

    asgard-cli skill status    what is here, and what the platform has
    asgard-cli skill update    write what the platform has

` + "`status`" + ` also reports the OTHER half of the material in a repository - the
skills this binary ships, which no platform is party to - because nobody asks
whether the material is current for only one half. ` + "`update`" + ` does not touch that half: it is
written by ` + "`asgard-cli init`" + ` and checked by ` + "`asgard-cli gate`" + `'s ` + "`shipped`" + ` step.

The version is a number the platform declares and a person increments. It is
not a digest of the material: several upstreams feed it - a cluster's CRDs,
the runtime's own constants, the written documents - and any of them can
change for a cosmetic reason, such as a map iterating in a different order.
The number only goes up.

As a result, the material can change without the number moving. ` + "`skill status`" + ` says so when it does, and ` + "`skill update`" + ` writes what the
platform has now whatever the number says.

The material is fetched because of on-prem installations. Asgard runs as a hosted platform and,
for some customers, on their own hardware. A customer's server can be several
versions behind this CLI or ahead of it, and a document saying what
` + "`llm-completion`" + ` takes is only true of one of them. Compiled into the binary,
it would be pinned to whichever release somebody installed - which has nothing
to do with the server their runs deploy to - and there would be no way to
correct it without shipping a new binary. Fetched, it describes the server that
will accept or reject the CR.

The files land in ` + "`.agents/skills/`" + ` (or ` + "`.claude/skills/`" + ` where a repository
already uses that), and they are meant to be committed: whoever clones the
repository, and whatever agent works in it, then has them without a fetch.

Once fetched, they stay in the directory they were fetched into. Which of
the two it is is decided by where the record already is, not by which directory
exists - ` + "`asgard-cli init`" + ` creates ` + "`.agents/skills`" + ` whether or not
anything has been fetched, so a repository that fetched into ` + "`.claude/skills`" + `
keeps using it. A copy in the directory that is not in use is reported by
` + "`skill status`" + ` and fails ` + "`asgard-cli gate`" + `, because nothing updates it.

Whether the files can be committed is checked. Many repositories carry a
` + "`.claude/`" + ` line in their ` + "`.gitignore`" + `, which excludes one of the two
directories, so where an ignore rule covers the material the command says so
instead of telling somebody to commit what git will not take.

They are generated and say so. Do not edit them: the next update overwrites
them, and an edit states something about the server that the server did not.
An edited file is reported rather than silently replaced.

They do not contain judgement. They describe shape - names, types, defaults -
not which mistakes apply cleanly and fail at runtime. That is what the
hand-written skills beside them are for.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newSkillStatusCmd(), newSkillUpdateCmd())
	return cmd
}

// skillRoot resolves where the material goes, and says so, so that a command
// can print a path rather than leave somebody guessing which of two
// directories was written.
func skillRoot(cmd *cobra.Command, override string) (root, repoRoot string, err error) {
	repoRoot, _, _ = locateRepo(cmd.Context())
	if repoRoot == "" {
		// Outside a checkout the working directory is the only honest answer.
		// It is also usually a mistake, which `status` says out loud.
		repoRoot, err = os.Getwd()
		if err != nil {
			return "", "", err
		}
	}
	return skills.Root(repoRoot, override), repoRoot, nil
}

func newSkillStatusCmd() *cobra.Command {
	var (
		profile string
		dir     string
		format  string
	)

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Report which version of the reference material is here, and which the platform has",
		Long: `Report which version of the reference material is in this repository, and
which the platform has.

It writes nothing and always exits 0. The question it answers is whether an
agent working here is reading facts about the server it deploys to, or facts
about some other version of Asgard.

If the two versions differ, the material was right when it was written and
the server has changed since, so what an agent reads here describes a
different version.

With no session it still answers the half that needs no platform, and says so
on the platform line instead of failing. This applies on a CI runner, in an
agent sandbox or offline, where only the local half is available.

It reports two halves. Above is the platform's material and the version it
declares. Below it is the material this CLI ships - AGENTS.md and the
design-time skills - and which version of the binary wrote what is here. The
two version numbers are unrelated, and the remedies are different commands. ` + "`asgard-cli gate`" + ` names the individual files.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := checkFormat(format); err != nil {
				return err
			}
			root, repoRoot, err := skillRoot(cmd, dir)
			if err != nil {
				return err
			}
			stamp, err := skills.ReadStamp(root)
			if err != nil {
				return err
			}

			// **Not having a session is a state, not a failure.** It is the
			// ordinary condition of a CI runner and of an agent sandbox, and
			// this command's own help says it always exits 0 - which it did
			// not, so the half of the answer that needs no platform was
			// unreachable in exactly the places it is the only half available.
			// `asgard-cli gate`'s skills step reaches the same conclusion for
			// the same reason and calls it a skip.
			var (
				remote      *platform.DocsVersion
				profileName string
				unanswered  string
			)
			pc, err := resolveContext(cmd, contextOptions{Profile: profile})
			switch {
			case err != nil:
				unanswered = "no session, so the platform was not asked (`asgard-cli login`)"
			default:
				profileName = pc.Session.Profile.Name
				remote, err = pc.Client.DocsVersionOnly(cmd.Context())
				if err != nil {
					unanswered = fmt.Sprintf("the platform did not answer: %v", err)
				}
			}

			local := ""
			fetchedAt := ""
			if stamp != nil {
				local = stamp.Version
				fetchedAt = stamp.FetchedAt
			}

			out := cmd.OutOrStdout()
			rel := root
			if r, relErr := filepath.Rel(repoRoot, root); relErr == nil {
				rel = r
			}

			if format == formatJSON {
				record := map[string]any{
					// The absolute path, as it has always been here: a reader
					// of this JSON depends on the shape, and changing a value
					// under it is the kind of change that is noticed by
					// whatever breaks.
					"directory":  root,
					"local":      local,
					"fetched_at": fetchedAt,
					"shipped":    shippedStatus(repoRoot),
					// Which directory, and whether git will carry it, are
					// facts about this checkout that no platform is party to -
					// and both are things a reader of only the version number
					// would get wrong.
					"other_roots": otherRoots(repoRoot, root),
					"ignored":     skillsDirIgnoredNote(cmd, repoRoot, root),
				}
				if unanswered != "" {
					record["unanswered"] = unanswered
				} else {
					record["platform"] = remote.Version
					record["profile"] = profileName
					record["platform_api"] = pc.Session.Profile.PlatformAPI
					record["sources"] = remote.Sources
					record["current"] = local != "" && local == remote.Version
				}
				return writeJSON(out, record)
			}

			fmt.Fprintf(out, "%-11s %s\n", "directory", rel)
			if unanswered != "" {
				fmt.Fprintf(out, "%-11s %s\n", "platform", unanswered)
				if local != "" {
					fmt.Fprintf(out, "%-11s %s", "here", local)
					if fetchedAt != "" {
						fmt.Fprintf(out, "  (fetched %s)", fetchedAt)
					}
					fmt.Fprintln(out)
				} else {
					fmt.Fprintf(out, "%-11s none\n", "here")
				}
			} else {
				fmt.Fprintf(out, "%-11s %s\n", "profile", profileName)
				fmt.Fprintf(out, "%-11s %s\n", "platform", remote.Version)
				for _, s := range remote.Sources {
					fmt.Fprintf(out, "%-11s   %-12s %s (%d)\n", "", s.Name, s.Digest, s.Records)
				}
				if local == "" {
					fmt.Fprintf(out, "%-11s none\n", "here")
					fmt.Fprintf(out, "\nNothing has been fetched into this repository, so an agent working here\n"+
						"has no statement of what this server accepts.\n\n    asgard-cli skill update\n")
				} else {
					fmt.Fprintf(out, "%-11s %s", "here", local)
					if fetchedAt != "" {
						fmt.Fprintf(out, "  (fetched %s)", fetchedAt)
					}
					fmt.Fprintln(out)
					printPlatformVerdict(out, stamp, remote, profileName)
				}
			}

			if note := skillsDirIgnoredNote(cmd, repoRoot, root); note != "" {
				fmt.Fprintf(out, "\n%s.\nThe files are on disk so that a clone has them without a fetch. An ignore\n"+
					"rule prevents that, even though every version number above reads as\n"+
					"current.\n", strings.ToUpper(note[:1])+note[1:])
			}
			printOtherRoots(out, repoRoot, root)
			printShippedStatus(out, repoRoot)
			return nil
		},
	}

	addProfileFlag(cmd, &profile)
	cmd.Flags().StringVar(&dir, "dir", "", "skills directory, relative to the repository root; defaults to wherever the material already is, else .agents/skills, else .claude/skills where that exists")
	cmd.Flags().StringVar(&format, formatFlag, formatText, formatUsage)
	return cmd
}

// printPlatformVerdict says what the two version numbers mean, and is the
// paragraph that used to end the command.
//
// It stopped ending it when there was a second half to report. Each of these
// was a `return nil` reached from a different branch, and reporting anything
// after them meant they all had to stop being exits - which is the kind of
// change that quietly drops a case, so they are all here, together, unaltered.
func printPlatformVerdict(out io.Writer, stamp *skills.Stamp, remote *platform.DocsVersion, profileName string) {
	local := stamp.Version
	if local != remote.Version {
		if skills.Behind(local, remote.Version) {
			fmt.Fprintf(out, "\nBehind: the platform has published a newer version of the material.\n\n    asgard-cli skill update\n")
			return
		}
		fmt.Fprintf(out, "\nDifferent: this was not written from %s.\n"+
			"A repository can hold material fetched from another platform - check the api line above.\n\n"+
			"    asgard-cli skill update\n", profileName)
		return
	}

	// Same version and different content is a state the declared version makes
	// possible on purpose: somebody publishes a change and does not consider
	// it worth telling everybody about. It still changes what an author reads,
	// so it is worth saying here.
	if moved := skills.MovedSources(stamp, sourceDigests(remote.Sources)); len(moved) > 0 {
		fmt.Fprintf(out, "\nSame version, different material: %s moved since this was fetched.\n"+
			"The version is declared rather than derived, so a change reaches you when you ask.\n\n"+
			"    asgard-cli skill update\n", strings.Join(moved, ", "))
		return
	}
	fmt.Fprintf(out, "\nCurrent.\n")
}

// shippedStatus counts the states of the material this CLI ships into a
// repository, for the half of `skill status` that needs no platform.
//
// **Both halves are reported by one command because nobody asks the question
// in halves.** Somebody asking whether the material here is current does not
// know that it has two authorities - and until this existed they got a
// confident answer about one of them with no hint that the other was there.
// They are two blocks rather than one verdict for the opposite reason: the
// version numbers are unrelated, the remedies are different commands, and one
// line covering both would be a claim neither authority made.
func shippedStatus(repoRoot string) map[string]any {
	out := map[string]any{"binary": version.Get().Version}

	projects, err := repo.Projects(repoRoot)
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	results, err := scaffold.InspectShipped(repoRoot, projects)
	if err != nil {
		out["error"] = err.Error()
		return out
	}

	// `Skipped` is Write's word for "there was nothing to do", and reading it
	// back as a state of the material would say thirteen files were skipped
	// when what is true is that thirteen files match. An inspection has its own
	// vocabulary for the one status whose name only makes sense to a writer.
	counts := map[string]int{}
	matching, missing := 0, 0
	for _, r := range results {
		switch r.Status {
		case scaffold.Skipped:
			matching++
		case scaffold.Missing:
			missing++
			counts[r.Status.String()]++
		default:
			counts[r.Status.String()]++
		}
	}
	if matching > 0 {
		counts["matches"] = matching
	}
	if writers, err := scaffold.Writers(repoRoot); err == nil && len(writers) > 0 {
		out["written_by"] = writers
	}
	out["files"] = len(results)
	out["states"] = counts
	out["present"] = missing < len(results)
	out["current"] = len(results) > 0 && matching == len(results)
	return out
}

// printShippedStatus is the second block, and it says which authority it is
// about in its first word - the two version numbers here are unrelated, and a
// reader who takes one for the other has the wrong answer to both.
func printShippedStatus(out io.Writer, repoRoot string) {
	st := shippedStatus(repoRoot)
	fmt.Fprintf(out, "\n%-11s AGENTS.md and the design-time skills, which come from this binary\n", "shipped")
	fmt.Fprintf(out, "%-11s %s\n", "binary", st["binary"])
	if msg, ok := st["error"].(string); ok {
		fmt.Fprintf(out, "%-11s %s\n", "", msg)
		return
	}
	if writers, ok := st["written_by"].([]string); ok {
		fmt.Fprintf(out, "%-11s %s\n", "written by", strings.Join(writers, ", "))
	}

	if present, _ := st["present"].(bool); !present {
		fmt.Fprintf(out, "\nNothing this CLI ships is here, so an agent working here has no contract\n"+
			"and no design-time skills.\n\n    asgard-cli init\n")
		return
	}
	if current, _ := st["current"].(bool); current {
		fmt.Fprintf(out, "\nCurrent.\n")
		return
	}

	counts, _ := st["states"].(map[string]int)
	var parts []string
	for _, name := range []string{"missing", "retired", "behind", "stale", "edited", "ahead", "updated"} {
		if counts[name] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[name], name))
		}
	}
	fmt.Fprintf(out, "\n%s. `asgard-cli gate` says which files, and what each one means.\n",
		strings.Join(parts, ", "))
}

func newSkillUpdateCmd() *cobra.Command {
	var (
		profile   string
		dir       string
		check     bool
		force     bool
		formatOut string
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Write the platform's reference material into this repository",
		Long: `Write the platform's reference material into this repository.

    asgard-cli skill update
    asgard-cli skill update --check    write nothing; exit 1 if it would

Every file is written, not only the changed ones: a partial write would leave
the repository holding two versions at once, with a record that matches
neither.

Commit what it writes. The files are on disk so that whoever clones this
repository has them without a fetch, and so that a change to them shows up in a
diff. A config key that becomes required is one changed line in a review.

A file that was edited by hand since it was last written is reported and left
alone. ` + "`--force`" + ` overwrites it. These files are generated and say so at the
top, so an edit is usually somebody correcting what they believed was a mistake
in the material - which belongs upstream, in the server, not in a file the next
update replaces.

--check is for a gate. It exits 1 when the repository is not holding what the
server has, because then an agent's next CR would be written against facts
that no longer hold.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSkillUpdate(cmd, skillUpdateOptions{
				Profile: profile, Dir: dir, Check: check, Force: force, Format: formatOut,
			})
		},
	}

	addProfileFlag(cmd, &profile)
	cmd.Flags().StringVar(&dir, "dir", "", "skills directory, relative to the repository root; defaults to wherever the material already is, else .agents/skills, else .claude/skills where that exists")
	cmd.Flags().BoolVar(&check, "check", false, "write nothing, and exit 1 if this repository is not holding what the platform has")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite a generated file that was edited here")
	cmd.Flags().StringVar(&formatOut, formatFlag, formatText, formatUsage)
	return cmd
}

// skillUpdateOptions is one run of the update.
type skillUpdateOptions struct {
	Profile string
	Dir     string
	Check   bool
	Force   bool
	Format  string
}

// runSkillUpdate writes the platform's material into the repository.
//
// Split out of the command for `init`, which composes it: fetching the material
// is part of onboarding a repository, and an onboarding that leaves it to be
// remembered separately is how a repository ends up with an agent writing CRs
// against no statement of what the server accepts.
func runSkillUpdate(cmd *cobra.Command, opts skillUpdateOptions) error {
	profile, dir, check, force := opts.Profile, opts.Dir, opts.Check, opts.Force
	formatOut := opts.Format
	if formatOut == "" {
		formatOut = formatText
	}
	if err := checkFormat(formatOut); err != nil {
		return err
	}
	root, repoRoot, err := skillRoot(cmd, dir)
	if err != nil {
		return err
	}
	stamp, err := skills.ReadStamp(root)
	if err != nil {
		return err
	}

	pc, err := resolveContext(cmd, contextOptions{Profile: profile})
	if err != nil {
		return err
	}
	bundle, err := pc.Client.DocsSkills(cmd.Context())
	if err != nil {
		return err
	}
	if len(bundle.Files) == 0 {
		return fmt.Errorf("the platform sent no files, so there is nothing to write; this is a platform fault, not a local one")
	}

	contents := make([]skills.Content, 0, len(bundle.Files))
	for _, f := range bundle.Files {
		contents = append(contents, skills.Content{Path: f.Path, Content: f.Content})
	}
	plan, err := skills.Plan(root, contents, stamp)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	rel := root
	if r, relErr := filepath.Rel(repoRoot, root); relErr == nil {
		rel = r
	}

	var edited []string
	changes := 0
	for _, f := range plan {
		if f.Status == skills.Edited {
			edited = append(edited, f.Path)
		}
		if f.Status != skills.Unchanged {
			changes++
		}
	}

	if formatOut == formatJSON {
		rows := make([]map[string]string, 0, len(plan))
		for _, f := range plan {
			rows = append(rows, map[string]string{"path": f.Path, "status": string(f.Status)})
		}
		if err := writeJSON(out, map[string]any{
			"directory": rel,
			"version":   bundle.Version,
			"applied":   !check && (force || len(edited) == 0),
			"files":     rows,
		}); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(out, "%s  (%s)\n", bundle.Version, pc.Session.Profile.Name)
		for _, f := range plan {
			fmt.Fprintf(out, "  %-10s %s\n", f.Status, filepath.Join(rel, f.Path))
		}
	}

	if check {
		if changes == 0 {
			if formatOut != formatJSON {
				fmt.Fprintf(out, "\nCurrent.\n")
			}
			return nil
		}
		return fmt.Errorf("%d file(s) are not what the platform has; run `asgard-cli skill update`", changes)
	}

	if len(edited) > 0 && !force {
		return fmt.Errorf("%d generated file(s) were edited here since they were written (%s).\n"+
			"They say not to edit them, so this is probably a correction that belongs in the server rather than\n"+
			"in a file the next update replaces. `--force` overwrites them",
			len(edited), edited[0])
	}

	if err := skills.Apply(root, contents, skills.Stamp{
		Version:   bundle.Version,
		Platform:  pc.Session.Profile.PlatformAPI,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
		Sources:   sourceDigests(bundle.Sources),
	}); err != nil {
		return err
	}

	if formatOut != formatJSON {
		if changes == 0 {
			fmt.Fprintf(out, "\nAlready current; the record beside them was refreshed.\n")
		} else {
			fmt.Fprintf(out, "\nWrote %d file(s) into %s.%s\n", len(contents), rel, commitAdvice(cmd, repoRoot, root))
		}
		printOtherRoots(out, repoRoot, root)
		noteShippedHalf(out, repoRoot)
	}
	return nil
}

// commitAdvice is the sentence after "Wrote N file(s) into <dir>".
//
// **"Commit them" is a claim about somebody else's repository**, and the whole
// argument for these files being on disk rests on it: whoever clones the
// repository has them without a fetch, and a change to them shows up in a diff.
// A directory an ignore rule excludes breaks both halves of that while the
// command congratulates itself, and `.claude/` is a line a great many
// repositories already carry - so where the material can land in one, the one
// thing this must not do is tell somebody to commit what git will not take.
//
// It asks git rather than reading a file, and says the ordinary thing when git
// cannot be asked. See gitrepo.Ignored.
func commitAdvice(cmd *cobra.Command, repoRoot, root string) string {
	ignored, known := gitrepo.Ignored(cmd.Context(), repoRoot, root)
	if !known || !ignored {
		return " Commit them."
	}
	return "\n\nAn ignore rule excludes that directory, so git will not take them and a\n" +
		"fresh clone will not have them, although that is why they are written to\n" +
		"disk rather than fetched on demand. Either commit the directory\n" +
		"deliberately (`git add -f`) or fetch into one that is tracked:\n\n" +
		"    asgard-cli skill update --dir .agents/skills"
}

// otherRoots is skills.Elsewhere as JSON, one object per copy that is not in
// use. An empty list is the ordinary answer and is reported as one rather than
// omitted, so a reader can tell "none" from "this version did not look".
func otherRoots(repoRoot, inUse string) []map[string]any {
	out := []map[string]any{}
	for _, o := range skills.Elsewhere(repoRoot, inUse) {
		out = append(out, map[string]any{"directory": o.Dir, "version": o.Version, "files": o.Files})
	}
	return out
}

// printOtherRoots says when a second copy of the material is sitting in the
// other candidate directory.
//
// **An agent's runtime reads that directory too.** Which of the two is in use
// is this tool's decision and no agent's, so a copy left in the other one is
// not inert: it is loaded, it describes whichever server it described when it
// was fetched, and nothing compares it against anything. See skills.Elsewhere
// for how a repository ends up holding two.
func printOtherRoots(out io.Writer, repoRoot, inUse string) {
	others := skills.Elsewhere(repoRoot, inUse)
	if len(others) == 0 {
		return
	}
	for _, o := range others {
		fmt.Fprintf(out, "\n%s also holds fetched material - version %s, %d file(s) - and is not the\n"+
			"directory in use. An agent's runtime reads it as readily as this one, and nothing\n"+
			"updates it. Read it and delete it, or point updates back at it with --dir %s.\n",
			o.Dir, o.Version, o.Files, o.Dir)
	}
}

// noteShippedHalf says when the material this command does NOT write needs
// attention.
//
// **It reports rather than acts, and the line is the authority split.** This
// command writes what the platform has, and the skills beside them come from
// the binary - the long help above spends its length on why that boundary
// exists, and a command that quietly refreshed both would erase it. What it can
// do is stop somebody concluding, from a screen that says "already current",
// that all the material here is.
func noteShippedHalf(out io.Writer, repoRoot string) {
	projects, err := repo.Projects(repoRoot)
	if err != nil {
		return
	}
	results, err := scaffold.InspectShipped(repoRoot, projects)
	if err != nil || len(results) == 0 {
		return
	}
	stale := 0
	for _, r := range results {
		switch r.Status {
		case scaffold.Missing, scaffold.Retired, scaffold.Behind, scaffold.Stale:
			stale++
		}
	}
	if stale == 0 {
		return
	}
	fmt.Fprintf(out, "\nThe other half of the material here does not come from a platform, and %d\n"+
		"file(s) of it are not what this binary carries. `asgard-cli gate` says which.\n", stale)
}

// sourceDigests flattens the bundle's per-upstream digests for the stamp and
// for the comparison.
func sourceDigests(sources []platform.DocsSource) map[string]string {
	if len(sources) == 0 {
		return nil
	}
	out := make(map[string]string, len(sources))
	for _, s := range sources {
		out[s.Name] = s.Digest
	}
	return out
}

// warnIfBehind says so when the version the platform reported during this
// command differs from what this repository holds.
//
// **It is the mechanism, not a nicety.** An agent finds out its reference
// material is stale only if something it already runs tells it - it is not
// going to remember to poll - so this rides on the header every `/v1/iac`
// response carries and prints at the end of whatever was run.
//
// It is silent when there is nothing to compare: no call was made, or no
// material has ever been fetched here. The one exception is a repository that
// holds a declaration and no material at all, which is an agent writing CRs
// with no statement of what the server accepts.
//
// **It says what is missing, not what is bound.** The condition is the
// declaration file, which `init` writes before anything is connected, so a
// message asserting that this repository "declares a pipeline" was printed on
// the first four commands anybody runs - at a moment when `.asgard-cli.yaml`
// held a workspace and no pipeline at all. What the reader needs is which
// version to fetch and the command that fetches it.
func warnIfBehind(cmd *cobra.Command) {
	remote := platform.LastDocsVersion()
	if remote == "" {
		return
	}
	// `skill` has already said it, in more detail and with the remedy in
	// context. Repeating it under the command whose job this is reads like a
	// second, different problem.
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "skill" || c.Name() == "gate" {
			return
		}
	}
	root, repoRoot, err := skillRoot(cmd, "")
	if err != nil {
		return
	}
	stamp, err := skills.ReadStamp(root)
	if err != nil {
		return
	}

	errOut := cmd.ErrOrStderr()
	if stamp == nil {
		if !isPipelineRepo(repoRoot) {
			return
		}
		// "No material yet" is for an agent about to write CRs. The plumbing
		// commands - connecting, creating a repository, the pipeline's
		// bookkeeping, issues, the audit log - write none, and a fresh
		// repository runs a string of them first, each of which used to end
		// with this line. A version mismatch below is still said everywhere.
		for c := cmd; c != nil; c = c.Parent() {
			switch c.Name() {
			case "pipeline", "workbench", "audit-log":
				return
			}
		}
		fmt.Fprintf(errOut, "\nno reference material here yet; this platform serves version %s\n"+
			"    asgard-cli skill update\n", remote)
		return
	}
	if stamp.Version == remote {
		return
	}
	if skills.Behind(stamp.Version, remote) {
		fmt.Fprintf(errOut, "\nthe platform has published version %s of the reference material; this repository holds %s\n"+
			"    asgard-cli skill update\n", remote, stamp.Version)
		return
	}
	fmt.Fprintf(errOut, "\nthis repository holds version %s of the reference material and this platform serves %s\n"+
		"    asgard-cli skill update\n", stamp.Version, remote)
}

// isPipelineRepo reports whether this repository holds a deployment
// declaration. A repository with none is not an IaC repository, and telling it
// to fetch CR-authoring material would be noise.
//
// The file declares RELEASES, and it declares none until somebody writes one -
// so this answers "is this the kind of repository that needs the material",
// never "is this checkout bound to a pipeline". The binding step of
// `asgard-cli gate` is what answers the second.
func isPipelineRepo(repoRoot string) bool {
	_, err := os.Stat(filepath.Join(repoRoot, pipelineconfig.FileName))
	return err == nil
}
