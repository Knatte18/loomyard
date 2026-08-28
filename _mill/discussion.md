# Discussion: reed: resume/down leak lock directories at the stale pre-rename session-name path

```yaml
task: 'reed: resume/down leak lock directories at the stale pre-rename session-name path'
slug: reed-lock-stale-session-name
status: discussing
parent: main
```

## Problem

Running `lyx reed resume` or `lyx reed down` from a worktree that was renamed while its tmux session was up leaves a brand-new, empty, REAL directory at the hub root named after the pre-rename worktree: `<hub>/<old-name>/.lyx/{reed.json.lock,reed.lock}`.
It is a real directory, not the `.lyx` symlink a fabric worktree normally carries, and it did not exist before the call — its birth timestamp matches the exact moment the verb ran.
Reed's own `reed.json` state is written correctly through the live worktree; only the lock-side directory lands somewhere else.

This is a landmine, not litter.
The operator workflow the sandbox suite itself prescribes for M20/M24/M25 is "rename the worktree away, test the refusal, rename it back".
Renaming back with `mv` into a destination name that now exists as a directory silently NESTS the real repository inside the stray instead of renaming cleanly.
That happened live during the M24/M25 suite run and required manual recovery (move the nested repo back out, delete the stray lock files, rename again).
Why now: the resize watchdog daemon (commit `8002cf976`, the newest reed commit) introduced the first long-lived in-session process that runs engine ops on a timer, which is what turned a latent geometry-lifetime assumption into an observable filesystem leak.

## Scope

**In:**

- A new liveness check on the told `Geometry.AnchorPath` at reed's operation-lock chokepoint (`internal/reedengine/lock.go`: `withOpLock` and `withTryOpLock`), refusing the op when the anchor directory no longer exists — BEFORE the `os.MkdirAll` that creates the stray.
- The refusal predicate itself, as a new I/O-touching validator alongside the existing pure `validateToldAnchorPath` in `internal/reedengine/server.go`.
- Terminating the resize watch loop (`internal/reedengine/watchloop.go`) when its anchor is proven gone, instead of retrying and log-spamming forever against a worktree that no longer exists.
- Test-fixture update: `newTestEngine` (`internal/reedengine/lock_test.go`) must materialize its `AnchorPath` on disk, since every op-running test in the package now depends on it.
- Unit coverage for the refusal at both lock helpers, for "no directory created", and for the watch loop's termination.
- Sandbox suite text (`tools/sandbox/SANDBOX-REED-SUITE.md`, M24 and M25): assert that no new hub-level directory appears after the escape routes run.
- Package-doc note in `internal/reedengine/doc.go` recording the geometry-lifetime rule this fix establishes.

**Out:**

- Any cleanup or sweeping of already-leaked stray directories. Reed never deletes hub-level directories; the operator removes the one-off stray by hand.
- `internal/state.ReadJSON`'s `MkdirAll` on a pure read path (`internal/state/state.go:62`). It is a genuine co-defect — a read materializing `<dir>/reed.json.lock` is what produced the second of the two stray files — but it is a different module with four other consumers (webster, treadle, shed, loomshed). Recorded as a follow-up below; not fixed here.
- Re-deriving geometry inside the engine. Banned by the Told-Geometry Invariant.
- The `window-resized` hook's baked absolute signal path going stale after a rename. With this fix the stale `.lyx` is never recreated, so the hook's `touch` simply fails and produces nothing; no separate change needed.
- Any change to the existing renamed-worktree refusal machinery (`generation.go`, `refuseLiveForeignSessionLocked`), which already behaves as M24/M25 specify.

## Decisions

### root-cause-is-a-frozen-geometry-in-a-long-lived-process

- Decision: The stray is created by the OLD session's still-running header-pane process (`lyx reed header --blocking`, whose keepalive tail parks in `Engine.Watch`), not by the `resume`/`down` process the operator invoked.
  `resume`/`down` are correlated, not causal — no CLI verb leaks on its own, because `internal/reedcli/cli.go`'s `PersistentPreRunE` resolves a fresh, correct `AnchorPath` on every invocation.
