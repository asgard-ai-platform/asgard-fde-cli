package gate

import (
	"strings"
	"testing"
)

func includePathsDocs(searchPaths []any, includePaths []any) []Doc {
	ss := Doc{Kind: "SourceSet", Name: "ss-x", Spec: map[string]any{},
		Annotations: map[string]string{annotationPrefix + "source-set-name": "x"},
		Labels:      map[string]string{managedByKey: managedBySkillSet}}
	git := map[string]any{"repoUrl": "https://example.com/r.git", "revision": "main"}
	if includePaths != nil {
		git["includePaths"] = includePaths
	}
	syn := syncerDoc("syn-x", "git", "ss-x", "git/", map[string]any{"git": git})
	syn.Labels = map[string]string{managedByKey: managedBySkillSet}
	sk := Doc{Kind: "SkillSet", Name: "sk-x", Spec: map[string]any{"sourceSetName": "ss-x", "searchPaths": searchPaths},
		Annotations: map[string]string{annotationPrefix + "skill-set-name": "x"}}
	return []Doc{ss, syn, sk}
}

// The CRD's item rules on includePaths, one per case, and the names the
// segment-based rule lets through.
func TestIncludePathsItemRules(t *testing.T) {
	cases := []struct{ path, want string }{
		{"/skills/", "must not start with /"},
		{"skills", "must end in /"},
		{"skills//pdf/", "consecutive slashes"},
		{"./skills/", ". or .. segment"},
		{"skills/../x/", ". or .. segment"},
		{"a/.git/b/", ".git segment"},
		{"", "must not be empty"},
	}
	for _, c := range cases {
		got := joined(Xref(includePathsDocs([]any{"git/"}, []any{c.path}), Options{}))
		if !strings.Contains(got, c.want) {
			t.Errorf("includePaths %q: want %q, got:\n%s", c.path, c.want, got)
		}
	}
	for _, ok := range []string{"skills/pdf/", "Acme Inc./", "v1.2/a..b/", ".github/", "*/"} {
		if got := joined(Xref(includePathsDocs([]any{"git/"}, []any{ok}), Options{})); strings.Contains(got, "includePaths[") {
			t.Errorf("includePaths %q is legal and was reported:\n%s", ok, got)
		}
	}
}

// A searchPath beside every included folder loads nothing; one inside an
// included folder, or at or above one, reaches skills.
func TestSearchPathOutsideIncludePaths(t *testing.T) {
	include := []any{"skills/pdf/", "skills/xlsx/"}
	reported := []string{"git/skills/docx/", "git/docs/", "git/skills/pdfx/"}
	for _, p := range reported {
		got := joined(Xref(includePathsDocs([]any{p}, include), Options{}))
		if !strings.Contains(got, "lies outside the folders its git Syncer checks out") {
			t.Errorf("searchPath %q outside includePaths went unreported:\n%s", p, got)
		}
	}
	clean := []string{"git/skills/pdf/", "git/skills/pdf/forms/", "git/skills/", "git/", "git/skills/xlsx"}
	for _, p := range clean {
		if got := joined(Xref(includePathsDocs([]any{p}, include), Options{})); strings.Contains(got, "lies outside") {
			t.Errorf("searchPath %q reaches an included folder and was reported:\n%s", p, got)
		}
	}
}

// Without includePaths, or with an empty list, the whole repository is synced
// and no searchPath under the destination is reported.
func TestSearchPathWithoutIncludePaths(t *testing.T) {
	for _, include := range [][]any{nil, {}} {
		got := joined(Xref(includePathsDocs([]any{"git/anything/"}, include), Options{}))
		if strings.Contains(got, "lies outside") {
			t.Errorf("includePaths %v syncs the whole repository, and a searchPath was reported:\n%s", include, got)
		}
	}
}

// The list's length is the CRD's maxItems, held by C1.
func TestIncludePathsMaxItems(t *testing.T) {
	var many []any
	for i := 0; i < 21; i++ {
		many = append(many, strings.Repeat("d", i+1)+"/")
	}
	got := strings.Join(Constraints(includePathsDocs([]any{"git/"}, many), Options{}).Warnings, "\n")
	if !strings.Contains(got, "includePaths has 21 entries") {
		t.Errorf("21 includePaths went unreported:\n%s", got)
	}
}
