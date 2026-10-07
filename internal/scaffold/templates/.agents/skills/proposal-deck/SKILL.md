---
name: proposal-deck
description: Use when deciding what to propose to the customer, or building any deck they will see - a discovery deck taken while the questions are still open, a proposal, a scope review, a phase kick-off, or a handover after the work lands. Covers how the shape is chosen and what phase 1 is, where a deck lives in this repo, which material it is allowed to be built from, the order a proposal argues in, how to say it in the customer's own words without claiming further than the evidence goes, what must never appear on a customer's screen, and what a link on a slide has to point at. The outline that records what each page rests on is `outline.md` beside it, and the design language and the slide contract are in references/.
---

# Proposal decks

A deck is the only thing in this repository the customer reads. Everything else
records what we think; a deck records what they were told, and they agree to
scope based on it.

A deck assembled from the chart is accurate and useless to them. A deck
assembled from an ambition is readable and becomes a broken promise. Build the
deck from their own material: say back to them what they said, then add one
paragraph of what we will do about it.

## Where it goes

A deck belongs to the meeting it was presented at, so it is filed as one. There
is no new directory for decks: a meeting note that has attachments becomes a
directory instead of a file.

    docs/meeting-notes/YYYY-MM-DD-<topic>/
      README.md    the meeting note - who was there, what they said
      deck.md      what we presented
      deck.pdf     the file they were given
      assets/

The meeting note is what makes the deck readable later. Without it nobody can
tell whether a deck is what was proposed or what was agreed, and those are
different things - what was agreed goes to `docs/decisions/`.

Add four lines to the note, recording who the deck was for and where its
content came from:

    - 對象:營運主管 + IT 窗口(4 人)
    - 材料來源:references/erp-manual/、requirements/requests/REQ-003-*.md
    - 當天結果:一階段範圍確認;退貨流程延到二階段
    - 講出去之後改過嗎:沒有。要改就開新的一場日期。

Do not edit a deck after it has been presented. The customer's copy does not
change with ours, and a repository version that differs from the one they hold
is worse than no copy. If you change your mind, use a new date and a new
directory.

## Step 1 - find the material, and refuse to invent any

A proposal is built from what the customer gave us and what they said, in this
order:

| where | what it gives you |
|---|---|
| `references/` | their own manuals, schema dumps, field dictionaries, screenshots |
| `docs/meeting-notes/` | what they said, before we tidied it |
| `requirements/requests/` | what they asked for, in their words |
| `docs/open-questions.md` | what is still unknown - and this one is also an output, see below |

`docs/open-questions.md` is both a source and an output. Working a deck through
with somebody rewrites the questions: one engagement's revisions reworded about
half of them, retired three and found six more, and none of it went back into
the file because nothing said it should.

    reworded a question on a slide   change the row, in the same edit
    dropped one                      retire the row, with why
    found one                        add it, with who can answer

Make the change in the same edit, not afterwards. The FDE who reported this
intended to reconcile at the end; after many rounds the two had diverged far
enough that the reconciliation kept being deferred.

This matters more than ordinary staleness because `asgard-cli question` prints
this file back, and it is the first thing whoever picks the repository up next
is told to read. A stale file sends them into a meeting with questions already
abandoned, and it also preserves a judgement that has since been overturned,
still argued convincingly. In that engagement, security reasoning the deck had
corrected was still in the file, well written.

`asgard-cli check` warns when a file under `docs/meeting-notes/` is dated after
the newest date inside the questions' own rows - a raised date, or the date
an answer was written beside one. It does not use the questions file's
timestamp, which any edit would reset, including one that touches only the
prose. So reformatting the file does not clear this warning, and answering a
question does.

Do not use `docs/spec/` or `projects/*/chart/`. Those describe our
implementation. A customer looking at a slide made from a chart sees a diagram of
something they did not ask about.

If a claim you want to make is in none of the four, it is not yet a claim. Ask,
or put it on the open-questions slide. Do not smooth it over: this repository
records three decisions that were made confidently, built, and reversed after
contact with reality. Any of the three, written into a proposal, would have been
a commitment we could not keep.

Every number carries where it came from:

- a number the customer gave us - say so, and say when they gave it
- a number we measured - say what was measured and when
- a number nobody has - leave it out, or put a question on the slide

A number that passes all three can still be in the wrong place. A title names
things; it does not count them. 「六個指令」 and 「12 個 CR,14 次部署,4 個坑」
are both accurate, and both use the three seconds a reader gives a title
without naming a subject: a reader who has not seen the slide learns nothing,
and one who has learns nothing new. What they needed was
「init、project add、add、push、tag、approve」. The quantity goes in the body.

The test needs no judgement and no context - does the title contain a
quantity - so it is a useful addition to the checks that need a reader. The
exception is a discovery deck, whose titles are the customer's own headings
unedited: if they counted, keep their count, because they wrote the title.
See step 5.

## Step 2 - decide what we are proposing

A deck cannot be written from open questions alone. Between what they said and
what goes on slide 4 there is a decision - what shape we are proposing - and
make it here, deliberately, rather than while typesetting. This step is the one
most often skipped, because the material from step 1 reads as if it already
contains an answer. It contains a problem, not an answer.

These rules are the interview's, repeated here because a deck gets built weeks
after the interview and often by somebody else. `asgard-cli guide
requirements` has them in full, with the worked examples.

The shape follows the audience, not the capability.

    internal, authenticated callers   -> the platform's agent hub, semantic layers
    public, anonymous visitors        -> your own BotProvider, fixed query tools

The two paths share neither an entry point nor a read path. A proposal covering
both audiences is two proposals, or one with two phases - never one shape
stretched over both. This repository records three decisions that were made the
other way, built, and reversed, and all three were reversed because of the
audience.

Take a shape that already exists. `../asgard-platform/usecase/` holds the ones taken
from deployments in production, and each says what it costs to assemble. A
proposal built on one of them can be estimated, because somebody has already
built it. A proposal built on a shape nobody has assembled is an estimate of
a guess.

To show what one looks like end to end, use a worked example rather than
inventing a scenario. `../asgard-platform/wiki/case-studies.md` has one event described from three
angles, which shows concretely how the work divides between the three
products, and a scenario written from it has already been checked.

If they ask to see it working and we have none of their systems, that is a
shape too, and it has been built: `../asgard-platform/usecase/demo-generation.md` - no
data, no credentials, and still a real deployment rather than a slide. Decide
whether the deck is promising one before writing a slide that implies it.

First, decide whether it is an agent at all. The vocabulary below is
agent-shaped, and so is most of what this tool carries, so this question gets
skipped. Something they want to watch - the same numbers every morning, a
figure trending the wrong way - is a Mimir dashboard, not an agent. Something
they want to ask in the moment is an agent. Many customers want both, and those
are two deliveries over one Semantic Model, not one delivery.

A proposal that answers "we want to see stock across all our channels" with an
agent has answered a question they did not ask. `../asgard-platform/wiki/mimir.md` is the
product; `../asgard-platform/wiki/product-suite.md` is the fork.