- Rationale: the header process resolves its geometry once, at launch, in that same `PersistentPreRunE`, and pins it into the `Engine` for the process's whole life.
  After `mv`, the process's own cwd follows the inode but the frozen `geom.AnchorPath` string does not.
  The exact chain, per tick: `watchLoop` → `reapplyLayout` (`internal/reedengine/reapply.go:90`) → `withTryOpLock` → `os.MkdirAll(e.stateDir())` creates `<hub>/<old-name>/` AND `.lyx` under it (`MkdirAll` creates every missing parent) plus `reed.lock`; then `requireSessionLocked` (`lifecycle.go:1134`) finds no session under the old name and its error path calls `LoadState(e.stateDir())` at `lifecycle.go:1148`, whose `internal/state.ReadJSON` creates `reed.json.lock`; the error returns before `SaveState` is ever reached.
  That is byte-for-byte the reported shape: a REAL directory (not a symlink), exactly two 0-byte lock files, and no `reed.json` beside them.
  It also explains the rest: read-only `status` produces no stray (it refuses before reaching a lock helper), and the stray's name varies between "the pre-rename name" and "a name from an earlier rename step" because it is whichever name the long-lived process happened to boot under.
  Why now: the watchdog daemon (`8002cf976`) is the newest reed commit and the first long-lived in-session process that runs engine ops on a timer.
- Rejected: a stale recorded session name read out of `reed.json` — no code path joins a recorded session name or `Strand.Worktree` onto a filesystem path; `state.Worktree`/`Strand.Worktree` are stamped (`strand.go:177`) and declared (`state.go:25`) but never read outside tests, and `Down`'s abandonment report reads `st.PaneGeneration.SessionName` purely as message text (`lifecycle.go:900-904`).
  Rejected: `lyxcwd` returning a stale worktree root — verified empirically that `git rev-parse --show-toplevel` returns the NEW path after `mv`, with or without `git worktree repair`; and `readRecordedAnchor` yields a worktree-relative `AnchorRel`, never a sibling worktree name.
  Rejected: the tmux resize hook creating it — `posixShell.Touch` is `: > 'path'` (`internal/shell/posix.go:36`), a redirection with no `mkdir`, and the hook is never installed at all (see the follow-up below).
  Rejected: `fabricengine` junction wiring (`junction.go:149`) — that `MkdirAll`s `_lyx` (`lyxdirs.LyxDirName`), not `.lyx` (`DotLyxDirName`).

### the-leak-is-continuous-not-per-event

- Decision: Treat the leak as a 2-second poll loop that recreates the stray indefinitely, not as a one-shot artifact of the verb that ran.
- Rationale: `watchLoop` starts in `watchModePoll` (`watchloop.go:180`) and poll mode calls `reapplyLayout` every `watchdogPollCycle` = 2s with no gating.
  Promotion to `watchModeSignal` requires `hookInstalledLocked()` to see the `window-resized` hook — which has no install site anywhere in the repo — so every watcher is pinned in poll mode permanently.
  This matters for two reasons.
  First, under M25 `down` is documented to leave the abandoned session running (`lifecycle.go:806-813`), so its stale-anchored watcher keeps recreating the stray every 2s for as long as that pane lives: deleting the lock files by hand does not stick until the session dies, which matches the manual recovery the suite run needed.
  Second, it means the watchdog-termination decision below is load-bearing, not cosmetic — without it, refusing at the lock chokepoint converts a filesystem leak into a `Warn` every 2 seconds forever.
- Rejected: modelling this as "resize events fire the watcher", which would have made the leak bounded and the termination optional.
- Rejected: a stale recorded session name read out of `reed.json` — no code path joins a recorded session name or `Strand.Worktree` onto a filesystem path; `hubgeom.ReedGeometry` builds `AnchorPath` only from `lyxcwd.Location`.
  Rejected: `lyxcwd` returning a stale worktree root — verified empirically that `git rev-parse --show-toplevel` returns the NEW path after `mv`, with or without `git worktree repair`.

### refuse-at-the-op-lock-chokepoint

- Decision: Refuse the operation when the told `AnchorPath` does not exist as a directory, checked in `withOpLock` and `withTryOpLock` immediately after the existing `validateToldAnchorPath` call and before the `os.MkdirAll(dotLyx)`.
- Rationale: those two helpers are the single chokepoint every public engine op passes, and the existing refusal ordering (identity → anchor shape → MkdirAll → lock) already exists precisely so a bad geometry cannot create substrate.
  This slots one more predicate into an established sequence and fixes every verb — `up`, `add`, `remove`, `status`, `attach`, `resume`, `down`, and the watchdog's `reapplyLayout` — with one change.
  It does not derive anything, so the Told-Geometry Invariant is untouched: reed still only refuses a geometry it was told.
