---
group: In practice
description: the commerce channels a customer will name, what SHOPLINE cost, and what we have not built
---
# The commerce channels a customer will name, and what we have

The same channel names come up in every engagement here. This page answers "do
you already integrate with X" for them.

## SHOPLINE: yes, and deeply

A commerce middleware deployment integrates SHOPLINE through two skills, and
they are the most developed channel material here. Read them before answering a
question about SHOPLINE:

    asgard-freyr-skills/shopline/              the Open API
    asgard-freyr-skills/shopline-backoffice/   the back office

They are separate because the two surfaces are separate, and the split carries a
constraint to know before promising anything:

The Open API cannot write the merchant's own fields. Store name, phone,
email are readable and not writable - there is no merchant write endpoint. What
the API can write is Merchant Metafields, the store-level custom fields, plus a
restricted direct product creation (`POST /v1/products`, with an image upload
to the media library, `POST /v1/media`, behind the same gate) that the skill
allows only to a user holding `products:force_publish` who has chosen it over Freyr's
review flow; it has no endpoint that modifies an existing product, and no
orders. Changing the store's own details means the back office.

The back office is mapped rather than browsed: 88 L1 page entry points, each
declared for whether anything deeper sits under them; 160 rows of operations
covering in-page tabs, dialogs, editor panels and apps inside an iframe; 14 API
domains recorded.

That 88 is the count the map states in its own two headings and asserts with a
script of its own, in `shopline-backoffice/references/page-map.md` in
asgard-freyr-skills - menu-level pages only, with tabs, dialogs and nested
apps in `operation-map.md` beside it. Read the count off the document rather
than recounting it or taking a figure from a neighbouring tally. The skill works
API first - where a contract was
observed the skill calls the back office's own API rather than opening a
browser, and browser operation is the fallback rather than the method.

That 88-page map is the one `../usecase/browser-operation.md` refers to when
it says a capability was "a skill describing 88 pages plus everything the menu
cannot see". This is that skill.

The token mechanism is decided and implemented. It is written up in the skill's
`access.md` rather than here, because it belongs with the calls it authorises.

## Everything else: no

Every reference deployment and the Freyr skills repository were searched for
PChome, momo, 蝦皮 / Shopee and Coupang. No chart, skill or document integrates
one. The only hits are this tool's own landed material and the SHOPLINE
glossary, where `shopee` is one of SHOPLINE's own sales-channel identifiers
under a merchant rather than an integration with Shopee. So:

    "Do you have a Shopee integration?"     "No. We have SHOPLINE, in depth.
                                             Here is what Shopee would take."

Give that answer rather than a hedge. The SHOPLINE work shows the customer that
this kind of integration is understood.

## What a new channel's answer depends on

This page does not list which platform offers an open API, because that changes.
The options, from cheapest to most expensive:

    an open API with a test environment   `../usecase/external-api.md`
    an open API, production only          the same shape - and ask in the
                                          meeting whether they permit testing
                                          against it, rather than assuming
    a data export only            a Syncer over files, not a live integration
    only a web back office        browser operation - and SHOPLINE is what that
                                  costs: 88 pages before the first useful call

"Sandbox" here means the platform's own sandbox, where the agent runs. Call a
customer's test environment a test environment, so the two are not confused.

Ask about each channel separately, in the interview, before anyone builds the
integration. Channels differ, and one back-office-only
channel among four sets the cost of the whole item. A customer answering "yes we
have API access" usually means the one they use most.

A channel is usually two surfaces. SHOPLINE's split - an API that
reads and a back office that writes the rest - is common to other channels. Ask
what the API cannot do before pricing the API.

## What this means for an estimate

Do not price a multi-channel integration as N copies of one integration. Before
pricing four, ask the question that most often reduces them to one:

    "Is there already something that pulls these together for you?"

An OMS, a middleware layer, a warehouse that consolidates the channels - which is
what the Freyr deployment is, from the other side. If one exists, four
integrations become one database and the estimate changes by an order of
magnitude. `../guide/requirements.md` puts this at question 3 for the
same reason.

If nobody knows, that is an open question, not an assumption to price on.

## Corresponding extracts

`../usecase/external-api.md` for an API integration's shape,
`../usecase/browser-operation.md` for a channel offering no API,
`../usecase/skill-layers.md` for how the SHOPLINE material is organised - it is the reference
for a channel skill at full size.

## Sources

- asgard-freyr-skills at `2ff0e1e`: `shopline/SKILL.md`, `shopline/glossary.md`,
  `shopline-backoffice/SKILL.md` and `shopline-backoffice/references/page-map.md`
  in asgard-freyr-skills
- Searched for PChome, momo, 蝦皮/Shopee, Coupang and 酷澎 across every
  reference deployment clone at the commits below, plus asgard-freyr-skills: unitech-e at `44e71a2`, xxentria at `967407c`,
  finance-ai at `d062197`, buy123 at `4dab85d`, freyr at `8f6d6c1`, auto-post at
  `62ccbe0`, industry-demo-generator at `1106771` (excluding `.agents/skills/asgard-platform/`,
  which is this corpus as `init` wrote it)

**Checked:** the SHOPLINE split, the merchant write limit and the map's own
counts against asgard-freyr-skills at `2ff0e1e`; the absence of every other
channel against the eight repositories at the commits above.

**Unchecked:** which of the other channels offers an open API today, which is
the vendor's to answer, and whether the SHOPLINE back-office map still matches
the live SHOPLINE back office.
