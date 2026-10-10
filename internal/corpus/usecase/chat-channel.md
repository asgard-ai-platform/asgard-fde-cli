---
group: More entry points and more read paths
description: LINE, Telegram, Discord or Slack instead of a web widget. No deployment runs this shape; read its provenance line first
---
# Reaching the agent from a chat platform

LINE, Telegram, Discord or Slack as the entry point, instead of a web widget or
your own front end. It is one field on the `BotProvider`, everything behind it is
unchanged - and that field is immutable after creation.

**Seen in:** the platform's BotProvider contract, and a deployment that decided
against LINE and wrote down exactly what taking it on would have cost.

**Checked:** against the CRD, field by field: all five credential blocks below match
`asgard-kube/pkg/apis/asgard/v1alpha1/types.go` exactly - `generic` takes
`apiKey` and `authMode` and both are optional, while every chat class's
credentials are required. So a `generic` BotProvider with no credential
block at all applies cleanly and is open; a `line` one missing
`channelSecret` is refused. That asymmetry is the reason the anonymous-entry
argument is about `generic` and not about the chat classes.

Two rules the CRD enforces:
`ExactlyOneOf=generic;telegram;line;discord;slack` - exactly one class block,
so leaving the old one behind while adding a new one is refused - and
`botProviderClass` carries `self == oldSelf`, which is where the immutability
below comes from.

Against asgard-core `478cf5d6` for how messages arrive and what a wrong
credential does: asgard-core `internal/edgeserver/handler/bot_provider.go` (the
LINE and Telegram webhook handlers),
asgard-core `internal/edgeserver/helper/kube.go` (how a `secretKeyRef` resolves),
asgard-core `internal/bpoperator/reconciler/bp_reconciler.go` and
asgard-core `cmd/bpoperator/main.go` (the connector Deployment, only for `discord` and
`slack`, image from `BP_CONNECTOR_IMAGE`). Against every BotProvider in every
deployment the extracts were written from, including
asgard-industry-demo-generator `718cc0e`: every one is `generic`. Against
asgard-js-sdk `56ad14e` `packages/react/src/utils/selectors.ts` for where
`embedConfig` is read.

**Unchecked:** no reference deployment runs a chat class, so none of this has
been seen working against a real LINE, Telegram, Discord or Slack account, and
what LINE does when two bots are pointed at one channel is one deployment's own
note rather than something anybody here has run.

Checking that yourself can mislead you. Grepping the parent
directory for `botProviderClass: line` returns hits - three of them, plus
`discord`, `slack` and `telegram`. Every one is inside a scratch repository this
tool scaffolded during a walk-through: they carry `.asgard-config.json`, the
`projects/<slug>/chart/app/templates/` layout, and workspace slugs like `line`
and `classes`. They are this page's own output and are not evidence for it:
they agree with it because they were generated from the same templates.

Read the platform side first: `../wiki/integration.md` -
which credentials each chat platform needs, and who fills in what. This page assumes you have.

What this shape costs to get is `../needs/chat-channel.md` - what has to come
from the customer before any of it can be built.

## When this shape, and when not

Use it when the customer's users already live on that platform. A LINE
official account with an existing following is a distribution channel you cannot
reproduce with a widget, and asking those users to visit a web page instead
loses most of them.

Do not use it when:

- the caller can authenticate. Then the agent hub is the shape - see
  `../usecase/agent-hub.md` - and it needs no BotProvider at all.
- you need control of the presentation. A chat platform owns its own avatar,
  display name and colours, in its own console. A web widget's appearance is
  `embedConfig`, a JSON value in the `asgard-ai.com/additional-annotation`
  annotation that the asgard-js-sdk widget reads, and no chat platform renders it.
- the customer wants both this and an internal hub. That is two projects,
  because the entry point and the read path follow the audience.

### The constraint that decides it

The platform's agent hub (`preset-agent-hub`) is reached by a caller that
authenticates and passes `agent_hub.agent_names`. It has no webhook receiver.
So a chat platform can only reach a `BotProvider`, and that forces the Flow Agent
shape:

    BotProvider (line)  ->  Workflow  ->  SandboxBlueprint  ->  Agent(s)

not

    caller -> preset-agent-hub -> Agent CR       <- no webhook, no LINE

One deployment chose the agent hub first and recorded this as the cost: "LINE
頻道無解" - restoring a LINE channel would mean changing shape, not adding a CR.

## The classes, and what each one costs

`spec.botProviderClass` is `generic | telegram | line | discord | slack`. Every
class takes the same `entrypoint`, so the Workflow, the SandboxBlueprint and the
tools behind it do not change when the channel does.

| class | credentials | how messages arrive | extra infrastructure |
|---|---|---|---|
| `generic` | `apiKey`, `authMode: api-key \| none` | your front end calls the HTTP API | none |
| `line` | `channelAccessToken`, `channelSecret` | LINE posts a webhook | none |
| `telegram` | `botToken`, `webhookSecretToken` | Telegram posts a webhook | none |
| `discord` | `botToken` | a Connector Pod holds a WebSocket to the Gateway | a Deployment the operator creates |
| `slack` | `appToken`, `botToken` | a Connector Pod | a Deployment the operator creates |

