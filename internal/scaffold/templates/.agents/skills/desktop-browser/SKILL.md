---
name: desktop-browser
description: Desktop only. NEVER use this skill when the environment variable ASGARD_SANDBOX_MODE is true (the Workbench assistant's sandbox) - use the sandbox's built-in "browser" skill instead; this one's scripts refuse to run there. On a person's own computer, drive one visible Chromium window over CDP, code-as-action style - start it, act one shell command at a time, save screenshots and an action log into final_runs/run_<id>/, and visually verify the result. Use when a web page has to be operated with the person watching and able to take over - an external system's admin console during an Extension's setup/, a vendor's developer portal, a form only a browser can fill - or when a web task should leave reusable scripts plus screenshot evidence.
version: 1.0.0
alwaysApply: false
---

# Desktop browser

> **Desktop only.** In the Workbench assistant's sandbox (`ASGARD_SANDBOX_MODE=true`,
> or `ASGARD_BROWSER_CDP_URL` is set) do not use this skill: use the sandbox's
> built-in `browser` skill, whose browser the member watches and takes over through
> the Workbench page. Every script here checks this first and exits with status 3.

You drive **one real, visible Chromium window on this computer**. It runs with its
own profile, outside the repository, and listens for the Chrome DevTools Protocol
(CDP) on `127.0.0.1`. You start it once, then connect to it over CDP and act
through Playwright, one shell command at a time, observing each command's output
before the next.

The window is **the person's as much as yours**: they are watching it, they can
click and type in it at any moment, and it stays open between your commands. So
you reuse the visible tab, navigate from wherever it currently is, and you **never
close the browser** - when the work is done the person closes the window, and the
next `start` opens it again with the same profile, still signed in.

This skill keeps a *workspace contract* (plan.md, `final_runs/run_<id>/` folders,
instrumented `final_script.py`, screenshots, action log) and you verify success
yourself: you open the saved PNGs and check them against `plan.md`. No external
image-QA service or API key is involved. It is the same contract as the sandbox's
built-in `browser` skill, so a task is done the same way in either place.

## The browser on this computer (read this first)

Everything goes through one script, run from the repository root:

```bash
.venv/bin/python .agents/skills/desktop-browser/scripts/desktop_browser.py start [<url>]
.venv/bin/python .agents/skills/desktop-browser/scripts/desktop_browser.py info
```

- **`start`** opens the window when it is not running, and otherwise leaves it as
  it is. It prints the CDP endpoint (`http://127.0.0.1:9333` unless
  `ASGARD_DESKTOP_BROWSER_PORT` says otherwise), the profile directory, the
  workspace directory and the downloads directory. Tell the person a Chromium
  window has opened and that it is the one you will be working in.
- **`info`** prints the same without opening anything, and whether it is running.
- **Connect over CDP**, never launch from a script. Every script imports the
  helpers in `scripts/desktop_browser.py` (`connect`, `live_page`, `snapshot`,
  `screenshot`), which also refuse to run in the sandbox. Never call
  `chromium.launch()` yourself: a browser a script launches is gone when the script
  ends, and it is not the window the person is watching.
- **Reuse the live tab.** `live_page(browser)` is the visible tab
  (`browser.contexts[0].pages[0]`), brought to the front. When a click opens a new
  tab, find that tab by its URL among `browser.contexts[0].pages`; never assume it
  is first or last.
- **Never `browser.close()` / `context.close()`.** With `connect_over_cdp` that
  would close the person's window. Just finish your script.
- **The engine is Chromium**, Playwright's own build, so every computer runs the
  same one. It is not the person's everyday browser and shares none of its
  cookies or passwords.
- **Downloads** land in the downloads directory `info` prints. Read them from
  there.
- **The workspace is outside the repository.** Put `WORKSPACE_DIR` under the
  workspace directory `info` prints (`<workspace>/<task_id>/`), never inside
  the repository: a screenshot of an admin console carries the customer's data,
  and nothing under it belongs in a commit.

The minimal connect-and-reuse skeleton (run it heredoc-style, one command/turn):

