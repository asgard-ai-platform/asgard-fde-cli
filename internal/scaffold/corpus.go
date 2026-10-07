package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/asgard-ai-platform/asgard-fde-cli/internal/brief"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/kb"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/needs"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/stage"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/usecase"
	"github.com/asgard-ai-platform/asgard-fde-cli/internal/wiki"
)

// corpusSkillDir is where the platform corpus is written in a repository.
//
// **Under `.agents/skills/` on purpose**: that is where an agent in a customer
// repository finds what it knows, and a copy anywhere else is one it has to be
// told about. `shipped` already covers that prefix, so these files get the
// stamp and the five states with no rule of their own.
//
// Why a copy is safe - the stamp makes staleness visible - is in
// `internal/corpus/wiki/README.md`, beside the rule it qualifies. Not repeated
// here.
const corpusSkillDir = ".agents/skills/asgard-platform"

// corpusJobs writes the wiki and the extracts into a repository as plain files.
//
// **Nothing here is interpolated.** A version string in any of these files
// would change their bytes on every release, and what finds a stale repository
// is a byte comparison - so every repository in the world would report behind
// on a release that touched no page. Same failure as the one at the top of
// stamp.go, from the other side.
func corpusJobs() ([]job, error) {
	var jobs []job
	// **Where a description cannot be parsed out of the body.** `needs` and
	// `brief` are Go rather than markdown, so what they render carries no
	// frontmatter - the description is a field on the struct, and it has to
	// reach the index some other way than by being read back off the file.
	described := map[string]string{}
	add := func(target string, body string) {
		jobs = append(jobs, job{target: filepath.Join(corpusSkillDir, target), body: []byte(body)})
	}
	addDescribed := func(target, description, body string) {
		add(target, body)
		described[filepath.Join(corpusSkillDir, target)] = description
	}

	// Not List: a corpus hides its own bookkeeping from a listing, and some of
	// that bookkeeping is exactly what an exported copy needs. Writing List's
	// view of the wiki produced a SKILL.md pointing at an index that was not
	// there.
	//
	// **But not all of it, either.** `wiki.Landing` drops the log, because the
	// two unlisted wiki documents are not alike: the index is the map and has
	// to travel, while the log is provenance for whoever maintains this
	// repository - the wiki's own index says an FDE looking for an answer
	// should never land there. Landing it also put two warnings in every
	// customer repository, for historical entries that name a command the tool
	// has since removed and are correct to.
	for _, half := range []struct {
		dir  string
		all  func() ([]kb.Doc, error)
		read func(string) (string, error)
	}{
		{"wiki", wiki.Landing, wiki.Read},
		{"usecase", usecase.All, usecase.Read},
	} {
		docs, err := half.all()
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", half.dir, err)
		}
		for _, d := range docs {
			body, err := half.read(d.Name)
			if err != nil {
				return nil, fmt.Errorf("read %s %s: %w", half.dir, d.Name, err)
			}
			add(filepath.Join(half.dir, d.Name+".md"), body)
		}
	}

	// The alias index is the only file outside either half, and it goes to the
	// skill root because it applies to both. It is also the first file to read:
	// a query in the customer's own words matches nothing in an English corpus,
	// and grep reports that identically to a subject the material genuinely
	// lacks.
	//
	// The wiki's own README needs no line here. It used to, when it sat outside
	// the corpus directory; since the material moved to `internal/corpus` it is
	// an unlisted document of the wiki half, so Landing carries it - and while
	// both were here, two jobs wrote the same path.
	aliases, err := wiki.Aliases()
	if err != nil {
		return nil, fmt.Errorf("read the alias index: %w", err)
	}
	add("aliases.md", aliases)

	// One file per shape, beside `wiki/` and `usecase/` so that the pointers in
	// them are `../` like everything else. What is landed here is the whole of
	// Goal's second point - what has to be obtained from the customer before a
	// shape can be built - and until now it was 125 lines of Go that no grep
	// could reach.
	for _, d := range needs.Documents() {
		addDescribed(filepath.Join("needs", d.Name+".md"), d.Description, d.Body)
	}

	// Four activities, beside the rest for the same reason.
	for _, d := range brief.Documents() {
		addDescribed(filepath.Join("brief", d.Name+".md"), d.Description, d.Body)
	}

	// The ten stages, minus the paragraphs that render this repository's own
	// state - see stage.Static for what that split is and why a guide could not
	// simply be written out.
	guides, err := stage.StaticDocuments()
	if err != nil {
		return nil, fmt.Errorf("render the guides: %w", err)
	}
	for _, d := range guides {
		add(filepath.Join("guide", d.Name+".md"), d.Body)
	}

	// The root index is generated from what actually landed, so it cannot
	// disagree with the tree beside it. It is deliberately not a second copy
	// of what `wiki/index.md` and `usecase/README.md` do - those group their
	// documents by the question each answers, which is judgement and is worth
	// reading. This carries what neither can: both halves in one place, as
	// paths; the rule that turns a pointer into a path; and what is NOT here.
	index, err := corpusIndex(jobs, described)
	if err != nil {
		return nil, err
	}
	add("index.md", index)

	add("SKILL.md", corpusSkill)
	return jobs, nil
}

