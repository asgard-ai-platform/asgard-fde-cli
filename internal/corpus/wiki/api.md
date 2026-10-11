---
group: While building
description: the endpoint and its actions, the SSE sequence, the integration patterns, the SDK
---
# API, SSE and the SDK

The whole path for reaching an agent from your own front end. Maps to a
`BotProvider` of class `generic`.

## The endpoint

Starting a channel, sending a message, uploading a file and sending a file all go
to the same endpoint:

```
POST {base_url}/ns/{namespace}/bot-provider/{bot_provider_name}/message/sse
Header: X-API-KEY
```

Hand a front end this URL, with no `/generic/` segment. The platform
registers both shapes, and they are different routes. The one above is the
current route: it checks the Workspace's subscription, meters the request, and
is the only one with `/message/feedback`, `/metadata`, `/channel/metadata` and
`DELETE /channel`. The `/generic/ns/...` form the API reference prints is a
legacy route that reaches the same handler without the subscription check
or the metering, and has none of those extra endpoints - asgard-core `478cf5d6`
asgard-core `internal/edgeserver/component/gin.go`, where the two groups are labelled
"legacy routes" and "modern BP routes". The SDK appends `/message/sse` to the
`botProviderEndpoint` it is given and adds nothing else (asgard-js-sdk
`packages/core/src/lib/client.ts`), so it reaches whichever one the caller
passes.

The BotProvider's name and namespace are in the chart, which is where the URL
is assembled from.

The header is `X-API-KEY`, and `X-Asgard-Webhook-Token` is accepted in its
place - the same check reads both
(asgard-core `internal/edgeserver/middleware/bot_provider.go` `GenericBotProviderAuth`). A
customer's relay that sends the second is using the platform's own alternative,
not a convention of its own.

They differ by the `action` field:

| action | `action` | `text` |
|---|---|---|
| start or reset the channel | `RESET_CHANNEL` | empty |
| send a message | `NONE` | the message |

The platform takes two more: `RESPONSE_TOOL_CALL_CONSENT`, which answers an
`asgard.tool_call.consent` event, and `NUDGE`, a turn that only advances the
workflow and produces no reply (asgard-core `internal/models/generic.go`).

Request parameters:

| field | |
|---|---|
| `customChannelId` | required, and yours to choose. Conversation memory is keyed on it |
| `customMessageId` | optional, for tracing and debugging |
| `text` | the message |
| `action` | see above |
| `blobIds` | optional, the uploaded files this message carries |
| `payload` | optional, an object the workflow receives alongside the text |

`customChannelId` is the only thing tying a conversation together. A front end
that generates a fresh id per message makes every message a new conversation.

`RESET_CHANNEL` discards the conversation history; it does not mean "open the
chat". A front end built from only the two actions above tends to send
`RESET_CHANNEL` when the widget mounts, and every page reload then destroys the
conversation the customer was having. It gets reported as the agent forgetting,
not as a bug. `RESET_CHANNEL` also cannot carry `blobIds`: the reset deletes
every file uploaded to the channel first, so the platform rejects the request
(asgard-core `internal/edgeserver/handler/dispatch.go`).

Rejoining an existing channel is a different path. The SDK's `Channel.restore`
asks `GET /channel/metadata` whether the channel exists and, if it does,
replays the server's transcript rather than resetting - a cold-start rejoin is a
`GET .../message/sse` carrying the channel id and no message. `Channel.reset` is
for a conversation the user has deliberately started over, and it does not send
`RESET_CHANNEL`: it calls `DELETE /channel`, waits for it, then opens the same id
with an ordinary `NONE` turn (asgard-js-sdk `packages/core/src/lib/channel.ts`).

So ask a front-end team what happens on reload, rather than how they open a
channel. If they are not using the SDK, they need the rejoin call as well as the
send.

## SSE events

The response is a Server-Sent Events stream and the connection stays open.

| event | when |
|---|---|
| `asgard.run.init` | the service initialises |
| `asgard.message.start` | a message begins, possibly with initial text |
| `asgard.message.delta` | content arrives progressively, possibly many times; `idx` gives the order |
| `asgard.message.complete` | the full text plus its Template form |
| `asgard.run.done` | the run ends |
| `asgard.run.error` | only on failure |
| `asgard.process.start` / `.complete` | a process begins and ends |
| `asgard.tool_call.start` / `.complete` | only when a Toolset is in use |

The ordinary sequence is `run.init`, `message.start`, one or more
`message.delta`, `message.complete`, then either back to `message.start` or on to
`run.done`.

