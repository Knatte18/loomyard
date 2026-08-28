# Batch: smoke-regressions

```yaml
task: 'reed: pane reap isn''t applied consistently across up/add''s mutating paths'
batch: 'smoke-regressions'
number: 4
cards: 6
verify: go test -tags smoke -timeout 20m ./internal/reedcli/ -run 'TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable|TestSmokeHeaderPaneSurvivesUpAddRemoveAndReconcile|TestSmokeForeignPaneIsReapedNotAdoptedByAdd|TestSmokeUpAfterScrubbedStateLeavesOnlyTheRebuiltHeader|TestSmokeStrandPaneSpawnsAtToldAnchorNotProcessCwd|TestSmokeRemoveLastStrandThenAddRunsTheNewCommand'
depends-on: [2]
```

## Batch Scope

This batch delivers the real-tmux tier: the two new regressions for M16 and M22, plus the repair of every existing smoke test whose premise batches 1 and 2 falsified.
It is one batch because all six cards live in `internal/reedcli`'s smoke files, share one harness (`hubforge.NewHub` + `t.Chdir` + a `down` cleanup + the `smoke_test.go` helper set), and are graded by the same `-tags smoke` run.

It depends on batch 2 because both new regressions are written against the fixed behaviour, and because three of the existing tests only start failing once the chokepoint lands.
It is parallel-safe with batch 3, which touches only `internal/reedengine` and `tools/sandbox` — a disjoint set.

Batch-local decisions beyond `## Shared Decisions`:

Both new regressions assert on `#{pane_pid}` and never on that pid's descendants.
The reap is `kill-pane`-only by decision, and tmux terminates a pane's children asynchronously — the process actually holding a worktree can be a deeper descendant — so descendant liveness is not what these tests pin.
`RemoveStrand` and `Down` are where subtree death is guaranteed and asserted.

Both assert the pid under a bounded poll rather than sampling once immediately after the verb returns, for the same asynchrony reason.

The pid check is not decoration.
A pane-id-only assertion would have passed for the adoption bug had ids been recycled, whereas under adoption the pane pid provably survives — that identity is exactly what M16 recorded.

## Cards

### Card 13: Rewrite the foreign-pane test to the post-reap premise

- **Context:**
  - `internal/reedcli/smoke_test.go`
  - `internal/reedengine/reconcile.go`
  - `internal/reedengine/apply.go`
  - `internal/reedengine/state.go`
