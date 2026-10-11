---
group: While building
description: chat platforms, the Applications pages, the architecture - and that there is no mail capability at all
---
# Reaching an agent from outside

Four ways.

| route | CR | when |
|---|---|---|
| a chat platform (LINE / Slack / Discord / Telegram) | `BotProvider`, that class | the customer already lives there |
| your own front end or API | `BotProvider` class `generic` | the customer's own site or app |
| the SDK (JS / React) | as above, plus the packages | embedding a ready-made chat UI |
| an MCP Server | `Toolset` | letting another agent call it |

## Chat platforms

Each platform needs different credentials, all supplied by the customer. For
Slack the answer depends on which path you are on, and that row has been got
wrong before:

| platform | the chart's `BotProvider` needs | the documentation's UI flow asks for |
|---|---|---|
| LINE | `channelAccessToken`, `channelSecret` | the same two, as Channel Access Token and Channel Secret |
| Slack | `appToken`, `botToken` - both required | Client ID, Client Secret, Signing Secret, Permission Scopes |
| Discord | `botToken` | Bot Token |
| Telegram | `botToken` and `webhookSecretToken`, both required | Bot Token only |

In two of those rows the chart asks for more than, or something other than,
what the documentation asks for.

Slack has two different sets of credentials. The UI's flow is an OAuth app
installation - client id, client secret, signing secret, scopes - and it is what
somebody clicking through Applications supplies. A chart uses none of them:
`spec.slack` takes an app-level token and a bot token, the `xapp-` and `xoxb-`
pair. Asking a customer for a client id and then writing a
chart leaves you without either field the CR requires.

Telegram's second field, `webhookSecretToken`, is in the CRD and in no
documentation page. It is required beside `botToken`. We generate it rather than
the customer supplying it - Telegram accepts a secret you choose - but it has to
exist before the CR applies. The platform compares it with the
`X-Telegram-Bot-Api-Secret-Token` header on every inbound call and refuses a
mismatch, so the webhook has to be registered with Telegram under the same value.

LINE is the only one needing two-way setup - Asgard gives a URL that has to go
back into LINE, which somebody has to paste into the LINE Developers Console,
verify, and then enable Use webhook on. The rest only take credentials inward.
LINE also has a prerequisite before any of that: Messaging API has to be
enabled on the Official Account, which is the customer's own step in a console
nobody here can reach. For Taiwanese customers LINE is usually the first one
asked about.

Discord also has a customer-side prerequisite that is not a credential: the
app and its bot are created in the Discord Developer Portal, authorised with
their permissions, and invited to the server. The customer does all of it in
their console. Until they do, a chart holding the right `botToken` cannot post
anywhere.

Discord and Slack get a Connector Pod rather than a webhook, because both
hold an outbound WebSocket - `../usecase/chat-channel.md` has what that costs.

`botProviderClass` is immutable in the CRD, so it has to be right the first time.

## What the platform does NOT own: the conversation

Every customer service engagement asks about this, and the answer is always
the same.

Handing over to a human, pausing the agent while a person replies, resuming
afterwards, counting how many questions one user has asked - none of these are
platform features. There is no CR for any of them, and no field: searching the
CRDs for a handoff, a takeover or a per-user quota finds nothing, and the only
suspend in them pauses a schedule. The API's `message/suspend` stops the run in
flight on a channel; it does not hold the channel for a person.

The platform quota covers capacity, not people. All eight numbers, and they
apply to the Workspace - projects inside it share them:

| per request | per workspace |
|---|---|
| 5 RPS per endpoint | 40 Projects |
| 3 minutes | 300 GB of Knowledge Base |
| 30 steps - a chart field, below | 500 Processors |
| | 10 Loaders |
| | 150 Indexers |

Ten Loaders is usually the first limit reached. A Loader is one recurring pull, so a
customer with a dozen document sources exceeds it before anything else on this
list - see [`knowledge.md`](../wiki/knowledge.md).

