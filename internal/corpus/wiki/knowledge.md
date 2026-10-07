---
group: While building
description: Drive, Context Index, and how Knowledge Base differs
---
# Drive and Knowledge Base

Both hold unstructured knowledge and both are live. Drive is what this repo
recommends for new work, and the reason is one engagement's experience rather
than anything the platform has announced.

| | Drive | Knowledge Base |
|---|---|---|
| status | live | live - no deprecation marker on any of its four CRDs, and the console ships the feature |
| CRs | `SourceSet` (+ `Syncer`) | `KnowledgeBase` + `Loader` + `Indexer` + `Source` |
| retrieval | a Context Index knowledge graph | RAG over chunks |

Start new knowledge on a Drive with a Context Index. The recommendation comes
from one deployment that moved off `KnowledgeBase` + `Loader` + a retrieval
workflow and found the Drive simpler to keep correct. It does not mean the
platform is retiring Knowledge Base. Before repeating it to a
customer as platform direction, ask the platform team, because nothing in the
CRDs or the product documentation says so.

## Drive

A mountable file store an agent can read and write. Creating one needs a Name and
a Description, plus optional Reference Paths (a path may not start with `/`, and
may not contain `../` or `./`). None of the three is a field of the `SourceSet`
CR, whose spec holds only `apiKey`, `labels` and `contextIndex`.

The detail page has four tabs:

| tab | |
|---|---|
| Files | a file browser whose toolbar works without an edit mode: New file, New folder, Upload, Download, Copy, Cut, Paste, Rename, Delete, Refresh. Also Open in Advance Editor |
| Syncers | scheduled pulls from external sources |
| Context Index | builds a searchable index over the Drive |
| Settings | the same fields as creation |

### Syncers

The table shows Folder, Source, Active, Status, Schedule. New Syncer opens a
three-step wizard: Type, Basic, Settings.

Not every `syncerClass` the CRD supports is reachable from the UI.

| source | in the UI | `syncerClass` |
|---|---|---|
| Google Drive | yes | `google-drive` |
| OneDrive | yes | `onedrive` |
| Git | yes | `git` |
| Web Crawler | yes | `web` |
| Data Source | yes | `database` |
| Bot | no | `bot` |
| Dropbox | no | `dropbox` |
| FTP / SFTP / SMB | no | `ftp` / `sftp` / `smb` |

The ones marked no can only be declared in a chart. If a customer's data lives on an
FTP server or an SMB share, that route works, but it has to be built in a chart.

An `sftp` Syncer can set `sftp.transfers` (files moved in parallel) and
`sftp.checkers` (files compared in parallel), each between 1 and 64. Leave both
unset unless the sync is too slow or the server objects: the right value
belongs to the customer's server, not to the platform. A small NAS, a managed
SFTP endpoint with a session limit, or a host behind fail2ban can treat high
concurrency as an attack, so ask whether their server limits sessions, for the
values to set. Raising `checkers` past what the server handles makes the scan
slower, not faster.

### Context Index

Builds a searchable index over the Drive so an agent queries the index instead of
reading every file each time. It refreshes on a schedule. Off by default; Enable
Context Index turns it on.

It maps to `SourceSet.spec.contextIndex`. Setting the field makes the platform
derive three CRs - a Workflow, a SandboxBlueprint and a Trigger - that run the
indexer, which builds the graph in the volume's `.context-index` directory.

To pause indexing use the label `asgard-ai.com/context-index-suspend`. Clearing
the `contextIndex` field instead tears down those three derived CRs and renames
the built index aside, to `.context-index_bak_<timestamp>`.

## The index runs after the Syncers, not with them

`contextIndex.cron` and each Syncer's `schedule` are independent and nothing
orders them. Put the index after the Syncers on the same day - the deployment
runs its two Syncers at 09:00 and the index at 10:00, both `Asia/Taipei`.

If the index runs before or at the same time as the Syncers, it walks the volume
before the day's content lands, so the graph always describes the previous day
and nothing reports a failure. An incremental update over an unchanged Drive
finishes in seconds, so the gap costs nothing.

The derived CRs take the SourceSet's name with a `-ci` suffix, so a Drive named
`ss-<name>-knowledge` produces `ss-<name>-knowledge-ci`. That is what to look for
on a cluster when the index is not running.

## Two labels, and neither reads the other

