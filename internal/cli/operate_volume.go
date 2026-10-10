package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

// The group a kind's file commands are listed under.
const operateGroupFiles = "files"

// volumeKind is what differs between a SourceSet's files and a SkillSet's.
type volumeKind struct {
	// Kind is the CR kind, as a live manifest names it.
	Kind string
	// Use is the command name the kind has under `operate`.
	Use string
	// volume names the files of one CR.
	volume func(name string) platform.Volume
	// Perm is the permission every file route of the kind takes.
	Perm string
	// Overwritten says what replaces files written by hand, for the help.
	Overwritten string
}

var (
	sourceSetFiles = volumeKind{
		Kind: "SourceSet", Use: "source-set", volume: platform.SourceSetVolume, Perm: "source-set/put",
		Overwritten: `A Syncer owns its destinationPath. A git Syncer empties it before every copy,
and the file-store classes mirror their source, so a file put under a Syncer's
destination is gone after its next run. Put files a person hands over in a
folder no Syncer writes.`,
	}
	skillSetFiles = volumeKind{
		Kind: "SkillSet", Use: "skill-set", volume: platform.SkillSetVolume, Perm: "skill-set/put",
		Overwritten: `A SkillSet filled by a Syncer has its whole tree rebuilt on every run, so a file
written here by hand lasts until the next sync. Change the repository the
Syncer reads instead; this is for looking, and for a fix that cannot wait.`,
	}
)

// addVolumeCmds puts the file commands under a kind's command, in a group of
// their own.
func addVolumeCmds(parent *cobra.Command, k volumeKind) {
	parent.AddGroup(&cobra.Group{ID: operateGroupFiles, Title: "Files - the volume, as the Console's Files tab shows it:"})
	addTo(parent, operateGroupFiles,
		newVolumeLsCmd(k), newVolumeStatCmd(k), newVolumeCatCmd(k), newVolumePutCmd(k),
		newVolumeMkdirCmd(k), newVolumeRmCmd(k), newVolumeMvCmd(k, false), newVolumeMvCmd(k, true),
	)
}

// volumeFlags are the flags every file command carries.
type volumeFlags struct{ operateFlags }

// resolveVolume finds the scope and checks the CR, and returns the volume.
func (f *volumeFlags) resolveVolume(cmd *cobra.Command, k volumeKind, name string) (*platformContext, *operateScope, platform.Volume, error) {
	pc, scope, err := f.resolve(cmd)
	if err != nil {
		return nil, nil, platform.Volume{}, err
	}
	if _, err := scope.object(k.Kind, name); err != nil {
		return nil, nil, platform.Volume{}, err
	}
	return pc, scope, k.volume(name), nil
}

// volumePath turns what somebody typed into a path the platform takes: a
// leading slash and a trailing one are dropped, so "/docs/" and "docs" are
// the same folder. The root is "" and only `ls` accepts it.
func volumePath(p string, rootOK bool) (string, error) {
	clean := strings.Trim(p, "/")
	if clean == "" {
		if rootOK {
			return "", nil
		}
		return "", fmt.Errorf("a path inside the volume is needed here; the root cannot be named")
	}
	for _, part := range strings.Split(clean, "/") {
		switch part {
		case "", ".", "..":
			return "", fmt.Errorf("%q is not a volume path: it is relative to the volume's root, with no empty, '.' or '..' segment", p)
		}
	}
	return clean, nil
}

func volumeLong(k volumeKind, what, usage, rest string) string {
	return what + "\n\n    " + usage + "\n\n" + rest + "\n\nNeeds " + k.Perm + " in the project; every file route takes it, reading included."
}

