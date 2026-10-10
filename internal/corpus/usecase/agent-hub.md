---
group: Entry points
description: every caller can authenticate; several specialists
---
# Agent hub

Several specialist agents reachable through the platform's own entry point. You
author no BotProvider.

**Seen in:** a deployment with several agents over several semantic layers, one
agent per source system.

**Checked:** against every Agent CR in a hub deployment (no BotProvider in that project, prompt.task byte-identical across every one) and the CRD. "One layer per Agent" is also held against a second chart set - a demo generator whose Agents are modelled per business role - where most of them mount more than one layer and most layers are bound by more than one Agent, all deliberately (counted off the rendered CRs of every chart in it). Against the contract: `Agent.spec.managed.semanticLayers` is an array with no `maxItems` (asgard-kube `cbd8d70`), and the platform's own rule list checks only that each name resolves. The same chart set is where the shared-prompt section's evidence comes from: most of its charts have Agents whose `task` and `format` differ in both fields, and the near-identical lines are, per chart, the lines present in every Agent but one.

Against asgard-core `478cf5d6` asgard-core `internal/bpoperator/reconciler/ns_reconciler.go` for what `preset-agent-hub` is (a `generic` BotProvider with `authMode: api-key`, whose SandboxBlueprint turns `agent_hub.agent_names` into the subagent set), and against asgard-kube `3da0365` `pkg/apis/asgard/v1alpha1/types.go` for `ManagedAgentSpec`, `AgentPrompt` and `SandboxBlueprint.agents`; and against the supervisor SandboxBlueprints of two production deployments, not named here, which list Agent CRs behind their own BotProvider.

**Unchecked:** the delegation-design guidance - how many agents, and where the line between two of them goes - is judgement, and no deployment settles it either way.

Read the platform side first: `../wiki/agents.md` -
what a Managed Agent and a Flow Agent each are, and which the audience decides. This page assumes you have.

## When this shape, and when not

Use it when every caller can authenticate to the platform - an internal
console, another service, anything that holds a credential. It costs nothing to
run and saves a BotProvider per agent.

It is not available to an anonymous caller. `preset-agent-hub` is a `generic`
BotProvider with `authMode: api-key`, so a caller selecting agents with
`agent_names` has to hold its key, and `BotProvider.entrypoint` accepts a
`Workflow`, never an `Agent`. A public widget reaches Agent CRs only through a
BotProvider of its own whose SandboxBlueprint lists them in `agents` - the
supervisor shape in `../usecase/flow-agent-supervisor.md`.

## The shape

    preset-agent-hub          provisioned by the platform in every namespace
      -> Agent  ag-<system>   pure subagent config; it spawns nothing
           semanticLayers[]   the read surface
           toolsetNames[]     any write path
           skillSetNames[]    domain skills

The caller selects agents per turn with `payload.agent_hub.agent_names[]`,
filled with the Agent CRs' `metadata.name`.

## Generate it

    asgard-cli add agent <name> --layer sl-<name>

That writes the structure below with the fields that fail silently already in
place - the display annotation, the labels the UI needs, the current field names.
When the skeleton is copied by hand these get lost, and nothing reports them
missing: not helm lint, not CRD validation, not a server dry-run.

The generated file marks the judgement calls TODO. The rest of this page covers
them.

## The skeleton

`projects/<project>/chart/app/templates/agent/ag-<system>.yaml`, one file per CR.

