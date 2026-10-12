---
group: Entry points
description: anonymous audience, one job
---
# Single-agent flow agent

A public entry point with no Agent CR at all. The smallest shape that serves an
anonymous audience.

**Seen in:** a public product-catalogue widget, and a deployment whose whole
chart is a handful of CRs.

**Checked:** against a public widget's BotProvider, Workflow and SandboxBlueprint, and the CRD. The security argument's platform half against asgard-core `478cf5d6` - `internal/edgeserver/middleware/bot_provider.go` (authMode `none` skips the key check on every route), `internal/bpoperator/reconciler/bp_reconciler.go` (an unset authMode defaults to `api-key`) and `internal/bpoperator/reconciler/sb_reconciler.go` (`readOnly: true` becomes a read-only volume mount).

Read the platform side first: `../wiki/agents.md` -
what a Managed Agent and a Flow Agent each are, and which the audience decides. This page assumes you have.

A brand-new channel's first message may never be answered. If the entry
processor runs into the wait point rather than into the agent, the platform
finalises the request when the flow reaches `listen-message` - so that first turn
runs the init and stops. A returning channel resumes from the wait point and is
unaffected, so this is invisible in every test after the first, and it looks like
a bug in a demo with a fresh user. One deployment accepts it deliberately and
documents the cost; if you cannot, the entry has to reach the agent on that
turn.

## When this shape, and when not

Use it when the audience is anonymous and there is one job. A public
support widget answering questions about one product line has one area of work,
and its prompt and capabilities are the whole of it.

An Agent CR adds nothing here. Its guidance would sit beside the Workflow's
prompt as a second set of instructions for the same job, so keep the prompt on
the Workflow's processor and the capabilities on the blueprint.

Use the supervisor shape instead when several areas of work each need their
own guidance, and the agent hub when every caller can authenticate.

## The shape

    BotProvider  bp-<name>       public, authMode: none
      -> Workflow  wf-<name>     the conversation loop; the prompt lives here
        -> SandboxBlueprint sbp-<name>
             toolsetNames        capabilities, declared directly
             skillSetNames
             sourceSetMounts     knowledge, read-only

No `Agent` CR anywhere. A chart in this shape renders zero agents, and the
gate accepts that.

The conversation graph stays minimal on purpose: greet, listen, answer, back to
listen, plus a failure branch to a maintenance message. Telling topics apart is
the agent's job, informed by the prompt - building a workflow per topic puts
that decision in two places.

## Generate it

    asgard-cli add flowagent <name> --public --toolset ts-<name>

That writes the structure below with the fields that fail silently already in
place - the display annotation, the labels the UI needs, the current field names.
When the skeleton is copied by hand these get lost, and nothing reports them
missing: not helm lint, not CRD validation, not a server dry-run.

The generated file marks the judgement calls TODO. The rest of this page covers
them.

## The skeleton

Three files, conventionally in one directory:

    templates/<name>/bot_provider.yaml
    templates/<name>/workflow.yaml
    templates/<name>/sandbox_blueprint.yaml

```yaml
apiVersion: asgard-ai.com/v1alpha1
kind: BotProvider
metadata:
  name: bp-<name>
  annotations:
    asgard-ai.com/bot-provider-name: "<display name>"
    # Widget appearance: avatar, theme, title.
    asgard-ai.com/additional-annotation: |-
      {"embedConfig": {...}}
  labels:
    {{- include "<chart>.labels" . | nindent 4 }}
spec:
  botProviderClass: generic
  entrypoint:
    workflow: wf-<name>
    entry: entry-main
  # Required. Omitting it is an apiserver rejection at apply time, which in
  # practice means during CD. The deployments run 30.
  maxUnsupervisedSteps: 30
  disabled: {{ .Values.botProviders.<name>.disabled }}
  adminApiKey:
    valueFrom:
      secretKeyRef:
        name: {{ include "<chart>.appSecretName" . }}
        key: asgard_resource_api_key
  generic:
    authMode: none          # anonymous visitors; see the security argument below
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
  # Comma-separated strings, not lists.
  toolsetNames:
    value: "ts-<name>"
  skillSetNames:
    value: "sk-base"
  # A JSON string the controller unmarshals.
  sourceSetMounts:
    value: '[{"sourceSetName": "ss-<name>", "mountPath": "/knowledge", "readOnly": true}]'
```

