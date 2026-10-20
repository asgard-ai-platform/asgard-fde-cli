#!/usr/bin/env python3
"""desktop_browser.py -- one visible Chromium window on this computer, driven over CDP.

Run from the repository root:

    .venv/bin/python .agents/skills/desktop-browser/scripts/desktop_browser.py start [<url>]
    .venv/bin/python .agents/skills/desktop-browser/scripts/desktop_browser.py info
    .venv/bin/python .agents/skills/desktop-browser/scripts/desktop_browser.py reset [--all]

and from a script, after `sys.path.insert(0, ".agents/skills/desktop-browser/scripts")`:

    from desktop_browser import connect, live_page, snapshot, screenshot

**Desktop only.** In the Workbench assistant's sandbox the browser is a sidecar the
member watches through the Workbench page, and the sandbox's built-in `browser`
skill is the one that drives it. Importing this module there exits with status 3,
so neither `start` nor a script built on these helpers can run in the wrong place.

The window is started detached, with its own profile, so it outlives the command
that started it and every later script connects to the same one. It is never
launched from a script: a browser a script launches closes when the script ends.

It is started with the switches Playwright passes to every browser it launches
itself, so it never asks the operating system for anything: no keychain prompt on
macOS, no keyring on Linux. The cost is that the profile's cookies are not
protected by the system keychain, so the profile directory is the person's alone
(0700), the browser does not offer to save passwords, and `reset` signs every
console out by deleting it.
"""
import json
import os
import shutil
import subprocess
import sys
import time
import urllib.request
from pathlib import Path

SANDBOX_EXIT = 3


def refuse_in_sandbox():
    """Exit with status 3 in the Workbench sandbox, saying which skill to use."""
    if os.environ.get("ASGARD_SANDBOX_MODE") == "true" or os.environ.get("ASGARD_BROWSER_CDP_URL"):
        sys.stderr.write(
            "desktop-browser is for a person's own computer, and this is the Workbench sandbox "
            "(ASGARD_SANDBOX_MODE=true or ASGARD_BROWSER_CDP_URL is set).\n"
            "Use the sandbox's built-in \"browser\" skill: it connects to the browser the member "
            "watches and takes over (ASGARD_BROWSER_CDP_URL).\n"
        )
        sys.exit(SANDBOX_EXIT)


refuse_in_sandbox()

PORT = int(os.environ.get("ASGARD_DESKTOP_BROWSER_PORT", "9333"))
CDP_URL = "http://127.0.0.1:%d" % PORT

# A run of characters a token, key or secret is made of, long enough that no
# button label or heading is one. Masked in everything this module reads.
SECRET_RUN = r"[A-Za-z0-9_\-:=+/.]{32,}"


def home():
    """Where the profile, the workspaces and the downloads live: beside asgard-cli's own
    configuration (ASGARD_CLI_HOME, or the user configuration directory), never in a
    repository."""
    base = os.environ.get("ASGARD_CLI_HOME")
    if not base:
        if sys.platform == "win32":
            base = os.path.join(os.environ.get("APPDATA") or str(Path.home()), "asgard-cli")
        elif sys.platform == "darwin":
            base = str(Path.home() / "Library" / "Application Support" / "asgard-cli")
        else:
            base = os.path.join(os.environ.get("XDG_CONFIG_HOME") or str(Path.home() / ".config"), "asgard-cli")
    return Path(base) / "desktop-browser"


PROFILE = home() / "profile"
WORKSPACE = home() / "work"
DOWNLOADS = home() / "downloads"


def running():
    """The browser's /json/version when it answers on CDP_URL, else None."""
    try:
        with urllib.request.urlopen(CDP_URL + "/json/version", timeout=1) as r:
            return json.loads(r.read().decode("utf-8"))
    except (OSError, ValueError):
        return None


