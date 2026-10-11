---
group: Credentials
description: "an API that will not take a static key: the two-call token chain"
---
# An API that needs a token first

Service-to-service auth against an API that will not take a static key: fetch a
token, then call the thing you wanted. Two HTTP calls in one Workflow, and the
mechanism that carries the token between them is not obvious.

**Seen in:** a notification path built against a corporate mail API with client
credentials and held behind a mock. The credential the customer then issued did
not fit that API, and the path that shipped is one call with a static key; the
mock-then-real rollout and the override list below are from the path that
shipped.

**Checked:** against a production deployment's chart, not named here, at two
versions of `projects/internal/chart/app/templates/workflow/wf-send-mail.yaml`:
the two-call client-credentials version, and the later one, read with
`values-dev.yaml` and `app/values.yaml` - the mock switch, and
`overrideRecipients`, a list. Against asgard-core `478cf5d6`:
asgard-core `internal/processor/task/http_request.go` (which configs are headers, what
`httpResponse` holds, and that a status of 400 or above takes `failure`) and
asgard-core `internal/bpcontroller/server/bp_controller.go` (where `prevPayload`
is set).

**Unchecked:** the two-call chain was written in one deployment and replaced
before it ever sent, so no deployment has run it.

Read the platform side first: `../wiki/api.md` -
the endpoint, the SSE event sequence, and the four integration patterns. This page assumes you have.

## When this shape, and when not

Use it when the API's auth is OAuth 2.0 client credentials - no user, no
consent screen, a service acting as itself. Corporate APIs, most cloud vendors'
management APIs, and anything behind an identity provider are this.

A per-user credential is a different shape. When the token belongs to the
person talking to the agent rather than to the service, it arrives in the
BotProvider payload every turn and lands in the sandbox through a hook - see
`../usecase/per-turn-credentials.md`.

Do not use it when:

- a static key works. A header with a key from the release's Secret is one processor
  instead of two, and no token to expire. Read
  `../usecase/external-api.md` for that shape - the whole of it applies
  here too, and this extract only adds the token step.
- the auth is per user. Client credentials authenticate the *service*.
  If the API needs to know which human is asking - and to enforce what that human
  may see - a token minted from a client secret is the wrong credential, and
  using it means your agent can reach everything any user could. Escalate
  that decision rather than implementing it.
- the token needs caching. It cannot be. See below.

### The cost

A token is fetched on every single call. `http-request` sends one request, and
nothing in a Workflow persists across turns, so there is no cache to put a token
in. A path handling single digits of calls per run can ignore this; one handling
thousands per minute cannot, and that is a reason to reconsider whether the agent
should be calling this API directly at all.

## Generate it

    asgard-cli add httptool notify --project <project> --toolset ts-notify

Then add the token processor in front of the call, and wire it as below. There is
no generator for the two-step form: the second call's shape depends entirely on
the API, and a generated guess would have to be undone.

What the generator does write, and you should keep: the display annotation,
the workflow-set labels, and the `variables` block - all three fail silently.

## The skeleton

```yaml
apiVersion: asgard-ai.com/v1alpha1
kind: Workflow
metadata:
  name: wf-send-notice
  annotations:
    asgard-ai.com/workflow-name: "寄送通知"
spec:
  variables:
    # THE ONLY WAY A SECRET REACHES A WORKFLOW. A config takes value, expression
    # or template - there is no valueFrom on a config - so a credential has to
    # come in here and be read as vars.<name> inside an expression.
    - name: clientSecret
      valueFrom:
        secretKeyRef:
          name: {{ include "<chart>.appSecretName" . }}
          key: graph_client_secret

  entries:
    - name: send
      handlingProcessor: get-token       # the token step is the entry, not the call
      tooling:
        name: send_notification
        description: |-
          Sends one notice. Returns ok: false when it did not go out - do not
          treat that as sent.
        allowUploadFile: false   # required, and it has no default
      inputSchema: |
        {
          "type": "object",
          "properties": {
            "to":      { "type": "string" },
            "subject": { "type": "string" },
            "body":    { "type": "string" }
          },
          "required": ["to", "subject", "body"]
        }

  exits:
    - name: finish

  processors:
    # (1) Client credentials. No user is involved.
    - name: get-token
      type: http-request
      configs:
        - name: url
          value: "https://login.example.com/<tenant>/oauth2/v2.0/token"
        - name: method
          value: POST
        - name: parseJson
          value: "true"
        # Any config key that is not url/method/body/parseJson is sent as a
        # header. That is the whole mechanism - there is no headers: block.
        - name: Content-Type
          value: application/x-www-form-urlencoded
        - name: body
          expression: |
            "grant_type=client_credentials"
              + "&client_id=" + encodeURIComponent("<client-id>")
              + "&client_secret=" + encodeURIComponent(vars.clientSecret)
              + "&scope=" + encodeURIComponent("https://api.example.com/.default")

    # (2) The call you wanted. httpResponse here is STILL (1)'s response.
    - name: send
      type: http-request
      configs:
        - name: url
          value: "https://api.example.com/v1.0/send"
        - name: method
          value: POST
        # A 202 with an empty body has no JSON to parse. Leaving this true
        # leaves a parse warning on every successful call.
        - name: parseJson
          value: "false"
        - name: Content-Type
          value: application/json
        - name: Authorization
          expression: '"Bearer " + httpResponse.json.access_token'
        - name: body
          expression: |
            JSON.stringify({
              to: prevPayload.to,
              subject: prevPayload.subject,
              body: prevPayload.body,
            })

    # (3) Tell the agent what happened. Without this a failure looks like success.
    - name: respond
      type: push-message
      configs:
        - name: payload
          expression: |
            ({
              ok: httpResponse.statusCode >= 200 && httpResponse.statusCode < 300,
              statusCode: httpResponse.statusCode,
              body: httpResponse.body,
            })

    - name: respond-error
      type: push-message
      configs:
        - name: payload
          expression: '({ ok: false, error: prevError })'

  relationships:
    - from: {processor: get-token, relationName: success}
      to: {processor: send}
    # A token that will not mint has to answer too. An expired secret takes this
    # branch, and without it the tool call ends in silence.
    - from: {processor: get-token, relationName: failure}
      to: {processor: respond-error}
    - from: {processor: send, relationName: success}
      to: {processor: respond}
    - from: {processor: send, relationName: failure}
      to: {processor: respond-error}
    - from: {processor: respond, relationName: success}
      to: {exit: finish}
    - from: {processor: respond-error, relationName: success}
      to: {exit: finish}
```

