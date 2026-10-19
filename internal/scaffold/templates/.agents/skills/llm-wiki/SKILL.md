---
name: llm-wiki
description: How to maintain this repo as an LLM wiki rather than a pile of files - which directories are the raw sources, the wiki and the schema, folding a new customer document or meeting into every page it changes, filing an answer you worked out so the next reader gets it for free, and auditing for the rot no linter can see (pages that contradict each other, claims a newer source superseded, concepts nobody owns). Use when a document, a meeting or a reading off a running system arrives, when you answered a question by reading a handful of files, when picking up a repo somebody else worked on, before a handover, after a burst of decisions, or when an answer you gave from the docs turned out to be wrong.
---

# This repo is an LLM wiki

Most of what is in here is not code. It is what an engagement learned about one
customer's systems, and the repository is the only place it exists - the next
agent to open it starts from these files and nothing else.

The repository follows the LLM wiki pattern. A source is read once, and what it
says is written into the pages it affects. The next reader reads those pages
instead of working the answer out from the sources again. That holds only while
the pages are current, and keeping them current is part of every change you make
here.

These documents describe the pattern. Read them for the reasoning; this skill
says how it applies to this repository.

  - Andrej Karpathy, "LLM Wiki", gist revision `ac46de1`:
    https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f - the
    layers, and the ingest, query and lint operations.
  - Google Cloud, Open Knowledge Format v0.2, `SPEC.md` in
    GoogleCloudPlatform/open-knowledge-format at `ad30107`:
    https://github.com/GoogleCloudPlatform/open-knowledge-format/blob/main/SPEC.md
    - a file format for the same pattern, with provenance, trust and lifecycle
    fields in YAML frontmatter.
  - Tang et al., Google Research, "WikiSkill: Compiling Agent Experience into
    Persistent Knowledge for Skill Evolution", arXiv 2608.27454:
    https://arxiv.org/abs/2608.27454 - the same three layers, used to turn an
    agent's own run history into skills.

How facts move between the directories is in
[`docs/README.md`](../../../docs/README.md); the loop that lands a decision is
in the `spec-workflow` skill. Do not restate either here.

## The layers, and where each one lives

| layer | who changes it | in this repository |
|---|---|---|
| raw sources | nobody: they are added and never edited | `references/`, `docs/meeting-notes/`, `docs/decisions/` |
| the wiki | you, in every change | `docs/spec/<slug>/`, `docs/open-questions.md`, `assets/skills/` |
| the schema | a person, deliberately | `AGENTS.md`, `docs/README.md`, `requirements/README.md`, `references/README.md` |

`requirements/` is in none of the three. It is a work queue: a task spec is read
until it is `done`, and anything durable in it has to move into the wiki before
then.

The pattern also has an index and a log.

  - The index is each directory's own: `docs/spec/<slug>/README.md` for the
    living spec, `requirements/_index.md`, and `references/_index.md`, which
    `asgard-cli reference add` writes. `asgard-cli check` fails when the spec's
    module index does not match the files on disk.
  - The log is git history and the dated decision records. Do not add a
    `log.md`. The living spec states only the present, and a log beside it
    becomes a second record of the same changes.

## Who does what

The bookkeeping is yours: updating the index, fixing the cross-reference,
noticing that three other pages now say something slightly wrong. You can touch
many files in one pass. The person you are working with curates, directs, and
questions - which page matters, whether a claim is true, what the customer
actually meant. When you land a change, land the bookkeeping with it in the same
pass, instead of reporting that an index needs updating.

The exception is judgement that belongs to a person: which of two contradicting
pages is right, whether a stale claim is worth re-verifying, whether a decision
should be reversed. Bring those back rather than resolving them quietly.

## Ingest: a new source arrives

A source is a document the customer hands over, a meeting, or something you
read off a running system. Fold it in this way:

