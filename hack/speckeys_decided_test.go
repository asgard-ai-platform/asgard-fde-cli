package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asgard-ai-platform/asgard-fde-cli/hack/internal/src"
)

// **Every decision still has the document that carries it.**
//
// `decided` is the one part of the spec-key gap no program derives: a judgement
// that a key belongs to a shape an engagement should not reach for. What makes
// it a pointer rather than an opinion typed into a map is that the document
// naming the key is checked - and until this test existed, it was checked only
// when somebody ran `spec-key-gap` by hand, which needs helm and the clones.
// This runs it in `go test ./...`, where a renamed page fails on the commit
// that renames it.
func TestEveryDecisionNamesADocumentThatStillNamesTheKey(t *testing.T) {
	root, err := src.Root()
	if err != nil {
		t.Fatal(err)
	}
	if bad := checkDecided(root); len(bad) > 0 {
		t.Errorf("a decision has lost its document:\n%s", strings.Join(bad, "\n"))
	}
}

// **A document that is gone, and one that has stopped naming the key, are the
// two ways a decision quietly becomes an absence** - and the second is the one
// that reads as fine, because the row is still there and still points
// somewhere.
func TestADecisionWithoutItsDocumentFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "doc"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A page that mentions the word in ordinary prose and nowhere as a field.
	// **This is the case worth the test**: a substring match passes here, and
	// passing here means the check is green over a page whose decision has
	// been deleted.
	if err := os.WriteFile(filepath.Join(dir, "doc", "page.md"),
		[]byte("# A page\n\nThe entry point is the default, and it can be raised.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	defer func(saved map[string]string) { decided = saved }(decided)
	decided = map[string]string{
		"entries.labels.default": "doc/page.md",
		"editorServer":           "doc/missing.md",
	}

	bad := checkDecided(dir)
	joined := strings.Join(bad, "\n")
	if !strings.Contains(joined, "doc/missing.md is not there") {
		t.Errorf("a decision pointing at a document that does not exist passed:\n%s", joined)
	}
	if !strings.Contains(joined, "entries.labels.default") {
		t.Errorf("a decision whose document only has the word in prose passed:\n%s", joined)
	}

	// The same page, now naming the key the way a document names a field.
	if err := os.WriteFile(filepath.Join(dir, "doc", "page.md"),
		[]byte("# A page\n\nDo not write `entries.labels.default`.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, b := range checkDecided(dir) {
		if strings.Contains(b, "entries.labels.default") {
			t.Errorf("a document naming the key was still reported:\n%s", b)
		}
	}
}

// **The check is the difference between naming a field and containing a word.**
// Every `yes` below is a form a document writes a field in; every `no` is the
// same letters arriving by accident, which is what a plain substring match
// cannot tell apart.
func TestNamesAFieldRatherThanAWord(t *testing.T) {
	const text = "" +
		"that is the default, and it can be raised\n" +
		"members are people\n" +
		"  enabled: true\n" +
		"the `docx` source class\n" +
		"> - **`members:` on the SourceSet**\n" +
		"    spec.deletedIndexerKeys         REQUIRED\n"

	for _, form := range []string{"enabled", "docx", "members", "deletedIndexerKeys"} {
		if !names(text, form) {
			t.Errorf("a field the document names was not found: %s", form)
		}
	}
	for _, form := range []string{"default", "raised", "people", "labels"} {
		if names(text, form) {
			t.Errorf("a word in prose was read as a field: %s", form)
		}
	}
	// **A dotted path is matched plainly**, because a dot between two field
	// names does not arrive by accident - and a document that writes one has
	// named every segment of it.
	const dotted = "Do not teach `entries.labels.default`\n"
	for _, form := range []string{"entries.labels.default", "labels.default", "default"} {
		if !names(dotted, form) {
			t.Errorf("a segment of a named path was not found: %s", form)
		}
	}
	if names(dotted, "entries.labels.relationship_id") {
		t.Error("a path the document does not write was found")
	}
}

// **A `<key>` segment is a name a customer chooses, so no document can name
// it** - and a check demanding one would be permanently red. What a document
// can name is the map it hangs off.
func TestEvidenceDropsACustomersOwnKey(t *testing.T) {
	got := evidence("docx.indexers.<key>.chunkSize")
	for _, f := range got {
		if strings.Contains(f, "<key>") {
			t.Errorf("a customer's own map key was asked of a document: %q", f)
		}
	}
	if len(got) == 0 || got[0] != "docx.indexers.chunkSize" {
		t.Errorf("evidence should start at the longest nameable path, got %v", got)
	}
	// The field below a customer's key is still a field, and a document
	// naming it is still carrying the decision.
	if got[len(got)-1] != "chunkSize" {
		t.Errorf("the field below the map key was dropped, got %v", got)
	}
}

// **A decision covers what is under it**, and the alternative is a hand-kept
// copy of a CRD: `add` writes no CompletionModel at all, so listing every field
// inside one - down to `apiKey.valueFrom.secretKeyRef.key` - would be a list
// somebody maintains against a contract that moves.
//
// It is also what keeps the third state honest. A generic leaf like `key` or
// `name` is commented in some template somewhere, so a field of a kind nobody
// generates would otherwise be reported as shown in a skeleton that belongs to
// another kind entirely.
func TestADecisionCoversTheFieldsUnderIt(t *testing.T) {
	defer func(saved map[string]string) { decided = saved }(decided)
	decided = map[string]string{"anthropic": "internal/corpus/wiki/settings.md"}

	for _, k := range []string{
		"anthropic",
		"anthropic.apiKey",
		"anthropic.apiKey.valueFrom.secretKeyRef.key",
	} {
		if got := decidedFor(k); got == "" {
			t.Errorf("%s is under a decision and was not covered by it", k)
		}
	}
	// And it covers nothing beside it: a prefix is a path, not a string.
	for _, k := range []string{"anthropicModel", "gemini.apiKey", "model"} {
		if got := decidedFor(k); got != "" {
			t.Errorf("%s is not under that decision and was covered by it: %s", k, got)
		}
	}
}
