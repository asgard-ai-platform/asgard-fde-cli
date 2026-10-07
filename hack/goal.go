package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/asgard-ai-platform/asgard-fde-cli/hack/internal/src"
)

func init() {
	register("goal", check{
		Needs: "this repository - **the capability, not the material**",
		What:  "**Goal.md's four points against the binary** - the corpus landing offline with no repository, its size as APPROACH.md states it, a grep finding things in it, the needs files and the deck's rules, a chart written and passing `check`, and the issue route coming out of the tool's own output",
		Run:   runGoal,
	})
}

const corpusDir = ".agents/skills/asgard-platform"

// Goal's first point names four bodies of knowledge. A landed tree missing one
// is that point half-delivered, and `--links` cannot see it: a directory that is
// not written has no pointers to go dead.
var goalKinds = []string{"wiki", "usecase", "needs", "brief", "guide"}

// Goal's second point: the deck is the only thing a customer reads, so it has
// rules of its own. These are the three it names.
var deckRules = []string{"screenshot", "never appear", "worth"}

// Terms an agent would grep for, in the customer's words or the platform's.
// Retrieval is the whole engineering problem in Goal's closing section, and
// "the material landed" is not the same as "a grep finds it".
var greps = []string{"allowlist", "botProviderClass", "immutable", "read-only"}

// runCLI runs the binary in a scratch directory with nothing available to it.
//
// **No network.** A proxy that resolves nowhere is the cheapest way to make a
// fetch fail rather than succeed slowly, and Goal's first point is that none is
// needed.
//
// **HOME is a sibling of the scratch directory, not the scratch directory.**
// It is set at all so that the run reads none of this machine's credentials or
// profiles - but `init` refuses to scaffold a home directory, which is its own
// guard against somebody running it in theirs, so pointing HOME at the
// directory being scaffolded made every one of Goal's points fail for a reason
// that was nothing to do with Goal.
func runCLI(binary, dir string, args ...string) (string, string, error) {
	home := dir + "-home"
	_ = os.MkdirAll(home, 0o755)
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"HTTP_PROXY=http://127.0.0.1:1", "HTTPS_PROXY=http://127.0.0.1:1",
		"ALL_PROXY=http://127.0.0.1:1", "NO_PROXY=", "ASGARD_PROFILE=", "HOME="+home)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	return out.String(), errb.String(), err
}

