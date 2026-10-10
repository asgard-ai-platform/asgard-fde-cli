---
group: Read paths
description: knowledge that is documents, not rows
---
# Knowledge drive

Unstructured knowledge - documents, FAQs, web pages - as a mounted `SourceSet`
with a knowledge graph over it.

**Seen in:** a public widget whose knowledge comes from a product catalogue
database, a crawl of its own marketing site, and manually uploaded documents.

**Checked:** against a Drive with two Syncers and a contextIndex, and the CRD;
against asgard-kube `cbd8d70` for the `web` class block - its
two required fields and the exactly-one-of over `urls` and `siteMapUrl` - and
against that deployment's own web Syncer, which is the second of the two.
How `contextIndex.prompt` reaches the indexer against asgard-core `478cf5d6` `internal/bpoperator/reconciler/ss_context_index_prompt.go`.

**Unchecked:** whether the three things listed for `contextIndex.prompt` are the ones that improve a given graph. That shows only against the customer's own files.

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

    asgard-cli add knowledgedrive <name> --connector dc-<name>

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
  # Where the incremental cursor is kept. Must NOT end with /.
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

`destinationPath` must end with `/`, for every syncer class;
`statePath` must not. Both are relative paths inside the volume, and the CRD
rejects a leading `/`, a `.` or `..` segment, or `//`.

The folder does not need declaring anywhere. Writing to it creates it, so a
typo produces a second, empty folder rather than an error.

The database Syncer is incremental. `isMaxValueColumn` is a
strictly-greater-than cursor kept in the Syncer's status; a day with no changes
writes nothing. Write these two consequences into the CR header:

- an updated row leaves its old copy in an older partition, so the index prompt
  has to say newest-partition-wins per key
- a soft-deleted row never disappears from old partitions. A full rebuild means
  deleting the partitions and clearing the sync state

`spec.database.columns` is immutable. Changing the projection means a new
Syncer, not an edit.

Keep the web Syncer's page list in version control rather than enabling a
deep crawl. What the agent can see should be reviewable, and `siteMapUrl` -
the other half of the CRD's exactly-one-of with `urls` - hands that decision
to whoever maintains the site. When more pages are wanted, add them to the
values list or stand up a second Syncer in sitemap mode; do not raise
`maxDepth` on the one holding the reviewed list.

`contextIndex.prompt` is appended to the platform's own indexing
instructions, so only domain knowledge belongs there.

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
