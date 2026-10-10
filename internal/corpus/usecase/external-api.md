---
group: Read paths
description: "a source that is an HTTP API, not a database - and the credential that decides whether the agent calls it from a Workflow or from its sandbox"
---
# External HTTP APIs

Reading from, and writing to, a system that is not a database - a SaaS platform,
a partner's REST API, an internal service.

**Seen in:** a shopping assistant whose entire read path is two HTTP APIs, and a
notification chain that posts outward to a mail endpoint.

**Checked:** against a search tool's http-request workflow - update-context first, parseJson, header configs, httpResponse - and the CRD. What an `http-request` does to the context against asgard-core `478cf5d6` - `internal/processor/task/http_request.go` and `internal/bpcontroller/server/bp_controller.go`, which is the only place `prevPayload` is written.

**Unchecked:** the advice on measuring the request against the real API. That step has to be taken against the customer's API and cannot be done here.

Read the platform side first: `../wiki/api.md` -
the endpoint, the SSE event sequence, and the four integration patterns. This page assumes you have.

## First: is an API the right route at all?

Per system, take the most capable route it offers:

| route | what you get | shape |
|---|---|---|
| a database you can read | the agent composes its own queries and joins across tables | `../usecase/semantic-layer.md` |
| an API | a fixed set of calls, but a real contract - reviewable, and gateable for writes | this extract |
| a screen a person clicks | last resort. Brittle, slow, and it breaks whenever the vendor changes their UI | `../usecase/browser-operation.md` |

A system offering both: read from the database, write through the API. The
database gives the agent questions nobody thought to expose an endpoint for; the
API gives writes a gate.

Before building one integration per external system, ask whether the customer
already consolidates them. Middleware, an OMS, a warehouse that already pulls
the channels in - if one exists, several external APIs collapse into one database
and the design gets simpler in every dimension. If nobody knows, record it as an
open question in `docs/open-questions.md` rather than designing on an assumption.

What this shape costs to get is `../needs/external-api.md` - what has to come
from the customer before any of it can be built.

## When this shape, and when not

There are two ways to reach an external API, and they differ in what can be
reviewed and gated:

| | A. `http-request` in a Workflow | B. the agent calls it from its sandbox |
|---|---|---|
| Where the call happens | the platform, inside a tool | inside the agent's sandbox, taught by a skill |
| Protocol | HTTP only | anything a client exists for - SNMP, SSH, a vendor CLI, a database client |
| The contract | an `inputSchema` you wrote | whatever the skill describes |
| Reviewable | yes - the URL, the body and the parsing are in version control | no - the agent composes the call |
| Can gate on human approval | yes, `requestConsent: true` | no |
| Use for | anything with a side effect, and reads you want pinned | reads only, where the surface is wide and exploratory |

Every write goes through A, with `requestConsent: true`. Never let a sandbox
make a side-effecting call to an external system: there is no gate, no record of
what was sent, and no way to review the request shape before it goes out.

Reads may use B in two cases:

- the API is large and the useful calls are not knowable in advance - a back
  office with fifty endpoints, where a tool per endpoint is the wrong trade.
  Give the agent a skill describing the contract and let it compose calls.
- the system does not speak HTTP at all. The sandbox is a real environment,
  so a protocol with a client - SNMP, SSH, a vendor CLI - is reached by running
  that client. `http-request` is one processor type, not the limit of what the
  platform can integrate.

Either way the reads stay read-only, and a write still goes through A.

For the second case the skill carries what a person would need: how to
authenticate, which commands are safe, how to read the output, and which
commands are refused outright.

