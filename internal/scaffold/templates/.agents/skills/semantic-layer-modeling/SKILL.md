---
name: semantic-layer-modeling
description: Use when modeling or extending a SemanticLayer CR — turning a schema you have already introspected into cubes / dimensions / measures / joins / sampleQueries, and the conventions that make a layer legible to the agent that queries it. Connecting to a database and reading its schema is the db-query skill; this one starts from what that found.
version: 1.1.0
alwaysApply: false
---

# SemanticLayer Modeling (design time)

This repo's job is to define the Asgard CRs that let the customer build Agentic applications.
The highest-value, highest-effort artifact in that set is the `SemanticLayer` CR —
the read surface the agent composes SQL against. Model it from the real database, not from
guesses.

This skill covers turning what an introspection found into a `SemanticLayer` CR: what a cube
is worth being, which joins earn a declaration, and the conventions that decide whether the
agent can actually use the layer. Reaching the database is the `db-query` skill - load that
first, come back with the schema.

> Design time, not runtime. This skill is for the coding agent working in this repo.
> It is never synced into the platform. Skills the deployed agent uses at runtime live in
> `assets/skills/<skill>/SKILL.md` and are bound via a `SkillSet` CR — a separate
> mechanism. Do not confuse the two.

## When To Use

- Adding or extending a `SemanticLayer` CR (new cube, dimension, measure, join, `sampleQueries`).
- Adding a `DataConnector` for a database not yet wired up.
- Verifying that a cube's `sqlTable` / dimension `sql` actually resolves against the live schema.
- Diagnosing an agent answer that looks wrong and might be a modeling problem rather than a prompt
  problem.

## When To Skip

- Pure chart plumbing that does not touch a `SemanticLayer` or `DataConnector`.
- Prompt / skill / `.asgard-pipeline.yaml` changes.

## Connecting, and looking at the data

Connection and introspection are covered by the `db-query` skill, not this one.
Load it and come back with what you found:

    .agents/skills/db-query/SKILL.md

It covers the readable `DataConnector` classes, how a connection is
configured in `.env`, how to get a credential without asking anybody to type a
password into a chat, and the recipes for listing tables, columns, primary keys
and foreign keys on each engine. It is read-only and enforces that.

The one-line version:

```bash
Q() { .venv/bin/python .agents/skills/db-query/scripts/query.py "$@"; }
Q --class postgres --prefix UOF_DB_ --columns sales.orders
Q --class postgres --prefix UOF_DB_ "select status, count(*) from sales.orders group by 1"
```

A function, not `Q="..."`: zsh does not word-split an unquoted `$Q`, so the
variable form is one word and exits 127 with nothing to say why.

Every database this repository models should be reachable that way before any
cube is written. Write cubes from the schema, not from a document.

### The map from a system to its CRs

Keep this table current as each system is wired up. The next reader opens it
first, and it is the only place that lists the three names for one system together.

| system | class | `.env` prefix | `DataConnector` | `SemanticLayer` |
|---|---|---|---|---|
| | | | | |

Adding a system means filling the row end to end: the `.env` group so design
time can reach it, the `DataConnector` CR, and the layer built on top. The
non-password coordinates are declared as `chartValues` in
`.asgard-pipeline.yaml` and their values set on the platform per release.

## Writing the `SemanticLayer` CR

Live under `projects/<project>/chart/app/templates/semantic_layer/sl-<system>.yaml`. Shape (see
existing files for full examples):