Then decide what their answers turn into. It is not one row per system:

| what they have | what we build |
|---|---|
| numbers somebody checks on a schedule | a **Dashboard** in Mimir, over the model below |
| a database we can read | a **Semantic Model** - the agent composes its own queries |
| a database, but only a few answers should be reachable | **fixed query tools** |
| an HTTP API | a **Workflow** wrapped as an **MCP Server** |
| documents, manuals, FAQs, a support site | a **Drive** with a Context Index |
| only a web console | **browser operation** - costs more than everything else combined |
| systems whose concepts do not line up with each other | a **Skill** |

The last row is the one proposals leave out, because it is not a system to
connect. It is knowledge about the systems, and it has no credential to ask
for, so it never comes up in the access conversation.

You need it whenever the customer's own systems disagree: four marketplaces
with four sets of order-status codes, a SKU that is written three ways,
"shipped" meaning something different in the WMS than on the channel, a
returns flow with vocabulary only their staff knows. Somebody has to write down
how those correspond, or the agent guesses. It guesses plausibly, which is
worse than failing, because the answer looks right.

Tell the customer two things about it, because neither is obvious:

  - It needs them more than anything else here, and it is not an IT task. A
    credential comes from their network admin; this comes from whoever knows
    that channel B's "processing" is channel A's "paid". It is an hour of an
    operations person's time, and nobody else can supply it
  - It is the cheapest item on the list, and it decides whether the rest is
    usable. Nothing to procure, nothing to provision - a document

It goes to `assets/skills/<name>/SKILL.md` and is synced to the platform. In
`references/` it is invisible to the running agent, and the failure then looks
like a model ignoring instructions rather than a file in the wrong place -
`../asgard-platform/guide/requirements.md` has the full rule.

An integration across several channels that proposes only the connections has
proposed half of it.

Explain the cost ladder for reaching a system to the customer in words, because
it decides the timeline:

    a database we can read   >   an API   >   a screen a person clicks

The last one is browser operation, and it costs more than the other two
combined. It is not technically hard, but somebody has to document every page
and every dialog the menu does not show first.

Phase 1 is the smallest thing that proves what they said they are testing, not
the smallest thing that is easy to build. If they handed over a verification
list, phase 1 has to reach the hardest item on it; a first delivery that avoids
the thing being judged has failed however fast it shipped. Phase 1 cuts a
mechanism, not a capability - "for now the user tells us which record they
mean" is a cut, "we will not read your system yet" is a failure.

A document they give us is a deliverable line, not only an input. The most
common omission in the right-hand column is the skill: "we take your RMA manual
and turn its fields, validation rules and status codes into something the agent
knows" is a concrete thing we do with what we asked for, and it answers "why do
you need our documentation". Every capability reading a system the customer
documented has one.

Decide what we are not doing in the same sitting, not at slide 7. A scope line
written while choosing the shape has a reason attached. Each one gets a note of
what would have to be answered before it comes back.

This step produces two things before anything is typeset:

  - a `docs/decisions/YYYY-MM-DD-<topic>.md` for the shape, and why the
    alternative was not taken. The deck states the choice; the decision record
    keeps the reasoning after the deck is presented
  - the request's own scope section, updated - what is in phase 1, what is
    deferred, and what is out

If you cannot write the decision record, the shape is not decided yet, and a
proposal built on it would only restate the same uncertainty as slides. You can
still say something. Run this step conditionally instead - "given a read-only
account and a route to it, this becomes X" - and make a discovery deck, which
is step 5's first shape. Do not state the conditional shape as a decided one.

## Step 3 - the order a proposal argues in

Nine slides, in this order, is a complete proposal. Longer decks usually repeat
a slide. This is the proposal's order. A discovery deck - the one taken while
the questions are still open - has its own, in step 5; forcing this one when the
facts are missing fills slides 4, 5 and 6 with a shape nobody has confirmed.

| # | slide | the job it does |
|---|---|---|
| 1 | cover | who this is for, and the date |
| 2 | what you told us | their words, close to verbatim. Earns the rest |
| 3 | what it costs today | the number they gave us, attributed to them |
| 4 | what we propose | one sentence a non-technical reader repeats correctly |
| 5 | what it looks like to use | the interaction, not the architecture |
| 6 | what it reads | which systems, and who can see what |
| 7 | what is NOT in scope | the slide that prevents the argument in month three |
| 8 | how we get there | phases with dates, and what we need from them |
| 9 | what we still do not know | the customer's unknowns only - see below |

Slide 2 is the one most often skipped, and the rest of the deck depends on it.
A customer who hears their own problem stated back accurately will accept a
rough solution; one who hears a polished solution to a problem they do not
recognise will argue with every slide after it.

Slide 7 keeps the engagement deliverable. Scope stated narrowly can be
delivered; scope left open grows until the proposal fails.

Slide 9 holds only one of the two kinds of unknown.

| whose unknown | on the slide | why |
|---|---|---|
| **theirs** - how their CRM is read, which network their devices are on, whether a channel has an API | **yes**, and it is most of the deck | only they can answer. Listing it asks for their help |
| **ours** - how far a platform quota can be raised, what the platform does on a channel we have not deployed | **no** | it is our product. Saying we do not know what it can do shows we did not prepare |

`../asgard-platform/wiki/platform-unknowns.md` is the second kind, and its own instruction
is to ask the platform team when a requirement touches one, before the meeting.
Printing that list onto a slide does the opposite of what the page says, and
shows the customer we have not read our own product.

If an answer has not come back in time, say it in the room, once, and put the
written reply in the follow-up. It does not get a slide.

So the closing slide is what each side does next: what we reply on and by
when, which accounts and documents they provide, and the date you meet again.
Both sides have an action on it.

## Step 4 - say it in their words, and no further than the evidence goes

This step covers two failures that pull in opposite directions. A deck written
to avoid the first usually commits the second.

### Speak the way they do

The test: can someone who was not in the room repeat the sentence to a
colleague and get it right? If the sentence needs one of our nouns, it has not
been translated yet.

    no    Workflow 會呼叫 Toolset 查詢 CRM,再由 Managed Agent 組出回覆
    yes   客戶在 LINE 問訂單到哪了,它去 CRM 查,查到就回,查不到就說查不到

The second sentence is longer and sounds worse, and it is the right one, because
the customer can check it. Nobody outside our team can check the first.

Three habits do most of the work:

  - name the person, not the component. Who is at the keyboard, what they
    typed, what came back. Slide 5 is an interaction, not an architecture
  - keep their vocabulary, even when ours is more precise. If they say 報修
    單, the deck says 報修單, not "RMA ticket entity". Correcting a customer's
    own word for their own thing reads as not having listened
  - say what it does when it fails. A deck that only describes the happy path
    strongly suggests nobody has built it yet. "查不到就說查
    不到" is worth a line

### Claim no further than the evidence goes

Overstating causes a delivery problem that shows up eight weeks later, at the
acceptance meeting, where the deck is read back to you.

There are four sources of it, in the order they usually happen:

**1. A capability the platform does not have.** The list is written down:

    `../asgard-platform/wiki/platform-unknowns.md`