B is only available when the credential arrives per turn, and this is the
constraint that decides between the two shapes. A static service key cannot
reach a sandbox. There is no `env` on `Agent.spec.managed`, on
`SandboxBlueprint.spec` or on `SkillSet.spec`; the sandbox container's
environment is a closed list the reconciler builds, and every secret in it is
the platform's own - the browser and editor passwords, out of a Secret the
operator manages. `credentialMounts` is not a general mount: its only field is
`oAuthCredentialName`, it resolves an `OAuthCredential`, and the token lands in
the directory as `access_token` and nothing else. That credential holds a token
only after somebody has completed its grant, which a chart cannot do
(`asgard-cli operate oauth-credential authorize`). `extraDirectories` names
directories to create and carries no content. A `hook` is an expression stored
in the CR spec, so a key written by one is a key baked into the chart, which
`../usecase/conventions.md` refuses.

So a system that needs a static key goes through A, however well it fits
B's description otherwise: `Workflow.spec.variables[].valueFrom.secretKeyRef` is
the only route a service credential has, and a processor config reads it as
`vars.<name>`. B is for a credential the caller brings - see
`../usecase/per-turn-credentials.md`. Checked against asgard-kube `cbd8d70`
and asgard-core `623ceb50`: the three CRs' fields and the sandbox pod's env in
`bpoperator/reconciler/sb_reconciler.go`.

## The shape (A)

    Workflow  wf-<verb-noun>
      entries[].inputSchema      the tool's parameters - real ones, unlike a SQL tool
      processors:
        update-context           pull the parameters out FIRST
        http-request             the call
        push-message             shape the response for the agent
    Toolset  ts-<name>
      tools[] -> (workflow, entry), requestConsent per tool

Only that field gates a call; the Toolset's class and the Workflow play no part.
`../wiki/tools.md` has why, and why an `mcp-server`
Toolset cannot carry one at all.

## Generate it

    asgard-cli add httptool <name> --toolset ts-<name>

That writes the structure below with the fields that fail silently already in
place - the display annotation, the labels the UI needs, the current field names.
When the skeleton is copied by hand these get lost, and nothing reports them
missing: not helm lint, not CRD validation, not a server dry-run.

The generated file marks the judgement calls TODO. The rest of this page covers
them.

## The skeleton

`templates/tool/wf-<verb-noun>.yaml`.

```yaml
apiVersion: asgard-ai.com/v1alpha1
kind: Workflow
metadata:
  name: wf-<verb-noun>
  annotations:
    asgard-ai.com/workflow-name: "<display name>"
    {{- include "<chart>.workflowSetAnnotations" (dict "name" "wf-<verb-noun>" "displayName" "<display name>") | nindent 4 }}
  labels:
    {{- include "<chart>.workflowSetLabels" (dict "name" "wf-<verb-noun>" "type" "automation_tool") | nindent 4 }}
    {{- include "<chart>.labels" . | nindent 4 }}
spec:
  # A static key for this API goes here, and a config below reads it as
  # vars.<name> - `../usecase/workflow-chain.md`'s skeleton has the shape.
  variables: []
  entries:
    - name: entry-main
      handlingProcessor: proc-input
      labels:
        display_name: Entry
      inputSchema: |-
        {
          "type": "object",
          "properties": {
            "q": {
              "type": "string",
              "description": "<what to pass, and what NOT to - e.g. the thing being searched for, not the whole conversation>"
            }
          },
          "required": ["q"]
        }
      tooling:
        name: <snake_case_name>
        description: |-
          <what it does, what it returns, and when the agent must call it>
        allowUploadFile: false
  exits: []
  processors:
    # Read the parameters into named context values first. See below.
    - name: proc-input
      type: update-context
      labels:
        display_name: <read parameters>
      configs:
        - name: searchQuery
          expression: |-
            (() => {
              return String(prevPayload.q || "").trim();
            })()

    - name: proc-call
      type: http-request
      labels:
        display_name: HTTP Request
      configs:
        - name: url
          value: {{ .Values.<service>.endpoint | quote }}
        - name: method
          value: POST
        - name: parseJson
          value: "true"
        - name: body
          expression: |-
            (() => {
              return JSON.stringify({ q: searchQuery });
            })()
        # Headers are configs, named after the header itself.
        - name: Content-Type
          value: application/json

    - name: proc-response
      type: push-message
      labels:
        display_name: Response
      configs:
        - name: payload
          expression: |-
            (() => {
              const items = (httpResponse && httpResponse.json && httpResponse.json.items) || [];
              return { items };
            })()

    # A failed call has to say so. Without this branch the run ends at
    # proc-call and the tool returns nothing, which a model reads as "no
    # result" rather than "the call failed" - and then answers from memory.
    - name: proc-error
      type: push-message
      labels:
        display_name: Error
      configs:
        - name: payload
          expression: |-
            (() => {
              return { error: (prevError && String(prevError)) || "the request failed" };
            })()

  # **`relationships` is what runs the chain.** Without it the run starts at
  # the entry's handlingProcessor and stops there: the arguments reach context
  # and the call is never made. The CR is legal either way, so every check is
  # green while the tool does a third of what this looks like it does.
  # `gate` W3 reports it.
  relationships:
    - from: {processor: proc-input, relationName: success}
      to: {processor: proc-call}
    - from: {processor: proc-call, relationName: success}
      to: {processor: proc-response}
    - from: {processor: proc-call, relationName: failure}
      to: {processor: proc-error}
```

