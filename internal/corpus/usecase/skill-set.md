---
group: Capabilities and scheduling
description: getting skills to a deployed agent
---
# SkillSet, SourceSet, Syncer

Getting skills to a deployed agent. A SkillSet is three CRs, not one, and
deployments of different ages wire the three differently.

**Seen in:** one file holding three CRs per skill set in most deployments, and
several skill sets sharing one store in an older one.

**Checked:** against both shapes and the CRD, and the 1:1:1 shape
in every deployment that converted to it on 2026-09-13, each Syncer
carrying both labels below. How a SkillSet reaches a sandbox against
asgard-core `478cf5d6` `internal/bpoperator/reconciler/sb_reconciler.go`; what
`searchPaths` means against asgard-kube `3da0365` `crd/asgard-ai.com_skillsets.yaml`.

**Unchecked:** how a searchPath resolves. asgard-core only passes the list to the
sandbox runtime, `asgard-agent-sandbox`, which is in no repository here.

Read the platform side first: `../wiki/tools.md` covers how MCP Server, Skillset
and Plugin differ. This page assumes you have read it.

What has to come from the customer before any of this can be built is in
`../needs/skill-set.md`.

## When this shape, and when not

Decide design time or runtime first; they are not interchangeable.

| | `.agents/skills/<skill>/SKILL.md` | `assets/skills/<skill>/SKILL.md` |
|---|---|---|
| read by | the coding agent working in the repo | the deployed agent, at runtime |
| when | while authoring CRs | while answering a user |
| delivery | straight from the working tree | commit -> SourceSet -> SkillSet -> bound by Agent or SandboxBlueprint |
| reaches the cluster | never | yes, that is the point |
| can reach | the laptop it runs on - `.env`, and any network path the FDE has | the sandbox's own environment, which is a closed list the reconciler builds |

The last row decides the design; the others are bookkeeping.
A runtime skill can do what a Workflow cannot - a non-HTTP protocol, a vendor
CLI, an API too large to enumerate as tools - and cannot hold a static service
key, so that route is open only when the credential arrives per turn from the
caller. `../usecase/external-api.md` is the argument and the fields it was read
off; do not re-derive it from a design-time skill that reads `.env`, which is a
property of running on a laptop.

Everything below is about the second kind. A skill in the first kind needs no CR
at all.

## The shape

    SourceSet  ss-<name>     the store. members are directories in it.
      <- Syncer  syn-<name>  fills one member. syncerClass git / database / web.
    SkillSet   sk-<name>     sourceSetName + searchPaths, one path per skill
      <- Agent.managed.skillSetNames, or SandboxBlueprint.skillSetNames

## One SourceSet per SkillSet

`platform-api`'s `POST /v1/skill-set/from-git` creates a SkillSet plus its own
SourceSet plus the git Syncer, bound 1:1:1, and the Platform UI reads that
structure. Keep one file per skill set holding all three CRs separated by `---`.

The Syncer writes to `destinationPath: "git/"`, trailing slash included, and
a searchPath is then `git/skills/pdf/`. The SourceSet declares no members;
the paths its Syncers write to are exactly what is in it.

`destinationPath`, `statePath` and `sourceSetName` are immutable, so a Syncer
never writes somewhere else: that is a new Syncer and a deleted one, and the
apiserver refuses the edit. `repoUrl` is not immutable, so "sync from that
repository instead" is an edit to the existing Syncer.
`../wiki/crd-rules.md` lists the Syncer's immutable fields.

> Two older shapes appear in charts that have not been touched recently.
> Do not copy either; the first one does not apply:
>
> - `members:` on the SourceSet, with `destinationMemberKey` / `stateMemberKey`
>   on the Syncer. The two halves are not in the same state. `members:` is
>   gone from the SourceSet - its spec carries `apiKey`, `contextIndex` and
>   `labels` and nothing else - so a chart setting it loses the field silently.
>   The Syncer's two key fields still exist, marked deprecated and kept "so
>   pre-rename Syncer objects stay readable during the transition", so one set
>   there does apply. Write `destinationPath` and `statePath`; the old pair is
>   readable, not usable.
> - One SourceSet shared across several skill sets, sliced apart with
>   searchPaths, for a skill set that is its own unit. Changed away from on
>   2026-08-28: it leaves the Platform UI
>   unable to find a skill set's git config, so it renders as a skill set with no
>   source. It is a UI failure, not a runtime one, so it goes unnoticed.

