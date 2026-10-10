---
name: consistency-checks
description: How to check this repository's material for consistency, and what consistency checking cannot answer. Running most-volatile first because upstream moves without anyone here touching it, what each check is blind to, the method for holding prose against the upstream it came from, and the two rules that end the loop - compute a number rather than copy one, and write no verdict a program could have produced. Use before saying anything here is correct, after changing any document, after pulling an upstream clone, and when a reading of this material turned out to be wrong.
---

# Checking this material

This is for asgard-fde-cli itself, not for a customer repository. A customer's
agent runs `asgard-cli gate`.

The inventory of what each check covers is `AGENTS.md` under "What is checked,
and what is not". This file is the method: the order to run them in, what each
one is blind to, and how to do the part no check does.

## Three kinds of check

Almost every check here is a consistency check. It asks whether two things
inside this repository agree - a pointer against the file it names, a command
against the command tree, a number against a measurement. Four need a clone and
compare against upstream: `go run ./hack tables`, `go run ./hack validate-crs`,
`go run ./hack coverage` and `go run ./hack sources`.

No check tests whether a sentence is true. A page can be internally consistent,
point at real files, name real commands, and still describe the platform
wrongly. Running more checks does not close that gap.

Consistency checks also do not test capability. Every check above can pass while
the tool has stopped doing what `Goal.md` says it is for: if `asgard-cli init`
required a session, the corpus would still be consistent and every pointer would
still resolve, but Goal's first point would be gone. `go run ./hack goal` is the
one check that runs the tool the way Goal describes - in a temporary directory
with no network, no account and no git repository - and the only one that
writes files.

So a claim about this repository says which of the three it rests on: a
consistency check, a capability check, or somebody's reading.

## Run the gate through `verified`

Run `go run ./hack verified` rather than the pass by hand. It runs the checks
whose inputs have moved and skips the ones whose inputs have not, recording
what passed against which inputs under `.out/`. This avoids re-checking
unchanged material in one sitting.

A skip is not a pass, and it is printed differently. It says this check
answered these exact inputs before, on this machine. It says nothing about the
surfaces no check reaches.

`--all` ignores the record and `--forget` deletes it. Use either when you have
changed something a check reads that its declared classes do not name: `reads`
in `hack/verified.go` is hand-written and deliberately coarse, and a class it
gets wrong can make it report a pass for an answer that moved underneath it.

Write no verdict for a mechanical check. `ok` beside `--links` copies a result
a script can produce into prose nothing can verify. Each check reports through its exit code
when run. The surfaces no program can answer are verified by reading, and
nothing records that the reading happened.

`go run ./hack pass-list` holds what no program can derive about the pass
itself: that every check says what it needs, and that this repository's own
maintenance skill never appears in a scaffolded tree. It compares no names,
because the pass is printed rather than copied.

## Then run them, most-volatile first

    go run ./hack pass

That prints the pass in the order to run it, derived from the binary's own
flags and the gate's own subcommands, so it is not written down anywhere,
including here.

Order by where the answer is most likely to have changed, not by cost:

  - Upstream first. Nobody here touches asgard-docs or asgard-kube, and they
    move without anyone noticing. Pull before running them, or they check a
    clone rather than the platform.
  - Then the material. Build the binary from the working tree first, because
    every audit reads what is embedded, so an older binary checks an older
    corpus.
  - Then the compiler.
  - The network last, because it is the only one that needs it, and a third
    party's outage is not this repository's failure.

Then the prose, which no check does; the section below describes how. Leave it
last, because it depends on the clones being current, and step 1 tells you
whether they are.

## What each one is blind to

`go run ./hack list` prints the gate's names with what each is for; this table
gives what each one cannot catch, which the tool does not print.