These are defaults, and apart from the steps they are per plan. They are
raised by contacting sales or writing to service@asgard-ai.com, so in a meeting
say that, not "that is the limit". The overview says a Workspace
has a price plan and that how many Projects it may hold depends on it - so 40 is
one plan's number rather than the platform's. What is not documented is which
plan gives what. See [`what-they-read.md`](../wiki/what-they-read.md).

The step ceiling is a field on the chart, not a plan quota. A step
is one hand-off from one processor to the next in a workflow, counted from the
user's message; an LLM processor is one step however many tools it calls inside
it. The ceiling is the BotProvider's `spec.maxUnsupervisedSteps`, and 30 is its
default, so it is raised by the chart rather than by sales. Reaching it ends the
request with `Max execution steps 30 reached. Consider setting a higher value
for maxUnsupervisedSteps in bot spec.` - asgard-core `478cf5d6`
asgard-core `internal/bpcontroller/server/bp_controller.go` (`sendTask` checks it,
`HandleTaskResult` adds one per hand-off, a new message or a consent resume
starts it again at 1) and asgard-core `internal/constants.go` `DefaultMaxUnsupervisedSteps`.
So a workflow that chains many processors per turn reaches it, and an agent that
calls many tools inside one processor does not.

The mechanism the platform's own case study describes puts the conversation
somewhere else entirely. A retail site's support desk receives the customer's
message, writes it into its own conversation log and answers the customer
immediately; only then does it forward the message to the Flow Agent in the
background, with a scope-limited credential. A human can join that same thread at
any time, because the thread was never the agent's to begin with. The demo
behind the case study is built that way: the site keeps every message in its own
tables with the sender marked as the customer, the agent or a human, uses its
own conversation id as `customChannelId`, and a staff login posts into the same
conversation.

So the shape is:

    the customer     ->  something that owns the conversation  ->  Asgard
                         (a support desk, a site, a relay)

"Pause the AI" is that middle layer deciding not to forward. "Three strikes then
a human" is that middle layer counting. "Ten questions a day" is that middle
layer counting too.

With a website the middle layer is the site itself. On a chat platform it is
whatever the customer already runs there. Find out which surface that is in the
interview, not here: it is the vendor's behaviour on the customer's own account,
it changes, and an answer written down here would go stale unnoticed. `../wiki/taiwan-channels.md` states the same
rule for the commerce channels and is the longer version.

None of the pause, the counting or the takeover is the platform's, so
whichever surface it turns out to be, that state lives outside Asgard. That
decides the shape, whatever the interview answer is.

## Two pages in Odin

- Applications -> Data Insight & Agent Hub - browse and open the Mimir and
  Sindri applications published to this Workspace, filtered by All, Data Insight
  or Agent Hub
- Applications -> Customized Integration - the list of every integration in
  the Project, filtered by All, Bot (what Flow Agents have released), API
  (workflows of the Automation Tool type) or MCP Servers (Toolsets). A card's
  switch disables an integration without deleting it. Nothing is created on
  this page: an integration is a Flow Agent's release, made from the Release
  panel of its workflow set, and a Flow Agent that was never released does not
  appear here. The release form takes an Application Type (Generic Chatbot,
  Telegram, Slack, LINE, Discord, or Sindri), a Name, an optional Description, a
  Debug Mode (`Never` by default, `Always`, or `On-Demand` for requests carrying
  `is_debug=true`), and Channel Max Idle Days. `Always` and `On-Demand` can send
  prompts, variable values and tool arguments to the end user's browser

## API and SSE

```
POST {base_url}/ns/{namespace}/bot-provider/{bot_provider_name}/message/sse
```

There is no `/generic/` segment. The API reference prints one, and that form is a
legacy route that skips the subscription check and the metering;
`../wiki/api.md` has both routes and the code that registers them.

Authenticated with an `X-API-KEY` header carrying the BotProvider's own
`spec.generic.apiKey` - `../wiki/api.md` has the check. It
is not the credential a CR reads: that one is a platform resource key, the
engagement's own value on the release, and `../usecase/conventions.md` says
what to set it to.
A generic integration released through the UI has no key field: the platform
issues its key at creation, and it can only be replaced by regenerating it.

