MILL_REVIEW_BEGIN
# Review: Launch ly-supervise and orchestrator via lyx reed add

```yaml
duration_s: 106.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:design] --if-absent table has no hidden-anchor branch
**Section:** `### add-if-absent-flag` **Issue:** A hidden strand never owns a pane (`render/types.go:35`), so it is permanently "name present and not alive" and the stated relaunch branch would materialize a pane for it — while `planResumeLaunches` deliberately skips hidden strands (`lifecycle.go:152`), contradicting the decision's own "identical to what resume would have done" rationale. **Fix:** State the fourth branch — a name-matched hidden strand is a no-op (or an error) and is never relaunched — and add it to the `reedengine` decision-table tests.

### [BLOCKING:design] Duplicate resolved names have no tie-break
**Section:** `### add-if-absent-flag` **Issue:** The decision keeps bare `add`'s permission to create duplicate names (`AddStrand`/`resolveStrandName` do no uniqueness check), so "matches it against this worktree's persisted strand table" is undefined when two or more strands carry the resolved name — first match, last match, and alive-preferred give different outcomes. **Fix:** Name the selection rule explicitly (e.g. first alive match, else first match in persisted order) and cover it in the decision-table tests.

### [BLOCKING:design] Attach task's keyboard focus unspecified
**Section:** `### sequenced-tasks-not-shell-operators` **Issue:** The per-task presentation split fixes `echo`/`reveal`/`panel` but never `presentation.focus`, whose VS Code default is `false` — the attach terminal would be revealed but not focused, defeating the stated requirement that it "must be the one they land in". **Fix:** Decide `focus` for each of the four tasks (at minimum `focus: true` on the attach step) and assert it in `internal/vscode/config_test.go`.

### [NIT:scope] Entry task's command-less shape not pinned
**Section:** `### sequenced-tasks-not-shell-operators` **Issue:** "The entry `Start Claude` task runs no command of its own" leaves its `type`/`command` keys unstated, and today's literal always carries `"type": "shell"` (`internal/vscode/config.go`). **Fix:** State the exact entry-task key set (label, dependsOn, dependsOrder, runOptions, no command) so the generated-shape test has something to assert.

## Verdict

REQUEST_CHANGES
Three decision gaps: hidden-strand branch, duplicate-name tie-break, and attach-task focus.
MILL_REVIEW_END
