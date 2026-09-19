# Conflict Resolution Brief

Your sole job is to resolve git conflict markers in the listed files, stage each resolved file, and report success.
Do NOT commit.
Do NOT run `git merge --continue` — the SKILL does that after receiving `{"status":"success"}`.

## Task intent

These excerpts describe what THIS branch is trying to accomplish.
When the merge introduces a parent-side change that conflicts with this branch's intent, the resolution preserves THIS branch's intent.
In particular: if a file appears under a batch's `Deletes:` list and the merge introduces a modified version of that file from the parent, the resolution is to delete the file (your branch's intent overrides).
Stage the deletion with `git -C /home/knatte/Code/loomyard/wts/reed-cold-worktree-selfheal rm <file>`.

### From discussion.md

# Discussion: reed: AddStrand and attach self-heal a cold worktree

```yaml
task: 'reed: AddStrand and attach self-heal a cold worktree'
slug: reed-cold-worktree-selfheal
status: discussing
parent: main
```

## Problem

A worktree nobody has visited has no tmux session.
Today the two entrypoints that actually *use* a session — `lyx reed add` (engine: `Engine.AddStrand`) and `lyx reed attach` — both refuse in that state rather than creating one:
`AddStrand` calls `requireSessionLocked` (`internal/reedengine/strand.go:386`) and `reedcli`'s attach verb pre-flights `Engine.Status()` (`internal/reedcli/attach.go:55`), which calls the same helper (`internal/reedengine/lifecycle.go:1166`).
Both fail with `noSessionMessage`'s friendly `no reed session; run "lyx reed up"`.
So every spawn into, or view of, a fresh worktree is a two-step ritual: `lyx reed up` first, then the thing you actually wanted.

Why now: `manifest/designs/worktree-lifecycle-shed-producers.md` — the Someday "worktree spawn/teardown as Shed producers" item — is designed on the explicit premise that no bootstrap producer row is needed, because "Up doesn't need an explicit owner: it's ambient, self-healing behavior inside reed's own entrypoints (`AddStrand`, `attach`), triggered by whoever is about to actually use the session."
That item is blocked on this one.
The self-heal property also survives a machine reboot for free — worktrees persist, tmux servers do not — which a one-time "set it up at creation" step never would.

## Scope

**In:**