The response is Server-Sent Events; the connection stays open and carries agent
messages, system events and end-user messages.

The endpoint, the event sequence, the four integration patterns and the SDK are
in [`api.md`](../wiki/api.md).

## SDK

```bash
npm install @asgard-js/core     # framework agnostic, Node.js or browser
npm install @asgard-js/react    # ready-made chat UI components and hooks
```

`@asgard-js/react` takes `@asgard-js/core` as a peer dependency; install both.

The SDK wraps REST requests and authentication, handles the SSE stream, and
persists a session for multi-turn conversation.

## Platform architecture

```
client (web / SDK / REST API / chat platform)
  -> API Gateway (X-API-KEY auth, SSE streaming)
    -> Asgard Core Engine (Workflow Engine, Processor Manager, Channel Manager)
      -> Processors
        -> AI resources (LLM providers, Knowledge Base, Data Source, MCP Server)
```

## Before writing the chart

`../usecase/chat-channel.md` has each class's credential block and what a
channel costs - which credentials have to be obtained from the channel's own
console and declared, and whether the class needs a connector pod.

## What the platform cannot send

There is no mail capability anywhere in the platform - no SMTP, no preset
mail Toolset, nothing in the core. Customers often ask for "email me when it
happens"; answer according to what they have:

    they have an HTTP endpoint that sends mail   an external-api call
    they have no endpoint                        it cannot be built yet

SMTP credentials are not an endpoint. The platform's only outbound capability
is `http-request`, which speaks HTTPS; SMTP is a multi-round protocol on its own
TCP port and a Workflow has no way to speak it. So a username, a password and
`smtp.<host>:587` cannot be used at all, with any amount of work, and that is
the shape of credential a customer hands over when asked
for mail access. One deployment asked for a mail API, was told which one, and
received SMTP credentials for it; they are different authentication mechanisms
and not interchangeable.

Give only that reason for saying no: the platform cannot speak SMTP. Do not
add "SMTP basic auth is being switched off soon" - it is not true: Microsoft has deferred that three times, the current schedule
turns it off by default at the end of 2026 with administrators able to turn it
back on, and no final removal date is announced. A deployment recorded the
deferral correction in its own record so that nobody repeats the argument.

### So mail means a provider with an HTTP API

Two shapes; raise the cost difference in the meeting:

| | a single-call API | a token-first API |
|---|---|---|
| HTTP calls per mail | 1 | 2 - fetch a token, then send |
| processors | send, respond | get-token, send, respond |
| secrets to hold | 1 | 3 |

The sender has to be verified with the provider, per address or per domain,
or the send is refused - and the customer's IT does that verification, on their
schedule. Ask for it in the same breath as the API key.

Three details read off a working send, each of which looks like a bug when you
hit it:

  - success can be a 2xx that is not 200, with an empty body. Test the
    status as a range rather than `== 200`, and set `parseJson: false` - there
    is no JSON to parse and every run leaves a parse warning otherwise
  - turn the provider's click tracking off. It rewrites every link in the
    mail to its own tracking domain, and an internal notification arriving with
    an unfamiliar redirect domain in it reads as phishing
  - on `http-request`, every config key that is not `url`, `method`, `body` or
    `parseJson` is sent as an HTTP header - which is how the API key and the
    content type get there, and `../wiki/processors.md` has the rest of what an
    extra key means per processor

A deployment that mocks mail sending has to disclose it. One deployment does
this in a way worth copying: `wf-send-mail` is a single `push-message` returning
`{ok: true, mocked: true, to, subject, body}`, so the whole pipeline runs and
the drafted mail lands in the invocation record for review. `ok: true` is
deliberate - a false would stop a Trigger's cursor and the path would never be
exercised.

So disclosure is the only safeguard. That deployment requires both
the tool's `tooling.description` and the agent prompt to lead every summary with
"MOCK - not actually sent", and to never say "notified".

The risk of a mock is a log that reads as though people were emailed. The same
applies to any mocked outward action - a ticket not created, an
order not placed.

## Sources

