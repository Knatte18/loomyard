# Discussion: reed: resume/down leak lock directories at the stale pre-rename session-name path

```yaml
task: 'reed: resume/down leak lock directories at the stale pre-rename session-name path'
slug: reed-lock-stale-session-name
status: discussing
parent: main
```

## Problem

Running `lyx reed resume` or `lyx reed down` from a worktree that was renamed while its tmux session was up leaves a brand-new, empty, REAL directory at the hub root named after the pre-rename worktree: `<hub>/<old-name>/.lyx/{reed.json.lock,reed.lock}`.
It is a real directory, not the `.lyx` symlink a fabric worktree normally carries, and it did not exist before the rename.
The sandbox report records its birth timestamp as matching the moment the verb ran; the mechanism below says it is created at the first watchdog poll tick after the rename, which at human resolution is the same few-second window but is NOT the same claim (see the note in `### the-leak-is-continuous-not-per-event`).
Reed's own `reed.json` state is written correctly through the live worktree; only the lock-side directory lands somewhere else.

This is a landmine, not litter.
The operator workflow the sandbox suite itself prescribes for M20/M24/M25 is "rename the worktree away, test the refusal, rename it back".
Renaming back with `mv` into a destination name that now exists as a directory silently NESTS the real repository inside the stray instead of renaming cleanly.
That happened live during the M24/M25 suite run and required manual recovery (move the nested repo back out, delete the stray lock files, rename again).
Why now: the resize watchdog daemon (commit `8002cf976`, the newest reed commit) introduced the first long-lived in-session process that runs engine ops on a timer, which is what turned a latent geometry-lifetime assumption into an observable filesystem leak.

## Scope

**In:**

- A new liveness check on the told `Geometry.WorktreeRoot` at reed's operation-lock chokepoint (`internal/reedengine/lock.go`: `withOpLock` and `withTryOpLock`), refusing the op when that directory no longer exists — BEFORE the `os.MkdirAll` that creates the stray.
- The refusal predicate itself, as a new validator alongside the existing pure `validateToldAnchorPath` in `internal/reedengine/server.go`: it checks `WorktreeRoot`'s shape (non-empty, absolute) and then its liveness, with a package-level sentinel error carried ONLY by the terminal (proven-gone) cases.
- Terminating the resize watch loop (`internal/reedengine/watchloop.go`) when its worktree root is proven gone, instead of retrying and log-spamming forever against a worktree that no longer exists.
- Test-fixture update across **every in-package test that reaches `withOpLock`/`withTryOpLock`** — whether it goes through `newTestEngine` or builds its own inline `Geometry` literal. Both kinds exist and the helper change alone does not cover the inline ones.
- Unit coverage for the refusal at both lock helpers, for "no directory created", and for the watch loop's termination.
- Sandbox suite text (`tools/sandbox/SANDBOX-REED-SUITE.md`, M24 and M25): assert that no new hub-level directory appears after the escape routes run.
- Package-doc note in `internal/reedengine/doc.go` recording the geometry-lifetime rule this fix establishes.
- An intended, user-visible behaviour change in standalone mode: `--target-dir <path-that-does-not-exist>` now refuses at the first engine op instead of proceeding and deriving a `stateDir` for it. Documented in the package doc alongside the rule above.

**Out:**

- Any cleanup or sweeping of already-leaked stray directories. Reed never deletes hub-level directories; the operator removes the one-off stray by hand.
- `internal/state.ReadJSON`'s `MkdirAll` on a pure read path (`internal/state/state.go:62`). It is a genuine co-defect — a read materializing `<dir>/reed.json.lock` is what produced the second of the two stray files — but it is a different module with four other consumers (webster, treadle, shed, loomshed). Recorded as a follow-up below; not fixed here.
- Re-deriving geometry inside the engine. Banned by the Told-Geometry Invariant.
- Any change to either `Geometry` teller (`internal/hubgeom/hubgeom.go`, `internal/standalonegeom/reedgeom.go`) or to the standalone wiring (`internal/burlercli/wiring.go:157`, `internal/webstercli/wiring.go:153`). The chosen predicate works against both tellers as they already are.
- Any new `Geometry` field. The struct's eight fields are unchanged.
- The `window-resized` signal hook's baked absolute path going stale after a rename. It is never installed today, and even if it were, with this fix the stale `.lyx` is never recreated so its `touch` (a bare `: > 'path'` redirection) would fail and produce nothing. No separate change needed.
- Any change to the existing renamed-worktree refusal machinery (`generation.go`, `refuseLiveForeignSessionLocked`), which already behaves as M24/M25 specify.

## Decisions

### root-cause-is-a-frozen-geometry-in-a-long-lived-process

- Decision: The stray is created by the OLD session's still-running header-pane process (`lyx reed header --blocking`, whose keepalive tail parks in `Engine.Watch`), not by the `resume`/`down` process the operator invoked.
  `resume`/`down` are correlated, not causal — no CLI verb leaks on its own, because `internal/reedcli/cli.go`'s `PersistentPreRunE` resolves a fresh, correct `AnchorPath` on every invocation.
