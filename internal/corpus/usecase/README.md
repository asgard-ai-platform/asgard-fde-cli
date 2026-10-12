---
description: every deployment shape, grouped by the decision it belongs to - entry point, read path, write path, credentials
---
# The extracts

Extracts from Asgard deployments that are already in production, organised by the
shape an engagement needs rather than by CR kind. They answer "how do I write
this", and an agent reads them when it is about to author a chart.

These are shipped to every engagement, so they name no customer, no tenant and
no deployment. Where two deployments disagree they are "an earlier one" and "a
later one", and what matters is which is newer and why.

## How to read an extract

Each file answers four questions in this order:

1. **When this shape, and when not.** The decision, with the alternative and why
   it loses. Read this part before writing anything.
2. **The shape.** Which CRs, how they reference each other, in dependency order.
3. **The fields that are not obvious.** Only what a reader cannot infer from the
   CRD, and what a wrong value does.
4. **What it cost someone.** Dated incidents, so a reader can see the rule came
   from something that happened.

## Two warnings that apply to every extract

Deployments disagree, and the disagreements come from different generations of
the platform. The platform keeps changing what it derives for you, and a chart
that has not been touched since still carries the old workaround. Where an
extract notes a conflict it says which side is newer, and why. Do not resolve
one by picking the version you saw first.

The top-level layout is not settled either. Different deployments group by
`projects/<name>/`, by `tenants/<name>/`, by domain, or use a single chart with
no grouping at all. The CR shapes below are independent of that choice.

## The extracts

**Read first:**

| file | what |
|---|---|
| [`conventions.md`](../usecase/conventions.md) | where every CR goes, what it is called, what all of them need |


**Entry points** - pick by audience, never by preference:

| file | when |
|---|---|
| [`agent-hub.md`](../usecase/agent-hub.md) | every caller can authenticate; several areas of work |
| [`flow-agent-single.md`](../usecase/flow-agent-single.md) | anonymous audience, one job |
| [`flow-agent-supervisor.md`](../usecase/flow-agent-supervisor.md) | anonymous or credentialed audience, several areas of work - and the one edge that loses the first message of every new conversation |

**Read paths** - also decided by audience:

| file | when |
|---|---|
| [`semantic-layer.md`](../usecase/semantic-layer.md) | internal audience, open-ended questions |
| [`fixed-query-tools.md`](../usecase/fixed-query-tools.md) | public audience, a known set of questions |
| [`knowledge-drive.md`](../usecase/knowledge-drive.md) | knowledge that is documents, not rows |
| [`external-api.md`](../usecase/external-api.md) | a source that is an HTTP API, not a database - and the credential that decides whether the agent calls it from a Workflow or from its sandbox |
| [`browser-operation.md`](../usecase/browser-operation.md) | a system with no database and no API - only a web UI. The last resort |

**Write paths** - anything with a side effect:

| file | when |
|---|---|
| [`write-path.md`](../usecase/write-path.md) | the approval gate. Read this before designing any action that changes something |

**Capabilities and scheduling:**

| file | when |
|---|---|
| [`skill-set.md`](../usecase/skill-set.md) | getting skills to a deployed agent |
| [`plugin.md`](../usecase/plugin.md) | capability bundles chosen per request |
| [`trigger.md`](../usecase/trigger.md) | work on a schedule with nobody watching |

**Credentials** - who the call is made as:

| file | when |
|---|---|
| [`api-oauth.md`](../usecase/api-oauth.md) | an API that will not take a static key: the two-call token chain |
| [`per-turn-credentials.md`](../usecase/per-turn-credentials.md) | the agent calls as the person talking to it, on a short-lived token |

**More entry points and more read paths**, which did not exist when the groups
above were written:

| file | when |
|---|---|
| [`chat-channel.md`](../usecase/chat-channel.md) | LINE, Telegram, Discord or Slack instead of a web widget. No deployment runs this shape; read its provenance line first |
| [`knowledge-base.md`](../usecase/knowledge-base.md) | documents through a `KnowledgeBase` rather than a Drive - against [`knowledge-drive.md`](../usecase/knowledge-drive.md), which is the other half of the same decision |
| [`mimir-dashboard.md`](../usecase/mimir-dashboard.md) | a SemanticLayer whose consumer is Data Insight, with no Agent at all, and why adding an Agent breaks it |

**The mechanism, and the scale:**

| file | when |
|---|---|
| [`workflow-chain.md`](../usecase/workflow-chain.md) | a Workflow with more than one step: the processor types, and what crosses between them |
| [`skill-layers.md`](../usecase/skill-layers.md) | a runtime skill per area over one middleware and one external platform - what the layering looks like past the first skill |
| [`demo-generation.md`](../usecase/demo-generation.md) | building something that looks like a prospect's business when you have none of their data |

## Keeping this list complete

The rows above are generated from each extract's own frontmatter: `group:`
names the question it answers and `description:` is the row's text. Do not edit
a row here; change the extract's frontmatter. If an extract fits no group,
change the group list.