The table is the documentation's; the platform emits more. Thinking and canvas
blocks (`asgard.message.thinking.*`, `asgard.message.canvas.*`),
`asgard.tool_call.consent`, `asgard.completion_model.usage`, sandbox and
subagent lifecycle events, `asgard.message.user` and `asgard.message.feedback`
on a replayed transcript, channel title and status updates, and
`asgard.prompt_suggestion` are all declared in
asgard-core `internal/constants.go`, each marked additive. A front end that ignores an event
type it does not know keeps working; one that treats an unknown type as an error
does not.

## Sending a file is two calls

A file does not travel with the message. Upload it, keep the id, send the
message referencing it:

    POST .../bot-provider/<name>/blob            multipart/form-data
      customChannelId, file                      -> a blobId

    POST .../bot-provider/<name>/message/sse     the ordinary send
      customChannelId, text, blobIds: [ ... ]    an array, several files

`customChannelId` has to match across both, as it does for everything else.
It is what ties a conversation together, and an upload that used a different one
is attached to a different conversation.

Inside the workflow the files arrive as `prevBlobs`, an array of Blob with
`blobId`, `fileType`, `fileName`, `size` and `mime` - see
[`processors.md`](../wiki/processors.md). Every one of those variables can be absent,
so each access is written defensively or throws on the turn somebody sends no
file.

## What an event actually looks like

Every event carries the same envelope, and the payload is a tagged union:

```json
{ "eventType": "asgard.message.delta",
  "requestId": "...", "eventId": "...",
  "namespace": "...", "botProviderName": "bp-...",
  "customChannelId": "...",
  "fact": { "runInit": null, "runDone": null, "runError": null,
            "messageStart": null,
            "messageDelta": { "message": { "text": "...", "idx": 3, ... } },
            "messageComplete": null } }
```

`fact` has one key per event type and only the one matching `eventType` is
populated; the rest are `null`. A front end reads `fact.<name>` rather than
inspecting the shape. The keys present in `fact` vary between events, so
matching on shape breaks the first time the platform adds one.

`requestId` groups every event of one run and is what to quote in a bug report.
`eventId` orders them; on a delta, `idx` orders within the message.

## The error event names the processor that failed

`asgard.run.error`'s `fact.runError.error` is the most useful field in this
stream:

    message   human-readable
    code      INVALID_ARGUMENT and friends
    inner     the underlying error, often empty
    location  namespace, workflowName, processorName,
              processorType, processorConfigName, processId

`location` is which node of which Workflow failed, so in a large chart you do
not have to work it out from the stream. The fields
are empty when the failure happens before a processor runs - the example in the
documentation is an empty message rejected at the channel - and an empty
`workflowName` is therefore itself information: it did not reach the flow.

Surface it. A front end that shows only `message` throws away the location, and
whoever debugs it later has to reproduce the failure to get it back.

## Four integration patterns

The documentation groups integrations into four patterns, and the choice governs
how auth, history and memory are handled:

- Hosted Embed
- Direct Connect
- Workflow Auth
- Backend Relay

The details are on each pattern's own page. The choice decides where the API key
lives - connecting from the browser means the key is in the browser, and Backend
Relay exists to avoid that.

## SDK

```bash
npm install @asgard-js/core     # framework agnostic
npm install @asgard-js/react    # chat UI components and hooks
```

`@asgard-js/react` takes `@asgard-js/core` as a peer dependency.

The SDK wraps the REST requests and authentication, handles the SSE stream, and
persists a session so multi-turn conversation works.

### What the client does besides opening a channel

Know these before a front-end team asks, because otherwise each is a REST
endpoint somebody has to find. All of them derive their endpoint from
`botProviderEndpoint`, and without it the error names which derivation failed:

    uploadFile                  a file, returning blob info to carry into a message
    downloadChannelHomeFile     one of that channel's own files
    channelMetadata             whether a channel exists; null when it does not
    suspendChannel              stop the run in flight on that channel
    deleteChannel               remove it
    sendMessageFeedback         the user's thumb up or down on one reply

`detach` and `close` differ. `close` shuts the client down at once and cuts off
any run still streaming. `detach({ timeoutMs })` lets runs in flight finish on
the platform: callbacks stop firing, and the client closes itself when the last
run settles or when the timeout passes, whichever comes first. It is the one to
call when a chat component unmounts mid-run.

A feedback comment is capped in bytes, not characters. `@asgard-js/core`
exports `FEEDBACK_COMMENT_MAX_BYTES` and `feedbackCommentByteLength` for that
reason - a Chinese comment reaches the cap in a third of the characters an
English one does.

### Version migration

The breaking change from 0.1.x to 0.2.x is new SSE events -
`asgard.process.start/complete` and `asgard.tool_call.start/complete` - which
need handlers adding. The `<Chatbot>` component's `config` should be checked too.

## From nothing to live

