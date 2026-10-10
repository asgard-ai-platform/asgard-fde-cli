---
group: Capabilities and scheduling
description: work on a schedule with nobody watching
---
# Trigger

This is written from one Trigger, in one chart. `../wiki/coverage.md`
counts them: of the reference deployments that declare CRs at all, exactly one
declares a Trigger, and it declares one. Every rule below about the cursor, the cold start
and what a scheduled run may not do is generalised from that single instance,
and none of it has a second arrangement to check against. Treat the reasoning as
the transferable part and the specifics as one worked example.


Scheduled work. A cron that starts an agent run rather than a pipeline.

**Seen in:** an hourly job that notices new arrivals in one system, matches them
against records in another, and mails whoever asked for them through the
customer's own mail endpoint.

**Checked:** against a cron Trigger and its entrypoint Workflow, and the CRD; the cron section against asgard-kube `cbd8d70`, which has no pattern on `schedule`. How state is saved and handed to the next run against asgard-core `478cf5d6` - `internal/bpcontroller/server/bp_controller.go`, `internal/processor/helper/clidriver_run.go` and `internal/constants.go`. That a fire makes its Job from the derived CronJob's own template, so it runs what the schedule would, against asgard-core `7346d2e5` `internal/bpcontroller/server/bp_controller.go` (`TriggerCronBasedTrigger`).

**Unchecked:** the cursor rules and the prompt guidance. They are one deployment's design, and there is no second Trigger to hold them against.

Read the platform side first: `../wiki/automation.md` covers Trigger and API,
and why only cron is left. This page assumes you have read it.

## When this shape, and when not

Use it when work should happen on a schedule with no one watching, and the
work needs judgement rather than a fixed transformation.

The main case for it is replacing an ETL that exists only to join two
systems. A scheduled agent run can mount both source systems at once and do
the matching in its own reasoning - no raw zone, no intermediate tables, no
pipeline to maintain.

The trade-off: the matching is guaranteed by a prompt rather than by SQL.
That is fine for a notification that can be re-sent, and wrong for anything
accounting-grade.

## The shape

    Trigger   tr-<name>      cron schedule + entrypoint
      -> Workflow  wf-<name> one stream-llm-completion-message processor
    Toolset   ts-<name>      the agent's outward action
      -> Workflow  wf-<verb> what the action does

Write only the Workflow and the Trigger. The Trigger reconciler provisions
the BotProvider, its API-key Secret, and the CronJob itself. Do not hand-write a
BotProvider for a Trigger.

## Generate it

    asgard-cli add trigger <name> --layers sl-<name>

That writes the structure below with the fields that fail silently already in
place - the display annotation, the labels the UI needs, the current field names.
Those get lost when the skeleton is copied by hand, and nothing reports them
missing: not helm lint, not CRD validation, not a server dry-run.

The generated file marks the judgement calls TODO, and the rest of this page
covers them.

## The skeleton

`templates/trigger/tr-<name>.yaml` and its entrypoint workflow.

```yaml
apiVersion: asgard-ai.com/v1alpha1
kind: Trigger
metadata:
  name: tr-<name>
  annotations:
    asgard-ai.com/trigger-name: "<display name>"
  labels:
    asgard-ai.com/trigger-suspend: {{ .Values.triggers.<name>.suspend | quote }}
    # Both of these are on the Trigger itself. Without them the schedule fires
    # correctly and the editor opens as a blank canvas.
    asgard-ai.com/workflow-set-id: wf-<name>
    asgard-ai.com/project-environment-id: {{ .Values.asgard.projectEnvironmentId | quote }}
    {{- include "<chart>.labels" . | nindent 4 }}
spec:
  triggerClass: cron
  cron:
    schedule: {{ .Values.triggers.<name>.schedule | quote }}
    timeZone: {{ .Values.triggers.<name>.timeZone | quote }}
  entrypoint:
    workflow: wf-<name>
    entry: main
```

### The entrypoint Workflow declares its capabilities inline

There is no Agent CR and no SandboxBlueprint in this shape. The processor
carries them directly:

```yaml
spec:
  entries:
    - name: main
      handlingProcessor: proc-run
  exits: []
  processors:
    - name: proc-run
      type: stream-llm-completion-message
      configs:
        # An ARRAY, as a JSON expression. This is the whole point of the shape:
        # one run can mount several source systems and do the cross-system
        # matching in its own reasoning, which is what replaces the ETL.
        - name: semanticLayers
          expression: '[{"name": "sl-a", "allowQuery": true, "allowWrite": false}, {"name": "sl-b", "allowQuery": true, "allowWrite": false}]'
        # Comma-separated string. The outward action, if there is one.
        - name: toolsets
          value: ts-<name>
        - name: prompt
          value: |-
            <the whole instruction for the run>
```

Write `allowWrite: false` on every entry. This processor resolves a
missing `allowWrite` to true, so leaving it off grants a silent write path into
the customer's systems, to the one kind of run that has nobody watching it. Spelled out, the source
systems stay read-only and the only side effect points outward through the
Toolset.

The same rule is why `Agent.managed.semanticLayers[]` always carries an
explicit `allowWrite: false`.

Do not add a `listen-message` processor. This is not a conversation: the Trigger
fires once, the run does the whole batch, and it ends. There is no second turn
to wait for, and adding a listen step leaves the run hanging.

The Workflow still needs the full workflow-set label set, with
`workflow-set-type: trigger`.

Do not write a BotProvider. The Trigger reconciler provisions it, its
API-key Secret, and the CronJob.


The schedule grammar is in "`schedule` is ordinary cron" below.

## The two labels that decide whether it is editable

