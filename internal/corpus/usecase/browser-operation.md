---
group: Read paths
description: a system with no database and no API - only a web UI. The last resort
---
# Operating a system through its web UI

The last resort, for a system with no database you can read and no API - an
appliance with only a web console, a vendor back office, an internal tool nobody
has an integration for.

**Seen in:** a deployment operating a commerce platform's back office, where the
capability is a skill describing 88 pages plus everything the menu cannot see.

**Checked:** against the skills repository it describes, which is not named here - the four map files under `shopline-backoffice/references/`, `scripts/validate.py` for the map schema, and `requirements/tasks/TASK-001-shopline-backoffice-operation.md` and `requirements/tasks/TASK-006-backoffice-operation-map.md` for how the maps were produced and the three crawl misses - and against asgard-kube `3da0365` `pkg/apis/asgard/v1alpha1/types.go` for `browser` on the Agent and the SandboxBlueprint.

**Unchecked:** the guidance on building the maps comes from one capability built by one team, and no second deployment has repeated it.

Read the platform side first: `../wiki/agents.md` -
what a Managed Agent and a Flow Agent each are, and which the audience decides. This page assumes you have.

What this shape costs to get is `../needs/browser-operation.md` - what has to come
from the customer before any of it can be built.

## When this shape, and when not

Take it only after establishing there is no better route:

    a database you can read   the agent composes its own queries      best
    an HTTP API               a fixed contract, reviewable, gateable  good
    another protocol          the agent runs the client in its
                              sandbox: SNMP, SSH, a vendor CLI        good
    a web UI                  this extract                            last

It is last for reasons that do not go away: it breaks when the vendor changes
their UI, it is slow, and verifying that the agent did the right thing is hard
in a way the other two are not.

Ask before designing it:

- Does the vendor have an API that nobody has asked for? Very often the console
  is a client of one - open the browser's network tab and look.
- Is there a command-line route? The sandbox can run any client that exists,
  so SSH, SNMP or a vendor CLI beats driving a web console on every axis:
  faster, verifiable, and it does not break when the UI is restyled.
- Is there an internal system that already holds this data - a monitoring
  system, a middleware layer, an existing integration?
- Would the customer accept a person doing the write, with the agent doing only
  the reading and the analysis?

If the answer is still the UI, the work is substantial. Budget for producing
the maps below, and say so in the plan.

## What has to exist before an agent can operate anything

The capability is a set of reference documents the agent reads, and producing
them is most of the work:

    references/page-map.md         every page the menu can reach:
                                   area | page name | route | menu path |
                                   what you can do here | deeper layers
    references/operation-map.md    everything the menu CANNOT see: in-page tabs,
                                   dialogs and wizards, editor panels, the apps
                                   inside nested iframes
    references/embedded-apps.md    each cross-domain iframe origin, and how to
                                   navigate the inner app directly
    references/self-exploration.md what to do when a request falls outside what
                                   is recorded
    SKILL.md                       the entry point, with the refusal list

Routes in those files are measured, never derived. Resource ids are
placeholders taken from the live UI, and the skill stores no customer
identifiers.

## Three ways a crawl misses things

A user asked to be taken to the announcement setting. The agent guessed a URL,
was bounced to the home page, and eventually found it by clicking into a page
builder. The post-mortem rejected the single obvious cause and found three
independent mechanisms. Any map you build will have all three unless you plan
against them:

1. Extraction shaped like controls throws away the prose. A crawl that
collects tabs, fields, buttons and column names discards sentences that name
deeper objects - including a banner on an already-visited page saying which
controls are global. The page had the answer printed on it.

2. Crawling follows links, and an editor entry is not a link. The way in was
a `<button>` with no `href` and no accessible name. A link-following crawl
cannot reach what is behind it.

3. "Cannot read the iframe" gets treated as "cannot cover it." The inner
origins had been recorded all along; nobody tried navigating to them directly,
so every page built that way stayed uncovered.

Counting pages visited does not measure coverage. Track what a user can do, and
keep a per-page ledger of whether its deeper layers were reached.

## Designing the maps

### Cover operations, not pages

Track what a user can do, and keep a per-page ledger with a column for the
deeper layers - tabs, dialogs, wizards, editors, embedded apps. An empty column
shows the gap before a user finds it.

### Write the route down as measured, never as derived

Every route in the map came from a link that was clicked. Resource ids stay as
placeholders, replaced from the live UI at run time, and the map stores no
customer identifiers.

### Say what each page is for, in the user's words