```yaml
spec:
  completionModelName: preset-balanced   # REQUIRED on SemanticLayer (unlike Agent)
  effort: medium                         # optional; omitting it is NOT "medium" — it
                                         # means the LLM processors send no effort at
                                         # all and the model's own default applies.
                                         # In this chart: {{ .Values.defaultSemanticLayerEffort | quote }}
                                         # low|medium|high|xhigh|max|auto|disabled
  dataConnectorName: dc-<system>
  locale: zh-TW
  timezone: Asia/Taipei
  instruction: |-                        # optional to the CRD, and the highest-value
    <rules the column names do not>      # field in the CR. Every rule here is one
    <carry: which column is the real>    # that returns a wrong answer or zero rows
    <key, which two "level" columns>     # if it is broken, and none of them is
    <are unrelated, what a status>       # visible in a schema dump. See
    <code means>                         # `../asgard-platform/usecase/semantic-layer.md`.
  sampleQuestions:                       # optional; plain strings, NOT objects.
    - <一句這個層真的答得出來的問題>       # Data Insight renders each as a button
    - <再一句,換一個主題>                  # under the prompt box and sends it when
                                         # clicked. Empty means a blank page.
                                         # NOT the same field as the Agent CRD's
                                         # `managed.sampleQuestions`, whose rules
                                         # and gate minimum do not transfer.
  cubes:
    - name: <schema>.<table>             # convention: fully-qualified, matches sqlTable
      sqlTable: <schema>.<table>
      title: <人類可讀的表名>            # 繁中
      description: <這張表記錄什麼,一句話>
      primaryKeyDimensions: [id]         # set it when the table has a PK
      dimensions:
        - name: <column>
          sql: '{CUBE}.<column>'
          title: <欄位的業務名稱>
          description: <這個欄位是什麼,以及它的值代表什麼>
          type: string                   # string | number | time | boolean
      measures:
        - name: <measure>
          sql: '{CUBE}.<column>'
          title: <這個彙總的業務名稱>
          description: <它加總的是什麼>
          type: sum                      # sum | count | avg | min | max ...
  joins:
    - name: <from_cube>_<from_dim>_<to_cube>_<to_dim>   # flattened, dots -> underscores
      description: <兩張表為什麼關得起來,關鍵是哪個欄位>
      relationship: one_to_many          # one_to_one | one_to_many | many_to_one
      from: {cube: <schema>.<table_a>, dimensions: [<column>]}
      to:   {cube: <schema>.<table_b>, dimensions: [<column>]}
  sampleQueries:
    - comment: <這個查詢回答什麼問題,以及任何口徑上的但書>
      sql: |
        select ...
```

### Conventions that are easy to get wrong

- Write every description to `.agents/skills/plain-chinese/SKILL.md` - a model reads
  them to choose a column, so a word like 至關重要 there can produce a wrong answer.
- `description` on every cube / dimension / measure, in 繁體中文. The agent reads it to decide
  which column answers a question, and the CRD refuses a cube, dimension or measure without
  one.
- Common analysis views go in top-level `sampleQueries[]` (`comment` + `sql`), never as a
  cube-level `sql:` virtual cube.
- Run every `sampleQuery` against the live DB before committing it. A `sampleQuery` that errors
  or returns nonsense misleads the agent. Record the row count you observed in the
  `comment` if it helps set expectations.
- Decide `effort` and the chart's model together. `disabled` is the value for a
  layer whose `completionModelName` is not a reasoning model - that pairing fails
  every turn rather than being ignored, including the turns that send no effort at
  all. `../asgard-platform/wiki/settings.md` has why. Settle the model before the
  layer, not after it.
- `measures` must be present on every cube; its contents are optional. The key is
  required by the CRD - `measures: []` is a declaration the apiserver accepts, omitting the
  key is rejected outright, and `helm lint` does not see the difference. So `measures: []` is the
  normal outcome when there is no real aggregate the business asks for; do not mechanically add
  `count` to every cube, and do not read "optional" as permission to leave the key out.
- `joins` is required the same way. A layer with no joins writes `joins: []`.
- A join's `cube` is a `cubes[].name` in the same layer and its `dimensions` are dimensions of
  that cube. Regenerating the cubes can leave a hand-written join naming neither, and the CRD
  accepts it; run `asgard-cli verify` after every regeneration. What it costs and what a
  dimension cap has to keep is in `../asgard-platform/usecase/semantic-layer.md`.
