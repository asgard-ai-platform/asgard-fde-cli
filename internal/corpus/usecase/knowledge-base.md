---
group: More entry points and more read paths
description: documents through a `KnowledgeBase` rather than a Drive - against [`knowledge-drive.md`](../usecase/knowledge-drive.md), which is the other half of the same decision
---
# KnowledgeBase, Loader and Source - the older knowledge path

Documents reaching an agent through a `KnowledgeBase` rather than a Drive: one
CR for the base, one `Source` per uploaded document, one `Loader` per recurring
pull.

**Seen in:** exactly one deployment, a content pipeline holding a research corpus
- three uploaded documents and a scheduled web crawl over encyclopaedia pages.
It is the platform's own deployment and the only one declaring any of these
kinds (`../wiki/coverage.md`), so it was written by the people who built the
CRs and reflects no customer's constraints.

**Checked:** against that chart - the four CRs, the `asgardBaseline`
class and its API key, `sourceClass: docx` with per-source indexer chunk sizes,
and a `web` Loader on a cron schedule - and against the CRD.

`asgard-cli add` writes none of these, deliberately. The four
CRs here are a shape to read rather than one to reach for -
`../wiki/knowledge.md` recommends a Drive for new work, so
a generator for `KnowledgeBase` would make the older shape the easy one, against
that recommendation. This page is for reading a chart that already has them; do
not write new ones.
`asgard-cli add knowledgedrive` is what to run instead.

**Unchecked:** the retrieval quality difference against a Drive. Only the same
documents indexed both ways in a running namespace can show it.

Read the platform side first: `../wiki/knowledge.md` -
Drive against Knowledge Base, and why the Drive is preferred for new work. This
page assumes you have.

## When this shape, and when not

Prefer a Drive. `../usecase/knowledge-drive.md` is the recommendation
and this is not a competing one: an engagement reversed from this path to a
SourceSet Drive with `contextIndex`, because the Loader-and-retrieval-workflow
route was harder to keep correct than a context index over files. That reversal
is one of the three this repository records.

KnowledgeBase is still live and still shipping, so this is experience rather
than a platform rule. Two situations where it still fits:

  - the corpus is documents rather than a repository. A Drive is a filesystem
    with a syncer over it; a KnowledgeBase takes one CR per document, uploaded,
    each with its own chunking
  - the source is the open web on a schedule. A `Loader` of class `web`
    takes a list of URLs and a cron, which is a smaller thing to declare than a
    crawler feeding a Drive

Neither is a strong reason. If you are choosing today, choose a Drive, and
if you take this one, write down why - the next reader will assume it was
inertia.

## The shape

    KnowledgeBase  kb-<name>          knowledgeBaseClass: asgard-baseline
      spec.asgardBaseline.aliasName   what the retrieval side refers to it by
      spec.asgardBaseline.apiKey      secretKeyRef into the release Secret

    Source  src-<name>-NNNN           one per document
      spec.knowledgeBaseName          binds it to the base
      spec.sourceClass                docx, and the per-class block below it
      spec.deletedIndexerKeys         REQUIRED, and `[]` is the ordinary value.
                                      The CRD refuses removing an indexer from
                                      the map directly - a key goes here instead
      spec.<class>.indexers.<key>     `chunkSize` per indexer
      metadata.labels
        asgard-ai.com/queryable       "true" to make it reachable

    Loader  ldr-<name>-<source>       one per recurring pull
      spec.knowledgeBaseName
      spec.loaderClass                web
      spec.schedule / timeZone        cron, and the timezone is not optional
      spec.web.urls                   the list, plus timeoutMs / waitForMs
      metadata.labels
        asgard-ai.com/loader-suspend  "true" stops it without deleting it

## Fields that are not obvious

`queryable` is a label, not a spec field. A Source without
`asgard-ai.com/queryable: "true"` is ingested and never retrieved, and nothing
about the CR looks wrong. It is the first thing to check when a document is
"loaded but the agent cannot find it".

Chunk size is per Source, not per base. The deployment uses 1500 uniformly,
which suggests nobody tuned it. That is reasonable; say so rather than
presenting it as a decision.

Loaders are capped per Workspace, shared by every project in it. One
recurring pull each, so a corpus fed from a dozen places does not fit and the
failure appears when the next one is created. `../wiki/integration.md` has the
number and `../wiki/knowledge.md` has what to count.

`loader-suspend` is how a scheduled pull is turned off. The deployment's own
web Loader ships suspended. A Loader deleted instead of suspended loses its
schedule and its URL list, and somebody rebuilds both from a diff.

Source names are numbered, not named. `src-<topic>-0001`, with the document's
real filename in a YAML comment above it. That is a workaround for names that are
not DNS-safe - which is most real document names, in any language other than
English - and the comment is the only place the mapping survives.

## Why an engagement moved off it

The reversal is recorded in the wiki rather than here: an
engagement built this path, found the Loader and its retrieval workflow harder to
keep correct than a context index over files, and moved. Nothing was wrong with
the CRs. It was the amount of machinery per document, and that only becomes
visible at the tenth one.

## Read the platform side first

`../wiki/knowledge.md` for the comparison and for what a Drive's
`contextIndex` does instead; `../usecase/knowledge-drive.md` for the shape
preferred over this one.

