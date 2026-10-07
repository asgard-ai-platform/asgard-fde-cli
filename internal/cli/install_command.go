package cli

import "runtime"

// installCommand is the one line that replaces this binary with the newest
// release.
//
// **It is here rather than written out at each site** so that the command the
// gate prints, the one `version --check` prints, and the one the README leads
// with cannot drift into three different answers - which is the failure this
// repository removes everywhere else.
func installCommand() string {
	// **Both Unixes, because the installer handles both.** It picks the asset
	// for the platform, verifies it the same way, and installs to
	// /usr/local/bin rather than to the package manager's directory - which is
	// what makes an install made this way one that `update` can replace.
	switch runtime.GOOS {
	case "darwin", "linux":
		return "curl -fsSL https://raw.githubusercontent.com/asgard-ai-platform/asgard-fde-cli/main/install.sh | sh"
	case "windows":
		return "irm https://raw.githubusercontent.com/asgard-ai-platform/asgard-fde-cli/main/install.ps1 | iex"
	}
	return "see https://github.com/asgard-ai-platform/asgard-fde-cli/releases/latest"
}

// upgradeCommand is what THIS installation should run to move to the newest
// release, which is not the same answer for everybody.
//
// **A message that names a command the reader cannot run is the same as no
// message.** `asgard-cli update` replaces the binary in place and is the answer
// for an install this tool made; a package manager's copy is that package
// manager's to move; a binary in a directory the user cannot write needs to be
// run as somebody who can; and when none of that can be worked out, the
// installer is what always works.
func upgradeCommand() string {
	target, err := selfPath()
	if err != nil {
		return installCommand()
	}
	if owner := notOursToReplace(target); owner != "" {
		return upgradeWith(owner)
	}
	if notWritable(target) != "" {
		return elevated()
	}
	return "asgard-cli update"
}