The Workflow carries the prompt on its `stream-llm-completion-message`
processor, with `sandboxBlueprint` in that processor's configs pointing at
`sbp-<name>`, and the full workflow-set label set described below.


### The label that decides which list it lands in

`asgard-ai.com/agent-hub-published: "true"` on the BotProvider publishes it to
the Agent Hub. A public widget must not carry it. The mistake usually arrives
by copying: a supervisor's BotProvider has it,
and the supervisor is the thing you copy from.

The Hub serves callers that can authenticate to the platform. An anonymous widget
is the opposite audience, so the label only puts something in the internal Hub
list that does not belong there. One deployment inherited it exactly that way and
removed it, with three consequences it checked first:

| | |
|---|---|
| the Hub stops serving it | not merely stops listing it - the API returns "not published" |
| it re-classifies as a channel release, typed from `botProviderClass` | which is what makes `additional-annotation` count as this bot's appearance again |
| outward reachability is unchanged | that is decided by `generic.authMode` alone |

This label and the workflow-set labels answer different questions and neither
substitutes for the other: the set labels decide whether the bot exists in the
UI, this one decides which list it lands in.

## A new Flow Agent is not blank

It ships a runnable default flow. Do not author these five nodes by hand; the
product already created them:

| node, as the canvas labels it | processor | what it is for |
|---|---|---|
| `main` | - | the entry |
| Init | `update-context` | initialise variables. Empty by default: the place to put fixed values |
| Agent Stream Message | `stream-llm-completion-message` | call the model and stream the reply. The body of the conversation |
| Listen Message | `listen-message` | wait for the next user message, which is what makes it a loop |
| Push Error Message | `push-message` | on the Agent node's Failure branch. The default already handles a model error |

So the work is editing that flow: the prompt on the Agent node, what Init sets,
and whatever the shape needs beyond the loop. The node menu behind Next Step
-> Add Target Node groups everything else as Flow, Message, Model, Action,
Query and API.

Beside the flow, the workflow-set page carries Sandbox for trying a run and
Release for publishing a version. A published Flow Agent can be embedded in
a site or wired to Telegram, Slack, LINE, Discord or Sindri - the channel is
`../usecase/chat-channel.md`, and `botProviderClass` is immutable once created.

## Fields that are not obvious

The prompt lives on the processor, in the `stream-llm-completion-message`
processor's config, in four sections: identity, when to use which tool, what the
data means, and response format.

The names fields are comma-separated strings in `value`, not lists.
`sourceSetMounts` is a JSON string the controller unmarshals
(`sourceSetName` / optional `subPath` / `mountPath` starting with `/` /
`readOnly`).

The processor must not re-declare `toolsets`. That would be a second
declaration of the same capability, and the two would drift.

Widget appearance belongs on the BotProvider, in
`asgard-ai.com/additional-annotation` -> `embedConfig` (avatar, theme colours,
title), so the front end does not own it.

The off switch is `disabled` in values. Flipping it takes the public endpoint
down without deleting anything.

## Writing the prompt the generator leaves TODO

There is no Agent CR, so the prompt is where all the behaviour lives. Five
sections, in this order.

### 1. Who it is talking to, and what follows from that

Name the audience explicitly, then draw the consequences rather than leaving them
implied:

    你面對的是公司外部的客戶,不是同事。所以:
    - 用字要讓沒有內部背景的人也看得懂,不要用料號、schema 這類內部術語
    - 你代表公司對外發言,答不出來就誠實說不知道並引導到人工客服,不要猜
    - 不要主動索取客戶的個人資料

Then state the working stance: it answers from tools and data, not from
memory. That sentence matters more than any later instruction.

### 2. When to use a tool at all

Four rules; the third is the most important:

1. If a tool or knowledge source can answer it, get the evidence rather than
   reasoning in prose.
2. If nothing can, say so plainly, say why, and do not speculate.
3. Answer only from what a tool returned or what the sources hold. Never
   invent a number, a spec, a price or a record.
4. When the user does not know what to ask, offer directions it can actually
   serve - so the conversation moves instead of stalling.

### 3. The sources, described so the model can choose

