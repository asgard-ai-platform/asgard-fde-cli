---
group: While building
description: Completion and Embedding Model, Data Source, Connection - and which of them a chart writes
---
# Models, data sources and connections

Four things under Odin's Settings. All of them are prerequisites for something
else.

| setting | used by | CR |
|---|---|---|
| Completion Model | agents, workflows, Semantic Models | `CompletionModel` |
| Embedding Model | semantic search and retrieval | `EmbeddingModel` |
| Data Source | Semantic Models and agent queries | `DataConnector` |
| Connection | Syncers and Loaders needing third-party authorisation | `OAuthProvider` + `OAuthCredential` |

## Completion Model

Four built-in tiers, marked Built-in and neither deletable nor editable:

| built-in | for |
|---|---|
| Builtin (Vision) | tasks with image input |
| Builtin (Balanced) | general use |
| Builtin (Complex) | harder reasoning |
| Builtin (Fast) | when latency matters most |

A custom model takes Name, Model Provider, Model Name and API Key.

The built-in tiers are semantic aliases rather than specific model names, which
is usually the right default: customers rarely have a view, and a hardcoded model
name becomes something to come back and fix when the model is retired.

A builtin tier is a logical model, and a router resolves it. None of the
following is visible from the platform side:

  - one logical name is backed by several provider-model pairs, and the
    selection policy is weighted-random, weighted round-robin or ordered
    fallback
  - automatic failover: a 5xx or a timeout from one provider retries the
    next candidate rather than failing the run
  - the managed key lives in the router's own environment

So a builtin tier is a pool of models with failover. A custom `CompletionModel`
gives that up: one provider, one key,
one point of failure, and an outage at that provider is an outage for the
customer.

A custom model is also only available on Odin. Sindri and Mimir use the platform's
designated models and the LLM cannot be swapped there - see
[`fehu.md`](../wiki/fehu.md). So a custom `CompletionModel` does not make a hub agent or
a dashboard use the customer's key, and "we will use our own model" has a
different answer per product.

State this trade when a customer asks for a specific model. A compliance
requirement, an existing contract or a model they have tested against are good
reasons to still want one. A plain preference usually is not.

In a chart, a custom model is a `CompletionModel` CR. The built-in tiers are
that CR's `builtin` class, not the absence of one. A chart using a built-in
tier declares no CR at all:
the namespace already carries them under the names `preset-balanced`,
`preset-complex`, `preset-fast` and `preset-vision`, and every field that takes a
model takes one of those as a plain string. Three reference deployments declare
their own, with the provider's key as a secretKeyRef into the release's own
Secret - whose name the Platform injects, and which a chart never writes out:

`asgard-cli add` has no `completionmodel` kind, by decision. The common case
writes no CR, so a generator for this would produce one whenever somebody
reached for it. The uncommon case is a contract with a provider: which model id,
whose key, and who pays for the tokens. A skeleton cannot guess those, and a
wrong provider block is a CR the apiserver accepts and every turn then fails on.
Copy the shape below once that decision is made.

| | |
|---|---|
| `completionModelClass` | `aoai-chat`, `openai-chat`, `gemini`, `anthropic`, `mistral`, `builtin` |
| provider block | exactly one of `aoaiChat`, `openaiChat`, `gemini`, `anthropic`, `mistral`, `builtin` |
| the model | `spec.<provider>.model`, the provider's own model id |
| the key | `spec.<provider>.apiKey.valueFrom.secretKeyRef` |

`completionModelClass` is immutable, so moving a customer from one provider
to another is a new CR rather than an edit - the same trap as
`BotProvider.botProviderClass`. The exactly-one rule is a CRD validation, so a CR
carrying two provider blocks is refused by the apiserver and passes `helm lint`.

Name the CR after the chart value everything else already reads, rather than
writing a name of its own. A chart refers to its model as a string, so the
deployment that does this cleanly sets the CR's `metadata.name` from the same
value its workflow configs take - one value, not two. The name is not a
reference the apiserver resolves: asgard-core `478cf5d6` builds it into the model
router's URL, `.../ns/<namespace>/completion-model/<name>/router`, so a name
that matches nothing is a 404 on the first turn, and no chart check sees it.

A declared model that nothing names costs a key and changes nothing. In the
demo generator's charts every `CompletionModel` is unreferenced - every
SemanticLayer names `preset-balanced` through one chart value, and Agents take
no model - and in the auto-post chart all but one are. Copying that block into an engagement's chart obtains a provider key,
declares an `appSecret`, and changes which model answers nothing at all. Before
writing the CR, find the field that will name it.

On a model that is not a reasoning model, the effort setting causes failures
rather than being ignored. asgard-core `478cf5d6` records the platform's own
`preset-fast` rejecting the reasoning-effort parameter outright - every turn on
it failed, including turns that sent none, because the agent supplies its own
default level - and the deployment bringing its own non-reasoning model pins its
chart's effort value to `disabled` for the same reason. So the model and the
effort value are one decision; `../usecase/semantic-layer.md` has the field.

