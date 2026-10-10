# asgard-fde-cli

**English** | [繁體中文](README.zh-TW.md)

Command line tool for Asgard FDE (`asgard-cli`).

## Install

On macOS or Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/asgard-ai-platform/asgard-fde-cli/main/install.sh | sh
```

On Windows, in PowerShell:

```powershell
irm https://raw.githubusercontent.com/asgard-ai-platform/asgard-fde-cli/main/install.ps1 | iex
```

`install.ps1` is the Windows half and makes the same decisions: it verifies the
download against the release's own checksums, installs under `%LOCALAPPDATA%`
and adds that to your user PATH. Installing where the user owns the files needs
no elevation and lets `asgard-cli update` replace the binary later.

`install.sh` takes the newest release for this platform, verifies the download
against the release's own checksums, installs to `/usr/local/bin`, and then runs
the binary once. macOS scans a newly written unnotarized binary on first
execution, and running it once in the installer means that scan does not happen
in front of a customer. Read `install.sh` at the repository root before piping
it into a shell.

It installs to `/usr/local/bin` on Linux too, on purpose. The `.deb` and the
`.rpm` install into `/usr/bin`, which belongs to the package manager, and a
binary there cannot replace itself: `asgard-cli update` refuses rather than
leaving dpkg describing a version that is not on disk. The filesystem standard
reserves `/usr/local` for software installed outside the package manager, so an
install made this way can update itself afterwards.

The rest of this section does the same thing by hand.

The URL carries no version, so it keeps working across releases. GitHub
resolves `/releases/latest/download/<name>` to the newest:

```bash
curl -fsSL https://github.com/asgard-ai-platform/asgard-fde-cli/releases/latest/download/asgard-cli_darwin_all.tar.gz \
  | tar xz asgard-cli
sudo install -m 0755 asgard-cli /usr/local/bin/asgard-cli
asgard-cli doctor          # says whether helm is on PATH
```

Swap `darwin_all` for `linux_amd64`, `linux_arm64` or `windows_amd64`.
`darwin_all` serves both Intel and Apple silicon, so there is nothing to choose
on a Mac, and the `.pkg` installs the same binary by double-clicking.

On Debian or Ubuntu the `.deb` is one command; RPM and Alpine hosts take the
`.rpm` or the `.apk` from the same release the same way:

```bash
arch=$(dpkg --print-architecture)      # amd64 or arm64
curl -fLO https://github.com/asgard-ai-platform/asgard-fde-cli/releases/latest/download/asgard-cli_linux_${arch}.deb
sudo dpkg -i asgard-cli_linux_${arch}.deb
```

That puts it in `/usr/bin`, so upgrades go through dpkg rather than through
`asgard-cli update`, and the update command says so if you run it there. Take
the tarball or the installer instead if you want the tool to keep itself
current.

To have the platform detected for you, or to take a specific release rather
than the newest:

```bash
repo=asgard-ai-platform/asgard-fde-cli
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')
# One binary serves both Macs; every other platform is per-architecture.
[ "$os" = darwin ] && arch=all

gh release download --repo "$repo" --pattern "*_${os}_${arch}.tar.gz" \
  --output - | tar xz asgard-cli
sudo install -m 0755 asgard-cli /usr/local/bin/asgard-cli
asgard-cli doctor          # says whether helm is on PATH
```

That takes the newest release too, because `gh release download` with no tag
does. Debian or RPM hosts can take the `.deb` / `.rpm` instead.

### macOS may stall or kill the first run

The binaries are ad-hoc signed (what Go's linker does so they run at all) and
are not notarized, so macOS scans the first execution. The same asset,
downloaded with `gh` on one machine, did all three of these: ran immediately,
stalled for minutes and then ran, and was killed with exit 137 and no output
before working on the next attempt. A browser download fails reliably: the
quarantine mark it adds got exit 137 every time.

If it appears to do nothing, this is the cause:

```bash
xattr -d com.apple.quarantine ./asgard-cli   # only if a browser downloaded it
./asgard-cli version                          # and try a second time
```

Install the tool before somebody needs it rather than during a meeting.

If you already build Go, the module works directly:

```bash
go install github.com/asgard-ai-platform/asgard-fde-cli/cmd/asgard-cli@latest
```

`asgard-cli version` reports what a release built; a `go build` with no ldflags
falls back to the module and VCS metadata rather than claiming a version it
does not have.

### Staying current

```bash
asgard-cli update              take the newest release
asgard-cli version --check     ask whether there is one, and change nothing
```

Every command that can reach a network asks whether a newer release is
published, at most once every two hours, and prints one line on stderr when
the answer is yes. The answer is recorded in `update-check.json` beside the
profiles, never in a customer's repository, which is somebody else's checkout
and is committed. The question is asked alongside the command rather than
before it, so runs that ask are not slower than runs that read the cached
answer.

`update` replaces this binary in place: it takes the version-less asset for
this platform, verifies the download against that release's own checksums by
HASH rather than by name, runs the new binary where it landed, and only then
renames it over the old one. Because of that order, a build macOS kills, a
truncated download or an empty archive all fail before anything has been
replaced: the result is either the new version or exactly what was there
before. On macOS this is also where the Gatekeeper scan of a newly written
unnotarized binary happens, instead of in front of a customer.

It refuses when it cannot update safely, and each refusal names what to run
instead: a package manager's copy is moved by that package manager, a directory
you cannot write needs `sudo asgard-cli update`, and a `go build` binary has no
release to be compared against. The line every command prints names whichever
of those applies to your install.

On Windows it takes two renames. A running `.exe` cannot be written or deleted,
but it can be renamed, so the old binary is moved aside to `asgard-cli.exe.old`
and the new one takes its name. The displaced file cannot be removed until the
process running from it exits, so a later run deletes it. A leftover
`asgard-cli.exe.old` beside the binary after an update is expected.

Nothing replaces the binary without being asked. A CLI that overwrites itself
in the background can land mid-command, mid-meeting, or while a package manager
believes it owns the file.

The check is off wherever stderr is not a terminal, so a CI log and a piped
stderr get nothing and make no call. A command whose help says it touches no
network never asks - `init`, `size` and `guide` among them, the commands used in
a meeting - because even a request that fails silently goes out over the
meeting's network. `ASGARD_NO_UPDATE_CHECK` turns off the background check
everywhere else; `--check` still answers.

A repository also records this without a network: `.asgard-scaffold.json`
records which version wrote each file this CLI ships, so in a checkout where
somebody has already run a newer binary, `asgard-cli gate` names that version.

## Development

```bash
make build       # .out/asgard-cli
make install     # onto your PATH
make gate        # everything CI checks
make help        # the rest
```

`make install` is `go install ./cmd/asgard-cli`: the binary lands in `GOBIN`,
or in `go env GOPATH`/bin when that is unset, and the target says so afterwards,
along with a warning when something earlier on your PATH will shadow what it
just installed. To install somewhere else, name it:

```bash
make install GOBIN=~/.local/bin
```

What lives in which directory is in [STRUCTURE.md](STRUCTURE.md) and is not
repeated here, so that there is one directory listing to keep current.

To add a subcommand, write a `newXxxCmd()` in `internal/cli/` and register it
through `addTo(cmd, group..., ...)` in `root.go`. The group is required - cobra
panics on a `GroupID` the parent does not have - so every command is placed in
a help group when it is added.

- [Goal.md](Goal.md) - what this tool is for, in four points.
- [APPROACH.md](APPROACH.md) - how the main capabilities are implemented: the
  corpus, the pointer form, the audits, retrieval, what `init` writes, and what
  only the platform can decide.
- [STRUCTURE.md](STRUCTURE.md) - what every directory is for, including the
  bodies of embedded material and which one a change belongs to.
- [AGENTS.md](AGENTS.md) - the conventions this repo follows, and what the gate
  is.
- [TASK.md](TASK.md) - what is still to do.

## Commands

### `init`

Write the repository skeleton here, so that a coding agent can take over.

```bash
mkdir acme-asgard-kube && cd acme-asgard-kube
asgard-cli init
```

```
This writes the Asgard repository skeleton into

    /path/to/acme-asgard-kube

