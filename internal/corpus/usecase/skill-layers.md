---
group: The mechanism, and the scale
description: a runtime skill per area over one middleware and one external platform - what the layering looks like past the first skill
---
# A skill library at full size, and what the layers are for

Runtime skills over one commerce middleware and one external commerce
platform. It is the largest skill library here and the only one that answers the
question `../usecase/skill-set.md` does not: what a skill is for, and when you need
another one.

**Seen in:** a middleware deployment with one SkillSet per Agent and one Toolset.
Capability is mostly skills; two of its agents also bind a read-only order and
inventory SemanticLayer.

**Checked:** against asgard-freyr-skills `2ff0e1e` - the declared
`skill-layer` of every one, the dependency the README states, the write gates,
the target-store check in `shopline/access.md` and the four-level exploration policy.

**Unchecked:** whether the back-office map still matches the product. It was
built by field exploration, nothing tracks drift, and only the live back office
can show it.

Read the platform side first: `../wiki/tools.md` covers how MCP Server, Skillset
and Plugin differ. `../usecase/skill-set.md` has the CR wiring. This page is
about the content.

## When this shape, and when not

Use it when the agent's job is to operate someone else's system, and the
system's behaviour cannot be inferred from its interface. A SemanticLayer answers "what is
the number"; this answers "what happens if I press this, and should I".

The deployment that produced it reads almost nothing through a layer. If the
capability is doing things rather than reporting them, the skills are the work
and the CRs are packaging.

Do not use it for what a query answers exactly. A count, a price, a status - those
belong to a layer or a query tool. A skill telling the model a number makes it
paraphrase a figure it should have read.

## Three layers, and only two are declared

The frontmatter carries `skill-layer` with two values; the library's own table
describes three. Both are right. Keep the third distinction even though only the
prose records it:

| layer | answers | example |
|---|---|---|
| `baseline` | how to call it safely - endpoints, auth, paging, errors, known traps | the API's operations, one runnable call each |
| `customization` / domain | what each screen does, and how a task completes | the back office: what is on each page, which flow gets a job done |
| `customization` / role | how one job holder reasons - what to compare, what to trust, what to escalate | the inventory manager's method; the sync manager's failure taxonomy |

The role layer is the one people do not think to write, and it carries most of
the value. It holds judgements, not facts:

    which numbers in this screen are real, and which are placeholder or fixture
    when the system heals itself and when a person has to act
    "the sync page cannot retry; a resend goes through the exception queue or a single-product resync"

None of that is in any API contract. It is what a competent operator knows after
six months, and without it an agent with full API access does wrong things
confidently.

Layers depend on each other and the dependency is stated. Two skills here
require the baseline one, and deploying without it breaks the whole flow: the
entry point that hands a human into a browser to complete a login lives on the
baseline skill's own operation. Ship the layers a skill declares, or ship
none.

## Disambiguation as the entry point

This is the most reusable idea in the library, and it sits at the top of a skill.

Two systems each have a "store name". They are independent fields with no
synchronisation, and a user saying "change the store name" may mean either. So
the skill starts with disambiguation, not an action:

    writing    unsure which system? ask, before doing anything
    reading    answer for both, and label each with its own system's term
    confirming name the system, and state that the other side will not change

Any integration where two systems name the same thing differently needs this,
and most do. `../guide/requirements.md` calls the cross-system
mapping a skill's job; here it is a canonical table, plus a signpost to it from
the other skill so a reader arriving from either side finds it.

## Writes: three gates, and a class that no confirmation unlocks

    reversible          the write can be undone
    observed contract   somebody has actually run this call, not inferred it
    confirmed per use   the user approved this instance

All three, together. And a class that is refused regardless:

    delete, publish, batch, payment, permission change

Confirmation does not unlock the second class. That is stricter than the
platform's own approval gate, which will run whatever a person approves, and it
belongs at the skill layer because the skill is the only thing that knows which
operations are irreversible in that system. One deployment keeps one exception,
a batch create the product owner approved, and it carries its own chain of
checks; nothing after creation is exempt.

A token that reaches several stores does not say which one to write to. In
that deployment the target store comes from the brand's own configuration,
not from the user: the configured store is checked against the system's own
list of stores, the write is refused if it is not there or if the
configuration names none, the user confirms before the write, and the list
itself is never shown, because the token can see other tenants' stores.

## Provenance per operation

Every recorded API is marked executable or for recognition and navigation
only. The second means: never triggered, no observed contract, and `method` and
`path` are left as `—` rather than guessed.

The extracts here follow the same practice. A row saying "not verified" is
worth more than a plausible guess, because the reader knows which rows to trust.
A skill full of inferred endpoints is worse than a short one, though it looks
better.

## When the request is outside what is recorded

A graded policy rather than a flat refusal:

    read-only exploration                    allowed
    find the way and take the user there     allowed
    an unrecorded reversible write           five conditions, all of them
    anything irreversible                    never

The condition that matters most in the third: go through the UI form, never
hand-assemble an API request. The form carries the system's own validation, and
an assembled call skips the checks that make the write safe.

If a skill has no such policy, the model invents one each turn.

## What it cost someone

The back-office material was built by watching network traffic, because the
vendor publishes no back-office API documentation. There was no generated source
and no shortcut: 88 L1 page entry points, each declared for whether anything deeper
sits beneath it, and 160 rows of operations covering in-page tabs, dialogs,
editor panels and apps inside nested iframes.

That is the number `../usecase/browser-operation.md` means when it says a
capability was "a skill describing 88 pages plus everything the menu cannot see".
Quote it as the size of the map, in the same sentence, and never as the cost
of the integration. That mistake has been made on a slide in front of a
customer, and `../usecase/browser-operation.md` records what it cost. The map is
the prerequisite; what gets built on it is separate and larger.

## Read the platform side first

`../wiki/tools.md` for what a Skillset is;
`../usecase/skill-set.md` for the CRs;
`../usecase/browser-operation.md` for the shape this library serves;
`../wiki/taiwan-channels.md` for the channel it integrates.