- Rationale: the header process resolves its geometry once, at launch, in that same `PersistentPreRunE`, and pins it into the `Engine` for the process's whole life.
  After `mv`, the process's own cwd follows the inode but the frozen `geom.AnchorPath` string does not.
  The exact chain, per tick: `watchLoop` → `reapplyLayout` (`internal/reedengine/reapply.go:90`) → `withTryOpLock` → `os.MkdirAll(e.stateDir())` creates `<hub>/<old-name>/` AND `.lyx` under it (`MkdirAll` creates every missing parent) plus `reed.lock`; then `requireSessionLocked` (`lifecycle.go:1134`) finds no session under the old name and its error path calls `LoadState(e.stateDir())` at `lifecycle.go:1148`, whose `internal/state.ReadJSON` creates `reed.json.lock`; the error returns before `SaveState` is ever reached.
  That is byte-for-byte the reported shape: a REAL directory (not a symlink), exactly two 0-byte lock files, and no `reed.json` beside them.
  It also explains the rest: a CLI `status` produces no stray because `PersistentPreRunE` resolves it a fresh, correct geometry — NOT because it stops short of the lock, which it does not (`Status` acquires `withOpLock` at `lifecycle.go:1165` and the renamed-worktree refusal fires inside it, after the `MkdirAll`).
  And the stray's name varies between "the pre-rename name" and "a name from an earlier rename step" because it is whichever name the long-lived process happened to boot under.
  Why now: the watchdog daemon (`8002cf976`) is the newest reed commit and the first long-lived in-session process that runs engine ops on a timer.
- Rejected: a stale recorded session name read out of `reed.json` — no code path joins a recorded session name or `Strand.Worktree` onto a filesystem path; `state.Worktree`/`Strand.Worktree` are stamped (`strand.go:177`) and declared (`state.go:25`) but never read outside tests, and `Down`'s abandonment report reads `st.PaneGeneration.SessionName` purely as message text (`lifecycle.go:900-904`).
  Rejected: `lyxcwd` returning a stale worktree root — verified empirically that `git rev-parse --show-toplevel` returns the NEW path after `mv`, with or without `git worktree repair`; and `readRecordedAnchor` yields a worktree-relative `AnchorRel`, never a sibling worktree name.
  Rejected: the tmux resize hook creating it — `posixShell.Touch` is `: > 'path'` (`internal/shell/posix.go:36`), a redirection with no `mkdir`, and the signal hook that would run it is never installed anyway (see the follow-up below).
  Rejected: `fabricengine` junction wiring (`junction.go:149`) — that `MkdirAll`s `_lyx` (`lyxdirs.LyxDirName`), not `.lyx` (`DotLyxDirName`).

### the-leak-is-continuous-not-per-event

- Decision: The poll model is the one this task builds on: an ungated 2-second `reapplyLayout` that creates the stray at the first tick after the rename and keeps re-entering it for as long as the stale-anchored watcher lives.
  The stray is NOT created by the verb, and its creation is not synchronised to any verb.
  Where the sandbox report's "birth timestamp matches the exact moment the verb ran" conflicts with this, the poll model wins — it is what the source says, and at human resolution the two are the same few-second window (rename → escape verb) rather than genuinely distinguishable observations.
- Rationale: `watchLoop` starts in `watchModePoll` (`watchloop.go:180`) and poll mode calls `reapplyLayout` every `watchdogPollCycle` = 2s with no gating.
  Promotion to `watchModeSignal` requires `hookInstalledLocked()` to see an exact match on `resizeHookCommand` in the `window-resized` option — and that signal hook has no install site, while the option itself is occupied by `installResizePinsLocked`'s `resize-pane` pin array — so the probe reports "not installed" every time and every watcher is pinned in poll mode permanently (see the follow-up Decision below).
  This matters for two reasons.
  First, under M25 `down` is documented to leave the abandoned session running (`lifecycle.go:806-813`), so its stale-anchored watcher keeps recreating the stray every 2s for as long as that pane lives: deleting the lock files by hand does not stick until the session dies, which matches the manual recovery the suite run needed.
  Second, it means the watchdog-termination decision below is load-bearing, not cosmetic — without it, refusing at the lock chokepoint converts a filesystem leak into a `Warn` every 2 seconds forever.
- Rejected: modelling this as "resize events fire the watcher", which would have made the leak bounded and the termination optional.
- Rejected: a stale recorded session name read out of `reed.json` — no code path joins a recorded session name or `Strand.Worktree` onto a filesystem path; `hubgeom.ReedGeometry` builds `AnchorPath` only from `lyxcwd.Location`.
  Rejected: `lyxcwd` returning a stale worktree root — verified empirically that `git rev-parse --show-toplevel` returns the NEW path after `mv`, with or without `git worktree repair`.