and the repository will be called acme-asgard-kube, after that directory.

Write it here? [Y/n]

This is not a git repository yet. The skeleton expects one: the customer's
design-time credentials live in a .env that a .gitignore line keeps out of git.

Run `git init` here? [Y/n]

  created      .agents/skills/asgard-fde-onboarding/SKILL.md
  ...
45 created in /path/to/acme-asgard-kube

note: there is no `origin` remote here yet. Connecting derives the provider
      account and the repository from it, so the first thing you will be asked
      for is this repository's remote URL.

Now open this directory in your coding agent and say:

    Connect this repo to the Asgard platform
```

This is the one command written for a person, and the only one that asks
questions. Every other command is written for a coding agent working in a
repository that already exists, and until `init` has run that repository has no
`AGENTS.md`, no `CLAUDE.md` and no `.agents/skills/`. An agent opened in an
empty directory knows nothing about Asgard, so it cannot be asked to run a
command that needs a workspace id.

It touches no network and needs no account. The skeleton depends only on this
tool, not on any platform, so it can be written offline, before a workspace
exists, or before anybody has signed in. That is why it can run first.

Connecting the checkout to a platform is not part of it. Signing in, choosing a
workspace, creating a pipeline and fetching the material describing the server
come afterwards, guided by the agent this command just equipped.
`asgard-cli gate` says what is still missing at any point.

Run it again whenever this CLI has moved on or a project was added: existing
files are left alone and reported as skipped. `--force` takes the newer shipped
material, discarding local edits to the skeleton; files this tool writes into -
the indexes, the open-questions table, the living spec - are preserved either
way and reported. In a file carrying an `asgard-cli:managed` region it replaces
the region and nothing else, because that region is all this CLI wrote there;
the scaffolded `AGENTS.md` tells its reader the same about the half above the
marker. `--yes` asks nothing, which is also what happens when stdin is not a
terminal, so a re-run from an agent or from CI needs no interaction.

A file that is already present may still be out of date, and this matters for
the files this CLI ships: AGENTS.md and the design-time skills. It records which
version of itself wrote each of them, in `.asgard-scaffold.json` beside the
declaration, and reports a difference as `behind` (this CLI moved on, nobody here
touched it), `edited` (somebody here did, and `--force` would discard it), `ahead`
(a newer CLI wrote this repository, and `--force` refuses to downgrade it) or
`retired` (an older CLI shipped the file and this one does not).

It refuses to write into a home directory or a filesystem root, to guard
against writing a whole scaffold one directory above where it was meant.

It writes the part of a customer repo that is the same for every engagement:

| | |
|---|---|
| `AGENTS.md` | the platform contract, with the customer-specific sections marked TODO |
| `docs/` | the four-layer model (meeting-notes / decisions / living spec) and the SDD rules |
| `requirements/` | the task and request indexes |
| `.agents/skills/` | the design-time skills that hold for any Asgard, `db-query` among them, plus `asgard-platform/` - the wiki and the extracts as greppable files; the ones describing a particular server come from `asgard-cli skill update` |
| `assets/` | the runtime-skill directory |
| `.asgard-pipeline.yaml` | the deployment declaration, with one release per project to fill in |
| `projects/<slug>/` | one chart skeleton per project |

It does not write the customer's own knowledge: which systems exist, how the
projects split, what the CRs look like. The onboarding produces that.

The generated skeleton passes its own gate on the first run:

```bash
asgard-cli gate                               # everything this machine can check
```

Do not run `helm lint` by hand. The platform injects a reserved
`.Values.asgard` block into every render, and a chart must not declare it in its
own `values.yaml`, so a bare lint fails on every chart that reads
`.Values.asgard.projectEnvironmentId`, which is every chart that labels
anything. `gate` supplies that one file and nothing else.

### `guide`

`guide` reads one decision against the repository you are in. The static half
of each piece lands as a file like everything else; the command adds this
repository's own state - which projects exist, what is still open.

```bash
asgard-cli guide                  # every piece of guidance, by name
asgard-cli guide requirements     # one, read against this repo

