package main

import (
	"os"
	"path/filepath"
	"testing"
)

// **The property every test here defends is that a skip never lies.** A
// recorded pass is keyed by the check's name and the digests of the classes it
// reads, so the two ways the cache can report a pass for an answer that moved
// underneath it are a key that does not change when an input does, and a key
// that matches something it should not.
//
// `checkKey` and `treeDigest` are pure and are driven directly. `runVerified`
// runs the whole gate off the working directory, so nothing here calls it - the
// half of the record rule that lives in it is stated as a property of the key
// instead, in TestCheckKeyForAnUnclassifiedCheckNeverMatchesARecord.

// allClasses is every class `classDigests` produces. It is written out rather
// than derived, because a new class that nothing here mutates is a class no
// test holds a key against.
var allClasses = []inputClass{
	classCorpus, classGo, classDocs, classKube, classCore, classDocsUp,
}

func baseDigests() map[inputClass]string {
	out := map[inputClass]string{}
	for _, c := range allClasses {
		out[c] = string(c) + "-before"
	}
	return out
}

func readsClass(classes []inputClass, c inputClass) bool {
	for _, got := range classes {
		if got == c {
			return true
		}
	}
	return false
}

// 1. A key changes when any class the check declares changes, and does not
// change when a class it does not declare does. Driven off the real `reads`, so
// a check added to it is covered the moment it is added.
func TestCheckKeyChangesWithEveryClassTheCheckDeclares(t *testing.T) {
	before := baseDigests()
	for _, c := range allClasses {
		after := baseDigests()
		after[c] = string(c) + "-after"
		for name, classes := range reads {
			was, now := checkKey(name, before), checkKey(name, after)
			if readsClass(classes, c) {
				if was == now {
					t.Errorf("%s reads %s and its key did not move when %s did: a pass recorded against the old bytes would be reported as current", name, c, c)
				}
				continue
			}
			if was != now {
				t.Errorf("%s does not read %s but its key moved when %s did: it re-runs for an input it never reads", name, c, c)
			}
		}
	}
}

// 2. A check that declares no class must always run, which is what `checkKey`'s
// own comment promises. The key it gets has to fail to match twice over: a
// record with no entry for the check, and the entry a previous run stored.
func TestCheckKeyForAnUnclassifiedCheckNeverMatchesARecord(t *testing.T) {
	digests := baseDigests()
	key := checkKey("a-check-nobody-has-classified", digests)

	// The record is a map[string]string and a check that has never passed has
	// no entry in it, so the value read back is "". A key of "" is therefore a
	// key that matches a check which has never run.
	unrecorded := map[string]string{}
	if key == unrecorded["a-check-nobody-has-classified"] {
		t.Errorf("an unclassified check's key is %q, which is what a record with no entry for it reads back as: the very first run would report it as skipped, having never run it", key)
	}

	// `runVerified` stores the key it just used, so a key that is stable across
	// runs is one the next run matches - and skips.
	if again := checkKey("a-check-nobody-has-classified", digests); key == again {
		t.Errorf("an unclassified check's key is the same on two calls (%q), so the run that records it makes every later run skip it", key)
	}
}

// 3. Two checks reading identical classes do not share a recorded pass. The two
// synthetic entries are added to `reads` at runtime and removed again - the
// property needs a pair whose class lists are equal, and which pairs the
// hand-written map happens to contain is not this test's business.
func TestCheckKeyDependsOnTheCheckName(t *testing.T) {
	classes := []inputClass{classCorpus, classGo}
	reads["verified-test-one"] = classes
	reads["verified-test-two"] = classes
	t.Cleanup(func() {
		delete(reads, "verified-test-one")
		delete(reads, "verified-test-two")
	})

	digests := baseDigests()
	if one, two := checkKey("verified-test-one", digests), checkKey("verified-test-two", digests); one == two {
		t.Errorf("two checks reading the same classes share the key %q, so one passing records a pass for the other", one)
	}
}

// 4. Class order **does** change the key, and this pins which direction that
// costs. `reads` is hand-written, so somebody will reorder a row; when they do,
// every check on that row would re-run once for no answer. `checkKey` sorts the
// classes so it does not, and the safety argument is unchanged: a reorder still
// cannot produce a match it should not have, because the set of classes is the
// same set and any change to that set changes the key.
//
// **Sorted, so the safe direction costs nothing.** The earlier behaviour - a
// reorder costing one re-run - was safe and wasteful, and `reads` is
// hand-written prose that somebody will tidy.
func TestReorderingAChecksClassesCostsNothing(t *testing.T) {
	reads["verified-test-order"] = []inputClass{classCorpus, classGo}
	t.Cleanup(func() { delete(reads, "verified-test-order") })

	digests := baseDigests()
	forward := checkKey("verified-test-order", digests)
	reads["verified-test-order"] = []inputClass{classGo, classCorpus}
	if reversed := checkKey("verified-test-order", digests); forward != reversed {
		t.Errorf("reordering a check's classes moved the key from %q to %q, which costs a re-run for no answer", forward, reversed)
	}

	// **And the set still matters.** Sorting must not have turned the key into
	// something a different set of classes can collide with.
	reads["verified-test-order"] = []inputClass{classGo}
	if narrowed := checkKey("verified-test-order", digests); narrowed == forward {
		t.Error("dropping a class left the key unchanged, so the key no longer depends on what the check reads")
	}
}

