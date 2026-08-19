# Discussion: loom: session bootstrap

```yaml
task: 'loom: session bootstrap'
slug: loom-session-bootstrap
status: discussing
parent: standalone-producers
```

## Problem

`internal/loomshed` carries loom's full 13-row producer list and its status-file seeder, and `internal/shedengine` carries the phase machine that walks it — but **nothing calls either of them**.
A repo-wide grep for `loomshed.` outside `internal/loomshed` returns zero production hits, and `cmd/lyx/main.go` registers twelve module subtrees, none of them `loom`.
The phase machine that the Done `loom: phase-machine scaffolding` item built is unreachable from the CLI.

This task builds the entry point that makes it reachable: `lyx loom run` (alias `lyx run`), which `manifest/designs/loom.md#entry-point--the-session-bootstrap` specifies as a **session bootstrap**, not merely a driver.
Run in a worktree, it brings up the worktree's tmux session, adds the status strand, spawns the loom driver detached, and hands the terminal to tmux — so loom runs in the background while the reed view takes the window.
It also drops a double-click run-launcher so the whole thing is one click.

**Why now:** every remaining item in the loom initiative (`landing: Publish + Finalize producers`, `loom: write and wire in the real LLM producers`) plugs producers into a machine no one can start.
Until this lands, none of that work is observable end-to-end.

## Scope

**In:**

- A new `internal/loomcli` cobra module — the 13th seam module — with four verbs: `run`, `drive`, `status`, `pause`.
- `lyx loom drive`: runs the phase machine (`loomshed.New` → `shedengine.Run`) in the foreground, no tmux, no strand.
- `lyx loom run`: the four-step bootstrap (reed up → add status strand → spawn `lyx loom drive` detached via `internal/proc` → attach terminal to the tmux session), re-entrant.
- `lyx loom status` (envelope) and `lyx loom status --watch` (the 1-line status strand).
- `lyx loom pause`: sets `pause_requested` in `_lyx/loom/status.json`.
- `lyx run` as a second top-level cobra command aliasing `lyx loom run`.
- Status-file seeding on `lyx loom run` when `_lyx/loom/status.json` is absent, via the already-idempotent `loomshed.Seed`.
- A new `parent_branch` provenance record written by `fabricengine.Topology.Add` into the weft, plus the `--parent` flag that writes it once for worktrees created before it existed.
- A third launcher script (`run.cmd`/`run.sh`) in the existing `<hub>/_launchers/<AnchorRel>/<slug>/` set.
- Driver stdout/stderr capture to a machine-local log.
- Registration/help/seam/sandbox test updates that adding a 13th module forces.
- Doc updates: `manifest/designs/loom.md`'s run-launcher paragraph, `docs/overview.md`'s module table and the root `Long` module list, `manifest/roadmap.md` item moved to Done.

**Out:**

- Any change to the producer list itself — all seven stubs stay stubs.
  Replacing them is `loom: write and wire in the real LLM producers`.
- `Publish`/`Finalize` — those are `landing: Publish + Finalize producers`.
- `Plan-Sweep`, rubrics, prompt content, `perch`/`burler`/`webster` internals.
- The cross-worktree multi-column reed view (deferred reed feature; scope stays one terminal per worktree).
- `--auto` mode (`lyx run --auto`) — the phase machine's yielding behaviour is producer-level, and every producer that would yield is still a stub, so there is nothing for the flag to change yet.
- Any new `CONSTRAINTS.md` invariant — nothing here crosses a seam not already governed by CLI/Cobra, Durable-vs-Ephemeral, Told-Geometry, Fabric Write-Side Containment, or Mutation Record.

## Decisions

### verb-surface

