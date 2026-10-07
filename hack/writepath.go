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
	register("write-path", check{
		Needs: "this repository",
		What:  "**every processor this repository teaches that reaches a database writes `allowWrite` out** - `query-database` and a mounted `semanticLayer` both resolve a missing one to TRUE, so a skeleton that configures only what it wants is a write path nothing in the rendered chart shows",
		Run:   runWritePath,
	})
}

// The two shapes that carry the field, and what they are called where they are
// written. A `semanticLayers` mount spells it inside a JSON string or as a YAML
// key; `query-database` spells it as a config entry.
var (
	queryProcessor = regexp.MustCompile(`(?m)^(\s*)(?:- )?(?:name:.*\n\s*)?type: query-database\s*$`)
	layersMount    = regexp.MustCompile(`(?m)^\s*(?:- name: )?semanticLayers:?\s*$`)

	// **The key, not the word.** Matching `allowWrite` anywhere passed a block
	// whose only mention was the comment explaining why the key matters - so
	// deleting the key it was explaining changed nothing, which is a check that
	// reports its own documentation as the thing it was looking for.
	allowWriteKey = regexp.MustCompile(`(?:^|[^A-Za-z])allowWrite"?\s*:|name:\s*allowWrite\b`)
)

// writePathFiles are the two places a reader copies a processor from: what the
// generator writes into a chart, and the skeletons an extract shows.
func writePathFiles(root string) ([]string, error) {
	var out []string
	for _, pattern := range []string{
		"internal/generate/templates/*.tmpl",
		"internal/corpus/usecase/*.md",
		"internal/corpus/wiki/*.md",
	} {
		paths, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil {
			return nil, err
		}
		out = append(out, paths...)
	}
	sort.Strings(out)
	return out, nil
}

// block returns the construct a match opens: every following line indented
// further than the matched line, stopping at a sibling list item at the same
// indent or at anything less indented.
//
// **Leading whitespace, not the first word.** Measuring from after the "- " of
// a list item makes the item's own value lines look like siblings, and the
// first version of this reported five correct blocks for that reason - which
// is the shape of check this repository deletes rather than ships.
func block(text string, at int) string {
	lines := strings.Split(text[at:], "\n")
	if len(lines) == 0 {
		return ""
	}
	ws := func(l string) int { return len(l) - len(strings.TrimLeft(l, " \t")) }
	base := ws(lines[0])
	var b strings.Builder
	b.WriteString(lines[0] + "\n")
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "" {
			b.WriteString("\n")
			continue
		}
		indent := ws(l)
		if indent < base {
			break
		}
		if indent == base && strings.HasPrefix(strings.TrimLeft(l, " \t"), "- ") {
			break
		}
		b.WriteString(l + "\n")
	}
	return b.String()
}

// uncommented drops what a reader is told from what a chart declares. A YAML
// comment is prose and every one of these blocks carries one saying the word.
func uncommented(s string) string {
	var b strings.Builder
	for _, l := range strings.Split(s, "\n") {
		t := strings.TrimLeft(l, " \t")
		if strings.HasPrefix(t, "#") {
			continue
		}
		if i := strings.Index(l, " #"); i >= 0 {
			l = l[:i]
		}
		b.WriteString(l + "\n")
	}
	return b.String()
}

func runWritePath(args []string) error {
	root, err := src.Root()
	if err != nil {
		return err
	}
	files, err := writePathFiles(root)
	if err != nil {
		return err
	}

	var bad []string
	checked := 0
	for _, p := range files {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		text := string(data)
		rel, _ := filepath.Rel(root, p)

		for _, kind := range []struct {
			re   *regexp.Regexp
			what string
		}{{queryProcessor, "a query-database processor"}, {layersMount, "a semanticLayers mount"}} {
			for _, loc := range kind.re.FindAllStringIndex(text, -1) {
				// A commented-out example is teaching too, and the ones here
				// say so in the comment beside them - so they count.
				checked++
				if allowWriteKey.MatchString(uncommented(block(text, loc[0]))) {
					continue
				}
				line := strings.Count(text[:loc[0]], "\n") + 1
				bad = append(bad, fmt.Sprintf("%s:%d: %s with no allowWrite, which resolves to true",
					rel, line, kind.what))
			}
		}
	}

	for _, b := range bad {
		fmt.Println(b)
	}
	fmt.Printf("\n%d place(s) mount a database, %d without allowWrite written out.\n", checked, len(bad))
	if len(bad) > 0 {
		fmt.Println("\nThe safe value is the one you have to write; the dangerous one is the")
		fmt.Println("default. A chart that configures only what it wants gets a write path,")
		fmt.Println("and nothing in the rendered chart says so.")
		return errFailed
	}
	return nil
}
