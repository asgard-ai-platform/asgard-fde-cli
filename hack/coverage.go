package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/asgard-ai-platform/asgard-fde-cli/hack/internal/src"
)

func init() {
	register("coverage", check{
		Needs: "$ASGARD_DOCS",
		What:  "the four numbers in the coverage row, at the commit the row names and with a page's URL taken from its `slug:` frontmatter; --head also measures at HEAD, --drift names the cited pages that have moved",
		Run:   runCoverage,
	})
}

// The three directories the page excludes on purpose. Kept here because the
// check has to count them, and stated on the page with the reason for each.
var excluded = regexp.MustCompile(`asgard-builtin/message-template|help-community/release-notes/|superpowers/`)

// The row this checks, and the four numbers in it.
var coverageRow = regexp.MustCompile(
	`\|\s*asgard-docs\s*\|[^|]*\|\s*(\d+)\s*/\s*(\d+)\s*cited at ` + "`" + `([0-9a-f]{7,})` + "`" +
		`;\s*(\d+)\s*published and uncited;\s*(\d+)\s*deliberately excluded`)

var slugFront = regexp.MustCompile(`(?m)^slug:\s*(\S+)`)
var docsURL = regexp.MustCompile(`https://docs\.asgard-ai\.com/[A-Za-z0-9/_.#-]*[A-Za-z0-9/_-]`)
var docsCommit = regexp.MustCompile("asgard-docs `([0-9a-f]{7,})`")

// docPages returns every published page at one commit, as a slug.
//
// **`.md` and `.mdx` both.** Counting only `.mdx` gives 148 where the page says
// 162, which is what sent one reading of this down a false trail.
func docPages(docs, ref string) (map[string]bool, error) {
	out, err := src.Git(docs, "ls-tree", "-r", "--name-only", ref, "--", "docs")
	if err != nil {
		return nil, fmt.Errorf("%s has no commit %s. `git -C %s fetch` first", docs, ref, docs)
	}
	got := map[string]bool{}
	for _, f := range strings.Fields(out) {
		for _, ext := range []string{".mdx", ".md"} {
			if strings.HasSuffix(f, ext) {
				got[strings.TrimSuffix(strings.TrimPrefix(f, "docs/"), ext)] = true
				break
			}
		}
	}
	return got, nil
}

// urlIndex maps a live URL path to the page that serves it, at one commit.
//
// **A page's URL is its `slug:` frontmatter when it declares one**, and 14 of
// the 158 pages do. Matching a cited URL against the file path instead drops
// every one of them - four channel pages served lower-cased, eight processor
// pages named for their family and served under the builder's name, and one
// directory index. That undercount is why the coverage row read 71 rather than
// 77.
func urlIndex(docs, ref string) (map[string]string, error) {
	paths, err := docPages(docs, ref)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, path := range sortedKeys(paths) {
		for _, ext := range []string{".mdx", ".md"} {
			body, err := src.Git(docs, "show", ref+":docs/"+path+ext)
			if err != nil {
				continue
			}
			head := body
			if len(head) > 1500 {
				head = head[:1500]
			}
			url := path
			if m := slugFront.FindStringSubmatch(head); m != nil {
				url = strings.TrimPrefix(strings.TrimLeft(strings.TrimSpace(m[1]), "/"), "docs/")
			} else if strings.HasSuffix(path, "/index") {
				// A directory's index page is served at the directory, with no
				// `index` in the URL, whether or not it says so.
				url = strings.TrimSuffix(path, "/index")
			}
			out[strings.TrimRight(url, "/")] = path
			break
		}
	}
	return out, nil
}

// materialBodies is everything that cites a documentation URL.
func materialBodies(root string) map[string]string {
	out := map[string]string{}
	for _, pattern := range []string{
		"internal/corpus/*.md", "internal/corpus/*/*.md",
		"internal/stage/prompts/*.md",
	} {
		paths, _ := filepath.Glob(filepath.Join(root, pattern))
		for _, p := range paths {
			data, err := os.ReadFile(p)
			if err == nil {
				rel, _ := filepath.Rel(root, p)
				out[rel] = string(data)
			}
		}
	}
	_ = filepath.Walk(filepath.Join(root, "internal/scaffold/templates"),
		func(p string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() || !strings.HasPrefix(filepath.Base(p), "SKILL.md") {
				return nil
			}
			data, err := os.ReadFile(p)
			if err == nil {
				rel, _ := filepath.Rel(root, p)
				out[rel] = string(data)
			}
			return nil
		})
	return out
}

