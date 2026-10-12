---
group: The mechanism, and the scale
description: "a Workflow with more than one step: the processor types, and what crosses between them"
---
# Chaining processors, and the graph that wires them

A Workflow with more than one step: what crosses between the processors, and how
`relationships` decides the order. This is the
mechanism every other shape is built out of.

**Seen in:** a two-call mail sender, a search tool that reshapes its API's
response, a conversation loop, and a nine-branch content pipeline.

**Checked:** across every reference
deployment. Every construct this page cites is written in at least one of
them, `??` included; `prevPayload` is in nearly all of them and is how a chain
passes anything at all. Also against asgard-kube `cbd8d70` for the
variable's exactly-one-of and its name pattern, for the open `labels` map on
Entry, Exit and Processor, and for the exactly-one-of on a relationship's `to` -
and against every `variables` block and every declared exit in the reference
deployments, where an exit is reached by a relationship naming it.
What crosses between processors against asgard-core `478cf5d6`:
what each processor copies forward and adds, in asgard-core `internal/processor/task/`,
and where `prevPayload` is written - only in asgard-core `internal/bpcontroller/server/bp_controller.go`.

Read the platform side first: `../wiki/workflow.md` covers the processor types
and the ways a config takes a value. This page assumes you have read it.

## When this shape, and when not

Almost every Workflow is a chain. A single-processor Workflow is the exception:
a tool that does one HTTP call still needs three processors, because the third
tells the agent whether the first worked.

Add more processors when:

- a step's output is another step's input - a token, a search result to
  reshape, an id to look up.
- the flow branches on a value - use a `router`.
- failure has to be reported rather than swallowed. This is the common case and
  the one most often left out.

Do not add processors to express what a prompt should decide. A workflow node
per conversation topic puts the routing in two places: one deployment removed its
topic workflows and let the agent tell the topics apart from the prompt instead. Keep the
conversation graph minimal - greet, listen, answer, back to listen, plus a
failure branch.

## Generate it

    asgard-cli add flowagent <name> --project <project>
    asgard-cli add querytool <name> --project <project> --connector dc-<name> --toolset ts-<name>
    asgard-cli add httptool <name> --project <project>

Each writes a working chain with its relationships already wired, and
`http-request`'s failure branch with it. Add processors to that rather than
starting from an empty `spec` - the parts that fail silently (the display
annotation, the workflow-set labels, the environment id) are already right.

Check that what you add is wired. A Workflow whose processors carry no
`relationships` is legal, passes every check, and answers nothing: the run
reaches the entry's `handlingProcessor` and stops there, with the rest dead.
`gate` W3 reports exactly that, and
`../wiki/green-and-doing-nothing.md` is the way in when the symptom is silence
and the kind is not yet known.

## The skeleton

