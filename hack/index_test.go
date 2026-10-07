package main

import (
	"strings"
	"testing"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/kb"
)

// The index renderer is tested against bodies built here rather than against
// the corpus, because a test that reads the real documents fails whenever one
// of them is edited - which is a fact about the corpus and not about this
// code. What the real tree gets is one integration test at the foot of this
// file, whose whole job is to notice an index that has drifted.
//
// **The two markups are taken off `indexFiles` rather than retyped**, so a
// change to either index's heading pattern or column heading is exercised
// here; only the path and the page list are replaced.

func shape(t *testing.T, kind string) indexFile {
	t.Helper()
	for _, f := range indexFiles {
		if f.kind == kind {
			f.path = kind + "-index.md"
			f.pages = nil
			return f
		}
	}
	t.Fatalf("no index file of kind %q", kind)
	return indexFile{}
}

func lines(l ...string) string { return strings.Join(l, "\n") }

func doc(name, group, description string) kb.Doc {
	return kb.Doc{Name: name, Group: group, Description: description}
}

func grouped(docs ...kb.Doc) map[string][]kb.Doc {
	byGroup := map[string][]kb.Doc{}
	for _, d := range docs {
		byGroup[d.Group] = append(byGroup[d.Group], d)
	}
	return byGroup
}

// render runs the function under test and fails on an error, which is what
// every test but the two about errors wants.
func render(t *testing.T, body string, f indexFile, byGroup map[string][]kb.Doc) (string, map[string]bool) {
	t.Helper()
	out, seen, err := renderIndexTables(body, f, byGroup)
	if err != nil {
		t.Fatalf("renderIndexTables: %v", err)
	}
	return out, seen
}

func equal(t *testing.T, got, want string) {
	t.Helper()
	if got == want {
		return
	}
	t.Errorf("output differs\n--- got ---\n%s\n--- want ---\n%s", got, want)
}

// TestRoundTripChangesNothing is the property the whole design rests on: an
// index already agreeing with its documents is rewritten to itself, byte for
// byte. Without it `--write` would churn every file it touched and a diff
// would stop meaning that a document changed.
func TestRoundTripChangesNothing(t *testing.T) {
	f := shape(t, "wiki")
	body := lines(
		"# Index",
		"",
		"## Products and scope",
		"",
		"| page | covers |",
		"|---|---|",
		"| [`console`](../wiki/console.md) | the permission layers |",
		"| [`fehu`](../wiki/fehu.md) | billing and usage |",
		"",
	)
	byGroup := grouped(
		doc("console", "Products and scope", "the permission layers"),
		doc("fehu", "Products and scope", "billing and usage"),
	)

	out, seen := render(t, body, f, byGroup)
	equal(t, out, body)
	if !seen["Products and scope"] {
		t.Errorf("the section was rendered and `seen` does not say so")
	}

	// And again from its own output, because a renderer that is stable only on
	// the input it was handed is not stable.
	twice, _ := render(t, out, f, byGroup)
	equal(t, twice, body)
}

// TestChangedDescriptionRewritesOnlyThatRow pins that a row is rendered from
// the document's own `description:` and that rewriting one leaves the rest of
// the file alone - the diff an FDE reads after `--write` is meant to be the
// document's line and nothing else.
func TestChangedDescriptionRewritesOnlyThatRow(t *testing.T) {
	f := shape(t, "wiki")
	body := lines(
		"## Products and scope",
		"",
		"| page | covers |",
		"|---|---|",
		"| [`console`](../wiki/console.md) | the permission layers |",
		"| [`fehu`](../wiki/fehu.md) | STALE |",
		"| [`mimir`](../wiki/mimir.md) | Thread, View, Dashboard |",
		"",
	)
	byGroup := grouped(
		doc("console", "Products and scope", "the permission layers"),
		doc("fehu", "Products and scope", "billing and usage, how cost is broken down"),
		doc("mimir", "Products and scope", "Thread, View, Dashboard"),
	)

	out, _ := render(t, body, f, byGroup)
	equal(t, out, lines(
		"## Products and scope",
		"",
		"| page | covers |",
		"|---|---|",
		"| [`console`](../wiki/console.md) | the permission layers |",
		"| [`fehu`](../wiki/fehu.md) | billing and usage, how cost is broken down |",
		"| [`mimir`](../wiki/mimir.md) | Thread, View, Dashboard |",
		"",
	))
}

// TestExistingRowOrderIsKept pins that the file's order wins over the order
// the documents arrive in. The sections are a reading order - products before
// the pages that assume you know them - and no field on a document recovers
// it. Sorting here destroyed it once.
func TestExistingRowOrderIsKept(t *testing.T) {
	f := shape(t, "wiki")
	body := lines(
		"## Products and scope",
		"",
		"| page | covers |",
		"|---|---|",
		"| [`product-suite`](../wiki/product-suite.md) | what each product is for |",
		"| [`console`](../wiki/console.md) | the permission layers |",
		"| [`fehu`](../wiki/fehu.md) | billing and usage |",
		"",
	)
	// Arriving alphabetically, which is neither the file's order nor a
	// reversal of it, so a renderer that sorted or reversed would both fail.
	byGroup := grouped(
		doc("console", "Products and scope", "the permission layers"),
		doc("fehu", "Products and scope", "billing and usage"),
		doc("product-suite", "Products and scope", "what each product is for"),
	)

	out, _ := render(t, body, f, byGroup)
	equal(t, out, body)
}

