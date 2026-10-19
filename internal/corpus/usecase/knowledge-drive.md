---
group: Read paths
description: knowledge that is documents, not rows - a written wiki read with no index when you do not know which to build, and a knowledge graph over raw feeds
---
# Knowledge drive

Unstructured knowledge - documents, FAQs, web pages - as a mounted `SourceSet`.
When you do not know how to build it, start with a written wiki the agent reads
directly; put a knowledge graph over the Drive when the material is raw feeds
nobody writes pages from.

**Seen in:** a public widget whose knowledge comes from a product catalogue
database, a crawl of its own marketing site, and manually uploaded documents.

**Checked:** against a Drive with two Syncers and a contextIndex, and the CRD;
against asgard-kube `cbd8d70` for the `web` class block - its
two required fields and the exactly-one-of over `urls` and `siteMapUrl` - and
against that deployment's own web Syncer, which is the second of the two.
How `contextIndex.prompt` reaches the indexer against asgard-core `478cf5d6` `internal/bpoperator/reconciler/ss_context_index_prompt.go`.
That the database Syncer keeps its cursor in its status and only writes
`statePath`, against asgard-syncer `8d278689` `internal/service/syncer.go`
(`LoadSyncState`, `WriteStateMember`) and asgard-syncer `internal/syncer/database.go`;
that the web Syncer deletes nothing against asgard-syncer `internal/syncer/web.go`
(`writePageFile`) at the same commit.
`contextIndex.includePaths` - its path rules, and that narrowing keeps what a
wider index built - against asgard-kube `3f9f6c8` `pkg/apis/asgard/v1alpha1/types.go`.
For the written wiki: the git Syncer's `includePaths` - its path rules, that
folders keep their place in the repository layout, what cone mode brings along,
and that a missing folder fails the run - against asgard-kube `1652e80`
`pkg/apis/asgard/v1alpha1/types.go` (`GitSyncerSpec`); the
`subPath` path rules, and that only a writable mount's missing directory is
created, against
asgard-core `f411b561` `internal/bpoperator/reconciler/sb_reconciler.go`
(`validateSourceSetMemberSubPath`, `ensureWritableSourceSetMountDirs`).

**Unchecked:** whether the three things listed for `contextIndex.prompt` are the ones that improve a given graph. That shows only against the customer's own files.
How asgard-syncer carries out a git Syncer's `includePaths` (partial clone and
cone sparse checkout, per asgard-kube `aab2d34`): only the contract was read,
not asgard-syncer.
The written wiki runs in a deployment, per the platform team, but this page was
not written from that deployment: how its agent read the wiki and how well it
answered are not recorded here.

Read the platform side first: `../wiki/knowledge.md` covers Drive, Context
Index, and how Knowledge Base differs. This page assumes you have read it.

What has to come from the customer before any of this can be built is in
`../needs/knowledge-drive.md`.

## When this shape, and when not

Use it when the customer's knowledge is not rows in a database: product
documents, FAQ spreadsheets, pages on a website. The agent queries a knowledge
graph and then reads the few files it points at.

Do not use it for structured data that a query answers exactly. The two are
complementary and neither does the other's job: a Drive answers "what is this
machine roughly, how do I choose, how do I fix it"; a query answers "how many,
which ones, what is the phone number".

Prefer this over `KnowledgeBase` for new work, with this caveat about what that
preference rests on. `KnowledgeBase`, `Loader`, `Indexer` and `Source` are all live CRDs
- though `Indexer` appears in no reference deployment at all, so what is said
about it here is read off the schema and nothing else -
none carries a deprecation marker, and the console ships the feature with its
own documented UI. The evidence is narrower: one engagement built knowledge on
`KnowledgeBase` + `Loader` + a retrieval workflow and moved it to a Drive with a
Context Index (TASK-013). An older chart containing one is not automatically
wrong; somebody chose that shape before that migration happened. Ask the
platform team before telling a customer the mechanism is going away.

## When unsure: a written wiki, with no index

Use this shape first when you do not know how to build the knowledge. Move to a
Context Index only in the cases listed at the end of this section.

The shape is the following, and nothing else is required:

1. The Drive holds pages somebody has written, kept as an LLM wiki, with an
   `index.md` in each folder that says what each page is for. The pages are
   already the synthesis, so the agent reads them directly.