```yaml
apiVersion: asgard-ai.com/v1alpha1
kind: Agent
metadata:
  name: ag-<system>
  annotations:
    # Required. Without it the CR applies cleanly and then shows up nameless in
    # the UI, which no lint or dry-run reveals.
    asgard-ai.com/agent-name: "<display name>"
  labels:
    # The on/off switch. Callers build agent_names[] from the published agents.
    asgard-ai.com/agent-published: "true"
    {{- include "<chart>.labels" . | nindent 4 }}
spec:
  agentClass: managed
  managed:
    aliasName: <system>
    description: |-
      <when to delegate here, in the customer's business nouns>
    prompt:
      persona: |-
        <who this agent is - per agent>
      task: |-
        <the operating rules - the shared block, copied into every agent>
      context: |-
        <what it can reach - per agent>
      format: |-
        <how to answer - the shared block, copied into every agent>
    sampleQuestions:
      - <at least two, required while published>
      - <...>
    skillSetNames:
      - sk-base
    semanticLayers:
      - name: sl-<system>
        allowQuery: true
        allowWrite: false
```

`toolsetNames` replaces or joins `semanticLayers` when the capability is a tool
rather than a read surface.

## One system, one agent

Each Agent mounts exactly one semantic layer, so its search space is that one
system's cubes rather than all of them combined. An agent mounting two has the
search space the split was meant to shrink.

This is advice, and nothing enforces it. The CRD takes a
plain array with no maximum, the platform's own rule list only checks that each
name resolves, and `asgard-cli verify` does not refuse it. On a 12-industry
demo chart set that models one agent per business role, the bindings that
share a layer are all deliberate. Roles share the systems they read
the way they do in a company: procurement, finance and production planning all
read the same ERP layer, and a management view reads several. A layer bound by
several agents is a normal shape. Whether an agent's search space is the one
you meant is your judgement, not the tool's.

No `allowedCubes` on the binding. The platform honours it, but this
repository's standing decision is that an agent may query any table in its own
layer, and the restriction is which layer it mounts rather than which cubes
within it. `gate` R4 refuses an Agent that sets it. A public audience, which is the
audience that would want it, gets fixed query tools instead of a layer.
See `../usecase/semantic-layer.md`.

Zero layers is legal when the agent's capability comes from toolsets instead.
Zero of both is not: an agent with no capability source at all is almost always
a layer deleted by accident, and the gate rejects it.

## Fields that are not obvious

`description` is routing text. The orchestrator sees it, with the Agent's
`sampleQuestions` appended, as the "when to spawn" line above that agent's full
prompt, and it does the work itself unless delegating helps - at
asgard-core `478cf5d6` `internal/processor/helper/clidriver_run.go` (`SubagentTeamNote`). The description is the line it matches a question against, so it
carries the business nouns. Where two agents look similar, each one's description says which
side of the line it is on.

The boundary that gets misrouted is the one where two systems hold overlapping
data. "How much of this item is left" and "which bin is it in, and when does it
expire" sound like the same question and belong to different systems. Say so in
both descriptions.

Publishing is the on/off switch. Callers build `agent_names[]` from the
published agents, so `asgard-ai.com/agent-published` alone decides whether an agent
gets delegated work. Leaving it `"true"` and omitting
`sampleQuestions` does not hold an agent back; the gate rejects that
combination, and a published agent needs at least two sample questions.

`aliasName` is the delegation name, `^[a-z0-9][a-z0-9-]*$`, conventionally the
CR name without its `ag-` prefix.

No `completionModelName` on an Agent. The orchestrator's model is chosen per
turn by the caller (`agent_hub.completion_model_name`), not set in the CR.
`SemanticLayer.spec.completionModelName` is a different, still-required field.

`prompt` requires all four of `persona` / `task` / `context` / `format`, and
`aliasName` + `description` are required too.

## Designing the parts the generator leaves TODO

### How many agents

One per source system, not one per question or per department. The
split keeps an agent's search space to one system's tables rather than all of
them combined, and the orchestrator picks between them per turn.

A system nobody asks questions about does not need an agent. A question that
spans two systems does not need a third agent - the orchestrator selects both and
reconciles the answers.

### `description` - the line the orchestrator matches on

Write it as an instruction to a router:

    當使用者的請求涉及<業務名詞>時,委派給<這個 agent>。
    它可以<能做什麼>。舉凡「<像這樣的問題>」的請求都應委派給它。

