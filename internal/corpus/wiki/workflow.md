---
group: While building
description: the processor types against the editor's groups; Expression is JavaScript, Template is Handlebars
---
# Workflow and Processor

A Workflow is the program that actually runs. In the UI it is a diagram; in a
chart it is the `Workflow` CR's `entries` / `exits` / `processors` /
`relationships`.

## Processor types

The values in the CRD's `ProcessorType` enum, against the group each sits
under in the editor's node menu:

| group | node, as the menu labels it | `type` |
|---|---|---|
| 流程控制 | Router | `router` |
| Message | Push Message | `push-message` |
| | Listen Message | `listen-message` |
| Model | LLM Completion | `llm-completion` |
| | Stream LLM Completion Message | `stream-llm-completion-message` |
| | Generate Embedding | `generate-embedding` |
| Action | Update Context | `update-context` |
| | Execute Script | `execute-script` |
| Query | SQL | `query-database` |
| | Retrieve Knowledge | `retrieve-knowledge` |
| API | HTTP 請求 | `http-request` |
| Automation Tool | Validate Payload | `validate-payload` |
| CRD only | LLM Query Database | `llm-query-database` |

Entry and Exit are nodes in that menu and are not processor types. The
editor draws them under 流程控制 and the documentation gives each its own page,
but a Workflow declares them as its own `entries` and `exits` arrays - so
looking for an `entry` value in the enum finds nothing, and a processor written
with one is rejected. Two more rows of the menu are not general either:
Validate Payload and Response appear only in an Automation Tool workflow, and
Response has no type at all - the output is a `push-message` scoped to
`automation_tool`, which `../wiki/processors.md` sets out. Validate Payload is
still in the menu and is deprecated: the entry's `inputSchema` replaces it. `llm-query-database`
is the opposite case: a legal type the node menu will not add.

At most 100 processors and 1000 relationships.

`stream-llm-completion-message` is the processor agent conversation runs on:
semanticLayers, toolsets and sandboxBlueprint all hang off its config.

## Three ways a value is set, exactly one at a time

Every config value is a Literal, an Expression or a Template, and the CRD
enforces exactly one.

| form | syntax | for |
|---|---|---|
| Literal (`value`) | plain text | something fixed |
| Expression (`expression`) | JavaScript | computation and logic |
| Template (`template`) | Handlebars | rendering |

Expression is JavaScript, not CEL, and it is not restricted to ECMA5 -
that limit is `execute-script`'s Engine field and applies to a script body, not
to these. One shipped tenant chart evaluates
`prevBlobs.map(b => b.blobId).join(',')`, which is the evidence;
`../wiki/processors.md` says why a deployed expression is the evidence to keep
rather than a tally of how many use one. `||`, `??`, `String()`
and `encodeURIComponent` work, as
do built-in helpers such as `history(0, -1)` and `urlEncode(...)`.

`const` and `let` need a wrapper. An Expression field holds one expression rather than statements, so a
bare declaration has nowhere to go - the charts write an immediately-invoked
arrow function around the statements instead, which is the form
`../wiki/processors.md` sets out. If you want statements without one, that is
`execute-script`, whose body runs in the same VM.

An expression written as CEL does not work and gives no error saying why. The
current documentation and the charts agree that Expression is JavaScript.

## Reading conversation context

Two built-ins with different reach:

| | returns | for |
|---|---|---|
| `prevMessage` | the previous message | echo, single-turn lookups |
| `history(start, end)` | a slice of the conversation | multi-turn, anything needing context |

```javascript
history(0, -1)    // everything
history(-3, -1)   // the last three
history(-1, -1)   // just the previous one
```

An LLM chatbot answering in context wants `history`; `prevMessage` only covers a
single turn.

## What crosses between processors

How `prevPayload` and `httpResponse` actually behave - in particular that an
`http-request` adds `httpResponse` and leaves `prevPayload` as it was - is in
`../usecase/workflow-chain.md`.

## Entry and tooling

A Workflow entry can carry a `tooling` block (name, description,
allowUploadFile). With it, a `Toolset` can expose that entry as a tool.

`tooling.description` is the only text the model reads when choosing between
tools.

An entry can also carry an `inputSchema` (a JSON Schema string) validating the
incoming payload, and `files` (per-slot type limits and context aliases). The
platform checks both before any processor runs, so they are the contract of a
tool Workflow, and they replace the deprecated `validate-payload` processor -
`../wiki/processors.md` has why. On a tool, the `inputSchema` is also what the
model is shown as the tool's parameters.

### Editing an inputSchema

