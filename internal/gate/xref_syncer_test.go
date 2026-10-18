package gate

import (
	"strings"
	"testing"
)

func syncerDoc(name, class, sourceSet, dest string, extra map[string]any) Doc {
	spec := map[string]any{
		"syncerClass":     class,
		"sourceSetName":   sourceSet,
		"destinationPath": dest,
		"schedule":        "0 3 * * 1",
	}
	for k, v := range extra {
		spec[k] = v
	}
	return Doc{Kind: "Syncer", Name: name, Spec: spec,
		Annotations: map[string]string{annotationPrefix + "syncer-name": name}}
}

// **A static-content Syncer writing the Global Directory's AGENTS.md is the
// shape the generator emits**, so it has to pass clean: the SourceSet is the
// platform's and never in the render, and the destination is a file.
func TestStaticContentIntoTheGlobalDirectoryIsClean(t *testing.T) {
	d := syncerDoc("syn-agents-md", "static-content", "preset-agent-hub-global", "AGENTS.md",
		map[string]any{"staticContent": map[string]any{"text": "# rules\n"}})

	if got := joined(Xref([]Doc{d}, Options{})); got != "" {
		t.Errorf("the generated shape was reported:\n%s", got)
	}
	if got := Shapes([]Doc{d}, Options{}); len(got.Warnings) > 0 {
		t.Errorf("the generated shape failed a CEL shape:\n%s", strings.Join(got.Warnings, "\n"))
	}
}

// The destination rule depends on the class, and both halves are checked: the
// folder rule must still hold for every other class.
func TestSyncerDestinationRuleFollowsTheClass(t *testing.T) {
	cases := []struct {
		class, dest, want string
	}{
		{"static-content", "docs/", "must not end in /"},
		{"git", "docs", "must end in /"},
		{"static-content", "../AGENTS.md", ". or .. component"},
	}
	for _, c := range cases {
		d := syncerDoc("syn-x", c.class, "ss-x", c.dest, nil)
		ss := Doc{Kind: "SourceSet", Name: "ss-x", Spec: map[string]any{}}
		if got := joined(Xref([]Doc{d, ss}, Options{})); !strings.Contains(got, c.want) {
			t.Errorf("%s with destinationPath %q: want %q, got:\n%s", c.class, c.dest, c.want, got)
		}
	}
}

// static-content does not take the deprecated spelling, and its state file
// cannot be the file it writes.
func TestStaticContentRefusals(t *testing.T) {
	ss := Doc{Kind: "SourceSet", Name: "ss-x", Spec: map[string]any{}}

	old := syncerDoc("syn-old", "static-content", "ss-x", "", map[string]any{"destinationMemberKey": "AGENTS.md"})
	if got := joined(Xref([]Doc{old, ss}, Options{})); !strings.Contains(got, "does not accept the deprecated spelling") {
		t.Errorf("a static-content Syncer on destinationMemberKey went unreported:\n%s", got)
	}

	same := syncerDoc("syn-same", "static-content", "ss-x", "AGENTS.md", map[string]any{"statePath": "AGENTS.md"})
	if got := joined(Xref([]Doc{same, ss}, Options{})); !strings.Contains(got, "is the file the Syncer writes") {
		t.Errorf("statePath equal to the written file went unreported:\n%s", got)
	}
}

// A chart declaring the platform's SourceSet is overwriting the platform's
// object; one declaring any other SourceSet is ordinary.
func TestChartMustNotDeclareThePlatformSourceSet(t *testing.T) {
	own := Doc{Kind: "SourceSet", Name: "preset-agent-hub-global", Spec: map[string]any{},
		Annotations: map[string]string{annotationPrefix + "source-set-name": "x"}}
	if got := joined(Xref([]Doc{own}, Options{})); !strings.Contains(got, "created by the platform") {
		t.Errorf("a chart declaring the Global Directory's SourceSet went unreported:\n%s", got)
	}
	mine := Doc{Kind: "SourceSet", Name: "ss-docs", Spec: map[string]any{},
		Annotations: map[string]string{annotationPrefix + "source-set-name": "x"}}
	if got := joined(Xref([]Doc{mine}, Options{})); strings.Contains(got, "created by the platform") {
		t.Errorf("an ordinary SourceSet was reported as the platform's:\n%s", got)
	}
}

// The overlap warning fires only for a class that deletes, and once per pair of
// static-content Syncers on one file.
func TestStaticContentOverlap(t *testing.T) {
	ss := Doc{Kind: "SourceSet", Name: "ss-x", Spec: map[string]any{},
		Annotations: map[string]string{annotationPrefix + "source-set-name": "x"}}
	file := syncerDoc("syn-file", "static-content", "ss-x", "repo/AGENTS.md",
		map[string]any{"staticContent": map[string]any{"text": "x"}})

	git := syncerDoc("syn-git", "git", "ss-x", "repo/", nil)
	if got := joined(Xref([]Doc{ss, file, git}, Options{})); !strings.Contains(got, "comes and goes") {
		t.Errorf("a file inside a git Syncer's folder went unreported:\n%s", got)
	}

	db := syncerDoc("syn-db", "database", "ss-x", "repo/", nil)
	if got := joined(Xref([]Doc{ss, file, db}, Options{})); strings.Contains(got, "comes and goes") {
		t.Errorf("a database Syncer deletes nothing, and was reported:\n%s", got)
	}

	twin := syncerDoc("syn-twin", "static-content", "ss-x", "repo/AGENTS.md",
		map[string]any{"staticContent": map[string]any{"text": "y"}})
	got := Xref([]Doc{ss, file, twin}, Options{})
	n := 0
	for _, w := range got.Warnings {
		if strings.Contains(w, "both write") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("two static-content Syncers on one file: want one warning, got %d:\n%s", n, joined(got))
	}
}

// The class implies its block: static-content without staticContent renders,
// lints and is refused at apply.
func TestStaticContentImpliesItsBlock(t *testing.T) {
	d := syncerDoc("syn-x", "static-content", "preset-agent-hub-global", "AGENTS.md", nil)
	got := strings.Join(Shapes([]Doc{d}, Options{}).Warnings, "\n")
	if !strings.Contains(got, "staticContent is absent") {
		t.Errorf("static-content without its block went unreported:\n%s", got)
	}
}