`asgard-ai.com/syncer-suspend: "true"` stops the scheduler and nothing
else. It does not stop a deploy from firing the Syncer. That is deliberate: the
skills Syncer relies on it.

What fires it on a deploy is a second, opt-in label,
`asgard-ai.com/auto-fire-on-rollout: "true"`: the platform's apply step fires
the Syncers of the release that carry it and waits for them. Only that runner
reads the label; the Syncer module ignores it, and neither label reads the
other. A suspended Syncer with no auto-fire label never runs. The symptom is an
empty drive or an agent with zero skills, while the gate passes, the run
succeeds and no error is reported.

Firing on deploy used to be the default, opted out of with
`asgard-ai.com/syncer-cd-trigger: "false"`. The platform does not read that
label; it is left from the CD workflows that predate the pipeline. Now a Syncer
does not fire on deploy unless it carries the auto-fire label. See
`../usecase/skill-set.md`.

Suspending a git Syncer has a cost the labels do not show. A git sync clones
into a temporary directory, then empties the destination and copies into it,
with no atomic swap. A sync interrupted between the two leaves a partial skill
tree, and the agent answers from it without any error. Every run rebuilds the
whole tree, so the next successful run repairs it - but on a suspended Syncer
the next run is the next deploy, and until then the tree stays partial. One
deployment leaves its skills Syncers unsuspended on a 30-minute schedule for
this reason, and accepts a few seconds every half hour in which the tree is
being rewritten. Which one to choose is a trade: suspended keeps the skills
tied to what was deployed, and scheduled repairs itself.

Changing a Syncer's spec bumps its generation, and the operator replaces its
CronJob. A Job created from that CronJob is deleted with it, so a deploy that
changes a Syncer's spec should not overlap another deploy that fires it.
Changing only its labels does not bump the generation.

## Knowledge Base

`../usecase/knowledge-base.md` has the `KnowledgeBase` + `Loader` +
`Indexer` + `Source` shape field by field, the way `../usecase/knowledge-drive.md` has the
Drive one. Reading a chart that uses it is the case it exists for.

For recognising older charts only. Creating one needs a Name and an Alias Name.
In the CR, `aliasName` matches `^[a-z_][a-z_0-9$]*$` - lowercase letters,
digits, underscores and `$`, not starting with a digit - and may not start with
`kb_` or `eh_`.

A spreadsheet needs more Indexer configuration than a document. `Indexer.spec` picks a
`sourceClass` and the block for it, and the two tabular ones demand more than
the document ones: `csv` requires `columns` and `skipHeader`, and `xlsx`
requires `columns`, `skipHeader` and `sheetName`. So the columns have
to be declared before anything is indexed, and `sheetName` means one Indexer
per sheet: a workbook with four sheets worth indexing is four of them. The
document classes (`pdf`, `docx`, `pptx`) and the media ones (`image`, `audio`,
`video`) require none of that.

`xlsx` is also the one immutable Indexer field - see `../wiki/crd-rules.md`.

Content is split across All, Manual Upload and Auto Load tabs.

Manual Upload takes CSV, XLSX, PDF, PPTX, DOCX and JSON Lines. A CSV goes
through three steps: Import File, Processing (preview the columns, choose which
to include, toggle Skip Header, pick an Identifier column - required), Finish.

Auto Load has three external sources: Crawler, Data Source, Asgard App. The
schedule is Daily or Weekly plus a time.

## What belongs in a Drive

Things a query cannot answer exactly: documents, FAQs, web pages. Counts, prices
and stock levels belong to a Semantic Model or a fixed query tool - putting a
number in a Drive makes the agent paraphrase a figure it should have read.

## Before writing the chart

`../usecase/knowledge-drive.md` has the full Drive-plus-Syncer shape.

## The Loader cap, workspace-wide

A Loader is one recurring pull, and the platform caps how many a Workspace may
have - shared across every project in it, not per knowledge base. Indexers and
Processors are capped on the same basis. `../wiki/integration.md` owns those
numbers and says why the Loader one bites first.

A customer with a dozen document sources exceeds it before anything else in
the quota list, and the failure arrives when the next Loader is created rather
than at design time. Two consequences to take into an interview:

  - count the sources, not the documents. A crawl of a whole site is one
    Loader; the same files spread across separate places is one each
  - a Drive with a Syncer is a different mechanism and is not counted here -
    which is one more reason it is the recommendation for new work