This is the longest section and needs the most effort. For each source:
what it returns, how much, and which questions it settles.

    list_products —— 89 款上架產品:料號/品名/品牌/分類路徑。
      產品的問題一律先叫這支。
    list_product_categories —— 33 個分類,含沒有產品的分類(product_count = 0),
      那代表「品類有、目前沒上架機型」,不等於「我們沒有這項產品」。

Two things to include, which you learn by using the system:

- the shape of the tool: "五支都不吃參數,一次回傳全部,你自己從結果裡挑" -
  otherwise the model waits for a filter that does not exist
- the misreading: an empty category is not a missing product line. Every
  source has one of these, and it is where a confident wrong answer comes from

End with an explicit routing rule, in the customer's terms rather than the
system's:

    數量、清單、聯絡方式、下載連結 → 查詢工具
    說明、比較、判斷、排除故障     → 知識來源
    不確定時兩邊都查,交叉印證。

### 4. 口徑 and boundaries, written as instructions

Write what to say because the data syncs daily, rather than only "the data
syncs daily":

    - 資料每天同步一次,不是即時。客戶問到很新的機型時要說明這一點。
    - 回傳的都已經是上架商品,不要自行推測還有別的。
    - FAQ 可能還在補齊中,查不到時改用其他來源回答,
      不要因此告訴客戶「我們沒有這項資訊」。
    - 你沒有任何管道查得到客戶的個人資料 —— 那些刻意未納入。被問到請轉人工客服。
    - 庫存、交期、報價不在你的資料範圍,說明需由業務確認並引導留下聯絡方式。

The distinction between "I cannot find it" and "we do not have it" is worth a
line of its own. Getting it wrong makes the agent tell a customer the company
does not sell something it sells.

The last two are the security boundary in prose. They match what the CRs actually
allow - and if a prompt has to ask the agent not to reach something, check
whether the capability should have been there at all.

### 5. Format

Structure, length, tone, and what never to appear. Keep it short: three sections,
bullets over paragraphs, and say whether emoji are wanted - models add them
otherwise.

### One rule across all five

Do not name CRs in a prompt. Tool names the model actually calls belong here;
`ts-`, `sl-`, `sbp-` names do not. A prompt that names infrastructure has to be
edited every time the infrastructure moves.

## The security argument, and its limit

The endpoint is public and unauthenticated: a key in front-end JavaScript is
readable by anyone, so there is nothing to authenticate with. The protection is on the
capability side, not the auth side:

- the whole chain is read-only
- the query tools take zero parameters, so no user input reaches SQL
- knowledge is mounted `readOnly: true`
- no path reaches personal data

The argument no longer holds once someone adds a parameterised-SQL or
write-capable tool; at that point revisit the shape, not just the tool.


`adminApiKey` is separate from visitor auth. It guards the admin API and
reads the namespace's platform resource key.

## The UI metadata that decides whether it exists

A Workflow with no workflow-set labels belongs to no set, so the UI has nothing
to list - while runtime is perfectly fine, and lint, CRD validation and
server-side dry-run all stay green. One deployment shipped a bot that was
invisible in the console for exactly this reason.

Required on the Workflow: `workflow-set-id`, `main-workflow-set: "true"` on
exactly one member, `workflow-key`, `workflow-set-type: bot`, and the
`workflow-set-name` annotation.

## Verify

```bash
asgard-cli gate               # every local check, the lint step included
asgard-cli verify <project>   # or one step alone, while iterating
```

Never run `helm lint` by hand: without the reserved `asgard` values file
that `gate` supplies, every chart that labels anything fails. `asgard-cli gate
--help` says why.

The xref check resolves the whole chain - `BotProvider.entrypoint` to a
`(workflow, entry)` pair, the processor's `sandboxBlueprint` config to a real
blueprint, and the blueprint's names fields to real CRs - plus the workflow-set
labels the UI needs.

`asgard-cli verify` prints "0 agent(s)" for this shape and passes. That
zero is the expected state here, and it is also the signal if a chart that should
have agents suddenly reports it.

What no check catches: `Workflow.spec.processors[].configs[].name` is a
free-form string, so an invented config key lints clean, passes CRD
validation, and then does nothing at runtime. Only a real conversation finds it.