1. Pick an integration pattern
2. Build the Bot and Workflow in Odin
3. Configure a Completion Model and the provider's API key
4. Publish as a Generic Integration and collect the `botProviderEndpoint` and API
   key - in a chart, the endpoint is the BotProvider's name and namespace and the
   key is its `spec.generic.apiKey`
5. Manage the API key - rotating it is changing that value
6. Connect through the SDK
7. Understand the SSE sequence
8. Design how replies render; message templates are rendered by the front end

The API key is the BotProvider's own `spec.generic.apiKey`. The platform
compares the header against that field, read at request time from a literal or
a Secret reference, and a BotProvider with `authMode: none` skips the check -
asgard-core `478cf5d6` `internal/edgeserver/middleware/bot_provider.go`
`GenericBotProviderAuth`. Where the value comes from depends on how the
BotProvider was made: one released from Odin's Release panel gets a key the
platform generates, which can only be regenerated; one written in a chart gets
whatever the chart sets, and the chart's author hands it to the front end's
owner. Keep it out of front-end code, version control and anywhere public. A
separate `spec.adminApiKey` guards the history route.

This is not the credential a CR uses. `SourceSet.apiKey`, `Toolset.apiKey`
and `BotProvider.adminApiKey` take a platform resource credential, which is the
engagement's own value on the release and no console page issues -
`../usecase/conventions.md` has what to set it to. The two are easily
confused.

`../wiki/integration.md` has the Release panel that generates it.

## Before writing the chart

`../usecase/external-api.md` covers an HTTP tool, `../usecase/api-oauth.md` the two-call
token chain, and `../usecase/workflow-chain.md` what passes between processors.

## Sources

- Every file under
  [send-message](https://docs.asgard-ai.com/docs/developer-reference/api-doc/send-message/introduction)
  - asgard-docs `f00e0ee`
- [Integration guide](https://docs.asgard-ai.com/docs/developer-reference/integration-guide)
  - asgard-docs `f00e0ee`
- [SDK overview](https://docs.asgard-ai.com/docs/developer-reference/sdk/overview),
  the JavaScript and React guides, and `asgard-sdk`
  - asgard-docs `f00e0ee`
- [Migration guide](https://docs.asgard-ai.com/docs/developer-reference/migration)
  - asgard-docs `f00e0ee`
- The examples under
  [examples](https://docs.asgard-ai.com/docs/developer-reference/examples)
  - asgard-docs `f00e0ee`
- [Authentication](https://docs.asgard-ai.com/docs/developer-reference/authentication)
  - asgard-docs `f00e0ee`

- The two-call file path and `blobIds`:
  [upload file](https://docs.asgard-ai.com/docs/developer-reference/api-doc/send-message/upload-file-api)
  and [append file and send](https://docs.asgard-ai.com/docs/developer-reference/api-doc/send-message/append-file-and-send-message-api)
  - asgard-docs `f00e0ee`
- The event envelope, the `fact` union and `runError.location`: the pages
  under `developer-reference/api-doc/send-message/sse-response/` - one page per
  event, starting at
  [run-init](https://docs.asgard-ai.com/docs/developer-reference/api-doc/send-message/sse-response/run-init).
  The directory itself has no landing page and 404s, so cite the pages
  rather than the directory

- The client's channel and file methods, `detach` against `close`, and the
  byte-capped feedback comment: asgard-docs `23409b3` `docs/developer-reference/sdk/javascript.mdx`

- The two routes, the header check and where the key is read from, at
  asgard-core `478cf5d6`: asgard-core `internal/edgeserver/component/gin.go` and
  asgard-core `internal/edgeserver/middleware/bot_provider.go`, and asgard-js-sdk
  `packages/core/src/lib/client.ts`

**Checked:** the routes, the header, the actions and request fields, the
event names, the `fact` envelope, `runError.location`, the blob upload and
`prevBlobs` against asgard-core `478cf5d6`:
asgard-core `internal/edgeserver/component/gin.go`,
asgard-core `internal/edgeserver/middleware/bot_provider.go`,
asgard-core `internal/edgeserver/handler/bot_provider.go`,
asgard-core `internal/models/generic.go`,
asgard-core `internal/models/edgeserver.go`,
asgard-core `internal/models/err.go`,
asgard-core `internal/models/blob.go` and
asgard-core `internal/constants.go`; the client methods,
`detach`, `close`, `Channel.restore`, `Channel.reset`, the feedback byte cap and
the React peer dependency against asgard-js-sdk `56ad14e`:
asgard-js-sdk `packages/core/src/lib/client.ts`,
asgard-js-sdk `packages/core/src/lib/channel.ts`,
asgard-js-sdk `packages/core/src/lib/feedback-message.ts` and
asgard-js-sdk `packages/react/package.json`.

**Unchecked:** no reference deployment has a front end on this endpoint, so none
of it has been seen working against a live BotProvider.
