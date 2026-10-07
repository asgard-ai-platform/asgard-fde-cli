# How it is built

This document describes how the main capabilities are implemented. What the
tool is for is [Goal.md](Goal.md); what is still to do is [TASK.md](TASK.md);
what the commands do is [README.md](README.md); what lives in which directory is
[STRUCTURE.md](STRUCTURE.md); the rules for changing it are
[AGENTS.md](AGENTS.md).

Present tense. Why something changed is `git log`.

## How far each half goes

The corpus is over 100,000 words as `asgard-cli init` lands it; `go run ./hack
goal` fails if the landed corpus falls below that.

The chart half is the least finished of Goal's four points: `add` writes a
starting point, and the rest of what a production chart sets is shown in
commented skeletons, because those values are the engagement's to choose.
`go run ./hack spec-key-gap` measures it, and fails if that stops being true.

## The corpus

Every body of material has one reader. `kb.Corpus` is that reader and every
body declares one; `needs` and `brief` have no files, so they render documents
from Go structs into an in-memory FS and are read the same way.

    kb.Doc     what a document says about itself
    kb.Link    one pointer out of it
    kb.Corpus  one body of material, and how to read it

The fields are not listed here, because the struct is the source and a copy
here goes stale. `go doc ./internal/kb Doc` prints them for the commit you are
on.

| body | package | kind |
|---|---|---|
| wiki pages | `internal/wiki` over `internal/corpus/wiki/` | `wiki` |
| deployment extracts | `internal/usecase` over `internal/corpus/usecase/` | `usecase` |
| what to obtain from a customer | `internal/needs` | `needs` |
| what an activity gets wrong | `internal/brief` | `brief` |
| stage guidance | `internal/stage` over `internal/stage/prompts/` | `guide` |
| design-time skills | `internal/scaffold` over `internal/scaffold/templates/.agents/skills/` | — |

Add material to one of these, not beside them. A body with its own reader and
its own parse drifts from the others and no check notices. If new material does
not fit `kb.Corpus`, change `kb`.

Almost everything a document declares about itself is parsed from its prose: a
`# ` title, a summary paragraph, the two provenance markers. Frontmatter carries
two exceptions, `group:` and `description:`, and `go run ./hack index` renders
both indexes (the wiki's and the extracts') from them.

These two are in frontmatter because they cannot be derived. A grouping is the
question a section asks, and no parse recovers it. A description is what
somebody would come to the page for, which differs from the page's opening
thesis; deriving one from the other was tried and gives rows like "They are not
two of the same thing". The rest of an index row (the title, the path) is
computed, so the only hand-written part of a row is the judgement, and it lives
on the page rather than in the index.

`Docs` and `ParseDoc` let a body that is not one `<name>.md` per document join
anyway: a stage is a numbered prompt file, a skill is a directory with YAML
frontmatter. Both supply their own, and everything downstream reads `kb.Doc`.

There are two listing calls, and they answer different questions:

    List()      what a reader listing documents should see
    All()       plus the corpus's own bookkeeping - an index, a README

`Unlisted` separates them. `wiki.Landing()`, which is what `init` writes, uses
`All()`, so that the index travels with the material. The audit's source set is
also `All()`: every document that ships is checked, and using `List()` there
would hide dead references in the files it skips.

## Pointers

A cross-reference is parsed with the document into `kb.Link`, and everything
downstream (`--links`, `--orphans`) reads that one graph. Every document
pointer is a path, because every kind is written into a repository:

    ../wiki/<name>.md

`--links` enforces both directions. A path whose target `init` does not write
resolves here, where the corpus is whole, and goes nowhere in the repository
the material was written into. A pointer written as an invocation is refused in
the material, because a document pointing at a document points at a file;
`kb.Link.Path` tells the two apart. A help screen is not material: `brief` and
`guide` are commands, and naming one there tells somebody to run it.

What precedes the kind depends on where the pointing document sits:

| written in | form |
|---|---|
| a document inside one of the directories | `../wiki/x.md` |
| `aliases.md` or `index.md`, at the landed root | `wiki/x.md` |
| a design-time skill beside the corpus | `../asgard-platform/wiki/x.md` |
| deeper in a customer repository | `.agents/skills/asgard-platform/wiki/x.md` |

