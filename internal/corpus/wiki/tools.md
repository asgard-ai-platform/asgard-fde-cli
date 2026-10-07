---
group: While building
description: MCP Server, Skillset and Plugin; hook events
---
# MCP Server, Skillset and Plugin

Three things that are easy to confuse. The difference is what each one holds.

| | holds | CRs |
|---|---|---|
| MCP Server | callable tools - functions, external APIs, data sources | `Toolset` |
| Skillset | instructions the agent reads | `SkillSet` + `SourceSet` + `Syncer` |
| Plugin | a bundle of those, plus Drives, Hooks and Managed Agents | `Plugin` |

In one line: an MCP Server is what the agent can do, a Skillset is what it
should know, and a Plugin is a set of capabilities behind one name.

The agent reads tool descriptions and skills at once, so a subject the two share needs a rule about
which one owns a fact: `../wiki/tool-description-and-skill.md`.

## MCP Server

A Model Context Protocol server, letting an agent reach external tools, data
sources and custom functions.

From Workflow wraps a Workflow as an MCP Server. It takes a Name and a
Description; creating it produces an empty Workflow where the actual tool logic
goes. Maps to `toolsetClass: workflow-tooling`.

From Existing MCP Server connects to one that already runs. Maps to
`toolsetClass: mcp-server`.

| field | required | |
|---|---|---|
| Name | yes | |
| Description | | |
| Transport Type | yes | STDIO or Streamable HTTP |
| Command | yes | the launch command; `npx` and `uvx` are supported |
| Arguments | | space separated |
| Environment Variables | | Name/Value pairs, passed over STDIO |
| MCP Server Volume Mounts | | mount a Drive into the server's environment, with Mount Path, Sub Path and Read Only |

With STDIO, Asgard starts a local process and talks to it over standard
input/output. With Streamable HTTP it connects to an endpoint already running.

## Consent: where it is set, and what it covers

`requestConsent` is a field on the Toolset, per tool - `spec.tools[].requestConsent` -
and not on the Workflow the tool wraps. A Workflow has no say in whether calling
it stops to ask; that decision lives one level up, in the Toolset that exposes
it. So the gate is not in the Workflow, and a second entry added to a Toolset
inherits nothing from the first.

A `workflow-tooling` Toolset does defer. The class is not a reason a tool never
stops to ask. The mechanism, read at asgard-core `623ceb5`:

    asgard-core internal/constants.go
                                   the built-in safe list "only governs asgard
                                   domain tools; mcp__<toolset>__* honor
                                   RequestConsent"
    processor/driverloop           a toolset's tools reach the CLI as
                                   `mcp__<toolset>__<tool>`, so they carry the
                                   prefix consent gates on
    bpcontroller/server            only a tool with `!RequestConsent` enters
                                   `AllowedToolRefs`
    processor/consentpolicy        an `mcp__` tool that is not in that list is
                                   returned as defer

So the single switch is that one field. Everything the agent's own sandbox runs -
Bash, Read, Edit, Grep - is auto-allowed before consent is considered at all,
because those are not Asgard tools.

An `mcp-server` Toolset cannot ask for consent at all. There is nowhere to
write it: `requestConsent` exists only on `tools[]`, and asgard-kube's CEL rule
requires `tools` to be empty for that class. A design that plans to gate an
external MCP server's calls per tool does not work, and the CRD refuses it
rather than ignoring it.

One bypass exists and is not a chart field. `bypass_tool_call_consent` is a
query parameter on the Edge Server's bot-provider endpoint, defaulting to false,
and it treats every tool call in that one request as consented. It is a caller's
switch, so a relay in front of the platform decides whether it is reachable at
all. Ask about that relay before promising that a gate cannot be skipped. The `/json` and `/form` trigger routes set it on every request, so a
tool reached through either never asks.

What a person approves is the tool, not the call. The pause stores the
pending calls with their arguments and sends them to the client, but the resume
does not replay what was shown: the model's session continues from its
transcript, and the decision is looked up by Toolset and tool name with the
arguments ignored. The one call that paused runs with the arguments it paused
with; any other call in the same batch is made again by the model with fresh
arguments, and for the rest of that turn every call to an approved tool is
allowed, whatever it carries and however often. `ALLOW_ALWAYS` records the tool
name too. So a design where the approved content is the deliverable - a
listing, a message, a document - cannot rely on the gate to guarantee that
what goes out is what was approved; carry the approved content through the
workflow as data and have the write read it from there.

On a `generic` BotProvider the request arrives as data - a
`toolCallConsentRequest` field on a plain reply, a `tool_call.consent` SSE event,
and again on rejoin - and the client decides what it looks like. Anyone who can
reach the endpoint with its key and the channel id can answer, with action
`RESPONSE_TOOL_CALL_CONSENT`; the only identity check is that
`X-ASGARD-USER-IDENTITY-HINT` matches the one recorded at the pause, which is
`primary` when nobody sends one. On a public channel the person approving is
whoever holds the conversation, usually the visitor, unless the front end in
between decides otherwise.

