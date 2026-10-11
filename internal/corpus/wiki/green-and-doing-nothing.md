---
group: In practice
description: "a chart that deployed and passed every check but does nothing: the shapes that have happened, why no layer of the toolchain catches them, and which have a rule. Start here when the symptom is silence rather than an error"
---
# It deployed, everything is green, and it does nothing

Every check this tool and this platform run asks whether a CR is legal. None
asks whether it can do what it visibly intends to. So a chart can render,
lint, pass CRD validation, pass the server-side dry run, apply, report `ok`, and
answer nothing. The first sign is usually an agent telling a customer
something untrue: a model that gets no data does not report a failure, it
answers anyway.

This page is organised by symptom rather than by CR kind, because the reader
arriving here does not know which kind is at fault.

## The ones that have actually happened

Each row is one somebody walked into. A chart can be legal, accepted and inert
in more ways than these.

| what you see | what it is | where |
|---|---|---|
| a tool returns nothing, and the agent answers from memory | processors present, `relationships` absent or partial, so only the entry's `handlingProcessor` runs | `../usecase/workflow-chain.md` |
| a value is set on the platform and the CR behaves as if it were empty | the key is declared under no release, so it is stored and never injected | `../usecase/write-path.md` |
| a capability that was working stops, with no change to it | the variable it read is still declared and set, and the thing that read it was replaced | this page, below |
| an agent has no skills at all | the Syncer is suspended and has no `asgard-ai.com/auto-fire-on-rollout` label, so nothing ever runs it | `../usecase/skill-set.md` |
| an agent has fewer skills than it should | a `searchPaths` entry names a directory the sync never wrote - a typo, or a path from before the repository moved - which loads nothing | `../usecase/skill-set.md` |
| an agent has more skills than it should | a `searchPaths` entry names a parent directory, and every `SKILL.md` under it loads, including one nested inside another skill | `../usecase/skill-set.md` |
| the very first message on a new channel is never answered, and every message after it is | the entry routes into `listen-message` before anything replies, and reaching a `listen-message` ends the request | `../usecase/flow-agent-supervisor.md` |

## Why the toolchain does not catch these

Each layer answers a different question, and none of them is this one:

    helm lint                does every `.Values.*` a template reads have a default
    CRD validation           are the fields the right types, are required ones present
    server-side dry run      does the apiserver accept this object
    asgard-cli verify        do the references between CRs resolve
    the run                  did apply return

"Accepted" and "doing something" are different claims, and treating the first
as the second costs hours. An engagement debugged two real bugs in a
tool's code - a misclassified 401, a wrong query parameter - before establishing
that the code had never executed at all. Both fixes were correct and neither was
reachable.

So when a shape does not work and every check is green, first establish whether
the code runs, then what it computes.

## What does catch them

`asgard-cli verify` has graph checks for this, and they are
the only ones here that ask about behaviour rather than legality:

- W3 - a Workflow declaring several processors and no `relationships`. Only
  the entry's processor runs; the rest are dead. This is a failure, because
  there is no shape it is correct for.
- the credential key check - a `secretKeyRef` reading a key
  `.asgard-pipeline.yaml` declares for no release.
- the placeholders step - the generator's own TODOs still in the render,
  which is how a published Agent comes to show `TODO` as its sample questions.

No check covers a capability that stops with no change to
itself: a declaration that was right when it was written and became dead when
the shape around it changed. Nothing compares what is declared against what the
render reads, so this drift is found only by a person asking what still uses the
declaration.

**Checked:** from engagements' own reports and the failures
behind them - a tool whose HTTP call was never wired, a variable left declared
after the tool that read it was replaced, and a credential nobody could obtain.
The gate rules named above were written from those and each was held against
every reference deployment to confirm it reports none of them. That a run starts
at the entry's `handlingProcessor` and that reaching a `listen-message` ends
the request, against asgard-core `478cf5d6` `internal/bpcontroller/server/bp_controller.go`.
