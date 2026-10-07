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
	register("sources", check{
		Needs: "the clones",
		What:  "what each upstream clone resolves to and how far behind its remote it is; --extracts is how far each extract's source chart has moved since the commit the extract was written from",
		Run:   runSources,
	})
}

// writtenFrom reads each deployment row's commit: the version of the chart its
// extracts describe.
var writtenFrom = regexp.MustCompile(`(?m)^\|\s*([a-z0-9-]+)\s*\|\s*` + "`" + `([0-9a-f]{7,})` + "`")

func sourcesDoc(root string) (string, error) {
	b, err := os.ReadFile(filepath.Join(root, "source/SOURCES.md"))
	return string(b), err
}

// referenceDeployments reads the eight reference deployments off
// `source/SOURCES.md`.
//
// **Not a list in this file.** That document is the only one allowed to name a
// customer's repository, and a second copy here is the drift this whole
// directory exists to catch. The two contract repositories are excluded by name
// because they are declared as sources in their own right.
func referenceDeployments(doc string) []string {
	re := regexp.MustCompile(`\[([a-z0-9-]+)\]\(https://github\.com/asgard-ai-platform/[a-z0-9-]+\)`)
	seen := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(doc, -1) {
		switch m[1] {
		case "asgard-kube", "asgard-docs", "asgard-core":
		default:
			seen[m[1]] = true
		}
	}
	return sortedKeys(seen)
}

func runSources(args []string) error {
	root, err := src.Root()
	if err != nil {
		return err
	}
	doc, err := sourcesDoc(root)
	if err != nil {
		return err
	}

	if len(args) > 0 && args[0] == "--extracts" {
		return extractsReport(root, doc)
	}

	width := 0
	for _, s := range src.Sources {
		if len(s.Env) > width {
			width = len(s.Env)
		}
	}
	for _, name := range src.Order {
		s := src.Sources[name]
		dir, err := src.Resolve(name)
		if err != nil {
			fmt.Printf("%-*s  not found   %s\n", width, s.Env, s.What)
			continue
		}
		at := src.Commit(dir)
		if at == "" {
			n := 0
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				if fi, err := os.Stat(filepath.Join(dir, e.Name(), ".git")); err == nil && fi != nil {
					n++
				}
			}
			fmt.Printf("%-*s  %-10s %d clone(s) under it        %s\n", width, s.Env, "-", n, dir)
			continue
		}
		state := "(current)"
		if b := behind(dir); b != "" {
			state = "(" + b + ")"
		}
		fmt.Printf("%-*s  %-10s %-34s %s\n", width, s.Env, at, state, dir)
	}
	fmt.Println("\nNothing here pulls. `git -C <path> pull` before relying on a clone.")
	return nil
}

// behind reports how far a clone is behind its own remote, as a phrase.
//
// **Read, never fetched.** This reports the clone as it stands; if the answer
// matters, pull first. A check that fetched would be answering a different
// question each time it ran.
func behind(dir string) string {
	head := src.Commit(dir)
	for _, ref := range []string{"origin/HEAD", "origin/main", "origin/master"} {
		out, err := src.Git(dir, "rev-parse", "--short", ref)
		if err != nil {
			continue
		}
		up := strings.TrimSpace(out)
		if up == head {
			return ""
		}
		n, err := src.Git(dir, "rev-list", "--count", "HEAD.."+ref)
		if err != nil {
			return ""
		}
		return fmt.Sprintf("%s commit(s) behind %s", strings.TrimSpace(n), up)
	}
	return ""
}

func toSet(list []string) map[string]bool {
	out := map[string]bool{}
	for _, s := range list {
		out[s] = true
	}
	return out
}

// extractsReport says how far each extract's source chart has moved.
//
// A distance between two moving commits cannot be written down and stay true,
// so it is computed here rather than kept in source/SOURCES.md.
func extractsReport(root, doc string) error {
	base, err := src.Resolve("deployments")
	if err != nil {
		return err
	}
	type row struct {
		name, at, head string
		since          int
	}
	var rows []row
	width := 0
	for _, m := range writtenFrom.FindAllStringSubmatch(doc, -1) {
		name, at := m[1], m[2]
		if len(name) > width {
			width = len(name)
		}
		clone := filepath.Join(base, name)
		if _, err := os.Stat(filepath.Join(clone, ".git")); err != nil {
			rows = append(rows, row{name, at, "no clone", -1})
			continue
		}
		n := src.Since(clone, at)
		head, _ := src.Git(clone, "log", "-1", "--format=%h %ad", "--date=short")
		rows = append(rows, row{name, at, strings.TrimSpace(head), n})
	}
	fmt.Println("Each extract's source chart, as the clone stands. Nothing here pulls.")
	fmt.Println()
	moved := 0
	for _, r := range rows {
		switch {
		case r.since < 0:
			fmt.Printf("  %-*s  written from %s  -- %s\n", width, r.name, r.at, r.head)
		case r.since == 0:
			fmt.Printf("  %-*s  written from %s  unmoved\n", width, r.name, r.at)
		default:
			moved++
			fmt.Printf("  %-*s  written from %s  %d commit(s) since, now at %s\n", width, r.name, r.at, r.since, r.head)
		}
	}
	fmt.Printf("\n%d of %d have moved since the extracts were written from them.\n", moved, len(rows))
	fmt.Println("An extract describes one version of one chart; that is the size of the re-read.")
	return nil
}