Check every claim on slides 4, 5 and 6 against it. Anything that appears there
belongs on slide 9, phrased as a question, and never on 4 or 6. Two items
catch people most often, because customers put them on their own verification
list as basic requirements, and both now have answers that are easy to
overstate. 操作紀錄可稽核 is yes: every prompt, reply and tool call is
an audit event - `../asgard-platform/usecase/conventions.md`. 限制不同使用者能查到什麼
is only half ours: the Console limits who manages a resource, and what one
caller may reach at runtime is decided in front of the platform -
`../asgard-platform/wiki/console.md` - so slide 6 names who owns that layer.
Writing it into a proposal as a platform feature is the most expensive sentence
in this document.

**2. A capability that belongs to the layer in front of us.** Handoff to a human,
pausing the AI while a person replies, resuming afterwards, counting how many
questions one user has asked in a day - no CR carries any of these. They
belong to whatever owns the conversation: a support desk, a helpdesk product,
their own relay. With a website that layer exists; with LINE it usually does not.

So a proposal that promises 轉真人 with nothing in the middle promises
something nobody can deliver. Say plainly which layer would have to own it, and
make it the customer's choice - put a desk in front, or drop the requirement.
Saying so is uncomfortable once, in a meeting; not saying so causes trouble for
the rest of the engagement.

**3. A number nobody measured.** Covered in step 1, and it is the easiest to
catch: every figure is theirs, ours-and-measured, or absent.

**4. A phase 8 we could not start on Monday.** Read slide 8 as though they said
yes today. Anything resting on a credential nobody has issued, a firewall change
nobody has approved, or a system nobody has connected to is a dependency, not a
phase. It goes on slide 8 as what we need from them, with an owner and a date,
or on slide 9.

### Say the same thing every time

If implementation nouns are banned without replacements, every FDE invents
their own, and the same customer sees two names for one thing across two
documents. These are the replacements. Use them rather than a fresh
paraphrase; `asgard-cli size <shape>` produces the same wording from a count -
the plain reading is its second output, and it is not optional.

| what it is | what the customer is told |
|---|---|
| a read-only Toolset with four query Workflows | 四個查詢 / four kinds of question it can answer |
| a SkillSet carrying status vocabulary | 一份狀態對照 / a translation of your own codes |
| a SkillSet built from their manual | 手冊裡的欄位與規則會變成 AI 知道的東西 / the manual becomes something it knows |
| SourceSet + Syncer + contextIndex | 產品知識庫 / it reads from your own documents |
| a tool with `requestConsent: true` | 需要人工確認的動作 / an action that stops for a person |
| BotProvider + Workflow + SandboxBlueprint | 對外客服入口 / one way in for people outside |
| an Agent mounting a SemanticLayer | 一個能自己查資料的專員 / it works out its own query |
| a Trigger | 定期自動跑 / it runs on a schedule and only reports |
| a SemanticLayer over a database | 讀得到那套系統 / it can read that system |
| the outbound-IP allowlist request | 把這四個位址加進防火牆白名單 / add these four addresses to the firewall's allowed list. The sentence, not the values |
| browser operation | 沒有介接管道,要照著人的操作做 / no way in but the screen |

Say the browser-operation row plainly. It is the most expensive answer on the
list and the customer is the only person who can change it, by finding an API.

The allowlist row has a rule of its own: ask their network team for the
allowlist, not for "a VPN or an allowlist or a jump host". Asgard is hosted and
the agent runs in a sandbox in that cloud, so there is nothing of ours to place
on their network. A VPN does not apply, and a jump host is not an alternative to
the allowlist: when one is used, the allowlist goes on it instead. A slide offering three
invites their network team to pick the wrong one, and that is discovered a week
later. The slide asks whether the change can be made; the addresses are sent
afterwards, directly to whoever makes it, read from
`../asgard-platform/wiki/operations.md` at the time.

A clumsy name used consistently is better than two names for one thing. If a
phrase here is wrong for a customer's industry, change it once and use the
changed one everywhere, including in the meeting notes.

### Who is pointing at whom - a rule for Chinese decks only

Do not write 「你們」. A deck that says it fourteen times reads as one side
addressing the other rather than as a shared document.

This does not carry over from English, which is why it is easy to miss. English
cannot avoid "you", so nobody notices it. Chinese can drop the subject
entirely, so writing 「你們」 is a choice, and the reader notices it as a finger
pointed across the table.

Three rewrites, none of which needs added politeness:

| instead of | write | how |
|---|---|---|
| 你們現在的日報怎麼做 | 現在的日報怎麼做 | drop the subject |
| 你們文件寫的是 | 測試計畫寫的是 | make their document the subject, not them |
| 哪些數值算異常由你們定義 | 異常門檻可自行定義 | state it as a capability |

Every row of the vocabulary table in step 4 is written this way. The rule is
not obvious even to people who know it. 「請你們把我們的四個位址加進白名單」 became 「把這四個位址加
進防火牆白名單」: same request, no finger.

Prefer the second rewrite. It keeps the quotation - this is what you said -
and removes the pointing. It is also the only one that is correct for the rest
of the room: a meeting has their IT lead and their network admin in it, and
「你們」 addresses them for a document they did not write.

「你們文件寫的是⋯」 looks respectful and is the worst case: it reads as bringing
up an old grievance rather than citing a source.

### The register

Confidence in a proposal comes from specific statements, not from adjectives.
全面、智慧化、無縫、大幅提升 say nothing and cost nothing to write, so a reader
discounts them. One checkable sentence is worth more than a paragraph of them:

    no    全面提升客服效率,智慧化整合各系統
    yes   客服現在要開四個系統才答得出一張報修單的狀態。第一階段目標是在 LINE
          裡問一句就回答得出來,前提是 CRM 給得到唯讀帳號。

Promise something small that you can deliver rather than something that sounds
complete. The customer is going to test this - that is what the test plan they
sent is for.

### The AI tells on itself, and the register rule does not catch it

The section above catches empty adjectives. It does not catch shapes - the
sentence patterns and section endings that mark a Chinese document as
machine-written even when every individual word is fine. A customer who reads
vendor decks recognises those in about four seconds.

Load `.agents/skills/plain-chinese/SKILL.md`. It owns every rule of that
kind, for this deck and for everything else this engagement writes in 繁體中文,
and it says which of them change on a slide - including one that inverts on a
slide and will damage the deck if you learn it from another context first.

Run it after the deck reads correctly end to end, not while drafting. You will
write these patterns anyway - they are what the model reaches for - and hunting
them mid-draft costs the argument, which matters more. The pass is one
question, asked page by page: 這一頁哪裡看得出來是機器寫的?

## Step 5 - what to show, and what must never be shown

### Three kinds of deck, and only one of them is a proposal

| | discovery | proposal | handover |
|---|---|---|---|
| when | the questions are still open | before the decision | after it - training, a readout, a kick-off with their IT |
| audience | whoever can answer them | whoever decides | whoever will operate it |
| what it asks for | answers and access | a yes | nothing - it explains |
| may show a console screen | no | no | yes - it is the point |
| may name platform parts | no | no | the ones they will click |
| built from | open questions, and step 2 run conditionally | steps 1-3 | the same, plus `../asgard-platform/wiki/setup-path.md` |