cat .agents/skills/asgard-platform/guide/requirements.md   # the static half
```

### The links to this engagement's own systems

A deck for partners is mostly links, and every id in one is already on disk:
`.asgard-cli.yaml` carries the workspace, the git remote carries the repository.
Assembling them by hand goes wrong in ways that still render correctly: the
site's root in place of the page, the URL spelled out beside the name that is
already the link, or the link dropped on a guess about who can open it.

```bash
asgard-cli links                  # what this checkout is bound to
```

It prints only what it knows. The Console is a different host from the API
and neither implies the other, so the Console is known for the hosted
installation and unknown for any other; a pipeline's Console path is written
down nowhere. Unknown values are printed as named absences, not as guessed URLs.
It reaches no network and needs no session.

No command says where the engagement is. `project`, `question`, `request` and
`task` each read one file back to you; none of them derives a position from the
others.

```
  requirements   Turn what the customer said into a request
  projects       Decide how the work splits into projects
  data-sources   Wire up the customer's databases
  read-path      Decide each project's read path
  entry-point    Decide each project's entry point
  knowledge      Decide where unstructured knowledge lives
  verify         Run the acceptance gate
  deploy         Deploy
  enhance        Add a capability to a repo that is already live
  idle           Nothing in flight
```

These are not steps in a sequence. An onboarding is not linear: three of the
most expensive decisions in the engagement this was built from were made, built
and reversed, and an engagement that has already gathered every requirement has
no stage at all. Guidance is reached by name with `guide`, or by grepping
`guide/` for the subject.

`read-path`, `entry-point` and `knowledge` print the wrong answer next to the
right one. Those are the three decisions this engagement got wrong once and
reversed, and in each case the wrong answer is the one that looks obvious.

A chart does not always end with an entry point, and no command here says
whether one is finished. A SemanticLayer with nothing mounted on it is either a
finished Mimir deliverable or an agent nobody has written yet, and the files
cannot tell the two apart. `asgard-cli size <shape>` lists what a shape is made
of, for a person to compare against. Nothing records a chart's intended shape,
because this tool cannot check a note of what somebody meant to build.

### `project`, `request`, `task`, `question`

Four commands read the repository back to you, one file each, each with
`--format json`. None of them infers anything from the others.

```bash
asgard-cli question    # what nobody has answered yet, and who each is with
asgard-cli request     # what the customer asked for and is not done
asgard-cli task        # the task specs that are open
asgard-cli project     # what each chart declares
```

Read `question` first. In a repository somebody else started, designing past a
question they already knew was open is the fastest way to do damage.

```
Projects:

  insight              DataConnector, SemanticLayer
  helpdesk             chart is empty
```

It says what each chart has and nothing about what it lacks. Comparing against
a recorded per-project shape would treat somebody's note of intent as a
specification. The list comes from the repository itself - the chart paths the
declaration names, and the directories under `projects/` - so there is no
second copy of it.

### `reference` - filing what the customer hands over

    asgard-cli reference add <file> --what "<what it is>" \
      --from "<who supplied it>" --dated <the document's own date>

`references/` is background for humans and spec-writing agents. The running
agent does not read it: domain knowledge the agent needs at run time belongs in
a skill, because a skill is synced into the platform and this directory is not.

Every engagement files documents, and without a command each invented its own
provenance table and directory name. This command copies the file
byte-identical, so a later version can be diffed against the filed one, and
puts the provenance in `references/_index.md` rather than in a header pasted
into the customer's own file.

`--dated` is the document's own date, not today. That date decides whether the
material is stale; record a document that carries no date as carrying none.
`asgard-cli check` warns about rows that are short.

### `local-env` - a form, because a password must not reach a transcript

    asgard-cli local-env
    asgard-cli local-env --focus UOF_DB_HOST,UOF_DB_PASSWORD

A coding agent must never ask anybody to give it a password, either in the
conversation or as "paste it and I will remove it after": a credential that has
been through a transcript is disclosed. Asking a non-engineer to edit a dotfile
by hand is also unreliable.

So the agent writes the key names with empty values, and this command opens a
form to fill them in: one page on 127.0.0.1 on a random port, a one-time token
in the URL, no other host name answered, and a policy that lets the page talk to
nothing but the process that served it. It closes as soon as the form is saved.

It returns only key names, never a value - not on save, not in an error, not in
the summary. `--focus` highlights the keys you are waiting for and does not hide
the others: the person filling it in may know about a second database nobody
has mentioned, and they can add keys. Re-read `.env` afterwards rather than
assuming you got back only what you asked for.

### `request`, `task`, `question`, `decision` - writing the records

Work arrives as a request: one thing the customer wants that the agent cannot
do today. Everything else hangs off it.

```bash
asgard-cli request add "warehouse staff want to ask about stock levels in chat"
asgard-cli request target REQ-001 erp
asgard-cli request ready REQ-001

asgard-cli task add "expose stock levels" --request REQ-001 --project erp --complexity M
asgard-cli task ready TASK-001
asgard-cli task start TASK-001
asgard-cli task done TASK-001

asgard-cli question add "which stock figure is authoritative" --blocks REQ-001 --ask "the warehouse lead"
asgard-cli question answered 1 "location 608 only" --decision 2026-09-04-safety-stock.md

asgard-cli decision add "the website reads through fixed query tools" --module architecture.md
```

None of this state lives in the CLI. Each command writes a file in the
customer's repository, because that repository is what the next agent opens:

| record | file | carries |
|---|---|---|
| request | `requirements/requests/REQ-xxx-<name>.md` + its registry row | date raised, status, target project, the customer's own wording |
| task spec | `requirements/tasks/TASK-xxx-<name>.md` + its queue row | date created, status, the SDD sections, a dated log of every transition |
| open question | a row in `docs/open-questions.md` | date raised, what it blocks, who can answer |
| decision | `docs/decisions/YYYY-MM-DD-<topic>.md` + a traceability row | the date in the file name, the module it changed |

These are commands rather than instructions to write a file because each record
lives in more than one place. A task's status is in the queue table, in the
spec's own `Meta`, and in the spec's execution log; a request's target project
is in the registry's Spec column and in its `Meta`. Moving one by hand means
three or four edits, and if two of them disagree the next reader cannot tell
which is current. Every command here updates all of them, and stamps the date
rather than asking for it.

`asgard-cli question`, `request`, `task` and `project` read them back, each with
`--format json`.

### `add`

Write a CR skeleton into a project's chart, correct in the parts that fail
silently.

```bash
asgard-cli add                                   # list the kinds
asgard-cli add dataconnector erp --db-class postgres --project erp
asgard-cli add flowagent support --bot-class line --project site
```

```
created projects/erp/chart/app/templates/data_connector/dc-erp.yaml
updated projects/erp/chart/app/values.yaml (added the values it reads)