| check | what it will not catch |
|---|---|
| `--links` | a pointer that resolves to the wrong page. It checks that the target exists, never that it is the right one |
| `--bare` | a reference with no hyphen in it. `agents` is a CR field, `verify` is a command, and telling those from a page name is not possible in prose |
| `--commands` | a command written in prose without backticks, and a wrong argument - an argument check over prose fires on correct lines |
| `--paths` | a path relative to the skill that writes it, which is correct and looks wrong from the root |
| `--unverified` | whether the marker is true. It reports the marker's presence, and for `needs` and `brief` the marker is one shared constant |
| `--sources` | whether a recorded commit is current. Nothing inside this repository can know that; `go run ./hack sources` reads the clones |
| `tables` | a constraint the CRD expresses in CEL rather than in the schema |
| `validate-crs` | whether the CR does what the page says it does |
| `coverage` | whether the pages behind the numbers say anything true. It counts them. `--drift` names the pages that have moved and never says what changed in one |
| `processors` | what a key means. The definitions say whether a processor takes dynamic config; that an extra key on `http-request` is an HTTP header is in the loop that reads it, and no table upstream states it |
| `counts` | a count of something nobody upstream counts. It recomputes the screenshot arithmetic off the asgard-docs tree, and a number invented here has nothing to be held against |
| `pass-list` | whether any check passed. It holds that every check says what it needs and that the maintenance skill does not leak into a scaffolded tree, and a verdict is not in its reach |
| `goal` | whether the material is any good. It asks whether the capability is there - the corpus lands, a grep finds things, a chart gets written, the issue route is printed - never whether what landed is right |
| `sources` | whether a page is still true. It reports how far each clone is behind its remote - never what changed or whether it matters |
| `doc-paths` | whether a document's prose is right. It resolves the paths, the Go symbols and whether every command in the tree is named in both READMEs, and says nothing about what the sentence around one claims |
| `--unchecked` | nothing - it does not fail. It prints what every document says it has not been held against, which is where the blocked list comes from |
| `--orphans`, `--crossref`, `--ask`, `--unmarked`, no flag | nothing - they do not fail. They are listings for a person |

## The one that is a query rather than a check

    asgard-cli audit-material --term <field>

Run it whenever you change anything a second place might restate - a renamed
platform field, a reworded rule, a check whose behaviour moved. It has no pass
or fail; it answers a question somebody asks it. So it is here and not in the
pass, which lists checks that can be run without being told what to look for.

Use its scope as the sweep's scope. It reads every surface this repository is
responsible for: the material, the scaffold templates, every `--help` screen,
the CLI's own string literals, these documents, the maintenance skills and the
gate under `hack/` - whose `What:` strings are what `go run ./hack list` prints.
A claim is often taught in several of those at once - a template that writes a
field, an extract that explains it, a prompt that mentions it, a help screen
that names it - and fixing one leaves the rest teaching what is no longer true.

Do not choose the set of files to sweep by hand. The files you remember are the
ones you have been editing, which is usually not where the other copy is, and a
sweep limited to named files reports clean over files it never opened.

For the re-read, use a worklist rather than the whole corpus.
`go run ./hack related` names the documents that point at the ones a change
touched.

Do not maintain a list of count patterns either. `go run ./hack introduced`
reads the lines this change adds and lists every count-shaped one. Over the
whole corpus that detector reports more than a thousand lines; over a diff it
reports a few dozen, which is a reviewable number. Existing counts are a
backlog to work through by reading: the corpus is a countable number of
documents, and one reviewed at a recorded digest does not come back until it
changes. Re-reviewing from zero each time makes the backlog look endless and
makes each pass expensive. `introduced` catches the counts a change adds, so
the backlog does not refill.

## Holding prose against its source

This is the part no check does. It scales with the amount of prose, so work by
claim rather than by document.

0. Run `asgard-cli audit-material --unchecked` first, because it gives the
   scope. Every document names the surface it has not been held against, and
   that output tells you what a pass is for before you spend it.
1. Run `go run ./hack sources` next. A reading held against a stale clone proves
   nothing. `git -C <path> pull` before reading.
2. Let `--drift` set the scope rather than the diff.
   `go run ./hack coverage --drift` lists every cited page that has moved since
   the commit the citing document names. That reduces "the wiki against
   asgard-docs" to the handful of pages that moved, which is a reading somebody
   can actually do. Reading the whole diff is not practical, and claiming to
   have done it would move every citation on a reading nobody did.
3. Move the commit only for what you read. A page's provenance is per claim, so
   one page can cite two commits of one upstream. Mark a commit you have not
   read with ` (unread)` so `--sources` lets it stand.
