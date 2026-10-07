package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/asgard-ai-platform/asgard-fde-cli/hack/internal/src"
)

func init() {
	register("aliases", check{
		Needs: "this repository",
		What:  "**every search term `aliases.md` sends a reader to appears somewhere in the corpus, and a search for it reaches the page the row names** - the half `--links` cannot do, because a row routes by word rather than by path. A word that lands nowhere reads exactly like a subject the material does not cover, and a word that lands on an index reads exactly like one that landed on an answer",
		Run:   runAliases,
	})
}

// aliasRow is a table row: what somebody said, and what to search for.
var aliasRow = regexp.MustCompile(`(?m)^\|\s*([^|]+?)\s*\|\s*(.+?)\s*\|\s*$`)

// pointer drops the `path/to.md` half of a cell. Those are checked by
// `audit-material --links`, and checking them twice reports one defect as two.
var pointer = regexp.MustCompile("`[^`]*`")

// aliasTerms pulls the searchable words out of one cell.
//
// **Everything after "and" is a pointer or a skill name**, not a word to grep,
// so the cell is cut there - "commerce, marketplace, channel - and
// `wiki/taiwan-channels.md`" contributes three terms.
func aliasTerms(cell string) []string {
	cell = pointer.ReplaceAllString(cell, "")
	if i := regexp.MustCompile(`\band\b`).FindStringIndex(cell); i != nil {
		cell = cell[:i[0]]
	}
	var out []string
	for _, t := range strings.Split(cell, ",") {
		t = strings.TrimSpace(strings.Trim(strings.TrimSpace(t), "-"))
		if t == "" || strings.HasPrefix(t, "the ") {
			continue
		}
		out = append(out, t)
	}
	return out
}

// corpusBody is everything a reader's grep would reach, minus the table itself.
// A term found only in `aliases.md` is the failure this check exists for: the
// row was the one document carrying every word of a translated query, and a
// search for a subject returned the word list rather than the page.
func corpusBody(root string) (string, error) {
	var b strings.Builder
	err := filepath.Walk(filepath.Join(root, "internal/corpus"), func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		if filepath.Base(p) == "aliases.md" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		b.WriteString(strings.ToLower(string(data)))
		b.WriteString("\n")
		return nil
	})
	if err != nil {
		return "", err
	}
	// The needs lists and the briefings land beside the corpus and a grep
	// reaches them, so they count as somewhere the word can be found.
	for _, rel := range []string{"internal/needs/needs.go", "internal/brief/brief.go"} {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err == nil {
			b.WriteString(strings.ToLower(string(data)))
			b.WriteString("\n")
		}
	}
	// **And the design-time skills**, which land in the same repository and
	// which several rows point at by name. Leaving them out reported `投影片 ->
	// slides` as dead when the deck skill is full of the word and the row says
	// so - a check firing on correct material, which is the one kind this
	// repository deletes rather than tunes.
	err = filepath.Walk(filepath.Join(root, "internal/scaffold/templates/.agents/skills"),
		func(p string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			b.WriteString(strings.ToLower(string(data)))
			b.WriteString("\n")
			return nil
		})
	if err != nil {
		return "", err
	}
	return b.String(), nil
}

func runAliases(args []string) error {
	root, err := src.Root()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(root, "internal/corpus/aliases.md"))
	if err != nil {
		return err
	}
	body, err := corpusBody(root)
	if err != nil {
		return err
	}

	seen := map[string]string{} // term -> the first row that sends somebody to it
	// said -> the page the row names, then its search terms. Only rows that
	// name one can be asked whether a search reaches it.
	rows := map[string][]string{}
	for _, m := range aliasRow.FindAllStringSubmatch(string(data), -1) {
		said := strings.TrimSpace(m[1])
		if said == "they said" || strings.HasPrefix(said, "---") {
			continue
		}
		terms := aliasTerms(m[2])
		for _, t := range terms {
			if _, ok := seen[t]; !ok {
				seen[t] = said
			}
		}
		if t := aliasTarget.FindStringSubmatch(m[2]); t != nil && len(terms) > 0 {
			row := []string{t[1]}
			for _, x := range terms {
				row = append(row, strings.ToLower(x))
			}
			rows[said] = row
		}
	}

	var dead []string
	for _, t := range sortedKeys(seen) {
		if !strings.Contains(body, strings.ToLower(t)) {
			dead = append(dead, fmt.Sprintf("  %q sends a reader to search for %q, which the corpus never uses", seen[t], t))
		}
	}
	sort.Strings(dead)
	for _, d := range dead {
		fmt.Println(d)
	}
	unreached, checked, err := checkAliasRetrieval(root, rows)
	if err != nil {
		return err
	}
	for _, u := range unreached {
		fmt.Println(u)
	}

	fmt.Printf("\n%d search term(s) in the alias table, %d that land nowhere.\n", len(seen), len(dead))
	fmt.Printf("%d row(s) name the page that answers them, %d a search does not reach.\n", checked, len(unreached))
	if len(dead) > 0 || len(unreached) > 0 {
		fmt.Println("\nA translated query that comes back empty reads as a subject nobody covered,")
		fmt.Println("and one that returns an index reads exactly like one that returned an answer.")
		fmt.Println("Point the row at the word a page actually writes, or write the page.")
		return errFailed
	}
	return nil
}