- Decision: `internal/loomcli` ships all four verbs — `run`, `drive`, `status`, `pause` — in this task.
- Rationale: `status` is a hard requirement of the bootstrap's own step 2 (the strand launches `lyx loom status --watch`; without the verb the pane is broken).
  `pause` is a ~20-line `state.UpdateJSON` flag flip against a machine that already honours the flag (`internal/shedengine/run.go:116`).
  Shipping three of four leaves `designs/loom.md`'s own named CLI (`lyx loom run|pause`, `lyx loom status`) half-built for no gain.
- Rejected: deferring `pause` to a later task — the gap is smaller than the coordination cost of a follow-up item.

### drive-as-real-verb

- Decision: the detached driver is a real, documented verb, `lyx loom drive`, which runs the phase machine in the foreground with no tmux involvement.
  `lyx loom run` spawns `lyx loom drive` detached via `proc.Detach`.
- Rationale: matches `designs/loom.md`'s step 3 ("spawn the loom driver DETACHED — `internal/proc`; it needs no TTY") almost verbatim.
  It makes the bootstrap a thin composition of two independently-testable verbs and gives a first-class no-tmux escape hatch for debugging and CI.
- Rejected: a hidden `--driver` flag on `lyx loom run` — an undocumented flag would be the only way to run the machine without tmux, which is exactly the surface the CLI/Cobra Invariant's "`Short` on every command" discipline exists to prevent.
  Also rejected: running the machine in-process in a goroutine — step 4 hands stdio to tmux and blocks, so the driver would die with the operator's terminal.

### self-seeding

- Decision: `lyx loom run` seeds `_lyx/loom/status.json` itself when absent, via `loomshed.Seed`.
- Rationale: `loomengine.Preflight` check-4 hard-fails when the seed is missing, so without this `lyx loom run` can never get past its own first producer.
  `Seed` already refuses to overwrite an existing file, so re-running is safe by construction.
  Keeps the bootstrap self-sufficient and touches no other module's verbs.
- Rejected: seeding from `lyx fabric spawn` — points the coupling the wrong way (loom may depend on fabric, never the reverse) and leaves every already-spawned worktree unseedable.
  Also rejected: a separate operator-invoked `lyx loom seed` verb — a mandatory manual step defeats the one-double-click entry point this task exists to deliver.

### parent-branch-is-recorded-never-guessed

- Decision: the seed's `parent` value comes from a **recorded** fabric provenance file, never from inference.
  `fabricengine.Topology.Add` writes `_lyx/fabric/origin.json` carrying `parent_branch` at worktree-creation time; loom reads it.
- Rationale: `contracts/specs/loom-status-spec.md` makes `product.parent` mandatory, and a wrong parent silently mis-targets the eventual merge-back.
  `Add` is the one place that already knows the answer — it computes `parentBranch` at `internal/fabricengine/add.go:141` from `rev-parse --abbrev-ref HEAD` and currently throws it away after deriving `parentWeftBranch`.
  The weft is the right home: `_lyx` is the tracked, fabric-synced weft side junctioned into the warp, so the record survives a fresh clone on another machine — which is what `designs/loom.md`'s "resume works across machines" property rests on.
  `state.WriteJSON` is already the precedent for this class of small fabric state in the same package (`mergestate.go:107`, `corrindex.go`).
- Rejected: defaulting to the Prime worktree's current branch via `fabricengine.PrimeName` + `gitrepo.Repo.CurrentBranch` — that is a guess, and it is wrong whenever the prime worktree has been switched or the pair was forked from a non-prime branch.
  Also rejected: a plain one-line `_lyx/.lyx-parent` marker (breaks with the JSON convention fabric already uses for this state class and has no room to grow), and a git-config entry `branch.<name>.lyxparent` (`.git/config` is machine-local and does not survive a fresh clone).

### missing-record-refusal

- Decision: in a worktree created before `origin.json` existed, `lyx loom run` refuses, names the missing record, and accepts `lyx loom run --parent <branch>` to write the record once.
  After that one write the launcher is zero-argument forever.
- Rationale: the operator supplies the fact explicitly; nothing is inferred.
  New worktrees never need the flag because `Add` writes the record from now on.
