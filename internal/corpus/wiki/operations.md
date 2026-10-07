---
group: In practice
description: Asgard's outbound IPs, checking model capability, the symptoms that turn out to be an account or a setting, vocabulary
---
# Connectivity, model capability, vocabulary

Things filed under help-community that come up in practice.

## Reaching a system inside the customer's network

Asgard is a hosted cloud service. It runs in Asgard's own cloud, not in the
customer's data centre or on their network, and no deployment option puts it
inside.

The agent runs in a sandbox that the platform starts in that cloud. So there is
no fixed machine of ours to put on their network, nothing to install behind
their firewall, and no endpoint of theirs we can connect out from. The traffic
leaves Asgard's cloud from the addresses below.

So there is one thing to ask for:

    they add Asgard's outbound addresses to their allowlist

Do not offer a VPN, a bastion or a jump host as alternatives. A VPN connects two
networks; here a hosted service calls in from fixed addresses. If you present
three options, their network team may pick the one that suits their habits and
spend a week finding out it does not apply. If their policy requires a VPN, how
they implement the allowlist is their decision; it does not change what we need
from them.

An SSH bastion changes which machine the allowlist is on, not whether one is
needed. A DataConnector of class postgres, mysql, mssql, oracle, hana or trino
can set `spec.sshTunnel`: the platform opens an SSH connection to a bastion the
customer already runs, and the bastion opens the connection to the database
from its own network. Use it when the database has no address reachable from
outside and the customer already has a bastion that does. Then the four
addresses go on the bastion's SSH port instead of the database's port, and the
connection needs the bastion's host, port, user, a password or a private key,
and its host key so the platform can verify it. Without `hostKey` the
bastion's identity is not checked. athena, salesforce and netsuite refuse
`sshTunnel`.

The customer does the work. We supply the addresses; they own the change, the
approval and the schedule.

Traffic from the platform to a customer's internal database or service leaves
from these four fixed addresses:

```
35.79.216.190
52.196.233.171
54.178.218.69
57.181.108.84
```

All four have to be allowlisted, not one. Do not rely on a given request
leaving from any particular one of them.

### How these are handed over, which is not "in the deck"

Do not copy them out of this page into a proposal, a decision record, or an
email that will be forwarded. Give them directly to the person making the
firewall change, once, and read them from here when you do.

This page is already in the repository, written by `asgard-cli init` under
`.agents/skills/asgard-platform/`, and replaced when the CLI's version moves.
That makes it the one copy allowed to exist. A second copy written by hand into
this engagement's own files is updated by nobody.

The screenshots page carries no images for the same reason: a copy in one
engagement goes stale without anyone noticing. A stale address list does more
damage than a stale screenshot: the customer's connection drops, and they come
back to us about it. A slide or a mail thread is a copy nobody will update; this
page is a copy the CLI replaces.

The `proposal-deck` skill forbids coordinates on a customer's screen -
hostnames, connection strings, account names, including in a screenshot's
corner. That rule is written about the customer's infrastructure, and it applies
to Asgard's addresses too.

In the meeting, ask whether the change can be made. Then send the addresses to
whoever will make it, afterwards and directly.

Find out who will make it, for the follow-up list in the engagement's `docs/open-questions.md`, not for a slide. The
destination is written inside the instruction on purpose: a caveat placed beside
an imperative gets read as elaboration and dropped when the imperative is
copied.

Nothing documents how these addresses change. They read as constants and there
is no documented channel for a revision, so do not treat a copy of them as
durable.

This changes two things in an interview.

Ask it in the first meeting, because the answer is known in advance. "Can four
addresses be added to that system's firewall allowlist?" - the question, not the
addresses and not the org chart. Asking early avoids finding out in week three
that the credential works, the query is right, and nothing can connect.

Track the outcome, not the person. Record it as an open question rather than a
note, because it blocks the first delivery and we cannot do it ourselves. What
is tracked is whether the allowlist can be changed, and then whether it has
been. Not who signs it, how many approvals it needs, or how long their process
takes.

    ours     can this be changed, and is it done yet
    theirs   who signs, which queue, how long

Do not ask who approves it. It fails filter 0 in `../guide/requirements.md`,
which tests whether an answer changes what we build; an approver's name does
not. Filter 0 names this case - turning an operational precondition into a
design question - and asking it in front of a customer reads as managing their
internal process.

Asking when they expect it done is fine, because a date changes our plan.
Asking who is not, because a name does not.

This page's reader is usually preparing for a meeting, not doing an
integration. Run any line here that says "ask this in the meeting" through
filter 0 before following it.

The order the rest of the setup follows once the path is open is
[`setup-path.md`](../wiki/setup-path.md).

## Checking what a model supports

Confirm a model's input and output types before choosing it, or a workflow breaks
at run time. GPT-4o, for instance, takes and produces text, takes images but does
not produce them, and does not handle audio at all.

