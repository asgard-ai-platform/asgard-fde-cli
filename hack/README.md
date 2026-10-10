# hack/

This is the maintainer's gate, written in Go: one binary with a subcommand
each, run as `go run ./hack <check>`.

    go run ./hack pass     the whole pass, derived rather than written down
    go run ./hack list     every check, and what each one needs

The checks are compiled so that errors surface at build time. Most of them run
only by hand, so an error in a rarely taken branch would otherwise survive for
weeks; AGENTS.md lists the five that got through while they were scripts, each
of them a compile error in Go. Porting them also corrected a number the Python
version had wrong and was validating a page against: it counted distinct CEL
rules by matching `rule:` with a regular expression over raw YAML, so two
spellings of one rule counted as two.

## Where the upstream clones are

Every check here needs one. Each source is located by one environment variable,
with a default that matches one person's layout. Do not write a local path into
a script or this file; it is true on one machine and wrong on every other.

    go run ./hack sources      what each one resolves to, and how far behind it is
    go run ./hack tables       the pinned gate tables against the CRDs
    go run ./hack coverage     the wiki's coverage row against the docs tree
    go run ./hack processors   wiki/processors.md's three tables against their owners
    go run ./hack counts       wiki/screenshots.md's arithmetic against the docs tree

    ASGARD_KUBE          the CRDs, the platform contract
    ASGARD_DOCS          the product documentation
    ASGARD_CORE          the processor definitions the CRDs come from

Nothing here clones or pulls. A check that fetched would change "read at this
commit" into "read at whatever was there when the script ran", which the
provenance rule forbids. Run `git -C <path> pull` yourself; `go run ./hack sources`
tells you when it is due.

This repository's own tooling. Not shipped, not embedded, and not the same thing
as `.agents/skills/db-query/scripts/`, which `asgard-cli init` writes into a
customer repo - that one reads the customer's own source systems, and is
described in `README.md`.

Everything here answers one question: is what this repo emits still accepted by
the platform contract? `helm lint`, `asgard-cli check` and a server-side
dry-run do not answer it. The dry-run is misleading: it drops a field it does
not recognise and reports success, while helm's own server-side apply refuses.

## Reading every instruction at once

    asgard-cli audit-material [--ask] [--unmarked] [--crossref]

This lives in the binary, not here. It began as a script in this directory and
was moved because the people who follow an instruction and find it wrong have
the binary and not this repository, so they can run it when that happens.

It also reads the embedded material, which is what an engagement gets. A script
over `internal/corpus/wiki/*.md` would audit the source files instead of what
somebody actually read.

Hidden from `--help`, because its reader edits this material and the help output
belongs to whoever is onboarding a customer.

## Two implementations of the .env format, and whether they agree

    go run ./hack/dotenv-agreement

There are two: `asgard-cli local-env` writes the file in Go, and the db-query
scripts read it in python. If they drift apart, the form shows one value and the
query connects with another, so this compares them.

It also checks that a save changes the one value it was asked to change and
nothing else, because the file is also edited by hand and comments in it must
survive. It has caught two ways of losing one: a trailing comment dropped when
its line was rewritten, and a quoted value re-spelled bare on a line nobody had
touched, which also changes its meaning to any shell that sources it.

Needs python3. Without it the python half reports as skipped, not passed.

## Checking a change against the CRDs

Pull asgard-kube first. Validating against a clone from three weeks ago proves
nothing, and it moves without announcing it.

```bash
KUBE=../asgard-kube
git -C $KUBE fetch && git -C $KUBE status -sb        # say so in the PR if behind
mkdir -p .out/crdjson
# `go run ./hack tables` reads the YAML itself; this is only for a JSON dump
for f in $KUBE/crd/*.yaml; do yq -o=json "$f" > .out/crdjson/$(basename $f .yaml).json; done
```

What the templates emit: build a throwaway repository, add every kind, render
both environments, validate each:

```bash
go build -o .out/asgard-cli ./cmd/asgard-cli
# init, scaffold, project add, then one `add <kind>` per kind, then:
asgard-cli render <release> --quiet | yq -o=json -I=0 '.' > .out/dev.ndjson
go run ./hack validate-crs .out/dev.ndjson
```