### A store somebody writes into needs its own, with no Syncer on it

Do not write into a SourceSet that has a Syncer. Every run of a git Syncer
re-fills the paths it owns, so a file put there by anything else is overwritten
or removed on the next sync, and with a Syncer on a 30-minute schedule that is a
silent loss rather than a visible conflict. So a store that both a Syncer and
somebody else writes into cannot be one SourceSet, however much the contents
look alike.

The shape read off a running deployment is two SourceSets with the same skills
in them for different reasons:

    ss-git-repos        members filled by Syncers from git. Read-only in
                        practice, whatever the CRD allows
    ss-brand-skills     no members, no Syncer, written through the Edge
                        Server's SourceSet volume API at a path the writer
                        chooses - there, `brands/<brand>/skills/<skill>/<rev>/`

The revision directory is the other half. Each write goes to a new
`<rev>/`, so a path is written once and afterwards only read or deleted; the
parent directories are created by the write itself, which is why that SourceSet
declares no members either. Without that, two writers land on one path and the
reader gets whichever finished last.

Do not rename it: the service writing into it holds the SourceSet name in its own configuration,
so renaming the CR silently breaks every route that writes to it.

**Checked:** against a production deployment's
`source_set/brand_skills.yaml`, whose own comment forbids reusing the
Syncer-backed SourceSet and cites that deployment's own service requirement for
it.

**Unchecked:** the mechanism. A Syncer runs as a CronJob of a separate
`asgard-syncer` image with the SourceSet's whole volume mounted
(asgard-core `478cf5d6` `internal/bpoperator/reconciler/syn_reconciler.go`), and that image's source is
in neither asgard-kube nor asgard-core. For a git Syncer the answer is known: it
empties its destinationPath before every copy (`../wiki/knowledge.md` cites
asgard-syncer for it), so a file it did not write there is gone after its next
run. For the other classes one deployment's chart comment is the whole of the
evidence. Treat it as a constraint that deployment hit rather than a documented
platform rule, and if a customer's design depends on writing into a synced
store, ask the platform team rather than this page.

### The one place a shared store is right

A Plugin bundle is the exception, and it is deliberate rather than a chart
that was never updated. A deployment carrying 29 Plugins keeps one
`ss-skill-repos` and lets each bundle's SkillSet slice it with `searchPaths`,
because the skills all live in one repository and one SourceSet per bundle
would be 29 clones of it.

That shape accepts the UI cost: those SkillSets carry no
`skill-set-name` annotation and no `managed-by` label, so they are not presented
as first-class skill sets in the UI at all; they are implementation detail of a
Plugin. See `../usecase/plugin.md`.

The 1:1:1 rule stands for anything a person picks in the UI. The exception is
for a shared skills monorepo behind a bundle, not for convenience.

## Generate it

    asgard-cli add skillset <name> --repo <git url>

That writes the structure below with the fields that fail silently already in
place - the display annotation, the labels the UI needs, the current field names.
Those get lost when the skeleton is copied by hand, and nothing reports them
missing: not helm lint, not CRD validation, not a server dry-run.

The generated file marks the judgement calls TODO, and the rest of this page
covers them.

## The skeleton

One file per skill set, `templates/skill_set/sk-<name>.yaml`, holding all three
CRs separated by `---`.

