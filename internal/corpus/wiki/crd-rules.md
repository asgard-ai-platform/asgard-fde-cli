---
group: While building
description: the validations helm lint does not run, and the one the schema cannot express
---
# Rules the schema enforces, and one it cannot

The CRDs carry validation beyond required-and-type, written as CEL expressions
the apiserver evaluates. `helm lint` does not run them and a server-side
dry-run does not report them faithfully, so a violation usually first shows up
as a failed deploy.

They are read off the Go type definitions in `asgard-kube/pkg/apis/`, which
carry the reasoning as comments. The generated CRD YAML keeps the rule but not
the reason.

## Immutable once set

Changing one of these is a new CR, not an edit. `helm upgrade` produces a
rejection, not a replacement.

| what | on |
|---|---|
| `botProviderClass` | `BotProvider` - see [`integration.md`](../wiki/integration.md) |
| `completionModelClass` | `CompletionModel` - see [`settings.md`](../wiki/settings.md) |
| the indexer | a `Source`'s indexer |
| `leaseId` | once set, on the resource that carries it |

## Toolset: which fields apply is decided by the class

This is not a blanket exactly-one rule, and the asymmetry is deliberate:

    toolsetClass: mcp-server     mcpServerConfig REQUIRED, and tools must
                                 stay empty
    any other class              tools optional and may be empty; no
                                 mcpServerConfig

A workflow-tooling Toolset with zero tools is legal, because that is the
state between "the MCP server exists, with a name and a description" and the
first tool added to it. So an empty `tools` is not a sign of an unfinished
chart, and a check that treats it as one is wrong.

## SemanticLayer: two rules that fail late

A Measure needs `sql` unless its type is `count`. Counting is the one
aggregate the platform can compose itself.

A Join's `from` and `to` must name the same number of dimensions. A join
written against a two-column key with one column listed looks reasonable in
review and is refused by the apiserver.

Check both before a tag, because a layer of any size has enough of
them that one is usually wrong, and neither `asgard-cli check` nor `helm
lint` looks.

## The builtin model holds no key, deliberately

`CompletionModel` of class `builtin` carries only a tier alias - no API key and
no concrete model name. The model router maps the alias to a real model and
supplies its own managed key, so no key lands in a CR or in a per-namespace
Secret.

So prefer a builtin tier when the customer has no view: it is
one less credential in the engagement, and a concrete model name is one more
thing to come back and fix when that model is retired.

## Which of them a render can be held against

There are 76 CEL rules written and 234 enforced. 76 is the number of `XValidation` markers in asgard-kube's Go
types; the generated CRDs carry 234 rule instances, 53 of them distinct,
because one marker on a struct several kinds embed lands in every CRD that
embeds it. Hold a render against the CRDs, not against the markers: the
generated schema is the contract and the Go types are only its source.

28 of the enforced rules are exactly `self == oldSelf` - 27 of the markers -
and they compare a proposed object against the one already on the cluster, so a
render, which is one object with no history, cannot see any of them.

This tool deliberately leaves them to the platform.
The apiserver evaluates them on write, synchronously, and refuses, so a
violation is reported at apply rather than at runtime, and a copy here would
disagree with the platform the first time either changed. What is useful
offline is which fields they are, because that decides a plan before anything
is applied:

### Which fields are chosen once

Most classes are immutable: `agentClass`, `botProviderClass`,
`completionModelClass`, `dataConnectorClass`, `embeddingModelClass`,
`imageGenerationModelClass`, `knowledgeBaseClass`, `loaderClass`,
the Source's `sourceClass`, `syncerClass` and `transcriptionModelClass`.
`botProviderClass` is the one an FDE usually meets first. Changing what kind of
thing such a resource is means a new resource with a new name, and the old
one's references have to move. `toolsetClass`, `triggerClass` and the
Indexer's `sourceClass` carry no such rule and can be edited in place.

The Syncer carries 8 of the 27: `syncerClass`, and the fields that decide
where it writes and what its cursor follows:

    sourceSetName            which store it fills
    destinationPath          the path inside that store
    statePath                where it keeps its cursor
    bot.botProviderName      the bot it reads; its cursor is a createdAt,
                             which a different provider would inherit
    database.columns         the projection it reads
    destinationMemberKey     the deprecated spellings of destinationPath and
    stateMemberKey           statePath, immutable the same way

Where a file-store Syncer reads from is not on that list. The `host` and
`remotePath` of an ftp, sftp or smb Syncer, the `folderId`, `folderPath` and
`oAuthCredentialName` of a googleDrive, oneDrive or dropbox Syncer, and a git
Syncer's `repoUrl` are edited in place. Those classes mirror their source, so
the run after the edit brings the destination into line with the new source
and deletes what the old one left behind, and their state records only when
the sync ran, so no cursor carries over.