Decide which one you are making before writing a slide. "Show them how the
agent gets set up" in a proposal is a request for slide 5 - the interaction -
not for the console. In a handover it is the whole deck.

### The discovery deck, because it is the one that gets made wrong

A first meeting usually happens with most of the interview still unanswered, and
the usual response is to force the nine-slide proposal anyway. Slides 4, 5 and 6
then get filled with a shape nobody has the facts for, which the pre-send
checklist correctly rejects, and the deck collapses back into a list of
questions.

A list of questions alone makes a bad meeting: the customer has to guess why
each one matters, and the ones that sound like bureaucracy get deferred. Pair
every question with what it unlocks, so the meeting asks for access rather than
for patience:

    cover
    capability A          their heading and their description. Asks nothing
      A, sub-topic 1      the question | what it produces, one page
      A, sub-topic 2      same, one page per sub-heading of theirs
    capability B          and so on
    close                 see below - one half of this is general, one is not

How to close. The general half: do not close on our own unknowns. Anything we
cannot answer about our own product is asked of the platform team before the
meeting and answered aloud, never printed - the same rule as elsewhere on this
page.

What to close on instead depends on the engagement, and the one worked example
does not generalise. That deck closed on what would be delivered that round,
which worked because there were three concrete things to name. An engagement
whose round delivers "an agent" would produce a thin page, and a thin closing
page is worse than none. If there are concrete deliverables, close there. If
not, close on what each side does next, which at least has an action on both
sides.

Give this closing advice less weight than the rest of this page. The deck that
worked closed on deliverables, and the reviewer specified that page's content;
it was not worked out. So it is one person's one instruction, on one deck, with
no comparison against any other ending, and the agent that built it has said
so.

Use three page kinds and no others. The deck that worked had cover, context,
subject and close, and every attempt to invent a fifth kind, or to group
subjects under headings, was removed. Eight different grouping labels were tried
across many pages and all were deleted.

Put both halves of each pair on one page, side by side.

Separate slides were tried, and the reaction was that the split version was worse: two slides
separate what you are asking for from what it buys, and the customer no longer
sees the trade. A customer looking at a full page of requests has finished
counting the cost before turning to the return. Side by side, both arrive at
once.

    left column    the question, as a plain sentence in a heading
    right column   產出 - what that answer produces

There is a working skeleton beside this file - `discovery-deck.html`, eight
pages, every customer noun neutralised and the question wording left exactly as
it ended up. Replace the content; do not redesign the page shapes.

The second half of each pair is step 2 run conditionally, and it is what makes
this deck work. State the condition in the same sentence as the capability -
"given a read-only account on the CRM and a route to it, this becomes X" - so
the customer sees the access request and what they get for it as one trade.
`../asgard-platform/usecase/` is where those shapes come from, so what you
promise is something somebody has already built.

The shape tempts two mistakes, and this deck must still refuse both:

  - A conditional is not a promise, and it does not let you skip step 4.
    "Given an API, we can integrate all five channels" is a claim about five
    APIs nobody has seen. Name the condition per system, not once for the slide
  - A question the platform cannot answer stays a question.
    `../asgard-platform/wiki/platform-unknowns.md` does not become answerable by being
    written as a conditional

It has no phase plan with dates. That is the proposal, and it comes after the
answers.

### If you are the one reviewing this

A reviewer caught every correction in this section, every time; that is the
only part of this process with a consistent record. It took twenty rounds, and
by the fifth the reviewer was asking how many more pages this would take.

Two things make that cheaper, and both are the reviewer's job rather than the
writer's:

  - say which of the six it is, not just that the line is wrong. "That is a
    question whose answer changes nothing" carries over to the next page; "take
    that out" has to be worked out again on the next page
  - watch the next page for overcorrection. A writer applies a correction as a
    replacement rather than an addition, so the page immediately after
    "describe the scene concretely" is where scene description appears where it
    does not belong. The writer experiences it as following the correction, so
    only the reviewer is likely to see it

### The correction count tracks how much of the page you wrote

The strongest pattern observed across twenty rounds on one deck:

    the customer's own words          zero corrections
    question sentences I wrote        three to five rounds each
    grouping labels I invented        eight rounds, then deleted entirely

The context pages needed no correction at all, and none of their words were
ours: their heading, their description, their own flow diagram, their own list
of what they wanted tested. Our only contribution was the frame.

That result depends on a condition. Those pages worked because that customer's
document was long and included a diagram they had drawn. With a document of two
sentences and three sub-items, the same page fills a third of the sheet, and
the likely next move is to fill the space with the only material available,
which is ours. That undoes the rule.

Leave the page thin. The emptiness is true and worth saying in the room: they
have not worked that capability out either, and a thin page shows that. If they
drew a flow, use it. If they did not, do not draw one for them here - that is
the one thing this page must not contain.

So when a page is being reworked for the third time, do not ask how to word it
better. Ask how much of it is ours, and whether that part needs to exist.

One line we write also never changed: the last line of the outcome column,
saying what happens when it fails. "It says it cannot find it rather than
guessing one." "It says it cannot reach the system rather than giving you
yesterday's number." Those survived untouched, probably because failure
behaviour has one correct answer while a capability can be phrased twenty ways,
so each phrasing of a capability invites another round and the failure line
does not.

### Late rounds catch a different kind of thing

The six below are early-round errors - something added that should not be there.
The last few rounds catch the opposite: things that are correct and still do
not belong.

    "this question decides everything after it"   true. But it is emphasis
    "without this, nothing here is possible"      true. Adding drama to a fact
    "have they registered on the account?"        a correct restatement of the
                                                  question already above it
    eight grouping labels                         each accurate for its group

In early rounds, ask whether a line is right. In late rounds, ask whether it
makes the page heavier without making it clearer. Emphasis, restatement and
categorisation are all true and all surplus, and they are harder to see because
nothing about them is wrong.

### Six ways a question page goes wrong

These come from one deck that took about twenty rounds of correction. They
share a cause: every one of them was writing something that looks like having
done the homework rather than something whose answer changes what happens. A
page of the second kind is short and each line can say why it is there; a page
of the first kind is long and reassuring.

They are not equally established. They came from one deck and one FDE
reconstructing his own decisions. Where a line below rests on one recollection
or a guess, it says so. Treat an untested one as a suggestion rather than a
rule, and if you find it wrong in practice, report that.

**0a. Every outcome ends with what happens when it fails.**

This comes first because it is the cheapest and the most certain. "It says it
cannot find that number rather than guessing one." "It says it cannot reach the
system rather than giving you yesterday's figure."

Those lines were written once and never touched through twenty rounds that
rewrote half of everything else, because failure behaviour has one correct
answer and a capability has twenty phrasings. A line with no room for choice
cannot be worded wrong, so it costs one line and does not come back.

