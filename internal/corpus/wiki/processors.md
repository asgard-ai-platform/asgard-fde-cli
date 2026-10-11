---
group: While building
description: what each processor type takes, and the fields that decide behaviour
---
# The processors, and the fields that decide behaviour

`../wiki/workflow.md` says which processors exist and how they wire
together. This page says what each one takes. The per-processor
documentation - a page each, and an introduction - had not been read into this
material before, so a chart author writing a Workflow was working from a type list.

Every processor except one has a Failure output. Only
`update-context` documents Success alone; `http-request` fails on
a 4xx or 5xx status or a network error and produces `prevError` - a 2xx or 3xx
other than 200 is Success - and `query-database`,
`retrieve-knowledge`, `execute-script` and `push-message` all document one too.
Draw the error branch.

`ProcessorDefinitions`' `StaticRelationships` is incomplete, so do not treat it
as the contract. It gives some processors a Failure relation and the rest
Success or nothing, and it is wrong in both directions: it gives
`listen-message` no relationships at all while every production chart continues
from one, and `http-request` Success only while production charts in two
repositories route `failure` off it and the documentation describes that branch
in full.

That list is not the only incomplete part of `ProcessorDefinitions` - the
config keys are another. Treat it as what the definitions
declare, not as what the platform accepts, and when the two disagree the
deployed charts win. Do not build a check on it.

## Every field is one of three kinds of value

The per-processor pages all assume this vocabulary, and `workflow.md` says
only that one is JavaScript and one is Handlebars:

    Literal      a fixed value
    Expression   JavaScript, evaluated per run. `prevMessage || '訪客'`
    Template     Handlebars, for producing text. `{{#if prevMessage}}...{{/if}}`

Expression fields are not limited to ECMA5. A shipped chart shows it:
`prevBlobs.map(b => b.blobId).join(',')` evaluates in production, so arrow
functions work.

To use `const` in an expression field, wrap it in an IIFE. This tool's own
generator writes that form and several deployments use it:

    expression: |-
      (() => {
        const body = (httpResponse && httpResponse.json) || null;
        return { raw: body };
      })()

The field holds one expression, so a bare `const x = 1` is not valid there. A
function body holds statements, so wrapping one is how to write anything longer
than a ternary.

`execute-script`'s Engine takes `ECMA5`, the only value, and the documentation
reads that as a language level. It is a name: the script runs in the same goja
VM the expression fields use, so its body accepts what an expression accepts,
arrow functions included
(asgard-core `478cf5d6` `internal/processor/task/execute_script.go` and
asgard-core `internal/bpcontroller/eval/value.go`). The body is statements rather than one
expression, and it reads its input as `payload`.

### The variables in scope

| variable | type | what it is |
|---|---|---|
| `prevMessage` | string | the previous user message |
| `prevBlobs` | array of Blob | files attached to it |
| `prevPayload` | object | the payload from whatever called in - this is what a BotProvider passes through, and what `pluginNames` and `sourceSetMounts` expressions read |
| `prevError` | | the previous step's error, on a Failure branch |
| `customChannelId` | string | the conversation key, chosen by the caller |
| `customMessageId` | string | the message id, optional |
| `prevToolCalls` | array | what the agent just called, and what came back - from the last LLM processor, always an array, empty when it called nothing. Not in the documentation |
| `userIdentityHint` | string | the caller's `X-ASGARD-USER-IDENTITY-HINT` header, `primary` when it sent none. Not in the documentation |
| `completionModelUsage` | object | the last LLM processor's `completionModelName`, `isPreset`, `inputTokens`, `outputTokens`, `totalTokens`. Not in the documentation |
| `vars` | object | the Workflow's own `variables`. Not in the documentation |

