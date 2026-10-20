# CLI Tool Mode

Default runs produce a one-shot `final_script.py` that solves the task for the
literal values the user provided. **CLI tool mode** instead produces a
**reusable, parameterized CLI tool**: the same script can be re-run later with
different argument values to perform the same kind of task.

You still verify every critical point yourself against `plan.md` (open the saved
screenshots and the action log).

## When to use

Trigger CLI tool mode when the user says "make it reusable", "parameterize",
"turn this into a CLI", "I want to call this again with different X", or similar.
Otherwise, stay in default one-shot mode.

## `plan.md` - add a `# Parameters` section

Before writing the script, identify every requirement the user could plausibly
vary and list them in `plan.md` **in addition to** the usual `# Critical Points`
checklist:

```markdown
# Task
<verbatim task description>

# Parameters
| name    | type | source phrase from task | default     | allowed / format        |
|---------|------|-------------------------|-------------|-------------------------|
| <arg_a> | str  | "..."                   | "<value>"   | <format / allowed set>  |
| <arg_b> | int  | "..."                   | <value>     | <range or units>        |

# Critical Points
- [ ] CP1: ...
- [ ] CP2: ...
```

Rules:

- Every entry in `# Parameters` must (a) become a function argument and (b)
  become an `argparse --flag` with the listed default.
- Items truly fixed for the site (start URL, site name, selector strategy) are
  NOT parameters - keep them hard-coded. The CDP endpoint and the downloads
  directory come from `desktop_browser` (`connect()`, `DOWNLOADS`), never
  parameters.
- Defaults reproduce the original task exactly. `.venv/bin/python final_script.py` with no
  arguments, run from the repository root, must reproduce the task.
- Critical Points are still required; they are the verification contract.

## `final_script.py` - required shape

1. **One reusable function** named after the task domain
   (`def search_<domain>(arg_a, arg_b, ...): ...`).

2. **Google-style docstring** with summary, full `Args:` block (name, type,
   meaning, format/units, default - mirroring the `# Parameters` table), and
   `Returns:`.

   ```python
   def search_<domain>(arg_a: str, arg_b: int) -> dict:
       """<One-line summary of what this tool does on the target site>.

       Args:
           arg_a: <what it represents>; <format / allowed values>. Default: "<value>".
           arg_b: <what it represents>; <range / units>. Default: <value>.

       Returns:
           dict with keys ``<key1>`` (<type>), ``<key2>`` (<type>).
       """
   ```

3. **`argparse` CLI** under `if __name__ == "__main__":`. Every function argument
   has a matching `--<arg>` flag with `type=`, `help=` (copied from the
   docstring), and `default=` equal to the concrete task value:

   ```python
   if __name__ == "__main__":
       import argparse
       parser = argparse.ArgumentParser(description=search_<domain>.__doc__.splitlines()[0])
       parser.add_argument("--arg-a", dest="arg_a", type=str, default="<value>", help="<from docstring>")
       parser.add_argument("--arg-b", dest="arg_b", type=int, default=<value>, help="<from docstring>")
       args = parser.parse_args()
       result = search_<domain>(**vars(args))
       print(result)
   ```

4. **Side-effect-free at import time.** No CDP connect, no network call, no file
   write at module top-level (importing `desktop_browser` is fine: it only checks
   it is not in the Workbench sandbox) - the reusable function must be importable from
   another process without triggering a run.

5. **Action-log parameter echo.** The first line written to `final_script_log.txt`
   after reset MUST be a `step 0 params: ...` line listing every resolved
   argument as `name=value` pairs:

   ```
   step 0 params: arg_a=<value> arg_b=<value>
   ```

6. Same instrumentation as default mode: connect over CDP with `connect()`, reuse
   the visible tab, never close the browser, screenshots through `screenshot()`, screenshots saved as
   `final_runs/run_<id>/screenshots/final_execution_<step>_<action>.png`, final
   datum appended to `final_script_log.txt`.

## Verification

In addition to the default self-verification (every CP ticked with cited
screenshot/log evidence), CLI mode requires:

1. **Reproduce the task with no arguments.** Inside a fresh `final_runs/run_<id>/`:

   ```bash
   .venv/bin/python <workspace>/<task_id>/final_runs/run_<id>/final_script.py
   ```

   The run must succeed end-to-end and produce the expected screenshots and the
   `step 0 params: ...` log line.

2. **Import-safety smoke test.** From the repository root:

   ```bash
   .venv/bin/python -c "import importlib.util; \
     spec = importlib.util.spec_from_file_location('fs', '<workspace>/<task_id>/final_runs/run_<id>/final_script.py'); \
     m = importlib.util.module_from_spec(spec); spec.loader.exec_module(m); \
     print([n for n in dir(m) if not n.startswith('_')])"
   ```

   This must complete instantly with no CDP connect and print the reusable
   function's name.

3. **Optional second run with a different argument value** to demonstrate
   parameterization actually works.

4. **Print `--help`** and show the user, so they know how to call it again:

   ```bash
   .venv/bin/python <workspace>/<task_id>/final_runs/run_<id>/final_script.py --help
   ```

## Completion gate (CLI mode)

Set the task complete only when **all** are true:

1. `plan.md` contains both `# Parameters` and `# Critical Points`.
2. `final_script.py` defines exactly one reusable function with a Google-style
   `Args:` docstring covering every parameter.
3. Every `# Parameters` entry maps 1-to-1 to a function argument **and** an
   argparse `--flag` whose default equals the concrete task value.
4. The script is import-safe (smoke test passes).
5. `.venv/bin/python final_script.py` (no args, from the repository root) reproduced the task; all CPs verified.
6. `step 0 params: ...` line is present in `final_script_log.txt`.
7. The user has seen the final datum **and** the `--help` output.

If any of those is false, do not declare done - diagnose, fix the script
(preserving the CLI shape), re-run inside `run_<id+1>/`, and re-verify.
