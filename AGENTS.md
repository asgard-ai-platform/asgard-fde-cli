# AGENTS.md

Rules for AI agents working in this repo. Human contributors follow the same ones.

Please remove all mannered prose. Write the plain sentence: say what is true and
what to do, without aphorisms, rhetorical reversals ("X is not Y, it is Z"),
dramatic asides or bold used for effect. This applies to everything this
repository ships and to its own documents, commit messages and PR bodies.

## What this repo is

[Goal.md](Goal.md) says what this tool is for. Read it first. It makes four
claims, and a change can break any of them:

  1. An agent can get the Asgard platform's knowledge - what the platform
     has, which CR a UI name maps to, how one shape is assembled field by
     field, and where each has been got wrong before. It works offline, with
     no repository, because the question is asked in a meeting and a tool that
     needs a directory first will not get asked.
  2. It is useful in a meeting: it names what has to be obtained from the
     customer before a shape can be built - a credential, an endpoint, a
     network path, a test environment, an approval queue.
  3. It helps write the Helm charts, and deliberately not the namespace,
     the environment id, or whether the thing deploys.
  4. When the knowledge is missing, the tool's own output says where to file
     that, so the agent does not have to work it out.

The four depend on each other. Point 1 is the base; 2 and 3 are the same
knowledge used in a meeting and in a chart; 4 is how missing knowledge gets
added. The knowledge base is therefore the product, and most of this file is
about the material rather than the commands.

Read the four before changing anything that prints: three of them are about
what an agent can reach and the fourth is about what the tool says when it
cannot.

It writes files into a *customer's* repository and never holds state of its
own. The division: `Goal.md` is the goal, [TASK.md](TASK.md) is what is still to
do, [README.md](README.md) is what the commands do,
[APPROACH.md](APPROACH.md) is how the main capabilities are implemented,
[STRUCTURE.md](STRUCTURE.md) is what lives in which directory, and this file is
how to change the code and the material without breaking it.

Before saying anything here is correct, load
`.agents/skills/consistency-checks/SKILL.md`. It is the method: what to run,
in what order, what each check cannot see, and how to do the part no check
does. `go run ./hack pass` derives the list of checks.

Most of the value is in the material, not the code. It is one corpus, and when
adding anything, first decide which part it belongs to:

| part | answers | lives in |
|---|---|---|
| wiki pages | what the platform is, and who each piece is for | `internal/corpus/wiki/` |
| usecase extracts | how one shape of deployment is assembled, field by field | `internal/corpus/usecase/` |
| needs | what to get from the customer before a shape can be built | `internal/needs/` |
| briefs | what has actually been got wrong before one activity | `internal/brief/` |
| stage prompts | what to weigh at one point in the work | `internal/stage/prompts/` |
| scaffold templates | the part of a customer repo that is the same every time, including the skills the customer's agent loads | `internal/scaffold/templates/` |

All of those but the scaffold templates are written into a customer
repository, under `.agents/skills/asgard-platform/`, by `asgard-cli init`;
the scaffold templates are the repository itself. So a change to any of them
ships twice: into the binary, and into every repository that runs `init` after
it. `internal/scaffold/corpus.go` is what writes them and
`scaffold.replaceCorpus` is what replaces them when the version moves.

`needs` and `brief` are Go rather than markdown because each row carries the
document that owns its claim, and that pointer is checked; the markdown is
rendered from them.

[STRUCTURE.md](STRUCTURE.md) walks every directory.

State a rule as the rule, in the present tense, and keep no log. Do not justify
a rule with what went wrong before: that reads as evidence, works as a
changelog, grows without bound, and every reader pays for it in context. A page
states the constraint and what it costs to break; what happened to this
repository belongs in `git log` and nowhere else. A platform or customer
failure is different: it is a fact about the platform, which is what the
material is for, and it carries its provenance.

The same applies to commit messages. The subject says what changed and the body
says why the rule is what it is. Do not write a post-mortem.

Do not keep a pending log. When something is done or decided against, delete it:
the TASK.md entry, the README section, the commented-out configuration. The
same goes for a question once it is answered - the answer goes on the page it
concerns and the question is removed. TASK.md holds only work still to do.

Keep each fact in one place and link to it from everywhere else. A trap that
belongs to a CR template is not also explained in a wiki page. When you are
about to repeat a paragraph, link instead: two copies drift, and the reader
cannot tell which is current.

In general, reference the source of truth instead of restating it. A copied
paragraph is easy to spot; the costly copies do not look like copies:

    a count in prose            the tree is the source, and every page added
                                makes the count wrong
    a constraint pinned here    the CRD is the source, and it deletes rules as
                                readily as it adds them
    a list of what exists       the binary is the source - the checks, the
                                commands, the kinds
    a value copied into a       the platform is the source, and copying it
    chart                       needs a route to obtain it that may not exist
    the same constant in        one declaration, and the other package reads it
    two packages

A checker that holds a copy does not fix the drift; it makes the copy
mandatory: every change to the source turns the check red, and the repair is to
retype a value the check has just computed. Check the claim itself instead -
`go run ./hack goal` checks that every document the binary carries lands in a
repository, set against set, with no number written anywhere.

