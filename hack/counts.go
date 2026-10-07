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
	register("counts", check{
		Needs: "$ASGARD_DEPLOYMENTS",
		What:  "**every count this material asserts about a reference deployment** - the page and operation ledgers, the API domains, the Plugin and SkillSet counts at the commit each claim names, and SOURCES.md's CR-file column at each read commit. A claim whose wording has drifted out of every pattern fails rather than passes. --dump prints what upstream counts",
		Run:   runCounts,
	})
}

var tableSeparator = regexp.MustCompile(`^\|[\s:|-]+\|$`)

// tableRowCounts is the data-row count of every markdown table in a document.
//
// A table is what follows a separator row, which is the only line a markdown
// table is obliged to have and the only one whose shape is unambiguous.
// Counting `|`-prefixed lines instead counts headers and separators too, and
// counting them across a whole file merges the tables - both of which turn a
// ledger of 88 into some other number that still looks like a count.
func tableRowCounts(text string) []int {
	var out []int
	open := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, " \t\r")
		switch {
		case tableSeparator.MatchString(line):
			out = append(out, 0)
			open = true
		case strings.HasPrefix(line, "|"):
			if open && len(out) > 0 {
				out[len(out)-1]++
			}
		default:
			open = false
		}
	}
	return out
}

// biggestTable is the largest ledger in a document. The explanatory tables are
// smaller.
func biggestTable(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	best := 0
	for _, n := range tableRowCounts(string(data)) {
		if n > best {
			best = n
		}
	}
	return best
}

func markdownFilesIn(path string) int {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			n++
		}
	}
	return n
}

// chartsIn counts chart directories under one clone.
//
// A vendored subchart under `charts/` is not one of ours and is excluded - it
// would inflate the sample-size floor this count exists to state without adding
// an arrangement anybody here read.
// skillDirsIn counts the runtime skills under a path: one SKILL.md each, which
// is what makes a directory a skill rather than a folder beside them.
func skillDirsIn(path string) int {
	n := 0
	_ = filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || filepath.Base(p) != "SKILL.md" {
			return nil
		}
		n++
		return nil
	})
	return n
}