Endpoints and non-secret settings are `chartValues` set on the platform; a token is
declared under `appSecret` and read with a `secretKeyRef`, never a value. The Secret
belongs to the release and the platform injects its name.

## Several systems of the same kind

Integrating five marketplaces, or three carriers, is not five copies of this
extract. Decide two things:

One tool per system, or one tool with a system parameter?

Prefer one tool per system when the APIs differ in shape - different auth,
different pagination, different field names - which is the normal case across
vendors. Each tool's `tooling.description` can then say what that vendor's data
actually means, and one vendor's outage does not take the others with it.

A single tool taking `platform` as a parameter only works when the calls are
genuinely uniform, which usually means someone has already built an abstraction -
and if they have, read from that, not from five APIs.

Who combines the results?

The agent does. It calls the tools it needs and reconciles the answers in its own
reasoning - the same mechanism that lets a scheduled run mount two semantic
layers and match records across them.

That puts two obligations on you:

- Each tool must return a shape the agent can line up with the others. Same
  field names for the same concept, same units, same identifier. Convert at the
  boundary, in the response processor, rather than hoping the model normalises
  five vendors' spellings of the same thing.
- The prompt must say what a missing or failed source means. Five sources
  means partial answers are routine, and the agent has to say "four of five
  reported, one timed out" rather than quietly presenting four as the total.
  A number that silently omits a channel is worse than an error.

Group them in one Toolset when they are one capability ("check stock everywhere")
and the agent should see them together.

## Designing the tools the generator leaves TODO

### One tool per call, or one tool per question?

Per question the user asks, which is usually coarser than the API. An
endpoint returning a page of raw records is not a tool; "what is the stock of
this item" is, even if it takes three calls behind the scenes.

Where the API is large and exploratory, stop writing tools and give the agent a
skill instead - see the two ways above.

### Shape the response before it reaches the model

The tool's output is the model's evidence, so convert at the boundary:

- Same concept, same field name, same units across sibling tools. Five
  vendors' spellings of "quantity" become one, in the response processor, not in
  the prompt.
- Types that survive the next hop. An id arriving as a number where the next
  tool wants a string is a classic silent break; cast it here and note the date
  you checked.
- Drop what nobody uses. A hundred fields of vendor metadata crowds out the
  five that matter.

### What the description has to say that a database tool's does not

The model can answer questions about an external system from memory, and it
will be wrong. Say so:

    回答任何關於 X 的問題之前一定要先呼叫這支 ——
    你自己的印象與外部網路的資訊一律是錯的。

And say what an empty result means, or "not found" gets treated as a failed
lookup worth retrying another way.

### Partial failure is normal with several sources

Decide what the agent says when one of five sources times out, and put it in the
prompt. The answer should say "four of five reported, one timed out" rather
than give a total that silently omits a channel.

## Fields that are not obvious

