# references

`references/` holds the raw sources this engagement's pages are written from, for humans
and spec-writing agents. Each is filed once and never edited. It is not the implementation
source of truth: that is `requirements/` and the living spec under `docs/spec/`.

Use this directory for:

- The customer's own design specs, architecture notes and system documentation - schema
  dumps, ERDs, field dictionaries, status-code tables.
- Readings off a running system: a query and its result, a CR or a config dumped from the
  cluster, an API response, a command's output. Save it to a file and file it here, so a
  claim written from it points at something on disk. Take out credentials and personal
  data first, and keep only the part that supports the claim: the file is committed, and a
  git Syncer without `includePaths` copies it onto the platform.
- External API documentation and third-party integration notes.
- Kubernetes / Helm / platform behavior notes and migration references.

Not here: meetings go in `docs/meeting-notes/`, and decisions in `docs/decisions/`.

Rules:

- Convert reference material into `requirements/` before implementation.
- If `references/` conflicts with `requirements/`, trust `requirements/` and record the conflict as an open question or decision.
- Prefer clear source names, such as `pm-spec/` or `api-reference.md`.
- File with `asgard-cli reference add`. It copies the file byte-identical and records in
  `_index.md` where it came from and when: who supplied it, or for a reading which system
  and which query or command; its date, or the day it was read; and whether anything in it
  was verified against the real system. Do not paste a header into the file. Material a
  customer wrote for their own staff describes the system they believe they have, and
  nothing on a stale page shows that it is stale.
- Do not edit a filed source. A newer version or a newer reading is a new file, and the two
  together show what changed.

## Filing something large

One customer manual or API reference is a file. A whole system's worth is a
directory, and it needs an index or nobody reads past the first page:

```
<system>/README.md              what this covers, where it came from, when
<system>/conventions.md         the cross-cutting rules to read first
<system>/api/<domain>.md        one file per domain, index table at the top
```

Two habits keep a reference set usable:

- Every index row carries its status: verified against the real system, or
  taken from the document and not yet checked. Mark unverified rows as
  unverified, so the reader knows which rows to trust.
- Say which values were measured and which were inferred. A value read off the
  running system is evidence; a value derived from another value is not. When a
  later reader hits a mismatch, this tells them whether they found drift or a
  guess.

Do not paste large material into a request or a task spec. Put it here and cite
it from there. A spec that inlines a whole API's documentation is hard to read as a spec.

> Related but not here: domain knowledge the *agent* needs at runtime (status-code semantics,
> cross-system entity mapping, aggregation conventions) belongs in a skill under
> `assets/skills/<skill>/SKILL.md`, because it has to be synced into the platform to be usable.
> Pages the agent should answer from - product literature, FAQs, policies, written from the
> files here - go in `assets/drives/<drive>/` for a knowledge Drive; see
> `assets/drives/README.md`. Nothing on the platform mounts this directory, but a git Syncer on
> this repository without `includePaths` copies it onto a volume with the rest of the repository.
> `references/` is for humans and spec-writing agents;
> `assets/skills/` and `assets/drives/` are for the running agent.
