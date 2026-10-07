---
description: the order to ask in, the answers that looked obvious and were reversed, the two filters before a question becomes a row
---
# Turn what the customer said into a request

This is the interview. It produces a request, which is not a design, a task or
a chart. Read it before the working session with the customer, and keep it
open during one.

## What this stage produces is a file

Everything below is how to think during the interview. This section is what to
do with it. It comes first because the thinking usually goes well and the
recording is what gets skipped. Run three commands as the answers arrive, not
at the end and not before the interview starts:

    asgard-cli request add "<what they asked for, in their words>"
    asgard-cli request target <<.RequestID>> <project>
    asgard-cli question add "<what blocks it>" --blocks <<.RequestID>> --ask "<who>"

Open one request per capability they asked for. A document with three scenarios
is three requests, because two capabilities in one record cannot be given
different target projects, and the target project is the decision this stage
exists to reach.

Do not open a request before the interview. A customer's own document arriving
ahead of the meeting is normal - their internal approval comes before they will
book one - and it is not a reason to open a request. Only section 1 exists at
that point, and it already exists, in `references/`. The other six - audience,
target project, how each system is reached, writes, success, scope - are what
the interview decides, so a request written first is one copied section and six
TODOs. It shows in the index as progress, it cannot pass any of `request
ready`'s checks, and it puts the customer's words in the same file as our
translation, which is the part the request exists to hold.

Before the interview the material has three homes, and they are enough:

    references/              their document, as they wrote it
    docs/open-questions.md   what to ask, and who can answer it
    docs/meeting-notes/      the meeting, and what is taken into it

An empty `requirements/requests/` before the interview is the correct state,
and `asgard-cli check` treats it that way.

This stage usually fails by leaving a good analysis in the conversation: the
material is read, the questions are filtered well, the answer goes to whoever
asked, and the repository ends the day unchanged. The next run of
`asgard-cli request` then correctly reports nothing in flight. Before telling
anyone anything, run `request add`; the message summarises the record and does
not replace it.

If you are answering a question rather than running a meeting - "list the open
questions in this document" - that is still this stage. Write the records, then
answer from them.

You will also need a deck to take into the room. It is the only thing in the
repository the customer reads, and its shape belongs to the `proposal-deck`
skill in `.agents/skills/`, including the deck to build while the questions are
still open. Read that skill before writing slides.

Decide two things here, before opening that skill, because getting either wrong
means work that gets thrown away:

  - Which of the three decks it is - discovery, proposal, or handover. They
    are built from different things and several of the proposal's rules invert
    for a discovery deck. An engagement built a discovery deck under the
    proposal's rules and had to take the titles back out: at interview stage
    nobody has earned a conclusion, so the titles are the customer's own
    section names rather than assertions.
  - Whether it takes images at all. An interview deck usually does not. One
    engagement downloaded four screenshots, cropped two and used none - filling
    every page with the customer's own words leaves no room, and one kind of
    evidence per page then excludes the picture. You can know this before
    starting, so decide it here.

Both are recorded here as well as in the skill because FDEs read this page
first and open the skill only after the first correction.

<<with .Requests>>Open requests:

<<range .>>  <<.ID>>  <<printf "%-8s" (printf "%s" .Status)>>  <<.Title>>
<<end>>
<<else>><<if and .References (not .Questions)>><<.References>> file(s) of customer material are filed in `references/`, and nothing records that it was read. Material
arriving before the meeting is the normal order here - their internal approval
comes before they will book one - so its arriving early is fine. The problem is
that no questions have been read out of it.

Turn it into questions, not into a request. Nearly every section of a
request is what the interview decides, so one written now is a copied section
and a file of TODOs; an empty `requirements/requests/` before the interview is
the correct state.

    asgard-cli question add "<what blocks it>" --ask "<who can answer>"

<<else>>Nothing is recorded yet. After the interview, `asgard-cli request add`.
<<end>><<end>>
## Read the later stages before this meeting, not after

Every other page here is written to be read when you arrive at it. This one is
the exception, because of what it produces:

    everything else produces files  a wrong one is edited, re-rendered, reverted
    this produces speech            a wrong one is in the customer's notes

You get one interview. At that point you have read none of the build guidance,
and half of what you will say out loud is settled there. So read at least
`../guide/data-sources.md`, `../guide/read-path.md`, `../guide/entry-point.md` and `../guide/knowledge.md` first. Reading them
an hour before the meeting costs less than correcting what was said in it.

### Run `../brief/customer-meeting.md` before the meeting

Run it rather than only reading it. It lists what reaches a customer wrong, and
it lives there rather than here because meetings happen at any point: an
engagement halfway through a chart with a meeting tomorrow would never reach a
briefing linked only from this page.
An intuitive answer goes wrong in both directions - some entries undersell the
platform or overstate a limit, and some promise something that cannot be built.
So when a customer asks whether something is possible and the honest-sounding
answer is "no" or "not yet", check before answering. A cautious answer that is
wrong is still wrong to the customer.

## Before writing a single slide

The deck is where these questions actually get written, and the skill that owns
it is long. These are the rules needed before the first page. They are here as
well as there because this text is printed to you and a file is not - an FDE
received every change made to this page and only the parts of that skill a
correction pointed at.

`.agents/skills/proposal-deck/` has the reasoning, the counter-examples and the
review checklist. This is the part you need to start.

End every outcome with what happens when it fails. "It says it cannot find
that number rather than guessing one." That line was written once and never
revised in many rounds, because failure behaviour has one right answer where a
capability has many phrasings. It is the cheapest line on the page and the most
persuasive.

Two sentence shapes are banned outright in Chinese, because they mark the
page as machine-written before anyone reads what it says: `不僅……更是……`, and
any page that ends by announcing its own importance (`為……奠定基礎`,
`具有重要意義`). Test the second by deleting the sentence: if the page loses no
information, the sentence carried none.

`.agents/skills/plain-chinese/` has the rest. It applies to every 繁體中文 thing
this engagement sends a customer, not only to the deck, and it carries one rule
that would damage a deck if applied whole - an essay's ban on bullet lists does
not transfer to slides. The two rules above are repeated here because this text
is printed and a skill file is not. No other rule from that skill is repeated.