2. The SourceSet has no `contextIndex`. Leaving the field out is the whole
   switch: no index, no derived CRs, no graph. `asgard-cli add knowledgedrive
   <drive> --repo <this repository>` writes this shape, and writes the field
   only when `--connector` is also given. The suspend label is a different
   thing: it pauses an index that exists and keeps its derived CRs and its
   graph.
3. The consuming agent's prompt says where to start, or it reads every file:

       先讀 /knowledge/index.md,照裡面的連結只打開跟問題有關的頁面 ——
       不要自己遍歷整個目錄逐檔閱讀。

### Where the pages live

In the engagement repository, the pages go in `assets/drives/<drive>/`, one
directory per Drive, for example `assets/drives/customer-service/`. The
customer's own files go in `references/` through `asgard-cli reference add`,
and the pages written from them name which file and date they came from.
`asgard-cli init` writes `assets/drives/README.md`, which says what goes
there and what does not. Do not invent another directory: the `subPath` in every
chart, and the next agent looking for the pages, depend on this one.

The repository reaches the Drive through a git Syncer on this repository:
`destinationPath: "git/"`, `revision` pinned to the release tag for a private
repository, and `includePaths: ["assets/drives/<drive>/"]`, which is what
`add knowledgedrive --repo` writes. The folder keeps its place in the
repository layout, so the pages are at `git/assets/drives/<drive>/` in the
volume, and that is the `subPath` to mount.

`includePaths` decides what reaches the volume. With it, `references/`,
`docs/` and every other Drive's folder stay off; git's cone mode still brings
the files directly in the repository root and directly in `assets/drives/`,
so keep nothing there that the platform must not hold. Without it, the whole
repository is cloned. Every entry has to be a folder at the synced revision:
a missing one fails the run and leaves the Drive as it was, so commit
`assets/drives/<drive>/index.md` before the release tag. Give the Drive its own
SourceSet and Syncer rather than reusing a SkillSet's, which the platform UI
expects to belong to that SkillSet alone.

```yaml
  sourceSetMounts:
    value: '[{"sourceSetName": "ss-<drive>", "subPath": "git/assets/drives/<drive>/", "mountPath": "/knowledge", "readOnly": true}]'
```

Keep the pages somewhere else only when the customer writes and maintains them
in their own tool, such as a Google Drive folder. Then a Syncer of that class
fills the Drive, and nothing goes under `assets/drives/`.

Everything else is the same as any Drive. The pages arrive through any Syncer
class - git, Google Drive, OneDrive, SFTP and the rest - or by upload, and a
SourceSet can be split whichever way the audiences need. To give different
agents different knowledge, choose between these:

| | one SourceSet per audience | one SourceSet, a `subPath` per agent |
|---|---|---|
| how | each SourceSet has its own Syncer, from its own source or from this repository narrowed by `includePaths` to that audience's folder; each agent mounts its own | one Drive holds every folder; each agent's `sourceSetMounts` entry takes a `subPath` |
| separation | a separate volume per audience; an agent cannot reach another's files | every folder is on one volume; the separation is only in what is mounted |
| cost | one sync per SourceSet | one sync |
| use it when | the sources differ, or one audience's material must not sit beside another's | one source is split by folder for different agents |

For the `subPath` route:

  - `subPath` is relative to the Drive's root, with no leading `/`, no `.` or
    `..` segment and no `//`. `contextIndex.includePaths` cannot do this job: it
    narrows what an index reads, and the agent still sees the whole Drive. A
    SkillSet cannot either, because it always mounts the whole SourceSet - see
    `../usecase/conventions.md`.
  - A git Syncer's `includePaths` lists every folder the Drive holds, and a
    `subPath` then picks one per agent. Material that must not reach the
    platform at all goes in another repository: this one is also synced by its
    SkillSets, whose Syncers narrow only if they set `includePaths` themselves -
    see `../usecase/skill-set.md`.
  - Mount every folder at the same relative place under one root, so links
    between pages still resolve: with `git/assets/drives/sales/` at
    `/knowledge/sales` and `git/assets/drives/common/` at
    `/knowledge/common`, a sales page's `../common/x.md` opens.

Run the Syncers before the first conversation. The platform creates a missing
`subPath` directory only for a writable mount, so a read-only mount of a folder
that has not been synced yet has nothing in it.

How good the answers are depends on the pages, and on each `index.md` saying
what its pages are for. Writing and maintaining them is the work this shape
asks for, and the `llm-wiki` skill under `.agents/skills/` describes it:
`assets/drives/` is part of the wiki it maintains, and `references/` holds
the raw sources the pages are written from.

