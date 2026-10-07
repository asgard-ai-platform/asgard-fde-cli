// Package usecase serves the extracts: how each shape of Asgard deployment is
// built, taken from ones already in production.
//
// An agent about to author a chart reaches for these. **The extracts themselves
// are in internal/corpus**, beside the wiki and in the layout a repository
// receives them - that package's doc comment says why. `asgard-cli init` writes
// both halves into a customer repository.
//
// The reading and searching are internal/kb's; this package is the way in.
package usecase

import (
	corpusfs "github.com/asgard-ai-platform/asgard-fde-cli/internal/corpus"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/kb"
)

// Extract is one shape. It is kb.Doc under a name that reads at the call site.
type Extract = kb.Doc

var corpus = kb.Corpus{
	FS:  corpusfs.FS,
	Dir: "usecase",
	// README explains how to read an extract; it is not one.
	Unlisted: map[string]bool{"README": true},
	Noun:     "extract",
	Command:  "ls .agents/skills/asgard-platform/usecase/",
}

// List returns every extract, sorted by name.
func List() ([]Extract, error) { return corpus.List() }

// All returns every extract including the README that explains how to read
// one, for writing the extracts out rather than listing them.
func All() ([]Extract, error) { return corpus.All() }

// Read returns one extract in full.
func Read(name string) (string, error) { return corpus.Read(name) }