- `internal/reedengine/lifecycle.go` — extract the body of `Up()` into a new locked helper `upLocked() (UpResult, booted bool, err error)`; `Up()` wraps it in `withOpLock` and discards the bool. Add `ensureSessionLocked() (booted bool, err error)` and its exported `withOpLock` wrapper `EnsureSession() (booted bool, err error)`. Factor the "session up and holding ≥1 pane" predicate out of `ensureServerAndSessionLocked` so both it and `ensureSessionLocked` read the same one.
- `internal/reedengine/strand.go` — `AddStrand` calls `e.ensureSessionLocked()` in place of `e.requireSessionLocked()`, after `validateIfAbsent`.
- `internal/reedcli/attach.go` — the pre-flight gains `c.eng.EnsureSession()` **ahead of** its existing `c.eng.Status()` call; `Status()` is kept, not replaced.
- Observability: one `logger.Info` at each self-heal site when that call returned `booted == true`.
- Doc updates in the same commit: `internal/reedengine/doc.go`, `AddStrand`'s and `AttachArgv`'s own doc comments, `internal/vscode/config.go`'s tasks.json rationale comment, `docs/overview.md`'s reed entry, `manifest/designs/worktree-lifecycle-shed-producers.md`'s "Today, without this item built yet" framing, and `manifest/roadmap.md` (Planned → Done).
- `internal/webstercli/run.go:100-103` and `internal/webstercli/recoverbatch.go:117-124` — both comments state the invalidated "requires a live session" / "impossible recourse" premise verbatim and must be rewritten; the calls stay (see `webster-standalone-boots-kept`).
- `tools/sandbox/SANDBOX-REED-SUITE.md` (scenario M1, split), `tools/sandbox/SANDBOX-WEBSTER-SUITE.md` (prerequisite 5) and `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md` (scenario S5's second-run step) — see the Sandbox Suite Coverage constraint for the exact edits.
- `manifest/designs/reed-fabric-standalone-api.md` — this doc **freezes `*Engine`'s exported surface by count and by name**, and `EnsureSession()` breaks it. Three sites must move together: `:124` (the count **17** and the enumeration of all seventeen method names), `:130` (the restated "the 17 methods reached through `*Engine`"), and `:169` ("one handle type carrying 17 methods"). All become **18**, with `EnsureSession` added to the enumeration. The line-count figure at `:112` is a `wc -l` measurement, not a frozen contract — leave it unless the change makes it grossly wrong.
- Tests: hermetic ordering/regression tests in `internal/reedengine`, plus a `smoke`-tagged cold-worktree test in `internal/reedcli`.

**Out:**

- `Engine.Status`, `Engine.RemoveStrand`, `Engine.UpdateStrand`, `internal/reedengine/io.go`'s three send/capture ops, and `internal/reedengine/reapply.go` keep `requireSessionLocked` exactly as-is. `requireSessionLocked` and `noSessionMessage` are not deleted or weakened.
- `internal/loomcli/run.go`'s own attach handover (`c.reed.Status()` at `run.go:228`) is unchanged.
- `internal/loomcli/sharedbootstrap.go:185`'s `ensureStatusStrand` and `internal/loomcli/drive.go:88`'s pre-producer bootstrap both keep their explicit `c.reed.Up()` call — see the `redundant-up-sites-kept` decision.
- The generated `.vscode/tasks.json` chain (`reed up` → `reed add --if-absent` → `reed attach`) keeps its `reed up` row.
- `Engine.AttachArgv` gains no boot: it stays a non-refusing, `reed.json`-read-only builder.
- No `Resume` semantics anywhere — this task never relaunches a persisted strand.
- The `born-as-strand` item (`manifest/designs/reed-born-as-strand.md`, `loom run`'s terminal handoff never calling `AddStrand`) is a different problem and stays untouched.
- Exactly one new exported engine method — `EnsureSession()`. No other exported-surface change: `UpResult`, `Up()`, `Status()`, `AttachArgv()` and `AddStrand()`'s signatures are all unchanged, and `reedcli`'s `up` verb keeps its current envelope keys.
- No pre-boot readability check for a corrupt `reed.json` — see the `corrupt-state-boots-then-fails` decision.
- **The warm paths change only by adding probe round trips.** A `reed add` or `reed attach` against a live session with ≥1 pane performs no reconcile, no layout apply, no `SaveState`, and no config validation it did not already perform, and it keeps every refusal it performs today. This is a hard boundary on the task, not a nice-to-have — see `ensure-session-is-the-seam` for the two regressions it prevents and `attach-preflight-keeps-its-status-call` for the third.

## Decisions

### upLocked-extraction

- **Decision:** extract `Up()`'s closure body verbatim into `func (e *Engine) upLocked() (UpResult, booted bool, err error)`, leaving `Up()` as a `withOpLock` wrapper around it that discards the bool (see `booted-means-actually-created` for why the bool is threaded out). `upLocked` is what `ensureSessionLocked` delegates to on the cold path — no caller reaches it directly.
- **Rationale:** `withOpLock` acquires a `gofrs/flock` write lock on `.lyx/<reedLockFileName>` (`internal/reedengine/lock.go:89-114`) and is not reentrant, so `AddStrand` cannot call the exported `Up()` from inside its own closure. Beyond the lock, the *whole* `Up()` body is what a **cold** worktree needs, not just the boot: on `booted == true` it clears every stale pane binding, stamps `StrippedEnv`, resets `HeaderPaneID`, then `ensureHeaderPaneLocked` builds the header pane and `reconcileApplyPersistLocked` applies the layout. Calling bare `ensureServerAndSessionLocked` would leave a session with no header pane, and `AddStrand` would then split its strand into a headerless window — a different substrate from the one `lyx reed up` produces. (What a *warm* call needs is none of this; that is the `ensure-session-is-the-seam` decision's job.)
- **Rejected:** (a) bare `ensureServerAndSessionLocked` — produces a substrate `reed up` never produces; (b) duplicating the booted-bookkeeping block at each call site — two copies of a comment block that already warns future editors not to "fix" it.

### up-semantics-not-resume

- **Decision:** the self-heal always has `up` semantics. A cold worktree whose `reed.json` records strands gets a bare substrate; those strands are **not** relaunched.
- **Rationale:** `reed add` must add one strand, not resurrect a whole prior session as a side effect. `noSessionMessage` deliberately distinguishes these two cases today and routes the operator to `resume` when there is content to rebuild; `resume` stays an explicit operator verb.
- **Rejected:** `resumeLocked` — surprising, and it would relaunch processes the caller never asked for.

### ensure-session-is-the-seam

- **Decision:** the self-heal entry point is a **new, narrow helper** — not `Up()`. In `internal/reedengine`:
  - `ensureSessionLocked() (booted bool, err error)` — asks whether this worktree's session is up *and* holds ≥1 pane. If so it returns `(false, nil)` immediately, having done nothing else. Otherwise it delegates to `upLocked()` and returns **`upLocked`'s own booted flag**, not a hardcoded `true` — see `booted-means-actually-created`.
  - `EnsureSession() (booted bool, err error)` — the exported `withOpLock` wrapper around it, for the CLI.

  `AddStrand` calls `ensureSessionLocked()` in place of `requireSessionLocked()`. `internal/reedcli/attach.go`'s pre-flight calls `c.eng.EnsureSession()` **and then keeps its existing `c.eng.Status()` call**, in that order — see `attach-preflight-keeps-its-status-call`.
- **Rationale — the warm path must not change at all.** Routing a warm call through `Up()` was the earlier answer and it is wrong, for two separate and independently sufficient reasons, both of which this helper's early return removes:
  1. **`upLocked`'s tail destroys panes.** `reconcileApplyPersistLocked` → `reconcileLocked` → `planReconcile` adds *every* live, non-exempt pane to `untrackedPanesToKill` whenever the header is alive (`internal/reedengine/reconcile.go:127-134`; its own log comment says it "destroys panes an operator may have created themselves"). Today's `Status()` pre-flight is read-only, so a warm `lyx reed attach` has never destroyed anything — and an operator who hand-split a pane inside the session would have lost it just by attaching.
  2. **`ensureServerAndSessionLocked` validates before its already-up early return.** `debugLogArgs`, `mouseOption`, `watchdogOption`, `ValidateHeader` and `probeCapabilityLocked` all run at `lifecycle.go:164-212`, *ahead of* the `up` early return at `:228-245`. So a `mouse:` typo or a below-floor tmux version would refuse `lyx reed attach` against a perfectly healthy live session — an operator locked out of *viewing* a session by a config error that has nothing to do with viewing it.

  Checking session-liveness first inverts that ordering deliberately: when there is nothing to boot, there is nothing to validate, nothing to reconcile, and nothing to write.

  Warm-path cost, per verb — they differ, and neither is "+2":
  - **`add`**: +1 round trip. `requireSessionLocked` already made the `has-session` call (`lifecycle.go:1135`); `ensureSessionLocked` adds only the `list-panes` that distinguishes a live session from a zero-pane husk.
  - **`attach`**: +2 round trips (`has-session`, `list-panes`) ahead of the `Status()` call it already made — see `attach-preflight-keeps-its-status-call`, which keeps that call rather than replacing it.
- **Why the pane check, not bare `has-session`:** a session that exists but holds zero panes is broken substrate that can never host a strand, which is why `ensureServerAndSessionLocked` kills the husk and re-boots (`lifecycle.go:228-247`). An early return on bare `has-session` would skip that repair and leave `add` failing forever. `ensureSessionLocked`'s predicate must therefore be the *same* "up and holding ≥1 pane" condition. **mill-plan must factor that predicate into a single helper both sites call**, rather than writing it twice — two copies of this condition can silently diverge, and the husk bug is exactly what a divergence would reintroduce.
- **Rejected:** (a) reusing `Up()` — the two reasons above; (b) booting inside `AttachArgv` — it would break both of that builder's documented properties at once ("returns no error, by contract" and "Read-only with respect to `reed.json`: this builder never calls `SaveState`"), and a boot failure there could only be logged, never surfaced on the envelope before stdio handover; (c) calling `Status()` first and `Up()` only on its error — `Status()`'s error is not classifiable without matching `noSessionMessage`'s text, so the retry would also fire on a corrupt `reed.json` and on a foreign-session refusal.
- **Pinned by:** the three warm-path smoke tests in the Testing section (untracked pane survives, config typo does not refuse, no state write).

### attach-preflight-keeps-its-status-call

- **Decision:** `internal/reedcli/attach.go`'s pre-flight becomes `EnsureSession()` **then** `Status()` — the existing `Status()` call is kept, not replaced. Any error from either aborts on the JSON envelope, exactly as today.
- **Rationale:** `EnsureSession` alone would silently drop two refusals the attach verb performs today. `Status()`'s job in that pre-flight was never only "is the session up" — it reaches `loadOrInitStateLocked` (`spawn.go:195`), which does a `LoadState` **and** `adoptPaneGenerationLocked` → `refuseLiveForeignSessionLocked` (`generation.go:142`). So today a warm `lyx reed attach` refuses, on the envelope, when `reed.json` is corrupt or when its persisted bindings were minted against a still-live session under another name. `EnsureSession`'s early return reads no state at all, and `AttachArgv`'s own `loadOrInitStateLocked` failure cannot restore the refusal because that builder degrades to the bare argv by contract and returns no error (`attach.go:111-115`). Keeping `Status()` makes the change strictly additive: every refusal attach performs today it still performs, and the only new behaviour is that a cold session is booted first.
- **Ordering is load-bearing:** `EnsureSession` must run **first**. `Status()` calls `requireSessionLocked`, so on a cold worktree the reverse order would refuse before anything booted — reinstating the exact bug this task removes.
- **Cost:** the warm path gains `EnsureSession`'s two probe round trips ahead of the `Status()` it already ran. The cold path runs `Status()` once against the session `EnsureSession` just booted.
- **Rejected:** (a) `EnsureSession()` alone — drops the two refusals above, and would make this file's "the warm paths do not change" claim false for `attach`; (b) accepting the loosening on the grounds that attach is an escape hatch and a bare attach still reaches the session — defensible in isolation, but it is a silent behaviour change to a verb's failure modes, decided as a side effect of a bootstrap task rather than on its own merits. If reed later wants attach to tolerate a corrupt `reed.json`, that is its own change with its own reasoning.
- **Note for `AddStrand`:** no equivalent gap exists there. `AddStrand`'s own `loadOrInitStateLocked` runs immediately after the pre-flight (`strand.go:390`), so it keeps both refusals with no extra call.

### shuttle-inherits-the-self-heal

- **Decision:** `internal/shuttleengine`'s `Runner.Start` (`run.go:252`) is a third self-healing entrypoint, and that is **intended**, not incidental. It is in scope behaviourally even though not one line of `shuttleengine` changes. This includes the standalone/detached runner path (`NewDetachedRunner`) — no carve-out.
- **Rationale:** `Runner.Start` calls `AddStrand` for every Go-launched agent spawn — loom, webster, burler, and standalone — so a shuttle run in a cold worktree now boots a session instead of failing with `no reed session`. That is precisely the headline case the roadmap item names ("any spawn OR view into a worktree nobody has visited just works"); a spawn reaching reed through `shuttleengine` rather than through `lyx reed add` is the same spawn. Carving standalone out would be worse, not safer: standalone is the mode *most* likely to meet a session nobody has started, since `lyx reed up` is hub-only and cannot reach standalone's derived geometry (`docs/overview.md`'s webster entry), which is why `webster run` boots its own private reed session in-process today.
- **Consequences a plan writer must not narrow away:** the change is not "two CLI verbs". Every caller of `Engine.AddStrand` inherits it, and `shuttleengine` has the widest blast radius of the three. This is also why the Testing section's regression surface is repo-wide rather than scoped to reed's own packages.
- **Rejected:** (a) gating the self-heal on a flag so only the two CLI verbs get it — reintroduces the two-step ritual for exactly the automated spawns that most need it, and adds a knob with no caller who would set it differently; (b) excluding `NewDetachedRunner` — see above, it is the mode with the strongest need.

### cold-path-converges-twice-and-that-is-fine

- **Decision:** on the **cold** `add` path, `upLocked` converges (header pane, reconcile, apply, persist) and then `AddStrand`'s own tail reconciles and applies again. Accepted, not optimised away.
- **Rationale:** it is exactly what an operator typing `lyx reed up && lyx reed add` gets today, and the second apply is the one that places the newly added strand. Collapsing them would mean threading a "skip the converge" flag through `upLocked`, which buys one avoided tmux round trip on a path that has just spawned a server.

### loom-run-handover-untouched

- **Decision:** `internal/loomcli/run.go:228`'s `c.reed.Status()` pre-flight stays.
- **Rationale:** `loom run` calls `ensureStatusStrand` at `run.go:101` (and `step.go:175`), which already calls `c.reed.Up()` first (`sharedbootstrap.go:185`). The session is therefore never cold by the time step 7's handover runs, so changing it buys nothing and touches the CLI/Cobra Invariant's interactive-handoff exception for no reason.

### redundant-up-sites-kept

- **Decision:** keep all three now-redundant explicit boots — `ensureStatusStrand`'s `c.reed.Up()` (`sharedbootstrap.go:185`), `drive.go:88`'s pre-producer `c.reed.Up()`, and the generated `.vscode/tasks.json` `reed up` row. Only the doc comments that now misdescribe the mechanism are updated.
- **Rationale:** all three become redundant, none becomes wrong. `ensureStatusStrand`'s `Up()` is the call whose `UpResult` and explicit ordering make loom's bootstrap legible; removing it would hide the boot inside `AddStrand`. `drive.go:88` is the same shape for a different reason — its own comment explains that `drive` adds no strand and hands no terminal over, so it is booting *ahead of* producers that will later spawn strands, precisely so a failure surfaces on the envelope at step 0 rather than several producers deep; the self-heal does not give it that early failure, so this one is not even redundant in the way the other two are and must stay. Removing the tasks.json row would only affect newly-generated worktrees while every existing one keeps the old chain, so the codebase would carry both shapes for no gain. `internal/vscode/config.go:70-74` does need editing: its safety argument is written in terms of "AddStrand pre-flights `requireSessionLocked` and attach pre-flights `Status`", which stops being true.
- **Rejected:** removing either — scope creep with no behavioural payoff.

### webster-standalone-boots-kept

- **Decision:** keep both of webster's standalone `c.reedUp()` calls — `internal/webstercli/run.go:104` and `internal/webstercli/recoverbatch.go:117-124` — and **rewrite both comments in the same commit**. Their code stays; their stated justification does not survive this task.
- **Rationale:** both exist because `lyx reed up` is hub-only and cannot reach standalone's derived geometry, so a standalone spawn had no recourse when it hit `no reed session`. The self-heal removes the dead end, which makes the *calls* redundant — but each comment asserts the now-false premise in so many words (`run.go`: "pre-fix the spawn died on `no reed session` with an impossible recourse"; `recoverbatch.go`: "SPAWNS a cold recovery strand through `reed.AddStrand`, which **requires a live session**"). A comment that misstates why its code exists is worse than a redundant call, and it is exactly what the next reader will trust. Keeping the calls follows the same reasoning as `redundant-up-sites-kept`: an explicit, early, envelope-reportable boot is more legible than an implicit one buried inside a later `AddStrand`, and `recoverbatch.go`'s call in particular is deliberately placed *after* its bad-batch/unparseable-plan/absent-run refusals so a rejected call boots no substrate — a property the self-heal does not provide, since `AddStrand`'s boot happens wherever `AddStrand` is called.
- **Rewrite both comments to say:** the call is now a deliberate early, explicit boot for envelope-reportable failure and placement control, not the only thing standing between a standalone spawn and a dead end.
- **Rejected:** (a) removing the calls — loses `recoverbatch.go`'s refuse-before-boot ordering, which is a real property, not incidental; (b) leaving the comments alone — the Documentation Lifecycle constraint covers comments that a change falsifies, and these are falsified verbatim.

### foreign-session-refusal-preserved

- **Decision:** rely on `ensureServerAndSessionLocked`'s own `refuseRecordedForeignSessionBeforeBootLocked` (`lifecycle.go:219`) for the foreign-session cases, and pin the outcome with a test. No extra refusal call is added to `AddStrand`.
- **Rationale:** this is the one genuine behavioural risk of the change. `requireSessionLocked` consults `refuseLiveForeignSessionLocked` on the *non-boot* path; the boot path has its own equivalent, placed deliberately ahead of anything that creates a session so a refusal never deposits a bare session as residue. Both routes into a foreign-session state (a worktree renamed while its session was up, a `.lyx` copied between worktrees of one hub — see `internal/reedcli/smoke_staterecovery_test.go`'s R5-F4/R5-F5 scenarios) leave *this* worktree's session absent, which is exactly the state that now boots instead of erroring. The refusal must still fire, and from `AddStrand` too.
- **Rejected:** an explicit pre-boot refusal inside `AddStrand` — duplicates a check the boot path already runs at the structurally correct point.

### config-errors-still-precede-the-boot

- **Decision:** `validateIfAbsent(spec)` stays the first statement in `AddStrand`'s closure, ahead of the boot. Inside `upLocked`, `ensureServerAndSessionLocked`'s existing pre-tmux validation block (`debugLogArgs`, `mouseOption`, `watchdogOption`, `ValidateHeader`, then `probeCapabilityLocked`) is untouched.
- **Rationale:** a **config-shaped** rejection must never boot a session as residue. `AddStrand`'s existing comment already states this for `validateIfAbsent` ("a rejected call must never deposit a friendly no-session error over what is actually a missing `--name`"); the same argument now covers a stronger consequence — a spawned tmux server.
- **Scope of the claim:** this covers config errors only. It deliberately does **not** claim that *every* rejection precedes the boot — a corrupt `reed.json` does not, by the decision below.

### corrupt-state-boots-then-fails

- **Decision:** on a cold worktree whose `reed.json` is unreadable, the self-healing `AddStrand`/`attach` boot the session first and *then* fail with `LoadState`'s corrupt-file diagnosis — leaving a bare session behind. No pre-boot readability refusal is added. This matches `lyx reed up`'s behaviour today, exactly.
- **Rationale:** this is a real regression in *residue posture* for these two verbs and is accepted knowingly. Today `AddStrand` reaches `requireSessionLocked`, whose `LoadState` error yields `noSessionMessage(0, false)`'s "reed's persisted state could not be read" text and spawns nothing (`lifecycle.go:1147-1157`). After the change the path is `ensureServerAndSessionLocked` → `refuseRecordedForeignSessionBeforeBootLocked`, which returns `nil` on a `LoadState` error by explicit design (`generation.go:170-175`, whose doc comment states "A corrupt or absent state file is not this check's business") → server spawn → `loadOrInitStateLocked` fails (`spawn.go:195-201`). Accepting it is the coherent choice because the whole premise of this task is "the self-healing verbs do what `up` does": `up` already boots-then-fails on a corrupt state file, so fixing it for `add`/`attach` alone would make the two paths diverge on exactly the axis this task is unifying. The operator-facing remedy is unchanged — `LoadState`'s error already names the file and points at `lyx reed down`, which clears both the corrupt file and the bare session.
- **Rejected:** (a) a pre-boot readability refusal inside `upLocked` or at the two self-heal call sites — diverges `add`/`attach` from `up`, adds a third `LoadState` read to the boot path, and contradicts `refuseRecordedForeignSessionBeforeBootLocked`'s documented scope; (b) widening `refuseRecordedForeignSessionBeforeBootLocked` to refuse on a load error — that changes `up` and `resume` too, which is a separate task about the booting verbs' own residue posture, not this one. If the residue is later judged unacceptable, the fix belongs at that shared helper so all four verbs move together.
- **Pinned by:** the "Cold add on a corrupt `reed.json`" smoke test in the Testing section, which asserts the failure text *and* the resulting residue, so the behaviour is recorded rather than discovered later.

### booted-is-the-helpers-return-value

- **Decision:** the boot signal is `ensureSessionLocked`/`EnsureSession`'s own `booted bool` return. `UpResult` is **not** changed — no `Booted` field, no shape change at all.
- **Rationale:** the helper the CLI already has to call is the natural carrier, so no extra surface is needed anywhere. This supersedes an earlier decision in this file that added `UpResult.Booted`; that field existed only because the self-heal was routed through `Up()`, and the `ensure-session-is-the-seam` decision removed that routing. `reedcli`'s `up` verb therefore keeps emitting exactly the envelope keys it emits today.
- **Rejected:** (a) `UpResult.Booted` — superseded, see above; (b) a second `has-session` round trip at the CLI site to infer it — a race, and a wasted tmux call.

### booted-means-actually-created

- **Decision:** `booted` means **"a session was actually created"**, never "the cold branch was taken". `upLocked` returns `(UpResult, booted bool, error)`, passing `ensureServerAndSessionLocked`'s own `booted` return straight out; `Up()` discards the bool; `ensureSessionLocked` returns it verbatim rather than hardcoding `true` on the delegate path.
- **Rationale:** the two are not the same. `ensureSessionLocked` probes, finds no live session, and delegates — but `ensureServerAndSessionLocked` probes again and can find the session *up by then*, returning `booted == false` (`lifecycle.go:228-245`). A sibling process racing the same worktree's boot is the realistic trigger; the op lock serialises reed's own callers, but nothing stops an operator's own `lyx reed up` in another terminal. Returning a hardcoded `true` would make the attribution `Info` line claim a boot that never happened — a log that lies about a spawn is worse than no log, and this is the one thing the line exists to report.
- **Note on the signature:** `upLocked` is unexported, so widening its return breaks no contract and leaves `UpResult`, `Up()` and this task's "exactly one new exported method" boundary untouched.
- **Rejected:** defining `booted` as "took the cold branch" — cheaper, and wrong for the only consumer it has.

### selfheal-observability

- **Decision:** each self-heal site logs once at `Info` when its `EnsureSession`/`ensureSessionLocked` call returned `booted == true`, naming the verb that caused the boot. No log on the warm path.
- **Rationale — legibility, not invariant compliance.** CONSTRAINTS.md's Live-Substrate Spawn Observability invariant is **already satisfied** without any new log: `lifecycle.go:331` logs `"reed: spawned tmux server"` at `Info` inside the spawn closure that every boot goes through, and that line is reached identically whether the caller is `Up`, `Resume`, or a self-healing `AddStrand`/`attach`. So this decision adds nothing the invariant demands. What it adds is attribution: that existing line records *that a server was spawned*, never *which verb asked for it*, and after this change a session can appear out of a bare `lyx reed add` or `lyx reed attach` with nothing in the log tying the two together. One `Info` line per self-heal closes that gap for anyone reading a log after the fact.
- **Rejected:** (a) no logging — the invariant permits it, but it leaves an unattributed session boot in the log; (b) logging unconditionally — noise on every warm `reed add`/`reed attach`; (c) moving the log inside `upLocked` or `ensureSessionLocked` — either would also fire for the plain `lyx reed up` verb, which is exactly the case that needs no attribution.

## Technical context

Everything in scope is inside `internal/reedengine`, `internal/reedcli`, and doc/comment sites.

Key files and line anchors as of this branch's HEAD:

- `internal/reedengine/lifecycle.go:163` `ensureServerAndSessionLocked` — validates config pre-tmux, probes capability, refuses a recorded foreign session, reaps a stale socket-holder, prunes server logs, spawns. Returns `(booted bool, strippedKeys []string, err error)`.
- `internal/reedengine/lifecycle.go:650` `Up()` — the body to extract. Note the `if booted { clearAllPaneBindings(st); st.StrippedEnv = stripped; st.HeaderPaneID = "" }` block and its comment warning that the `HeaderPaneID` clear deliberately lives here and not inside `clearAllPaneBindings`.
- `internal/reedengine/lifecycle.go:702` `Resume()` — the sibling with the same booted-bookkeeping block; **do not** fold the two into one shared helper as a drive-by, they diverge after `ensureHeaderPaneLocked`.
- `internal/reedengine/lifecycle.go:1109` `noSessionMessage` / `:1134` `requireSessionLocked` — stay, still called from eight other sites once `AddStrand` stops calling it: `Status` (`lifecycle.go:1166`), `SendText`/`SendKey`/`CapturePane` (`io.go:81`, `:111`, `:137`), `reapplyLayout` (`reapply.go:128`), `UpdateStrand` (`strand.go:484`), `RemoveStrand` (`strand.go:594`), and `AttachArgv` (`attach.go:84`, degrade-only).
- `internal/reedengine/strand.go:376` `AddStrand` — the call site; its doc comment's "Pre-flights the session's existence (mirroring Status)" sentence must be rewritten.
- `internal/reedengine/strand.go:481` `UpdateStrand` and `RemoveStrand` — their doc comments say "like AddStrand" about the pre-flight; those cross-references go stale and must be re-pointed at `Status` instead.
- `internal/reedengine/attach.go:74` `AttachArgv` — unchanged code, but its header comment and doc comment describe the pre-flight world and need a sentence acknowledging that its caller now boots first.
- `internal/reedengine/lock.go:89` `withOpLock` — the non-reentrancy that forces the `upLocked` extraction.
- `internal/reedengine/generation.go:170` `refuseRecordedForeignSessionBeforeBootLocked` — returns `nil` on a `LoadState` error by documented design, which is what makes the corrupt-state path boot-then-fail. Its doc comment already states the reasoning; do not change it.
- `internal/reedengine/spawn.go:195` `loadOrInitStateLocked` — where the corrupt-state failure actually lands, after the boot.
- `internal/reedengine/lifecycle.go:331` — the pre-existing `Info` spawn log that already satisfies the Live-Substrate Spawn Observability invariant.
- `internal/reedengine/apply.go` — `applyLayoutLocked`'s two skip guards (`len(live) < 2`, `anyPlacedStrand`), reproduced in `AttachArgv`. Untouched by this task.
- `internal/reedengine/reconcile.go:127-134` `planReconcile` — the untracked-pane reap. With an alive header, every live non-exempt pane goes into `untrackedPanesToKill`. This is the single most important reason the warm path must not be routed through `upLocked`; read it before touching the seam.
- `internal/reedengine/lifecycle.go:164-212` — `ensureServerAndSessionLocked`'s pre-tmux validation block, which runs *before* the already-up early return at `:228-245`. The second reason the warm path must not be routed through `upLocked`.
- `internal/reedengine/lifecycle.go:228-247` — the already-up / zero-pane-husk branch. `ensureSessionLocked`'s early-return predicate must be this same condition, shared rather than copied.
- `internal/loomcli/drive.go:88` — the third `c.reed.Up()`, kept (see `redundant-up-sites-kept`).
- `internal/reedcli/attach.go:55` — `EnsureSession()` is inserted **ahead of** the retained `Status()` call; nothing is swapped out. The file header comment names `c.eng.Status()` as "the only pre-flight step that can abort with an envelope error" and must be updated, since there are now two.
- `internal/vscode/config.go:70-74` — the tasks.json `dependsOrder` rationale comment.
- `internal/reedengine/spawn.go:210-212` — `loadOrInitStateLocked`'s doc comment **enumerates its callers by route**: "Every call site reaches here with the told session already up — Up/Resume via `ensureServerAndSessionLocked`, every other op via `requireSessionLocked`". `AddStrand` now arrives via `ensureSessionLocked`, so that list is wrong. In scope, same commit.
- `internal/reedengine/doc.go` — the package doc is long and pins reed's contracts in prose. At minimum `:207` (the `requireSessionLocked and never re-boots` sentence) and the surrounding lifecycle grammar need revisiting; **Widen the grep past `doc.go`**: search all of `internal/reedengine` (production files, not just `doc.go`) for `requireSessionLocked`, `AddStrand` and `AttachArgv`, and reconcile every comment hit. `spawn.go`'s route enumeration above is the known one this wider sweep catches and a `doc.go`-only sweep misses; assume there are others.

Gotchas:

- `ensureServerAndSessionLocked`'s already-up early return (`lifecycle.go:241-245`) is what makes the warm path cheap. It returns `false, nil, nil` *only* when the session has ≥1 pane; a zero-pane husk is killed and re-booted. Self-heal inherits that husk repair for free.
- `upLocked` must not be called before `validateIfAbsent` in `AddStrand`.
- `AddStrand`'s `--if-absent` branch lists panes and classifies against `st.Strands`. After a self-heal boot every binding has just been cleared, so `classifyIfAbsent` correctly reaches `ifAbsentRelaunch` (not `ifAbsentNoOpAlive`) for a name that exists in the persisted table — that is the right answer and needs a test, because it is the one place where up-semantics and `--if-absent` interact.
- The `.vscode/tasks.json` chain runs `reed add --if-absent` immediately after `reed up`; with the self-heal, the `reed up` row failing no longer means "no strand, no pane, no bare claude" — `reed add` will attempt its own boot and fail the same way. The net operator outcome is unchanged (still no strand), but the comment's reasoning is not.
- `internal/shuttleengine/reed.go:19` declares a `reed` interface with `AddStrand`, and `internal/shuttleengine/run.go:252` (`Runner.Start`) calls it on **every** Go-launched agent spawn. The signature is unchanged so no seam breaks — but this is a third behavioural entrypoint, not just a compile-compatibility note; see the `shuttle-inherits-the-self-heal` decision.

## Constraints

From `CONSTRAINTS.md`:

- **Live-Substrate Spawn Observability** — **already satisfied, no new obligation.** `AddStrand` and the attach pre-flight now reach a real tmux-server spawn, but that spawn is `lifecycle.go:331`, which already logs at `Info` via `internal/logger` on every path into it. The per-verb self-heal log this task adds is attribution, not compliance — see the `selfheal-observability` decision. mill-plan must not treat it as invariant-mandated and must not weaken `lifecycle.go:331`.
- **Told-Geometry Invariant** — `reedengine` is a bound package: it is handed its paths and imports no `internal/lyxcwd`. Nothing here derives geometry, so this is satisfied by not regressing it.
- **CLI / Cobra Invariant** — `reed attach` is a registered interactive-handoff exception: every fallible step runs pre-flight on the JSON envelope, and only the stdio-handover tail is exempt. Adding `EnsureSession()` ahead of the existing `Status()` keeps the pre-flight fallible and adds a second fallible step to it; both must stay *before* `exec.Command`/`attach.Run()`, and neither may be moved into the handover tail.
- **Test Tier Purity Invariant** — no `exec.Command` in untagged test files. Any test that touches a real tmux server must carry a `smoke` or `integration` build tag.
- **Documentation Lifecycle** — docs land in the same commit (see Scope).
- **Sandbox Suite Coverage** — **two suites assert the refusal this task deletes, and both must be edited in the same commit.** This is not a "check whether anything needs updating" item; the hits are known:
  - `tools/sandbox/SANDBOX-REED-SUITE.md`, scenario **M1 "Pre-up ergonomics"** (`:106-109`). It names `lyx reed add --cmd ...` failing with `no reed session; run "lyx reed up"` as the **`OK` outcome**. After this task `add` succeeds there, so M1 as written would report a defect for correct behaviour. M1 covers **two** verbs in one Watch line and they now diverge — `remove <guid>` still refuses, `add` no longer does — so **M1 must be split, not merely reworded**: one scenario for the verbs that still refuse, one for the self-healing `add` whose `OK` outcome is now "a session comes up and the strand is added".
  - `tools/sandbox/SANDBOX-WEBSTER-SUITE.md` (`:29-30`), prerequisite 5 "`lyx reed up` before any spawn", which states "without it the spawn fails loud with `no reed session; run \"lyx reed up\"`". Falsified by `shuttle-inherits-the-self-heal` — `webster run` spawns Master through shuttle, which reaches `AddStrand`. Rewrite the prerequisite; do not simply delete it, since an operator may still want to boot explicitly.
  - `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md` (`:178`, scenario S5). Its recovery step says to start a second `lyx shuttle run` with `reed.json` still absent, parenthesised as "it will fail at `add strand: no reed session` — that is expected and **costs no tokens**". Falsified by `shuttle-inherits-the-self-heal`, and the token claim *inverts*: that second run now boots a session and spawns a real agent. The step's actual purpose — confirming the first run's directory under `.lyx/shuttle/` survives — must be preserved while its mechanism is replaced, since an operator following it verbatim would burn tokens the suite promises they will not. Rewrite the step and drop or re-scope the "costs no tokens" parenthetical.
  - **Sweep method:** grep `tools/sandbox/*SUITE.md` for `no reed session` and for `reed up` before finalising, rather than trusting this three-item list — a suite added between now and implementation would otherwise be missed.

From `manifest/designs/worktree-lifecycle-shed-producers.md`: `fabricengine` must never import `reedengine` and vice versa. This change keeps the self-heal entirely inside reed, so that boundary is untouched — which is the roadmap item's "Fully internal to `reedengine` — fabric never needs to know reed exists."

## Testing

**`internal/reedengine` — hermetic (untagged, no tmux contact).**
The existing hermetic tests in this package work by asserting *validation order* — they run against an engine whose tmux binary does not exist, and assert the error is the config error rather than the binary's (see `TestUp_BadHeaderTemplateFailsBeforeAnyTmuxContact`, `lifecycle_test.go:32`). Follow that shape:

- `AddStrand` with an invalid `--if-absent` spec (no name) fails with the `validateIfAbsent` error, not a tmux error — pins that config rejection still precedes any tmux contact at all.
- `AddStrand` on a cold engine no longer returns `noSessionMessage`'s text — a negative assertion on the error string, pinning the regression this task removes.
- `planUpLaunches`/`planResumeLaunches` are untouched; their existing tests must still pass unmodified.

**Note on what is *not* hermetically reachable any more.** `ensureSessionLocked`'s first act is a `has-session` round trip, so against a nonexistent tmux binary `AddStrand` now fails with the binary's error before reaching `ensureServerAndSessionLocked`'s config-validation block. A `TestUp_BadHeaderTemplateFailsBeforeAnyTmuxContact`-shaped test therefore **cannot** be written for `AddStrand`; do not attempt one and do not "fix" the ordering to make one possible — the liveness probe running first is the whole point of the `ensure-session-is-the-seam` decision. The existing `Up`/`Resume` tests of that shape are unaffected and must keep passing unmodified. Cold-path validation ordering is covered by the smoke tier instead (the "Warm attach survives a config error" test's cold half).

**`internal/reedcli` — `smoke`-tagged, real tmux.**
Follow `smoke_staterecovery_test.go`'s `hubforge`-built-hub + `RunCLIIn` pattern. TDD candidates:

- **Cold add.** A freshly forged worktree, never `up`ped, no `reed.json`: `RunCLIIn(cwd, out, []string{"add", ...})` exits 0, the envelope carries a `guid`, and `tmux -L <socket> list-sessions` afterwards names this worktree's session. This is the task's headline scenario.
- **Cold add builds the same substrate as `up` + `add`.** After the cold add, the session holds the header pane *and* the strand's pane — proving `upLocked`'s `ensureHeaderPaneLocked` ran, not just the bare boot.
- **Cold attach.** On a never-`up`ped worktree, `reed attach` with no controlling terminal must fail with tmux's own terminal error, **not** with a `no reed session` JSON envelope, and must leave the session existing on the socket afterwards. The distinction between those two failure modes is the whole assertion.
- **Foreign-session refusal survives.** Reuse the R5-F4 `.lyx`-copied-between-worktrees fixture: a cold `reed add` in the copy must still refuse with the foreign-session error and must **not** create a session on the socket. Pair it with the R5-F5 renamed-worktree fixture.
  **This extends an existing test rather than adding a sibling.** `internal/reedcli/smoke_staterecovery_test.go:299`, `TestSmokeDiagnosticVerbsNameTheOrphanSessionRatherThanPointingAtResume`, already drives exactly this: it renames a worktree with a live session, then loops `status`, `attach` and `add` against the renamed copy asserting each names the orphan session. Keep all three verbs and all three assertions — the refusal must survive for every one of them — but its framing comment is falsified for two of the three and must be rewritten: it currently says "Every verb that goes through `requireSessionLocked` rather than through a boot must report the same diagnosis", when `attach` and `add` now go through a boot path and still refuse, which is the stronger and more interesting property. Add the `.lyx`-copy (R5-F4) case and the no-session-created assertion into the same test or an adjacent one in that file; do **not** write a parallel test that duplicates the fixture.
  **Note on why this needed calling out:** this test falls outside both sweep rules stated elsewhere in this section — it asserts the foreign-session text rather than `no reed session`, and it lives in `internal/reedcli`, which the repo-wide `AddStrand` grep deliberately excludes. Neither sweep would have surfaced it.
- **Cold add with a persisted strand table and `--if-absent`.** `reed.json` names a strand, the server is dead: `reed add --if-absent --name X` must boot and *relaunch* X (one pane for X, not two entries under one name), and must not relaunch any *other* persisted strand. This is the up-semantics decision's observable consequence.
- **Cold add on a corrupt `reed.json`.** A never-`up`ped worktree whose `reed.json` is unparseable: `reed add` exits non-zero with `LoadState`'s corrupt-file diagnosis (naming the file and `lyx reed down`), **and** a session now exists on the socket. Both halves are the assertion — the second records the accepted residue of the `corrupt-state-boots-then-fails` decision, so a later change to that posture fails this test loudly instead of passing silently.
- **Warm attach does not reap an operator's pane.** `up`, `add` a strand, then create an untracked pane directly via tmux (`split-window` on the session, outside reed's bookkeeping), then run the attach verb's pre-flight path. The untracked pane must still be alive afterwards. This is the sharpest test in the suite: it fails loudly if the pre-flight is ever routed back through `upLocked`, whose `planReconcile` would kill that pane.
- **Warm attach survives a config error.** `up`, then set an invalid `mouse:`/`watchdog:` value in the worktree's reed config, then run the attach verb's pre-flight path against the still-healthy live session: it must **not** refuse. Pins that `ensureSessionLocked`'s early return precedes `ensureServerAndSessionLocked`'s validation block. The cold equivalent must still refuse — same bad config, no session — so assert both directions.
- **Warm attach writes no state.** `reed.json`'s mtime and contents are unchanged across a warm attach pre-flight.
- **Warm attach still refuses on a corrupt `reed.json`.** Live session, `reed.json` made unparseable underneath it: the attach verb must abort on the JSON envelope with `LoadState`'s diagnosis, exactly as today. Pins `attach-preflight-keeps-its-status-call` — this test fails if `Status()` is ever dropped from the pre-flight.
- **Warm attach still refuses a live foreign session.** Reuse the R5-F5 renamed-worktree fixture against a live session: the attach verb must abort on the envelope with the foreign-session refusal, not proceed to the handover.
- **Warm add unchanged.** `up` then `add` still yields one session, one header pane, one strand, and `add`'s own reconcile/apply tail is the only converge that runs — no doubled apply on the warm path.
- **Zero-pane husk is still repaired — best-effort, and droppable.** A session that exists but holds no panes must be killed and re-booted, not early-returned past. Pins that `ensureSessionLocked`'s predicate is "up **and** ≥1 pane", not bare `has-session`.
  **Construction is the open question, and mill-plan decides it at implementation time.** No existing fixture in the repo builds a zero-pane session: `lifecycle.go:229-247` reaches that state only via a layout string that reaped every pane, and `listPanes` returns an error rather than an empty slice when tmux cannot list at all. If a reliable construction is found (feeding `select-layout` a string enumerating no panes against a scratch socket is the likeliest route), write the smoke test. If it is not reliably constructible, **drop the test rather than weakening the predicate to fit** — the repair path itself is pre-existing and unchanged by this task, so the predicate's correctness is argued from `lifecycle.go:228-247`'s own documented reasoning and the shared-helper requirement, not from this test's existence. Leaving the predicate untested is acceptable; changing it to something testable is not.
- **Boot attribution** (in `internal/reedengine`, **`//go:build integration`-tagged**, *not* in the hermetic tier described above and not in `reedcli`). Assert `EnsureSession`'s `booted` return directly: `true` on a cold worktree, `false` against a live session with ≥1 pane. **Both halves need a real tmux server** — `ensureSessionLocked`'s first act is a `has-session` round trip, so even the cold half cannot run hermetically — so both land in the same tagged file; there is no cold/warm tier split to make. Follow `contract_integration_test.go`'s existing pattern (own scratch socket, self-skips when the configured binary is absent). That return value is the primitive; both `Info` lines are one-line derivatives of it, so pinning it covers both call sites without a log-capture seam in two packages.
  For `AddStrand`'s own log line specifically, `internal/reedengine` already has the capture seam — `captureLogOutput` (`logcapture_test.go`), which sets both `logger.SetOutput` and `logger.SetVerbosity(1)` because neither alone captures at `Info`. Use it. Do **not** replicate that helper into `internal/reedcli` for the attach line: the `booted` assertion above already pins the condition the line is guarded on, and a second copy of a logger-capture helper is more machinery than the assertion is worth.

**Regression surface — repo-wide, not reed-local.** The whole existing `internal/reedengine` and `internal/reedcli` suites (both tiers) must pass unchanged; any existing test asserting the `no reed session` refusal *from `AddStrand` or the attach verb specifically* is the behaviour being deliberately removed and should be converted, not deleted silently. Existing tests asserting the same refusal from `status`, `remove`, `reapply`, or the `io.go` ops must keep passing untouched — if one of those breaks, the change leaked past its scope.

But the surface does not stop at those two packages, and this is the part a plan writer is most likely to get wrong. **Any test anywhere that relies on `AddStrand` failing fast without spawning is now a test that boots a real tmux server** — and in an untagged file that is also a Test Tier Purity Invariant breach, not merely a behaviour change.

- **Enumeration method:** grep `_test.go` files repo-wide for `AddStrand` and for `reedengine.` construction, outside `internal/reedengine`/`internal/reedcli`. For each hit, decide whether it drives a *real* `reedengine.Engine` (affected) or a fake/stub implementing reed's interface (unaffected — `internal/shuttleengine/fakes_test.go` is the common case). Then check the file's build tag.
- **Known hit, already confirmed:** `internal/burlercli/wiring_test.go:356` (`TestWireStandalone_RunnerReachesPublicEntryPointWithoutToldPathError`). It is untagged, drives a real engine through `c.engine.Run`, and its own comment states the premise this task invalidates verbatim — "It reaches no live reed session (none was ever started, so `requireSessionLocked` fails fast) and spawns no process". After the change it would spawn one.
- **Disposition rule for every hit:** the test's *intent* decides. Where the point is the told-path/wiring assertion and reed's refusal is only the cheap way to stop early (which is exactly `wiring_test.go`'s case), rework it to reach that assertion without a live engine call, and update the comment that names `requireSessionLocked`. Never move an untagged test to the `smoke` tier just to let it spawn — that trades a purity breach for a slower tier-1 suite and hides the intent change.
- This is one finding about the *method*, not a file list: mill-plan runs the grep against the tree at implementation time, because the set can drift between now and then.

## Q&A log

- **Q:** Which helper should `AddStrand` call — the bare boot, or all of `Up()`'s work? **A:** [auto-pick] Extract `upLocked()` (Up's whole body minus `withOpLock`) and call it. **Why:** `withOpLock` is a non-reentrant flock, so the exported `Up()` cannot be nested; and bare `ensureServerAndSessionLocked` leaves no header pane, producing a substrate `lyx reed up` never produces.
- **Q:** On a cold worktree whose `reed.json` holds strands, should the self-heal use `up` or `resume` semantics? **A:** [auto-pick] `up` semantics — bare substrate, no relaunch. **Why:** the roadmap names `Up()`'s helper, and `reed add` resurrecting a whole prior session would be a side effect nobody asked for.
- **Q:** Where does attach's self-heal land — the engine's `AttachArgv`, or the CLI pre-flight? **A:** [auto-pick] The CLI pre-flight, not the engine builder. (The exact call was `c.eng.Up()` here, narrowed to `c.eng.EnsureSession()` in r3.) **Why:** the pre-flight is the actual refusal site; `AttachArgv` only degrades. Booting inside `AttachArgv` would break both its "never returns an error" and its "never calls `SaveState`" contracts, and a boot failure there could only be logged, never surfaced on the envelope before stdio handover.
- **Q:** Should `loom run`'s own attach handover (`run.go:228`) change too? **A:** [auto-pick] No. **Why:** `ensureStatusStrand` already calls `Up()` earlier in the same command, so that path is never cold.
- **Q:** Remove the now-redundant `reed up` sites — `ensureStatusStrand`'s `Up()` call and the generated VS Code task row? **A:** [auto-pick] Leave both; fix only the doc comments that now misdescribe the mechanism. **Why:** redundant is not wrong, and dropping the tasks.json row would leave existing worktrees on the old chain while new ones differ.
- **Q:** How is the foreign-session refusal preserved once a cold `AddStrand` boots instead of erroring? **A:** [auto-pick] Rely on `ensureServerAndSessionLocked`'s own `refuseRecordedForeignSessionBeforeBootLocked`, and pin it with tests on both the renamed-worktree and copied-`.lyx` fixtures. **Why:** that check is already placed ahead of anything that creates a session, precisely so a refusal leaves no residue; duplicating it in `AddStrand` would only risk divergence.
- **Q:** Should the other `requireSessionLocked` verbs (`Status`, `RemoveStrand`, `UpdateStrand`, `io.go`, `reapply.go`) self-heal too? **A:** [auto-pick] No — out of scope. **Why:** the roadmap names two entrypoints. `UpdateStrand`'s hidden→visible surface is the only near-miss and it is engine-API-only with no CLI verb in v1.
- **Q:** What test tiers? **A:** [auto-pick] Hermetic validation-order tests in `internal/reedengine`, plus a `smoke`-tagged real-tmux cold-worktree suite in `internal/reedcli`. **Why:** the package has no fake-tmux harness — hermetic tests here assert ordering against a nonexistent binary, and anything touching a real server must carry a build tag under the Test Tier Purity Invariant.
- **Q:** Does anything have to keep `validateIfAbsent` ahead of the boot? **A:** [auto-pick] Yes, it stays the first statement in `AddStrand`'s closure. **Why:** a config-shaped rejection must not spawn a tmux server as residue — the stronger form of the argument the existing comment already makes about the no-session error. The claim is scoped to config errors only; corrupt state is the documented exception.
- **Q:** (r2 gap) A cold worktree with an unreadable `reed.json` refuses pre-boot today, but the self-heal would boot then fail — which disposition? **A:** [auto-resolved] Accept boot-then-fail, matching `lyx reed up` exactly, and pin both the error text and the residue with a test. **Why:** `refuseRecordedForeignSessionBeforeBootLocked` returns `nil` on a load error by explicit design (`generation.go:170-175`), and `up` already boots-then-fails on corrupt state; refusing pre-boot for `add`/`attach` alone would diverge the very paths this task is unifying. If the residue is later judged unacceptable the fix belongs at that shared helper, so all four verbs move together.
- **Q:** (r2 gap) The attach self-heal lives in `reedcli` and can only call exported `Up()` — how does it see the `booted` signal? **A:** [auto-resolved] Originally: an additive `UpResult.Booted` field. **Superseded in r3** — the self-heal no longer goes through `Up()` at all, so the signal is `EnsureSession()`'s own `booted` return and `UpResult` is left untouched.
- **Q:** (r2 gap) Is the new self-heal log required by Live-Substrate Spawn Observability? **A:** [auto-resolved] No — the invariant is already satisfied by `lifecycle.go:331`. The log is kept, re-grounded on verb attribution. **Why:** the existing line records that a server was spawned, never which verb asked for it, so a session appearing out of a bare `reed add` is currently untraceable in the log. Keeping it costs one `Info` line; the decision no longer claims invariant compliance as its motive.
- **Q:** (r2 gap) A warm `reed attach` would now write `reed.json` and apply a layout right before `AttachArgv` chains a second one — accept or avoid? **A:** [auto-resolved] Originally: accept, priced as a re-tile plus a state write. **Superseded in r3** — avoid, via a boot-only `EnsureSession` seam.
- **Q:** (r3 gap) The r2 answer claimed neither apply could reap a pane — is that right? **A:** [auto-resolved] No, it was wrong, and this is what forced the seam rework. `upLocked`'s tail reaches `planReconcile`, which adds every live non-exempt pane to `untrackedPanesToKill` whenever the header is alive (`reconcile.go:127-134`) — so a warm attach through `Up()` would have destroyed a pane the operator hand-split, on a verb that is read-only today. **Why the new answer:** an `ensureSessionLocked` that early-returns on "session up with ≥1 pane" never reaches that tail at all, which is strictly better than dispositioning the damage.
- **Q:** (r3 gap) `ensureServerAndSessionLocked` validates config *before* its already-up early return — so would a warm `reed attach` start refusing on a `mouse:` typo? **A:** [auto-resolved] Under the r2 design, yes — an operator locked out of viewing a healthy live session by a config error unrelated to viewing it. The `EnsureSession` seam removes it: liveness is probed first, so when there is nothing to boot there is nothing to validate. Pinned in both directions (warm must not refuse, cold must).
- **Q:** (r3) Does warm `reed add` gain a duplicated converge too? **A:** [auto-resolved] Under the r2 design it would have; under the `EnsureSession` seam it does not — the pre-flight is a liveness probe and `add`'s own `reconcileApplyPersistLocked` tail stays the only converge. Note `add` already reaps untracked panes today via that tail, so nothing changes for it either way; `attach` was the verb at risk.
- **Q:** (r3) Why probe "up **and** ≥1 pane" rather than bare `has-session`? **A:** [auto-resolved] A session with zero panes is broken substrate `add` can never use; `ensureServerAndSessionLocked` kills and re-boots that husk (`lifecycle.go:228-247`). A bare `has-session` early return would skip the repair and leave `add` failing forever. The predicate is shared between the two sites, never copied.
- **Q:** (r2) Is `drive.go:88`'s `c.reed.Up()` also redundant now? **A:** [auto-resolved] It is named in the "Out" list and kept — and it is the least redundant of the three. **Why:** `drive` adds no strand and hands no terminal over, so it boots *ahead of* the producers that will spawn strands, to fail on the envelope at step 0 rather than several producers deep. The self-heal does not give it that early failure.
- **Q:** (r4 gap) Would replacing attach's `Status()` with `EnsureSession()` drop refusals? **A:** [auto-resolved] Yes — `Status()` reaches `loadOrInitStateLocked`, so today's warm attach refuses on a corrupt `reed.json` and on a live foreign-session generation collision; `EnsureSession`'s early return reads no state, and `AttachArgv` degrades rather than erroring. Resolved by **keeping** `Status()` and running `EnsureSession()` ahead of it. **Why:** that makes the change strictly additive — every refusal attach performs today it still performs — at the cost of two probe round trips on the warm path. Order matters: `Status()` first would refuse before anything booted.
- **Q:** (r4) Is the regression surface really just `reedengine` + `reedcli`? **A:** [auto-resolved] No. Any test relying on `AddStrand` failing fast now spawns a tmux server, and in an untagged file that breaches Test Tier Purity. `internal/burlercli/wiring_test.go:356` is a confirmed hit whose own comment states the invalidated premise. The Testing section now carries the enumeration method (grep `_test.go` repo-wide for `AddStrand`, keep the real-engine hits, check build tags) and a disposition rule — rework by intent, never retag an untagged test just to let it spawn.
- **Q:** (r4) How does a test observe the attribution log? **A:** [auto-resolved] Assert `EnsureSession`'s `booted` return directly in `internal/reedengine`; use the existing `captureLogOutput` helper (`logcapture_test.go`) for `AddStrand`'s line only. **Why:** `booted` is the primitive both log lines derive from, and duplicating a logger-capture helper into `reedcli` costs more than the assertion is worth.
- **Q:** (r4) Is the warm-path cost really "+2 round trips" for both verbs? **A:** [auto-resolved] No — `add` gains one (`requireSessionLocked` already called `has-session`), `attach` gains two. Stated per verb now.
- **Q:** (r6) Are webster's two standalone `reedUp` boots in or out? **A:** [auto-resolved] The calls stay, both comments get rewritten in the same commit. **Why:** each comment asserts the now-false premise verbatim, and `recoverbatch.go`'s call is deliberately placed after its own refusals so a rejected call boots nothing — an ordering property the self-heal does not provide, since `AddStrand`'s boot happens wherever `AddStrand` is called.
- **Q:** (r6) Does the sandbox suite need changing after all? **A:** [auto-resolved] Yes — the earlier "not required to change" was wrong. `SANDBOX-REED-SUITE.md`'s M1 names the deleted refusal as its `OK` outcome and must be **split** (its `remove` half still refuses, its `add` half no longer does), and `SANDBOX-WEBSTER-SUITE.md`'s prerequisite 5 is falsified by the shuttle path. Both named, plus a grep sweep so a suite added later is not missed.
- **Q:** (r6) Which tier does the boot-attribution test belong to? **A:** [auto-resolved] `//go:build integration` in `internal/reedengine`, following `contract_integration_test.go`. **Why:** `ensureSessionLocked` opens with a `has-session` round trip, so **both** halves need a real tmux server — there is no hermetic half to split off, and writing it untagged would breach Test Tier Purity.
- **Q:** (r6) Does `booted` mean "took the cold branch" or "a session was created"? **A:** [auto-resolved] A session was actually created. `upLocked` widens to `(UpResult, bool, error)` and passes `ensureServerAndSessionLocked`'s own flag out; `ensureSessionLocked` returns it verbatim. **Why:** the session can come up between the two probes, so a hardcoded `true` would make the attribution log claim a spawn that never happened. `upLocked` is unexported, so the wider signature costs no contract.
- **Q:** (r7) Does `loadOrInitStateLocked`'s doc comment go stale? **A:** [auto-resolved] Yes — `spawn.go:210-212` enumerates its callers by route ("every other op via `requireSessionLocked`") and `AddStrand` now arrives via `ensureSessionLocked`. Named as an in-scope comment edit, and the doc grep widened from `doc.go` alone to all of `internal/reedengine`'s production files.
- **Q:** (r7) Is the foreign-session test new or an extension? **A:** [auto-resolved] An extension of `smoke_staterecovery_test.go:299`, which already loops `status`/`attach`/`add` over the renamed-worktree fixture. Keep all three verbs; rewrite its framing comment, which says these verbs go through `requireSessionLocked` "rather than through a boot" — now false for two of them, and the surviving refusal is the stronger property. **Why it needed naming:** the test matches neither stated sweep — it asserts the foreign-session text, not `no reed session`, and lives in `reedcli`, which the repo-wide `AddStrand` grep excludes.
- **Q:** (r7) Is the sandbox hit list complete at two? **A:** [auto-resolved] No — `SANDBOX-SHUTTLE-SUITE.md:178` (S5) is a third, and the worst of them: it tells the operator a second run "will fail at `add strand: no reed session` — that is expected and costs no tokens", which now inverts into booting a session and spawning a real agent. The step's purpose (the first run's directory must survive) is preserved; its mechanism and the token claim are rewritten.


