---
group: The mechanism, and the scale
description: building something that looks like a prospect's business when you have none of their data
---
# A demo, and the pipeline that generates one

Building something that looks like a prospect's business when you have none of
their systems, none of their data and no credentials - and having it be a real
deployment rather than a slide.

**Seen in:** a generator carrying an industry per demo, not all of them with a
complete chart. It is the largest body of Asgard chart material there is,
and every CR in it is produced from one source shape repeated per industry.

**Checked:** against that repository at `1106771` - `.asgard-pipeline.yaml`,
`docs/spec/asgard/delivery.md`, the frontmatter of a retail story, its
consistency checker's code, and the CR kinds each industry's chart declares.

**Unchecked:** whether a demo built this way has ever converted into an
engagement, and what the second one costs once the first industry exists. The
repository records neither. The consistency checker at `1106771` still looks
for industries at the repository root and so finds none under `projects/`;
what it enforces is read from its code, not from a clean run.

Read the platform side first: `../wiki/product-suite.md` -
which product a request lands in. This page assumes you have.

## When this shape, and when not

Use it when the other side is a prospect rather than a customer. No access,
no credentials, no network path, and a date. Everything the interview asks for -
who is on the other end, which systems, who issues the account - has no answer
yet and asking produces a meeting rather than a demo.

Do not use it for a customer who has systems. The MVP rule holds: a first
delivery runs against their real channel with their real documents, and a demo on
invented data proves nothing they are measuring. This shape is what you build
before that conversation, so that it happens.

The difference: this deployment is complete and invented, where an MVP is
partial and real. Both are legitimate; do not present one as the other.

## The pipeline

    stories/*.md          the story, and its frontmatter is the design
      + attachments/      letters, drawings, notices - the unstructured sources
        |
    systems/db/*.sql      the data, and the SQL is the source of truth
        |  applied to Postgres, one schema per system
    skills/<name>/SKILL.md
        |
        v   one transform
    projects/<industry>/chart/app   a Helm chart of asgard-ai.com CRs
        |
        v   Platform Pipeline, one release per industry, on a tag
    the namespace of the Platform Project the release is bound to

The story is the entry point and its frontmatter is the whole design. Before
a CR exists, one file declares what the demo is made of:

    id, title, trigger
    agents:  [ag-store-ops, ag-allocation, ag-replenishment, ...]
    systems: [pos, wms, erp, eshop, supplier, crm]
    skills:  [stockout-detect, demand-forecast, transfer-optimize, ...]

That is the same decision the interview reaches by asking - which systems, what
capability, who acts - written down first because there is nobody to ask.

## Two scenarios per industry, and they are the two halves

Every industry carries a flagship and an insight, and the split follows the
read/write divide:

| | flagship | insight |
|---|---|---|
| retail | a stockout becomes a cross-store transfer | clearing slow-moving stock |
| semiconductor | an ECO hot lot pushed in | WAT drift, root-caused |
| finance | a large redemption's liquidity | portfolio concentration |

The flagship ends at an approval gate - a write, a person, a decision. The
insight ends at an analysis - no write, and it is the Mimir half of the same
data. A demo showing only the flagship looks like automation nobody controls; one
showing only the insight looks like a report.

`../usecase/write-path.md` is the flagship's mechanism and
`../usecase/mimir-dashboard.md` is the insight's.

## The read/write rule

    read   -> SemanticLayer, and the agent decides for itself
    write  -> Toolset + Workflow, and a person approves

Stated once, at the top, and every industry follows it. Anything SELECT can do
goes through the layer rather than becoming a tool. A demo that wires a query
as a tool has spent effort producing something narrower than the layer already
offered, and the result is a tool list too long to keep track of.

## Nothing exists that no story uses

The consistency checker enforces referential integrity in both directions, and
the second direction is the one worth copying:

  - every skill, system and attachment a story names must exist
  - every skill, system and attachment that exists must be named by a story

An orphan is an error, not a warning. That is what keeps twelve industries from
accumulating half-built assets nobody can date, and it is close to the check a
customer repository does not have. `asgard-cli check` resolves references that
exist and says nothing about a thing nothing references. `asgard-cli verify`
has one such rule - R11, a SemanticLayer no Agent binds - and it is a
warning, because a render cannot tell a deliberate choice from unfinished work.
No check covers the other kinds.

It runs across the whole repository and has to be clean, not clean for the
industry you touched.

## Fields that are not obvious

One release per industry, each bound to its own Platform Project, so the
namespace belongs to the Project rather than to the chart. The platform injects
the environment id, the Secret and ConfigMap names and the chart's appVersion
on every run, and the chart uses `required` rather than `default` for them,
because a default name renders, applies and resolves to nothing at runtime.
`.asgard-pipeline.yaml` declares the keys each release takes and never their
values; the values are set on the platform.

The pipeline does not touch the database. The repository's own CI applies the
SQL to Postgres on every tag, because a demo with no data is empty.

Each industry declares its own pair of `CompletionModel` CRs, and nothing in
any chart names either of them. Every `completionModelName` across the
generator resolves to `preset-balanced`, and the one industry that has dropped
the value entirely still declares the pair - so the CRs are inert, and copying
the block out of here obtains a provider key for a model no layer and no agent
will use. `../wiki/settings.md` has that finding, the class enum, and the two
rules the CRD enforces that helm does not.

The SQL is the source of truth, not the database. Data is applied from files
per industry, so a demo can be rebuilt from the repository after somebody has
changed it through the UI.


## Read the platform side first

`../wiki/product-suite.md` for which product a scenario belongs to;
`../usecase/write-path.md` for the approval gate;
`../usecase/mimir-dashboard.md` for the insight half.