1. File the raw source first. A document goes in through
   `asgard-cli reference add`, which records who supplied it and when. A
   meeting goes in `docs/meeting-notes/` under a dated filename. Do not edit a
   raw source afterwards, to correct it or to bring it up to date.
   Nothing under `references/` reaches the deployed agent. When the agent is
   meant to answer from the material, suggest a knowledge Drive to the person
   you are working with, and when they do not know how to build one, the
   written wiki with no index first: "When unsure: a written wiki, with no
   index" in `../asgard-platform/usecase/knowledge-drive.md`.
2. Read it once and list every wiki page it changes: a spec module, an open
   question it answers or raises, a runtime skill that says something it
   contradicts. One source usually changes several pages. Updating only the
   page you were looking at leaves the others saying the old thing.
3. Edit the affected passages. Do not rewrite a whole page to change part of
   it: a rewrite drops sentences nobody meant to remove.
4. Write provenance into each claim, in the form `requirements/README.md`
   gives: how it was established and when. A claim taken from a customer
   document says "not verified" until somebody checks it against the running
   system.
5. When the source contradicts a decision record, leave the record as it is.
   Put the conflict in `docs/open-questions.md`, or write a new dated decision
   if somebody has settled it.
6. Say which pages you changed and which existing claims the source
   contradicts.

## Query: file what you worked out

When you answer a question by reading several pages, the answer is new
knowledge. If it stays in the chat, the next person asks the same question and
reads the same files again. This is the most common way effort is wasted here.

So after answering a question that took real assembly, ask: would the next
reader have to redo this? If yes, file it, in whichever layer owns it:

  - it describes how the system behaves -> a living spec module, or a
    paragraph added to one
  - it explains why something is the way it is and you had to reconstruct
    that from several sources -> a decision record only if a decision was
    actually taken; otherwise it belongs in the spec module as context
  - it is how to work in this repo -> `AGENTS.md`
  - the running agent needs it -> `assets/skills/<skill>/SKILL.md`, because
    nothing else reaches the cluster
  - you could not settle it -> `docs/open-questions.md`, with what it blocks and
    who can answer

Not every answer earns a page. A one-line lookup does not; a rule you had to
infer by comparing three files does. Decide by how much assembly it took, not by
length.

## The wiki is for whoever writes; the running agent reads skills

The deployed agent reads only what a SkillSet's `searchPaths` selects, which is
under `assets/skills/`. It does not read `docs/`. So when the running agent
needs something the wiki knows, write it into a runtime skill in the form the
agent can act on. Do not point the skill at the wiki.

WikiSkill measured the same split. A wiki given to the agent that wrote skills
made the skills better. Giving the same wiki to the agent that answered the
tasks made its answers worse.

## Lint: the audit

`asgard-cli check` finds the structural rot: a page with no inbound link, a
module missing from its index, a filename that is not dated, a link that does
not resolve. Run it first, and fix its findings before reading for
contradictions.

    asgard-cli check

What is left needs somebody to read the pages against each other. Neither
design document above solves it. OKF says it standardises the format and
nothing else, and WikiSkill's own limitations say it has no way to prune its
wiki. So a reader finds stale pages by holding them against their sources, which
is why every claim records its source and date. Read for the following.

### 1. Contradictions

Two pages that cannot both be true. Read for these in this order, cheapest
first:

1. A decision record against the living spec module it changed. A decision
   is applied by rewriting the module; if the module still describes the old
   behaviour, the decision was recorded and never landed. This is the most
   common one.
2. A `done` task spec against the living spec. A task that changed behaviour
   is not done until its delta reached the spec, but the status was moved by
   hand and the delta was not.
3. Two decision records on the same topic. Both are immutable and both stay,
   which is fine as long as one says which one won. If neither does, write a
   third that supersedes, instead of editing either.
4. `AGENTS.md` against the living spec. These answer different questions -
   how to change the repo, versus what the system does - so overlap between them
   means a fact has two homes, and the two copies will drift.
5. A `assets/skills/` skill against the living spec. The most expensive kind,
   because the skill is what the running agent believes. A stale skill changes
   production behaviour.

