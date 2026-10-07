package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/asgard-ai-platform/asgard-fde-cli/hack/internal/src"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/brief"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/kb"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/needs"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/scaffold"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/stage"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/usecase"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/wiki"
)

func init() {
	register("related", check{
		Needs:   "this repository",
		What:    "which documents point at the ones THIS change touched, so a re-read has a worklist instead of the whole corpus. **It cannot fail**: pointing at a changed document is not a defect, it is a question",
		Listing: true,
		Run:     runRelated,
	})
}

// runRelated answers the question a term sweep answers by accident.
//
// **`--orphans` asks whether anything points at a document; this asks what
// does.** The graph has always carried the answer - `kb.Link` is parsed with
// every document - and nothing read it in that direction, so "what else claimed
// the thing you just changed" was a grep for a word and came back with whatever
// shared vocabulary.
//
// **This is also what makes a pass cheap.** Re-reading the corpus after a change
// to three documents is the cost that makes a reading backlog look endless. The
// in-edges of those three are the reading somebody can actually do.
//
// It reports rather than fails, because a document pointing at a changed one is
// a question and not a defect: the pointer may be about a paragraph nothing
// touched.
func runRelated(args []string) error {
	root, err := src.Root()
	if err != nil {
		return err
	}
	in, err := corpusInEdges(root)
	if err != nil {
		return err
	}

	base := "HEAD"
	if len(args) > 0 {
		base = args[0]
	}
	out, err := exec.Command("git", "-C", root, "diff", "--name-only", base, "--", "internal/corpus", "internal/stage/prompts").CombinedOutput()
	if err != nil {
		return fmt.Errorf("git diff %s failed:\n%s", base, out)
	}
	var changed []string
	for _, f := range strings.Split(string(out), "\n") {
		if k := docKey(strings.TrimSpace(f)); k != "" {
			changed = append(changed, k)
		}
	}
	sort.Strings(changed)

	if len(changed) == 0 {
		fmt.Printf("No document under internal/corpus or internal/stage/prompts differs from %s.\n", base)
		return nil
	}
	relatedReport(os.Stdout, changed, in)
	return nil
}

// corpusInEdges returns, for each document, the documents that point at it,
// keyed the way a pointer resolves: `kind/name`.
//
// **Every part of the corpus is a body, and the graph is over all five.**
// `needs` and `brief` are Go rather than markdown, so their pointers are in
// what they render and not in a parsed field; reading four of the five would
// leave a whole kind of document silently pointing at nothing.
func corpusInEdges(root string) (map[string][]string, error) {
	in := map[string][]string{}
	pages, err := wiki.All()
	if err != nil {
		return nil, err
	}
	for _, d := range pages {
		addIn(in, "wiki/"+d.Name, d.Links)
	}
	extracts, err := usecase.All()
	if err != nil {
		return nil, err
	}
	for _, d := range extracts {
		addIn(in, "usecase/"+d.Name, d.Links)
	}
	for _, d := range needs.Documents() {
		addIn(in, "needs/"+d.Name, linksOf(d.Body))
	}
	for _, d := range brief.Documents() {
		addIn(in, "brief/"+d.Name, linksOf(d.Body))
	}
	// **The root documents point into the corpus constantly and are not in it.**
	// `doc-paths` checks that a path they name exists; nothing asks whether the
	// sentence around it still holds. They are sources here and never targets:
	// nothing in the corpus points back at them, because they do not land.
	for _, name := range []string{"AGENTS.md", "APPROACH.md", "STRUCTURE.md", "README.md", "README.zh-TW.md", "TASK.md", "Goal.md"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		// A root document with no pointer is not a node: `Goal.md` names no
		// corpus document, so nothing points from it.
		addIn(in, "root/"+strings.TrimSuffix(strings.TrimSuffix(name, ".md"), ".zh-TW"), linksOf(string(data)))
	}
	skills, err := scaffold.Skills()
	if err != nil {
		return nil, err
	}
	for _, sk := range skills {
		body, err := scaffold.Body(sk.Name)
		if err != nil {
			return nil, err
		}
		addIn(in, "skill/"+sk.Name, linksOf(body))
	}
	guides, err := stage.StaticDocuments()
	if err != nil {
		return nil, err
	}
	for _, d := range guides {
		addIn(in, "guide/"+d.Name, linksOf(d.Body))
	}
	return in, nil
}

// addIn records that from points at each of links. A document that names one
// target twice is one entry: the reading is of a document, not of a pointer.
func addIn(in map[string][]string, from string, links []kb.Link) {
	for _, l := range links {
		if l.Kind == "" || l.Name == "" {
			continue
		}
		t := l.Kind + "/" + l.Name
		if !contains(in[t], from) {
			in[t] = append(in[t], from)
		}
	}
}

// relatedReport writes the worklist for changed against the in-edge graph, and
// returns the re-read set: every document pointing at a changed one, less the
// changed documents themselves, sorted.
//
// **Separated from the git diff so it can be driven.** What the report has to
// get right - that a changed document with no in-edges is named rather than
// omitted, that a changed document is not its own re-read, that two changed
// documents sharing a pointer count it once - is a property of this function
// and of nothing in git.
func relatedReport(w io.Writer, changed []string, in map[string][]string) []string {
	reads := map[string]bool{}
	for _, c := range changed {
		fmt.Fprintf(w, "\n%s\n", c)
		if len(in[c]) == 0 {
			// **Named rather than omitted**: a reader has to be able to tell
			// "nothing points at it" from "I forgot to look".
			fmt.Fprintln(w, "    nothing points at it")
			continue
		}
		sort.Strings(in[c])
		for _, f := range in[c] {
			fmt.Fprintf(w, "    %s\n", f)
			reads[f] = true
		}
	}
	for _, c := range changed {
		delete(reads, c)
	}
	out := make([]string, 0, len(reads))
	for f := range reads {
		out = append(out, f)
	}
	sort.Strings(out)
	fmt.Fprintf(w, "\n%d changed, %d other document(s) point at them.\n", len(changed), len(out))
	fmt.Fprintln(w, "\nThose are the documents to re-read against the change. A pointer may be")
	fmt.Fprintln(w, "about a paragraph nothing touched, so this cannot fail.")
	return out
}

// linksOf parses the pointers out of a rendered body. `needs` and `brief` are
// Go rather than markdown, so their documents carry no parsed Links field - the
// pointer is in the text they render, and this reads it the same way every
// other document's is read.
func linksOf(body string) []kb.Link {
	links, _ := kb.Links(body)
	return links
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// docKey turns a repository path into the kind/name a pointer resolves to.
func docKey(p string) string {
	switch {
	case strings.HasPrefix(p, "internal/corpus/wiki/") && strings.HasSuffix(p, ".md"):
		return "wiki/" + strings.TrimSuffix(strings.TrimPrefix(p, "internal/corpus/wiki/"), ".md")
	case strings.HasPrefix(p, "internal/corpus/usecase/") && strings.HasSuffix(p, ".md"):
		return "usecase/" + strings.TrimSuffix(strings.TrimPrefix(p, "internal/corpus/usecase/"), ".md")
	case strings.HasPrefix(p, "internal/stage/prompts/") && strings.HasSuffix(p, ".md"):
		return "guide/" + strings.TrimSuffix(strings.TrimPrefix(p, "internal/stage/prompts/"), ".md")
	}
	return ""
}
