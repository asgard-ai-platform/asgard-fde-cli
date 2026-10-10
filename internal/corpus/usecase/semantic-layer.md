---
group: Read paths
description: internal audience, open-ended questions
---
# SemanticLayer

The read surface an agent composes SQL against. It is the highest-effort
artifact in a chart, and the one where looking at the real database matters
most.

**Seen in:** deployments with one layer per source system; the largest single
layer runs to hundreds of cubes.

**Checked:** against every SemanticLayer CR in three deployments -
`completionModelName` is present on all of them - and the CRD; against
asgard-kube `cbd8d70` for the `Measure` block - the `format`
enum, `drillMembers`' minimum item count and the rule that `count` is the type
taking no `sql` - and against every measure in the reference charts that sets
`filters`, `format` or `drillMembers`. What the agent is handed from a layer
against asgard-core `478cf5d6` `internal/processor/domaintools/domaintools.go`.
How a join names its cubes against asgard-core `f7fd5f4e`, the same file, and
asgard-kube `0765c58`, whose only rule on a join is the equal length of its two
`dimensions` lists.

**Unchecked:** the modelling guidance - cube granularity, what belongs in `instruction`. Only the customer's database can show whether it holds.

Read the platform side first: `../wiki/semantic-model.md` covers what a
Semantic Model is, how it is built, and its limits. This page assumes you have
read it.

What has to come from the customer before any of this can be built is in
`../needs/semantic-layer.md`.

## When this shape, and when not

Use it when the audience is internal and authenticated, and the questions are
open-ended. The agent writes its own SQL over the cubes you expose, so it can
answer a question nobody wrote a tool for.

Do not use it for a public audience. Mounted without `allowedCubes` it lets
the agent compose arbitrary SQL over every cube in it, and the exposed surface
grows every time a cube is added; nobody goes back and narrows it. Excluding
the sensitive tables does not fix this, because the risk comes from the shape.
Use fixed query tools there instead.

## Do not guess a schema

Connect to the database and introspect it. Reasoning about a schema does not
verify it, and these failures do not show in column names:

- a receipt line can have a twin line with the quantity negated (the
  offsetting accounting entry), so a sum without the right filter returns zero
- a log table can be ~90% duplicate rows, so a raw join fans out ~30x and
  silently inflates every count
- a view with a window function can take 1m51s where the same expressions
  against the base table take 2.1s, because no filter pushes down
- a "safety stock" figure can be defined as coming from one specific location
  only, never the row's own, and must never be summed across locations

Each of those was found by running a query, and each is now a line in a
layer's `instruction`.

## The shape

    DataConnector  dc-<system>     connection coordinates + secretKeyRef
      <- SemanticLayer  sl-<system>
           cubes[]                 one per table
             dimensions[]          one per column you expose
             measures[]            aggregates. The key is REQUIRED on every
                                   cube - `measures: []` is accepted, leaving
                                   the key out is refused at apply
           joins[]                 REQUIRED as a key too - `joins: []` when
                                   the layer has none
           sampleQueries[]         the analysis views
      <- Agent.managed.semanticLayers[]

## Generate it

    asgard-cli add dataconnector <name> --db-class postgres
    asgard-cli add semanticlayer <name> --connector dc-<name>

That writes the structure below with the fields that fail silently already in
place - the display annotation, the labels the UI needs, the current field names.
Those get lost when the skeleton is copied by hand, and nothing reports them
missing: not helm lint, not CRD validation, not a server dry-run.

The generated file marks the judgement calls TODO, and the rest of this page
covers them.

## The skeleton

`templates/data_connector/dc-<system>.yaml` and
`templates/semantic_layer/sl-<system>.yaml`.

