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
| Connection | Syncers, sandboxes and Workflows that act on a third-party account | `OAuthProvider` + `OAuthCredential` |

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

The form takes Name and Provider; the fields after those - Host, Port,
Database, User, Password and so on - come from the provider's schema, which the
Platform API serves, so they differ by provider. Test Connection can be run
before Save, and stays disabled until the fields that schema requires are
filled. A provider whose schema carries the SSH tunnel also gets a tunnel
section - bastion host, port, user, a password or a private key with its
passphrase, and the host key - which is `spec.sshTunnel` on the
`DataConnector`; athena, salesforce and netsuite never show it.

In a chart, the non-secret coordinates are declared as `chartValues` and the password as `appSecret`,
both set on the platform per release. The password is only ever a secretKeyRef, and the Secret's name
comes from `.Values.asgard.appSecretName` - a literal name in a template points at nothing.

An HTTP API does not go here. Data Source is these nine database providers
and nothing else, and Connection below is OAuth with a client the chart declares. A REST
API with a key or a bearer token is configured on the tool that calls it - an
`http-request` step in a Workflow, or an MCP Server's environment variables.
[`setup-path.md`](../wiki/setup-path.md) has the fork; `../usecase/external-api.md`
has the shape.

## Connection

Authorisation to a third-party account over OAuth, as distinct from a Data
Source's host-and-password. Two CRs: an `OAuthProvider` is the client - the
vendor's endpoints, the scopes, and the client ID and secret - and an
`OAuthCredential` is one grant of it, whose token a Syncer, a sandbox's
`credentialMounts` or a Workflow reads.

The platform puts no provider in a namespace. It used to create nine
`preset-*` ones, backed by Asgard's own clients, and now deletes them, so a
provider exists only where a chart declares one, with the customer's own
client. `spec.clientId` and `spec.clientSecret` are values of the same shape as
a DataConnector's password: `valueFrom.secretKeyRef` to a key of this
release's Secret, declared under `appSecret`. The key names are the chart's to
choose, so one release can hold the clients of several providers.
`spec.secretRef` is deprecated and read by nothing; a provider that has only
that has no client.

An `OAuthCredential` names its provider and how to refresh, and nothing else.
Its tokens - `access_token`, `refresh_token` and `expires_at` - are in a Secret
with the credential's own name, which the platform creates and owns, and
`spec.secretRef` is deprecated and read by nothing. A Workflow reads the token
as a variable, `valueFrom.secretKeyRef` with `name` the credential's name and
`key: access_token`, and the platform keeps that Secret current as it
refreshes. A chart cannot declare a Secret, so a collision is a Secret of that
name that something else made first: the credential then stays Failed, says
so in its status, and never writes to it. Rename the credential.

The Console's Connection page was built around the presets - its Type list
grouped them by name - and is being redesigned as Settings › Connection Points,
so a chart's credential is granted from here rather than there. The labels
`asgard-ai.com/oauth-provider-type` and `asgard-ai.com/correlated-resource-type`
belonged to that page; nothing reads them any more, so a chart does not write
them.

**Members' own accounts.** A provider can also let each member of the project
connect their own account, in Connections where they talk to agents; an agent
then works as the member it is talking to, through `asgard-keyring` in its
sandbox. The chart allows it on the `OAuthProvider`:

- the label `asgard-ai.com/personal-account: eligible`;
- the annotation `asgard-ai.com/display-name` - the name members see. A
  provider without one cannot be opened: the platform refuses, since members
  would see the CR name;
- `asgard-ai.com/description` and `spec.endpoints.userInfoURL` - recommended:
  without them members do not know what connecting is for, or which account
  they connected;
- `asgard-ai.com/example-prompts` (a JSON array of strings) and
  `asgard-ai.com/keyring-env` (the variable `asgard-keyring token --env`
  sets) are optional.

Allowing is not opening. Whether members can connect, in a given project, is a
switch in Asgard Connector that no deploy turns on: after the deploy,
`asgard-cli operate oauth-provider enable <name> --release <release>`, or the
project's admin in Odin. The Agent's prompt names the provider and when to
connect; how to use `asgard-keyring` is in its `--help`, and the platform
tells the agent it is there.

## Before writing the chart

`../usecase/semantic-layer.md` has the DataConnector fields - coordinates in
a platform variable, password always a secretKeyRef. Connection has no extract of
its own. A chart can declare the `OAuthProvider` and the `OAuthCredential` that
a Syncer or a Workflow reads, but not the grant: a token exists only once
somebody signs in to the service and consents. Until then the credential is
PENDING and whatever reads it fails at runtime while every check before it
passes. The grant is a post-deploy step, started with
`asgard-cli operate oauth-credential authorize <name> --release <release>`;
whoever signs in decides whose data it reads, and the platform does not
check which account it was. A provider for members' own accounts has a
post-deploy step too: `asgard-cli operate oauth-provider enable <name>
--release <release>` opens it in the project, and it needs the display-name
annotation first.

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
the model router's URL shape; that an OAuthCredential stays PENDING until a grant writes its Secret, that the Secret has the credential's name and is created and owned by it, that one it does not own is left alone, and that the presets are deleted against asgard-core `6d855e88` `internal/shared/oauthcred`, `internal/bpoperator/reconciler/oac_reconciler.go` and `ns_reconciler.go`; the provider's and the credential's fields against asgard-kube `v1.2.0`; against `asgard-router`'s README;
against every `CompletionModel` CR in those three deployments held against every
reference to a model name in the same charts. The form fields against the
Console's own source, asgard-ai-platform-web `e59474e7`:
`src/constants/trigger.ts`,
`src/utils/data-source-ssh-tunnel.ts` and
`settings/data-source/DataSourceForm.tsx`.

**Unchecked:** the provider-specific fields of the model and data source forms,
which the Platform API serves at run time and no repository here writes down.
What the Console's Connection page shows for a provider a chart declared,
while it is being redesigned.
