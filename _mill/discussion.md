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

- `internal/reedengine/lifecycle.go` — extract the body of `Up()` into a new locked helper `upLocked() (UpResult, error)`; `Up()` becomes `withOpLock(upLocked)`.
- `internal/reedengine/strand.go` — `AddStrand` calls `e.upLocked()` in place of `e.requireSessionLocked()`, after `validateIfAbsent`.
- `internal/reedcli/attach.go` — the pre-flight switches from `c.eng.Status()` to `c.eng.Up()`.
- Observability: one `logger.Info` when a self-heal actually boots a session (`booted == true`).
- Doc updates in the same commit: `internal/reedengine/doc.go`, `AddStrand`'s and `AttachArgv`'s own doc comments, `internal/vscode/config.go`'s tasks.json rationale comment, `docs/overview.md`'s reed entry, `manifest/designs/worktree-lifecycle-shed-producers.md`'s "Today, without this item built yet" framing, and `manifest/roadmap.md` (Planned → Done).
- Tests: hermetic ordering/regression tests in `internal/reedengine`, plus a `smoke`-tagged cold-worktree test in `internal/reedcli`.

**Out:**

- `Engine.Status`, `Engine.RemoveStrand`, `Engine.UpdateStrand`, `internal/reedengine/io.go`'s three send/capture ops, and `internal/reedengine/reapply.go` keep `requireSessionLocked` exactly as-is. `requireSessionLocked` and `noSessionMessage` are not deleted or weakened.
- `internal/loomcli/run.go`'s own attach handover (`c.reed.Status()` at `run.go:228`) is unchanged.
- `internal/loomcli/sharedbootstrap.go`'s `ensureStatusStrand` keeps its explicit `c.reed.Up()` call.
- The generated `.vscode/tasks.json` chain (`reed up` → `reed add --if-absent` → `reed attach`) keeps its `reed up` row.
- `Engine.AttachArgv` gains no boot: it stays a non-refusing, `reed.json`-read-only builder.
- No `Resume` semantics anywhere — this task never relaunches a persisted strand.
- The `born-as-strand` item (`manifest/designs/reed-born-as-strand.md`, `loom run`'s terminal handoff never calling `AddStrand`) is a different problem and stays untouched.
- No new exported engine API beyond what already exists — `Up()` is already exported and is what the CLI calls.

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
- **Rationale:** the attach verb's actual refusal site is that pre-flight, not the engine. `AttachArgv` never refuses by contract — its own `requireSessionLocked` (`internal/reedengine/attach.go:84`) only degrades to the bare `attach-session` argv, which then fails at the tmux layer. Making `AttachArgv` boot would break two of its documented properties at once: "returns no error, by contract" and "Read-only with respect to `reed.json`: this builder never calls `SaveState`" — `upLocked`'s `reconcileApplyPersistLocked` writes state. Booting in the pre-flight instead keeps the builder pure, keeps a boot failure (bad `header.template`, invalid `mouse`/`watchdog`/`debug_log`, a capability-probe failure, a recorded-foreign-session refusal) reportable on the JSON envelope *before* stdio is handed to the child, and means `AttachArgv`'s own `requireSessionLocked` passes by the time it runs. `Up()` is idempotent — `ensureServerAndSessionLocked` returns early on an already-up session with panes — so the warm path costs one extra `has-session` round trip and a layout re-apply, not a re-boot.
- **Rejected:** (a) booting inside `AttachArgv` — side-effecting builder, and a boot failure could only be logged, never surfaced; (b) adding a new exported `Engine.EnsureSession()` — `Up()` already is exactly that.

### loom-run-handover-untouched

- **Decision:** `internal/loomcli/run.go:228`'s `c.reed.Status()` pre-flight stays.
- **Rationale:** `loom run` calls `ensureStatusStrand` at `run.go:101` (and `step.go:175`), which already calls `c.reed.Up()` first (`sharedbootstrap.go:185`). The session is therefore never cold by the time step 7's handover runs, so changing it buys nothing and touches the CLI/Cobra Invariant's interactive-handoff exception for no reason.

### redundant-up-sites-kept

- **Decision:** keep `ensureStatusStrand`'s `c.reed.Up()` and the generated `.vscode/tasks.json` `reed up` row. Only the doc comments that now misdescribe the mechanism are updated.
- **Rationale:** both become redundant, neither becomes wrong. `ensureStatusStrand`'s `Up()` is the call whose `UpResult` and explicit ordering make loom's bootstrap legible; removing it would hide the boot inside `AddStrand`. Removing the tasks.json row would only affect newly-generated worktrees while every existing one keeps the old chain, so the codebase would carry both shapes for no gain. `internal/vscode/config.go:70-74` does need editing: its safety argument is written in terms of "AddStrand pre-flights `requireSessionLocked` and attach pre-flights `Status`", which stops being true.
- **Rejected:** removing either — scope creep with no behavioural payoff.

### foreign-session-refusal-preserved

- **Decision:** rely on `ensureServerAndSessionLocked`'s own `refuseRecordedForeignSessionBeforeBootLocked` (`lifecycle.go:219`) for the foreign-session cases, and pin the outcome with a test. No extra refusal call is added to `AddStrand`.
- **Rationale:** this is the one genuine behavioural risk of the change. `requireSessionLocked` consults `refuseLiveForeignSessionLocked` on the *non-boot* path; the boot path has its own equivalent, placed deliberately ahead of anything that creates a session so a refusal never deposits a bare session as residue. Both routes into a foreign-session state (a worktree renamed while its session was up, a `.lyx` copied between worktrees of one hub — see `internal/reedcli/smoke_staterecovery_test.go`'s R5-F4/R5-F5 scenarios) leave *this* worktree's session absent, which is exactly the state that now boots instead of erroring. The refusal must still fire, and from `AddStrand` too.
- **Rejected:** an explicit pre-boot refusal inside `AddStrand` — duplicates a check the boot path already runs at the structurally correct point.

### config-errors-still-precede-the-boot

- **Decision:** `validateIfAbsent(spec)` stays the first statement in `AddStrand`'s closure, ahead of the boot. Inside `upLocked`, `ensureServerAndSessionLocked`'s existing pre-tmux validation block (`debugLogArgs`, `mouseOption`, `watchdogOption`, `ValidateHeader`, then `probeCapabilityLocked`) is untouched.
- **Rationale:** a rejected call must never boot a session as residue. `AddStrand`'s existing comment already states this for `validateIfAbsent` ("a rejected call must never deposit a friendly no-session error over what is actually a missing `--name`"); the same argument now covers a stronger consequence — a spawned tmux server.

### selfheal-observability

- **Decision:** log once, at `Info`, when a self-heal actually boots: inside `upLocked` is wrong (it would also fire for the plain `lyx reed up` verb), so the log goes at the two self-heal call sites, guarded on the `UpResult`/`booted` signal. `upLocked` must therefore return enough for a caller to tell a boot from a no-op.
- **Rationale:** CONSTRAINTS.md's Live-Substrate Spawn Observability invariant requires every path that starts a real OS process to log its spawn at `Info`. `AddStrand` reaching `ensureServerAndSessionLocked` now spawns a tmux server, and the operator needs to see in the log that `reed add` booted the session rather than joined one. `UpResult` today is `{Session, Socket, Strands}` and carries no boot flag; `upLocked` returning `(UpResult, bool, error)` with the `booted` flag — `Up()` discarding it — is the smallest change that keeps `UpResult`'s exported shape stable. mill-plan picks the exact signature; the requirement is that the two self-heal sites can log "booted" versus "already up" without a second tmux round trip.
- **Rejected:** no logging — violates the invariant; logging unconditionally — noise on every warm `reed add`.

## Technical context

Everything in scope is inside `internal/reedengine`, `internal/reedcli`, and doc/comment sites.

Key files and line anchors as of this branch's HEAD:

- `internal/reedengine/lifecycle.go:163` `ensureServerAndSessionLocked` — validates config pre-tmux, probes capability, refuses a recorded foreign session, reaps a stale socket-holder, prunes server logs, spawns. Returns `(booted bool, strippedKeys []string, err error)`.
- `internal/reedengine/lifecycle.go:650` `Up()` — the body to extract. Note the `if booted { clearAllPaneBindings(st); st.StrippedEnv = stripped; st.HeaderPaneID = "" }` block and its comment warning that the `HeaderPaneID` clear deliberately lives here and not inside `clearAllPaneBindings`.
- `internal/reedengine/lifecycle.go:702` `Resume()` — the sibling with the same booted-bookkeeping block; **do not** fold the two into one shared helper as a drive-by, they diverge after `ensureHeaderPaneLocked`.
- `internal/reedengine/lifecycle.go:1109` `noSessionMessage` / `:1134` `requireSessionLocked` — stay, still used by five other ops.
- `internal/reedengine/strand.go:376` `AddStrand` — the call site; its doc comment's "Pre-flights the session's existence (mirroring Status)" sentence must be rewritten.
- `internal/reedengine/strand.go:481` `UpdateStrand` and `RemoveStrand` — their doc comments say "like AddStrand" about the pre-flight; those cross-references go stale and must be re-pointed at `Status` instead.
- `internal/reedengine/attach.go:74` `AttachArgv` — unchanged code, but its header comment and doc comment describe the pre-flight world and need a sentence acknowledging that its caller now boots first.
- `internal/reedengine/lock.go:89` `withOpLock` — the non-reentrancy that forces the `upLocked` extraction.
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

- **Live-Substrate Spawn Observability** — `AddStrand` and the attach pre-flight now reach a real tmux-server spawn; each must log it at `Info` via `internal/logger`. See the `selfheal-observability` decision.
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
- **Warm path unchanged.** `up` then `add` still yields one session, one header pane, one strand — the self-heal must be a no-op on an already-up session.

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
- **Q:** Does anything have to keep `validateIfAbsent` ahead of the boot? **A:** [auto-pick] Yes, it stays the first statement in `AddStrand`'s closure. **Why:** a rejected call must not spawn a tmux server as residue — the stronger form of the argument the existing comment already makes about the no-session error.