- Rejected: refusing with no flag at all — no fabric verb can supply the parent for an existing worktree without guessing either, so old worktrees would be permanently unrunnable.
  Also rejected: seeding `parent: ""` — `loom-status-spec.md:77` makes `product.parent` mandatory and counts an empty string as absent.

### reentrancy-ensure-and-attach

- Decision: a second `lyx loom run` while a driver is alive ensures substrate and attaches, and never spawns a second driver.
  `lock.TryAcquireWriteLock(loomengine.LoomRunLock(l))` is the liveness probe: held ⇒ a driver is running, skip step 3; not held ⇒ release immediately and spawn.
  Steps 1, 2 and 4 are idempotent.
- Rationale: makes the double-click launcher safe to hit repeatedly, and gives the operator a way to reattach after closing the terminal — the common case.
- Rejected: refusing when the lock is held (leaves no reattach path), and always spawning (the second driver would die instantly on `shedengine.Run`'s own lock, leaving a confusing dead pane).

### status-output-split

- Decision: bare `lyx loom status` emits one normal `output.Ok` JSON envelope.
  `lyx loom status --watch` polls and prints the one-line composed `Activity` forever, taking the CLI/Cobra Invariant's self-displays-then-blocks-forever exception explicitly; everything fallible stays pre-flight on the envelope.
- Rationale: the only shape satisfying both the JSON-envelope rule and `designs/loom.md`'s "1-line pane at the top" requirement.
- Rejected: `--watch` emitting JSON lines — a pane of raw JSON is not the status line the design asks for.

### driver-failure-visibility

- Decision: the detached child's stdout/stderr are redirected to `.lyx/loom/driver.log`.
- Rationale: `shedengine.Run` persists `state: error` plus `error` into the status file for in-machine failures, but a crash *before* the first persist (wiring panic, bad geometry, a nil producer) leaves the operator with an empty pane and no diagnosis.
  `.lyx/loom/` is the mirrored ephemeral sibling of `_lyx/loom/`, exactly the Durable-vs-Ephemeral State Invariant's pattern, and it is where `LoomStatusLock`/`LoomRunLock` already live.
- Rejected: discarding to `os.DevNull` — makes the pre-first-persist failure class silent.

### launcher-reuses-existing-set

- Decision: the run-launcher is a third script, `run.cmd`/`run.sh`, written into the existing `<hub>/_launchers/<AnchorRel>/<slug>/` directory beside `ide` and `fabric-checkout`, by the same `writeLaunchers`/`removeLaunchers` pair and the same `launcherScript` content builder.
- Rationale: strictly reuse — the mechanism is already cross-platform, `os.Root`-contained, mutation-recorded, and torn down.
  The relative-path climb (`%~dp0` / `$(dirname $0)`) means no absolute path is embedded, so nothing is machine-bound.
- Rejected: `.lyx/lyxrun.cmd` in the worktree as `designs/loom.md` currently says — that text describes a mechanism that does not exist in this repo; it would need a new write path, a new teardown obligation, and there is no double-click surface pointing at it.
  The doc paragraph is stale and is rewritten in this same commit per the Documentation Lifecycle.

### run-alias-as-registered-command

- Decision: `lyx run` is a second top-level cobra command registered from `loomcli` into `newRoot()`, sharing `loom run`'s RunE builder and pre-run resolution.
- Rationale: stays inside the module `Command()`/`RunCLI` seam and inside `helptree_test.go`/`registration_test.go` coverage, and is discoverable in `--help`.
- Rejected: argv rewriting in `cmd/lyx/main.go` (splice `loom` in front of a leading `run`) — invisible to `--help` and outside every module's own seam.

### fabric-change-stays-in-this-task

- Decision: the `origin.json` write lands in this task, not a separate fabric task.
- Rationale: it is one new field, one `state.WriteJSON` call, one rollback entry and one `KindFileWritten` mutation record inside an already-transactional verb — no new module, no new invariant, reusing a pattern already present in the same package.
  Splitting it out would add a fourth parallel coordination surface (alongside `landing`, this task, and `fabric-merge-crucible-hardening`) for a change driven entirely by loom's own need.
- Rejected: a standalone fabric task.
- Caveat carried forward: because this touches a chokepoint-guarded write point, that part of the diff gets extra verification focus in this task's own review — ordinary thoroughness on that commit, not a separate crucible round.

## Technical context

**What already exists and must be reused, not rebuilt:**

- `internal/loomshed.New(Deps) (*shedengine.Shed, error)` — the 13-row producer list. `Deps` is told absolute paths only: `StatusPath`, `LockPath`, `StatusLockPath`, `MaxBounces`, `AnchorPath`, `WorktreeRoot`, `DecisionRecordPath`, `SupportLogPath`, `Preflight`, `WebsterRun`, `WebsterDeps`.
- `internal/loomshed.Seed(statusPath, statusLockPath, slug, parent) error` — refuses when the file exists.
- `internal/loomengine` path accessors, all `AnchorPath`-anchored, and per the Cwd Resolution Invariant the **only** legal constructors for these paths: `LoomStatusFile`, `LoomStatusLock` (under `.lyx`), `LoomRunLock` (under `.lyx`), `DiscussionDecisionRecord`, `DiscussionSupportLog`, `DiscussionDir`.
  `LoomRunLock` must never equal `LoomStatusLock` — `Shed.validate()` rejects that outright.
- `internal/loomengine.Preflight` — already built, engine-only, injected as `Deps.Preflight`.
- `internal/loomengine.LoadConfig(baseDir, module)` — loom's `loom.yaml`; note it maps a `not initialized` error onto `run "lyx fabric reconcile"`.
- `internal/shedengine.Shed.Run(ctx) (Result, error)` — persists `CurrentProducer`/`State`/`Error`/`Activity`/`History` on every step and consumes `PauseRequested` at the loop head.
- `internal/shedengine.Activity` — composed mechanically by `composeActivity`; this is what `status --watch` renders as one line.
- `internal/reedengine.Engine`: `Up() (UpResult, error)` (documented no-op when already up), `AddStrand(AddSpec) (Strand, error)`, `Status() (StatusResult, error)` returning `[]StrandStatus{GUID, Name, PaneID, Live}`, `Socket()`, `SessionName()`, `TmuxPath()`.
- `internal/reedcli/attach.go` — the exact in-place `tmux attach-session` handover pattern (`attachArgv`, pre-flight `Status()` on the envelope, then stdio handover with exit-code propagation). Step 4 should follow it rather than invent one.
- `internal/reedcli/add.go` — the `AddSpec` shape for a `below-parent` strand with `ShrinkWhenWaitingOnChild: true`.
- `internal/proc.Detach(cmd)` / `DetachBreakaway(cmd)` / `IsAlive(pid)` / `KillPID(pid)` — cross-OS, `internal/proc` is the sole allowlisted spawn-in-untagged-tests package.
- `internal/lock.TryAcquireWriteLock(lockPath) (*FileLock, bool, error)` — non-blocking, reports `(nil, false, nil)` when held; not an error.
- `internal/state.ReadJSON` / `WriteJSON` / `UpdateJSON` — lock-guarded JSON state.
- `internal/hubgeom` — the hub-mode geometry adapter; `internal/standalonegeom` is its told-mode twin.
- `internal/clihelp` — `Execute`, `ExecuteIn`, `GroupRunE`, `Abort`, `ShouldAbort`, `SetExit`, `InstallJSONHelp`.
- `internal/output` — `Ok`/`Err` envelopes.

**Wiring precedent to follow:** `internal/perchcli` is the closest analogue — a `PersistentPreRunE` that resolves cwd once, a `wiring.go` that builds the engine stack onto a CLI receiver, and per-verb construction of anything whose inputs are only known after flag parsing.
`internal/reedcli` is the precedent for the cobra group shape and the `cmd.Name() == "<group>"` guard that lets a bare group listing work outside a git repo.

**Mode question:** loom needs a wired fabric (Preflight validates warp/weft pairing and sync), so it is hub-only — its pre-run resolves via `lyxcwd.Resolve` like `reedcli`, not `preflight.ResolveMode`'s degrade-to-standalone path that `perchcli` uses.

**Seed inputs:** `slug` is `lyxcwd.Location.WorktreeName`.
`parent` is read from the new `_lyx/fabric/origin.json`.

**The fabric change, concretely:** `internal/fabricengine/add.go` already computes `parentBranch := strings.TrimSpace(headStdout)` at line 141 and uses it only to derive `parentWeftBranch`.
The new write records that same value.
Ordering matters — the record lives on the weft side, so the write must happen after `createWeftWorktree`/weft adoption and after `createPortal`, or be written directly to `WeftWorktreePath(l, slug)` rather than through the junction; the plan should pick one and say which.
The write must append a `KindFileWritten` mutation to `rec` **after** it observably succeeds, and `rollbackAdd` must clean it up, per the Mutation Record Invariant.
`state.WriteJSON` lives in another package, so the raw-write tokens the Fabric Write-Side Containment guard scans for (`os.WriteFile(` etc.) do not appear in `fabricengine` source — the same reason `mergestate.go`'s existing `state.WriteJSON` call passes.

**Registration fallout of a 13th module** — all of these are machine-enforced and will fail until updated:

- `cmd/lyx/main.go`: import, `root.AddCommand(loomcli.Command())`, the `lyx run` alias command, and the root `Long` "Available modules:" list.
- `cmd/lyx/seamsignature_test.go` — pins `RunCLI` across "all twelve modules" and `RunCLIIn` across eleven.
- `cmd/lyx/registration_test.go`, `helptree_test.go`, `longlist_test.go`, `drift_test.go` (non-empty `Short` on every command).
- `cmd/lyx/sandbox_coverage_test.go` — needs a `**Covers:** loom` scenario in a `tools/sandbox/*SUITE.md` file.
- `cmd/lyx/notransients_test.go` / `constructoranchoring_test.go` — the new `.lyx/loom/driver.log` path must be constructed by a `loomengine` accessor, not built inline.

**Gotcha — strand identity:** `reedengine.AddStrand` has no upsert semantics; a second call appends a second strand.
Re-entrancy requires looking the status strand up by a fixed `NameOverride` in `eng.Status().Strands` before adding.

**Gotcha — no `--watch` precedent:** nothing in the repo currently implements a polling watch loop, and the Test Tier Purity Invariant flags a constant `time.Sleep(...)` of ≥ 1s in an untagged test file.
The poll interval belongs behind a variable or a flag so tests can drive it fast.

## Constraints

From `CONSTRAINTS.md`, the ones this task is bound by:

- **Cwd Resolution Invariant** — `internal/lyxcwd` owns cwd resolution alone; every loom path comes from a `loomengine` accessor, never assembled in `loomcli`.
- **CLI / Cobra Invariant** — `Command()` + `RunCLI(out, args) int` (+ `RunCLIIn`), `Short` on every command, JSON envelopes via `internal/output`, `RunE` checks `ShouldAbort` **first**, one envelope per invocation, parent group sets `RunE = clihelp.GroupRunE`.
  `status --watch` and `run`'s attach tail both take the narrow interactive-handoff / self-displays-then-blocks exception; everything fallible stays pre-flight.
- **Told-Geometry Invariant** — `loomshed` is machine-enforced against importing `internal/lyxcwd` (`seam_enforcement_test.go`'s `TestToldGeometryInvariant_AllowlistOnly`). `loomcli` is the adapter that resolves the Location and threads plain values down.
- **Durable-vs-Ephemeral State Invariant** — `driver.log` and the lock files go under `.lyx/loom/` at the mirrored subpath of `_lyx/loom/`; `_lyx` holds tracked content only.
- **Lyxdirs Single-Declarer Invariant** — never write the literals `_lyx` / `.lyx`; use `lyxdirs.LyxDirName` / `lyxdirs.DotLyxDirName`.
- **Mutation Record Invariant** — the new `origin.json` write appends `KindFileWritten` to `rec` only after it observably succeeded, and `AddResult` already embeds `MutationRecord`.
- **Fabric Write-Side Containment Invariant** — no `os.`-qualified raw write tokens in `fabricengine` production source.
- **Fabric Destruction Chokepoint Invariant** — `rollbackAdd`'s cleanup of the new record goes through the existing gated removal path, not a raw `os.Remove`.
- **Test Tier Purity Invariant** — untagged tests spawn nothing (no `exec.Command`, no `gitexec.Run`, no `hubforge.NewHub`); the tmux/detach work is `//go:build smoke`.
- **Hermetic Git Test Environment Invariant** — any new git-spawning test package needs a `TestMain` calling `gitkit.HermeticGitEnv()`.
- **Sandbox Suite Coverage** — a new registered module needs a `**Covers:** loom` scenario or an allowlist entry; the allowlist is not the answer here.
- **Documentation Lifecycle** — the doc updates listed in Scope land in the same commit.

## Testing

Three layers, deliberately.

**Tier 1 (untagged, no spawns) — `internal/loomcli`:**

- `loomshed.Deps` assembly from a fake `lyxcwd.Location`: every path field equals the corresponding `loomengine` accessor's output, and `LockPath != StatusLockPath`.
- Seed-input resolution: slug comes from `WorktreeName`; parent comes from a fixture `origin.json`; a missing/empty `parent_branch` produces the refusal message naming `--parent`, and `--parent` supplied writes the record.
- Re-entrancy decision function — a pure predicate over "run lock held?" returning spawn-or-skip, so the branch is testable without a real lock or process. **TDD candidate.**
- Status-strand lookup: given a `[]StrandStatus`, decide add-or-reuse by fixed name. **TDD candidate.**
- One-line `Activity` rendering for `status --watch`: given a `shedengine.Status`, produce the exact line. **TDD candidate.**
- Bare `lyx loom status` envelope shape, and the missing/corrupt status-file error paths.
- `lyx loom pause` sets `pause_requested` true and leaves every other field untouched; pausing an absent status file errors rather than creating one.
- `lyx loom drive` argument/flag validation up to the point of engine construction.

**Tier 1 — `internal/fabricengine`:** the `origin.json` record's encode/decode round-trip and its path derivation, kept git-free so the file stays untagged (the same discipline `corrindex.go`'s tests already follow).

**Integration (`//go:build integration`) — `internal/fabricengine`:** `Add` writes `origin.json` with the correct `parent_branch` for a pair forked from a non-default branch; a forced post-write failure rolls the record back; the `Mutations` snapshot contains exactly one `KindFileWritten` for it.
This is the chokepoint-guarded part of the diff and gets the extra verification focus agreed in the `fabric-change-stays-in-this-task` decision.

**Smoke (`//go:build smoke`) — the bootstrap itself, the part nothing else can catch:**

- `lyx loom run` in a real wired hub: tmux session up, exactly one status strand present with the fixed name, a detached driver PID alive, and the status file seeded.
- A second `lyx loom run` while the first driver holds the run lock: still exactly one strand, still exactly one driver PID, no new process.
- `lyx loom drive` standalone with no tmux at all: the machine advances and the status file records it.
- Driver failure before the first persist: `.lyx/loom/driver.log` is non-empty and names the failure.
- `run.sh`/`run.cmd` exists in `<hub>/_launchers/<AnchorRel>/<slug>/` after `lyx fabric add`, and is gone after the matching remove.

**Sandbox:** one scenario in a `tools/sandbox/*SUITE.md` file tagged `**Covers:** loom`, exercising `lyx loom status` and `lyx loom pause` against a seeded status file — the two verbs that need no tmux.

**Scenarios that must be covered somewhere:** missing `origin.json`; `--parent` on an already-recorded worktree (should it refuse or overwrite — the plan must decide and test whichever); a status file present but incoherent (Preflight check-4's half-finished case); attach failing because tmux is absent; the run lock held by a *dead* process (stale lock).

## Q&A log

- **Q:** Which verbs does `internal/loomcli` ship in this task? **A:** All four — `run`, `drive`, `status`, `pause`. `status` is a hard requirement of bootstrap step 2 and `pause` is a ~20-line flag flip; deferring either leaves the designed CLI half-built.
- **Q:** How does the detached driver get launched? **A:** A real `lyx loom drive` verb spawned via `proc.Detach`. A hidden `--driver` flag is exactly the surface the CLI/Cobra Invariant's `Short`-on-every-command rule exists to prevent.
- **Q:** Who seeds `_lyx/loom/status.json`? **A:** `lyx loom run` itself, via the already-idempotent `loomshed.Seed`. Seeding from `lyx fabric spawn` would point the coupling backwards and strand existing worktrees.
- **Q:** Where does the run-launcher live? **A:** A third `run<ext>` script in the existing `<hub>/_launchers/<AnchorRel>/<slug>/` set — verified against `writeLaunchers`/`removeLaunchers`, not just the doc. `designs/loom.md`'s `.lyx/lyxrun.cmd` text is stale and is corrected in the same commit.
- **Q:** How is the `lyx run` alias wired? **A:** A second registered cobra command from `loomcli`, so it stays inside the seam and inside help-tree test coverage. argv-splicing in `main.go` would be invisible to `--help`.
- **Q:** Where does the seed's `parent` come from? **A:** A recorded fabric provenance file — never inference. The operator's explicit correction: it must *know* the parent branch, not guess it. Defaulting to the prime worktree's current branch was rejected for that reason.
- **Q:** What shape is that record and where does it live? **A:** `_lyx/fabric/origin.json`, written by `fabricengine.Topology.Add` via `state.WriteJSON` (the `mergestate.go` precedent). The weft side, because it is tracked and fabric-synced, so it survives a fresh clone — which is what loom's cross-machine resume rests on. A `.git/config` entry would not.
- **Q:** What about worktrees created before the record existed? **A:** Refuse, name the missing record, and accept `--parent <branch>` to write it once. `parent: ""` is not an option — `loom-status-spec.md:77` makes it mandatory.
- **Q:** Second `lyx loom run` while a driver is alive? **A:** Ensure-and-attach, never double-spawn; `lock.TryAcquireWriteLock` on `LoomRunLock` is the liveness probe. Erroring would leave no reattach path; blindly respawning would leave a dead pane with no explanation.
- **Q:** `lyx loom status` output contract? **A:** Bare call emits a normal JSON envelope; `--watch` takes the documented self-displays-then-blocks-forever exception and prints the 1-line `Activity`.
- **Q:** How does a detached driver failure surface? **A:** `.lyx/loom/driver.log` plus the status file's own `state: error`. The log is necessary because a crash before the first persist would otherwise be completely silent.
- **Q:** Should the fabric change be split into its own task? **A:** No. One field, one `state.WriteJSON`, one rollback entry, one mutation record, inside an already-transactional verb, reusing a same-package pattern — splitting it would add a fourth parallel coordination surface for a change driven by loom's own need. The chokepoint-guarded part of the diff gets extra verification focus within this task instead.
- **Q:** Test strategy? **A:** Tier-1 units for wiring and pure decisions, an integration test for the `Add` write plus rollback, a smoke test for the tmux/detach interaction (the only place it can be caught), and a sandbox scenario for module coverage. Adding `loom` to `excludedModules` is not acceptable for a newly registered module.
