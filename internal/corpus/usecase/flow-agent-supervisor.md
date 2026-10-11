---
group: Entry points
description: "anonymous or credentialed audience, several specialists - and the one edge that loses the first message of every new conversation"
---
# Supervisor with subagents

A public entry point that delegates to several specialist agents.

**Seen in:** a commerce back-office, a manufacturing deployment, a finance one
and a shopping guide, each with its own roster of specialists. The first three
share one graph; the shopping guide differs by a single edge, and that edge
costs it the first message of every new conversation - see below.

**Checked:** against every supervisor deployment and the CRD; the agents field is a stringified JSON array, not a YAML list. The conversation loop's graph against every reference deployment at its prod values - edge for edge identical but for one, whose single differing edge is recorded below. What that edge costs, against asgard-core `623ceb50` - `bp_controller.go` treats a `listen-message` processor as a request terminal, so a first contact routed into the wait point is finalized with nothing sent. What the orchestrator is given about its subagents and which capabilities it holds, against asgard-core `478cf5d6` - `internal/processor/helper/clidriver_run.go`, `internal/processor/driverloop/runtask.go` and `internal/bpcontroller/server/sandbox_orchestration.go`.

**Unchecked:** whether a given split into subagents, and its routing prose, delegates well. That shows only in conversations against a running supervisor.

Read the platform side first: `../wiki/agents.md` -
what a Managed Agent and a Flow Agent each are, and which the audience decides. This page assumes you have.

## When this shape, and when not

Use it when one audience needs several specialists and the caller should not
have to know which one to ask. The supervisor is an orchestrator, but not a
router that always hands off: the runtime gives it every subagent's tools and
skills and tells it to do the work itself by default, spawning a subagent for
independent sub-tasks in parallel, to keep its own context lean, or when the
user asks for a specialist (asgard-core `internal/processor/helper/clidriver_run.go`,
`SubagentTeamNote`).

Do not use it for a single-purpose agent. A chat with one specialist has no
delegation decision to make, so the extra hop only adds a paraphrase step - the
orchestrator restating the question to the subagent, and restating the answer
back. One deployment removed exactly that hop from its public widget; the prompt
moved onto the Workflow and the capabilities onto the blueprint.

The other alternative is the agent hub: no BotProvider at all, one `Agent` CR
per system, the caller passing `agent_hub.agent_names[]` per turn. That is the
right shape when every caller can authenticate to the platform. It is not
available to an anonymous caller, and `BotProvider.entrypoint` takes a
`Workflow`, never an `Agent`.

## The shape

    BotProvider  bp-<name>          public entry, authMode none or api-key
      -> Workflow  wf-<name>        the conversation loop
        -> SandboxBlueprint sbp-<name>
             skillSetNames          capabilities of the supervisor itself,
                                    to which every subagent's are added
             agents[]               -> Agent CRs, the subagents
             hooks                  optional, see below

### What "the conversation loop" is

    asgard-cli add flowagent <name> --project <p> --supervisor

writes it, with the prompt left TODO and the subagents left to be added to the
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
    subagent/<name>.yaml            one per specialist

## Generate it

    asgard-cli add flowagent <name> --toolset ts-<name>
    asgard-cli add agent <specialist> --layer sl-<name>   # one per specialist

That writes the structure below with the fields that fail silently already in
place. When a skeleton is copied by hand these get lost, and nothing reports
them missing: not helm lint, not CRD validation, not a server dry-run. The
generated file marks the judgement calls TODO, and the rest of this page covers
them.

## The skeleton

    templates/supervisor/<name>/bot_provider.yaml
    templates/supervisor/<name>/workflow.yaml
    templates/supervisor/<name>/sandbox_blueprint.yaml
    templates/subagent/<name>.yaml        one per specialist

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
      [{"baseAgentName":"ag-<specialist>"},
       {"baseAgentName":"ag-<other>"}]
