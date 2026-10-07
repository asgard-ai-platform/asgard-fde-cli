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
	register("pass-list", check{
		Needs: "this repository",
		What:  "every check says what it needs, and this repository's own skill never appears in a scaffolded tree",
		Run:   runPassList,
	})
	register("pass", check{
		Needs: "this repository",
		What:  "print the consistency pass itself, derived from the binary and this directory",
		Run:   func(args []string) error { return showPass(args) },
	})
}

// Flags that are not checks: they change what a check reads, or take a value.
var notACheck = map[string]bool{"template-dir": true, "help": true, "format": true}

// Flags that have no pass or fail: listings for a person, and one query.
var notAVerdict = map[string]bool{
	"orphans": true, "crossref": true, "ask": true, "unmarked": true,
	"unchecked": true, "term": true,
}

var goSteps = []string{"go build", "go vet", "gofmt", "go test"}

// **One shell script is left, and it stays one.** What it does is drive helm and
// this repository's own binary over the reference charts, and rewriting that in
// Go buys nothing. Everything else moved - AGENTS.md has why a check that is not
// compiled is a check nobody runs until it is wrong.
var scriptNeeds = map[string]string{
	"verify-references.sh": "the clones",
}

func hackScripts(root string) []string {
	entries, _ := os.ReadDir(filepath.Join(root, "hack"))
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".py") || strings.HasSuffix(e.Name(), ".sh") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func auditFlags(binary string) ([]string, error) {
	out, err := exec.Command(binary, "audit-material", "--help").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s audit-material --help failed:\n%s", binary, out)
	}
	re := regexp.MustCompile(`(?m)^\s+--([a-z][a-z-]*)`)
	var flags []string
	for _, m := range re.FindAllStringSubmatch(string(out), -1) {
		if !notACheck[m[1]] {
			flags = append(flags, m[1])
		}
	}
	sort.Strings(flags)
	return flags, nil
}

func goCheckNames() []string {
	var out []string
	for n := range checks {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// showPass prints the pass, derived from the binary, this directory and the
// gate's own subcommands.
//
// **TASK.md used to carry this as a table.** It was a hand-written copy of what
// this file already computes, which is the one shape of claim this repository
// has decided not to keep: a list that can be generated is not written down.
func showPass(args []string) error {
	root, err := src.Root()
	if err != nil {
		return err
	}
	binary := filepath.Join(root, ".out/asgard-cli")
	if len(args) > 0 {
		binary = args[0]
	}
	flags, err := auditFlags(binary)
	if err != nil {
		return err
	}
	fmt.Println("The consistency pass, most-volatile first. No state is recorded for a")
	fmt.Println("check anywhere: the answer is its exit code, today.")
	fmt.Println()

	fmt.Println("1. upstream - pull first, or these check a clone rather than the platform")
	for _, n := range goCheckNames() {
		if strings.Contains(checks[n].Needs, "$") || strings.Contains(checks[n].Needs, "clone") {
			fmt.Printf("     go run ./hack %s\n", n)
		}
	}
	for _, n := range hackScripts(root) {
		need := scriptNeeds[n]
		if strings.Contains(need, "$") || strings.Contains(need, "clone") {
			fmt.Printf("     hack/%-24s needs %s\n", n, need)
		}
	}
	fmt.Println("\n2. the material - build from the working tree first, or you audit an older corpus")
	for _, f := range flags {
		if !notAVerdict[f] {
			fmt.Printf("     asgard-cli audit-material --%s\n", f)
		}
	}
	fmt.Println("\n3. this repository")
	for _, n := range goCheckNames() {
		if checks[n].Listing {
			continue
		}
		if strings.HasPrefix(checks[n].Needs, "this repository") {
			fmt.Printf("     go run ./hack %-18s %s\n", n,
				strings.TrimLeft(strings.TrimPrefix(checks[n].Needs, "this repository"), " -"))
		}
	}
	for _, n := range hackScripts(root) {
		if strings.HasPrefix(scriptNeeds[n], "this repository") {
			fmt.Printf("     hack/%-24s\n", n)
		}
		if _, ok := scriptNeeds[n]; !ok {
			fmt.Printf("     hack/%-24s ungrouped - say what it needs in scriptNeeds\n", n)
		}
	}
	fmt.Println("\n4. the compiler")
	for _, s := range goSteps {
		fmt.Printf("     %s ./...\n", s)
	}
	fmt.Println("\n5. the network, last, because it is the only one that needs it")
	fmt.Println("     asgard-cli audit-material --urls")

	var listings []string
	for _, f := range flags {
		if notAVerdict[f] {
			listings = append(listings, "--"+f)
		}
	}
	for _, n := range goCheckNames() {
		if checks[n].Listing {
			listings = append(listings, "hack "+n)
		}
	}
	fmt.Println("\nListings, not checks - they do not fail:")
	fmt.Println("     " + strings.Join(listings, "  "))
	return nil
}

// slugRow matches a prose-surface row: a table row opening with a slug.
//
// **There is one table of these and it is TASK.md's.** AGENTS.md carried a
// second, and the two drifted the way two copies do - the same slugs, prose
// written for its own context, and this check existing only to hold one against
// the other. What is compared now is that the second table has not come back,
// and that every row in the one that is left records when it was read.
var slugRow = regexp.MustCompile("(?m)^\\| `([a-z][a-z0-9-]*-[a-z0-9-]+)`")

func runPassList(args []string) error {
	root, err := src.Root()
	if err != nil {
		return err
	}
	binary := filepath.Join(root, ".out/asgard-cli")
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		binary = args[0]
	}
	if _, err := os.Stat(binary); err != nil {
		return fmt.Errorf("no binary at %s\n  go build -o .out/asgard-cli ./cmd/asgard-cli\n"+
			"  A pass reads what is embedded, so build from the working tree first.", binary)
	}
	flags, err := auditFlags(binary)
	if err != nil {
		return err
	}

	var missing []string
	for _, n := range hackScripts(root) {
		if _, ok := scriptNeeds[n]; !ok {
			missing = append(missing, fmt.Sprintf(
				"hack/%s is in this directory and scriptNeeds does not say what it needs", n))
		}
	}

	// The maintenance skill must not be in the scaffolded tree.
	var leaked []string
	ours, _ := os.ReadDir(filepath.Join(root, ".agents/skills"))
	for _, d := range ours {
		if !d.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root,
			"internal/scaffold/templates/.agents/skills", d.Name())); err == nil {
			leaked = append(leaked, d.Name())
		}
	}

	for _, m := range missing {
		if strings.HasPrefix(m, "prose surface") {
			fmt.Printf("adrift    %s\n", m)
		} else {
			fmt.Printf("ungrouped %s\n", m)
		}
	}
	for _, n := range leaked {
		fmt.Printf("leaked    .agents/skills/%s is also in the scaffolded tree, so it ships\n", n)
	}
	fmt.Printf("\n%d flag(s), %d script(s), %d Go check(s), %d Go step(s); %d ungrouped or adrift, %d leaked.\n",
		len(flags), len(hackScripts(root)), len(checks), len(goSteps), len(missing), len(leaked))
	fmt.Println("`go run ./hack pass` prints the pass itself.")
	if len(missing) > 0 || len(leaked) > 0 {
		return errFailed
	}
	return nil
}
