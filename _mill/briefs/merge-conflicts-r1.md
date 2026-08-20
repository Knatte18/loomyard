# Conflict Resolution Brief

Your sole job is to resolve git conflict markers in the listed files, stage each resolved file, and report success.
Do NOT commit.
Do NOT run `git merge --continue` — the SKILL does that after receiving `{"status":"success"}`.

## Task intent

These excerpts describe what THIS branch is trying to accomplish.
When the merge introduces a parent-side change that conflicts with this branch's intent, the resolution preserves THIS branch's intent.
In particular: if a file appears under a batch's `Deletes:` list and the merge introduces a modified version of that file from the parent, the resolution is to delete the file (your branch's intent overrides).
Stage the deletion with `git -C /home/knatte/Code/loomyard/wts/loom-session-bootstrap rm <file>`.

### From discussion.md

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
- `lyx run` as a second top-level cobra command aliasing `lyx loom run`, including its own disposition for the two guards that enumerate every root child.
- Status-file seeding on `lyx loom run` when `_lyx/loom/status.json` is absent, via `loomshed.Seed` plus a new `loomshed.ErrSeedExists` sentinel so a re-run can tolerate the already-seeded case — and an immediate weft-side commit of that seed, before the driver is spawned, so loom's own write does not fail loom's own cleanliness gate.
- A new narrow weft-commit helper, `fabricengine.CommitWeftPaths` — positive-pathspec, no push — shared by `Add`'s `origin.json` commit and `loomcli`'s seed commit.
- Full `websterengine.RunDeps` wiring in `loomcli`, so `loomshed.Deps.WebsterDeps` is a real production value rather than a zero struct.
- A new `parent_branch` provenance record — `_lyx/fabric/origin.json` — written **and committed on the weft branch** by `fabricengine.Topology.Add`, with `fabricengine`-owned path accessor and read/write functions, plus the `--parent` flag that writes it once for worktrees created before it existed.
- A third launcher script (`run.cmd`/`run.sh`) in the existing `<hub>/_launchers/<AnchorRel>/<slug>/` set.
- Driver stdout/stderr capture to a machine-local log.
- Registration/help/seam/sandbox test updates that adding a 13th module and a second root command force.
- Doc updates: `manifest/designs/loom.md`'s run-launcher paragraph, `docs/overview.md`'s module table and the root `Long` module list, `manifest/roadmap.md` item moved to Done, `contracts/specs/loom-status-spec.md`'s spawn-time-seed binding, and `CONSTRAINTS.md`'s CLI/Cobra Invariant seam counts.

**Out:**

- Any change to the producer list itself — all eight stub rows (`Discussion-Write`, `Discussion-Review`, `Plan-Sweep`, `Plan-Write`, `Plan-Review`, `Webster-Review`, `Publish`, `Finalize`, per `internal/loomshed/stub.go:11-16`) stay stubs.
  Replacing them is `loom: write and wire in the real LLM producers`.
- `Publish`/`Finalize` — those are `landing: Publish + Finalize producers`.
- `Plan-Sweep`, rubrics, prompt content, `perch`/`burler`/`webster` engine internals.
- The cross-worktree multi-column reed view (deferred reed feature; scope stays one terminal per worktree).
- `--auto` mode (`lyx run --auto`) — the phase machine's yielding behaviour is producer-level, and every producer that would yield is still a stub, so there is nothing for the flag to change yet.
- Any new `CONSTRAINTS.md` invariant — nothing here crosses a seam not already governed by CLI/Cobra, Durable-vs-Ephemeral, Told-Geometry, Cwd Resolution, Fabric Write-Side Containment, or Mutation Record.

## Decisions

### verb-surface

- Decision: `internal/loomcli` ships all four verbs — `run`, `drive`, `status`, `pause` — in this task.
- Rationale: `status` is a hard requirement of the bootstrap's own step 2 (the strand launches `lyx loom status --watch`; without the verb the pane is broken).
  `pause` is a ~20-line `state.UpdateJSON` flag flip against a machine that already honours the flag (`internal/shedengine/run.go:116`).
  Shipping three of four leaves `designs/loom.md`'s own named CLI (`lyx loom run|pause`, `lyx loom status`) half-built for no gain.
- Decision: `loomcli` carries `RunCLIIn` alongside `RunCLI`, joining the eleven modules that already do rather than `selfreportcli`'s no-`RunCLIIn` exception — loom resolves cwd through `lyxcwd` throughout, so a seeded cwd is meaningful to it, which is the exact criterion that exception rests on.
  `CONSTRAINTS.md`'s CLI/Cobra Invariant therefore moves from "Eleven of the twelve seam modules also carry `RunCLIIn`" to twelve of thirteen, and `cmd/lyx/seamsignature_test.go`'s repeated counts (`:1`, `:29`, `:46`) move with it.
- Rejected: deferring `pause` to a later task — the gap is smaller than the coordination cost of a follow-up item.
  Also rejected: omitting `RunCLIIn` — it would make `loomcli` a second exception to a rule that currently has exactly one, for no reason `selfreportcli`'s reason covers.

### drive-as-real-verb

- Decision: the detached driver is a real, documented verb, `lyx loom drive`, which runs the phase machine in the foreground with no tmux involvement.
  `lyx loom run` spawns `lyx loom drive` detached via `proc.Detach`.
- Rationale: matches `designs/loom.md`'s step 3 ("spawn the loom driver DETACHED — `internal/proc`; it needs no TTY") almost verbatim.
  It makes the bootstrap a thin composition of two independently-testable verbs and gives a first-class no-tmux escape hatch for debugging and CI.
- Decision: `drive` never seeds and never commits — those are `run`'s job, because only `run` owns the commit-before-Preflight ordering.
  On a pair with no `_lyx/loom/status.json`, `drive` refuses in its own pre-flight, on the envelope, with a message naming `lyx loom run`, rather than letting the operator discover it as `Preflight` check-4's `CheckSeedMissing` failure (`preflight.go:98`) written into `driver.log`.
- Rejected: a hidden `--driver` flag on `lyx loom run` — an undocumented flag would be the only way to run the machine without tmux, which is exactly the surface the CLI/Cobra Invariant's "`Short` on every command" discipline exists to prevent.
  Also rejected: running the machine in-process in a goroutine — step 4 hands stdio to tmux and blocks, so the driver would die with the operator's terminal.

### self-seeding

- Decision: `lyx loom run` seeds `_lyx/loom/status.json` itself when absent, via `loomshed.Seed`.
  `Seed` is **not** idempotent — `internal/loomshed/seed.go:43-46` returns an error when the file already exists, deliberately, so an in-flight run's history is never destroyed.
  So `Seed` gains an exported `ErrSeedExists` sentinel, and `lyx loom run` calls `Seed` unconditionally and treats exactly that sentinel as success.
- Rationale: `loomengine.Preflight` check-4 hard-fails when the seed is missing, so without this `lyx loom run` can never get past its own first producer.
  A sentinel keeps the decision inside the single lock `Seed` already holds — a stat-then-seed probe in `loomcli` would reintroduce the TOCTOU window `Seed`'s own doc comment says it exists to avoid, and a bare "call and ignore all errors" would swallow real write failures.
  Re-entrancy (`reentrancy-ensure-and-attach`) requires the second `lyx loom run` to succeed, which this delivers.
- Decision: the seed write is **committed weft-side immediately, before the driver is spawned**, via `fabricengine.CommitWeftPaths` (see `weft-commit-mechanism`).
  The same applies to a `--parent`-triggered `origin.json` write.
- Rationale: `fabricengine.Clean` runs `git status --porcelain` against the **weft sibling** as well as the warp (`warpclean.go:26-34`), untracked files included, and `seedWeftArtifactExcludes` covers only `.lyx/`, the `.weft/` lock dir, the push-lock and `*.lock`/`*.swaplock` — not `_lyx/loom/status.json`.
  So an uncommitted seed leaves the weft dirty, `preflight.Check`'s `CheckWorktreeClean` fails, and row 1 (`Preflight`, `OnStuck: ""`) goes Stuck straight to `StateBlocked` on the very first run — loom blocked by its own write.
  Committing before the spawn closes it.
  `Shed` re-dirties `status.json` on its first persist, which is harmless: `Preflight` runs once at row 1, and a resume re-enters at `current_producer`, never back at row 1.
- Decision: `contracts/specs/loom-status-spec.md` is updated in the same commit.
  It is a pinned Contract doc that today says the seed is written "before any `lyx loom run` has executed", while also stating that which command the role binds to "is pinned when that command lands".
  This task is that landing, so the spec's role binding is pinned to `lyx loom run`'s seed-if-absent and the "before any `lyx loom run`" clause is corrected.