So "sync from this folder instead" is an edit, and "write somewhere else" is a
new Syncer and a deleted one. The cursor does not come with it, so the
replacement re-reads from the beginning unless `statePath` is handed over
deliberately. `../usecase/knowledge-drive.md` is where that costs something,
because a database Syncer is the one that keeps a cursor; a git Syncer
declares no `statePath` at all and re-clones every run, which
`../usecase/skill-set.md` says at the field.

The Loader keeps the rule on its source, unlike the Syncer. `knowledgeBaseName`
is immutable, so a Loader cannot be pointed at a different knowledge base,
along with its `loaderClass`, its Drive folder ids and its credential names.

`Indexer.spec.xlsx` is also immutable.
Whether a Drive's index treats spreadsheets as tables is decided when the
Indexer is created.

**Checked:** by walking every `self == oldSelf` rule in asgard-kube
`42e8722` `crd/` back to the property that carries it - 27 properties across
twelve kinds. The count of rules is 28 because one kind carries the same rule
at two paths. The mirror sync and the state that holds no cursor, which are
why the file-store Syncer fields were made editable, against
asgard-syncer `8d278689` `internal/utils/rclone.go` (`buildRcloneSyncArgs`
runs `rclone sync --delete-after`) and the six class files beside it in
asgard-syncer `internal/syncer/`, each of which writes only `syncedAt` as its state. The evaluation-time rule was checked against
asgard-core `478cf5d6` `internal/bpcontroller/server/sandbox_orchestration.go`.

The rest are two families, and `asgard-cli verify` checks both:

    exactly one of [...]        a credential that is neither a literal nor a
                                reference, or both; a class block that is
                                missing or doubled
    class implies its block     `toolsetClass: mcp-server` without
                                `mcpServerConfig`; a `documentClass` without
                                the block named after it

Every one of those renders, lints and passes a server-side dry-run, and is
refused at apply. Run over every renderable reference chart, the check reports nothing on either family, as expected for rules
the platform already enforces.

## An undeclared field is pruned, and a dry run says success

This one can break a release after every check has passed. A CRD
silently discards a field its schema does not declare:

    kubectl apply --dry-run=server     reports success, field already discarded
    helm's server-side apply, in CD    fails with `field not declared in schema`

So the two are different checks. `crd/dry-run-rejected` answers "will it be
accepted"; `crd/unknown-field` answers "will it be kept". The platform's plan report runs both; nothing local
runs the second, because pruning is an apiserver behaviour and no client is
issued cluster credentials.

This has happened. `Toolset.spec.instruction` was removed from the CRD and added
back by hand; the chart passed every dry run and the deploy failed.
`../usecase/write-path.md` and `../usecase/fixed-query-tools.md` both carry it
against the field they concern.

So a passing local gate does not show that a field survives. `asgard-cli gate`
says so, and the plan is the authority:

    asgard-cli pipeline runs watch --release <name> --ref <tag>

## The one the schema cannot enforce

A `SandboxBlueprint`'s subagent must set exactly one of `baseAgentName` or
`aliasName`, and nothing in the CRD checks it. The shape rides inside a
JSON-string value, so CEL cannot see it. It is enforced by the blueprint
controller at evaluation time, which fails the run with
`blueprint <namespace>/<name>: agents[<i>]: exactly one of baseAgentName / aliasName must be set`
(asgard-core `478cf5d6` `internal/bpcontroller/server/sandbox_orchestration.go`).

That is a different failure from every other rule on this page:

    a CEL rule          the apiserver refuses the CR. You find out at deploy
    this one            the CR is accepted, and the run fails when it is used

So a blueprint carrying both, or neither, deploys green and breaks the first
time somebody talks to the agent. It is the only rule here that needs a
blueprint to be read by hand.

## Sources

- `asgard-kube/pkg/apis/asgard/v1alpha1/types.go` - the type
  definitions the CRDs are generated from, with the reasoning in comments
  - asgard-kube `3f9f6c8`; the immutability walk above was made at `42e8722`,
    and nothing between the two adds a `self == oldSelf` rule
- The generated CRDs carry the same rules without the reasoning:
  [asgard-kube `crd/`](https://github.com/asgard-ai-platform/asgard-kube/tree/main/crd)
- The pruning behaviour: read off the two extracts that carry it against the
  field they concern, `../usecase/write-path.md` and
  `../usecase/fixed-query-tools.md`, which took it from a deployment. The two
  plan-report codes are the platform's own

**Unchecked:** what a CEL refusal looks like in CD, which needs an apiserver
that has refused one.