- [LINE](https://docs.asgard-ai.com/docs/integration/line),
  [Slack](https://docs.asgard-ai.com/docs/integration/slack),
  [Telegram](https://docs.asgard-ai.com/docs/integration/telegram)
  - asgard-docs `f00e0ee`. `integration/Discord.mdx` is an empty file; the
  Discord content is under `integration-with-asgard/`
- [SDK](https://docs.asgard-ai.com/docs/integration/sdk)
  - asgard-docs `23409b3`. The page is a two-line pointer at
  the two SDK guides, and what moved since `f00e0ee` is the form of those two
  links - nothing this page says rests on it
- The pages under `integration-with-asgard/` (api, line, slack, discord)
  - asgard-docs `f00e0ee`. All four are marked `draft`, and they describe an
  interface called "Published -> add integrated" inside a Project. Odin's current
  interface creates an integration from the Release panel of a Flow Agent's
  workflow set, which may be the same step renamed. Possibly stale
  - Discord's own prerequisite - the app and bot created in the Developer
  Portal, authorised, and invited to the server - is `integration-with-discord.md`,
  at asgard-docs `23409b3`. It is the same page and the same
  `draft: true`; what is recorded here is the customer-side step, which the
  Applications interface above does not bear on
- [Authentication](https://docs.asgard-ai.com/docs/developer-reference/authentication)
  and [architecture](https://docs.asgard-ai.com/docs/developer-reference/architecture)
  - asgard-docs `f00e0ee`
- [Applications overview](https://docs.asgard-ai.com/docs/product-suite/odin/features/applications-overview)
  and [Customized Integration](https://docs.asgard-ai.com/docs/product-suite/odin/features/applications-customized-integration)
  - asgard-docs `6261fdff`
- That SMTP cannot be reached at all, and what a real mail send costs: read
  off a production deployment, which asked for a mail API, received
  SMTP credentials for it, and wrote down why neither route worked. The
  single-call shape, the empty-bodied 202, the click-tracking rewrite and the
  verified-sender requirement are from that deployment's own working send; the
  deferral correction is recorded in its living spec. That an extra config key
  on `http-request` is sent as an HTTP header is at asgard-core
  `623ceb5`, asgard-core `internal/processor/task/http_request.go`, which is also
  where the value having to be a string comes from
- `botProviderClass` being immutable, and each class's credential fields:
  [asgard-kube](https://github.com/asgard-ai-platform/asgard-kube) `3da0365` -
  `BotProviderSpec` and the per-class specs
- The platform owning no handoff, takeover, suspend or per-user quota:
  [asgard-kube](https://github.com/asgard-ai-platform/asgard-kube) `3da0365` -
  no CRD and no field carries any of those concepts
- The quota numbers, all eight, that they are Workspace-level and shared, and
  that they are raised through sales - apart from the steps, which asgard-core
  `478cf5d6` shows are a chart field:
  [Quota and limits](https://docs.asgard-ai.com/docs/help-community/quota-limits)
  - asgard-docs `f00e0ee`, read in full
- The support desk owning the conversation:
  [AI customer service answering order enquiries](https://docs.asgard-ai.com/docs/product-suite/odin/case-studies/retail-ai-customer-service)
  - asgard-docs `6261fdff`

**Checked:** the chart column of the credentials table against
asgard-kube `3da0365` `pkg/apis/asgard/v1alpha1/types.go`; the Connector Pod for
Slack and Discord only against asgard-core `478cf5d6` `internal/bpoperator/reconciler/bp_reconciler.go`;
that Telegram's `webhookSecretToken` is read on every inbound call against
asgard-core `478cf5d6` `internal/edgeserver/handler/bot_provider.go`; the support
desk owning the conversation against the retail demo,
asgard-industry-demo-generator `718cc0e` `common/worker/src/retail/customer-support/`
and `retail/chart/app/templates/supervisor/customer_service/` in the same repository.

**Unchecked:** every BotProvider in every reference deployment is `generic`, so
no chat-platform class has been seen running, and the UI's credential flows
come from the product documentation only.