```bash
.venv/bin/python - <<'PY'
import sys
sys.path.insert(0, ".agents/skills/desktop-browser/scripts")
from desktop_browser import connect, live_page, snapshot   # refuses in the sandbox
from playwright.sync_api import sync_playwright

with sync_playwright() as p:
    browser = connect(p)
    page = live_page(browser)       # the tab the person sees
    print(page.url, page.title())
    print(snapshot(page))           # ARIA snapshot, long secret-looking runs masked
    # ... act here ... do NOT browser.close() / context.close()
PY
```

**Read `reference/playwright_patterns.md` before you write any script** - it has
the full connect/reuse/screenshot/download recipes, selector patterns, and the
action-log format you must follow.

## Working with the person in the window

Most of what this skill is for on an FDE engagement is a page the person has to be
part of: an external system's admin console, during an Extension's `setup/`.

- **They sign in.** Open the page, tell them to sign in in that window, and wait
  for them to say they have. Never type a password, a one-time code or a recovery
  code, and never ask for one in the conversation.
- **Say what you are about to do**, in plain words, before each step that changes
  something ("I'm creating an app called Asgard AI"), and stop for their choice
  where the material says a person decides (which permissions to keep).
- **Never read a secret.** Read pages through `snapshot()` and capture them
  through `screenshot()`: both mask long secret-looking strings without touching
  the page the person sees. Do not click a control that reveals a token, read an
  input's value, or copy a key. When a page shows a secret, ask the person to copy
  it themselves.
- **Do not leave a page that shows a secret** until they say it is saved. Many
  consoles show it only once. When it is saved, run `asgard-cli local-env` (it
  opens its own form) for them to paste it into, and confirm the key is filled
  before moving on.
- **Ask before creating** anything in an external system that is not part of the
  task you were given.

## Modes

- **Default (one-shot).** `final_script.py` solves the task for the literal
  values the user provided.
- **CLI tool (parameterized).** `final_script.py` is a reusable CLI: one function
  with a Google-style `Args:` docstring + an `argparse` wrapper whose flags
  default to the concrete task values, so it can be rerun later with different
  arguments. Use this when the user asks to "parameterize", "make it reusable",
  or "turn this into a CLI". See `reference/cli_tool_mode.md`.

## Prerequisites (once per computer)

The repository's `.venv` is the one `db-query` uses; create it if it is not there.

```bash
python3 -m venv .venv
.venv/bin/python -m pip install playwright
.venv/bin/python -m playwright install chromium
```

`start` says which of these is missing. Do not install anything else.

## Workspace Contract

- Pick a `WORKSPACE_DIR` (e.g. `<workspace>/<task_id>/`, where `<workspace>` is
  what `info` prints) and work **only** there. Keep all generated code,
  screenshots, logs, and notes inside it.
- The required final artifact is `final_script.py`.
- Every clean execution of the final script lives in its own
  `final_runs/run_<id>/` folder (`<id>` is an integer higher than any existing
  `run_*`). Inside each run folder:
  - `final_runs/run_<id>/final_script.py`
  - `final_runs/run_<id>/screenshots/final_execution_<step>_<action>.png`
  - `final_runs/run_<id>/final_script_log.txt` - reset at the start of each clean
    run; one `step <n> action: <reason and action>` line per constraint-relevant
    interaction; the final datum printed at the end.
- **Connect to the running browser every run** (`connect`), reuse the visible
  tab, and reconstruct the on-page state you need by navigating/clicking - do not
  assume the tab is where a previous run left it.

## Workflow

1. **Plan.** Parse the task into a numbered checklist of *critical points* -
   every explicit constraint, filter, sort, selection, or required datum. Write
   it to `WORKSPACE_DIR/plan.md`:

   ```markdown
   # Critical Points
   - [ ] CP1: <description>
   - [ ] CP2: <description>
   ```

   Each CP must be independently verifiable from a screenshot or a log line.

2. **Explore.** Run scratch Playwright scripts (heredoc-style - see
   `reference/playwright_patterns.md`) against the running browser to discover
   stable selectors and confirm controls exist. Open saved PNGs to inspect UI
   state. Print ARIA snapshots, URLs, titles, and visible labels each step.

