# SOURCES.md

Where the material in `internal/corpus/usecase/` came from.

> Internal only. This file lives outside `internal/`, so it cannot be embedded.
> `internal/corpus/usecase/` ships to every engagement, so those files name no customer
> and no deployment. This one exists so we can trace an extract back to the chart
> it was taken from.

Keep it that way: when adding an extract, put the customer-facing shape in
`internal/corpus/usecase/` and the attribution here.

Record the commit you read, so that it is possible to say which version of a
chart an extract describes.

## What each clone held when its extracts were written

Charts move, and an extract describes one version of one chart. Written from is
that version.

| deployment | written from |
|---|---|
| unitech-e-asgard-kube | `44e71a2` |
| xxentria-asgard-kube | `967407c` |
| finance-ai-asgard-kube | `d062197` |
| buy123-asgard-kube | `4dab85d` |
| asgard-freyr-kube | `8f6d6c1` |
| asgard-auto-post-kube | `62ccbe0` |
| asgard-industry-demo-generator | `1106771` |
| asgard-freyr-skills | `2ff0e1e` |

How far each has moved since is computed by `go run ./hack sources --extracts`,
which asks whether an extract still describes that chart. Do not add a column
of distances: the distance between two moving commits goes stale within days.

Two of these are each the only source for part of an extract.
`asgard-freyr-skills` is the only source for
`internal/corpus/usecase/browser-operation.md`, and `asgard-freyr-kube` is
where `agents.expression` and the sandbox hooks were read.
`internal/corpus/wiki/coverage.md` reports extracts that rest on a single
deployment.

These rows are not in `asgard-cli audit-material --sources`. That check reads
what is embedded in the binary, and this file is not embedded because it names
customers. `go run ./hack sources --extracts` and `go run ./hack counts` hold
these rows against the clones here instead.

## The deployments read so far

Both columns below are counted at the written-from commit in the table above.
`internal/corpus/usecase/plugin.md` counts its Plugins at `edb0ad0` and says
so; `go run ./hack counts` holds both counts against the clone.

| deployment | shape it demonstrates | CR files | referred to in extracts as |
|---|---|---|---|
| unitech-e | agent hub (5 agents / 6 semantic layers) and a single-agent flow agent; trigger; knowledge drive. Archived 2026-09: the project moved to the customer's own repository (github.com/UnitechE/unitech-e-asgard-kube) and its namespaces were destroyed, so this clone is a record of the pilot and nothing newer | 41 | "a later one", "a deployment with an internal hub and a public widget" |
| freyr | supervisor + 5 subagents, `agents.expression`, sandbox hooks, shared SourceSet; a DataConnector and two SemanticLayers bound with `allowedCubes`; two tenant charts, the second an internal demo copy of the first | 52 | "a commerce back-office with five specialists", "an earlier deployment" |
| xxentria | supervisor + 9 agents | 17 | "a manufacturing one with nine" |
| finance-ai | supervisor, 3 semantic layers | 12 | "a finance one with three" |
| buy123 | the minimal flow agent - no Agent CR at all. Moved 2026-09-13 to github.com/xxtechec/infra-buy123-asgard-kube and deployed by the platform pipeline; this clone is the pre-move history | 8 | not yet cited |
| auto-post | 29 Plugin CRs, knowledge bases, api workflows | 64 | not yet cited |
| industry-demo-generator | 12 industries under `projects/`, one Platform Pipeline release each, read/write governance split, a Claude Code plugin of commands + skills | many | "a 12-industry demo chart set", "one agent per business role" |

### Which customer each one is

This is the only file that makes this link. Nothing under `internal/` may name a
customer.

