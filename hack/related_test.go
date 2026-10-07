package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/asgard-ai-platform/asgard-fde-cli/hack/internal/src"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/kb"
)

// addIn is the dedupe: a document that names one target twice is one entry,
// because what gets re-read is a document and not a pointer.
func TestAddInRecordsEachPointingDocumentOnce(t *testing.T) {
	in := map[string][]string{}
	addIn(in, "wiki/knowledge", []kb.Link{
		{Kind: "wiki", Name: "processors"},
		{Kind: "wiki", Name: "processors"},
		{Kind: "usecase", Name: "plugin"},
		{Kind: "", Name: "processors"}, // not a pointer: no kind
		{Kind: "wiki", Name: ""},       // not a pointer: no name
	})
	addIn(in, "brief/interview", []kb.Link{{Kind: "wiki", Name: "processors"}})

	if got := in["wiki/processors"]; len(got) != 2 ||
		!contains(got, "wiki/knowledge") || !contains(got, "brief/interview") {
		t.Errorf("wiki/processors in-edges = %v, want the two documents once each", got)
	}
	if got := in["usecase/plugin"]; len(got) != 1 || got[0] != "wiki/knowledge" {
		t.Errorf("usecase/plugin in-edges = %v, want [wiki/knowledge]", got)
	}
	if len(in) != 2 {
		t.Errorf("targets = %v, want only the two with a kind and a name", keysOf(in))
	}
}

// Every part of the corpus is a body, and the graph has to be over all five.
// `needs` and `brief` are Go rather than markdown and carry no parsed Links
// field, so they are the two that drop out silently when a reader is missed:
// their documents would point at nothing and nothing would say so.
func TestCorpusInEdgesReadsEveryKindOfBody(t *testing.T) {
	in, err := corpusInEdges(mustRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, froms := range in {
		for _, f := range froms {
			seen[strings.SplitN(f, "/", 2)[0]] = true
		}
	}
	for _, kind := range kb.Kinds {
		if !seen[kind] {
			t.Errorf("no %s document points at anything, so that body is not being read", kind)
		}
	}
}

// A changed document nothing points at is named rather than omitted: the
// reader has to be able to tell "nothing points at it" from "I forgot to
// look".
func TestRelatedReportNamesADocumentNothingPointsAt(t *testing.T) {
	var b strings.Builder
	reads := relatedReport(&b, []string{"wiki/orphan"}, map[string][]string{
		"wiki/other": {"wiki/knowledge"},
	})
	out := b.String()
	if !strings.Contains(out, "wiki/orphan") {
		t.Errorf("the changed document is not in the report:\n%s", out)
	}
	if !strings.Contains(out, "nothing points at it") {
		t.Errorf("no in-edges and the report does not say so:\n%s", out)
	}
	if len(reads) != 0 {
		t.Errorf("re-read = %v, want none", reads)
	}
}

// A changed document is already being read. Listing it as its own re-read
// would pad the worklist with the work that produced it.
func TestRelatedReportDoesNotRereadAChangedDocument(t *testing.T) {
	var b strings.Builder
	reads := relatedReport(&b, []string{"wiki/a", "wiki/b"}, map[string][]string{
		"wiki/a": {"wiki/b", "usecase/plugin"},
		"wiki/b": {"wiki/a"},
	})
	if len(reads) != 1 || reads[0] != "usecase/plugin" {
		t.Errorf("re-read = %v, want only usecase/plugin", reads)
	}
	if !strings.Contains(b.String(), "1 other document(s)") {
		t.Errorf("count line does not match the re-read set:\n%s", b.String())
	}
}

// One document pointing at several changed ones is one re-read.
func TestRelatedReportDeduplicatesTheReread(t *testing.T) {
	var b strings.Builder
	reads := relatedReport(&b, []string{"wiki/a", "wiki/b", "wiki/c"}, map[string][]string{
		"wiki/a": {"guide/02-projects", "brief/interview"},
		"wiki/b": {"guide/02-projects"},
		"wiki/c": {"guide/02-projects", "brief/interview"},
	})
	if len(reads) != 2 || reads[0] != "brief/interview" || reads[1] != "guide/02-projects" {
		t.Errorf("re-read = %v, want each pointing document once, sorted", reads)
	}
	if !strings.Contains(b.String(), "3 changed, 2 other document(s)") {
		t.Errorf("count line wrong:\n%s", b.String())
	}
}

// docKey maps a repository path to the kind/name a pointer resolves to, and
// returns empty for anything outside the corpus - which is what keeps a
// diff touching Go, templates or a root document out of the worklist.
func TestDocKey(t *testing.T) {
	for _, c := range []struct{ path, want string }{
		{"internal/corpus/wiki/processors.md", "wiki/processors"},
		{"internal/corpus/wiki/index.md", "wiki/index"},
		{"internal/corpus/usecase/plugin.md", "usecase/plugin"},
		{"internal/stage/prompts/02-projects.md", "guide/02-projects"},
		{"internal/corpus/aliases.md", ""},                // above both halves, not a page
		{"internal/corpus/wiki/README.md", "wiki/README"}, // unlisted, but a document All() carries
		{"internal/corpus/wiki/notes.txt", ""},
		{"internal/scaffold/templates/AGENTS.md.tmpl", ""},
		{"AGENTS.md", ""},
		{"hack/related.go", ""},
		{"", ""},
	} {
		if got := docKey(c.path); got != c.want {
			t.Errorf("docKey(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

// This exercises the git invocation and the wiring, and nothing else: the
// check is a listing and cannot fail, so a nil return says only that it did
// not panic and that `git diff` was accepted. It is not coverage of what the
// report says.
func TestRunRelatedAgainstThisRepository(t *testing.T) {
	if out := captureStdout(t, func() {
		if err := runRelated(nil); err != nil {
			t.Errorf("runRelated: %v", err)
		}
	}); out == "" {
		t.Error("runRelated printed nothing")
	}
}

func keysOf(m map[string][]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// captureStdout runs fn with os.Stdout redirected, so a listing's own output
// does not bury the test output. The pipe is drained in a goroutine because a
// listing can print more than a pipe buffer holds.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() {
		os.Stdout = saved
		w.Close()
	}()
	fn()
	os.Stdout = saved
	w.Close()
	return <-done
}

func mustRoot(t *testing.T) string {
	t.Helper()
	root, err := src.Root()
	if err != nil {
		t.Fatal(err)
	}
	return root
}