```

Each subagent is an ordinary `Agent` CR - see the agent-hub extract for its
skeleton. The difference is only how it is reached.

Each entry carries exactly one of `baseAgentName` or `aliasName`; giving
both, or neither, is an error raised while the blueprint is evaluated rather
than at apply time.

- `baseAgentName` references an existing Agent CR as the base, and the other
  fields on the entry are an override delta on top of it: `description` and the
  four `prompt` sections are appended to the base, `skillSetNames` and
  `toolsetNames` are added and deduped, `sourceSetMounts` and
  `semanticLayers` replace the base entry with the same mountPath or name,
  and `browser` overrides outright.
- `aliasName` defines an ephemeral subagent with no Agent CR at all, and the
  same fields are then its whole definition rather than a delta.

Either way the resolved description and the fused prompt must be non-empty.

A resolved agent can turn the browser on for the whole sandbox. `browser` is
OR-aggregated: a blueprint saying `enabled: "false"` does not hold if any agent
it resolves asks for one. The same aggregation applies to the subagents'
skillSets, toolsets and semanticLayers, which land on the main orchestrator too -
so listing a capability on the blueprint as well is redundant rather than
required. Most supervisor deployments list skillSets and toolsets anyway, so
the supervisor's own capability set reads without computing the aggregation;
one relies on the aggregation for semanticLayers and sets only
`semanticLayers.dataVisualization` on the supervisor's processor.

## Designing the split the generator leaves TODO

### How many subagents

One per area of responsibility a person would recognise, not one per system
and not one per tool. The user asks in their own terms; the roster should match
those terms.

Two tests before adding one:

- Can you state, in one sentence, what goes to it and what does not? If two
  subagents need each other's names in their descriptions to be told apart, they
  are one subagent.
- Would a person in this business recognise it as a job? "Inventory" and
  "exceptions" are jobs. "The API-calling one" is not.

One specialist means no subagent at all - put the prompt on the workflow and
the capabilities on the blueprint.

### The supervisor's own prompt

It decides between answering and delegating. It needs to know what to do
when no specialist fits - and not the domain knowledge, which belongs to the
specialists. The runtime already appends every subagent's description and
full prompt to it, so the supervisor's prompt does not repeat them.

Its capabilities cannot be kept minimal. The toolsets, skill sets and source
set mounts of every subagent are aggregated onto the supervisor's own sandbox
(asgard-core `internal/bpcontroller/server/sandbox_orchestration.go`), so
anything a subagent can call, the supervisor can call directly. A split buys
focused prompts and a smaller context per sub-task; it does not fence one
specialist's tools off from the supervisor.

### Each subagent's `description`

It is routing text: the orchestrator reads it as "when to spawn", with the
Agent's `sampleQuestions` appended as example questions, beside the
subagent's full prompt. Business nouns, and an explicit boundary where two look
similar.

### When to compute the roster

A static list is right until the caller genuinely needs a different roster per
conversation - per tenant, per user's permissions, per brand. Then the
`expression` form is worth its complexity. Do not start there.


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

A static list of subagents is the simple case. Computing it lets the caller shape
the roster per conversation:

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

That is how a caller extends a subagent's persona or skills per conversation
without a CR change. `skillSetNames` on the blueprint itself is a plain
comma-separated string in `value`.

### The subagent is an ordinary `Agent` CR

`agentClass: managed`, `aliasName` (the delegation name, `^[a-z0-9][a-z0-9-]*$`,
conventionally the CR name without its `ag-` prefix), and `description` - which
is routing text for the orchestrator, not a self-introduction. The
orchestrator sees it, with any `sampleQuestions` appended, as the "when to
spawn" line above that subagent's full prompt, so it carries the business
nouns.

`prompt` requires all four of `persona` / `task` / `context` / `format`, but they
may be empty strings. Subagents in one deployment use `persona` only and leave
the rest `""` with a comment saying why - the CRD requires the keys, not the
content.

### BotProvider auth is a choice, and it is the security boundary

    authMode: none      an anonymous public widget. There is no key a browser
                        could keep secret, so protection has to be on the
                        capability side: read-only chain, zero-parameter tools,
                        read-only mounts - across every subagent, because
                        their tools are the supervisor's too.
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
out of the JSON the CRD stores it in - a typo there silently drops a subagent,
and the symptom is an agent that "can't call any tools".

Two things no check catches, both needing a real conversation:

- an `agents` or `hooks` expression that evaluates to the wrong thing for a
  given payload. The syntax is checked, the logic is not
- a wrong `configs[].name` on a processor, since that field is a free-form string

For a hook, confirm what it produced rather than that it ran:

```bash
kubectl get -n <namespace> sandboxes.asgard-ai.com
# then, in a conversation, have the agent read the file the hook writes
```
