package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/extension"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/skills"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/version"
)

// The commands that read a Stack's Extension index. They go through the
// platform, which relays its registry: the git credentials stay with the
// registry, so neither this machine nor a sandbox needs git access to an
// Extension's repository. None of them takes a workspace - an Extension belongs
// to the whole Stack.

func newExtensionSearchCmd() *cobra.Command {
	var (
		profile, usage, authFlag, category, formatOut string
	)
	cmd := &cobra.Command{
		Use:   "search [words...]",
		Short: "Find the Extensions this platform offers",
		Long: `Find the Extensions this platform offers.

    asgard-cli extension search                    every Extension
    asgard-cli extension search google drive
    asgard-cli extension search --usage personal-account --auth oauth-customer
    asgard-cli extension search --category erp --format json

The words match the id, name, summary, vendor, product or a tag, as one
case-insensitive phrase. Each Extension is described by its latest version:
its routes and whether each has been walked (verified) or not yet
(experimental). Unlisted Extensions are included; listing only decides what the
public website shows.

When the customer names an external system, search for it first. When there is
an Extension, ` + "`asgard-cli extension get <id>`" + ` brings it into the repository
and its SKILL.md is where to start; when there is none, the system is
connected with the platform material alone.

It needs a session and no repository.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFormat(formatOut); err != nil {
				return err
			}
			pc, err := resolveContext(cmd, contextOptions{Profile: profile})
			if err != nil {
				return err
			}
			entries, err := pc.Client.SearchExtensions(cmd.Context(), platform.ExtensionQuery{
				Query: strings.Join(args, " "), Usage: usage, Auth: authFlag, Category: category,
			})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if formatOut == formatJSON {
				if entries == nil {
					entries = []platform.ExtensionEntry{}
				}
				return writeJSON(out, entries)
			}
			if len(entries) == 0 {
				fmt.Fprintf(out, "no Extension on %s matches; connect the system with the platform material, or search with fewer words\n", pc.Session.Profile.Name)
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			for _, e := range entries {
				latest := ""
				if e.Latest != nil {
					latest = e.Latest.Version
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.Metadata.ID, latest, systemName(e.Metadata.System), e.Metadata.System.Category)
				if e.Metadata.Summary != "" {
					fmt.Fprintf(tw, "\t%s\n", e.Metadata.Summary)
				}
				if routes := routeList(e.Metadata.Routes); routes != "" {
					fmt.Fprintf(tw, "\t%s\n", routes)
				}
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			fmt.Fprintf(out, "\n%d Extension(s) on %s. `asgard-cli extension show <id>` for one in full, `asgard-cli extension get <id>` to bring it in.\n",
				len(entries), pc.Session.Profile.Name)
			return nil
		},
	}
	addProfileFlag(cmd, &profile)
	cmd.Flags().StringVar(&usage, "usage", "", "only Extensions with a route of this usage: build-time, shared-account or personal-account")
	cmd.Flags().StringVar(&authFlag, "auth", "", "only Extensions with a route of this auth, e.g. oauth-customer; with --usage, on the same route")
	cmd.Flags().StringVar(&category, "category", "", "only Extensions of this system category, e.g. erp or documents")
	cmd.Flags().StringVar(&formatOut, formatFlag, formatText, formatUsage)
	return cmd
}

func newExtensionShowCmd() *cobra.Command {
	var profile, formatOut string
	cmd := &cobra.Command{
		Use:   "show <id-or-path>[@<version>]",
		Short: "One Extension in full: its routes, brakes, requirements and versions",
		Long: `One Extension in full: its routes, brakes, requirements and versions.

    asgard-cli extension show asgard-ext-google-drive
    asgard-cli extension show asgard-ext-google-drive@v1.3.0-rc.1
    asgard-cli extension show github.com/asgard-ai-platform/asgard-ext-netsuite

Without a version it describes the latest one and lists every version: the ones
offered, the tags the registry refuses with the reason, and tags moved to
another commit after they were first seen (the first commit keeps being
served).

With a version it asks for that version, and the registry fetches the
Extension's repository on the spot when it has not seen the tag yet. That is
how an author confirms a tag was taken in: tag, push, then run this with the
new version. A refused version fails with the registry's reason.

It needs a session and no repository; inside one it also says which version
this repository holds.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFormat(formatOut); err != nil {
				return err
			}
			ext, ver, err := splitExtensionRef(args[0])
			if err != nil {
				return err
			}
			pc, err := resolveContext(cmd, contextOptions{Profile: profile})
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			var list *platform.ExtensionVersionList
			if ver == "" {
				if list, err = pc.Client.ExtensionVersions(ctx, ext); err != nil {
					return err
				}
				latest, err := pc.Client.LatestExtensionVersion(ctx, ext)
				if err != nil {
					if platform.NotFound(err) && len(list.Versions) == 0 {
						return showVersions(cmd, formatOut, list)
					}
					return err
				}
				ver = latest.Version
			}
			info, err := pc.Client.ExtensionVersionInfo(ctx, ext, ver)
			if err != nil {
				return err
			}
			if formatOut == formatJSON {
				payload := map[string]any{"version": info}
				if list != nil {
					payload["versions"] = list
				}
				return writeJSON(cmd.OutOrStdout(), payload)
			}
			printVersionInfo(cmd.OutOrStdout(), info)
			if list != nil {
				printVersionList(cmd.OutOrStdout(), list)
			}
			if held := heldVersion(info.Version.Path); held != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "\nthis repository holds %s\n", held)
			}
			return nil
		},
	}
	addProfileFlag(cmd, &profile)
	cmd.Flags().StringVar(&formatOut, formatFlag, formatText, formatUsage)
	return cmd
}

