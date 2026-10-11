---
group: While building
description: what no source answers, and who to ask
---
# What the platform's documentation does not answer

Platform questions that neither the source nor the documentation settles. Each
row is one an engagement hit and had to work around.

Before adding a row, look in the source: the CRDs and Go types in asgard-kube,
the runtime in asgard-core, the product documentation in asgard-docs, the
Console in asgard-ai-platform-web and its back end in asgard-platform-api,
projects and display annotations in asgard-workflow-service, the
pipeline runner in asgard-iac, the Syncer runtime in asgard-syncer, and the
cluster's network and logging in tf-asgard. Most questions about how the
platform behaves are answered there. A row belongs here
only when the thing exists and none of them says how it behaves; then ask the
platform team, before the meeting rather than in it, and do not assume an
answer. If the source has no such feature, write that on the page it concerns
instead of adding a row.

When a row is answered, put the answer on the page it concerns, with the source
it was read from, and delete the row.

**Checked:** against asgard-core `478cf5d6`, asgard-kube `3da0365`
and asgard-docs `56feb925`; every question those repositories answer is on
the page it concerns.

**Unchecked:** the platform's services that are not in that list - the agent
sandbox runtime, Agent Hub's own back end - so a question about them may still
belong here.

## The list

No open questions.

| # | Question | When it matters |
|---|---|---|

## Why this is not in the customer's repository

These questions are the platform's, not one engagement's, so they do not go in
a customer's `docs/open-questions.md`. A copy in one engagement goes stale without
anyone noticing; a fix on this page reaches every engagement in one release. The
customer repo's `docs/open-questions.md` holds that engagement's own questions.

## Adding a row

Add a row only for a question an engagement actually hit: a customer's test plan
asking for something with no answer, or a design that had to proceed on an
assumption with nothing to cite. Name the engagement's requirement that produced
it, and number it after the highest number used so far (P16).