## Embedding Model

Only one built-in, Builtin (Balanced).

The custom form's fields change with the provider, and Azure OpenAI needs the
most from the customer. The default, Azure OpenAI Embedding Model, takes six
required fields - Name, Model Provider, Resource Name, Deployment ID, API
Version and API Key - where plain OpenAI takes two. In a chart that is
`spec.aoai` with all four of `resourceName`, `deploymentId`, `apiVersion` and
`apiKey` required, against `spec.openai` needing `apiKey` and `model`.

So for a customer on Azure, three of the four are things only their Azure
administrator has: `deploymentId` is what they named the deployment and is not
the model name, `resourceName` is the resource rather than the endpoint, and
`apiVersion` is a dated version string that has to be given rather than guessed.
Ask for all three together - a request that comes back one field at a time costs
a round trip each.

`ImageGenerationModel` and `TranscriptionModel` carry the same `aoai` block with
the same four required fields. Both are internal and neither is a route an
engagement takes, per the platform team,
so a customer wanting image generation or transcription is a conversation with
the platform team rather than a CR to write. The field shape is recorded in case
that changes; do not write these CRs.

## Data Source

Nine providers, matching `DataConnectorClass` exactly:

MySQL, SAP Hana, SalesForce, Oracle, Microsoft SQL Server, PostgreSQL, NetSuite,
Trino, Athena.

The form takes Name, Provider, Host, Port, Database, User and Password. Test
Connection can be run before Save. The form marks nothing as required.

In a chart, the non-secret coordinates are declared as `chartValues` and the password as `appSecret`,
both set on the platform per release. The password is only ever a secretKeyRef, and the Secret's name
comes from `.Values.asgard.appSecretName` - a literal name in a template points at nothing.

An HTTP API does not go here. Data Source is these nine database providers
and nothing else, and Connection below is OAuth to five named services. A REST
API with a key or a bearer token is configured on the tool that calls it - an
`http-request` step in a Workflow, or an MCP Server's environment variables.
[`setup-path.md`](../wiki/setup-path.md) has the fork; `../usecase/external-api.md`
has the shape.

## Connection

Manages authorisation to third-party apps and services over OAuth, as distinct
from a Data Source's host-and-password. The form has only Name and Type, and the
button says Authorize.

The Type list is grouped by purpose:

| group | options |
|---|---|
| syncer | Dropbox, Google Drive, OneDrive |
| For Loader | Google Drive, OneDrive |
| For Trigger | Google Drive, Google Sheets, OneDrive, OneDrive Workbook (Excel) |

The same service appears in more than one group, so the services that can
actually be authorised are Dropbox, Google Drive, OneDrive, Google Sheets and
OneDrive Workbook - five, not eleven.

The "For Trigger" group is stale: the trigger classes it corresponds to were
removed and only cron is left. See [`automation.md`](../wiki/automation.md).

## Before writing the chart

`../usecase/semantic-layer.md` has the DataConnector fields - coordinates in
a platform variable, password always a secretKeyRef. Connection has no extract of
its own, because OAuth authorisation happens in the UI rather than being declared
in a chart.

## Sources

- [Completion Model](https://docs.asgard-ai.com/docs/product-suite/odin/features/settings/completion-model),
  [Embedding Model](https://docs.asgard-ai.com/docs/product-suite/odin/features/settings/embedding-model),
  [Data Source](https://docs.asgard-ai.com/docs/product-suite/odin/features/settings/data-source),
  [Connection](https://docs.asgard-ai.com/docs/product-suite/odin/features/settings/connection)
  - asgard-docs `6261fdff`
- The CR mapping and the provider list: checked against
  [asgard-kube](https://github.com/asgard-ai-platform/asgard-kube) `3da0365` -
  `DataConnectorClass`, `CompletionModelClass`, `EmbeddingModelClass`

- The router's behaviour behind a builtin alias - logical models, the three
  selection policies, failover on 5xx or timeout, and the managed key in its own
  environment: `asgard-router`'s README

**Checked:** against asgard-kube `3da0365` `pkg/apis/asgard/v1alpha1/types.go`
(the `completionModelClass`, `embeddingModelClass` and `dataConnectorClass`
enums, the immutability rules, the ExactlyOneOf validations, the required
`aoai` embedding fields and the builtin alias enums) and against three
deployments that declare their own model; against asgard-core `478cf5d6`
asgard-core `internal/constants.go` for the `preset-*` names and the
`preset-fast` effort guard, and asgard-core `internal/processor/component/model_router_client.go` for
the model router's URL shape; against `asgard-router`'s README;
against every `CompletionModel` CR in those three deployments held against every
reference to a model name in the same charts.

**Unchecked:** the UI form fields and the Connection Type list come from the
product documentation, and nobody here has opened the forms in a Console account.