def chromium():
    """Playwright's own Chromium, or None with the reason printed."""
    try:
        from playwright.sync_api import sync_playwright
    except ImportError:
        sys.stderr.write("Playwright is not installed in this Python. Once per computer:\n"
                         "    .venv/bin/python -m pip install playwright\n"
                         "    .venv/bin/python -m playwright install chromium\n")
        return None
    with sync_playwright() as p:
        path = p.chromium.executable_path
    if not path or not os.path.exists(path):
        sys.stderr.write("Playwright's Chromium is not downloaded yet. Once per computer:\n"
                         "    .venv/bin/python -m playwright install chromium\n")
        return None
    return path


# Playwright passes these to every Chromium it launches. Without them Chromium keeps
# the key that encrypts cookies in the system keychain, and macOS stops the person
# with a password dialog about "Chromium Safe Storage" they have no reason to trust.
NO_OS_KEYCHAIN = ["--use-mock-keychain", "--password-store=basic"]

# With no keychain behind it, a password the browser saved would sit in the profile
# all but readable, and the agent drives this browser. So it never offers to.
PREFS = {
    "credentials_enable_service": False,
    "profile": {"password_manager_enabled": False},
    "autofill": {"profile_enabled": False, "credit_card_enabled": False},
}


def merge(into, extra):
    for key, value in extra.items():
        if isinstance(value, dict) and isinstance(into.get(key), dict):
            merge(into[key], value)
        else:
            into[key] = value


def seed_profile():
    """Before the window opens: a profile only this user can read, downloads going to
    DOWNLOADS without a prompt (first use only), and no saved passwords or autofill
    (every start, since the person can switch them back on)."""
    prefs = PROFILE / "Default" / "Preferences"
    prefs.parent.mkdir(parents=True, exist_ok=True)
    DOWNLOADS.mkdir(parents=True, exist_ok=True)
    if sys.platform != "win32":
        os.chmod(PROFILE, 0o700)
    data = {}
    if prefs.exists():
        try:
            data = json.loads(prefs.read_text(encoding="utf-8"))
        except ValueError:
            data = {}
    else:
        data = {"download": {"default_directory": str(DOWNLOADS), "prompt_for_download": False}}
    merge(data, PREFS)
    prefs.write_text(json.dumps(data), encoding="utf-8")


def start(url=None):
    if running():
        print_info()
        return 0
    exe = chromium()
    if not exe:
        return 1
    seed_profile()
    WORKSPACE.mkdir(parents=True, exist_ok=True)
    args = [exe,
            "--remote-debugging-port=%d" % PORT,
            "--user-data-dir=%s" % PROFILE,
            "--no-first-run",
            "--no-default-browser-check",
            *NO_OS_KEYCHAIN,
            url or "about:blank"]
    kwargs = {"stdin": subprocess.DEVNULL, "stdout": subprocess.DEVNULL, "stderr": subprocess.DEVNULL}
    if sys.platform == "win32":
        kwargs["creationflags"] = subprocess.DETACHED_PROCESS | subprocess.CREATE_NEW_PROCESS_GROUP
    else:
        kwargs["start_new_session"] = True
    subprocess.Popen(args, **kwargs)
    for _ in range(60):
        if running():
            print_info()
            return 0
        time.sleep(0.25)
    sys.stderr.write("Chromium was started but %s does not answer. Another program may hold port %d; "
                     "set ASGARD_DESKTOP_BROWSER_PORT to another port and start again.\n" % (CDP_URL, PORT))
    return 1


def reset(everything=False):
    """Delete the profile, which signs every console out. With everything, the workspace
    and downloads go too: screenshots and files from an admin console carry the
    customer's data. Refuses while the window is open, so nothing is deleted under it."""
    if running():
        sys.stderr.write("The window is still open. Ask the person to close it (quit Chromium), then reset again.\n")
        return 1
    targets = [PROFILE] + ([WORKSPACE, DOWNLOADS] if everything else [])
    for path in targets:
        if path.exists():
            shutil.rmtree(path)
            print("deleted:   %s" % path)
    print("The next start opens a new profile; the person signs in again.")
    return 0