- **Edits:**
  - `internal/reedcli/smoke_lifecycle_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite `TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable`, whose premise is now false: with the header alive and zero strands, the reap fires on that test's second `lyx reed up` and kills both the unadopted initial pane and the foreign pane, while its comments assert the opposite ("the foreign panes survive an up", "Every pane must survive it").
  Its surviving `len(panes) == 0` assertion is loose enough to keep passing while its stated premise is false, which is worse than a failure.
  Rewrite the comments — the doc comment above the function and the two inline comments naming the pre-fix expectation — to the new behaviour.
  Tighten the post-`up` assertion from "not empty" to the exact expected pane set: after the second `up` the session holds exactly one pane, and it is the persisted `HeaderPaneID` the test already reads out of `reedengine.LoadState`.
  Use the existing `listPaneLines` and `paneLiveOnSession` helpers.
  What the test exists to pin is unchanged and must stay pinned, in both the code and the rewritten comments: `up` with zero placeable strands must never emit a zero-cell layout string, because tmux answers that by destroying every pane and wedging the session.
  Say so explicitly in the rewritten doc comment, since after this change the "exactly one pane survives" assertion is what stands in for it and a future reader could otherwise mistake the header-only end state for the destruction this test guards against.
  Keep the post-`add` assertion (exactly the strand's pane and the header pane, with neither displaced) as it is — it still holds.
  Keep the foreign `split-window` via the real tmux binary, the `socketAndSession` read, and the `down` cleanup unchanged.
- **Commit:** `test(reedcli): retarget the foreign-pane smoke test to the post-reap pane set`

### Card 14: Correct the two smoke tests whose comments assert the deleted adoption seam

- **Context:**
  - `internal/reedcli/smoke_test.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/strand.go`
- **Edits:**
  - `internal/reedcli/smoke_lifecycle_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two existing tests in `internal/reedcli/smoke_lifecycle_test.go` keep asserting what they exist to assert, but describe it in terms of a seam that no longer exists.
  In `TestSmokeHeaderPaneSurvivesUpAddRemoveAndReconcile`, the comment before the first `add` claims the header is excluded from adoption so the strand "lands on the session's other (pre-header) pane".
  The new gate reaps that pane on the preceding `up`, so the strand now lands on a pane split off the header.
  Rewrite that comment and the two later ones that repeat the adoption framing (before the second `add`, and the `t.Errorf` message asserting the header "must never be adopted").
  What the test exists to pin is unchanged and must stay asserted: the header is never a strand's pane and stays alive across `up`, `add`, `remove` and reconcile, including while the strand table is momentarily empty.
  Keep every `requireHeaderAlive` call, both liveness assertions, and the non-header-pane-id assertion exactly as they are.
  In `TestSmokeRemoveLastStrandThenAddRunsTheNewCommand` (psmux/Windows-only, skipped on tmux), the whole framing is that "the old adopt path bound the next strand to that corpse", and the skip string says the remove "never reaches an 'adopt a corpse or not' decision".
  The mechanics still hold with adoption gone — the sole pane is corpsed by `kill-pane`, the corpse is not the header, and the next `add` splits rather than adopting — so what the test asserts (the new strand is live and stays live across the next reconciling verb) is unchanged and must stay asserted.
  Rewrite only the adoption framing: the doc comment, the inline comment before the `runtime.GOOS` check, the inline comment before the reconciling `up`, and the `t.Errorf` message's parenthetical.
  Rewrite the `t.Skip` string too — it is operator-facing output that would otherwise name a decision the code no longer makes — while keeping its pointer to `TestRemoveStrand_SoleStrandEmptiesSessionSucceeds` for the tmux-side coverage.
  Do not change the `runtime.GOOS != "windows"` skip condition itself, and do not change either test's assertions.
- **Commit:** `test(reedcli): drop the deleted adoption framing from two smoke tests`

### Card 15: Add the M16 regression — a foreign pane is reaped, never adopted

- **Context:**
  - `internal/reedcli/smoke_test.go`
  - `internal/reedcli/smoke_procalive_linux_test.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/reconcile.go`
  - `internal/reedengine/state.go`
- **Edits:**
  - `internal/reedcli/smoke_lifecycle_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `TestSmokeForeignPaneIsReapedNotAdoptedByAdd` to `internal/reedcli/smoke_lifecycle_test.go`, the faithful M16 regression.
  M16 fires only when the *sole* alive non-header pane is the foreign one.
  A session that still holds its unadopted initial `new-session` pane has two alive non-header panes, which is why `TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable` passed today despite the bug.
  So the fixture must first drive the session to a header-plus-foreign-pane-only state: `up`, `add` a strand, `remove` that strand, then create the foreign pane with a raw `tmux -L <socket> split-window -t <session>` through the real binary, exactly as the existing foreign-pane test does.
  Capture the foreign pane's id from `listPaneLines` — it is the one pane that is neither the persisted `HeaderPaneID` (read via `reedengine.LoadState` from `.lyx`, as the neighbouring tests do) nor a pane the removed strand held — and capture its `#{pane_pid}` via the existing `paneRootPID` helper, before the `add` under test.
  Then `add` a strand and assert three things.
  The strand's `paneId` from `lyx reed status` is not the foreign pane id.
  The foreign pane id is gone from `list-panes`.
  And the recorded foreign `#{pane_pid}` is no longer alive, polled with the existing `processGone` helper under a bounded deadline rather than sampled once — `kill-pane` terminates a pane's process asynchronously.
  Use the existing `addStrand`, `statusStrand`, `tmuxBinaryPath`, `socketAndSession`, `listPaneLines` and `deferHubRelease` helpers and the `hubforge.NewHub(t, ".")` plus `t.Chdir(h.PrimeWorktree())` plus `down`-cleanup shape every test in this file uses.
  Use `smokeReapLaunchCmd()` for the strand's command.
  Assert nothing about the foreign pid's descendants — the reap is `kill-pane`-only by decision, and descendant liveness is pinned by `RemoveStrand`/`Down`'s own tests, not here.
  Say so in the test's doc comment, alongside why the pid assertion is the load-bearing one: a pane-id-only assertion would have passed for the adoption bug had ids been recycled, whereas under adoption the pane pid provably survives, and that identity is exactly what M16 recorded.
