# Discussion: reed: pane reap isn't applied consistently across up/add's mutating paths

```yaml
task: 'reed: pane reap isn''t applied consistently across up/add''s mutating paths'
slug: reed-pane-reap-consistency
status: discussing
parent: main
```

## Problem

reed's contract is that it owns every pane in its session window: a pane reed did not create is foreign, and reconcile is supposed to kill it deterministically rather than tolerate or reuse it.
Two independent sandbox findings show that guarantee does not hold on every mutating verb.

**M16 (FAIL)** — after an operator creates a pane behind reed's back (`tmux -L <socket> split-window -t <session>`), `lyx reed up` leaves it alone, and the next `lyx reed add` *adopts* it: `lyx reed status` reports the new strand's `paneId` as that same foreign pane id, and `#{pane_pid}` is unchanged across `up` and `add` — the strand's command is typed via `send-keys` into a shell process reed never created, whose environment, cwd and scrollback reed does not control.
Reproduced twice with fresh foreign panes.

**M22 (WARN)** — with a live session, deleting `.lyx/reed.json` and then running `lyx reed up` returns `{"ok":true,...,"strands":0}` but leaves THREE panes: a freshly split header (`%2`, height 12 rather than the configured `height_rows: 1`), the OLD header (`%1`) still alive and unreaped, and the OLD orphaned strand pane (`%0`) still running its original command — none of it tracked in the rewritten `reed.json`.
The very next mutating verb (`add`) self-heals it, reaping both as untracked panes; so the session converges, but only one verb late, and in between there is a duplicated header rendered at the wrong height plus a leaked live process pane.

**Why now:** both findings come from the same sandbox pass and both trace to one gap — reap-before-use/allocate is not guaranteed on every code path through reed's reconcile logic.
M16 is a correctness failure of reed's isolation guarantee, not cosmetics: a strand's command can execute in a process reed does not own.

## Scope

**In:**

- `internal/reedengine/reconcile.go` — `planReconcile`'s untracked-pane reap authorization, and a `logger.Info` line in `reconcileLocked`'s `kill-pane` loop.
- `internal/reedengine/spawn.go` — `planPaneTarget` (drop pane adoption), `soleAliveNonHeaderPane` (delete), and `launchStrandLocked` (reap-before-allocate chokepoint).
- `internal/reedengine/doc.go` — the package-doc paragraphs describing the deterministic untracked-reap policy and the header's exclusion seams, which currently document the behaviour being changed.
- Unit tests in `internal/reedengine/reconcile_test.go` and `internal/reedengine/spawn_test.go`.
- Real-tmux smoke regressions in `internal/reedcli/smoke_lifecycle_test.go` for both M16 and M22.

**Out:**

- The header pane's own lifecycle (`ensureHeaderPaneLocked`, `splitHeaderPaneAtTopLocked`) — unchanged; the header stays reap-exempt, stays healed on `up`/`resume` only, and its even-vertical split retry is not touched.
- Layout/geometry (`apply.go`, `render`, `windowsize.go`) — the observed height-12 header in M22 is a *consequence* of the unreaped duplicates plus `applyLayoutLockedOpts`'s deliberate skip when no strand owns a present pane, not a separate layout defect.
  No change to `planLayout`, `anyPlacedStrand`, or the box resolution.
- Process-subtree reaping (`descendantClosurePIDs` / `reapPaneChildren`) — stays confined to `RemoveStrand` and `Down`.
- The read-only verbs: `Status`, `CapturePane`, `SendText`/`SendKey`, and the unattended watcher (`Watch` → `reapply.go`) never reconcile today and must not start.
- `RemoveStrand`, `Down`, generation/foreign-session refusal (`generation.go`), `clearConflictingPaneBindings` — untouched.
- `manifest/roadmap.md` — this is a bugfix, not a planned-item completion.

## Decisions

### reap-gate-accepts-the-header-as-survival-anchor