The split that matters: webhook classes cost one CR, socket classes get a pod.
`discord` and `slack` maintain an outbound WebSocket, so the operator creates a
Deployment for each and its image comes from a platform-level env var rather than
from your chart - nothing in values, nothing to review, and one more thing that
can be pending when you look for why the bot is silent.

## Generate it

    asgard-cli add flowagent support --project <project> --bot-class line

That writes the three CRs of the flow agent with the channel's credential block
in place, and prints what the channel costs: which keys the release's Secret
needs, and
whether the class needs a connector pod. `--bot-class` accepts any class the CRD
declares and defaults to `generic`.

Do not hand-copy this from another chart. The credential block differs per
class, `botProviderClass` is immutable once applied, and a wrong key name in a
`secretKeyRef` resolves to an empty string rather than an error. On the token the
bot answers with (`channelAccessToken`, `botToken`) that is a webhook that returns
200 and a bot that never answers; on the secret that authenticates the webhook
(`channelSecret`, `webhookSecretToken`) every delivery is refused, a 500 for LINE
and a 401 for Telegram.

## The skeleton

```yaml
apiVersion: asgard-ai.com/v1alpha1
kind: BotProvider
metadata:
  name: bp-support
  annotations:
    # Required. Without it the BotProvider is nameless in the Platform UI.
    asgard-ai.com/bot-provider-name: "客服機器人"
spec:
  botProviderClass: line          # IMMUTABLE after creation
  entrypoint:
    workflow: wf-support
    entry: entry-main             # both halves are checked; a wrong entry is as dead
  disabled: false                 # the off switch, and what makes a cutover reversible
  maxUnsupervisedSteps: 30        # required by the CRD; processor hand-offs per request
  adminApiKey:                    # guards the admin API, separate from the channel
    valueFrom:
      secretKeyRef:
        name: {{ include "<chart>.appSecretName" . }}
        key: asgard_resource_api_key
  line:
    # Both from the LINE Developers console for this channel. They are NEW
    # keys, so each has to be declared under `appSecret:` and then set - a
    # value set against no declaration is stored and never injected.
    channelAccessToken:
      valueFrom:
        secretKeyRef:
          name: {{ include "<chart>.appSecretName" . }}
          key: line_channel_access_token
    channelSecret:
      valueFrom:
        secretKeyRef:
          name: {{ include "<chart>.appSecretName" . }}
          key: line_channel_secret
```

It goes in `projects/<project>/chart/app/templates/<name>/bot_provider.yaml`,
beside the `workflow.yaml` and `sandbox_blueprint.yaml` of the same chain.

## Replacing an existing LINE bot

One LINE 官方帳號 cannot host two bots. A LINE channel has one webhook URL.
Point a second `BotProvider` at the same channel and both receive every event
and both answer - the user sees two replies, and neither implementation is
wrong.

So replacing an existing LINE bot is a cutover, not a deployment:

1. deploy the new `BotProvider` with `disabled: true`
2. verify against a test channel, not the live one
3. remove the old webhook
4. flip `disabled` to `false`

A deployment that considered LINE recorded this as a decision rather than a task,
so nobody would later read the absence of a LINE bot as an open item
and add one casually.

## Designing the conversation

The prompt lives on the Workflow's processor, not on an Agent CR, and a chat
platform changes what it has to say:

- The conversation is long-lived and mostly idle. A widget session ends when
  the tab closes and a LINE thread does not, so the prompt cannot assume the
  previous turn was recent. `channelMaxIdleMs` is not the dial for that -
  `../usecase/workflow-chain.md` has what it bounds and what it does not.
- There is no page around the bot. A widget can rely on the surrounding page
  for scope; a chat bot cannot, so the prompt has to say what it does and what
  it does not do in its first turn, or users ask it anything.
- Keep the graph minimal. 招呼 -> listen -> answer -> back to listen, plus a
  failure branch to a maintenance message. Routing between topics is the
  orchestrator's job informed by the prompt, not a workflow node per topic -
  building the latter puts the routing in two places.
- `debugMode` is `never | on-demand | always`. Not `always` on a live channel.

### Security on a chat channel

There is no `authMode` on a chat class: the channel's credentials authenticate
the channel, and anyone who can message the account reaches this bot. So
the protection is on the capability side, exactly as for an anonymous widget -
the whole chain read-only, tools that take no parameters, mounts read-only. That
no longer holds once a parameterised or write-capable tool is added.


## Verify

    asgard-cli verify <project>

It resolves `entrypoint.workflow` and `entrypoint.entry`, because a wrong
entry name is as dead as a wrong workflow name and apply accepts both, and it
requires the `bot-provider-name` annotation.

What it cannot check, and what to check by hand:

- whether each credential key is declared AND set, and belongs to the channel
  you think it does. Declared without set fails the run; set without declared is
  stored and never injected, and every local check stays green. The first symptom
  of a wrong token is a webhook that returns 200 and a bot that never answers.
- whether the old bot on that channel is off. Nothing in the repo can see it.
- whether a connector pod came up, for `discord` and `slack`: the operator names
  it `bp-connector-<name>`, so `kubectl get deploy -n <namespace>`.
