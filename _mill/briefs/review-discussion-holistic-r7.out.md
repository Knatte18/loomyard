MILL_REVIEW_BEGIN
# Review: Replace reed's header pane with a status-line and Selvage

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model, as reported by the harness (claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:design] Grep token set cannot find header-adjacent sites
**Section:** Testing → "Disposition per header-adjacent site, re-derived by a stated method"
**Issue:** The four tokens (`reed header`, `HeaderPaneID`, `console-header`, `headerpane`) match none of the real out-of-package prose sites — verified: `tools/sandbox/` contains zero occurrences of `reed header`, while `SANDBOX-REED-SUITE.md` carries "header pane", "header.height_rows", "topmost", and "the header pane's log"; likewise `internal/tokenvocab/doc.go:5`/`render.go:2` ("reed's header pipeline") and `internal/standalonegeom/reedgeom.go:43` + `standalonegeom_test.go:178` ("the header pane's display token"), the last being a file the task already edits.
**Fix:** Widen the stated method to a case-insensitive `header` scan over `.md` and `.go` (minus `.git/`, `_mill/`, and HTTP/markdown false positives), so the plan's "any site not listed below is unhandled" rule is actually enforceable.

### [BLOCKING:decision] Sandbox reed suite has no stated disposition
**Section:** Scope / Constraints / Testing
**Issue:** `tools/sandbox/SANDBOX-REED-SUITE.md` scenario M19 ("Always-on header pane (operator console)") asserts a topmost pane whose *visible content* is the rendered text, `header.height_rows`, "the freshly rebuilt header ends up alone, full-height, at the very top" (line 373), and — for the watchdog scenarios at lines 409/427/451 — "the header pane's log" as the place the watchdog's warnings appear. All four are falsified by this task, and the daemon's logs now land in `HubLogsDir`. CONSTRAINTS.md's **Sandbox Suite Coverage** invariant is also absent from the discussion's Constraints list.
**Fix:** State per-scenario dispositions for `SANDBOX-REED-SUITE.md` (M19 rewrite for Selvage, height key rename, watchdog-log relocation) and confirm `SANDBOX-REED-WATCH-SUITE.md` needs none, and add Sandbox Suite Coverage to the Constraints section.

### [NIT:decision] loomcli comment left as an either/or
**Section:** Testing → outside-the-reed-packages list
**Issue:** `internal/loomcli/bootstrap.go:183` is dispositioned as "repointed (the daemon spawn in `reedcli` is the surviving analog) **or** dropped" — two alternatives with no choice, in a file the plan will touch once.
**Fix:** Pick one so the plan writer does not have to.

## Verdict

REQUEST_CHANGES
Enumeration method misses the sandbox reed suite, which is left undispositioned.
MILL_REVIEW_END