- Rejected: seeding from `lyx fabric spawn` — points the coupling the wrong way (loom may depend on fabric, never the reverse) and leaves every already-spawned worktree unseedable.
  Also rejected: a separate operator-invoked `lyx loom seed` verb — a mandatory manual step defeats the one-double-click entry point this task exists to deliver.

### parent-branch-is-recorded-never-guessed

- Decision: the seed's `parent` value comes from a **recorded** fabric provenance file, never from inference.
  `fabricengine.Topology.Add` writes `_lyx/fabric/origin.json` carrying `parent_branch` at worktree-creation time; loom reads it.
- Rationale: `contracts/specs/loom-status-spec.md` makes `product.parent` mandatory, and a wrong parent silently mis-targets the eventual merge-back.
  `Add` is the one place that already knows the answer — it computes `parentBranch` at `internal/fabricengine/add.go:132` from `rev-parse --abbrev-ref HEAD` and currently discards it after deriving `parentWeftBranch`.
  The weft is the right home: `_lyx` is the tracked, fabric-synced weft side junctioned into the warp, so a **committed** record survives a fresh clone on another machine — which is what `designs/loom.md`'s "resume works across machines" property rests on.
- Rejected: defaulting to the Prime worktree's current branch via `fabricengine.PrimeName` + `gitrepo.Repo.CurrentBranch` — that is a guess, and it is wrong whenever the prime worktree has been switched or the pair was forked from a non-prime branch.
  Also rejected: a plain one-line `_lyx/.lyx-parent` marker (breaks with the JSON convention fabric already uses for its own records and has no room to grow), and a git-config entry `branch.<name>.lyxparent` (`.git/config` is machine-local and does not survive a fresh clone).

### origin-record-is-committed-and-is-a-new-class

- Decision: `Add` **commits** `origin.json` on the new pair's weft branch, after junction wiring (`add.go`'s step 10b) and before step 12's existing `pushWeftBranch`, so the existing push carries it to the remote with no new push call.
- Decision: this record is acknowledged as a **new class** — the first *tracked* fabric-owned record under `_lyx`.
  Its tracking and commit contract is stated here rather than borrowed from an existing file.
- Rationale: an uncommitted file under `_lyx` would contradict the Durable-vs-Ephemeral State Invariant's "`_lyx` holds tracked content only", and an uncommitted record cannot survive a fresh clone, which is the only reason the weft was chosen over `.git/config` in the first place.
  Committing inside `Add` keeps the record and the pair atomic **on the created-branch path**: `rollbackAdd` deletes the weft branch the Add created, taking the record commit with it.
- Decision: on the **adopted-weft-branch path** (`weftBranchAlreadyExists`), the record commit is deliberately **left in place** if a later step of `Add` fails.
  `rollbackAdd` passes `!weftBranchAdopted` to `removeWeftWorktree` (`add.go:243`) precisely so a rollback never destroys pre-existing weft history, and reverting or resetting an adopted branch to undo one commit would be exactly that destruction.
  The residue is harmless: the record's content is the correct provenance for that slug either way, and a later re-add of the same slug overwrites it.
  This is an accepted outcome, not an oversight — the integration test asserts "no stray commit" for the created-branch path only, and asserts the retained-commit outcome explicitly for the adopt path.
- Correction carried forward: `mergestate.go:72` and `corrindex.go` place their records in the weft **gitdir** via `f.weftGitDir()` — machine-local and never tracked.
  They are precedent for `state.WriteJSON` as a *mechanism*, not for a tracked `_lyx` file.
  An earlier draft of this discussion cited them as a tracking precedent; that citation was wrong.
- Decision: the commit call is `fabricengine.CommitWeftPaths` (see `weft-commit-mechanism`), not `Fabric.Commit` and not `commitWeftAt`.
- Rejected: putting the record in the weft gitdir like its two neighbours — machine-local, so `--parent` would become a mandatory one-time step on every new machine and every fresh clone rather than only for legacy worktrees, defeating the zero-argument launcher.
  Also rejected: writing it uncommitted and letting a later ordinary `lyx fabric commit` sweep it up — leaves a window in which the record exists locally but for nobody else, and violates the tracked-only rule for the duration.

### weft-commit-mechanism

- Decision: this task adds one narrow exported helper to `fabricengine`:

  ```
  CommitWeftPaths(weftPath, anchorRel string, relPaths []string, msg string, opts SyncOptions) (sha string, committed bool, err error)
  ```

  `relPaths` are **anchor-relative** (e.g. `_lyx/fabric/origin.json`); the helper performs the `ScopedPathspec(anchorRel, relPaths)` join itself, so no caller ever joins `AnchorRel` or names `_lyx` in a pathspec.
  It stages that **positive-only** list and commits through `gitrepo.New(weftPath).StageAndCommit(msg, ...)`.
  It performs **no push**.
- Decision: `CommitWeftPaths` **acquires the combined weft write lock itself** — it does not push that duty onto callers.
  It reaches the lock dir without a `Fabric` by calling the **already-existing** package-level `ensureWeftLockDirAt(weftPath)` (`weftgit.go:53`; `Fabric.ensureWeftLockDir` at `:46-48` is already a thin delegation to it, and `coalesce.go:101` already calls it directly), then takes `lock.AcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))` exactly as `Commit` (`commit.go:198-202`), `Pull` (`pull.go:359-363`), `Merge` (`merge.go:125`) and the merge lifecycle already do.
  No refactor of `Fabric.ensureWeftLockDir` is in scope — an earlier draft of this discussion proposed extracting a helper that was already there.
- Rationale: `commitWeftAt`'s own doc (`weftgit.go:333-336`) states it "does not acquire the weft write lock; the caller is responsible" — a contract that works for `Bolt`, whose single caller owns the board weft, but not here.
  loom's seed commit and a concurrent webster or `lyx fabric` weft commit in the same pair would otherwise race on `.git/index.lock`.
  Locking inside the helper means neither of its two callers can forget, and it matches every other committing path in the package.
- Decision: `SkipGit` returns `("", false, nil)` with no lock taken and nothing staged, exactly as `commitWeftAt` does — this is what lets `Add`'s existing `SkipGit` test paths thread their opts straight through.
  `SkipPush` is accepted and ignored, documented as such on the function, because the helper never pushes; `SyncOptions` is kept rather than narrowed to a bool so `Add` passes its own `opts` unchanged instead of destructuring them at the call site.
- Decision: two callers, both passing an anchor-relative path list.
  `Add` commits `origin.json` against the **new pair's** weft worktree (`WeftWorktreePath(l, slug)`) with `anchorRel = l.AnchorRel`, after junction wiring, so step 12's existing `pushWeftBranch` carries it — no extra push.
  `loomcli` commits the status seed (and any `--parent` write) against `fabricengine.WeftWorktree(l)` with `anchorRel = l.AnchorRel`, before spawning the driver; the ordinary `lyx fabric sync` carries it later.
  The seed's anchor-relative path comes from a new `loomengine.LoomStatusRel()` (built from `lyxdirs.LyxDirName`), so `loomcli` still constructs no `_lyx` path of its own.
- Rationale: neither existing path works here.
  `Fabric.Commit` is bound to one `warpPath`/`weftPath` pair and resolves its routing from `f.warpPath`, so it cannot address the *new* pair `Add` just created; it also fires `spawnDetachedPushFn` unconditionally whenever anything lands (`commit.go:109-166`), which contradicts "the existing push carries it with no new push call".
  `commitWeftAt` is `gitrepo.StageAllAndCommit` (`weftgit.go:337-341`) — a stage-all, which the Fabric Git Invariant's cross-module-exclusions rule forbids for `_lyx` commits: "every weft-commit caller passes a **positive-only** file list ... built via `fabricengine.ScopedPathspec`". `_lyx` is shared by every round-loop module, so a stage-all here would sweep up another module's in-flight state.
- Rationale: the helper stays inside `fabricengine`, so the Fabric Git Invariant's module-ownership rule holds — `loomcli` never runs git itself.
- Rejected: widening `Fabric.Commit` with a no-push option or a second pair — a behaviour change to the shared commit verb, for two callers that need neither its routing nor its push.
  Also rejected: letting `loomcli` shell out or call `gitrepo` directly — a direct Fabric Git Invariant violation.

### origin-record-ownership-seam

