---
group: Products and scope
description: Project, delegation, the Sandbox and its files, the governance gate
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

## The home page and delegation

The centre of the home page is a prompt box, with the published Available Agents
listed below it. Each card shows a name and a line about when to delegate to it.

A user does not have to pick the right agent first. The selector above the
prompt box defaults to Sindri rather than any one agent, and Sindri decides
whether to delegate and to whom.

What the runtime gives that decision: Sindri is an orchestrator that already
holds the union of every published agent's Toolsets, Skillsets and semantic
layers, and each agent is offered to it as a subagent. For each one it sees the
`description` with the agent's `sampleQuestions` appended under "Example
questions this agent handles", and the agent's full fused prompt (Persona, Task,
Context, Format). Its standing instruction is to do the work itself by default
and spawn a subagent only for parallel work, to keep its own context lean, or
when the user asks for a specialist by name.

So a Managed Agent's `description` is routing text rather than a
self-introduction: it is the line labelled "when to spawn", the one line on the
card a user sees, and the text a sample question is attached to. It is not the
only thing the decision reads - the prompt is in front of it too - and a
question Sindri can answer with the union of tools may never be delegated at
all.

After sending, the screen shows, in order: "Thought for a moment", an
automatically generated title, a Subagents panel naming the agents delegated to
and the steps they are running, and finally the reply.

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
that topic, and files uploaded to Files are available to whichever agent is
delegated to.

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

Configured in Odin under Agent Hub -> Configuration -> Global Directory. Its
contents are shared with every Agent Hub conversation in that Project and look
the same to everyone. Agents can read but not change them; editing happens only
on that page.

The convention is to put durable working rules in a top-level `AGENTS.md` and
file other reference material however suits.

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
asgard-core `478cf5d6` - asgard-core `internal/bpcontroller/server/sandbox_orchestration.go`
(each Agent becomes a subagent, its description gets its sample questions
appended, its Toolsets and Skillsets are aggregated onto the orchestrator),
asgard-core `internal/processor/helper/clidriver_run.go` (`SubagentTeamNote`, the
do-it-yourself-by-default rule) and asgard-core `internal/models/consent.go` (the
three consent results). Routines against asgard-agent-hub `3c317865`:
asgard-agent-hub `internal/model/routine.go` (the fields, the required
Directory, the system-side disable reason and the overlap reason),
asgard-agent-hub `internal/service/routine_schedule.go` (the cron form and the
hourly floor) and asgard-agent-hub `cmd/main.go` (the jitter and retention
defaults); against asgard-agent-hub-api `b88b7d4e`:
asgard-agent-hub-api `internal/service/routine_executor.go` (the owner's
membership check, the billing gate, the main agent); and against
asgard-ai-agent-hub-web `33d002a8` `src/components/routine/routine-list.tsx`
(Show disabled) and asgard-ai-agent-hub-web `src/types/routine.ts`.

**Unchecked:** how often Sindri delegates in practice rather than answering
itself has not been observed on a live Agent Hub.
