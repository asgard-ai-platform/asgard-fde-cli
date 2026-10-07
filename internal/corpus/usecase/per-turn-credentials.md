---
group: Credentials
description: the agent calls as the person talking to it, on a short-lived token
---
# A credential the caller supplies, per turn

The agent calls a customer API as the person talking to it, using a
short-lived credential the caller puts in the request rather than one the chart
holds. The query scope is fixed by the credential, not by the prompt.

**Seen in:** a commerce back-office where the front end passes a user JWT and an
optional per-brand API token on every turn, and a public support widget where the
site forwards a scope-limited token so the agent can read that customer's own
orders and nothing else.

**Checked:** written directly from a supervisor's SandboxBlueprint hook in asgard-freyr-kube `3ebd2be`, whose comment there records the incident that shaped it. Checked against the CRD's hook events. The public-widget variant against asgard-industry-demo-generator `718cc0e`, `retail/chart/app/templates/supervisor/customer_service/` and `retail/skills/customer-service-api/SKILL.md`.

Read the platform side first: `../wiki/api.md` covers the endpoint, the SSE
event sequence, and the four integration patterns. This page assumes you have
read it.

## When this shape, and when not

Use it when which rows the agent may see depends on who is asking. A customer
service agent reading "your orders", an internal tool acting with the operator's
own permissions, anything multi-tenant where the tenant is decided per request.

The alternative that looks simpler is a semantic layer or a query tool holding one
service credential, with the prompt told to filter by the caller's id. Do not do
that for per-user data. The model then chooses which id to query, and a user can
talk it into choosing a different one. Here the scope is enforced one layer
down: the credential itself cannot see anything else.

Do not use it when:

- the credential belongs to the *service* rather than to a user - a static API key
  or an OAuth client-credentials token is `../usecase/api-oauth.md`
- nothing about the answer depends on who asked. A public catalogue does not need
  per-turn identity, and adding it means the front end now has to hold something

## The shape

    caller  ->  BotProvider payload   identity + token, every turn
      -> Workflow                     prevPayload.* carries them
        -> SandboxBlueprint
             hooks  user-prompt-submit    writes them into the sandbox filesystem
      -> a SkillSet that reads that file and calls the API

The credentials never enter a CR spec, never reach a Secret, and are not part of
what helm deploys. They exist for the length of one turn.

## Generate it

There is no generator kind for this - it is a hook expression on a blueprint the
`flowagent` or supervisor generator already wrote. Add it to an existing
`SandboxBlueprint`.

## The skeleton

```yaml
spec:
  hooks:
    expression: |-
      (() => {
        const cfg = {
          api_base_url: {{ .Values.<name>ApiBaseUrl | quote }},
          tenant: prevPayload.tenant || null,
          user: prevPayload.user
            ? {
                id: prevPayload.user.id,
                auth: prevPayload.user.auth?.access_token
                  ? { access_token: prevPayload.user.auth.access_token }
                  : null,
              }
            : null,
        };
        const json = JSON.stringify(cfg, null, 2);
        return [{
          event: "user-prompt-submit",
          handlerType: "command",
          commandHandler: {
            command: `umask 077; cat > /tmp/.cfg.json.tmp <<'EOF'\n${json}\nEOF\nmv /tmp/.cfg.json.tmp /tmp/cfg.json`
          }
        }];
      })()
```

## Fields that are not obvious

### It must be `user-prompt-submit`, not `session-start`

This is why the shape is written this way; a live incident found it.

A `session-start` hook is part of the Sandbox CR spec. A JWT carries `jti` and
`iat`, so its string changes on every issue; a hook whose content changes bumps
the CR's generation, and a generation bump recreates the pod, mid-conversation.

`user-prompt-submit` is re-evaluated by the driver from that turn's payload and
delivered with the task. It never enters the spec, so nothing is recreated. It
also means the token is fresh every turn, which also fixed a separate
problem: a token that had been captured once went stale after eight hours.

The two remaining hook events are not an option either: `pre-tool-call` and
`post-tool-call` were never implemented and declaring one is a silent no-op.

### Three details in the write command

```
umask 077; cat > /tmp/.cfg.json.tmp <<'EOF' ... EOF; mv ... /tmp/cfg.json
```

- `umask 077` so the file holding a token is mode 600.
- A quoted heredoc plus `JSON.stringify` so token contents cannot break out
  into the shell. With a raw interpolation, a token containing a quote injects
  into the command.
- Write a temp file and `mv`, because the replacement has to be atomic. A user who
  sends a second message mid-run triggers a rewrite, and a tool already running
  would otherwise read half a file.

### Optional fields become `null`, and the reader must expect it

A payload omits what does not apply - no tenant, no connected brand, an
unauthenticated visitor. Write `null` rather than omitting the key, and say in
the consuming skill that `null` is a normal value. A skill that assumes the field
is present fails only on the anonymous path, which is the one least often tested.

### What the prompt does

The prompt does not enforce the scope. Tell the agent what it may do when the
credential is absent, and make that branch explicit, because "no token" is a
normal state, not an error:

    <<if logged in>> you may look up this customer's own orders through the
    API, using the connection details injected this turn. Never guess a URL or
    rewrite the host.
    <<else>>       you cannot look up any order. Say that signing in is
    required, and do not try another route.

Also name the injected values as the only ones it may use. That is an extra
guardrail; the credential's own scope is the control.

## Verify

`asgard-cli check` and `verify` see none of this - it is a JavaScript expression
in a string field, and no schema validates its contents.

- Render the chart and read the hook back. An expression that throws is a runtime
  failure on the first turn, not a deploy failure.
- Send one turn with the credential and one without, and confirm the second is
  refused rather than answered from a stale file.
- Confirm the file is mode 600 in the sandbox, and that a second message
  mid-run does not corrupt it.

## Source

- Written from a commerce back-office supervisor's `SandboxBlueprint` in
  asgard-freyr-kube `3ebd2be`, whose hook comment records the 2026-08-21 pod-recreation incident that moved it off
  `session-start`.
- The public-widget variant is the same mechanism reached from the other side,
  read out of a retail customer-service supervisor in asgard-industry-demo-generator
  at `718cc0e`.
  Its BotProvider is `authMode: api-key`, so it is the site's back end that
  calls it and forwards a short-lived member token instead of a user JWT. The
  token is optional there: a guest turn writes `user: null` into the same file,
  and the skill still answers what needs no token, such as stock. So in that
  variant a turn without a credential is refused only for the member's own data,
  not refused outright.
- `SandboxHookEvent` is from the platform CRD. That `pre-tool-call` and
  `post-tool-call` are a silent no-op is not: the generated CRD lists both
  in the enum with no marking. It is stated in the API types themselves
  ([asgard-kube](https://github.com/asgard-ai-platform/asgard-kube) `cbd8d70`,
  `pkg/apis/asgard/v1alpha1/types.go`): "Deprecated: never implemented ...
  declaring a hook with either event is a silent no-op." Validation will accept
  one, so nothing catches this for you.