- Decision: `fabricengine` owns the path and both accessors, exported: `OriginRecordPath`, `ReadOrigin`, `WriteOrigin`.
  `loomcli` calls the functions and never constructs the path itself.
- Decision: the two sides use different anchors, deliberately, and **both are anchor-aware** — there are two path accessors, not one.
  `OriginRecordPath(l)` is the read side: `filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName, "fabric", "origin.json")`. `AnchorPath()` already carries `AnchorRel`, so loom reading through the warp junction is correct in a subpath-anchored hub with no extra join.
  `OriginRecordPathFor(l, slug)` is the write side: `filepath.Join(WeftWorktreePath(l, slug), l.AnchorRel, lyxdirs.LyxDirName, "fabric", "origin.json")` — the same `WeftWorktree → AnchorRel → _lyx` shape `WeftLyxDir` already uses (`fabric.go:148-151`). `Add` needs it because the new pair is not the acting worktree and its junction is not yet the caller's own.
- Rationale for two accessors: a single `WeftWorktreePath(l, slug)`-rooted path with no `AnchorRel` segment is wrong in any subpath-anchored hub, and making `loomcli` supply the join would contradict this same decision's "loom constructs no `_lyx` path" rule.
  Keeping both joins inside `fabricengine` is what satisfies the Cwd Resolution Invariant on both sides.
- Decision: `ReadOrigin`/`WriteOrigin` go through `state.ReadJSON`/`state.WriteJSON`, and the `lockPath` both require is **`<weftPath>/.weft/origin.json.lock`**, obtained by calling the existing `ensureWeftLockDirAt(weftPath)` and joining `origin.json.lock` onto its return.
  `weftPath` is the acting worktree's weft sibling for the read side and the new pair's weft worktree for the write side, matching each accessor's own anchor.
- Rationale: the package's usual `path + ".lock"` idiom (`mergestate.go:112`, `corrindex.go:36`) is safe only because those records live in the weft **gitdir**.
  A `.lock` file sitting beside a *tracked* `_lyx/fabric/origin.json` would put a never-tracked artifact inside `_lyx`, which the Durable-vs-Ephemeral State Invariant forbids outright.
  `.weft/` is already fabric's own lock directory, already git-excluded by `seedWeftArtifactExcludes` (named explicitly in the Fabric Git Invariant's cross-module-exclusions clause), so a fabric-owned lock there is the established placement rather than a new one.
- Rationale: this also settles lock-parent creation with no new write.
  `state.WriteJSON` creates only the *record's* parent directory, not the lock's — the gap `loomshed/seed.go:31` works around with its own `os.MkdirAll` — and a raw `os.MkdirAll(` in a new `fabricengine` file would trip `TestNoUncontainedWrite_FabricengineProductionSource`'s per-file allowlist.
  `ensureWeftLockDirAt` already `MkdirAll`s `.weft` inside an already-allowlisted file, so the lock's parent is created by code that exists.
- Rejected: `origin.json.lock` beside the record — violates Durable-vs-Ephemeral, and would need a new `MkdirAll` site that trips the containment guard.
  Also rejected: a `.lyx`-mirrored lock at `<weft>/<AnchorRel>/.lyx/fabric/origin.json.lock` — invariant-clean, but it needs a new parent-creating write in `fabricengine` for a directory `.weft` already provides.
- Rationale: `_lyx/fabric/` is a new fabric-owned module subdirectory, so per the Cwd Resolution Invariant its relative-path constant belongs to `fabricengine` alone.
  Giving loom a read accessor rather than a path keeps `loomcli` free of any `_lyx` path construction, which is what the invariant actually requires.
  `WriteOrigin` takes the `*Mutations` recorder so the write records `KindFileWritten` at its own success site, per the Mutation Record Invariant.
- Decision: on loom's side, `lyx loom run --parent` passes a throwaway `&Mutations{}` and discards it.
  Loom's envelope gains no `mutations`/`partial` keys.
  The Mutation Record Invariant binds *fabric verb outcomes* — `AddResult` and its siblings, which embed `MutationRecord` — not every caller of a function that happens to take a recorder, and loom's own result type is not a fabric verb result.
  `WriteOrigin` keeps the parameter regardless so `Add`, its real recording caller, threads its own `rec` through unchanged.
- Rejected: routing both sides through `WeftWorktreePath` — uniform, but loom would then need the slug and hub geometry to read its own record, which the junction hands it for free.
  Also rejected: `loomcli` building the path itself — a direct Cwd Resolution Invariant violation.

### missing-record-refusal

- Decision: in a worktree created before `origin.json` existed, `lyx loom run` refuses, names the missing record, and accepts `lyx loom run --parent <branch>` to write the record once.
  After that one write the launcher is zero-argument forever.
- Decision: `--parent` on a worktree that **already** has a record refuses unless the supplied value equals the recorded one (in which case it is a no-op).
  Changing a recorded parent is not a loom operation and there is no override flag.
- Rationale: the operator supplies the fact explicitly; nothing is inferred.
  New worktrees never need the flag because `Add` writes the record from now on.
  Silently overwriting a recorded provenance value would re-target the merge-back — the exact failure this whole decision exists to prevent — so the mismatch case must be loud.
- Rejected: refusing with no flag at all — no fabric verb can supply the parent for an existing worktree without guessing either, so old worktrees would be permanently unrunnable.
  Also rejected: seeding `parent: ""` — `loom-status-spec.md:77` makes `product.parent` mandatory and counts an empty string as absent.
  Also rejected: letting `--parent` overwrite freely, or adding a `--force-parent` — an override flag invites exactly the silent re-targeting the refusal is there to stop.

### webster-deps-wired-for-real

- Decision: `loomcli` builds a full `websterengine.RunDeps` and passes it as `loomshed.Deps.WebsterDeps`, mirroring `internal/webstercli/run.go:34`'s `runDeps()` construction (`Starter`, `Reed`, `Engine`, `ShuttleCfg`, `Roles`, `Config`, `Batcher`, `Geom`, `RefMatcher`, `OpenBisector`).
  `Deps.WebsterRun` stays nil, which `shedadapters.NewWebsterProducer` (`internal/shedadapters/webster.go:48-54`) documents as "defaults to `websterengine.Run`, the production seam".
- Decision: `RefMatcher` is pinned to `fabricengine.NewRefScanner(loc)`, built **eagerly** in loom's pre-run from the resolved `*lyxcwd.Location`, exactly as `webstercli/wiring.go:130` does — `RefScanner` is a type, and `NewRefScanner` only compiles a regexp and cannot fail, so eager construction is safe.
  Never `websterengine.NeverMatches`.
  The Fabric Git Invariant makes `RefScanner` mandatory for hub-mode webster runs and allows `NeverMatches` only in standalone, where there is no weft worktree and no fabric verb for a fork to drive.
  loom is hub-only (see "Mode question"), so `NeverMatches` would silently disable a hard, round-failing guard.
- Rationale: `Webster` is row 10 and is a **real** producer, not a stub (`loomshed.go:108`).
  Leaving `WebsterDeps` zero would hand `websterengine.Run` a struct with nil `Starter`/`Engine`/`Reed`/`Batcher`/`Geom` and an empty `Roles` map.
  That it is currently unreachable — the stub `Discussion-Write` returns `Done` (`stub.go:31-37`) and the real `Discussion-Validate` then fails and bounces back to it until `MaxBounces` is spent — is a fact about today's stub set, not a property to depend on; it evaporates the moment the real LLM producers land.
- Rejected: substituting a stub or refusing `WebsterRunner` for this task — it would make `lyx loom run` structurally unable to reach the implement phase, which contradicts this task's purpose of making the phase machine reachable, and it would have to be unpicked by the very next task.

### reentrancy-ensure-and-attach

- Decision: a second `lyx loom run` while a driver is alive ensures substrate and attaches, and never spawns a second driver.
  `lock.TryAcquireWriteLock(loomengine.LoomRunLock(l))` is the liveness probe: held ⇒ a driver is running, skip step 3; not held ⇒ release immediately and spawn.
  Steps 1, 2 and 4 are idempotent.
- Decision: the probe-and-spawn sequence is serialised by its own short-lived lock, `.lyx/loom/bootstrap.lock` (a third `loomengine` accessor beside `LoomStatusLock` and `LoomRunLock`), held with `lock.AcquireWriteLock` across steps 1–3 and released before the attach in step 4.
- Decision: the bootstrap lock alone is **not** sufficient, and is paired with an explicit **spawn handshake**.
  After `proc.Detach` + `Start` returns, and still holding `bootstrap.lock`, the spawner polls until it observes `LoomRunLock` as *held* — `lock.TryAcquireWriteLock(LoomRunLock)` reporting `locked == false` — checking `proc.IsAlive(childPid)` on each iteration, at a short interval under a bounded deadline.
  Only once the run lock is observed held does the spawner release `bootstrap.lock` and proceed to attach.