func newVolumeLsCmd(k volumeKind) *cobra.Command {
	var f volumeFlags
	cmd := &cobra.Command{
		Use:   "ls <" + k.Use + "> [path]",
		Short: "List a folder of the " + k.Kind + "'s files",
		Long: volumeLong(k, "List a folder of the "+k.Kind+"'s files, folders first.",
			"asgard-cli operate "+k.Use+" ls <name> docs/ --release <r>",
			`Each line is the type (d for a folder), the size, when it last changed and the
name. With no path it lists the root. A path is relative to the volume's root;
a leading or trailing slash is dropped. --format json prints every entry as the
platform gives it.`),
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) == 2 {
				path = args[1]
			}
			p, err := volumePath(path, true)
			if err != nil {
				return err
			}
			pc, scope, v, err := f.resolveVolume(cmd, k, args[0])
			if err != nil {
				return err
			}
			entries, err := pc.Client.ListVolume(cmd.Context(), scope.ProjectID, v, p)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if f.format == formatJSON {
				if entries == nil {
					entries = []platform.VolumeEntry{}
				}
				return writeJSON(out, entries)
			}
			sort.SliceStable(entries, func(i, j int) bool {
				if entries[i].IsDir != entries[j].IsDir {
					return entries[i].IsDir
				}
				return entries[i].Name < entries[j].Name
			})
			if len(entries) == 0 {
				fmt.Fprintf(out, "%s is empty.\n", rootOr(p))
				return nil
			}
			for _, e := range entries {
				typ, size, name := "-", strconv.FormatInt(e.SizeBytes, 10), e.Name
				if e.IsDir {
					typ, size, name = "d", "-", e.Name+"/"
				}
				fmt.Fprintf(out, "%s %12s  %s  %s\n", typ, size, time.Unix(e.MtimeUnix, 0).Local().Format("2006-01-02 15:04"), name)
			}
			return nil
		},
	}
	f.register(cmd)
	return cmd
}

func rootOr(p string) string {
	if p == "" {
		return "the root"
	}
	return p
}

func newVolumeStatCmd(k volumeKind) *cobra.Command {
	var f volumeFlags
	cmd := &cobra.Command{
		Use:   "stat <" + k.Use + "> <path>",
		Short: "Say whether a path exists in the " + k.Kind + "'s files, and what it is",
		Long: volumeLong(k, "Say whether a path exists in the "+k.Kind+"'s files, and what it is.",
			"asgard-cli operate "+k.Use+" stat <name> docs/faq.md --release <r>",
			`It exits zero when the path exists and non-zero when it does not, so it can
gate a step. --format json prints the platform's answer, which says exists:
false rather than failing.`),
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := volumePath(args[1], false)
			if err != nil {
				return err
			}
			pc, scope, v, err := f.resolveVolume(cmd, k, args[0])
			if err != nil {
				return err
			}
			st, err := pc.Client.StatVolume(cmd.Context(), scope.ProjectID, v, p)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if f.format == formatJSON {
				if err := writeJSON(out, st); err != nil {
					return err
				}
				if !st.Exists {
					return ErrSilent
				}
				return nil
			}
			if !st.Exists {
				return fmt.Errorf("%s does not exist in %s %s", p, k.Kind, args[0])
			}
			typ := "file"
			if st.IsDir {
				typ = "folder"
			}
			fmt.Fprintf(out, "%-8s %s\n%-8s %s\n%-8s %d\n%-8s %s\n%-8s %o\n", "path", p, "type", typ, "bytes", st.SizeBytes,
				"changed", time.Unix(st.MtimeUnix, 0).Local().Format("2006-01-02 15:04:05"), "mode", st.Mode)
			return nil
		},
	}
	f.register(cmd)
	return cmd
}

func newVolumeCatCmd(k volumeKind) *cobra.Command {
	var (
		f             volumeFlags
		offset, limit int64
		output        string
	)
	cmd := &cobra.Command{
		Use:   "cat <" + k.Use + "> <path>",
		Short: "Print a file of the " + k.Kind + ", or save it",
		Long: volumeLong(k, "Print a file of the "+k.Kind+" to standard output, or save it with -o.",
			"asgard-cli operate "+k.Use+" cat <name> docs/faq.md --release <r> --limit 4096",
			`The bytes are written as they are, so a binary file is better saved with -o
than printed. --offset and --limit read part of a file; when bytes remain past
what was read, a line on stderr says how many the file has.`),
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if offset < 0 || limit < 0 {
				return fmt.Errorf("--offset and --limit cannot be negative")
			}
			p, err := volumePath(args[1], false)
			if err != nil {
				return err
			}
			pc, scope, v, err := f.resolveVolume(cmd, k, args[0])
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			var file *os.File
			if output != "" {
				// Written beside the target and renamed over it, so a failed
				// read leaves no half file under the name asked for.
				file, err = os.CreateTemp(filepath.Dir(output), ".asgard-cat-*")
				if err != nil {
					return err
				}
				defer os.Remove(file.Name())
				w = file
			}
			total, truncated, err := pc.Client.ReadVolumeFile(cmd.Context(), scope.ProjectID, v, p, offset, limit, w)
			if file != nil {
				if cerr := file.Close(); err == nil {
					err = cerr
				}
				if err == nil {
					err = os.Rename(file.Name(), output)
				}
			}
			if err != nil {
				return err
			}
			if truncated {
				fmt.Fprintf(cmd.ErrOrStderr(), "read part of %s, which has %d bytes; --offset reads on from where this stopped\n", p, total)
			}
			if output != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "saved %s to %s\n", p, output)
			}
			return nil
		},
	}
	f.register(cmd)
	cmd.Flags().Int64Var(&offset, "offset", 0, "start this many bytes into the file; 0, the default, is the start")
	cmd.Flags().Int64Var(&limit, "limit", 0, "read at most this many bytes; 0, the default, reads to the end")
	cmd.Flags().StringVarP(&output, "output", "o", "", "save to this local file instead of printing; it is replaced if it exists")
	return cmd
}