```yaml
apiVersion: asgard-ai.com/v1alpha1
kind: DataConnector
metadata:
  name: dc-<system>
  annotations:
    asgard-ai.com/data-connector-name: "<display name>"
  labels:
    {{- include "<chart>.labels" . | nindent 4 }}
spec:
  dataConnectorClass: mssql          # or postgres
  mssql:
    host: {{ .Values.<system>DB.host | quote }}
    port: {{ .Values.<system>DB.port }}
    user: {{ .Values.<system>DB.user | quote }}
    database: {{ .Values.<system>DB.database | quote }}
    password:
      valueFrom:
        secretKeyRef:
          key: <system>_db_password
          name: {{ include "<chart>.appSecretName" . }}
---
apiVersion: asgard-ai.com/v1alpha1
kind: SemanticLayer
metadata:
  name: sl-<system>
  annotations:
    asgard-ai.com/semantic-layer-name: "<display name>"
  labels:
    {{- include "<chart>.labels" . | nindent 4 }}
spec:
  completionModelName: {{ .Values.defaultCompletionModelName | quote }}
  effort: {{ .Values.defaultSemanticLayerEffort | quote }}
  dataConnectorName: dc-<system>
  locale: zh-TW
  timezone: Asia/Taipei
  instruction: |-
    <business rules the column names do not convey: 口徑, tables not to trust,
    joins that fan out, views that do not push filters down>
  cubes:
    - name: <schema>.<table>
      sqlTable: <schema>.<table>
      title: <human readable>
      description: <what this table is, in the customer's language>
      primaryKeyDimensions: [<pk column>]
      dimensions:
        - name: <ColumnName>          # the column its sql selects, not an alias
          sql: '{CUBE}.<ColumnName>'
          title: <human readable>
          description: <what it means - a dimension without this is invisible>
          type: string                 # string | number | time | boolean
      measures:
        - name: total_<thing>
          sql: '{CUBE}.<Column>'
          title: <human readable>
          description: <what it aggregates>
          type: sum
          # format: currency | percent - optional, presentation only, and it
          # names no unit. The charts that set it put the currency in the title.
        # A measure may carry its own predicate, which is how one business term
        # gets one definition. `count` is the type that takes no sql.
        - name: <thing>_count
          title: <human readable>
          description: <what it counts, the condition included>
          type: count
          filters:
            - sql: '{CUBE}.<flag> = TRUE'
  joins:
    - name: <from_cube>_<from_dim>_<to_cube>_<to_dim>
      description: <what the relationship is>
      relationship: one_to_many
      from: {cube: <schema>.<table_a>, dimensions: [<col>]}
      to:   {cube: <schema>.<table_b>, dimensions: [<col>]}
  sampleQueries:
    - comment: <what it answers, and any caveat about the numbers>
      sql: |
        select ...
```

Connection coordinates are `chartValues` declared in `.asgard-pipeline.yaml` and set on the
platform, one group per connector.
The password is only ever a `secretKeyRef`.

## Fields that are not obvious

`completionModelName` is required on a SemanticLayer, unlike an Agent. Take
it from a chart value rather than writing a model name into the template.

`effort` omitted is not the same as `medium`. Omitting it means the LLM
processors send no effort parameter at all and the model's own default applies.
Set it explicitly, from a value - and `disabled` where the chart's model is not a
reasoning model, because that combination fails every turn rather than being
ignored. `../wiki/settings.md` has why; settle the model before the layer.

A dimension's `name` is the column its `sql` selects - `name: ProductId` with
`sql: '{CUBE}.ProductId'`, never a re-cased alias. Measure names are aggregates
rather than columns, so they stay descriptive (`total_shipped_qty`).

Write `description` on every cube, dimension and measure, in the customer's
language. The agent reads it to decide which column answers a question. A
dimension with no description still reaches the model, but as a bare column
name, which in an older business system is often a code.

A measure's own `filters` are where a business term gets one definition. A
`count` narrowed by `{CUBE}.<flag> = TRUE` gives the customer's own phrase a name
the agent can ask for, instead of every query re-deriving the predicate and some
of them getting it wrong. It is not a substitute for `instruction`: a filter
defines one measure, while a rule the model must never break - rows that are
always excluded, a flag stored as a string - has to hold for every query it
writes. Put the definition in a filter and the prohibition in `instruction`.

