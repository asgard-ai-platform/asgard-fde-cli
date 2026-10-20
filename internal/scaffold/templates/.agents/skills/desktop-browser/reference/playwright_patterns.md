# Playwright Patterns (the desktop browser, over CDP)

You run these heredoc-style, one shell command per turn, from the repository
root. The browser is the window `desktop_browser.py start` opened - you
**connect** to it, you do not launch it. Every script starts by importing the
helpers, which is also what stops it in the Workbench sandbox.

## Connect + reuse the visible tab (the canonical skeleton)

```bash
.venv/bin/python - <<'PY'
import os, sys
from pathlib import Path
sys.path.insert(0, ".agents/skills/desktop-browser/scripts")
from desktop_browser import WORKSPACE, connect, live_page, snapshot, screenshot
from playwright.sync_api import sync_playwright

WORKSPACE_DIR = Path(os.environ.get("WORKSPACE_DIR", WORKSPACE / "<task_id>"))
SCREENSHOTS = WORKSPACE_DIR / "screenshots"

with sync_playwright() as p:
    browser = connect(p)              # the running window, over CDP
    page = live_page(browser)         # the tab the person sees, brought to the front

    page.goto("<START_URL>", wait_until="domcontentloaded")
    screenshot(page, SCREENSHOTS / "explore_1_start.png")

    print("URL:", page.url)
    print("TITLE:", page.title())
    print("ARIA:", snapshot(page))

    # NOTE: do NOT call browser.close() / context.close().
    # connect_over_cdp only attaches to the person's window; closing it would
    # close what they are looking at. Just let the script end.
PY
```

Rules:

- **Connect, never launch.** `connect(p)` attaches to the window on
  `127.0.0.1`; it exits with a message when the window is not running, and the
  fix is `desktop_browser.py start`, not a `launch()`.
- **Reuse the live tab** from `live_page()`. Don't create a new context. Avoid
  spawning throwaway tabs - the person is watching this one.
- **Never close** the browser or context.
- **Read and capture through the helpers.** `snapshot(page)` instead of
  `page.locator("body").aria_snapshot()`, and `screenshot(page, path)` instead of
  `page.screenshot()`: they mask password fields and long secret-looking strings
  in what you read and in the image, and leave the page itself alone.
- Don't resize the viewport - the person shares this window. `screenshot()`
  captures the visible viewport (what the person sees), which is exactly the
  evidence you want.
- Each run reconnects fresh and navigates from the current state - reapply
  filters / re-open the target page in code rather than assuming where the tab is.

## A click that opens a new tab

Admin consoles often open a section in a new tab. Find it by URL rather than by
position, and work in it from then on:

```python
with page.context.expect_page() as new:
    page.get_by_role("link", name="Developer Hub").click()
tab = new.value
tab.wait_for_load_state("domcontentloaded")
tab.bring_to_front()
print(tab.url)
```

In a later script the tab is still there: pick it with
`[t for t in browser.contexts[0].pages if "<part of its URL>" in t.url][0]`.

## Targeting elements with role + name

```python
page.get_by_role("button", name="Filters").click()
page.wait_for_timeout(1000)

# Snapshot the parent of the control to see siblings/options
print(snapshot(page, "role=button[name='Filters'] >> xpath=.."))

page.get_by_role("checkbox", name="BMW").check()
page.wait_for_timeout(1000)
```

A field whose label is a heading rather than a `<label>` has no accessible name:
`get_by_label` does not find it. Take the textbox inside the dialog or section
instead (`page.get_by_role("dialog").get_by_role("textbox").first`).

If a selected state becomes hidden after a drawer/dropdown closes, reopen it
before capturing the verification screenshot.

## Prefer interactive form filling over deep-link URLs

When a task requires parameterizing a search (locations, dates, filters, query
strings), **drive the on-page form interactively** rather than constructing a
deep-link URL with parameters baked into the query string. Deep links are brittle
as a reusable surface: sites silently drop params they can't parse, parsers vary
by locale / A-B bucket / signed-in state, and a working link for one input set
tells you nothing about another. Make interactive filling the **primary** path;
use a deep link only as an opportunistic shortcut, and always verify the form
state afterwards and fall back to interactive filling when any field is wrong.

Guidelines for the interactive path:

- Use `get_by_role` / `aria-label` selectors, not brittle CSS classes.
- Type the value, wait for the suggestion listbox, then click the option whose
  text contains the canonical token.
- For paired fields in one modal (date-range pickers, stepper groups), open the
  modal **once** and `Tab` between fields instead of clicking each input - clicking
  the second input often gets blocked by the modal's overlay.
- After filling, click the explicit submit control rather than relying on
  auto-submit.
- Re-read the form state and assert each checkpoint before extracting results.

## Retrieving downloaded files

The window saves downloads into the downloads directory `info` prints
(`desktop_browser.DOWNLOADS`), without asking where. Trigger the download, wait
for it to finish, then read it from there.

```bash
.venv/bin/python - <<'PY'
import sys, time
sys.path.insert(0, ".agents/skills/desktop-browser/scripts")
from desktop_browser import DOWNLOADS, connect, live_page
from playwright.sync_api import sync_playwright

with sync_playwright() as p:
    page = live_page(connect(p))
    before = set(DOWNLOADS.glob("*"))
    page.get_by_role("button", name="Download").click()

    # Wait for a new, fully-written file (".crdownload" = Chromium still writing).
    for _ in range(60):
        new = [f for f in DOWNLOADS.glob("*")
               if f not in before and not f.name.endswith(".crdownload")]
        if new:
            print("DOWNLOADED:", new[0])
            break
        time.sleep(1)
PY
```

A downloaded export of an admin console is the customer's data: read what the
task needs from it and leave it where it is, outside the repository.

## Final-script instrumentation

`final_runs/run_<id>/final_script.py` must:

- connect over CDP with `connect()` and reuse the visible tab,
- write to `final_runs/run_<id>/screenshots/final_execution_<step>_<action>.png`
  through `screenshot()`,
- reset and append to `final_runs/run_<id>/final_script_log.txt`,
- print the final datum at the end of the log,
- **not** close the browser.

It runs from the repository root, so the import path is the same as above:

```python
import sys
from pathlib import Path
sys.path.insert(0, ".agents/skills/desktop-browser/scripts")
from desktop_browser import connect, live_page, screenshot
from playwright.sync_api import sync_playwright

RUN_DIR = Path(__file__).parent
SCREENSHOTS = RUN_DIR / "screenshots"
LOG = RUN_DIR / "final_script_log.txt"
LOG.write_text("")  # reset

def log(step: int, msg: str) -> None:
    line = f"step {step} action: {msg}\n"
    with LOG.open("a") as f:
        f.write(line)
    print(line, end="")

def main():
    with sync_playwright() as p:
        page = live_page(connect(p))

        page.goto("<START_URL>", wait_until="domcontentloaded")
        screenshot(page, SCREENSHOTS / "final_execution_1_open_start_page.png")
        log(1, "open start page")

        # ... apply CP1, screenshot, log ...
        # ... apply CP2, screenshot, log ...

        final_value = "<extracted price / code / winner>"
        with LOG.open("a") as f:
            f.write(f"\nFINAL_RESPONSE: {final_value}\n")
        # no browser.close()

main()
```

## Inspection commands

```bash
ls -R <workspace>/<task_id>/final_runs/run_<id>
cat <workspace>/<task_id>/final_runs/run_<id>/final_script_log.txt
```

For visual checks, open the individual PNG files inside
`final_runs/run_<id>/screenshots/` and judge them yourself against `plan.md`.