func newVolumePutCmd(k volumeKind) *cobra.Command {
	var (
		f          volumeFlags
		createOnly bool
		mode       string
	)
	cmd := &cobra.Command{
		Use:   "put <" + k.Use + "> <local-file> <path>",
		Short: "Upload a local file into the " + k.Kind + "'s files",
		Long: volumeLong(k, "Upload a local file into the "+k.Kind+"'s files.",
			"asgard-cli operate "+k.Use+" put <name> ./faq.pdf manual/faq.pdf --release <r>",
			`The path is the file's whole path in the volume, name included, and the
folders above it are created. A file already there is replaced unless
--create-only, which refuses it. "-" as the local file reads standard input.

`+k.Overwritten),
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := volumePath(args[2], false)
			if err != nil {
				return err
			}
			var perm uint64
			if mode != "" {
				if perm, err = strconv.ParseUint(mode, 8, 32); err != nil || perm > 0o777 {
					return fmt.Errorf("--mode is octal permission bits such as 644; got %q", mode)
				}
			}
			var r io.Reader = cmd.InOrStdin()
			if args[1] != "-" {
				file, err := os.Open(args[1])
				if err != nil {
					return err
				}
				defer file.Close()
				r = file
			}
			pc, scope, v, err := f.resolveVolume(cmd, k, args[0])
			if err != nil {
				return err
			}
			actingOn(cmd, pc.Session)
			n, err := pc.Client.WriteVolumeFile(cmd.Context(), scope.ProjectID, v, p, filepath.Base(p), r,
				platform.VolumeWrite{Mode: uint32(perm), CreateOnly: createOnly})
			if err != nil {
				if platform.Conflict(err) {
					return fmt.Errorf("%w\n%s is already there and --create-only keeps it; without the flag it is replaced", err, p)
				}
				return err
			}
			if f.format == formatJSON {
				return writeJSON(cmd.OutOrStdout(), map[string]any{"path": p, "bytes_written": n})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Wrote %d bytes to %s in %s %s.\n", n, p, k.Kind, args[0])
			return nil
		},
	}
	f.register(cmd)
	cmd.Flags().BoolVar(&createOnly, "create-only", false, "refuse when the path already exists, instead of replacing it")
	cmd.Flags().StringVar(&mode, "mode", "", "permission bits in octal, e.g. 644; the platform's default when not given")
	return cmd
}

func newVolumeMkdirCmd(k volumeKind) *cobra.Command {
	var f volumeFlags
	cmd := &cobra.Command{
		Use:   "mkdir <" + k.Use + "> <path>",
		Short: "Create a folder in the " + k.Kind + "'s files",
		Long: volumeLong(k, "Create a folder in the "+k.Kind+"'s files, and the folders above it.",
			"asgard-cli operate "+k.Use+" mkdir <name> manual/2026 --release <r>", "A folder that already exists is not an error."),
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := volumePath(args[1], false)
			if err != nil {
				return err
			}
			pc, scope, v, err := f.resolveVolume(cmd, k, args[0])
			if err != nil {
				return err
			}
			actingOn(cmd, pc.Session)
			if err := pc.Client.MakeVolumeDir(cmd.Context(), scope.ProjectID, v, p); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created %s/ in %s %s.\n", p, k.Kind, args[0])
			return nil
		},
	}
	f.register(cmd)
	return cmd
}

