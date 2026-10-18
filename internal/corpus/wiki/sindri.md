---
group: Products and scope
description: Project, the home page and @mentions, the Sandbox and its files, the governance gate, the Global Directory and the Syncer that writes its AGENTS.md
---
# Sindri - Agent Hub

Where published agents run. Sindri offers no creating or editing - agents are
built in Odin and published here.

## Project

The switcher at the top of the screen, standing for one real business situation.
A Workspace can hold several Projects - retail, semiconductor, financial
investment - each publishing its own agents.

A Project is decided at build time. In Sindri a user can only switch between the
Projects they have access to, not create one. Switching changes the Available
Agents, Routines, Directory and My Chats.

## The home page and @mentions

The centre of the home page is a prompt box, with the published Available Agents
listed below it. Each card shows a name and a line about what the agent covers.

A user does not have to pick the right agent first. The selector above the
prompt box defaults to Sindri rather than any one agent.

What the runtime gives a turn: its agent holds the capabilities of every Agent
the user may use in the Project - their Toolsets, Skillsets and semantic layers -
and does the work itself. No Agent runs as an assistant of its own. For each
Agent the turn's agent is shown one capability block: the `description`, the
`sampleQuestions` as example requests, what the Agent provides (its tools,
skills and semantic models, listed by the platform under the names the agent
calls them), and the Agent's prompt (Persona, Task, Context, Format).

A message that `@`-mentions an agent by its alias marks that capability as the
most relevant one: the agent gives its guidance priority and starts from the
resources it provides, and still uses everything it holds.

So a Managed Agent's `description` says what the capability covers rather than
introducing the agent: it is the one line on the card a user sees, and the text
the agent reads to decide when that Agent's guidance applies.

After sending, the screen shows, in order: "Thought for a moment", an
automatically generated title, and the reply. A Subagents panel appears only
when the agent starts a general-purpose subagent for a separate sub-task; a
Managed Agent never appears there.

Below the prompt box are two more selectors - the model tier (the Builtin series)
and Reasoning effort - and a paperclip for attaching files.

### The governance gate

When an agent decides it needs to change data in a system, it stops and asks
which option the user wants, then raises an approval dialog. Nothing runs until
the user allows or refuses.

This is what `requestConsent` looks like to the person. A scheduled run has
nobody to press the button.

There is no feature page for the approval gate, under Sindri or under Odin. The
screens are documented in one place: the Sindri case study
[門市缺貨 -> 跨店調撥（使用者視角）](https://docs.asgard-ai.com/docs/product-suite/sindri/case-studies/retail-stockout-transfer),
which the home page links to for this. Its dialog names the Toolset and the
tool being called, shows the call's input expandable, counts the pending calls
(1 / 3), and offers allow for this conversation, allow once, or refuse - the
runtime's `ALLOW_ALWAYS`, `ALLOW_ONCE` and `DENY_ONCE`. Customers ask for
documentation of it, because "important actions can be confirmed by a person
before they run" often appears on their acceptance list; that case study is the
page to link.

What else there is:

    the screen        `../wiki/screenshots.md`, the four-image sequence.
                      Crop the dialog first - it names a `ts-` prefix and an
                      internal tool name
    the mechanism     `../usecase/write-path.md`
    the limit         a schedule cannot approve anything, so a gated tool and a
                      scheduled run cannot share a Toolset

## Routines

A Routine is a recurring piece of work a Sindri user sets up themselves: a name,
a description, the instruction sent as the first message of each run, the
Directory every run is filed into, and a schedule. The main agent is Sindri
unless a Flow Agent is picked; with Sindri, a model, a reasoning effort and the
agents to mention can be set, and they seed the first turn only. A Directory is
required, and a Directory a Routine uses cannot be deleted.

The schedule is a cron expression in the owner's time zone, at most once an
hour; Every hour, Daily, Weekdays and Weekly are presets of it, and a Routine
with no schedule runs only from Run now. A scheduled run starts up to a few
minutes after the set time, never on it. Each run is a whole conversation the
owner can carry on, and one that stops to ask something waits until somebody
replies. A scheduled run that fires while the previous run is still going fails
with "The previous run has not finished yet" rather than queueing. Finished
runs are purged with their conversations after a retention period, 90 days
unless the platform is configured otherwise, and deleting a Routine deletes its
conversations. A Routine can be paused with its Active box; a paused one drops
out of the Routines list until Show disabled is ticked there.

A Routine belongs to one person. Only its owner sees it, its runs are billed to
the owner, and every run first checks that the owner is still a member of the
Project: if not, the run fails and the Routine is disabled, because nobody else
could see it or act on its failures.

A Routine is not a chart's to write: no CR describes it, and it lives in the
Project it was created in. It is the answer when one person wants an agent's
work repeated on a schedule and reviews each result. A `Trigger` in the chart
is the answer when the run needs no person, or when the work has to outlive
whoever set it up - it cannot use a tool that asks for consent -
`../usecase/trigger.md`.

## My Chat and Directory

Two ways of organising conversations:

| | My Chat | Directory |
|---|---|---|
| structure | each conversation stands alone | a folder holding one topic's conversations and files |
| suited to | one-off questions | a long-running subject returned to repeatedly |

A Directory has Chats and Files tabs. Conversations opened inside it stay under
that topic, and files uploaded to Files are available to the agent in every
conversation of that Directory.

Each conversation's menu offers Share (add an email to the access list, or copy a
link), Rename and Delete.

## The Sandbox and its files

Every conversation has a Sandbox of its own behind it, and that is where the
agent's queries, computation and file access happen. The Panels menu beside the
conversation title opens a Files panel, which is that Sandbox's file browser.