Next:
  1. what it is:      .agents/skills/asgard-platform/wiki/settings.md
  2. how to build it: .agents/skills/asgard-platform/usecase/semantic-layer.md
  3. fill in the TODOs
  4. verify:          asgard-cli check
                      asgard-cli verify
```

Ten kinds: `dataconnector`, `semanticlayer`, `agent`, `httptool`, `querytool`,
`skillset`, `trigger`, `knowledgedrive`, `plugin`, `flowagent`.

It generates a skeleton: the structure and the traps are right, and the content
is marked TODO. It generates the parts nothing else catches: a missing display
annotation shows a nameless resource in the UI, a Workflow without its set
labels is invisible there, a Trigger without its own two labels opens as a blank
canvas, and a field renamed upstream still lints clean under its old name. None
of those is caught by `helm lint`, by CRD validation, or by a server-side
dry-run.

It reads the chart before writing into it, so a reference it emits points at
something that exists: one SemanticLayer in the chart is mounted, several are
refused by name, a SkillSet is referenced only if one is there. A second query
tool does not re-emit a Toolset the first one already wrote.

The two pointers it prints are in reading order and answer different questions:
`wiki` says what the thing is, `usecase` says how it is assembled and assumes you
already know the first.

### The material, as files

Five bodies of reference material, compiled into the binary and written into a
customer repository by `asgard-cli init`, under one directory:

```
.agents/skills/asgard-platform/
  index.md    the map: all five, as paths, and what is deliberately absent
  aliases.md  what a customer said -> what to search for
  wiki/       what the platform has, and which CR a UI name maps to
  usecase/    how ONE deployment shape is assembled, field by field
  needs/      what to get from the customer before a shape can be built
  brief/      what has actually been got wrong, before you do the thing
  guide/      which decision to make now, and what it costs to change later
```

| | answers | written from |
|---|---|---|
| `wiki/` | what the platform is, who each piece is for, and where the UI's names stop matching the resources a chart declares | the product documentation, [asgard-docs](https://github.com/asgard-ai-platform/asgard-docs), checked against the CRDs |
| `usecase/` | how one shape of deployment is assembled, field by field, and what a wrong value costs | deployments already in production |
| `needs/` | what to obtain from the customer before a shape can be built at all | the interview, the per-channel credential tables, and what an engagement found out too late |
| `brief/` | what this activity gets wrong, before you do it | activities somebody has actually got wrong |
| `guide/` | one decision, the obvious answer, and what reversing it costs | three decisions reversed in production |

An extract assumes you already know the platform has that shape; a wiki page is
where that knowledge comes from. `asgard-cli add` prints one of each.

There is no search command. The documents are on disk, so `cat` and `grep` are
the interface:

```bash
grep -ril "allowlist" .agents/skills/asgard-platform/
cat .agents/skills/asgard-platform/wiki/processors.md
```

Read two files first, because a grep does not cover what they do:

- `aliases.md`, if the question did not arrive in English. The material is
  English and a customer conversation usually is not, so a term taken from what
  somebody said matches nothing, which looks the same as a subject the material
  does not cover.
- `wiki/glossary.md`, for the word you searched. A result in the wrong sense
  looks like an answer: `payment` is billing between Asgard and the customer,
  and also the customer's own payment gateway.

With no repository, run `asgard-cli init` in an empty directory. It needs no
account and touches no network, so it works in a meeting, before there is a
directory.

```bash
mkdir -p /tmp/asgard && cd /tmp/asgard && asgard-cli init
```

The wiki's own conventions - its three layers, what a page must carry, and how
it is kept from going stale as the platform moves - are in `wiki/README.md`.

### `size`, `issue-report`

Two commands that read no repository.

`size` shows what one capability is made of before it is written, which is the
first question a proposal is asked and the basis of a quote. The counts come
from deployments in production rather than from reasoning, which matters where
the intuitive answer is wrong: the flow-agent shapes contain no `Agent` CR at
all.

```bash
asgard-cli size                      # the shapes, and what each costs empty
asgard-cli size flow-agent-single --databases 2 --queries 4
```

`issue-report` files a gap in this tool, and it is the only way what one
engagement learned reaches the next. The gap does not belong in the customer
repository, where only that engagement would see it. `--send` sends it as a
User Feedback to the maintainers' Sentry project, which only they read, not as
an issue on this repository, which is public. It is for the tool, never for the
customer: what is wrong in the customer's systems goes on the Workbench (see
`workbench`).

```bash
asgard-cli issue-report                          # the route, and what a report has to say
asgard-cli issue-report --new > report.md        # a body with the evidence already in it
asgard-cli issue-report --send report.md --email you@example.com   # once every TODO is answered
```

The report ends with "What I now know", for a discovery rather than a defect:
something the platform does that the material does not say, found out on a
deployment.

### `check`

The first step of the acceptance gate, and the only one that needs no external
tool. It verifies the invariants a chart render cannot see, which otherwise
surface at deploy time or when the next person picks the repo up:

```bash
asgard-cli check                    # whole repo
asgard-cli check erp                # project-scoped checks limited to erp
asgard-cli check --format json      # errors and warnings as separate arrays
```

```
warn   assets/skills/ has no skill directories yet
ok  structure is consistent (1 project(s): [erp])
```

- the root README's project table matches the directories under `projects/`
- `.asgard-pipeline.yaml` parses, names no release twice, and every release it
  declares points at a chart directory that has a `Chart.yaml`
- runtime skills under `assets/skills/` carry `name` and `description`
  frontmatter, with the name matching the directory
- the SDD entry points under `requirements/` are present
- the `docs/` spec layer is intact: required files, the living spec's module index
  matching the files on disk, dated filenames, and every relative link inside
  `docs/` resolving
- no page is an orphan: a document under `docs/` or `requirements/` that
  nothing links to is not read. This is a warning rather than an error, because
  a decision recorded today and not yet applied is an orphan until it is.

`asgard-cli verify` adds the invariants a render carries, including the CRDs'
conditional CEL rules: a credential that sets neither a literal nor a reference
or both, a class block missing or doubled, a `toolsetClass` without the block it
requires. Every one of those renders, lints and passes a server-side dry-run,
and is refused at apply. 27 of the 69 `XValidation` markers are
`self == oldSelf`, comparing a proposal against the object already on the
cluster, and cannot be seen offline at all.

Naming projects limits the project-scoped checks to those; the repo-wide checks
always run. It exits non-zero when anything fails, and warnings do not fail it.

It cannot see two pages that contradict each other, a claim a newer source
superseded, or a concept every document explains in passing and none owns;
those need a reader. The `knowledge-base` skill under `.agents/skills/` in the
generated repo is the pass for that.

### `render`, `verify`, `doctor`

These three let the acceptance gate run on Windows.

```bash
asgard-cli render internal-dev         # manifests to stdout, summary to stderr
asgard-cli verify                      # render every release, check the invariants
asgard-cli verify --rendered file.yaml # check a stream that is already rendered
asgard-cli verify --format json        # one record per render, each check named
asgard-cli doctor                      # which external tools are here, and how to get them
```

`render` takes a release, not a project and an environment. Where a chart
deploys is a release in `.asgard-pipeline.yaml`, and one chart can have several;
`asgard-cli pipeline releases` lists the ones the platform has.

`check` and `verify` are the gate an agent is trying to turn green, so both
take `--format json`. In text a warning and a failure differ by one word at the
left margin and only one of them is fatal; in JSON they are separate arrays. A
failing JSON run exits 1 and prints nothing to stderr, because the report
already says it failed.

`render` replaces the generated repo's `common/render.sh`, and `verify` replaces
its `check_chart_xref.py` and `check_agent_split.py`. The old chain was:

```
bash render.sh  ->  yq  ->  helm template  ->  python3 + PyYAML
```

Four external dependencies, of which three do not work on Windows without WSL
or Git Bash, while helm and kubectl both have native Windows builds. The
prerequisite is now `helm` alone, and one binary:

```
asgard-cli verify  ->  helm template  ->  internal/gate
```

`verify` renders in process, so there is no pipeline and no temporary file, and
`render` keeps the manifests on stdout with everything else on stderr so the pipe
forms work identically in cmd, PowerShell and bash.

The apiserver's own CEL, pattern and required validation, and the unknown-field
pruning a dry run hides, need a cluster, and no client is ever issued
credentials for one, so they run in the platform's plan. The local half is
`asgard-cli gate`; the authority is the plan report.

### helm and kubectl are prerequisites, not dependencies

Nothing about how asgard-cli is distributed can install them. A tar.gz, a zip and
`go install` carry no dependency metadata.

So the binary handles it. Every command that needs helm resolves it through
`internal/tool` first and refuses with the install line for the machine it is on,
and `asgard-cli doctor` reports all of them at once:

```
$ asgard-cli doctor
platform  darwin/arm64