### From _mill/plan/00-overview.md


```yaml
task: 'reed: AddStrand and attach self-heal a cold worktree'
slug: 'reed-cold-worktree-selfheal'
approved: true
started: '20260919-050951'
parent: 'main'
root: ""
verify: null
discussion_sha: b8bbaa85a790e4055f1193a11329bb24a9984a0e
```

### From _mill/plan/01-engine-seam.md


```yaml
task: 'reed: AddStrand and attach self-heal a cold worktree'
batch: 'engine-seam'
number: 1
cards: 4
verify: go test ./internal/reedengine/
depends-on: []
```



- **Edits:**
  - `internal/reedengine/lifecycle.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/reedengine/lifecycle.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/reedengine/strand.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/reedengine/strand_test.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/02-attach-preflight.md


```yaml
task: 'reed: AddStrand and attach self-heal a cold worktree'
batch: 'attach-preflight'
number: 2
cards: 1
verify: go test ./internal/reedcli/ ./internal/reedengine/
depends-on: [1]
```



- **Edits:**
  - `internal/reedcli/attach.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/03-comment-sweep.md


```yaml
task: 'reed: AddStrand and attach self-heal a cold worktree'
batch: 'comment-sweep'
number: 3
cards: 3
verify: go test ./internal/reedengine/ ./internal/burlercli/ ./internal/webstercli/ ./internal/vscode/ ./cmd/lyx/
depends-on: [1, 2]
```