- Nothing in this repository narrows a layer once it is mounted on an Agent. `allowedCubes`
  is not a field on `SemanticLayer` - it exists on `Agent.spec.managed.semanticLayers[]`, and the
  platform honours it, but `gate` R4 refuses an Agent that sets it, on the standing decision that
  a bound layer is queryable in full. So every
  cube and every dimension you add widens the reach of whatever consumes the layer, with no
  second setting that takes it back. That is why this needs a spec (below).

  A Flow Agent can narrow. Where the layer is mounted on a
  processor rather than on an Agent CR, `semanticLayer.allowedCubes` is a config key on both
  LLM processors and on `query-database` as `allowedTables`, and setting it restricts what that
  processor may compose SQL over. So "the layer is queryable in full" applies to the
  Agent path, not to the platform. On the flow-agent path the narrowing exists, and it is
  the main reason to use that path with an anonymous audience. Both default to empty, which means
  unrestricted, so the widening still happens by default either way.
- `Agent.managed.semanticLayers[]` needs `allowQuery: true` and (today) `allowWrite: false`.

## Spec Gate

Per `docs/spec-driven-development.md`, a new `SemanticLayer`/`DataConnector`, or widening which
cubes the agent may query, requires SDD — the databases are real. Write the task spec under
`requirements/tasks/`, register it in `requirements/tasks/_index.md`, get it to `ready`, and wait
for explicit instruction before implementing.

Introspection and ad-hoc read queries are research and do not need a spec — they are how
you gather the "known context" a spec needs.

## Verify

After editing a `SemanticLayer`, run the repo's gate (the `asgard-cr-verification`
skill under `.agents/skills/` has what the platform then checks, and
`asgard-cli skill update` is what writes it, from the platform):

```bash
asgard-cli gate               # every local check, the lint step included
asgard-cli verify <project>   # or one step alone, while iterating
```

Never run `helm lint` by hand: without the reserved `asgard` values file that `gate` supplies,
every chart that labels anything fails. `asgard-cli gate --help` says why.

`asgard-cli verify` confirms `SemanticLayer.dataConnectorName` resolves to a real
`DataConnector` and that the `Agent` references only layers that exist. It does not validate
SQL against the live schema — that is what your introspection queries are for.

> Run the queries; reading the schema is not enough. One NetSuite layer's first Item Receipt
> draft was written from a spec document's SuiteQL and looked plausible. Running it
> found that `transaction.createdfrom` does not exist, that a missing `isinventoryaffecting = 'T'`
> filter makes the quantities sum to zero, and that over half the Item Receipts were transfer
> orders rather than purchases - so the number the agent would have reported answered a
> different question. None of that is visible in column names.

**Checked:** against asgard-kube `cbd8d70`. `SemanticLayer.spec`
requires `completionModelName`, `cubes`, `dataConnectorName`, `joins`, `locale`
and `timezone` (asgard-kube `3da0365`), and the Agent CRD has no
`completionModelName` at all - so the asymmetry this page warns about is real
and a layer written from an Agent's shape fails on a required field. A
dimension requires `description`, `name`, `sql`, `title` and `type`; a measure
requires the same less `sql`, which a CEL rule demands unless its type is
`count`; a cube requires `description` too. That makes the description a contract requirement rather than only a
convention here; that it is written in 繁體中文 is ours.

Against asgard-core `478cf5d6`, asgard-core `internal/constants.go`:
"nothing narrows a layer once it is mounted" is true of the Agent path only.
Both LLM processors declare `semanticLayer.allowedCubes` and `query-database`
declares `allowedTables`, so a Flow Agent can restrict what it composes SQL
over - and both default to empty, meaning unrestricted, which is why the
widening still happens by default. The `get_database_semantic_model` tool
hands the agent every `sampleQuery` unchanged and nothing runs one first
(asgard-core `internal/processor/domaintools/domaintools.go`).

**Unchecked:** the modelling itself - a cube per business entity over a cube per
table, which joins are worth declaring, what makes a `sampleQuery` useful - is
this engagement's practice against its customers' schemas, and no source states
it. Keep running every `sampleQuery` against the live database before committing
it even if you discount the rest.