Put the customer's own nouns in it - the words they use for their orders, their
parts, their tickets. The orchestrator matches on those.

Where two agents look similar, each description says which side of the line it
is on. "How much stock is left" and "which shelf is it on, and when does it
expire" sound like one question and belong to different systems. If you cannot
state the boundary in one sentence per side, the split is wrong.

### The prompt, in four sections that do not overlap

| section | holds | per agent? |
|---|---|---|
| `persona` | who it is, and that it works through tools rather than from memory | yes |
| `task` | the operating rules: when to reach for a tool, what to do when it cannot | usually shared - see below |
| `context` | what this agent can reach, described by capability | yes |
| `format` | how to answer, and what never to say | usually shared - see below |

Keep it high level. Do not name CRs, tools, skills or columns in a prompt.
Say "query through the semantic model" and "act through the system's tools", so
that adding a cube or renaming a tool does not mean editing prose. A capability
the agent should have is bound with a SkillSet, not described in a prompt.

The two shared sections are duplicated because there is no include mechanism,
so a change to them is one global replace. The next section covers what that
costs and the other shape.

### `sampleQuestions` - two per agent, and they are demo openers

Each one has to trigger the multi-step reasoning on its own, without the agent
having to ask a follow-up for an id nobody knows:

- Anchor a real subject. Use something that exists in the data - a customer
  name, a part number - rather than "this order". "這張單" with no antecedent
  forces the agent to ask "which one?" and the demo stalls.
- Match how people refer to things. Business subjects in plain language
  (customer plus item); technical identifiers as the codes people actually say
  on the floor (a part number, a spec).
- Point at a decision, not a single number. "Which of these is at risk"
  pulls several steps; "what is the stock of X" pulls one.
- Keep each one self-contained, and inside what this agent can actually
  reach.

## The shared prompt text, and the one thing no check can do for you

The common shape is one `task` and one `format`, byte-identical in every Agent
of a chart, with everything per-agent in `persona` and `context`. Agent CRs
have no include mechanism, so shared prompt text can only be duplicated. While
the copies are identical, a later change is one substitution and a diff
shows it landed in all of them.

The other shape is also legitimate, and a chart set that models one agent per
business role tends to reach it. There, `task` and `format` are a shared
skeleton with the role's own substance inside them: the same four operating
principles in every agent, and the capability line in the middle of them saying
what this role may read and write. `format` goes the same way - lines 1 and 4
identical everywhere, lines 2 and 3 saying what this role leads with.

`asgard-cli verify` does not refuse that shape. A byte-identical rule fails on
charts of one such set that are correct, and nothing short of redesigning
every prompt clears it.

Nothing checks this. A shared line edited in
one agent and not the others is invisible to the tool and to a reviewer
reading one file. Comparing only the lines every Agent shares does not help on that
same chart set: the drifts it reports are
deliberate - a read-only role whose capability line says "read" where the
others say "read and write". No rule separates those two, so keep the copies
in step yourself:

- copy a block whole, and when you change it, change every copy in the same
  edit.
- read a diff before committing a prompt change across
  several agents - `git diff` over the agent templates is the whole check.
- if a section is genuinely per-role, put it in `persona` or `context`,
  where nobody later reads it as a copy that drifted.

## Verify

```bash
asgard-cli gate               # every local check, the lint step included
asgard-cli verify <project>   # or one step alone, while iterating
```

Do not run `helm lint` by hand: without the reserved `asgard` values file
that `gate` supplies, every chart that labels anything fails. `asgard-cli gate
--help` says why.

The last one catches this shape's specific mistakes: an agent with no
capability source at all, `allowedCubes` on a binding, one agent listing the
same layer twice, and a published agent with fewer than two sample questions.
It does not check prompt text - see the section above.

## Adding a system

Adding a semantic layer means adding an Agent for it, with its routing
`description` and prompt `context`. The CR binding alone tells neither the
orchestrator nor the agent that the capability exists; nothing infers it.
