# STRUCTURE.md

What every directory in this repo is for. What the tool is for is
[Goal.md](Goal.md), how the main capabilities are implemented is
[APPROACH.md](APPROACH.md), what the commands do is [README.md](README.md), the
rules for changing them are [AGENTS.md](AGENTS.md), and what is still to do is
[TASK.md](TASK.md).

## The shape in one line

A single Go binary that answers questions about integrating with Asgard, and
writes chart skeletons into somebody else's repository. It holds no state:
everything an engagement knows ends up in the customer's repo, because that repo
is what the next agent opens.

The question-answering half works with no repository at all and has to keep
doing so, because the question gets asked in a meeting, before there is a
directory. `Goal.md` states the goal.

```
CLAUDE.md             @AGENTS.md, so the rules load without being asked for
.agents/skills/       this repo's own maintenance skills, not the ones that ship
cmd/asgard-cli/       main; signal handling and exit codes only
internal/             every package, none exported
hack/                 the maintainer's gate, in Go: `go run ./hack pass`
.github/             CI, the tag-driven release, and the PR template
.goreleaser.yaml      how the binary is built and published
install.sh            the one-command macOS and Linux install; it is piped
                      into a shell, so it verifies what it downloads
install.ps1           the same for Windows, and it makes the same decisions:
                      verify the download, install where the user owns the files
Makefile              the commands this repo is worked with; `make help`
```

## `internal/` - the code

Largest first. The top of the table is the command surface and the records it
keeps; below it are the checks, the material servers and the plumbing. There
are no line counts, because they go stale with every change.

| package | what it holds |
|---|---|
| `cli` | the cobra command tree, one file per subcommand, plus `root.go`, `repo.go`, `format.go` and `audit.go` (which spans every corpus rather than serving one) |
| `gate` | the invariant checks on a rendered chart - xref, agent split, deployability, enums, constraints, conditional CEL shapes |
| `scaffold` | writes the non-customer-specific tree, serves the design-time skills inside it, writes the platform corpus under `.agents/skills/asgard-platform/`, and keeps `.asgard-scaffold.json` - the record of which CLI wrote the files this binary ships |
| `auth` | the OAuth 2.0 + PKCE sign-in and the credential store, which is the only file this CLI keeps outside a repository |
| `selfupdate` | whether a newer release is published, cached beside the profiles, and the download-verify-run-rename that replaces this binary with it. It never writes into a repository - the answer is about the install rather than about an engagement |
| `work` | the customer repo's own records: requests, task specs, open questions and decisions |
| `check` | repository structure: indexes, dated names, links, orphan pages |
| `platform` | the platform API client: workspaces, the whole `/v1/iac` surface, and `/v1/docs` |
| `generate` | CR skeletons, wired to what the chart already declares |
| `localenv` | the local environment file a chart's placeholders are filled from |
| `kb` | one implementation of listing, reading, provenance and the link graph, shared by every corpus |
| `stage` | the onboarding guidance: a static half that lands as files, and the half rendered against this repository |
| `size` | the deployment shapes, counted off production, and what one costs before anything is added |
| `brief` | what one activity gets wrong, addressed by intent rather than by position |
| `tool` | resolves helm/kubectl/python3 and says how to install one |
| `skills` | the platform's fetched reference material, and the record of which version is here. Not the design-time skills - those are `scaffold/templates/.agents/skills/`, and the two are different halves that land in the same directory |
| `render` | renders via `helm template`, with the reserved `asgard` block supplied as placeholders |
| `binding` | reads and writes `.asgard-cli.yaml`, the checkout's platform binding |
| `gitrepo` | the checkout's root and its remotes, read and never compared to anything |
| `needs` | what a shape has to be given by the customer, written into a repository beside the extracts |
| `repo` | what a customer repository is made of, by looking at it |
| `pipelineconfig` | reads `.asgard-pipeline.yaml`, the deployment declaration |
| `chart` | reads a project's unrendered templates for (kind, name) |
| `wiki` | serves the platform wiki, and the tables in `aliases.md` beside it |
| `version` | build information, injected by GoReleaser via ldflags |
| `browser` | opens a URL, or says it could not |
| `usecase` | serves the deployment-shape extracts |
| `corpus` | the material itself, in the layout a repository receives it: `wiki/` and `usecase/` side by side, so a pointer can become a path that resolves in both trees |
| `selfsrc` (module root) | this repository's own Go source, embedded so the binary can audit the commands it prints. At the root because `go:embed` only reaches downward |

A value belongs in a config file only when nothing on disk implies it and the
platform cannot be asked.

Nothing decides when a chart is finished. The files cannot say which shape a
chart is meant to be: a SemanticLayer with nothing mounted on it is either a
finished Mimir deliverable or an agent nobody has written yet, and those are
identical on disk. There is deliberately nowhere to record which one it is,
because that record would be a note of intent this tool cannot check.
`asgard-cli size` counts the shapes off deployments in production, for a person
to compare against.

