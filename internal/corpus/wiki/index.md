---
description: every wiki page grouped by the question it answers - start here when you do not know which page has your answer
---
# Index

    ../wiki/<page>.md

## Products and scope

| page | covers |
|---|---|
| [`product-suite`](../wiki/product-suite.md) | what each product is for, and who uses it |
| [`console`](../wiki/console.md) | the permission layers, the pages that disagree with each other, Workspace Management - and the path each thing sits at, for when the answer is where to click |
| [`sindri`](../wiki/sindri.md) | Project, delegation, the Sandbox and its files, the governance gate |
| [`mimir`](../wiki/mimir.md) | Thread, View, Dashboard, Knowledge |
| [`fehu`](../wiki/fehu.md) | billing and usage, how cost is broken down - and which product lets a customer bring their own model |

## While building

| page | covers |
|---|---|
| [`setup-path`](../wiki/setup-path.md) | the order: where a credential goes, what to build from it, why Sindri needs no import |
| [`agents`](../wiki/agents.md) | Flow Agent against Managed Agent, how to choose, UI-to-CR names |
| [`knowledge`](../wiki/knowledge.md) | Drive, Context Index, and how Knowledge Base differs |
| [`semantic-model`](../wiki/semantic-model.md) | the modelling flow, its limits, the Mimir side |
| [`tools`](../wiki/tools.md) | MCP Server, Skillset and Plugin; hook events |
| [`automation`](../wiki/automation.md) | Trigger and API, and why only cron is left |
| [`processors`](../wiki/processors.md) | what each processor type takes, and the fields that decide behaviour |
| [`workflow`](../wiki/workflow.md) | the processor types against the editor's groups; Expression is JavaScript, Template is Handlebars |
| [`settings`](../wiki/settings.md) | Completion and Embedding Model, Data Source, Connection - and which of them a chart writes |
| [`integration`](../wiki/integration.md) | chat platforms, the Applications pages, the architecture - and that there is no mail capability at all |
| [`api`](../wiki/api.md) | the endpoint and its actions, the SSE sequence, the integration patterns, the SDK |
| [`crd-rules`](../wiki/crd-rules.md) | the validations helm lint does not run, and the one the schema cannot express |
| [`platform-unknowns`](../wiki/platform-unknowns.md) | what no source answers, and who to ask |
| [`coverage`](../wiki/coverage.md) | how many deployments each CR shape was read from - which extracts rest on a sample of one |
| [`tool-description-and-skill`](../wiki/tool-description-and-skill.md) | a tool's description and a runtime skill reach the model in one context - which of the two owns a fact when both could hold it |

## In practice

| page | covers |
|---|---|
| [`green-and-doing-nothing`](../wiki/green-and-doing-nothing.md) | a chart that deployed and passed every check but does nothing: the shapes that have happened, why no layer of the toolchain catches them, and which have a rule. Start here when the symptom is silence rather than an error |
| [`operations`](../wiki/operations.md) | Asgard's outbound IPs, checking model capability, the symptoms that turn out to be an account or a setting, vocabulary |
| [`taiwan-channels`](../wiki/taiwan-channels.md) | the commerce channels a customer will name, what SHOPLINE cost, and what we have not built |
| [`glossary`](../wiki/glossary.md) | words that mean one thing here, and what the other senses are called. Check it for the word you searched, because a result in the wrong sense looks like an answer |
| [`what-they-read`](../wiki/what-they-read.md) | the picture a customer arrives with, and where it is wrong |
| [`case-studies`](../wiki/case-studies.md) | the retail stockout from several angles, plus a Flow Agent help desk |
| [`screenshots`](../wiki/screenshots.md) | which picture answers which question, and the URL to fetch it from |

## Every UI name, and the CR it is

This table maps every UI name to the CR a chart writes for it. Before it
existed, only the agent pages had such a table; every other mapping was in the
prose of whichever page discusses the feature, where a grep for the UI name
finds it but nothing can check that every UI name has one.

Read it as "the customer said X, so the chart writes Y". The page column is
where the judgement is; this table is only the name.