A hand-written Trigger needs `workflow-set-id` (equal to its entrypoint
Workflow's) and `project-environment-id` on the Trigger CR itself. The
editor reads both straight off the Trigger, and the query that loads the set is
gated on them.

The platform copies both onto a Trigger it creates itself. Nothing copies them
onto a hand-written one, and without them the schedule fires correctly while the
editor opens as a blank canvas.

The entrypoint Workflow needs the full workflow-set label set with
`workflow-set-type: trigger`. That type belongs to the Trigger: its breadcrumb
returns to the Trigger list, and it is deliberately absent from the automation
list.

Do not copy the platform's `created-by-trigger` marker onto a hand-written
one. That is an ownership marker: it hides the workflow from the generic list and
blocks deletion through the API, and lifecycle here belongs to helm.

## State is one cursor

The agent saves `{"watermark": "<ISO 8601>"}` through the builtin
`set_trigger_state` tool. It lands in the Trigger's `status.initState` and is
injected into the next run's system prompt as a `<trigger_state>` block, so it
is already there at startup, and the prompt should tell the agent to read that
block rather than to call a tool for it. Each save replaces the previous state
whole, and a state past the platform's inline limit is not inlined - the block
then tells the agent to read it with `get_trigger_state` - which is one more
reason to keep it to a cursor.

### Do not add a look-back window, and do not keep a seen-set

It is tempting to widen the window so back-dated records are not missed. First
check which field actually gets back-dated: if the cursor column is stamped at
write time, a record entered today for last week's event still has today's
timestamp and a strictly-greater-than cursor catches it.

One deployment added a look-back for exactly that reason, found the premise was
false, and in removing it also removed the seen-set the look-back had required -
which had hit a size cap, needed pruning, become a sliding window, and opened a
real duplicate-notification hole. The look-back guarded nothing.

### Two cursor rules that are not obvious

Cold start sends nothing. On empty state, seed the cursor to the current
maximum and exit. Otherwise the first run mails everyone about every historical
record at once.

On a send failure the cursor stops short of the failed record, so the next
run retries it. The few already-sent records after it get one duplicate. Missing
a notification is worse than repeating one.

## Writing the prompt - the part the generator leaves TODO

A scheduled prompt is not a chat prompt. Nobody is online, there is no second
turn, and a mistake is discovered hours later in a log.

### Open by saying what this is

    你不是聊天助手 —— 沒有人在線上,你這一輪要自己把整批做完,然後結束。

Without that, a model trained on conversation asks a clarifying question nobody
will answer, and the run ends having done nothing.

### Explain why the job exists

Two or three sentences on the business situation - what happens in which system,
who is left uninformed, what this run fixes. An agent that understands the point
handles the edges better than one following steps, and the person reading the
prompt next year needs the context.

### Then the steps, each with its reason

Reading state. Say that it is already in the system prompt and needs no
tool call, give its exact shape, and say what is not in it:

    <trigger_state> 就是你上一輪存下來的狀態,啟動時已經在你手上了。
    形狀就只有一個游標:{"watermark": "<ISO 8601>"}
    沒有別的欄位 —— 你不需要記「看過哪些」,游標本身就是全部的記憶。

That last line stops the model inventing a seen-set, the design that had to be
removed once.

Cold start. Spell out that the first run sends nothing, and why:

    第一次跑:查出目前最大的時間戳,存成游標,什麼都不要送,直接結束並說明這是初始化。
    理由:歷史上累積數千筆,第一輪照常送信會讓幾百個人同時收到早就過期的通知。

The query, and why there is no look-back. This part needs the most words.
Name which timestamp is written when, and therefore why a
strictly-greater-than cursor is safe:

    條件是「時間欄位 > watermark」,嚴格大於。不要回看、不要減任何天數。
    為什麼:「事件日期」會被回填 —— 有人今天補登上週的事,事件日期就是上週。
    但「最後修改時間」是寫入當下蓋的,補登的資料它就是今天。所以用最後修改時間
    當游標,補登的一定會在它被寫入的那一輪撈到,不會漏。回看只會重複撈到已通知過的。

Point at the sample query rather than repeating the SQL. The layer already
holds a tested version; say which one and what to change in it.

Then the rules that make the query correct, each with the measurement behind
it. These are the same rules the layer's `instruction` carries, restated here
only where this run depends on them - with the number that proves it:

    - 過濾沖銷行:每筆都有一列同絕對值的負數量,少了這個過濾,總和會得到 0
    - 過濾母單類型:實查母單 54% 是調撥單,不過濾會大量誤發
    - 用 TO_CHAR 讀時間:直接 SELECT 會吃掉時分秒,游標會退化成「天」的精度,
      同一天稍後寫入的資料會被永遠跳過

Include the measured number. A model follows "54% are transfers" more reliably
than a bare "filter by type".

### Close with what to report

The run's summary is the only thing a person will read. Say what it must
contain - how many processed, how far the cursor moved, what failed - and, if any
part is mocked, that it must lead with the fact that nothing was actually sent.

## Fields that are not obvious

`requestConsent: false` is mandatory on a Trigger-driven Toolset. Nobody is
online at 03:00; `true` parks the whole run waiting for an approval that never
comes.

A mock must announce itself. Where the outward action is not wired up yet,
the tool returning success is deliberate: a failure would stop the cursor and
the chain would never be exercised. So both the tool description and the prompt
must require the run summary to lead with the fact that nothing was actually
sent. The harm a mock can do is a log that reads as if people were notified.

Suspending is per-env, through values. A mocked or read-only chain in
production still means a scheduled hit on live source systems, which is usually
reason enough to keep it suspended there.


## `schedule` is ordinary cron

Write the five-field expression you would write anywhere, or an `@descriptor`:

    0 9 * * 1-5      weekdays at nine
    0 9,15 * * *     twice a day
    @daily           and every other @-macro
    0 0 1 JAN *      month and day names

The grammar is exactly what `batch/v1` CronJob's `spec.schedule` accepts,
because the value is copied verbatim into the derived CronJob. The CRD has no
pattern on the field. The platform validates it with `cron.ParseStandard()` in
its admission webhook on write, and again at the public API boundary, so a
malformed expression comes back with a message rather than deploying and never
firing. `helm lint` and `asgard-cli verify` do not check it.

## Verify

```bash
asgard-cli check
asgard-cli verify <project>
```

The xref check resolves the entrypoint's `(workflow, entry)` pair and enforces
the Trigger's own two labels.

Fire it once after the deploy rather than waiting for the schedule. A fire runs
what the schedule would, cursor included:

```bash
asgard-cli operate trigger fire <trigger> --release <release> --wait 5m
asgard-cli operate trigger logs <trigger> <invocation> --release <release>
```

Check the result of that run before trusting the schedule. An invocation that
succeeded with the agent's verdict NEEDS_INPUT stopped to ask a question rather
than finishing; `asgard-cli operate trigger runs` shows the verdict beside the
status, and the question is read and answered in that invocation's
conversation (`asgard-cli operate chat replay trigger/<trigger> --invocation <id>`,
then `asgard-cli operate chat send` with the answer).

Read the run's output and confirm the cursor advanced to what you expect. On a
cold start it should have sent nothing.