It is also the most persuasive thing on the page. In a support scenario "it says
it cannot find it, it does not invent one" carries more weight with a customer
than any description of what it can do.

**0. A slide is not an agenda, and it is not a tracking list.**

The other six follow from this rule, and an FDE said it would have stopped him
where a test would not:

> Ask "Who is responsible for this?" in the room; do not print it. Asking it
> is professional - it says you intend to follow through, and it tells you who
> to chase afterwards. The question is fine; a slide is the wrong place for it.

    ask aloud     who issues the account, who maintains it, who does it today
    put on a page only what changes what we build

The test is where the answer goes, not what it says. If the answer is somebody
we chase after the meeting, say it aloud. If it is something we build, it can
go on a page.

*(Established. Six occurrences in one deck, and the FDE identified this as the
rule that would have stopped him where a correctness test would not.)*

This rule works because it does not require deciding the question is wrong.
Half the times this went wrong, the FDE was copying an instruction from this
material rather than making a judgement, and a test you have to remember to
apply does not fire when you do not think you are choosing. A rule about where
a correct question goes does fire, because it does not ask you to overrule
anything.

**0b. What a question that works looks like.**

Everything else here is a counter-example. This one survived twenty rounds
untouched:

> 現在要看各平台庫存,是一個一個後台登,還是有一個地方全看得到?
> *(To see stock across your channels today, do you log into each back office,
> or is there one place that shows all of it?)*

It does three things, and the third is why it earns a page:

  - it asks what they do now, not what they have. People answer questions about
    their own behaviour immediately and accurately; they answer "do you have an
    OMS" with "no", because their name for it is not ours
  - it offers two concrete possibilities, so the answer comes at once rather
    than after a pause
  - the two answers lead to different designs. One consolidating system is one
    integration; separate back offices are six

The counter-example is needed, because the rule alone still produces the wrong
question:

    no    有沒有 OMS 或電商中台?
          do you have an OMS or a middleware layer?
    yes   現在要看各平台庫存,是一個一個後台登,還是有一個地方全看得到?

The first gets a "no" - not because they have nothing, but because their name
for it is not ours.

*(Established: this exact sentence went through twenty rounds of correction
without being touched.)*

**1. Asking what changes nothing.** Six of the twenty. Who maintains the
documents, who approves the firewall change, who issues the account, whose name
the ticket goes under, who does the daily report and how long it takes.

The two columns will not be the same length, and should not be. The left is
what they have to give; the right is what they get for it. There is no reason
those are the same size, and a page with three lines on the left and five on the
right means this item needs little, not that something is missing.

Two columns create a pull to make them the same length that nothing else on a
page does. That is a property of the layout rather than of one deck, which is
why this paragraph is here.

*(The observation that a three-line column beside a five-line one reads as
unfinished is one person's account of one deck. If it does not feel that way to
you, the paragraph below still stands and this part does not.)*

Two things follow, and the second is more specific:

  - if you are adding a line and cannot say what its answer changes, the page
    is finished, not short
  - *(One case, from recollection. If a short left column does not in fact feel
    unfinished to you, ignore this.)* padding happens when a column is short
    and a cheap true sentence is available. The pages that were not padded had
    short left columns too - one had a single line. The difference was that no
    cheap sentence existed for them. "Who is responsible" is the cheapest true
    sentence available about any system: always true, always sensible, and it
    sounds like concern

The test below is for reviewing, not for writing. It works only when you know
you are making a choice, which was two of the six times, both while adding a
line. The other four were copying or complying, and no check you have to
remember to run fires then. Use it when re-reading a page, and see the review
section above.

The test: assume they give you the most specific answer possible, and ask what
you would do differently. "Ming issues it" and "Ming spends five hours a day on
it" are perfect answers that change nothing. Three of the six were copied out
of this material, not reasoned into existence - from filter 0 saying to ask who
issues an account, from an instruction to get the name of whoever approves a
firewall change, and from the LINE integration needing an owner on their side.
The first and the third are correct for tracking; the second is not, and
`../asgard-platform/wiki/operations.md` refuses it outright - what it tracks is
whether the allowlist can be changed and whether it has been, never who signs.
None is slide content, and each now says so where it stands, because somebody
copying reads one place and not the canonical one.

A third came from overcorrecting. Told a minute earlier to describe the scene
concretely, the FDE added "whose name does this ticket go under", which in many
ticket systems is a design consequence and so felt like the scene-thinking that
had just been asked for. That one is the hardest to catch, because at the time
it feels like fixing the page.

One habit helps, and it works because you use it when you are not defending
anything:

> After acting on a correction, re-run the check you had before it, not only
> the correction.

The failure is that a correction replaces the earlier check rather than adding
to it. "Am I describing the scene? Yes" ran; "does each line's answer change
anything?" never ran again. Re-running the older one takes a moment and happens
before anything is shown to anybody.

*(Untested. Nobody has run this habit deliberately; it is reconstructed from one
incident.)*

Do not expect it to prevent this. It reduces rounds. The reviewer caught all
six of these, every time, so that path works, but it costs patience, and by the
fifth round the reviewer was asking how many more pages this would take. Judge
this habit by whether the count comes down, not by whether the error stops.

**2. Asking, then supplying the consequence yourself.** Five of the twenty:

    is there a test environment?   ...and if not, may we read production?
    is there an existing binding?  ...if not, they can give us an order number
    is there a controller?         ...if not, this costs weeks more

Ask the customer, in the meeting, and stop talking. The consequence is theirs
to state. Supplying it answers for them, and every one of those additions
invented a branch that does not exist - "no test environment" does not mean no
integration.

**3. Inventing groupings, then bending content to fit them.** Eight different
group labels across many pages, and content distorted to sit under them: three
things that are all required presented as a ladder, a question filed under
"things you can give us".

Do not look for better labels; remove the need for them. Write the question as
a complete sentence in the title, put supporting detail under it, and leave the
detail empty when there is none. A title that is a real question stands on its
own. The labels only existed because the content had been cut into fragments
that then needed grouping.

**4. Printing the narration.** Nine caption lines, all deleted - and then the
same sentence came back as a label above the title, as a line under it, and as
the third column of a table with no class on it at all. Removing the slot does
not remove the sentence, so the rule is not a list of banned containers: it is
`references/design.md`'s "the title is the argument", and it is stated where a
slide is written rather than here.

**4b. Counting what we deliver instead of describing how it behaves.** The same
number, rejected one way and accepted the other:

    no    四個查詢:客戶資料、已登錄產品、報修案件、維修進度
          four queries: customer record, registered product, ticket, repair status
    yes   它只查這四件事,不會自己去翻別的資料
          it looks up these four things and nothing else in your systems

Same four, same systems. The first is an inventory of what we hand over; the
second is what it does in front of their customer, and it answers the question
they were actually holding: whether the thing will go searching through their
data.

This is the subject rule applied to a number: the subject has to be theirs,
and the quantity has to be expressed as behaviour.

**5. Getting the subject wrong.** A page described the approval screen as
showing "the customer's name, product and fault description" - which is the
shape of an internal supervisor reviewing somebody else's record. The actual
scenario was the customer, in their own chat, being asked whether to open the
ticket.