So `pathLinkRe` fixes only the tail: the kind, a lower-case name, `.md`. The
lower case stops a prose mention of `wiki/README.md` becoming a pointer.

`kb.Landed(prefix, invocation)` converts one form to the other and is the only
place that knows the set of kinds. `needs` and `brief` store invocations and
call it when rendering: the stored value says which document a claim belongs
to, and where the text is written decides the form.

`internal/corpus/` holds the material in the layout a repository receives it,
so that a path like `../usecase/x.md` resolves both here and in a customer
repository.

## The audits

    asgard-cli audit-material --help     every flag, and what each one answers

The list of flags is in `--help` and not repeated here. What follows is why the
less obvious ones work the way they do.

`--links` and `--orphans` read the same graph from opposite ends. A dead
pointer is noticed, because the reader follows it and finds nothing. A document
nothing points at is not noticed: it is correct and never read. An index does
not count as a pointer in `--orphans`, because a document reachable only from a
list is found only by somebody who already suspects it exists.

`--bare` keeps the graph complete. A reference written without a path (a
same-directory markdown link, or a name on its own) resolves for a reader today
and is checked by nothing, so a renamed page breaks it silently. Indexes collect
most of these because they consist of references. The check is safe because it
only matches hyphenated tokens: `agents` is a CR field and `verify` is a
command, while a hyphenated token matching a document name has no other reading.

`--unverified` reports `needs` and `brief` separately. They render every
document from one shared provenance string, so the marker is present by
construction and the check cannot fail on them. Counting them as passed would
overstate what was checked.

`--sources` collects the upstream commit recorded at every point of synthesis.
A page's Sources block, a pinned table's `const` in `internal/gate` and the
raw-sources table in the wiki README each record which commit of which upstream
a claim was read at.

It does not require them to agree. Provenance is per claim, so a corpus read
over time normally cites two commits of one upstream; requiring one commit
everywhere would fail every time somebody re-reads a single section.

So it prints the spread. A sweep that was meant to move every citation and
moved only some looks the same as a corpus read over several days, and no check
can tell them apart. It fails on an upstream cited somewhere and missing from
the raw-sources table: an undeclared dependency, which cannot be explained by
timing.

It does not check whether a commit is current, because nothing inside this
repository can; that is why the commit is recorded. `go run ./hack sources`
reads the clones and says how far behind each is.

`--commands` does for the tool what `--links` does for documents: it resolves
every `asgard-cli <command>` against the command tree this binary answers to,
in the material, in the scaffold templates, and in this repository's own string
literals, which a customer sees only when the tool prints one. `selfsrc` embeds
the source and `internal/cli/self.go` parses it, so a comment recording that a
command was removed does not count as naming it.

A command reference is what is written as code: backticked, fenced, indented,
or inside a double-quoted span that contains only a command. The last form
exists because a Go raw string cannot hold a backtick, and the root help is one.
Two things are checked separately because that form cannot see them: a bare
name with no `asgard-cli` in front of it, swept for over the closed set of
removed names in `replacements`; and a path that is not a document pointer,
which is `--paths`.

`--paths` exists because these documents land in somebody else's repository,
where "this repo" is theirs and `source/SOURCES.md` is not there. Naming a path
is allowed, and provenance should name the file it came from; the rule is to
name the repository the path is inside, on the same line.

The audits check the material, not the capability: `asgard-cli init` could
start requiring a session with every audit still passing. `go run ./hack goal`
covers that side. It runs the tool in a temporary directory with no network, no
account and no repository, and holds Goal.md's four points against what
happens.

The checks that need somebody else's repository are not in the binary; they
are Go under `hack/`. `go run ./hack tables` holds the gate's pinned tables
against the generated CRDs, `processors` and `counts` hold the material's own
tables and figures against what they were distilled from, and
`hack/verify-references.sh` runs the gate over the reference deployments.
`go run ./hack list` says what each one needs. They do not ship because the
repositories they read are not vendored. AGENTS.md says why that directory is
compiled rather than scripted.

## Retrieval