On a processor with a Schema field, the expand icon at the top right of the
editor opens a window that switches between Schema and JSON modes with live
two-way preview.

The editor does not save. Return to the processor's settings page and press save,
or the change never reaches the system.

## Platform built-ins

Two further reference sets live under `developer-reference/asgard-builtin/` and
are worth reading when needed rather than summarising here: the Expression
variable and ECMAScript function lists, and the Handlebars helpers plus the
message templates - text, hint, image, video, audio, location, button, carousel
and chart, with quick replies and the emit, message and uri action objects as
parts they share. Hint does not show quick replies even when they are set.

The runtime and the SDK carry four more template types the documentation does
not cover - table, attachment, question and canvas. They are what the
platform's own card tools emit (`../wiki/tools.md`), so a chart does not need to
write one, and a client built on the SDK has to render them.

## Before writing the chart

`../usecase/workflow-chain.md` covers what actually passes between
processors; `../usecase/fixed-query-tools.md` is the shape of a zero-parameter query tool.

## What goes in a field

Every processor field takes one of three kinds of value - Literal, Expression
(JavaScript) or Template (Handlebars) - and the variables in scope, the
built-in functions and the `Blob` shape are in
[`processors`](../wiki/processors.md). `execute-script`'s `ECMA5` Engine value
is a name, not a limit: its body and every Expression run in the same VM, and
one deployed chart evaluates an arrow function. `const` and `let` need the immediately-invoked wrapper described
above, which is structural rather than a limit - the field holds one
expression, not statements.

## The editor's canvas is a ConfigMap

A Workflow renders as a diagram in the platform's editor, and where each node
sits is not stored on the Workflow. It is a plain Kubernetes `ConfigMap` holding one
key:

```yaml
kind: ConfigMap
metadata:
  name: cfgmap-<workflow>
data:
  node_positions: |-
    { "workflow": {"x":40,"y":40},
      "entry":     {"main": {"x":40,"y":140}},
      "processor": {"submit-claim": {"x":420,"y":140}, ... },
      "exit":      {"finish": {"x":1180,"y":140}} }
```

The Workflow binds it by annotation - `asgard-ai.com/workflow-config-name`.

Do not write one for a new chart. The platform lays the graph out itself. A
ConfigMap like this in a chart you are reading is an older shape and does not
need copying, and hand-written positions go stale once anybody edits the graph.

What still decides whether a chart-authored Workflow is usable in the UI is
the label, and a missing label fails silently:

    project-environment-id missing   the editor opens as a blank canvas

One deployment carries one per Workflow. `ConfigMap` is not an
Asgard CR and appears in no CRD, which is why nothing else here mentions it.

## Sources

- Every file under
  [processor](https://docs.asgard-ai.com/docs/developer-reference/processor/introduction)
  - asgard-docs `23409b3`. The type table above is the CRD's
  `ProcessorType` enum at asgard-kube `3da0365` against that page's groups; the
  two are not the same list, which is why the page says what Entry and Exit are
- [Expression forms](https://docs.asgard-ai.com/docs/developer-reference/asgard-builtin/expression-introduction)
  - asgard-docs `23409b3`
- [Architecture](https://docs.asgard-ai.com/docs/developer-reference/architecture)
  - asgard-docs `f00e0ee`
- [Conversation context](https://docs.asgard-ai.com/docs/help-community/other/retrieve-conversation-context)
  and its near-duplicate `compare-conversation-context-retrieve-method`
  - asgard-docs `f00e0ee`
- [JSON Schema editor](https://docs.asgard-ai.com/docs/help-community/other/json-schema)
  - asgard-docs `f00e0ee`
- Processor list, the limits and the exactly-one rule: checked against
  [asgard-kube](https://github.com/asgard-ai-platform/asgard-kube) `3da0365` -
  `ProcessorType`, `WorkflowSpec`, `crd/asgard-ai.com_workflows.yaml`
- Message templates: asgard-docs `21c920f6`,
  `docs/developer-reference/asgard-builtin/message-template-*.mdx`;
  asgard-core `478cf5d6` `internal/constants.go` (`MessageTemplateType`);
  asgard-js-sdk `56ad14e` `packages/core/src/constants/enum.ts`
- Expression being JavaScript: from both sides - the product
  documentation states it, the CRD makes no claim, and the deployments use
  JavaScript constructs throughout

**Checked:** the processor list and value forms against asgard-kube `3da0365`,
the message templates against asgard-docs `21c920f6`, asgard-core `478cf5d6` and
asgard-js-sdk `56ad14e`, as listed under Sources.