- Rejected: re-resolving geometry per op (violates the Told-Geometry Invariant outright — `reedengine` may not import `lyxcwd`).
  Rejected: having the watchdog alone re-derive its anchor (leaves every other verb able to leak, and puts cwd resolution inside an engine).
  Rejected: making `stateDir()` refuse (it is a pure path join used by non-mutating callers such as `resizeSignalPath`; a refusal there has nowhere to go).

### the-anchor-must-pre-exist-but-dot-lyx-need-not

- Decision: The check is `AnchorPath` exists and is a directory. The `os.MkdirAll` that creates `.lyx` under it stays.
- Rationale: a brand-new worktree's first reed op legitimately has no `.lyx` yet — that is exactly why the `MkdirAll` is there.
  What reed must never do is conjure the *worktree/anchor directory itself*, which is the act that produced the landmine.
  Checking the anchor rather than `.lyx` keeps first-op creation working while making the stray impossible.
- Rejected: requiring `.lyx` to pre-exist (breaks every first run).
  Rejected: checking `WorktreeRoot` instead (`AnchorPath` is what `stateDir` joins onto and can be a committed subpath under a non-`.` `AnchorRel`; checking the wrong one leaves a gap).

### a-separate-live-validator-keeps-the-pure-one-pure

- Decision: Add a new function (e.g. `validateAnchorPathLive(geom Geometry) error`) beside `validateToldAnchorPath` in `internal/reedengine/server.go`, rather than adding a `os.Stat` inside `validateToldAnchorPath`.
- Rationale: `validateToldAnchorPath` is documented and tested as a pure shape validator over the told contract (empty / relative), driven by a table test in `server_test.go`.
  Mixing filesystem I/O into it would make that table test I/O-dependent and blur "the caller told me something unusable" against "the world changed under a valid value" — two different diagnoses that deserve two different error messages.
- Rejected: extending `validateToldAnchorPath` in place.

### the-error-names-the-vanished-path-and-the-remedy

- Decision: The refusal error states that the anchor path no longer exists, quotes the path, and tells the operator that this worktree was renamed or removed and that reed must be run from the worktree's current path.
- Rationale: this error is reachable in two very different situations — an operator standing in a deleted worktree, and an abandoned session's own header pane logging it — so it has to be self-describing without any surrounding context.
  It matches the house style already set by `validateToldTmuxIdentity` and `validateToldAnchorPath`, both of which name the offending value and the corrective action.
- Rejected: a bare "anchor path not found".

### the-watch-loop-terminates-when-its-anchor-is-gone

- Decision: When `reapplyLayout` returns the anchor-gone refusal, `watchLoop` stops looping: log one `Warn` naming the vanished anchor and the session, then park on `ctx.Done()` exactly as the `watchdog: off` branch already does, returning `ctx.Err()`.
  The header pane process itself is NOT killed.
- Rationale: the condition is permanent, not transient — the anchor string is frozen for this process's lifetime, so every subsequent tick would refuse identically.
  Current `handleWatchOutcome` treats every error as retryable, and since every watcher is pinned in poll mode (see above) that is a `Warn` every 2s forever, indefinitely, on every abandoned session `down` leaves behind.
  Parking rather than returning early preserves the loop's existing contract that ctx-liveness is the caller's only "watcher is done" signal, and reuses a branch that already exists.
  Not killing the pane matters: the abandoned session may still be hosting the operator's live strand processes (M25 explicitly requires `down` to leave them running), and tearing down its header pane would disturb a window reed has deliberately walked away from.