- **Edits:**
  - `internal/reedengine/doc.go`
  - `internal/reedengine/spawn.go`
  - `internal/reedengine/strand.go`
  - `internal/reedengine/attach.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/vscode/config.go`
  - `internal/webstercli/run.go`
  - `internal/webstercli/recoverbatch.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/burlercli/wiring_test.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/04-docs-and-suites.md


```yaml
task: 'reed: AddStrand and attach self-heal a cold worktree'
batch: 'docs-and-suites'
number: 4
cards: 2
verify: go test ./internal/lyxcwd/ ./cmd/lyx/
depends-on: [1, 2]
```



- **Edits:**
  - `docs/overview.md`
  - `manifest/roadmap.md`
  - `manifest/designs/worktree-lifecycle-shed-producers.md`
  - `manifest/designs/reed-fabric-standalone-api.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `tools/sandbox/SANDBOX-REED-SUITE.md`
  - `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
  - `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/05-tagged-tests.md


```yaml
task: 'reed: AddStrand and attach self-heal a cold worktree'
batch: 'tagged-tests'
number: 5
cards: 4
verify: go test -tags smoke ./internal/reedcli/ -skip '^TestSmokeClaudeResumeRecallsCodeword$' && go test -tags integration ./internal/reedengine/
depends-on: [1, 2]
```



