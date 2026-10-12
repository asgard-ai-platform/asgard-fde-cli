---
group: Entry points
description: "anonymous or credentialed audience, several areas of work - and the one edge that loses the first message of every new conversation"
---
# Supervisor with several Agents

A public entry point whose agent holds the capabilities of several Agents - one
capability group per area of work - and does the work itself.

**Seen in:** a commerce back-office, a manufacturing deployment, a finance one
and a shopping guide, each with its own set of Agents. The first three
share one graph; the shopping guide differs by a single edge, and that edge
costs it the first message of every new conversation - see below.

**Checked:** against every supervisor deployment and the CRD; the agents field is a stringified JSON array, not a YAML list. The conversation loop's graph against every reference deployment at its prod values - edge for edge identical but for one, whose single differing edge is recorded below. What that edge costs, against asgard-core `623ceb50` - `bp_controller.go` treats a `listen-message` processor as a request terminal, so a first contact routed into the wait point is finalized with nothing sent. What the agent is given about each Agent and which capabilities it holds, against asgard-core #368 - `internal/processor/helper/clidriver_run.go` (`AgentCapabilitiesNote`), `internal/processor/driverloop/runtask.go` and `internal/bpcontroller/server/sandbox_orchestration.go`.

**Unchecked:** whether a given split into Agents, and its descriptions, lead the agent to the right guidance. That shows only in conversations against a running deployment.

Read the platform side first: `../wiki/agents.md` -
what a Managed Agent and a Flow Agent each are, and which the audience decides. This page assumes you have.

## When this shape, and when not

Use it when one audience needs several areas of work, each with its own tools
and guidance, and the caller should not have to know which applies. No Agent
runs as an assistant of its own. The agent of the turn holds every Agent's
tools, skills and semantic layers and does the work itself. For each Agent it is
shown one capability block - the description, the sample questions, what that
Agent provides (its tools, skills and semantic models, listed by the platform),
and the prompt - so it knows which guidance goes with which tools
(asgard-core `internal/processor/helper/clidriver_run.go`, `AgentCapabilitiesNote`).

Do not use it for a single-purpose agent. With one area of work an Agent CR adds
nothing: put the prompt on the Workflow and the capabilities on the blueprint,
so the instructions for the one job are in one place.

The other alternative is the agent hub: no BotProvider at all, one `Agent` CR
per system, the caller passing `agent_hub.agent_names[]` per turn. That is the
right shape when every caller can authenticate to the platform. It is not
available to an anonymous caller, and `BotProvider.entrypoint` takes a
`Workflow`, never an `Agent`.

## The shape

    BotProvider  bp-<name>          public entry, authMode none or api-key
      -> Workflow  wf-<name>        the conversation loop
        -> SandboxBlueprint sbp-<name>
             skillSetNames          capabilities of the entry agent itself,
                                    to which every Agent's are added
             agents[]               -> Agent CRs, one per area of work
             hooks                  optional, see below

### What "the conversation loop" is

    asgard-cli add flowagent <name> --project <p> --supervisor

writes it, with the prompt left TODO and the Agents left to be added to the
blueprint. What follows is what it writes and why each edge is where it is.


It is the graph below, and every supervisor deployment but the shopping guide
has it edge for edge identical at their prod values:

    entry  ->  update-context

    update-context                  --success-->  stream-llm-completion-message
    stream-llm-completion-message   --success-->  listen-message
    stream-llm-completion-message   --failure-->  push-message
    listen-message                  --success-->  stream-llm-completion-message
    push-message                    --success-->  listen-message

It is a loop and it has no exit: `exits: []` is intentional.
Most Workflows across the reference deployments declare none - plenty of them
outside the demo generator, which adds many more of one shape. A run ends when
its terminal processor finishes; only a Trigger-driven Workflow, which has
somewhere to report to, tends to declare one.

Read the loop as: prime the context once, answer, then wait for the next turn.
`listen-message` is what makes it a conversation rather than a request - it
returns to the completion processor, and the completion processor returns to it.

The failure branch says something and stays in the loop. `failure` goes to
`push-message`, which goes back to `listen-message`, so a turn that failed does
not end the conversation. A branch that failed and one that answered must not
look the same to the caller, which is why it is a separate processor rather than
the same one.

One deployment differs by a single edge: a
shopping guide sends `update-context --success--> listen-message`, waiting
before it answers rather than answering first.