4. Take the strongest source available. For a CR field the CRD beats the
   documentation; for how a field behaves in practice a chart beats both. For
   example, the docs give the `effort` field's levels, and a chart's own values
   file shows that omitting it is not disabling it, which affects cost.

## Claim shapes that go stale

A number copied from somewhere that moves goes stale. A judgement does not.

    rots      a count of anything upstream: CRs, Plugins, pages, nodes
    rots      a field name, an enum value, a file path somebody else owns
    rots      a distance between two things that both move
    holds     a rule, a trade-off, a failure mode, what a word means here
    holds     a count with a commit beside it

A count with no commit beside it cannot be rechecked later. "29 Plugins" cannot
be checked; "29 Plugins at `edb0ad0`" can, because the commit does not move. A
distance cannot be fixed the same way - "26 commits behind" compares two moving
things and is wrong as soon as either moves. Compute a distance; never write
one.

Be most careful with a count copied out of a document that states its own. A
wrong number reads no differently from the right one. If the source counts
itself, read its count; if it does not, compute yours and leave the method
beside it.

Take the same care with two numbers that describe the same thing. The
`XValidation` markers in asgard-kube's Go types and the rules the generator
emits from them are different counts; do not write one as the other. A live
URL against a file path is the same trap: a page served at a name that is not
its file's makes a comparison of the two miscount.

A count on a provenance line is not evidence of a reading. `**Checked:**`
records what was read and the commit; a tally beside it - "against 72 gated and
14 ungated tool entries" - says only that something was counted, but reads like
a statement that somebody looked. A correct tally makes the check pass while
the page stays unread. Name the scope instead - every CR of that kind, in these
deployments, at this commit - because a scope cannot be satisfied by counting.

Before asking whether a count can be computed, ask what the reader does with
it. There are three cases, and only one of them needs a number:

    the count IS the claim        how much of a chart `add` never writes, how
                                  many pages a back office has, how far a clone
                                  is behind. Keep it, and compute it.
    the count is evidence         "does an arrow function evaluate" - the
                                  answer is yes or no, and a tally is a weak
                                  way to say yes. Name the deployed example
                                  instead: it cannot rot, and it is checkable
                                  by opening one file.
    the count is decoration       "the 13 processors" in an index row. The row
                                  reads the same without it and cannot go
                                  stale. Delete it.

The tally that stands in for a yes goes wrong most often, because each recount
picks its own denominator. A deployed expression such as
`prevBlobs.map(b => b.blobId)` in a shipped chart answers the same question and
does not rot. Do not write a checker for a number like that; remove the number.

An index carries no arithmetic. An index says which page answers a question.
The one number that stays in `internal/corpus/wiki/index.md` is the row
`go run ./hack coverage` recomputes and fails on.

Before writing a count, decide whether it can be computed instead. A number a
script can derive does not belong in prose.

Before restating a field rule, point at it. `internal/corpus/wiki/README.md`
says chart-writing cautions belong to the extracts and field rules to the CRD.
A restatement of something another repository owns goes stale when that
repository changes.

The same applies to a check's own result. Do not write `ok` beside `--links`:
a script produces that result, so prose must not copy it. A document may record
a check's name, and in its `**Checked:**` line, what it was held against.
Anything else is a claim nothing can check.

## When a reading turns out to be wrong

Fix the claim, then ask whether a check would have caught it.

- If yes, and there is no check, write it.
- If no, say so where the claim lives, with an `**Unchecked:**` line that names
  what nobody has held it against.
- If a check would fire on correct material, do not write it. `AGENTS.md` asks
  that under "Would this check fire on material that is correct?".

**Checked:** every command and script named here is in this repository and
does what is said - `--commands` and `go run ./hack doc-paths` resolve them,
and this file is in the set both read. The ordering claim is checkable too:
`go run ./hack sources` reports how far each clone is behind, which is the measure
of what moves without anyone here touching it.

**Unchecked:** whether most-volatile-first is the best order. That upstream
moves most is measured - `go run ./hack sources` reports it - but that running it
first finds more, sooner, is a design rather than a measurement. Cheapest-first
is the wrong principle: it optimises for the time of whoever runs the pass.