There is no search command. `asgard-cli init` writes the material into the
repository and `grep` is how it is searched. The landed `SKILL.md` and
`index.md` state two things a grep does not do for itself.

`aliases.md` is applied before searching, not after a search fails. The corpus
is English and a customer conversation usually is not, so a term taken from
what somebody said matches nothing, which looks the same as a subject the
material lacks. There are three tables, and they differ in what a row does to
the query:

| table | the row | why |
|---|---|---|
| what a customer says | *replaces* the term | a Chinese term appears nowhere in an English corpus, so keeping it only adds a term that lands nowhere |
| names the material covers | *adds* to the term | SHOPLINE is written verbatim in a page, and that page is the best answer there is |
| names it only routes | *adds*, and says nothing answers it | the material does not name 綠界; what it has is the shape the thing belongs to |

The third table needs its own heading because a row that routes looks like a
row that answers, and a reader who cannot tell them apart takes results about a
shape as results about the product they asked for. A row moves up a table only
after somebody did the search and wrote down what came back.

`glossary.md`'s first table lists words with two senses here. `payment` is
billing between Asgard and the customer, and also the customer's own payment
gateway. Both sets of results are correct, so the wrong one looks like an
answer, and the search cannot report the mistake because it found something.

The table is reachable because it contains the word. A grep for `payment`
returns `wiki/glossary.md` among its hits for every word with a row, so the
warning arrives in the same result set as the ambiguity, without the reader
having to read an instruction first. That is why the table lists words rather
than describing them in prose.

When the material has no answer, `asgard-cli issue-report --new` writes the
report with what the tool already knows filled in: the version, what the charts
declare, the `check` report, how much work is open, and the paths of shipped
files the engagement edited in place. This is the only way missing knowledge
gets back in, and it is Goal's fourth point.

The report's last section, "What I now know", is for a discovery rather than a
defect: something the platform does that the material does not say, found out
on a deployment. The maintainer writes it onto the page it concerns as a fact.
`audit-material --unchecked` ends with the filing line.

No embeddings, no vector index. Synthesis happens once, into a document, rather
than on every query. That is the [llm-wiki](https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f)
form, and the reason for using it.

## Landing

`asgard-cli init` writes the material into `.agents/skills/asgard-platform/`,
so an agent in a customer repository reaches it with `cat` and `grep`:

    index.md    generated from the jobs actually written
    aliases.md  the alias index
    wiki/ usecase/ needs/ brief/ guide/

`internal/scaffold/corpus.go` builds those as jobs in `scaffold`'s own plan, so
they follow the existing contract instead of getting rules of their own:
`shipped()` covers the prefix, the stamp records them, `gate`'s `shipped` step
checks them.

`guide` lands as its static half only. A stage renders this repository's own
state (which projects exist, what is still open), and that half stays a
command, because a file would freeze one moment of it. The split is by
paragraph: placeholders are substituted first, then any paragraph still
carrying a template action is dropped.

Nothing written is interpolated. Staleness is found by byte comparison, so a
version string in a landed document would make every repository report behind
on a release that touched no page.

### The record

`.asgard-scaffold.json` holds a digest and a CLI version per file, committed
beside the material. A byte comparison says a file differs; the record says in
which direction, which is what these states report:

| state | meaning |
|---|---|
| `behind` | this binary is newer, nobody here edited it — `--force` takes it |
| `edited` | somebody here changed it; `--force` would discard that |
| `ahead` | written by a **newer** CLI than the one running; `--force` refuses |
| `stale` | differs, and which way round is not knowable |
| `retired` | this binary no longer ships it |

### Two rules of its own

A version change replaces the corpus subtree outright. It is the only delete
the scaffold performs, and `replaceCorpus` requires all three: the stamp has
records under that prefix, some record's version differs, and the path is a
directory rather than a symlink. The whole directory is generated, and a page
renamed upstream would otherwise leave both names on disk where grep returns
the old one.

A file with a managed region is never rewritten whole. `AGENTS.md` ships
sections an engagement fills in; markers bound the part this CLI owns and only
that part is replaced. The digest recorded after a merge says this CLI wrote
those bytes, which does not mean it wrote all of them.