// showVersions answers for an Extension that offers no version at all: what is
// left to say is why.
func showVersions(cmd *cobra.Command, formatOut string, list *platform.ExtensionVersionList) error {
	if formatOut == formatJSON {
		return writeJSON(cmd.OutOrStdout(), map[string]any{"versions": list})
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%s offers no version\n", list.Path)
	printVersionList(out, list)
	return nil
}

func printVersionInfo(out io.Writer, info *platform.ExtensionVersionInfo) {
	m := info.Metadata
	v := info.Version
	pre := ""
	if v.Prerelease {
		pre = ", a pre-release"
	}
	fmt.Fprintf(out, "%s %s (commit %s%s)\n", v.Path, v.Version, short(v.Commit), pre)
	if info.Moved != nil {
		fmt.Fprintf(out, "  the tag now points at %s; %s is still served\n", short(info.Moved.CurrentCommit), short(info.Moved.FirstSeenCommit))
	}
	fmt.Fprintf(out, "%s - %s\n", m.Name, m.Summary)
	fmt.Fprintf(out, "  system   %s, category %s\n", systemName(m.System), m.System.Category)
	fmt.Fprintf(out, "  kind     %s, listing %s, source %s\n", m.Kind, m.Listing, m.Source)
	if len(m.Owners) > 0 {
		fmt.Fprintf(out, "  owners   %s\n", strings.Join(m.Owners, ", "))
	}
	fmt.Fprintln(out, "routes")
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	for _, r := range m.Routes {
		fmt.Fprintf(tw, "  %s\t%s", r.ID, r.Status)
		if len(r.Produces) > 0 {
			fmt.Fprintf(tw, "\tproduces %s", strings.Join(r.Produces, ", "))
		}
		if len(r.Brakes) > 0 {
			fmt.Fprintf(tw, "\tbrakes %s", strings.Join(r.Brakes, ", "))
		}
		fmt.Fprintln(tw)
	}
	_ = tw.Flush()
	if req := requiresLine(m.Requires); req != "" {
		fmt.Fprintf(out, "requires %s\n", req)
	}
	if info.LogoProblem != "" {
		fmt.Fprintf(out, "logo     none: %s\n", info.LogoProblem)
	}
}

func printVersionList(out io.Writer, list *platform.ExtensionVersionList) {
	var names []string
	for _, v := range list.Versions {
		if v.Prerelease {
			names = append(names, v.Version+" (pre-release)")
		} else {
			names = append(names, v.Version)
		}
	}
	if len(names) > 0 {
		fmt.Fprintf(out, "versions %s\n", strings.Join(names, ", "))
	}
	if len(list.RefusedVersions) > 0 {
		fmt.Fprintln(out, "refused, and not offered:")
		for _, r := range list.RefusedVersions {
			fmt.Fprintf(out, "  %s  %s\n", r.Version, r.Reason)
		}
	}
	if len(list.MovedTags) > 0 {
		fmt.Fprintln(out, "moved after they were first seen (the first commit is still served):")
		for _, m := range list.MovedTags {
			fmt.Fprintf(out, "  %s  first seen at %s, now at %s\n", m.Version, short(m.FirstSeenCommit), short(m.CurrentCommit))
		}
	}
}

// heldVersion is the version of path this repository holds, when the command
// runs inside one that holds it.
func heldVersion(fullPath string) string {
	root, err := fdeRepoRoot()
	if err != nil {
		return ""
	}
	lock, err := extension.ReadLock(root)
	if err != nil {
		return ""
	}
	if e, ok := lock.Extensions[fullPath]; ok {
		return e.Version
	}
	return ""
}

// fetchExtension takes version of ext from the platform into the repository
// at root. An empty version is the latest.
func fetchExtension(cmd *cobra.Command, root string, pc *platformContext, ext, ver string, force bool) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()
	if ver == "" {
		latest, err := pc.Client.LatestExtensionVersion(ctx, ext)
		if err != nil {
			return err
		}
		ver = latest.Version
		if latest.Prerelease {
			fmt.Fprintf(out, "%s has no release yet, so this is its latest pre-release, %s\n", ext, ver)
		}
	}

	var buf bytes.Buffer
	arc, err := pc.Client.DownloadExtension(ctx, ext, ver, &buf)
	if err != nil {
		return err
	}
	files, skipped, err := extension.FromArchive(&buf)
	if err != nil {
		return fmt.Errorf("%s %s: %w; nothing was written", ext, ver, err)
	}
	readme, ok := files["README.md"]
	if !ok {
		return fmt.Errorf("%s %s has no README.md, so there is nothing to tell which Extension it is; nothing was written", arc.Path, ver)
	}
	meta, _, err := extension.ParseMeta(readme.Data)
	if err != nil {
		return fmt.Errorf("%s %s: README.md: %w; nothing was written", arc.Path, ver, err)
	}
	id := path.Base(arc.Path)
	if !extension.ValidID(id) || meta.ID != id {
		return fmt.Errorf("%s %s says its id is %q; an Extension's id starts with %s and is the last segment of its path, so it cannot be placed. Nothing was written",
			arc.Path, ver, meta.ID, extension.Prefix)
	}

	lock, err := extension.ReadLock(root)
	if err != nil {
		return err
	}
	key := arc.Path
	if existing, ok := lock.Find(id); ok && existing != key {
		if lock.Extensions[existing].Source == extension.SourceLocal {
			return fmt.Errorf("%s already holds %s as a local copy (source: local); this one would take the same directory. Nothing was written", extension.LockName, id)
		}
		return fmt.Errorf("%s already holds %s, and %s would take the same directory. Nothing was written", extension.LockName, existing, key)
	}
	skillsRoot := skills.Root(root, "")
	dst := filepath.Join(skillsRoot, id)
	previous := lock.Extensions[key]

	conflicts, err := extension.Conflicts(dst, previous.Files)
	if err != nil {
		return err
	}
	if len(conflicts) > 0 && !force {
		for _, c := range conflicts {
			fmt.Fprintf(cmd.ErrOrStderr(), "- %s: %s\n", c.Path, c.Reason)
		}
		return fmt.Errorf("%s holds files this would replace; nothing was written. Move the edits out first, or --force replaces them", rel(root, dst))
	}

	digests, err := extension.Replace(dst, files)
	if err != nil {
		return err
	}
	entry := previous
	entry.Source, entry.Path, entry.Dirty = "", "", false
	entry.Version = arc.Version
	if entry.Version == "" {
		entry.Version = ver
	}
	entry.Commit = arc.Commit
	entry.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	entry.Files = digests
	lock.Extensions[key] = entry
	if err := lock.Write(root); err != nil {
		return err
	}

	was := ""
	if previous.Version != "" && previous.Version != entry.Version {
		was = ", was " + previous.Version
	}
	fmt.Fprintf(out, "fetched %s %s (%d files%s) into %s\n", id, entry.Version, len(digests), was, rel(root, dst))
	fmt.Fprintf(out, "  %s at %s, from %s\n", key, short(entry.Commit), pc.Session.Profile.Name)
	if len(skipped) > 0 {
		fmt.Fprintf(out, "  left out of the archive, not something this asgard-cli copies: %s\n", strings.Join(skipped, ", "))
	}
	fmt.Fprintf(out, "recorded in %s; commit both\n", extension.LockName)
	if warnings := extension.CheckRequires(meta.Requires, skillsRoot, lock, version.Get().Version); len(warnings) > 0 {
		fmt.Fprintln(out, "\nrequires, which only warns:")
		for _, w := range warnings {
			fmt.Fprintf(out, "- %s\n", w)
		}
	}
	fmt.Fprintf(out, "\nNext: read %s, which starts with which route to take.\n", rel(root, filepath.Join(dst, "SKILL.md")))
	return nil
}

