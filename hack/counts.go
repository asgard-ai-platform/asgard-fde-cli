package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/asgard-ai-platform/asgard-fde-cli/hack/internal/src"
)

func init() {
	register("counts", check{
		Needs: "$ASGARD_DOCS",
		What:  "**the screenshot arithmetic `wiki/screenshots.md` states**, recomputed at the asgard-docs commit that page names - the images held, the ones a live page uses, the orphans, and how many this material names. A claim whose wording has drifted out of every pattern fails rather than passes",
		Run:   runCounts,
	})
}

func runCounts(args []string) error {
	root, err := src.Root()
	if err != nil {
		return err
	}
	docs, err := src.Resolve("docs")
	if err != nil {
		return err
	}
	bad := checkImages(root, docs)
	fmt.Printf("screenshot arithmetic recomputed off %s at %s\n", docs, imagesRef)
	for _, b := range bad {
		fmt.Printf("  %s\n", b)
	}
	fmt.Printf("%d disagreement(s)\n", len(bad))
	if len(bad) > 0 {
		return errFailed
	}
	return nil
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
