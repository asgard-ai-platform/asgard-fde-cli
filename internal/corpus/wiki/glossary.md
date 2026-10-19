---
group: In practice
description: "words that mean one thing here, and what the other senses are called. Check it for the word you searched, because a result in the wrong sense looks like an answer"
---
# Words with one meaning here

When a word means two things in the same material, nothing contradicts
anything and the reader takes the wrong sense without noticing. The main
example is "sandbox": the platform's agent runtime, and the customer's test
environment. A paragraph that uses both without saying which leads an FDE to
read the second as the first.

Each word below has one meaning here, and the other senses have their own words.
Check this page before introducing a term, and before using one of these for
something else.

Also check it for the word you searched. The table contains the word, so a
search for an ambiguous term returns this page alongside the ambiguous results,
which is why the senses are laid out as a table.

| word | means, here | not |
|---|---|---|
| **step** | one hand-off from one processor to the next in a workflow; an LLM processor is one step however many tools it calls. The per-request ceiling is the BotProvider's `maxUnsupervisedSteps` - `../wiki/integration.md` | a tool call, or a model turn |
| **sandbox** | the isolated runtime the platform starts to run an agent in | the customer's test environment - call that a **test environment** |
| **project** | one Helm chart deployed to one namespace, under `projects/<slug>/` | the platform's own Project object, which is a division inside a Workspace - say **platform Project** |
| **environment** | `dev` or `prod`: one release in `.asgard-pipeline.yaml`, bound to its own platform Project | the platform's Environment inside a platform Project - say **platform Environment**. There are no per-environment values files; a release's values are variables set on the platform |
| **pipeline** | the platform's IaC pipeline: one repository bound to a workspace, deploying its releases. The Workbench page and the Console call it a **Deployment**, and `--pipeline` is the flag for it everywhere in the CLI | one release of it (`dev`, `prod`) - say **release**; a Kubernetes `kind: Deployment` in a chart, which is a workload; and a reference deployment, one installation the extracts were taken from |
| **agent** | a `Agent` CR, which the UI calls a Managed Agent | the flow-agent shape, which contains no `Agent` CR at all; and not the coding agent working in a repository |
| **workspace** | the customer, and the repository root | nothing else. It is also the platform's billing unit, which is the same thing seen from Fehu |
| **template** | a Go template this tool renders | the "Template" node type the product overview describes, which maps to nothing else here - see `../wiki/what-they-read.md` |
| **skill** | `assets/skills/<name>/SKILL.md`, read by the deployed agent at runtime | `.agents/skills/`, which the coding agent reads while authoring. Say **design-time skill** for the second |
| **knowledge-base** | the extract `../usecase/knowledge-base.md`, about `KnowledgeBase` / `Loader` / `Source` - the older knowledge path | a repository's own documents kept as a wiki, which is the `llm-wiki` design-time skill. Say **wiki** for those |
| **request** | a record under `requirements/requests/` | an HTTP request, and not one run of an agent. For the platform's per-request limits, say **per run** |
| **source** | a `Source` CR under a KnowledgeBase | source code, and not a documentation source. For the material's provenance say **source block**; for code say **source code** |
| **check** | `asgard-cli check`, the structural gate | a layout or content checker in a typesetting skill - say which one |
| **payment** | billing between Asgard and this customer - see `../wiki/fehu.md` | the customer's own payment gateway, which is an external system with side effects: `../usecase/write-path.md` and the ladder on `../wiki/taiwan-channels.md`. Say **payment gateway** for theirs |

## Two more that collide with the customer's vocabulary

These come from the customer's side. Know them before a meeting.

"Project" is the word a customer uses for the engagement. When they say
"how many projects", they mean pieces of work; when this tool says it, it means
charts and namespaces. Ask which they mean rather than answering.

"Agent" is the word they will use for the whole thing they are buying. "How
many agents do we need" is a question about capability, and the answer
starts by saying the count of `Agent` CRs is not the same number - see
`asgard-cli size`.

## What a customer says

That table is not on this page. It is an index, not knowledge about
the platform. A table that lists every alias contains every word a translated
query is rewritten into, so while it lived here it matched whole queries and
searches returned the word list instead of the page about the subject.

It lives beside the pages rather than among them, where `index.md`
also lives:

    aliases.md

Apply it to a query before searching, so a question asked in
the customer's own words reaches material written in English. Add a row when a
search came back empty and the subject turned out to exist under
another name. That is the only test; the rules are on that page.

## How a word gets onto this page

Add a word when it has been read the wrong way once, not when it is ambiguous in
principle. This page is only useful while it is short enough to read, and every
entry here was a real misreading.

## Where two senses meet in the tool's own output

Nothing enforces this page, and a static check cannot: telling which sense a
bare word is in needs a reader. A new page can redefine any of these words and
no command will notice.

Three commands put both senses in front of a reader at once, and each says
which it means. `pipeline release create --project` takes a platform Project,
not a chart under `projects/<slug>/`. `pipeline projects` reports a main
platform Environment beside releases named `dev` and `prod`. `asgard-cli skill`
fetches design-time skills, while the bare word here means the runtime ones a
Syncer feeds.

## Corresponding extracts

None. This is about the material rather than about a deployment.

## Sources

- Each row is a term already defined somewhere in this material; this page adds
  no meanings and only collects the ones that collide. `sandbox` was raised by an
  engagement

**Checked:** the `step` row against
asgard-core `478cf5d6` `internal/bpcontroller/server/bp_controller.go`, which counts one step per
processor run in a request and stops the request past `maxUnsupervisedSteps`;
the three commands above against their own `--help`.