It goes in `projects/<project>/chart/app/templates/workflow/wf-<name>.yaml`, or
under `templates/tool/` if the chart groups tool workflows there.

## How the token crosses between the two calls

A processor's configs are evaluated immediately before that processor runs. So
when `send`'s `Authorization` is computed, `httpResponse` still holds
`get-token`'s response - which is exactly where `access_token` is. By the time
`respond` is evaluated, `httpResponse` has been replaced by `send`'s response.

This looks like a bug but is how it works. Three consequences:

- `httpResponse` means "the most recent successful HTTP response", not "this
  processor's". Every processor passes the context on, and only an
  `http-request` that succeeds replaces it. Insert another `http-request`
  between the two and the token is gone; a `push-message` or `update-context`
  between them leaves it.
- a failed `http-request` does not touch it. On `failure`, `httpResponse` is
  still the previous call's, so in `respond-error` it may be the token response;
  read `prevError` there, never `httpResponse`.
- `prevPayload` stays the tool's arguments for the whole run. An `http-request`
  copies the context through and adds `httpResponse` beside it, and
  `prevPayload` is replaced only when the next turn or tool call arrives, which
  is why `send` can still read `prevPayload.to`.

## Designing the credential and its scope

These parts need a conversation with the customer's IT:

- Application permission, not delegated. Client credentials cannot use a
  delegated permission - there is no user to delegate. Asking for the wrong one
  produces a token that mints fine and is refused by the API.
- Scope is `<resource>/.default`, not a list of individual scopes. Application
  permissions are granted on the app registration; `.default` says "whatever this
  app was granted".
- Narrow the permission at the provider, not in the prompt. A mail-send
  permission is global by default. One deployment's customer required an access
  policy restricting the app to a single sender address, and that requirement came
  from the customer explicitly - write it down as a decision record, because the
  next person will not know it was asked for.
- Only the secret is a secret. Tenant id, client id, sender address are not:
  they go in values, so they are reviewable in a diff. Declaring them under
  `appSecret` instead makes them invisible for no benefit - a `chartValue` is in
  the plan report the reviewer reads, and a Secret key is not.

### The rollout that made this safe

The deployment this came from built the real sender and pointed the tool at a
mock for days, with the mock's contract - entry name, `tooling.name`,
`inputSchema`, the `ok` field - byte-identical to the real one. Every run was
reviewable: who would have been mailed, and what the message said. Switching was
changing one `entrypoint`.

It also kept an `overrideRecipients` list: non-empty and every message goes
to those addresses instead of the real one, so routing is verifiable without
involving anyone. Emptying it is the irreversible step that turns the feature
on, and the two environments empty it at different times - dev keeps an
engineer in it indefinitely, prod empties it at go-live.

It is a chart value, not a CR field. The workflow declares it as a
`variables` entry - `value: {{ join "," .Values.mail.overrideRecipients | quote }}` -
because a variable's value is a string, so what the body expression reads as
`vars.overrideRecipients` is one comma-joined string it splits back, and an
empty list is an empty string meaning "no override". Do not look for it in the
CRD.

Not the same thing as a BCC list. An override *redirects*, temporarily; a
BCC leaves the real recipient receiving and adds a silent copy, permanently.
One deployment carries both, and confusing them sends mail to a customer that
was meant to go nowhere.

Configs are evaluated when the tool is called, not when the chart is applied.
So the unfinished real workflow could sit in the chart referencing a secret key
that did not exist yet, and nothing failed, because nobody called it. That makes
this rollout possible. It also means a broken config does not fail the deploy;
it fails at runtime on first use.

## Verify

    asgard-cli verify <project>

It checks the display annotation, the workflow-set labels, and that anything
pointing at this Workflow names an entry it actually declares.

What it cannot check, and what to check by hand:

- that the token endpoint and scope are right. Call it with curl using the
  same body the expression builds, and confirm you get a token.
- that the API accepts that token. A token can mint and still be refused: a
  wrong permission type fails only at the second call.

- that every failure branch goes somewhere. A processor with no `failure`
  relationship ends the run silently, and the agent sees a tool that returned
  nothing rather than a tool that failed.
- the status code range. `== 200` is wrong for anything that answers 201 or
  202, and both are normal for a send.