### refuse-at-the-op-lock-chokepoint

- Decision: Refuse the operation when the told `WorktreeRoot` does not exist as a directory, checked in `withOpLock` and `withTryOpLock` immediately after the existing `validateToldAnchorPath` call and before the `os.MkdirAll(dotLyx)`.
  (See the next Decision for why the predicate is `WorktreeRoot` and not `AnchorPath`.)
- Rationale: those two helpers are the single chokepoint every public engine op passes, and the existing refusal ordering (identity → anchor shape → MkdirAll → lock) already exists precisely so a bad geometry cannot create substrate.
  This slots one more predicate into an established sequence and fixes every verb — `up`, `add`, `remove`, `status`, `attach`, `resume`, `down`, and the watchdog's `reapplyLayout` — with one change.
  It does not derive anything, so the Told-Geometry Invariant is untouched: reed still only refuses a geometry it was told.
- **Stat-error semantics:** only `errors.Is(statErr, fs.ErrNotExist)` — plus the exists-but-is-not-a-directory case — yields the terminal, watchdog-parking sentinel.
  Any OTHER `Stat` failure (EACCES, EIO, a stalled network mount) is a plain retryable error: it refuses this one op, and it must NOT match the sentinel.
  This matters because the sentinel makes `watchLoop` park permanently (next Decision but one).
  A transient stat blip on a healthy session must not silently kill that session's self-heal for the rest of the header pane's life on the strength of one `Warn`.
  The distinction is the difference between "the world proved this worktree root is gone" and "reed could not find out", and only the first is permanent.
- Rejected: re-resolving geometry per op (violates the Told-Geometry Invariant outright — `reedengine` may not import `lyxcwd`).
  Rejected: treating every `Stat` error as the terminal sentinel (turns a momentary NFS hiccup into a dead watchdog).
  Rejected: having the watchdog alone re-derive its own geometry (leaves every other verb able to leak, and puts cwd resolution inside an engine).
  Rejected: making `stateDir()` refuse (it is a pure path join used by non-mutating callers such as `resizeSignalPath`; a refusal there has nowhere to go).

### the-predicate-is-worktreeroot-exists-not-anchorpath-exists

- Decision: The check is that **`Geometry.WorktreeRoot`** exists and is a directory — NOT `AnchorPath`.
  The `os.MkdirAll(e.stateDir())` that creates `AnchorPath/.lyx` stays exactly as it is.
- Rationale: `AnchorPath` is the wrong field to gate on, because reed has two `Geometry` tellers and they mean different things by it.
  In hub mode `AnchorPath` is inside a worktree that must pre-exist (`hubgeom.ReedGeometry`: `AnchorPath = l.AnchorPath()`, `WorktreeRoot = l.WorktreePath()`).
  In standalone mode `AnchorPath` is a *derived state directory* reed legitimately owns and creates (`standalonegeom.ReedGeometry`: `AnchorPath = stateDir` from `standalonestate.Derive`, `WorktreeRoot = target`).
  `standalonestate.Derive` creates nothing on disk, and neither `burlercli/wiring.go:157` nor `webstercli/wiring.go:153` `MkdirAll`s `stateDir` — `withOpLock`'s existing `MkdirAll` is what materializes it on standalone first run.
  Gating on `AnchorPath` would therefore break standalone first-run outright, and would report "this worktree was renamed" about a path that is not a worktree at all.
  `WorktreeRoot` is the one field both tellers agree on: it is `<hub>/<name>` in hub mode — precisely the directory that vanishes on rename and precisely the one the stray conjured out of nothing — and it is the operator's own target directory in standalone mode.
  One predicate, both modes, no new `Geometry` field, no change to either wiring, and nothing derived.
- **Standalone targets are NOT guaranteed to exist, and refusing them is an intended behaviour change.**
  An earlier draft of this Decision claimed the standalone target "always exists by the time reed runs". That is false, and the plan must not rely on it: `resolveStandaloneTarget` (`burlercli/wiring.go:173-181`, `webstercli/wiring.go:232-239`) only absolutises `--target-dir`, with no `Stat`, and `standalonestate.derive` explicitly falls back to `Clean` alone "when the target does not exist on disk yet" (`standalonestate.go:57-59`).
  So `lyx … --target-dir /nope` reaches `withOpLock` today with a non-existent `WorktreeRoot`, proceeds, and creates `stateDir`.
  Under this predicate it will refuse instead.
  That is the right outcome — reed running a session against a directory that does not exist is not a thing to support, and silently deriving a `stateDir` for it is the same class of bug as the stray this task removes — but it IS a user-visible change to standalone mode, so it is called out here, gets its own test, and must not arrive as a surprise in review.
