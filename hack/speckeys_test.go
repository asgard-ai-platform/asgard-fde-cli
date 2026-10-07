package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/asgard-ai-platform/asgard-fde-cli/hack/internal/src"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/generate"
)

// **What was wrong was the collection, not the comparison.** `spec-key-gap` ran
// one flag combination per kind, so every key that lives behind a flag it did
// not pass - the `--db-class` class blocks, `--private`, `--toolset`,
// `--supervisor` - was counted as a key `add` never writes: 48 of the 165 it
// reported. That is the expensive direction, because the number is what
// somebody reads before implementing one of them, and the check's own failure
// condition is that the gap closes.
//
// These tests hold the matrix and not the gap. The gap moves whenever a
// reference chart or a template does; what must not move is that a flag with a
// branch behind it is run both ways.

// matrixRuns is every run the check plans, by kind.
func matrixRuns(t *testing.T) map[string][][]string {
	t.Helper()
	root, err := src.Root()
	if err != nil {
		t.Fatal(err)
	}
	matrix, err := addMatrix(root)
	if err != nil {
		t.Fatal(err)
	}
	byKind := map[string][][]string{}
	for _, p := range matrix {
		byKind[p.kind] = append(byKind[p.kind], p.runs...)
	}
	return byKind
}

func runSets(runs [][]string, flag string) (on, off bool) {
	for _, run := range runs {
		if hasFlag(run, flag) {
			on = true
		} else {
			off = true
		}
	}
	return on, off
}

func hasFlag(run []string, flag string) bool {
	for _, a := range run {
		if a == "--"+flag {
			return true
		}
	}
	return false
}

func hasFlagValue(run []string, flag, value string) bool {
	for i, a := range run {
		if a == "--"+flag && i+1 < len(run) && run[i+1] == value {
			return true
		}
	}
	return false
}

// A flag a template branches on has two branches, and a key in either is a key
// `add` writes. Running only one of them is exactly the shape that reported 48
// generated keys as missing.
func TestEveryFlagWithABranchBehindItIsRunBothWays(t *testing.T) {
	root, err := src.Root()
	if err != nil {
		t.Fatal(err)
	}
	flags, err := addFlags(root)
	if err != nil {
		t.Fatal(err)
	}
	byKind := matrixRuns(t)

	checked := 0
	for _, k := range generate.Kinds {
		required := toSet(requiredArgs[k.Name])
		for _, f := range k.Files {
			branch, _, err := fieldsUsed(filepath.Join(root, "internal/generate/templates", f.Template))
			if err != nil {
				t.Fatal(err)
			}
			for field := range branch {
				flag, ok := flags[strings.ToLower(field)]
				if !ok {
					// Chart state - .SkillSets, .ToolsetExists - which the
					// seeded probe covers and the test below holds.
					continue
				}
				if required[flag.name] {
					// Passed on every run by definition: `add` refuses without it.
					continue
				}
				if enumDomain(flag.name) != nil {
					continue // held by value, below
				}
				checked++
				on, off := runSets(byKind[k.Name], flag.name)
				if !on {
					t.Errorf("%s branches on .%s and no planned run passes --%s, so every key\n"+
						"inside that branch would be reported as one `add` never writes",
						f.Template, field, flag.name)
				}
				if !off {
					t.Errorf("%s branches on .%s and every planned run passes --%s, so the keys\n"+
						"in its other branch are never collected", f.Template, field, flag.name)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no template branches on a flag, so this test has stopped checking anything")
	}
	t.Logf("%d flag branch(es) held, across %d kind(s)", checked, len(byKind))
}

// `--db-class` writes a whole class block and there is no branch anywhere in
// the template to find it by: nine values are nine different sets of keys. So
// the values are held one by one, from the generator's own vocabulary, which is
// what stops a class added upstream from reading as a gap.
func TestEveryValueOfAnEnumeratedFlagIsProbed(t *testing.T) {
	byKind := matrixRuns(t)
	var all [][]string
	for _, runs := range byKind {
		all = append(all, runs...)
	}
	for _, flag := range []string{"db-class", "bot-class"} {
		domain := enumDomain(flag)
		if len(domain) == 0 {
			t.Fatalf("--%s has no vocabulary, so nothing below is being enumerated", flag)
		}
		for _, v := range domain {
			found := false
			for _, run := range all {
				if hasFlagValue(run, flag, v) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("no planned run passes --%s %s, so that class's own fields would be\n"+
					"reported as keys `add` never writes", flag, v)
			}
		}
		t.Logf("--%s: %d value(s) probed", flag, len(domain))
	}
}

// The four flags the one-run-per-kind shape was missing, named so that a
// regression says which one rather than only that a count moved.
func TestTheMatrixIsNotOneRunPerKind(t *testing.T) {
	byKind := matrixRuns(t)
	for _, want := range []struct{ kind, flag string }{
		{"skillset", "private"},     // git.auth, and the PAT's secretKeyRef
		{"httptool", "write"},       // the consent-gated write path
		{"flowagent", "supervisor"}, // the four-processor loop's configs.template
		{"flowagent", "toolset"},    // the blueprint's toolsetNames
		{"agent", "layer"},
	} {
		if on, _ := runSets(byKind[want.kind], want.flag); !on {
			t.Errorf("no `add %s` run passes --%s; the keys behind it were 48 of the 165\n"+
				"this check used to report as never written", want.kind, want.flag)
		}
	}
	runs := 0
	for _, r := range byKind {
		runs += len(r)
	}
	if runs <= len(generate.Kinds) {
		t.Errorf("%d run(s) for %d kind(s): the matrix is back to one combination per kind,\n"+
			"which is the defect these tests exist for", runs, len(generate.Kinds))
	}
	t.Logf("%d run(s) across %d kind(s)", runs, len(generate.Kinds))
}

// A branch decided by what the chart already holds is not reachable by a flag,
// so it needs a second project with that thing already in it - and both states
// matter, because an Agent in a chart with no SkillSet writes a comment where
// one with a SkillSet writes managed.skillSetNames.
func TestChartStateBranchesGetASeededProbe(t *testing.T) {
	root, err := src.Root()
	if err != nil {
		t.Fatal(err)
	}
	matrix, err := addMatrix(root)
	if err != nil {
		t.Fatal(err)
	}
	seeded := map[string]bool{}
	bare := map[string]bool{}
	for _, p := range matrix {
		if len(p.seeds) > 0 {
			seeded[p.kind] = true
		} else {
			bare[p.kind] = true
		}
	}
	want := 0
	for _, k := range generate.Kinds {
		for _, f := range k.Files {
			branch, _, err := fieldsUsed(filepath.Join(root, "internal/generate/templates", f.Template))
			if err != nil {
				t.Fatal(err)
			}
			for field := range branch {
				if _, ok := stateSeed[field]; !ok {
					continue
				}
				want++
				if !seeded[k.Name] {
					t.Errorf("%s branches on .%s, which nothing typed sets, and no probe seeds a\n"+
						"project for it - so that branch is never taken", f.Template, field)
				}
				if !bare[k.Name] {
					t.Errorf("%s is only probed in a seeded project, so the keys it writes when the\n"+
						"chart is empty are never collected", f.Template)
				}
			}
		}
	}
	if want == 0 {
		t.Fatal("no template branches on chart state, so this test has stopped checking anything")
	}
	t.Logf("%d chart-state branch(es) held", want)
}
