---
description: a semantic layer or fixed query tools, the layer nobody asks questions of, what zero-parameter does not mean
---
# Decide each project's read path

This stage applies to a project whose connectors exist and nothing reads through
them. It is the first of the three decisions that are often answered wrong.

Needing a read path:
<<range .Projects>><<if not (.Has "SemanticLayer" "Toolset")>>  - <<.Slug>>
<<end>><<end>>
## The decision

| audience | what they do with it | answer |
|---|---|---|
| internal, authenticated | ask, in the moment | SemanticLayer, one per source system, one Agent each |
| internal, authenticated | watch, every day | SemanticLayer and nothing else - see below |
| public, anonymous | ask, in the moment | a Toolset of zero-parameter fixed queries, no semantic layer |

The second row is a different product from the first, not a variant of it.

## When nobody is asking

A layer with no Agent on it can be a finished deliverable. If what the customer
wants is to see the same numbers each morning, the consumer is Data Insight
(Mimir): they explore the model by conversation and save what
is useful as Views and Dashboards, in the product, and no Agent, Toolset, entry
point or BotProvider is written at all.

    ask one thing, now        an agent. Everything below applies
    watch the same numbers    a layer, and the chart stops there
    both, different people    both - two deliveries over one model

When the chart stops there, say so in a comment in the chart, next to the
layer. There is no field for it, because this tool cannot check such a claim,
and a comment next to the layer is where the next reader will look.

`../usecase/mimir-dashboard.md` is the shape, including the trap: a later
reader finds a SemanticLayer nothing references, assumes it is a missed
connection, and binds it to an Agent - which hands agents deliberately restricted
to an API a second path into the database. Nothing in the gate catches that,
because cross-reference checking validates references that exist, never one that
should not. The comment next to the layer is what prevents it.

`../guide/requirements.md` asks this as question 2b, so the answer
should already be in the request. If it is not, it was not asked. Ask it before
building: the costly outcome is building the agent and finding out at the demo
that they wanted a dashboard.

## Why a semantic layer is wrong for a public audience

A bound layer lets the agent compose arbitrary SQL over every cube in it, and
the exposed surface grows every time a cube is added. Nobody goes back to
narrow it.

Narrowing it on the Agent is refused.
`allowedCubes` exists on the Agent's mount, and `asgard-cli verify` rejects any
Agent that sets it (R4), on the standing decision that a bound layer is
queryable in full. That is ours rather than the platform's: the CRD allows the
field. The reason to refuse it is that a per-agent allowlist makes the exposed
surface look bounded while the layer underneath keeps growing. Excluding the
sensitive tables does not fix it; the shape is the risk.

That is the Agent path, and a public audience is rarely on it. A flow agent
has no Agent CR, and there the `allowedCubes` of an entry in the completion
processor's `semanticLayers` does narrow, but it refuses correct queries too;
`../usecase/semantic-layer.md` says which binding is which, and that the
database user's grants are what limit the tables. The case for fixed tools is
the paragraph below: no user input reaches SQL at all.

With fixed tools, what can be asked is decided by a few statements in version
control, no user input reaches SQL so the injection surface is zero, and widening
it takes a CR change and a review.

> This was answered wrong once already: a public site got a SemanticLayer first,
> and it was later removed and replaced with five zero-parameter query tools.

## "Zero-parameter" means the model supplies nothing, not that everyone sees the same rows

The two are often confused, and the confusion leads to telling a customer that
something is impossible when it is possible.

An anonymous channel can answer "where is MY order". What it cannot do is let
the model choose whose case to look up. The customer's
identity is injected server-side on every turn - it arrives in the request the
front end sends, never as a tool argument the model fills in - and the query
filters on it. The tool still takes no parameters from the model, so the
injection surface is still zero, which is what the rule requires.

    the model         picks WHICH question         zero parameters
    the caller        supplies WHO is asking       every turn, server-side

Whatever sits in front - a website, a chat channel, a support desk - is what
knows who is talking and passes it through. If nothing in front knows, then the
per-customer half genuinely cannot be built, and that is a question for the
customer rather than a design to work around.

    ../usecase/per-turn-credentials.md

Read that before promising anything of the form "查我的...". It is also where the
failure mode lives: a query that forgets to filter on the injected identity fails
on the anonymous path only, which is the path nobody tests.

Read the shape before writing it:

    ../usecase/semantic-layer.md
    ../usecase/fixed-query-tools.md

A fixed query tool is a Workflow, so read how a chain passes values between its
processors before writing one; that is where failures go unreported:

    ../usecase/workflow-chain.md

## If the answer is a semantic layer

One system, one layer, one Agent. An Agent mounting two layers has the search
space the split was meant to shrink. This is advice with a reason; nothing
refuses it. Agents modelled per business role share the systems those
roles read, and `../usecase/agent-hub.md` says when that is the shape to build
instead.

  - Every cube, dimension and measure needs a description in 繁體中文. It is what
    the agent reads to decide which column answers a question; one without a
    description is refused by the CRD. Write these descriptions in plain
    Chinese - `.agents/skills/plain-chinese/` - because a word like 至關重要 in a
    description is noise the agent reads every time it chooses a column.
  - A dimension's name is the column its sql selects, never a re-cased alias.
  - Common analysis views go in top-level sampleQueries, never as a cube-level
    sql: virtual cube. Run every one against the live database before
    committing it; a sampleQuery that errors misleads the agent.

## If the answer is fixed tools

Every tool takes zero parameters, and requestConsent is false because they are
read-only.

Do not ship two tools that sit on the same FROM/JOIN and differ only in
projection. Each one's description then has to name the other as the
alternative, and a model often picks the wrong one. Merge them and make
the difference a column value instead of a tool choice.

Tool usage guidance goes in each tool's Workflow entries[].tooling.description.
Toolset.spec.instruction does not exist any more - adding it back passes
dry-run and then fails the real deploy.

A description is per tool and a skill is per subject, and the agent reads
both at once, so what is true across several of these tools goes in the skill
instead - `../wiki/tool-description-and-skill.md`.

Done when: every project reads through one shape or the other, and
asgard-cli check plus asgard-cli verify are green.

**Checked:** against asgard-kube `cbd8d70`.
`Toolset` declares no `instruction` field, so the note about adding it back is
current; `SemanticLayer.spec` carries `cubes` and top-level `sampleQueries`;
`allowedCubes` is on the Agent's semanticLayers mount rather than on the
layer, and the CRD permits it - refusing it is `gate` R4 and now says so.
`requestConsent` is a required field on each of a Toolset's tools, so
"false because they are read-only" is a value somebody writes rather than a
default they get, and tool guidance really is
`Workflow.spec.entries[].tooling.description`, which the CRD also requires.
The SemanticLayer CRD at asgard-kube `3da0365` requires `description` on every
cube, dimension and measure.

Against asgard-core `478cf5d6`: the `get_database_semantic_model` tool hands the
agent every cube with its descriptions and every sampleQuery unchanged, and the
platform runs no sampleQuery before handing it over:
asgard-core `internal/processor/domaintools/domaintools.go`,
asgard-core `internal/models/semantic.go`.

**Unchecked:** which audience gets a layer and which gets fixed tools, that
繁體中文 is the language the model matches best on, and how far an erroring
sampleQuery misleads a model are one engagement's judgement, with no source to
hold them against.