```yaml
apiVersion: asgard-ai.com/v1alpha1
kind: SourceSet
metadata:
  name: ss-sk-<name>
  annotations:
    asgard-ai.com/source-set-name: "<display name>"
  labels:
    # Hides the Syncer from the generic list and is what the platform cascades
    # on when the SkillSet is deleted. Needed on both CRs.
    asgard-ai.com/managed-by: skill-set
    {{- include "<chart>.labels" . | nindent 4 }}
spec:
  apiKey:
    valueFrom:
      secretKeyRef:
        name: {{ include "<chart>.appSecretName" . }}
        key: asgard_resource_api_key
---
apiVersion: asgard-ai.com/v1alpha1
kind: Syncer
metadata:
  name: syn-sk-<name>
  annotations:
    asgard-ai.com/syncer-name: "<display name>"
  labels:
    asgard-ai.com/managed-by: skill-set
    # Scheduler off; the deploy's apply step fires it once, which needs both.
    asgard-ai.com/syncer-suspend: "true"
    asgard-ai.com/auto-fire-on-rollout: "true"
    {{- include "<chart>.labels" . | nindent 4 }}
spec:
  sourceSetName: ss-sk-<name>
  # A relative path inside the volume. The trailing slash is load bearing:
  # every syncer class decides "this is a directory" from it, and without one
  # the CRD rejects the resource outright.
  destinationPath: "git/"
  syncerClass: git
  schedule: "0 */6 * * *"
  timeZone: Asia/Taipei
  git:
    repoUrl: "https://github.com/<org>/<repo>.git"
    # A public repo: pin main. This repo, private: pin the release tag, which
    # means the tag has to exist before the Syncer can find its commit.
    revision: "main"
    # auth only for a private repo
    auth:
      type: http
      username:
        value: "git"
      password:
        valueFrom:
          secretKeyRef:
            name: {{ include "<chart>.appSecretName" . }}
            key: asgard-github-pat-password
---
apiVersion: asgard-ai.com/v1alpha1
kind: SkillSet
metadata:
  name: sk-<name>
  annotations:
    asgard-ai.com/skill-set-name: "<display name>"
    asgard-ai.com/skill-set-description: "<what these skills cover>"
  labels:
    {{- include "<chart>.labels" . | nindent 4 }}
spec:
  sourceSetName: ss-sk-<name>
  searchPaths:
    # One path per skill directory. A parent directory resolves to nothing.
    - git/skills/pdf/
    - git/skills/docx/
```

The skill files themselves live in `assets/skills/<skill>/SKILL.md` when they
come from this repo, with `name` in the frontmatter matching the directory.

## Designing a skill - the part the generator leaves TODO

### What belongs in a runtime skill

Domain knowledge the CR vocabulary cannot carry: what a status code actually
means in this business, how two systems' entities correspond, which of two
similar figures people mean, the operating procedure for a task.

Not anything expressible as data or as a tool. A skill that says "call the
API and read the third field" should have been a tool.

Not what the tools already say either. The agent reads this skill and the
`tooling.description` of every tool it was given in one context, so a skill
describing the transport a tool now hides - the base URL, the header, the field
that needs a second parse - instructs the model to do something it cannot.
`../wiki/tool-description-and-skill.md` is which of the two owns a fact, and the
short form is that the skill does: it is per subject where a description is per
tool.

### The description is the loading decision

An agent decides whether to load a skill from its `description` alone, so write
it as a trigger condition, not a summary:

    ✗ 關於庫存管理的知識
    ✓ Use when the user asks about stock levels, reorder points, or why a
      figure differs between two systems.

When two skills compete for the same words, the description is not enough. In
one deployment's test, rewriting both descriptions to match the question word
for word still loaded the wrong skill; a routing rule in the supervisor's prompt
fixed it.

### Do not ship an empty skeleton

A skill directory with a heading and no content produces an agent that has been
told a capability exists and then finds nothing. One deployment ended up
declaring in the description that the skill was incomplete and must not be
used. Create the skill when there is knowledge to put in it.

### Dependencies are not resolved for you

If one skill only works when another is also bound, the platform does not
detect that. Say it in the frontmatter, and make sure every consumer binds
both - a SkillSet lists paths, nothing more.

## Fields that are not obvious

A searchPath is one skill directory: `.../pdf/`, not `.../skills/`. The
deployments state that one searchPath resolves as one skill and that naming a
parent directory silently resolves to nothing. The CRD's own description says
the opposite - each path is searched for `SKILL.md` recursively, and an empty
list searches the whole SourceSet - and the code that decides it is the sandbox
runtime, which nothing here can read. One directory per path is right under
either reading, so write it that way. Not checkable from the CR - a wrong path
shows up only as an agent with fewer skills than expected.

`asgard-ai.com/managed-by: skill-set` goes on the SourceSet and the Syncer
in the 1:1:1 shape. It hides the Syncer from the generic list, and it is what the
platform cascades on when a SkillSet is deleted.

`destinationPath` and `statePath` are relative paths inside the volume, and
the CRD enforces the shape: no leading `/`, no `.` or `..` segments, no `//`.
`destinationPath` must end with `/`; `statePath` must not.

Do not set `statePath` on a git Syncer. It clones the whole tree every run; the
incremental cursor belongs to a database syncer's `isMaxValueColumn`.

`revision` decides what a private repo syncs:

```yaml
    revision: {{ .Chart.AppVersion | quote }}   # this repo, private: needs the release tag
    revision: "main"                            # a public repo, or one not tied to releases
```

Pinning `AppVersion` means the tag must be pushed before the Syncer can find
its commit, because only CI stamps the release tag into `appVersion`. It is
also why a local `helm upgrade` breaks the Syncer: the placeholder version
renders as a git ref that does not exist.

Set auth only for a private repo. A public one carries no `auth` block and needs
no `asgard-github-pat-password` declared. Adding a private source adds that key -
under `appSecret:` in `.asgard-pipeline.yaml`, and then set on the platform.

```yaml
    auth:
      type: http
      username: {value: "git"}
      password:
        valueFrom:
          secretKeyRef: {name: ..., key: asgard-github-pat-password}
```

The username is a constant, not a secret. The PAT in `password` is what
authenticates, so `value: "git"` is written literally and a private repo costs
exactly one declared key. One reference deployment reads the username out of
the release Secret as well, under `asgard-github-pat-username`; that chart works
and needs no change, but it is a second key to declare, set and rotate for a
value that never varies, so do not copy it into a new one.

## Two labels, and neither reads the other

The rule is in `../wiki/knowledge.md`. This section covers its effect on this
shape, and what `add` writes to handle it.

    asgard-ai.com/syncer-suspend: "true"        stops the SCHEDULER, and nothing else
    asgard-ai.com/auto-fire-on-rollout: "true"  the deploy fires it once, and waits

What `add` writes sets `syncer-suspend: "true"` and lets the deploy fire each
Syncer once - sync timing is tied to deploys on purpose. That takes both labels.
`../wiki/knowledge.md` says what suspending costs a git Syncer, and why one
deployment schedules its skills Syncers instead.
The platform's apply step fires only the Syncers of this release that carry
`auto-fire-on-rollout`, and firing works fine against a suspended CronJob.

A suspended Syncer with no auto-fire label never runs on its own. Nothing
reports it: the chart renders, the apiserver accepts it, the gate is green, the
run succeeds, and the agent has zero skills. `asgard-cli add skillset` writes
both labels; a Syncer written by hand is where this goes wrong.

The label is read off the applied manifest, so off the CR. The reconciler
copies only `syncer-name` onto the derived CronJob, so anything that goes
looking for these labels on the CronJob silently finds nothing.

An older chart reads back-to-front because the polarity flipped.
Firing on deploy used to be the default, opted out of per Syncer with
`asgard-ai.com/syncer-cd-trigger: "false"` - a label the platform does not
read at all, left behind by the GitHub Actions workflows that predate the
pipeline. In a repo that still deploys through its own CD it still means
something; in one deployed by `asgard-cli pipeline` it means nothing, and the
label that matters is the opt-in one above.

## Whether a deploy fails with no Syncer

The platform decides this now, not an `if` in your own workflow. The
apply step fires what carries the label and waits for it, so with nothing to
fire there is nothing after the dry run that proves the platform accepted any of
it. That is why `asgard-cli gate` warns at zero Syncers, and why a succeeded
run then only means helm returned. A project whose only skills are design-time still
wants one.

## Verify

```bash
asgard-cli check     # frontmatter name matches the directory
asgard-cli verify <project>
```

The xref check resolves `sourceSetName`, confirms every `searchPath` falls under
a declared member, that a SourceSet backs at most one SkillSet, and that
`managed-by` is on both CRs.

Neither catches the "one skill directory per searchPath" rule, because it is not
visible in the CRs. It shows up only as an agent with fewer skills than expected,
so check the resolved skills after the first sync, by listing what the sync
wrote under each searchPath:

```bash
asgard-cli operate skill-set ls <skill-set> <searchPath> --release <release>
```

After a deploy, confirm the fire in the apply step's log:

```bash
asgard-cli pipeline runs log <run-id> apply
```

Its syncers section lists what was fired. No section, or zero Syncers opted in,
means the auto-fire label is missing. `kubectl get job` is not a check: the Job
is deleted an hour after it finishes, and the CronJob's last-schedule column
stays empty for a fired run. The Syncer's own history keeps the outcome after
the Job is gone, and is read through the SkillSet:

```bash
asgard-cli operate skill-set executions <skill-set> --release <release>
asgard-cli operate skill-set sync <skill-set> --release <release> --wait <duration>
```
