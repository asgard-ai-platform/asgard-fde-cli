---
group: In practice
description: the retail stockout from several angles, plus a Flow Agent help desk
---
# Case study: a stockout and a cross-store transfer

The same retail demo project has three case studies describing one event from
three angles. Together they are usable conversation material and show how the
three products divide the work.

| angle | product | what it covers |
|---|---|---|
| before the decision | Mimir | one question yields the stock spread across every store |
| how it is built | Odin | building the transfer agent, mounting a Semantic Model, setting the governance gate |
| how it is used | Sindri | one question in a chat window, then approving the transfer order |

A fourth is a different shape: an AI help desk on an e-commerce site, using a
Flow Agent rather than a Managed Agent.

## The angles in sequence

Mimir, before the decision. An operations person selects the POS store
semantic model and asks which stores hold more of a product than their safety
stock and can therefore transfer some, with a bar chart of the on-hand figures.
Mimir runs seven steps and answers in two groups: eleven stores able to transfer,
led by one with a surplus of 56, and one store short by 34. It then suggests
drawing from the two largest surpluses.

The question explicitly asks for a bar chart. Without that, the default
is text and a table.

Odin, how it is built. An administrator builds the cross-store transfer
agent, mounts the Semantic Model it may query, and configures the governance
gate.

Sindri, how it is used. The person using it does not need to know how the
agent was built. They switch to the retail Project, see five published agents
(cross-store transfer, product insight, promotion analysis, replenishment, store
operations), and without deciding which to ask, type a question naming the
product and the store that is about to run out.

Sindri delegates to the `allocation` agent, and the Subagents panel shows the
delegation and its live queries. The reply covers the current position, the total
available to transfer and a per-store ranked list. It then offers three
options (A: top three stores, B: all stores, C: custom) and waits rather than
acting.

After the user picks A, an approval dialog names the toolset (`ts-wms`) and the
tool (`create_transfer_order`), shows the actual input expandably, and gives the
count of pending calls (1 / 3). The choices are allow for this conversation,
allow once, or refuse.

## What this set demonstrates

Read and write are separated. Reading goes through a Semantic Model; writing
goes through a Toolset with a governance gate. Having finished the analysis the
agent does not write; it proposes.

Delegation needs no judgement from the user. They do not have to know which
agents exist or which to ask, which is what a Managed Agent's `description`
achieves as routing text.

One dataset is consumed two ways. The same POS store semantic model is explored
in Mimir and analysed by the Sindri agent.

The demo's chart is the same arrangement. The five agents are five `Agent` CRs;
the transfer agent's `aliasName` is `allocation`, it reads through two
`semanticLayers` entries with `allowWrite: false`, and it writes only through
`ts-wms`, whose one tool is set `requestConsent: true` - that setting is the
approval dialog.

## The AI help desk, a Flow Agent shape

Same retail project, lighter shape. A customer types "I want to check my order"
into the site's help-desk window. The site immediately writes it into its own
help-desk record and replies to the customer without waiting for the AI, and in
the background forwards the message together with a short-lived,
scope-limited access credential to the Flow Agent.

The agent uses that credential to read that customer's own orders and writes a
reply back into the same conversation within seconds. A human agent can step into
the same thread at any point.

The Flow Agent corresponds to a `wf-customer-service` Workflow with five nodes:
Entry, Init (setting up context), Agent (the LLM stream reply), Listen, and a
push on the Agent node's Failure branch. That is the shape a new Flow Agent
is created with - see `../usecase/flow-agent-single.md` - so this chart is
the default flow edited rather than one built from nothing.

In the retail chart at `718cc0e` the site sends the credential in the message's `payload`, as a
member id and an access token valid for ten minutes, and a `SandboxBlueprint`
hook writes it to a file in the sandbox before each turn; the agent calls the
site's own API with it. No Toolset is mounted, so this flow reads and replies
and writes nothing.

The short-lived scoped credential lets the agent
read "this customer's own" data rather than granting it the order database. That
mechanism is `../usecase/per-turn-credentials.md`.

## Corresponding extracts

The read/write separation is `../usecase/write-path.md`; the delegation
topology is `../usecase/flow-agent-supervisor.md` and `../usecase/agent-hub.md`.

## Sources

- [Mimir: finding the stores that can transfer](https://docs.asgard-ai.com/docs/product-suite/mimir/case-studies/retail-stockout-demand)
  - asgard-docs `6261fdff`
- [Odin: stockout to cross-store transfer](https://docs.asgard-ai.com/docs/product-suite/odin/case-studies/retail-stockout-transfer)
  - asgard-docs `6261fdff`
- [Sindri: the same, from the user's side](https://docs.asgard-ai.com/docs/product-suite/sindri/case-studies/retail-stockout-transfer)
  - asgard-docs `6261fdff`
- [Odin: an AI help desk answering order questions](https://docs.asgard-ai.com/docs/product-suite/odin/case-studies/retail-ai-customer-service)
  - asgard-docs `6261fdff`

**Checked:** the agents, the read and write split and the approval gate
against the retail chart, asgard-industry-demo-generator `1106771`
`projects/retail/chart/app/templates/`; the help-desk workflow against
asgard-industry-demo-generator `718cc0e` `retail/chart/app/templates/supervisor/customer_service/`,
which the chart no longer carries.

**Unchecked:** the Mimir and Sindri screens - the seven steps, the counts in the
answer, the three options and the approval dialog - which only a Console
account running the demo can show.