- Decision: the two failure exits of that poll are explicit.
  If `proc.IsAlive` reports the child gone before the run lock is ever observed held, or the deadline expires, the spawner releases `bootstrap.lock`, does **not** attach, and refuses on the envelope naming `.lyx/loom/driver.log` as where the reason is.
  The result is a clean state — no driver, no held lock, an operator who was told — rather than a silent half-start.
- Rationale: the bootstrap lock by itself still leaves the window open, because the run lock is taken by the **child**, at the top of `shedengine.Run` (`run.go:56`), only after `lyx loom drive` has parsed flags, resolved cwd, loaded config and built the whole `RunDeps` stack.
  Caller A's `Start` returns long before that, so A could release `bootstrap.lock` while its child had not yet taken `LoomRunLock`; B would then acquire the bootstrap lock, probe the run lock free, and spawn a second driver — the exact `ErrShedBusy`-into-`driver.log` failure this decision exists to prevent.
  Holding the bootstrap lock until the run lock is *observed* held is what actually closes it.
- Rejected: having `lyx loom drive` take `LoomRunLock` itself before its wiring, so the lock is held by the time `Start` returns — `shedengine.Run` acquires that same lock unconditionally and would then deadlock against its own caller.
- Rationale: `bootstrap.lock` is released before step 4 because attach blocks for the operator's whole session and must not hold a lock.
- Rationale: makes the double-click launcher safe to hit repeatedly, and gives the operator a way to reattach after closing the terminal — the common case.
- Rejected: refusing when the lock is held — leaves no reattach path.
  Also rejected: always spawning — the second detached driver would die immediately on `Shed`'s own `LockPath`, and because it is detached with its output going to `.lyx/loom/driver.log` rather than into a pane, the failure would be invisible on screen: the operator sees an apparently-normal attach with nothing new happening, and the only evidence is in the log file.

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
- Decision: `removeLaunchers`'s hardcoded script-name list is an explicit, non-optional edit point, called out here because omitting it is worse than a leak.
  `launchers.go:253` iterates a literal `{"ide"+ext, "fabric-checkout"+ext}` and then removes the launcher directory **non-recursively**, so a `run<ext>` left off that list makes the directory non-empty and `Remove`/`rollbackAdd` fail outright rather than merely orphaning a file.
- Rationale: strictly reuse — the mechanism is already cross-platform, `os.Root`-contained, mutation-recorded, and torn down.
  The relative-path climb (`%~dp0` / `$(dirname $0)`) means no absolute path is embedded, so nothing is machine-bound.
- Rejected: `.lyx/lyxrun.cmd` in the worktree as `designs/loom.md` currently says — that text describes a mechanism that does not exist in this repo; it would need a new write path, a new teardown obligation, and there is no double-click surface pointing at it.
  The doc paragraph is stale and is rewritten in this same commit per the Documentation Lifecycle.

### run-alias-as-registered-command

- Decision: `lyx run` is a second top-level cobra command registered from `loomcli` into `newRoot()`, sharing `loom run`'s RunE builder and pre-run resolution.
  Its exported constructor is `loomcli.RunAliasCommand() *cobra.Command`, beside the module's own `Command()`.
- Decision: one module contributing two root children is a **new seam shape**, so the CLI/Cobra Invariant gains a one-line clause covering it in the same commit — a module may register an alias command beside its subtree, via a separately-named exported constructor, and that alias carries no `RunCLI`/`RunCLIIn` of its own because it delegates into the subtree's verb.
  Without the clause the invariant's "Every lyx CLI module is a cobra subtree assembled under one root" reads as forbidding this.
- Decision: because it is a root child, two guards see it as a "module" and both get an explicit disposition in this task: it is named in `newRoot()`'s `Long` "Available modules:" list (`cmd/lyx/longlist_test.go:18-26` requires it), and it gets an `excludedModules` entry in `cmd/lyx/sandbox_coverage_test.go` reading "alias of `lyx loom run`; covered by the `loom` scenario" rather than a duplicate `**Covers:** run` scenario.
- Rationale: staying inside the module `Command()`/`RunCLI` seam keeps the alias inside `helptree_test.go`/`registration_test.go` coverage and discoverable in `--help`.
  A separate sandbox scenario for `run` would exercise byte-identical behaviour to the `loom` one; the allowlist with a truthful reason is what that mechanism is for.
- Rejected: argv rewriting in `cmd/lyx/main.go` (splice `loom` in front of a leading `run`) — invisible to `--help` and outside every module's own seam.

### fabric-change-stays-in-this-task

- Decision: the `origin.json` write, its commit, and its accessors land in this task, not a separate fabric task.
- Rationale: it is one new record type, one write, one commit on an already-pushed branch, one rollback entry and one `KindFileWritten` mutation inside an already-transactional verb — no new module, no new invariant.
  Splitting it out would add a fourth parallel coordination surface (alongside `landing`, this task, and `fabric-merge-crucible-hardening`) for a change driven entirely by loom's own need.
- Rejected: a standalone fabric task.
- Caveat carried forward: because this touches a chokepoint-guarded write point, that part of the diff gets extra verification focus in this task's own review — ordinary thoroughness on that commit, not a separate crucible round.

## Technical context

**What already exists and must be reused, not rebuilt:**

- `internal/loomshed.New(Deps) (*shedengine.Shed, error)` — the 13-row producer list. `Deps` is told absolute paths only: `StatusPath`, `LockPath`, `StatusLockPath`, `MaxBounces`, `AnchorPath`, `WorktreeRoot`, `DecisionRecordPath`, `SupportLogPath`, `Preflight`, `WebsterRun`, `WebsterDeps`.
- `internal/loomshed.NewPreflightProducer(cwd string) shedengine.ShedProducer` (`preflight.go:34`) — the adapter `loomcli` passes as `Deps.Preflight`. `loomengine.Preflight` itself is `func(cwd string) (Report, error)` and is **not** a `ShedProducer`; do not pass it directly.
- `internal/loomshed.Seed(statusPath, statusLockPath, slug, parent) error` — **refuses** when the file exists (`seed.go:43-46`); it does not no-op. This task adds the `ErrSeedExists` sentinel described under `self-seeding`.
- `internal/loomengine` path accessors, all `AnchorPath`-anchored, and per the Cwd Resolution Invariant the **only** legal constructors for these paths: `LoomStatusFile`, `LoomStatusLock` (under `.lyx`), `LoomRunLock` (under `.lyx`), `DiscussionDecisionRecord`, `DiscussionSupportLog`, `DiscussionDir`.
  `LoomRunLock` must never equal `LoomStatusLock` — `Shed.validate()` rejects that outright.
  The new `.lyx/loom/driver.log` and `.lyx/loom/bootstrap.lock` each need their own accessor here, not an inline path.
- `internal/loomengine.LoadConfig(baseDir, module)` — loom's `loom.yaml`; note it maps a `not initialized` error onto `run "lyx fabric reconcile"`.
- `internal/shedengine.Shed.Run(ctx) (Result, error)` — persists `CurrentProducer`/`State`/`Error`/`Activity`/`History` on every step and consumes `PauseRequested` at the loop head.
- `internal/shedengine.Activity` — composed mechanically by `composeActivity`; this is what `status --watch` renders as one line.
- `internal/shedadapters.NewWebsterProducer(name, run, deps)` — a nil `run` defaults to `websterengine.Run` (`webster.go:48-54`), so `Deps.WebsterDeps` must be a real value.
- `internal/webstercli/run.go:34` `runDeps()` — the construction `loomcli` mirrors for `RunDeps`. Note `OpenBisector` must stay nil when there is no fabric opener rather than being wrapped in a non-nil closure over a nil opener.
- `internal/reedengine.Engine`: `Up() (UpResult, error)` (documented no-op when already up), `AddStrand(AddSpec) (Strand, error)`, `Status() (StatusResult, error)` returning `[]StrandStatus{GUID, Name, PaneID, Live}`, `Socket()`, `SessionName()`, `TmuxPath()`.
- `internal/reedcli/attach.go` — the exact in-place `tmux attach-session` handover pattern (`attachArgv`, pre-flight `Status()` on the envelope, then stdio handover with exit-code propagation). Step 4 should follow it rather than invent one.
- `internal/reedcli/add.go` — the `AddSpec` shape for a `below-parent` strand with `ShrinkWhenWaitingOnChild: true`.
- `internal/proc.Detach(cmd)` / `DetachBreakaway(cmd)` / `IsAlive(pid)` / `KillPID(pid)` — cross-OS; `internal/proc` is the sole allowlisted spawn-in-untagged-tests package.
- `internal/lock.TryAcquireWriteLock(lockPath) (*FileLock, bool, error)` — non-blocking, reports `(nil, false, nil)` when held; not an error.
- `internal/state.ReadJSON` / `WriteJSON` / `UpdateJSON` — lock-guarded JSON state.
- `internal/fabricengine.ScopedPathspec(relPath, dirs)` (`fabric.go:158`) and `internal/gitrepo.Repo.StageAndCommit(msg, files)` (`gitrepo.go:96`) — the two pieces the new `CommitWeftPaths` composes.
  Note `StageAndCommit` returns `("", false, nil)` for an empty file list, so the helper must not treat "nothing staged" as success-with-a-commit.
  `ScopedPathspec` at `relPath == "."` returns `dirs` unchanged, so the anchor join is a no-op in a root-anchored hub and correct in a subpath-anchored one — no branch needed.