That edge costs the first message of every new conversation. Reaching a
`listen-message` processor is a terminal: the controller calls `finalizeRequest`
and then `commitChannel` and returns, so the run ends there. On a channel that
has never been seen, the first request walks `update-context`, arrives at the
wait point, and stops - nothing answers it. The channel is parked, and every
message after that resumes from the wait point into the agent, so a returning
user never sees it. The deployment that ships this records the same consequence
in its own chart comment; the mechanism is asgard-core `623ceb50`,
`bpcontroller/server/bp_controller.go`.

Answer first unless you intend to drop that message. `update-context --success-->
stream-llm-completion-message` is what the other deployments write, and it is
what `asgard-cli add flowagent` generates. A greeting that is silent exactly
once, on first contact, is the case `../wiki/green-and-doing-nothing.md`
describes: deployed, green, and dropping traffic without an error.

The two-processor query tool is a different shape; do not confuse it with
this one. Deployments have `update-context --success--> http-request`, with
the request's `success` and `failure` both going to `push-message`: one turn,
no waiting, and the failure path says so rather than being silent.
`../wiki/processors.md` has each type's outputs - and read the warning beside
them rather than the table alone: the declared list under-reports Failure
branches, and `http-request` is one it gets wrong, so a reader checking that
branch against it would delete the edge this paragraph is about.

Files group as one directory per supervisor:

    supervisor/<name>/bot_provider.yaml
    supervisor/<name>/workflow.yaml
    supervisor/<name>/sandbox_blueprint.yaml
    agent/<name>.yaml               one per area of work

## Generate it

    asgard-cli add flowagent <name> --toolset ts-<name>
    asgard-cli add agent <area> --layer sl-<name>   # one per area of work

That writes the structure below with the fields that fail silently already in
place. When a skeleton is copied by hand these get lost, and nothing reports
them missing: not helm lint, not CRD validation, not a server dry-run. The
generated file marks the judgement calls TODO, and the rest of this page covers
them.

## The skeleton

    templates/supervisor/<name>/bot_provider.yaml
    templates/supervisor/<name>/workflow.yaml
    templates/supervisor/<name>/sandbox_blueprint.yaml
    templates/agent/<name>.yaml           one per area of work

```yaml
apiVersion: asgard-ai.com/v1alpha1
kind: BotProvider
metadata:
  name: bp-<name>
  annotations:
    asgard-ai.com/bot-provider-name: "<display name>"
  labels:
    {{- include "<chart>.labels" . | nindent 4 }}
spec:
  botProviderClass: generic
  entrypoint:
    workflow: wf-<name>
    entry: entry-main
  maxUnsupervisedSteps: 30
  disabled: false
  debugMode: on-demand
  adminApiKey:
    valueFrom:
      secretKeyRef:
        name: {{ include "<chart>.appSecretName" . }}
        key: asgard_resource_api_key
  generic:
    authMode: api-key       # or none, for an anonymous audience
    apiKey:
      valueFrom:
        secretKeyRef:
          name: {{ include "<chart>.appSecretName" . }}
          key: asgard_resource_api_key
---
apiVersion: asgard-ai.com/v1alpha1
kind: SandboxBlueprint
metadata:
  name: sbp-<name>
  annotations:
    asgard-ai.com/sandbox-blueprint-name: "<display name>"
  labels:
    {{- include "<chart>.labels" . | nindent 4 }}
spec:
  skillSetNames:
    value: "sk-base,sk-<domain>"
  # A STRINGIFIED JSON ARRAY, not a YAML list. Every field on a
  # SandboxBlueprint is a ValueExprTemplate - an object taking value,
  # expression or template - so a bare YAML list here is rejected by the
  # apiserver. skillSetNames two lines up has the same shape for the same
  # reason; agents is the one people get wrong, because its contents look
  # like a list.
  agents:
    value: |-
      [{"baseAgentName":"ag-<area>"},
       {"baseAgentName":"ag-<other>"}]
```

Each entry names an ordinary `Agent` CR - see the agent-hub extract for its
skeleton. The difference is only how it is selected.

Each entry carries exactly one of `baseAgentName` or `aliasName`; giving
both, or neither, is an error raised while the blueprint is evaluated rather
than at apply time.