Check who the subject is on every line. On an anonymous channel the person
approving a write is usually the person in the conversation, not staff.

**5b. A write with no field saying who it is for.** The counterpart to 6 below,
and the same boundary confused in the other direction.

    ours     which field on the record says which of their customers it is
             for. The agent fills it, so it is a data question
    theirs   whose account the record sits under, whether their system routes
             or counts SLA by creator. Do not ask

This is easy to get backwards because a write makes every question feel
responsible. It was asked wrongly the first time.

**6. A self-service lookup with no identity.** The deck offered "if there is no
account binding, the customer can give us their ticket number" - and presented
it as a clean scope cut.

Ticket numbers are usually sequential. A lookup keyed on one alone lets anybody
enumerate other people's cases, and it is not authentication. Any self-service
query on an anonymous channel has to say what identifies the person. "A number
they type" does not answer that -
`../asgard-platform/guide/read-path.md` has what a real one looks like, and it
carries the failure mode: a query that forgets to filter on the injected
identity fails only on the anonymous path, which is the path nobody tests.

### The discovery deck's own rules, which are not the proposal's

Three rules elsewhere on this page belong to the proposal and produce a bad
discovery deck when applied. Each has been tried and rejected by the person who
had to present it.

A page you created must say where it came from. Splitting one of their items
into two pages is often right - one of them carried a security question that
deserved its own - but a title that is not theirs makes the page look like a
demand we invented. The reaction to one was "stop drifting the titles".

    the title    a phrase that does appear in their document, even when the
                 split is ours
    the lead     "your document's item 2 says: ..." - so the page traces back

Titles: use the customer's own section names, unedited. The ghost deck test -
titles as assertions - assumes you have facts and a conclusion. In discovery you
have neither, and an asserted title causes two problems:

    no    「你們要測的三件事,卡點都不在 AI,而在能不能讀到你們的系統」
    no    「情境一能不能做,取決於 CRM 和 RMA 能用什麼方式讀」
    yes   「測試情境一:AI 客服與 CRM / RMA 整合」   ← their document's heading

The first states a conclusion before asking the questions that would support
it, on slide 2, contradicts the customer's framing in front of them, and
pre-empts what the question slides exist to find out. The second is a smaller
problem: 情境一 is our numbering. Their document gives the whole heading, and
using it verbatim costs nothing and gives the meeting a shared vocabulary.
They recognise the slide instead of learning what our number refers to.

A customer who wrote a document has already classified their own problem, in
the words they use internally. Use their classification.

Their own words go on the slide, at whatever length they wrote them. The
density rules work against this.

A proposal is us talking, and cutting words saves the reader's time. A
discovery deck is the customer talking - we are putting their own document on
the table so everyone can look at it together - and compressing it distorts it.

The people in the room are the reason. The person who wrote the plan is there,
but so are their IT lead, their network admin, their e-commerce manager, and
none of those read the document. A five-bullet summary is legible only to its
author; everyone else sees questions with no source. Their own 情境說明,
verbatim, is a page every person in the room recognises as theirs.

So give each capability a context slide before any question slide: their
description as they wrote it, their sub-items, their diagram if they drew one.
That page asks nothing. It gives the pages after it their context.

    the one-line-per-bullet rule    a proposal rule. Does not apply here
    the three-to-five items bound   a proposal rule. Does not apply here
    a paragraph of theirs           goes on whole

Do not design the page list. Their document already decided it: three
capabilities with three or four sub-items each are those pages. Working out how
to group them or how many pages is right re-decides something already settled,
and it took several rounds in one deck.

Density: one sub-topic per slide, following their sub-headings. Three to five
items is the proposal's density rule. Applied here it compresses a whole
capability's questions into one table, and five questions with no context read
as a questionnaire - three such tables read as an audit.

Their document is already broken down: a capability with four sub-items is four
slides, each saying what we need and what it unlocks. More slides are fine;
compressed slides are the problem. This deck is worked through line by line in
the room, and a page per subject makes that possible.

A sentence about the slide is not content, wherever you put it. There is no
caption, callout or lead field to fill any more - `references/design.md` says why
- but the sentence does not need a field, and it takes these three shapes:

    no    「以上引號內文字出自你們 8/31 的測試計畫」     where the material came from
    no    「今天要談的,是每個情境要拿到什麼才做得起來」   what we are about to do
    no    「這一頁說明三個情境的差異」                  what this slide is

All three are things you say out loud. Printing what you are about to say wastes
the line and tells the room you are reading it. If a reader would not copy it
into their notes, delete it, and delete it rather than moving it, because the
next container will take it just as readily.

The last two slides show the two filters: what came out as deferred scope, with
what has to be answered before it comes back, and the questions that are ours
to chase rather than theirs - the ones
`../asgard-platform/wiki/platform-unknowns.md` says nobody has settled. Both are written as sentences a
reader outside this repository can follow, not as counts of rows.

### What a handover deck actually walks through

`../asgard-platform/wiki/setup-path.md` is the order, written for exactly this: where a
credential goes, what gets built from it, the agent's configuration, and why
there is no import step on the Sindri side. Two things on that page prevent
common errors in a handover deck:

  - An HTTP API's credential has no home under Settings. Data Source is nine
    database providers; Connection is OAuth to five named services. A deck that
    shows their API key going into Data Source teaches them something that
    does not work
  - Nothing is published to Sindri manually. Every Managed Agent is there
    already; enabled serves, disabled does not. A slide with a "publish to the
    hub" step describes a button that does not exist

### Screenshots

A discovery deck almost never needs one, and you can know that before starting.
Its pages fill with the customer's own words, and one evidence shape per page
then leaves no room. One deck's images were downloaded, cropped and checked one
by one, and none was used.

Decide whether the deck takes images before doing any of that work, and
default to no for a discovery deck. Cropping and checking are the slow part,
and the time is lost if the images are not used.

There are none in this repository and none in `asgard-cli`. They live in the
product documentation and are fetched by URL:

    https://docs.asgard-ai.com/img/docs/<path>

`../asgard-platform/wiki/screenshots.md` is the index - every picture, what it shows,
and which situation it is for. Read it rather than browsing the docs site: it is
grouped by what you are trying to say, which the documentation is not.

Two things in it are worth knowing before you go looking.

A proposal usually wants the case-study set, not the console. The usual
instinct is to screenshot the admin screens, and those show our side of the
work. The four `sindri-retail-stockout-transfer/` images are one continuous
story from a user's question to a completed action, and the third of them is
the approval dialog, which is the only way anybody has found to explain the
governance gate on a slide without a paragraph.

A handover wants the setup path, and `../asgard-platform/wiki/setup-path.md` is the
narration for it. `agent-hub-managed-agent/create.png` is the one screen most
handovers need.

The console is in English; only the documentation's captions are zh-TW, so a
zh-TW deck can carry these and write its own captions.

Open every one before it goes on a slide. Nothing records when any of them
was captured, so a form that has since changed looks exactly like a current one,
and a stale screen in front of a customer who uses that screen daily is worse
than no picture. A screenshot is also the most common way a credential or another
customer's name reaches a deck, in a corner nobody read.