The scope is open. Every key in the channel's context becomes a variable, so
the rows above are only what the platform itself writes.
A processor adds its own: `update-context` writes each key it is configured
with, `http-request` writes `httpResponse`, `query-database`,
`retrieve-knowledge` and `generate-embedding` write under their `resultField`,
and an LLM processor with structured output writes each of its top-level keys.
In scope beside them are the helper functions `urlEncode`, `vecToStr`,
`xpathExtract`, `toToon`, `history`, `historySize`, `isoNow`, `isoToday` and
`validateVegaV5Spec` - asgard-core `478cf5d6`
asgard-core `internal/bpcontroller/eval/context.go` (`BuildEvalContext`) and
asgard-core `internal/bpcontroller/eval/value.go`, with the names in
asgard-core `internal/constants.go` and `prevToolCalls` written by
asgard-core `internal/processor/driverloop/driverloop.go`.

    interface Blob {
      blobId: number; fileType: FileType; fileName?: string;
      size: number; mime: string;
    }
    type FileType = 'BINARY' | 'IMAGE' | 'VIDEO' | 'AUDIO' | 'DOCUMENT'

`prevToolCalls` is undocumented, and it is what makes post-processing possible.
The documentation's variable page does not list it. Each entry carries the
tool's name, the arguments it was called with, and its result:

    prevToolCalls[i].toolsetName       the Toolset the tool belongs to
    prevToolCalls[i].toolName          the tool that was called
    prevToolCalls[i].parameter.<arg>   the arguments, by their own names
    prevToolCalls[i].output.isSuccess  whether it worked
    prevToolCalls[i].output.data       what it returned

So a step after the model can check whether the agent called a given tool and
whether it worked. A chart can react to what an agent did without asking the
model to report it. The idiom in the chart it came from is
`prevToolCalls.filter(c => c.toolName == "x" && c.output.isSuccess &&
c.output.data).length > 0`, and reading `.parameter` back off the same entry is
how the arguments reach the reply.

Any of these variables can be absent, and reading a field of an absent one
throws. The documentation's own examples guard every read, and a chart that
does not will throw at run time on the turn where a user sends no file:

    prevBlobs && prevBlobs[0] && prevBlobs[0].fileName
    prevPayload && 'property' in prevPayload ? prevPayload.property : '預設值'

### The built-in functions

| function | what it does |
|---|---|
| `history(start, end)` | conversation history as plain text, one line per turn. Indices are inclusive and negative counts from the end - `history(0, -1)` is everything |
| `historySize()` | how many turns there are |
| | `prevMessage` against `history` is the choice: one turn, or context. `history(-3, -1)` is the last three, and an echo or a single lookup wants neither |
| `urlEncode(s)` | for building a URL in an `http-request` |
| `xpathExtract(...)` | pull a value out of XML or HTML |
| `vecToStr(...)` | a vector as a string |
| `isoNow()` / `isoToday()` | the timestamp and the date |

`history` affects the design. Feeding a whole conversation into a prompt is one
call, and it also brings a run to the three-minute ceiling and the context
window sooner. Call `historySize()` first, then use a bounded window.

## The two LLM processors have different jobs

They share almost every field and differ in these:

| | `llm-completion` | `stream-llm-completion-message` |
|---|---|---|
| Output Schema | required | not present |
| Await | not present | present |
| Payload / Template | not present | present |
| what it is for | a decision or an extraction the flow then uses | text going to a person, as it is produced |

Choose by whether the output needs a shape. A processor whose result another
processor reads needs a schema and therefore `llm-completion`. A processor
whose result a human reads needs the streaming one, and gets no schema.

`Await`, on the streaming one, is offered by the editor and described by the
documentation, and the platform no longer reads it. Every
`stream-llm-completion-message` blocks until the stream finishes, as if `await`
were on; the key a chart sets is passed through as dynamic config and ignored
(asgard-core `478cf5d6` `internal/models/processor.go`,
`StreamLlmCompletionMessageStaticConfig`, which has no such field). So nothing
after this processor runs while the model is still producing output, whatever
the chart says.

## Where `allowedCubes` and `allowWrite` actually live

Both LLM processors carry the semantic layer configuration, and the warnings in
the extracts refer to these fields:

    Semantic Layer                a layer this processor may use
    Semantic Layer Allow Query
    Semantic Layer Allow Write
    Semantic Layer Allowed Cubes

`Allowed Cubes` is the field the anonymous-audience argument depends on, and it
is easy to misread. It is a key here, so on the flow-agent path a layer can be
narrowed. It does not narrow anything by default: it defaults to empty, meaning
unrestricted, so a processor configured by adding only what you want composes
SQL over every cube, and the surface grows each time a cube is added.
The case against mounting a layer for an anonymous audience is that no user
input should reach SQL at all, not that there is nothing to narrow with.
`../guide/read-path.md` has both halves.