- Decision: in `planReconcile`, change the untracked-reap authorization from `anyBoundPresent` alone to `anyBoundPresent || headerPresent`, where `headerPresent` means `headerPaneID != ""` and that id is **present** in `live` (presence, not aliveness).
  `anyBoundPresent` itself keeps being computed from real strand bindings alone, exactly as its current comment demands; the new disjunct is added beside it, not folded into it.
- Rationale: the reap is gated at all only to guarantee the session survives it — killing every pane would end the session.
  The header pane is already unconditionally exempt from the reap (`exemptPaneIDs`) and is a permanent, first-class per-session construct, so whenever it is present it is exactly the survival anchor the gate is looking for.
  With zero strands bound the current gate is false, which is precisely M22's and M16's shared precondition.
  Presence rather than aliveness is the right test because `remain-on-exit` keeps a dead header enumerable, and an enumerable pane keeps the session alive; a header corpse is separately healed by `ensureHeaderPaneLocked` on the next `up`/`resume`.
- Rejected: reaping unconditionally whenever ≥2 panes are present (loses the explicit survival reasoning the current code is built around and would reap with no anchor at all when the header is missing);
  adding an ad-hoc reap call inside `Up` only (leaves the same hole on every other verb, which is the defect being fixed).

### drop-pane-adoption-entirely

- Decision: delete adoption from `planPaneTarget` — it always returns a split target and never an `adoptID`.
  Delete `soleAliveNonHeaderPane`.
  Simplify `launchStrandLocked` accordingly (it always splits).
  Whether the two-return-value shape is collapsed to a single `splitTargetID` return or kept for call-site stability is mill-plan's call.
- Rationale: adoption exists for one case — the idle pane `new-session` leaves behind on a fresh boot — but it identifies that pane by a heuristic (*"the sole alive non-header pane"*) that cannot distinguish reed's own initial pane from a foreign one.
  That heuristic has now produced two live findings: R4-F5 (adopting the previous header pane after a `reed.json` scrub, typing the strand's command into `lyx reed header --blocking` where it never ran) and M16 (adopting an operator's `split-window`).
  M16's exact precondition is a session whose only non-header pane is the foreign one — reachable whenever the initial pane was already consumed or removed — at which point the heuristic fires with certainty.
  Once the reap gate above is fixed, the initial pane is disposed of by the reap like any other untracked pane, so adoption no longer buys anything: a fresh split is idle by construction and reed-owned end to end.
- Rejected: persisting an `InitialPaneID` in `ReedState` and adopting only that exact id — correct, but it adds a persisted state field, a new clear-on-adopt/clear-on-rebirth lifecycle, and a fourth pane-identity concept, all to save one `kill-pane` + one `split-window` on the first `add` of a fresh session.
  Narrowing the heuristic further (e.g. "sole non-header pane AND the table has never held a strand") — still a guess, still silent when wrong.

### reap-before-allocate-is-a-chokepoint-in-launchStrandLocked

- Decision: `launchStrandLocked` reconciles before it plans a pane target.
  Concretely: it already calls `listPanes` first;
  it then calls `e.reconcileLocked(st, live)`, re-enumerates when that killed anything, and only then calls `planPaneTarget` against the post-reap pane set.
- Rationale: `AddStrand` and `UpdateStrand` today launch the pane inside `addStrandLocked`/`updateStrandLocked` and reconcile only afterwards, in the `reconcileApplyPersistLocked` tail — so on those paths a foreign pane is guaranteed to still be there when the target is chosen.
  `Resume` happens to reconcile before its launches, which is why the bug is invisible there.
  Putting the reap inside the one shared realization helper makes "reap before allocate" true by construction on every path, present and future, which is exactly the property the task brief names as missing.
  It is safe in every caller: the new strand is appended with `PaneID == ""` before `launchStrandLocked` runs, so reconcile never clears or kills anything belonging to the strand being launched, and during `Resume`'s per-strand loop the already-relaunched strands are bound and therefore exempt.