So before writing a number, a list, or a constant, ask what owns it and whether
you can point at that instead. If nothing owns it, it is a judgement and
belongs in prose. If something does, the prose carries the judgement and the
program carries the value.

The reading order is wiki, then extract: an extract assumes you already know the
platform has that shape. `asgard-cli add <kind>` prints both, in that order.

## The corpus is a wiki, and these are its rules

The shape is the [llm-wiki
pattern](https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f) -
raw sources that are never vendored in, a corpus that is rewritten continuously,
and a schema a person changes deliberately. The [Open Knowledge
Format](https://cloud.google.com/blog/products/data-analytics/how-the-open-knowledge-format-can-improve-data-sharing)
arrived at the same shape independently and is worth reading for the
vocabulary. It describes itself as solving the format and nothing else. Going
stale, what else a change is owed, and whether what a document points at still
exists are outside it; the rules below and their checks cover those, and
`Goal.md` says why that half matters. Take its frontmatter, but do not use it
as a reason to relax a rule below. `internal/corpus/wiki/README.md` states the
rules for the wiki at more length. The rules below apply to every part, and
each has a check, so a rule that stops holding shows up in `asgard-cli gate`
rather than in a list somebody has to maintain.

Every part uses one schema. A document opens with a `# ` title and a summary
paragraph, and carries `**Checked:**` / `**Unchecked:**` - what it has been
held against, and what it has not. `internal/kb` parses that and nothing else.
Material that needs its own reader and parser drifts from the rest without any
check noticing, so if new material does not fit `kb.Corpus`, change `kb`.
`Docs` and `ParseDoc` are the two hooks for a body that is not one
`<name>.md` per document - a numbered prompt file, a skill directory with YAML
frontmatter - and both still produce `kb.Doc`.
Check: `audit-material --unverified`.

A document declares two things about itself in YAML frontmatter at the very
top, and the index is rendered from them. Nothing else goes in the frontmatter:

    ---
    group: While building
    description: what each processor type takes, and the fields that decide behaviour
    ---
    # The processors, and the fields that decide behaviour

Both are needed because neither can be derived. A grouping is the question a
section asks. A description is what somebody would come to the document for,
which differs from what it opens by saying - `console.md`'s thesis is "The Console
does no business work", and what an index owes a reader is "the permission
layers, the pages that disagree with each other, Workspace Management". A
description rendered from the opening sentence does not tell a reader why to
open the page. Everything else in a row - the title, the path, the order - is
computed.

Never edit an index by hand. `go run ./hack index --write` renders
`wiki/index.md` and `usecase/README.md` from the documents; the check fails when
either has drifted. Adding a page means writing the page and its two
frontmatter lines. What stays hand-written is which sections exist, their
order, and the order of rows inside each. That order is a reading order rather
than alphabetical, it is read back out of the file, and a new document is
appended so that adding one does not move anything somebody arranged.

A description does not update when its page changes. It is authored, so the
row can stop describing the page without anyone noticing. When you change a
page, read its description again.
Check: `go run ./hack index`.

Every pointer is a path, because every document lands in a customer repository:

    ../wiki/<page>.md      from a document inside one of the directories
    wiki/<page>.md         from `aliases.md` or `index.md`, at the landed root

`internal/corpus/` holds the material in the layout a repository receives it, so
one form resolves in both trees. It is written `../` even between two documents
in the same directory, because one form that states its own kind is simpler
than two forms that depend on where the reader is standing.

The gate refuses two other ways of writing a pointer, and both look fine to a
person:

- **an invocation.** The tool's name, a kind and a page name. A document
  pointing at a document points at a file. No example is written here, because `--commands` reads this file
  and would resolve it. A help screen is different: `guide` is a command, and
  naming it there tells somebody to run it.
- **a name with no path.** `write-path` and `[`x`](x.md)` both resolve for
  the current reader and no check reads them, so a renamed page breaks them
  silently. They accumulate in the two indexes, because an index consists
  entirely of references.

Check: `audit-material --links` and `--bare`.

Do not point at anything a customer repository does not have. These files land
in somebody else's checkout, where "this repo" is theirs and `APPROACH.md`
does not exist. Citing a file in another repository is correct, because that is
provenance; name the repository on the same line.
Check: `audit-material --paths`.

Retrieve guidance by subject, never by position. "You are at step 4" describes
a sequence no engagement actually follows, and a reader cannot check a position
against anything. A reader can act on a claim about the repository in front of
them or about what they are about to do - which is what `brief/` is and why
`guide` renders against this repository. A document reachable only by arriving
at a particular step is in practice unreachable, because a reader opens only
the stage they are told they are in and what that stage points at. Check: `audit-material --orphans`, which checks the
same rule from the other end.

Output an agent reads has to be parseable. Column-aligned output is for the
FDE and is not an interface; a command an agent acts on must offer a form that
does not have to be parsed out of `%-9s`. The material itself has no format
flag, because it is whole documents on disk.

## Language

Go code, command output, error messages, command help, and every document
this repository ships are in English. That is the whole corpus, plus
`README.md`, `TASK.md`, `STRUCTURE.md`, `APPROACH.md` and this file.

Two documents here are in Chinese on purpose. `Goal.md` is the goal as its
author states it, and `README.zh-TW.md` is the Chinese half of the README. Both
are read by people rather than shipped to an engagement.

```go
// Save writes cfg to path.
return fmt.Errorf("write %s: %w", path, err)
```

One body of material is in Chinese on purpose: parts of
`internal/scaffold/templates/`, because the generated repository is read by the
customer's own engagement, in their language.

The corpus uses one language, and the mapping from the customer's words lives in
a separate file. `internal/corpus/aliases.md` is the mapping, and it lands
beside the material so it can be read before a grep rather than after one comes
back empty. Translate the customer's words first, then search. Translating only
after a search fails is worse, because the query matches the alias table's own
row about the word. Product labels keep their own names (Managed
Agent, Drive, Context Index are what the UI says).

An index is not a page and is not stored among the pages. Because the alias
table lists every alias, it is the one document carrying every term of a
translated query, so among the pages a search for a subject returns the word
list rather than the page. `aliases.md`
sits at `internal/corpus/`, above both halves, because it applies to both;
`internal/corpus/wiki/index.md` is the wiki's own catalogue and stays with the
pages it lists. Both are `Unlisted`, so `List()` hides them and `All()` does
not. `audit-material --links` reads them - their rows carry pointers - and
`--orphans` deliberately does not count them, because a list that names every
page would make every page look reached.

That the alias table stays out of the way is checked. A row that translates a word correctly but sends the reader
somewhere the search does not surface has moved the failure rather than fixed
it, because the query still returns something and it looks like an answer. So
for every row that names the page answering it, a search for that row's words
has to reach that page, and no page listing other pages may outrank it. One
word of difference between the row and the page is enough to break this.
Check: `go run ./hack aliases`.

Do not "fix" the scaffold templates into English. Do not start a second exception
without saying why it is needed.

## `hack/` is Go

A check that is not compiled goes unnoticed until it is wrong. These
scripts are the maintainer's gate and most of them run only when somebody runs
them by hand, so a typo in a branch that is rarely taken goes unnoticed. A
missing import, a missing map key, a helper called with the wrong signature
are compile errors in Go, and `go build ./...` and
`go vet ./...` already run on every push.

So a check goes in `hack/` as Go, under the one `main` package with a
subcommand each, and shares what it needs through `hack/internal/`. Three things
follow:

  - No `yq`, and no subprocess for YAML. `gopkg.in/yaml.v3` is already a
    dependency, so a CRD is unmarshalled rather than shelled out to and parsed
    back out of JSON.
  - Reading the corpus goes through `internal/kb`, not a second regular
    expression. That package exists because two readers of one format drift.
  - `go test ./...` can reach them.

A script that only drives other programs - helm piped into the binary, say -
may stay a shell script, because rewriting that in Go gains nothing. None does
today.

## Plain ASCII, no emoji

No emoji, and no decorative Unicode either - no check marks, arrows, box-drawing
characters. They render inconsistently across terminals, get mangled in logs and
CI output, and are awkward to grep for. This applies to the Chinese material too:
Chinese text is fine, and `->` is used instead of an arrow glyph.

Mark status with words or ASCII punctuation:

```
ok  .asgard-pipeline.yaml
- workspace must not be empty
```

Some scaffold templates and extracts still carry decorative characters from
before this rule. Fix one when you are editing the file for another reason; do
not sweep them all as a change of its own.

## Every command must document itself

A command is not finished until it can explain itself:

- `Short` is a one-line summary, shown in the parent's command list.
- `Long` explains what the command does, its defaults, and how it fails. It must
  say more than `Short` repeats. Include an example when the usage is not obvious.
- Every flag needs usage text, and says there if it is required or what it
  defaults to.
- Never register a flag the code does not read. An option that silently does
  nothing is worse than no option, because the user believes it took effect.

cobra gives every command a `--help` for free but does not stop it being empty,
and nothing here checks. Read the `Long` of a neighbouring command and match it.

To add a subcommand: write `newXxxCmd()` in `internal/cli/`, and register it
through `addTo(cmd, group..., ...)` in `root.go`. Do not use `AddCommand`: the
group is required, cobra panics on a `GroupID` the parent does not have, and a
command added the other way lands in "Additional Commands", where the next
reader will see it and ask why.

## Every directory the scaffold writes must document itself

Same rule as the commands above, applied to `internal/scaffold/templates/`. A
directory that arrives in a customer's repository with nothing in it but its own
name is a question the FDE has to ask somebody.

Say both what goes in it and what does not.

The second half matters most. `assets/skills/` and `.agents/skills/` differ by
one word and are opposite mechanisms - one is synced into a running system, the
other is read by the agent editing this repository - and a sentence in a README
saying which is which is what stops somebody putting a file in the wrong one.
Every "not" in those files is there because the confusion is likely.

So, for a directory whose name is ours to choose:

- It contains a `README.md` (or an `_index.md` where a command writes into the
  same file) that names its purpose in the first line.
- It says what it is not, naming the directory somebody would otherwise
  confuse it with.
- One that starts empty says why it exists - `assets/skills/` is
  empty in a fresh repository on purpose, because a path invented per engagement
  is a path that differs per engagement, and `SkillSet.searchPaths` carries it.

git cannot track an empty directory, so the README is also what makes the
directory exist in a clone.

The rule does not reach a directory whose shape somebody else decided.
`.agents/skills/` and the Claude Code plugin under
`internal/scaffold/templates/plugins/` are somebody else's layouts,
`.claude-plugin/` holds a manifest with a schema, and `db-query/scripts/` holds
the scripts its own `SKILL.md` documents. The external convention says what
those are for, and a README in each would be a second answer to a question
already answered.

## A command that changes something says what it is changing

Before it acts, on stderr, in one line: the Platform API it is about to change,
the profile in effect, and how that profile came to be the one in effect.
`actingOn` in `internal/cli/context.go` is the helper; `actingLocally` is the
variant for the two commands that write the checkout's binding rather than
reaching the platform.

The URL identifies the installation; the profile name is a local label. Two
people's `dev` can point at different installations and one of them can be
production, so a line naming only the profile gives only the part that means
nothing to anyone else.

The line is always printed. If it were printed only when the profile was
implicit - not typed on the command line - it would disappear once somebody
types `--profile` out of habit, and then its absence would carry no
information, in the same way that a skipped gate step is not a pass.

It goes to stderr so that `--format json` keeps a parseable stdout. Adding the
same facts *into* the JSON was not done for the pipeline group: those commands
return a bare object or a bare array, and wrapping them to add three fields
would change the shape every existing reader depends on. `gate` was already
emitting a map, so it carries a `platform` key.

## The gate

Prose and material carry almost all of the risk here, so the checks are aimed
there. A handful of Go tests cover parsing and matching rules where a wrong
answer is silent; everything else is checked by reading what ships.

    go run ./hack verified run the gate, skipping what has not changed
    go run ./hack pass     every check, in the order to run them
    go run ./hack list     what each one is for, and what it needs

Run `verified`. It keys each check on the digests of the classes of input it
reads and skips the ones whose inputs have not changed, so a pass does not start
from zero. A skip is printed differently from a pass, and the record lives in `.out/` rather
than in the repository, because it describes this machine's tree and a shared
record would describe somebody else's.

The list of checks is not written here. It is derived from the binary's own
flags, this directory's contents and the gate's own subcommands, so it cannot go
stale. A list in a markdown file does.

`make gate` runs all of it except `--urls`, which is `make audit-urls`. The
Makefile mirrors CI and does not define it: `.github/workflows/ci.yml` is
what decides whether a change merges, so a check added there and not to the
Makefile is still enforced, and one added to the Makefile alone is a
convenience.

`--paths` and `go run ./hack doc-paths` check the same rule from the two sides.
The audit reads what lands in a customer repository and fails on a path only we
have; the script reads the documents that never land - the root documents, both
READMEs and `hack/`'s own - where naming our own paths is intended, and fails
when one of them is gone. It checks a package-qualified Go symbol the same way,
and resolves it inside the package that owns it: a search of the whole tree
cannot tell one package's Index from strings.Index, and would pass a reference
to a symbol whose package has been deleted. Do not write a
deleted symbol in backticks, even to explain that it is deleted: this check
reads this file and will try to resolve it. It needs the checkout, which is why
it is in `hack/`.

CI runs the checks that need nothing but this repository: the audits, the
Go steps, and the `hack` checks that read only this tree. Which ones those
are is not written here: `.github/workflows/ci.yml` is the list, and it
carries a comment naming what it leaves out and why. The rest need a clone of
somebody else's repository - and asgard-core is private - so they are the
maintainer's to run, and `go run ./hack sources` says how far each clone is
behind. `--urls`
is excluded for a different reason: a third party's outage should not fail this
repository's build, and the gate has to work offline.

A customer repository's gate is one command, `asgard-cli gate`. That difference
is deliberate: what an agent runs after every edit has to be one command whose
definition lives in a binary. The gate here is for the maintainer, who is
editing that binary.

Three of the audits need more explanation than `--help` gives:

  - `--links` resolves every pointer, in prose, in the templates and in the
    generator's own `Wiki:`, `Extract:` and `AlsoRead:` fields - and fails a
    path whose target `init` does not write into a repository, because that one
    resolves here and not there. Run it after renaming or removing a page,
    which is the only way to leave a dead pointer behind. A dead pointer reads
    correctly and resolves to nothing, and the reader who follows it cannot tell
    that from a page they failed to find.
  - `--commands` does the same for the tool itself, and it reads what is
    embedded, which is what an engagement gets - plus these root documents,
    which `selfsrc.Docs` embeds for this purpose. They land nowhere, so the
    provenance rules do not apply to them, but a command they name has to
    exist. A sample of the tool's own output is read as prose, so a fenced
    block reproducing a line where `asgard-cli` is followed by an ordinary
    English word counts as a reference to a command of that name. Elide it
    rather than rewriting what the tool prints.
  - `--urls` reports rather than fails on a citation that already says the
    link 404s - a page marked `draft: true`, which asgard-docs does not
    publish. Disclosing it that way is how you keep such a citation.

None of those catches a wrong string in an embedded template, a pointer that
resolves to the wrong page rather than to none, or a generated CR the apiserver
would reject. So exercise the change by hand, against a scratch repository
outside this one:

```bash
go build -o .out/asgard-cli ./cmd/asgard-cli
cd $(mktemp -d)
/path/to/.out/asgard-cli init -y
/path/to/.out/asgard-cli project add app
/path/to/.out/asgard-cli add <kind> <name> --project app
/path/to/.out/asgard-cli check && /path/to/.out/asgard-cli render <release>
```

If the change touched a CR template, validate the rendered output against the
CRD schemas in [`asgard-kube/crd/`](https://github.com/asgard-ai-platform/asgard-kube/tree/main/crd) before believing it.
`helm lint` and `asgard-cli check` do not compare against the platform contract,
so a missing required field passes both.

## What is checked, and what is not

This section says which of three kinds of coverage a surface has; the method
is `.agents/skills/consistency-checks/SKILL.md` and the list is
`go run ./hack pass`. The pass runs the most volatile inputs first - upstream
before the material, because nobody here edits asgard-docs and it changes
without anyone here noticing.

Every check passing does not mean the repository is correct, and the three
groups below show the difference. Defects found by reading come from the third
group. So a claim that something is
done says which group it was in.

### Never report a pass as green while `TASK.md` has work in it

Do not answer "all green", or anything that reads like it, while `TASK.md`
carries an unfinished item. Report the checks as a list of exit codes, then
name what is still open.

Green means every check this repository has passes, but defects come from the
surfaces with no check. A green run is evidence about the checks and says
almost nothing about the material, and reporting it as a verdict on the
repository misleads the reader.

`TASK.md` is where the open items are read from, together with
`asgard-cli audit-material --unchecked`, which lists what every document says
it has not been held against. If either has an entry, the answer is "the checks
pass and these are open", never "green".

**Mechanical, and fails the build.** The result is an exit code, not a
judgement. What each check covers is in its own description, printed by
`go run ./hack list` and by `asgard-cli audit-material --help`. A description
kept here would be a second copy that drifts from the code.

**Reported, and deliberately not enforced.** Each needs a person to read it,
and a green build says nothing about them:

| surface | why it cannot fail |
|---|---|
| `--orphans` | a document reached only by search is still reached |
| `--crossref` | a sentence describing another command reads correctly alone; only opening that command settles it |
| `audit-material` with no flag, `--ask`, `--unmarked` | every bold imperative on one screen, because the failure is two opposing ones never being in front of the same reader. `--ask` narrows to the ones telling a reader to ask a customer, which is where filter 0 applies |
| `--unchecked` | what every document says it has NOT been held against. This is the opposite question to `--unverified`: a page whose marker names a whole surface passes that check, and this puts the surface in front of a reader. Every document is expected to have one, so it cannot fail. `TASK.md` reads its blocked list from it instead of keeping one |
| `--term <field>` | the sweep for a renamed platform field, across prose and templates. It cannot fail on its own: it only answers a question somebody asks it |
| `go run ./hack sources` | how far each upstream clone is behind its remote. It is information rather than a verdict |
| `go run ./hack related` | which documents point at the ones a change touched. `--orphans` asks whether anything points at a document; this asks what does, using the same graph. It lists what a change owes a re-read, instead of the whole corpus. It cannot fail, because a pointer is a question rather than a defect |
| `go run ./hack introduced` | the count-shaped lines this change adds. Over the whole corpus the same detector reports more than a thousand lines, which would make it a rule to delete; over a diff it reports a few dozen. Existing counts are a backlog worked through by reading, and it shrinks: a document reviewed at a recorded digest does not come back until it changes. This check keeps new counts from refilling it |

**Checked by nothing, and verified by reading.** Some surfaces no program can
check: whether a sentence is true of the platform, whether a pointer leads to
the right page rather than to some page, whether a rule still matches the
upstream it came from. These are verified by reading.

Nothing records that a reading happened. A verdict written into a document -
`ok` beside a script, `read` beside a page - cannot be checked by any program,
so do not write one. What a reading finds goes where it is fixed: a correction
on the document it corrects, a rule in this file, and the history in
`git log`.

## Before you say it is done

The gate above is mechanical and does not catch the defects below. Ask these
questions before calling a change done.

`.github/pull_request_template.md` asks for the answers to three of them,
because a PR body is written when somebody believes the work is finished, and
that is where they are hardest to skip. The rest are here because they are
cheaper to ask while the work is still in progress.

`gh pr create --body` replaces that template rather than filling it in. The
template only prefills the web form, so a PR opened from the command line -
which is how PRs are opened here - skips it without warning. Structure the body
you pass around the template's sections, or read it first with
`cat .github/pull_request_template.md`.

**Did you run it, or only read it?**
Auditing material is not the same as using the tool. A command can leave the
config and the disk disagreeing so that `check` passes and `render` fails, and
no review of the material shows it. Run `init`, `project add`, `add`, `check`
and `render` in sequence, in a throwaway directory, for anything that touches a
command.

**Does it still work with no repository?**
Half the job is answering a question, and that question gets asked in a meeting,
before the engagement has a directory. `asgard-cli init` in an empty directory
writes the whole corpus, and `asgard-cli size` and `asgard-cli guide` answer
outside a repository too. Reference material that requires an engagement is
unavailable when somebody is deciding whether to have one. None of those needs
a repository, a session or a network. A new command that resolves a profile, or
reads the checkout's binding, before it can say anything no longer works
without a repository - `actingLocally` in `internal/cli/context.go` is the
line where that starts. Run it in an empty directory.

**Which kind of artefact is this recipe for, and what inverts for the others?**
A rule that produces a good artefact of one kind can produce a bad one of
another, and a checker that validates shape cannot tell them apart. A proposal
deck's rules applied to a discovery deck invert: titles as assertions state
conclusions before the questions that would support them, and a three-to-five
item bound compresses a customer's document into something only its author can
read. Say which kind a recipe is for.

**What else claimed the thing you just changed?**
A trap lives in a template, an extract, a stage prompt and a wiki page, and only
one of them is in your diff. Changing `allowWrite` in a template while an extract
still teaches the old value leaves the repository disagreeing with itself.
A term sweep - `asgard-cli audit-material <term>` - finds the other copies.

**Did you break the thing that was enforcing it?**
Code can depend on the shape of a document - `work.go` appends status
transitions under a request-template heading found by its literal number, so
renumbering the sections breaks it without any error. If something was keeping a rule, changing the shape it relies on is
part of the same change.

**If it is a list, does it say which of its items are load-bearing?**
A reference that lists things uniformly invites use of all of them. Prose
says what a thing is for and a schema says what shape it takes; only the
worked example shows how often a thing is right, and nobody reads an example
for frequency. An agent given a flat list of classes uses all of them, because
a list of thirteen suggests thirteen things worth using.

There are two fixes, and both cost less than the defect:

    split the list           a working set and a specialised one, so the
                             specialised ones are opted into rather than
                             defaulted to
    put the rule where it    on the thing somebody is looking at while
    is used                  writing the line

Do not fix it by adding a copy at the point of use; that adds another home for
the fact. Move it, or point at it.

**Does anything point at what you added?**
Material nothing links to is not read, and the writer does not find out, because
the file exists. A document with no `group:` is in no index, which
`go run ./hack index` reports as its own failure rather than leaving to a
reader. Writing something and pointing at it are separate steps, and it is easy
to stop after the first. `audit-material --orphans` is the mechanical half;
grep for the name of what you added is the other.

**Is the claim verified, or asserted?**
A countable claim stated in prose is an assertion until it is counted. If a
claim is countable, count it before writing it, and put the number where the
next person can recount it - `internal/corpus/wiki/index.md` carries that one
and `go run ./hack coverage` recomputes it.

**Would this be recognisable to the customer it came from?**
Nothing in this repository may name or portray a customer - not in a `--help`
example, not in an illustration modelled on one engagement's actual systems,
and not in a provenance marker. That covers a customer's repository name, its
GitHub organisation, a namespace or workspace name carrying either, and the
customer's own name. Describe the deployment by its shape instead - "a
production deployment", "a commerce back-office deployment" - and cite the
platform's own source for the claim where one exists. Everything under
`internal/` also lands in other customers' repositories, so a name there
reaches them.

**Is "done" as wide as what you checked?**
Say what was verified and what was not. "The extracts are correct" and "the
extracts' YAML skeletons validate against the CRD" are different claims, and
reporting the first when you did the second is how a review passes something
broken.

**Do not ask whether to commit until you have finished checking.**
Asking moves the checking onto the person answering, and they answer assuming
you already did it. Do not end a pass with that question before the following
are done.

What that means in practice, after the last edit and before the question:

    every check, again, from the top   not the ones you were watching
    what else claimed the thing        `go run ./hack related` for the
                                       documents, `audit-material --term` for
                                       a word - let their scope be the scope
    what YOU just introduced           `go run ./hack introduced`, then the id
                                       you added, the pointer you moved, the
                                       wording a check matches on

Let the tool decide the scope of a sweep. Choosing by hand which files a sweep
covers is the same mistake as writing a list that can be generated, and it
fails the same way: you remember the files you have been editing, and the other
copy is somewhere else. `--term` reads every surface this
repository is responsible for - the material, the scaffold templates, every
`--help` screen, the CLI's own string literals, these documents, the maintenance
skills and the gate under `hack/` - because `everySurface` assembles it once.
Separate hand-written assemblies cover different sets, and a sweep over the
narrowest reports clean over surfaces it never read.

The third item is the one that gets skipped, because nobody listed it as a
surface: it is a surface you created in the last ten minutes. A new id that
duplicates an existing one fails no check, because no check knows to hold a new
identifier against anything.

Asking about the work does not replace finishing it, and "the gate is green"
does not mean "I checked what I changed".

**Never annotate a source with a count.**
A provenance marker records what was read and the commit it was read at, and
nothing else. "Checked against 72 gated and 14 ungated tool entries" and
"checked against 11 SemanticLayer CRs across three deployments" do not say
that anybody read those CRs. They say that something was counted, and they
read the same as a statement that something was read. That is the harm, and it
is worse when the number is right, because a correct count stops the next
person from opening the page: the check is green, the figure recomputes, and
everyone who has passed the sentence has believed it.

So a marker names the scope - which deployment, which shape, which commit - and
the claim carries no tally:

    no    checked against 11 SemanticLayer CRs across three deployments
    yes   checked against every SemanticLayer CR in three deployments,
          and completionModelName is present on all of them

Counting alone cannot satisfy the second form, which is why it is the one to
use. When you delete a count, delete its checker too: `go run ./hack counts`
reports a row whose claim has gone rather than passing silently, so the two
stay in step.

**Does the number need to be there at all?**
Ask that before asking whether it can be computed. A count belongs when the
count is the claim - how much of a chart `add` never writes, how many
pages a back office has. It does not belong when it stands in for a yes: name
the deployed example that answers the question instead. Do not write a checker
for a number like that; remove the number. An index carries no counts.
`.agents/skills/consistency-checks/SKILL.md` has the three cases.

**Did you say how the number was measured?**
A coverage figure goes wrong through its denominator: a URL matched against a
file path, or a count divided by every file rather than by the ones in scope.
Nobody can check a percentage
with no method beside it, and people repeat it anyway. Write the
denominator and how you got it, or write the raw counts and no percentage - and
prefer a check that recomputes it to either.

**Would this check fire on material that is correct?**
A static check over prose cannot tell an instruction from a mention. Know that
limit before writing one. Validating an invocation's argument count against
cobra's own `Args` looks exact and is not: a quoted argument is four words, a
line continuation is a token, `(asgard-cli render)` in a parenthetical takes
the next three words, and an example block aligns a trailing description with a
single space - and one space is significant here by rule. A check that fires on
correct material is worse than none, because people respond by turning it off.
Prefer a check that runs where the mistake is made: `render` rejects a
project-and-environment with its own message at the moment somebody types it.
If every finding a check produces on real material is correct material, the
check should not be written. A checker that raises false alarms teaches people
to change what it can see rather than what is wrong. Delete such a check; do
not soften it.

**Does the report name what it counts, in every category, always?**
A number with no list is not actionable, and a category that prints only when
another one is empty is worse than one that never prints: the count keeps
saying something is owed while the report stops saying what. Print the strongest category first and cap it rather than suppressing it:
a capped list with "and N more" under it can be worked through, whereas a bare
number can only be believed. This is the same problem as a skip that looks like
a pass: an answer that is right but unusable looks the same as one that is
right and usable.

**If it is a pinned copy of the platform's contract, which way can it go stale?**
Both ways, and the more costly direction is the less expected one. A
constraint that loosens upstream leaves a pin that reports correct charts as
wrong, which costs more than a pin that has stopped catching something: the
platform deletes constraints as readily as it adds them, and a regex it
decides was wrong becomes a warning against something it now accepts.
`go run ./hack tables` catches it, and it takes the asgard-kube checkout as its
only argument so that running it is one command. `gate` holds three: the
processor definitions, the CRD enums, the CRD patterns. A copy can only be
wrong by being behind, so a rule built on one is a warning: the platform adds a
value and a correct chart looks wrong. The exception is a condition broken
against every version, like a required key with no default, and that one may
fail. Say which you are writing before you write it.

## Four ways material contradicts itself

None is enforced by anything, which is why they are written down.

**1. Opposite instructions in two pages.** The reader follows the page in front
of them, so two pages that disagree produce whichever answer that page gives.

    anything that tells a reader to ask a customer something
    has to pass filter 0 first: does the answer change what we build?

**2. One word, two meanings.** A word used in two senses close together
contradicts nothing, and the reader takes the wrong sense.

    internal/corpus/wiki/glossary.md lists the terms that already mean
    something specific. Check it before introducing a word, and before using
    one of those for something else.

**3. An instruction right for one reader, copied by another.** This one does
the most damage as the material grows. An instruction to get somebody's name is
correct for tracking and wrong on a customer slide, and a reader copying it
does not know which it was for.

    a correction only in the canonical place does not work.
    somebody copying reads one page, and it is not that one.

So when a line tells a reader to find out who somebody is, or to ask a customer
anything, put the destination inside the imperative, not beside it:

    no    who issues the credential          ...and a paragraph explaining
                                             that the name is for tracking
    yes   who issues the credential, for the tracking row

A caveat next to the imperative does not work: the reader copies only the
imperative. Readers scanning for "what do I do" pick out imperatives, and
prose beside one reads as elaboration rather than as a limit - especially when
the imperative is short and bold and the caveat is a paragraph.

When the destination is inside the imperative, it is copied with it. Removing it
then takes a deliberate decision, and that decision is the judgement that was
missing.

**4. Handing work to another skill without its constraints.** A page that
delegates to another skill and omits that skill's requirements leaves the
reader unaware they exist. This is an omission rather than a contradiction, and
it fails the same way: the reader follows the page and finds out only when it
goes wrong.

    when a page delegates, it carries the constraints of what it delegates to,
    or it says explicitly which of them do not apply here.

None of the four is enforced by anything, which separates them from the rules
above, each of which has a check.

A detector would not help, because all four read correctly line by line. What
helps is putting every instruction in the material on one screen, because the
cause is that two opposing instructions are never in front of the same reader.
`audit-material` with no flag is the closest thing: it lists every bold
imperative across every part, and `--ask` narrows to the ones that tell a
reader to ask a customer something, which is where filter 0 applies.

## Three questions the material has to keep answering

The questions above are asked of a change. These three are asked of the tool,
because they are what it is for. Reading does not settle any of them; run the
check.

**Does what it generates still satisfy the contract?**

This repo does not define CRDs; it consumes them. The claim to check is that
what the templates emit, and what the extracts show a reader, are both accepted
by the CRDs as they stand today. The CRDs change without notice.

Pull first. Validating against an old clone proves nothing, and asgard-kube changes without announcement. Record the commit, and whether it
was head when you looked; "checked against the CRD" without a commit is not an
answer.

Render every kind and validate the result against the schemas: required fields,
fields that are not in the schema, enums, patterns, and the `ExactlyOneOf` rules.
Do the same for the YAML skeletons in the extracts, because those are what
somebody copies by hand. `go run ./hack extract-crs` pulls them out and
`go run ./hack validate-crs` checks them; do not do this check by eye, and
`hack/README.md` is the procedure.

If the contract has moved since this repo last looked, say what changed and what
it means here. A retired field, a flipped default and a new required field each
land differently - the first breaks a template silently, the second changes
behaviour with no diff at all, and only the third fails visibly.

```bash
asgard-cli render <release> | \
  <validate each document against asgard-kube/crd/*.yaml openAPIV3Schema>
```

Neither `helm lint` nor `asgard-cli check` nor a server-side dry-run does this,
so a missing required field or a field of the wrong type passes all three. A
dry-run is misleading here: it silently drops a field it does not
recognise and reports success, while helm's own server-side apply refuses.

**Can it get an FDE to the right integration by asking?**

Every entry point the platform offers has to be reachable through a question the
interview actually asks. `botProviderClass` is immutable once applied, so a
channel decided by assumption means a new BotProvider rather than an edit.

```bash
# the classes the platform has
grep -A3 'botProviderClass' ~/asgard-kube/crd/asgard-ai.com_botproviders.yaml | grep enum

# whether the interview asks about each route
grep -in 'chat\|channel\|LINE\|other end\|API\|console' \
    internal/stage/prompts/11-requirements.md
```

Who is on the other end (hub against flow agent) and which channel are separate
questions, and the interview has to ask both.

**Does an agent have enough to assemble a chart?**

Every CR kind a chart may need has to have a generator, an extract, or a written
statement that it is not for an engagement to reach for. A kind with a CRD and
nothing else leaves the reader with a schema and no judgement.

```bash
# every kind the platform defines
ls ~/asgard-kube/crd/*.yaml | sed 's/.*com_//;s/s.yaml//'

# what any of the material mentions
cat internal/corpus/usecase/*.md internal/generate/templates/*.tmpl \
    internal/corpus/wiki/*.md | grep -o 'kind: [A-Z][A-Za-z]*' | sort -u
```

That comparison leaves three, and all three are deliberately absent rather than
gaps: `ImageGenerationModel`, `TranscriptionModel` and
`SourceSetEditorServer` are internal. A kind with a CRD
and no material is what this comparison exists to find. Ask before writing the page, because the two
mistakes do not cost the same: material for a kind nobody may use invites
somebody to use it.

## Reference material lives outside this repo

The reference repositories are read-only and never vendored in. The URL is the
source of truth, not where you cloned it; a local path is true on one machine
and wrong on every other:

**The contract:**

| what | source of truth |
|---|---|
| CRD definitions, the platform contract | https://github.com/asgard-ai-platform/asgard-kube |
| product documentation | https://github.com/asgard-ai-platform/asgard-docs |
| the processor definitions the CRD is generated from | https://github.com/asgard-ai-platform/asgard-core (private) |

`asgard-kube` is read at two depths and they answer differently: `crd/` is the
contract, and `pkg/apis/` is the Go types it is generated from, where the
reasoning survives as comments. `internal/corpus/wiki/crd-rules.md` was written
from the second.

`internal/gate/processors.go` holds a pinned copy of asgard-core's
`ProcessorDefinitions`, so the rule above about which way a pinned copy can go
stale applies to it. That list is also a subset of what a chart may set, not
the config contract, because a processor declaring dynamic config takes keys no
definition names. `internal/corpus/wiki/processors.md` is where that is
recorded, and "would this check fire on material that is correct" above has the
cost of treating it as closed.

The extracts under `internal/corpus/usecase/` were written from production
deployments read before this tool generated charts. Those deployments are not
tracked here and are not a source to re-read: most now generate their charts
with this tool, so reading them again would check the tool against its own
output. A claim about how a shape is built is held against the contract above,
and what is learned in the field is written with the platform source that
explains it, describing the deployment by its shape and never by name.

Clone the three wherever you like and point an environment variable at each:
`ASGARD_KUBE`, `ASGARD_DOCS` and `ASGARD_CORE`. `go run ./hack sources` prints
what they resolve to and how far behind each one is.

Pull before relying on any of them, and record the commit you read in whatever
you write; a copy taken into this repo stops tracking upstream and still looks
current. `asgard-cli audit-material --sources` holds every recorded commit
against every other, but it cannot tell you a recorded commit has gone stale.
`go run ./hack sources` reports how far each clone is behind its remote.

## Put generated files in `.out/`

Anything a command produces that does not belong in version control goes to
`.out/` at the repo root - not scattered, and not in `/tmp`:

- throwaway debug scripts and scratch programs
- command output, logs, sample data downloaded for inspection
- binaries built by hand
- profiling reports (`*.pprof`)

```bash
mkdir -p .out
go build -o .out/asgard-cli ./cmd/asgard-cli   # or: make build
```

`.out/` is gitignored and can be deleted at any time: `rm -rf .out`.