// corpusIndex builds the map at the root of the landed copy.
//
// **It takes the jobs rather than the corpus** so that it lists what was
// actually written. Reading the corpus again would let the two drift, and the
// drift that matters is the one where the index names a document the export
// skipped - a map to a file that is not there is worse than no map.
func corpusIndex(jobs []job, described map[string]string) (string, error) {
	var b strings.Builder
	b.WriteString(corpusIndexHead)

	for _, half := range []struct{ dir, what, guide string }{
		{"wiki", "what the platform has, and which CR a UI name maps to", "wiki/index.md"},
		{"usecase", "how one deployment shape is assembled, field by field", "usecase/README.md"},
		{"needs", "what to get from the customer before a shape can be built", ""},
		{"brief", "what has actually been got wrong, before you do the thing", ""},
		{"guide", "which decision to make now, and what it costs to change later", ""},
	} {
		fmt.Fprintf(&b, "\n## `%s/` - %s\n\n", half.dir, half.what)
		// A half with its own grouped index is listed there, by the question
		// each document answers; a second flat copy here only doubles what a
		// reader has to get through before the first answer.
		if half.guide != "" {
			fmt.Fprintf(&b, "Listed, grouped by the question each answers, in [`%s`](%s).\n", half.guide, half.guide)
			continue
		}
		b.WriteString("| document | covers |\n|---|---|\n")

		prefix := filepath.Join(corpusSkillDir, half.dir) + string(filepath.Separator)
		for _, j := range jobs {
			if !strings.HasPrefix(j.target, prefix) {
				continue
			}
			rel := filepath.ToSlash(strings.TrimPrefix(j.target, filepath.Join(corpusSkillDir)+string(filepath.Separator)))
			name := strings.TrimSuffix(filepath.Base(j.target), ".md")
			doc := kb.Parse(name, j.body)
			// **The description, and the title only when there is none.** A
			// title says what a document is about; a row in an index has to say
			// whether it is the one the reader needs, and those are different
			// claims - "Decide how the work splits into projects" tells nobody
			// what is inside it. Every document that carries a `description:`
			// renders that here, which is why the field is on the document and
			// not in this function.
			covers := described[j.target]
			if covers == "" {
				covers = doc.Description
			}
			if covers == "" {
				covers = doc.Title
			}
			if covers == "" {
				covers = name
			}
			fmt.Fprintf(&b, "| [`%s`](%s) | %s |\n", rel, rel, covers)
		}
	}

	b.WriteString(corpusIndexTail)
	return b.String(), nil
}

// corpusSkill makes the directory discoverable and says what the exported form
// needs that the pages themselves do not carry. What each half is for is in the
// READMEs written beside it, so it does not restate them.
//
// **Every kind that lands is listed here.** Three of the five arrived after
// this file was written and were not added to it, so an agent reading the one
// document meant to introduce the directory was told about two of them.
const corpusSkill = `---
name: asgard-platform
description: The Asgard platform as greppable files - what the platform has, which CR a UI name maps to, how each deployment shape is assembled field by field, what to get from the customer before one can be built, and where each has been got wrong before. Use when writing or reading an Asgard CR or Helm chart, when a customer names something and you need to know what it maps to, before a customer meeting, or before answering any question about what the platform can do. Read aliases.md first when the question came in a language other than English.
---

# The Asgard platform, as files

The platform knowledge an agent in a customer repository does not otherwise
have. That repository describes one customer's systems, not the platform they
run on.

    wiki/       what the platform has, and which CR a UI name maps to
    usecase/    how ONE deployment shape is assembled, field by field
    needs/      what to get from the customer before a shape can be built
    brief/      what this activity gets wrong, before you do it
    guide/      which decision to make now, and what reversing it costs
    aliases.md  what a customer said -> what to search for

Start at [` + "`index.md`" + `](index.md): it says which directory answers which
kind of question, how a pointer becomes a path, and what is not here.
` + "`wiki/index.md`" + ` and ` + "`usecase/README.md`" + ` list their documents grouped by
the question each answers.

This directory is generated. ` + "`asgard-cli init`" + ` writes it from the corpus
inside the binary and the next run replaces it, so do not edit it; file a
correction instead.

## Grep it

    grep -ril "<term>" .
    grep -n "<term>" wiki/processors.md

## Two things grep will not do for you

Read ` + "`aliases.md`" + ` before searching a question that arrived in another
language. The material is English, so a word taken from what somebody said may
match nothing, which looks the same as a subject the material lacks. The file
has two tables: words that replace a term, and names that are added to it.

Check ` + "`wiki/glossary.md`" + ` for the word you searched. A few words mean one
thing here and something else to a customer. ` + "`payment`" + ` is billing between
Asgard and the customer, and also the customer's own payment gateway; results
in the wrong sense look like an answer.

## When the answer is not here

File a page that is wrong, or a question these files do not answer, rather
than working around it. This material is compiled into the binary, so a note in
one repository reaches no other engagement:

    asgard-cli issue-report --new > report.md
    asgard-cli issue-report --send report.md

## Staleness

These files came from one binary and a newer one may carry different pages.
Nothing in this directory can tell you which:

    asgard-cli init

That compares what is here against the running binary and reports its state.
` + "`ahead`" + ` means these files were written by a newer build than the one you are
running: your binary is the stale one, and ` + "`--force`" + ` would be a downgrade.

The platform's own reference material is a separate half with its own record -
` + "`asgard-cli skill status`" + ` - because a customer's server can be several
versions from this CLI in either direction, and only the server can say what it
accepts.
`