## Keeping it current

A corpus needs two things that no static check answers. Each is a listing
rather than a check because each reports a question.

What a change owes a re-read. `go run ./hack related` reads the in-edges of
every document a change touched, off the same `kb.Link` graph `--links` walks
from the other end. `--orphans` asks whether anything points at a document;
this asks what does. A change to one page names about a dozen documents, which
is a reading somebody will do, where "read the corpus again" is one they will
not.

What has already been answered. `go run ./hack verified` keys each check on
the digests of the classes of input it reads (the corpus, the Go source, the
root documents, each upstream clone at its commit) and skips the ones whose
inputs have not moved. The classes are deliberately coarse: listing the exact
files a check reads would be a hand-kept copy of what the check does, and it
would go stale by missing inputs. A wrong skip is worse than re-running
everything, so a check that declares no class always runs, and a skip is
printed differently from a pass.

## Verification

`asgard-cli gate` runs, in order: `tools`, `repo`, `shipped`, `binding`,
`skills`, `lint`, `render`, `verify`. Two steps need the platform, and a skip
is printed differently from a pass.

`verify` reads rendered CRs against each other and against tables pinned from
the CRDs: enums in `internal/gate/enums.go`, field constraints in
`constraints.go`. Both are warnings, because a pinned copy can go stale in
either direction. The platform deleted a cron pattern its own regex had copied
wrong, and the pinned row then reported four schedules an FDE would want as
violations while admitting one the apiserver refuses. `constraints.go` carries
that reasoning; `go run ./hack tables` holds both against the CRDs.

Those tables are keyed by kind and path, not by field name. A json field name
is not a location: `Loader.spec.schedule` is an unconstrained string while
`Trigger.spec.cron.schedule` carries a pattern, and a table keyed on the name
alone would apply one's rule to the other.

It does not reproduce the platform's checks. Whether a CR is admitted is the
apiserver's decision and no client is issued cluster credentials, so a copy of
those rules here would drift, and would still miss the two that matter: a field
the CRD silently prunes, and a rejection only the apiserver produces. 28 of
the CRDs' 227 enforced CEL rules are `self == oldSelf`, comparing a proposal
against the object already on the cluster, and a render is one object with no
history. (A marker in the Go types and a rule on the cluster are different
questions with different answers; `internal/corpus/wiki/crd-rules.md` has both,
and `go run ./hack tables` recomputes them.)

A passing gate means the change is worth pushing. The authority is the plan:

    asgard-cli pipeline runs watch --release <name> --ref <tag>

## Chart authoring

`asgard-cli add <kind> <name>` writes a CR skeleton into a project's chart.
`generate.Kinds` is the set, each naming the wiki page and the extract that
explain it. Those names are pointers like any other and `--links` resolves
them, which a prose search cannot do.

What the generator writes is the conventions applied: naming, the display
annotations, what belongs in `values.yaml` and what stays in the template.

A field that is a choice gets its shape written into the template as a
comment, rather than a line telling the author to read the extract. The extract
and the pointer remain, as the reading order `add` prints, but the shape needed
while typing the field sits beside the field. The measure entry's required
fields, the join's matched dimension lists, the web Syncer's timings that have
no defaults: each is in `internal/generate/templates/` and also in the extract
it came from.

This duplication is kept on purpose, because of what a pointer costs at the
moment of writing. A skeleton is the only place that says how often a thing is
right: prose says what a field is for and a schema says what shape it takes,
and neither says "most layers need none of these three". `AGENTS.md` states
this as its own rule ("if it is a list, does it say which of its items are
load-bearing?"), with the deck it came from.

[WikiSkill](https://arxiv.org/abs/2608.27454) measured the same effect and is
relevant before anyone removes these comments: giving its agent access to the
wiki degraded the skills it produced, because knowledge available through a
pointer was fetched rather than compiled into the artefact. Their conclusion is
that a pointer does not replace the fact at the point of use, not that an agent
should not read a wiki; this tool's first goal is that it can.

Out of scope deliberately, and Goal.md says why: the namespace and the
environment id, which the platform injects as `.Values.asgard.*` on every run,
and whether the thing deploys at all.