| repo | who |
|---|---|
| [unitech-e-asgard-kube](https://github.com/asgard-ai-platform/unitech-e-asgard-kube) | 台新 |
| [xxentria-asgard-kube](https://github.com/asgard-ai-platform/xxentria-asgard-kube) | 森鉅 |
| [finance-ai-asgard-kube](https://github.com/asgard-ai-platform/finance-ai-asgard-kube) | FinanceAI |
| [buy123-asgard-kube](https://github.com/asgard-ai-platform/buy123-asgard-kube) | Buy123 |
| [asgard-freyr-kube](https://github.com/asgard-ai-platform/asgard-freyr-kube) | Freyr |
| [asgard-auto-post-kube](https://github.com/asgard-ai-platform/asgard-auto-post-kube) | Heimdall |
| [asgard-industry-demo-generator](https://github.com/asgard-ai-platform/asgard-industry-demo-generator) | Demo Generator |
| [asgard-freyr-skills](https://github.com/asgard-ai-platform/asgard-freyr-skills) | Freyr (runtime skills, a separate repo from the chart) |

The demo generator and auto-post are Asgard's own rather than a customer engagement. `Heimdall` is
also the name of a product in the suite (Media & PR AI, see `internal/corpus/wiki/
product-suite`) - the repo and the product are not the same thing, and an extract
saying "Heimdall" without saying which is ambiguous.

The platform contract itself is
[asgard-kube](https://github.com/asgard-ai-platform/asgard-kube), and the product
documentation is
[asgard-docs](https://github.com/asgard-ai-platform/asgard-docs). Neither is a
deployment; both are listed in `AGENTS.md` alongside these.

## Generational conflicts found so far

Each row is a place where two charts disagree, with the date that decides which
one is current.

| topic | earlier | later | which wins |
|---|---|---|---|
| SkillSet to SourceSet | one shared store, several skill sets slicing it with searchPaths (freyr) | 1:1:1, one file per skill set (unitech-e, changed 2026-08-28) | later. The earlier shape leaves the UI unable to find a skill set's git config |
| `bot-provider-type` label | stamped, "front end breaks without it" (freyr `39a2b8c`, removed in `7f6e564`) | dropped, platform derives it from `botProviderClass` (unitech-e, workflow-service #336) | later, but stamping it anyway is harmless |
| canvas metadata | hand-written `node_positions` ConfigMaps (demo generator at `718cc0e`, removed upstream in `927a386`) | dropped, platform auto-lays-out (unitech-e, #336-#340, 2026-08-31) | later |
| `Agent.managed.completionModelName` | required (demo generator's GOAL.md before `0202bc2`) | Agent takes no model; the caller picks per turn (unitech-e) | later |
| `KnowledgeBase` | in use (auto-post) | one deployment replaced it with a SourceSet Drive + contextIndex (TASK-013). Live and unmarked in the CRDs; do not restate as a platform deprecation | later |

## Where the platform's own documentation disagrees with every chart

This is a documentation error rather than a generational conflict, and an agent
would act on it.

| topic | the CRD documentation says | every chart does | evidence |
|---|---|---|---|
| `config.expression` | "CEL 表達式" | JavaScript: arrow functions (26 occurrences), `const` (11), `String()` (7), `encodeURIComponent` (5), `??` (4), `JSON.stringify` (3) | CEL has none of those constructs. Counted across every chart in the deployments listed above |

`internal/corpus/usecase/workflow-chain.md` states the corrected version and
says the docs are wrong, because an agent told it is CEL writes an expression
that cannot work and has no way to find out why.

unitech-e is the most recently maintained, so it wins a conflict unless there
is a reason to think otherwise. Record the reason when there is.

## Which file each kind's material came from

Attribution for `internal/corpus/usecase/`, kept here because this is the file
that is allowed to name a deployment. Sizes are a rough guide to how much of the knowledge is in the header
comments rather than the YAML.

| kind | source |
|---|---|
| `DataConnector` | `unitech-e/.../data_connector/dc-bpm.yaml` (585B) |
| `Toolset` | `unitech-e/.../toolset/ts-catalog.yaml` (3.2KB, exemplary comments) |
| `SandboxBlueprint` | `unitech-e/.../agent/sbp-website.yaml` (2.3KB) |
| `Agent` | `unitech-e/.../agent/ag-bpm.yaml` (5.5KB) |
| `Workflow` | `unitech-e/.../workflow/wf-website.yaml` (12KB) |
| `SkillSet` trio | `unitech-e/.../skill_set/sk-base.yaml` (3.9KB) |
| `SourceSet` + `Syncer` | `unitech-e/.../source_set/ss-website-knowledge.yaml` (4.6KB) |
| `Trigger` | `unitech-e/.../trigger/tr-pr-arrival-notify.yaml` (2.6KB) |
| `BotProvider` | `unitech-e/.../agent/bp-website.yaml` (4.4KB) |
| `SemanticLayer` | too large to ship whole (23KB-253KB); take one cube plus the `sampleQueries` shape |
| `CompletionModel`, write-path `Workflow` with `requestConsent` | `industry-demo-generator/projects/retail/chart/app/templates/completion_model.yaml`; the write path is `.../toolset/wms/toolset.yaml` (`requestConsent: true`) and its tools |

`SemanticLayer` has no single source small enough to ship: take one cube plus the
`sampleQueries` shape. `CompletionModel` and the write-path `Workflow` with
`requestConsent` come from the demo generator rather than a kube repo.

Where the two disagree, prefer `unitech-e`, which is the more recently
maintained. The generational conflicts table above records the dated
disagreements.