- Rejected: duplicating a reconcile call into `AddStrand` and `UpdateStrand` (two call sites to keep in sync, and a future third realization path would silently miss it);
  leaving the ordering alone and relying on the tail (this is the defect).

### untracked-reap-stays-kill-pane-only

- Decision: the untracked reap keeps issuing `kill-pane` only.
  It does not snapshot `#{pane_pid}` subtrees and does not wait for descendant exits.
- Rationale: reconcile's existing dead-pane kills already do not reap subtrees, so this keeps one consistent rule for the whole function.
  `descendantClosurePIDs` + `reapPaneChildren(…, reapExitTimeout)` is a synchronous, saturation-tolerant wait, and reconcile runs on every mutating verb and once per strand inside `Resume`'s launch loop — putting that wait there would add seconds to routine `add`s.
  Subtree reaping stays where destruction is deliberate and the operator is already waiting for it: `RemoveStrand` and `Down`.
- Rejected: reaping subtrees in reconcile (hot-path cost, for a case where `kill-pane`'s SIGHUP already terminates the ordinary pane process).

### the-reap-logs-what-it-destroys

- Decision: `reconcileLocked` emits a `logger.Info` line when it kills panes, carrying the existing `"socket"`/`"session"` key shape plus the killed pane ids.
  One line per `reconcileLocked` call that killed anything, not one per pane — the killed set is the interesting unit and a per-pane line would be noise during `Resume`'s launch loop.
  Whether the line distinguishes dead-pane kills from untracked-pane kills (e.g. two id lists, or a `reason` key) is mill-plan's call, but the untracked reap must be identifiable in the log, since that is the destructive half.
- Rationale: `reconcile.go` has no `logger` call today at all — the only nearby one is `spawn.go`'s `clearConflictingPaneBindings` warning.
  That was tolerable while the untracked reap essentially never fired without a bound strand;
  this task deliberately makes it fire on exactly the zero-strand precondition M16 and M22 share, so the reap goes from near-dormant to routine, and it destroys panes an operator may have created.
  A pane vanishing with no trace in the log is the worst possible shape for the class of bug this task exists to fix — M22 was only diagnosable because the operator watched it happen live.
  `Info` rather than `Debug` per the Live-Substrate Spawn Observability invariant's lifecycle-vs-probe split: this is a lifecycle teardown, not a polling probe.
- Rejected: declaring `kill-pane` out of the Spawn Observability invariant's scope on the grounds that it destroys rather than starts a process — defensible as a reading of the invariant, but it answers the letter of the rule while ignoring why the observability matters here;
  a silent reap is precisely what made these two findings expensive to characterise.
  Also rejected: a per-pane log line (noisy inside `Resume`'s per-strand loop) and a `Warn` level (the reap is designed behaviour, not an anomaly).

### zero-strand-sessions-end-up-header-only

- Decision: accept that after the fix, `up` against a session with zero tracked strands leaves exactly one pane — the header — occupying the full window, because `applyLayoutLockedOpts` deliberately skips `select-layout` when no strand owns a present pane.
  Do not synthesise or preserve an idle non-header pane to keep the header at `height_rows`.
- Rationale: this is the honest end state of "reed owns the window" with nothing to show;
  the header is full-height only because it is the sole pane, and it snaps back to `height_rows` the moment a strand pane exists.
  It also resolves M22 in the strongest form available: the scrubbed-state `up` converges immediately (one pane, no zombie header, no leaked process) instead of one verb late.
  Splitting a sole full-height header is always possible, so the next `add` is unaffected — `planPaneTarget`'s existing "no non-header pane exists at all: split the header itself" fallback covers it, and `validateSplitCreatedNewPane` still guards the result.
- Rejected: keeping one idle pane alive as a layout spacer (invents a pane with no owner, which is the class of thing this task removes).

### no-new-CONSTRAINTS-entry

- Decision: document the tightened rule in `internal/reedengine/doc.go` only.
  No new `CONSTRAINTS.md` invariant, no `docs/overview.md` change, no `manifest/roadmap.md` move.
- Rationale: "every pane in a reed session is either the header or a bound strand's pane" is a module-internal invariant enforced inside one package, not a cross-cutting structural form other modules must take — which is what `CONSTRAINTS.md` is for.
  `docs/overview.md`'s module table and execution stack are unchanged.
  Per CLAUDE.md, the roadmap moves only on completing or adding a planned item, and this is a bugfix.
- Rejected: adding a "Reed Pane Ownership Invariant" to `CONSTRAINTS.md` (would be the only entry there scoped to a single package's internal bookkeeping).

## Technical context

Module layout: `internal/reedengine` (engine) and `internal/reedcli` (cobra seam).
There is no `manifest/designs/reed.md` — `internal/reedengine/doc.go` (477 lines) is reed's design document and carries the multiplexer contract surface and the load-bearing behavioural assumptions.

**The reap itself — `internal/reedengine/reconcile.go`:**

- `planReconcile(strands, live, headerPaneID) (clearedGUIDs, panesToKill, keptDeadPane)` is pure and unit-testable.
  It computes `boundPaneIDs` from strand bindings, then `anyBoundPresent` (is any bound pane present in `live`), then `exemptPaneIDs` = bound panes + the header.
  The untracked reap is the `if anyBoundPresent { … }` block near the end — this is the single line to change.
  Note the existing comment on `exemptPaneIDs`: it gates only *which* panes escape the reap, while `anyBoundPresent` stays derived from real strand bindings — preserve that separation.
- The header is exempt from the dead-pane kill too, for a documented reason (a killed dead header leaves the session headerless with a stale `HeaderPaneID` until the next `up`/`resume`).
- `reconcileLocked(st, live)` composes the plan with `kill-pane` I/O and clears bindings for cleared GUIDs.

**Allocation — `internal/reedengine/spawn.go`:**

- `planPaneTarget(strands, live, headerPaneID) (adoptID, splitTargetID, err)`: `anyBound` false → try `soleAliveNonHeaderPane` → adopt.
  Otherwise split the *tallest alive* non-header pane, falling back to any present non-header pane (a corpse), falling back to `live[0]` (the header itself).
  The doc comment above it explains why adoption was already narrowed to the sole-pane case and cites R4-F5 — that comment is the one that must be rewritten, not merely deleted, since it records why the whole seam existed.
- `launchStrandLocked(st, s, launchCmd)`: `listPanes` → `planPaneTarget` → `split-window -t <target> -c e.geom.PaneCwd -P -F '#{pane_id}'` → `validateSplitCreatedNewPane` → set `s.PaneID` → `send-keys -l` (via `sendKeysLiteralArg`) → `send-keys Enter`.
  The `-c e.geom.PaneCwd` pin is load-bearing (a split issued from outside tmux otherwise inherits the *client's* cwd) — keep it.
- `reconcileApplyPersistLocked(st)` is the shared tail: `listPanes` → `reconcileLocked` → re-`listPanes` if anything was killed → `applyLayoutLocked` → `SaveState`.
  The kill → re-enumerate → compute → apply ordering is explicitly load-bearing; the new in-`launchStrandLocked` reap must follow the same shape.

**Callers and current ordering:**

- `Up` (`lifecycle.go:650`): `ensureServerAndSessionLocked` → `loadOrInitStateLocked` → (on rebirth) `clearAllPaneBindings` + clear `HeaderPaneID` → `ensureHeaderPaneLocked` → `reconcileApplyPersistLocked`.
  The header is created *before* reconcile, which is what makes the relaxed gate fix M22 — do not move the reconcile ahead of `ensureHeaderPaneLocked`, or the gate would still be false at that point.
- `Resume` (`lifecycle.go:702`): reconciles before its launch loop and again per launch.
- `AddStrand` (`strand.go:288`) → `addStrandLocked` appends the strand (with `PaneID == ""`) then calls `launchStrandLocked` → `SaveState` → `reconcileApplyPersistLocked`.
  **No reconcile before the launch** — M16's path.
- `UpdateStrand` (`strand.go:331`) → `updateStrandLocked` → `launchStrandLocked` on a hidden→visible transition.
  Same missing reconcile.
- `Status` (`lifecycle.go:1163`), `SendText`/`SendKey`/`CapturePane` (`io.go`), and `Watch` (`watchloop.go` → `reapply.go`, which calls `applyLayoutLockedOpts` with `SkipFocus`) never reconcile.
  That must stay true — the watcher runs unattended and must not kill operator panes on a window resize.

**Mechanisms that shape the observed symptoms:**

- `anyPlacedStrand` (`apply.go`) makes `applyLayoutLockedOpts` skip `select-layout` entirely when no strand owns a present pane — deliberately, because tmux answers a zero-cell layout string by destroying every pane (`TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable` pins this).
  It is why M22's `up` leaves the panes at tmux's own 50/50 split heights rather than at `height_rows`, and why nothing gets destroyed there.
- `applyLayoutLockedOpts` also returns early when `len(live) < 2`, which is why a sole header pane is full-height and therefore always splittable.
- `remain-on-exit` is `on` for every session (`lifecycle.go:406`), so a pane whose command exits stays enumerable as `pane_dead=1`.
  `liveIDSet` = present; `aliveIDSet` = present and not dead.
- `validateSplitCreatedNewPane` exists because psmux's `split-window` on a too-small pane exits 0, creates nothing, and prints an *existing* pane id.
  Every split site must keep calling it.
- `clearConflictingPaneBindings` (`reconcile.go`) runs on every `LoadState` and repairs tables where two owners claim one pane.
  Unrelated to this fix but shares the file — do not disturb it.

**Reproduction preconditions worth knowing when writing the smoke tests:**

- M16 fires when the *only* alive non-header pane is the foreign one.
  A session that still holds its unadopted initial `new-session` pane has two alive non-header panes, so `soleAliveNonHeaderPane` returns false and the current code splits instead — which is why `TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable` passes today despite the bug.
  A faithful M16 regression must first drive the session to a header-plus-foreign-pane-only state (e.g. `up`, `add`, `remove` the strand, then foreign-split), not merely `up` + foreign-split.
- M22 fires from `up`, `add`, `rm .lyx/reed.json`, `up`.

**Test harness:** `internal/reedcli` smoke tests are `//go:build smoke`, use `hubforge.NewHub(t, ".")` + `t.Chdir(h.PrimeWorktree())` + a `down` cleanup, and read the header pane id straight out of `reedengine.LoadState(filepath.Join(worktree, ".lyx"))`.
Helpers already present: `tmuxBinaryPath`, `socketAndSession`, `listPaneLines`, `paneLiveOnSession`, `addStrand`, `statusStrand`.
`internal/reedengine/contract_integration_test.go` is `//go:build integration`.

## Constraints

From `CONSTRAINTS.md`:

- **Told-Geometry Invariant** — `reedengine` is a bound package: it is handed absolute paths and never imports `internal/lyxcwd`.
  Nothing in this change may derive a path.
- **CLI / Cobra Invariant** — `reedcli` imports `reedengine`, never the reverse;
  errors are JSON via `internal/output`.
  This task adds no CLI verb and no flag, so the help-tree tests are unaffected.
- **Shell Mechanics Seam** — pane-shell command strings are built only via `internal/shell`.
  `launchStrandLocked` passes the caller's opaque `launchCmd` through `send-keys -l` and must keep doing so.
- **Live-Substrate Spawn Observability** — every path that starts a real OS process logs its spawn via `internal/logger`.
  A retry loop around a real spawn caps attempt *count*, not only elapsed time.
- **Test Tier Purity Invariant** — untagged test files perform no `exec.Command`/`gitexec`/`hubforge.NewHub` and no `time.Sleep` ≥ 1s.
  All real-tmux work must sit behind the `smoke` (or `integration`) build tag;
  the `planReconcile`/`planPaneTarget` unit tests stay untagged and pure.
- **Hermetic Git Test Environment Invariant** — any test package spawning git runs under `gitkit.HermeticGitEnv()` in `TestMain`.
  `internal/reedcli` and `internal/reedengine` already have `testmain_test.go` files;
  no new test package is introduced.
- **Config Strictness Invariant** — `reedengine` is a *degrading* consumer (`LoadOrTemplate`).
  Unchanged here.
- **Documentation Lifecycle** — docs land in the same commit as the code.

Discovered during discussion:

- The reap must never run from a read-only verb or from the unattended watcher.
- `logger` calls in reed carry `"socket"` and `"session"` keys;
  the reap log line committed to by the-reap-logs-what-it-destroys follows that shape.
  `reedengine` already imports `internal/logger`, so no import change and no Spawn Observability exemption note is needed.

## Testing

**Unit — `internal/reedengine/reconcile_test.go` (untagged, pure):**

`planReconcile` is already table-driven there and has cases asserting foreign panes are left alone when nothing is bound.
Those cases encode the old policy and must be updated, not merely added to.
Scenarios to cover:

- Header present, zero strands, one untracked alive pane → the untracked pane is killed, the header is not.
- Header present, zero strands, several untracked panes (M22's shape: an old header corpse-or-alive pane plus an orphaned strand pane) → all of them killed, the current header spared.
- Header **absent** (`headerPaneID == ""`) and no strand bound → nothing reaped (the gate must still refuse without an anchor).
- Header present and a strand bound → unchanged behaviour, both exempt.
- The dead-pane rules are untouched: `keptDeadPane` still spares one dead pane when nothing is alive, and a dead header is still never killed.

**Unit — the reap log line:**

`reconcileLocked` is the composing half, not the pure one, so this needs the existing fake-tmux harness `reconcile_test.go`/`lifecycle_test.go` already use plus `logger.SetOutput(&buf)` (with a `t.Cleanup` restoring it — `internal/reedengine` has no logger-capture helper today, so one may need adding beside the fake).
Assert that a reconcile which kills untracked panes emits an `Info` line naming those pane ids, and that a reconcile which kills nothing emits none.
Keep this to one focused test — the point is that the destructive path is observable, not that the log's exact wording is pinned.

**Unit — `internal/reedengine/spawn_test.go` (untagged, pure):**

- `planPaneTarget` never returns an adopt id, for every input shape the old adoption branch used to catch (sole alive non-header pane, zero strands bound).
- It still picks the *tallest alive* non-header pane, still falls back to a present non-header corpse, and still falls back to the header when no non-header pane exists.
- Delete or rewrite whichever existing cases assert adoption;
  `soleAliveNonHeaderPane`'s own tests go with the function.

**Smoke — `internal/reedcli/smoke_lifecycle_test.go` (`//go:build smoke`, real tmux):**

- **M16 regression.** Drive the session to header-plus-foreign-only (`up`, `add`, `remove`, then a raw `tmux -L <socket> split-window -t <session>`), capture the foreign pane's id and `#{pane_pid}`, then `add` a strand.
  Assert the strand's `paneId` from `lyx reed status` is not the foreign pane id, that the foreign pane id is gone from `list-panes`, and that the recorded foreign pid is no longer alive.
  The pid check is what actually distinguishes "reaped and recreated" from "adopted" — a pane-id-only assertion would have passed for the adoption bug had ids been recycled.
- **M22 regression.** `up`, `add` a strand, delete `.lyx/reed.json` while the session is live, `up`.
  Assert the session holds exactly one pane, that it is the newly persisted `HeaderPaneID`, that the old header id and old strand pane id are both gone, and that the old strand's process is gone.
  Asserting *on that `up`* — with no intervening verb — is the point of the test.
- **Non-regression.** `TestSmokeUpWithOnlyForeignPanesKeepsSessionUsable` must keep passing unchanged: `up` with zero placeable strands must still not destroy the pane set, and the post-`add` session must still hold exactly the strand pane and the header pane.
  Verify it still asserts what it means to after the reap change (it will now reap the initial pane as well as the foreign one during the `add`).

**Also run:** the rest of `internal/reedengine` (untagged), `internal/reedcli` smoke, and the `integration`-tagged `contract_integration_test.go`, since the reap change alters which tmux calls are issued and in what order.

**TDD candidates:** the `planReconcile` gate cases and the `planPaneTarget` no-adoption cases — both are pure functions with existing table harnesses, so write the failing cases first.
The smoke regressions should also be written first and confirmed red against the current binary, since both M16 and M22 are reproducible on demand.

## Q&A log

- **Q:** How should the untracked reap be authorized when no strand is bound to a present pane? **A:** [auto-pick] Gate on `anyBoundPresent || headerPresent`. **Why:** the gate exists only to guarantee session survival, and the header is already a permanent, reap-exempt pane — it is exactly the anchor the gate is looking for, and its absence from the condition is the shared precondition of both findings.
- **Q:** What happens to `planPaneTarget`'s sole-alive-non-header pane adoption? **A:** [auto-pick] Drop adoption entirely; always split. **Why:** the heuristic cannot distinguish reed's own initial pane from a foreign one and has now caused two live findings (R4-F5, M16);
  once the reap gate is fixed the initial pane is disposed of like any other untracked pane, so adoption buys nothing a fresh split does not.
- **Q:** Where does the reap-before-allocate ordering fix belong? **A:** [auto-pick] Inside `launchStrandLocked`, as a single chokepoint. **Why:** it makes the property true by construction for `AddStrand`, `UpdateStrand`, `Resume` and any future realization path, rather than requiring two call sites to stay in sync — which is exactly the "not guaranteed on every code path" gap the brief names.
- **Q:** Should the untracked reap also wait on the killed panes' descendant process subtrees? **A:** [auto-pick] No — `kill-pane` only. **Why:** reconcile's existing dead-pane kills already do not reap, and the synchronous `reapExitTimeout` wait would land in the hot path of every mutating verb and every per-strand `Resume` iteration;
  subtree reaping stays in `RemoveStrand`/`Down` where destruction is deliberate.
- **Q:** Is a header-only, full-height session an acceptable end state for `up` with zero strands? **A:** [auto-pick] Yes, accept it. **Why:** it is the honest result of "reed owns the window" with nothing to lay out, it makes M22 converge on the very `up` under test instead of one verb later, and a sole full-height header is always splittable so the next `add` is unaffected.
- **Q:** What test tiers cover this? **A:** [auto-pick] Pure unit tests for `planReconcile` and `planPaneTarget`, plus two real-tmux smoke regressions in `internal/reedcli/smoke_lifecycle_test.go`. **Why:** the decisions are in pure functions, but both findings are ordering/identity bugs that only a real tmux server reproduces — and the pid assertion is what separates "reaped" from "adopted".
- **Q:** Which docs move? **A:** [auto-pick] `internal/reedengine/doc.go` only. **Why:** reed has no `manifest/designs/` doc — `doc.go` is it, and it currently documents the adoption seam and the reap policy being changed;
  the invariant is module-internal so it does not belong in `CONSTRAINTS.md`, and a bugfix does not move `manifest/roadmap.md`.
- **Q:** Is M22's height-12 header a separate layout defect? **A:** [auto-pick] No — out of scope. **Why:** `applyLayoutLockedOpts` deliberately skips `select-layout` when no strand owns a present pane (tmux destroys every pane on a zero-cell layout string), so the wrong height is a consequence of the unreaped duplicates, and it disappears once the reap fires.