- **Commit:** `test(reedcli): add the M16 regression for foreign-pane reaping`

### Card 16: Add the M22 regression — a scrubbed reed.json converges on the up under test

- **Context:**
  - `internal/reedcli/smoke_test.go`
  - `internal/reedcli/smoke_procalive_linux_test.go`
  - `internal/reedengine/reconcile.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/state.go`
  - `internal/reedengine/apply.go`
- **Edits:**
  - `internal/reedcli/smoke_lifecycle_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `TestSmokeUpAfterScrubbedStateLeavesOnlyTheRebuiltHeader` to `internal/reedcli/smoke_lifecycle_test.go`, the M22 regression.
  Drive the reproduction exactly: `up`, `add` one strand, then delete `.lyx/reed.json` while the session is live, then `up`.
  Before the scrub, capture the old `HeaderPaneID` via `reedengine.LoadState`, the strand's pane id via `lyx reed status`, and that strand pane's `#{pane_pid}` via the existing `paneRootPID` helper.
  After the recovering `up`, re-read the state file and assert three things.
  The session holds exactly one pane.
  That pane is the newly persisted `HeaderPaneID`, which must differ from the captured old header id.
  And both the old header id and the old strand pane id are gone from `list-panes`.
  Asserting on that `up`, with no intervening verb, is the entire point of the test — the pre-fix behaviour converged one verb late, and an assertion placed after a follow-up `add` would pass either way.
  Then assert the orphaned strand pane's captured `#{pane_pid}` is no longer alive, polled with `processGone` under a bounded deadline on the same terms as card 15.
  Note in the test's doc comment that the launched command is a *child* of the pane process rather than `#{pane_pid}` itself, so it is deliberately not what this test asserts on: the leak this pins is the pane and its own process, not the whole subtree.
  Use `smokeReapLaunchCmd()` for the strand's command and the same harness shape as the neighbouring tests (`hubforge.NewHub`, `t.Chdir`, `deferHubRelease`, `down` cleanup, `tmuxBinaryPath`, `socketAndSession`, `listPaneLines`, `addStrand`, `statusStrand`).
  Delete the state file with `os.Remove` against `filepath.Join(h.PrimeWorktree(), ".lyx", "reed.json")`;
  both packages are already imported by this file.
  This test asserts a header-only, full-height end state, which is the accepted outcome rather than a layout defect — `applyLayoutLockedOpts` deliberately skips `select-layout` when no strand owns a present pane.
  Say so in the doc comment so a future reader does not mistake it for a regression and "fix" it by synthesising a spacer pane.
- **Commit:** `test(reedcli): add the M22 regression for scrubbed-state convergence`

### Card 17: Rewrite the pane-cwd test's collapsed control/exercise contrast

- **Context:**
  - `internal/reedcli/smoke_test.go`
  - `internal/reedengine/spawn.go`