- **Edits:** none
- **Creates:**
  - `internal/reedcli/smoke_coldstart_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/reedcli/smoke_warmpath_test.go`
- **Deletes:** none
- **Edits:**
  - `internal/reedcli/smoke_staterecovery_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/reedengine/ensuresession_integration_test.go`
- **Deletes:** none

## Conflicting files

- `docs/overview.md`
- `internal/reedengine/lifecycle.go`
- `manifest/designs/reed-fabric-standalone-api.md`
- `manifest/designs/worktree-lifecycle-shed-producers.md`
- `manifest/roadmap.md`

## Instructions

For each file listed above:

1. Read the file and locate every conflict block (`<<<<<<<`, `=======`, `>>>>>>>`).
2. Understand both sides of the conflict — what each branch intended.
3. Write a resolution that preserves the intent of both sides.
   When both sides modify **different, non-overlapping parts** of the same conflict region — for example, different columns of one table row, different keys of one object, or disjoint lines of a prose block — **combine both edits** into a single resolved structure.
   Do NOT pick one side wholesale just because the region overlaps syntactically;
   picking one side wholesale is correct only when the two changes are genuinely mutually exclusive (e.g. the same key is renamed to two different values).
   Worked example: if `ours` changes column A and `theirs` changes column B of the same table row, the resolution keeps both column changes in a single row — it does not discard either.