| the UI calls it | the chart writes | and the page is |
|---|---|---|
| Agent Hub > Flow Agent | `Workflow` + `SandboxBlueprint` + `BotProvider`, and no `Agent` | `../usecase/flow-agent-single.md` |
| Agent Hub > Managed Agent | `Agent` | `../wiki/agents.md` |
| Agent Hub > Configuration > Models | no CR of its own - it selects `CompletionModel`s that already exist | `../wiki/settings.md` |
| Agent Hub > Configuration > Global Directory | not a chart's to write - it is edited on that page and shared with every Agent Hub conversation in the platform Project | `../wiki/sindri.md` |
| Applications (Data Insight & Agent Hub) | no CR - a listing of what is already published | `../wiki/integration.md` |
| the Release panel of a Flow Agent's workflow set, listed afterwards under Applications > Customized Integration | `BotProvider` | `../usecase/chat-channel.md` |
| Automation > API | `Workflow`, with the `automation_tool` workflow-set type | `../wiki/automation.md` |
| Automation > Trigger | `Trigger`, plus the `Workflow` it enters | `../usecase/trigger.md` |
| Data Insight > Semantic Model | `SemanticLayer` | `../usecase/semantic-layer.md` |
| Drive | `SourceSet`, one `Syncer` per source | `../usecase/knowledge-drive.md` |
| Drive > Context Index | no CR of its own - `SourceSet.spec.contextIndex`, from which the reconciler derives a `Workflow`, a `SandboxBlueprint` and a `Trigger` | `../wiki/knowledge.md` |
| Knowledge Base | `KnowledgeBase`; a `Loader` per Auto Load source, a `Source` per item, and an `Indexer` the Source owns per indexer key | `../usecase/knowledge-base.md` |
| MCP Servers | `Toolset` | `../wiki/tools.md` |
| Plugins | `Plugin` | `../usecase/plugin.md` |
| Skillsets | `SkillSet` + its own `SourceSet` + the `Syncer` that fills it | `../usecase/skill-set.md` |
| Settings > Completion Model | `CompletionModel` | `../wiki/settings.md` |
| Settings > Embedding Model | `EmbeddingModel` | `../wiki/settings.md` |
| Settings > Data Source | `DataConnector` | `../wiki/settings.md` |
| Settings > Connection | `OAuthProvider` + `OAuthCredential`. Not Data Source: this is third-party OAuth, where Data Source is a credential you type | `../wiki/settings.md` |

Four kinds are in the contract and no chart creates them. `Sandbox` is the
runtime object a blueprint produces, so a chart never writes one; and
`ImageGenerationModel`, `TranscriptionModel` and `SourceSetEditorServer` are
internal, per the platform team. They are absent from the
documentation on purpose, so a customer asking for image generation or
transcription needs the platform team, not a chart.

**Checked:** the UI's own vocabulary from asgard-docs `f00e0ee` -
every page under `product-suite/odin/features/` - held against the 24
kinds in asgard-kube `cbd8d70` `crd/`. Each row's CR is the one that page says
gets created, or the one the extract in the third column writes.

**Unchecked:** Mimir's and Sindri's own pages are not in it. Their features
(Thread, View, Dashboard, My Chat, Directory) are reached rather than authored,
so a chart writes nothing for them - but nobody has confirmed that a Mimir View
leaves no CR behind.

## Known gaps between the documentation and the CRD

Each is written on the page it affects:

| gap | page |
|---|---|
| the UI's Flow Agent is three CRs; `agentClass` has one value | `../wiki/agents.md` |
| the Drive Syncer UI offers five sources, the CRD supports ten | `../wiki/knowledge.md` |
| Knowledge Base and Drive both exist and which one new work should use | `../wiki/knowledge.md` |
| Connection's "For Trigger" group names removed trigger classes | `../wiki/automation.md`, `../wiki/settings.md` |
| Semantic Model lists six data sources, the settings page nine (unexplained) | `../wiki/semantic-model.md` |
| the four `integration-with-asgard/` pages are `draft` and name an older UI | `../wiki/integration.md` |
| the glossary's Processor list does not match the current `ProcessorType` | `../wiki/operations.md` |
| `overview/asgard-features` is `draft` and links to removed paths | `../wiki/product-suite.md` |

## Coverage

A coverage number names its denominator in the same sentence. A percentage
measured against one source out of nine looks like a statement about all the
material, and so does an in-scope denominator restated as a count of
citations. A fraction whose numerator counts links and whose denominator counts
pages overstates coverage.

