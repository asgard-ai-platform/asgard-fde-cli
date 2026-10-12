---
group: While building
description: "the order: where a credential goes, what to build from it, why Sindri needs no import"
---
# From a credential to an agent someone can talk to

Every other page here describes one object. This one describes the order in
which to build them. The product documentation has a page per screen and the
extracts have a shape per CR; neither states the order.

Read it when a customer has just handed over access and the question is "so what
happens now", and when writing a deck or a handover that has to show them.

## The first fork: where does this credential even go?

The most common wrong turn is at the very first step: the console has a place
called Data Source and people try to put every credential there.

| what the customer handed over | where it goes |
|---|---|
| host, port, database, user, password | Data Source. Nine DB providers only - see [`settings.md`](../wiki/settings.md) |
| an OAuth app for Dropbox / Google Drive / OneDrive / Google Sheets / OneDrive Workbook | Connection - [`settings.md`](../wiki/settings.md) |
| an API key or token for an HTTP API | neither. It is a tool's config, not a data source |
| a chat platform's channel secret and access token | a `BotProvider` - [`integration.md`](../wiki/integration.md) |

An HTTP API has no home under Settings. Data Source takes nine database providers and nothing else; Connection is OAuth to
five named services. A REST API with a bearer token is reached by a tool that
carries its own credentials - either an `http-request` step inside a Workflow, or
an MCP Server started with environment variables. `../usecase/external-api.md`
is the whole shape, including which of the two to take and why.

So API credentials cannot be added as a data source. Take them to the tool that
calls the API.

## The order

Five steps. Each one can be finished and tested before the next, which is the
reason to follow this order rather than start from the agent.

**1. Land the credential.** Data Source, Connection, or a tool's own config, per
the table above. A Data Source has Test Connection on the form - use it
before Save. A network path failure also shows up here first, and it has nothing
to do with the credential: see
[`operations.md`](../wiki/operations.md), because a hosted platform reaching an internal
system needs the customer's firewall opened first.

**2. Turn access into something the agent can call.** Which one depends on what
the source is, and they are not interchangeable:

| the source | what to build | page |
|---|---|---|
| a database the agent should query freely | a Semantic Model | [`semantic-model.md`](../wiki/semantic-model.md) |
| a database, but only a fixed set of answers | query tools in a Workflow | `../usecase/fixed-query-tools.md` |
| an HTTP API | a Workflow with `http-request`, wrapped as an MCP Server | `../usecase/external-api.md` |
| an existing MCP server somebody already wrote | MCP Server, From Existing | [`tools.md`](../wiki/tools.md) |
| documents, manuals, FAQs | a Drive with a Context Index | [`knowledge.md`](../wiki/knowledge.md) |
| how the customer's systems correspond, where their concepts do not line up - status codes, an id written three ways, a word that means two things | a Skillset. This row is often forgotten because it has no credential to ask for | [`tools.md`](../wiki/tools.md) |

New MCP Server has two entries and they lead to different work. From
Workflow asks only for Name and Description, then opens an empty
Workflow editor where the actual tool is assembled; most of the work is there,
not in the form. From Existing connects to a server that already exists,
over STDIO (Asgard starts a local process with a Command, Arguments and
Environment Variables) or Streamable HTTP (an endpoint already running).

**3. Configure the agent.** A Managed Agent is where the pieces meet - see
[`agents.md`](../wiki/agents.md) for the fields. Two of them decide more than the rest:

  - Description is not a self-introduction. It says what the agent's
    capability covers; the agent of each turn reads it, with the sample
    questions and the tools the platform lists for the agent, to decide when
    this agent's guidance applies. Write it as "covers ...: ...", not as "I am
    a helpful assistant for ..." and not as "hand it to me when ...". Leave the
    tool names out; the platform lists them
  - Prompt is four separate fields - Persona, Task, Context, Format - not one
    box. Putting everything in Persona stops working once the agent has two tasks

Then mount what step 2 produced: MCP Servers, Skillsets, Drives, Semantic Model,
and the Browser Configuration switch.

Five built-in templates exist; start from one rather than an empty form: 簡易線上客服, Customer Support, Knowledge Base Q&A, Data Analyst,
General Assistant.

**4. There is no step 4 for Sindri.** This is the question people ask most often:

> Every Managed Agent is published to Sindri. There is no import, no
> install, no "add agent to hub". An enabled agent serves immediately; a
> disabled one stops serving and keeps its configuration.

What a user sees in Sindri is the Available Agents list on the home page, each
card showing a name and a line about what the agent covers - that line being the
Description from step 3. The user does not have to pick the right agent: the
selector defaults to Sindri, whose agent holds every agent's capabilities and
reads those descriptions to decide which guidance applies. See
[`sindri.md`](../wiki/sindri.md).

So an agent is found in the hub through the Description written in step 3, not
through a publish action.

**5. Only if someone outside has to reach it.** This is a different path, and
it is not console work: a Flow Agent as the entry point, and a
`BotProvider` for the channel. An anonymous visitor cannot authenticate to the
hub at all. [`integration.md`](../wiki/integration.md) has the four routes.

## Console or chart - the same five steps either way

The steps above are the console. An engagement builds them as a chart instead,
and the order is the same - each step still depends on the one before it. What is
not the same is the count: a console object is often several CRs.