The safe field defaults off and the dangerous one defaults on. In the
definitions, `semanticLayer.allowQuery` defaults to false and
`semanticLayer.allowWrite` defaults to true, on both LLM processors. So a
processor configured by adding only what you want has a write path, and the
rendered chart shows nothing. That is why every deployment writes
`allowWrite: false` out on every entry that has a layer, even where it is
obviously false.

`query-database` carries the same pair and is easier to miss, because the
name reads as a read tool: `allowWrite` defaults to true there too, and its
`allowedTables` is the counterpart of `allowedCubes` - both default to empty,
meaning unrestricted. A scheduled run with a missing `allowWrite` has a write
path into the customer's systems.

The chart keys are `semanticLayer.allowQuery`, `semanticLayer.allowWrite` and
`semanticLayer.allowedCubes`, not the spaced labels the builder shows.

`Toolsets Fault Tolerant` is gone from the platform. A tool that fails is
always handed back to the model as an error result, and the model decides what
to do next - which is what the setting used to turn on
(asgard-core `478cf5d6` `internal/processor/helper/mcp_session.go`). So a failed write is never retried
by the platform, and nothing stops the model retrying it; a write that must not
be repeated says so in its own `tooling.description` and in the prompt.

## `effort` fails the turn when the model does not take it

The reasoning-effort field takes `low`, `medium`, `high`, `xhigh`, `max`, or
`auto` to leave it to the model. It sets thinking depth and token spend
together, so a higher level is slower and costs more.

Sending a level a model does not support fails that turn outright; the answer
is not degraded, the turn fails. Some models do not accept the parameter at
all: a non-reasoning model like `gpt-4.1-mini` rejects it, and the
speed-oriented Haiku class is the other to know.

Omitting the field does not disable it. There are three states:

    a level          `--effort <level>` is sent
    the field absent no `--effort` is sent - and the driver treats a model id
                     it does not recognise as supporting effort, so it supplies
                     a level of its own, on the high side
    `disabled`       no `--effort`, and the spawn declares the capability as
                     unsupported, so nothing is supplied

So a chart on a non-reasoning model must write `disabled` explicitly. Leaving
the field out is how one deployment's token spend went up without anyone
changing a prompt. `effort` is per model rather than per chart: switching
`completionModelName` can turn a working chart into one that fails on every
turn, or one that pays for reasoning nobody asked for.

`input` on `stream-llm-completion-message` is normally left empty. Empty
means the processor uses what the pipe handed it - `prevMessage` in context,
which is what the user actually said. Set it only when the input has been
worked on first: a summary prepended, or the question rewritten into something
clearer.

## Fields that decide something and look like detail

| processor | field | what it decides |
|---|---|---|
| `execute-script` | Engine | only `ECMA5` is accepted, and it is a name rather than a language level: the body runs in the same VM as the expression fields and accepts what they accept, arrow functions included |
| `http-request` | Parse JSON | off by default. On, `httpResponse` gains a `json` field. Off, the body is a string and every downstream expression has to parse it |
| `validate-payload` | Schema (required), File Requirements | this is the entry contract of an automation tool - what the caller must supply, and which file types are accepted |
| `query-database` | SQL Type Arguments | parameterised queries, supplied as extra keys rather than a static field. The alternative is string-building a query, which is the injection surface `../usecase/fixed-query-tools.md` exists to remove. The docs page for this processor is called `query-sql` - see the naming table below |
| `retrieve-knowledge` | Similarity Threshold (required) | there is no safe default to fall back on. Too high returns nothing and looks like an empty knowledge base |
| `retrieve-knowledge` | Filter Tags, Path Exists, Path Predicate | retrieval can be scoped without splitting the knowledge base |
| `generate-embedding` | Result Field (required) | where the vector lands. Every model processor that produces data names its own output field |
| `push-message` | Flush | whether the reply buffer is sent immediately |
| `router` | Else | the unmatched branch. A router without one silently drops what does not match |
| `router` | its own config keys | a router has no fixed fields: each config key you add is a branch name, and its value is a boolean expression. A relationship out of the router carries that key as its `relationName`, and `else` is the static one. See below |

## Defaults the documentation does not give

