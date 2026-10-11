---
group: While building
description: Trigger and API, and why only cron is left
---
# Trigger and API

Two entry points that start a run without a user conversation.

| | Trigger | API |
|---|---|---|
| started by | a schedule | an external system calling in |
| CRs | `Trigger` + its entrypoint `Workflow` | `Workflow` + `Toolset` |
| generate with | `asgard-cli add trigger` | `asgard-cli add httptool` / `querytool` |

## Trigger

Starts a conversation with an agent on a schedule. The list shows Name, Schedule,
Active, Last run.

| field | required | |
|---|---|---|
| Name | yes | |
| Description | | what this schedule is for |
| Schedule | yes | a five-field cron expression or an `@descriptor`, plus a Timezone (Asia/Taipei by default) |
| Model | | a built-in model tier, not one of the project's Agents |
| Prompt | yes | the agent's role, and what each run has to finish |

Two collapsible sections follow: Advanced Sandbox Settings and Advanced. The
second sets what the Trigger says when it opens a conversation, which is
`spec.message` in the CR: optional, and with no default in the schema, so an
empty one is filled in when the Trigger fires rather than stored. In the CR,
`cron.timeZone` is required and has no default; Asia/Taipei is the form's
default, not the platform's, so a chart writes it out.

### Only cron is left

`TriggerClass` now has one value, `cron`. The google-sheet, google-drive,
google-mail, onedrive, onedrive-workbook and imap classes were removed - each of
them had the platform decide how to read a customer's data, which meant inventing
conventions the customer then could not hold to. With cron the customer decides
the shape of the data, and one firing can work through a whole batch.

Settings -> Connection still lists a "For Trigger" group (Google Drive, Google
Sheets, OneDrive, OneDrive Workbook). Those correspond to the removed classes,
so that group is stale.

### The schedule

A five-field cron expression or an `@descriptor`, copied verbatim into the
derived CronJob. `../usecase/trigger.md` has the grammar and how it is
validated.

A SourceSet syncer's schedule has no pattern in the CRD either.

## API

Publishes a workflow as a callable HTTP API. The list shows Name, Group,
Description, Active, Last Modified. Creating one needs only a Name; the
Description is optional.

### As a webhook, which is what a customer usually means

"When an order arrives, do X" is this, not a Trigger: an external system calls
in when something happens rather than us checking on a schedule. The shape is
three stages and each is a processor - see [`processors`](../wiki/processors.md):

    the external system  ->  the endpoint
                               |
      1. validate-payload      the Payload Schema, a Secret Signature to prove
                               where it came from, and the allowed Content-Type
                               |
      2. whatever it does      query, model, http-request - the ordinary middle
                               |
      3. push-message          what the caller gets back. The documentation
                               calls this page Response, at
                               `processor/automation-tool-response`; there is
                               no `response` type, and a chart writes
                               `push-message` scoped to `automation_tool`

Enable the Secret Signature. Without it the endpoint runs whatever anyone who
finds the URL sends it, and a webhook URL travels: it is pasted into somebody's
CI, their vendor console, a ticket.

The endpoint is the same URL a chat message goes to:

    POST {{base_url}}/ns/{{namespace}}/bot-provider/{{name}}/message/sse

So an inbound webhook and a person typing reach the platform the same way, and
`../wiki/api.md` describes the request and its SSE response for both. What
differs is what is on the other end of the Workflow, not the route in. The API
reference prints it with a `/generic/` segment, which is a legacy route;
`../wiki/api.md` says why the one above is the one to hand over.

A webhook and a schedule are not interchangeable, even though both start a run
with nobody watching. A webhook fires when their system decides; a Trigger fires
when we decide. If the customer cannot make their system call out, a schedule is
the fallback and it will always be later than the event.

## Before writing the chart

`../usecase/trigger.md` has the rules a scheduled run needs: the cursor and
the cold start, why not to write a BotProvider yourself, and why a schedule
cannot use a tool that asks for consent. Not repeated here.

## Sources

- The webhook shape, its three stages, the Secret Signature and that the
  endpoint is the same one:
  [Webhook integration](https://docs.asgard-ai.com/docs/developer-reference/examples/webhook-integration)
  - asgard-docs `f00e0ee`

- [Trigger](https://docs.asgard-ai.com/docs/product-suite/odin/features/automation-trigger)
  - asgard-docs `95a27895`
- [API](https://docs.asgard-ai.com/docs/product-suite/odin/features/automation-api)
  - asgard-docs `ffed9a00`
- [Connection](https://docs.asgard-ai.com/docs/product-suite/odin/features/settings/connection)
  - asgard-docs `6261fdff`
- Cron being the only class left: [asgard-kube](https://github.com/asgard-ai-platform/asgard-kube)
  `3da0365` - `TriggerClass`, `TriggerCronSpec`, `TriggerSpec.Message`. The
  `schedule` field carries no pattern: the grammar is whatever `batch/v1`
  CronJob accepts, validated by a real cron parse in an admission webhook rather
  than by a regex in the schema

**Checked:** `TriggerClass`, `TriggerCronSpec` and `TriggerSpec.Message`
against asgard-kube `3da0365` `pkg/apis/asgard/v1alpha1/types.go` and
`crd/asgard-ai.com_triggers.yaml`; the Trigger shape against the one `Trigger`
the extracts were written from (`../wiki/coverage.md`); the form fields against asgard-docs
`95a27895` `docs/product-suite/odin/features/automation-trigger.mdx`.

**Unchecked:** the Trigger and API forms themselves, which only a Console
account can show.