- Rejected: gating on `AnchorPath` (breaks standalone first-run — this is what the round-2 review caught).
  Rejected: requiring `.lyx` to pre-exist (breaks every first run in both modes).
  Rejected: adding a told `AnchorMustExist bool` to `Geometry` so each teller declares its own answer.
  It would work and would not violate the Told-Geometry Invariant, but it adds a field, two teller changes, and a new way for a future teller to get it wrong, to express something `WorktreeRoot` already expresses correctly in both modes.
  Rejected: having the standalone wiring `MkdirAll` its `stateDir` before constructing the engine, so the `AnchorPath` gate could stand — it relies on every future standalone teller remembering, and the hub/standalone asymmetry stays unexpressed in the engine.
- Note for the plan: a non-`.` `AnchorRel` makes `AnchorPath` a committed subdirectory *inside* `WorktreeRoot`. That subpath is tracked content, so it cannot be missing while the worktree exists, and the two vanish together on a rename. Gating on `WorktreeRoot` leaves no gap there.

### worktreeroot-gets-a-shape-check-before-it-gets-a-liveness-check

- Decision: The new validator checks `WorktreeRoot`'s SHAPE first — non-empty and absolute — and only then its liveness.
  A shape violation is a plain told-contract error that does **NOT** match the proven-gone sentinel.
  Both live in one function; the ordering is internal to it.
- Rationale: this task promotes `WorktreeRoot` from a decorative field to the load-bearing control-flow predicate, and it has no shape validator anywhere in `internal/reedengine` today — it appears only at `strand.go:176-177` (stamped onto a strand) and as message decoration inside `validateToldTmuxIdentity` / `validateToldAnchorPath`.
  Promoting it without a shape check is unsafe in two specific ways.
  An EMPTY `WorktreeRoot` makes `os.Stat("")` return `fs.ErrNotExist`, which under the semantics above would yield the terminal sentinel: a permanently parked watchdog, and a "this worktree was renamed or removed" message about `""`.
  A RELATIVE `WorktreeRoot` silently stats against whatever working directory the process happens to have — precisely the failure `validateToldAnchorPath` exists to backstop for `AnchorPath`, and precisely the class of silent-wrong-tree bug this whole task is about.
  Non-sentinel is the load-bearing half: a shape violation is a *caller bug*, not proof the world changed, so it must refuse this op loudly and let the watchdog keep running rather than park forever and bury the bug.
- Rejected: leaving `WorktreeRoot` unvalidated and declaring that `Geometry`'s contract covers it.
  It does not — `geometry.go` documents `WorktreeRoot` only as "what `Strand.Worktree` is stamped with", written when nothing branched on it.
  Rejected: mapping a shape violation onto the sentinel (parks the watchdog permanently on a caller bug).
  Rejected: adding the shape checks to `validateToldAnchorPath` (it is a pure, table-tested validator for a different field; see the Decision below).
- Ordering at both lock helpers: `validateToldTmuxIdentity` → `validateToldAnchorPath` → the new `WorktreeRoot` validator (shape, then liveness) → `os.MkdirAll(dotLyx)` → acquire `reed.lock`.
  The new check goes last among the validators because it is the only one that touches the filesystem, and the two cheap contract refusals should still fire first on a geometry that is wrong in several ways at once.

### a-separate-validator-keeps-the-pure-one-pure

- Decision: Add a new function — `validateToldWorktreeRootLive(geom Geometry) error`, or whatever the plan names it, as long as the name says **WorktreeRoot** — beside `validateToldAnchorPath` in `internal/reedengine/server.go`, rather than touching `validateToldAnchorPath` at all.
  It validates `Geometry.WorktreeRoot`; it does not look at `AnchorPath`.
- Rationale: `validateToldAnchorPath` is documented and tested as a pure shape validator over a DIFFERENT field, driven by a table test in `server_test.go`.
  Mixing filesystem I/O into it would make that table test I/O-dependent, and folding a second field's rules into it would blur three separate diagnoses — "you told me an unusable anchor", "you told me an unusable worktree root", and "the worktree root you told me is gone" — that deserve distinct messages.
- Rejected: extending `validateToldAnchorPath` in place.
  Rejected: any name built on "AnchorPath" (an earlier draft of this discussion proposed `validateAnchorPathLive`, a leftover from when the predicate was `AnchorPath`; the name would now lie about which field is checked).

### the-error-names-the-vanished-path-and-the-remedy

- Decision: Two distinct messages, not one.
  **Vanished** (`fs.ErrNotExist`): states that the told worktree root does not exist, quotes the path, and says reed never creates it — then names BOTH causes plainly without asserting which applies: in a hub worktree the directory was renamed or removed; in standalone mode `--target-dir` names a directory that does not exist.
  It must not assert a rename. The engine cannot tell the two modes apart (it is told its geometry and nothing else, per the Told-Geometry Invariant), and telling a standalone operator to fix a rename that never happened is exactly the misdiagnosis this Decision exists to prevent.
  **Not a directory** (the path exists but `Stat` reports a non-directory): states that the told worktree root is a file rather than a directory and quotes the path — no rename remedy, because nothing was renamed.
  A third, non-sentinel case — any other `Stat` failure — reports the underlying error verbatim and says reed could not determine whether the worktree root exists.