Make one page per sub-heading of theirs; their document already decides the
page list. Put two columns on that page: the question on the left, what it
produces on the right. Keep them on one slide; split across two, the reader no
longer sees the trade.

Use their headings as titles, unedited. A page titled with a name of our
invention drew "stop drifting the titles" - it reads as a demand nobody made. If
you split one of their items, the title is still a phrase from their document and
the lead says which item it came from.

A capability's context page contains nothing you wrote - their heading, their
description, their list. Those pages went through many rounds without a single
correction. The number of revisions tracked how much of the page we had
written.

When their document is thin, leave the page thin. The urge is to fill the
space, and the only material to fill it with is ours. A thin page is accurate:
it shows they have not worked that capability out either, and the meeting can
start there.

Ask the question in the meeting and let the customer answer. The consequence is
theirs to state, and supplying it invents branches that do not exist. The shape
it takes:

    有沒有測試環境?
    有沒有測試環境?沒有的話讀正式資料可以嗎     <- added and rejected repeatedly

Each addition rehearses their answer. "No test environment" does not mean no
integration, and writing the second half creates a fork nobody was standing at.

Keep agenda and tracking items off slides. "Who is responsible" is right to ask
aloud and wrong to print. Test each line by asking whether the answer is
somebody we chase or something we build; only the second goes on a slide.

A question that works asks what they do now, offers two concrete
possibilities, and the two answers fork the design. The version in our
vocabulary collects a no:

    no    有沒有 OMS 或電商中台?
    yes   現在要看各平台庫存,是一個一個後台登,還是有一個地方全看得到?

State the condition in the same sentence as the capability.

    no    given an API, we can integrate all five channels
    yes   given a read-only account on the CRM and a route to it, this becomes X

Claim nothing past `../wiki/platform-unknowns.md`. Ask the platform team about
anything on that list before the meeting, and give the answer aloud, never in
print. Do not tell a customer that we do not know what our own product does.

Do not write 「你們」 in a Chinese deck. Drop the subject, or make their document
the subject. Chinese can omit it, so writing it reads as deliberate.

On an anonymous channel the person approving a write is the visitor in the
conversation, not staff. One page described the approval screen as showing
"the customer's name, product and fault description" - the shape of a supervisor
reviewing somebody else's record, when the scenario was that customer, in their
own chat, being asked whether to open the ticket. Check the subject of every
line.

Say what identifies the person, both ways.

    no    if there is no account binding, the customer gives us their
          ticket number and we look it up
    why   ticket numbers are usually sequential. That lets anybody
          enumerate other people's cases, and it is not authentication

A self-service lookup has to name what identifies the caller, and a number they
type is not an answer. A write has to name which field says who the record is
for - the agent fills that one, so it is ours; whose account it sits under is
theirs and is not asked.

Leave out anything only we can resolve. No question numbers, no `REQ-` ids, no
"the five the filters removed" - if a reader with only the deck cannot resolve
the reference, it does not go on.

Borrow the layout skill's visual language, not its authoring process. Keep one
source, the HTML. Run its layout checks, not its content checks.

Never edit a slide to make a checker pass. A content check reported an
eyebrow as missing; it was there, and the fix that turned the check green
removed the sub-numbering from every eyebrow - after which every page claimed the
wrong level. If a check fails on text you can see, the check is wrong.

Write back to `docs/open-questions.md` in the same edit, not afterwards.
Rewording a question, dropping one, finding one - each belongs in the row now.

One engagement deferred it and after many rounds the two had diverged too far
to reconcile. What survived in the file was a security judgement the deck had
already corrected, still argued convincingly. `asgard-cli question` prints that
file before anything else, so the next person reads the superseded version
first. `asgard-cli check` warns when the deck is newer.

## Why the interview is a stage of its own

Of the target repo's task specs, three were superseded and one was
reverted, and they were the three most expensive decisions in the engagement.
The code was right each time. They flipped because the question that decides
them was answered from the design already in mind rather than asked out loud.

All three are decided by the same fact, and it is the cheapest thing to ask for:

    who is on the other end

Ask it once per capability the customer wants, before anything else.

## The order to ask in

Each answer narrows the next question, so asking out of order means designing
against an audience nobody confirmed.

### 0. What have they already read or been told?

One sentence, before describing anything. A customer who read the product site
arrives believing their own staff will build workflows in Odin, carrying a
vocabulary - Basic Function, Template - that maps to nothing in the product as
documented anywhere else, and expecting Mimir to forecast.

`../wiki/what-they-read.md` has the three and what is actually true. Find out
which picture they have first: describing the platform over a different one
produces a customer who nods and disagrees later.

### 1. What can the agent not do today?

In their words, before translation. Write down the sentence they actually said,
including the parts that sound imprecise - "the warehouse people keep phoning to
ask about stock" carries who, where and why, and "a stock query API" carries
none of it. The translation into platform terms is the request's own section 2,
and keeping the original is the only way a later reader can check that the
translation was right.

One request per thing they asked for. Two capabilities in one record cannot be
given different target projects, and the target project is the decision this
whole stage exists to reach.

### 2. Who is on the other end?

    internal, authenticated callers   -> the platform's agent hub, semantic layers
    public, anonymous visitors        -> your own BotProvider, fixed query tools

This decides how they reach it. 2b decides what "it" is, and the two are
asked together.

Settle it now. The two paths share neither an entry point nor a read path, so a request that mixes both audiences is two
requests. Same audience as an existing project means this request goes into that
project; a new audience means a new project.

Ask it concretely. "Do they log in to something today, and is it ours?" gets an
answer; "are they internal users?" gets a yes that means nothing, because a
contractor with a company address is internal to the person answering and
anonymous to the platform.

#### 2b. What do they do with the answer?

Ask it in the same breath as question 2, because it decides which product this
is, and everything from question 3 down assumes the answer.

The rest of this interview is agent-shaped, and so is whoever is running it.
Every question after this one - who is on the other end, which systems hold the
data, what it may write - assumes the deliverable is something you talk to. An
agent asked what to propose therefore proposes an agent, fluently, which makes
the mistake hard to notice. It has cost one proposal: a customer's
cross-channel inventory question, which is exactly what
Mimir is for and is the subject of the product documentation's own case study,
came back as an agent over a semantic layer. Nothing had asked what they do with
the answer.

