package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/selfupdate"
)

// The background check asks on an ordinary command and stays out of the way of
// the runs where an answer is somebody else's or nobody's, and of the commands
// that touch no network.
//
// **`version` is the one that matters.** It carries `--check`, which asks the
// same question with its own wording, and a second answer printed underneath
// that one reads as a second question rather than as the same one twice.
func TestWhichRunsAskWhetherANewerReleaseExists(t *testing.T) {
	root := NewRootCmd()

	// cobra adds `help` and `completion` on the first run rather than when the
	// tree is built, so they are asked for by name here.
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()

	find := func(path ...string) *cobra.Command {
		c := root
		for _, name := range path {
			next, _, err := c.Find([]string{name})
			if err != nil || next == c {
				t.Fatalf("no command %q under %q", name, c.Name())
			}
			c = next
		}
		return c
	}

	cases := []struct {
		name string
		cmd  *cobra.Command
		want bool
	}{
		{"an ordinary command asks", find("render"), true},
		{"a subcommand of one asks", find("project", "add"), true},
		{"version does not, because --check is its own answer", find("version"), false},
		{"help does not", find("help"), false},
		{"a completion run is a shell's, not a person's", find("completion"), false},
		{"init does not, because it answers with no network", find("init"), false},
		{"size does not, for the same reason", find("size"), false},
		{"guide does not, for the same reason", find("guide"), false},
		{"a marked subcommand does not", find("profile", "show"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := wantsUpdateCheck(tc.cmd); got != tc.want {
				t.Fatalf("wantsUpdateCheck(%s) = %v, want %v", tc.cmd.Name(), got, tc.want)
			}
		})
	}

	t.Run("the off switch stops it on every command", func(t *testing.T) {
		t.Setenv(selfupdate.EnvDisable, "1")
		if wantsUpdateCheck(find("render")) {
			t.Fatalf("%s set and the check still wanted to run", selfupdate.EnvDisable)
		}
	})
}

// A command whose help says it touches no network is marked, so the background
// check keeps the promise too. The help is where the claim is made and the mark
// is what the check reads, and a claim added to one without the other is a
// command that says it is offline and asks GitHub anyway.
//
// Only a claim a command makes about itself counts - a sentence opening "It
// touches no network", or "Nothing here reaches the network". A help screen describing the rule, or
// what another command does, is a mention, and the root is left out for that
// reason: its help says what `init` does.
func TestAnOfflineClaimIsKept(t *testing.T) {
	claim := regexp.MustCompile(`(?m)(^|[.!?]\s+|\*\*)It\b[^.]{0,30}\b(touches|reaches) no network|Nothing here reaches the network`)
	root := NewRootCmd()
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if claim.MatchString(sub.Long) && !isOffline(sub) {
				t.Errorf("%q says it touches no network and is not marked with touchesNoNetwork()", sub.CommandPath())
			}
			walk(sub)
		}
	}
	walk(root)
}

// A check that has not answered by the time the command has is dropped rather
// than waited for. The record it writes is what carries the answer to the next
// run, so nothing is lost by not waiting - and the command the user actually
// ran is not held up by a question they did not ask.
func TestAnUnfinishedCheckDoesNotHoldTheCommandUp(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetErr(discard{})

	// Never written to, and never closed: the goroutine is still in flight.
	stalled := updateCheck{
		result:   make(chan selfupdate.Result),
		deadline: time.Now().Add(backgroundLeash),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		warnIfNewerRelease(cmd, stalled)
	}()

	select {
	case <-done:
	case <-time.After(backgroundLeash + 2*time.Second):
		t.Fatal("warnIfNewerRelease waited past its own leash")
	}
}

// A zero updateCheck is what startUpdateCheck returns when it decided not to
// ask, and reading its nil channel blocks for ever. The caller has to
// recognise it - and so does a deadline of the zero time, which has passed.
func TestNoCheckWasStartedAndNothingIsWaitedFor(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetErr(discard{})

	done := make(chan struct{})
	go func() {
		defer close(done)
		warnIfNewerRelease(cmd, updateCheck{})
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("warnIfNewerRelease blocked on a check that was never started")
	}
}