// citedSlugs is every docs page the material links to, resolved through the
// URL index. A miss means "not a page" - a screenshot under `img/`, or a
// directory with no index - and never "spelled differently".
func citedSlugs(root string, index map[string]string) map[string]bool {
	out := map[string]bool{}
	for _, body := range materialBodies(root) {
		for _, u := range docsURL.FindAllString(body, -1) {
			u = strings.TrimRight(strings.SplitN(u, "#", 2)[0], "/")
			slug := strings.SplitN(u, "docs.asgard-ai.com/", 2)[1]
			slug = strings.TrimPrefix(slug, "docs/")
			if strings.HasPrefix(slug, "img/") {
				continue
			}
			if index == nil {
				out[slug] = true
				continue
			}
			if page := index[strings.TrimRight(slug, "/")]; page != "" {
				out[page] = true
			}
		}
	}
	return out
}

func measure(root, docs, ref string) (cited, total, uncited, excl int, err error) {
	pages, err := docPages(docs, ref)
	if err != nil {
		return
	}
	index, err := urlIndex(docs, ref)
	if err != nil {
		return
	}
	for p := range citedSlugs(root, index) {
		if pages[p] {
			cited++
		}
	}
	total = len(pages)
	uncited = total - cited
	for p := range pages {
		if excluded.MatchString(p) {
			excl++
		}
	}
	return
}

func runCoverage(args []string) error {
	root, err := src.Root()
	if err != nil {
		return err
	}
	docs, err := src.Resolve("docs")
	if err != nil {
		return err
	}
	if len(args) > 0 && args[0] == "--drift" {
		return driftReport(root, docs)
	}

	index, err := os.ReadFile(filepath.Join(root, "internal/corpus/wiki/index.md"))
	if err != nil {
		return err
	}
	m := coverageRow.FindStringSubmatch(string(index))
	if m == nil {
		return fmt.Errorf("the asgard-docs row in internal/corpus/wiki/index.md does not match the shape\n" +
			"this checks: `<cited> / <total> cited at `<commit>`; <n> published and uncited;\n" +
			"<n> deliberately excluded`. Keep the shape or update this check.")
	}
	ref := m[3]
	claim := [4]int{atoi(m[1]), atoi(m[2]), atoi(m[4]), atoi(m[5])}

	cited, total, uncited, excl, err := measure(root, docs, ref)
	if err != nil {
		return err
	}
	got := [4]int{cited, total, uncited, excl}
	names := [4]string{"cited", "total", "uncited", "excluded"}
	fmt.Printf("at %s, the commit the page names:\n", ref)
	bad := 0
	for i := range names {
		flag := ""
		if claim[i] != got[i] {
			flag = "   <-- page says " + strconv.Itoa(claim[i])
			bad++
		}
		fmt.Printf("  %-10s %d%s\n", names[i], got[i], flag)
	}

	if len(args) > 0 && args[0] == "--head" {
		head := src.Commit(docs)
		c, t, u, e, err := measure(root, docs, head)
		if err != nil {
			return err
		}
		fmt.Printf("\nat %s, the clone's HEAD:\n", head)
		for i, v := range [4]int{c, t, u, e} {
			fmt.Printf("  %-10s %d\n", names[i], v)
		}
		fmt.Println("\nA difference here is not a defect: the page records a reading, and the")
		fmt.Println("clone has moved since. It is the size of what re-reading would cover.")
	}

	if bad > 0 {
		fmt.Printf("\n%d number(s) in the coverage row do not match a measurement at its own\n", bad)
		fmt.Println("commit. Fix the row, or say what else it counts.")
		return errFailed
	}
	return nil
}

