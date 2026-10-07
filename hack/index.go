package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/asgard-ai-platform/asgard-fde-cli/hack/internal/src"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/kb"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/usecase"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/wiki"
)

func init() {
	register("index", check{
		Needs: "this repository",
		What:  "the index's page tables against the pages themselves - each row is rendered from the `group` and `description` the page declares, so adding a page cannot leave the index behind. `--write` rewrites it",
		Run:   runIndex,
	})
}

// The order the sections appear in, which is judgement and is the one thing
// here that is written down rather than derived.
//
// **A page's group is on the page; which groups exist, and in what order, is
// not.** Sorting them alphabetically would put "While building" before the
// products it assumes you know about, and deriving the order from first
// appearance makes it depend on filenames. So the sequence is here, and a
// group a page names that is not in it is reported rather than silently
// dropped.
var indexGroups = []string{
	"Products and scope",
	"While building",
	"In practice",
}

// An index file, and how to find a section heading and a table row in it.
//
// **Two indexes, two markups, and neither is ours to change.** The wiki's
// sections are `## ` headings; the extracts' are bold lead-ins with an
// explanatory tail after them, because that file reads as an argument rather
// than a catalogue. Normalising one to the other would rewrite prose to suit a
// renderer, which is the wrong way round.
type indexFile struct {
	path    string
	kind    string         // the pointer prefix a row writes: wiki, usecase
	heading *regexp.Regexp // captures the group name
	column  string         // the table's first column heading
	pages   func() ([]kb.Doc, error)
}

var indexFiles = []indexFile{
	{
		path:    "internal/corpus/wiki/index.md",
		kind:    "wiki",
		heading: regexp.MustCompile(`^## (.+?)\s*$`),
		column:  "| page ",
		pages:   func() ([]kb.Doc, error) { return wiki.List() },
	},
	{
		path:    "internal/corpus/usecase/README.md",
		kind:    "usecase",
		heading: regexp.MustCompile(`^\*\*([^*]+?)\*\*`),
		column:  "| file ",
		pages:   func() ([]kb.Doc, error) { return usecase.List() },
	},
}

// runIndex renders the index's tables from the pages and holds the file to
// them.
//
// **The index was the last hand-written list of what exists.** Every row
// restated a page's own subject, so adding a page meant writing the page and
// then writing about it somewhere else, with nothing checking that the two
// agreed - and the prose describing a page is exactly the kind of claim that
// goes stale in the direction nobody notices, because a wrong row still reads
// like a right one.
//
// What stays written is the grouping: which sections exist and in what order.
// That is the question each section asks, and no parse recovers it.
func runIndex(args []string) error {
	root, err := src.Root()
	if err != nil {
		return err
	}
	write := false
	for _, a := range args {
		if a == "--write" {
			write = true
		}
	}

	var bad []string
	total, stale := 0, 0
	for _, f := range indexFiles {
		docs, err := f.pages()
		if err != nil {
			return err
		}
		total += len(docs)
		byGroup := map[string][]kb.Doc{}
		for _, d := range docs {
			switch {
			case d.Group == "":
				bad = append(bad, fmt.Sprintf("%s/%s.md declares no `group:`, so nothing puts it in %s", f.kind, d.Name, f.path))
				continue
			case d.Description == "":
				bad = append(bad, fmt.Sprintf("%s/%s.md declares a `group:` and no `description:`, so its row would be blank", f.kind, d.Name))
			}
			byGroup[d.Group] = append(byGroup[d.Group], d)
		}

		path := filepath.Join(root, f.path)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out, seen, err := renderIndexTables(string(data), f, byGroup)
		if err != nil {
			bad = append(bad, err.Error())
			continue
		}
		for g := range byGroup {
			if !seen[g] {
				var names []string
				for _, d := range byGroup[g] {
					names = append(names, d.Name)
				}
				sort.Strings(names)
				bad = append(bad, fmt.Sprintf("group %q is declared by %s and %s has no section for it",
					g, strings.Join(names, ", "), f.path))
			}
		}
		if out == string(data) {
			continue
		}
		stale++
		if write {
			if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
				return err
			}
			fmt.Printf("wrote %s from the documents it lists.\n", f.path)
			continue
		}
		bad = append(bad, fmt.Sprintf("%s does not match the documents it lists", f.path))
	}

	if len(bad) > 0 {
		for _, b := range bad {
			fmt.Printf("adrift    %s\n", b)
		}
		fmt.Printf("\n%d problem(s). A document declares where it belongs and what it is for;\n", len(bad))
		fmt.Println("the index renders that. `go run ./hack index --write` rewrites the index")
		fmt.Println("from the documents - then read the diff, because a row that changed means")
		fmt.Println("a document's own line changed.")
		return errFailed
	}
	if stale > 0 {
		return nil
	}
	fmt.Printf("ok  %d document(s) across %d index file(s), each matching what they list.\n", total, len(indexFiles))
	return nil
}