// TestNewPageIsAppendedNotInserted pins that a document the index does not yet
// list arrives at the foot of its section. Any other rule moves rows nobody
// touched, which turns a one-line addition into a diff somebody has to read -
// and "last" is where a reader looks for what was added.
func TestNewPageIsAppendedNotInserted(t *testing.T) {
	f := shape(t, "wiki")
	body := lines(
		"## While building",
		"",
		"| page | covers |",
		"|---|---|",
		"| [`setup-path`](../wiki/setup-path.md) | the order |",
		"| [`tools`](../wiki/tools.md) | MCP Server, Skillset and Plugin |",
		"",
	)
	byGroup := grouped(
		// `agents` sorts before both existing rows, so an insert would land it
		// first; `zzz-last` sorts after, so an append that happened to be a
		// sort would look right on one document and not on two.
		doc("agents", "While building", "Flow Agent against Managed Agent"),
		doc("setup-path", "While building", "the order"),
		doc("tools", "While building", "MCP Server, Skillset and Plugin"),
		doc("zzz-last", "While building", "the catch-all"),
	)

	out, _ := render(t, body, f, byGroup)
	equal(t, out, lines(
		"## While building",
		"",
		"| page | covers |",
		"|---|---|",
		"| [`setup-path`](../wiki/setup-path.md) | the order |",
		"| [`tools`](../wiki/tools.md) | MCP Server, Skillset and Plugin |",
		"| [`agents`](../wiki/agents.md) | Flow Agent against Managed Agent |",
		"| [`zzz-last`](../wiki/zzz-last.md) | the catch-all |",
		"",
	))
}

// TestRemovedPageLosesItsRow pins the other direction: a row whose document is
// gone from the corpus goes with it, so the index cannot point a reader at a
// page that no longer exists.
func TestRemovedPageLosesItsRow(t *testing.T) {
	f := shape(t, "wiki")
	body := lines(
		"## In practice",
		"",
		"| page | covers |",
		"|---|---|",
		"| [`operations`](../wiki/operations.md) | outbound IPs |",
		"| [`deleted`](../wiki/deleted.md) | a page that is gone |",
		"| [`glossary`](../wiki/glossary.md) | words that mean one thing here |",
		"",
	)
	byGroup := grouped(
		doc("operations", "In practice", "outbound IPs"),
		doc("glossary", "In practice", "words that mean one thing here"),
	)

	out, _ := render(t, body, f, byGroup)
	equal(t, out, lines(
		"## In practice",
		"",
		"| page | covers |",
		"|---|---|",
		"| [`operations`](../wiki/operations.md) | outbound IPs |",
		"| [`glossary`](../wiki/glossary.md) | words that mean one thing here |",
		"",
	))
}

// TestProseAroundTheTablesSurvives pins the reason this renders sections in
// place rather than rewriting the file: both real indexes carry sections that
// are not listings at all - what is deliberately not covered, how to read an
// extract, the warnings that apply to all of them - plus prose between a
// heading and its table, and a rewrite-whole would delete every word of it.
func TestProseAroundTheTablesSurvives(t *testing.T) {
	f := shape(t, "wiki")
	body := lines(
		"---",
		"description: every wiki page grouped by the question it answers",
		"---",
		"# Index",
		"",
		"    ../wiki/<page>.md",
		"",
		"## Products and scope",
		"",
		"Read this one as \"the customer said X, so the chart writes Y\".",
		"",
		"| page | covers |",
		"|---|---|",
		"| [`console`](../wiki/console.md) | STALE |",
		"",
		"A note that belongs to the section and not to a row.",
		"",
		"## Every UI name, and the CR it is",
		"",
		"Not a group any document declares, and it has a table of its own that",
		"nothing here may touch.",
		"",
		"| the UI calls it | the chart writes |",
		"|---|---|",
		"| Managed Agent | Agent |",
		"",
	)
	byGroup := grouped(doc("console", "Products and scope", "the permission layers"))

	out, seen := render(t, body, f, byGroup)
	equal(t, out, strings.Replace(body,
		"| [`console`](../wiki/console.md) | STALE |",
		"| [`console`](../wiki/console.md) | the permission layers |", 1))
	if seen["Every UI name, and the CR it is"] {
		t.Errorf("a section no document declares was treated as a group")
	}
}