`drillMembers` is the one optional measure field where an empty list is worse
than no key. The key may be omitted, but `drillMembers: []` is refused at
apply - the inverse of `measures: []` on the cube above it, which is the safe
form. Only one chart set writes it, always on a cube's row count and always
listing that cube's identifying dimensions; what reads it is not visible from the
CRD or from asgard-core, so copy the shape rather than inferring a behaviour
from it.

Analysis views go in top-level `sampleQueries[]` (`comment` + `sql`), never as
a cube-level `sql:` virtual cube. A `sql:` cube is also invisible to the
platform's raw SQL tools, which see only `sqlTable` cubes as tables; one
deployment that needed a derived cube added a second, `sqlTable`, cube over the
same table so raw SQL could still reach it. Run every one against the live
database before committing it: a sampleQuery that errors or returns nonsense
misleads the agent that reads it.

Set `primaryKeyDimensions` where the table has a key. Foreign keys are often
absent in older business systems; infer relationships from naming, then verify
with a join query before writing a `joins` entry:

    select count(*) from a join b on a.sno = b.sno;
    select count(distinct sno) from b;

A join names its cubes by `cubes[].name`, not by `sqlTable`, and each entry in
its `dimensions` by the `name` of a dimension on that cube. The CRD checks only
that the two `dimensions` lists are the same length, so a join to a cube or a
dimension the layer does not declare applies cleanly, and the agent is handed a
relationship it cannot traverse, with no error anywhere. When the cubes are
generated and the joins are hand-written, this breaks without anyone editing
the join: a table that drops out of the generator's selection, or a cap on
dimensions per cube that keeps the top-ranked columns, leaves the join pointing
at nothing. A column a join uses has to survive any such cap, whatever else the
ranking weighs. `asgard-cli verify` reports every join that does not resolve,
and every `primaryKeyDimensions` entry that names no dimension of its cube.

Business rules that are not in the column names go in `instruction`, so the
traps above are recorded in the layer that owns them.

## Designing the parts the generator leaves TODO

### Which tables become cubes

Work backwards from the questions, not forwards from the schema. Take the
queries the customer already runs - the reports someone maintains, the SQL in a
spreadsheet, the two or three things they ask every week - and reverse-engineer
which tables those need. Those tables are the layer.

A layer covering part of a system is a normal, finished state, not a
half-built one. Say so at the top of the file:

    # partial 語意層,目前涵蓋兩塊:
    #   1. 料件 × 倉別庫存 —— 由兩支常用的查詢逆推
    #   2. 到貨 —— TASK-002 為到貨通知新增
    # 其餘模組尚未建模。

The cube count follows the kind of question, not the size of the database:

| the agent's job | cubes, roughly |
|---|---|
| point lookups - "what is the status of this one" | a handful |
| history and elapsed-time analysis | ten or so |
| operational statistics across a whole process | dozens |

A system with 400 tables where people ask three questions gets a small layer.
Adding cubes "because they are there" widens what the agent can be asked without
widening what it can answer well, and every added cube is more search space
between the question and the right table.

### `instruction` - the rules the column names do not carry

This is the highest-value field in the CR. The agent gets it with the cubes
from the semantic-model tool, and whole even when the caller narrows the
cubes it may see. What belongs in it:

> rules that the column names cannot tell you, and that produce a wrong answer
> if broken.

It is not a description of the data; it lists the ways a plausible query is
wrong. The recurring cases:

- a value that must come from one specific row, never the row's own. A
  "target level" configured in one place, joined a second time with that filter
  pinned, and therefore never to be summed across rows.
- a column whose type is not what it looks like. A flag stored as the string
  `'T'`/`'F'`, where `= false` silently matches nothing.
- rows that must always be excluded. Codes with a suffix meaning
  discontinued, soft-deleted rows, test records.
