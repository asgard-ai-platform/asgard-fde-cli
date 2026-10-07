package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/platform"
)

// A tag moves the version labels on every resource, so most updates of a
// release are nothing else. The platform says which ones, and the report folds
// them into one line unless --version-bumps asks for each.
func TestPrintRunFoldsVersionBumps(t *testing.T) {
	report := func(res ...*platform.ResourceDiff) *platform.Run {
		return &platform.Run{Number: 7, State: platform.RunAwaitingReview, RunId: "r1", Report: &platform.PlanReport{
			PreviousAppVersion: "0.1.2",
			AppVersion:         "0.1.4",
			Variables:          []*platform.VariableDiff{{Kind: "secret", Key: "pw", Change: "unchanged"}},
			Resources:          res,
		}}
	}
	bump := func(name string) *platform.ResourceDiff {
		return &platform.ResourceDiff{Kind: "Agent", Name: name, Change: "update", UpdateCause: platform.UpdateCauseVersionBump}
	}
	content := &platform.ResourceDiff{Kind: "Workflow", Name: "wf-mail", Change: "update", UpdateCause: "content"}
	legacy := &platform.ResourceDiff{Kind: "Workflow", Name: "wf-old", Change: "update"}

	cases := []struct {
		name         string
		run          *platform.Run
		versionBumps bool
		want         []string
		not          []string
	}{
		{
			name: "bumps fold into one line, content is listed",
			run:  report(content, bump("ag-a"), bump("ag-b")),
			want: []string{
				"  update       Workflow                 wf-mail",
				"  version bump 2 resources, version labels only  0.1.2 -> 0.1.4  (--version-bumps lists them)",
			},
			not: []string{"ag-a", "only version labels change"},
		},
		{
			name:         "--version-bumps lists each",
			run:          report(content, bump("ag-a")),
			versionBumps: true,
			want:         []string{"  version bump Agent                    ag-a"},
			not:          []string{"version labels only"},
		},
		{
			name: "nothing but bumps says so, and review is still required",
			run:  report(bump("ag-a")),
			want: []string{
				"plan: only version labels change (0.1.2 -> 0.1.4); review is still required",
				"  version bump 1 resource, version labels only  0.1.2 -> 0.1.4",
			},
		},
		{
			name: "an update with no cause is a plain update",
			run:  report(legacy),
			want: []string{"  update       Workflow                 wf-old"},
			not:  []string{"version"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			printRun(&buf, c.run, c.versionBumps)
			got := buf.String()
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in:\n%s", w, got)
				}
			}
			for _, n := range c.not {
				if strings.Contains(got, n) {
					t.Errorf("unexpected %q in:\n%s", n, got)
				}
			}
		})
	}
}

// A report with no versions still folds, it just cannot name them.
func TestPrintRunVersionBumpsWithoutVersions(t *testing.T) {
	var buf bytes.Buffer
	printRun(&buf, &platform.Run{Report: &platform.PlanReport{Resources: []*platform.ResourceDiff{
		{Kind: "Agent", Name: "a", Change: "update", UpdateCause: platform.UpdateCauseVersionBump},
		{Kind: "Agent", Name: "b", Change: "create"},
	}}}, false)
	if got := buf.String(); !strings.Contains(got, "  version bump 1 resource, version labels only  (--version-bumps lists them)") {
		t.Errorf("output:\n%s", got)
	}
}
