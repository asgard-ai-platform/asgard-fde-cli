---
group: While building
description: which CR shapes were read from a single deployment, or from none - the extracts that rest on a thin sample
---
# Which shapes rest on a thin sample

The extracts describe CR shapes. This says which of them were read from a single
deployment, or from none, because an extract written from one chart describes
that chart, and the extract itself does not say so.

Most kinds - `Workflow`, `BotProvider` and `SandboxBlueprint`, `SkillSet` with its
`SourceSet` and `Syncer`, `DataConnector`, `Toolset`, `Agent`, `SemanticLayer` -
were read from several independent deployments, and the extracts for them
compare more than one arrangement. The kinds below were not.

| kind | sample |
|---|---|
| **Trigger** | one chart, and one instance in it, in an internal-audience project |
| **Plugin** | one deployment, which bundles many - `../usecase/plugin.md` |
| **KnowledgeBase / Loader / Source** | one deployment, the platform's own |
| **Indexer** | none. A live CRD that no deployment read here configures. `../usecase/knowledge-drive.md` names it because the contract has it |

Treat anything this material says about `Indexer` as read off the schema.

## What that means for the extracts

`../usecase/trigger.md` is written from one Trigger in one chart. Every
rule in it about the cursor, the cold start and what a scheduled run may not do
is generalised from a single instance. It is written confidently, but its sample
is the thinnest of any extract.

The knowledge-base shapes come from one deployment too, so
`../usecase/knowledge-base.md` describes that deployment's arrangement of
`KnowledgeBase`, `Loader` and `Source`. That it is the platform's own deployment
rather than a customer's has two effects: it was written by the people who built
the CRs, and it does not reflect a customer's constraints.

`Plugin` has a different problem. One deployment uses the shape heavily and no
other uses it at all, so there is no second arrangement to compare against and
no way to tell which of that deployment's choices are the shape and which are
the deployment.

Agent is absent wherever the shape is a Flow Agent. An engagement that reaches
for an Agent CR because the material talks about Agents is choosing one of two
shapes without being told there are two - see `../usecase/agent-hub.md` against
`flow-agent`.

## What this does not say

It names kinds, not uses. Two charts declaring a `Toolset` may be using it in
ways that share nothing. A thin sample is a floor on what was seen, not a
measure of whether the shape was understood.

**Checked:** by rendering every chart of the deployments the extracts were
written from with `helm template`, against its own values plus the values the
platform injects (`.Values.asgard.*`), and grouping the rendered `kind:` lines
by deployment. Those deployments are not named here: this page ships to every
engagement, and shipped material names no customer, no tenant and no
deployment.

**Unchecked:** anything deployed since. Most of those deployments now generate
their charts with this tool, so a later rendering would count the tool's own
templates rather than an independent arrangement, and it is not repeated.