**Checked:** against asgard-core `623ceb5` - its
`internal/constants.go`, asgard-core `internal/processor/consentpolicy/consentpolicy.go`,
asgard-core `internal/bpcontroller/server/bp_controller.go` and asgard-core
`internal/edgeserver/handler/bot_provider.go` - and against asgard-kube
`cbd8d70`, its `pkg/apis/asgard/v1alpha1/types.go`.

What is approved, and where it can be answered: asgard-core `478cf5d6`
asgard-core `internal/processor/consentpolicy/consentpolicy.go` (`BuildResumeVerdicts` and
the decider that ignores the input),
asgard-core `internal/processor/driverloop/driverloop.go` (`buildPause`),
asgard-core `internal/bpcontroller/server/bp_controller.go` (`handleConsentPreResume`) and
asgard-core `internal/edgeserver/handler/bot_provider.go`.

**Unchecked:** what a front end shows for the `generic` consent event is the
front end's, and no reference deployment has one on a public channel.

## Skillset

A reusable set of skills an agent loads at run time.

From Scratch takes a Name and optional Search Paths. From Git imports
from a repository.

Search Paths are where the agent looks for skill files, comma separated, and
each path must end in a slash.

The detail page has Files and Settings tabs. Files is that Skillset's own
browser, and Open in Advance Editor edits the skill files in a separate tab.

A Skillset maps to three CRs: `SkillSet` plus its own `SourceSet` plus the
`Syncer` that fills it, 1:1:1. A Skillset a Plugin bundles is the exception:
when the skills all live in one repository, several bundles share one SourceSet
and slice it with searchPaths, at the cost of those Skillsets not being presented
as first-class objects a person picks. For the chart details see
`../usecase/skill-set.md` and `../usecase/plugin.md`.

## Plugin

Packages MCP Servers, Skillsets, Drives, Hooks and Managed Agents into one
reusable sandbox bundle.

Use one when the same agent needs different capabilities on
different turns, and a blueprint decides per request which bundles to load. If
the capability set is fixed, mount the pieces directly and skip this layer.

A Plugin carries Hooks, and a SandboxBlueprint can also declare them directly
in `spec.hooks`; `../usecase/flow-agent-supervisor.md` shows one.

### Hook events

| event | fires | status |
|---|---|---|
| `session-start` | once, when the container starts | usable |
| `session-end` | on SIGTERM | usable |
| `user-prompt-submit` | before each user message reaches the CLI | usable |
| `pre-tool-call` / `post-tool-call` | - | never implemented; declaring one is a silent no-op |

