# Discussion: loom's status file can conflict on the landing merge

```yaml
task: loom's status file can conflict on the landing merge
slug: loom-status-file-merge-conflict
status: discussing
parent: main
```

## Problem

`_lyx/loom/status.json` is loom's orchestration status file.
It is tracked in git on the task's weft branch, and Finalize's own squash-merge carries it onto the parent's weft branch too — so from a hub's second task onward the file exists on both sides of the very merge Finalize performs, and both sides have modified it since their merge base.
Every task's seed commit rewrites the file with its own slug and its own history, so the two sides never share content: git cannot auto-merge it, and the file lands carrying conflict markers.

Conflict markers make the file unparseable, and it is the single source of truth for orchestration state.
That breaks `Shed`'s own persist re-read, `lyx loom status`, and — worst — `lyx loom pause`, the emergency brake `manifest/designs/loom.md` deliberately keeps independent of every module config precisely so a fault can never take it away.

**Why now:** found live during crucible round 2 (wiki task #120, finding F13 in the round's `loom-review-opus-high-r1.md`, branch `loom-crucible-hardening-round2`, archive tag `archive/loom-crucible-hardening-round2`).
F12 from the same round — already merged — makes both landing producers commit this file at `Publish`/`Finalize` to fix a different, more severe bug: `fabricengine`'s merge guard refuses any *tracked* modification on either side of the pair, and by the time the landing rows run `Shed` has rewritten this tracked file once per transition, so without that checkpoint the last row of every run refused on the run's own bookkeeping with no `OnStuck` target.
F12 did not create this conflict surface, but it widened it, and it is now the reason the file is guaranteed to be committed on the task branch at merge time rather than merely likely to be.

## Scope

**In:**

- Move loom's status file from the durable, git-tracked `_lyx/loom/status.json` to the never-tracked `.lyx/loom/status.json`, beside the three ephemeral files loom already keeps there (`status.json.lock`, `run.lock`, `driver.log`, `bootstrap.lock`).
  This is a one-constructor change in `internal/loomengine/config.go` (`LoomStatusFile`); every reader already goes through it.
- Delete `loomengine.LoomStatusRel()` — it exists only so callers can build fabric commit pathspecs for this file, and after this change nothing commits it.
- Drop `loomengine.LoomStatusRel()` from `lyx loom run`'s seed-commit pathspec in `internal/loomcli/run.go` (step 3), leaving `fabricengine.OriginRecordRel()` as that commit's only path, and rewrite the step's comment, which currently explains the status file's inclusion at length.
- Remove F12's `CommitStatus` seam in full: `landingshed.Deps.CommitStatus`, `Publish`'s step 3b, `Finalize`'s step 1b, and the closure `internal/loomcli/landingdeps.go` fills it with.
  An untracked file cannot trip `pairDirtyReason`, which scans `scopeTracked` only, so the checkpoint has nothing left to protect against.
- Update the status-file contract, `contracts/specs/loom-status-spec.md`: it currently pins the file as durable fabric-overlay state under `_lyx/`, "which is what makes resume work across machines".
- Update `manifest/designs/loom.md`: the `Publish`/`Finalize` commit-the-status-file section (now describing removed code), the resume-across-machines paragraph, and the `State & contracts` bullet naming the `_lyx/loom/status.json` path.
- Update `internal/shedengine`'s `StatusPath` doc comment, which calls it "the durable status file" — a word this repo's Durable-vs-Ephemeral State Invariant gives a specific, now-wrong meaning.
- Update `internal/landingshed/doc.go` if it mentions the status-file checkpoint, and `internal/loomshed`'s seed doc comment where it describes the seed being committed weft-side.
- Update the Tier-1 path guards that pin the old location: `cmd/lyx/constructoranchoring_test.go` and `cmd/lyx/notransients_test.go` (`LoomStatusFile` moves from `durableSet` to the transient set).
- **Update `contracts/stencils/loom/loom-rubric-webster-review.md` — this one is behavioral, not doc drift.**
  Its "Determining the review range" section is live agent prompt text: step 1 instructs the reviewer to read `_lyx/loom/status.json` and take `product.parent`, and the paragraph below it instructs the reviewer to raise a BLOCKING finding and review nothing when that file cannot be read.
  The stencil is wired into two live recipe rows (`rubric_stencil: loom-rubric-webster-review` at `contracts/recipes/loom-recipe.yaml:241` for `Webster-Bouncer` and `:285` for `Webster-Burler`), so after the move it would read a path that no longer exists and force a spurious BLOCKING finding into every Webster-Review round of every future loom run — the review segment would never converge.
  Both occurrences change to `.lyx/loom/status.json`.
- **Enumerate the remaining references by full-text grep over the whole tree for the literal `_lyx/loom/status.json`, not by tracing Go call sites.**
  The constructor trace is sound for code that *resolves* the path, but blind to text that *names* it — prompts, recipe comments, doc comments, and fixture instructions. Run the grep before implementation and update every hit.
  The hits as of this discussion, all doc/comment drift once the stencil above is handled: `contracts/recipes/loom-recipe.yaml:293` (the `tool-use: true` justification comment on `Webster-Burler`), `internal/loomengine/status.go:8`, `internal/loomengine/report.go:25,27,31,34,39` (the four `CheckSeed` verdict doc comments), `internal/loomshed/seed.go:2`, `manifest/designs/loom.md:60,300`, `manifest/designs/shed.md:245`, `manifest/designs/self-report.md:15`, and `contracts/specs/loom-status-spec.md:3,8,19,24`.
- Update `tools/sandbox/SANDBOX-CORE-SUITE.md`'s scenario S8 ("Loom status and pause over a seeded fixture", tagged `**Covers:** loom`), whose fixture note tells the operator to hand-write the status file at the old path.
  The scenario is the black-box coverage `CONSTRAINTS.md`'s Sandbox Suite Coverage invariant requires for the `loom` module, so a stale fixture path makes it fail at the first step.
- Add regression coverage that a full task landing no longer carries loom's status file into the parent (see [Testing](#testing)).

**Out:**

- **Any replacement carrier for cross-machine loom resume.**
  See the decision below — it is dropped, not relocated.
- Committing the status file on every producer transition (`Shed`'s persistence policy), which `loom.md` names as what genuine cross-machine resume would take.
- Any new `fabricengine` capability: no `merge=ours` driver, no per-path merge-exclusion pathspec, no git-config writes at clone/reconcile time.
  This task's fabric-side change is subtraction only.
- Renaming, reshaping, or versioning the status schema itself.
  `shedengine.Status` and loom's `product` payload are untouched; only the file's location and its git-tracked-ness change.
- `.gitattributes` at the repo root — the seeded-`.gitattributes` precedent in `internal/stencilstore/reconcile.go` is noted below as prior art for a rejected option, not as something this task touches.
- Automatic migration of hubs that already carry a tracked `_lyx/loom/status.json` (see the migration decision below).
- `manifest/roadmap.md` — this is a bugfix, not a planned item completing or being added, per `CLAUDE.md`'s task-completion rule.

## Decisions

### Drop the status file from git rather than teach the merge to survive it

- **Decision:** move the file to `.lyx/loom/status.json` and stop tracking it in git.
  The brief's three candidate fixes were a `merge=ours` gitattributes driver, dropping the file from git, and excluding it from the landing merge's pathspec; this takes the second.
- **Rationale:**
  The file is per-task orchestration state.
  It has no meaning on the parent branch, and every fix that keeps it tracked leaves the parent's weft branch permanently carrying some arbitrary earlier task's `status.json` — junk state whose only purpose is to be conflicted with by the next task.
  Removing it from git removes the conflict *and* the junk, and it removes code rather than adding it: F12's `CommitStatus` seam, the seed's commit pathspec, and `LoomStatusRel()` all become dead the moment the file is untracked, because `fabricengine`'s merge guard (`pairDirtyReason`) refuses tracked modifications only.
  The move is also invariant-conforming rather than invariant-bending: `CONSTRAINTS.md`'s Durable-vs-Ephemeral State Invariant says every never-tracked file lives under `.lyx` at the mirrored subpath of the `_lyx` content it relates to, and `loomengine` already exposes exactly that mirrored directory — `LoomStatusLock`, `LoomRunLock`, `LoomDriverLog`, and `LoomBootstrapLock` all resolve under `.lyx/loom/` today.
  The status file joins its own lock.
  The one property lost is stated in its own decision below and is already non-functional.
- **Rejected — `merge=ours` gitattributes driver:**
  git has no built-in `ours` merge driver; `merge=ours` in `.gitattributes` only names one, and the driver itself must be defined as `merge.ours.driver=true` in each repo's local git config, which is machine-local and never committed.
  `fabricengine` writes no git config in production today (the only `git config` calls in the tree are in the `gitkit`/`hubforge` test fixtures), so this option means introducing config-writing at clone/add/reconcile time plus repair logic for when it goes missing.
  Worse, its failure mode is silent: when the named driver is undefined, git falls back to the ordinary text merge and reproduces the exact bug, with nothing to signal it.
  It also leaves the junk-on-parent problem entirely intact.
- **Rejected — exclude the file from the landing merge's pathspec:**
  `git merge` takes no pathspec, so this is not a configuration but a feature — either a post-conflict forced resolution of named paths inside `fabricengine`'s merge, or a "never merge these paths" list in fabric config.
  Either shape puts knowledge of one product's status file into a generic two-sided merge engine that today "expects a conflict-free merge" and self-aborts on any conflict, and either shape is strictly more machinery than the problem needs.
  It also leaves the junk-on-parent problem intact.
- **Rejected — keep the file at `_lyx/loom/status.json` and gitignore it:**
  forbidden outright.
  The Durable-vs-Ephemeral State Invariant states `_lyx` holds tracked content only, and it is machine-enforced by `cmd/lyx/notransients_test.go`.

### Cross-machine loom resume is dropped, not relocated

- **Decision:** no replacement carrier is built, designed, or scaffolded.
  The contract doc stops claiming the property and the design doc's paragraph about it is rewritten to say plainly that loom state is machine-local.
- **Rationale:**
  The property is already non-functional.
  `manifest/designs/loom.md` says so in its own words today: the status file "lives in the weft repo, but it is not continuously committed there, so **resume across machines does not work today** — this line used to claim it did", because only the seed and the landing checkpoint are ever committed and every persist in between is an uncommitted working-tree modification.
  So a second machine pulling the weft already sees the task frozen at whatever the last commit left behind, which is worse than no state at all — it is misleading state.
  Making it real needs commit-on-every-transition, a `Shed` persistence-policy decision with a real per-transition git cost, which is explicitly out of scope here.
  Nothing in `manifest/roadmap.md`'s Planned or Someday sections commits to loom cross-machine resume; the nearest item, "session sync", is about copying Claude `.jsonl` transcripts so `--resume` works elsewhere, a different mechanism for a different artifact.
  Designing a carrier for an unrequested, unscheduled property is the hypothetical-requirement design this codebase avoids.
- **Rejected:** carrying the status via the board (`<hub>/_board`), or committing it on every transition as part of this task.
  Both are new subsystems' worth of decision, neither is asked for, and either can be built later on top of a machine-local file with no rework this task would have saved.

### F12's `CommitStatus` seam is removed, not left nil-tolerant

- **Decision:** delete `landingshed.Deps.CommitStatus`, both producers' calls to it, and `internal/loomcli/landingdeps.go`'s closure.
- **Rationale:**
  The seam exists for exactly one reason, stated in its own doc comment and in `loom.md`: fabric's merge guard refuses tracked modifications, and `Shed` rewrites this tracked file once per transition.
  Untracked, the file cannot produce a tracked modification, so both landing producers' commit steps become unconditional no-ops.
  `landingshed` is generic and shared by reference with the Someday `Hardener` product, which will have its own status file — but `Hardener` is unbuilt, and `landingshed`'s own `doc.go` already states the house rule for this exact situation: "an interface with a permanently-nil implementation is exactly the hypothetical-requirement design this codebase avoids."
  When `Hardener` lands and genuinely needs a pre-merge commit hook, it can reintroduce one against its own real requirement.
- **Rejected:** keeping the field and passing `nil` from `loomcli`.
  That leaves a documented seam with no production filler and no test that would notice if it broke — dead weight that reads as live wiring.

### No code-level migration for hubs carrying the tracked file

- **Decision:** ship no self-healing removal path.
  The change is accompanied by a documented one-time operator step: finish or abandon any in-flight `loom` run before upgrading, then delete the now-orphaned `_lyx/loom/status.json` from the weft branches that carry it (the parent's and any live task's).
- **Rationale:**
  A stale tracked `_lyx/loom/status.json` left behind on both a task's and the parent's weft branch would keep conflicting on the landing merge exactly as it does today, so the removal genuinely has to happen — but it has to happen once, per hub, by a human who knows which runs are in flight.
  Encoding a one-time cleanup as permanent production code in `lyx loom run`'s seed path means a branch that is dead the moment it has run, with no way to ever delete it confidently.
  Loomyard is pre-release with a single operator and a single hub; a documented step is proportionate.
  An in-flight run cannot be migrated automatically in any case: after the move, `lyx loom run` finds no file at the new path and seeds a fresh one, discarding the old run's `history` — which is budget-bearing (per-producer bounce budgets are derived by counting `stuck` entries), so silently starting over is materially wrong, not merely lossy.
  Making the operator finish or abandon first is the honest gate.
- **Rejected:** a one-shot migration in `lyx loom run` that detects a tracked `_lyx/loom/status.json`, copies it to `.lyx/loom/status.json`, and commits its removal.
  It is more code than the manual step, it is exercised once, and it would need its own tests and its own removal task later.
- **Where the step is documented:** as a short note in `manifest/designs/loom.md` alongside the rewritten status-file section.
  mill-plan should treat "which doc carries the operator note" as settled here, not reopen it.

## Technical context

**The geometry.**
A fabric "pair" is two checkouts: the warp (the ordinary repo worktree) and the weft (a sibling worktree, on branch `WeftBranchName(<branch>)`, holding the lyx overlay).
`_lyx` and `.lyx` are both weft-backed junction names in the wired name-set, but only `_lyx` is in the commit-routing set (`PathspecNames`); `.lyx` is `structuralNeverCommittedDirs` (`internal/fabricengine/junctionnames.go:34`) and is never committed.
Weft branches are non-orphan and fork from the parent's weft branch, sharing a merge-base — which is exactly why the two sides' divergent `status.json` cannot auto-merge.

**Where the path is declared and consumed.**
`internal/loomengine/config.go` is the sole declarer: `loomDirName = "loom"`, `loomStatusFileName = "status.json"`, `LoomStatusRel()`, `LoomStatusFile(l)`, plus the `.lyx`-side siblings `LoomStatusLock`, `LoomRunLock`, `LoomDriverLog`, `LoomBootstrapLock`, `LoomScratchDir`.
Every consumer reaches the file through `shedPaths.StatusPath`, wired once in `internal/loomcli/wiring.go` (`wireStatusPathsOnly` at line 49 and the full `wire` at line 140) from `loomengine.LoomStatusFile(location)`.
Readers: `internal/shedengine/run.go` (step-1 read gate and every persist, via `internal/state`), `internal/loomcli/status.go`, `internal/loomcli/pause.go`, `internal/loomcli/drive.go`, `internal/loomengine.CheckSeed` (row 2, `Loom-Preflight`).
None of them build the path themselves, so every Go *call site* is covered by moving the one constructor and its two guard tests.

**That constructor trace is not the whole consumer set, and must not be mistaken for it.**
A second class of consumer names the path as text rather than resolving it, and is invisible to any search over Go call sites: agent prompt stencils, recipe comments, doc comments, and sandbox fixture instructions.
One of them is behavioral — `contracts/stencils/loom/loom-rubric-webster-review.md`, the live rubric two recipe rows hand to the Webster-Review agent, which tells it to read the file and to raise a BLOCKING finding and review nothing if it cannot.
The enumeration method for this class is a full-text grep over the whole tree for the literal `_lyx/loom/status.json`, run at implementation time rather than trusted from this document; the hits known today are listed in [Scope](#scope).

**Where it is written and committed today.**
`internal/loomshed/seed.go`'s `Seed` is the only production writer of the initial file (refusing to overwrite via `ErrSeedExists`, decided under the lock through `state.UpdateJSON`).
`internal/loomcli/run.go` step 2 calls it, then step 3 commits `[]string{loomengine.LoomStatusRel(), fabricengine.OriginRecordRel()}` via `fabricengine.CommitAnchoredPaths`.
That step's comment explains at length why the status path is included unconditionally and asserts that the commit must precede the driver spawn because "the phase machine's very first precondition row scans the fabric including untracked files, and neither file is on the never-tracked exclude list" — the second half of that claim stops being true for the status file once it moves under `.lyx`, so the comment must be rewritten, not merely shortened.
`internal/landingshed/publish.go` (step 3b, ~line 109) and `internal/landingshed/finalize.go` (step 1b, ~line 115) each call `deps.CommitStatus()` immediately before their merge, mapping a failure to a returned error rather than `Stuck`.

**Why untracked is sufficient, mechanically.**
`internal/fabricengine/mergeguards.go`'s `pairDirtyReason` (line 137) calls `worktreeDirty(scopeTracked, …)` on both sides — tracked scope only.
So an untracked file under `.lyx` cannot produce `mergeReasonWorktreeDirty`, which is the whole hazard F12's checkpoint was introduced to avoid.
The preflight cleanliness check that *does* scan untracked files excludes `.lyx` structurally; this is already proven in production by `LoomBootstrapLock` and `LoomDriverLog`, which are created under `.lyx/loom/` in `lyx loom run` step 4 — before the driver spawns and before row 1's precondition scan runs — and do not fail it.

**The merge path the bug travels.**
`Finalize.Call` step 2 catches the task worktree up via `internal/mergeresolve` (`Resolve`, LLM-backed conflict resolution with a mechanical marker re-scan), step 3 opens the parent pair, step 4 calls `fabricengine.Fabric.Merge(taskBranch, MergeOptions{Squash: cfg.Squash})`.
`Merge` merges the warp branch into the parent's warp and `WeftBranchName(source)` into the parent's weft, expects a conflict-free merge, and self-aborts both sides with `*ErrMergeInRequired` on any conflict; step 5 then re-runs the resolver in the task worktree and retries once.
So today's `status.json` conflict surfaces on the **weft** side and is pushed into `mergeresolve`'s LLM session — an expensive, non-deterministic resolution of a file that should never have been merged at all.

**Prior art worth knowing but not reusing.**
`internal/stencilstore/reconcile.go` seeds a `.gitattributes` (`*.md text eol=lf`) into its own weft-side directory — the precedent that would have made a `merge=ours` attribute file cheap to place.
It is recorded here so a plan reader does not rediscover it and reopen the rejected option; the missing piece was never the attributes file, it was the undefined driver.

**Docs that make claims this change falsifies.**
`contracts/specs/loom-status-spec.md` — "What it is" (durable fabric-overlay state under `_lyx/`, git-synced via fabric, "which is what makes resume work across machines"), and "The seed / handover" ("commits the seed weft-side before it spawns the detached driver").
`manifest/designs/loom.md` — the resume-across-machines paragraph, the `Publish`/`Finalize` checkpoint section, and the `State & contracts` bullet.
Both are durable, kept docs, so they are edited in place, not deleted.

## Constraints

From `CONSTRAINTS.md`:

- **Durable-vs-Ephemeral State Invariant** — every never-tracked file lives under `.lyx`, at the mirrored subpath of the `_lyx` content it relates to; `_lyx` holds tracked content only.
  No engine derives its own `.lyx` path — each module exposes a scratch accessor beside its durable one.
  This is the invariant that both forbids the gitignore-in-place shortcut and dictates the exact destination path.
  Machine-enforced by `cmd/lyx/notransients_test.go`, `cmd/lyx/constructoranchoring_test.go`, `internal/fabricengine/structuraldirs_test.go`.
  Note the ordering consequence: after the move, `loomengine.LoomStatusFile` is a transient constructor, and `loomengine` no longer exposes any `_lyx`-rooted path under `loom/` at all — `_lyx/loom/` ceases to exist as a directory.
- **Cwd Resolution Invariant** — a module's own durable-storage subdirectory is that module's own private relative-path constant joined onto `AnchorPath()` directly, never a `lyxcwd` call.
  `loomengine` stays the sole declarer of `loomDirName` and `loomStatusFileName`; the move must not push either literal into `lyxcwd` or anywhere else.
- **Told-Geometry Invariant** — `internal/landingshed` and `internal/mergeresolve` take told absolute paths and have no production import of `internal/lyxcwd`.
  Removing `CommitStatus` must not tempt a replacement that has `landingshed` resolve loom's path itself.
- **Fabric Vocabulary Invariant** — `landingshed` and `mergeresolve` are not in the owner set: none of their identifiers, string literals, or comments may name either fabric-internal side (warp/weft).
  AST-enforced by `internal/lyxcwd`'s `TestEnforcement_FabricVocabulary`.
  Doc-comment edits in those two packages are subject to it.
- **Lyxdirs Single-Declarer Invariant** — the `_lyx`/`.lyx` literals come from `lyxdirs`, never hand-built joins.
  The moved constructor uses `lyxdirs.DotLyxDirName`, exactly as `LoomStatusLock` already does.
- **CLI/Cobra Invariant** — no command surface changes here, but `lyx loom run`'s and `lyx loom status`'s help text must not end up describing a git-committed status file.
- **Markdown Link Integrity** — `loom.md`'s `#crash-recovery--resume-on-output-files-not-live-processes` heading is linked from `manifest/roadmap.md` and from within `loom.md` itself; the doc edits must not change that heading's text.
- **Documentation Lifecycle** — `contracts/specs/loom-status-spec.md` and `manifest/designs/loom.md` are durable docs, edited in the same commit as the code per `CLAUDE.md`'s task-completion rule.
- **Sandbox Suite Coverage Invariant** — every registered lyx module is exercised by the black-box sandbox suite or explicitly excluded; `loom`'s coverage is scenario S8 in `tools/sandbox/SANDBOX-CORE-SUITE.md`, tagged `**Covers:** loom`, and its fixture note names the status-file path.
  Enforced by `cmd/lyx/sandbox_coverage_test.go`, which checks tagging rather than fixture correctness — so a stale path there fails the operator, not the test suite.
- **Test Tier Purity Invariant** — `cmd/lyx/notransients_test.go` and `constructoranchoring_test.go` are Tier 1 (pure `filepath.Join` arithmetic over hand-built `*lyxcwd.Location` fixtures, no process spawned, no fixture tree copied).
  Keep them that way.

Discovered during exploration:

- `manifest/roadmap.md` must not move: this is a bugfix, not a planned item completing.
- No new cross-cutting invariant is created by this change, so `CONSTRAINTS.md` needs no new section — the existing Durable-vs-Ephemeral entry already covers the destination.

## Testing

**Tier 1 — path guards (update, do not add new files).**
`cmd/lyx/notransients_test.go`: `loomengine.LoomStatusFile` moves out of `durableSet` and into the transient set, where it must resolve under `.lyx` at the mirrored subpath, for both `AnchorRel == "."` and `AnchorRel == "backend"` fixtures.
`cmd/lyx/constructoranchoring_test.go`: the four existing assertions (lines ~84, ~93, ~140, ~149) currently pin `LoomStatusFile` under `lyxBase` and `LoomStatusLock` under `dotLyxBase`; `LoomStatusFile` moves to `dotLyxBase/loom/status.json`, sitting beside its own lock.
These two are the TDD candidates — write the moved assertions first, watch them fail, then move the constructor.

**Tier 1 — dead-code removal is proven by compilation.**
Deleting `LoomStatusRel()` and `landingshed.Deps.CommitStatus` breaks every remaining caller at build time; the existing `internal/landingshed/commitstatus_test.go` is deleted along with the seam, and `internal/loomcli/landingdeps_test.go`'s drift guard is updated to match the reduced `Deps` shape.
`internal/landingshed/publish_test.go` and `finalize_test.go` lose their commit-status cases.
No replacement test is written for "the seam is gone" — a deleted field needs no guard.

**The scenario that must be covered — the bug itself.**
An integration test that lands two tasks in sequence off the same parent and asserts the second one's `Finalize` parent-side merge completes with no conflicts.
This is the only test that would have caught F13, and it is the one worth the fixture cost.
It belongs where the two-sided merge fixtures already live (`internal/fabricengine`'s integration tests, e.g. alongside `mergecrucible_integration_test.go`, or `internal/landingshed/finalize_integration_test.go` — mill-plan picks, based on which fixture can drive a full seed-through-Finalize cycle most cheaply).
The load-bearing assertion is the absence of conflicts on the second landing; a secondary assertion that the parent's weft branch tracks no `loom/status.json` after either landing pins the junk-on-parent property too.

**Scenarios that must also be covered:**

- `Seed` writes to, and `CheckSeed`/`Shed`'s step-1 read gate read from, the new `.lyx` path — covered by existing `internal/loomshed/seed_test.go` and `internal/loomengine/seed_test.go` once their fixtures follow the constructor; no new cases needed, but confirm none of them hardcode `_lyx` in a path literal.
- `lyx loom run` on a clean worktree still succeeds when its commit pathspec is reduced to the origin record alone, including the re-run (already-seeded) case that `ErrSeedExists` tolerates.
- `Publish` and `Finalize` still merge cleanly when the status file has been rewritten by `Shed` and never committed — the direct proof that `pairDirtyReason`'s tracked-only scope makes the removed checkpoint unnecessary.
- `lyx loom pause` and `lyx loom status` read the file at its new location.
  Existing `internal/loomcli` tests cover the behaviour; they need only follow the constructor.
- The edited `loom-rubric-webster-review.md` stencil still validates and still resolves through both recipe rows that name it — covered by `lyx stencil validate` and the existing recipe/stencil wiring tests, which need no new cases, only a passing run.
  Note the operational consequence for mill-plan to record rather than solve: per `tools/sandbox/SANDBOX-CORE-SUITE.md`'s S7 durability note, the board's stencils tree is seeded on first run and persists, so an operator's already-seeded board copy of this rubric does not pick the edit up on its own — it is `lyx stencil`'s existing promote/sync surface that reconciles it, not this task's code.

**Not tested:** cross-machine resume, in either direction.
It is removed as a claim, so there is nothing to assert about it.

## Q&A log

- **Q:** Which of the three named fixes does this take — a `merge=ours` gitattributes driver, dropping the file from git, or excluding it from the landing merge's pathspec? **A:** [auto-pick] Drop the file from git: move `_lyx/loom/status.json` to `.lyx/loom/status.json`. **Why:** it is the only option that removes machinery instead of adding it (F12's `CommitStatus` seam, the seed commit pathspec, and `LoomStatusRel()` all become dead), the only one that stops depositing a meaningless per-task artifact on the parent branch, and it lands the file exactly where `CONSTRAINTS.md`'s Durable-vs-Ephemeral State Invariant already says never-tracked state belongs — beside the lock that guards it.
- **Q:** The move makes the status file untracked — is that acceptable, or must it stay in git? **A:** [operator] Untracked is the intent, not a side effect; keep the `.lyx/loom/status.json` decision. **Why:** git-tracking is the defect — the file sits on both sides of the merge Finalize itself performs, both sides have rewritten it since the merge base, and the resulting conflict markers land in the one file `lyx loom pause` depends on. The operator was shown the three tracked-preserving alternatives (`merge=ours`, merge-pathspec exclusion, commit-on-every-transition) and rejected all three.
- **Q:** Does cross-machine resume get a replacement carrier in this task? **A:** [auto-pick] No — the property is dropped and the docs stop claiming it. **Why:** `loom.md` already states it does not work today, because only the seed and the landing checkpoint are ever committed; nothing on the roadmap commits to it; and building a carrier for an unscheduled property is the hypothetical-requirement design this codebase avoids.
- **Q:** Is F12's `CommitStatus` seam removed outright, or kept and passed `nil`? **A:** [auto-pick] Removed outright, including the `landingshed.Deps` field. **Why:** its sole purpose was satisfying fabric's tracked-modification merge guard, which an untracked file cannot trip; and `landingshed`'s own `doc.go` states the house rule against a permanently-nil implementation, so Someday-`Hardener` is not a reason to keep it.
- **Q:** Does `lyx loom run` still commit anything at step 3? **A:** [auto-pick] Yes — `fabricengine.OriginRecordRel()` only. **Why:** the origin record is genuinely durable provenance and its self-healing unconditional-inclusion rationale is unaffected; only the status path leaves the list, and the step's comment is rewritten since its "neither file is on the never-tracked exclude list" claim stops being true.
- **Q:** Do hubs already carrying a tracked `_lyx/loom/status.json` get an automatic migration? **A:** [auto-pick] No — a documented one-time operator step in `manifest/designs/loom.md`, gated on finishing or abandoning in-flight runs first. **Why:** the stale tracked file must be deleted from the weft branches or the bug persists, but an in-flight run cannot be migrated safely in any case — `history` is budget-bearing (per-producer bounce budgets are counted from it), so silently reseeding would hand every producer a fresh budget; and a one-shot migration path is permanent code exercised once.
- **Q:** What proves the fix, given the bug was found by review rather than by a failing test? **A:** [auto-pick] An integration test landing two sequential tasks off one parent and asserting the second `Finalize`'s parent-side merge is conflict-free. **Why:** it is the only test that reproduces the actual failure — one task alone never conflicts, since the divergence requires both sides to have rewritten the file since their merge base.
- **Q:** How is the consumer set enumerated — by tracing Go call sites, or by searching the whole tree for the literal path? **A:** [round-1 gap fix] Full-text grep over the whole tree, treated as the primary method; the constructor trace covers only code that resolves the path. **Why:** the trace missed `contracts/stencils/loom/loom-rubric-webster-review.md`, live prompt text wired into two recipe rows, which instructs the Webster-Review agent to raise a BLOCKING finding and review nothing when the status file cannot be read — after the move that fires on every review round of every future run, so the review segment would never converge.
- **Q:** Does `CONSTRAINTS.md` gain a new invariant? **A:** [auto-pick] No. **Why:** the Durable-vs-Ephemeral State Invariant already covers the destination and already machine-enforces it; this task moves a file into compliance rather than establishing a new cross-cutting rule.