- the convention for what to show. "Only list locations where at least one
  quantity is above zero" - obvious to a person, invisible to a model.
- a table that is already aggregated, so summing it double-counts.

Each one is a sentence and a reason. Write them as you find them during
introspection; they cannot be recovered from the schema later.

### `description` on every cube, dimension and measure

This is what the agent reads to decide which column answers a question, so write
it as what it means to the business, not what it is technically:

    ✗ 供應商 ID 欄位
    ✓ 每個供應商獨一無二的識別碼,用於區分不同供應商

A dimension with no description reaches the model as its name alone, and a
description that only restates the name adds nothing to that.

### `sampleQueries` - the analysis views

These are the reports the customer already lives on, expressed once so nobody
rebuilds them per conversation. Take them from what people actually run.

Execute every one against the live database before committing it, and put the
row count you saw in the `comment`: a query that returns 89 rows sets a
different expectation from one that returns 4.

## Two rules about mounting

Set `allowWrite: false` on every binding; the standing architecture is read-only.

Do not set `allowedCubes`. The platform honours the field and restricts the agent to
the cubes it names (at `478cf5d6`,
asgard-core `internal/bpcontroller/server/bp_controller.go` passes it to the query tools),
and one reference deployment uses it; this repository's
standing decision is that an agent may query any table in its own layer, so the
restriction is which layer it mounts, and `gate` R4 refuses an Agent that sets
the field. Note where it is: `Agent.spec.managed.semanticLayers[]`, never the
`SemanticLayer` itself, which has no such field. So for a layer with no Agent on
it at all there is nothing to set and nothing to narrow, and the only exposure
control is which cubes and dimensions the layer declares - see
`../usecase/mimir-dashboard.md`.

That covers the Agent path, not the whole platform. Where a layer is
mounted on a completion processor instead - the flow-agent shape, which has no
`Agent` CR - `semanticLayer.allowedCubes` is a config key that does restrict
what that processor may compose SQL over, and `../wiki/processors.md` owns it.
It defaults to empty, so the surface still widens with every cube added, but
there it can be narrowed again. Do not read the paragraph above as "nothing
narrows a mounted layer, anywhere"; `../wiki/semantic-model.md` weighs this
when it compares fixed query tools with a layer for a public audience.

`sampleQuestions` on the layer is not this field's counterpart either. It is
what Data Insight renders as the buttons under a layer's prompt box, it takes
plain strings, and it is the one field in the chart that changes what a person
sees before they type anything. `../usecase/mimir-dashboard.md` is where
it is written up, because that is the shape whose consumer is a person.

## The OLAP exception

A company-wide warehouse layer is a superset of the per-system layers, with
unrestricted cross-schema joins. A warehouse needs that, and it makes the layer
the wrong tool for a chat agent. Such a layer is mounted on no agent at all.

Nothing enforces that, by design. Enforcing it would need a list of layer names
recorded per customer, and this tool cannot know what a customer is building.
The gate reports a layer no Agent binds, as an observation; a layer bound to an
Agent that should not be is caught in review.

Where a system has no live database - a third-party SaaS reached only through an
ETL - a layer over the warehouse is the only option. Say so in the layer's
`instruction` and make the agent disclose the lag whenever recency matters.

## Verify

```bash
# every sampleQuery, against the real database, before committing
.venv/bin/python .agents/skills/db-query/scripts/query.py \
  --class <class> --prefix <PREFIX> -f query.sql

asgard-cli gate               # every local check, the lint step included
asgard-cli verify <project>   # or one step alone, while iterating
```

Do not run `helm lint` by hand: without the reserved `asgard` values file
that `gate` supplies, every chart that labels anything fails. `asgard-cli gate
--help` says why.

`asgard-cli verify` confirms `dataConnectorName` resolves and that the agents
reference only layers that exist. It does not validate SQL against the live
schema; the introspection queries do that.
