---
group: While building
description: the modelling flow, its limits, the Mimir side
---
# Semantic Model

Connects a database, has AI understand its structure, and then answers in natural
language, producing charts and reports. Built in Odin; the finished model appears
in Data Insight (Mimir).

CRs: `SemanticLayer`, plus the `DataConnector` it points at.

## Building one

Three steps: Basic Information, Table Setting, Modeling.

### Basic Information

| field | required | |
|---|---|---|
| Name | yes | |
| Completion Model | yes | must support at least 60,000 Max Output Tokens |
| Effort | | reasoning depth, applied to every query; Default unless set |
| Language | yes | the language the AI answers in |
| Timezone | yes | |
| Data Source | yes | PostgreSQL, MySQL, MS SQL, Oracle, Trino, Athena |
| MCP Servers | | |
| Plugins | | |

The floor is not only a modelling limit. The platform builds both the layer's
modelling workflow and its chat workflow with `maxTokens` set to 60000, so a
model below it fails while modelling and again on every question afterwards.

Fewer data sources are listed here than the Data Source settings page supports -
it also has SAP Hana, SalesForce and NetSuite. Those three cannot be modelled:
their connectors in the platform do not implement table listing, which Table
Setting and the modelling assistant both depend on. The six listed are the six
that do. A Data Source on one of those three still serves queries elsewhere.

### Table Setting

The left pane lists every table under that Data Source across all schemas,
searchable and selectable, up to 100. The right pane previews the columns and
rows of whichever table is selected.

### Modeling

The AI assistant connects to the database and works through each table: reading
columns and types, sampling rows, checking which column combinations are unique
enough to be a primary key, then writing the Name, Description, Primary Key and
Columns.

On entry the panel offers three quick actions: Build all the tables, Start with
one table, Find the relationships. The left side holds Table semantics,
Relationships, Summary, Sample Questions, Rule and SQL Scripts, each with its own
Discuss button.

The documentation puts ten tables at about eight minutes.

Before adding relationships the assistant asks about the things the data cannot
settle - whether a column that currently holds one value will hold others later,
what two similar columns each mean - and continues once answered.

> Save is manual, bottom right. Switching or closing the tab without saving
> discards both the table selection and the built schema, and reopening starts
> from an empty Table Setting.

## The Mimir side

Mimir reads and presents a semantic model and does not edit it. Building and
publishing happen in Odin.

Mimir's Data Model button expands the current model's structure:

- Summary - the business area it covers
- Model Source - which semantic model this structure came from
- Table / Columns - names, types and descriptions
- Relationships - the columns joining this table to others
- Measures - predefined measures with their aggregation and source column. A
  question can name a measure directly instead of describing the arithmetic
- Preview Data - ten real rows

## Semantic Model or fixed query tools

For an anonymous public audience the answer is usually fixed zero-parameter query
tools rather than a Semantic Model, and `../usecase/fixed-query-tools.md` is that
shape. The argument turns on `allowedCubes` - the field that would narrow a mounted
layer, which sits on the binding rather than on the layer. Which binding
depends on the path, and a reader who knows only one path may conclude no
narrowing exists: on the Agent path it is the Agent's, and `verify` R4 refuses it there;
on the flow-agent path it is the `allowedCubes` of an entry in the completion
processor's `semanticLayers`, which `../wiki/processors.md` owns.
`../usecase/semantic-layer.md` advises against both, because the check refuses
correct queries, and points at the database user's grants instead.
Opening a write path is `../usecase/write-path.md`.

## Two join failures nothing local catches

A join that renders fine can match zero rows. Neither `helm lint` nor the
CRD can see that the two sides hold the same code in different *formats*; only
running it can. Both cases found in one deployment returned empty sets silently
rather than erroring:

    a single-character code  against  a four-character one   0 rows
    a 12-char zero-padded id against  the 10-char form       0 rows

Both were fixed with a derived dimension - `substr(...)`, `regexp_replace(...)` -
rather than by changing either table. And in the same layer, two similar-looking
tables disagreed: one pair needed the prefix stripped and another carried the
full code and must not be touched.

Run `count(*)` on every new join against the live database before committing it.
A cardinality check catches fan-out but does not catch a zero-row join.

`joins[].relationship` has no `many_to_many`. The enum is `one_to_one`,
`one_to_many`, `many_to_one`. Do not force a many-to-many into one of them:
the agent joins detail rows and fans out, and the measured inflation in one case
was 18.2x and 5.6x on the same layer's sums. A genuine many-to-many is left
undeclared, and the correct pre-aggregation goes in `sampleQueries` and the
layer's `instruction` instead.

Nothing errors when this happens, and the customer reads the inflated number as
an answer.

## Sources

- [Semantic Model](https://docs.asgard-ai.com/docs/product-suite/odin/features/data-insight-semantic-model)
  - asgard-docs `6261fdff`
- [Data Model](https://docs.asgard-ai.com/docs/product-suite/mimir/features/data-model)
  - asgard-docs `21c920f6`
- `completionModelName` being required: checked against every real
  `SemanticLayer` CR in three deployments
- The six-versus-nine data source difference: compared against the
  [Data Source](https://docs.asgard-ai.com/docs/product-suite/odin/features/settings/data-source)
  page at asgard-docs `6261fdff`, and explained by which connectors implement `ListTables`

**Checked:** against asgard-docs `6261fdff` for both pages above; against
asgard-kube `3da0365` `pkg/apis/asgard/v1alpha1/types.go` for `SemanticLayerSpec`
(`completionModelName` required, `effort` optional with no default) and the
`JoinRelationship` enum; against asgard-core `478cf5d6`
asgard-core `internal/bpoperator/reconciler/sl_reconciler.go` for the 60000
`maxTokens` on the modelling and chat workflows, and
asgard-core `internal/bpcontroller/dataconnector/` for which connector classes
implement `ListTables`; `completionModelName` against every real `SemanticLayer` CR in
three deployments.

**Unchecked:** the UI steps, the 100-table limit and the Mimir Data Model panel
come from the product documentation, and nobody here has walked them in a
Console account.