// updateFromPlatform moves a fetched Extension to its latest version.
func updateFromPlatform(cmd *cobra.Command, root, key string, entry extension.Entry, profile string, force bool) error {
	pc, err := resolveContext(cmd, contextOptions{Profile: profile})
	if err != nil {
		return err
	}
	latest, err := pc.Client.LatestExtensionVersion(cmd.Context(), key)
	if err != nil {
		return err
	}
	if latest.Version == entry.Version {
		fmt.Fprintf(cmd.OutOrStdout(), "%s is at %s, the latest on %s; nothing to do\n", key, entry.Version, pc.Session.Profile.Name)
		return nil
	}
	if !extension.Newer(latest.Version, entry.Version) {
		fmt.Fprintf(cmd.OutOrStdout(), "%s is at %s and the latest on %s is %s, which is not newer; nothing changed. `asgard-cli extension get %s@%s` takes it anyway\n",
			key, entry.Version, pc.Session.Profile.Name, latest.Version, key, latest.Version)
		return nil
	}
	return fetchExtension(cmd, root, pc, key, latest.Version, force)
}

func newExtensionStatusCmd() *cobra.Command {
	var (
		profile, formatOut string
		offline            bool
	)
	cmd := &cobra.Command{
		Use:   "status",
		Short: "The Extensions this repository holds, and whether they are current",
		Long: `The Extensions this repository holds, and whether they are current.

    asgard-cli extension status
    asgard-cli extension status --offline

For each Extension ` + "`" + extension.LockName + "`" + ` records: the version or local path it
came from, the routes this repository recorded, and any file in its directory
edited since it was written, missing, or not written by asgard-cli. An
` + "`asgard-ext-*`" + ` directory the record does not know is listed too.

It also asks the platform for each fetched Extension's latest version. Without
a session, or with --offline, that part is left out and says so; everything
else needs nothing but the repository. ` + "`asgard-cli gate`" + ` checks the same
things as its extensions step.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkFormat(formatOut); err != nil {
				return err
			}
			root, err := fdeRepoRoot()
			if err != nil {
				return err
			}
			lock, err := extension.ReadLock(root)
			if err != nil {
				return err
			}
			states, unrecorded, err := extension.Inspect(skills.Root(root, ""), lock)
			if err != nil {
				return err
			}
			latest, asked := latestVersions(cmd, profile, offline, states)

			if formatOut == formatJSON {
				rows := make([]map[string]any, 0, len(states))
				for _, s := range states {
					row := map[string]any{
						"key": s.Key, "id": s.ID, "source": sourceOf(s.Entry), "version": s.Entry.Version,
						"commit": s.Entry.Commit, "routes": nonNilStrings(s.Entry.Routes), "absent": s.Absent,
						"changed": s.Changed, "missing": nonNilStrings(s.Missing),
					}
					if v, ok := latest[s.Key]; ok {
						row["latest"] = v
					}
					rows = append(rows, row)
				}
				return writeJSON(cmd.OutOrStdout(), map[string]any{
					"extensions": rows, "unrecorded": nonNilStrings(unrecorded), "latest_asked": asked,
				})
			}

			out := cmd.OutOrStdout()
			if len(states) == 0 && len(unrecorded) == 0 {
				fmt.Fprintln(out, "no Extension here. `asgard-cli extension search <system>` finds one")
				return nil
			}
			for _, s := range states {
				fmt.Fprintf(out, "%s  %s\n", s.ID, describeEntry(s, latest))
				for _, line := range stateProblems(root, s) {
					fmt.Fprintf(out, "  - %s\n", line)
				}
			}
			for _, d := range unrecorded {
				fmt.Fprintf(out, "%s  not in %s: commit it with an entry (source: local), or rename a skill of this repository's own - the prefix is for Extensions\n", d, extension.LockName)
			}
			if asked != "" {
				fmt.Fprintf(out, "\nlatest versions: %s\n", asked)
			}
			return nil
		},
	}
	addProfileFlag(cmd, &profile)
	cmd.Flags().BoolVar(&offline, "offline", false, "do not ask the platform for the latest versions")
	cmd.Flags().StringVar(&formatOut, formatFlag, formatText, formatUsage)
	return cmd
}

// latestVersions asks the platform for the latest version of every fetched
// Extension. asked says what was asked, or why nothing was.
func latestVersions(cmd *cobra.Command, profile string, offline bool, states []extension.State) (map[string]string, string) {
	latest := map[string]string{}
	var fetched []extension.State
	for _, s := range states {
		if s.Entry.Source != extension.SourceLocal {
			fetched = append(fetched, s)
		}
	}
	if len(fetched) == 0 {
		return latest, ""
	}
	if offline {
		return latest, "not asked (--offline)"
	}
	pc, err := resolveContext(cmd, contextOptions{Profile: profile})
	if err != nil {
		return latest, "not asked: no session (`asgard-cli login`)"
	}
	var failed []string
	for _, s := range fetched {
		v, err := pc.Client.LatestExtensionVersion(cmd.Context(), s.Key)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", s.ID, err))
			continue
		}
		latest[s.Key] = v.Version
	}
	asked := "asked " + pc.Session.Profile.Name
	if len(failed) > 0 {
		asked += "; no answer for " + strings.Join(failed, ", ")
	}
	return latest, asked
}

func describeEntry(s extension.State, latest map[string]string) string {
	if s.Entry.Source == extension.SourceLocal {
		if s.Entry.Path != "" {
			return "local, copied from " + s.Entry.Path
		}
		return "local, committed here"
	}
	d := s.Entry.Version
	if v, ok := latest[s.Key]; ok {
		if extension.Newer(v, s.Entry.Version) {
			d += fmt.Sprintf(", %s is out: `asgard-cli extension update %s`", v, s.ID)
		} else {
			d += " (latest)"
		}
	}
	if len(s.Entry.Routes) > 0 {
		d += ", routes " + strings.Join(s.Entry.Routes, ", ")
	}
	return d
}

// stateProblems is what is wrong with one Extension's directory, a line each.
func stateProblems(root string, s extension.State) []string {
	if s.Absent {
		return []string{fmt.Sprintf("%s is not there; `asgard-cli extension get %s` brings it back", rel(root, s.Dir), s.Key)}
	}
	var out []string
	for _, c := range s.Changed {
		out = append(out, fmt.Sprintf("%s: %s", c.Path, c.Reason))
	}
	for _, m := range s.Missing {
		out = append(out, m+": deleted since it was written")
	}
	return out
}

func sourceOf(e extension.Entry) string {
	if e.Source == extension.SourceLocal {
		return extension.SourceLocal
	}
	return "platform"
}

func newExtensionRecordCmd() *cobra.Command {
	var routes []string
	cmd := &cobra.Command{
		Use:   "record <id> --route <route>",
		Short: "Record which of an Extension's routes this repository took",
		Long: `Record which of an Extension's routes this repository took.

    asgard-cli extension record asgard-ext-intercom --route personal-account.oauth-customer

It adds the routes to the Extension's entry in ` + "`" + extension.LockName + "`" + `, so whoever
updates the Extension later knows which route documents to read again. A route
must be one the Extension declares. Recording one twice changes nothing.

It writes only to this repository and reports nothing to anybody: whether a
route has been walked is the Extension author's to track.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(routes) == 0 {
				return errors.New("--route is required: which route this repository took")
			}
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
				return fmt.Errorf("%s records no Extension %q; `asgard-cli extension status` lists the ones it does", extension.LockName, args[0])
			}
			id := path.Base(key)
			meta, _, err := extension.ReadMeta(filepath.Join(skills.Root(root, ""), id, "README.md"))
			if err != nil {
				return fmt.Errorf("read %s's README.md to know its routes: %w", id, err)
			}
			var declared []string
			for _, r := range meta.Routes {
				declared = append(declared, r.ID)
			}
			entry := lock.Extensions[key]
			for _, r := range routes {
				if !slices.Contains(declared, r) {
					return fmt.Errorf("%s declares no route %q; it declares %s", id, r, strings.Join(declared, ", "))
				}
				if !slices.Contains(entry.Routes, r) {
					entry.Routes = append(entry.Routes, r)
				}
			}
			lock.Extensions[key] = entry
			if err := lock.Write(root); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "recorded for %s in %s: %s\n", id, extension.LockName, strings.Join(entry.Routes, ", "))
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&routes, "route", nil, "a route this repository took, as <usage>.<auth>; required, and may be repeated")
	return cmd
}

