// Package gitrepo reads the checkout the CLI is standing in.
//
// This is not the git integration TASK.md rules out. That non-goal is about
// deciding which repository an engagement is about - creating one on a
// provider, naming it, authenticating to it - because Asgard grows its own
// mechanism for handing a customer a repository, and an agent that fills that
// gap by guessing binds a pipeline to the guess. Reading what a
// checkout already says is a different thing, and it is what keeps platform
// identifiers out of the customer's repository: the pipeline a command acts on
// is found by matching the origin remote against the workspace's pipelines,
// rather than by a file somebody has to keep in sync with the platform.
//
// Every function here degrades rather than fails. A directory that is not a
// checkout, a checkout with no origin, a machine with no git binary: each is a
// reason a command cannot resolve context on its own and has to be told, not a
// reason for it to stop.
package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
)

// ErrNotARepository reports that the directory is not inside a git checkout.
var ErrNotARepository = errors.New("not a git repository")

// ErrNoOrigin reports a checkout with no origin remote.
var ErrNoOrigin = errors.New("no origin remote")

// ErrNoGit reports that the git binary was not found. It is separated from the
// other two because the remedy is entirely different: install git, rather than
// run the command somewhere else.
var ErrNoGit = errors.New("git is not installed or not on PATH")

// Root returns the top level of the checkout containing dir.
func Root(ctx context.Context, dir string) (string, error) {
	out, err := run(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		if errors.Is(err, ErrNoGit) {
			return "", err
		}
		return "", fmt.Errorf("%s: %w", dir, ErrNotARepository)
	}
	return out, nil
}

// OriginURL returns the origin remote's URL.
func OriginURL(ctx context.Context, dir string) (string, error) {
	out, err := run(ctx, dir, "remote", "get-url", "origin")
	if err != nil {
		if errors.Is(err, ErrNoGit) {
			return "", err
		}
		return "", ErrNoOrigin
	}
	return out, nil
}

// HeadSHA returns the full commit SHA of HEAD.
func HeadSHA(ctx context.Context, dir string) (string, error) {
	return run(ctx, dir, "rev-parse", "HEAD")
}

// Dirty reports whether the working tree has uncommitted changes, untracked
// files included.
func Dirty(ctx context.Context, dir string) (bool, error) {
	out, err := run(ctx, dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

// scpLike matches git's abbreviated ssh syntax, which is not a URL and cannot
// be parsed as one: the colon separates host from PATH, not host from port.
//
//	git@github.com:owner/name.git
var scpLike = regexp.MustCompile(`^(?:([^@/]+)@)?([^/:]+):(.+)$`)

// RemoteHost is the host a remote URL points at, lowercased, without any port.
// Empty when the URL is not a shape this understands.
func RemoteHost(remoteURL string) string {
	host, _, ok := splitRemote(remoteURL)
	if !ok {
		return ""
	}
	return host
}

// splitRemote reduces a remote URL to its host and its path.
//
// It is deliberately conservative. Anything it is not sure about is reported as
// unrecognised rather than reduced to a best guess: a wrong guess here does not
// fail, it succeeds at the wrong thing, and the caller has no way to tell.
func splitRemote(remoteURL string) (host, path string, ok bool) {
	raw := strings.TrimSpace(remoteURL)
	if raw == "" {
		return "", "", false
	}

	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			// file:///srv/git/x has no host: a local path, not a provider.
			return "", "", false
		}
		// Hostname() drops the port, which is why this is parsed rather than
		// pattern-matched: in ssh://host:2222/owner/name the 2222 is a port,
		// and a pattern that treats the colon as a separator reads it as the
		// owner.
		return strings.ToLower(u.Hostname()), strings.Trim(u.Path, "/"), true
	}

	m := scpLike.FindStringSubmatch(raw)
	if m == nil {
		// A bare path (/srv/git/x, ../sibling.git) is not a provider remote.
		return "", "", false
	}
	return strings.ToLower(m[2]), strings.Trim(m[3], "/"), true
}

// FullName reduces a remote URL to "owner/name", or returns false when the URL
// is not a shape it recognises.
//
// Exactly two path segments. A deeper path — a GitLab subgroup, say — is
// refused rather than folded into an owner and a name with a slash in it: this
// names the repository a pipeline binds, and that shape has two parts.
func FullName(remoteURL string) (string, bool) {
	_, path, ok := splitRemote(remoteURL)
	if !ok {
		return "", false
	}
	path = strings.TrimSuffix(path, ".git")
	owner, name, found := strings.Cut(path, "/")
	if !found || owner == "" || name == "" || strings.Contains(name, "/") {
		return "", false
	}
	return owner + "/" + name, true
}

// OriginFullName is the origin remote reduced to "owner/name".
func OriginFullName(ctx context.Context, dir string) (string, error) {
	remote, err := OriginURL(ctx, dir)
	if err != nil {
		return "", err
	}
	full, ok := FullName(remote)
	if !ok {
		return "", fmt.Errorf("origin %q is not a shape this understands, so the repository it names cannot be matched against a pipeline", remote)
	}
	return full, nil
}

// OriginHost is the host the origin remote points at, or empty.
func OriginHost(ctx context.Context, dir string) string {
	remote, err := OriginURL(ctx, dir)
	if err != nil {
		return ""
	}
	return RemoteHost(remote)
}

// Ignored reports whether path is excluded from this checkout by an ignore
// rule, and whether that could be established at all.
//
// **Only git knows.** The rules are in the repository's own `.gitignore`, in
// files under any directory above it, in `.git/info/exclude` and in a global
// core.excludesFile - so a command that guesses by reading one file is wrong
// for every repository that used any of the others, and a customer's checkout
// is not ours to make assumptions about.
//
// It degrades like everything else here: no git, no checkout, or a git that
// answers unexpectedly all come back known=false, and the caller says less
// rather than something untrue.
func Ignored(ctx context.Context, dir, path string) (ignored, known bool) {
	cmd := exec.CommandContext(ctx, "git", "check-ignore", "-q", "--", path)
	cmd.Dir = dir
	err := cmd.Run()
	if err == nil {
		return true, true
	}
	// check-ignore is one of the few git commands whose exit code is the
	// answer rather than a failure: 1 means "not ignored", and anything else -
	// 128 for "not a repository", or no binary at all - means it did not
	// answer.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, true
	}
	return false, false
}

// run executes one git command and returns its trimmed stdout.
func run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var execErr *exec.Error
		if errors.As(err, &execErr) {
			return "", ErrNoGit
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
