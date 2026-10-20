# Workflow

Detailed expansion of the six-step loop. You verify screenshots yourself (open
the PNG files and reason against `plan.md`) - there is no external image-QA
service.

## 1. Plan

Parse the task into critical points (CPs) and write `WORKSPACE_DIR/plan.md`:

```markdown
# Task
<verbatim task description>

# Critical Points
- [ ] CP1: <constraint / filter / sort / selection / required datum>
- [ ] CP2: ...
```

Rules for CPs:

- One CP per independently verifiable requirement.
- Numeric, date, quantity, and unit CPs must be exact.
- Ranking CPs ("cheapest", "best-selling", "highest-rated", …) must reference the
  site's actual sort/filter control.
- If the task asks for a final datum, make it its own CP
  (e.g. `CP5: Record the displayed cheapest economy fare`).

## 2. Explore

Goal: discover stable selectors, confirm every required control exists, and
identify how to capture evidence for each CP.

- Run scratch Playwright scripts (see `playwright_patterns.md`) against the
  running window, with `WORKSPACE_DIR/` (outside the repository) for everything
  they write. Save scratch PNGs under
  `WORKSPACE_DIR/screenshots/` (separate from `final_runs/`).
- Print URL, title, and `snapshot()` for the region of interest each step.
- Open saved PNGs to confirm UI state when ARIA evidence is ambiguous.
- If a filter looks unavailable, expand drawers / accordions / mobile filter
  panels and inspect again before concluding it doesn't exist.
- A search-box query never substitutes for a dedicated filter control.
- Remember the window is the person's too and stays open - leave it on a
  sensible page, and don't close it. Where the page needs them (signing in, a
  choice that is theirs), stop and ask; that part is done step by step with them,
  not inside `final_script.py`.

## 3. Author `final_script.py`

Create a fresh `final_runs/run_<id>/` (next integer above any existing `run_*`)
and place `final_script.py` inside it. Instrument per `playwright_patterns.md`:

- connect over CDP with `connect()`, reuse the visible tab, never close;
- one `final_execution_<step>_<action>.png` per CP, saved through `screenshot()`;
- one `step <n> action: <reason and action>` log line per constraint-relevant
  interaction;
- the final datum printed into `final_script_log.txt` at the end.

Each screenshot should map to a CP from `plan.md` so verification is trivial.

## 4. Execute

Run the script once. If it crashes, fix it inside the same run folder and
re-execute - but if a partial run already produced screenshots that don't match
the fixed flow, delete them so the run folder reflects a single clean execution.

## 5. Self-verify

For every CP in `plan.md`:

1. Identify the screenshot(s) and/or log line that provide evidence.
2. Open each cited PNG.
3. Confirm the evidence is **unambiguous**:
   - Filter chip / selected state visibly applied (not hidden behind a closed
     drawer);
   - Numeric / date values match exactly (not broadened);
   - Sort applied via the site's control (not implied by result order);
   - Required submit / search / apply action visibly taken;
   - Final datum legibly displayed.
4. Tick the CP only when the evidence is concrete. Be harsh on partial, occluded,
   or ambiguous states.

If any CP fails, diagnose the *specific* issue - wrong filter value, missing
control, hidden chip, broadened range, missing confirmation, missing screenshot,
etc. Fix `final_script.py`, run it again inside `final_runs/run_<id+1>/`, and
re-verify against `plan.md`. Empty result sets are acceptable when the correct
filters were demonstrably applied.

## 6. Done

Stop only when **all** of the following are true:

1. `plan.md` exists with every CP enumerated as a checklist item.
2. `final_runs/run_<id>/final_script.py` ran cleanly and produced
   `final_script_log.txt` plus all CP screenshots.
3. Every CP is checked off with a cited screenshot and/or log line.
4. The final datum (if asked for) is reported to the user verbatim and is also
   present in `final_script_log.txt`.
5. `ls -R final_runs/run_<id>` and `cat final_runs/run_<id>/final_script_log.txt`
   show the expected artifacts.

If any of those is false, do not declare done - diagnose, fix, and re-run in a
new `run_<id+1>/`.