For each one, report which side you believe and why. For example: "the decision
is dated later and the spec module was last touched before it, so the spec is
stale". Reporting only "these disagree" leaves the reader to redo the work.

### 2. Superseded claims

A statement that was true when written and has been overtaken. Unlike a
contradiction, nothing in the repo disagrees with it; what it describes changed
and the page did not.

  - A claim about the platform that a newer CRD or deployment contradicts.
    Platform vocabulary moves: a field is retired, a default flips, a resource
    is deprecated. Nothing on a page shows that a field it names no longer
    exists.
  - A claim about the customer's systems with no provenance, or with
    provenance old enough to doubt. `requirements/README.md` requires every
    claim about a real system to say how it was established and when; anything
    undated is a finding on its own.
  - An open question that a later decision answered, and that was never moved
    to Answered in `docs/open-questions.md`.
  - A `references/` document presented as current that the customer supplied
    long enough ago that nobody should rely on it unchecked.

Verify before reporting where verifying is cheap: a schema claim against the
database, a CR field against the platform's CRD. Where it is not cheap, say what
would settle it and leave it as an open question with an owner. Do not record a
guess as a finding.

### 3. Concepts nobody owns

These are the hardest to see, because nothing is wrong on any single page. A term appears
across five documents, everybody explains it in passing, and no page owns it.
The explanations drift, and a new reader assembles a definition from whichever
ones they happened to read.

Find them by noticing repetition: a term explained more than twice in different
files, or explained differently in two of them, needs a home. One fact, one
home, everywhere else links - the same rule `requirements/README.md` states.

When you propose a page, say which sentences in which files it replaces, and
remove them. A new page that replaces nothing adds another copy.

## Reporting

Write findings; do not fix contradictions silently. Which side is right is
frequently a question for a person - the FDE who was in the room, or the
customer.

Order by the cost of being wrong, not by how obvious it is:

    1. anything under assets/skills/     the running agent believes it today
    2. the living spec                   the next person will believe it
    3. decisions and requirements        found only when somebody goes looking

For each: the pages, which one you believe, the evidence, and the smallest fix.
Land the settled ones through the normal loop rather than by editing pages
directly - a new dated decision record, a delta applied to the living spec
module, a row moved to Answered. Do not edit a decision record to make it agree
with the current state; decision records are a history and stay as written.

End by re-running `asgard-cli check`. Applying a delta often leaves a new page
that nothing links to yet, and the check finds that.

## Open Knowledge Format is not this repository's format

The pages here carry no OKF frontmatter, and `asgard-cli check` does not read
it. The questions OKF puts into frontmatter are answered here as follows:

| OKF asks, in its field | answered here by |
|---|---|
| what the page was made from (sources) | the provenance on each claim, and the table in `references/_index.md` |
| how far to trust it (verified) | "verified" or "not verified" in that provenance. A reading off the running system is stronger than a customer document |
| whether it is the current version (status) | the layer: the living spec is current by rule, and a decision record is a dated snapshot |
| whether it is still true (stale_after) | nothing automatic. The audit above is how it is found |

If the customer asks for this knowledge as a portable bundle, OKF is a format to
export to. Do not change this repository's pages to match it for that reason.

**Checked:** the sources at the revisions named at the top: the gist at
`ac46de1`, OKF `SPEC.md` at `ad30107`, and the WikiSkill paper as published on
arXiv. The layer table and the index paths are held against
`docs/README.md`, `requirements/README.md` and `references/README.md` as
`asgard-cli init` writes them, and against what `asgard-cli check` enforces.

**Unchecked:** that the three kinds of rot named here are the ones that matter in
a customer's repository. They come from maintaining this tool's corpus, where a
fourth - upstream moved and the corpus did not - is recorded on the wiki's own
README; a reader maintaining a different corpus should expect their own fourth.
That the WikiSkill result carries over to a deployed Asgard agent: the paper
measured benchmark tasks, not a running agent with a SkillSet.