Some products still have no shape to propose instead - Heimdall, Fehu, the
Management Console as work in its own right, and Knowledge Base as distinct from
a Drive - so if 2b lands on one of those, `../wiki/product-suite.md` is
what to reason from and there is no extract to lean on.

    look one thing up, in the moment     an agent
    watch the same numbers every day     a Dashboard - this is Mimir
    both, for different people           both, and they are separate deliveries

`../wiki/product-suite.md` says Mimir is often what the customer actually
wants. "I want an AI that answers stock questions" is a statement about stock
questions, and the two products answer it differently. Glancing at a figure
each morning is a Dashboard; looking one thing up when a customer is on the
phone is an agent.

Getting this wrong is expensive and nothing flags it: an agent gets built, it
works, and the customer keeps asking the same three questions every morning
because what they needed was a page that was already open. Both read the same
Semantic Model, so the modelling work is not wasted, but the delivery is, and so
is the meeting where it is demonstrated.

Ask it concretely, the way question 2 is asked: "when you have this number, what
happens next - does somebody act on it there and then, or is it something you
check?" A recurring report is a Dashboard, whatever words they used to ask for
it. Anything they want pushed to them - mailed, posted to a group - is a
third answer again, and a schedule cannot run anything needing approval.

### 3. Which systems hold the data, and how can each one be reached?

One row per system, including the ones that sound obvious.

| system | what it holds | how to reach it | who asks | what they want | read or write |
|---|---|---|---|---|---|

"How to reach it" has a preference order, set by cost:

    a database we can read   >   an API   >   a screen a person clicks

Take the highest available per system, and ask about every system rather than
inferring. A warehouse system with a REST API is a different design from one
with only a database, and both are different from one with only a web console -
the last of which is browser operation, and costs more than the other two
combined.

Then ask the question that saves the most work in the whole interview:

    "Is there already something that pulls these together for you?"

A middleware layer, an OMS, a warehouse that consolidates the channels. If one
exists, several rows collapse into a single database, and that single answer
changes the design more than anything else on this page.

#### 3a. Each answer names a shape, and you can say so in the room.

The mapping is mechanical, and each half of it has a page of its own, with the
case that got it wrong. Read whichever the answer reaches; they are not steps
and there is no order to arrive in:

    the systems and how each is reached      ../guide/data-sources.md
    the read surface, per audience           ../guide/read-path.md
    the entry point, per audience            ../guide/entry-point.md
    where unstructured knowledge goes        ../guide/knowledge.md

Read them before the meeting rather than when you arrive at the stage. They
are written as build-time decisions, but every one of them is settled by an
answer the customer gives here, and knowing which answer produces which shape is
what lets you say it out loud:

    "if that is a read-only account we build A; if it is only the web console it
     becomes B, and B costs considerably more than everything else together"

That turns an interview into a design conversation, makes the expensive answer
visible while they can still change it, and gives a discovery deck its
right-hand column - every question paired with what answering it produces.
`asgard-cli size <shape>` turns the shape into a count.

#### 3b. For each system we will actually connect to, get the coordinates.

Ask in the meeting, not by email afterwards. Every one of these has turned an
integration from days into weeks by being discovered late:

  - host, port, database or schema, and the account name
  - is the account read-only? Ask explicitly. The one offered first usually
    is not, and finding out later means going back for a second credential
  - is it reachable from outside their network? Asgard is a hosted cloud
    service and the agent runs in a sandbox the platform starts, in that cloud.
    There is nothing of ours to put on their network. So the ask has one shape:
    they add Asgard's four outbound addresses to their allowlist.

    Ask their network team for exactly that, not for "a VPN, an allowlist or a jump host". Offering
    options invites their network team to choose one that does not apply, and
    finding that out takes the week you were trying to save.
    If their policy needs a VPN, that is their side's business about how the
    allowlist gets implemented; what we need from them is unchanged.
    A database with no address reachable from outside can be reached
    through an SSH bastion they already run, and then the allowlist goes on
    the bastion: `../wiki/operations.md` says which database classes allow it
    and what it needs.

    Ask whether it can be done and roughly when - a date changes our plan. This
    is the most expensive thing to discover in week three, and it takes one
    sentence to ask in week one. Do
    not ask the customer who approves it, and do not put an approver's name on a
    slide: a name does not change what we build, and filter 0
    below names this exact case. In most companies it is a ticket, an approval
    and a change window rather than something done that afternoon, and that
    queue is theirs to manage, not ours to chase.

    Ask the customer for the person's name, for the row in
    `docs/open-questions.md`; do not put the addresses in your answer. They go
    to whoever makes the change, once, read fresh from
    `../wiki/operations.md` - not into this repository, not onto a slide, not
    into a thread
    that gets
    forwarded. The addresses can change and a copy will not, and a stale
    allowlist drops the customer's connection. Section 4 already keeps
    coordinates out of a committed record, and ours are the same class as theirs.
  - who issues the credential, for the tracking row in `docs/open-questions.md` - by name or role. A
    credential with no owner turns into a delay
  - for an API instead of a database: the auth scheme, who holds the client id
    and secret, and the rate limit - the rate limit decides whether a Syncer can
    backfill at all
  - is there a test environment for this system? Ask it of every system, not
    only of APIs. It decides what the first delivery can actually do:

        there is one        the test period runs against it, and the whole path
                            is genuinely proved - fields, validation, status
                            codes, all of it
        there is not, read  ask whether they permit reading production data
                            during a test. Some security policies do not, and
                            that is a go/no-go for the first meeting rather
                            than for week three
        there is not, write drafting or a mock, and say so now

    Do not call it a sandbox in front of anyone. In this material a sandbox
    is the thing the platform starts to run an agent in - a different subject
    that appears a few paragraphs above this one.

    And when there is one: production and test schemas differ, in shape and
    in volume. A query built against a test database is not proved against
    production, which is the same problem as guessing a schema.

