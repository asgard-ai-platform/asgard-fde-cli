package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/auth"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/selfupdate"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/version"
)

func newUpdateCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Replace this binary with the newest release",
		Long: `Replace this binary with the newest published release.

It takes the version-less release asset for this platform, verifies the download
against that release's own checksums by hash rather than by name, runs the new
binary where it landed, and only then renames it over this one.

Because of that order, a build macOS kills, a truncated download or an empty
archive all fail before anything has been replaced. The result is either the
new version or exactly what was there before, never a binary that does not run.
On macOS this is also when the Gatekeeper scan of a newly written unnotarized
binary happens; it can take a minute, and it happens here rather than on the
first run in front of a customer.

It refuses rather than guessing when this binary is not a release build,
when something else owns the file, or when the directory is not writable - and
each refusal names what to run instead. A package manager's copy is upgraded
with the package manager: replacing the file underneath it leaves its database
describing a version that is no longer there.

    asgard-cli update              take the newest release
    asgard-cli version --check     ask whether there is one, and change nothing

Every command that can reach a network already says when a newer release is
published, at most once every ` + selfupdate.IntervalText() + `. This is the command that acts on it.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()
			running := version.Get().Version

			target, err := selfPath()
			if err != nil {
				return err
			}
			selfupdate.SweepOld(target)

			if owner := notOursToReplace(target); owner != "" {
				return fmt.Errorf("%s was installed by %s, so this will not replace it.\n    %s",
					target, owner, upgradeWith(owner))
			}
			if why := notWritable(target); why != "" {
				return fmt.Errorf("%s, so nothing was changed.\n    %s", why, elevated())
			}
			if strings.ContainsAny(running, "-+") && !force {
				return fmt.Errorf("this is %s, which is not a release build, so there is nothing to\n"+
					"compare it against - taking the newest release could be a downgrade.\n"+
					"    asgard-cli update --force   do it anyway", running)
			}

			home, err := auth.Home()
			if err != nil {
				return err
			}
			res := selfupdate.Check(cmd.Context(), home, running, true)
			switch {
			case res.Latest == "":
				return fmt.Errorf("could not reach the release list, so nothing was changed")
			case !res.Newer && !force:
				fmt.Fprintf(out, "%s is the newest release, and it is what this is.\n", res.Latest)
				return nil
			}

			// A command that changes something says what it is changing before
			// it acts, and the thing being changed here is the file this
			// process is executing from.
			fmt.Fprintf(errOut, "replacing %s: %s -> %s\n", target, running, res.Latest)

			if err := selfupdate.Apply(cmd.Context(), res.Latest, target, out); err != nil {
				return err
			}
			fmt.Fprintf(out, "%s -> %s\n", running, res.Latest)
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false,
		"replace the binary even when this is not a release build or is not older than the newest release")

	return cmd
}

// selfPath is the file this process is executing from, with symlinks resolved.
//
// **Resolved, because the link is not what gets replaced.** A Homebrew install
// puts a symlink in its bin directory pointing into its own tree, and renaming
// over the link would leave the package manager's copy in place and its link
// gone - so the resolved path is both what this writes and what decides whether
// somebody else owns it.
func selfPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find this binary: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", exe, err)
	}
	return resolved, nil
}

// notOursToReplace names whoever installed this binary, or "" when nobody did.
//
// **A file inside somebody else's tree is theirs to replace.** Writing a new
// binary into a package manager's prefix leaves its database describing a
// version that is not there any more, and the next thing it does - an upgrade,
// a reinstall, a doctor - reports a repository that is broken rather than one
// that is current.
func notOursToReplace(target string) string {
	slash := filepath.ToSlash(target)
	switch {
	case strings.Contains(slash, "/Cellar/"), strings.Contains(slash, "/linuxbrew/"):
		return "Homebrew"
	case strings.Contains(slash, "/nix/store/"):
		return "Nix"
	}
	// **The distribution's directories, which the .deb and the .rpm install
	// into.** The filesystem standard reserves /usr/bin and /bin for the
	// package manager and /usr/local for locally installed software, so the
	// path is the answer and no package database has to be consulted for it.
	// Replacing a file dpkg or rpm records leaves that database naming a
	// version that is not there, which `dpkg -V` then reports as a damaged
	// package.
	if dir := filepath.ToSlash(filepath.Dir(slash)); distroDirs[dir] {
		return packageManager()
	}
	// `go install` writes here, and the module path is the way to move it.
	//
	// **Both sides are resolved before they are compared.** `selfPath` already
	// resolved the target, so comparing it against an unresolved GOPATH misses
	// every installation reached through a symlink - which on macOS is any
	// path under /var, because /var is itself one.
	for _, dir := range goBinDirs() {
		if dir == "" {
			continue
		}
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			dir = real
		}
		if strings.HasPrefix(slash, filepath.ToSlash(dir)+"/") {
			return "go install"
		}
	}
	return ""
}

// distroDirs are the directories a package manager owns.
//
// /usr/local/bin is deliberately absent: the standard reserves it for software
// installed outside the package manager, which is where `install.sh` puts this
// and is why an install made that way can replace itself.
var distroDirs = map[string]bool{
	"/usr/bin": true, "/bin": true, "/usr/sbin": true, "/sbin": true,
}

// packageManager names the one on this machine, by what is on PATH.
//
// **The tool is the question, not the distribution.** Naming Ubuntu would be
// wrong on Debian and on Mint and right by accident on both, where dpkg is the
// thing that actually records the file.
func packageManager() string {
	for _, tool := range []string{"dpkg", "rpm", "apk"} {
		if _, err := exec.LookPath(tool); err == nil {
			return tool
		}
	}
	return "your package manager"
}

func goBinDirs() []string {
	var out []string
	if v := os.Getenv("GOBIN"); v != "" {
		out = append(out, v)
	}
	if v := os.Getenv("GOPATH"); v != "" {
		for _, p := range filepath.SplitList(v) {
			out = append(out, filepath.Join(p, "bin"))
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(home, "go", "bin"))
	}
	return out
}

// upgradeWith is the command that moves an installation somebody else owns.
func upgradeWith(owner string) string {
	switch owner {
	case "Homebrew":
		return "brew upgrade asgard-cli"
	case "go install":
		return "go install github.com/asgard-ai-platform/asgard-fde-cli/cmd/asgard-cli@latest"
	case "Nix":
		return "update it the way the rest of your profile is updated"
	case "dpkg":
		return fmt.Sprintf("curl -fLO %s && sudo dpkg -i %s", packageURL("deb"), packageAsset("deb"))
	case "rpm":
		return fmt.Sprintf("sudo rpm -U %s", packageURL("rpm"))
	case "apk":
		return fmt.Sprintf("curl -fLO %s && sudo apk add --allow-untrusted %s", packageURL("apk"), packageAsset("apk"))
	case "your package manager":
		return "reinstall it with whatever installed it, or take the tarball into /usr/local/bin"
	}
	return installCommand()
}

// packageAsset and packageURL name this platform's Linux package in the newest
// release. The name carries no version, so the URL keeps working across
// releases - the same property the tarball download relies on.
func packageAsset(format string) string {
	return fmt.Sprintf("asgard-cli_%s_%s.%s", runtime.GOOS, runtime.GOARCH, format)
}

func packageURL(format string) string {
	return "https://github.com/asgard-ai-platform/asgard-fde-cli/releases/latest/download/" + packageAsset(format)
}

// notWritable says why the target cannot be replaced from here, or "".
//
// **Checked before anything is downloaded**, so an install that needs root
// gets a line telling somebody how to run it rather than a permission error at
// the end of a minute of work. It probes rather than reading the mode, because
// what decides this is the effective user against the directory, which a mode
// alone does not answer.
func notWritable(target string) string {
	dir := filepath.Dir(target)
	probe, err := os.CreateTemp(dir, ".asgard-cli-writable-")
	if err != nil {
		return fmt.Sprintf("%s is not writable by you", dir)
	}
	name := probe.Name()
	probe.Close()
	os.Remove(name)
	return ""
}

// elevated is how to run this command as somebody who can write there.
func elevated() string {
	if runtime.GOOS == "windows" {
		return "run asgard-cli update from an elevated prompt"
	}
	return "sudo asgard-cli update"
}