- Rationale: these errors are reachable in two very different situations — an operator standing in a deleted worktree, and an abandoned session's own header pane logging into a serverlog — so each has to be self-describing with no surrounding context.
  Sharing one message across them would make the not-a-directory case tell the operator to fix a rename that never happened, sending them looking for the wrong thing.
  It matches the house style already set by `validateToldTmuxIdentity` and `validateToldAnchorPath`, both of which name the offending value and the corrective action.
- Rejected: a bare "anchor path not found".
  Rejected: one shared message for all three cases (misdiagnoses two of them).

### the-watch-loop-terminates-when-its-anchor-is-gone

- Decision: When `reapplyLayout` returns the proven-gone sentinel, `watchLoop` stops looping: log one `Warn` naming the vanished worktree root and the session, then park on `ctx.Done()` exactly as the `watchdog: off` branch already does, returning `ctx.Err()`.
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
- Rationale: the *signal* hook — `resizeHookCommand` (`watchdog.go`), the `run-shell -b` that touches `reed-resize.signal` — has no install site anywhere in the repo.
  The `window-resized` option itself is NOT unused, though: `resizePinHookArgvs` / `installResizePinsLocked` (`internal/reedengine/windowsize.go:197-237`) issue `set-hook -w -t <window> window-resized "resize-pane …"` on every successful apply (`apply.go:235`) and every attach (`attach.go:144`), building a pin array on exactly the same window-scoped option that `hookInstalledLocked` reads back (`reapply.go:59`).
  So the two contend for one option, and the pin array wins: it clears and rebuilds `window-resized` unconditionally on every apply, which would wipe a signal hook even if one were installed.
  Meanwhile the probe demands an exact match on `resizeHookCommand`, which the pin bodies never satisfy, so it reports "not installed" every time.
  The conclusion stands — `watchModeSignal`, `watchdogSignalTick`, and the whole debounce/retry state machine are dead in production and every watcher runs poll-only, forever — but by contention plus a missing install site, not by the option being untouched.
  `contracts/.../template_posix.yaml:10` still advertises that it "enables ... the session's window-resized hook", which the shipped behaviour does not deliver.
  This may be deliberate batch staging, but the config text and the code disagree, and it is what makes this leak continuous rather than event-driven.
  Whoever picks up the follow-up must resolve the contention, not merely add the missing `set-hook`: appending the signal touch to the pin array (or moving one of the two off `window-resized`) is the actual design question.
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
- `internal/hubgeom/hubgeom.go:18-30` — hub-mode `ReedGeometry(l)`: `AnchorPath = l.AnchorPath()`, `WorktreeRoot = l.WorktreePath()` (i.e. `<hub>/<name>`). Not a defect site; listed so the plan does not go looking for one, and because `WorktreeRoot`'s value here is what makes the chosen predicate work.
- `internal/standalonegeom/reedgeom.go:19-31` — standalone `ReedGeometry(target, stateDir, hash8)`: `AnchorPath = stateDir`, `WorktreeRoot = target`, and the doc comment states outright that the two "deliberately diverge". This is the second teller the predicate has to survive.
- `internal/standalonestate/standalonestate.go:31` — `Derive` computes `stateDir` and creates nothing on disk.
  Neither standalone wiring `MkdirAll`s `stateDir` directly, but the claim "nothing creates it" is too strong: with no `--stencils-dir`, both wirings call `stencilstore.Reconcile(standalonegeom.StencilsDir(stateDir))` — `<stateDir>/_lyx/stencils` (`standalonegeom/stencilsdir.go:25`, `burlercli/wiring.go:139`, `webstercli/wiring.go:159`) — which does create `stateDir` before the engine ever runs.
  So `withOpLock`'s `MkdirAll` is the SOLE materializer only on the `--stencils-dir` path, where `Reconcile` is skipped.
  The `WorktreeRoot` decision is unaffected either way — it just must not rest on the overstated version.
  That `MkdirAll` must keep working: a plan that gates it on `AnchorPath` breaks standalone `--stencils-dir` first-run.
- `internal/reedengine/windowsize.go:197-237` — `resizePinHookArgvs` / `installResizePinsLocked` write the `window-resized` window-hook array. Relevant only as context for the hook follow-up below; not a change site in this task.
- `internal/reedengine/watchloop.go:159` — `watchLoop`. The `watchdog: off` branch (`<-ctx.Done(); return ctx.Err()`) is the parking pattern to reuse.
  `handleWatchOutcome` (same file) is where an error is currently classified as retryable; the proven-gone case has to be distinguishable there, which is why the plan needs a package-level sentinel error (`errors.Is`-matchable) rather than a string match — a non-`ErrNotExist` stat failure must NOT match it.