For a system we will write into, that list asks nothing useful. Host and
port and read-only do not describe creating a record. Ask instead:

  - what is the token allowed to do? Not how many credentials they will
    issue and not whose name it sits under - what the one we get may do
  - what does creating one of these require? The mandatory fields and the
    validation rules, and ask for the document rather than the answer - see the
    asking ladder above
  - which field on the record says which of their customers this is for?
    This one is ours, because the agent fills it. Everything else about
    that record is their system's business

Their side of that record is not ours to design. Whether the token is a
service account or sits under a named person, whether their system routes or
counts SLA by creator, how they notify - do not ask the customer any of it,
in the meeting or in a follow-up. It changes nothing we
build, and asking it in front of a customer is designing their permissions for
them. Filter 0 below is the test; this is the case it catches most often,
because a write makes the questions feel responsible.

They go in the request's section 4, one block per system.

Passwords do not. The request record is committed to git, and deleting the line
does not remove a secret from a commit - the history keeps it. The only fix is
rotating the credential, which means going back to the customer to ask for a
new one, having just told them we leaked the last one.

    coordinates  -> the request record, and the platform's variables
    passwords    -> .env locally (gitignored), the release's own Secret in the cluster

Write the *name* of the key in the record - `<TARGET>_DB_PASSWORD` locally,
`<target>_db_password` in the release Secret - and never its value. `.env.example` at
the repo root has the full pattern, including the three places a new database
has to be registered before it works end to end.

A credential the customer's own users supply is a different problem again,
and it comes up whenever the agent acts on behalf of individual people rather
than as one service account: each user's token for a third-party platform has to
be stored and replayed, so it cannot live in the release's Secret and cannot
live in `.env` either.

That is a service with a database, not a chart - one existing deployment holds
them AES-256-GCM sealed in a column, with the key from its own environment,
masked for display, and the whole boundary isolated behind one package that the
build refuses to let other layers import. If a requirement implies this, say
so early: it is the point at which the engagement stops being a chart and
needs somewhere to run code. `../usecase/per-turn-credentials.md` is the
lighter alternative - the caller supplies the credential each turn and nothing
is stored - and it is worth checking whether that is enough before agreeing to
hold anything.

If the customer wants to hand over a password during the meeting, take it into
`.env` there and then and say why it is not going in the notes. Doing that once
in front of them is usually the last time they paste one into a chat.

#### 3c. What does it have to fit into, not just read from?

Section 3 asks where the data is. This asks what already exists around it, and
it is a separate question because customers do not volunteer the answer - the
systems they mention are the ones holding data, not the ones the agent will have
to live alongside.

  - Is there already a bot or a helpdesk the staff use? Nobody opens a new
    channel that duplicates one. Sometimes the right answer is to
    become a tool inside theirs rather than a front end beside it
  - Is there an internal portal or admin console this should sit inside? That
    decides the entry point as firmly as the audience does
  - Has anyone tried to build this before? Ask directly. A previous attempt
    tells you which part turned out to be hard, and they will not mention it
    unless asked, because it did not work
  - Is there a system with no API and no database - only a web console a
    person clicks? That is browser operation, and it costs more than every other
    integration on the list combined. Establish it now, not in week three
  - Who owns each system internally? Not the credential owner - the person
    whose approval is needed before anything touches it. Integration work stalls
    on this more often than on anything technical

#### 3d. If they are reached through a chat platform, which one?

Question 2 decides whether the entry point is the platform's hub or one of your
own. This is the separate question of which channel, and it is worth asking
in the same breath because `botProviderClass` is immutable once created -
changing it later means a new BotProvider, not an edit. The CRD enforces both
that and exactly one class block being present.

    LINE / Telegram          the platform posts a webhook; one CR
    Slack / Discord          a connector pod holds a socket; the operator creates it
    your own front end       generic, and you own the appearance

Ask where their users already are, not where it would be convenient to put them.
An official account with a following is a distribution channel a widget cannot
reproduce, and asking those people to visit a web page instead loses most of
them.

#### 3e. Is there anything between the channel and us?

Ask this whenever the answer to 3d is a chat platform, and ask it early, because
a whole class of requirement depends on it and the customer will not raise it
themselves.

    the customer  ->  ???  ->  Asgard

Whatever sits in that middle - a support desk, a helpdesk product, their own
relay, or nothing at all - is what owns the conversation. The platform does not:
there is no CR for handing over to a human, for pausing while a person replies,
for resuming afterwards, or for counting how many questions one user has asked.
`../wiki/integration.md` has the detail and the sources.

So every requirement of this shape belongs to that middle layer, not to us:

    "transfer to a real agent"           the desk takes the thread
    "pause the AI while a human replies"  the desk stops forwarding
    "three failures then a human"         the desk counts
    "ten questions per user per day"      the desk counts

With a website the middle layer is obvious, because the site is already there.
On a chat platform it is whatever they already run on that account, so ask who
owns the account and what their agents use on it today, for the middle-layer
row - not whether the platform supports handoff, which it does not either way.
Whatever that surface turns out to be, the pause/resume state and the counters
live outside Asgard. See `../wiki/integration.md`.

If the answer is "nothing", say so plainly rather than designing around it. The
choice is theirs: put a desk in front, or drop the requirement. Do not propose
handoff with nothing in the middle; nobody can deliver it.

Two platform limits worth handing over in the same conversation, because they
shape what can be asked for: one request gets 3 minutes, and an endpoint
serves 5 requests per second. A troubleshooting conversation that consults a
knowledge base, then a CRM, then a ticket system, then asks a follow-up, is what
runs into them.

There is a third, and it is set by us rather than by them: steps per request. A
step is one hand-off between processors, and the ceiling is a field on the
chart, 30 by default - so say what a step is, and that the number is set per
deployment rather than by the platform. `../wiki/integration.md` has what
counts and what does not.

LINE also needs two-way setup - Asgard issues a webhook URL that somebody has
to paste back into the LINE console and verify - so it needs an owner on their
side, not just a credential. See `../wiki/integration.md`.

#### 3f. Listen for the sentences that are a skill.

Section 3 asks where the data is. This asks for something the customer will say
in passing and never volunteer, because to them it is how things are rather than
a fact about a system:

    "這個代碼的意思是⋯"                a status vocabulary
    "我們內部把 A 和 B 算成同一件事"     a cross-system mapping
    "這個數字要這樣加總"                an aggregation convention
    "那一欄我們只在退貨的時候填"          a field's real meaning