// splitExtensionRef reads "<id-or-path>[@<version>]".
func splitExtensionRef(arg string) (ext, ver string, err error) {
	ext, ver, found := strings.Cut(arg, "@")
	if found && !strings.HasPrefix(ver, "v") {
		return "", "", fmt.Errorf("%q: a version is a git tag such as v1.2.0", arg)
	}
	if ext == "" {
		return "", "", fmt.Errorf("%q names no Extension", arg)
	}
	return strings.Trim(ext, "/"), ver, nil
}

func systemName(s platform.ExtensionSystem) string {
	switch {
	case s.Product != "" && s.Vendor != "" && !strings.Contains(s.Product, s.Vendor):
		return s.Product + " (" + s.Vendor + ")"
	case s.Product != "":
		return s.Product
	}
	return s.Vendor
}

// routeList is the routes on one line, with how far each has been walked.
func routeList(routes []platform.ExtensionRoute) string {
	parts := make([]string, 0, len(routes))
	for _, r := range routes {
		parts = append(parts, r.ID+" "+r.Status)
	}
	return strings.Join(parts, ", ")
}

func requiresLine(r platform.ExtensionRequires) string {
	var parts []string
	if r.AsgardCLI != "" {
		parts = append(parts, "asgard-cli "+r.AsgardCLI)
	}
	if len(r.Skills) > 0 {
		parts = append(parts, "skills "+strings.Join(r.Skills, ", "))
	}
	if len(r.Extensions) > 0 {
		parts = append(parts, "Extensions "+strings.Join(r.Extensions, ", "))
	}
	if len(r.Platform) > 0 {
		parts = append(parts, "platform "+strings.Join(r.Platform, ", "))
	}
	return strings.Join(parts, "; ")
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
