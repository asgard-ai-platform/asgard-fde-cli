---
description: the split follows the audience, ask against watch, two releases of one chart per environment, the table the interview builds
---
# Decide how the work splits into projects

How the work splits into projects is decided by interviewing the customer,
not by looking at their systems. The output is a decision; no code is written
in this stage.

<<with .InterviewRequests>><<range .>>  <<.ID>>  <<.Title>>
<<end>>
That request has no target project yet, and deciding it is this stage. The output
is a decision; no code is written here.<<else>>No request is open, so this page is being read out of order. This stage is the
interview that decides how the work splits into projects, and it starts from
something a customer asked for: `asgard-cli request add "<what they asked for>"`.

The interview that produces one is `../guide/requirements.md`.<<end>>

A project is one Helm chart deployed to one namespace, and it always lives
under this workspace at `projects/<slug>/` - `asgard-cli project add` puts it
there. There is nothing to decide about where it goes.

The split follows the audience. Two audiences need different entry points and
different read paths, and those cannot be shared:

  internal, authenticated callers   -> the platform's agent hub, semantic layers
  public, anonymous visitors        -> your own BotProvider, fixed query tools

The question that decides the split is who is on the other end, asked once per
capability the customer wants.

A second question decides whether it is a project at all: what do they do with
the answer - ask it in the moment, or watch the same numbers every day? The
second is a Mimir dashboard: a chart of DataConnector and SemanticLayer with
no entry point, no Agent and nothing to publish, and the deliverable after that
is built by the customer in the product rather than by us in a chart.

    ask     -> a project, in the sense this page means
    watch   -> a read surface, and the Views and Dashboards are theirs to make
    both    -> one model, two deliveries. Do not merge them into one estimate

It still lives under `projects/<slug>/` and still deploys to a namespace, so it
is a project mechanically, but it is not shaped like the rest of this page:
`../guide/entry-point.md` and `../guide/knowledge.md` have nothing to say
about it, and `asgard-cli verify` will say `0 agent(s)` and report R11 - the
SemanticLayer no Agent binds. Both are expected here and neither is a failure.
R11 is an observation, because a render cannot tell a layer that is deliberately
unbound from a read path somebody has not finished.
`../usecase/mimir-dashboard.md` is the shape.

A third question is how many environments. It is not about the split, and it is
often missed. A project is one chart, and that chart normally deploys more than
once - `dev` and `prod` at least. Those are one project and one chart, with two
releases in `.asgard-pipeline.yaml` naming the same `chart:` directory, differing
by `on.pattern`, and each created against a different platform project:

    - name: <slug>-dev     pattern '^dev-[0-9]+\.[0-9]+\.[0-9]+$'   -> platform project A
    - name: <slug>-prod    pattern '^[0-9]+\.[0-9]+\.[0-9]+$'       -> platform project B

The platform project decides the namespace, so the two have to be different
ones. That is also why a deployed namespace reads
`asgard-<workspace>-<project>-<env>` and carries the `-<env>` at all. There are
no per-environment values files - what differs between them is the variables set
on each release on the platform.

One release is the shape for a POC nobody will maintain. That is a valid answer;
write it down as a decision. If nobody asks, the problem appears the first time
somebody needs a staging deploy, when the platform project, the namespace and the
release name are already the ones production uses. `asgard-cli gate` warns when a
chart is named by only one release and that release says which environment it
is.

## What to ask the customer

  - Which business systems hold the data an agent would need to read?
    Get the system's name, what it is for, and how it can be reached. The last
    one decides the whole integration, so get a specific answer:

        "Is this one of ours, and can we read its database directly?"
        "Does it have an API? Who has the credentials and the docs?"
        "If neither - is the only way in a person clicking through a screen?"

    Ask it about every system, including the ones that sound obvious. A
    warehouse system that turns out to have a REST API is a different design
    from one that only has a database, and both are different from one that
    only has a web console.
  - For each one: who asks the questions? Employees who log in to something, or
    anonymous visitors on a public page?
  - Is any of the knowledge unstructured - product documents, FAQs, pages on a
    website - rather than rows in a database?
  - Is there anything the agent should be able to change, not just read?
    Every write path needs a spec and human approval, so find out early.
  - Which environments does this have to run in - is there a staging or UAT the
    customer expects to see it in before production, and who signs off there?
    Ask it even when the answer is obviously "just prod". That answer is a
    decision, and the release names and platform projects are cheap to change now
    and expensive later.

## Build this table as you ask

It is the whole output of the interview, and the split follows from it:

| system | what it holds | how to reach it | who asks | what they want | read or write |
|---|---|---|---|---|---|
| ERP | 料件庫存、採購單 | MSSQL, ours | 內部,登入 | 現有量、在途量 | read |
| a marketplace | 該通路的庫存與訂單 | REST API, theirs | 內部,登入 | 跨通路比較 | read + write |
| 官網型錄 | 產品、分類、規格 | PostgreSQL, ours | 匿名訪客 | 產品查詢 | read |

*Composed from several engagements rather than taken from one. Nothing shipped
in this tool describes a particular customer's systems, and that includes worked
examples.*

"How to reach it" has a fixed preference order:

    1. a database we can read      most capable: the agent composes its own
                                   queries and joins across tables
    2. an API                      a fixed set of calls, but a real contract,
                                   reviewable and gateable
    3. a screen a person clicks    last resort: brittle, slow, and it breaks
                                   whenever the vendor changes their UI

Take the highest one available per system, and expect a mix. Ask whether the
customer already has something that has done this consolidation for them - a
middleware layer, an OMS, a warehouse that already pulls the channels in. If they
do, several external systems collapse into one database, and that changes the
design more than any other single answer.

Group the rows by "who asks", not by system. Each distinct audience is a
project, because an audience determines the entry point and the read path and
those cannot be shared. Two systems read by the same audience belong in one
project; one system read by both audiences is read twice, through two shapes.

A row wanting write does not change the split; treat it as a warning. Every write path
needs its own spec and human approval, and the standing architecture is
read-only. Note it and move on - `../usecase/write-path.md` is where the
approval gate is described, when it comes to designing one.

## What to write down

The table goes into the request, section 3 of
`requirements/requests/<<.RequestID>>-*.md`, and the audience you settled
on goes into its Meta as the target project. That file already carries the date
and the status, which is why the interview output belongs there rather than in a
loose note.

Then, in this order:

  1. Anything the interview did not settle, one question each:

         asgard-cli question add "<question>" --blocks <<.RequestID>> --ask "<who>"

     Do it as soon as a question blocks a decision, and do not put it in a task
     spec instead: a task's open questions disappear when it reaches `done`,
     while `asgard-cli question` prints this file.

  2. The split, as a decision record:

         asgard-cli decision add "how the work splits into projects" --module architecture.md

     Fill in why the rejected split was rejected; nobody can reconstruct that
     later. The command stamps the date and links the record from the living
     spec.

  3. The raw discussion, in `docs/meeting-notes/YYYY-MM-DD-<topic>.md`, if it
     was long enough to be worth keeping. Optional: a decision record can cite a
     GitHub issue or a direct instruction instead.

  4. The first module of the living spec, `docs/spec/<<.SpecSlug>>/architecture.md`:
     the table, the audience of each project, and the invariants that hold from
     day one (which systems are read-only, where any write path points). This is
     what the next person reads to understand the system.

     Add it to the module index in `docs/spec/<<.SpecSlug>>/README.md` in the
     same change. `asgard-cli check` compares that index against the files
     actually present, so a module written without being indexed turns the gate
     red.

## Then register each project, and point the request at it

    asgard-cli project add <slug>

Then point the request at it. That is what moves this stage on: until the
request names a project this repository has, `asgard-cli request` lists it as
`no project yet` and says its audience has not been decided.

    asgard-cli request target <<.RequestID>> <slug>
    asgard-cli request ready <<.RequestID>>          once the audience and the scope are settled

Keep the slug short. It becomes part of every release name, and of the
namespace the platform creates for the project - `asgard-<workspace>-<project>-<env>`
in the deployments this was written from. A Kubernetes object name is capped at
63 characters, so everything derived from the slug inherits its length. This
tool does not build either name: the namespace comes back from the platform
and a chart reads it out of the injected `asgard` block.

Done when: projects/ has a directory per project, the root README table lists
them, and asgard-cli check is green.

**Checked:** the shapes it names are real (`asgard-cli size` counts
them off production, the extracts in `../usecase/` assemble each); every command
and flag it writes is in the binary; and the namespace pattern was read off the
namespaces of three production deployments rather than off this tool, which
does not derive it. They are not quoted, because a namespace carries the
customer's workspace name. No CRD claim is made here:
which CRs a project ends up with belongs to
`../guide/read-path.md`, `../guide/entry-point.md` and the extracts.

**Unchecked:** whether `asgard-<workspace>-<project>-<env>` is the platform's
rule or a convention these workspaces share: the Platform API that builds the
namespace is in no repository here, so the three namespaces above are the only
evidence. Also unchecked: the split rule itself - that a project follows the
audience rather than the data or the system - which no source states; a reader
whose customer has one audience and six systems should get one project.