| step | console | where the CRs are stated |
|---|---|---|
| 1 | Data Source | `DataConnector` - [`settings.md`](../wiki/settings.md) |
| 2 | Semantic Model | `SemanticLayer` - [`semantic-model.md`](../wiki/semantic-model.md) |
| 2 | MCP Server, query tool | a `Workflow` - [`tools.md`](../wiki/tools.md) |
| 2 | Drive | `SourceSet` (+ `Syncer`) - [`knowledge.md`](../wiki/knowledge.md) |
| 2 | Skillset | `SkillSet` + `SourceSet` + `Syncer` - [`tools.md`](../wiki/tools.md) |
| 3 | Managed Agent | one `Agent` - [`agents.md`](../wiki/agents.md) |
| 5 | Flow Agent | three CRs, not one - [`agents.md`](../wiki/agents.md) |

A console object is not one CR, and the names do not match. There is no
`FlowAgent` kind, no `KnowledgeDrive` and no `HttpTool` - those are
`asgard-cli add` template names, and the CRs they write are in the column above.
[`agents.md`](../wiki/agents.md) has the full UI-to-CR table and is where that fact
lives; this one is only here so the order can be followed in a chart.

Connection is the exception in the other direction: OAuth authorisation
happens in the UI and is not declared in a chart at all.

So a screenshot of the console is a fair illustration of what a chart does - the
customer's admin sees step 3 as a form whether or not the form is what we filled
in - and the documentation's screenshots can be used in a handover deck.

## Screenshots, for a deck or a handover

[`screenshots.md`](../wiki/screenshots.md) is the index - every picture, what it shows,
and which situation it is for. It carries the setup path as its own section, so
this is the short version. Fetch them from:

    https://docs.asgard-ai.com/img/docs/<path>

The console itself is in English; only the documentation's captions are
zh-TW, so a deck in either language can use them.

The three that carry this page:

| step | path |
|---|---|
| 1 | `settings/data-source/data-source-create-provider-open.png` - nine providers, all databases |
| 3 | `agent-hub-managed-agent/create.png` - the form an admin fills in |
| 4 | `sindri-home/home-available-agents.png` - what their users see |

Check the file before using it. These were captured against the product at
some point and nothing here tracks when. Do not put a screenshot of an older
form in a customer deck; they will compare it to what they see.

The last of the three shows Descriptions written as routing text - every card
on it reads 「當使用者要⋯時,委派給 X Agent」. Do not copy that form; write
「涵蓋⋯:⋯」, as step 3 says.

## Corresponding extracts

No single extract covers this path, because it is the order rather than a shape.
The shapes it passes through are `../usecase/agent-hub.md`,
`../usecase/external-api.md`, `../usecase/semantic-layer.md`, `../usecase/fixed-query-tools.md`, `../usecase/knowledge-drive.md` and
`../usecase/skill-set.md`.

## Sources

- [Managed Agent](https://docs.asgard-ai.com/docs/product-suite/odin/features/agent-hub-managed-agent),
  [MCP Servers](https://docs.asgard-ai.com/docs/product-suite/odin/features/mcp-servers),
  [Skillsets](https://docs.asgard-ai.com/docs/product-suite/odin/features/skillsets),
  [Configuration](https://docs.asgard-ai.com/docs/product-suite/odin/features/agent-hub-configuration),
  [Data Source](https://docs.asgard-ai.com/docs/product-suite/odin/features/settings/data-source),
  [Connection](https://docs.asgard-ai.com/docs/product-suite/odin/features/settings/connection)
  - asgard-docs `6261fdff`
- Screenshot paths read off that same checkout's `static/img/docs/`
- The sequence itself is assembled and only its first fork is verified. Each
  step comes from the page that owns it; no source puts them in an order. The one part held against a real console screen is
  that an HTTP API's credential has no home under Settings - the fork that sends
  you to a Workflow instead. The rest is a reading of what each step needs from
  the one before, and the charts corroborate the dependency direction without
  saying anything about the console flow. If you work through it against a live
  console and it is wrong anywhere, tell the maintainer
- Checked against asgard-kube `3da0365`: the CR column names only
  kinds the CRDs define. `HttpTool`, `KnowledgeDrive` and `FlowAgent` are not
  among them
- Checked against three of those images, opened rather than
  listed: the Data Source Provider list (every entry a database, no
  HTTP option), the Managed Agent form (the four Prompt fields and the live
  preview), and the Sindri home page (the Available Agents, each card showing
  its Description)

**Checked:** asgard-docs `21c920f6` - `docs/product-suite/odin/features/settings/data-source.mdx`
(nine providers, Test Connection before Save), `settings/connection.mdx` (the five
OAuth services), `mcp-servers.mdx` (From Workflow and From Existing, STDIO or
Streamable HTTP), `agent-hub-managed-agent.mdx` (the five built-in templates), and
`docs/quickstarts-guide.mdx` plus the two Odin case studies, none of which states a
build order; asgard-core `478cf5d6`
asgard-core `internal/bpcontroller/server/sandbox_orchestration.go`, and
asgard-core #368 asgard-core `internal/processor/helper/clidriver_run.go`
(`AgentCapabilitiesNote`), for what the turn's agent is given about each agent; asgard-kube `3da0365` `pkg/apis/asgard/v1alpha1/types.go` for the
dependency direction - `SemanticLayer.spec.dataConnectorName`, and
`Agent.spec.managed` naming its SkillSets, Toolsets, SourceSet mounts and
semantic layers.

**Unchecked:** the order as a whole has not been walked through against a live console.
