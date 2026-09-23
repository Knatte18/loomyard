MILL_REVIEW_BEGIN
# Review: llm-driven child can park forever on Claude Code's own workspace-trust dialog — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewed_file: plan/
date: 2026-09-23
```

## Findings

None. Verified against source and against `_mill/discussion.md`:

- Card 1's `AwaitStarted` pseudocode's outcome/error branches match `checkLivenessTick`'s actual return space (`wait.go`): `classifyStartupWindow`/`classifyDeadlineExpiry` only ever yield `""`, `OutcomeDied`, or `OutcomeDone`, so the switch is exhaustive.
- The `completionsignal_enforcement_test.go` edits are counted correctly: the three retry-cap `fmt.Errorf` returns in the pseudocode key as `"AwaitStarted [Errorf]": 3` under `scanNegativeVerdictReturns` (the `case errStrandNotTracked:`/`errStrandPaneBindingCleared:` guards sit in the switch tag, not the return's own results, so they add no extra marker); the two `allOutputFilesExist` calls inside `AwaitStarted` key as `"AwaitStarted": 2`, and the existing map's per-function counts sum to 8, so "Eight sites" → "ten" is arithmetically correct once both land.
- Card 3's quoted doc-comment substrings (`driverHandle`'s doc, `startLLMDriverArm`'s "run the pane-liveness probe against it", `runDriverSpawnAndWait`'s "the llm arm's strand launch and pane-liveness probe", the `Long` text's "the strand's own pane coming alive for an ly-drive driver", the `--no-attach` usage string) are verified verbatim against `internal/loomcli/start.go`/`driverlaunch.go` as they exist today.
- The `bootstrapLock`/run-lock handshake claim ("the ly-drive session's own first `lyx shed step` waits on the same lock until this bootstrap releases it") is verified against `internal/loomcli/arm.go`'s `loomPreStep`, which calls the blocking `lock.AcquireWriteLock(bootstrapLockPath)` on the same `loomengine.LoomBootstrapLock` path `start.go` holds.
- Card 4's `driverShuttleConfig`/`writeStubDriverScript` claims check out against `internal/shuttleengine/template.yaml` (`startup_timeout_s: 90` and the `claude:` key text both present verbatim) and `claudeengine/startup.go`'s `Startup` (`strings.Contains(normalized, "shortcuts")`); the `"? for shortcuts"` marker text matches the established fixture convention already used throughout `claudeengine/startup_test.go`, not a stray character.
- Card 3's deletions (`TestAwaitDriverPane_*` x5, the now-unused `errors` import) match `driverlaunch_test.go`'s actual contents, and `countingWait` (kept) is confirmed still used by `bootstrap_test.go`'s own handshake tests independent of the deleted tests.
- `integration_driverbootstrap_test.go` is confirmed to call `startLLMDriverArm` directly and never `runDriverSpawnAndWait`/`AwaitStarted`, supporting the "left to `done_gate`" claim; `mill-config.yaml`'s `done_gate` matches the overview's quoted value verbatim.
- `docs/overview.md`'s bootstrap bullet is confirmed to name only what is spawned, not the readiness signal, supporting the "not edited" decision; a repo-wide grep confirms no `docs/` file currently carries the pane-liveness wording the batch's own pre-commit grep guards against.
- `00-overview.md`'s "All Files Touched" list is the exact union of both batches' `Edits:`/`Creates:` targets (10 files, no drift); the Batch Index DAG is acyclic and both named batch files exist; global step numbering (1–4) is sequential with no gaps; every card has all five required fields and no `Moves:` (so no Rename mechanic section is needed, correctly).

## Verdict

APPROVE
No constraint violations, decision-alignment gaps, or scope/consistency defects found across either batch.
MILL_REVIEW_END