// driftReport names the cited pages that have moved since the commit the citing
// document records.
//
// **This is the checkable half of "the wiki against asgard-docs".** Whether
// somebody read a page is theirs to claim; whether the page has changed under
// the reading is a fact, and this is the list of readings that would have to be
// redone. A page changing does not make the prose wrong - it makes it
// unconfirmed.
func driftReport(root, docs string) error {
	index, err := urlIndex(docs, "HEAD")
	if err != nil {
		return err
	}
	type row struct{ doc, slug, since, what string }
	var rows []row
	width := 0

	// **A citation's commit is on the entry that cites it, not on the
	// document.** A Sources section is a list of entries, each naming its own
	// pages and then the commit they were read at:
	//
	//	- [a page](url), [another](url)
	//	  - asgard-docs `f00e0ee`
	//
	// so the commit governs that entry and nothing else. This used to measure
	// every URL in a document from the document's oldest commit, which
	// **reports a re-read as drift**: a page re-read at a newer commit and
	// recorded as such kept being listed, because some other entry on the same
	// page still stood at the older one - and a check that goes on reporting
	// work already done teaches the reader to ignore its output.
	//
	// A document's oldest is still the fallback, for a URL cited outside any
	// entry that names a commit. That direction over-collects, which is the
	// safe one.
	bodies := materialBodies(root)
	for _, doc := range sortedKeys(bodies) {
		body := bodies[doc]
		var refs []string
		for _, m := range docsCommit.FindAllStringSubmatch(body, -1) {
			refs = append(refs, m[1])
		}
		if len(refs) == 0 {
			continue
		}
		// **The oldest, not the first alphabetically.** A page legitimately
		// cites two commits of one upstream - one section re-read, the rest
		// standing - and drift is measured from the older, because the newer
		// one has nothing behind it. Sorting the hashes as text picked
		// `23409b3` over `f00e0ee` and under-reported by three pages.
		oldest := oldestRef(docs, refs)
		seen := map[string]bool{}
		for _, entry := range citationEntries(body) {
			since := entry.ref
			if since == "" {
				since = oldest
			}
			for _, loc := range docsURL.FindAllStringIndex(entry.text, -1) {
				// **A URL template is not a citation.** The page tells a reader
				// to open `.../processor/<name>`, and the match stops at the
				// `<`, leaving the directory - which is not a page and never
				// becomes one, so it was reported as moved for ever with
				// nothing anybody could do about it.
				if loc[1] < len(entry.text) && entry.text[loc[1]] == '<' {
					continue
				}
				u := entry.text[loc[0]:loc[1]]
				u = strings.TrimRight(strings.SplitN(u, "#", 2)[0], "/")
				slug := strings.TrimPrefix(strings.SplitN(u, "docs.asgard-ai.com/", 2)[1], "docs/")
				if strings.HasPrefix(slug, "img/") || seen[slug] {
					continue
				}
				seen[slug] = true
				what := movedSince(docs, index, slug, since)
				if what == "" || what == "?" {
					continue
				}
				if len(doc) > width {
					width = len(doc)
				}
				rows = append(rows, row{doc, slug, since, what})
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].slug != rows[j].slug {
			return rows[i].slug < rows[j].slug
		}
		return rows[i].doc < rows[j].doc
	})
	for _, r := range rows {
		fmt.Printf("  %-*s  %s  since %s: %s\n", width, r.doc, r.slug, r.since, r.what)
	}
	fmt.Printf("\n%d cited page(s) have moved since the commit the citing document names.\n", len(rows))
	fmt.Println("A page changing does not make the prose wrong - it makes it unconfirmed, and")
	fmt.Println("this is the size of the re-read.")
	return nil
}

// citationEntry is one list entry of a Sources section: the text it spans, and
// the asgard-docs commit named inside it, if any.
type citationEntry struct {
	text string
	ref  string
}

// citationEntries splits a document into the list entries a Sources section is
// written as. An entry starts at a line beginning `- ` in column 0 and runs
// until the next one or the next heading, so its indented continuation lines -
// which is where the commit goes - belong to it.
//
// **Fenced blocks are dropped first.** `internal/corpus/wiki/README.md` shows
// the shape of a source block inside a fence, with a real URL and a real
// commit, and nothing is claimed by it: it is the schema, not a reading. Read
// as a citation it drifts for ever and can never be cleared, which is the
// shape of a check that fires on correct material.
func citationEntries(body string) []citationEntry {
	var entries []citationEntry
	var cur *citationEntry
	fenced := false
	flush := func() {
		if cur != nil {
			cur.ref = firstDocsCommit(cur.text)
			entries = append(entries, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			flush()
			continue
		}
		if fenced {
			continue
		}
		switch {
		case strings.HasPrefix(line, "- "):
			flush()
			cur = &citationEntry{text: line + "\n"}
		case strings.HasPrefix(line, "#"):
			flush()
			entries = append(entries, citationEntry{text: line + "\n"})
		case cur != nil && (line == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")):
			cur.text += line + "\n"
		default:
			flush()
			entries = append(entries, citationEntry{text: line + "\n"})
		}
	}
	flush()
	return entries
}

func firstDocsCommit(text string) string {
	if m := docsCommit.FindStringSubmatch(text); m != nil {
		return m[1]
	}
	return ""
}

func movedSince(docs string, index map[string]string, slug, since string) string {
	if page := index[strings.TrimRight(slug, "/")]; page != "" {
		slug = page
	}
	for _, ext := range []string{".mdx", ".md"} {
		path := "docs/" + slug + ext
		out, err := src.Git(docs, "log", "--format=%h", since+"..HEAD", "--", path)
		if err != nil {
			return "?"
		}
		commits := strings.Fields(out)
		_, statErr := src.Git(docs, "cat-file", "-e", "HEAD:"+path)
		exists := statErr == nil
		if len(commits) > 0 || exists {
			if !exists {
				return "deleted"
			}
			if len(commits) == 0 {
				return ""
			}
			return fmt.Sprintf("%d commit(s)", len(commits))
		}
	}
	return "not a page at HEAD"
}

// oldestRef returns the commit with the earliest date, which is the one drift
// is measured from.
func oldestRef(docs string, refs []string) string {
	best, bestAt := refs[0], int64(1<<62)
	for _, r := range refs {
		out, err := src.Git(docs, "log", "-1", "--format=%ct", r)
		if err != nil {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
		if err == nil && n < bestAt {
			best, bestAt = r, n
		}
	}
	return best
}