// corpusPrefix is the stamp-key prefix of everything corpusJobs writes.
var corpusPrefix = stampKey(corpusSkillDir) + "/"

// replaceCorpus removes the exported corpus when this repository's copy was
// written by a different version of this CLI, so that the write which follows
// lays it down fresh. It reports whether it removed anything.
//
// **This is the one place a scaffold deletes from a customer's repository**, and
// the exception is narrow on purpose. Everywhere else a file this binary no
// longer ships is reported as `Retired` and left, because somebody may have come
// to rely on it. A wiki page is different in one way that settles it: the whole
// directory is declared generated in its own SKILL.md, so nothing in it is
// anybody's work - and a page that was renamed upstream would otherwise leave
// both names on disk, where grep returns the old one with nothing marking it
// stale. Merging cannot fix that; only replacing can.
//
// Three conditions, all of which must hold:
//
//   - The stamp has records under the prefix. **An unrecorded directory is
//     somebody else's**, and this must not delete a directory it cannot prove
//     it wrote.
//   - Some record's CLI version differs from the running one. Same version means
//     the material is this binary's already, and the byte comparison in the main
//     loop handles an edit to it.
//   - The path is a directory rather than a symlink, so the removal cannot
//     escape the repository by following one out.
func replaceCorpus(root string, recorded map[string]Entry, running string) (bool, error) {
	var ours, differ int
	for key, rec := range recorded {
		if !strings.HasPrefix(key, corpusPrefix) {
			continue
		}
		ours++
		if rec.CLIVersion != running {
			differ++
		}
	}
	if ours == 0 || differ == 0 {
		return false, nil
	}

	dir := filepath.Join(root, filepath.FromSlash(corpusSkillDir))
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", corpusSkillDir, err)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("%s is not a directory, so it will not be replaced", corpusSkillDir)
	}
	if err := os.RemoveAll(dir); err != nil {
		return false, fmt.Errorf("replace %s: %w", corpusSkillDir, err)
	}
	return true, nil
}

// corpusIndexHead and corpusIndexTail are the judgement around the generated
// listing: what the two halves are, how a pointer becomes a path, and what is
// not here. None of it varies per page, so none of it is derived - and nothing
// in it carries a version or a count, for the reason at the top of this file.
const corpusIndexHead = `# The Asgard platform: the map

Five kinds of document, and a pointer from one to another is a path you can
follow:

    wiki/       what the platform has, and which CR a UI name maps to
    usecase/    how ONE deployment shape is assembled, field by field
    needs/      what to get from the customer before a shape can be built
    brief/      what has actually been got wrong, before you do the thing
    guide/      which decision to make now, and what it costs to change later
    aliases.md  what a customer said -> what to search for

They answer different questions and it is worth knowing which you have.
"Can Asgard do X" is ` + "`wiki/`" + `; "what goes in this field" is ` + "`usecase/`" + `; "what do I
have to ask them for" is ` + "`needs/`" + `; "where does this go wrong" is ` + "`brief/`" + `; and
"which decision am I making" is ` + "`guide/`" + `.

A pointer is a path, relative to the document it is written in:

    ../wiki/<name>.md     from a document inside one of the directories
    wiki/<name>.md        from ` + "`aliases.md`" + ` or this file, which are at the root

Written with ` + "`../`" + ` even between two documents in the same directory, so that a
pointer carries which kind it points at. Following one is opening a file, and
this lists everything a document points at:

    grep -o '\.\./[a-z]*/[a-z0-9-]*\.md' wiki/agents.md

Read ` + "`aliases.md`" + ` first if the question did not arrive in English. The corpus
is English, so a word taken from what somebody said may match nothing, which
looks the same as a subject the material lacks.
`

const corpusIndexTail = `
## What is not here

Everything the material points at is a path you can follow. There is one
exception, and it needs the ` + "`asgard-cli`" + ` binary:

| pointer | why it is not a file |
|---|---|
| ` + "`asgard-cli guide <name>`" + ` | half of it is here. A guide renders this repository's own state into its guidance - which projects exist, what is still open - and a file would freeze one moment of that. The decisions are in ` + "`guide/`" + `; run the command for where this repository stands |

The commands that answer that half directly, when it is all you want:

    asgard-cli project     what each chart declares
    asgard-cli question    what nobody has answered yet
    asgard-cli request     what the customer asked for
    asgard-cli task        the open task specs

## Staleness

These files came from one binary. A newer one may carry different pages, and
nothing in here can tell you which:

    asgard-cli init

It reports what is here against what the running binary carries, and replaces
this directory outright when the version has moved. Read it as the authority
over these files rather than anything written inside them.
`