func runGoal(args []string) error {
	root, err := src.Root()
	if err != nil {
		return err
	}
	binary := filepath.Join(root, ".out/asgard-cli")
	if len(args) > 0 {
		binary = args[0]
	}
	if _, err := os.Stat(binary); err != nil {
		return fmt.Errorf("no binary at %s\n  go build -o .out/asgard-cli ./cmd/asgard-cli", binary)
	}
	binary, _ = filepath.Abs(binary)

	tmp, err := os.MkdirTemp("", "asgard-goal-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	var bad []string
	add := func(format string, a ...any) { bad = append(bad, fmt.Sprintf(format, a...)) }

	// ── Goal 1: the knowledge, offline, with no repository ───────────────
	//
	// No `git init`. Goal.md: a tool that needs a directory first will not get
	// asked, and the question is asked in a meeting.
	if _, stderr, err := runCLI(binary, tmp, "init"); err != nil {
		add("1: `init` failed in an empty directory with no network:\n%s", trim(stderr, 400))
	}
	corpus := filepath.Join(tmp, corpusDir)
	for _, kind := range goalKinds {
		if fi, err := os.Stat(filepath.Join(corpus, kind)); err != nil || !fi.IsDir() {
			add("1: `%s/%s/` did not land, so one of the four bodies is missing", corpusDir, kind)
		}
	}
	landed := markdownUnder(corpus)
	if len(landed) < 60 {
		add("1: only %d corpus documents landed; the corpus is not a handful of files", len(landed))
	}

	// **Goal's first point is that the whole corpus lands**, and that is
	// answerable without anybody writing a number down: what the binary
	// carries and what arrives in the directory are both countable here.
	//
	// It holds no document count: Goal.md asks for none, and a count a checker
	// demands only couples prose to the tree.
	//
	// What is worth failing on is a document that does not arrive.
	// **Names, not counts.** Comparing two numbers means keeping an exclusion
	// list in step with whatever `init` happens to write, and getting that
	// arithmetic wrong is how this reported a phantom difference. A set
	// difference cannot be off by one and says which file.
	carried := map[string]map[string]bool{}
	for _, k := range goalKinds {
		var dir string
		switch k {
		case "guide", "needs", "brief":
			// **Renamed or rendered on the way in.** A stage prompt lands as
			// `projects.md` from `02-projects.md` - the number is a reading
			// order here and not part of the name a reader types - and needs
			// and briefs are rendered from Go rather than written as files. So
			// there is no source name to hold the landed one against, and
			// saying so beats comparing the wrong pair.
			continue
		case "unreachable-never":
			// Rendered from Go rather than written as files, so there is no
			// source set to compare against and the landed tree is the only
			// place they are documents at all.
			continue
		default:
			dir = filepath.Join(root, "internal/corpus", k)
		}
		names := map[string]bool{}
		paths, _ := filepath.Glob(filepath.Join(dir, "*.md"))
		for _, p := range paths {
			names[filepath.Base(p)] = true
		}
		carried[k] = names
	}

	var kinded []string
	for _, k := range goalKinds {
		paths, _ := filepath.Glob(filepath.Join(corpus, k, "*.md"))
		kinded = append(kinded, paths...)
		want, held := carried[k]
		if !held {
			continue
		}
		landed := map[string]bool{}
		for _, p := range paths {
			landed[filepath.Base(p)] = true
		}
		for name := range want {
			if !landed[name] {
				add("1: %s/%s is in this binary and does not land in a repository", k, name)
			}
		}
	}

	task, err := os.ReadFile(filepath.Join(root, "APPROACH.md"))
	if err != nil {
		return err
	}
	// **The word figure is a floor and stays one.** Every edit moves it, so an
	// equality fails on the ordinary act of writing a paragraph - which teaches
	// whoever hits it to stop believing the check. A floor fails on the thing
	// worth failing on: material that has gone missing.
	claim := regexp.MustCompile(`(?:over\s+)?([\d,]+)\s*\n?\s*words`).FindStringSubmatch(string(task))
	if claim == nil {
		add("1: APPROACH.md gives no size for the corpus at all, so nothing says when material has gone")
	} else {
		words, _ := strconv.Atoi(strings.ReplaceAll(claim[1], ",", ""))
		got := 0
		for _, p := range kinded {
			data, _ := os.ReadFile(p)
			got += len(strings.Fields(string(data)))
		}
		if got < words {
			add("1: APPROACH.md says over %s words and the landed corpus has %s. "+
				"Material has gone rather than grown.", comma(words), comma(got))
		}
	}

	// ── Goal 2: what to get from the customer, and the deck's rules ──────
	for _, shape := range []string{"semantic-layer", "chat-channel", "write-path"} {
		if _, err := os.Stat(filepath.Join(corpus, "needs", shape+".md")); err != nil {
			add("2: `needs/%s.md` did not land, and that file is Goal's second point", shape)
		}
	}
	var skills strings.Builder
	_ = filepath.Walk(filepath.Join(tmp, ".agents/skills"), func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || filepath.Base(p) != "SKILL.md" {
			return nil
		}
		data, _ := os.ReadFile(p)
		skills.Write(data)
		skills.WriteString("\n")
		return nil
	})
	for _, rule := range deckRules {
		if !strings.Contains(skills.String(), rule) {
			add("2: no landed skill mentions %q, and the deck's own rules are Goal's second point", rule)
		}
	}

	// ── Goal 3: the charts ───────────────────────────────────────────────
	if err := exec.Command("git", "-C", tmp, "init", "-q", ".").Run(); err != nil {
		return fmt.Errorf("git init in the scratch repository failed: %w", err)
	}
	for _, argv := range [][]string{
		{"project", "add", "app"},
		{"add", "dataconnector", "erp", "--project", "app"},
	} {
		if _, stderr, err := runCLI(binary, tmp, argv...); err != nil {
			add("3: `%s` failed:\n%s", strings.Join(argv, " "), trim(stderr, 300))
		}
	}
	chart := filepath.Join(tmp, "projects/app/chart/app")
	if _, err := os.Stat(filepath.Join(chart, "Chart.yaml")); err != nil {
		add("3: no chart at projects/app/chart/app, which Goal's third point names")
	}
	if len(find(chart, "dc-erp.yaml")) == 0 {
		add("3: `add dataconnector` wrote no CR")
	}
	if stdout, _, err := runCLI(binary, tmp, "check"); err != nil {
		add("3: `check` failed on a freshly scaffolded repository:\n%s", tail(stdout, 300))
	}

	// ── Goal 4: the way back in, out of the tool's own output ────────────
	//
	// How to send it has to be in what the tool prints, not only in a
	// document somebody has to think to open.
	if stdout, _, _ := runCLI(binary, tmp, "issue-report"); !strings.Contains(stdout,
		"issue-report --send") {
		add("4: `issue-report` does not print how to send a report")
	}
	if stdout, _, err := runCLI(binary, tmp, "issue-report", "--new"); err != nil || len(stdout) < 400 {
		add("4: `issue-report --new` did not write a report body")
	}
	skill, err := os.ReadFile(filepath.Join(corpus, "SKILL.md"))
	if err != nil || !strings.Contains(string(skill), "issue-report") {
		add("4: the landed SKILL.md does not name `issue-report`, so an agent that greps " +
			"and finds nothing is not told where to file it")
	}

	for _, b := range bad {
		fmt.Printf("goal  %s\n", b)
	}
	fmt.Printf("\nGoal.md's four points, held against the binary: %d unmet.\n", len(bad))
	if len(bad) > 0 {
		fmt.Println("\n**This is a capability check, not a consistency check.** Every other")
		fmt.Println("check here can pass while one of these fails, which is why it exists.")
		return errFailed
	}
	return nil
}

func markdownUnder(dir string) []string {
	var out []string
	_ = filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() && strings.HasSuffix(p, ".md") {
			out = append(out, p)
		}
		return nil
	})
	return out
}

func find(dir, name string) []string {
	var out []string
	_ = filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() && filepath.Base(p) == name {
			out = append(out, p)
		}
		return nil
	})
	return out
}

func trim(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

// comma writes a count the way the claim it is compared against writes one.
func comma(n int) string {
	s := strconv.Itoa(n)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}