```yaml
apiVersion: asgard-ai.com/v1alpha1
kind: Workflow
metadata:
  name: wf-search
  annotations:
    asgard-ai.com/workflow-name: "商品搜尋"
spec:
  variables:
    # A variable takes exactly one of value and valueFrom. `value` is the
    # per-environment constant, read as vars.<name> from an expression or a
    # Handlebars template, so the chart value is written once rather than
    # repeated in every config that needs it. Every reference deployment
    # declares locale and timezone this way.
    - name: locale
      value: "zh-TW"
    - name: baseUrl
      value: {{ .Values.<system>BaseUrl | quote }}
    # The only way a secret reaches a workflow: a config has value, expression
    # or template, and no valueFrom.
    - name: apiKey
      valueFrom:
        secretKeyRef:
          name: {{ include "<chart>.appSecretName" . }}
          key: search_api_key

  entries:
    - name: search
      handlingProcessor: proc-input          # where the run starts
      # display_name is this node's title on the canvas and description is the
      # line under it. Both optional - a missing display_name shows the node's
      # own name - and both worth writing, on entries, exits and processors
      # alike, because a nine-node graph read by `proc-*` name is unreadable.
      labels:
        display_name: Search
        description: 商品搜尋工具的進入點
      tooling:
        name: search_products
        description: |-
          What the model reads to decide whether to call this. Name the tool it
          could be confused with.
        allowUploadFile: false   # required, and it has no default
      inputSchema: |
        { "type": "object",
          "properties": { "q": { "type": "string" } },
          "required": ["q"] }

  # A run ends when its terminal processor finishes, so `exits: []` is the
  # ordinary answer for a tool workflow and for a conversation loop alike.
  # Declare one where a named end is worth showing on the canvas, or where an
  # exit hands off to another workflow through its handlingWorkflow - and give
  # it the same display_name / description pair as any other node.
  #   - name: finish
  #     labels:
  #       display_name: Exit
  # Nothing reaches it by falling off the end: `to` takes exactly one of
  # processor and exit, so the last hop is routed at it by name -
  # `to: {exit: finish}`.
  exits: []

  processors:
    # (1) Copy the arguments into named context values first, where they are
    #     trimmed and validated once. prevPayload itself survives the
    #     http-request; its result goes to httpResponse.
    - name: proc-input
      type: update-context
      configs:
        - name: searchQuery
          expression: 'String(prevPayload.q || "").trim()'
        # Validate here rather than trusting the model: an enum it half-remembers
        # arrives as a plausible wrong string.
        - name: sortType
          expression: |-
            (() => {
              const allowed = ["DEFAULT", "SOLD_DESC", "RATING_DESC"];
              return allowed.indexOf(prevPayload.sortType) >= 0
                ? prevPayload.sortType : "DEFAULT";
            })()

    - name: proc-search
      type: http-request
      configs:
        - name: url
          value: "https://api.example.com/search"
        - name: method
          value: POST
        - name: parseJson
          value: "true"
        # Any key that is not url/method/body/parseJson becomes a header.
        - name: Content-Type
          value: application/json
        - name: Authorization
          expression: '"Bearer " + vars.apiKey'
        - name: body
          expression: 'JSON.stringify({ q: searchQuery, sortType: sortType })'

    # (3) Reshape before it reaches the model. The raw response is not the answer.
    - name: proc-response
      type: push-message
      configs:
        - name: payload
          expression: |-
            (() => {
              const items = ((httpResponse.json && httpResponse.json.results) || [])
                .map((r) => ({ id: String(r.id), name: r.name }));
              return { ok: true, itemCount: items.length, items: items };
            })()

    - name: proc-error
      type: push-message
      configs:
        - name: payload
          expression: '({ ok: false, error: prevError })'

  relationships:
    - from: {processor: proc-input,    relationName: success}
      to:   {processor: proc-search}
    - from: {processor: proc-search,   relationName: success}
      to:   {processor: proc-response}
    # Every processor that can fail needs this, or the run ends in silence and
    # the agent sees a tool that returned nothing.
    - from: {processor: proc-search,   relationName: failure}
      to:   {processor: proc-error}
```

It goes in `projects/<project>/chart/app/templates/workflow/wf-<name>.yaml`, or
`templates/tool/` where the chart groups tool workflows.

## The processor types

The list lives in `../wiki/processors.md`, held against the platform's own
definitions. It is not restated here, so that there is one copy to keep
current.

In practice a handful carry almost everything: `query-database` and
`push-message` dominate the real charts, with `update-context` and
`http-request` appearing wherever a system is reached over HTTP.

## What crosses between processors

| name | holds | when it changes |
|---|---|---|
| `prevPayload` | the tool call's arguments, or the previous message's data | when a request or a new turn arrives - an `http-request` leaves it in place |
| `prevMessage` | the previous user message | on each `listen-message` |
| `prevBlobs` | files the user uploaded | on upload |
| `httpResponse` | the most recent successful HTTP response | after every `http-request` that succeeds |
| `prevError` | why the previous processor failed | on a `failure` branch |
| `vars.<name>` | a `variables` entry, including secrets | never |
| anything an `update-context` set | by the name you gave it | when you set it again |

These are the ones a chain passes along; every key in the channel's context is
in scope, including `prevToolCalls` after an agent, and `../wiki/processors.md`
has the full set.

Configs are evaluated immediately before their processor runs. That is what
makes a two-call chain work - at the second call's config time, `httpResponse` is
still the first call's, because every other processor copies the context
forward when it succeeds - and it is also why inserting another `http-request`
between them loses the value. Evaluation at run time also means a broken
expression is a runtime failure on first use, not a deploy failure: nothing
evaluates configs at apply time.

### `expression` is JavaScript

Arrow functions, `const`, `String()`, `encodeURIComponent`, `JSON.stringify`,
`??`, `.map()` - every real chart uses them, and none of them is CEL.

The platform documentation once described `expression` as a CEL expression
while every chart wrote JavaScript. It now says JavaScript directly, and the CRD
makes no claim either way.