- `internal/reedengine/reapply.go:90` — `reapplyLayout` calls `withTryOpLock` and returns `(ReapplyResult, error)`; the refusal surfaces through that `error`.
- `internal/reedengine/lifecycle.go:1134,1148` — `requireSessionLocked` is what calls `LoadState` on the failure path, creating the second lock file. Not a change site: once `withTryOpLock` refuses, it is never reached.
- `internal/reedcli/cli.go:63-98` — `PersistentPreRunE`, where geometry is resolved once per process and pinned into the `Engine`. This is correct for one-shot verbs and is the origin of the frozen anchor for the header pane. Not a change site under the chosen fix, but the plan should not "fix" it by re-resolving per op.
- `internal/reedengine/lifecycle.go:501-515,627` — where the header pane is split with `-c e.geom.PaneCwd` running `headerLaunchLine`; the launched process re-resolves its own geometry from that cwd.
- `internal/reedengine/lock_test.go:29` — `newTestEngine` builds `WorktreeRoot = <tmp>/worktree` and `AnchorPath = <tmp>/worktree/anchor`, creating neither on disk. It must now `os.MkdirAll` the WORKTREE ROOT only, leaving `AnchorPath` and `PaneCwd` uncreated — which preserves the fixture's stated intent (distinct values so a field mix-up surfaces) and makes it stand in for the standalone shape.
  14 test files call the helper, but "callers of `newTestEngine`" is NOT the right inventory criterion: the package also has op-running tests that build an inline `Geometry` literal with an uncreated `WorktreeRoot`, e.g. `lock_test.go`'s `TestWithOpLock_PathIsUnderDotLyx` (`worktreeRoot := filepath.Join(hub, "worktree")` with no `MkdirAll`, then `e.withOpLock`). The correct criterion is "every in-package test that reaches `withOpLock`/`withTryOpLock`".
  Inline sites materialize their own `WorktreeRoot` rather than migrating onto the helper: each one builds a deliberate geometry (`TestWithOpLock_PathIsUnderDotLyx` exists precisely to make `AnchorPath` diverge from `WorktreeRoot`), and folding them into the shared helper would erase the distinction the test was written to observe.
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

- `withOpLock` refuses when `WorktreeRoot` names a directory that does not exist: the op body never runs, the error names the vanished path, and neither the worktree directory, nor `AnchorPath`, nor `.lyx`, nor `reed.lock` is created afterwards.
  Mirror `TestWithOpLock_RefusesAnUnusableAnchorPathBeforeCreatingState`'s shape.