Each of those is a skill. Write it down when they say it.

There are two halves to this and the second is easier to miss:

    what they say out loud     write a skill there and then
    a document they hand you   it becomes a skill - it does not stay in references/

Nobody can reconstruct it later from the schema, because it is not in the schema.
An agent without it fails without showing it: it answers confidently and
wrongly, having interpreted a code that meant something else.

It goes to `assets/skills/<name>/SKILL.md`, which is synced to the platform. In
`references/` it is invisible to the running agent, and the failure then looks
like a model ignoring instructions rather than a file in the wrong place.

This is also the row a proposal forgets, because it is knowledge rather than a
system: there is no credential to ask for, so it never comes up in the access
conversation. `../usecase/skill-layers.md` is what one looks like at full
size, and what its layers are for.

### 4. Is any of it unstructured?

Documents, FAQs, pages on a website - things a query cannot answer exactly.
Those become a Drive with a knowledge graph, and they are a different shape from
rows in a database. Ask separately; customers rarely volunteer documents when
the conversation has been about systems.

Also ask whether they need to see where an answer came from. Citations are
available and are not automatic: the sources arrive on the completion event
inside the message's `template`, but only if the Workflow is built to return them
and the front end reads that field. It is decided when the chart is written, and
retrofitting it means changing the workflow and the front end together - so it
belongs in the request rather than in a later conversation. Regulated industries
and anything replacing a human who cites a manual will want it.

Retrieval quality depends on how their people ask. Specific keywords work
better than vague ones, one topic per question, and context helps. A customer
whose staff ask "tell me everything about X" will judge the knowledge base as
bad when the problem is the questions. Sample questions on the agent teach this
without anyone reading a guide.

Anything a query DOES answer exactly - counts, prices, stock levels, contact
details - belongs to a query tool, not to a Drive. Putting a number in a Drive
makes the agent paraphrase a figure it should have read.

#### 4b. Does something have to happen when their system does something?

"When an order comes in", "when a ticket is escalated", "when the stock drops
below" - that is a webhook, not a schedule, and the two get confused because
both run with nobody watching:

    their system calls us when it happens    a webhook. `../wiki/automation.md`
    we look on a timer                       a Trigger, and always later than the event

What decides it is whether their system can call out at all.
Many cannot - an old ERP, a vendor SaaS with no outbound hooks - and then a
schedule is the fallback, with a delay the customer should hear about now rather
than at acceptance.

Ask who can configure that on their side, for the row in `docs/open-questions.md`. It is
usually a different person from whoever gives you a database account.

#### 4c. Do they expect it to send anything outward?

Mail, SMS, a message into a group. Customers ask for this constantly and it
sounds trivial next to reading a database, so it gets nodded through.

The platform cannot send mail. No SMTP, no preset mail toolset, nothing in
the core. The only outbound call it can make is `http-request`, which speaks
HTTPS. So:

    they have an HTTP mail API                   we can call it
    they have SMTP credentials                   that is not an endpoint
    they have neither                            it cannot be built yet, and
                                                 that is a question for them

The middle row costs a week, because it sounds like a yes.
A username, a password and `smtp.<host>:587` is what a customer hands over when
asked for mail access, and it cannot be used at all - SMTP is a multi-round
protocol on its own port and a Workflow has no way to speak it. One engagement
asked for a specific mail API, was told which one, and received SMTP credentials
for it; they are different authentication mechanisms and not interchangeable.

Ask for the specific thing, not for "access": an HTTP mail API, a key for it, and a
sender address already verified with that provider. The verification is their
IT's to do and an unverified sender is refused outright, so it belongs in the
same sentence as the key rather than in a second round trip.

The same applies to SMS and to a message into a group: the question is always
whether there is an HTTP API, never whether they "have" the channel.

Do not let it be mocked silently. A mocked send that returns success and
writes "notified" into a log is worse than no send: somebody later reads that
log and believes people were told. If a mock is right for a test phase, say now
that every summary will lead with "not actually sent".
`../wiki/integration.md` has how one deployment does it.

### 5. Is there anything it should change, and not just read?

The standing architecture is read-only. Do not absorb a write into this request
as a project decision. Treat it as a warning: it needs its own spec and human
approval, and it cannot be reached from a scheduled run, because a schedule has
nobody to approve it.

Record it in the row and say so out loud in the meeting. A write path discovered
after the read path is built is the most expensive rework in this repo's
history.

### 6. How will they know it worked?

Ask for the sentence they would use to tell a colleague it was working, then
keep asking until it names something observable. "It answers stock questions" is
not verifiable; "the warehouse lead stops phoning about location 608" is.

This becomes the acceptance criteria of the task specs, and a request whose
success nobody can describe produces tasks nobody can close.

#### 6b. What is the smallest version they would accept as proof?

Get their answer. You will have one in mind and it will be the one that is
easiest to build; theirs is the one that gets judged.

    "if it only did ___, would that be worth putting in front of someone?"
    "of everything here, which one would you want to see working first?"
    "what would make you say this is not going to work?"

If they handed over a verification list, read it back and ask which item they
would keep if they could only keep one. That item is the MVP, whatever it costs
to build - a first delivery that skips the item being judged has failed however
fast it shipped.

To read it back, arrive with a reading. A customer who wrote a document
listing what they want tested has already answered most of this, and asking them
cold - "so which of your three would you like first?" - hands them our
sequencing problem and reads as though we cannot do all of it. Their document is
the answer; bring your reading of it and ask them to correct it:

    no    "三個情境要先做哪一個?"
    yes   "你們驗證項目裡寫了 X。我們讀下來,情境二最快能證明它,因為 ___。
           這樣對嗎?"

The second takes the same minute and produces a decision. It also surfaces
disagreement, which the open version cannot: a customer correcting your reading
tells you what they think; a customer picking from a list tells you what was
easiest to say.

So this is a meeting question, not an open-questions row. It belongs on the
agenda, and it only becomes a tracked row in one case: they handed over nothing
that speaks to it and would not answer when asked. Filing it as a blocker when
their own document answers it puts a question at the top of a list that the
customer can see they already answered, and everything under it inherits that
impression.