MISSING helm               render a chart (asgard-cli render) and lint it
ok    kubectl              v1.35.1
ok    python3 (optional)   Python 3.14.7

helm is not on PATH ... to render a chart (asgard-cli render) and lint it

Install it with:

    brew install helm
```

It works out the command for the machine it runs on, including which Linux
distribution: neither kubectl nor helm is in the Debian or Ubuntu default
repositories, so the answer there is not an `apt install`. It exits non-zero
when a required tool is missing, so it works as a CI preflight.

`init`, `project`, `request`, `task`, `question`, `decision`
and `check` need none of these tools. `render`, `verify` and the chart
steps of `gate` need helm. kubectl is optional: nothing in this binary talks
to a cluster, and `doctor` lists it because a person debugging a deployment
still wants to know whether it is there.

### The files

Six files, each answering a different question.

| file | who writes it | who reads it | committed |
|---|---|---|---|
| `.asgard-pipeline.yaml` | a person | the platform, on every run | yes |
| `.asgard-cli.yaml` | `asgard-cli` | `asgard-cli` only | yes |
| `.asgard-scaffold.json` | `asgard-cli init` | `asgard-cli` only | yes |
| `.agents/skills/.asgard-docs.json` | `asgard-cli skill update` | `asgard-cli` only | yes |
| `os.UserConfigDir()/asgard-cli/credentials.json` | `asgard-cli login` | `asgard-cli` | never |
| `os.UserConfigDir()/asgard-cli/profiles.json` | `asgard-cli profile set` | `asgard-cli` | never (but it can be handed to a colleague) |

`.asgard-pipeline.yaml` is the declaration, and the only file a deployment
depends on: which releases exist, which chart each deploys, what triggers it,
which keys it takes. See `internal/pipelineconfig`.

`.asgard-cli.yaml` is the binding: which workspace and which pipeline this
checkout acts on, and nothing else. Both fields are required and neither is
derived. See `internal/binding`, whose package comment explains why it lives
beside the declaration rather than at the repository root.

The two records answer the same question about different halves of the
material, and neither is a version check. `.asgard-scaffold.json` says which
version of this binary wrote each of the files this binary ships - AGENTS.md and
the design-time skills - and what it wrote, so a difference can be reported as
`behind`, `edited`, `ahead` or `retired`. See `internal/scaffold`.
`.asgard-docs.json` is the platform's side: which version of the fetched
reference material is here, and each upstream's digest as it was when it was
fetched. See `internal/skills`. The two version numbers are unrelated, and so
are the commands that move them.

`profiles.json` names a platform this binary does not have compiled in -
an on-prem deployment, or a stack running locally. It holds no secret and it is
optional: with no file, every profile is the hosted platform. See
[`profile`](#profile).

`credentials.json` is the one secret this CLI keeps outside a repository:
0600, one file for every profile, and nothing beside it. The earlier
`config.json` there, holding a default profile, a default workspace per profile
and a map of custom profiles, is gone. Every field in it was a preference some
flag or environment variable already expressed. A credential has to live there
because it is a secret, it is per-person rather than per-repository, and it
cannot be re-derived.

A leftover `config.json` is an error, not a warning. The retired
`defaultProfile` was usually `dev`, so ignoring the file silently would move
every command to `prod` - a customer's platform. The first command that resolves
a profile refuses instead, names each retired key and what replaces it, and says
to delete the file.

Deliberately not stored anywhere: which projects the repository has (read off
the declaration's chart paths and `projects/*/`), the customer's display name
(asked of the platform when a template needs it), and what a chart is "meant to
be" (a claim about intent no tool can check). The rule: *a value belongs in a
config file only when nothing on disk implies it and the platform cannot be
asked.* See `docs/decisions/2026-09-05-asgard-cli-config-surface.md` in
`asgard-odin-pm`.

### `login`, `logout`, `whoami`

Sign in to the Asgard platform, so that the `pipeline` commands can act as you.

```bash
asgard-cli login                     # sign in to the hosted platform
asgard-cli login --profile onprem    # sign in to one you configured
asgard-cli login --no-browser        # print the URL instead of opening one
asgard-cli whoami                    # ask the platform who the session is
asgard-cli logout --all              # forget every stored session
```

OAuth 2.0 authorization code with PKCE over a loopback redirect, which is what
RFC 8252 asks for on a machine that has a browser. The binary ships no client
secret. The session is stored under this user account - never inside a customer
repository - at `os.UserConfigDir()/asgard-cli/`, 0600.

A profile is one Asgard installation. `--profile` picks per command and
`ASGARD_PROFILE` sets it for a shell; with neither it is `default`, which is the
hosted platform. Nothing records a current profile, and `login --set-default`
no longer does: a preference stored on one machine is not shared by two people
running the same command.

Profiles other than the hosted one are written with
[`asgard-cli profile`](#profile). `ASGARD_PLATFORM_API`, `ASGARD_ISSUER` and
`ASGARD_CLIENT_ID` still override one field at a time on top of whichever
profile applies, for a one-off.

`ASGARD_PLATFORM_API` is named for the service, not for "the API". This tool
talks to one Asgard service today and is expected to grow into others - the
Control Center API is the next one - so the general name is not given to the
first. `ASGARD_ISSUER`, `ASGARD_CLIENT_ID` and `ASGARD_TOKEN` stay general on
purpose: every Asgard service authenticates against the same Casdoor and
accepts the same token, so those three apply to all of them. It was called
`ASGARD_API` in v0.1.0; a shell that still exports that name is refused with the
rename rather than ignored, because ignoring it would send every command to the
built-in prod URL.

With no browser - CI, a container, an agent sandbox - set `ASGARD_TOKEN` to an
access token instead. It bypasses the store completely, reading nothing from
disk and writing nothing to it.

### The Workbench assistant's sandbox

The platform's Workbench assistant runs this CLI inside a sandbox, as the member
talking to it. The image sets `ASGARD_SANDBOX_MODE=true`, and then:

- The identity is the session file the platform writes at the start of every
  turn (`/tmp/.asgard/session.json`, 0600; `ASGARD_SESSION_FILE` moves it): the
  member's access token, the platform's address, and the workspace of the
  conversation. There is nothing to `login` to, and `login` says so.
  `ASGARD_TOKEN` still wins, and `--workspace` and `ASGARD_WORKSPACE` still come
  before the session's workspace, which comes before a checkout's binding.
- Every request carries `X-Asgard-Via-Assistant: true`, reads included.
- Every change is stamped for the page. With
  `ASGARD_CLI_SIDE_EFFECT_TIMESTAMP_FILE` set (the image sets
  `/work/.asgard/side-effect-at`), each call that changed something on the
  platform - an issue opened, a comment, a pipeline or release written, a Run
  approved, a connection that appeared - rewrites that file with
  `{"at":"<RFC 3339 UTC>"}`, so the Workbench page watching it refetches. Reads,
  refusals and the repository tokens git asks for are not stamped. The
  directory is created when missing, and a stamp that cannot be written is a
  warning, never a failed command. Only `at` is promised; a reader ignores keys
  it does not know.
- git goes through the workspace's GitHub Connection:
  `asgard-cli pipeline git-auth` makes this CLI git's only github.com credential
  helper, and each fetch or push gets a token for exactly that repository,
  signed by the GitHub App and never written to disk. Pushing needs workspace
  administration; `asgard-cli pipeline repo create` makes a new repository
  under an organization's connection.
- `init` refuses `/work` itself, which holds every repository side by side.
- Nothing opens the desktop's browser, because there is none. A page on the
  internet - GitHub's install and authorize pages - is printed as a link for the
  member's own browser: `pipeline connect --account <login>` prints it and ends,
  and `pipeline connect --continue` waits for the connection. The account (the
  organisation or user the app is installed on, not the person authorizing) is
  the member's to name: without one, and without an origin remote to take it
  from, connect starts nothing and says to ask; `--continue --account <login>`
  supplies it later on the same authorization, and an account that does not have
  the app yet leads to its install page. A page only the sandbox
  can reach - `local-env`'s form on 127.0.0.1 - is opened in the sandbox's browser
  over CDP, in the tab the member sees; the agent hands it over with
  `open_sandbox_browser`, and `local-env --wait` waits for the save, a few minutes
  per call. The form server runs in the background between the two.

### `profile`

If you use the hosted Asgard platform, you need none of this. With no file
at all, every command reaches it - that is what `default` means. These commands
exist for the two installations this binary cannot know about: an on-prem
deployment, and a stack running locally.

```bash
asgard-cli profile list              # what is configured, and what applies now
asgard-cli profile show [name]       # each value, and where it came from
asgard-cli profile set onprem --platform-api https://asgard.acme.internal \
    --issuer https://iam.acme.internal --client-id abc123
asgard-cli profile remove onprem
```

Each of a profile's values falls back on its own to the hosted platform's:

| | |
|---|---|
| `--platform-api` | where the Asgard Platform API is |
| `--issuer` | the Casdoor that issues tokens for it |
| `--client-id` | the application this CLI presents itself as (not a secret - it is disclosed to the browser on every sign-in) |

An on-prem installation sets all three. Its API and the Casdoor that issues
tokens for it are the same deployment, and a token from one is not accepted by
the other, so setting the API alone signs you in against the hosted Casdoor and
presents that token to somebody else's server. `profile show` prints where every
value came from and warns when the two disagree:

```
profile        onprem
platform api   https://asgard.acme.internal    profiles.json
issuer         https://iam.asgard-ai.com       the hosted platform (this profile does not set it)
client id      r21ntx0eb5igyokl3px4            the hosted platform (this profile does not set it)

WARNING: profile "onprem" takes its Platform API from profiles.json and its
identity provider from the hosted platform ...
```

It warns rather than refuses, because a local Platform API against a real
Casdoor is a legitimate way to develop.

There is no `profile use`. A recorded current profile is invisible on the
machine that has it and absent on every other, so it is not coming back from
the retired `config.json`. The profile in effect is `--profile`,
`ASGARD_PROFILE`, or `default`.

`profile set` is the only command that creates `profiles.json`, and only when
run. Nothing writes it as a side effect. It holds no secret, so it can be handed
to a colleague setting up the same installation; credentials are a separate file
and cannot.

#### Working against our own development platform

`dev` is not a built-in name. Our development platform is one installation like
any other, and compiling it in would put an internal endpoint in every
customer's binary.

Write it like any other, with the values from the internal setup notes. They are
not in this repository:

```bash
asgard-cli profile set dev \
    --issuer       <internal>  \
    --client-id    <internal>  \
    --platform-api <internal>
asgard-cli login --profile dev
export ASGARD_PROFILE=dev        # for a shell
```

### `workspace`, `pipeline use`

Which workspace and which pipeline this checkout acts on - the two facts nothing
in the repository implies and the platform cannot be asked on your behalf.

```bash
asgard-cli workspace list            # what this account can reach
asgard-cli workspace use <id>        # record the workspace for this checkout
asgard-cli pipeline list             # what that workspace has
asgard-cli pipeline use <id>         # record the pipeline
asgard-cli workspace show            # which apply here, and why
```

Both land in `.asgard-cli.yaml`, beside the declaration they belong to, and
that file is committed, so whoever clones the repository, and whatever agent
works in it, needs no flags. The platform never reads it.

Nothing is inferred, including from a list of one. A command with nothing
recorded lists the candidates and refuses. A rule that picks the only entry
would silently pick something nobody chose once the list holds two.

No git remote is read as identity. A repository may have any number of
remotes, and which one is called `origin` is its owner's choice. The remaining
gap - a repository copied wholesale into another repository of the *same*
workspace keeps a pipeline id that still resolves - is stated in
`.asgard-cli.yaml`'s own header. Run `pipeline use` after copying a repository.

`workspace use` clears the pipeline line when the workspace changes, because a
pipeline belongs to one workspace. Every pipeline command then refuses and names
the remedy, and `asgard-cli gate`'s `binding` step goes red, so the failure
shows on the next command rather than on a later destructive one. Commit both
lines together.

Resolution order, highest first: `--workspace`, `ASGARD_WORKSPACE`, the
checkout's `.asgard-cli.yaml`. That is the whole list. `workspace show` reports
why that workspace applies, so you can catch acting in the wrong one.

### `pipeline`

Deploy the repository through the platform's IaC pipeline.

```bash
asgard-cli pipeline connect                     # connect GitHub to this workspace
asgard-cli pipeline connections                 # the installations connected
asgard-cli pipeline repos                       # what one can reach
asgard-cli pipeline create --name <name>        # bind this repository
asgard-cli pipeline show                        # the pipeline bound to this checkout
asgard-cli pipeline projects                    # projects a release can deploy into
asgard-cli pipeline release create <name> --project <id>
asgard-cli pipeline release update <name> --auto-apply    # the one create-time setting that moves
asgard-cli pipeline releases                    # created releases, and the ghost rows
asgard-cli pipeline variables list --release <name>
asgard-cli pipeline variables set --release <name> --kind secret <key> --from-file <path>
asgard-cli pipeline variables set --release <name> --kind secret <key> --random    # a secret nobody issues
asgard-cli pipeline runs watch --release <name> --ref <tag>
asgard-cli pipeline runs approve <run-id>
```

These hold no rules of their own. Whether a change is deployable is the
platform's answer: it renders the chart, checks every rendered CR against the
cluster's own CRDs with a server-side dry run, and reports back. That cannot be
reproduced here, because no cluster credential is ever issued to a client, so
the loop is: change the chart, check what can be checked locally with
`asgard-cli gate`, push, and read the plan back with `runs watch`. Do not run
`helm lint` by hand: a bare lint has no reserved asgard values file and fails
on every chart that labels anything.

Which release a command acts on comes from the name the declaration uses. Which
workspace, and which pipeline when a repository carries more than one, come from
`.asgard-cli.yaml` beside the declaration - written by `workspace use` and
`pipeline create`, and committed, so a clone and an agent both inherit it.

The platform never reads that file. A run reads the declaration at the
pipeline's config path and the chart it names, and nothing else, so nothing in
`.asgard-cli.yaml` can make a deployment succeed or fail. It exists so the
commands need no `--workspace`, and so an agent landing in a fresh clone can see
what the checkout is pointed at without a call. `--workspace` and
`ASGARD_WORKSPACE` outrank it, which is the safe direction: acting on a test
workspace when the customer's was meant costs a confusing error, and the reverse
deploys to a customer.

A secret's value can only be given with `--from-file` (or `--from-file -` for
standard input): a value typed as an argument is in the shell history and in the
process list. Files are read verbatim, so a PEM keeps its newlines.

### `workbench`

Read and write the issues on the workspace's Workbench - the tracker the FDE and
the customer work through on the platform - and file their attachments.

```bash
asgard-cli workbench list --status in_progress --label blocked
asgard-cli workbench show ISS-12
asgard-cli workbench create --type question --title "<what has to be answered>"
asgard-cli workbench update ISS-12 --status in_review --add-label data-source
asgard-cli workbench comment ISS-12 --body-file draft.md
asgard-cli workbench attach ISS-12 minutes.pdf --what "<what it is>" --from "<a role>" --dated <YYYY-MM-DD>
asgard-cli workbench pull --pipeline <name>         # attachments into references/
```

Every write is made as the member's assistant: it carries
`X-Asgard-Via-Assistant: true`, so the platform authorizes it as the signed-in
account and the timeline says "via Asgard AI". What the platform keeps for the
member alone - pinning, locking, labels, deleting an issue, an attachment or a
comment - this command does not offer, and the platform refuses it from an
assistant.

This is not `question`, `request` or `task`, and not `issue-report`. Which
of the three places a thing goes is decided by who has to act on it:

| who has to act on it | where | command |
|---|---|---|
| the customer's side has to see, answer, supply or decide it - or something is wrong in what is live | the workspace's Workbench (the customer reads it) | `workbench create` |
| whoever builds next needs it: the spec, the design, an open decision | the customer repository's records | `question add`, `request add`, `task add` |
| the makers of this tool, or of the platform behind it | upstream, a feedback only the maintainers read | `issue-report --new`, then `--send` |

A failure of this tool is never a Workbench `bug` - that type is what is wrong
in the customer's deployment, and the customer reads it. Nothing about the
customer goes upstream, because a report leaves the engagement for a third
party's service. In the Workbench
assistant's sandbox, what the member has to keep track of goes on the
Workbench, and a gap in the tool is still reported with `issue-report`.
The same table is in `asgard-cli workbench --help` and in the `AGENTS.md` that
`init` writes.

`pull` files each attachment byte-identical under
`references/workbench/ISS-<n>/<attachment id>/`, checks its SHA-256 against the
platform's record, and writes what, from and dated into `references/_index.md`,
the same row `reference add` writes. It never overwrites a filed copy.

### `audit-log`

The workspace's audit log, as Asgard Console's Explore records it, read with
your own session - so Console decides who may read it (a workspace owner or a
platform admin).

```bash
asgard-cli audit-log summary --days 7    # counted by event, account, project, agent
asgard-cli audit-log query --days 1      # the events, as JSON Lines
asgard-cli audit-log dictionary          # the names behind the raw keys
```

Rows carry raw keys only, there is no success/failure dimension (events are
named), and the data lags by up to about 20 minutes.

### `operate`

Run and read back the CRs a release has already put on the cluster. Everything
above ends when the CRs are deployed; what is left is their behaviour, which a
chart cannot say and a succeeded pipeline run does not report.

```bash
asgard-cli operate syncer sync <name> --release <r> --wait 10m   # run it now, and wait for the outcome
asgard-cli operate syncer executions <name> --release <r>        # what its runs did
asgard-cli operate skill-set sync <name> --release <r>           # a SkillSet's Syncer, through the SkillSet
asgard-cli operate skill-set executions <name> --project <x>     # outside a checkout
asgard-cli operate trigger fire <name> --release <r> --wait 5m    # a Trigger's invocation, now
asgard-cli operate trigger runs <name> --release <r>              # its invocations, and the agent's verdict
asgard-cli operate trigger logs <name> <invocation> --release <r>
asgard-cli operate source-set reindex <name> --release <r>        # a context index refresh, now
asgard-cli operate source-set index-runs <name> --release <r>     # its refreshes; index-logs reads one
asgard-cli operate oauth-credential authorize <name> --release <r> # the grant a chart cannot make
asgard-cli operate oauth-credential status <name> --release <r> --wait 10m
```

A Trigger's invocation and a context index's refresh are conversations with an
agent, and an agent that stops to ask a question ends its invocation as
succeeded. `runs` and `index-runs` print the agent's own verdict beside the
status for that reason.

Only what IaC cannot do is here. Nothing under `operate` edits, publishes,
suspends or lists a CR: those belong to the chart, and `pipeline manifest`
already reads back what a release has.

A CR is named by its rendered `metadata.name`. `--release` checks the name
against what that release deployed and supplies the platform Project the routes
are scoped by; `--project` takes an id, name or namespace for use without a
checkout. The project id is not the namespace, which is why one of the two is
required and neither is guessed.

## Releasing

Releases are driven by [GoReleaser](https://goreleaser.com). Pushing a tag triggers
`.github/workflows/release.yml`:

```bash
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

One run produces binaries for linux / darwin / windows x amd64 / arm64, `.deb` /
`.rpm` / `.apk` packages and checksums, all attached to the GitHub Release. The
changelog is grouped automatically from conventional commit messages (`feat:`,
`fix:`).

To verify locally without publishing anything (output lands in `dist/`):

```bash
goreleaser check
goreleaser release --snapshot --clean --skip=publish
```

Other things worth knowing:

- CGO: builds run with `CGO_ENABLED=0` so cross-compilation fits on a single
  runner. Pulling in a cgo dependency (sqlite and friends) means switching to
  zig cc or per-platform runners.
- macOS signing: the binaries are unsigned. That works because the install path
  is a CLI download: `gh` and `curl` do not set the `com.apple.quarantine`
  attribute that makes Gatekeeper refuse an unsigned binary; a browser does.
  Handing somebody a release URL to click is the case that breaks, and
  `anchore/quill` is the answer if that becomes the normal way in.
- main builds a release, a pull request does not. `ci.yml`'s `build` job
  runs `goreleaser release --snapshot --clean --skip=publish` on a push to
  `main`, so a config or cross-compilation break is caught before it is a failed
  tag. It is off on a pull request because it takes three minutes to answer what
  `test` answers in forty seconds - whether the code compiles - and the part it
  alone checks cannot break on the merge commit without having been broken on
  the branch. `make snapshot` is the same build on a laptop; run it when a
  change touches `.goreleaser.yaml`.