Move to a Context Index, as in the rest of this page, when:

  - the material is something nobody writes pages from: rows a database Syncer
    exports, crawled pages, uploaded PDFs. There is no `index.md` to start from,
    and the graph takes its place.
  - there are too many pages for an agent to find the right one by following
    `index.md` and searching the files.

The two can share one Drive: a wiki folder mounted with a `subPath`, and a raw
feed in another folder that an index reads through `includePaths`.

This shape is the platform team's recommendation for when the right one is not
clear. It is not a platform rule. After the first deploy, ask the agent a few
questions with `asgard-cli operate chat send` and check that it opens
`index.md` first.

## The shape

    SourceSet  ss-<name>-knowledge     declares no members - the paths its
      catalog/                         Syncers write to are what is in it
      website/
      docs/                            uploaded by hand, no Syncer
      .context-index/                  the platform's, from spec.contextIndex
      <- SandboxBlueprint.sourceSetMounts, readOnly at /knowledge

`spec.contextIndex` is the whole switch: setting it makes the reconciler
derive three same-named CRs (a Workflow, a SandboxBlueprint and a Trigger) that
mount the Drive writable and run the indexer on a cron.

## Generate it

    asgard-cli add knowledgedrive <drive> --repo <this repository's git url>   # the written wiki
    asgard-cli add knowledgedrive <name> --connector dc-<name>                  # a raw feed with a graph

Both flags together write both into one Drive, with the index narrowed to the
database Syncer's `data/` folder. `--private` pins the git Syncer to the
release tag and adds the PAT. The rest of this section is the raw-feed shape.

That writes the structure below with the fields that fail silently already in
place. Those fields get lost when a skeleton is copied by hand, and nothing
reports them missing: not helm lint, not CRD validation, not a server dry-run.
The generated file marks the judgement calls TODO, and the rest of this page
covers them.

## The skeleton

`templates/source_set/ss-<name>-knowledge.yaml`, plus one Syncer per member.

```yaml
apiVersion: asgard-ai.com/v1alpha1
kind: SourceSet
metadata:
  name: ss-<name>-knowledge
  annotations:
    asgard-ai.com/source-set-name: "<display name>"
    asgard-ai.com/source-set-description: "<what knowledge this holds>"
  labels:
    {{- include "<chart>.labels" . | nindent 4 }}
spec:
  apiKey:
    valueFrom:
      secretKeyRef:
        name: {{ include "<chart>.appSecretName" . }}
        key: asgard_resource_api_key
  # No member registry: what is in the Drive is whatever its Syncers write,
  # plus anything uploaded by hand. The paths are the truth.
  #
  # Setting contextIndex derives three same-named CRs that build the graph on a
  # cron. To pause, label the SourceSet context-index-suspend - do not remove it.
  contextIndex:
    cron:
      schedule: {{ .Values.<name>Knowledge.contextIndex.schedule | quote }}
      timeZone: {{ .Values.<name>Knowledge.timeZone | quote }}
    prompt: |-
      <domain knowledge only - this is appended to the platform's own indexing
      instructions. e.g. newest partition wins per product_id>
    # includePaths:          # only when a Syncer brings in more than the
    #   - website/           # knowledge; absent indexes the whole Drive
---
apiVersion: asgard-ai.com/v1alpha1
kind: Syncer
metadata:
  name: syn-<name>-catalog-db
  annotations:
    asgard-ai.com/syncer-name: "<display name>"
  labels:
    asgard-ai.com/syncer-suspend: {{ .Values.<name>Knowledge.catalogSync.suspend | quote }}
    asgard-ai.com/auto-fire-on-rollout: "true"
    {{- include "<chart>.labels" . | nindent 4 }}
spec:
  sourceSetName: ss-<name>-knowledge
  # Relative path inside the volume: no leading /, no . or .. segment, no //.
  # Must end with /, for every syncer class.
  destinationPath: "catalog/"
  # A record of the last run, written and never read back; the cursor is in
  # the Syncer's status. Must NOT end with /.
  statePath: ".syncer-state/<name>-catalog"
  syncerClass: database
  schedule: {{ .Values.<name>Knowledge.catalogSync.schedule | quote }}
  timeZone: Asia/Taipei
  database:
    dataConnectorName: dc-<system>
    batchSize: {{ .Values.<name>Knowledge.catalogSync.batchSize }}
    # Immutable. Changing the projection means a new Syncer.
    columns:
      - name: product_id
        isIdentifier: true
      - name: <...the rest of the projection...>
      # Strictly-greater-than cursor, kept in the Syncer status. At most one.
      - name: row_updated_at
        isMaxValueColumn: true
    query: |
      select ... from ...
---
apiVersion: asgard-ai.com/v1alpha1
kind: Syncer
metadata:
  name: syn-<name>-pages
  annotations:
    asgard-ai.com/syncer-name: "<display name>"
  labels:
    # Per Syncer, like the one above - a Drive's two feeds are suspended and
    # scheduled separately because they cost different amounts to run.
    asgard-ai.com/syncer-suspend: {{ .Values.<name>Knowledge.pagesSync.suspend | quote }}
    asgard-ai.com/auto-fire-on-rollout: "true"
    {{- include "<chart>.labels" . | nindent 4 }}
spec:
  sourceSetName: ss-<name>-knowledge
  destinationPath: "website/"
  statePath: ".syncer-state/<name>-pages"
  syncerClass: web
  schedule: {{ .Values.<name>Knowledge.pagesSync.schedule | quote }}
  timeZone: Asia/Taipei
  web:
    # Both required, neither defaulted: how long a page may take, and how long
    # to wait after it loads before reading it. A page whose content arrives
    # after the first paint needs the second one raised.
    timeoutMs: {{ .Values.<name>Knowledge.pagesSync.timeoutMs }}
    waitForMs: {{ .Values.<name>Knowledge.pagesSync.waitForMs }}
    # Exactly one of urls and siteMapUrl, and the list comes from values - see
    # the page-list rule below.
    urls:
      {{- range .Values.<name>Knowledge.pagesSync.urls }}
      - {{ . | quote }}
      {{- end }}
    # maxDepth, limitPerUrl and delayMs left out: omitting them fetches the
    # listed pages and nothing else. maxDepth is the field that turns the list
    # into a crawl; delayMs is the throttle to set before pointing one at a
    # customer's live site.
```