| provider | list |
|---|---|
| OpenAI | platform.openai.com/docs/models |
| Azure AI | ai.azure.com/catalog |
| Anthropic | docs.anthropic.com/en/docs/about-claude/models/overview |
| Mistral | docs.mistral.ai/getting-started/models/models_overview |
| Gemini | ai.google.dev/gemini-api/docs/models |
| Voyage | docs.voyageai.com/docs/pricing |

Semantic Model adds a hard floor of its own: the Completion Model must support at
least 60,000 Max Output Tokens. The workflows the platform derives from a
`SemanticLayer` set `maxTokens` to 60000 on their model processors, so a model
with a lower output ceiling fails there.

## Common LLM Completion failures

| symptom | cause | what to do |
|---|---|---|
| the step fails | the prompt or history exceeds the model's context window | check the input length, chat history and embedded data especially; cap output with `MaxToken` |
| no response | the provider account has no billing enabled, or the quota is spent | check the payment method and remaining quota |
| the request is refused | the API key is wrong, expired, or has stray whitespace | check the key |

To reproduce a context-window overflow deliberately, set `MaxToken` to `0`.

## An empty iFrame

Usually the iFrame has not been made public. Check Public iFrame under App-Share
Settings, save, and try again.

## Connecting an OpenAI-compatible model

Services whose API is OpenAI-compatible, such as DeepSeek, need no new provider:

1. Model Provider: OpenAI Chat Completion
2. Model Name: Other, then the real model name (`deepseek-chat`)
3. Endpoint: the service's address (`https://api.deepseek.com`)

## Environment

A Project can hold several Environments. The system creates Main by default and a
user can add others - Main as production and another for development, say - and
merge a finished one back into the main environment.

This is not the same thing as a chart's `dev` / `prod`. Those are releases: each
is bound to its own platform Project, deploys into that Project's namespace,
and takes its values from variables set on the platform rather than from a
values file. The two meet at one point: the platform injects the Project's main
Environment id into every run as `.Values.asgard.projectEnvironmentId`, and a
chart stamps it on its Workflows, CompletionModels and Triggers as the
`asgard-ai.com/project-environment-id` label.

## Vocabulary

| term | meaning |
|---|---|
| Workspace | the smallest unit a subscription is billed against |
| Project | a project under a Workspace; its resources - knowledge bases, settings, apps - are shared within it |
| Collection | a set of workflows, a workflow set |
| Workflow | processors connected into a flow, with a start and an end |
| Processor | the smallest processing node |

When a customer asks about cost structure, start from the Workspace: it is the
billing unit, so how resources are divided into Workspaces affects cost.

## Corresponding extracts

Connectivity and vocabulary produce no CRs, so there is no extract for them.

## Sources

- [Outbound IPs](https://docs.asgard-ai.com/docs/help-community/other/vpn-white-list-ip),
  [checking model support](https://docs.asgard-ai.com/docs/help-community/other/check-model-support),
  [an empty iFrame](https://docs.asgard-ai.com/docs/help-community/faq/iframe-display-blank),
  [LLM Completion troubleshooting](https://docs.asgard-ai.com/docs/help-community/other/troubleshooting-llm-completion),
  [DeepSeek](https://docs.asgard-ai.com/docs/help-community/faq/deepseek),
  and asgard-docs `docs/help-community/faq/how-to-show-channel-log.mdx`
  - Cited as a file because the published page does not exist, and this one
  is not a draft. It carries `draft` nowhere, `hidden: false` and its own
  slug, and its two neighbours in that directory serve 200; it is missing from
  `sidebars.js`. That is an upstream omission rather than a decision; tell
  whoever maintains asgard-docs
  - asgard-docs `f00e0ee`
- asgard-docs `docs/overview/asgard-environment.md` - cited as a file: it is
  `draft: true`, so there is no page to link to. The file is readable in a
  checkout and that is where it was read
  - asgard-docs `f00e0ee`
- [Glossary](https://docs.asgard-ai.com/docs/help-community/glossary)
  - asgard-docs `f00e0ee`. Its Processor entry lists an older set of nodes and
  does not match the current `ProcessorType`; take [`workflow.md`](../wiki/workflow.md)
  as the current one

**Checked:** the 60,000-token floor against
asgard-core `478cf5d6` `internal/bpoperator/reconciler/sl_reconciler.go`; the four addresses against
asgard-docs `95a27895` `docs/help-community/other/vpn-white-list-ip.mdx`; the
Environment id reaching a chart against
xxentria-asgard-kube `967407c` `.asgard-pipeline.yaml` and its Workflow labels;
`sshTunnel`, the classes that take it and the three that refuse it against
asgard-kube `42e8722` `crd/asgard-ai.com_dataconnectors.yaml` and against
asgard-core `001bbf69` `internal/bpcontroller/dataconnector/`.

**Unchecked:** the addresses are the documentation's, and the egress
configuration behind them is in none of the repositories here. That Asgard
always needs the customer to open the path inward is the FDE team's account of
every engagement so far and no document states it. That the SSH connection
to a bastion leaves from the same four addresses, and that production runs an
asgard-core release carrying `sshTunnel`, is per the platform team; no
repository here states either.