- The same for `withTryOpLock` — separately, not as a shared subtest, since the leak lived on this branch and a regression that fixes only one helper must fail.
- `WorktreeRoot` exists but is a regular file, not a directory: also refused, with the not-a-directory message rather than the vanished-path one.
- **Standalone with a non-existent target:** `WorktreeRoot` does not exist and `AnchorPath` is a different, also non-existent path — the `--target-dir /nope` shape. Refused with the vanished-path message, and neither path is created. This is the regression guard on the intended standalone behaviour change.
- The vanished-path message names both causes (renamed/removed worktree, and a `--target-dir` that does not exist) and asserts neither — a message that claims a rename is a test failure.
- **The standalone shape:** `WorktreeRoot` exists as a directory and `AnchorPath` is a DIFFERENT path that does not exist yet (`newTestEngine`'s fixture already has them diverge). The op must SUCCEED and `MkdirAll` must create `AnchorPath/.lyx`. This is the regression guard for standalone first-run, and it is the test that fails if a later change re-gates the predicate on `AnchorPath`.
- Hub-shaped first run: `WorktreeRoot` exists, `AnchorPath` equals it, no `.lyx` yet — op SUCCEEDS and creates `.lyx`.
- The proven-gone error is matchable by the watch loop via the package sentinel, asserted with `errors.Is` rather than by substring.
- A non-`ErrNotExist` stat failure does NOT match the sentinel. Provoke a REAL one — no injectable stat seam is introduced, so no production indirection is added and §Scope is unaffected. On POSIX, `chmod 0000` the parent directory so `Stat` on the child returns `EACCES`; skip the test on Windows and when the process is running as root (both defeat the permission bit). The whole point of this distinction is that it is invisible until it misfires, so it gets a real test rather than a skip.
- `WorktreeRoot` empty: refused with the told-contract error, and that error does NOT match the sentinel — the case that would otherwise turn `os.Stat("")`'s `ErrNotExist` into a permanently parked watchdog.
- `WorktreeRoot` relative: refused with the told-contract error, non-sentinel, and asserted to refuse regardless of the test process's working directory.
- `validateToldAnchorPath`'s existing table test must remain I/O-free and unchanged; the new validator gets its own focused test.

Unit, watch loop:

- `watchLoop` in poll mode, given a proven-gone refusal from `reapplyLayout`, stops issuing further reconcile attempts and does not return until ctx is cancelled. Assert a call-count that stays flat after the first refusal — `watchloop_test.go` already has hook-call-count fixtures to model on.
- The same in signal mode, so the retry-streak machinery cannot swallow the termination.
- A transient error — including a non-`ErrNotExist` stat failure — still retries exactly as today. This is the regression guard on the narrowing, and the reason the sentinel is matched rather than "any error from the anchor check".

Fixture:

- `newTestEngine` creates its `WorktreeRoot`, and deliberately does NOT create `AnchorPath` — preserving the fixture's existing intent that the fields are distinct values so a mix-up surfaces, while now also standing in for the standalone shape.
- Every in-package test that reaches `withOpLock`/`withTryOpLock` through an INLINE `Geometry` literal materializes its own `WorktreeRoot` in place (`lock_test.go`'s `TestWithOpLock_PathIsUnderDotLyx` is the known one; sweep for others rather than trusting that list). They are not migrated onto the helper — each was written to observe a specific field arrangement that the shared fixture would erase.
  Run the whole `internal/reedengine` package afterwards **with the `integration` and `smoke` build tags as well as untagged** — `go test` with no tags never compiles the tag-gated files, so a sweep that stops at the default build silently skips them.
  Those files also drive real ops: `contract_integration_test.go:648,757` and `mouse_boot_integration_test.go:48` build inline `Geometry` literals and are in scope on exactly the same criterion as the untagged inline sites.
  `newIntegrationEngine` (`mouse_boot_integration_test.go:27`) is a SECOND shared helper — `attachgeometry_integration_test.go` and others build engines through it rather than inline — and it already `os.MkdirAll`s its worktree directory, so it needs no change. Verify that rather than assuming it, and do not treat `attachgeometry_integration_test.go` as an inline site; it has no `Geometry` literal at all.
  Any test that was silently relying on the worktree root being absent is a real finding, not something to paper over.

Sandbox (manual, live tmux — the suite is an agent-driven black box, so this is doc text, not code):

- M24: after `kill-session` + `resume` in the renamed worktree, the hub root contains no directory named after the pre-rename worktree.
- M25: after `down` in the renamed worktree, the same assertion — and, because `down` deliberately leaves the abandoned session running, re-check the hub root after waiting well past the 2s poll cycle, so a watcher that resumed leaking is caught rather than masked by checking too early.
- Both should also confirm the abandoned session's header pane stopped logging reconcile failures rather than spinning.

## Q&A log

- **Q:** What actually creates the stray — the invoked verb, or something else? **A:** [auto-pick] The old session's still-running header-pane watchdog, whose `Engine` geometry is frozen at the pre-rename `AnchorPath`. **Why:** it is the only mechanism that accounts for all of the observed evidence simultaneously — a real (not symlinked) directory, both lock files with no `reed.json`, birth timestamps matching geometry-changing verbs only, no stray from read-only `status`, a name that varies with which rename step the process booted under, and the fact that this surfaced immediately after the watchdog daemon landed. Note the timestamp evidence was restated during review: the poll model puts creation at the first tick after the rename, not at the verb, and the two are only indistinguishable because the rename and the escape verb are seconds apart in the prescribed workflow.
- **Q:** Where does the fix live? **A:** [auto-pick] Refuse at the op-lock chokepoint (`withOpLock` + `withTryOpLock`) when the told worktree root no longer exists. **Why:** one chokepoint covers every verb including the watchdog, it slots into an existing refusal sequence built for exactly this purpose, and it refuses a told value rather than deriving one, so the Told-Geometry Invariant holds.
- **Q:** Gate on `AnchorPath` or `WorktreeRoot`? **A:** `WorktreeRoot`. **Why:** raised as BLOCKING by discussion review round 2, and correct — reed has two `Geometry` tellers, and gating on `AnchorPath` breaks standalone mode, where `AnchorPath` is a derived state directory `standalonestate.Derive` never creates and `withOpLock`'s own `MkdirAll` is what materializes on first run. `WorktreeRoot` is `<hub>/<name>` in hub mode (exactly what the stray conjured) and the operator's real target repo in standalone mode (always present), so one predicate serves both tellers with no new field and no wiring change.
- **Q:** Does a non-`ErrNotExist` stat failure (EACCES, EIO, stalled mount) also mean "gone"? **A:** No — only `fs.ErrNotExist` and the exists-but-not-a-directory case yield the terminal sentinel; every other stat error is retryable. **Why:** raised as BLOCKING by round 2. The sentinel parks the watchdog permanently, so mapping a momentary stat blip onto it would kill a healthy session's self-heal for the rest of that header pane's life on the strength of one `Warn`.
- **Q:** `WorktreeRoot` has no shape validator today — does the new predicate need one? **A:** Yes: non-empty and absolute, checked before liveness, and a shape violation must NOT carry the terminal sentinel. **Why:** raised as BLOCKING by round 3. An empty value makes `os.Stat("")` return `ErrNotExist`, which would park the watchdog permanently and report a rename of `""`; a relative value stats against the process cwd, the exact silent-wrong-tree failure this task is about. Non-sentinel because a shape violation is a caller bug, and parking forever on a caller bug buries it.
- **Q:** Is "callers of `newTestEngine`" the right way to enumerate the fixture work? **A:** No — the criterion is "every in-package test that reaches `withOpLock`/`withTryOpLock`", helper or inline literal. **Why:** round 3 caught that `TestWithOpLock_PathIsUnderDotLyx` builds an inline `Geometry` with an uncreated `WorktreeRoot` and calls `withOpLock`, so the helper change misses it. Inline sites materialize their own root rather than migrating onto the helper, because each was written to observe a field arrangement the shared fixture would erase.
- **Q:** Does testing the non-`ErrNotExist` stat path require an injectable stat seam? **A:** No. Provoke a real `EACCES` via `chmod 0000` on the parent, skipping on Windows and as root. **Why:** a seam would be production code this task's scope does not include, and the case is worth a real test rather than a skip precisely because it is invisible until it misfires.
- **Q:** Does the not-a-directory case share the vanished-path error message? **A:** No, it gets its own. **Why:** the vanished-path message is about a path that is absent; saying that about a path that exists as a regular file sends the operator looking for the wrong thing.
- **Q:** Is the standalone target guaranteed to exist when reed runs, as an earlier draft claimed? **A:** No — `resolveStandaloneTarget` never `Stat`s it and `standalonestate.derive` handles a non-existent target explicitly, so `--target-dir /nope` reaches `withOpLock` today and succeeds. Under this predicate it refuses. **Why:** raised as BLOCKING by round 5. Refusing is correct — reed should not run a session against a directory that does not exist — but it is a user-visible standalone behaviour change, so it is stated outright, given its own test, and the vanished-path message was reworded to name both causes rather than assert a rename the engine cannot verify.
- **Q:** Check the anchor's existence, or also pin its identity (dev+inode) so a REUSED name is caught too? **A:** [auto-pick] Existence only. **Why:** the reported defect and the landmine both come from the anchor vanishing; pinning inode identity would add per-op state to `Engine`, which `New` is contractually forbidden from populating, for a case (a new worktree created under the abandoned name while the old header pane still runs) that the recorded-session generation check already degrades safely on. Recorded here so a later reviewer sees it was considered, not missed.
- **Q:** Extend `validateToldAnchorPath`, or add a separate validator? **A:** [auto-pick] A separate, I/O-touching validator beside it. **Why:** the existing one is a documented pure shape check with a table test; mixing a `Stat` in would make that test I/O-dependent and merge two diagnoses that deserve distinct messages.
- **Q:** What should the watchdog do once it learns its worktree root is gone? **A:** [auto-pick] Log one `Warn` and park on `ctx.Done()`, without killing the header pane. **Why:** the condition is permanent, so retrying is pure log spam; and the abandoned session may still host the operator's live strands, which M25 requires `down` to leave running.
- **Q:** Should reed clean up strays that already exist? **A:** [auto-pick] No. **Why:** hub-level deletion is chokepointed by design, and an empty hub directory is indistinguishable from a legitimate one; a single manual `rmdir` clears the historical case.
- **Q:** Is `internal/state.ReadJSON`'s `MkdirAll` on a read path in scope? **A:** [auto-pick] Out of scope, recorded as a follow-up. **Why:** it is a genuine defect in a different module with four other consumers, and once reed refuses at the chokepoint no reed path reaches it with a bogus directory — so it is defense-in-depth, not part of this fix.
- **Q:** Do the other verbs (`up`, `add`, `remove`, `status`, `attach`) need their own handling? **A:** [auto-pick] No — they inherit the fix. **Why:** all of them pass through `withOpLock`, which is the single chokepoint the check is added to. Strictly, none of them leaks on its own either: every CLI invocation resolves a fresh, correct anchor. The leak belongs to whichever `up`/`resume` created the header pane that is still running.
- **Q:** The signal hook is never installed, so signal mode is dead and every watcher polls every 2s — fix that here too? **A:** [auto-pick] No, record it as a follow-up. **Why:** it is a separable watchdog feature-completion change with its own live-tmux verification burden; bundling it would make this fix unreviewable and unrevertable on its own, and the refusal is correct under either watch mode.
- **Q:** Is the claim "no `set-hook` install site anywhere" accurate? **A:** No — corrected. **Why:** round 2 caught it. `installResizePinsLocked` does install `window-resized`, with `resize-pane` pin bodies, on every apply and attach. What has no install site is the signal `touch` (`resizeHookCommand`), and the two contend for the same window-scoped option — the pin array rebuilds it unconditionally, so it would wipe a signal hook even if one were added. The poll-only conclusion survives; the inventory behind it did not, and the follow-up now names the contention as the real design question.