`isMaxValueColumn` and `isIdentifier` sit on a column, not on the `database`
block. Written one level up, beside `dataConnectorName`, they are unknown
fields: the apiserver drops them silently, and the Syncer then re-reads the
whole table every run. Nothing in the rendered chart, in
`helm lint` or in the apply output says so.

`batchSize` and `query` are both required. A `database` Syncer missing
either is rejected at apply time, which in practice means during CD.

The Syncer wraps the query as

```sql
select <columns> from (query) where <cursor> > $cursor order by <cursor> asc
```

so every name in `columns` has to appear in the projection spelled exactly the
same way, and the cursor column has to be comparable.

Mount it read-only from the blueprint:

```yaml
  sourceSetMounts:
    value: '[{"sourceSetName": "ss-<name>-knowledge", "mountPath": "/knowledge", "readOnly": true}]'
```

## Scheduling, and the two labels

Both are the platform's, and `../wiki/knowledge.md` owns them. Read it for
why the index goes after the Syncers rather than with them, for the hour a
deployment actually puts between the two, for the `-ci` suffix the derived CRs
take (look for it on a cluster when the index is not running), and for
`syncer-suspend` against `auto-fire-on-rollout`: a suspended Syncer with no
auto-fire label never runs on its own, while the gate is green and the run
succeeded.

`../usecase/skill-set.md` has what those two labels look like on a Syncer
somebody writes by hand.

## Pausing the index

To pause indexing, label the SourceSet `context-index-suspend: "true"`.

Clearing the `contextIndex` field instead tears down the three derived CRs and
renames the index aside, so the next run rebuilds the graph from scratch.
Use the field to create or remove the index, not to schedule it.

## Designing it - the part the generator leaves TODO

### What goes in the Drive, and what does not

Documents a person would read to answer the question: product literature, FAQs,
policies, the pages of a public site. Not anything a query answers exactly:
counts, prices, stock, contact details. Those belong to a query tool, and putting
them in a Drive makes the agent paraphrase a number it should have read.

The two are complementary: the Drive answers "what is this, how do I choose, how
do I fix it"; a query answers "how many, which ones, what is the number".

### `contextIndex.prompt` - what the indexer needs to know

It is appended to the platform's own instructions, in a section that takes
precedence over the indexer's guesses about the files but not over its
boundaries - it may write only the graph directory and an `AGENTS.md` at the top
of the Drive. So only domain knowledge belongs there, and "reorganise the
folders" is not something it can ask for. The indexer also keeps its own
corrections in that `AGENTS.md`, which it reads back on every run. In practice,
three things:

