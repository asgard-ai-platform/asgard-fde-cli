// Package repo answers what a customer repository is made of, by looking at it.
//
// **A value belongs in a config file only when nothing on disk implies it and
// the platform cannot be asked.** Nothing this package answers needs one: the
// project list is the declaration's chart paths, the customer's name is the
// platform's to answer, a deployment "shape" is an intent this tool has no
// business judging, and the workspace slug named a directory that needs no
// name. `LegacyConfigName` is here only to recognise a repository scaffolded
// before that.
//
// See asgard-odin-pm docs/decisions/2026-09-05-asgard-cli-config-surface.md.
package repo

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/binding"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/pipelineconfig"
)

// SpecSlug is the directory under `docs/` that holds the living spec.
//
// **The same in every repository, and that is the point.** It used to be
// `<workspace-slug>-asgard`, which wrote the customer's name into a path inside
// that customer's own repository - `acme-asgard-kube/docs/spec/acme-asgard/` -
// and was the last reason `.asgard-config.json` existed. A constant needs no
// storage, no derivation and no guess.
const SpecSlug = "asgard"

// LegacyConfigName is the file this package replaced.
//
// Named here only so `check` can report one that is still lying around. A
// repository scaffolded before this change has one, and it records four things
// that are now either derived, asked for, or gone - so leaving it in place
// would mean two answers to every question it used to answer.
const LegacyConfigName = ".asgard-config.json"

// RepoSuffix is the convention for a customer repository's name.
const RepoSuffix = "-asgard-kube"

// projectsDir is where a project's chart lives.
const projectsDir = "projects"

// Projects returns the project slugs this repository has, sorted.
//
// It is the union of two facts, and neither is a stored copy: the chart paths
// the declaration names, and the directories under `projects/`. The union is
// what covers the window in the middle of onboarding - a chart is scaffolded
// before its release is declared, and a release can be declared before anyone
// writes the chart - and in both directions the honest answer is "this project
// is here".
func Projects(root string) ([]string, error) {
	seen := map[string]bool{}

	if cfg, err := pipelineconfig.LoadFromRepo(root, ""); err == nil {
		for _, chart := range cfg.Charts() {
			if slug := slugOfChart(chart); slug != "" {
				seen[slug] = true
			}
		}
	}

	entries, err := os.ReadDir(filepath.Join(root, projectsDir))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			seen[e.Name()] = true
		}
	}

	out := make([]string, 0, len(seen))
	for slug := range seen {
		out = append(out, slug)
	}
	sort.Strings(out)
	return out, nil
}

// slugOfChart reads the project out of a declared chart path.
//
// `projects/<slug>/chart/app` is the layout `scaffold` writes and the one the
// declaration points at. A chart somewhere else is legitimate - the platform
// takes any path - and simply names no project here.
func slugOfChart(chart string) string {
	parts := strings.Split(filepath.ToSlash(chart), "/")
	if len(parts) >= 2 && parts[0] == projectsDir {
		return parts[1]
	}
	return ""
}

// ChartDir is where a project's chart lives, relative to the repository.
func ChartDir(slug string) string {
	return filepath.Join(projectsDir, slug, "chart", "app")
}

// HasLegacyConfig reports whether the file this package replaced is still here.
func HasLegacyConfig(root string) bool {
	_, err := os.Stat(filepath.Join(root, LegacyConfigName))
	return err == nil
}

// Root finds the repository containing dir, or "" when there is none.
//
// The declaration is the test: it is the file the platform reads on every run.
func Root(dir string) string {
	declPath, _, err := binding.Locate(dir)
	if err != nil || declPath == "" {
		return ""
	}
	return filepath.Dir(declPath)
}

// slugPattern is the DNS-label shape every name here has to take, because these
// names become parts of Kubernetes object names.
var slugPattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// ValidateSlug rejects a name that cannot be part of a Kubernetes object name.
func ValidateSlug(field, s string) error {
	switch {
	case s == "":
		return fmt.Errorf("%s is required", field)
	case len(s) > 40:
		return fmt.Errorf("%s %q is %d characters; keep it under 40, because names derived from it inherit its length", field, s, len(s))
	case !slugPattern.MatchString(s):
		return fmt.Errorf("%s %q must be lower-case letters, digits and hyphens, starting and ending with a letter or digit", field, s)
	}
	return nil
}

// SpecSlugIn returns the living-spec slug this repository actually uses.
//
// **The slug is a fact on disk, not a constant, once a repository exists.** An
// engagement may rename `docs/spec/<slug>/`, and `docs/spec/README.md` names
// the one in use - so a scaffolder that assumes the default writes a second
// living-spec root beside the first and reports it as created. Two indexes
// that disagree is the failure the whole four-layer split exists to prevent,
// and the next reader cannot tell which is current.
//
// So: the directory that is there wins, and `SpecSlug` is only the default for
// a repository that has none yet. Where more than one exists the repository is
// already in the broken state - this prefers the default so the choice is
// stable, and `check` reports the duplicate.
func SpecSlugIn(root string) string {
	entries, err := os.ReadDir(filepath.Join(root, "docs", "spec"))
	if err != nil {
		return SpecSlug
	}
	var found []string
	for _, e := range entries {
		if e.IsDir() {
			found = append(found, e.Name())
		}
	}
	switch {
	case len(found) == 0:
		return SpecSlug
	case slices.Contains(found, SpecSlug):
		return SpecSlug
	default:
		sort.Strings(found)
		return found[0]
	}
}

// SpecRoots returns every living-spec directory under `docs/spec/`. More than
// one is a defect: see SpecSlugIn.
func SpecRoots(root string) []string {
	entries, err := os.ReadDir(filepath.Join(root, "docs", "spec"))
	if err != nil {
		return nil
	}
	var found []string
	for _, e := range entries {
		if e.IsDir() {
			found = append(found, e.Name())
		}
	}
	sort.Strings(found)
	return found
}