// The help screen quotes the interval and the off switch rather than repeating
// them, so moving either moves what the screen says.
//
// **A number typed beside a constant is the drift this repository removes
// everywhere else.** The interval was written out as "two hours" in the Long,
// in the flag's usage and in both READMEs; the two the program renders now read
// the declaration.
func TestTheHelpScreenQuotesTheConstantsRatherThanRepeatingThem(t *testing.T) {
	root := NewRootCmd()
	cmd, _, err := root.Find([]string{"version"})
	if err != nil {
		t.Fatalf("no version command: %v", err)
	}
	screen := cmd.Long + "\n" + cmd.Flags().FlagUsages()

	for _, want := range []string{selfupdate.IntervalText(), selfupdate.EnvDisable} {
		if !strings.Contains(screen, want) {
			t.Errorf("`version --help` does not say %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "two hours") {
		t.Errorf("`version --help` writes the interval out by hand as well as reading it:\n%s", screen)
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// **A binary inside somebody else's tree is theirs to replace**, and each of
// those gets the command that actually moves it rather than one that would
// leave a package manager describing a version that is not there.
func TestWhoOwnsTheBinaryDecidesHowItIsUpgraded(t *testing.T) {
	cases := []struct {
		name, path, owner, upgrade string
	}{
		{
			"Homebrew on Apple silicon",
			"/opt/homebrew/Cellar/asgard-cli/0.1.2/bin/asgard-cli",
			"Homebrew", "brew upgrade asgard-cli",
		},
		{
			"Homebrew on Linux",
			"/home/linuxbrew/.linuxbrew/Cellar/asgard-cli/0.1.2/bin/asgard-cli",
			"Homebrew", "brew upgrade asgard-cli",
		},
		{
			"a Nix store path",
			"/nix/store/abc123-asgard-cli-0.1.2/bin/asgard-cli",
			"Nix", "update it the way the rest of your profile is updated",
		},
		{
			"an install this tool made",
			"/usr/local/bin/asgard-cli",
			"", "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			owner := notOursToReplace(tc.path)
			if owner != tc.owner {
				t.Fatalf("notOursToReplace(%s) = %q, want %q", tc.path, owner, tc.owner)
			}
			if tc.owner == "" {
				return
			}
			if got := upgradeWith(owner); got != tc.upgrade {
				t.Errorf("upgradeWith(%q) = %q, want %q", owner, got, tc.upgrade)
			}
		})
	}
}

// **The .deb and the .rpm install into /usr/bin, and that is the distribution's
// directory.** Replacing a file dpkg records leaves its database naming a
// version that is not on disk, which `dpkg -V` then reports as a damaged
// package - so the path alone decides it, with no package database consulted.
//
// /usr/local/bin is the other half of the same rule: the filesystem standard
// reserves it for software installed outside the package manager, which is
// where `install.sh` puts this, and is what makes an install made that way one
// that can replace itself.
func TestTheDistributionsDirectoriesAreNotOursToReplace(t *testing.T) {
	for _, path := range []string{
		"/usr/bin/asgard-cli",
		"/bin/asgard-cli",
		"/usr/sbin/asgard-cli",
		"/sbin/asgard-cli",
	} {
		if notOursToReplace(path) == "" {
			t.Errorf("%s was treated as ours to replace, and a package manager records it", path)
		}
	}
	for _, path := range []string{
		"/usr/local/bin/asgard-cli",
		"/opt/asgard/bin/asgard-cli",
	} {
		if got := notOursToReplace(path); got != "" {
			t.Errorf("notOursToReplace(%s) = %q, and nothing else owns that path", path, got)
		}
	}
}

// Each package manager gets the command that actually moves its own copy, and
// each names the version-less asset so the URL keeps working across releases.
func TestEachPackageManagerGetsItsOwnUpgradeCommand(t *testing.T) {
	cases := map[string][]string{
		"dpkg": {"dpkg -i", "asgard-cli_" + runtime.GOOS + "_" + runtime.GOARCH + ".deb"},
		"rpm":  {"rpm -U", "asgard-cli_" + runtime.GOOS + "_" + runtime.GOARCH + ".rpm"},
		"apk":  {"apk add", "asgard-cli_" + runtime.GOOS + "_" + runtime.GOARCH + ".apk"},
	}
	for tool, wants := range cases {
		got := upgradeWith(tool)
		for _, want := range wants {
			if !strings.Contains(got, want) {
				t.Errorf("upgradeWith(%q) = %q, which does not mention %q", tool, got, want)
			}
		}
		// A version in the URL is a URL that stops working at the next release.
		if strings.Contains(got, "/download/asgard-cli_0.") {
			t.Errorf("upgradeWith(%q) = %q, which pins a version in the asset name", tool, got)
		}
	}
}

// `go install` writes into GOBIN, and the module path is how that one moves.
//
// **Both sides are resolved before they are compared.** The running binary's
// path has its symlinks resolved, so an unresolved GOBIN matches nothing - and
// on macOS every path under /var is reached through one, which is where this
// was first seen to miss.
func TestAGoInstallBuildIsRecognisedThroughASymlink(t *testing.T) {
	root := t.TempDir()

	real := filepath.Join(root, "real", "bin")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(filepath.Join(root, "real"), link); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}

	// GOBIN as the user set it, through the link; the binary as `selfPath`
	// would report it, resolved.
	t.Setenv("GOBIN", filepath.Join(link, "bin"))
	t.Setenv("GOPATH", "")

	resolved, err := filepath.EvalSymlinks(filepath.Join(link, "bin", "asgard-cli"))
	if err != nil {
		// The file does not exist, so resolve the directory and rejoin.
		dir, derr := filepath.EvalSymlinks(filepath.Join(link, "bin"))
		if derr != nil {
			t.Fatalf("resolve: %v", derr)
		}
		resolved = filepath.Join(dir, "asgard-cli")
	}

	if got := notOursToReplace(resolved); got != "go install" {
		t.Errorf("notOursToReplace(%s) = %q, want %q", resolved, got, "go install")
	}
}