The map is read by an agent trying to match a request like "where do I set the
announcement" to a place. A row saying "設定 > 網店" helps nobody; a row saying
what a person accomplishes there does.

### Record the exceptions as their own inventory

Pages whose content comes from another origin, and menu items that leave for a
different system, behave differently enough that they need their own list with
the recipe for reaching each one. Otherwise every future crawl rediscovers that
it cannot read them, and stops.

## Never guess a route

A route may come from exactly two places:

1. a link visible in the current UI - a menu item, a row, a breadcrumb
2. a route already recorded and measured in the maps

Not derived from another route, not by changing an id or a version segment,
not "let me see if this loads".

The reason:

> Guessing wrong and being bounced to the home page is the lucky outcome.
> Guessing a page that is semantically similar but not the one you wanted,
> and then operating on it - that is the failure mode nobody detects.

When the map has no route, the answer is to walk the visible menu again, not to
invent one.

### Pin it in the schema

Where a tool takes a path, make every legal value an `enum` in its
`inputSchema`, and say in the description that ids must be substituted from the
live UI rather than invented. Schema is enforced; a prompt is advice.

The enum is a copy of the web app's routes and drifts when the app changes, so
say in the description that it may lag. Do not let the path list stand in for
the page's own document either: in one deployment an agent answered from the
enum without opening the screen reference, and the prompt now requires reading
the reference first.

## Exploration beyond what is recorded

The records will always lag the system. Grade what the agent may do:

| level | what | who authorises |
|---|---|---|
| read only | navigate and report. Look, do not click things whose effect is unknown | nobody, it is the default |
| reversible action | a setting that can be set back | the user, in the conversation |
| significant action | affects others, or is awkward to undo | the approval gate |
| refused at every level | deletion, publishing or unpublishing, bulk writes, payments, permission changes | nobody. Not reachable by exploring |

The last row applies while the agent is exploring too. Write it in the skill,
and write it in the prompt.

Apply the customer's data-masking rules at every level, including read-only.

## Generate it

There is no CR specific to this shape - it is an ordinary skill, plus a flag on
whatever holds the capability:

    asgard-cli add skillset <system>-ops --repo <where the skill files live>

Then set `browser.enabled: true` on the Agent or the blueprint that binds it,
and bind the SkillSet there. The generated SkillSet already carries the trio and
the field names; it does not write the reference documents, which are most of
the work.

## The skeleton

Two sides, in two places.

The CR side goes where the capability is bound - `templates/agent/ag-<name>.yaml`
for the hub shape, or the blueprint under
`templates/supervisor/<name>/sandbox_blueprint.yaml` for a flow agent - plus a
`templates/skill_set/sk-<system>-ops.yaml` for the trio:

The two CRs spell the same two settings differently, and the blueprint form
is the one that is easy to get wrong: there is no `managed` block on it, and
every field on it is a `value` / `expression` / `template` rather than a plain
boolean or a list - `../usecase/conventions.md` has the rule and the
comma-separated `*Names`.

```yaml
# Agent - a boolean, and a YAML list
spec:
  managed:
    browser:
      enabled: true
    skillSetNames:
      - sk-<system>-ops
---
# SandboxBlueprint, for a flow agent - the same two settings, both as strings
spec:
  browser:
    enabled:
      value: "true"
  skillSetNames:
    value: "sk-<system>-ops"
```

A blueprint's `browser.enabled: false` is not authoritative: if any Agent it
resolves turns the browser on, the sandbox opens it anyway.

The blueprint's other sidecar is `editorServer`, and it is the opposite of
this one. The browser sidecar is a screen the agent drives; `editorServer`
starts code-server in the sandbox so a person can open an IDE onto the
sandbox's own filesystem, at `workingDirectory`, after `initCommand` has run.
Building an agent never reaches for it: the one deployment that sets it runs an
internal article-authoring workspace a person works in, and leaves it off on the
blueprint its customers talk to. A chart that sets it has a person working in
the sandbox by hand; the agent gains no capability from it. If an engagement
ever does need somebody working inside the sandbox, the fields are described in
asgard-kube's `pkg/apis/asgard/v1alpha1/types.go`, and that is the reference;
there is no extract for it.

It is also not the `SourceSetEditorServer` kind, which is a leased editor
onto a SourceSet's volume rather than a sandbox's, is its own CR with its own
lifetime, and is internal to the platform.
The two are both code-server and the names are one word apart, so a reader who
greps for "editor server" and lands on P12 gets an answer about the wrong one.