Download it into the deck's own `assets/` rather than hot-linking the docs site
or a path on somebody's machine: the first breaks when the docs are rebuilt, the
second the moment anyone else opens the deck.

### Links, on a deck that has any

A customer proposal rarely carries one. An internal or partner deck is mostly
links, and all three ways of getting one wrong render identically - only
clicking the link catches them.

- Do not assemble one by hand. `asgard-cli links` prints what this checkout
  is bound to, from the ids already on disk, and prints nothing it would have to
  guess. What it says it did not print is left out for a reason - accept that
  rather than building the URL yourself.
- A link points at the page being discussed, not at the site it lives on. A
  root URL usually means nobody looked up the real one.
- The name is the link. A row that already says what the thing is does not
  also spell the URL out beside it.
- Do not infer who can open it. A private repository is evidence about the
  repository and about nothing else; partners in the same org have access.
  Ask who is in the room before removing a link, because replacing a link with
  prose looks careful and is a mistake.

### What must never be on a customer's screen

- **Implementation nouns**, in a proposal. No CR kinds, no `sl-` / `dc-` / `ss-`
  prefixes, no namespaces, no Helm, no chart, no CRD. The customer bought an
  outcome. A handover may name what they will click - Managed Agent, Drive,
  Skillset - and still never needs the CR behind it.
- **The second person, in a Chinese deck.** 「你們」 on every page turns a shared
  document into two sides. See step 4 - it is a register rule rather than a leak,
  and it does not apply to a deck in English.
- **Anything only we can resolve.** Question numbers and `REQ-` ids are the
  obvious form, and the rule covers more than numbers: "the five the filters
  removed", "that mechanism mentioned earlier" and "item 3 above" fail the same
  way. The test is whether a reader who has only this deck can resolve the
  reference. No question numbers, no `REQ-` or
  `TASK-` ids, no "the items the two filters removed", no "out of scope table".
  These are the easiest words to leak, because you have just finished writing
  them and they feel like the content, but they are an index into files the
  customer cannot open, and a slide made of them is unreadable to everyone
  except whoever built the records.

  The test is the same one as for platform nouns: would a reader who has never
  seen this repository know what the slide says? Write what the question asks,
  not the row it lives in.

      no    第 14-16 題:平台未解項目
      yes   有三件事要回去跟平台團隊確認,確認後書面回覆

      no    被兩道篩選刷掉的五項
      yes   這五項這一輪不做,以及各自要先有什麼答案才會回來

  This also applies to an outline you show internally before the deck exists.
  An outline written in row numbers cannot be reviewed by the person who has to
  present it.
- **Another customer.** No name, no logo, no screenshot, no "we did this for a
  retailer with 200 stores" that is recognisable. Their engagement is not ours to
  use.
- **Credentials and coordinates - theirs and ours.** No hostnames, no connection
  strings, no account names, not even in a screenshot's corner. Check the
  screenshots.

  Not Asgard's outbound addresses either. They belong in the firewall
  ticket, given to the person making the change - not on a slide, not in the
  repository, not in a mail thread that gets forwarded. They can change, and a
  copy will not; a stale allowlist drops the customer's connection.
  `../asgard-platform/wiki/operations.md` is the source to read them from each time. What
  goes in the deck is the request - 把這四個位址加進防火牆白名單 - and never
  the values.
- **A capability nobody has verified.** If it is not in `references/`, in a
  meeting note, or measured, it is not a promise.
- **An implementation noun inside a screenshot.** The rule above also covers
  images: an approval dialog naming `ts-wms` and
  `create_transfer_order`, or a console screenshot with the whole build-platform
  navigation down its left edge, breaks it exactly as a slide title would. Crop
  before using; `../asgard-platform/wiki/screenshots.md` marks the ones known to need it.
- **Precision we do not have.** "Around 10 minutes, from your own figures" beats
  "11.4 minutes" when 11.4 came from one afternoon's sample.

## Step 6 - write it

Read this before the first slide. The next two sections describe mistakes that
cost an FDE a working afternoon and are invisible until they happen; everything
after them is content.

### Never edit a slide to satisfy a checker

This is the most expensive mistake made with this skill so far, and while it
happens it looks like diligence.

A content check reported a scene line as missing. It was there - the checker
collapses whitespace when comparing CJK, so a cover date running straight into
it made the string unfindable. To turn the check green, the FDE removed the
sub-numbering from every scene line. The check went green, and every sub-topic
slide then claimed the wrong level: the customer's own numbering says the
scenario, and the slides were now saying it about a sub-topic of it.

The user caught it. The checker could not, because the regression was in
something it does not look at.

    a check that fails on text you can see    the check is wrong. Investigate it
    a change to the slide to make it pass     never

On a discovery deck, do not run the content checks at all. They are the right
tool for a document whose text was fixed before layout, and a discovery deck is
not that document. Running them produces failures you then have to decide to
ignore, and the section above shows what happens when somebody stops ignoring
one.

Where they are run - on a proposal - they never go fully green on slides
anyway: `audience` and each slide's `layout` are schema fields and are not
printed anywhere. Read the list, fix what is missing, and stop.

### Four things about the template that nothing warns you about

  - Slides are fixed height with `break-after`. Anything added to normal
    flow can push content onto a new page with no error - one deck went from
    14 pages to 18 by gaining a line of links. Footers, links and page marks go
    in absolutely positioned elements, and re-count the pages after every
    edit
  - `<b>` does nothing. The CJK faces embed weights 400 and 500 only, so
    bold silently falls back to normal. Write `font-weight: 500`
  - Do not keep a second copy of the content. For a proposal, whose text is
    settled before layout, writing the structured form first and generating from
    it is right. For a discovery deck it is not - see below - and a half-kept
    second copy is worse than neither

### Borrow the layout language, not the process

    take        the template, the palette, the serif hierarchy, the page
                geometry, the CJK font handling, the PDF output
    do not take the structured-content step, or the content checks built on it

That process is a good one, and it is designed to prevent exactly what went
wrong here. Writing the content first, in a file with no columns and no pages,
makes it impossible to add a line because the left column looks short. The
mechanism existed, it was documented, it was skipped, and then the error it
prevents was made repeatedly.

The reason not to take it is what a discovery deck is. Its content is not
settled before layout; it changes each time a page is rejected. Many rounds of
revision reworded about half the question sentences, and those sentences are
what a locked content file exists to fix in place. Maintaining two files through
those rounds doubles the work of each one.

Two sources also drift. In that engagement the content file caught exactly one
thing: an inconsistency between itself and the HTML, which could only exist
because there were two sources. The single source is the HTML.

The cost: without a structured source there is no coverage gate, so nothing
verifies that no fact was dropped while filling the layout. That is acceptable
here for the same reason - with one source there is nothing to drop between;
an edit is the edit.

Layout checks still run, and they are worth it. Style and placeholder checks
and the page count caught real defects in that deck: a line-height over its
bound, and the silent overflow that turned 14 pages into 18. Those measure the
page, not the words.