Then work out what that one item genuinely needs, and the two filters below turn
the rest into deferred scope rather than open questions.

#### 6c. Do they need to know what it will cost to run?

Not our fee - the platform's usage billing, which is a separate question and one
a customer with a procurement process will ask before signing anything.

`../wiki/fehu.md` has how it is broken down: by Service (Platform,
Knowledge Base, Data Insight, Agent Hub, Heimdall) and by Item (Project
Usage, Processor Usage, Seat), in units of Units-Days, GB-Days and Times.

Worth knowing before answering:

  - the Workspace is the billing unit. So how the work splits into projects
    and workspaces has a cost consequence, and the split is decided in
    `../guide/projects.md` - before anybody has asked this question. Ask it now
  - a seat is a line item. "Everyone in the company can use it" is a
    sentence with a price, and the customer usually has not connected the two
  - only Odin lets them bring their own model. Sindri and Mimir use the
    platform's models and the LLM cannot be swapped, so "we will use our own
    Claude account" has a different answer per product - and which product this
    is was decided at 2b. A customer with a model contract or a rule about where
    inference happens has to hear it there, not here
  - a Loader and an Indexer each cost several times a Project, per day, and
    a Processor is billed per node per day. So a workflow's node count is a
    standing cost, and a design that routes through sub-workflows where one
    prompt would do pays for it every day it exists

If they do not raise it, say the shape of it anyway, once. A cost discovered
after a pilot is the reason a pilot does not convert.

### 7. What is explicitly out of scope?

Write down what you are NOT building, particularly the things they mentioned in
passing. An unrecorded "we could also..." returns as an assumption three weeks
later, and by then nobody remembers whether it was agreed.

## What they ask us

An interview runs both ways. A customer who wrote a test plan usually ends it
with a list of things they want us to confirm - account permissions, whether a channel can do X, what we
recommend for their existing system. Those have nowhere to live: the
open-questions file is this engagement's own questions, and
`../wiki/platform-unknowns.md` is what no source settles.

They go in `docs/open-questions.md`, in its own section, and the file the
scaffold writes now has one.

Check each against `../wiki/platform-unknowns.md` before answering. Much of
what a customer asks us is already on that list, because
they ask about the same things every engagement hits - how far a quota can be
raised, for one. When one matches:

    say so, plainly, in writing. "We do not have a confirmed answer to this
    yet and are checking with the platform team" is a real answer and an
    honest one

This avoids answering from a reasonable assumption, having it written into
their evaluation, and discovering in week six that the platform does not do it.
Their list is usually also their acceptance criteria.

Answer in writing, with a date, and put the answer next to the question. A
verbal answer in a meeting leaves no record, and the next person cannot tell
what we committed to.

## Material they hand you

Ask for it in the meeting, before you need it. Customers usually have more
written down than they think, and none of it arrives unless asked for.

Asking has its own order, different from the one in question 3. Question 3 is
how we reach a system once we have it; this is what to ask them for first:

    question 3, how we READ a system   a database  >  an API  >  a screen
    here, what we ASK THEM FOR         docs > source > API spec > the DB > the UI

| ask for | what it gives you |
|---|---|
| system documentation / operating manual | best. Fields, validation rules, status codes and the process are all in it, and it explains what things mean |
| the source code | better than a spec, and people forget to ask. The code is what the system does; a spec is what somebody wrote down about it once. For a system they built themselves this is usually available and usually decisive |
| the API spec | a clear contract, and it drifts. Good for shape, weak on business meaning |
| the database | the data without the rules. You can see every field and not what any of them means |
| the back office screen | last. You can look at it and cannot quote it |

Source code ranks above a spec because a spec describes an intention and the
code is the behaviour, including the special cases nobody documented and the
field that means two things depending on another field. Customers rarely offer
it and often will hand it over when asked, so ask.

For a system they bought, skip that row and go to the API spec. The order does
not change; there is nothing to ask for, and asking anyway spends a request on
it.

So establish which kind it is before working down the list. If you do not know
yet, ask without assuming: *"the API documentation, and if it is something you
built yourselves, the source as well."*

Ask for documents first so that you do not have to question them field by
field. Do not go through fields, validation rules or status codes one at a time
in the meeting: it is slow, and what you get is the version the person
remembers. Meeting time is for what only they can answer - whether the network
reaches it, who issues the account, who approves a write.

Say why you want it, because it makes them more willing to give it: their
manual becomes what the agent knows. A field dictionary is what stops it
inventing a status code.

Do not design their permissions while asking. "A read-only account for
queries and a separate writable one" is our implementation preference stated as
a request, and it is not always even possible - plenty of systems issue one
account with different rights. Ask what access they can give and what it allows;
let them tell you how many credentials that is.

The material does not go in the request record. There are three directories,
and they differ by who reads them:

    references/            background, for humans and spec-writing agents
    requirements/          the implementation source of truth
    assets/skills/         what the RUNNING agent needs, synced to the platform

File the material into `references/`, and have the request cite it. A
request that inlines the whole of an API's documentation stops being readable as a
request, and the section that matters - what the customer asked for and who is
on the other end - disappears into an appendix.

    asgard-cli reference add <file> --what "<what it is, in your words>" \
      --from "<who supplied it>" --dated <the document's own date>

Do not invent a provenance table. Before this command existed every engagement
invented its own, and one invented a directory name that then read like a
convention. The command copies the document byte-identical - so a second version
can be diffed against the filed one - and puts the provenance in
`references/_index.md` instead of a header pasted into the customer's file.

`--dated` is the document's own date, not today. That date decides whether the
material is stale; record a document with no date as carrying none. Fill all
three in at the moment of filing: whoever handed it over is the only person who
knows, and they are usually unavailable by the time somebody needs to know. `asgard-cli check` warns about the rows
that are short.

The rule the repo already enforces: convert reference material into
`requirements/` before implementing, and if the two conflict, `requirements/`
wins and the conflict becomes an open question or a decision record. Nobody
implements straight from `references/`, because material a customer wrote for
their own staff describes the system they believe they have.