// tokenIn counts occurrences of a word across the YAML and templates of a
// clone, which is what a reader's grep would find.
func tokenIn(word string) func(string) int {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`)
	return func(path string) int {
		n := 0
		_ = filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() {
				return nil
			}
			if strings.Contains(p, string(filepath.Separator)+".git"+string(filepath.Separator)) {
				return nil
			}
			switch filepath.Ext(p) {
			case ".yaml", ".yml", ".tmpl":
			default:
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			n += len(re.FindAllString(string(data), -1))
			return nil
		})
		return n
	}
}

// nullishIn counts `??`, which has no word boundary to anchor on.
func nullishIn(path string) int {
	n := 0
	_ = filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		if strings.Contains(p, string(filepath.Separator)+".git"+string(filepath.Separator)) {
			return nil
		}
		switch filepath.Ext(p) {
		case ".yaml", ".yml", ".tmpl":
		default:
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		n += strings.Count(string(data), "??")
		return nil
	})
	return n
}

func chartsIn(path string) int {
	n := 0
	_ = filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || filepath.Base(p) != "Chart.yaml" {
			return nil
		}
		if strings.Contains(p, string(filepath.Separator)+"charts"+string(filepath.Separator)) {
			return nil
		}
		n++
		return nil
	})
	return n
}

// kindCount is how many CRs of one kind a clone holds at one commit.
//
// **At a commit rather than in the working tree**, because the claim is dated:
// a deployment that gained a Plugin yesterday does not make yesterday's count
// wrong. A count with no commit beside it cannot be checked twice.
func kindCount(kind string) func(clone, ref string) int {
	return func(clone, ref string) int {
		// `-c` without `-h`: the count needs its file name to be parseable, and
		// `-h` prints a bare number per file that cannot be told from a path.
		out, err := src.Git(clone, "grep", "-c", "^kind: "+kind+"$", ref)
		if err != nil && out == "" {
			return -1
		}
		total := 0
		for _, line := range strings.Split(out, "\n") {
			if i := strings.LastIndex(line, ":"); i >= 0 {
				total += atoi(strings.TrimSpace(line[i+1:]))
			}
		}
		return total
	}
}

// expressionValues counts the `expression:` values under a path, and how many
// of them use an arrow function or a declaration.
//
// **The value, not the key.** An expression is usually a block scalar, so the
// line carrying the key says nothing about what is in it.
//
// This exists because the numbers behind "Expression is ordinary JavaScript"
// were taken over a set that left out the deployment writing most of them, and
// came out as "exactly one arrow function, no `const` anywhere".
func expressionValues(path string) (total, arrow, decl int) {
	key := regexp.MustCompile(`^(\s*)-?\s*expression:\s*(.*)$`)
	declRe := regexp.MustCompile(`\b(?:const|let)\b`)
	_ = filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		if strings.Contains(p, string(filepath.Separator)+".git"+string(filepath.Separator)) {
			return nil
		}
		switch filepath.Ext(p) {
		case ".yaml", ".yml", ".tmpl":
		default:
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		for i := 0; i < len(lines); i++ {
			m := key.FindStringSubmatch(lines[i])
			if m == nil {
				continue
			}
			indent := len(m[1])
			body := m[2]
			switch strings.TrimSpace(body) {
			case "|", "|-", "|+", ">", ">-", "":
				body = ""
				j := i + 1
				for ; j < len(lines); j++ {
					if strings.TrimSpace(lines[j]) == "" {
						body += "\n"
						continue
					}
					if len(lines[j])-len(strings.TrimLeft(lines[j], " \t")) <= indent {
						break
					}
					body += lines[j] + "\n"
				}
				i = j - 1
			}
			total++
			if strings.Contains(body, "=>") {
				arrow++
			}
			if declRe.MatchString(body) {
				decl++
			}
		}
		return nil
	})
	return
}

func expressionTotal(path string) int { n, _, _ := expressionValues(path); return n }
func expressionArrow(path string) int { _, n, _ := expressionValues(path); return n }
func expressionDecl(path string) int  { _, _, n := expressionValues(path); return n }

type countRow struct {
	Slug  string
	Clone string
	Of    string
	How   func(string) int
	What  string
	Says  []string
	Only  []string
}

// Each count: where it comes from, how, and every way this material states it.
var counts = []countRow{
	{
		Slug: "runtime-skills", Clone: "asgard-freyr-skills", Of: ".",
		How:  skillDirsIn,
		What: "runtime skills in the skills repository",
		Says: []string{`(\d+) runtime skills`},
	},
	{
		Slug: "shopline-l1-pages", Clone: "asgard-freyr-skills",
		Of:   "shopline-backoffice/references/page-map.md",
		How:  biggestTable,
		What: "L1 page entry points in the back-office page ledger",
		Says: []string{
			`(\d+) L1 page entry points`, `That (\d+) is the count`, `That (\d+)-page map`,
			`describing (\d+) pages`, `covering (\d+) pages`, `costs: (\d+) pages`,
			`costs: (\d+) menu-level page entry points`,
		},
	},
	{
		Slug: "deployment-charts", How: chartsIn,
		What: "charts across the reference deployments",
		Says: []string{`(\d+) charts\s+in all`, `(\d+) charts between them`},
		Only: []string{"unitech-e-asgard-kube", "xxentria-asgard-kube", "finance-ai-asgard-kube",
			"buy123-asgard-kube", "asgard-freyr-kube", "asgard-auto-post-kube",
			"asgard-industry-demo-generator", "asgard-freyr-skills"},
	},
	{
		Slug: "demo-generator-industries", Clone: "asgard-industry-demo-generator", Of: ".",
		How:  chartsIn,
		What: "industry charts in the demo generator",
		Says: []string{`(\d+) industries`},
	},
	{
		Slug: "shopline-operations", Clone: "asgard-freyr-skills",
		Of:   "shopline-backoffice/references/operation-map.md",
		How:  biggestTable,
		What: "rows in the back-office operation ledger",
		Says: []string{`(\d+) rows of operations`},
	},
	{
		Slug: "shopline-api-domains", Clone: "asgard-freyr-skills",
		Of:   "shopline-backoffice/references/api",
		How:  markdownFilesIn,
		What: "back-office API domains recorded",
		Says: []string{`(\d+) API\s+domains recorded`, `(\d+)\s+API domains recorded`},
	},
}

type pinnedRow struct {
	Slug, Clone, Ref, What string
	How                    func(clone, ref string) int
	Says                   []string
}

// Counts of a deployment AT A NAMED COMMIT. Each claim says which commit it was
// counted at, so the answer never changes - which is what separates a dated
// count from the ones above, recomputed off whatever the clone now holds.
var pinned = []pinnedRow{
	{Slug: "auto-post-plugins", Clone: "asgard-auto-post-kube", Ref: "edb0ad0",
		How: kindCount("Plugin"), What: "Plugin CRs",
		Says: []string{`(\d+) Plugins and \d+ SkillSets`, `carrying (\d+) Plugins`}},
	{Slug: "auto-post-plugins-as-read", Clone: "asgard-auto-post-kube", Ref: "d11b802",
		How: kindCount("Plugin"), What: "Plugin CRs at the commit its extracts were read at",
		Says: []string{`(\d+) Plugin CRs`}},
}

type namedBody struct{ rel, text string }

func countMaterial(root string) []namedBody {
	var out []namedBody
	// **Everywhere a count about a reference deployment is stated.** `internal/gate`
	// and `internal/brief` were outside this and both state them: a gate warning
	// said four deployments run the shared-SourceSet shape and none has a Plugin,
	// where six do and the one with 29 Plugins is the exemption itself.
	for _, dir := range []string{"internal/corpus", "internal/needs", "internal/stage",
		"internal/gate", "internal/brief", "source"} {
		_ = filepath.Walk(filepath.Join(root, dir), func(p string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() {
				return nil
			}
			if !strings.HasSuffix(p, ".md") && !strings.HasSuffix(p, ".go") {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			out = append(out, namedBody{rel, string(data)})
			return nil
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].rel < out[j].rel })
	return out
}

// claimsAgainst reports every place the material states this count with a
// different number, and whether any place states it at all.
//
// **A claim whose wording has drifted out of every pattern is a failure**, not
// a pass, because that is how a count stops being checked without anybody
// deciding to stop checking it.
func claimsAgainst(bodies []namedBody, patterns []string, want int, describe func(said string, line int, rel string) string) (findings []string, found int) {
	for _, body := range bodies {
		for _, pattern := range patterns {
			re := regexp.MustCompile(pattern)
			for _, m := range re.FindAllStringSubmatchIndex(body.text, -1) {
				found++
				said := body.text[m[2]:m[3]]
				if atoi(said) != want {
					line := strings.Count(body.text[:m[0]], "\n") + 1
					findings = append(findings, describe(said, line, body.rel))
				}
			}
		}
	}
	return
}

func runCounts(args []string) error {
	dump := len(args) > 0 && args[0] == "--dump"
	root, err := src.Root()
	if err != nil {
		return err
	}
	base, err := src.Resolve("deployments")
	if err != nil {
		return err
	}
	bodies := countMaterial(root)
	var bad []string
	counted := 0

	for _, row := range counts {
		if len(row.Only) > 0 {
			var missing []string
			for _, n := range row.Only {
				if fi, err := os.Stat(filepath.Join(base, n)); err != nil || !fi.IsDir() {
					missing = append(missing, n)
				}
			}
			if len(missing) > 0 {
				bad = append(bad, fmt.Sprintf("%s: %s not under %s, so this count would be "+
					"short by however many charts they hold",
					row.Slug, strings.Join(missing, ", "), base))
				continue
			}
			want := 0
			for _, n := range row.Only {
				want += row.How(filepath.Join(base, n))
			}
			counted++
			if dump {
				fmt.Printf("%-22s %4d  %s\n", row.Slug, want, row.What)
				fmt.Printf("%-22s       %d clone(s) under %s\n", "", len(row.Only), base)
				continue
			}
			findings, found := claimsAgainst(bodies, row.Says, want,
				func(said string, line int, rel string) string {
					return fmt.Sprintf("%s:%d says %s %s, and the clones hold %d",
						rel, line, said, row.What, want)
				})
			bad = append(bad, findings...)
			if found == 0 {
				bad = append(bad, fmt.Sprintf(
					"%s: no claim matches any of its patterns, so a count of %d is going unchecked",
					row.Slug, want))
			}
			continue
		}

		clone := filepath.Join(base, row.Clone)
		target := filepath.Join(clone, row.Of)
		if _, err := os.Stat(target); err != nil {
			bad = append(bad, fmt.Sprintf("%s: %s/%s is not in the clone, so this count cannot "+
				"be recomputed. `git -C %s fetch` first.", row.Slug, row.Clone, row.Of, clone))
			continue
		}
		want := row.How(target)
		counted++
		if dump {
			fmt.Printf("%-22s %4d  %s\n", row.Slug, want, row.What)
			fmt.Printf("%-22s       %s/%s at %s\n", "", row.Clone, row.Of, orQuestion(src.Commit(clone)))
			continue
		}
		findings, found := claimsAgainst(bodies, row.Says, want,
			func(said string, line int, rel string) string {
				return fmt.Sprintf("%s:%d says %s %s, and %s has %d",
					rel, line, said, row.What, row.Clone, want)
			})
		bad = append(bad, findings...)
		if found == 0 {
			bad = append(bad, fmt.Sprintf("%s: no claim in this material matches any of its "+
				"patterns, so a count of %d is going unchecked. Either the wording moved and "+
				"the pattern has to move with it, or the claim is gone and so should this row be.",
				row.Slug, want))
		}
	}

	for _, row := range pinned {
		clone := filepath.Join(base, row.Clone)
		want := row.How(clone, row.Ref)
		if want < 0 {
			bad = append(bad, fmt.Sprintf("%s: %s has no commit %s. `git -C %s fetch` first.",
				row.Slug, row.Clone, row.Ref, clone))
			continue
		}
		counted++
		if dump {
			fmt.Printf("%-26s %4d  %s at %s %s\n", row.Slug, want, row.What, row.Clone, row.Ref)
			continue
		}
		findings, found := claimsAgainst(bodies, row.Says, want,
			func(said string, line int, rel string) string {
				return fmt.Sprintf("%s:%d says %s %s, and %s has %d at %s",
					rel, line, said, row.What, row.Clone, want, row.Ref)
			})
		bad = append(bad, findings...)
		if found == 0 {
			bad = append(bad, fmt.Sprintf(
				"%s: no claim matches any of its patterns, so a count of %d at %s is going unchecked",
				row.Slug, want, row.Ref))
		}
	}

	if dump {
		return nil
	}

	bad = append(bad, sourcesTable(root, base)...)
	counted++
	if docs, err := src.Resolve("docs"); err != nil {
		bad = append(bad, "$ASGARD_DOCS is not set, so the screenshot arithmetic is unchecked")
	} else {
		bad = append(bad, checkImages(root, docs)...)
		counted++
	}

	fmt.Printf("%d count(s) recomputed off %s\n", counted, base)
	for _, b := range bad {
		fmt.Printf("  %s\n", b)
	}
	fmt.Printf("%d disagreement(s)\n", len(bad))
	if len(bad) > 0 {
		return errFailed
	}
	return nil
}

var crFileRow = regexp.MustCompile(`(?m)^\|\s*([a-z0-9-]+)\s*\|[^|]*\|\s*(\d+)\s*\|`)

// sourcesTable holds `source/SOURCES.md`'s CR-file count per deployment against
// its read commit.
//
// Two tables in that file are read together: the first gives each deployment's
// read-at commit, the second its CR-file count under a short name. **Both
// columns are pinned to that commit**, so the answer is fixed - which is the
// difference between this and the "since then" column that file used to carry.
func sourcesTable(root, base string) []string {
	doc, err := sourcesDoc(root)
	if err != nil {
		return []string{err.Error()}
	}
	readAt := map[string]string{}
	for _, m := range writtenFrom.FindAllStringSubmatch(doc, -1) {
		readAt[m[1]] = m[2]
	}
	var out []string
	for _, m := range crFileRow.FindAllStringSubmatch(doc, -1) {
		short, said := m[1], m[2]
		// The short name in the second table is a prefix of the clone name in
		// the first - "freyr" against "asgard-freyr-kube" - and the two are
		// written for their own readers rather than to be joined, so match on
		// the clone containing it and require exactly one.
		var hits []string
		for _, n := range sortedKeys(readAt) {
			if strings.Contains(n, short) && !strings.HasSuffix(n, "-skills") {
				hits = append(hits, n)
			}
		}
		if len(hits) != 1 {
			out = append(out, fmt.Sprintf("source/SOURCES.md: '%s' in the CR-file table matches "+
				"%d deployment(s) in the read-at table, so its count cannot be held against a commit",
				short, len(hits)))
			continue
		}
		clone, ref := filepath.Join(base, hits[0]), readAt[hits[0]]
		tree, err := src.Git(clone, "ls-tree", "-r", "--name-only", ref)
		if err != nil {
			out = append(out, fmt.Sprintf("source/SOURCES.md: %s has no commit %s, so its %s "+
				"CR files cannot be recounted", hits[0], ref, said))
			continue
		}
		n := 0
		for _, f := range strings.Split(tree, "\n") {
			if strings.HasSuffix(f, ".yaml") && strings.Contains(f, "/templates/") {
				n++
			}
		}
		if n != atoi(said) {
			out = append(out, fmt.Sprintf("source/SOURCES.md: %s says %s CR files and %s has %d "+
				"under templates/ at %s", short, said, hits[0], n, ref))
		}
	}
	return out
}

const imagesRef = "f00e0ee"

var imgPath = regexp.MustCompile(`/img/docs/([A-Za-z0-9/_.@-]+\.png)`)
var imgBacktick = regexp.MustCompile("`([A-Za-z0-9/_.@-]+\\.png)`")

// The screenshot arithmetic, at the commit that page names. Its "names N of
// that 168" moves whenever somebody adds an image to the page, which is why it
// is here rather than trusted.
var imageClaims = []struct {
	pattern *regexp.Regexp
	name    string
}{
	{regexp.MustCompile(`holds \*\*(\d+)\*\* ` + "`" + `\.png` + "`" + ` files`), "held"},
	{regexp.MustCompile(`only \*\*(\d+)\*\* of\n?\s*them are referenced`), "used"},
	{regexp.MustCompile(`(\d+) of those sit under ` + "`" + `user-guide/` + "`"), "orphaned_user_guide"},
	{regexp.MustCompile(`denominator that means anything is (\d+)`), "used"},
	{regexp.MustCompile(`page names \*\*(\d+)\*\* of that \d+`), "named"},
	{regexp.MustCompile(`names \*\*\d+\*\* of that (\d+)`), "used"},
	{regexp.MustCompile(`\*\*The (\d+) not named here carry no alt text\*\*`), "unnamed"},
	{regexp.MustCompile(`Coverage is (\d+)% of the images`), "coverage"},
}

// docsImages is the screenshot arithmetic at one commit.
//
// Four numbers that only mean something together: the images the repository
// holds, the ones a live page actually uses, the orphans in one dead directory,
// and how many of the used ones this material names. **The denominator is the
// used set**, not the tree.
func docsImages(root, docs, ref string) map[string]int {
	tree, err := src.Git(docs, "ls-tree", "-r", "--name-only", ref)
	if err != nil {
		return nil
	}
	files := strings.Split(tree, "\n")
	under := map[string]bool{}
	orphaned := 0
	for _, f := range files {
		if strings.HasSuffix(f, ".png") {
			if strings.Contains(f, "/user-guide/") {
				orphaned++
			}
			if strings.HasPrefix(f, "static/img/docs/") {
				under[strings.TrimPrefix(f, "static/img/docs/")] = true
			}
		}
	}
	used := map[string]bool{}
	for _, f := range files {
		if !strings.HasPrefix(f, "docs/") ||
			(!strings.HasSuffix(f, ".md") && !strings.HasSuffix(f, ".mdx")) {
			continue
		}
		body, err := src.Git(docs, "show", ref+":"+f)
		if err != nil {
			continue
		}
		for _, m := range imgPath.FindAllStringSubmatch(body, -1) {
			if under[m[1]] {
				used[m[1]] = true
			}
		}
	}
	page, err := os.ReadFile(filepath.Join(root, "internal/corpus/wiki/screenshots.md"))
	if err != nil {
		return nil
	}
	named := map[string]bool{}
	for _, m := range imgBacktick.FindAllStringSubmatch(string(page), -1) {
		named[m[1]] = true
	}
	for _, m := range imgPath.FindAllStringSubmatch(string(page), -1) {
		named[m[1]] = true
	}
	both := 0
	for k := range named {
		if used[k] {
			both++
		}
	}
	coverage := 0
	if len(used) > 0 {
		coverage = int(float64(100*both)/float64(len(used)) + 0.5)
	}
	return map[string]int{
		"held": len(under), "used": len(used), "orphaned_user_guide": orphaned,
		"named": both, "unnamed": len(used) - both, "coverage": coverage,
	}
}

func checkImages(root, docs string) []string {
	counts := docsImages(root, docs, imagesRef)
	if counts == nil {
		return []string{fmt.Sprintf(
			"asgard-docs has no commit %s, so the screenshot arithmetic cannot be recomputed", imagesRef)}
	}
	page, err := os.ReadFile(filepath.Join(root, "internal/corpus/wiki/screenshots.md"))
	if err != nil {
		return []string{err.Error()}
	}
	text := string(page)
	var out []string
	seen := 0
	for _, claim := range imageClaims {
		for _, m := range claim.pattern.FindAllStringSubmatchIndex(text, -1) {
			seen++
			said := text[m[2]:m[3]]
			if atoi(said) != counts[claim.name] {
				line := strings.Count(text[:m[0]], "\n") + 1
				out = append(out, fmt.Sprintf(
					"internal/corpus/wiki/screenshots.md:%d says %s for %s, and asgard-docs has %d at %s",
					line, said, claim.name, counts[claim.name], imagesRef))
			}
		}
	}
	if seen < len(imageClaims) {
		out = append(out, fmt.Sprintf("only %d of the %d screenshot claims still match a pattern, "+
			"so the rest are going unchecked - either the wording moved and the pattern has to "+
			"move with it, or the claim is gone", seen, len(imageClaims)))
	}
	return out
}