- **Edits:**
  - `internal/reedcli/smoke_panecwd_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `TestSmokeStrandPaneSpawnsAtToldAnchorNotProcessCwd` builds its two-case table on the contrast between an adopted first pane — correct by construction, since `new-session` carried `-c` — and a split second pane, the path the `-c` defect broke.
  After this change both strands are splits, so the control/exercise contrast silently collapses into two identical cases and the test would keep passing while no longer testing what its comment claims.
  Rewrite it.
  Drop the adoption framing from the function's doc comment (the paragraph beginning "The FIRST strand adopts the session's initial pane") and from the table, renaming the `adopted` local and the `"adopted initial pane (control)"` case label so neither names a seam that no longer exists.
  Keep both cases: a first-and-subsequent split pair is still worth asserting, because they take different `planPaneTarget` branches — the first strand's split targets the header (the header-as-last-resort fallback, since the reap has by then removed the initial pane), the second targets the tallest alive non-header pane.
  Say that in the rewritten comment, so the two cases read as genuinely distinct rather than duplicated.
  Restate the doc comment so the `-c` regression it guards is still plainly the stated subject: `launchStrandLocked`'s `split-window` carried no `-c`, so tmux resolved the new pane's cwd from the invoking client rather than from the anchor reed was told, and under the `RunCLIIn` seam that is observably the wrong tree.
  Keep the `RunCLIIn` seam, the `elsewhere` temp dir, the `paneCurrentPath` assertion, and the file-header comment explaining why this file drives `RunCLIIn` rather than `t.Chdir` — none of that depends on adoption.
- **Commit:** `test(reedcli): restate the pane-cwd contrast without the adoption control`

### Card 18: Correct the teardown test's stale parenthetical

- **Context:**
  - `internal/reedengine/spawn.go`
- **Edits:**
  - `internal/reedcli/smoke_teardown_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** One comment in `internal/reedcli/smoke_teardown_test.go` reads "Keeper first (adopts the initial pane), then the victim we remove."
  The test's mechanics do not depend on which pane the keeper strand got — only the parenthetical is now false.
  Correct it so it no longer names pane adoption;
  the keeper-then-victim ordering it also conveys is still true and must stay.
  Change nothing else in this file: no assertions, no fixture, no other comment.
- **Commit:** `test(reedcli): drop the stale adoption parenthetical from the teardown fixture`

## Batch Tests

`verify:` runs the `smoke`-tagged suite in `internal/reedcli` filtered to exactly the six tests this batch touches: the two rewritten in cards 13 and 14 (`TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable`, `TestSmokeHeaderPaneSurvivesUpAddRemoveAndReconcile`), the second card-14 test whose comments and skip string change (`TestSmokeRemoveLastStrandThenAddRunsTheNewCommand` — Windows-only, so it skips on this host and is listed to catch a compile break rather than to run), the two new regressions from cards 15 and 16, and card 17's rewritten `TestSmokeStrandPaneSpawnsAtToldAnchorNotProcessCwd`.

Card 18 touches only a comment in `smoke_teardown_test.go` and adds no test to the filter;
`go test -tags smoke` builds the whole package before applying `-run`, so a mis-scoped edit there still fails this gate.

The scope is per-batch rather than the full smoke suite because these tests drive a real tmux server, and the untouched smoke files (`smoke_test.go`'s own suite, resume, state-recovery, proctree, scrollback) each boot their own session — running them all would add minutes for no coverage this batch's changes can affect.
`-timeout 20m` is set explicitly because the package's default `go test` timeout applies to the whole run and these six tests poll real processes.

The rest of the reed suite is not skipped, only deferred: `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) covers `internal/reedengine`'s untagged suite and the `integration`-tagged `contract_integration_test.go` before the task is marked done, which matters here because the reap change alters which tmux calls are issued and in what order.
The remaining smoke files are outside every automated gate in this hub and are exercised by the sandbox pass card 11 of batch 3 restates.