Domain knowledge the agent needs at runtime is the third case, and people get
it wrong most often. Status-code meanings, cross-system entity mapping,
aggregation conventions - those belong in `assets/skills/<skill>/SKILL.md`,
because they have to be synced into the platform to be usable at all. Left in
`references/`, they are invisible to the running agent, and the failure looks
like a model that ignores instructions rather than a file in the wrong place.

### Filing a large body of material

A manual worth keeping is worth structuring. One shape already carries 300K of a
web console's operation map without becoming unreadable:

    <topic>/SKILL.md                  the router: what this covers, and when
    <topic>/references/conventions.md the cross-cutting rules read first
    <topic>/references/page-map.md    broad and shallow: every entry point
    <topic>/references/api/<domain>.md one file per domain, index table on top

Two habits inside it make it usable, more than the layout does:

  - Every index row carries its provenance. That set records, per operation,
    whether the contract was actually observed or is only good enough to
    navigate to. A row that says "not verified" is worth more than a plausible
    one, because the reader knows which to trust
  - It records what was measured rather than inferred. Routes were read off the
    real UI rather than derived from other routes, and the file states that.
    When a later reader finds a mismatch they know it is drift, not a guess

Both are what the request record asks for in section 4: say what was
confirmed, how, and when. Without provenance, accurate material and material
three years stale read the same.

## The three answers that look obvious and were wrong

Each of these was decided one way, built, and reversed in a real engagement.

| the obvious answer | what it turned out to be | why the obvious one failed |
|---|---|---|
| a public website should read through a SemanticLayer, like everything else | five zero-parameter query tools | a bound layer is arbitrary SQL over every cube, and the exposed surface grows by itself every time a table is added - `../guide/read-path.md` has why narrowing it is refused rather than forgotten |
| a public website is reached through the platform's agent hub, like everything else | its own BotProvider -> Workflow -> SandboxBlueprint | an anonymous visitor cannot authenticate to the agent hub, and the field that would let them does not exist - `../wiki/agents.md` |
| unstructured knowledge is a KnowledgeBase with Loaders and a retrieval workflow | a SourceSet Drive with `contextIndex` | the Loader-and-retrieval-workflow path was harder to keep correct than a Context Index over files. `KnowledgeBase` is still live and still shipping, so this one is experience rather than a platform rule |

In all three, "like everything else" was the reason given, and it is the wrong
reason: the audience decides, and the audience is what differs from everything
else.

## Two filters before a question becomes a row

An interview that ends with a long table of open questions has narrowed
nothing. It has moved the customer's whole document into a table, and the
meeting that follows spends its time on questions nobody needed answered yet.

Apply both to every question before filing it. Each turns a question into
something other than a blocker - not our problem, or not now - and most questions
are one of the two. Keep what survives short; the meeting needs a short list.

### Filter 0 - is this ours to answer at all?

Run this filter over your question list before the meeting, because it
removes the most rows. We are delivering an agent. We
are not designing the customer's support operation, and an interview that drifts
into how their own systems and teams fit together has stopped being a
requirements interview.

The test is narrow: does the answer change what we build?

Apply it by imagining the most specific answer possible. Do not ask "would this
be useful to know"; assume they answer perfectly, then ask what you would do
differently. "Ming issues it" and "Ming spends five hours a day on it" are both
perfect answers and neither changes anything. That version of the test catches
in one pass what the categories below catch one at a time.

    ours        what we need FROM them to build it - a credential, an endpoint,
                a network path, a document, an account, a decision only they can
                make about our behaviour
    theirs      how they staff a channel, who maintains a document, how their
                two systems relate to each other, what their people do today

Filter 0 decides what to track, not what to put on a slide. These are two
different lists and this section has been read as one - an FDE saw "who issues
the read-only account: ask" and put that question on a customer slide, where it
was rejected on sight.

    tracked      knowing who to chase is project management. It belongs in
                 the open-questions row, with the name in the ask column
    on a slide   only questions whose answer changes the design

Everything below is about the first list.

Whether to ask for a name depends on which of two kinds of person it is:

    the person who will hand us the thing      ask. Without a name, a
                                               dependency is just a delay
    the person who authorises them internally  do not ask. Their org chart,
                                               their queue, and it changes
                                               nothing we build

So: who issues the read-only account - yes, we will be chasing them. Who signs
off the firewall change - no. Ask whether it can be done and roughly
when, because a date changes our plan; a name in their approval chain does
not, and asking for one in front of a customer reads as managing their internal
process. This question has reached a customer slide before.

Do not file a question about their internal arrangements as an open question.
Either hand it back as a note - "this is worth deciding before you go live, and
it is yours" - or drop it.

Two ways this goes wrong, and both look like diligence:

- Doing their integration analysis for them. How their channel binds to their
  CRM is their business unless we are the thing doing the binding. Asking it
  makes us look thorough and produces a table nobody uses.
- Turning an operational precondition into a design question. "Is a person
  already answering on this account" matters, but it is one line in the handover
  - a thing they must sort out before we attach anything - not a row we track
  and chase.

What survives filter 0 is almost always a small set of the same shapes: a
credential, an endpoint, a network path, a document, an account, and the two
answers only they can give (2b and 6b).

Put those last two on the agenda before filing them as rows. Both are answered in
the meeting by a person in the room, so a row for either is a note that the
meeting has not happened yet - and 6b in particular has usually been answered
already, in whatever they handed over. Bring a reading and ask them to correct
it. File a row only if you asked and got nothing.

### Filter 1 - the minimum that proves it works (MVP)

Before the meeting, work out the smallest thing that proves what they said they
are testing.

That is different from the smallest thing that is easy to build. A customer who hands over a test
plan has already written down what counts as success, and a first delivery that
avoids the item they most want to see has failed, however quickly it shipped.

So read their verification list first, then find the smallest slice that reaches
the hardest item on it. Record what that slice does not need as a line in the
request's section 5, Scope, not as an open question, with a note of what would
have to be answered before it comes back.

Cut mechanisms, not capabilities. This filter rests on that distinction, and it
goes wrong in both directions when the distinction is missed.

    cutting a capability     "we will not read your system yet"
                             fails their test. It is the thing being judged
    cutting a mechanism      "for now the user tells us which record they mean"
                             passes it. The integration is still proved

