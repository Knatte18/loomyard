MILL_REVIEW_BEGIN
# Review: Launch ly-supervise and orchestrator via lyx reed add

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5
reviewed_file: _mill/discussion.md
date: 2026-09-18
```

## Findings

### [BLOCKING:design] `--cmd claude` keeps the PATH trap the lyx stamp removes
**Section:** §Decisions / vscode-task-is-the-launch-surface **Issue:** The stamped-absolute-path rationale ("a `folderOpen` task's shell does not inherit a login shell's PATH") applies verbatim to the `claude` in `--cmd claude`: `reed up` boots the per-hub tmux server with `CleanClaudeEnv(os.Environ())` of the calling process (`internal/reedengine/env.go`, `lifecycle.go` boot path), so the pane shell inherits the same impoverished PATH, and because the server is long-lived per hub that PATH is frozen at first boot — plus `Cmd` is persisted, so every later `resume` replays the same bare string. **Fix:** State a disposition for how `claude` is spelled in `--cmd` (bare name accepted with stated reason, or resolved via `exec.LookPath` at `ide spawn` time and stamped like `lyx`), and say whether the bare-name fallback rule matches the `lyx` one.

### [BLOCKING:design] Hidden-only-match branch has no stated success envelope
**Section:** §Decisions / add-if-absent-flag (row 4) + §Testing / `internal/reedcli` **Issue:** The branch table's `matched` non-empty / `candidates` empty row is specified as "a no-op, nothing added" but never says what `AddStrand` returns or what `output.Ok` prints — there is no candidate strand to report — while §Testing promises "the success envelope shape is unchanged in the no-op case (an operator script reading `guid` must not have to special-case it)". A plan writer can equally implement an empty-`Strand` return (`{"guid":"","name":""}`) or the first matched hidden strand. **Fix:** Name the envelope contents for this row (and, for symmetry, for the relaunch row) explicitly.

### [NIT:design] Relaunch branch's persist/apply obligations unstated
**Section:** §Decisions / add-if-absent-flag, relaunch branch **Issue:** The alive-no-op branch explicitly states "Nothing is persisted and no layout apply runs", but the relaunch branch says only that it goes through `launchStrandLocked`; `Resume` persists after each launch and re-applies the layout per launch (`lifecycle.go:765-780`), and `AddStrand` persists then calls `reconcileApplyPersistLocked` — the asymmetry leaves the new `PaneID`'s durability and the layout apply undecided. **Fix:** State that the relaunch branch persists the new binding before the apply and re-applies the layout, matching `Resume`/`AddStrand`.

## Verdict

REQUEST_CHANGES
Two unresolved decisions: `claude`'s PATH spelling and the hidden-match no-op envelope.
MILL_REVIEW_END
