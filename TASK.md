# TASK.md

What is still to do in this repository. Nothing done and nothing decided
against is kept here; `git log` has the history. What the tool is for is [Goal.md](Goal.md), how the
capabilities are implemented is [APPROACH.md](APPROACH.md), what lives where is
[STRUCTURE.md](STRUCTURE.md), how to change it is [AGENTS.md](AGENTS.md), and
what the commands do is [README.md](README.md).

A finding a reader needs goes on the document it concerns: an `**Unchecked:**`
marker on the page, a row on
`.agents/skills/asgard-platform/wiki/platform-unknowns.md`, or a rule in
AGENTS.md.

`go run ./hack pass` prints the checks, derived from the binary's flags,
`hack/`'s contents and the gate's subcommands. Each check reports through its
exit code. `.agents/skills/consistency-checks/SKILL.md` is the method.

## What is not done

Every document names what it has not been held against:

    asgard-cli audit-material --unchecked

How far the sources have moved is reported by `go run ./hack sources`,
`go run ./hack sources --extracts` and `go run ./hack coverage --drift`.

## `asgard-cli operate`: the rest of the runtime surface

What a deployed CR does at runtime, where IaC cannot reach. `operate syncer`
and `operate skill-set sync|executions` exist. Still to come, each its own
change:

  - trigger: run now, its invocations, an invocation's logs
  - source-set: the context index's reindex and its invocations. Whether the
    derived Trigger's invocation-logs route answers for a context index has not
    been tried against a live platform
  - oauth-credential: authorize, and its status
  - source-set and skill-set volume files
  - chat: an Agent's and a workflow set's preview, and a Trigger's or a
    context index's conversation, over the platform's SSE relay
