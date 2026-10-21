// Package extension reads, checks and copies an Asgard FDE Extension: a git
// repository of design-time material that lands in an FDE repository under
// `.agents/skills/<id>/` and teaches the FDE Agent one external system or one
// body of domain knowledge.
//
// The design is asgard-odin-pm `docs/concepts/extension-connector/spec.md`
// section 4. This package holds the parts an author needs offline: what a valid
// Extension is (Lint), what its examples' CRs get wrong (CheckExamples), what
// leaves the repository (Shipped, Copy) and the record an FDE repository keeps
// of what it holds (Lock).
package extension

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Prefix starts every Extension id. An Extension lands beside the skills
// `asgard-cli init` writes and the ones the platform hands out, in the same
// directory, so it needs a namespace of its own; a list of reserved names would
// have to follow both of those as they change.
const Prefix = "asgard-ext-"

// idPattern is Prefix followed by kebab-case.
var idPattern = regexp.MustCompile(`^asgard-ext-[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidID reports whether id has the shape every Extension id has.
func ValidID(id string) bool { return idPattern.MatchString(id) }

// Shipped are the top-level entries that leave an extension repository. A copy
// from a local path takes these and nothing else, and so does a fetch from the
// platform: the registry builds its archive from a list of its own, and
// whatever else an archive holds is left out here, so what an author tests is
// what an FDE repository receives.
//
// Everything else in the repository stays there: `.git`, CI configuration, and
// any AGENTS.md or CLAUDE.md the author keeps for their own coding agent. The
// FDE Agent's way in is SKILL.md.
var Shipped = []string{
	"README.md",
	"SKILL.md",
	"CHANGELOG.md",
	"routes",
	"brakes",
	"setup",
	"knowledge",
	"examples",
	"scripts",
	"verify",
	"assets",
}

// Brakes are the ids a route may name under `brakes:`. A brake points the FDE
// Agent at a platform mechanism that fits better than the route it was about to
// take, so the catalogue follows the platform rather than any one Extension.
// This catalogue is the one that decides: the registry does not check brakes,
// so `extension lint` is the only place a brake id is held against it.
var Brakes = map[string]string{
	"sourceset-syncer":            "a Syncer into a SourceSet, mounted into the sandbox, when the need is reading files",
	"dataconnector-semanticlayer": "a DataConnector and a SemanticLayer, when the need is querying data the platform already has a class for",
}

// RepoID finds the Extension repository dir is in: the nearest README.md at or
// above dir whose frontmatter id starts with Prefix. It returns "" for both
// when there is none.
//
// It exists for the one place a coding agent arrives without being told which
// kind of repository it is in - `asgard-cli gate`, run out of habit from an FDE
// repository - so that command can say which gate this one has.
func RepoID(dir string) (root, id string) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", ""
	}
	for {
		if m, _, err := ReadMeta(filepath.Join(dir, "README.md")); err == nil && strings.HasPrefix(m.ID, Prefix) {
			return dir, m.ID
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ""
		}
		dir = parent
	}
}