To add a subcommand: write `newXxxCmd()` in `internal/cli/` and register it
through `addTo(cmd, group..., ...)` in `root.go`. The group is required - cobra
panics on a `GroupID` the parent does not have - so a command cannot be added
without deciding where in the help it belongs.

### Why `chart` reads unrendered templates

`generate` has to know what a chart already declares before it writes into it:
whether a SemanticLayer exists to mount, whether a Toolset was already emitted.
That has to work without helm, before values are filled in, so `chart` parses the
templates as text rather than rendering them.

## `internal/` - the embedded material

Most of this repo's value is material rather than code. It is compiled into the
binary, and the first question when adding anything is which part it belongs to.

Most of these are one corpus: grep reaches them together and they share one
schema, a `# ` title, a summary, and `**Checked:**` / `**Unchecked:**`.
`asgard-cli audit-material --unverified` is the check, and it reports nothing
unmarked in any of them. `generate/templates/` is the exception and is not
searched: it is what `add` writes, not something anybody reads to decide.

| where | answers | language | searched |
|---|---|---|---|
| `corpus/wiki/` | what the platform is, and who each piece is for | English | yes |
| `corpus/usecase/` | how one shape of deployment is assembled, field by field | English | yes |
| `stage/prompts/` | what to weigh at one point in the work | English | yes |
| `scaffold/templates/.agents/skills/` | what the agent in a customer repo loads to do one kind of work | mixed | yes |
| `.agents/skills/asgard-platform/` | the wiki and the extracts written out so an agent can grep them, with a generated `index.md` mapping both halves as paths; from `scaffold/corpus.go`, not a template | md | yes |
| `scaffold/templates/` | the part of a customer repo that is the same every time | mixed | the skills only |
| `generate/templates/` | the CR skeletons `asgard-cli add` writes | English | no |

They are compiled into the binary and also written into a customer repository
by `asgard-cli init`, under `.agents/skills/asgard-platform/`. That copy is
generated and replaced when the binary's version moves; edit the originals here.

Each fact has one home and everywhere else links to it. A trap belonging to a
CR template is not also explained in a wiki page.

No images. The platform's screenshots belong to the product documentation and
are fetched by URL; `corpus/wiki/screenshots.md` indexes which one answers which
question. Carrying the files here would create a copy that goes stale against
asgard-docs while looking current. Text is embedded because the judgement is in
the text; for assets that reasoning does not apply.

### `stage/prompts/`

One file per piece of guidance. Only the filenames are numbered. The numbers
give the order the guidance is usually reached in and keep the files sorted;
they do not mark a position an engagement is at.

No command selects guidance automatically. An onboarding is not linear, so a
command that derived one stage from the earliest missing CR kind told an
engagement working in a different order that it was behind, and could name
only one thing at a time. Guidance is reached by name with `asgard-cli guide`,
or by grepping `guide/` for the subject.

The prompts are Go templates with `<< >>` delimiters, rendered against the
repository's state, so a prompt can name the actual projects and requests rather
than placeholders. The half that needs no repository lands as a file; a
paragraph still carrying a template action after substitution is dropped from
what lands, because a file would freeze one moment of this repository's state.

### The index, and why it is not a page

`corpus/wiki/index.md` and `corpus/aliases.md` are the corpus's own
bookkeeping, readable by name and absent from any list of pages.

An index inside a searched corpus competes with what it points at. The alias
table lists every alias, so it carries every term of any translated query and
is reliably the document matching all of them; a query for a Chinese term
returned the word list rather than the page about it. So it sits beside the
pages rather than among them.

`aliases.md` is applied to a query before searching, so the question can be
asked in the customer's own words. Its tables behave differently: an alias
replaces the word, because a Chinese term appears nowhere in an English corpus
and keeping it only adds a term that lands nowhere; an entity (a marketplace, a
product) is added to the query, because the name may be written verbatim in a
page and replacing it would throw away the best answer.

Rows come from searches that came back empty in a real engagement, not from
guesses. `asgard-cli issue-report --new` is how those gaps are reported.

### The link graph

Every document carries `kb.Doc.Links`, read when the document is parsed rather
than when a reader is printed one. Resolving pointers at the point of use would
need a second regular expression that can disagree with the first about what a
document points at.

Holding the graph as data makes `--orphans` possible: it reports what nothing
points at. A dead pointer gets noticed; a document nothing points at does not,
and costs more. The index does not count as a pointer there, because a page can
sit in it under a title nobody recognises while somebody spends a day on its
subject.

### `corpus/wiki/` and `corpus/usecase/`

Both are reference material and they answer different questions. The reading
order is wiki first: an extract assumes you already know the platform has that
shape.

`corpus/wiki/README.md` is the schema: the three layers, what a page must
carry, and how it is kept from going stale. `corpus/wiki/index.md` is
bookkeeping rather than a page about the platform, so it is readable by name
and not listed.