- `internal/fabricengine.WeftLyxDir(l)` (`fabric.go:148-151`) — `WeftWorktree(l) / l.AnchorRel / _lyx`. This is the shape `OriginRecordPathFor(l, slug)` mirrors for a non-acting worktree, and the proof that a bare `WeftWorktreePath(l, slug)` root is wrong.
- `ensureWeftLockDirAt(weftPath)` (`weftgit.go:53`) + `weftWriteLockFile` — the existing no-`Fabric` form of the weft lock-dir accessor, already used by `coalesce.go:101`, and the combined write lock every committing path takes (`commit.go:198-202`, `pull.go:359-363`, `merge.go:125`, `mergelifecycle.go:115/165`). `CommitWeftPaths` and the origin-record accessors call it as-is; nothing about it changes.
  It also `MkdirAll`s the `.weft` directory itself, which is what makes it the lock-parent creator for the new record too.
- `internal/fabricengine.Fabric.Commit` and `commitWeftAt` — both deliberately **not** used here; see the `weft-commit-mechanism` decision for why (pair binding + unconditional detached push; stage-all barred by the positive-only rule).
- `internal/fabricengine.Clean(l)` (`warpclean.go:20-47`) — checks warp **and** the weft sibling with `git status --porcelain`, untracked included, and reports weft dirt as "uncommitted state changes under `_lyx`". This is what makes the seed-commit ordering mandatory.
- `internal/fabricengine.seedWeftArtifactExcludes` (`weftgit.go:83`) — excludes `.weft/`, the push-lock, `.lyx/`, and `*.lock`/`*.swaplock` only. `_lyx/loom/status.json` is **not** excluded and never will be; it is tracked content.
- `internal/shedengine.ErrShedBusy` (`run.go:56-62`) — `Run` itself takes `LockPath` with `TryAcquireWriteLock` and fails fast when held. This is the backstop the bootstrap lock keeps the operator from ever hitting.
- `internal/hubgeom` — the hub-mode geometry adapter; `internal/standalonegeom` is its told-mode twin.
- `internal/clihelp` — `Execute`, `ExecuteIn`, `GroupRunE`, `Abort`, `ShouldAbort`, `SetExit`, `InstallJSONHelp`.
- `internal/output` — `Ok`/`Err` envelopes.

**Wiring precedent to follow:** `internal/perchcli` is the closest analogue — a `PersistentPreRunE` that resolves cwd once, a `wiring.go` that builds the engine stack onto a CLI receiver, and per-verb construction of anything whose inputs are only known after flag parsing.
`internal/reedcli` is the precedent for the cobra group shape and the `cmd.Name() == "<group>"` guard that lets a bare group listing work outside a git repo.
`internal/webstercli`'s pre-run is the precedent for the `RunDeps` half of the wiring.

**Mode question:** loom needs a wired fabric (Preflight validates warp/weft pairing and sync), so it is hub-only — its pre-run resolves via `lyxcwd.Resolve` like `reedcli`, not `preflight.ResolveMode`'s degrade-to-standalone path that `perchcli` uses.

**Seed inputs:** `slug` is `lyxcwd.Location.WorktreeName`.
`parent` is `fabricengine.ReadOrigin`'s `parent_branch`.

**The fabric change, concretely:** `internal/fabricengine/add.go` already computes `parentBranch := strings.TrimSpace(headStdout)` at line 132 and uses it only to derive `parentWeftBranch`.
The new record stores that same value.
Placement in `Add`'s sequence: after the junction wiring at step 10b (so the pair is fully wired) and before step 11/12's pushes, so the weft-side commit rides step 12's existing `pushWeftBranch` and no new push call is introduced.
`rollbackAdd` gains **no new step**: it already removes the whole weft worktree via `removeWeftWorktree` (`add.go:238-247`), and deletes the weft branch when this `Add` created it, so the record and its commit go with them.
A separate record-removal call would be a no-op on the created-branch path and a history-destroying act on the adopted one — see the `origin-record-is-committed-and-is-a-new-class` decision for the adopt-path disposition.
`WriteOrigin` appends `KindFileWritten` to `rec` **after** the write observably succeeds, per the Mutation Record Invariant.
`state.WriteJSON` lives in another package, so the raw-write tokens the Fabric Write-Side Containment guard scans for (`os.WriteFile(` etc.) do not appear in `fabricengine` source — the same reason `mergestate.go`'s existing `state.WriteJSON` call passes that guard.

**Registration fallout of a 13th module plus a second root command** — all of these are machine-enforced and will fail until updated:

- `cmd/lyx/main.go`: import, `root.AddCommand(loomcli.Command())`, the `lyx run` alias command, and the root `Long` "Available modules:" list — which must name **both** `loom` and `run` (`longlist_test.go:18-26` iterates every non-help/completion root child).
- `cmd/lyx/seamsignature_test.go` — pins `RunCLI` across "all twelve modules" and `RunCLIIn` across eleven; both counts move to thirteen and twelve, and `CONSTRAINTS.md`'s CLI/Cobra Invariant text moves with them in the same commit.
- `cmd/lyx/registration_test.go`, `helptree_test.go`, `drift_test.go` (non-empty `Short` on every command).
- `cmd/lyx/sandbox_coverage_test.go` — iterates every non-help/completion root child (`:40-47`), so it sees `loom` *and* `run`. `loom` gets a `**Covers:** loom` scenario in a `tools/sandbox/*SUITE.md` file; `run` gets an `excludedModules` entry with the reason "alias of `lyx loom run`; covered by the `loom` scenario".
- `cmd/lyx/notransients_test.go` / `constructoranchoring_test.go` — the new `.lyx/loom/driver.log` path must be constructed by a `loomengine` accessor, not built inline.

**Gotcha — strand identity:** `reedengine.AddStrand` has no upsert semantics; a second call appends a second strand.
Re-entrancy requires looking the status strand up by a fixed `NameOverride` in `eng.Status().Strands` before adding.

**Gotcha — no `--watch` precedent:** nothing in the repo currently implements a polling watch loop, and the Test Tier Purity Invariant flags a constant `time.Sleep(...)` of ≥ 1s in an untagged test file.
The poll interval belongs behind a variable or a flag so tests can drive it fast.

**Gotcha — stub rows return `Done`:** `stub.go:31-37` reports `Done` unconditionally, so the machine walks through every stub row rather than stopping at it.
Do not build any behaviour on "the machine cannot reach row N today".

## Constraints

From `CONSTRAINTS.md`, the ones this task is bound by:

- **Cwd Resolution Invariant** — `internal/lyxcwd` owns cwd resolution alone; every loom path comes from a `loomengine` accessor and the origin-record path from a `fabricengine` accessor, never assembled in `loomcli`.
- **CLI / Cobra Invariant** — `Command()` + `RunCLI(out, args) int` (+ `RunCLIIn`), `Short` on every command, JSON envelopes via `internal/output`, `RunE` checks `ShouldAbort` **first**, one envelope per invocation, parent group sets `RunE = clihelp.GroupRunE`.
  `status --watch` and `run`'s attach tail both take the narrow interactive-handoff / self-displays-then-blocks exception; everything fallible stays pre-flight.
