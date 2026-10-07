package scaffold_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/generate"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/scaffold"
)

// **`--force` must leave a repository that still renders.**
//
// This is the test that was missing. `.asgard-pipeline.yaml` was made an
// accumulator with a test asserting the accumulator set - the mechanism, not
// the outcome - and `projects/<slug>/chart/app/values.yaml` was not on it.
// `asgard-cli add` appends the keys each CR reads to that file, so after one
// `add` the templates beside it depend on what is in it; regenerating it left a
// chart reading `.Values.dbDB.host` with the block gone, which `helm template`
// fails on with a nil pointer.
//
// `asgard-cli check` reported `ok` throughout, because the structure was
// intact. The failure is one command further on, which is exactly why this
// asserts what survives rather than what the list contains.
func TestForceLeavesWhatAddWroteIntoTheChart(t *testing.T) {
	root := t.TempDir()
	if _, err := scaffold.Write(root, nil, false); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := scaffold.Write(root, []string{"erp"}, false); err != nil {
		t.Fatalf("project add: %v", err)
	}

	var dc generate.Kind
	for _, k := range generate.Kinds {
		if k.Name == "dataconnector" {
			dc = k
		}
	}
	if dc.Name == "" {
		t.Fatal("no dataconnector kind")
	}
	if _, err := generate.Write(root, dc, generate.Options{Project: "erp", Name: "db"}); err != nil {
		t.Fatalf("add dataconnector: %v", err)
	}

	values := filepath.Join(root, "projects", "erp", "chart", "app", "values.yaml")
	before, err := os.ReadFile(values)
	if err != nil {
		t.Fatalf("read values: %v", err)
	}
	if !strings.Contains(string(before), "dbDB") {
		t.Fatalf("add wrote no values block, so this test proves nothing:\n%s", before)
	}

	if _, err := scaffold.Write(root, []string{"erp"}, true); err != nil {
		t.Fatalf("init --force: %v", err)
	}

	after, err := os.ReadFile(values)
	if err != nil {
		t.Fatalf("read values after force: %v", err)
	}
	if !strings.Contains(string(after), "dbDB") {
		t.Errorf("--force discarded the values block `add` wrote, and the CR beside it still reads it:\n%s", after)
	}
}

// **`--force` reaches only what this CLI owns outright**, and the two
// directions are one test because getting either wrong is a silent failure.
//
// Overwriting what the engagement wrote loses work with no symptom: the repo
// still renders, `check` still passes, and the diff is buried among dozens of
// files. Refusing to overwrite what the CLI owns is the opposite failure and is
// just as quiet - `gate` reports staleness only for shipped material, so a file
// wrongly left out of `shipped` stops being updated and says nothing.
//
// This asserts by asking `shipped` rather than by listing paths, so a file
// added to the skeleton is covered without editing the test - which is how the
// previous version of this protection went stale. It was a whitelist of names,
// and the second incident took six files, five of which the record never
// claimed and none of which were on it.
func TestForceOverwritesWhatIsShippedAndNothingElse(t *testing.T) {
	root := t.TempDir()
	if _, err := scaffold.Write(root, nil, false); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := scaffold.Write(root, []string{"erp"}, false); err != nil {
		t.Fatalf("project add: %v", err)
	}

	const marker = "ENGAGEMENT WROTE THIS"

	// Every file the scaffold left behind, split the way `shipped` splits it.
	// A managed region is a third case and is excluded: it merges rather than
	// choosing, and `AGENTS.md` is the file that proves that path works.
	var ours, theirs []string
	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		switch {
		case rel == scaffold.StampName, rel == "AGENTS.md", rel == "README.md":
			return nil
		case strings.HasSuffix(rel, ".yaml"), strings.HasSuffix(rel, ".json"),
			strings.HasSuffix(rel, ".tpl"), strings.HasSuffix(rel, ".py"),
			strings.HasSuffix(rel, ".html"), strings.HasSuffix(rel, ".txt"):
			// Appending a marker line to these makes them invalid rather than
			// edited, and what is being tested is whose content survives.
			return nil
		}
		if scaffold.OwnedByCLI(rel) {
			ours = append(ours, rel)
		} else {
			theirs = append(theirs, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ours) == 0 || len(theirs) == 0 {
		t.Fatalf("nothing to compare: %d owned by the CLI, %d by the engagement", len(ours), len(theirs))
	}

	for _, rel := range append(append([]string{}, ours...), theirs...) {
		f := filepath.Join(root, filepath.FromSlash(rel))
		body, readErr := os.ReadFile(f)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if writeErr := os.WriteFile(f, append(body, []byte("\n"+marker+"\n")...), 0o644); writeErr != nil {
			t.Fatal(writeErr)
		}
	}

	if _, err := scaffold.Write(root, []string{"erp"}, true); err != nil {
		t.Fatalf("init --force: %v", err)
	}

	edited := func(rel string) bool {
		body, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		return readErr == nil && strings.Contains(string(body), marker)
	}
	for _, rel := range theirs {
		if !edited(rel) {
			t.Errorf("--force discarded %s, which `shipped` does not claim - "+
				"a difference there is the engagement's work, not drift", rel)
		}
	}
	for _, rel := range ours {
		if edited(rel) {
			t.Errorf("--force left %s, which this CLI owns outright - it would stop "+
				"being updated, and `gate` reports staleness only for shipped material", rel)
		}
	}
}
