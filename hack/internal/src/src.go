// Package src resolves where this repository's upstream sources are cloned.
//
// **The URL is the source of truth and a local path is not**, which is why
// nothing upstream is vendored in. But every check needs a clone to read, and a
// path written into a script is true on one machine and wrong on every other -
// the same reason. So each source has an environment variable and a default,
// and the default is one person's layout and is documented as such.
//
// **Nothing here clones or pulls.** `git pull` is the reader's act: a check
// that fetched would turn "read at this commit" into "read at whatever was
// there when it ran", which is the one thing the provenance rule exists to
// prevent.
package src

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Source is one upstream, by the name a check asks for it by.
type Source struct {
	Env     string
	Default string // under $HOME
	What    string
}

// Order is the order a reader wants them in - the contract first - rather than
// alphabetical.
var Order = []string{"kube", "docs", "core"}

var Sources = map[string]Source{
	"kube": {"ASGARD_KUBE", "projects/asgard/asgard-kube",
		"the CRDs: https://github.com/asgard-ai-platform/asgard-kube"},
	"docs": {"ASGARD_DOCS", "projects/asgard-docs",
		"the product documentation: https://github.com/asgard-ai-platform/asgard-docs"},
	"core": {"ASGARD_CORE", "projects/asgard/asgard-core",
		"the processor definitions: https://github.com/asgard-ai-platform/asgard-core"},
}

// Root is this repository, found from the working directory upwards so that a
// check works from anywhere inside it.
func Root() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod above %s, so this is not the repository", dir)
		}
		dir = parent
	}
}

// dotenv reads `.env` at the repository root. Empty when there is none.
//
// **So the template is load-bearing.** `.env.example` documents these, and a
// template nobody loads is decoration: the environment still wins, this fills
// in what it does not set, and the default fills in the rest.
func dotenv() map[string]string {
	out := map[string]string{}
	root, err := Root()
	if err != nil {
		return out
	}
	f, err := os.Open(filepath.Join(root, ".env"))
	if err != nil {
		return out
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		k, v, _ := strings.Cut(line, "=")
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		out[strings.TrimSpace(k)] = v
	}
	return out
}

// Resolve returns the clone for one source. Environment first, then `.env`,
// then the default.
func Resolve(name string) (string, error) {
	s, ok := Sources[name]
	if !ok {
		return "", fmt.Errorf("no source called %q", name)
	}
	raw := os.Getenv(s.Env)
	if raw == "" {
		raw = dotenv()[s.Env]
	}
	if raw == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		raw = filepath.Join(home, s.Default)
	}
	if strings.HasPrefix(raw, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		raw = filepath.Join(home, raw[2:])
	}
	if fi, err := os.Stat(raw); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("%s is not a directory: %s\n  %s\n"+
			"  Clone it wherever you like, then set %s - in your shell, or in a `.env`\n"+
			"  at the repository root; `.env.example` is the template. Nothing here\n"+
			"  pulls for you.", s.Env, raw, s.What, s.Env)
	}
	return raw, nil
}

// Git runs one git command in a clone and returns its stdout.
func Git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	return string(out), err
}

// Commit is the short commit a clone is on, or "" when it is not a repository.
func Commit(dir string) string {
	out, err := Git(dir, "rev-parse", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}