4. Before keeping content from either side inside a conflict hunk, search the rest of the file (outside the hunk) for that same content.
   This judgment call is scoped narrowly — it applies only when a hunk's content might be a moved duplicate of content living elsewhere in the file;
   it does NOT apply to every ordinary step-3 disjoint-region combine (e.g. the column-A/column-B worked example above), which remains today's silent, high-confidence success path.
   Two branches:
   - **Confident case:** if the content clearly already exists elsewhere and the surrounding context makes it unambiguous that this is the same item having been moved (not two independent, separately-intended copies) — do not re-add it in the hunk;
     keep only the other side's unrelated edit.
     Worked example: one side moves a roadmap item from `## Planned` to `## Done`, while the other side makes an unrelated edit elsewhere in the file.
     The resolution keeps the item only under `## Done`;
     it is not re-added under `## Planned`.
   - **Ambiguous case:** if you cannot confidently tell whether this is the same moved content or a legitimate independent duplication — fall back to step 3's default (keep both) rather than guessing, and report the ambiguity via the `discarded` field (see Report section) with the description `"kept both sides of a conflict, ambiguous move-vs-duplicate"`.
     Worked example: a similarly-worded item appears in two different sections and you cannot tell whether it is the same item moved or a legitimate second, independently-added item.
     The resolution keeps both occurrences and reports the ambiguity via `discarded`.