// aliasTarget is the document a row sends a reader to, when it names one.
//
// A row's second cell is search terms, optionally followed by the page that
// answers them. `--links` resolves that pointer; what it cannot ask is whether
// a search for those terms actually SURFACES that page, which is the question
// below.
var aliasTarget = regexp.MustCompile("`((?:wiki|usecase|needs|brief|guide)/[a-z0-9-]+\\.md)`")

// corpusDocs is the landed tree as one lowered body per document, keyed the way
// a row names it.
func corpusDocs(root string) (map[string]string, error) {
	out := map[string]string{}
	base := filepath.Join(root, "internal/corpus")
	err := filepath.Walk(base, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		rel, relErr := filepath.Rel(base, p)
		if relErr != nil {
			return nil
		}
		// The query's own source is not a result. See indexPage.
		if filepath.Base(p) == "aliases.md" {
			return nil
		}
		data, readErr := os.ReadFile(p)
		if readErr != nil {
			return nil
		}
		out[filepath.ToSlash(rel)] = strings.ToLower(string(data))
		return nil
	})
	return out, err
}

// indexPage is a document that lists other documents. They are `Unlisted` for
// the same reason they are ranked apart here: a page naming every subject
// matches every query.
//
// **`aliases.md` is not one of them, because it is not a result at all.** It is
// where the query comes from - it carries every term in the table by
// construction, so it scores maximally on every row and reported all of them
// as unreachable the first time this ran. That is the cry-wolf shape, and the
// answer is the same one the corpus already applies: the table is `Unlisted`
// and sits above both halves, so it is excluded from the search rather than
// ranked in it.
func indexPage(name string) bool {
	b := filepath.Base(name)
	return b == "index.md" || b == "README.md"
}

// checkAliasRetrieval asks the question `Goal.md` says is the whole engineering
// problem - **whether an agent searching the customer's words lands on the page
// that answers them** - and it is the only check that asks it.
//
// An agent does not browse an index. It greps, and takes what comes back. So a
// row that translates a word correctly and sends the reader to a page the search
// does not surface has moved the failure rather than fixed it: the query returns
// something, which reads exactly like an answer.
//
// **This repository has already paid for the specific failure below.** The alias
// table used to live among the wiki pages, and because it lists every alias it
// was reliably the one document carrying every term of a translated query - so a
// search for a subject returned the word list instead of the page. It was moved
// out for that, and the rule that keeps it out is here: **no page that lists
// other pages may outrank the page a row names.**
//
// Scored by how many of the row's terms a document contains, which is what a
// grep gives an agent rather than what a ranker would. A tie counts against the
// index page, because a tie is enough to put it first in somebody's output.
func checkAliasRetrieval(root string, rows map[string][]string) ([]string, int, error) {
	docs, err := corpusDocs(root)
	if err != nil {
		return nil, 0, err
	}
	var bad []string
	checked := 0
	for _, said := range sortedKeys(rows) {
		terms := rows[said]
		if len(terms) < 2 {
			continue
		}
		target := terms[0]
		body, ok := docs[target]
		if !ok {
			// `--links` owns a pointer that resolves to nothing; reporting it
			// here as well would call one defect two.
			continue
		}
		checked++
		// **Occurrences, not presence.** Scoring a term as present-or-absent
		// makes an index row that NAMES a page tie with the page ABOUT it -
		// a description contains its subject's words once, by construction -
		// and a tie was enough to report thirteen correct rows as broken.
		// What a grep hands an agent is lines, so the count of lines is the
		// thing to rank by.
		score := func(text string) int {
			n := 0
			for _, t := range terms[1:] {
				n += strings.Count(text, t)
			}
			return n
		}
		want := score(body)
		if want == 0 {
			bad = append(bad, fmt.Sprintf(
				"  %q -> `%s`: that page carries none of the words the row says to search for, so the "+
					"pointer is right and the search never reaches it", said, target))
			continue
		}
		for _, name := range sortedKeys(docs) {
			if !indexPage(name) || name == target {
				continue
			}
			// Strictly greater: outranking is what the recorded failure was,
			// and a tie between a page and a list of pages is not it.
			if score(docs[name]) > want {
				bad = append(bad, fmt.Sprintf(
					"  %q -> `%s`: `%s` matches those words as well or better, and it is a list of pages "+
						"rather than an answer. An index that outranks a page is how a translated query "+
						"returns the word list", said, target, name))
				break
			}
		}
	}
	sort.Strings(bad)
	return bad, checked, nil
}