3. **Author `final_script.py`** in a fresh `final_runs/run_<id>/`. Instrument it
   per the contract: reset the log, write a step line for every
   constraint-relevant action, save a uniquely-named screenshot per critical
   point, and print the final datum at the end.

4. **Execute** the final script once. Capture stdout/stderr.

5. **Self-verify.** Walk `plan.md`:
   - For each CP, cite a screenshot path AND/OR a log line that proves it. Open
     each cited PNG and confirm the evidence is unambiguous (filter chip visible,
     date matches exactly, result list reflects the constraint, etc.).
   - Tick a CP only when the evidence is concrete. Be harsh with ambiguous,
     occluded, or partially-applied states.
   - If a CP fails, diagnose the specific issue, fix `final_script.py`, re-run in
     `final_runs/run_<id+1>/`, and re-verify.

6. **Done.** Only when every CP is checked off with cited evidence. Report the
   final datum to the user.

A page operated *with* a person - signing in, a choice that is theirs - is not a
single unattended run. Walk it step by step with scratch scripts, and keep the
plan and the screenshots as the record of what was done; write `final_script.py`
only for the part that runs without them.

## Hard Rules

- One shell command per step; observe its output before issuing the next.
- Connect over CDP with `connect()`; reuse the visible tab; never launch from a
  script and never close the browser.
- Read pages with `snapshot()` and capture them with `screenshot()`, never a raw
  `aria_snapshot()` or `page.screenshot()`: those are the ones that mask secrets.
- Use stable selectors and current-run evidence - never guess UI state.
- If a site exposes a dedicated control for a requirement, you **must** use it. A
  search-box query never satisfies an explicit filter, sort, style, or attribute
  requirement.
- Ranking language (`cheapest`, `best-selling`, `most reviewed`, `highest-rated`,
  `lowest`, `latest`, …) must be grounded in the site's actual sort/filter - not
  your own ordering of results.
- Numeric, date, quantity, and unit constraints are **exact**. Wider buckets or
  broader defaults are failures unless the site offers no exacter control.
- If a selected state becomes hidden after a drawer / accordion / modal / dropdown
  closes, reopen it or capture a visible chip/summary before treating it verified.
- Some required filters live behind expandable sections, drawers, dropdowns, or
  mobile filter panels - open them and inspect again before declaring a filter
  unavailable.
- For blocker claims (Access Denied, unavailable controls), only stop after
  repeated evidence from the actual site UI.
- To retrieve a file the task downloads, wait for the download to finish, then
  read it from the downloads directory `info` prints.
- If the task asks for a final datum (code, price, quote, winner, benefit list),
  state it explicitly to the user **and** append it to `final_script_log.txt`.
- Do **not** install extra packages beyond the prerequisites above.
- Once `final_script.py` exists, prefer incremental edits over full rewrites.

## Reference Files

- `reference/playwright_patterns.md` - connect/reuse skeleton, screenshot &
  download recipes, `aria_snapshot()` patterns, log format.
- `reference/workflow.md` - detailed plan → explore → final → self-verify, plus
  the completion checklist.
- `reference/cli_tool_mode.md` - contract for the parameterized CLI tool mode.

## Provenance

**Checked:** on macOS, with Playwright's Chromium 148. `start` opened one visible
window that outlived the command, and a second `start` reused it. Connected
through `connect()` / `live_page()`, a page holding a 56-character token and a
password field came back from `snapshot()` with both masked, the `screenshot()`
image had both blacked out and the text field beside them legible, and the page
the person sees was unchanged afterwards. The refusal exits 3 under both sandbox
signals; `desktop_browser_test.go` holds it. The same approach - a visible
Chromium the person signs in to, driven over CDP, secrets masked before
anything is read - walked an Intercom Developer Hub end to end on 2026-10-20.
The contract and the rules are the sandbox's built-in `browser` skill's
(asgard-agent-sandbox `7f93dab`), with the browser section rewritten.

**Unchecked:** Windows and Linux desktops; the downloads directory the profile
is seeded with; a console that refuses a browser it can tell is automated, or
signs in with a hardware key.

Adapted from Microsoft's [Webwright](https://github.com/microsoft/Webwright),
by way of the Asgard sandbox's built-in `browser` skill; MIT licensed, notice in
`LICENSE-webwright.txt`.