5. Run `git -C /home/knatte/Code/loomyard/wts/reed-cold-worktree-selfheal add <file>` to stage the resolved file.
6. For modify/delete (DU) conflicts: if Task intent above lists this file under a batch's `Deletes:`, run `git -C /home/knatte/Code/loomyard/wts/reed-cold-worktree-selfheal rm <file>` instead of editing;
   that stages the intentional deletion.
7. For UD conflicts — files this branch **modified** that the parent branch **deleted**: do not silently keep the modification.
   Instead: a. Run `git log --diff-filter=D --oneline MERGE_HEAD -- <file>` to find the deletion commit on the parent. b. Run `git show <deletion-commit>` to inspect context. c. If the deletion commit message mentions a replacement file (e.g. "replaced by", "moved to", "consolidated into"),
   or the commit also adds a file in the same directory with overlapping content: stage the deletion — `git -C /home/knatte/Code/loomyard/wts/reed-cold-worktree-selfheal rm <file>`. d. If detection is inconclusive: report `{"status":"stuck","stuck_type":"logic","reason":"modify/delete conflict on <file>: cannot determine if parent deletion is a replacement -- operator must decide"}` and halt.
   Do NOT silently keep the modification.
8. Before reporting `{"status":"success"}` (with or without `discarded`), re-read each file listed in Conflicting files in full and explicitly verify no contradictory losing-side claims survive the resolution — e.g. a stale value from one side of the conflict left alongside the correct value from the other side, or a claim that only made sense before the other side's edit was applied.
   If you find a contradiction you missed, fix it before reporting.
   If you find a contradiction you cannot confidently resolve, report `{"status":"stuck","stuck_type":"logic","reason":"self-verification found an unresolved contradiction in <file>: <description>"}` instead of `{"status":"success"}`.

