package extension

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/gate"
)

// helmSentinel stands in for every template action in an example. It is a
// plain word so the YAML around it still parses as a string, and it is unusual
// enough that a finding quoting it can be recognised as being about the
// stand-in rather than about the example.
const helmSentinel = "__helm__"

var (
	// actionLine is a line that is nothing but a template action: an include
	// that writes a block of labels, a `with`, an `end`. Each one writes YAML
	// that only the FDE repository's chart can produce, so the line is dropped
	// rather than replaced - a sentinel standing alone in a mapping would not
	// parse.
	actionLine = regexp.MustCompile(`^\s*\{\{-?.*-?\}\}\s*$`)
	// action is one template action inside a line.
	action = regexp.MustCompile(`\{\{-?.*?-?\}\}`)
	// placeholder is one of the <…> blanks an example leaves for the FDE
	// repository to fill: <chart>, <project>, <fde-repo-git-url>.
	placeholder = regexp.MustCompile(`<[a-z][a-z0-9-]*>`)
)

// neutralize makes a pseudo chart's template parse as YAML: lines that are only
// a template action go, and every other action becomes helmSentinel.
//
// **It is a stand-in, not a render.** What a value really is depends on the
// FDE repository's values and helpers, so nothing here can say whether it is
// right; the checks that run afterwards ignore any finding about the stand-in.
func neutralize(src []byte) []byte {
	lines := strings.Split(string(src), "\n")
	out := lines[:0]
	for _, line := range lines {
		if actionLine.MatchString(line) {
			continue
		}
		out = append(out, action.ReplaceAllString(line, helmSentinel))
	}
	return []byte(strings.Join(out, "\n"))
}

// aboutStandIn reports whether a finding is about a value the example leaves
// to the FDE repository rather than about the example.
func aboutStandIn(msg string) bool {
	return strings.Contains(msg, helmSentinel) || placeholder.MatchString(msg)
}

// CheckExamples reads every CR under examples/ and holds each against the
// checks `asgard-cli gate` runs on one rendered CR at a time: processor types,
// their required configs and branches, the CRDs' enums, patterns and bounds,
// and their one-of rules. It returns what it found and how many CRs it read.
//
// **What it leaves out is everything that needs the rest of a chart.** A
// reference to a CompletionModel, a Secret's name, a Workflow's set labels and
// the declaration's keys all live in the FDE repository the example is copied
// into, and checking them here would report every example as broken. Those
// are found by transcribing the example into a test FDE repository and running
// `asgard-cli gate` there.
func CheckExamples(dir string) ([]Finding, int) {
	root := filepath.Join(dir, "examples")
	var findings []Finding
	crs := 0
	add := func(level Level, path, msg string) {
		findings = append(findings, Finding{Level: level, Path: path, Message: msg})
	}

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			// A runtime skill's files are Markdown and scripts that a Syncer
			// copies, not CRs.
			if runtimeSkillDir(rel) {
				return fs.SkipDir
			}
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".yaml", ".yml":
		default:
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			add(Warning, rel, fmt.Sprintf("cannot read it: %v", err))
			return nil
		}

		docs, err := gate.Read(strings.NewReader(string(neutralize(data))))
		if err != nil {
			// Warned rather than failed: a template with a range or an if
			// spread over several lines is a legitimate example that the
			// stand-in cannot make parse.
			add(Warning, rel, fmt.Sprintf("not checked: with its template actions replaced it still does not parse as YAML (%v)", err))
			return nil
		}
		crs += len(docs)

		for _, r := range []gate.Result{
			gate.Processors(docs, gate.Options{}),
			gate.Enums(docs, gate.Options{}),
			gate.Constraints(docs, gate.Options{}),
			gate.Shapes(docs, gate.Options{}),
		} {
			for _, p := range r.Problems {
				if !aboutStandIn(p) {
					add(Error, rel, p)
				}
			}
			for _, w := range r.Warnings {
				if !aboutStandIn(w) {
					add(Warning, rel, w)
				}
			}
		}
		return nil
	})
	return findings, crs
}

// runtimeSkillDir reports whether rel is examples/<set>/assets/skills or under it.
func runtimeSkillDir(rel string) bool {
	parts := strings.Split(rel, "/")
	return len(parts) >= 4 && parts[0] == "examples" && parts[2] == "assets" && parts[3] == "skills"
}