`corpus/wiki/glossary.md` carries the table of words with two senses here. It
is on the page rather than in Go so that somebody reading the glossary can see
and extend it, and so `audit-material --links` resolves the pointers its rows
carry.

Every wiki page ends with two things: a source block linking the rendered page
on docs.asgard-ai.com plus the commit it was read at, and an `**Unchecked:**`
line saying which parts were never checked against a deployment. The first makes
"upstream moved and this page did not" detectable; the second tells a reader
the wiki is not checked as deeply as the extracts.


### `generate/templates/` and `scaffold/templates/`

Both use `<< >>` delimiters so that Helm's own `{{ }}` survives into the output.

`scaffold/templates/` mirrors the generated repo one-for-one. Path segments in
capitals are placeholders expanded at write time:

```
projects/__PROJECT__/chart/values-__ENV__.yaml.tmpl
docs/spec/__SPEC_SLUG__/README.md.tmpl
```

A `.tmpl` suffix means the file is rendered; anything else is copied verbatim.
`.agents/skills/` under it holds the design-time skills the coding agent
in the customer repo loads.

What ships here is decided by authority, not subject. It is what is true of
any Asgard, whoever is running it: the repository skeleton, the docs layers,
the declaration template, how to write plain Chinese, how to model a semantic
layer from a customer's own database. A skill stating what a particular server
accepts, rejects or calls things (the CRD shapes, the processor catalogue) is
served from the platform by `asgard-cli skill update` instead, because a
customer's server can be several versions from whichever release they installed
this from. See the embed comment in `scaffold/scaffold.go`.

`scaffold/skills.go` reads them as a `kb.Corpus`, so they carry links and
provenance like every other body of material and the same audits reach them.
They land in the same directory as the platform corpus but are a different
half: these say how to work, the corpus says what the platform is.

## `hack/` - the maintainer's gate

Go, one binary with a subcommand each, and `hack/internal/src` for where the
upstream clones are. AGENTS.md has why it is compiled rather than scripted.

    go run ./hack pass     every check, in the order to run them
    go run ./hack list     what each one is for, and what it needs

What each one does is not restated here: the descriptions live in the checks'
own registrations, beside the code, and `list` prints them. This section says
why the directory exists.

`helm lint`, `asgard-cli check` and a server-side dry-run all pass a document
the apiserver would reject or silently prune. The checks that would catch it
need a clone of somebody else's repository, which is why they are here rather
than in the binary, and why CI runs only the ones that need nothing but this
repository. `hack/README.md` is the procedure, and the PR template asks for its
output.

Nothing here clones or pulls. A check that fetched would turn "read at this
commit" into "read at whatever was there when it ran", which the provenance
rule exists to prevent. `.env.example` is the template for the environment
variables. Do not name it `.env.template`: that would be gitignored, because
the rule is `.env.*` with `!.env.example` carved out.

`.agents/skills/consistency-checks/` is this repository's own maintenance
skill: how to run a consistency pass, what each check is blind to, and how to
hold prose against the upstream it came from. It is not one of the design-time
skills, which are `scaffold/templates/.agents/skills/` and land in a customer
repository. This one never leaves here, and `selfsrc` embeds it so the audits
read it. It is also unrelated to `.agents/skills/db-query/scripts/`, which
`asgard-cli init` writes into a customer repo as the tool-chain for reading the
customer's own source systems at design time.

## Reference material that is not in this repo

Read-only, never vendored in. The URL is the source of truth, not the local
clone path, so the clone location is an environment variable rather than a path
in a script: `ASGARD_KUBE`, `ASGARD_DOCS`, `ASGARD_CORE`.
`go run ./hack sources` prints what each resolves to and how far behind it is.

| what | source of truth |
|---|---|
| CRD definitions, the platform contract | https://github.com/asgard-ai-platform/asgard-kube |
| product documentation | https://github.com/asgard-ai-platform/asgard-docs |
| the processor definitions the CRD is generated from | https://github.com/asgard-ai-platform/asgard-core (private) |

A copy taken into this repo stops tracking upstream and then looks like a
current one. Record the commit you read instead.

## What is not here

- Few tests, placed wherever a wrong answer would be silent. In the packages
  that is parsing and matching: a credential reference, an environment name, a
  pipeline manifest, a provenance marker. In `hack/` it is the checks whose own
  logic decides what a reader is sent back to, because a digest taken over the
  wrong bytes reports a pass that looks real. Prose and material are covered by
  `audit-material` instead, which reads what ships rather than a copy of it.
  `go test ./...` runs in CI alongside `go vet` and `gofmt -l`; see "The gate"
  in `AGENTS.md`.
- No `.out/` in version control. It is gitignored and holds anything a command
  produces: hand-built binaries, command output, scratch programs, and
  `.out/verified.json`, which records what passed on one machine.
- No customer data anywhere, and no customer's name: not in the material, not in
  provenance, not in this repository's own documents. The customer's own
  knowledge lives in the repo the CLI writes, not in the CLI.
