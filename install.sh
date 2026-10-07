#!/bin/sh
# One-command install for macOS and Linux.
#
#   curl -fsSL https://raw.githubusercontent.com/asgard-ai-platform/asgard-fde-cli/main/install.sh | sh
#
# **It verifies what it downloaded and it warms the first run.** Neither is
# decoration. The binaries are ad-hoc signed and not notarized, so macOS scans
# the first execution of a newly written one - which takes anything from no time
# to minutes, and has been seen to kill it outright. Getting that over with here
# means the customer's first real use is not the one that stalls, and a failure
# happens while they are still looking at an installer rather than in a meeting.
#
# **It installs to /usr/local/bin on purpose, including on Linux.** The .deb and
# the .rpm put the binary in /usr/bin, which is the package manager's, and a
# binary there cannot replace itself - `asgard-cli update` refuses rather than
# leaving dpkg describing a version that is not on disk. /usr/local is what the
# filesystem standard reserves for software installed outside the package
# manager, so an install made here is one that can update itself afterwards.
set -eu

repo=asgard-ai-platform/asgard-fde-cli
base="https://github.com/$repo/releases/latest/download"

os=$(uname -s)
case "$os" in
Darwin)
	# One binary, both architectures: nothing for the person installing to know
	# about Intel against Apple silicon.
	asset=asgard-cli_darwin_all.tar.gz
	;;
Linux)
	case "$(uname -m)" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*)
		echo "No release is built for $(uname -m). The ones there are:" >&2
		echo "  https://github.com/$repo/releases/latest" >&2
		exit 1
		;;
	esac
	asset="asgard-cli_linux_${arch}.tar.gz"
	;;
*)
	echo "This installer is for macOS and Linux. For anything else take an asset directly:" >&2
	echo "  https://github.com/$repo/releases/latest" >&2
	exit 1
	;;
esac

# **The checksum tool is not the same on both**, and this is the whole of the
# difference: coreutils has sha256sum, macOS ships shasum. Neither is optional -
# a script piped into a shell is already a trust decision, so what it fetches is
# verified before it is run.
if command -v sha256sum >/dev/null 2>&1; then
	sha256() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
	sha256() { shasum -a 256 "$1" | awk '{print $1}'; }
else
	echo "Neither sha256sum nor shasum is here, so the download cannot be verified." >&2
	echo "Nothing was installed." >&2
	exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

printf 'Downloading the latest asgard-cli...\n'
curl -fsSL --retry 3 -o "$tmp/$asset" "$base/$asset"
curl -fsSL --retry 3 -o "$tmp/checksums.txt" "$base/checksums.txt"

# **Matched by hash rather than by name.** The release carries the same bytes
# twice - once with a version in the filename and once without, so that the URL
# above needs no version - and only the versioned name is in checksums.txt.
got=$(sha256 "$tmp/$asset")
grep -q "^$got  " "$tmp/checksums.txt" || {
	echo "Checksum mismatch: $got is not in this release's checksums.txt." >&2
	echo "Nothing was installed. Report this - it should not happen." >&2
	exit 1
}
printf '  checksum ok\n'

tar xzf "$tmp/$asset" -C "$tmp" asgard-cli
# curl does not mark a download the way a browser does, but a file that reached
# this machine some other way might be, and the mark is what gets it killed.
# macOS only; there is no such mark on Linux and no xattr command to remove it.
[ "$os" = Darwin ] && { xattr -d com.apple.quarantine "$tmp/asgard-cli" 2>/dev/null || true; }

dir=/usr/local/bin
if [ -w "$dir" ]; then
	install -m 0755 "$tmp/asgard-cli" "$dir/asgard-cli"
elif sudo -n true 2>/dev/null || [ -t 1 ]; then
	printf 'Installing to %s (needs sudo)\n' "$dir"
	sudo install -d -m 0755 "$dir"
	sudo install -m 0755 "$tmp/asgard-cli" "$dir/asgard-cli"
else
	# **No terminal to ask on**, which is what happens inside some CI and some
	# terminals when this is piped. Installing somewhere the user owns beats
	# failing, as long as it says so.
	dir="$HOME/.local/bin"
	install -d -m 0755 "$dir"
	install -m 0755 "$tmp/asgard-cli" "$dir/asgard-cli"
	printf '\nInstalled to %s, which may not be on your PATH. Add it:\n' "$dir"
	printf '  echo '\''export PATH="$HOME/.local/bin:$PATH"'\'' >> ~/.zshrc\n'
fi

if [ "$os" = Darwin ]; then
	printf 'Checking it runs (macOS scans a new binary once, which can take a minute)...\n'
else
	printf 'Checking it runs...\n'
fi
"$dir/asgard-cli" version

printf '\n'
"$dir/asgard-cli" doctor || true