- `baseAgentName` references an existing Agent CR as the base, and the other
  fields on the entry are an override delta on top of it: `description` and the
  four `prompt` sections are appended to the base, `skillSetNames` and
  `toolsetNames` are added and deduped, `sourceSetMounts` and
  `semanticLayers` replace the base entry with the same mountPath or name,
  and `browser` overrides outright.
- `aliasName` defines an Agent with no Agent CR at all, and the same fields
  are then its whole definition rather than a delta.

Either way the resolved description must be non-empty.

A resolved agent can turn the browser on for the whole sandbox. `browser` is
OR-aggregated: a blueprint saying `enabled: "false"` does not hold if any agent
it resolves asks for one. The same aggregation applies to every Agent's
skillSets, toolsets and semanticLayers, which land on the entry agent - so
listing a capability on the blueprint as well is redundant rather than
required. Most supervisor deployments list skillSets and toolsets anyway, so
the entry agent's capability set reads without computing the aggregation;
one relies on the aggregation for semanticLayers and sets only
`semanticLayers.dataVisualization` on the supervisor's processor.

## Designing the split the generator leaves TODO

### How many Agents

One per area of responsibility a person would recognise, not one per system
and not one per tool. The user asks in their own terms; the Agents should match
those terms.

Two tests before adding one:

- Can you state, in one sentence, what it covers and what it does not? If two
  Agents need each other's names in their descriptions to be told apart, they
  are one Agent.
- Would a person in this business recognise it as a job? "Inventory" and
  "exceptions" are jobs. "The API-calling one" is not.

One area of work means no Agent CR at all - put the prompt on the workflow and
the capabilities on the blueprint.

### The entry agent's own prompt

It holds the rules every turn follows - the answer language, how to confirm a
write, a login order - and what to do when no Agent's area fits. State those
once here rather than in each Agent: every Agent's prompt is shown to the same
agent in one turn, so a block copied into each is read once per Agent.

It does not repeat the Agents' descriptions or prompts, and it does not need a
table of which Agent has which tools. The platform shows the agent each Agent's
description, sample questions, the tools, skills and semantic models it
provides, and its prompt. Nor does it need instructions for handing a request
to an Agent: there is nobody to hand it to.

Its capabilities cannot be kept minimal. The toolsets, skill sets and source
set mounts of every Agent are aggregated onto the entry agent's own sandbox
(asgard-core `internal/bpcontroller/server/sandbox_orchestration.go`), and it
calls all of them itself. A split buys guidance tied to the right tools; it
does not fence one Agent's tools off from another's.

### Each Agent's `description`

It says what the area covers, in business nouns, with an explicit boundary
where two look similar, and without tool names - the platform lists what the
Agent provides beside it. `../usecase/agent-hub.md` has the form.

### When to compute the list of Agents

A static list is right until the caller genuinely needs a different set of
Agents per conversation - per tenant, per user's permissions, per brand. Then
the `expression` form is worth its complexity. Do not start there.


### Publish the supervisor to the Agent Hub

`asgard-ai.com/agent-hub-published: "true"` on the BotProvider is what makes this
supervisor appear in the Hub's agent list, which is where an internal console
finds it. The supervisor deployments carry it.

The opposite case is a public widget, which must not carry it - see
`../usecase/flow-agent-single.md`. Copying a supervisor's BotProvider into a
public one is how that mistake has happened.

## Fields that are not obvious

### `agents` can be an expression, not a list

Every blueprint field is a `ValueExprTemplate`, and `../usecase/conventions.md`
has the three forms and the rule that exactly one of them may be set. The form
used here is `expression:` - JavaScript the platform evaluates
per turn, with the BotProvider's payload available as `prevPayload`.

A static list of Agents is the simple case. Computing it lets the caller shape
the list per conversation:

```yaml
  agents:
    expression: |-
      (() => {
        const addons = prevPayload.subagent_addons || {};
        const bases = [
          { baseAgentName: "ag-brand-manager", alias: "brand-manager" },
          ...
        ];
        return bases.map(b => {
          const addon = addons[b.alias] || {};
          const entry = { baseAgentName: b.baseAgentName };
          if (addon.prompt) entry.prompt = { persona: addon.prompt };
          if (Array.isArray(addon.skill_set_names) && addon.skill_set_names.length > 0) {
            entry.skillSetNames = addon.skill_set_names;
          }
          return entry;
        });
      })()
```

