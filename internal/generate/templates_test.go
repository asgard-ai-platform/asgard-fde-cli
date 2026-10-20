package generate

import (
	"io/fs"
	"strings"
	"testing"
)

// Every template that renders a Syncer has to opt in to being fired by the
// rollout.
//
// The platform's apply step fires only the Syncers of the release that carry
// `asgard-ai.com/auto-fire-on-rollout: "true"`, and the two labels read nothing
// of each other: `syncer-suspend` stops the scheduler and the runner never
// looks at it. So a suspended Syncer without the opt-in never runs at all -
// silently, because the chart renders, the apiserver accepts the CR and the run
// succeeds. The skills or the drive simply resolve to nothing.
//
// If a future Syncer deliberately runs on its schedule only, it belongs in an
// exception here with the reason - not left to be read as an oversight.
func TestEveryGeneratedSyncerOptsInToTheRollout(t *testing.T) {
	const label = `asgard-ai.com/auto-fire-on-rollout: "true"`

	found := 0
	err := fs.WalkDir(templates, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := fs.ReadFile(templates, path)
		if err != nil {
			return err
		}
		text := string(body)
		if !strings.Contains(text, "kind: Syncer") {
			return nil
		}
		found++
		if !strings.Contains(text, label) {
			t.Errorf("%s renders a Syncer without %s, so nothing will ever run it", path, label)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk templates: %v", err)
	}
	if found == 0 {
		t.Fatal("no template renders a Syncer; this test has stopped checking anything")
	}
	t.Logf("%d Syncer-rendering template(s) checked", found)
}

// Every template that writes the Toolset named by --toolset has to skip it when
// the chart already has one.
//
// A Toolset normally holds several tools, so the second tool generated into the
// same set finds it already there. Writing it again renders two CRs with one
// name, and whichever applies last replaces the other: the tools in the
// discarded copy are gone, with nothing failing. Resolve sets ToolsetExists and
// says only the tool was written, so a template that ignores it also makes that
// note untrue.
func TestEveryToolsetTemplateSkipsAnExistingToolset(t *testing.T) {
	found := 0
	err := fs.WalkDir(templates, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := fs.ReadFile(templates, path)
		if err != nil {
			return err
		}
		text := string(body)
		if !strings.Contains(text, "kind: Toolset") || !strings.Contains(text, "<<.Toolset>>") {
			return nil
		}
		found++
		if !strings.Contains(text, "<<if .ToolsetExists>>") {
			t.Errorf("%s writes the --toolset Toolset without checking .ToolsetExists, so a second tool duplicates it", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk templates: %v", err)
	}
	if found == 0 {
		t.Fatal("no template writes a --toolset Toolset; this test has stopped checking anything")
	}
	t.Logf("%d Toolset-writing template(s) checked", found)
}