// renderIndexTables replaces the rows under each section a document names, and
// touches nothing else in the file.
//
// **The prose around the tables is not generated and must survive.** Both
// indexes carry sections that are not listings at all - what is deliberately
// not covered, how to read an extract, the warnings that apply to all of them -
// and rewriting a file whole would delete them.
func renderIndexTables(data string, f indexFile, byGroup map[string][]kb.Doc) (string, map[string]bool, error) {
	lines := strings.Split(data, "\n")
	seen := map[string]bool{}
	var out []string
	i := 0
	for i < len(lines) {
		line := lines[i]
		out = append(out, line)
		i++
		m := f.heading.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		group := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(m[1]), ":"))
		docs, ours := byGroup[group]
		if !ours {
			continue
		}
		seen[group] = true
		for i < len(lines) && !strings.HasPrefix(lines[i], f.column) {
			if f.heading.MatchString(lines[i]) {
				return "", nil, fmt.Errorf("%s: section %q is a group a document declares and has no `%s|` table", f.path, group, f.column)
			}
			out = append(out, lines[i])
			i++
		}
		if i >= len(lines) {
			return "", nil, fmt.Errorf("%s: section %q has no table", f.path, group)
		}
		out = append(out, lines[i], "|---|---|")
		i += 2
		// **The order within a section is the index's, and it is kept.** It is
		// a reading order rather than an alphabet - products before the pages
		// that assume you know them - and no field on a document recovers it.
		// Sorting here destroyed it once. So the existing rows fix the order
		// for what is already listed, anything new is appended, and moving a
		// row is a deliberate edit that survives.
		var was []string
		for i < len(lines) && strings.HasPrefix(lines[i], "| [") {
			r := rowName.FindStringSubmatch(lines[i])
			if r == nil {
				// **A row this cannot read would be silently moved to the
				// end**, on every `--write`, with no diff anybody could
				// attribute. `kb` takes a document's name from its filename
				// and validates nothing, so a name outside this pattern is
				// possible even though none exists today. Loud beats a
				// reordering nobody can explain.
				return "", nil, fmt.Errorf("%s: section %q has a row this cannot read a document name out of, so rendering would move it: %s",
					f.path, group, strings.TrimSpace(lines[i]))
			}
			was = append(was, r[1])
			i++
		}
		for _, d := range inIndexOrder(docs, was) {
			out = append(out, fmt.Sprintf("| [`%s`](../%s/%s.md) | %s |", f.rowLabel(d.Name), f.kind, d.Name, d.Description))
		}
	}
	return strings.Join(out, "\n"), seen, nil
}

// rowLabel is what the first cell shows. The wiki writes a page name; the
// extracts write a file name, because that column is headed `file`.
func (f indexFile) rowLabel(name string) string {
	if f.column == "| file " {
		return name + ".md"
	}
	return name
}

// rowName reads the page name out of an existing index row.
var rowName = regexp.MustCompile("^\\| \\[`([a-z0-9-]+)(?:\\.md)?`\\]")

// inIndexOrder puts the pages in the order the index already lists them, with
// anything new after.
//
// **New goes last rather than alphabetically**, because last is where a reader
// looks for what was added and because any other rule moves rows nobody
// touched, which turns a one-line addition into a diff somebody has to read.
func inIndexOrder(pages []kb.Doc, was []string) []kb.Doc {
	by := map[string]kb.Doc{}
	for _, p := range pages {
		by[p.Name] = p
	}
	var out []kb.Doc
	seen := map[string]bool{}
	for _, name := range was {
		if p, ok := by[name]; ok && !seen[name] {
			out = append(out, p)
			seen[name] = true
		}
	}
	var fresh []kb.Doc
	for _, p := range pages {
		if !seen[p.Name] {
			fresh = append(fresh, p)
		}
	}
	sort.Slice(fresh, func(a, b int) bool { return fresh[a].Name < fresh[b].Name })
	return append(out, fresh...)
}