The type definitions carry a default per config key and the documentation pages
do not, so a field marked 必填 there can still have a value it falls back to -
which changes whether leaving it out is an error or a silent choice.

The three worth knowing before the table:

| processor | field | |
|---|---|---|
| `retrieve-knowledge` | `sampleK` | required, and defaults to 20. How many chunks come back. Nothing in the documentation gives the number, so a retrieval returning "too much" or "not enough" is being tuned against an invisible 20 |
| `validate-payload` | `path` | defaults to `"$"` - the whole payload. Set it to validate a subtree instead |
| both LLM processors, and `query-database` | `allowWrite` | defaults to true, while `allowQuery` beside it defaults to false. The section above is about this one |

`retrieve-knowledge` requires five fields - the knowledge bases, the query
text, the similarity threshold, the result field and `sampleK` - and four of
them have no default at all, so a partially configured one fails rather than
guessing. `stream-llm-completion-message` requires only two, and not
`prompt`, where `llm-completion` requires it: the streaming one is a channel,
and what it says can come from its `input` instead.

### Every processor, its outputs, and what it requires

Extracted from the definitions, so these are the keys a chart writes rather than
the labels the builder shows. `=` marks a required key that also has a
default - omitting one of those is a silent choice rather than an error.

This table is a subset of what a chart may set, not the contract. `await` and
`temperature`, set in production deployments, are in neither
`ProcessorDefinitions` nor the CRD, and the streaming processor reads neither: an
undeclared key reaches the processor as dynamic config, and one that takes none
ignores it. So a key missing from the row below is not an error, and not
necessarily read either - the row lists what is declared. The
wider set is the editor palette, in its own table, "What the editor lets an
author set" below, which has both of those keys and says which keys are the
platform's rather than yours. `asgard-cli verify` reflects this: it fails
on a required key with no default, because that is broken against any
version, and does not complain about a key it has never heard of.

