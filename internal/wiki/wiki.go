// Package wiki serves the platform wiki: what Asgard is made of and who each
// piece is for, distilled from the product documentation and the CRD contract.
//
// It answers a different question from internal/usecase. An extract there says
// how one shape of deployment is assembled, field by field, and assumes you
// already know the platform has that shape. These pages are what an agent needs
// before that: it has the customer's repository and nothing else, and that
// repository describes one customer's systems, never the platform they run on.
//
// **The pages themselves are in internal/corpus**, beside the extracts and in
// the layout a repository receives them - that package's doc comment says why.
// `asgard-cli init` writes both halves into a customer repository, and
// `scaffold.replaceCorpus` replaces them when the binary's version moves.
//
// The reading and searching are internal/kb's; this package is the way in.
package wiki

import (
	"strings"

	corpusfs "github.com/asgard-ai-platform/asgard-fde-cli/internal/corpus"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/kb"
)

// Page is one wiki page. It is kb.Doc under a name that reads at the call site.
type Page = kb.Doc

var corpus = kb.Corpus{
	FS:  corpusfs.FS,
	Dir: "wiki",
	// The index and the conventions are the wiki's own bookkeeping rather than
	// pages about the platform: readable by name, absent from a listing.
	Unlisted: map[string]bool{"index": true, "README": true},
	Noun:     "wiki page",
	Command:  "ls .agents/skills/asgard-platform/wiki/",
}

// List returns every page, sorted by name.
func List() ([]Page, error) { return corpus.List() }

// All returns every page including the index and the log, for writing the wiki
// out rather than listing it.
func All() ([]Page, error) { return corpus.All() }

// Read returns one page in full.
func Read(name string) (string, error) { return corpus.Read(name) }

// The two tables in aliases.md. Matched on the heading rather than on position,
// so the file can be reordered.
const (
	aliasFile      = "aliases.md"
	aliasHeading   = "## What a customer says, in the words this material uses"
	coveredHeading = "## Names the material covers"
	routedHeading  = "## Names it only routes"
)

// Aliases returns `aliases.md` in full - the mapping from what a customer
// said to what to search for. It was called Index, which is the one other
// thing in this corpus it is not.
func Aliases() (string, error) { return corpus.File(aliasFile) }

// table reads one two-column table out of the index file.
//
// A missing or renamed section returns nothing rather than an error. This
// decorates a search; it does not gate one, and a search that fails because a
// heading moved would be worse than one that gives no hint.
func table(heading string) map[string]string {
	body, err := corpus.File(aliasFile)
	if err != nil {
		return nil
	}
	_, rest, found := strings.Cut(body, heading)
	if !found {
		return nil
	}
	if next := strings.Index(rest, "\n## "); next >= 0 {
		rest = rest[:next]
	}

	out := map[string]string{}
	for _, line := range strings.Split(rest, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cols := strings.Split(strings.Trim(line, "|"), "|")
		if len(cols) != 2 {
			continue
		}
		word := strings.TrimSpace(cols[0])
		means := strings.TrimSpace(cols[1])
		// Skips the header row and the |---|---| separator without having to
		// count lines: neither has a word in the first column that is a word.
		if word == "" || means == "" || word == "they said" || strings.HasPrefix(word, "-") {
			continue
		}
		out[word] = means
	}
	return out
}

// Landing returns the pages written into a customer repository, which is every
// page including the index. The index is the map and a copy without it has
// none.
func Landing() ([]Page, error) { return corpus.All() }
