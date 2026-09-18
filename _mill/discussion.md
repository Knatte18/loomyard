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

- `internal/reedengine/lifecycle.go` — extract the body of `Up()` into a new locked helper `upLocked() (UpResult, error)`; `Up()` becomes `withOpLock(upLocked)`. `UpResult` gains a `Booted bool` field.
- `internal/reedengine/strand.go` — `AddStrand` calls `e.upLocked()` in place of `e.requireSessionLocked()`, after `validateIfAbsent`.
- `internal/reedcli/attach.go` — the pre-flight switches from `c.eng.Status()` to `c.eng.Up()`.
- Observability: one `logger.Info` at each self-heal site when that call actually booted a session (`UpResult.Booted`).
- Doc updates in the same commit: `internal/reedengine/doc.go`, `AddStrand`'s and `AttachArgv`'s own doc comments, `internal/vscode/config.go`'s tasks.json rationale comment, `docs/overview.md`'s reed entry, `manifest/designs/worktree-lifecycle-shed-producers.md`'s "Today, without this item built yet" framing, and `manifest/roadmap.md` (Planned → Done).
- Tests: hermetic ordering/regression tests in `internal/reedengine`, plus a `smoke`-tagged cold-worktree test in `internal/reedcli`.

**Out:**

- `Engine.Status`, `Engine.RemoveStrand`, `Engine.UpdateStrand`, `internal/reedengine/io.go`'s three send/capture ops, and `internal/reedengine/reapply.go` keep `requireSessionLocked` exactly as-is. `requireSessionLocked` and `noSessionMessage` are not deleted or weakened.
- `internal/loomcli/run.go`'s own attach handover (`c.reed.Status()` at `run.go:228`) is unchanged.
- `internal/loomcli/sharedbootstrap.go:185`'s `ensureStatusStrand` and `internal/loomcli/drive.go:88`'s pre-producer bootstrap both keep their explicit `c.reed.Up()` call — see the `redundant-up-sites-kept` decision.
- The generated `.vscode/tasks.json` chain (`reed up` → `reed add --if-absent` → `reed attach`) keeps its `reed up` row.
- `Engine.AttachArgv` gains no boot: it stays a non-refusing, `reed.json`-read-only builder.
- No `Resume` semantics anywhere — this task never relaunches a persisted strand.
- The `born-as-strand` item (`manifest/designs/reed-born-as-strand.md`, `loom run`'s terminal handoff never calling `AddStrand`) is a different problem and stays untouched.
- No new exported engine **method**. `Up()` is already exported and is what the CLI calls. The one exported-surface change is additive: `UpResult` gains a `Booted bool` field (see the `booted-is-an-upresult-field` decision).
- No pre-boot readability check for a corrupt `reed.json` — see the `corrupt-state-boots-then-fails` decision.

## Decisions

### upLocked-extraction

- **Decision:** extract `Up()`'s closure body verbatim into `func (e *Engine) upLocked() (UpResult, error)`, leaving `Up()` as a `withOpLock` wrapper around it. `AddStrand` calls `upLocked()` from inside its own already-held `withOpLock` closure.
- **Rationale:** `withOpLock` acquires a `gofrs/flock` write lock on `.lyx/<reedLockFileName>` (`internal/reedengine/lock.go:89-114`) and is not reentrant, so `AddStrand` cannot call the exported `Up()`. Beyond the lock, the *whole* `Up()` body is what a cold worktree needs, not just the boot: on `booted == true` it clears every stale pane binding, stamps `StrippedEnv`, resets `HeaderPaneID`, then `ensureHeaderPaneLocked` builds the header pane and `reconcileApplyPersistLocked` applies the layout. Calling bare `ensureServerAndSessionLocked` would leave a session with no header pane, and `AddStrand` would then split its strand into a headerless window — a different substrate from the one `lyx reed up` produces.
- **Rejected:** (a) bare `ensureServerAndSessionLocked` — produces a substrate `reed up` never produces; (b) duplicating the booted-bookkeeping block at each call site — two copies of a comment block that already warns future editors not to "fix" it.

### up-semantics-not-resume

- **Decision:** the self-heal always has `up` semantics. A cold worktree whose `reed.json` records strands gets a bare substrate; those strands are **not** relaunched.
- **Rationale:** `reed add` must add one strand, not resurrect a whole prior session as a side effect. `noSessionMessage` deliberately distinguishes these two cases today and routes the operator to `resume` when there is content to rebuild; `resume` stays an explicit operator verb.
- **Rejected:** `resumeLocked` — surprising, and it would relaunch processes the caller never asked for.

### attach-heals-at-the-cli-preflight

- **Decision:** `internal/reedcli/attach.go`'s RunE replaces `if _, err := c.eng.Status(); err != nil` with `if _, err := c.eng.Up(); err != nil`. `Engine.AttachArgv` is not changed.
- **Rationale:** the attach verb's actual refusal site is that pre-flight, not the engine. `AttachArgv` never refuses by contract — its own `requireSessionLocked` (`internal/reedengine/attach.go:84`) only degrades to the bare `attach-session` argv, which then fails at the tmux layer. Making `AttachArgv` boot would break two of its documented properties at once: "returns no error, by contract" and "Read-only with respect to `reed.json`: this builder never calls `SaveState`" — `upLocked`'s `reconcileApplyPersistLocked` writes state. Booting in the pre-flight instead keeps the builder pure, keeps a boot failure (bad `header.template`, invalid `mouse`/`watchdog`/`debug_log`, a capability-probe failure, a recorded-foreign-session refusal) reportable on the JSON envelope *before* stdio is handed to the child, and means `AttachArgv`'s own `requireSessionLocked` passes by the time it runs.
- **Rejected:** (a) booting inside `AttachArgv` — side-effecting builder, and a boot failure could only be logged, never surfaced; (b) adding a new exported `Engine.EnsureSession()` — `Up()` already is exactly that; (c) calling `Status()` first and `Up()` only on its error, to leave the warm path byte-for-byte unchanged — rejected because `Status()`'s error is not classifiable without matching `noSessionMessage`'s text, so the retry would also fire on a corrupt `reed.json` and a foreign-session refusal, and reed exposes no cheap `HasSession()` predicate to gate it on instead. The warm-path cost is priced and accepted in the next decision.

### warm-attach-gains-one-apply-and-one-write

- **Decision:** accept that a **warm** `lyx reed attach` now reconciles, applies a layout, and writes `reed.json` in its pre-flight, immediately before `AttachArgv` computes and chains a second, client-sized `select-layout`. This is a deliberate behavioural change to a verb that was previously read-only, not an incidental one.
- **Rationale:** `Up()` is idempotent at the *boot* level — `ensureServerAndSessionLocked` early-returns `false, nil, nil` for an already-up session holding ≥1 pane (`lifecycle.go:241-245`) — but `upLocked`'s tail is not read-only: `ensureHeaderPaneLocked` then `reconcileApplyPersistLocked` run unconditionally. Three consequences, each accepted deliberately:
  1. **Two applies, not one.** The pre-flight apply is computed against the *live, pre-attach* window box; the chained one against the told client box (`attach.go:145`). The first is therefore at the wrong size for the incoming client and is superseded a moment later by the second. That is wasteful, not wrong — and it is exactly what an operator who typed `lyx reed up && lyx reed attach` already gets today.
  2. **Neither apply can reap a pane.** `select-layout` destroys panes absent from its string, which is why `applyLayoutLocked` and `AttachArgv` both carry the `len(live) < 2` / `anyPlacedStrand` guards. Both strings here are built from a fresh `listPanes` enumeration, so no live pane is ever absent from either. The guards stay untouched.
  3. **Other attached clients see one re-tile,** and `reed.json` gains a write on a verb that had none. Both are visible; neither is destructive.
- **Rejected:** suppressing the pre-flight apply for the warm case — that would mean either a second exported "boot only, don't converge" entry point (a new engine method this task explicitly does not add) or duplicating `upLocked`'s tail conditionally, and it would make `attach`'s substrate convergence differ from every other reed entrypoint's for no operator-visible gain. `manifest/designs/worktree-lifecycle-shed-producers.md` frames the self-heal as "ambient, self-healing behavior inside reed's own entrypoints, triggered by whoever is about to actually use the session" — attach *is* such a use, so converging the substrate there is coherent with the design, not a leak from it.
- **Pinned by:** the "Warm attach preserves panes, clients and bindings" smoke test in the Testing section.

### loom-run-handover-untouched

- **Decision:** `internal/loomcli/run.go:228`'s `c.reed.Status()` pre-flight stays.
- **Rationale:** `loom run` calls `ensureStatusStrand` at `run.go:101` (and `step.go:175`), which already calls `c.reed.Up()` first (`sharedbootstrap.go:185`). The session is therefore never cold by the time step 7's handover runs, so changing it buys nothing and touches the CLI/Cobra Invariant's interactive-handoff exception for no reason.

### redundant-up-sites-kept

- **Decision:** keep all three now-redundant explicit boots — `ensureStatusStrand`'s `c.reed.Up()` (`sharedbootstrap.go:185`), `drive.go:88`'s pre-producer `c.reed.Up()`, and the generated `.vscode/tasks.json` `reed up` row. Only the doc comments that now misdescribe the mechanism are updated.
- **Rationale:** all three become redundant, none becomes wrong. `ensureStatusStrand`'s `Up()` is the call whose `UpResult` and explicit ordering make loom's bootstrap legible; removing it would hide the boot inside `AddStrand`. `drive.go:88` is the same shape for a different reason — its own comment explains that `drive` adds no strand and hands no terminal over, so it is booting *ahead of* producers that will later spawn strands, precisely so a failure surfaces on the envelope at step 0 rather than several producers deep; the self-heal does not give it that early failure, so this one is not even redundant in the way the other two are and must stay. Removing the tasks.json row would only affect newly-generated worktrees while every existing one keeps the old chain, so the codebase would carry both shapes for no gain. `internal/vscode/config.go:70-74` does need editing: its safety argument is written in terms of "AddStrand pre-flights `requireSessionLocked` and attach pre-flights `Status`", which stops being true.
- **Rejected:** removing either — scope creep with no behavioural payoff.

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

### booted-is-an-upresult-field

- **Decision:** `UpResult` gains a `Booted bool` field. `upLocked` returns `(UpResult, error)` with `Booted` set from `ensureServerAndSessionLocked`'s own `booted` return; `Up()` returns that `UpResult` through unchanged. Both self-heal sites read `result.Booted` — `AddStrand` in-package, `internal/reedcli/attach.go` across the exported seam.
- **Rationale:** the attach self-heal lives in `internal/reedcli` and can reach nothing but exported API, so a boot signal returned only to in-package callers is unreachable there. An additive field on the existing result struct is the smallest seam that serves both sites: no new exported method, and `reedcli`'s `up` verb keeps emitting whatever envelope keys it emits today unless it is deliberately changed (it is not, in this task). This supersedes the earlier framing of keeping `UpResult`'s shape frozen — an additive field is the price of an exported boot signal, and it is cheaper than the alternatives.
- **Rejected:** (a) `upLocked` returning a bare extra `bool` that `Up()` discards — leaves the `reedcli` attach site with no signal at all; (b) a new exported `Engine.Booted()`/`EnsureSession()` method — more surface for one bool; (c) a second `has-session` round trip at the CLI site to infer it — a race, and a wasted tmux call.

### selfheal-observability

- **Decision:** each self-heal site logs once at `Info` when `result.Booted` is true, naming the verb that caused the boot. No log on the warm path.
- **Rationale — legibility, not invariant compliance.** CONSTRAINTS.md's Live-Substrate Spawn Observability invariant is **already satisfied** without any new log: `lifecycle.go:331` logs `"reed: spawned tmux server"` at `Info` inside the spawn closure that every boot goes through, and that line is reached identically whether the caller is `Up`, `Resume`, or a self-healing `AddStrand`/`attach`. So this decision adds nothing the invariant demands. What it adds is attribution: that existing line records *that a server was spawned*, never *which verb asked for it*, and after this change a session can appear out of a bare `lyx reed add` or `lyx reed attach` with nothing in the log tying the two together. One `Info` line per self-heal closes that gap for anyone reading a log after the fact.
- **Rejected:** (a) no logging — the invariant permits it, but it leaves an unattributed session boot in the log; (b) logging unconditionally — noise on every warm `reed add`/`reed attach`; (c) moving the log inside `upLocked` — it would then also fire for the plain `lyx reed up` verb, which is exactly the case that needs no attribution.

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
- `internal/reedengine/apply.go` — `applyLayoutLocked`'s two skip guards (`len(live) < 2`, `anyPlacedStrand`), reproduced in `AttachArgv`. Untouched by this task, but they are what keep the warm-attach double apply safe.
- `internal/loomcli/drive.go:88` — the third `c.reed.Up()`, kept (see `redundant-up-sites-kept`).
- `internal/reedcli/attach.go:55` — the `Status()` → `Up()` swap; the file header comment names `c.eng.Status()` as "the only pre-flight step that can abort with an envelope error" and must be updated.
- `internal/vscode/config.go:70-74` — the tasks.json `dependsOrder` rationale comment.
- `internal/reedengine/doc.go` — the package doc is long and pins reed's contracts in prose. At minimum `:207` (the `requireSessionLocked and never re-boots` sentence) and the surrounding lifecycle grammar need revisiting; mill-plan should grep `doc.go` for `requireSessionLocked`, `AddStrand`, and `AttachArgv` and reconcile every hit.

Gotchas:

- `ensureServerAndSessionLocked`'s already-up early return (`lifecycle.go:241-245`) is what makes the warm path cheap. It returns `false, nil, nil` *only* when the session has ≥1 pane; a zero-pane husk is killed and re-booted. Self-heal inherits that husk repair for free.
- `upLocked` must not be called before `validateIfAbsent` in `AddStrand`.
- `AddStrand`'s `--if-absent` branch lists panes and classifies against `st.Strands`. After a self-heal boot every binding has just been cleared, so `classifyIfAbsent` correctly reaches `ifAbsentRelaunch` (not `ifAbsentNoOpAlive`) for a name that exists in the persisted table — that is the right answer and needs a test, because it is the one place where up-semantics and `--if-absent` interact.
- The `.vscode/tasks.json` chain runs `reed add --if-absent` immediately after `reed up`; with the self-heal, the `reed up` row failing no longer means "no strand, no pane, no bare claude" — `reed add` will attempt its own boot and fail the same way. The net operator outcome is unchanged (still no strand), but the comment's reasoning is not.
- `internal/shuttleengine/reed.go:19` declares a `reed` interface with `AddStrand`; the signature is unchanged, so no seam breaks.

## Constraints

From `CONSTRAINTS.md`:

- **Live-Substrate Spawn Observability** — **already satisfied, no new obligation.** `AddStrand` and the attach pre-flight now reach a real tmux-server spawn, but that spawn is `lifecycle.go:331`, which already logs at `Info` via `internal/logger` on every path into it. The per-verb self-heal log this task adds is attribution, not compliance — see the `selfheal-observability` decision. mill-plan must not treat it as invariant-mandated and must not weaken `lifecycle.go:331`.
- **Told-Geometry Invariant** — `reedengine` is a bound package: it is handed its paths and imports no `internal/lyxcwd`. Nothing here derives geometry, so this is satisfied by not regressing it.
- **CLI / Cobra Invariant** — `reed attach` is a registered interactive-handoff exception: every fallible step runs pre-flight on the JSON envelope, and only the stdio-handover tail is exempt. Swapping `Status()` for `Up()` keeps a fallible pre-flight in place; it must stay *before* `exec.Command`/`attach.Run()`.
- **Test Tier Purity Invariant** — no `exec.Command` in untagged test files. Any test that touches a real tmux server must carry a `smoke` or `integration` build tag.
- **Documentation Lifecycle** — docs land in the same commit (see Scope).
- **Sandbox Suite Coverage** — reed is covered by `tools/sandbox/SANDBOX-REED-SUITE.md`. That suite is not required to change for this task; if mill-plan finds a scenario there that asserts the old "run `lyx reed up` first" refusal, it must be updated rather than left contradicting the new behaviour.

From `manifest/designs/worktree-lifecycle-shed-producers.md`: `fabricengine` must never import `reedengine` and vice versa. This change keeps the self-heal entirely inside reed, so that boundary is untouched — which is the roadmap item's "Fully internal to `reedengine` — fabric never needs to know reed exists."

## Testing

**`internal/reedengine` — hermetic (untagged, no tmux contact).**
The existing hermetic tests in this package work by asserting *validation order* — they run against an engine whose tmux binary does not exist, and assert the error is the config error rather than the binary's (see `TestUp_BadHeaderTemplateFailsBeforeAnyTmuxContact`, `lifecycle_test.go:32`). Follow that shape:

- `AddStrand` with an invalid `--if-absent` spec (no name) fails with the `validateIfAbsent` error, not a tmux/boot error — pins that config rejection still precedes the boot.
- `AddStrand` with a bad `header.template` on a cold engine fails with the stencil unfilled-marker error — pins that `AddStrand` now reaches `ensureServerAndSessionLocked`'s pre-tmux validation block at all, which is the cheapest hermetic proof the self-heal is wired.
- `AddStrand` on a cold engine no longer returns `noSessionMessage`'s text — a negative assertion on the error string, pinning the regression this task removes.
- `planUpLaunches`/`planResumeLaunches` are untouched; their existing tests must still pass unmodified.

**`internal/reedcli` — `smoke`-tagged, real tmux.**
Follow `smoke_staterecovery_test.go`'s `hubforge`-built-hub + `RunCLIIn` pattern. TDD candidates:

- **Cold add.** A freshly forged worktree, never `up`ped, no `reed.json`: `RunCLIIn(cwd, out, []string{"add", ...})` exits 0, the envelope carries a `guid`, and `tmux -L <socket> list-sessions` afterwards names this worktree's session. This is the task's headline scenario.
- **Cold add builds the same substrate as `up` + `add`.** After the cold add, the session holds the header pane *and* the strand's pane — proving `upLocked`'s `ensureHeaderPaneLocked` ran, not just the bare boot.
- **Cold attach.** On a never-`up`ped worktree, `reed attach` with no controlling terminal must fail with tmux's own terminal error, **not** with a `no reed session` JSON envelope, and must leave the session existing on the socket afterwards. The distinction between those two failure modes is the whole assertion.
- **Foreign-session refusal survives.** Reuse the R5-F4 `.lyx`-copied-between-worktrees fixture: a cold `reed add` in the copy must still refuse with the foreign-session error and must **not** create a session on the socket. Pair it with the R5-F5 renamed-worktree fixture.
- **Cold add with a persisted strand table and `--if-absent`.** `reed.json` names a strand, the server is dead: `reed add --if-absent --name X` must boot and *relaunch* X (one pane for X, not two entries under one name), and must not relaunch any *other* persisted strand. This is the up-semantics decision's observable consequence.
- **Cold add on a corrupt `reed.json`.** A never-`up`ped worktree whose `reed.json` is unparseable: `reed add` exits non-zero with `LoadState`'s corrupt-file diagnosis (naming the file and `lyx reed down`), **and** a session now exists on the socket. Both halves are the assertion — the second records the accepted residue of the `corrupt-state-boots-then-fails` decision, so a later change to that posture fails this test loudly instead of passing silently.
- **Warm attach preserves panes, clients and bindings.** `up`, `add` a strand, attach a second client, then run the attach verb's pre-flight path again: every pane alive before is alive after, the strand table's GUID→PaneID bindings are unchanged, and the strand count is unchanged. This pins the `warm-attach-gains-one-apply-and-one-write` decision's claim that neither the pre-flight apply nor the chained one can reap a pane.
- **Warm path unchanged.** `up` then `add` still yields one session, one header pane, one strand — the boot half of the self-heal must be a no-op on an already-up session.
- **Boot attribution.** The `Info` log naming the self-healing verb appears on a cold `add`/`attach` and does **not** appear on the warm equivalents — the observable consequence of `UpResult.Booted`.

**Regression surface.** The whole existing `internal/reedengine` and `internal/reedcli` suites (both tiers) must pass unchanged; any existing test asserting the `no reed session` refusal *from `AddStrand` or the attach verb specifically* is the behaviour being deliberately removed and should be converted, not deleted silently. Existing tests asserting the same refusal from `status`, `remove`, `reapply`, or the `io.go` ops must keep passing untouched — if one of those breaks, the change leaked past its scope.

## Q&A log

- **Q:** Which helper should `AddStrand` call — the bare boot, or all of `Up()`'s work? **A:** [auto-pick] Extract `upLocked()` (Up's whole body minus `withOpLock`) and call it. **Why:** `withOpLock` is a non-reentrant flock, so the exported `Up()` cannot be nested; and bare `ensureServerAndSessionLocked` leaves no header pane, producing a substrate `lyx reed up` never produces.
- **Q:** On a cold worktree whose `reed.json` holds strands, should the self-heal use `up` or `resume` semantics? **A:** [auto-pick] `up` semantics — bare substrate, no relaunch. **Why:** the roadmap names `Up()`'s helper, and `reed add` resurrecting a whole prior session would be a side effect nobody asked for.
- **Q:** Where does attach's self-heal land — the engine's `AttachArgv`, or the CLI pre-flight? **A:** [auto-pick] The CLI pre-flight: `c.eng.Status()` → `c.eng.Up()`. **Why:** the pre-flight is the actual refusal site; `AttachArgv` only degrades. Booting inside `AttachArgv` would break both its "never returns an error" and its "never calls `SaveState`" contracts, and a boot failure there could only be logged, never surfaced on the envelope before stdio handover.
- **Q:** Should `loom run`'s own attach handover (`run.go:228`) change too? **A:** [auto-pick] No. **Why:** `ensureStatusStrand` already calls `Up()` earlier in the same command, so that path is never cold.
- **Q:** Remove the now-redundant `reed up` sites — `ensureStatusStrand`'s `Up()` call and the generated VS Code task row? **A:** [auto-pick] Leave both; fix only the doc comments that now misdescribe the mechanism. **Why:** redundant is not wrong, and dropping the tasks.json row would leave existing worktrees on the old chain while new ones differ.
- **Q:** How is the foreign-session refusal preserved once a cold `AddStrand` boots instead of erroring? **A:** [auto-pick] Rely on `ensureServerAndSessionLocked`'s own `refuseRecordedForeignSessionBeforeBootLocked`, and pin it with tests on both the renamed-worktree and copied-`.lyx` fixtures. **Why:** that check is already placed ahead of anything that creates a session, precisely so a refusal leaves no residue; duplicating it in `AddStrand` would only risk divergence.
- **Q:** Should the other `requireSessionLocked` verbs (`Status`, `RemoveStrand`, `UpdateStrand`, `io.go`, `reapply.go`) self-heal too? **A:** [auto-pick] No — out of scope. **Why:** the roadmap names two entrypoints. `UpdateStrand`'s hidden→visible surface is the only near-miss and it is engine-API-only with no CLI verb in v1.
- **Q:** What test tiers? **A:** [auto-pick] Hermetic validation-order tests in `internal/reedengine`, plus a `smoke`-tagged real-tmux cold-worktree suite in `internal/reedcli`. **Why:** the package has no fake-tmux harness — hermetic tests here assert ordering against a nonexistent binary, and anything touching a real server must carry a build tag under the Test Tier Purity Invariant.
- **Q:** Does anything have to keep `validateIfAbsent` ahead of the boot? **A:** [auto-pick] Yes, it stays the first statement in `AddStrand`'s closure. **Why:** a config-shaped rejection must not spawn a tmux server as residue — the stronger form of the argument the existing comment already makes about the no-session error. The claim is scoped to config errors only; corrupt state is the documented exception.
- **Q:** (r2 gap) A cold worktree with an unreadable `reed.json` refuses pre-boot today, but the self-heal would boot then fail — which disposition? **A:** [auto-resolved] Accept boot-then-fail, matching `lyx reed up` exactly, and pin both the error text and the residue with a test. **Why:** `refuseRecordedForeignSessionBeforeBootLocked` returns `nil` on a load error by explicit design (`generation.go:170-175`), and `up` already boots-then-fails on corrupt state; refusing pre-boot for `add`/`attach` alone would diverge the very paths this task is unifying. If the residue is later judged unacceptable the fix belongs at that shared helper, so all four verbs move together.
- **Q:** (r2 gap) The attach self-heal lives in `reedcli` and can only call exported `Up()` — how does it see the `booted` signal? **A:** [auto-resolved] `UpResult` gains an additive `Booted bool` field; `upLocked` sets it, `Up()` returns it through. **Why:** a bool returned only in-package is unreachable from the CLI site. This supersedes the earlier "keep `UpResult`'s shape frozen" framing, and the Scope "Out" bullet now says "no new exported **method**" rather than no exported change at all.
- **Q:** (r2 gap) Is the new self-heal log required by Live-Substrate Spawn Observability? **A:** [auto-resolved] No — the invariant is already satisfied by `lifecycle.go:331`. The log is kept, re-grounded on verb attribution. **Why:** the existing line records that a server was spawned, never which verb asked for it, so a session appearing out of a bare `reed add` is currently untraceable in the log. Keeping it costs one `Info` line; the decision no longer claims invariant compliance as its motive.
- **Q:** (r2 gap) A warm `reed attach` would now write `reed.json` and apply a layout right before `AttachArgv` chains a second one — accept or avoid? **A:** [auto-resolved] Accept, with the three consequences named explicitly and a smoke test pinning that panes, clients and bindings survive. **Why:** avoiding it needs either a new exported boot-only method or a conditional duplicate of `upLocked`'s tail, and the alternative of gating on `Status()`'s error is unclassifiable without string-matching `noSessionMessage`. Neither apply can reap a pane, since both enumerate live panes; the cost is one superseded re-tile and one state write.
- **Q:** (r2) Is `drive.go:88`'s `c.reed.Up()` also redundant now? **A:** [auto-resolved] It is named in the "Out" list and kept — and it is the least redundant of the three. **Why:** `drive` adds no strand and hands no terminal over, so it boots *ahead of* the producers that will spawn strands, to fail on the envelope at step 0 rather than several producers deep. The self-heal does not give it that early failure.