// TestGroupWithNoSectionIsReported pins that a `group:` the index file has no
// section for comes back through `seen` rather than being dropped. A document
// silently absent from the index is the failure the whole check exists to
// prevent: the page is there, nothing points at it, and the writer never finds
// out.
func TestGroupWithNoSectionIsReported(t *testing.T) {
	f := shape(t, "wiki")
	body := lines(
		"## Products and scope",
		"",
		"| page | covers |",
		"|---|---|",
		"| [`console`](../wiki/console.md) | the permission layers |",
		"",
	)
	byGroup := grouped(
		doc("console", "Products and scope", "the permission layers"),
		doc("stray", "A group the file has never heard of", "nowhere to land"),
	)

	out, seen := render(t, body, f, byGroup)
	equal(t, out, body)
	if !seen["Products and scope"] {
		t.Errorf("`seen` missing the section that was rendered")
	}
	if seen["A group the file has never heard of"] {
		t.Errorf("`seen` claims a section that is not in the file; runIndex would report nothing")
	}
}

// TestSectionWithoutATableIsAnError pins that a heading a document declares
// and a file that has no table under it fails, rather than producing a file
// with rows in the wrong place. Both ends are covered: the next heading, and
// the end of the file.
func TestSectionWithoutATableIsAnError(t *testing.T) {
	f := shape(t, "wiki")
	byGroup := grouped(doc("console", "Products and scope", "the permission layers"))

	t.Run("another heading follows", func(t *testing.T) {
		body := lines(
			"## Products and scope",
			"",
			"Prose, and no table.",
			"",
			"## In practice",
			"",
			"| page | covers |",
			"|---|---|",
			"",
		)
		if _, _, err := renderIndexTables(body, f, byGroup); err == nil {
			t.Fatalf("no error for a section whose table is missing")
		}
	})

	t.Run("the file ends", func(t *testing.T) {
		body := lines(
			"## Products and scope",
			"",
			"Prose, and then nothing.",
			"",
		)
		if _, _, err := renderIndexTables(body, f, byGroup); err == nil {
			t.Fatalf("no error for a section at the end of the file with no table")
		}
	})
}

// TestExtractsIndexShape pins the second markup: bold lead-ins with an
// explanatory tail instead of `## ` headings, a `| file ` column, and a row
// label carrying `.md`. That file reads as an argument rather than a
// catalogue, and normalising it to the wiki's shape would rewrite prose to
// suit a renderer.
func TestExtractsIndexShape(t *testing.T) {
	f := shape(t, "usecase")
	body := lines(
		"## The extracts",
		"",
		"**Read first:**",
		"",
		"| file | what |",
		"|---|---|",
		"| [`conventions.md`](../usecase/conventions.md) | where every CR goes |",
		"",
		"**Entry points** - pick by audience, never by preference:",
		"",
		"| file | when |",
		"|---|---|",
		"| [`agent-hub.md`](../usecase/agent-hub.md) | STALE |",
		"",
	)
	byGroup := grouped(
		doc("conventions", "Read first", "where every CR goes"),
		doc("agent-hub", "Entry points", "every caller can authenticate; several specialists"),
		doc("flow-agent-single", "Entry points", "anonymous audience, one job"),
	)

	out, seen := render(t, body, f, byGroup)
	equal(t, out, lines(
		"## The extracts",
		"",
		"**Read first:**",
		"",
		"| file | what |",
		"|---|---|",
		"| [`conventions.md`](../usecase/conventions.md) | where every CR goes |",
		"",
		"**Entry points** - pick by audience, never by preference:",
		"",
		"| file | when |",
		"|---|---|",
		"| [`agent-hub.md`](../usecase/agent-hub.md) | every caller can authenticate; several specialists |",
		"| [`flow-agent-single.md`](../usecase/flow-agent-single.md) | anonymous audience, one job |",
		"",
	))
	for _, g := range []string{"Read first", "Entry points"} {
		if !seen[g] {
			t.Errorf("section %q was rendered and `seen` does not say so", g)
		}
	}
}

// TestInIndexOrder covers the ordering rule on its own, because it is the one
// piece of judgement in the renderer and the tests above reach it only through
// a whole file.
func TestInIndexOrder(t *testing.T) {
	pages := []kb.Doc{doc("c", "g", ""), doc("a", "g", ""), doc("b", "g", ""), doc("new-z", "g", ""), doc("new-a", "g", "")}
	want := []string{"c", "a", "b", "new-a", "new-z"}

	var got []string
	for _, d := range inIndexOrder(pages, []string{"c", "a", "b", "gone"}) {
		got = append(got, d.Name)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("order %v, want %v", got, want)
	}

	// A name listed twice in the file yields one row, not two.
	got = nil
	for _, d := range inIndexOrder([]kb.Doc{doc("a", "g", "")}, []string{"a", "a"}) {
		got = append(got, d.Name)
	}
	if len(got) != 1 {
		t.Errorf("a name listed twice produced %d rows, want 1", len(got))
	}
}

// TestRunIndexAgainstTheRepository is the only test here that reads the real
// corpus, and its job is the one the unit tests above cannot do: notice that a
// real index has drifted from the documents it lists. It must not re-test the
// rendering rules - that is what everything above is for.
func TestRunIndexAgainstTheRepository(t *testing.T) {
	if err := runIndex(nil); err != nil {
		t.Fatalf("go run ./hack index: %v\nrun `go run ./hack index --write` and read the diff", err)
	}
}
