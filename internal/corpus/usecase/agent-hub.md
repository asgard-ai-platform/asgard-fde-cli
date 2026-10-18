---
group: Entry points
description: every caller can authenticate; several areas of work
---
# Agent hub

Several Agents - one capability group per source system - reachable through the
platform's own entry point. You author no BotProvider.

**Seen in:** a deployment with several agents over several semantic layers, one
agent per source system.

**Checked:** against every Agent CR in a hub deployment (no BotProvider in that project, prompt.task byte-identical across every one) and the CRD. "One layer per Agent" is also held against a second chart set - a demo generator whose Agents are modelled per business role - where most of them mount more than one layer and most layers are bound by more than one Agent, all deliberately (counted off the rendered CRs of every chart in it). Against the contract: `Agent.spec.managed.semanticLayers` is an array with no `maxItems` (asgard-kube `cbd8d70`), and the platform's own rule list checks only that each name resolves. The same chart set is where the shared-prompt section's evidence comes from: most of its charts have Agents whose `task` and `format` differ in both fields, and the near-identical lines are, per chart, the lines present in every Agent but one.

Against asgard-core `478cf5d6` asgard-core `internal/bpoperator/reconciler/ns_reconciler.go` for what `preset-agent-hub` is (a `generic` BotProvider with `authMode: api-key`, whose SandboxBlueprint resolves `agent_hub.agent_names` to the turn's Agents), and against asgard-kube `1c07e20` (v1.1.9) `pkg/apis/asgard/v1alpha1/types.go` for `ManagedAgentSpec`, `AgentPrompt` and `SandboxBlueprint.agents`; and against the supervisor SandboxBlueprints of two production deployments, not named here, which list Agent CRs behind their own BotProvider. What the turn's agent is shown for each Agent, against asgard-core #368 - asgard-core `internal/processor/helper/clidriver_run.go` (`AgentCapabilitiesNote`) and asgard-core `internal/processor/driverloop/runtask.go`. That `agent_names` is every Agent the user may use in the Project, against asgard-agent-hub-api `b88b7d4` `internal/service/agent.go` (`ListAccessibleAgentNamesInProject`).

**Unchecked:** the split guidance - how many agents, and where the line between two of them goes - is judgement, and no deployment settles it either way.

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
      -> Agent  ag-<system>   a capability group; it deploys nothing
           semanticLayers[]   the read surface
           toolsetNames[]     any write path
           skillSetNames[]    domain skills

The caller selects agents per turn with `payload.agent_hub.agent_names[]`,
filled with the Agent CRs' `metadata.name`. The agent of that turn holds the
capabilities of every Agent selected and does the work itself; no Agent runs as
an assistant of its own. Sindri selects every Agent the user may use in the
Project.

For each selected Agent, that agent is shown one capability block: the
`description`, the `sampleQuestions`, what the Agent provides - its tools,
skills and semantic models, listed by the platform under the names the agent
calls them - and the four `prompt` sections. A user writing `@<aliasName>` in a
message marks that capability as the most relevant one; the agent still uses
everything it holds.

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
      <what this capability covers, in the customer's business nouns>
    prompt:
      persona: |-
        <who this agent is - per agent>
      task: |-
        <the operating rules for this area of work>
      context: |-
        <what it can reach - per agent>
      format: |-
        <how to answer, where this area needs its own form>
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

Each Agent mounts exactly one semantic layer, so one system's tables, tools and
guidance stay together. The turn's agent holds every selected Agent's layers at
once, so the split does not narrow what a turn can query. What it buys is that
the agent can tell which guidance goes with which system - the semantic models
are listed under the Agent that brings them - that a user can point at one
system with `@<aliasName>`, and that access is granted per Agent: a user who
may not use an Agent does not get its layers. An Agent that mounts several
layers still works; its guidance just no longer says which system a rule is
about.

This is advice, and nothing enforces it. The CRD takes a
plain array with no maximum, the platform's own rule list only checks that each
name resolves, and `asgard-cli verify` does not refuse it. On a 12-industry
demo chart set that models one agent per business role, the bindings that
share a layer are all deliberate. Roles share the systems they read
the way they do in a company: procurement, finance and production planning all
read the same ERP layer, and a management view reads several. A layer bound by
several agents is a normal shape. Whether an agent groups the systems you meant
is your judgement, not the tool's.

No `allowedCubes` on the binding. The platform honours it, but this
repository's standing decision is that an agent may query any table in its own
layer, and the restriction is which layer it mounts rather than which cubes
within it. `gate` R4 refuses an Agent that sets it. A public audience, which is the
audience that would want it, gets fixed query tools instead of a layer, and
which tables a layer can reach is set by the database user's grants.
See `../usecase/semantic-layer.md`.

Zero layers is legal when the agent's capability comes from toolsets instead.
Zero of both is not: an agent with no capability source at all is almost always
a layer deleted by accident, and the gate rejects it.

## Fields that are not obvious

`description` says what the capability covers. The turn's agent reads it, with
the Agent's `sampleQuestions` and the tools, skills and semantic models the
platform lists for it, to decide when that Agent's guidance applies; Sindri also
shows it on the agent's card. It carries the business nouns. Where two agents
look similar, each one's description says which side of the line it is on.

The boundary that gets confused is the one where two systems hold overlapping
data. "How much of this item is left" and "which bin is it in, and when does it
expire" sound like the same question and belong to different systems. Say so in
both descriptions.

Publishing is the on/off switch. Callers build `agent_names[]` from the
published agents, so `asgard-ai.com/agent-published` alone decides whether a
turn holds an agent's capabilities. Leaving it `"true"` and omitting
`sampleQuestions` does not hold an agent back; the gate rejects that
combination, and a published agent needs at least two sample questions.

`aliasName` is the handle a user writes as `@<aliasName>` to point at this
capability, `^[a-z0-9][a-z0-9-]*$`, conventionally the CR name without its
`ag-` prefix. Two references to the same agent are deduplicated on it.

No `completionModelName` on an Agent. The model is chosen per turn by the
caller (`agent_hub.completion_model_name`), not set in the CR.
`SemanticLayer.spec.completionModelName` is a different, still-required field.

`prompt` requires all four keys - `persona` / `task` / `context` / `format` -
but a section with nothing specific to say may be an empty string.
`aliasName` + `description` are required and non-empty.

## Designing the parts the generator leaves TODO

### How many agents

One per source system, not one per question or per department. The split
keeps one system's resources and guidance together, so the turn's agent can
tell which rules go with which tools.

A system nobody asks questions about does not need an agent. A question that
spans two systems does not need a third agent - the turn's agent holds both and
combines the answers.

### `description` - what the capability covers

Write what the area of work covers, not an instruction to hand work to
someone:

    涵蓋<業務範圍>:<查得到什麼、能做什麼>。<與相近能力的分界,若有>。

Put the customer's own nouns in it - the words they use for their orders, their
parts, their tickets. The agent recognises a request by those.

Do not name tools, skills or semantic models in it. The platform lists what each
Agent provides beside its description, under the names the agent calls them,
and a hand-written list goes stale when a tool is renamed. Typical requests go
in `sampleQuestions`, not in the description.

Do not write it as routing: no "when the user asks X, hand it to this agent",
no "send it to the right assistant". No other assistant receives the work; the
agent reading the description is the one doing it.

Where two agents look similar, each description says which side of the line it
is on. "How much stock is left" and "which shelf is it on, and when does it
expire" sound like one question and belong to different systems. If you cannot
state the boundary in one sentence per side, the split is wrong.

### The prompt, in four sections that do not overlap

| section | holds | per agent? |
|---|---|---|
| `persona` | who it is, and that it works through tools rather than from memory | yes |
| `task` | the operating rules for this area: when to reach for a tool, what to do when it cannot | yes - rules for every turn go elsewhere, see below |
| `context` | what this agent can reach, described by capability | yes |
| `format` | how to answer, and what never to say, where this area differs | yes, or empty - see below |

Keep it high level. Do not name CRs, tools, skills or columns in a prompt.
Say "query through the semantic model" and "act through the system's tools", so
that adding a cube or renaming a tool does not mean editing prose. A capability
the agent should have is bound with a SkillSet, not described in a prompt.

Write each section as guidance for this area of work, and only that. Do not
tell the agent to report back, hand work over, or send the user to another
assistant: the agent reading it holds every other Agent's capabilities too, so
a sentence saying something is outside this Agent's scope marks the edge of
this capability, not of what the agent can do. The next section covers rules
that every Agent would otherwise repeat.

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

## Rules for every turn, and the one thing no check can do for you

Every selected Agent's prompt is shown to the same agent in one turn. A block
copied into each Agent - the answer language, how to confirm a write, a login
order - is therefore read once per Agent, and two copies that have drifted
apart give the agent two different rules.

State a rule for every turn once. In the agent hub the place for it is the
Global Directory's `AGENTS.md`, which every Agent Hub conversation in the
Project reads, and a chart writes it with a Syncer (`../wiki/sindri.md`). A
Flow Agent states such rules once in its Workflow's prompt
(`../usecase/flow-agent-supervisor.md`).

A chart set that models one agent per business role may still write `task` and
`format` as a skeleton with the role's own substance inside: the same operating
principles in every agent, and the capability line in the middle saying what
this role may read and write. `asgard-cli verify` does not refuse that shape.
Keep the skeleton short, because it is repeated per Agent, and move what is
identical in every Agent to the Global Directory.

Nothing checks the copies that remain. A shared line edited in one agent and
not the others is invisible to the tool and to a reviewer reading one file.
Comparing only the lines every Agent shares does not help on that same chart
set: the drifts it reports are deliberate - a read-only role whose capability
line says "read" where the others say "read and write". No rule separates those
two, so keep the copies in step yourself:

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

Adding a semantic layer means adding an Agent for it, with its `description`
and prompt `context`. The platform lists the layer by name under that Agent,
but a name does not say what the system holds or when to use it; the
description and the context do, and nothing infers them.