### `update-context` comes first

`prevPayload` is the tool's arguments for the whole run. An `http-request`
copies the context forward and adds `httpResponse`; it does not touch
`prevPayload`, which only the controller writes, when a request or a new turn
arrives. Some deployments carry a comment saying the opposite, and the
processor code has never done it.

Read every parameter into a named context value before the call anyway. It is
where the model's input gets trimmed and validated, once, and the body
expression then reads `searchQuery` rather than reaching into the payload. A
parameter read under the wrong name is the mistake this shape most often
produces, and nothing catches it: the workflow runs, the body is built from
`undefined`, and the API returns something unhelpful.

### Headers are configs

A header is a config entry named after the header - `Content-Type`,
`Authorization`. There is no headers map.

### The response lives in `httpResponse`

`httpResponse.json` when `parseJson` is `"true"` (a string, like every config
value). Guard the whole path: `(httpResponse && httpResponse.json && ...) || []`.
A failed call does not set it - it is absent, or still an earlier call's - and
an expression that throws takes the tool with it.

### The parameters come from the model

Unlike a fixed SQL tool, this one takes input from the model. That is
acceptable because the input goes into a JSON body against a typed API, not into
a query language. Keep it that way:

- validate in `update-context` rather than trusting the model - clamp an enum to
  its allowed set, trim a string, default anything missing
- never interpolate a parameter into a URL path or a SQL string
- keep `required` honest, so a missing argument fails loudly instead of
  silently searching for `""`

### Write down what you measured about the response

Field types across a boundary are a classic source of silent breakage - an id
that arrives as a number where the next tool wants a string. Convert at the
boundary and say so in a comment with the date you checked. The API can change,
and the note lets the next reader find out that it has.

## The `tooling.description` matters more here than for a database tool

With a database tool, the agent cannot answer without it. With an external API it
can - from memory, and it will be wrong. Say so explicitly:

> call this before answering any question about X. Your own impression, and
> anything from the open web, is wrong here.

And say what an empty result means, or the agent will treat it as a failed
lookup and try something else:

> an empty list means the platform does not carry it. Do not go looking another
> way.

What is true of the system rather than of this call belongs in the skill, not in
a paragraph repeated in every description. A skill written for a subject the
tools now cover has to drop whatever the tools hide.
`../wiki/tool-description-and-skill.md` is the division.

## Writes: the approval gate

A tool with a side effect sets `requestConsent: true` on its entry in the
Toolset. The harness intercepts the call and waits for a human. The prompt
should not describe an approval flow - the gate is the harness's, and prompting
around it only invites the model to work past it.

Two consequences worth planning for:

- A scheduled run cannot use a consenting tool. Nobody is there at 03:00, so
  `requestConsent: true` parks the run until it times out. A Trigger's toolset
  needs `requestConsent: false`, which means a write path and a scheduled path
  cannot share a Toolset. Keep them separate CRs and say why in the header.
- A mock must announce itself. Where the endpoint is not wired up yet,
  returning success is deliberate - a failure would stop a cursor and the chain
  would never be exercised. So disclosure is what keeps it safe: the tool

  description and the prompt must both require the summary to say nothing was
  actually sent.

## Verify

```bash
asgard-cli gate               # every local check, the lint step included
asgard-cli verify <project>   # or one step alone, while iterating
```

Never run `helm lint` by hand: without the reserved `asgard` values file
that `gate` supplies, every chart that labels anything fails. `asgard-cli gate
--help` says why.

The xref check resolves the `(workflow, entry)` pairs. It cannot check the URL,
the body, or the parsing - and a wrong `configs[].name` lints clean, passes CRD
validation and then does nothing at runtime, because that field is a free-form
string.

So exercise it for real before trusting it:

```bash
# call the API directly with the same body the expression builds
curl -X POST "<endpoint>" -H 'Content-Type: application/json' -d '<body>'
```

Compare that against what the tool returns in a conversation. Anything the two
disagree about is in your expressions.