The hardest question in an engagement is usually how the agent knows who it is
talking to, and it is often cuttable,
because the user can be asked. A lookup keyed on something the user types proves
the same integration as a lookup keyed on a recognised identity, and the identity
question moves to phase two without the delivery losing anything the customer is
measuring.

Check whether it needs cutting at all before you cut it. The previous
paragraph, misapplied, has already produced a promise we did not have to make.

An anonymous channel can answer "where is MY order". What it cannot do is let
the model choose whose case to look up. Whatever sits in front - a website, a
chat channel, a support desk - is what knows who is talking, and it passes the
identity through server-side on every turn. LINE's webhook carries a userId.
So if the customer's system has the binding stored, per-customer lookup is in the
first delivery and there is nothing to defer.

    is there a layer in front that knows who is speaking?
      yes  -> keep it. Cutting it gives away something you had
      no   -> now it is genuinely cuttable, and that is a question for
              them rather than a design to work around

Getting this backwards costs more than a deferred feature: it tells a customer,
on a slide, that their channel cannot recognise their own customers when it
can. `../guide/read-path.md` has the mechanism and
`../usecase/per-turn-credentials.md` has the shape - read one of them before
promising anything of the form 「查我的⋯」.

When it is kept, it brings an acceptance criterion with it, which belongs in
section 6 now rather than being discovered at verification: a query that
forgets to filter on the injected identity fails on the anonymous path only,
which is the path nobody tests.

What that leaves blocking the first delivery is normally more mundane and more
useful to raise in a meeting - whether the system is reachable from a cluster at
all, and who can grant an account this month.

Apply the filter per question, not per row. Some rows hold two questions in one
sentence, and the filter then defers the half that should have stayed. "Is there
anything in front of the channel" is the common case: the half about identity
defers cleanly, while the half about whether that channel is already staffed
today does not. Attaching a webhook to an account real
people are already answering on changes their experience on day one, before any
of the deferred machinery exists. That is an operational precondition of the
first delivery, not a phase-two design. Split the row and keep that half.

Also note what "we already have that system" does not tell you. It says the data
exists. It says nothing about a network path, a read replica, or an account.

The MVP runs against their real channel with their real documents, and a real
person can use it. A demo on sample data proves nothing, and the questions it
defers all come back at once.

### What is left after both

What neither filter removes are the real questions, and there are usually two
or three:

  - how a system is reached - the one that blocks the MVP nearly every time,
    and where "we have that system" means the data exists and nothing more. A
    hosted platform reaching an internal system needs a firewall change only
    they can make, with an approver and a lead time
  - anything the customer must do before we can - provision an account, paste a
    webhook URL back, open a network path
  - anything where two of their answers contradict each other

If a surviving row is not one of those shapes, run it through filter 0 again.
Most of what gets past these filters and still turns out to be noise is a
question about the customer's own arrangements that felt too important to drop.

## Write it down as you go

Write it during the meeting. A record written afterwards holds what you
remember, which is the design you were already forming. The three commands are
at the top of this page.

`request add` writes `requirements/requests/REQ-xxx-<name>.md` with today's date
and `draft` on it, and its sections are this interview in the same order.
Fill them in the file; the TODOs are the questions above.

Open questions go to `docs/open-questions.md` as well as into the request, one
row each, with what they block and who can answer. A question kept only in a
spec disappears when that spec reaches `done`, and `asgard-cli question` reads
the open-questions file on every run and prints it before anything else.

Ask who can answer, for the row's ask column - by name or by role, in the
meeting. Nobody follows up a question with no owner.

## When the request is ready

`asgard-cli request ready <<.RequestID>>` when all of these hold:

  - the customer's own wording is in section 1, unedited
  - the audience is decided, and the target project follows from it
  - what they do with each answer is decided - an agent, a dashboard, or both -
    because the product follows from it and every section below assumes one
  - every system has a row, and every row says how it is reached
  - the open questions have been through both filters, so what is left blocks the
    MVP rather than describing everything still unknown
  - the customer has said which single item they would keep, or corrected the
    reading you brought them. It is theirs to settle and cannot be decided for
    them - but arriving without a reading asks them to do the work of the
    meeting
  - every system we will connect to has a section 4 block, with a named
    credential owner and an answer on network reach - and no secret in it
  - unstructured knowledge is either listed or explicitly ruled out
  - every write is marked as a write
  - success is described in terms somebody could check
  - nothing that blocks the work is missing an owner

Not ready is a normal state to be in. A `draft` that names its open questions is
more useful than a `ready` that guessed at them, and `asgard-cli question` will keep
the request in front of you either way.

What usually comes before the task specs is saying it back to them: what we
propose to do, what phase 1 is, and what we are not doing. That is the
`proposal-deck` skill in `.agents/skills/` - it owns choosing the shape as well
as the deck, and it is where the rule about claiming no further than the
evidence goes lives. Do not decide the shape here and write the deck from
memory afterwards.

Then split it into task specs:

    asgard-cli task add "<title>" --request <<.RequestID>> --project <project> --complexity M

**Checked:** against asgard-kube `cbd8d70` and asgard-docs `23409b3` for the platform claims it
carries, which are few by design - it is an interview, and the shapes belong to
the pages it points at. The three numbers here - 5 requests per second, 3
minutes and 30 steps per request - are the quota page's own; the step ceiling
is the chart's `maxUnsupervisedSteps` at asgard-core `478cf5d6`, which is why it is explained as a design choice rather
than handed over as a limit. In 4c,
the answer that actually arrives is SMTP credentials, which look like "an HTTP
endpoint that sends mail" and are "they do not". `botProviderClass` is immutable (`self == oldSelf`) and
exactly one of [generic telegram line discord slack] must be present, so asking
which channel in the same breath is a contract requirement rather than a
courtesy. Constraints the wiki and `../guide/read-path.md` own are pointed at
from the "obvious answer" table rather than restated.

**Unchecked:** filter 0 - "does the answer change what we build" - and the
question ladder under it are this material's own judgement, and no source states
them. A question that fails filter 0 and still mattered is worth filing:
`asgard-cli issue-report --new`, then `--send`.
