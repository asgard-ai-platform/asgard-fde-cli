package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/asgard-ai-platform/asgard-fde-cli/hack/internal/src"
)

func init() {
	register("introduced", check{
		Needs:   "this repository",
		What:    "every count-shaped claim THIS change adds, for a person to read. **It cannot fail** - most numbers in an added line are correct - and its whole value is that its scope is the diff rather than somebody's memory",
		Listing: true,
		Run:     runIntroduced,
	})
}

// countShaped is a number, or a number written as a word, within a word or two
// of a plural noun.
//
// **Deliberately loose, and deliberately not a check.** Run over the whole
// corpus it reports more than a thousand lines, almost all of them correct -
// which is the shape of a rule AGENTS.md says to delete rather than soften.
// Run over the lines one change adds it reports a few dozen, and that is a
// thing a person reads.
var countShaped = regexp.MustCompile(`(?i)\b(one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|thirteen|fourteen|fifteen|sixteen|seventeen|eighteen|nineteen|twenty|twenty-\w+|thirty|forty|fifty|sixty|seventy|eighty|ninety|hundred|\d{1,4})\s+(?:\w+\s+)?([a-z]{3,}s)\b`)

// aDate is a mask rather than a veto, which the other three are not.
//
// **A date cannot be read as a count, and its digits can**: the pattern needs
// whitespace after a number, so `2026-09-14` never matches on its own, and
// `2026-09-14 pages` matches as "14 pages". Masking the date takes that out
// and leaves the rest of the line alone.
//
// **Vetoing the whole line took out the one place the forbidden shape lives.**
// A count annotating a source is what AGENTS.md says never to write, and every
// line that annotates a source carries the date it was read at - so a dated
// line is the last one this listing should be blind to.
var aDate = regexp.MustCompile(`\b20\d{2}-\d{2}-\d{2}\b`)

// Links, commits, and quotas quoted with their unit. A line that is one of
// these says nothing about a set that can move.
var notAClaim = []*regexp.Regexp{
	regexp.MustCompile(`https?://`),
	regexp.MustCompile("`[0-9a-f]{7,12}`"),
	regexp.MustCompile(`(?i)\b\d+\s*(ms|s|m|h|kb|mb|gb|bytes?|chars?|characters?|seconds?|minutes?|hours?|days?)\b`),
}

// runIntroduced lists the count-shaped lines a change adds.
//
// **The scope is the diff, which is the point.** Deciding by hand which files
// to sweep is the same mistake as writing down a list that can be generated:
// the set somebody remembers is the set they have been editing, which is not
// where the other copy is. Existing counts are a backlog found by reading, and
// a backlog closes: the corpus is a countable number of documents, and one
// reviewed at a recorded digest does not come back until it changes. What made
// it look endless was reviewing from zero every time. The ones a change ADDS
// are the regression, and this is what keeps the backlog from refilling behind
// the reading.
//
// AGENTS.md's "what YOU just introduced" is the item that gets skipped, because
// it is not a surface anybody listed - it is a surface created in the last ten
// minutes. This is that item, run rather than remembered.
func runIntroduced(args []string) error {
	root, err := src.Root()
	if err != nil {
		return err
	}
	base := "HEAD"
	if len(args) > 0 {
		base = args[0]
	}
	out, err := exec.Command("git", "-C", root, "diff", "-U0", base).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git diff %s failed:\n%s", base, out)
	}

	file, shown, added := "", 0, 0
	var header bool
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "+++ b/"):
			file, header = strings.TrimPrefix(line, "+++ b/"), false
			continue
		case !strings.HasPrefix(line, "+") || strings.HasPrefix(line, "+++"):
			continue
		}
		text := strings.TrimPrefix(line, "+")
		added++
		claim := aDate.ReplaceAllString(text, " ")
		if !countShaped.MatchString(claim) {
			continue
		}
		skip := false
		for _, re := range notAClaim {
			if re.MatchString(claim) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		if !header {
			fmt.Printf("\n%s\n", file)
			header = true
		}
		trimmed := strings.TrimSpace(text)
		if len(trimmed) > 110 {
			trimmed = trimmed[:107] + "..."
		}
		fmt.Printf("  %s\n", trimmed)
		shown++
	}

	fmt.Printf("\n%d line(s) added against %s, %d of them count-shaped.\n", added, base, shown)
	if shown == 0 {
		return nil
	}
	fmt.Println("\nFor each: is the count the claim, or is it standing in for a yes?")
	fmt.Println("A count earns its place when somebody acts on the number. It does not")
	fmt.Println("when the set is enumerated in the same sentence, or when a check, the")
	fmt.Println("CRD or the binary already owns it - reference that instead.")
	fmt.Println("\nThis cannot fail, and a clean run is not a verdict: it says only that")
	fmt.Println("nothing count-shaped was added, never that what was added is right.")
	return nil
}