### The rest

The design language is `references/design.md` beside this file, and the
content contract is `references/slides.json`. Both are here so that building a
deck needs nothing but this repository: the palette, the type scale, the slide
classes, the print rules and the layouts, taken from the `kami` skill's design
system and reduced to the half a deck uses.

Take the layout language, not an authoring process. If your environment has
`kami` installed you may render with it, and the decision above still holds -
which rules apply differs between a proposal and a discovery deck, and a layout
skill's own density checks do not know the difference. This skill has already
decided what goes on each slide.

For a discovery deck there is a working starting point beside this file:
`discovery-deck.html`, plain HTML with no separate content file,
every customer noun neutralised and the question wording left as it ended up.

The four rules worth knowing before you write, which `references/design.md`
carries in full:

- **The ghost deck test**, for a proposal. Reading only the titles, in order,
  must tell the whole argument. If a title is a topic label - "Current
  situation", "Architecture" - it fails. Titles are assertions: "Support answers
  one ticket by opening four systems". This inverts for a discovery deck,
  where you have no facts to assert and the customer's own section headings are
  the right titles - see step 5.
- **One evidence shape per slide.** A slide is a claim plus one thing that backs
  it: three to five items, or a chart, or a quote. Not two of them. Both the
  three-to-five bound and "trim each bullet to one line" are proposal rules, and
  a layout skill's own density checks will enforce them on a discovery deck where
  they do not belong - the checks go green on a deck that has compressed the
  customer's document into something only its author can read. See step 5.
- **There is no caption, callout or lead field.** A line about the slide is not
  content wherever it sits, and `references/design.md`'s "the title is the
  argument" is the test: cover the title, and if the sentence still says
  something the slide did not show, it stays. A slide that looks empty is not a
  reason to write one.
- The content contract is `references/slides.json`, and the layouts are
  `cover`, `chapter`, `content`, `quote`, `metrics`, `close`.

Write the deck in the customer's language. Hand over a PDF; produce an editable
file only when the customer has said they want to edit it.
`references/design.md` says why HTML to PDF rather than a presentation format,
which is about CJK rendering rather than preference.

With no typesetting skill installed the deck is still typeset, because the
design language is `references/design.md` and `discovery-deck.html` applies it.
Plain Marp markdown - `---` between slides - is the fallback below that, and
then say in the handover that it has not been typeset. Either way, do not
invent a second house style: a plain deck that says what it means beats a
decorated one that does not.

## Before you send it

Run question 0 over every slide, one sentence at a time. It is the only one
that has to be run against each sentence rather than each deck, and it is the
one this skill has been corrected on most: the same sentence has come back as a
label above a title, as a line under it, as a caption under a screenshot, and as
the third column of a table that had no class on it at all. The containers were
deleted; the sentence was not, because it never needed one.

Eleven questions, and the last five are the ones that get skipped:

0. Cover the title and read the sentence. Does it still tell you something
   the slide did not already show? If not, delete it. This applies to every
   sentence, including one in a table cell, a closing line or a parenthesis,
   and including one that reads to you as argument rather than as narration:
   that exemption has failed every time, because writers classify their own
   reasoning as argument. Delete it rather than moving it. Moving it produced
   four of the recurrences above.
1. Do the titles alone tell the argument?
1b. Which of the three decks is this? A proposal made while the interview is
    still open is a discovery deck with the wrong slide order.
2. Is every number attributed - their figure, our measurement, or absent?
2b. Does any title we wrote count something instead of naming it?
3. Does slide 7 say what is not in scope, specifically enough to point at later?
4. Is there an implementation noun anywhere, including in a screenshot?
5. Is another customer recognisable anywhere, including in a screenshot?
6. Could we deliver everything on slide 8 if they said yes today? If any part
   rests on something nobody has built or verified, first ask whose unknown it
   is: theirs moves to slide 9, ours goes back to the platform team before the
   meeting and is answered out loud, never printed.
7. Is there a decision record for the shape, written before the deck?
8. Has every claim on slides 4, 5 and 6 been held against
   `../asgard-platform/wiki/platform-unknowns.md`? Anything on that list is a slide 9
   question, not a feature.
9. Read slide 4 aloud to someone who does not work here. Do they repeat it back
   correctly? That is the whole plain-language test, and it takes a minute.
9b. Is there a question number, a `REQ-`/`TASK-` id, or a phrase like "the items
    the filters removed" anywhere on a slide - or in the outline you showed
    internally? Those name rows in files the reader cannot open.
9c. Chinese deck: count 「你們」. Every one of them is a choice, because the
    subject could have been dropped. Most should be the document instead.
10. If it shows a console screen: was every screenshot opened, is it current,
    and is this a handover rather than a proposal? A proposal showing admin
    screens has usually skipped `../asgard-platform/wiki/screenshots.md`, where the
    customer-facing set is.

## The outline beside the deck

Write `outline.md` beside the deck, and write it before you edit a slide.
`outline.md` beside this skill is the worked one, for the deck beside it. A
deck without one is unusable six months later, because nobody can tell whether it
is what was proposed or what was agreed. That label is the smaller benefit: the
outline is where a page's claim is held against its source, and a slide is the
one artefact here that carries no record of its own sources.

It opens with who it was for and on what date, then the argument in its two or
three main points - not the page list. If those points do not make sense read
alone, the deck has no argument yet.

Then one row per page: what that page does, and what it rests on. The last
column is the important one:

    a diagram      what it actually draws, in enough words to redraw it
    a screenshot   the exact path it was taken from - product, page, tab - and
                   what is visible on it
    a link         where it points
    a chart file   **the path in this repository, and the rule that the slide
                   changes when that file does**

The chart-file row matters most. A slide that copies a chart's own logic -
the five outcomes a tool's `proc-response` sorts into, the fields an Agent
carries - is a copy, and this repository removes copies with no pointer back
everywhere else. Write the path, and write that editing the chart means editing
the slide.

Two sections close it:

- How each screenshot was captured, and when to retake it. A platform
  release ages every screen, and an out-of-date screen looks exactly like a
  current one, so open those pages once before you present.
- What is not done, each item pointing at the open question it is waiting
  on. A deck shown with a blank box in a diagram is honest; the same deck with
  nothing recording why leaves a question nobody asks again.

**Checked:** The CR vocabulary in the translation table is real at
asgard-kube `cbd8d70` - `SkillSet`, `SourceSet` + `Syncer` + `contextIndex`,
`BotProvider` -> `Workflow` -> `SandboxBlueprint`, an `Agent` mounting a
`SemanticLayer`, and `requestConsent` on a gated tool all exist and mean what
the right-hand column says. Every shape it tells a deck to take is one
`../asgard-platform/usecase/` assembles from a deployment in production, which is what
makes an estimate on it an estimate rather than a guess.

**Unchecked:** the argument about what persuades - the order a proposal argues
in, what must never be shown, which questions are worth a customer's time -
comes from one deck for one customer. Read the prohibitions as earned by that
deck's failures and the positive advice as untested.