The index carries no hand-derived figures. Every number in the row below is
computed from the asgard-docs clone by the tool that maintains this material,
which fails when the row drifts, so the row is the only place a number belongs.

A page's live URL is not always its path under `docs/`, so the count is
resolved through `slug:` frontmatter: channel pages are served from capitalised
files, a directory's `index.mdx` answers without the `index`, and fourteen
pages declare a slug that differs from where they sit. Matching paths literally
undercounts. A computed number can still be wrong; computing it makes it
possible to re-derive and correct.

The sources this material is actually built from:

| source | what it holds | state |
|---|---|---|
| asgard-docs | the product documentation | 77 / 162 cited at `f00e0ee`; 85 published and uncited; 28 deliberately excluded below. Computed, not counted |
| asgard-kube `crd/` | the contract | read per page, per field, and dated on the page |
| asgard-kube `pkg/apis/` | the Go types the CRDs are generated from, with the reasoning as comments | read for the validation rules - `../wiki/crd-rules.md`. 134KB of declarations; what has been taken is the behavioural comments, not the field list |
| [asgard-core](https://github.com/asgard-ai-platform/asgard-core) `internal/constants.go` | the processor definitions the CRD is generated from | walked at `623ceb5`. Every processor's required keys, defaults and declared outputs are in `../wiki/processors.md`, and that table is held against the literal mechanically |
| asgard-freyr-skills | runtime skills, incl. the SHOPLINE pair | `../usecase/skill-layers.md`, `../usecase/browser-operation.md` and `../wiki/taiwan-channels.md`. The eighth reference repository, and the only one with no CRs |
| the reference deployments | every shape the extracts describe | `../wiki/coverage.md` counts the charts per deployment and says why |

Deployment coverage cannot be measured from this material. An extract names no
customer and no deployment - it says "seen in a deployment whose..." - so
nothing here can be counted against the charts it came from. That inventory is
run separately, over the charts, and its result is
[`coverage`](../wiki/coverage.md) - how many deployments each CR shape was actually read
from.

## Deliberately not covered

`asgard-builtin/` is covered except its message-template pages. Several of its
pages are the expression language every processor field is written in,
including what `execute-script`'s `ECMA5` engine value means - a name, not a
language limit - and the variables in scope, several of which the
documentation never mentions. Those are in
[`processors`](../wiki/processors.md).

Each exclusion is a judgement made once. Recheck one before relying on it,
particularly one that excludes a whole directory, since nobody may have looked
inside it.

An exclusion can also stop applying: `superpowers/` is gone from asgard-docs,
so at the clone's HEAD the excluded set is smaller than the row above says.
Both numbers are still correct for the commit they were taken at.

Uncited pages are not necessarily unread. Two families are pointed at by URL
pattern rather than by link: the per-processor reference pages
(`../wiki/processors.md` gives the pattern and one example) and the SSE event
pages (`../wiki/api.md` says "one page per event" and links one). This is
deliberate, because a link per page whose content is a field table would be
that many links to keep resolving. So the figure measures how much is linked,
not how much has been read; keep that in mind before treating the uncited
count as a backlog.

| excluded | why |
|---|---|
| `developer-reference/asgard-builtin/message-template-*` | Message template shapes - button, carousel, image, video, location. Genuinely lookup material, and per-channel. Read the source when writing one |
| `help-community/release-notes/` | historical, and does not describe the present |
| `superpowers/` | the documentation site's own redesign plans, not an Asgard feature. Gone from asgard-docs since |

## Keeping this index complete

Every page in this directory has a row above.

A page's provenance is on the page. Each one ends with a source block naming
the rendered documentation page and the commit it was read at, plus an
`**Unchecked:**` line. There is no separate log of what was read when.

Adding a page means adding a row, under the question it answers rather than at
the end. A page with no row still appears in the directory listing, so nothing
breaks, but a reader who does not already know its name cannot find which page
answers their question.

## Outside this wiki

| what you want | where |
|---|---|
| how a deployment shape is assembled, field by field | `../usecase/` |
| whether a CR field is legal or required | [asgard-kube's `crd/*.yaml`](https://github.com/asgard-ai-platform/asgard-kube/tree/main/crd) |
| what this customer's systems look like | that customer repo's `docs/spec/` |
| the original product documentation | [asgard-docs](https://github.com/asgard-ai-platform/asgard-docs), rendered at https://docs.asgard-ai.com |
