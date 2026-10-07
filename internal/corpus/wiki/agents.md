---
group: While building
description: Flow Agent against Managed Agent, how to choose, UI-to-CR names
---
# Flow Agent and Managed Agent

A Managed Agent is the configuration of one agent. A Flow Agent is an entry
point from outside, and it can mount Managed Agents.

| | Managed Agent | Flow Agent |
|---|---|---|
| what it is | one LLM conversational agent's configuration | the entry point that receives requests and coordinates Managed Agents |
| published to | the Agent Hub (Sindri) | it is the outward endpoint itself |
| who can reach it | callers that can authenticate | can be opened to anonymous visitors |
| can mount | MCP Server, Skillset, Drive, Semantic Model, Browser | Plugin, MCP Server, Skillset, Managed Agent |

A Flow Agent can mount one or more Managed Agents, so the two are used together
rather than as alternatives.

## Managed Agent

To create one:

- **Agent Name** (required)
- **Agent Alias** (required) - `aliasName` in the CR, whose pattern is
  `^[a-z0-9][a-z0-9\-]*$`: lowercase letters, digits and hyphens, starting with
  a letter or a digit. It is the name the orchestrator invokes the agent by
- **Description** (required) - what this agent can do. It is shown on the agent's
  profile, and it is what the orchestrator reads to decide whether to delegate
- **Prompt** (required), in four fields
  - Persona - who this agent is
  - Task - the list of things it does
  - Context - the background it needs while doing them
  - Format - the shape its replies follow
- **Profile Picture**, **On-boarding Settings** (optional) - sample questions and
  a custom menu. The sample questions are `sampleQuestions` in the CR; the
  picture and the menu have no field in it

Mountable resources: MCP Servers, Skillsets, Drives, Semantic Model, Browser
Configuration.

Five built-in templates can be applied directly: a simple online help desk,
Customer Support, Knowledge Base Q&A, Data Analyst, General Assistant.

Every Managed Agent is published to Sindri and can also be managed from the
Management Console. An enabled agent serves immediately; a disabled one stops
serving but keeps its configuration. Enabled and disabled are Console state: the
`Agent` CR has no field for either. There is no publish step and nothing to
import on the Sindri side. What makes an agent findable there is the
Description: the orchestrator reads it, with the sample questions appended, as
the "when to spawn" line above the agent's prompt. In a chart the
same holds: one `preset-agent-hub` per namespace serves every managed `Agent` CR,
and nothing in the `Agent` names the Hub.
[`setup-path.md`](../wiki/setup-path.md) is the order this sits in.

## Flow Agent

Creating one needs only a Name; the Description is optional. Everything else is
optional too, under Advanced Sandbox Settings: Plugins, MCP Servers, Skillsets, Managed Agents.

## Choosing between them

The deciding fact is whether the caller can authenticate.

| situation | what to use |
|---|---|
| internal users, all signed in | Managed Agents alone - users pick one in the Agent Hub |
| anonymous visitors, a single job | a Flow Agent alone, mounting no Managed Agent |
| anonymous visitors, several specialisms | a Flow Agent mounting several Managed Agents |

The middle row is the one most often got wrong. Mounting one Managed Agent for a
single-job widget makes the Flow Agent restate the question to that one agent and
restate the answer back, a round trip that adds nothing. One deployment
removed that layer after shipping it.

## Two hard limits

These cannot be built:

1. An anonymous caller cannot use the Agent Hub. The Hub requires a caller
   that can authenticate.
2. An outward entry point can only point at a Workflow, never at an Agent.
   `BotProvider.entrypoint` takes a `Workflow`.

So a public website cannot point straight at a Managed Agent, even though the
UI makes it look possible.

One project first designed a public website around the Agent Hub and later moved
it to a Flow Agent (TASK-007 to TASK-011). The deciding question is the caller's
ability to authenticate, not what other projects did.

## UI names against CR names

The product documentation describes objects in an interface; a chart declares
resources. They are not one to one.

| UI | chart |
|---|---|
| Managed Agent | one `Agent` CR (`agentClass: managed`) |
| Flow Agent | three CRs: `BotProvider` + `Workflow` + `SandboxBlueprint` |
| Agent Hub | `preset-agent-hub`, provisioned by the platform in every namespace |
| the Managed Agents a Flow Agent mounts | `SandboxBlueprint.spec.agents`, a JSON array written as a string |
| the Prompt's four fields | `Agent.spec.managed.prompt.{persona,task,context,format}` |

`agentClass` has only one value, `managed`, so the CRD's `Agent` is the UI's
Managed Agent and there is no other kind, although the name suggests a choice
between two. An entry in `SandboxBlueprint.spec.agents` either names an `Agent`
CR (`baseAgentName`) or defines a subagent inline (`aliasName`) with no `Agent`
CR behind it.

For how each shape is assembled: `../usecase/agent-hub.md` (an internal
hub), `../usecase/flow-agent-single.md` (anonymous, one job), `../usecase/flow-agent-supervisor.md`
(anonymous, several specialists), `../usecase/browser-operation.md` (giving one a browser).

## Sources

- [Managed Agent](https://docs.asgard-ai.com/docs/product-suite/odin/features/agent-hub-managed-agent)
  - asgard-docs `f00e0ee`
- [Flow Agent](https://docs.asgard-ai.com/docs/product-suite/odin/features/agent-hub-flow-agent)
  - asgard-docs `6261fdff`. What a new Flow Agent arrives with - a runnable
  five-node default flow - is in `../usecase/flow-agent-single.md` rather than
  here, because it is how one shape is assembled. What this page claims is that
  creating one needs only a Name, and everything else, Description included, is
  optional

**Checked:** the two limits, the name mapping and the Managed Agent's required
fields against asgard-kube `3da0365` `pkg/apis/asgard/v1alpha1/types.go`
(`BotProviderSpec.Entrypoint`, `AgentClass`, `ManagedAgentSpec`, `AgentPrompt`,
`SandboxBlueprintSpec.Agents`) and asgard-kube `crd/asgard-ai.com_agents.yaml`;
the Hub serving every managed `Agent` against
asgard-core `478cf5d6` `internal/bpoperator/reconciler/ns_reconciler.go`; the `Agent` shape against the
`Agent` CRs in unitech-e-asgard-kube `44e71a2`, finance-ai-asgard-kube
`d062197`, asgard-freyr-kube `8f6d6c1` and xxentria-asgard-kube `967407c`.

**Unchecked:** the Console side - the five templates, the profile picture and
custom menu, enabling and disabling, and the Flow Agent creation form - which
only a Console account can settle.