| processor | outputs | extra keys | required keys, with any default |
|---|---|---|---|
| `execute-script` | Success | no | `engine` `script` |
| `generate-embedding` | Success + Failure | no | `embeddingModel` `input` `resultField` |
| `http-request` | Success | **yes** | `url` `method` `parseJson` =false |
| `listen-message` | none | no | *none* |
| `llm-completion` | Success + Failure | **yes** | `completionModel` `prompt` `outputSchema` |
| `llm-query-database` | Success + Failure | no | `semanticLayer` `query` `resultField` `completionModel` `maxTokens` |
| `push-message` | Success | no | `message` ="" `flush` =false `isDebug` =false **(the platform's)** |
| `query-database` | Success | **yes** | `dataConnector` `sql` `resultField` |
| `retrieve-knowledge` | Success | **yes** | `knowledgeBases` `textQuery` `similarityThreshold` `resultField` `sampleK` =20 |
| `router` | Else | **yes**, + branches | *none* |
| `stream-llm-completion-message` | Success + Failure | no | `completionModel` `isDebug` =false **(the platform's)** |
| `update-context` | Success | **yes** | *none* |
| `validate-payload` | Success + Failure | **yes** | `schema` |

`update-context` and `router` require nothing and take arbitrary keys; that is
how they work. On `update-context` the extra key names are the
context variables you are setting, and on `router` they are the cases, with a
dynamic output per case and the static `Else` for what matches none. So
neither has a static field list to look up.

### What an extra key means, per processor

Each processor reads extra keys its own way, and the key name is the
parameter - so a typo does not raise an error; it becomes a different parameter
or is silently ignored.

| processor | an extra key is | the shape |
|---|---|---|
| `update-context` | a context variable you are setting | the key **is** the variable name |
| `router` | a branch | the key is the branch name, the value a boolean or an expression returning one. An unknown value type fails the step |
| `http-request` | an **HTTP header** | the key is the header name. **The value must be a string** or the step fails - a number written bare is a failure at run time, not at render |
| `query-database` | one SQL placeholder | `sql.args.<n>.type` **and** `sql.args.<n>.value`, both, numbered **from 1** |
| `retrieve-knowledge` | one JSON-path filter | `path.<n>.exists` or `path.<n>.predicate`, each series numbered **from 0** and counted independently |
| `validate-payload` | one file requirement | `file.<n>.type`, `file.<n>.alias`, numbered **from 0** |
| `llm-completion` | **nothing.** It declares dynamic config and no code reads it | an extra key here is accepted and ignored |

The numbered ones stop at the first gap. Each is a loop that breaks as soon as
an index is missing, so `sql.args.1.*` and `sql.args.3.*` with no `2`
sends one argument, not two, with no warning. The same holds for a
`file.0.type` with no `file.0.alias` where the alias series is read separately,
and for `path.<n>.*`. Renumber after deleting one.

Two of them start at 0 and one starts at 1. There is no rule behind it; it is
per processor, and getting it wrong on `query-database` gives an argument list
that is silently empty.

`sqlTypeArguments` is also an extra key. The documentation gives
`query-sql` a SQL Type Arguments field for the `$1`, `$2` placeholders, and
there is no such static key - the parameters are dynamic config on the
processor. Not finding the key in the type definitions does not mean the
documentation is wrong.

The documentation and the definitions still disagree about what is required on
the streaming processor: its page marks MaxTokens（必填） where the definition
has `maxTokens` optional with no default, and `Temperature` 可選 where the
definition has no such key on that processor at all.

So neither side is reliable for requiredness. The definitions are
what the runtime validates against - that is what `asgard-cli verify` reflects -
and a field the documentation calls 必填 may be one the platform accepts without.
Write it out anyway: reviewers will hold a chart to a page that says 必填.

## What the editor lets an author set, which is a third source

Three sources describe a processor's configuration and they do not agree.
The CRD enum says which types exist; `ProcessorDefinitions` in asgard-core says
which keys are declared; and the editor palette says which keys an author
can actually set in the builder. asgard-docs has a page per processor, each
verified against the palette rather than only against the code, and states why
the code alone is not enough: the definitions "omit the
section a `has_dynamic_config` processor generates, and list keys the editor
hides".

The table below is the palette's view. `author` is what somebody filling in a
node sees; `platform` is set for them and is not theirs to write.

| type | author sets | platform sets | dynamic | scope |
|---|---|---|---|---|
| `execute-script` | `engine` `script` | - | no | general |
| `update-context` | *none* | - | **yes** | general |
| `http-request` | `url` `method` `parseJson` `body` | - | **yes** | general |
| `router` | one per branch | - | **yes** | general |
| `listen-message` | *none* | - | no | general |
| `push-message` | `message` `template` `flush` `payload` | `isDebug` | no | **bot, automation_tool** |
| `validate-payload` | `schema` `path` | - | no | **automation_tool** |
| `generate-embedding` | `embeddingModel` `input` `resultField` | - | no | general |
| `llm-completion` | `completionModel` `prompt` `outputSchema` `maxTokens` `temperature` `effort` `toolsets` `semanticLayers` `openai.webSearch.enabled` `openai.webSearch.searchContextSize` `sandboxBlueprint` | `blobs` `semanticLayer` `semanticLayer.allowQuery` `semanticLayer.allowWrite` `semanticLayer.allowedCubes` | no | general |
| `stream-llm-completion-message` | the same, plus `input` `await` `semanticLayers.dataVisualization`, minus `outputSchema` | the same, plus `isDebug` | no | general |
| `query-database` | `dataConnector` `sql` `resultField` | `allowWrite` `allowedTables` | **yes** | general |
| `retrieve-knowledge` | `knowledgeBases` `textQuery` `similarityThreshold` `resultField` `filterTags` `sampleK` | - | **yes** | general |
| `llm-query-database` | `semanticLayer` `query` `resultField` `completionModel` `maxTokens` `temperature` | - | no | **not in the palette** |

Four things in it that are not anywhere else:

`await` and `temperature` are author keys on the streaming processor in the
palette, and production deployments set them. The `model-stream-llm-completion`
page has an Await section, a Temperature section and a worked example that
writes `await` as a config key. The platform reads neither on that processor:
`await` was removed from it, and `temperature` is read only by
`llm-query-database` (asgard-core `478cf5d6` `internal/models/processor.go`).
So the palette and the documentation show what an author can type, not what
runs; the task implementation is what settles whether a key is read.

`llm-query-database` is in the CRD and in the definitions and not in the
builder. An author cannot add it from the editor; a chart can still declare
it. Nothing says whether that is deliberate.

A processor is scoped to a kind of workflow set. `validate-payload` is
`automation_tool` only, `push-message` is `bot` and `automation_tool`, and
everything else is `general`. So `push-message` is the same CRD type reached
from two different places, which is why the documentation has two pages for it -
`message-push` and `automation-tool-response`.

`isDebug`, `allowWrite`, `allowedTables` and the `semanticLayer.*` keys are
set by the platform, not the author. The definitions list `isDebug` as a
required key with a default of false, which reads as something to write out; the
palette says the platform sets it. The same holds for the per-layer permissions
- see "Where `allowedCubes` and `allowWrite` actually live" above, which the
palette confirms.

The definitions under-report Failure branches, and the palette agrees with
the documentation against them: `execute-script`, `http-request`,
`push-message`, `query-database` and `retrieve-knowledge` all have one. The row
in the table above this section says Success alone for several of those, because
it was extracted from the definitions. Read the definitions as declared, not as
complete.

For a field's meaning, read the documentation page for that processor. Each
carries the property panel as a screenshot, every field's default, and a worked
example - `https://docs.asgard-ai.com/docs/developer-reference/processor/<name>`,
where `<name>` is the documentation's name and not the chart's. The next section
lists where the two differ.

## The documentation's names differ from the chart's names

A page per processor sits under `developer-reference/processor` as of asgard-docs
`23409b3`, plus an introduction, and one of them is new since `f00e0ee`. The
new one is `query-llm-database`, which is the one type that is documented and
is not in the editor palette: a chart can declare it, an author cannot add it
from the builder. The CRD enum is a list of its own. They do not line up, and at
each mismatch a search for the name you read finds nothing:

| the page is called | the chart writes |
|---|---|
| SQL, at `processor/query-sql` | `query-database` |
| Entry, at `processor/entry` | *not a processor* - `spec.entries` |
| Exit, at `processor/exit` | *not a processor* - `spec.exits` |
| Response, at `processor/automation-tool-response` | `push-message`. There is no `response` type - an Automation Tool's final output is the same processor a bot replies with, scoped to `automation_tool` |
| LLM Database, at `processor/query-llm-database` | `llm-query-database` - the two words are swapped, so the page and the type do not find each other |
| MCP Servers, the field label on both LLM processors | `toolsets`, a comma-separated list of Toolset names |

### And a page has a third name: the file it is in

Some processor pages are served at a URL that is not their file name.
A page declaring a `slug:` in its frontmatter answers at that slug, so
`flow-entry.mdx` answers at `processor/entry`, `message-push.mdx` at
`processor/push-message`, `model-stream-llm-completion.mdx` at
`processor/stream-llm-completion`. The file is named for the processor's family
and the URL for the builder's label.

This matters in two ways. A link built from a file name 404s; the Entry row in
the table above once had a URL with `flow-` on the front of it for that reason.
And a grep of the docs repository finds the family name, so searching it for
`entry` finds a file called something else.

The documentation's own index page links by URL and gets them right. Its
category headings are the node menu's, which is a fourth naming of the same
things and the one an author actually sees:

    流程控制    Entry, Exit, Router
    Message     Push Message, Listen Message
    Model       LLM Completion, Stream LLM Completion Message, Generate Embedding
    Action      Update Context, Execute Script
    Query       SQL, Retrieve Knowledge
    API         HTTP 請求
    Automation Tool   Validate Payload, Response - not in the Flow Agent
                menu at all, only in an Automation Tool workflow
    CRD only    LLM Query Database

`llm-query-database` is the one type the editor will not add. It
takes `semanticLayer`, `query`, `resultField`, `completionModel` and `maxTokens`
- all five required - plus an optional `temperature`. It is the processor for
"let a model answer this question against the layer" as a single step, where
the alternative is an `llm-completion` with a layer mounted and a prompt. It
has no `allowQuery`, no `allowWrite` and no `allowedCubes`, so the field the
read-path argument depends on does not exist on it, and its access is whatever
the SemanticLayer itself permits. Check that before choosing it as the safer
option: it is not obviously safer, only scoped in a different place.

## Router at scale is a chain of routers

A common wrong model is "a router is a switch with N branches".
The largest use of routers read here - a content-generation deployment's, across
two workflows - has a different shape. Each router asks one boolean
question and has one named branch, and its `else` goes to the next router.

    proc-route-if-vscode-open-file  is-true -> push the "open in VSCode" CTA
                                    else    -> proc-route-if-download-link
    proc-route-if-download-link     is-true -> push the download CTA
                                    else    -> the next one

Written out, the mechanism is:

  - the router's config key is the branch name, and its value is an
    expression returning a boolean. `is-true` is a name somebody chose, not a
    keyword
  - a relationship out of the router names that same key as its `relationName`
  - `else` is the one relationship the type declares statically

So each branch costs one processor, and branches compose by chaining.
Each question is independent of the others, several can fire in one turn, and
adding a sixth means adding one router and re-pointing one `else` rather than
editing a nine-way condition.

### When a branch belongs in the graph rather than in the prompt

FDEs often get this decision wrong, and the deployment shows the answer.
Every one of those routers runs after the model has finished, and asks about
`prevToolCalls` - did the agent call this tool, did it succeed. None of them
asks the model anything.

Put a branch in the graph when it depends on a fact about what happened, and in
the prompt when it is a judgement about what to say. "Did the agent
successfully call `vscode_open_file`" is a fact and can be checked, so a chart
that checks it is right every time, while a prompt asking the model to
remember to mention the file is right most of the time. The reply the router
builds also reads `.parameter.absolute_path` back off the tool call, so the
content of the message comes from the call rather than from the model's
recollection of it.

The test: if you would have to ask the model whether something happened, that
branch belongs in the graph.

## Entry, Exit and Router

Neither Entry nor Exit is a processor type. The documentation files them under
`developer-reference/processor` and the builder draws them as nodes, but neither
is in the CRD enum: a Workflow carries
`spec.entries` and `spec.exits` as their own lists, siblings of
`spec.processors`. An entry is `{name, handlingProcessor}` - a named way in that
points at the processor which handles it - and an exit is a named end: `name`,
optional `labels` for what the canvas shows, and an optional `handlingWorkflow`
pointing at another workflow's entry. A run reaches one because a relationship
says so rather than because a processor finished - `../usecase/workflow-chain.md`
has that shape. A chart has no `flow-entry` processor to search for.

Two things worth knowing from their pages:

  - a workflow can have several entries, which is how one Workflow serves
    more than one caller shape
  - an exit carrying a `handlingWorkflow` connects workflows to each other,
    which is the mechanism behind `../usecase/workflow-chain.md`. One without it
    is only a name on the canvas

## Corresponding extracts

`../usecase/external-api.md` uses `http-request` field by field;
`../usecase/workflow-chain.md` uses `router` and the entry/exit connection;
`../usecase/fixed-query-tools.md` uses `query-database`.

No extract uses `retrieve-knowledge`, because of how the material is shaped:
`../usecase/knowledge-drive.md` is a Drive with a Context Index, which an agent
queries as a knowledge graph from its sandbox rather than through a processor,
and `../usecase/knowledge-base.md` - the RAG shape this processor retrieves
from - is written for reading an older chart and stops at the four CRs.
`../wiki/knowledge.md` is which of the two a design starts from.

## Sources

- The value types, variables and functions:
  [expression-introduction](https://docs.asgard-ai.com/docs/developer-reference/asgard-builtin/expression-introduction),
  [expression-variable](https://docs.asgard-ai.com/docs/developer-reference/asgard-builtin/expression-variable),
  [expression-ecma-script-functions](https://docs.asgard-ai.com/docs/developer-reference/asgard-builtin/expression-ecma-script-functions)
  - asgard-docs `23409b3`, which documents `ECMA5` as a limit; the runtime treats it as a name. What moved since `f00e0ee` is a heading
  anchor and nothing else
- The pages under `developer-reference/processor/`, whose landing page is
  [introduction](https://docs.asgard-ai.com/docs/developer-reference/processor/introduction)
  - the bare directory URL 404s; cite the introduction, not the directory
  - asgard-docs `23409b3`. The introduction was rewritten at
  that commit to follow the editor's node menu rather than the source, which is
  where the group names, the two Automation Tool nodes being absent from the
  menu, and `llm-query-database` being CRD-only all come from
- `effort`'s levels, that an unsupported one fails the turn, and that an empty
  `input` falls back to `prevMessage`: asgard-docs `23409b3`
  `docs/developer-reference/processor/model-llm-completion.mdx` and
  `model-stream-llm-completion.mdx`
- The three states of `effort`, and that omitting it is not disabling it:
  read off a production deployment, which carries
  `defaultEffort: "disabled"` with the reasoning in its own values file and in
  `cm-gpt-4.1-mini.yaml`, and cites asgard-core `internal/processor/clidriver/options.go`.
  Confirmed there at `623ceb5`:
  `disabled` becomes a `ModelCapabilities` declaration on the spawn rather than
  a flag, and unset makes the CLI "supply its own default level for any model it
  believes supports effort, and it believes that of every model id it cannot
  recognize (which is all of ours)". This is the one part of it seen to
  fire - that deployment's token spend was the symptom. The documentation says
  the same thing more softly (未設定時採用模型預設值), which is why the strong form
  is sourced to the driver
- The editor palette per processor - which keys are the author's, which the
  platform sets, which types accept dynamic config, and which workflow-set
  types each is scoped to: asgard-docs `23409b3`, from every
  `metadata.json` file under
  `content-generator/services/developer-reference/docs/processor/`. Those
  record the palette as a third source beside the CRD enum and asgard-core's
  definitions, and the pages are verified against it rather than only against
  the code. This is the only part of this page at that commit - the
  prose above it is at `f00e0ee`
- `ProcessorDefinitions` in asgard-core `internal/constants.go`: the per-key
  `IsRequired` and `DefaultValue` the documentation does not carry, the
  Success/Failure declarations, and which processors take arbitrary extra keys.
  Extracted at asgard-core `623ceb5` by walking that literal and resolving the
  key constants to their string values. Both tables on this page are held
  against that literal mechanically
  - and it is incomplete: checked against the task implementations and the
  rendered production charts
- What an extra key means, per processor: read at asgard-core
  `623ceb5` off the task implementations themselves - one file per processor
  under asgard-core `internal/processor/task/`. The definitions say only
  whether a processor takes dynamic config; the key shapes, the two different
  starting indices and the break-at-the-first-gap behaviour are in the loops that
  read them, and nowhere else. `llm-completion` declaring dynamic config that no
  code reads was found the same way
- The Failure outputs: the documentation, one page per processor, after the
  type definitions were found to disagree with four production charts. Checked
  across `api-http-request`, `query-sql`, `query-retrieve-knowledge`,
  `action-execute-script`, `message-push` and `action-update-context` - all but
  `update-context` document a Failure branch
- `prevToolCalls`, the router branch mechanism and the cascade shape: read
  off `asgard-auto-post-kube`'s two agent workflows, which are the
  only charts anywhere in the reference set that use `prevToolCalls` - every use
  post-processing. It appears in no documentation page and in
  no other deployment
- The variables in scope, and that the scope is every context key: asgard-core
  `478cf5d6`, the files named under the table
- The `ProcessorType` enum in asgard-kube
  `pkg/apis/asgard/v1alpha1/types.go`, and `WorkflowSpec` beside it, which is
  what settles that entries and exits are not processors. Held against
  asgard-kube `cbd8d70` for Exit's own fields - `handlingWorkflow` is optional -
  and for the
  exactly-one-of on a relationship's `to`, which is how a run reaches one

**Checked:** the Failure branch on `http-request` and what counts as a failure,
`parseJson`, the static config the streaming processor reads, `input` falling
back to `prevMessage`, `temperature` being read only by `llm-query-database`,
the `ECMA5` engine, `history()` and the removed fault-tolerance setting, against
asgard-core `478cf5d6` `internal/processor/task/http_request.go`,
asgard-core `internal/processor/task/execute_script.go`, `internal/models/processor.go`,
asgard-core `internal/bpcontroller/server/bp_controller.go`, `internal/bpcontroller/eval/value.go`
and asgard-core `internal/processor/helper/mcp_session.go`.

**Unchecked:** the palette is at second hand, from asgard-docs'
`metadata.json` files; the file they cite, `asgard-ai-platform-web`
`src/components/react-flow/workflow/processors.json`, is in a repository
nothing here has a clone of.
