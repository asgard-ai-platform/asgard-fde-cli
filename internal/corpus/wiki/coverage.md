---
group: While building
description: how many deployments each CR shape was read from - which extracts rest on a sample of one
---
# Which shapes each reference deployment actually uses

The extracts describe CR shapes. This says how many deployments each shape was
read from, because an extract written from one chart describes that chart, and
the extract itself does not say so.

Rendered with `helm template` against each chart's own values, then counted by
kind. The count is per deployment, not per chart, because the question is how
many independent sources a shape was read from: one deployment ships a chart per
industry that differ only by industry, and counting those as twelve would
overstate the sample.

There are seven deployments. The eighth reference repository, the Freyr skills, holds
runtime skills and no CRs at all - it is the only source for
`../usecase/browser-operation.md` and it contributes nothing here.

| kind | in how many | where |
|---|---|---|
| Workflow | 7 of 7 | everywhere |
| BotProvider / SandboxBlueprint | 6 of 7 | all but one, whose callers reach its agents through the platform's agent hub |
| SkillSet / SourceSet / Syncer | 6 of 7 | all but the minimal flow agent |
| DataConnector | 6 of 7 | all but one, which reaches its data through tools rather than a layer |
| Toolset | 5 of 7 | |
| Agent | 5 of 7 | absent from two, and both are Flow Agent shapes |
| SemanticLayer | 6 of 7 | |
| CompletionModel | 3 of 7 | the platform's own deployment, one customer, the demo generator |
| **Trigger** | **1 of 7** | one internal-audience project, and one instance of it |
| **Plugin** | **1 of 7** | one deployment, which bundles many - `../usecase/plugin.md` |
| **KnowledgeBase / Loader / Source** | **1 of 7** | auto-post only |
| **Indexer** | **0 of 7** | a live CRD in no reference deployment. `../usecase/knowledge-drive.md` names it because the contract has it; nothing here has seen one configured |

Workflow is the one kind every deployment has. BotProvider and SandboxBlueprint
are the entry point a deployment authors itself; the one without them uses the
platform's per-namespace agent hub, which nobody authors, with an Agent CR per
system - `../usecase/agent-hub.md`. The three at 1 of 7 are the thin samples, and the sections below are about them. `Indexer`
at 0 of 7 has no sample: the material names it because the CRD does, and nobody
here has seen one in a chart. Treat anything this material says about it as
read off the schema.

Every other kind in the rendered charts is named somewhere in this material,
which is the check in the other direction.

## What that means for the extracts

`../usecase/trigger.md` is written from one Trigger in one chart. Every
rule in it about the cursor, the cold start and what a scheduled run may not do
is generalised from a single instance. It is written confidently, but its sample
is the thinnest of any extract.

The knowledge-base shapes come from one deployment too. `KnowledgeBase`,
`Loader` and `Source` appear only in auto-post, so `../usecase/knowledge-base.md`
describes auto-post's arrangement of them. That it is the platform's own
deployment rather than a customer's has two effects: it was written by the
people who built the CRs, and it does not reflect a customer's constraints.

`Plugin` has a different problem. One deployment uses the
shape heavily and no other uses it at all, so there is no second arrangement to
compare against and no way to tell which of auto-post's choices are the shape
and which are auto-post.

Agent is absent wherever the shape is a Flow Agent, and those are the Flow Agent
projects. An engagement that reaches for an Agent CR because the material talks
about Agents is choosing one of two shapes without being told there are two -
see `../usecase/agent-hub.md` against `flow-agent`.

## What this does not say

It counts kinds, not uses. Two charts declaring a `Toolset` may be using it in
ways that share nothing, and this table would show 2 either way. It is a floor
on the sample size, not a measure of whether the shape was understood.

**Checked:** the counts are the whole of this page and they were measured, not
estimated - by rendering every chart in every reference
repository with `helm template` against its own values plus the values the
platform injects (`.Values.asgard.*`), 20 charts in all, then
grouping by deployment and counting `kind:`. The denominator is stated with
what it excludes, so the number can be re-derived: the table counts out of the deployments that declare CRs, and
one reference repository declares none. The table's denominator is
re-rendered and held against what is on disk in both directions, so a kind in a
chart with no row fails, and so does a row that counts out of the wrong total.

## Sources

Every chart rendered with `helm template` against its own
production values, then counted by `kind:`. They are not named here: this
page ships to every engagement, and shipped material names no customer, no
tenant and no deployment, the same rule the extracts follow. They are the
platform's own deployment, six customer or demo charts, and one industry demo
standing for the rest built from the same template - counting those would
inflate every row without adding a second arrangement.

Whoever maintains this tool can re-derive the list from the parent directory in
one command. Nobody reading it in an engagement needs it, because this page is
about the sample size, not whose sample it is.