- Rejected: retry forever (log spam, and it is the behavior that keeps the leak's engine running).
  Rejected: exiting the header process (destroys a window belonging to work reed just promised not to touch).

### no-cleanup-of-existing-strays

- Decision: Ship prevention only. No sweeper, no new verb, no delete-on-`down`.
- Rationale: reed deleting hub-level directories is exactly the class of action the Fabric Destruction Chokepoint Invariant exists to keep out of ad-hoc code paths, and a stray is indistinguishable from a legitimately empty sibling worktree directory the operator created.
  One manual `rmdir` clears the historical case; the fix makes new ones impossible.
- Rejected: sweeping in `down` (destructive, hub-level, and racy against a sibling worktree mid-creation).

### state-readjson-mkdirall-is-recorded-not-fixed

- Decision: Leave `internal/state/state.go`'s `ReadJSON` `MkdirAll` alone in this task; record it here as a follow-up worth its own wiki task.
- Rationale: a pure read materializing a directory and a lock file is wrong on its face, and it is the mechanism behind the `reed.json.lock` half of the stray.
  But once reed refuses at the lock chokepoint, no reed path reaches `ReadJSON` with a bogus directory, so it is defense-in-depth rather than part of this fix.
  Changing it alters the miss-semantics for four other engines (webster, treadle, shed, loomshed) and belongs in a change scoped to `internal/state`.
- Follow-up shape, for whoever picks it up: `ReadJSON` should treat a missing parent directory the same way it already treats a missing file — return `(zero, false, nil)` — instead of creating it.
- Rejected: fixing it here (widens the task into another module for no additional coverage of the reported defect).

### window-resized-hook-has-no-install-site

- Decision: Record, do not fix. Out of scope for this task; worth its own wiki task.
- Rationale: exploration turned up that `windowResizedHookName` (`internal/reedengine/watchdog.go:59`) appears in exactly three places — the const, the `show-options` read-back in `reapply.go:59`, and a `set-hook -u` UNSET in `windowsize.go:138`.
  There is no `set-hook` install site anywhere in the repo.
  So `watchModeSignal`, `watchdogSignalTick`, and the whole debounce/retry state machine are dead in production — every watcher runs poll-only, forever — while `contracts/.../template_posix.yaml:10` advertises that it "enables ... the session's window-resized hook".
  This may be deliberate batch staging, but the config text and the shipped behaviour disagree, and it is what makes the leak continuous rather than event-driven.
- Rejected: installing the hook as part of this task.
  It is a feature-completion change to the watchdog with its own live-tmux verification burden, entirely separable from the stray-directory defect, and bundling it would make this fix impossible to review or revert on its own.
  The fix here is correct whether the watcher polls or waits on a signal.

## Technical context

- `internal/reedengine/lock.go` — `withOpLock` (blocking, every public op) and `withTryOpLock` (non-blocking, used by `reapplyLayout`).
  Both run `validateToldTmuxIdentity` → `validateToldAnchorPath` → `os.MkdirAll(e.stateDir())` → acquire `reed.lock`.
  The new check goes between the second validator and the `MkdirAll`, in BOTH helpers — fixing only `withOpLock` would leave the watchdog, the actual leak source, untouched.
- `internal/reedengine/lifecycle.go:32` — `stateDir()` is `filepath.Join(e.geom.AnchorPath, lyxdirs.DotLyxDirName)`. Pure; leave it alone.
- `internal/reedengine/server.go` — home of `validateToldTmuxIdentity` and `validateToldAnchorPath`, with long doc comments explaining why each refusal is a contract backstop.
  The new validator belongs beside them and should follow the same commenting depth.
- `internal/reedengine/geometry.go` — `Geometry`'s documented contract: `New` validates no field, and no method recomputes one. Preserve this.
- `internal/hubgeom/hubgeom.go:34` — `ReedGeometry(l)` sets `AnchorPath = l.AnchorPath()`. Not a defect site; listed so the plan does not go looking for one.
- `internal/reedengine/watchloop.go:159` — `watchLoop`. The `watchdog: off` branch (`<-ctx.Done(); return ctx.Err()`) is the parking pattern to reuse.
  `handleWatchOutcome` (same file) is where an error is currently classified as retryable; the anchor-gone case has to be distinguishable there, which argues for a package-level sentinel error (`errors.Is`-matchable) rather than a string match.
- `internal/reedengine/reapply.go:90` — `reapplyLayout` calls `withTryOpLock` and returns `(ReapplyResult, error)`; the refusal surfaces through that `error`.
- `internal/reedengine/lifecycle.go:1134,1148` — `requireSessionLocked` is what calls `LoadState` on the failure path, creating the second lock file. Not a change site: once `withTryOpLock` refuses, it is never reached.
- `internal/reedcli/cli.go:63-98` — `PersistentPreRunE`, where geometry is resolved once per process and pinned into the `Engine`. This is correct for one-shot verbs and is the origin of the frozen anchor for the header pane. Not a change site under the chosen fix, but the plan should not "fix" it by re-resolving per op.
- `internal/reedengine/lifecycle.go:501-515,627` — where the header pane is split with `-c e.geom.PaneCwd` running `headerLaunchLine`; the launched process re-resolves its own geometry from that cwd.
- `internal/reedengine/lock_test.go:29` — `newTestEngine` builds `AnchorPath = <tmp>/worktree/anchor` and never creates it on disk. It must now `os.MkdirAll` that path (and keep `PaneCwd` deliberately distinct and uncreated, which is the point of that fixture's comment).
  14 test files call it; a single helper change covers all of them.
- `internal/reedengine/server_test.go:296` — `TestWithOpLock_RefusesAnUnusableAnchorPathBeforeCreatingState` is the existing template for "refused before any directory was created". The new tests should mirror its shape, including asserting the lock file was never created.
- `internal/reedengine/doc.go` — the package doc carries a long "verified live" section on tmux/watchdog behavior; the geometry-lifetime rule belongs there.
- Empirically verified during exploration: `git rev-parse --show-toplevel` returns the NEW path after `mv`, both before and after `git worktree repair`, so `lyxcwd` is not implicated and needs no change.

## Constraints

From `CONSTRAINTS.md`:

- **Told-Geometry Invariant** — `internal/reedengine` must not import `internal/lyxcwd` and must derive none of its own paths. The fix only refuses a told value; it derives nothing.
- **Cwd Resolution Invariant** — no `os.Getwd` or `git rev-parse --show-toplevel` outside `internal/lyxcwd`. The new check is a `Stat` on a told path, not a cwd query.
- **Lyxdirs Single-Declarer Invariant** — `.lyx` is named only via `lyxdirs.DotLyxDirName`; keep using `stateDir()`.
- **Durable-vs-Ephemeral State Invariant** — ephemeral state stays under `.lyx` beside `AnchorPath()`; unchanged by this fix.
- **Hub Containment Invariant** — nothing hub-level is created or junctioned from a worktree. The stray directly violated the spirit of this; the fix restores it.
- **Fabric Destruction Chokepoint Invariant** — the reason no stray-cleanup sweeper is added.
- **Sandbox Suite Coverage** — M24 and M25 already exercise these two escape routes; their "Watch" text gains the no-stray-directory assertion.
- **CLI / Cobra Invariant** — no CLI surface changes in this task; `Short` strings and the help tree are untouched.
- **Documentation Lifecycle** — `internal/reedengine/doc.go` and the sandbox suite doc update in the same commit. `docs/overview.md`'s module table does not change (no new module, no changed CLI verb set), and `manifest/roadmap.md` does not move — this is a bugfix.

## Testing

Unit, `internal/reedengine` (TDD candidates — write these first, they fail against `main`):

- `withOpLock` refuses when `AnchorPath` names a directory that does not exist: the op body never runs, the error names the vanished path, and neither the anchor directory, nor `.lyx`, nor `reed.lock` is created afterwards.
  Mirror `TestWithOpLock_RefusesAnUnusableAnchorPathBeforeCreatingState`'s shape.
- The same for `withTryOpLock` — separately, not as a shared subtest, since the leak lived on this branch and a regression that fixes only one helper must fail.
- `AnchorPath` exists but is a regular file, not a directory: also refused.
- `AnchorPath` exists as a directory with no `.lyx`: the op SUCCEEDS and creates `.lyx` — the first-run path must not regress.
- The anchor-gone error is matchable by the watch loop (whatever sentinel the plan chooses), asserted directly rather than by substring.
- `validateAnchorPathLive` (or whatever it is named) gets its own focused test; the existing `validateToldAnchorPath` table test must remain I/O-free and unchanged.

Unit, watch loop:

- `watchLoop` in poll mode, given an anchor-gone refusal from `reapplyLayout`, stops issuing further reconcile attempts and does not return until ctx is cancelled. Assert a call-count that stays flat after the first refusal — `watchloop_test.go` already has hook-call-count fixtures to model on.
- The same in signal mode, so the retry-streak machinery cannot swallow the termination.
- A transient (non-anchor) error still retries exactly as today — the regression guard on the narrowing.

Fixture:

- `newTestEngine` creates its `AnchorPath`. Run the whole `internal/reedengine` package afterwards; any test that was silently relying on the anchor being absent is a real finding, not something to paper over.

Sandbox (manual, live tmux — the suite is an agent-driven black box, so this is doc text, not code):

- M24: after `kill-session` + `resume` in the renamed worktree, the hub root contains no directory named after the pre-rename worktree.
- M25: after `down` in the renamed worktree, the same assertion — and, because `down` deliberately leaves the abandoned session running, re-check the hub root after waiting well past the 2s poll cycle, so a watcher that resumed leaking is caught rather than masked by checking too early.
- Both should also confirm the abandoned session's header pane stopped logging reconcile failures rather than spinning.

## Q&A log

- **Q:** What actually creates the stray — the invoked verb, or something else? **A:** [auto-pick] The old session's still-running header-pane watchdog, whose `Engine` geometry is frozen at the pre-rename `AnchorPath`. **Why:** it is the only mechanism that accounts for all of the observed evidence simultaneously — a real (not symlinked) directory, both lock files with no `reed.json`, birth timestamps matching geometry-changing verbs only, no stray from read-only `status`, a name that varies with which rename step the process booted under, and the fact that this surfaced immediately after the watchdog daemon landed.
- **Q:** Where does the fix live? **A:** [auto-pick] Refuse at the op-lock chokepoint (`withOpLock` + `withTryOpLock`) when the told `AnchorPath` no longer exists. **Why:** one chokepoint covers every verb including the watchdog, it slots into an existing refusal sequence built for exactly this purpose, and it refuses a told value rather than deriving one, so the Told-Geometry Invariant holds.
- **Q:** Check the anchor's existence, or also pin its identity (dev+inode) so a REUSED name is caught too? **A:** [auto-pick] Existence only. **Why:** the reported defect and the landmine both come from the anchor vanishing; pinning inode identity would add per-op state to `Engine`, which `New` is contractually forbidden from populating, for a case (a new worktree created under the abandoned name while the old header pane still runs) that the recorded-session generation check already degrades safely on. Recorded here so a later reviewer sees it was considered, not missed.
- **Q:** Extend `validateToldAnchorPath`, or add a separate validator? **A:** [auto-pick] A separate, I/O-touching validator beside it. **Why:** the existing one is a documented pure shape check with a table test; mixing a `Stat` in would make that test I/O-dependent and merge two diagnoses that deserve distinct messages.
- **Q:** What should the watchdog do once it learns its anchor is gone? **A:** [auto-pick] Log one `Warn` and park on `ctx.Done()`, without killing the header pane. **Why:** the condition is permanent, so retrying is pure log spam; and the abandoned session may still host the operator's live strands, which M25 requires `down` to leave running.
- **Q:** Should reed clean up strays that already exist? **A:** [auto-pick] No. **Why:** hub-level deletion is chokepointed by design, and an empty hub directory is indistinguishable from a legitimate one; a single manual `rmdir` clears the historical case.
- **Q:** Is `internal/state.ReadJSON`'s `MkdirAll` on a read path in scope? **A:** [auto-pick] Out of scope, recorded as a follow-up. **Why:** it is a genuine defect in a different module with four other consumers, and once reed refuses at the chokepoint no reed path reaches it with a bogus directory — so it is defense-in-depth, not part of this fix.
- **Q:** Do the other verbs (`up`, `add`, `remove`, `status`, `attach`) need their own handling? **A:** [auto-pick] No — they inherit the fix. **Why:** all of them pass through `withOpLock`, which is the single chokepoint the check is added to. Strictly, none of them leaks on its own either: every CLI invocation resolves a fresh, correct anchor. The leak belongs to whichever `up`/`resume` created the header pane that is still running.
- **Q:** The `window-resized` hook has no install site, so signal mode is dead and every watcher polls every 2s — fix that here too? **A:** [auto-pick] No, record it as a follow-up. **Why:** it is a separable watchdog feature-completion change with its own live-tmux verification burden; bundling it would make this fix unreviewable and unrevertable on its own, and the anchor refusal is correct under either watch mode.