// writeTree builds a scratch tree and returns its root. **It is not under
// `.out/`**, which is where scratch belongs in this repository, because
// `treeDigest` excludes every path containing `/.out/` - a tree built there
// digests as though it were empty, which is the finding
// TestTreeDigestIgnoresTestFilesAndOutputDirectories records.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func digestOf(t *testing.T, root string, paths ...string) string {
	t.Helper()
	d, err := treeDigest(root, paths)
	if err != nil {
		t.Fatalf("treeDigest(%s, %v): %v", root, paths, err)
	}
	return d
}

// 5. The digest moves when the bytes move: a file edited, a file added, a file
// removed. Each of the three is a way a check's inputs change with no other
// signal that they did.
func TestTreeDigestMovesWhenTheTreeDoes(t *testing.T) {
	root := writeTree(t, map[string]string{
		"src/a.md":     "alpha",
		"src/sub/b.md": "beta",
	})
	before := digestOf(t, root, "src")

	if err := os.WriteFile(filepath.Join(root, "src/a.md"), []byte("alpha edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	edited := digestOf(t, root, "src")
	if edited == before {
		t.Errorf("editing a file left the digest at %q, so a check that read it would be skipped", before)
	}

	if err := os.WriteFile(filepath.Join(root, "src/c.md"), []byte("gamma"), 0o644); err != nil {
		t.Fatal(err)
	}
	added := digestOf(t, root, "src")
	if added == edited {
		t.Errorf("adding a file left the digest at %q", edited)
	}

	if err := os.Remove(filepath.Join(root, "src/sub/b.md")); err != nil {
		t.Fatal(err)
	}
	if removed := digestOf(t, root, "src"); removed == added {
		t.Errorf("removing a file left the digest at %q", added)
	}
}

// 6. The digest is over names as well as contents. Two files whose contents are
// swapped hold exactly the same bytes between them, so a digest over content
// alone cannot tell that from leaving them where they were - which is a rename
// no check would be re-run for.
func TestTreeDigestCoversNamesAndNotOnlyContent(t *testing.T) {
	placed := writeTree(t, map[string]string{
		"src/a.md": "alpha",
		"src/b.md": "beta",
	})
	swapped := writeTree(t, map[string]string{
		"src/a.md": "beta",
		"src/b.md": "alpha",
	})
	if digestOf(t, placed, "src") == digestOf(t, swapped, "src") {
		t.Error("swapping two files' contents did not change the digest, so the digest is over content alone and a rename is invisible to it")
	}
}

// 7. Both exclusions, and both are load-bearing. A test file changes what is
// proved about a check rather than what it answers - without that, writing this
// very file invalidates every recorded pass - and `.out/` is generated output
// that several checks write as they run.
func TestTreeDigestIgnoresTestFilesAndOutputDirectories(t *testing.T) {
	root := writeTree(t, map[string]string{"src/a.go": "package src"})
	before := digestOf(t, root, "src")

	for _, added := range []string{"src/a_test.go", "src/.out/scratch.json", "src/sub/.out/deep/log.txt"} {
		p := filepath.Join(root, filepath.FromSlash(added))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("ignored"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := digestOf(t, root, "src"); got != before {
			t.Errorf("adding %s changed the digest from %q to %q: it should be excluded", added, before, got)
		}
	}
}

// 8. The same tree digests the same, whatever order the files were created in,
// wherever the root is, and whichever order the paths are given in. A digest
// that depended on a walk's order would invalidate records on a machine that
// happened to walk differently, and one that depended on the root would
// invalidate them on every checkout.
func TestTreeDigestIsStableForAnUnchangedTree(t *testing.T) {
	root := writeTree(t, map[string]string{
		"one/a.md":       "alpha",
		"one/sub/b.md":   "beta",
		"two/c.md":       "gamma",
		"two/sub/d/e.md": "delta",
	})
	first := digestOf(t, root, "one", "two")
	if second := digestOf(t, root, "one", "two"); second != first {
		t.Errorf("two digests of one unchanged tree differ: %q then %q", first, second)
	}
	if reversed := digestOf(t, root, "two", "one"); reversed != first {
		t.Errorf("giving the same paths in the other order changed the digest: %q against %q", reversed, first)
	}

	// The same tree, built in the opposite order under a different root. Map
	// iteration makes the creation order in writeTree arbitrary already; this
	// says the digest does not carry the root either.
	elsewhere := writeTree(t, map[string]string{
		"two/sub/d/e.md": "delta",
		"two/c.md":       "gamma",
		"one/sub/b.md":   "beta",
		"one/a.md":       "alpha",
	})
	if other := digestOf(t, elsewhere, "one", "two"); other != first {
		t.Errorf("the same tree under a different root digested as %q against %q", other, first)
	}
}
