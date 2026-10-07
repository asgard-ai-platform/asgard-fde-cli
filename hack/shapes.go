package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/asgard-ai-platform/asgard-fde-cli/hack/internal/src"
)

func init() {
	register("shapes", check{
		Needs: "the clones and helm",
		What:  "**`wiki/coverage.md`'s table of how many deployments each CR kind appears in** - rendered per deployment and recounted. Those counts are the claim: `Indexer` at 0 of 7 is what tells a reader the material describes it off the schema and nobody here has seen one",
		Run:   runShapes,
	})
}

// deployments are the reference clones this page counts, in the order
// `source/SOURCES.md` lists them. The skills repository is excluded and the
// page says why: it holds runtime skills and no CRs.
var deployments = []string{
	"unitech-e-asgard-kube", "xxentria-asgard-kube", "finance-ai-asgard-kube",
	"buy123-asgard-kube", "asgard-freyr-kube", "asgard-auto-post-kube",
	"asgard-industry-demo-generator",
}

// chartDirs finds every chart in a clone, across all three layouts the set
// uses. A layout this misses reports a deployment as having no kinds at all,
// which reads as a real 0 - the same silence that hid a 105-CR chart from
// `hack/verify-references.sh`.
func chartDirs(repo string) []string {
	var out []string
	for _, pattern := range []string{"projects/*/chart", "tenants/*/chart", "chart", "*/chart"} {
		paths, _ := filepath.Glob(filepath.Join(repo, pattern))
		for _, p := range paths {
			if fi, err := os.Stat(filepath.Join(p, "app")); err == nil && fi.IsDir() {
				out = append(out, p)
			}
		}
	}
	sort.Strings(out)
	return out
}

var kindLine = regexp.MustCompile(`(?m)^kind:\s*([A-Za-z]+)\s*$`)

// kindsIn renders every chart in a clone and returns the set of Asgard kinds it
// declares. ConfigMap is not an Asgard CR and the page does not count it.
func kindsIn(repo string) (map[string]bool, int, error) {
	kinds := map[string]bool{}
	charts := chartDirs(repo)
	for _, chart := range charts {
		values, err := referenceValues(chart)
		if err != nil {
			return nil, 0, err
		}
		out := helmRender(filepath.Join(chart, "app"), values)
		for _, m := range kindLine.FindAllStringSubmatch(out, -1) {
			if m[1] == "ConfigMap" || m[1] == "Secret" {
				continue
			}
			kinds[m[1]] = true
		}
	}
	return kinds, len(charts), nil
}

// tableRow reads "| Kind | **3 of 7** | ..." and the slash-joined form the page
// uses for kinds that always appear together.
var tableRow = regexp.MustCompile(`(?m)^\|\s*\**([A-Za-z][A-Za-z /]*?)\**\s*\|\s*\**(\d+) of (\d+)\**\s*\|`)

func runShapes(args []string) error {
	base, err := src.Resolve("deployments")
	if err != nil {
		return err
	}
	root, err := src.Root()
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("helm"); err != nil {
		return fmt.Errorf("helm is not on PATH; this renders every reference chart")
	}

	perKind := map[string]int{}
	present := 0
	for _, d := range deployments {
		repo := filepath.Join(base, d)
		if fi, err := os.Stat(repo); err != nil || !fi.IsDir() {
			fmt.Printf("  (%s is not cloned; the counts below are against %d deployments)\n", d, present)
			continue
		}
		present++
		kinds, charts, err := kindsIn(repo)
		if err != nil {
			return err
		}
		if len(kinds) == 0 {
			return fmt.Errorf("%s rendered no Asgard CR at all - a chart layout this does not know reads as a real zero", d)
		}
		_ = charts
		for k := range kinds {
			perKind[k]++
		}
	}

	data, err := os.ReadFile(filepath.Join(root, "internal/corpus/wiki/coverage.md"))
	if err != nil {
		return err
	}

	var bad []string
	rows := 0
	stated := map[string]bool{}
	for _, m := range tableRow.FindAllStringSubmatch(string(data), -1) {
		names := strings.Split(m[1], "/")
		says, of := atoi(m[2]), atoi(m[3])
		if of != present {
			bad = append(bad, fmt.Sprintf("  the table counts out of %d and %d deployment(s) are here", of, present))
			continue
		}
		for _, n := range names {
			n = strings.TrimSpace(n)
			if n == "" {
				continue
			}
			rows++
			stated[n] = true
			if perKind[n] != says {
				bad = append(bad, fmt.Sprintf("  %s: the page says %d of %d, and it renders in %d",
					n, says, of, perKind[n]))
			}
		}
	}
	// **The other direction.** A kind that turns up in a chart and in no row is
	// a shape nobody decided about, which is the state every finding came from.
	for _, k := range sortedKeys(perKind) {
		if !stated[k] {
			bad = append(bad, fmt.Sprintf("  %s renders in %d deployment(s) and has no row", k, perKind[k]))
		}
	}

	sort.Strings(bad)
	for _, b := range bad {
		fmt.Println(b)
	}
	fmt.Printf("\n%d kind(s) counted across %d deployment(s), %d disagreement(s).\n", rows, present, len(bad))
	if len(bad) > 0 {
		return errFailed
	}
	return nil
}