The skill side is a normal SkillSet trio (`../usecase/skill-set.md`)
pointing at wherever the skill files live:

```yaml
kind: SkillSet
metadata:
  name: sk-<system>-ops
spec:
  sourceSetName: ss-<name>
  searchPaths:
    - <member>/<system>-backoffice
```

Credentials do not go in the skill. A runtime config file written into the
sandbox by a hook is how a session gets its base URL and the user's token -
see `../usecase/flow-agent-supervisor.md` for the hook, including why it
must be `user-prompt-submit` rather than `session-start`.

That is the caller's token, and it is the only kind that arrives this way. A
hook is an expression stored in the CR spec, so a static service key written by
one is a key baked into the chart, and nothing else puts a value into the
sandbox's environment at all. A system that authenticates with a service key is
reached from a Workflow instead, whatever else recommends this shape -
`../usecase/external-api.md` has the fields that was read off.

Where a login cannot be automated, hand the browser to the person. A real
user completing the login in the sandbox is a legitimate step, and better than
storing a long-lived credential.

## Inside the capability, the browser is still the fallback

The choice above is made once, at design time. There is a second one made on
every request, and the reference set that works answers it explicitly: when an
API contract has been observed for this operation, call the API - do not open the
browser.

The console is a client of its own API. Exploration that records the request each
operation makes turns most of the capability into HTTP calls, and the browser is
left for the cases that genuinely need a screen:

| open the browser when | |
|---|---|
| no contract was observed | the operation was never exercised, so nothing was recorded |
| the user asked to be taken to a page | the navigation is the request |
| the user wants company while they work through the console | the navigation is the request |
| the page's data is not on the main API | a nested cross-domain app, served from somewhere else |

Everything else is an HTTP call. Driving a screen to do something an API does is
slower, more fragile, and breaks on the next restyle.

That distinction has to be in the reference set, per operation - each row
carrying either its method and path, or an explicit marker that none was
observed. A set that only says "here is the page" pushes the agent to the browser
by default.

## Skills as their own repository

Once the reference documents grow, they outgrow the chart repo. The shape
that works is three repos with an explicit contract:

    <name>-skills     the skills, versioned and released on their own
    <name>-api        the API contract, if the system has one, as its source
                      of truth. A script vendors it in and generates the
                      operation pages - which are never hand-edited
    <name>-kube       injects runtime config and loads the skills via SkillSet

Keep skills that depend on each other in the same repo, and say so in the
frontmatter. The platform does not resolve dependencies between skills - a
SkillSet lists paths, and a skill whose prerequisite is not also bound simply
does not work.

## Verify

```bash
asgard-cli check
asgard-cli verify <project>
```

Those confirm the SkillSet resolves. Nothing verifies the maps, which is the
part that decides whether the capability works, so verification is its own task:

- a per-page ledger, not a page count. For every page: were its tabs,
  dialogs, editors and embedded apps reached, or is that column empty?
- spot-check the recorded routes by navigating them from a cold session.
- repeat the announcement-setting request above: ask for something the map does not
  name, and see whether the agent walks the menu or invents a URL.

Write an automated check on the maps themselves. The reference set has one: a
script that fails when a page-map or operation-map row lacks a required column
or carries a value outside its fixed set - including the deeper-layers column,
so a page nobody explored says so rather than leaving a blank - and when a local
link does not resolve.

## What producing the maps costs

The reference set was produced by a coding agent driving a browser against the
vendor's test store, with a developer doing the login and staying with it.
Reading was read-only; a write contract was captured by saving a harmless change
and reverting it, then reading the value back. So producing a map is agent time
supervised by a person with a test login, not a person transcribing screens -
and it needs a store the customer lets you save into, which is why
`../needs/browser-operation.md` asks whether a non-production login exists.

The second pass - the operation map for everything the menu cannot see - was
declared and closed on the same day by its own log. The first pass has no
duration recorded that separates the map from the rest of the skill. Whatever
figure is given for the map is the cost of producing the map only - not of the
integration, the capability, or an item on a proposal. It has been quoted one size too large, on a slide, in front of a
customer, where it also read as pressure. The map is a prerequisite; what
follows it is separate and larger. If a number has to be said out loud, say
what it measures in the same sentence.

Nothing regenerates the maps when the vendor ships a UI change. The reference
set regenerates its own system's API pages from a vendored contract with a
script; the vendor back office has no such source, so a restyle means exploring
again by the same method. That is the maintenance cost of every integration
built this way, and it belongs in the plan.
