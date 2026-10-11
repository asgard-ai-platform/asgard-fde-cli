---
description: what each step of the gate is for, the step that only the platform can run, why a skipped step is not a pass
---
# Run the acceptance gate

Every project has a read path and an entry point. Run the gate before committing
anything, and stop at the first red step. Do not push until it is fixed.

Run it with one command:

    asgard-cli gate

That is the whole local half. It is one command because a checklist written in
prose goes stale and the binary does not, and a reader cannot tell when the
prose has gone stale. A step it could not run is reported as skipped, and a
skipped step has not passed.

Load the `asgard-cr-verification` skill under .agents/skills/ for what the
PLATFORM checks, which is the other half and the authoritative one. It comes
from the platform - `asgard-cli skill update` writes it - so it states what the
server this repository deploys to actually checks, rather than what some
version of it did when this tool was built.

What `asgard-cli gate` runs, and what each step is for:

  tools    helm on PATH. Without it the chart steps cannot run, and the
           gate says skipped rather than passed.

  repo     the repository's structure: the README project table against the
           folders on disk, .asgard-pipeline.yaml against the charts it names,
           the docs/ layers, the requirements indexes. Alone: asgard-cli check

  shipped  whether the files this CLI wrote here still match the ones it
           carries - a page renamed upstream, or one edited here. It reads
           .asgard-scaffold.json, which records a digest and a CLI version per
           file. Alone: asgard-cli init, which reports and does not overwrite

  binding  whether this checkout records which workspace and which pipeline it
           deploys through. Nothing recorded is the ordinary state of a
           repository nobody has connected yet, and it is reported as skipped.
           Alone: asgard-cli pipeline show

  skills   whether the reference material here still describes the server this
           repository deploys to. Alone: asgard-cli skill status

  lint     helm lint on each chart, with the reserved asgard block and
           nothing else. That is what proves values.yaml declares a default
           for every .Values.* the chart itself owns; overlay an environment
           file and a missing default is masked until it nil-pointers for
           somebody running plain helm template. Linting with no -f at all
           fails on every chart that reads .Values.asgard.*, and a chart must
           not declare that block.

  render   each release renders, with placeholder platform values.

  verify   the invariants helm cannot see, because to helm these are opaque
           CRs: a dangling reference, a missing display annotation, a Workflow
           with no set labels, the agent split. A wrong entry name is as
           fatal as a wrong workflow name, and apply catches neither.
           Alone: asgard-cli verify [release]

Then the step that cannot be run here. Push, and read the plan back:

         asgard-cli pipeline runs watch --release <name> --ref <tag>

     This step cannot be run here, and it is required. The
     plan renders with the release's real values and sends every CR to the
     apiserver with a server-side dry run, so CEL rules, patterns, required
     fields and unknown-field pruning are checked against the real cluster.
     Nothing local can do that: no cluster credential is issued to a client,
     which is why the local steps above check a different class of thing -
     what a dry run passes and runtime still fails.

     It answers two questions: `crd/dry-run-rejected` is "will it be
     accepted", `crd/unknown-field` is "will it be kept". A field the CRD does
     not declare passes a plain dry run and breaks the deploy, because CRDs
     prune what they do not declare while helm's server-side apply refuses it.
     A refusal names the CR and carries the apiserver's own message, and the
     run ends `plan_failed` at Render & Dry-run with Review and Apply skipped,
     so nothing is written.

     If the run was never created, the push matched nothing - no pattern
     matched, or the release was never created on the platform.
     `asgard-cli pipeline deliveries` says which.

     What this step catches is written down: `../wiki/crd-rules.md`
     lists the CEL validations the apiserver evaluates, which `helm lint` does
     not run and a dry-run does not report faithfully - and the one rule the
     schema cannot express at all. Read it when a deploy fails here.

`asgard-cli gate` needs only `helm` on PATH, plus a session for the one step
that asks the platform (`--offline` skips it). Reading the plan needs a remote
and a signed-in session, and no cluster access at all. `asgard-cli doctor` says whether helm is
installed and how to install it.

Done when: every step is green, and you have said which ones could not be run.

No command says a chart is complete, and `asgard-cli project` does not either.
It lists what each chart declares and says nothing about what it lacks: a chart
with a SemanticLayer and no Agent may be finished or unfinished, and the files
cannot tell the two apart. Completeness is your judgement against the request,
and after it new capability is added with the loop in `../guide/enhance.md`.

**Checked:** the steps above are `gate`'s own, in its own order, as
`asgard-cli gate --help` lists them; each names the command that runs it alone
and each of those exists. The scaffold ships no dry-run or fidelity script. The
claim that a dry-run reports success while dropping an undeclared field is the CRD's
documented pruning behaviour. The plan's half against a run on the platform's dev
cluster, 2026-10-11, for a release of the platform's own demo repository carrying
one Toolset that broke a CEL rule: `watch` printed the steps, the
`crd/dry-run-rejected` finding naming the Toolset with the apiserver's message,
the variables, the per-CR changes, and the dry run's count of rejected resources
and undeclared fields; the run ended `plan_failed`, Review and Apply skipped.

**Unchecked:** that the local steps and the plan together are the
whole gate; a deploy that fails for a reason neither looks at is what would show
otherwise.