Write JavaScript, and the idiom the charts use is an immediately-invoked arrow
function when there is more than one statement:

```yaml
expression: |-
  (() => {
    const x = prevPayload.thing;
    return { ok: true, value: x };
  })()
```

Note the parentheses around a bare object literal - `({ ok: true })` - without
them it parses as a block.

The config value types are `value` (static string), `expression`, and
`template` (Handlebars, `{{name}}`). One per config.

## The graph

```yaml
relationships:
  - from: {processor: a, relationName: success}
    to:   {processor: b}
  - from: {processor: b, relationName: success}
    to:   {exit: finish}
```

Relation names:

| processor | relations |
|---|---|
| most types | `success`, `failure` |
| `router` | one per condition you define in its configs, plus a built-in `else` |

A `router` is the only type with dynamic relations: each config's name is a
relation, and its expression decides whether that branch is taken.

```yaml
- name: route
  type: router
  configs:
    - name: isVIP
      expression: "userLevel === 'VIP'"
    - name: hasQuestion
      expression: "question != null"

relationships:
  - from: {processor: route, relationName: isVIP}       ; to: {processor: vip}
  - from: {processor: route, relationName: hasQuestion} ; to: {processor: ask}
  - from: {processor: route, relationName: else}        ; to: {processor: default}
```

### Two required rules

1. Every processor that can fail needs a `failure` relationship. Without one
   the run stops there. A tool call that stops has returned nothing, which the
   model reads as an empty answer rather than an error, and then it tells the
   user something that did not happen. This is the most common omission.
2. A tool's last processor reports success or failure explicitly, as a field
   the prompt is told to read (`ok: true|false`). The status code is not visible
   to the model; only what you push is.

## Designing the chain

- Reshape the response before the model sees it. An API's raw JSON has
  fields the model will quote, ids in the wrong type, and nesting it will get
  wrong. One deployment measured that its search API returned numeric ids while
  the downstream tool needed strings, and cast in the workflow. Write down what
  you measured, with the date, because the next person cannot tell a deliberate
  cast from a leftover.
- Validate an enum in `update-context`, not in the prompt. A model that
  half-remembers an allowed value produces a plausible wrong one, and the API
  answers 400 to something that looks fine in the log.
- Name processors for what they do, not their type. `proc-search` beats
  `http-request-1` in a nine-node graph, and `labels.display_name` is the node's
  title on the canvas with `labels.description` as the line under it. Both are
  optional - a node with neither is drawn under its own `name` - so they are
  written for the person who opens the graph later rather than for any check,
  and the reference charts write them as a pair on entries, exits and
  processors. The runtime reads no label key on any workflow element - the
  maps are open in the CRD and asgard-core `478cf5d6` never indexes one - so
  these two are taught because the charts and the documentation's examples
  carry them, for the canvas, and nothing else is. Not
  `entries.labels.default`: the entry a conversation starts at is the
  BotProvider's `spec.entrypoint.entry`, matched by exact name, and a name that
  matches no entry fails the request - there is no automatic default, one entry
  or several (asgard-core `internal/bpcontroller/server/bp_controller.go`). Not
  `relationships.labels` either, where a generator writes a `relationship_id`
  that nothing reads.
- Keep one Workflow to one job. Two jobs in one graph share a failure path,
  and then one job's error message answers the other job's caller.

## Verify

    asgard-cli verify <project>

It resolves every reference into and out of this Workflow - including both halves
of an entrypoint, because a wrong entry name is as dead as a wrong workflow name
- and requires the workflow-set labels and the display annotation, without which
the Platform UI has nothing to list.

What it cannot check, and what to check by hand:

- that every `failure` has a relationship. Nothing enforces it.
- that expressions parse. They are strings until called. Render the chart and
  read them, and exercise the tool once against the real system.
- that a `router` covers its cases. A value matching no condition and no
  `else` stops the run.
- that a conversation loop reaches an exit. It does not have to and should
  not be made to: every supervisor in the reference deployments loops back to
  `listen-message` and declares `exits: []`, and so does what
  `asgard-cli add flowagent --supervisor` writes. A run ends when its terminal
  processor finishes. `channelMaxIdleMs` is not the mechanism somebody reaches
  for here either - asgard-core `623ceb50` types it as the bound on the Redis
  message-history cache and says explicitly that it is not the channel
  or transcript lifetime; leaving it unset means the durable channel never
  expires, which is intended, not a leak.