A Directory's Files tab and a conversation's Files panel show the same data -
the conversation's Sandbox working directory is mounted into that Directory's
shared file space.

## User settings

Settings, bottom left, has two tabs.

General - Language (the interface's own language), Spoken Language (the
user's preferred language, which decides what Sindri answers in; one not on the
list can still be detected automatically) and Appearance. Account and password
changes happen in the Management Console; this tab only shows the current account
and last sign-in, with an Edit in Console link.

Personalization - Base Tone, Characteristics (Warm, Enthusiastic, Headers &
Lists, Emoji, each with a Default) and Memory.

Memory's "Reference saved memories" toggle lets Sindri store and draw on earlier
interactions, with a Manage link for reviewing them. This is a per-user
memory, distinct from an agent's prompt and from the Global Directory.

## Global Directory

Shown in Odin under Agent Hub -> Configuration -> Global Directory. Its
contents are shared with every Agent Hub conversation in that Project and look
the same to everyone. Agents can read but not change them.

Behind the page is a SourceSet the platform creates in every namespace, and a
chart reaches it through the reserved value
`.Values.asgard.agentHubGlobalSourceSetName`. Durable working rules go in a
top-level `AGENTS.md`, which the agent reads on every turn; other reference
material can be filed however suits.

A chart writes `AGENTS.md` with a `static-content` Syncer: one Syncer, one
file, the content in the CR's own `text`. `asgard-cli add agentsmd` generates
it, and the served skill `asgard-cr-verification` lists what applies cleanly
there and is still wrong. The repository is then where the rules live, and the
text that used to be kept in the repository and pasted into the page by hand
is that Syncer's `text` instead.

The generated Syncer is suspended and fires on each deploy, so:

- the file changes when a deploy changes it, and not between deploys;
- Odin shows a suspended Syncer as not Active wherever it lists one, and its
  Manual Trigger still runs it;
- an edit made to `AGENTS.md` on the page stays until the next deploy writes the
  Syncer's text over it.

The page can still upload and edit files, and that is the way to put up a file
no chart owns.

## Corresponding extracts

Sindri only runs published agents and produces no CRs of its own. Building them
is `../usecase/agent-hub.md` and `../usecase/flow-agent-supervisor.md`.

## Sources

- [Project](https://docs.asgard-ai.com/docs/product-suite/sindri/features/project),
  [home](https://docs.asgard-ai.com/docs/product-suite/sindri/features/home)
  - asgard-docs `6261fdff`
- [My Chat](https://docs.asgard-ai.com/docs/product-suite/sindri/features/my-chat),
  [Directory](https://docs.asgard-ai.com/docs/product-suite/sindri/features/directory),
  [general](https://docs.asgard-ai.com/docs/product-suite/sindri/features/settings/general)
  and [personalization](https://docs.asgard-ai.com/docs/product-suite/sindri/features/settings/personalization)
  - asgard-docs `6261fdff`
- [Routines, in the 2026-09-24 release notes](https://docs.asgard-ai.com/docs/help-community/release-notes/20260924)
  - asgard-docs `7e02bc8`
- [retail stockout transfer](https://docs.asgard-ai.com/docs/product-suite/sindri/case-studies/retail-stockout-transfer)
  and [Configuration](https://docs.asgard-ai.com/docs/product-suite/odin/features/agent-hub-configuration)
  - asgard-docs `6261fdff`

**Checked:** asgard-docs `6261fdff` - every page under
`docs/product-suite/sindri/` and `docs/product-suite/odin/features/agent-hub-configuration.mdx`;
asgard-core #368 - asgard-core `internal/bpcontroller/server/sandbox_orchestration.go`
(each Agent's Toolsets, Skillsets and semantic layers are aggregated onto the
turn's agent) and asgard-core `internal/processor/helper/clidriver_run.go`
(`AgentCapabilitiesNote`: the capability block, and what an `@`-mention means);
asgard-core `478cf5d6` asgard-core `internal/models/consent.go` (the three
consent results); the Agents a turn holds against asgard-agent-hub-api
`b88b7d4` asgard-agent-hub-api `internal/service/agent.go`
(`ListAccessibleAgentNamesInProject`). Routines against asgard-agent-hub `3c317865`:
asgard-agent-hub `internal/model/routine.go` (the fields, the required
Directory, the system-side disable reason and the overlap reason),
asgard-agent-hub `internal/service/routine_schedule.go` (the cron form and the
hourly floor) and asgard-agent-hub `cmd/main.go` (the jitter and retention
defaults); against asgard-agent-hub-api `b88b7d4e`:
asgard-agent-hub-api `internal/service/routine_executor.go` (the owner's
membership check, the billing gate, the main agent); and against
asgard-ai-agent-hub-web `33d002a8` `src/components/routine/routine-list.tsx`
(Show disabled) and asgard-ai-agent-hub-web `src/types/routine.ts`. The
Global Directory written from a chart against asgard-kube `d0f8a8e`
(`staticContent`, and the destinationPath rule per class), and at asgard-iac
`cece0c2` against asgard-iac `internal/helm/values.go` (the reserved value) and
asgard-iac `internal/apply/cluster.go` (`SyncersToFire` reads only
auto-fire-on-rollout, never syncer-suspend).

**Unchecked:** Odin does not list the Global Directory's Syncers yet - the
Configuration page shows its files only - so the not-Active display is taken
from a Drive's Syncers tab, not from that page. That an `@`-mention changes which tools the agent starts from
has been observed in asgard-core's own end-to-end test, not on a live Agent Hub.
The asgard-docs pages above still describe the home page in terms of
delegation; this page follows the platform source where they differ.