The last row exists only so older CRs stay valid, and nothing catches it for
you: both events are still in the CRD's enum, so a CR declaring one is accepted
and then does nothing. The deprecation is stated only in the `event` field's
description, which the apiserver does not act on. The statement comes from the
API types themselves ([asgard-kube](https://github.com/asgard-ai-platform/asgard-kube)
`3da0365`, `pkg/apis/asgard/v1alpha1/types.go`): "Deprecated: never implemented
... declaring a hook with either event is a silent no-op". A `session-start` hook's
content has to be stable; anything derived from the turn's payload belongs in
`user-prompt-submit`.

## The sandbox's own tools cannot be switched off

An agent runs in a sandbox that is a coding-agent CLI, and that CLI's built-in
tools are present in every sandbox - web search, web fetch, task and schedule
listing. They are not Asgard domain tools, they are not a leak from the agent
hub, and no Toolset or SandboxBlueprint setting removes them. The platform
hard-codes its disallow list to two planning tools and there is no field on any
CRD to opt out.

One reference deployment also denies a built-in skill through the CLI's own
settings file: a SandboxBlueprint `session-start` hook writes a fixed
`permissions.deny` list and `disableWorkflows` into the CLI's user settings in
the sandbox's home directory. The platform does not support that path, so it
can stop working with any sandbox image, and a hook that fails is only logged,
after which the CLI starts with no deny at all. It is a deployment's own
workaround, not a control to offer a customer.

So the control the platform supports is the prompt, which is a weak control:

    a deployment that needs them off   says so in the prompt, explicitly
    that instruction                   must not be deleted as redundant

When a customer asks what the agent can reach, the answer is that it can search
the web unless told not to, and that the instruction not to is a prompt rather
than a permission. "It only sees what you connect" is wrong.

## The card tools the platform adds by itself

Separate from a Toolset and from the sandbox's own CLI tools, the platform
adds these by itself, and no Toolset declares one. Each is registered only where
it can do something, rather than in every conversation:

    show_result_set_table,        only when semantic layers are bound and the
    show_vega_visualization       processor has semanticLayers.dataVisualization
                                  on - the one chart field that decides a card tool
    show_channel_home_download_link, show_canvas
                                  whenever the node has a user-facing reply path,
                                  so not on the silent llm-completion node
    open_sandbox_file,            when there is also a sandbox; the two are always
    open_sandbox_folder           registered as a pair
    open_sandbox_browser          only when that sandbox has the browser sidecar
    update_channel_title          only on the conversational path

They render something for the user or set conversation metadata, and they are on
the consent safe list - none of them stalls waiting for an approval, because
none has an effect beyond rendering.

| tool | what the user gets |
|---|---|
| `show_result_set_table` | a query's rows as a table |
| `show_vega_visualization` | a chart over a result set |
| `show_channel_home_download_link` | a download card |
| `open_sandbox_file` | a card that opens one file in the file viewer |
| `open_sandbox_folder` | a card that opens a directory in the file explorer's tree |
| `open_sandbox_browser` | a card that hands the sandbox's browser to the user |
| `show_canvas` | an HTML/SVG fragment the model wrote, rendered as a card |
| `update_channel_title` | the conversation's title |

The file card and the folder card are not interchangeable; the wrong one
produces a card that always fails. The viewer reads and tails its path
(`fs/file` + `fs/watch`) and the sandbox filesystem API rejects both for a
directory. `open_sandbox_folder` exists because the model had one card and a
folder to show, so it aimed the file card at a directory.

Both can only address paths inside the working directory, because that is
where the file explorer is rooted - including for a file derived from a user's
attachment, which lands outside it. The Agent Hub orchestrator prompt tells the
model so; a chart's own prompt has to say it for itself. An agent asked to unpack an attached `.zip`
extracted beside the attachment and then pushed a card at that directory; the
user tapped a card pointing outside the tree.

`show_canvas` is the one that is not delivered by a handler. The fragment is
the tool's own `html` argument, so it streams to the client *before* the tool
executes - which is why consent on it would leave a half-drawn canvas on screen
waiting for an answer about content the user can already see.

## A query tool's rows are truncated, and the full set is a file

`execute_database_query` returns at most 20 rows to the model, with
`has_more` when there are more. There are two different things to do with the
rest, and the platform's own tool instruction says not to confuse them:

    to show the user      pass `result_set_id` to show_result_set_table
                          or show_vega_visualization - they get every row
    to use it yourself    read `result_set_path`, a JSON file already written
                          inside the sandbox

`result_set_path` holds `{dataConnectorName, sql, rows}`, where `rows` is keyed
by that query's own output column names. Point a script at it - the rows
never enter the model's context, so the size of the result set does not matter.
Do not re-run the query with LIMIT/OFFSET to page rows into context, and do not
open the file with a Read tool. The field is
absent when no file was written, and only then is paging the right answer.

When a customer asks whether the agent can work over a large table: it can, and
the mechanism is a file in the sandbox rather than a bigger context.

## `Toolset.spec.instruction` is gone

Removed from the live CRD. Tool usage guidance now lives on
`Workflow.entries[].tooling.description`. A chart carrying `spec.instruction` is
carrying a field the apiserver no longer knows, and re-adding it is a common
repair to make when guidance seems to be missing.

## Sources

- [MCP Servers](https://docs.asgard-ai.com/docs/product-suite/odin/features/mcp-servers)
  - asgard-docs `ffed9a00`
- [Skillsets](https://docs.asgard-ai.com/docs/product-suite/odin/features/skillsets)
  - asgard-docs `6261fdff`
- [Plugins](https://docs.asgard-ai.com/docs/product-suite/odin/features/plugins)
  - asgard-docs `6261fdff`. That page is short - it covers the list and the New
  Plugin button, and does not document the creation form's fields
- Hook events, the SkillSet trio and the shared-store exception: checked
  against [asgard-kube](https://github.com/asgard-ai-platform/asgard-kube)
  `3da0365` - `SandboxHookEvent`, `PluginSpec`, and `crd/asgard-ai.com_plugins.yaml`,
  `crd/asgard-ai.com_toolsets.yaml`, `crd/asgard-ai.com_skillsets.yaml` - and
  against the reference deployment carrying 29 Plugins, at `62ccbe0`, where
  every Plugin's SkillSet slices the one shared SourceSet with searchPaths
- The settings-file deny: asgard-auto-post-kube `62ccbe0`
  `chart/app/templates/agent/global/sandbox_blueprint.yaml` and
  `requirements/tasks/TASK-006-forbid-builtin-deep-research-skill.md`. That
  the platform does not support its path is per the platform team
- The card tools, when each is registered, the file/folder distinction, the
  working-directory rule and the result-set file: read from
  asgard-core `478cf5d6` `internal/constants.go` - `BuiltinToolCallSafeList`,
  the `ToolName*` constants and the `execute_database_query` tool instruction -
  asgard-core `internal/processor/domaintools/domaintools.go` (`buildTools`),
  asgard-core `internal/processor/domaintools/open_folder_card.go` and
  asgard-core `internal/bpoperator/reconciler/ns_reconciler.go`
  (`workspacePromptSection`). No product documentation covers any of it
- The sandbox's disallow list:
  asgard-core `478cf5d6` `internal/processor/driverloop/driverloop.go`
- The UI form fields: asgard-docs `21c920f6`,
  `docs/product-suite/odin/features/mcp-servers.mdx`, `skillsets.mdx` and
  `plugins.mdx`

**Checked:** against asgard-kube `3da0365` and asgard-core `478cf5d6`, the files
named under Sources.

**Unchecked:** the UI form fields against the Console itself, and how a card
renders in a front end.