Never use `git checkout --ours` or `git checkout --theirs` — they silently discard one side of the conflict.

## Report

Your last output line MUST be a bare JSON object (no code fence, no backticks):

On success (nothing discarded):

{"status":"success"}

On success with discarded content — if you had to drop content from one side (e.g. two sides made mutually exclusive changes and only one could survive), list each dropped item:

{"status":"success","discarded":["<short description of what was dropped from which side>"]}

An empty or absent `discarded` field means nothing was lost.
If anything was discarded, you MUST list it;
an empty list when content was actually dropped is a protocol violation. `discarded` also carries the step 4 ambiguous-case entry `"kept both sides of a conflict, ambiguous move-vs-duplicate"` — even though nothing was technically dropped in that case, the field's purpose is to surface anything the operator should double-check before `git merge --continue`, which covers both a genuine drop and a kept-both ambiguity.
The `mill-merge-in` frontend reads this field and surfaces any losses (or ambiguities) to the operator before continuing, rather than silently running `git merge --continue`.

If you cannot resolve one or more conflicts:

{"status":"stuck","stuck_type":"logic","reason":"<one-line description of what you could not resolve>"}

Anything other than this JSON object on the last line is a protocol violation;
the merge-in dispatcher treats that as stuck_type: logic with reason "no structured report" — your work is lost.
Do not wrap the JSON in a code fence;
do not add commentary after it.

## Tools

Available: Read, Edit, Write, Bash, Grep, Glob.
Use `git -C /home/knatte/Code/loomyard/wts/reed-cold-worktree-selfheal` for any git commands;
do not `cd`.
Worktree cwd is `/home/knatte/Code/loomyard/wts/reed-cold-worktree-selfheal`.