- **Told-Geometry Invariant** — `loomshed` is machine-enforced against importing `internal/lyxcwd` (`seam_enforcement_test.go`'s `TestToldGeometryInvariant_AllowlistOnly`). `loomcli` is the adapter that resolves the Location and threads plain values down.
- **Durable-vs-Ephemeral State Invariant** — `driver.log` and the lock files go under `.lyx/loom/` at the mirrored subpath of `_lyx/loom/`; `_lyx` holds tracked content only, which is why `origin.json` is committed rather than left loose.
- **Lyxdirs Single-Declarer Invariant** — never write the literals `_lyx` / `.lyx`; use `lyxdirs.LyxDirName` / `lyxdirs.DotLyxDirName`.
- **Mutation Record Invariant** — `WriteOrigin` appends `KindFileWritten` to `rec` only after it observably succeeded, and `AddResult` already embeds `MutationRecord`.
- **Fabric Write-Side Containment Invariant** — no `os.`-qualified raw write tokens in `fabricengine` production source.
- **Fabric Destruction Chokepoint Invariant** — no new removal site is introduced; the record is cleaned up by `rollbackAdd`'s existing gated weft-worktree/branch removal, so nothing new lands at a chokepoint-guarded site.
- **Test Tier Purity Invariant** — untagged tests spawn nothing (no `exec.Command`, no `gitexec.Run`, no `hubforge.NewHub`); the tmux/detach work is `//go:build smoke`.
- **Hermetic Git Test Environment Invariant** — any new git-spawning test package needs a `TestMain` calling `gitkit.HermeticGitEnv()`.
- **Sandbox Suite Coverage** — `loom` gets a `**Covers:** loom` scenario; the `run` alias gets an `excludedModules` entry with a reason.
- **Documentation Lifecycle** — the doc updates listed in Scope land in the same commit.

## Testing

Three layers, deliberately.

**Tier 1 (untagged, no spawns) — `internal/loomcli`:**

- `loomshed.Deps` assembly from a fake `lyxcwd.Location`: every path field equals the corresponding `loomengine` accessor's output, `LockPath != StatusLockPath`, and `Deps.Preflight` is the `NewPreflightProducer` adapter rather than a bare `Preflight`.
- `RunDeps` assembly: every field `webstercli.runDeps()` fills is non-zero, `RefMatcher` is a non-nil `*fabricengine.RefScanner` from `NewRefScanner(loc)` (never `NeverMatches`), and `OpenBisector` is nil exactly when there is no fabric opener.
- Seed handling: a fresh path seeds; a path whose file exists returns `ErrSeedExists` and `lyx loom run` treats it as success; any other `Seed` error propagates. **TDD candidate.**
- Seed-input resolution: slug comes from `WorktreeName`; parent comes from `ReadOrigin`; a missing record produces the refusal message naming `--parent`; `--parent` on a recordless worktree writes it; `--parent` matching an existing record is a no-op; `--parent` conflicting with an existing record refuses and names both values. **TDD candidate.**
- Re-entrancy decision function — a pure predicate over "run lock held?" returning spawn-or-skip, so the branch is testable without a real lock or process. **TDD candidate.**
- The spawn-handshake poll as a pure function over injected `lockHeld()` / `alive()` / clock seams: returns ready on lock-observed-held, `child-died` when `alive()` goes false first, and `deadline` when neither happens — no real process, no real lock, no wall-clock sleep (Test Tier Purity forbids a constant `time.Sleep` ≥ 1s here). **TDD candidate.**
- Status-strand lookup: given a `[]StrandStatus`, decide add-or-reuse by fixed name. **TDD candidate.**
- One-line `Activity` rendering for `status --watch`: given a `shedengine.Status`, produce the exact line. **TDD candidate.**
- Bare `lyx loom status` envelope shape, and the missing/corrupt status-file error paths.
- `lyx loom pause` sets `pause_requested` true and leaves every other field untouched; pausing an absent status file errors rather than creating one.
- `lyx loom drive` argument/flag validation up to the point of engine construction.
- `lyx run` and `lyx loom run` resolve to the same RunE behaviour, and both carry a non-empty `Short`.

**Tier 1 — `internal/fabricengine`:** the `Origin` record's encode/decode round-trip and `OriginRecordPath`'s derivation on both anchors (junction-side read path, weft-side write path), plus `CommitWeftPaths`'s pathspec construction (positive-only, `ScopedPathspec`-built, `anchorRel` joined for both `"."` and a subpath anchor, empty list is a no-op not a commit, `SkipGit` returns uncommitted) — all kept git-free so the files stay untagged, the same discipline `corrindex.go`'s tests already follow.

**Integration (`//go:build integration`) — `internal/fabricengine`:**

- `Add` writes `origin.json` with the correct `parent_branch` for a pair forked from a non-default branch, and lands it at the anchor-relative path in a **subpath-anchored** hub, not only a root-anchored one.
- `CommitWeftPaths` serialises against a concurrently-held weft write lock rather than racing it.
- The record is **committed** on the weft branch and present in that branch's tree, not merely on disk.
- A forced post-write failure on the **created-branch** path rolls the record back and leaves no stray commit.
- A forced post-write failure on the **adopted-branch** path leaves the record commit in place on the preserved pre-existing weft branch, and leaves that branch otherwise untouched — the accepted outcome, asserted rather than merely tolerated.
- The `Mutations` snapshot contains exactly one `KindFileWritten` for it.
- `removeLaunchers`/`Remove` leave nothing of the record or the `run<ext>` launcher behind, and the non-recursive launcher-directory removal still succeeds — the regression guard for the hardcoded script-name list.
- **Live-state matrix:** `Add` is a matrix verb at both anchors (`livestate_verbs_test.go:480`) and every cell runs `AssertRecordMatchesDiff` (`livestate_matrix_test.go:242`) in both directions.
  The expectation is that it passes **unchanged**: the record's single `KindFileWritten` entry must account for the new file, its commit, and the second path the warp junction exposes for the same file.
  If the oracle sees the junction-side path as a separate creation, the adjustment is enumerated in the plan rather than absorbed by loosening the assertion.

This is the chokepoint-guarded part of the diff and gets the extra verification focus agreed in the `fabric-change-stays-in-this-task` decision.

**Smoke (`//go:build smoke`) — the bootstrap itself, the part nothing else can catch:**

- `lyx loom run` in a real wired hub: tmux session up, exactly one status strand present with the fixed name, a detached driver PID alive, and the status file seeded.
- A second `lyx loom run` while the first driver holds the run lock: still exactly one strand, still exactly one driver PID, no new process, and `Seed`'s `ErrSeedExists` did not surface as a failure.
- `lyx loom drive` standalone with no tmux at all, on a pair the fixture seeded by running `lyx loom run` once and then killing the driver: the machine advances and the status file records it.
- `lyx loom drive` on a never-seeded pair: refuses on the envelope naming `lyx loom run`, writes no `driver.log`, and leaves the weft clean.
- Driver failure before the first persist: `.lyx/loom/driver.log` is non-empty and names the failure.
- `run.sh`/`run.cmd` exists in `<hub>/_launchers/<AnchorRel>/<slug>/` after `lyx fabric add`, and is gone after the matching remove.
- **The cleanliness ordering:** in a freshly-added, never-run pair, `lyx loom run` reaches past `Preflight` rather than blocking — the regression test for the round-2 blocker. `fabricengine.Clean` reports clean immediately after the seed commit, and the weft has exactly one new commit touching only `_lyx/loom/status.json`.
- **The spawn handshake:** two `lyx loom run` invocations started concurrently produce exactly one driver process and no `ErrShedBusy` line in `driver.log` — the round-4 regression test.
- **The handshake's failure exit:** a `drive` binary rigged to die immediately makes `lyx loom run` refuse on the envelope naming `driver.log`, hold no lock afterwards, and not attach.

**Sandbox:** one scenario in a `tools/sandbox/*SUITE.md` file tagged `**Covers:** loom`, exercising `lyx loom status` and `lyx loom pause` — the two verbs that need no tmux.
The scenario reaches a seeded state by **writing `_lyx/loom/status.json` as a hand-written fixture** before invoking either verb.
No shipped verb seeds one without going through `lyx loom run`'s tmux bootstrap, and `pause` on an absent file is specified to error, so the fixture is the only tmux-free route.
The scenario asserts the fixture's own values round-trip through `status`, which also pins the envelope against `loom-status-spec.md`.

**Scenarios that must be covered somewhere:** missing `origin.json`; `--parent` conflicting with a recorded value; a status file present but incoherent (Preflight check-4's half-finished case); attach failing because tmux is absent; the run lock held by a *dead* process (stale lock).

## Q&A log

- **Q:** Which verbs does `internal/loomcli` ship in this task? **A:** All four — `run`, `drive`, `status`, `pause`. `status` is a hard requirement of bootstrap step 2 and `pause` is a ~20-line flag flip; deferring either leaves the designed CLI half-built.
- **Q:** How does the detached driver get launched? **A:** A real `lyx loom drive` verb spawned via `proc.Detach`. A hidden `--driver` flag is exactly the surface the CLI/Cobra Invariant's `Short`-on-every-command rule exists to prevent.
- **Q:** Who seeds `_lyx/loom/status.json`? **A:** `lyx loom run` itself. Seeding from `lyx fabric spawn` would point the coupling backwards and strand existing worktrees.
- **Q:** Where does the run-launcher live? **A:** A third `run<ext>` script in the existing `<hub>/_launchers/<AnchorRel>/<slug>/` set — verified against `writeLaunchers`/`removeLaunchers`, not just the doc. `designs/loom.md`'s `.lyx/lyxrun.cmd` text is stale and is corrected in the same commit.
- **Q:** How is the `lyx run` alias wired? **A:** A second registered cobra command from `loomcli`, so it stays inside the seam and inside help-tree test coverage. argv-splicing in `main.go` would be invisible to `--help`.
- **Q:** Where does the seed's `parent` come from? **A:** A recorded fabric provenance file — never inference. The operator's explicit correction: it must *know* the parent branch, not guess it. Defaulting to the prime worktree's current branch was rejected for that reason.
- **Q:** What shape is that record and where does it live? **A:** `_lyx/fabric/origin.json` on the weft side, written by `fabricengine.Topology.Add`. A `.git/config` entry would not survive a fresh clone.
- **Q:** What about worktrees created before the record existed? **A:** Refuse, name the missing record, and accept `--parent <branch>` to write it once. `parent: ""` is not an option — `loom-status-spec.md:77` makes it mandatory.
- **Q:** Second `lyx loom run` while a driver is alive? **A:** Ensure-and-attach, never double-spawn; `lock.TryAcquireWriteLock` on `LoomRunLock` is the liveness probe. Erroring would leave no reattach path; blindly respawning would leave a second process dying on `Shed`'s lock with the failure visible only in `driver.log`.
- **Q:** `lyx loom status` output contract? **A:** Bare call emits a normal JSON envelope; `--watch` takes the documented self-displays-then-blocks-forever exception and prints the 1-line `Activity`.
- **Q:** How does a detached driver failure surface? **A:** `.lyx/loom/driver.log` plus the status file's own `state: error`. The log is necessary because a crash before the first persist would otherwise be completely silent.
- **Q:** Should the fabric change be split into its own task? **A:** No — one record type inside an already-transactional verb, driven by loom's own need. Splitting it would add a fourth parallel coordination surface. The chokepoint-guarded part of the diff gets extra verification focus within this task instead.
- **Q:** Test strategy? **A:** Tier-1 units for wiring and pure decisions, an integration test for the `Add` write/commit plus rollback, a smoke test for the tmux/detach interaction (the only place it can be caught), and a sandbox scenario for module coverage. Adding `loom` to `excludedModules` is not acceptable for a newly registered module.
- **Q (review r1, BLOCKING):** Does `Add` commit `origin.json`, or does the clone-survival rationale go? **A:** Commit it on the weft branch inside `Add`, after junction wiring and before the existing `pushWeftBranch`, so the record is tracked (as `_lyx` requires) and genuinely survives a fresh clone.
- **Q (review r1, BLOCKING):** How is the precedent stated, given `mergestate.go` lives in the weft gitdir? **A:** Acknowledge this as a new class — the first tracked fabric-owned record under `_lyx` — and state its commit contract outright. The earlier `mergestate.go` citation was factually wrong and is corrected in place.
- **Q (review r1, BLOCKING):** Who owns the origin-record path and its read/write functions, and which side does each use? **A:** `fabricengine` owns `OriginRecordPath`/`ReadOrigin`/`WriteOrigin`; loom calls the functions only. Loom reads through the warp junction at `AnchorPath()`; `Add` writes to `WeftWorktreePath(l, slug)`, because during `Add` the new pair is not the acting worktree.
- **Q (review r2, BLOCKING):** Loom's own seed write dirties the weft, and `Preflight`'s cleanliness check then blocks the run — how is that ordered? **A:** `lyx loom run` commits the seed weft-side before spawning the driver. `Clean` scans the weft including untracked files and `_lyx/loom/status.json` is not excluded, so an uncommitted seed makes row 1 go Stuck to `StateBlocked` on the very first run. `Shed` re-dirtying the file on its first persist is harmless — `Preflight` runs once, and resume re-enters at `current_producer`.
- **Q (review r2, BLOCKING):** What concrete call makes `Add`'s weft-side commit? **A:** A new `fabricengine.CommitWeftPaths` — positive pathspec via `ScopedPathspec`, `gitrepo.StageAndCommit`, no push. `Fabric.Commit` is bound to one pair and always fires a detached push; `commitWeftAt` is a stage-all the Fabric Git Invariant forbids for `_lyx`.
- **Q (review r2, NIT):** `loom-status-spec.md` says the seed is written before any `lyx loom run` — what is its disposition? **A:** Updated in the same commit. The spec explicitly defers which command the spawn-time role binds to "when that command lands"; this task is that landing, so the binding is pinned to `lyx loom run` and the contradicting clause corrected.
- **Q (review r2, NIT):** The run-lock probe releases before spawning — does that not leave a double-spawn window? **A:** It does, so a `.lyx/loom/bootstrap.lock` held across probe-and-spawn serialises it, released before the blocking attach. The window is closed, not accepted.
- **Q (review r2, NIT):** How does the sandbox scenario obtain a seeded status file? **A:** It writes one as a hand-written fixture; no tmux-free verb seeds one, and `pause` on an absent file errors by design.
- **Q (review r2, NIT):** What is `RefMatcher` in loom's `RunDeps`? **A:** `fabricengine.RefScanner`. loom is hub-only, and the Fabric Git Invariant permits `NeverMatches` only in standalone.
- **Q (review r3, BLOCKING):** Does `CommitWeftPaths` take the weft write lock, or must callers? **A:** It takes the lock itself, via the already-existing package-level `ensureWeftLockDirAt(weftPath)`. `commitWeftAt`'s caller-responsible contract suits `Bolt`'s single owner, not two independent callers who would otherwise race on `.git/index.lock`.
- **Q (review r3, BLOCKING):** Where is `AnchorRel` joined for the new weft write path and commit pathspec? **A:** Both inside `fabricengine`. `CommitWeftPaths` gains an `anchorRel` parameter and does the `ScopedPathspec` join itself; the origin record gets two accessors, `OriginRecordPath(l)` (read, via `AnchorPath()`) and `OriginRecordPathFor(l, slug)` (write, `WeftWorktreePath/AnchorRel/_lyx/...`, mirroring `WeftLyxDir`). A bare `WeftWorktreePath` root was wrong in any subpath-anchored hub.
- **Q (review r3, NIT):** What happens to the record commit when `Add` fails on the adopted-weft-branch path? **A:** It is left in place, deliberately — `rollbackAdd` preserves pre-existing weft history by design, and the record's content is correct provenance either way. The integration test asserts the retained-commit outcome for that path rather than "no stray commit".
- **Q (review r3, NIT):** Does `loomcli` carry `RunCLIIn`, and what about the pinned seam counts? **A:** Yes — loom resolves cwd throughout, so a seeded cwd is meaningful, which is the criterion `selfreportcli`'s exception rests on. `CONSTRAINTS.md` and `seamsignature_test.go` move to twelve-of-thirteen in the same commit.
- **Q (review r3, NIT):** What do `SkipGit`/`SkipPush` mean on a helper that never pushes? **A:** `SkipGit` returns `("", false, nil)` with no lock taken, as `commitWeftAt` does; `SkipPush` is accepted and ignored, documented on the function. `SyncOptions` is kept so `Add` threads its own opts through unchanged.
- **Q (review r4, BLOCKING):** Does the bootstrap lock actually close the double-spawn window? **A:** No — the run lock is taken by the child at the top of `shedengine.Run`, long after `Start` returns. So the spawner now holds `bootstrap.lock` until it *observes* `LoomRunLock` held (bounded poll, checking `proc.IsAlive` each iteration), and refuses on the envelope naming `driver.log` if the child dies or the deadline passes. Pre-taking the lock in `drive` was rejected: `Shed.Run` takes the same lock unconditionally and would deadlock.
- **Q (review r4, NIT):** Which rollback contract for `origin.json` is the real one? **A:** `rollbackAdd` gains no new step — its existing weft-worktree/branch removal already covers the record. The contradicting "must also unwind the record" sentence is deleted, and no new chokepoint-guarded removal site is introduced.
- **Q (review r4, NIT):** What does loom pass for `WriteOrigin`'s `*Mutations`? **A:** A throwaway `&Mutations{}`, discarded. The Mutation Record Invariant binds fabric verb outcomes, not every caller of a recorder-taking function, so loom's envelope is unaffected.
- **Q (review r4, NIT):** Is `fabricengine.RefScanner` callable as written? **A:** No — it is a type. The spelling is `fabricengine.NewRefScanner(loc)`, built eagerly in loom's pre-run as `webstercli` does.
- **Q (review r4, NIT):** Is one module registering two root children covered by the seam rule? **A:** Not today, so the CLI/Cobra Invariant gains a clause for it in the same commit, and the alias's constructor is named `loomcli.RunAliasCommand()`.
- **Q (review r5, BLOCKING):** What `lockPath` do `ReadOrigin`/`WriteOrigin` pass to `state.ReadJSON`/`WriteJSON`? **A:** `<weftPath>/.weft/origin.json.lock`, via the existing `ensureWeftLockDirAt`. The package's usual `path + ".lock"` idiom is gitdir-safe but would put a never-tracked file inside `_lyx`; `.weft/` is already fabric's git-excluded lock dir and already creates its own parent, so no new `MkdirAll` site trips the containment guard.
- **Q (review r5, NIT):** Does this task extract `ensureWeftLockDirAt`? **A:** No — it already exists at `weftgit.go:53`, already as the no-`Fabric` form, already called by `coalesce.go:101`. The extraction claim was wrong and is deleted; nothing about that function changes.
- **Q (review r5, NIT):** Is `removeLaunchers` really covered by "the same pair"? **A:** No — its hardcoded `{ide, fabric-checkout}` list is an explicit edit point. Omitting `run<ext>` makes the non-recursive directory removal fail outright, not just leak a file.
- **Q (review r5, NIT):** What happens to `Add`'s live-state matrix cells? **A:** Expected to pass unchanged, with the new file, its commit and the junction-side path all accounted for by the single `KindFileWritten` entry. Any adjustment is enumerated in the plan, never absorbed by loosening the oracle.
- **Q (review r3, NIT):** What does `lyx loom drive` do on a never-seeded pair? **A:** Refuses in its own pre-flight, on the envelope, naming `lyx loom run` — it never seeds, because only `run` owns the commit-before-Preflight ordering. The smoke fixture seeds by running `lyx loom run` once and killing the driver.


### From _mill/plan/00-overview.md


```yaml
task: 'loom: session bootstrap'
slug: loom-session-bootstrap
approved: true
started: '20260819-180839'
parent: standalone-producers
root: ""
verify: go build ./...
```

### From _mill/plan/01-fabric-origin-record.md


```yaml
task: 'loom: session bootstrap'
batch: fabric-origin-record
number: 1
cards: 4
verify: go test ./internal/fabricengine/
depends-on: []
```



- **Edits:** none
- **Creates:**
  - `internal/fabricengine/origin.go`
- **Deletes:** none
- **Edits:**
  - `internal/fabricengine/origin.go`
- **Creates:** none
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/commitweftpaths.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/origin_test.go`
  - `internal/fabricengine/commitweftpaths_test.go`
- **Deletes:** none

### From _mill/plan/02-fabric-add-and-launcher.md


```yaml
task: 'loom: session bootstrap'
batch: fabric-add-and-launcher
number: 2
cards: 6
verify: go test ./internal/fabricengine/ && go test -tags integration ./internal/fabricengine/
depends-on: [1]
```



- **Edits:**
  - `internal/fabricengine/add.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/fabricengine/launchers.go`
  - `internal/fabricengine/launcher_content.go`
  - `internal/fabricengine/launcher_content_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/fabricengine/launchers.go`
  - `internal/fabricengine/portallauncher_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/fabricengine/launchers_containment_integration_test.go`
  - `internal/fabricengine/destroy_containment_toctou_integration_test.go`
- **Creates:**
  - `internal/fabricengine/origin_integration_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/03-loom-paths-and-seed-sentinel.md


```yaml
task: 'loom: session bootstrap'
batch: loom-paths-and-seed-sentinel
number: 3
cards: 4
verify: go test ./internal/loomengine/ ./internal/loomshed/ ./cmd/lyx/
depends-on: []
```



- **Edits:**
  - `internal/loomengine/config.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/loomshed/seed.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `cmd/lyx/notransients_test.go`
  - `cmd/lyx/constructoranchoring_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/loomengine/config_test.go`
  - `internal/loomshed/seed_test.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/04-loomcli-core.md


```yaml
task: 'loom: session bootstrap'
batch: loomcli-core
number: 4
cards: 6
verify: go test ./internal/loomcli/
depends-on: []
```



- **Edits:** none
- **Creates:**
  - `internal/loomcli/cli.go`
- **Deletes:** none
- **Edits:**
  - `internal/loomcli/cli.go`
- **Creates:**
  - `internal/loomcli/wiring.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/loomcli/drive.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/loomcli/status.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/loomcli/pause.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/loomcli/testmain_test.go`
  - `internal/loomcli/cli_test.go`
  - `internal/loomcli/wiring_test.go`
  - `internal/loomcli/status_test.go`
- **Deletes:** none

### From _mill/plan/05-loomcli-run-bootstrap.md


```yaml
task: 'loom: session bootstrap'
batch: loomcli-run-bootstrap
number: 5
cards: 5
verify: go test ./internal/loomcli/
depends-on: [1, 2, 3, 4]
```



- **Edits:** none
- **Creates:**
  - `internal/loomcli/seedinput.go`
  - `internal/loomcli/seedinput_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/loomcli/bootstrap.go`
  - `internal/loomcli/bootstrap_test.go`
- **Deletes:** none
- **Edits:**
  - `internal/loomcli/cli.go`
  - `manifest/designs/loom.md`
- **Creates:**
  - `internal/loomcli/run.go`
- **Deletes:** none
- **Edits:**
  - `internal/loomcli/run.go`
  - `internal/loomcli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `contracts/specs/loom-status-spec.md`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/06-registration-and-guards.md


```yaml
task: 'loom: session bootstrap'
batch: registration-and-guards
number: 6
cards: 6
verify: go test ./cmd/lyx/ ./internal/loomcli/
depends-on: [2, 5]
```



- **Edits:**
  - `cmd/lyx/main.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `cmd/lyx/seamsignature_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `cmd/lyx/helptree_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `cmd/lyx/sandbox_coverage_test.go`
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/07-smoke-tests-and-roadmap.md


```yaml
task: 'loom: session bootstrap'
batch: smoke-tests-and-roadmap
number: 7
cards: 2
verify: go vet -tags smoke ./internal/loomcli/ && go test ./internal/loomcli/
depends-on: [6]
```



- **Edits:**
  - `internal/loomcli/run.go`
- **Creates:**
  - `internal/loomcli/smoke_test.go`
- **Deletes:** none
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none

## Conflicting files

- `docs/overview.md`
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
5. Run `git -C /home/knatte/Code/loomyard/wts/loom-session-bootstrap add <file>` to stage the resolved file.
6. For modify/delete (DU) conflicts: if Task intent above lists this file under a batch's `Deletes:`, run `git -C /home/knatte/Code/loomyard/wts/loom-session-bootstrap rm <file>` instead of editing;
   that stages the intentional deletion.
7. For UD conflicts — files this branch **modified** that the parent branch **deleted**: do not silently keep the modification.
   Instead: a. Run `git log --diff-filter=D --oneline MERGE_HEAD -- <file>` to find the deletion commit on the parent. b. Run `git show <deletion-commit>` to inspect context. c. If the deletion commit message mentions a replacement file (e.g. "replaced by", "moved to", "consolidated into"),
   or the commit also adds a file in the same directory with overlapping content: stage the deletion — `git -C /home/knatte/Code/loomyard/wts/loom-session-bootstrap rm <file>`. d. If detection is inconclusive: report `{"status":"stuck","stuck_type":"logic","reason":"modify/delete conflict on <file>: cannot determine if parent deletion is a replacement -- operator must decide"}` and halt.
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
Use `git -C /home/knatte/Code/loomyard/wts/loom-session-bootstrap` for any git commands;
do not `cd`.
Worktree cwd is `/home/knatte/Code/loomyard/wts/loom-session-bootstrap`.