func newVolumeRmCmd(k volumeKind) *cobra.Command {
	var (
		f         volumeFlags
		recursive bool
		yes       bool
	)
	cmd := &cobra.Command{
		Use:   "rm <" + k.Use + "> <path>",
		Short: "Delete a file or a folder from the " + k.Kind + "'s files",
		Long: volumeLong(k, "Delete a file or an empty folder from the "+k.Kind+"'s files; -r deletes a folder\nand everything in it.",
			"asgard-cli operate "+k.Use+" rm <name> manual/old.pdf --release <r>",
			`Nothing is kept: a deleted file is gone. -r asks first; --yes answers, and with
no terminal and no --yes it refuses. The root cannot be deleted.`),
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := volumePath(args[1], false)
			if err != nil {
				return err
			}
			pc, scope, v, err := f.resolveVolume(cmd, k, args[0])
			if err != nil {
				return err
			}
			if recursive {
				if err := confirmChange(cmd, yes, fmt.Sprintf("This deletes %s/ and everything in it from %s %s, in %s.", p, k.Kind, args[0], scope.Where)); err != nil {
					return err
				}
			}
			actingOn(cmd, pc.Session)
			if err := pc.Client.RemoveVolumePath(cmd.Context(), scope.ProjectID, v, p, recursive); err != nil {
				if platform.BadRequest(err) && !recursive {
					return fmt.Errorf("%w\nA folder that is not empty is deleted with -r", err)
				}
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted %s from %s %s.\n", p, k.Kind, args[0])
			return nil
		},
	}
	f.register(cmd)
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "delete a folder and everything in it")
	cmd.Flags().BoolVar(&yes, "yes", false, "do not ask before -r; required when there is no terminal to ask on")
	return cmd
}

// newVolumeMvCmd is `mv`, or `cp` when copy is set: the two take the same
// arguments and the same --overwrite.
func newVolumeMvCmd(k volumeKind, copy bool) *cobra.Command {
	var (
		f         volumeFlags
		overwrite bool
	)
	use, verb, long := "mv", "Move or rename", "Move or rename a file or a folder within the "+k.Kind+"'s files."
	if copy {
		use, verb, long = "cp", "Copy", "Copy a file or a folder within the "+k.Kind+"'s files."
	}
	cmd := &cobra.Command{
		Use:   use + " <" + k.Use + "> <from> <to>",
		Short: verb + " a file or a folder within the " + k.Kind + "'s files",
		Long: volumeLong(k, long,
			"asgard-cli operate "+k.Use+" "+use+" <name> drafts/faq.md manual/faq.md --release <r>",
			`Both paths are in the volume; "put" and "cat" are the way in and out of it.
<to> is the whole new path, not a folder to put it in. A destination that
already exists is refused unless --overwrite.`),
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			src, err := volumePath(args[1], false)
			if err != nil {
				return err
			}
			dst, err := volumePath(args[2], false)
			if err != nil {
				return err
			}
			pc, scope, v, err := f.resolveVolume(cmd, k, args[0])
			if err != nil {
				return err
			}
			actingOn(cmd, pc.Session)
			defer func() {
				if platform.Conflict(err) {
					err = fmt.Errorf("%w\n%s is already there; --overwrite replaces it", err, dst)
				}
			}()
			if copy {
				n, err := pc.Client.CopyVolumePath(cmd.Context(), scope.ProjectID, v, src, dst, overwrite)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Copied %s to %s (%d bytes).\n", src, dst, n)
				return nil
			}
			if err := pc.Client.MoveVolumePath(cmd.Context(), scope.ProjectID, v, src, dst, overwrite); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Moved %s to %s.\n", src, dst)
			return nil
		},
	}
	f.register(cmd)
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "replace the destination when it exists")
	return cmd
}

// confirmChange asks before something that cannot be undone. yes answers;
// with no terminal and no yes it refuses, so a script has to say it is meant.
func confirmChange(cmd *cobra.Command, yes bool, what string) error {
	if yes {
		return nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("%s\nThere is no terminal to confirm on; pass --yes to say it is meant", what)
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%s\n\n", what)
	ok, err := confirm(bufio.NewReader(os.Stdin), out, "Type y to go ahead:", false)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("nothing was changed")
	}
	return nil
}