That is how a caller extends an Agent's persona or skills per conversation
without a CR change. The payload key is the caller's own contract with the
blueprint; the platform reads only what the expression returns. `skillSetNames` on the blueprint itself is a plain
comma-separated string in `value`.

### Each Agent is an ordinary `Agent` CR

`agentClass: managed`, `aliasName` (the handle a user writes as `@<aliasName>`,
`^[a-z0-9][a-z0-9-]*$`, conventionally the CR name without its `ag-` prefix),
and `description` - what the capability covers, not a self-introduction and not
an instruction to hand work over. The agent reads it, with any
`sampleQuestions` and the resources the platform lists for the Agent, to decide
when that Agent's guidance applies, so it carries the business nouns.

`prompt` requires all four of `persona` / `task` / `context` / `format`, but they
may be empty strings. Agents in one deployment use `persona` only and leave
the rest `""` with a comment saying why - the CRD requires the keys, not the
content.

### BotProvider auth is a choice, and it is the security boundary

    authMode: none      an anonymous public widget. There is no key a browser
                        could keep secret, so protection has to be on the
                        capability side: read-only chain, zero-parameter tools,
                        read-only mounts - across every Agent, because the
                        entry agent holds all of their tools.
    authMode: api-key   a caller that can hold a credential, sending the value
                        the skeleton reads from the release's own Secret as
                        its X-API-KEY.

`adminApiKey` is separate from visitor auth: it guards the admin API
(`/history`), and the skeleton reads it, like `generic.apiKey`, from
`asgard_resource_api_key` in the release's own Secret. With `authMode: api-key`
that value is what the caller sends, so it is generated and handed to the
caller; with `authMode: none` nothing outside the platform needs it and a
random value is the whole answer. `../usecase/conventions.md` has both, and why
it is never `preset-agent-hub`.

Also on the BotProvider: `maxUnsupervisedSteps` (30 in one deployment, and the
platform default) caps how many processor hand-offs one request may make -
`../wiki/integration.md` has what counts as a step - and `debugMode: on-demand`.

## Problems deployments have hit


### Sandbox hooks: anything derived from the turn goes in `user-prompt-submit`

A deployment that writes runtime config into the sandbox with a hook records
why the obvious event is wrong (a commerce back-office deployment, 2026-08-21):

> session-start hook 進 Sandbox CR spec,內容一變 generation +1 -> pod 對話中被
> 重建。user-prompt-submit 由 driver 每 turn 用當輪 payload 重新評估、走 task
> 交付、完全不進 CR spec.

A JWT carries `jti`/`iat`, so the string changes on every issue. Putting it in a
`session-start` hook meant the Sandbox spec changed every time, the generation
bumped, and the pod was rebuilt mid-conversation. `user-prompt-submit` is
evaluated per turn by the driver and never enters the CR spec, which also fixed
tokens going stale after 8 hours.

Two more details in that hook worth copying: `umask 077` so a file holding a
token is 600, and writing to a temp file then `mv` for an atomic replace - a
mid-run message otherwise lets a running tool read half a config file.

### One label you will see in older charts

`asgard-ai.com/bot-provider-type` is derived by the platform from
`spec.botProviderClass` (workflow-service #336), so new charts do not stamp it.
Older ones do, with a comment saying the front end breaks without it - that was
true before #336. Stamping it anyway is harmless; omitting it against a cluster
older than #336 is not, so check the cluster you deploy to before removing it
from a chart that has it.

## Verify

```bash
asgard-cli gate               # every local check, the lint step included
asgard-cli verify <project>   # or one step alone, while iterating
```

Never run `helm lint` by hand: without the reserved `asgard` values file
that `gate` supplies, every chart that labels anything fails. `asgard-cli gate
--help` says why.

The xref check follows the whole chain including `agents[].baseAgentName` parsed
out of the JSON the CRD stores it in - a typo there fails the blueprint's
evaluation on the first turn, and the chat errors instead of answering.

Two things no check catches, both needing a real conversation:

- an `agents` or `hooks` expression that evaluates to the wrong thing for a
  given payload. The syntax is checked, the logic is not
- a wrong `configs[].name` on a processor, since that field is a free-form string

For a hook, confirm what it produced rather than that it ran:

```bash
kubectl get -n <namespace> sandboxes.asgard-ai.com
# then, in a conversation, have the agent read the file the hook writes
```