def print_info():
    info = running()
    print("running:   %s" % ("yes, %s" % info.get("Browser", "") if info else "no - run start"))
    print("cdp:       %s" % CDP_URL)
    print("profile:   %s" % PROFILE)
    print("workspace: %s" % WORKSPACE)
    print("downloads: %s" % DOWNLOADS)
    return 0 if info else 1


# ── helpers for scripts ──────────────────────────────────────────────────


def connect(p):
    """Connect a sync_playwright() instance to the running window. Never launch one."""
    if not running():
        sys.stderr.write("No desktop browser on %s. Start it first:\n"
                         "    .venv/bin/python .agents/skills/desktop-browser/scripts/desktop_browser.py start\n" % CDP_URL)
        sys.exit(1)
    return p.chromium.connect_over_cdp(CDP_URL)


def live_page(browser):
    """The tab the person sees, brought to the front."""
    context = browser.contexts[0]
    page = context.pages[0] if context.pages else context.new_page()
    page.bring_to_front()
    return page


def snapshot(page, locator="body", limit=None):
    """The ARIA snapshot of `locator`, with long secret-looking runs masked.

    The mask is applied to the text read back, not to the page, so the person sees
    the page unchanged."""
    import re
    # A password or one-time code is often short, so the length rule does not catch
    # it; its value is masked wherever a textbox line shows it. The values are read
    # only to be compared and are never printed.
    hidden = set(v for v in page.evaluate(_HIDDEN_VALUES) if v)
    text = page.locator(locator).aria_snapshot()
    lines = []
    for line in text.splitlines():
        m = re.match(r"^(\s*- textbox[^:]*: )(.*)$", line)
        if m and m.group(2).strip('"') in hidden:
            line = m.group(1) + "[MASKED]"
        lines.append(line)
    text = re.sub(SECRET_RUN, "[MASKED]", "\n".join(lines))
    if limit and len(text) > limit:
        text = text[:limit] + "\n... (truncated)"
    return text


_HIDDEN_VALUES = r"""() => [...document.querySelectorAll('input')]
  .filter(el => el.type === 'password' || /one-time-code|password/.test(el.autocomplete || ''))
  .map(el => el.value)"""

_MARK = r"""(re) => {
  const rx = new RegExp(re);
  let n = 0;
  for (const el of document.querySelectorAll('input, textarea, [contenteditable="true"]')) {
    if (el.type === 'password' || /one-time-code|password/.test(el.autocomplete || '') || rx.test(el.value || el.textContent || '')) { el.setAttribute('data-desktop-browser-mask', ''); n++; }
  }
  const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  let t;
  while ((t = walker.nextNode())) {
    if (rx.test(t.nodeValue) && t.parentElement) { t.parentElement.setAttribute('data-desktop-browser-mask', ''); n++; }
  }
  return n;
}"""

_UNMARK = r"""() => { for (const el of document.querySelectorAll('[data-desktop-browser-mask]')) el.removeAttribute('data-desktop-browser-mask'); }"""


def screenshot(page, path):
    """Save the visible viewport to `path`, with password fields and anything holding a
    long secret-looking run blacked out in the image. Nothing the person sees changes."""
    Path(path).parent.mkdir(parents=True, exist_ok=True)
    page.evaluate(_MARK, SECRET_RUN)
    try:
        page.screenshot(path=str(path), mask=[page.locator("[data-desktop-browser-mask]")])
    finally:
        page.evaluate(_UNMARK)
    return path


def main(argv):
    cmd = argv[0] if argv else "info"
    if cmd == "start":
        return start(argv[1] if len(argv) > 1 else None)
    if cmd == "info":
        return print_info()
    if cmd == "reset":
        return reset("--all" in argv[1:])
    sys.stderr.write("usage: desktop_browser.py start [<url>] | info | reset [--all]\n")
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
