# Discussion: .lyx hygiene -- relocate transients out of _lyx, fix .lyx junction geometry (slice 9)

```yaml
task: .lyx hygiene -- relocate transients out of _lyx, fix .lyx junction geometry (slice 9)
slug: dotlyx-scratch-hygiene
status: discussing
parent: main
```

## Problem

Loomyard has two lyx directories and the split is structural: `_lyx` is durable, weft-synced, **tracked** content, while `.lyx` is ephemeral, machine-bound, **never-tracked** scratch.
The rule the operator states plainly: **every file that must not survive a move between machines — that is, every file that SHALL stay on one machine — lives under `.lyx`.**

Today the rule is violated in two directions.
Several modules write never-tracked transients into `_lyx` (perch/treadle's run and state locks and its pause flag, webster's pause flag, rendered fork prompts and three lock files, builder's pause flag and three lock files, loom's `status.json.lock`), which is the entire reason `internal/fabricengine` carries the `crossModuleMachineLocalExcludes` machinery at all — a wildcard-pattern exclusion layer that exists purely to stop misplaced files from being committed.
And `.lyx` itself is kept out of the host repo by a **committed** `.gitignore` block (`gitignore.Ensure(l.AnchorPath(), ".lyx/")`, `internal/fabriccli/clone.go:81`), which is wrong on principle: a tracked `.gitignore` entry in the *user's own* repo advertises that LYX is in use, and a host→weft junction must never leave a tracked artifact behind in the user's repo.

**Why now:** slice 7 (`6a93049c`) shrank `internal/hubgeometry` out of existence and moved junction geometry into `internal/fabricengine`, and `fabric-collapse-external-surface` (`63218a66`) removed the per-call `:(exclude)` pathspec magic and `git add -f`, installing `seedWeftArtifactExcludes` as an interim guard.
Both preconditions this task waited on are now on `main`.
This task removes the *need* for that interim guard by putting the files where they belong, and is **sequenced before slice 10** (`fabric-warp-binding-in-weft`) because both edit `internal/fabriccli/clone.go`'s `runCloneWithReset` in the same ~45-line span.

## Scope

**In:**

- A new zero-dependency leaf package `internal/lyxdirs` owning both directory-name tokens: `LyxDirName` (`_lyx`, moved out of `internal/configengine`) and `DotLyxDirName` (`.lyx`, replacing five private `dotLyxDirName` consts).
- Relocating **every** never-tracked transient currently written under `_lyx` into the mirrored subpath under `.lyx` (full inventory in Technical context).
- A new scratch-dir seam on `internal/treadleengine` so perch's run/state locks and pause flag can leave the run dir while the engine stays geometry-blind.
- Registering `.lyx` as a weft-backed junction through `fabric.yaml`'s existing `pathspec`, replacing the committed `.gitignore` block in `clone.go`/`unwire.go` with the warp `.git/info/exclude` seeding every other junction already uses.
- A `neverCommittedNames` set in `internal/fabricengine` keeping `.lyx` out of every pathspec and out of weft commit routing, plus `--exclude-standard` on `weftPathspecFilter`'s `git ls-files` probe.
- Replacing the three `crossModuleMachineLocalExcludes` patterns with a single `.lyx/` entry in the weft repo's `.git/info/exclude`, and deleting the `crossModuleMachineLocalExcludes` var.
- Recognising `<hub>/.lyx` as a hub-level geometry element owned by fabric, the way `<hub>/_board` already is.
- Two enforcement tests (single-declarer for the two directory tokens; no-transients-under-`_lyx`).
- Docs in the same commit: `CONSTRAINTS.md`, `docs/overview.md`, the affected package docs, and `manifest/designs/fabric-unified-view.md` slice 9.

**Out:**

- **Migration of any kind.**
  A worktree that already holds a real `.lyx` directory where the junction belongs gets a hard error with a remedy message, not an automatic conversion.
  Transients a pre-fix `sync` already committed into weft history stay committed; `git rm --cached <path>` is documented as the manual remedy, not automated.
- Moving `.weft/` or `gitrepo.PushLockFileName` out of `seedWeftArtifactExcludes` — those are fabric's own runtime artifacts, not module transients, and stay exactly where slice 7/8 put them.
- Splitting `fabric.yaml`'s `pathspec` into separate junction/commit fields — a schema change well outside this slice.
- `_raddle`, `_pattern` and `_board` content sweeps — they hold no known transients today.
  `internal/fabricengine`'s own `board.push.lock` (`bolt.go:34`, under `<hub>/_board`) and the correspondence index (`index.go:75`, inside the git dir) are fabric-internal and stay put.
