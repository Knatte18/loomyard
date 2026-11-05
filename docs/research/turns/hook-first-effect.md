# Effect of `shuttle-hook-first` (commit `86982fa43`)

Measured 2026-10-13 by a Sonnet agent, read-only.
The change makes shuttle read a Claude session's state from recorded hook lines (SessionStart, PermissionRequest, Notification, SessionEnd) before the pane.

## Answer

No measurable effect either way; confidence is low.

## Caveats

- Log survivorship: task-pair logs are deleted at teardown, so shuttle and batten trace logs cover only the hub, the fan-lab runs and the pairs alive on 10-12 and 10-13.
- No per-slug batten logs exist for 10-08 to 10-12.
- Deploy date uncertain: `Land-deploy.log` files start 10-12 09:26, run settings with the new `PermissionRequest` hook appear only from 10-12 19:04, so "after" in practice starts 10-12 between 09:26 and 20:00, not 10-11 19:43.
- Windows: before 10-09 00:00 to 10-11 19:43 (2.8 days), after 10-11 19:43 to 10-13 09:00 (1.6 days).

## Counts

| Measure | Before | After | Source |
|---|---|---|---|
| Archived tasks (denominator) | 69 | 47 | `archive/*` tags in `loomyard-weft` |
| Drive reports (denominator) | 131 | 60 | `_lyx/shed/*/drive-reports` in the archive tags |
| Shuttle runs in traces (denominator) | 23 (20 fan-lab, 1 orch, 2 unfinished) | 77 (6 live, 71 finished) | `.lyx/logs`, `shuttle: run started/finished` |
| Startup failures of a spawned session | 3 in 131 reports (`loom-small-bugs` "provider never became ready", `batten-notice-trim` re-step, `webster-replan-state` fix half); issues #527, #549 | 0 in 60 reports; two alignment-agent `startup_timeout` deaths on 10-12 (blank pane) | drive reports, `run-window-bugs`, issues |
| Runs ended `died` or not `done` in traces | 0 of 20 finished | 0 of 71 finished | `shuttle: run finished` |
| Runs that sat out a watchdog or deadline | 0 | 2 (#566, #567; a fix chair with no hook events for about 11 h, watchdog after 10h50m) | issues, drive reports `config-layers-run-settings`, `driver-arbiter` |
| Registry reading `state=dead` | 0 | 3 (10-12 19:14, 20:09, 20:10) | `shuttle: session state` |
| Lone stray-character holds | 1 orch draft hold `"O"` (5 ticks) | 0 | `orch: injection held` |
| Draft holds (mostly operator typing) | 274 lines, 57 distinct drafts, 0 in batten | 135 lines, 18 distinct | `orch: injection held` |
| Batten notices | no data | 189 `session state at notice` (173 quiet/busy, 16 idle-stalled), 397 `producer stuck` | `battenshed` |
| Sends not landed or retried | 1 "send released an unmatched turn start" | 4 color sends refused ("a turn is running") | shuttle WARN lines |
| Gate re-prompts (review-file YAML, unrelated) | 1 of 23 runs | 6 of 77 runs | `shuttle: gate: re-prompting` |
| GitHub issues on these | 4 (#485, #527, #549, #554) | 2 (#566, #567, both stalls) | `gh issue list` |

## Reading

- Stray-character and draft reports dropped (#554 closed, none after), and startup failures in drive reports went from 3 to 0; suggestive, not proof.
- The two blank-pane startup timeouts on 10-12 fit the change making startup stricter (it needs a `SessionStart` line and a ready pane).
- The two 11-hour stalls started 10-11 21:22 local, possibly before the binary had the change; their events files hold only `SessionStart` and `UserPromptSubmit`, which the dead-process check would not catch.
- Main finding: such changes cannot be measured while pair logs are deleted at teardown.