What the extracts teach: these are what somebody copies by hand, so they are
checked the same way. They are chart fragments rather than parseable YAML, so
they are defused first - Helm actions and `<placeholder>` text become sentinels
the validator knows not to report on:

```bash
go run ./hack extract-crs .out/extracts.ndjson
go run ./hack validate-crs .out/extracts.ndjson
```

Both should print `0 schema violation(s)`. Put the counts and the asgard-kube
commit in the PR body - `.github/pull_request_template.md` asks for them.

## Checking the pinned tables against the CRDs

    go run ./hack tables

`internal/gate` holds three copies of the platform contract, extracted from
asgard-kube's Go types. The contract is the generated CRDs, and they differ
from the Go types. `status` carries three values
in the Asgard types and six in the CRD, because Kubernetes' own condition
schema uses that field name - the wrong three sat in the enum table for a day.

Run it after regenerating a table and whenever asgard-kube moves. A field it
reports as absent from the CRD is not automatically a bug - `baseAgentName`
lives inside a JSON string rather than in the schema - but it is always
something to explain rather than leave.

It also holds every CEL-rule count this repository states, for the same reason: 79 is the `XValidation` markers in the Go
types, 231 is what the generator emits from them, and this material had the
marker count written down as the CRDs' own for a week.

It also holds every required field of a per-class block, which is the set an
FDE asks a customer for. `BotProvider.spec.telegram` requires `webhookSecretToken`
beside `botToken`, no documentation page mentions it, and this material listed
"the Bot Token" - half the ask, and a CR that is refused. Matched across
everything that ships rather than the prose alone, because a field can be
taught by the generator that writes it, and on a word boundary, because a
substring test passes `region` on the word "regional".

It also holds every immutable field. 41 of the enforced rules are `self == oldSelf`,
carried on 40 kind-and-property pairs across twelve kinds. Nothing offline can
tell you a chart will be refused at apply, but which fields are immutable is
computable, and an immutable field nobody has written down is one an FDE meets
after the tag is pushed. So this checks that `wiki/crd-rules.md` names all
eleven class fields, states the Syncer's 21 and the total, and that no immutable
Syncer field is missing from the corpus. Pairs rather than distinct paths:
`bot.botProviderName` is immutable on the Loader and on the Syncer, and those
are two fields somebody can be refused on.

## Recomputing a count that came out of somebody else's document

    go run ./hack counts           against the asgard-docs clone as it stands

A number copied out of somebody else's tree is easy to get wrong and hard to
notice: "88" reads no differently from "93". `wiki/screenshots.md` states how
many images asgard-docs holds, how many a live page uses and how many this
material names, so those are recomputed at the commit the page names, and every
place the page states one has to agree. A claim whose wording no longer matches
any pattern fails, so that a count cannot silently stop being checked.

## Re-walking the processor definitions

    go run ./hack processors            against the clones as they stand
    go run ./hack processors --dump     print what upstream says, and stop

`wiki/processors.md` is the most claim-dense page in the corpus - thirteen
processors, their required keys, their defaults, their outputs, and which keys
an author may set - and every one of those claims belongs to a file in somebody
else's repository. The two tables on it have different owners, and they are
expected to differ:

    the definitions table   asgard-core `internal/constants.go` -
                            what the runtime validates a Workflow against
    the palette table       asgard-docs' per-page `metadata.json` -
                            what the builder lets an author type

So each table is checked against its own owner and never against the other. A
processor appearing or vanishing fails: the page says thirteen in four places.

The literal is walked by brace depth rather than matched by pattern, because a
pattern attributes one processor's fields to the next. Every identifier
must resolve to a string or the script exits: an unresolved one means the
literal grew a shape the walk does not understand, and its output cannot be
trusted.

## What this catches that nothing else does

Required fields with no default (a `Workflow` entry's `tooling.allowUploadFile`
was missing from two extracts, so a reader copying one got a rejected CR), fields
absent from the schema, enums, patterns, `maxItems`, and the `ExactlyOneOf` CEL
rules.