- This repository's own hand-written `.gitignore` `.lyx/` block — loomyard is not fabric-wired, so its `.lyx` is a plain directory and the entry is correct as it stands.
- Non-Claude engines, reed/shuttle/scout/burler/logger `.lyx` paths that are **already** correct — they change only by switching to the new shared const.

## Decisions

### dotlyx-is-a-weft-backed-junction

- Decision: `.lyx` lives physically inside the weft worktree at `<weft>/<AnchorRel>/.lyx` and is exposed in the warp/host worktree as a directory junction created by fabric, registered through `fabric.yaml`'s existing `pathspec` exactly like `_lyx` and `_pattern`.
  The committed `.gitignore` `.lyx/` block is removed from both `clone.go` and `unwire.go`; the junction name is excluded through the warp's `.git/info/exclude` via the existing `seedGitExclude` path.
- Rationale: it is the design doc's stated target (slice 9, `manifest/designs/fabric-unified-view.md`) and needs no bespoke code — `.lyx` becomes one more pathspec entry, inheriting wire/unwire/reconcile/health/drift handling for free.
  A committed `.gitignore` in the user's own repo is the specific thing being eliminated: it makes LYX visible in a repo LYX does not own.
- Rejected: keeping `.lyx` a plain host directory with only a standalone exclude-seeding call (introduces a third junction-ish shape alongside `_lyx` and `_board`);
  anchoring `.lyx` at the hub instead of per worktree (a far larger relocation of every module's current worktree-anchored path, and it destroys per-worktree isolation of machine-local state).

### never-committed-is-structural-not-configurable

- Decision: `internal/fabricengine` declares a named `neverCommittedNames` set (today exactly `lyxdirs.DotLyxDirName`).
  Names in that set are kept out of every constructed pathspec and out of `classifyPaths`' weft-commit routing, so `.lyx` is never even *named* in a git invocation.
  Separately, `weftPathspecFilter`'s `git ls-files --cached --others` probe gains `--exclude-standard`.
- Rationale: this is not belt-and-braces — the two halves fix different failures.
  `fabric.yaml`'s `pathspec` is read for **two** purposes: the junction name-set (`WiredNames`) *and*, via the raw `cfg.Dirs()`, the pathspec `lyx fabric sync` passes to `Fabric.Commit` (`internal/fabriccli/weft_verbs.go:100`).
  Adding `.lyx` to the pathspec therefore puts `<AnchorRel>/.lyx` straight into the sync pathspec.
  Verified empirically: `git add` on an ignored path **fails the entire invocation with exit 1** and stages nothing — not even the legitimate `_lyx` files named in the same call — and `git ls-files --cached --others -- .lyx` *does* match ignored files when `--exclude-standard` is absent, so `weftPathspecFilter` currently forwards a doomed entry.
  Without the name set, every `lyx fabric sync` would hard-fail from the first commit; without `--exclude-standard`, any stray ignored file matching any pathspec entry still topples a whole commit.
- Rejected: relying on git's own refusal alone (the guarantee is absolute but the failure is a cryptic hard error that blocks unrelated content);
  silently dropping `.lyx` paths instead of keeping them out of the pathspec (hides caller bugs);
  encoding "never committed" as a fabric.yaml field (CONSTRAINTS: *geometry is structural, never config/env-overridable* — a repo must not be able to switch it off).

### weft-side-exclusion-via-git-info-exclude

- Decision: the weft repo keeps `.lyx` untracked through a single `.lyx/` line in its `.git/info/exclude`, seeded by the existing `seedWeftArtifactExcludes`.
  The three `crossModuleMachineLocalExcludes` patterns are deleted and the var removed.
  No `.gitignore` is committed in weft either.
- Rationale: the warp rule ("never a committed `.gitignore`") is about not exposing LYX in the user's repo, and weft is LYX's own repo — so the choice there is purely practical, and `.git/info/exclude` wins on practicality.
  It is seeded from `ensureWeftLockDir`, the choke point every weft-git verb passes through, so it self-heals on every new machine with no commit, no pathspec change and no new file in the weft root.
  One `.lyx/` pattern replaces three wildcard patterns and covers every future transient without a `fabricengine` change.
  The only difference from a committed `.gitignore` is that the entry is per-clone rather than travelling with the repo — precisely the contract `.weft/` already has.
- Rejected: a committed `.gitignore` at the weft root (needs a new staging path, since the weft root sits outside every caller's pathspec, and adds a file to the weft root for no gain);
  keeping the three deep wildcard patterns (the machinery this task exists to delete).

### no-migration

- Decision: no migration support anywhere.
  Wiring `.lyx` onto a worktree that already holds a real `.lyx` directory produces a hard error via `seedLyxJunction`'s existing host-pristine guard, with the remedy text extended to name `.lyx` and say the directory is safe to delete.
  Transients already committed into weft history stay there; `git rm --cached` is documented, not automated.
- Rationale: `.lyx` is by definition disposable machine-local scratch, so the operator can always resolve the error by deleting the directory — an automatic conversion buys little and risks destroying live reed/scout daemon state mid-flight.
  Adopting the real directory instead (no junction, no error) would leave two legal geometries coexisting and take fabric out of control of its own topology.
- Rejected: moving existing contents into the weft target automatically;
  deleting the real directory silently;
  adopting it as-is.

### lyxdirs-owns-both-directory-tokens

- Decision: a new leaf package `internal/lyxdirs` (stdlib-only, zero internal imports) declares `LyxDirName = "_lyx"` and `DotLyxDirName = ".lyx"`.
  `configengine.LyxDirName` is moved there (≈115 references across 19 non-test files plus tests — a mechanical identifier rename), and the five private `dotLyxDirName` consts (`shuttleengine/rundir.go:28`, `scoutengine/daemonstate.go:31`, `reedengine/lifecycle.go:30`, `burlerengine/engine.go:29`, `logger/sink.go:27`) are deleted in favour of it.
- Rationale: the two names are one structural pair — tracked versus never-tracked — and splitting their ownership across two packages is worse than either extreme.
  `configengine` owns `_lyx` only because config files happen to live under it, which is inherited, not principled.
  `fabricengine` cannot own the token: `internal/logger` needs it, and `fabricengine → logger → lyxcwd` must stay acyclic.
  `internal/lyxcwd` cannot own it either — the Cwd Resolution Invariant forbids it from exposing junction paths or per-module subdirectories, and `.lyx` is now a junction name.
  A zero-dependency leaf is importable by every one of them.
  `shuttleengine/rundir.go:27` already records the promise: *"It stays unpoliced this slice; slice 9 registers a single owner."*
- Rejected: `configengine.DotLyxDirName` (small diff, but cements an ownership we just established is wrong);
  putting only `.lyx` in a new package (splits the pair);
  leaving five private declarers (leaves the promise unkept and five sites to drift).

### treadle-gains-an-explicit-scratch-dir

- Decision: `internal/treadleengine`'s engine gains an explicit scratch-directory input, defaulting to the existing `runDir` when unset.
  `run.lock`, `state.json.lock` and the `pause` flag are written there instead of in `runDir`; `state.json` and round artifacts stay in `runDir`.
  `internal/perchcli` supplies `<worktree>/.lyx/perch/<block>` as the scratch dir while `runDir` stays `_lyx/perch/<block>`.
- Rationale: it keeps the engine geometry-blind — it is *told* where to write, never deriving it — which is the exact constraint `internal/perchcli/run.go:324-333` documents as the blocker.
  Every treadle-based module inherits the seam.
  `internal/state`'s `WriteJSON(path, lockPath, …)` / `ReadJSON(path, lockPath, …)` already take the lock path separately, so a lock can move without moving the file it guards.
- Rejected: having `perchengine` compute both directories and hand treadle two paths (same effect, but lands the decision in perch, so the next treadle-based module repeats the work);
  leaving perch's locks in `_lyx` behind a narrowed exclude (blocks deleting `crossModuleMachineLocalExcludes`, which is half this task).

### full-sweep-mirrored-subpaths

- Decision: sweep every module-owned directory, not just the four artifacts the original brief named.
  Every never-tracked artifact moves to the **same relative subpath** under `.lyx` that it had under `_lyx` — so `_lyx/webster/state.json.lock` becomes `.lyx/webster/state.json.lock`, guarding `_lyx/webster/state.json` from a sibling tree.
  Treadle's `pause` flag moves with the locks (it is the same never-tracked class as webster's and builder's).
- Rationale: the principle is the invariant, and a half sweep leaves behind exactly the cases the exclusion machinery being deleted existed for.
  Mirroring the subpath keeps the mapping mechanical and reviewable, and keeps per-module ownership of relative paths intact per the Cwd Resolution Invariant.
- Rejected: relocating only run-dir-scoped artifacts (leaves locks that guard durable state files in `_lyx`);
  extending the sweep into `_raddle`/`_pattern`/`_board` (no known transients there today — YAGNI).

### hub-level-dotlyx-is-a-recognised-geometry-element

- Decision: `<hub>/.lyx` (today `reedengine.HubLogsDir` at `<hub>/.lyx/logs`) becomes a fabric-recognised hub-level geometry element, the way `<hub>/_board` already is: created and named by fabric using `lyxdirs.DotLyxDirName`, documented in the geometry invariant, and reserved so no worktree slug can claim the name.
  It stays a **real directory, not a junction** — `<hub>` is not a git repo, so there is nothing to exclude and no weft to point at.
  `reedengine` stops declaring the name itself.
- Rationale: "all machine-local data in `.lyx`" holds at hub level too, and one shared reed server per hub needs a deterministic hub-anchored place for its log.
  Recognising it makes the hub layout complete rather than leaving an ad-hoc `mkdir` outside the geometry model.
- Rejected: junctioning `<hub>/.lyx` into `_board`'s `weft:main` checkout (puts machine-local state in a synced branch for no gain);
  moving reed's server logs down to a worktree `.lyx` (destroys the one-server-per-hub deterministic location).
- **Implementation gotcha, must be handled:** `HubReservedNames()` currently serves two different jobs — reserving names a worktree slug may not claim, *and*, through `filterHubReserved`, stripping names from the wired junction set.
  Adding `.lyx` to that one list would make `filterHubReserved` delete `.lyx` from the wired names and the per-worktree junction would never be created.
  The two jobs must be split: a slug-reservation set (gains `.lyx`) and a junction-wiring block set (keeps exactly today's `_board`, `_portals`, `_launchers`, `_raddle`).

### commit-ordering

- Decision: transients move first, then the exclusion machinery is deleted, then the geometry changes.
- Rationale: every commit leaves the tree green, and the exclusions are removed only once nothing depends on them.
  The reverse order keeps both mechanisms alive simultaneously for longer.
- Rejected: geometry first.

## Technical context

### Where junction geometry lives after slice 7

`internal/hubgeometry` **no longer exists**.
Junction geometry is now in `internal/fabricengine`:

- `junction.go` — `HostJunctions`/`HostJunctionsHere` (the `{Name, Link, Target}` records), `WireJunctions` → `seedLyxJunction` + `seedGitExclude`, `UnwireJunctions` → `unseedLyxJunction` + `unseedGitExclude`, and `wireBoardLink` (the standalone `_board` case).
  `seedLyxJunction:163` is the host-pristine guard whose message must be extended for `.lyx`.
- `junctionnames.go` — `BoardDirName`, `HubReservedNames`, `IsReservedHubName`, `filterHubReserved`, `junctionNames`/`WiredNames`/`RepoWiredNames`.
  `WiredNames(baseDir)` loads `fabric.yaml` and returns `filterHubReserved(cfg.Dirs())`.
- `config.go` — `Config.Pathspec` (a space-separated string) and `Config.Dirs()` (`strings.Fields`).
  The repo-wide file is `<Hub>/_board/_lyx/config/fabric.yaml`.
- `commit.go:91` — `Fabric.Commit` calls `RepoWiredNames(l)` then `classifyPaths(l.AnchorRel, wiredNames, files)` to split caller paths into warp-side and weft-side.
- `weftgit.go` — `crossModuleMachineLocalExcludes` (lines 56-96, to delete), `seedWeftArtifactExcludes` (98-162, gains `.lyx/`, loses the cross-module patterns), `weftPathspecFilter` (177-199, gains `--exclude-standard` at `entryMatchesWeft:205`).

### The committed-`.gitignore` sites to remove

- `internal/fabriccli/clone.go:81` — `gitignore.Ensure(l.AnchorPath(), ".lyx/")` inside `runCloneWithReset`, a few lines below the `WireJunctions` call at :77.
  **Slice 10 edits the same function**, hence the sequencing.
- `internal/fabricengine/unwire.go:113` — `gitignore.Remove(cwd, ".lyx/")`, plus the doc comments at `unwire.go:44` and `fabriccli/fabric.go:262`.
- `internal/fabricengine/unwire_test.go:137,156` — `TestUnwire_RevertsGitignore` asserts the behaviour being removed and must go with it.
- `internal/gitignore` itself stays — it is still used for other entries; only the `.lyx/` callers go.

### Full transient inventory (the sweep)

Under `_lyx`, to relocate to the mirrored `.lyx` subpath:

| Module | Artifact | Declared at |
| --- | --- | --- |
| webster | `pause` | `internal/websterengine/pause.go:27,31` |
| webster | `prompts/*` (rendered fork prompts) | `internal/websterengine/state.go:52-57`; consumers `beginbatch.go:201-204`, `runlevel.go:398,460-463` |
| webster | `run.lock` | `internal/websterengine/runlevel.go:38` |
| webster | `mutate.lock` | `internal/websterengine/state.go:70` |
| webster | `state.json.lock` | `internal/websterengine/state.go:195,211` |
| builder | `pause` | `internal/builderengine/pause.go:23,31` |
| builder | `run.lock` | `internal/builderengine/runlevel.go:34,304` |
| builder | `mutate.lock` | `internal/builderengine/state.go:55` |
| builder | `state.json.lock` | `internal/builderengine/state.go:135,149` |
| perch/treadle | `run.lock` | `internal/treadleengine/run.go:42,98` |
| perch/treadle | `state.json.lock` | `internal/treadleengine/state.go:101,140,156` |
| perch/treadle | `pause` | `internal/treadleengine/state.go:32,209-216` |
| loom | `status.json.lock` | `internal/loomengine/config.go:79` |

Already correct (change only to adopt `lyxdirs.DotLyxDirName`): `scoutengine/daemonstate.go:43,50`, `shuttleengine/rundir.go:51` and `run.go:185`, `burlerengine/engine.go:106`, `logger/sink.go:37`, `reedengine` state at `spawn.go:143,183`, `strand.go:309,348,471`, `lifecycle.go:545,676,778,961`, `lock.go:66`, and `reedengine.HubLogsDir` at `lifecycle.go:38`.

Out of scope, confirmed not under `_lyx`: `fabricengine`'s correspondence index (`index.go:75`, inside the git dir) and `board.push.lock` (`bolt.go:34`, under `<hub>/_board`).

### Call sites that consume the raw pathspec

`internal/fabriccli/weft_verbs.go:100` builds `fabricengine.ScopedPathspec(l.AnchorRel, cfg.Dirs())` from the **unfiltered** `Dirs()` and passes it to `fab.Commit` at :155, :184 and :240.
This is the site that breaks if `.lyx` is not filtered out by `neverCommittedNames`.
`Topology.Add`'s reserved-name union also reads raw `Dirs()` — that use is correct and should keep seeing `.lyx` (a slug must not claim the name).

Module weft-commit callers build their own pathspec from `configengine.LyxDirName` only (`internal/perchcli/run.go:334`, `internal/fabricengine/unwire.go:104`, webster's and builder's `weft.go`), so they are unaffected beyond the const rename.

### Empirically verified git behaviour

Run in a scratch repo with `.lyx/` in `.git/info/exclude`:

- `git add _lyx/webster/state.json .lyx/webster/state.json.lock` → exit 1, `The following paths are ignored by one of your .gitignore files`, **nothing staged at all**.
- `git add .lyx` (directory pathspec) → same hard failure.
- `git ls-files --cached --others -- .lyx` → **matches** `.lyx/webster/state.json.lock` (exit 0), because `--exclude-standard` is absent.

### Existing patterns to reuse

- `internal/fslink` — `CreateDirLink`, `IsLink`, `PointsTo`, `Remove`.
  Directory-only contract; junctions on Windows, symlinks elsewhere.
- `internal/fabricengine/junction_pattern_integration_test.go` — the template for a `.lyx` junction integration test; it already resolves the exclude file the way `seedGitExclude`/`unseedGitExclude` do.
- `internal/lyxtest` — synthetic hub construction for integration tests (note `lyxtest.go:231` joins `configengine.LyxDirName`; it participates in the rename).
- `cmd/lyx/boardguard_test.go` and `cmd/lyx/rawgitmutation_test.go` — the shape to copy for the two new enforcement tests.

## Constraints

From `CONSTRAINTS.md`:

- **Cwd Resolution Invariant** — `internal/lyxcwd` owns cwd resolution alone and exposes only `RepoName`, `HubPath`, `WorktreeName`, `AnchorRel`, `WorktreePath()`, `AnchorPath()`.
  It never resolves a weft path, a junction path, or any per-module subdirectory; weft-sibling and junction construction belong to `internal/fabricengine`.
  A module's own subdirectory is that module's private relative-path constant joined onto `AnchorPath()` — per-segment joins, never a fused `"_lyx/..."` literal.
  `lyxcwd`'s imports stay capped at stdlib plus `internal/gitexec`, which is what keeps `fabricengine → logger → lyxcwd` acyclic.
  **Geometry is structural, never config/env-overridable** — the direct basis for the `neverCommittedNames` decision.
- **Weft Git Invariant / "Cross-module exclusions"** (`CONSTRAINTS.md:144-152`) — the mechanism this task retires.
  Its "known limitation" note (a pre-fix sync's already-committed artifacts need manual `git rm --cached`) survives as the documented manual remedy.
- **Documentation Lifecycle** — a task touching a module doc, the module table, or a cross-cutting invariant updates them in the same commit.
- **lyxtest Leaf Invariant** and **CLI/Cobra Invariant** — no new commands here, but `internal/lyxdirs` must stay a leaf (stdlib-only) or it defeats its own purpose.
- **Tier 1/2 substrate rule** — tests must be tiered correctly; junction wiring on a synthetic hub is substrate-backed.

New invariant this task records in `CONSTRAINTS.md`:

- Every never-tracked file lives under `.lyx`, at the mirrored subpath of the `_lyx` content it relates to; `_lyx` holds tracked content only.
- Every host→weft junction is excluded through the warp's `.git/info/exclude`, never through a committed `.gitignore` in the user's repo.
- `internal/lyxdirs` is the single declarer of both `_lyx` and `.lyx`.

## Testing

**`internal/lyxdirs`** — no behaviour to test beyond the enforcement tests below; it is two consts.

**Enforcement tests (both TDD candidates — write them red first, they define the rules):**

1. Single-declarer test: no Go file outside `internal/lyxdirs` contains the literal `".lyx"` or `"_lyx"`.
   Model it on `cmd/lyx/boardguard_test.go`.
   Note the existing doc comments and error strings that mention `_lyx` in prose — the test must match declarations, not free text.
2. No-transients test: no path built on `lyxdirs.LyxDirName` ends in `.lock`, `pause`, or resolves into a `prompts` directory.
   This is what replaces the deleted `crossModuleMachineLocalExcludes` as a machine-checked rule.

**`internal/fabricengine`:**

- Integration test on a `lyxtest` synthetic hub (mirroring `junction_pattern_integration_test.go`): wiring creates the `.lyx` junction pointing at `<weft>/<AnchorRel>/.lyx`, seeds `.lyx` into the warp's `.git/info/exclude`, and seeds `.lyx/` into the weft's; unwiring removes the junction and the warp entry.
- Host-pristine guard: a pre-existing real `.lyx` directory produces the hard error, and the message names `.lyx` and the remedy (the no-migration decision).
- `neverCommittedNames`: a pathspec built from a config whose `pathspec` includes `.lyx` never contains a `.lyx` entry, and `classifyPaths` never routes a `.lyx` path to the weft side.
  TDD candidate.
- `weftPathspecFilter` with `--exclude-standard`: an entry matching only ignored files is filtered out and reported non-positive, so the commit is a clean no-op instead of a hard `git add` failure.
  TDD candidate — this one has a verified pre-fix failure to reproduce.
- `seedWeftArtifactExcludes` seeds `.lyx/` and no longer seeds the three cross-module patterns; idempotent on re-run.
- Reconcile/drift/health still converge with `.lyx` in the wired name-set.
- The split of `HubReservedNames` into slug-reservation versus junction-wiring sets: `.lyx` is refused as a worktree slug **and** survives into the wired junction set.
  TDD candidate — the naive one-list implementation fails exactly this pair.

**`internal/treadleengine`** — the scratch-dir seam: with no scratch dir set, locks and `pause` land in `runDir` (back-compat default); with one set, `run.lock`, `state.json.lock` and `pause` land there while `state.json` and round artifacts stay in `runDir`.
Mutual exclusion (`ErrBlockBusy`) and pause-clearing still work when the two directories differ.
TDD candidate.

**`internal/perchcli`** — the run handler passes the `.lyx`-anchored scratch dir, and the weft pathspec it builds still names only `_lyx`.

**`internal/websterengine` / `internal/builderengine` / `internal/loomengine`** — per-module path tests asserting each relocated artifact resolves under `.lyx` at the mirrored subpath, and that the durable files (`state.json`, `status.json`, reports) stay under `_lyx`.
Locking still serialises correctly when the lock file sits in a sibling tree from the file it guards.

**`internal/fabriccli`** — clone no longer writes a `.gitignore` entry; the resulting host worktree has `.lyx` in `.git/info/exclude` and a clean `git status`.
`unwire` no longer reverts a `.gitignore` block (delete `TestUnwire_RevertsGitignore`).

**Cross-platform** — junction creation goes through `internal/fslink`, already covered by `fslink_test.go`; no new platform-specific assertions needed beyond keeping the new tests off hard-coded separators.

## Q&A log

- **Q:** Skal `.lyx` bli en weft-backed junction registrert via fabric.yaml, eller forbli en vanlig host-katalog? **A:** Junction — `.lyx` skal fysisk ligge i weft og eksponeres fra warp via en junction som fabric lager ut fra sin yaml.
- **Q:** Skal eksisterende ekte `.lyx`-kataloger migreres automatisk? **A:** Nei. Ingen migration-støtte i det hele tatt — hard feil via den eksisterende host-pristine-guarden, operatøren sletter selv.
- **Q:** Gir det mening at `configengine` eier `.lyx`-navnet? **A:** Nei — eierskapet er arvet, ikke prinsipielt. Ny bladpakke `internal/lyxdirs` eier begge navn.
- **Q:** Hvordan flytter perch sine låser når treadle-motoren er geometri-blind? **A:** Treadle får en eksplisitt scratch-dir-inngang; perch-CLI-en oppgir `.lyx`-stien. Motoren blir fortalt, ikke utleder.
- **Q:** Skal treadles egen `pause`-flagg (ikke nevnt i briefen) også flytte? **A:** Ja — samme aldri-tracked klasse som webster og builder.
- **Q:** `.gitignore` eller `.git/info/exclude` på weft-siden? **A:** `.git/info/exclude`. Warp får aldri en committed `.gitignore` fordi den ville avslørt at LYX brukes i brukerens eget repo; weft er vår egen, så der er valget rent praktisk, og exclude-fila seedes allerede fra `ensureWeftLockDir`.
- **Q:** Hvor langt går sveipen etter feilplasserte transienter? **A:** Alle aldri-tracked artefakter under enhver modul-eid katalog, inkludert hver `*.lock` under `_lyx`, speilet til samme subpath under `.lyx`. Merk at enkelte moduler har egne toppnivå-mapper (som `_raddle`) — de er sjekket og holder ingen transienter i dag.
- **Q:** Hva med hub-nivå `.lyx` (`<hub>/.lyx/logs`)? **A:** Fabric skal anerkjenne den som et geometri-element, på samme måte som den anerkjenner `<hub>/_board` som weft:main. Fortsatt en ekte katalog, ingen junction.
- **Q:** Blir ikke en `.lyx`-fil i lista til `Fabric.Commit` bare ekskludert av `git add` selv, siden hele mappen er git-excluded? **A:** Filen blir aldri committet — men `git add` hopper ikke over den, den feiler hele invokasjonen (exit 1, ingenting staged, heller ikke lovlige filer i samme kall), og `lyx fabric sync` bygger pathspec-en sin fra den rå `cfg.Dirs()`. Derfor kreves begge deler: `neverCommittedNames` som holder `.lyx` ute av pathspec-bygging og commit-ruting, **og** `--exclude-standard` på `weftPathspecFilter`.
- **Q:** Rekkefølge internt i tasken? **A:** Transienter først, så slett eksluderings-maskineriet, så geometrien.
- **Q:** Testtilnærming? **A:** `lyxtest`-syntetisk hub-integrasjonstest for junction-livssyklusen, pluss unit-tester på hver flyttet sti, pluss to enforcement-tester (single-declarer og ingen-transienter-under-`_lyx`).
- **Q:** Dokumentasjon? **A:** `CONSTRAINTS.md` (pensjoner «Cross-module exclusions», ny invariant), `docs/overview.md` (ny `lyxdirs`-rad i modultabellen), berørte pakkedocs, og `manifest/designs/fabric-unified-view.md` slice 9.