- what each folder holds, one line each
- the key, and which copy wins. With incremental sync, the same record
  appears in several dated partitions; say which one is current, or the graph
  treats stale versions as facts
- what a field means where the name does not carry it

### Telling the agent how to read it

Put this in the consuming agent's prompt, not in the Drive:

    先用 graphify 查 /knowledge 的知識圖,拿到相關檔案與段落後再去讀那幾個檔案 ——
    不要自己遍歷整個 Drive 逐檔閱讀。

Without it the agent reads everything, slowly, and still misses things.

### Before the first demo

Manually uploaded documents are not there until someone uploads them, and the
graph is not useful until it has run once. Write this in the chart README as a
post-deploy step with an owner. Otherwise a demo shows the agent answering the
structured questions well and the knowledge ones badly.

## Fields that are not obvious

`destinationPath` must end with `/`, for every syncer class but
`static-content`, whose destinationPath is the one file it writes
(`../wiki/sindri.md`); `statePath` must not. Both are relative paths inside the volume, and the CRD
rejects a leading `/`, a `.` or `..` segment, or `//`.

The folder does not need declaring anywhere. Writing to it creates it, so a
typo produces a second, empty folder rather than an error.

The database Syncer is incremental. `isMaxValueColumn` is a
strictly-greater-than cursor kept in the Syncer's status; a day with no changes
writes nothing. Write these two consequences into the CR header:

- an updated row leaves its old copy in an older partition, so the index prompt
  has to say newest-partition-wins per key
- a soft-deleted row never disappears from old partitions. A full rebuild means
  deleting the partitions and clearing the sync state, which is in the
  Syncer's status, so a replacement Syncer starts without it

`spec.database.columns` is immutable. Changing the projection means a new
Syncer, not an edit.

Keep the web Syncer's page list in version control rather than enabling a
deep crawl. What the agent can see should be reviewable, and `siteMapUrl` -
the other half of the CRD's exactly-one-of with `urls` - hands that decision
to whoever maintains the site. When more pages are wanted, add them to the
values list or stand up a second Syncer in sitemap mode; do not raise
`maxDepth` on the one holding the reviewed list. Taking a page off the list does
not take it out of the Drive: the web Syncer writes each page it crawls and
deletes nothing, so a dropped page stays indexed until somebody deletes its
file.

`contextIndex.prompt` is appended to the platform's own indexing
instructions, so only domain knowledge belongs there.

`contextIndex.includePaths` narrows the index to some folders of the Drive;
absent or empty, the whole Drive is indexed. It is for a Syncer that brings in
far more than the knowledge - a whole git repository when only its `docs/`
matters. Each entry is a folder written like a `destinationPath`
(`repos/handbook/docs/`): relative, ending in `/`, no `.` or `..` segment, and
not under `.context-index/`. Names are literal, never globs, and entries are a
union. A folder that does not exist yet is not an error; it adds nothing until
something lands in it. Narrowing never removes what a wider index already
holds: to start over, remove `contextIndex` and set it again, which sets the old
graph aside and builds from scratch.

## Querying it

Mount read-only, and tell the agent in its prompt to query the graph first and
then read only the files the graph points at. Without that instruction it
crawls the whole Drive.

`readOnly: true` is part of the security design: an agent that answers
questions about knowledge has no reason to be able to change it.

## The manual step that has to be written down

Documents nobody can sync automatically - the PDFs, the spreadsheet of FAQs -
have to be uploaded after deploy (`asgard-cli operate source-set put <source-set>
<local-file> docs/<name> --release <release>`, into a folder no Syncer writes),
and the Syncers and index have to run once.
Until then the knowledge answers are poor. Put it in the chart README as a
post-deploy step. A Syncer the deploy does not fire is run with
`asgard-cli operate syncer sync <syncer> --release <release> --wait <duration>`,
and once the Syncers have finished, the first index refresh with
`asgard-cli operate source-set reindex <source-set> --release <release> --wait <duration>`.

## Verify

```bash
asgard-cli check
asgard-cli verify <project>
```

The xref check resolves `sourceSetName` and `database.dataConnectorName`, and
validates the path rules on `destinationPath` / `statePath`.

After deploying, confirm the first sync ran before judging the answers:

```bash
asgard-cli pipeline runs log <run-id> apply
```

Its syncers section lists what the deploy fired. No section, or zero Syncers
opted in, means the auto-fire label is missing. What each run did is in the
Syncer's own history, whoever started it:

```bash
asgard-cli operate syncer executions <syncer> --release <release>
```
