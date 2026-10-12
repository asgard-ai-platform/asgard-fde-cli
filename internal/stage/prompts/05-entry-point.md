---
description: the agent hub or your own BotProvider, why a public widget cannot have the hub, the project that needs no entry point at all
---
# Decide each project's entry point

This stage applies to a project with a read path and no way to reach it. It is
the second decision that is often answered wrong.

Needing an entry point:
<<range .Projects>><<if .NeedsEntryPoint>>  - <<.Slug>>
<<end>><<end>>
## First: does this project need one at all?

A chart can be finished at the read path. If what the customer wants is to
watch the same numbers each morning, the consumer is Data Insight (Mimir): they
explore the model by conversation and save Views and Dashboards in the product,
and no Agent, Toolset, entry point or BotProvider is written at all.

Nothing on disk can tell that apart from an agent nobody has written yet, so
write it in a comment in the chart, next to the layer. There is no field for it,
because no tool can check the claim.

Write that comment only when the answer to "who asks it a question" is nobody;
it is not a way to skip this stage. Without the comment, a later reader who finds a
SemanticLayer that nothing references will read it as a missed connection and
bind an Agent to it, which hands agents deliberately restricted to an API a
second path into the database. No gate catches that: cross-reference
checking validates references that exist, never one that should not.

    ../guide/read-path.md
    ../usecase/mimir-dashboard.md

## The decision

| audience | answer |
|---|---|
| a caller that can authenticate to the platform | the per-namespace preset-agent-hub. You author no BotProvider - one Agent CR per system is the whole entry surface |
| an anonymous visitor | your own BotProvider -> Workflow -> SandboxBlueprint |

## Why the agent hub cannot serve a public widget

The constraint is a field on the CRD and it is not this page's to state:
`../wiki/agents.md` has what `BotProvider.entrypoint` takes, and
`../usecase/agent-hub.md` has what that means for the shape. It cannot be
configured around: a public audience forces the second row of the table above.

> Answered wrong once: a public widget was specced onto the agent hub, then
> re-decided as a self-hosted chain after the constraint surfaced. The
> constraint was documented; ask the audience question before committing to an
> entry point.

Read the shape before writing it:

    ../usecase/agent-hub.md
    ../usecase/flow-agent-single.md
    ../usecase/flow-agent-supervisor.md

If the audience already lives on LINE, Telegram, Discord or Slack, that decides
it: those platforms reach a BotProvider and nothing else, so the agent hub is
out. One field on the CR, and it is immutable after creation:

    ../usecase/chat-channel.md

## If it is the agent hub

The Agent CR is a capability group and deploys nothing: the agent of each turn
holds the capabilities of every selected Agent and does the work itself. Its
`description` says what the capability covers, not who the agent is. The agent
reads it, with the sample questions and the tools, skills and semantic models
the platform lists for that Agent, to decide when that Agent's guidance
applies, so put the business nouns in it and leave the tool names out. Where
two agents look similar, say in each which side of the line it is on.

Publishing is the on/off switch: callers build agent_names[] from the *published*
agents, so the agent-published label decides whether an agent gets work.

A published agent needs at least two sampleQuestions. That is our rule, not the
platform's: the CRD's `sampleQuestions` has no minimum, so
nothing on the cluster refuses one without them and `asgard-cli verify` does
(R7). Knowing which of the two refuses it tells you whether the fix is a chart
edit or a conversation. `../usecase/agent-hub.md` has why two.

Every selected Agent's prompt is shown to the same agent in one turn, so a block
copied into each Agent is repeated once per Agent. Write in each Agent only what
is specific to its area of work, and put rules for every turn in the Global
Directory's `AGENTS.md`. `../usecase/agent-hub.md` has the split.

## If it is a self-hosted chain

    visitor -> BotProvider (public) -> Workflow -> SandboxBlueprint -> capabilities

The capabilities live on the SandboxBlueprint, one CR further out than you
would expect. Searching the Workflow CRD for the link finds nothing; do not
conclude from that that the link does not exist.

A single public agent needs no Agent CR. Put the prompt on the Workflow's
processor and the capabilities on the blueprint, so the instructions for the
one job are in one place.

The endpoint takes the BotProvider's own key (`generic.authMode: api-key`) or
nothing at all (`none`, the anonymous shape), and a key in front-end JavaScript
is not a secret either way. There is no per-visitor identity. The protection is on the capability side: the whole chain read-only,
zero-parameter tools so no user input reaches SQL, mounts readOnly. That
protection is lost as soon as someone adds a parameterised or write-capable tool.

## Either way: the UI metadata

Every Workflow needs its full workflow-set label set, or it belongs to no set and
the UI has nothing to list, even though it runs correctly. A hand-written
Trigger needs two labels of its own or its editor opens as a blank canvas. See
AGENTS.md, "Workflow sets".

Done when: asgard-cli verify resolves the whole chain including the entry names.

**Checked:** against asgard-kube `cbd8d70`. `BotProvider.spec.entrypoint`
is `{entry, workflow}`, both required, with no agent field; `agentClass` carries
`self == oldSelf`; the
capability fields (`agents`, `skillSetNames`, `pluginNames`, `sourceSetMounts`,
`credentialMounts`, `hooks`) are on `SandboxBlueprint.spec` and not on
`Workflow`. Two claims that read as platform rules are ours and now say so:
the two-sampleQuestions minimum (the CRD sets none - `gate` R7 does). Shared
prompt text across per-role agents is nobody's rule: a chart set may interleave
per-role content into `task` and `format`, and `gate` does not check it.
Against asgard-kube `1c07e20` (v1.1.9): `ManagedAgentSpec` reads "The Agent CR
is a capability group ... It produces no deployment of its own and is not a
separate assistant: the agent of every turn that resolves it holds its
capabilities directly", and `description` "need not name the tools: the
platform lists, next to it, the tools, skills and semantic models this
capability brings".

Against asgard-core `478cf5d6`,
asgard-core `internal/edgeserver/middleware/bot_provider.go`: a generic BotProvider's routes
check its own key in `X-API-KEY` or `X-Asgard-Webhook-Token`, or nothing under
`authMode: none`, so the protection of a public endpoint can only be on the
capability side.

**Unchecked:** which audience forces which shape, and that a single public agent
needs no Agent CR, are one engagement's judgement rather than a platform
contract.