They are defaults rather than ceilings, and `../wiki/integration.md` has how
each is raised.

## Citations are possible, and they are not automatic

Most customers ask where an answer came from. The platform can return sources
if the Workflow is built to return them.

Retrieval happens entirely server-side - there is no separate knowledge endpoint,
the same message call runs the retrieve processor and the model - and the sources
come back on `asgard.message.complete`, inside the message's `template`:

    fact.messageComplete.message.template.references[]
      title, uri

The documentation's example reads `template.sources[]`, and the platform has no
such field. `template` is a fixed type, so a key it does not declare is dropped
before the event is sent; `references` is the one for citations, and a Workflow
that needs something else returns it in the message's `payload`, which is passed
through as written. So citations have to be designed when the chart is written;
there is no switch for them. A front end
that does not read `template` shows an answer with no provenance no matter what
the retrieval did. Decide it before the chart, because retrofitting it means
touching the workflow and the front end together.

## Retrieval quality depends on how the question is asked

Give the customer this guidance at handover. The documentation says:

    specific keywords beat vague ones     "how many working days for a refund"
    one topic per question                not "tell me everything you have"
    context helps                         "as a business customer, what is the
                                          renewal process"

If the customer's staff ask broad questions, they will judge the knowledge base
as bad even when the corpus is fine. Put this in the handover and in the sample
questions on the agent: `On-boarding Settings` is for this, and good starter
questions show users how to ask without a guide.

## Sources

- [Drive](https://docs.asgard-ai.com/docs/product-suite/odin/features/drive)
  - asgard-docs `6261fdff`
- [Knowledge](https://docs.asgard-ai.com/docs/product-suite/odin/features/knowledge-base-knowledge)
  - asgard-docs `6261fdff`
- The syncer-class table and the contextIndex behaviour:
  [asgard-kube](https://github.com/asgard-ai-platform/asgard-kube)
  `3da0365` - `SyncerClass`, `SourceSetSpec`, `SourceSetContextIndex`
- The schedule ordering and the two switches: read off a deployment's own Drive
- Preferring a Drive over `KnowledgeBase`: one deployment's migration. Held
  against
  [asgard-kube](https://github.com/asgard-ai-platform/asgard-kube) `cbd8d70` -
  `knowledgebases`, `loaders`, `indexers` and `sources` all exist
  and none is marked deprecated - and against asgard-docs `f00e0ee`, which
  documents the feature as current

- That retrieval is server-side on the same endpoint, and the question-shape
  guidance; the example's `template.sources` is corrected above:
  [Knowledge base query](https://docs.asgard-ai.com/docs/developer-reference/examples/knowledge-base-query)
  - asgard-docs `f00e0ee`

**Checked:** the syncer classes, `SourceSetSpec`, the context index's derived
CRs and backup name, the Knowledge Base `aliasName` and the Indexer's required
fields against asgard-kube `3da0365` `pkg/apis/asgard/v1alpha1/types.go`; the
`-ci` suffix and the suspend label being read by the Syncer reconciler against
asgard-core `478cf5d6` `internal/constants.go` and `internal/bpoperator/reconciler/syn_reconciler.go`;
the git sync's clear-then-copy against
asgard-syncer `8d278689` `internal/syncer/git.go` (`GitSyncer.Run`, `clearDir`),
the unsuspended schedule against asgard-freyr-kube `8f6d6c1`
`tenants/xxtechec/chart/app/templates/source_set/git_repos.yaml`, and the
CronJob replacement against
asgard-core `001bbf69` `internal/bpoperator/reconciler/syn_reconciler.go`;
the SFTP concurrency fields against asgard-kube `42e8722`
`crd/asgard-ai.com_syncers.yaml`;
the citation field against asgard-core `478cf5d6` `internal/models/edgeserver.go`
(`MessageTemplate`, `MessageTemplateReference`) and
asgard-core `internal/models/processor.go` (`PushMessageStaticConfig`), and asgard-js-sdk `56ad14e` `packages/core/src/types/sse-response.ts`.

**Unchecked:** the Drive and Knowledge Base screens come from the product
documentation only, and only a Console account can show them; the auto-fire
label is read by the platform's apply step, which is in none of the
repositories here.
