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

- A new zero-dependency leaf package `internal/lyxdirs` owning both directory-name tokens: `LyxDirName` (`_lyx`, moved out of `internal/configengine`) and `DotLyxDirName` (`.lyx`, replacing five private `dotLyxDirName` consts), plus amending every leaf-invariant allowlist and CONSTRAINTS clause that must now permit importing it.
- Making both `_lyx` and `.lyx` **structural** directories injected by `internal/fabricengine` in code, so neither is read from `fabric.yaml`'s configurable `pathspec` any more (the field survives for genuinely optional directories such as `_pattern`).
- Re-anchoring `.lyx` onto `AnchorPath()` so it is a directory sibling of `_lyx`, and updating the existing `WorktreePath()`-anchored consumers.
- Relocating **every** never-tracked transient currently written under `_lyx` into the mirrored subpath under `.lyx` (full inventory in Technical context).
- A new scratch-dir seam on `internal/treadleengine` so perch's run/state locks and pause flag can leave the run dir while the engine stays geometry-blind.
- Wiring `.lyx` as a weft-backed junction, replacing the committed `.gitignore` block in `clone.go`/`unwire.go` with the warp `.git/info/exclude` seeding every other junction already uses, plus a `.lyx`-only content-adoption path for worktrees that already hold a real `.lyx` directory.
- Removing `Unwire`'s deletion of the weft-side `_lyx` content, so unwire only reverses wiring and never destroys weft content.
- Splitting `fabricengine`'s never-committed directory set from its committed one, so `.lyx` never reaches a pathspec or weft commit routing, plus `--exclude-standard` on `weftPathspecFilter`'s `git ls-files` probe.
- Replacing the three `crossModuleMachineLocalExcludes` patterns with a single `.lyx/` entry in the weft repo's `.git/info/exclude`, seeded at wiring time, and deleting the `crossModuleMachineLocalExcludes` var.
- Recognising `<hub>/.lyx` as a hub-level geometry element owned by fabric, the way `<hub>/_board` already is.
- Two enforcement tests (single-declarer AST rule for the two directory tokens; a runtime no-transients-under-`_lyx` test).
- Docs in the same commit: `CONSTRAINTS.md`, `docs/overview.md`, the affected package docs, `manifest/designs/fabric-unified-view.md` slice 9, `tools/sandbox/SANDBOX-BUILDER-SUITE.md`, `tools/sandbox/SANDBOX-WEBSTER-SUITE.md` and `docs/reference/builder-contract.md`.

**Out:**

- Content adoption for any junction name other than `.lyx`.
  `_lyx` and `_pattern` keep today's hard refusal on a pre-existing real directory.
- A cleanup path for the committed `.gitignore` `.lyx/` block in repos cloned by a pre-fix binary.
  Both the `Ensure` and the `Remove` call sites are deleted outright, so no code path removes a leftover block;
  the only known affected repo is the sandbox, which is re-cloned anyway.
  The manual remedy (delete the `.lyx/` line from the lyx-managed block) is documented.
- Untracking transients a pre-fix `sync` already committed into weft history.
  They stay committed; `git rm --cached <path>` is documented as the manual remedy, not automated.
- Value-level migration of `fabric.yaml` (see the `structural-dirs-are-not-config` decision's rejected alternatives).
  `lyx config reconcile` already propagates new template **keys**; changing the **value** of an existing key is deliberately out of its contract, and the structural decision removes the need.
- Moving the weft-side `_lyx` deletion into the merge path.
  Unwire stops deleting it here; where (and whether) a merge step should delete it is a separate task.
- Moving `.weft/` or `gitrepo.PushLockFileName` out of `seedWeftArtifactExcludes` — those are fabric's own runtime artifacts, not module transients.
- Removing `fabric.yaml`'s `pathspec` field entirely — `_pattern` is genuinely optional per repo, so the field survives with a narrower job.
- `_raddle`, `_pattern` and `_board` content sweeps — they hold no known transients today.
  `internal/fabricengine`'s own `board.push.lock` (`bolt.go:34`, under `<hub>/_board`) and the correspondence index (`index.go:75`, inside the git dir) are fabric-internal and stay put.
- Supporting downgrade to a pre-fix binary (see `structural-dirs-are-not-config`).
- This repository's own hand-written `.gitignore` `.lyx/` block — loomyard is not fabric-wired, so its `.lyx` is a plain directory and the entry is correct as it stands.

## Decisions

### dotlyx-is-a-weft-backed-junction

- Decision: `.lyx` lives physically inside the weft worktree at `<weft>/<AnchorRel>/.lyx` and is exposed in the warp/host worktree as a directory junction created by fabric through `internal/fslink`, wired and unwired by exactly the same primitives as `_lyx` and `_pattern`.
  The committed `.gitignore` `.lyx/` block is removed from both `clone.go` and `unwire.go`; the junction name is excluded through the warp's `.git/info/exclude` via the existing `seedGitExclude` path.
- Rationale: it is the design doc's stated target (slice 9, `manifest/designs/fabric-unified-view.md`) and it needs no bespoke code — `.lyx` inherits wire/unwire/reconcile/health/drift handling for free, and one mechanism covers every host→weft directory instead of two.
  The committed `.gitignore` in the user's own repo is the specific thing being eliminated: it makes LYX visible in a repo LYX does not own.
- Rejected: **plain real `.lyx` directory in the host worktree, excluded only through the warp's `.git/info/exclude`.**
  This also achieves "no tracked artifact in the user's repo" and avoids placing machine-local state inside a git worktree, but it needs a standalone exclude-seeding call site (a third shape alongside `_lyx`'s junction and `_board`'s hub link), and `.lyx` then has no lifecycle at all — nothing wires it, nothing reports it, nothing tears it down with the pair.
  Also rejected: anchoring `.lyx` at the hub instead of per worktree (a far larger relocation of every module's current path, destroying per-worktree isolation of machine-local state).
- Consequence to keep in view: machine-local scratch now sits inside a git worktree, so every operation that touches the weft worktree must be checked against it — which is why the unwire decision below exists and why the weft-side exclude must be seeded before the first write.

### structural-dirs-are-not-config

- Decision: `_lyx` and `.lyx` are **structural** directories injected by `internal/fabricengine` in code, not entries in `fabric.yaml`'s `pathspec`.
  `fabricengine` declares two sets: `structuralCommittedDirs` (today exactly `lyxdirs.LyxDirName`) and `structuralNeverCommittedDirs` (today exactly `lyxdirs.DotLyxDirName`).
  The `pathspec` field survives for genuinely optional directories only; `template.yaml`'s default shrinks from `_lyx _pattern` to `_pattern`.
- Rationale: CONSTRAINTS states *geometry is structural, never config/env-overridable*, and both directories must always exist — every lyx module fails without them.
  A `fabric.yaml` that drops `_lyx` from `pathspec` would tear away the entire durable tree, and one that omits `.lyx` would leave machine-local scratch unwired.
  Making one configurable and the other not would be incoherent.
  This also removes the rollout problem: an already-deployed `fabric.yaml` needs no migration, because neither name is read from it any more.
- Rejected: adding `.lyx` to the yaml `pathspec` and changing `template.yaml`'s default (leaves obligatory geometry switchable per repo, and every existing repo needs a value-level migration `lyx config reconcile` deliberately does not do);
  code-injecting `.lyx` into the loaded pathspec only when missing (same effect while pretending yaml owns it);
  **listing every junction — structural ones included — in one yaml list** (tidier and more uniform on its face, and the operator's own first instinct, but it makes mandatory geometry look optional: a list you can edit is a list you can empty, and `_lyx`/`.lyx` must simply be there);
  removing `pathspec` altogether (`_pattern` really is per-repo optional).
- **Deployed configs still parse.**
  An existing `fabric.yaml` in `weft:main` keeps `pathspec: _lyx _pattern`, and `Config.Dirs()` (`config.go:28`, `strings.Fields`) still parses it — so `_lyx` arrives from **two** sources.
  Every set below is therefore a **deduplicated** union, not a concatenation;
  without dedup, duplicate names reach `HostJunctions`, `ScopedPathspec` and status output.
- **Set composition, exactly** (each a dedup-preserving union in the stated order — structural names first, then config names in `Dirs()` order, first occurrence wins):
  - Wired junction name-set (`WiredNames`/`RepoWiredNames`) = `structuralCommittedDirs` ∪ `structuralNeverCommittedDirs` ∪ `filterHubReserved(cfg.Dirs())`.
    This is what gets junctions and warp `.git/info/exclude` entries.
  - Pathspec / commit-routing set = `structuralCommittedDirs` ∪ `filterHubReserved(cfg.Dirs())` — **never** `structuralNeverCommittedDirs`.
    This is what `internal/fabriccli/weft_verbs.go:100` builds `ScopedPathspec` from and what `classifyPaths` routes on.
  - Slug-reservation union (`Topology.Add`, `IsReservedHubName`) = both structural sets ∪ raw `cfg.Dirs()` ∪ the hub-reserved names.
    No filtering here: a worktree slug must be refused for every one of these names.
- **Downgrade is unsupported, one-way upgrade only.**
  A pre-fix binary's `applyStaleRemoval` (`reconcile.go:391`) removes on-disk junctions absent from *its* `RepoWiredNames`, so running an older `lyx fabric reconcile` after this change unwires `.lyx` and strands scratch inside the weft worktree.
  Record this in the module doc; do not attempt to make the change downgrade-safe.

### never-committed-is-structural-not-configurable

- Decision: membership in `structuralNeverCommittedDirs` is what makes `.lyx` uncommittable.
  The filtering lives where the pathspec and the commit routing are **constructed** — `ScopedPathspec`'s callers and `classifyPaths` — and never inside `Config.Dirs()`, `WiredNames`, or the slug-reservation union, all three of which must keep seeing every name.
  Separately, `weftPathspecFilter`'s `git ls-files --cached --others` probe gains `--exclude-standard`.
- Rationale: the two halves fix different failures.
  Keeping `.lyx` out of the pathspec means it is never even *named* in a git invocation — necessary because `git add` on an ignored path **fails the entire invocation with exit 1** and stages nothing, not even the legitimate `_lyx` files named in the same call (verified empirically, see Technical context).
  `--exclude-standard` fixes a separate latent bug: `git ls-files --cached --others -- <entry>` currently *matches* ignored files, so any stray ignored file matching any pathspec entry makes `weftPathspecFilter` forward a doomed entry and topple a whole commit.
  With the flag, such an entry is filtered out and the commit is a clean no-op.
  Encoding "never committed" as a fabric.yaml field is forbidden by the same CONSTRAINTS clause as above.
- Rejected: relying on git's own refusal alone (the guarantee is absolute but the failure is a cryptic hard error that blocks unrelated content);
  silently dropping `.lyx` paths at commit time (hides caller bugs);
  filtering inside `Dirs()` or `WiredNames` (would silently undo the junction wiring and the slug reservation).

### unwire-never-deletes-weft-content

- Decision: `Unwire` stops deleting the weft-side `_lyx` content.
  It reverses wiring only — removes host junctions and their warp `.git/info/exclude` entries — and leaves every weft-side directory intact, `_lyx` and `.lyx` alike.
  The `os.RemoveAll(weftLyxDir)` call and the follow-on `"lyx fabric unwire: clear _lyx"` commit and push (`unwire.go:92-105`) are removed, and `UnwireVerbResult.WeftContent` loses its `"cleared"` state.
  The weft-side `.lyx` is never touched by unwire either; it disappears with the weft worktree when `Remove` tears the pair down.
- Rationale: "unwire" means the inverse of "wire" — remove the junction coupling — not "delete the source directory".
  The current behaviour is already self-inconsistent: `unwire.go:30` records that weft `_pattern` content is *preserved by design* while `_lyx` is deleted and the deletion committed, with no stated reason for the asymmetry, and the doc comment simultaneously promises that "a later `lyx fabric reconcile` re-wire can recreate this worktree's wiring" — true of the wiring, false of the content it destroyed.
  Once `.lyx` lives in the weft worktree the stakes rise sharply: `_lyx` deletions are at least recoverable from git history, whereas `.lyx` is never committed, so any deletion of it is final.
  Fixing it here rather than later avoids giving the new `.lyx` contract an inconsistency we have already named, and this task edits `unwire.go` regardless.
- Rejected: deleting `.lyx` symmetrically with `_lyx` (simplest to explain, but destroys live daemon state in an otherwise reversible operation);
  leaving `_lyx` deletion alone and defining only `.lyx` behaviour (the incoherence would survive at least through slice 10);
  moving the deletion into the merge path in this task (the operator's view is that weft `_lyx` should be cleared before a branch merges, but that is a new step in the merge sequence, not a removal, and belongs to its own task).
- Test consequence: existing unwire tests asserting the clear-and-commit behaviour are rewritten to assert preservation.

### dotlyx-and-lyx-are-directory-siblings

- Decision: `.lyx` is anchored at `AnchorPath()`, exactly like `_lyx` — the two are directory siblings.
  The consumers currently anchored at `l.WorktreePath()` re-anchor as part of this slice: `logger/sink.go:37`, `shuttleengine/rundir.go:51` and `run.go:185`, `scoutengine/daemonstate.go:43,50`, `burlerengine/engine.go:106`, and every `reedengine` state site (`spawn.go:143,183`, `strand.go:309,348,471`, `lifecycle.go:545,676,778,961`, `lock.go:66`).
- Rationale: the junction lands at `<worktree>/<AnchorRel>/.lyx` (`HostJunctions`, `junction.go:61-62`) and `_lyx` is `AnchorPath()`-anchored (`builderengine/state.go:33`, `perchengine/identity.go:33`).
  Leaving those sites on `WorktreePath()` gives a subpath-anchored repo (`AnchorRel != "."`) two distinct `.lyx` roots — one junctioned and excluded, one not — which is exactly the class of bug this task exists to remove.
- Rejected: moving the junction to `WorktreePath()` (makes `.lyx` the only junction not `AnchorRel`-anchored);
  leaving those sites alone (two legal `.lyx` roots).
- Note: `reedengine.HubLogsDir` (`lifecycle.go:38`) is hub-anchored and deliberately unaffected — see the hub-level decision below.

### dotlyx-content-adoption-no-other-migration

- Decision: when wiring finds a **real directory** (`fslink.IsLink(link) == false`) at the `.lyx` junction path, fabric moves its contents into the weft-side target and replaces it with the junction.
  This adoption is **`.lyx`-only**: `_lyx`, `_pattern` and every other junction name keep today's hard refusal in `seedLyxJunction:163`.
  **Collision rule: refuse.**
  If any entry being moved already exists in the weft-side target, adoption aborts with an error naming the colliding path and leaves both sides untouched — fabric never overwrites or deletes content.
  A collision means an earlier adoption already ran, and `.lyx` is disposable enough that the operator can delete the host-side copy.
  **Precondition: no live process may hold the directory.**
  On Windows — the platform junctions exist for — moving a directory with open handles fails, and on POSIX a running writer keeps writing into the moved inode.
  Adoption must surface that failure as an actionable error ("stop reed/scout, then re-run `lyx fabric reconcile`"), never a partial move.
  No other migration exists — transients already committed into weft history stay committed, with `git rm --cached` documented as the manual remedy.
- Rationale: every worktree in existence today has a real `.lyx` (logger, reed, shuttle, scout and burler write it unconditionally), so without adoption the first `reconcile` after this change hard-errors everywhere, and the only remedy would be deleting live reed/scout daemon state.
  Content under `.lyx` is always lyx's own machine-local scratch, so the guard's rationale — never touch what might be the user's hand-authored content — does not apply there.
  It does apply to `_lyx` and `_pattern`, where the refusal stays.
- Rejected: hard-erroring on `.lyx` and documenting a manual stop-daemons-then-delete step (an upgrade cliff on every existing worktree, destroying live state);
  deleting the real directory silently (same destruction, without warning);
  extending adoption to every junction name (fabric would start moving user content unasked);
  letting the weft-side copy win on collision (silently discards data fabric was asked to preserve).

### weft-side-exclusion-via-git-info-exclude

- Decision: the weft repo keeps `.lyx` untracked through a single `.lyx/` line in its `.git/info/exclude`.
  `seedWeftArtifactExcludes` remains the **sole owner** of that file's content;
  `WireJunctions` calls it for the weft side at wiring time, and `ensureWeftLockDir` keeps calling it as the self-healing path.
  The three `crossModuleMachineLocalExcludes` patterns are deleted and the var removed.
  No `.gitignore` is committed in weft either.
- Rationale: the warp rule ("never a committed `.gitignore`") is about not exposing LYX in the user's repo;
  weft is LYX's own repo, so the choice there is purely practical, and `.git/info/exclude` wins — no commit, no pathspec change, no new file in the weft root, and one `.lyx/` pattern replaces three deep wildcard patterns.
  Seeding at wiring time closes the ordering hole: wiring already materialises the weft-side target (`junction.go:118`), so the exclude entry is guaranteed to exist before anything writes into `.lyx`.
  Seeding only from `ensureWeftLockDir` would leave the window between wiring and the first weft-git verb open, during which scratch shows as untracked dirt and trips `Remove`'s no-force dirty gate.
  Keeping `ensureWeftLockDir` as a second *call site* of the same single owner preserves self-healing on machines that never re-wire.
- Rejected: a committed `.gitignore` at the weft root (needs a new staging path, since the weft root sits outside every caller's pathspec);
  seeding only from `ensureWeftLockDir` (ordering hole);
  seeding only from wiring (loses self-healing);
  keeping the three deep wildcard patterns (the machinery this task exists to delete).

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
- **Blocking consequence: leaf allowlists must be amended in the same change.**
  `internal/scoutengine/leaf_enforcement_test.go:22-28` allows exactly `{configengine, lock, proc, logger, yaml}`, so `daemonstate.go` importing `lyxdirs` fails `TestLeafInvariant_AllowlistOnly` on day one, and CONSTRAINTS' Scoutengine Leaf Invariant names the same set in prose.
  The plan must sweep every enforcement test and matching CONSTRAINTS clause and add `internal/lyxdirs` where the package now needs it: `internal/scoutengine/leaf_enforcement_test.go` (confirmed), plus `internal/githubclient/leaf_enforcement_test.go`, `internal/lyxtest/leaf_enforcement_test.go`, `internal/treadleengine/seam_enforcement_test.go`, `internal/shuttleengine/seam_enforcement_test.go`, `internal/lyxcwd/enforcement_test.go` and `cmd/lyx/constructoranchoring_test.go` — each checked, amended only where that package actually imports `lyxdirs`.

### treadle-gains-an-explicit-scratch-dir

- Decision: `internal/treadleengine`'s engine gains an explicit scratch-directory input, defaulting to the existing `runDir` when unset.
  `run.lock`, `state.json.lock` and the `pause` flag are written there instead of in `runDir`; `state.json` and round artifacts stay in `runDir`.
  `internal/perchcli` supplies `<AnchorPath>/.lyx/perch/<block>` as the scratch dir while `runDir` stays `_lyx/perch/<block>`.
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
  `seedLyxJunction:163` is the host-pristine guard that gains the `.lyx`-only adoption branch;
  `junction.go:118` is where the weft-side target is materialised, which is the natural home for the weft-side exclude seeding.
- `junctionnames.go` — `BoardDirName`, `HubReservedNames`, `IsReservedHubName`, `filterHubReserved`, `junctionNames`/`WiredNames`/`RepoWiredNames`.
  `WiredNames(baseDir)` today returns `filterHubReserved(cfg.Dirs())` and must gain the structural sets with dedup.
- `config.go` — `Config.Pathspec` (a single free-form space-separated string, `config.go:23`) and `Config.Dirs()` (`strings.Fields`, `config.go:28`).
  The repo-wide file is `<Hub>/_board/_lyx/config/fabric.yaml`;
  the default lives in `internal/fabricengine/template.yaml:2` (`pathspec: _lyx _pattern`, shrinking to `_pattern`).
- `commit.go:91` — `Fabric.Commit` calls `RepoWiredNames(l)` then `classifyPaths(l.AnchorRel, wiredNames, files)`.
  This call site must switch from the wired set to the pathspec/commit-routing set.
- `weftgit.go` — `crossModuleMachineLocalExcludes` (lines 56-96, to delete), `seedWeftArtifactExcludes` (98-162, gains `.lyx/`, loses the cross-module patterns), `weftPathspecFilter` (177-199, gains `--exclude-standard` at `entryMatchesWeft:205`).
- `unwire.go:87-105` — the weft-`_lyx` clear-and-commit block to remove, with `UnwireVerbResult.WeftContent` (`unwire.go:29-31`) losing `"cleared"`;
  `unwire.go:30` already documents that `_pattern` weft content is preserved by design, which is the behaviour `_lyx` converges on.
- `reconcile.go:391` — `applyStaleRemoval`, the reason downgrade is unsupported.

### Config reconciliation, for context

`lyx config reconcile` (`internal/configcli/configcli.go:334`, dry-run by default, `--apply` to write) → `configsync.ReconcileAll` → `yamlengine.Reconcile` (`reconcile.go:21`) merges each config file against its template at **leaf key-path** level: keys present in the template but missing from the file are added, keys absent from the template are reported and removed, and existing values are preserved.
`configsync` even has value-migration precedent (`legacyFabricConfigModules` folds pre-cutover `warp.yaml`/`weft.yaml` into `fabric.yaml`).
What it deliberately does not do is rewrite the **value** of an existing key — which is exactly what adding `.lyx` to `pathspec`'s free-form string would have required, and exactly why the structural decision avoids the problem instead of solving it.

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

Already in `.lyx` but **`WorktreePath()`-anchored**, so they change twice — adopt `lyxdirs.DotLyxDirName` *and* re-anchor onto `AnchorPath()`: `scoutengine/daemonstate.go:43,50`, `shuttleengine/rundir.go:51` and `run.go:185`, `burlerengine/engine.go:106`, `logger/sink.go:37`, and `reedengine`'s state sites (`spawn.go:143,183`, `strand.go:309,348,471`, `lifecycle.go:545,676,778,961`, `lock.go:66`).

Hub-anchored and deliberately unchanged in anchor: `reedengine.HubLogsDir` (`lifecycle.go:38`), which only adopts the shared const.

Out of scope, confirmed not under `_lyx`: `fabricengine`'s correspondence index (`index.go:75`, inside the git dir) and `board.push.lock` (`bolt.go:34`, under `<hub>/_board`).

### Call sites that consume the pathspec

`internal/fabriccli/weft_verbs.go:100` builds `fabricengine.ScopedPathspec(l.AnchorRel, cfg.Dirs())` and passes it to `fab.Commit` at :155, :184 and :240.
With `_lyx` no longer in `template.yaml`'s default, this site must build from the pathspec/commit-routing set (`structuralCommittedDirs` ∪ `filterHubReserved(cfg.Dirs())`) or a freshly-initialised repo silently stops syncing `_lyx` at all.
This is the single most breakage-prone edit in the task.

`Topology.Add`'s reserved-name union also reads raw `Dirs()`;
it must gain both structural sets so no worktree slug can be named `_lyx` or `.lyx`.

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
- `cmd/lyx/boardguard_test.go` and `cmd/lyx/rawgitmutation_test.go` — the shape to copy for the new AST enforcement test.

### Docs that assert the old behaviour

- `tools/sandbox/SANDBOX-BUILDER-SUITE.md:276,279` — an entire scenario built on machine-local artifacts living under `_lyx` and being held back by the exclude layer.
  It must be rewritten to assert the artifacts are under `.lyx` instead;
  the underlying property it checks (never committed, never materialised on another machine) survives.
  The sandbox is also the one known repo carrying a committed `.gitignore` `.lyx/` block from a pre-fix binary.
- `tools/sandbox/SANDBOX-WEBSTER-SUITE.md:128` — names `_lyx/webster/prompts/02-*.md`.
- `docs/reference/builder-contract.md:166` — describes `*.lock` and `*/builder/pause` being kept out "solely at the git-exclude layer (`fabricengine.seedWeftArtifactExcludes`)", the mechanism this task deletes.

## Constraints

From `CONSTRAINTS.md`:

- **Cwd Resolution Invariant** — `internal/lyxcwd` owns cwd resolution alone and exposes only `RepoName`, `HubPath`, `WorktreeName`, `AnchorRel`, `WorktreePath()`, `AnchorPath()`.
  It never resolves a weft path, a junction path, or any per-module subdirectory; weft-sibling and junction construction belong to `internal/fabricengine`.
  A module's own subdirectory is that module's private relative-path constant joined onto `AnchorPath()` — per-segment joins, never a fused `"_lyx/..."` literal.
  `lyxcwd`'s imports stay capped at stdlib plus `internal/gitexec`, which is what keeps `fabricengine → logger → lyxcwd` acyclic.
  **Geometry is structural, never config/env-overridable** — the direct basis for the structural-dirs and never-committed decisions.
- **Scoutengine Leaf Invariant** — names the allowlist that must gain `internal/lyxdirs`; the prose clause and `leaf_enforcement_test.go` must change together.
- **lyxtest Leaf Invariant** and **CLI/Cobra Invariant** — no new commands here, but `internal/lyxdirs` must itself stay a leaf (stdlib-only) or it defeats its own purpose.
- **Weft Git Invariant / "Cross-module exclusions"** (`CONSTRAINTS.md:144-152`) — the mechanism this task retires.
  Its "known limitation" note (a pre-fix sync's already-committed artifacts need manual `git rm --cached`) survives as the documented manual remedy.
- **Documentation Lifecycle** — a task touching a module doc, the module table, or a cross-cutting invariant updates them in the same commit.
- **Tier 1/2 substrate rule** — tests must be tiered correctly; junction wiring on a synthetic hub is substrate-backed.

New invariants this task records in `CONSTRAINTS.md`:

- Every never-tracked file lives under `.lyx`, at the mirrored subpath of the `_lyx` content it relates to; `_lyx` holds tracked content only.
- `_lyx` and `.lyx` are directory siblings under `AnchorPath()`, both structural — injected by `fabricengine`, never read from `fabric.yaml`.
- Every host→weft junction is excluded through the warp's `.git/info/exclude`, never through a committed `.gitignore` in the user's repo.
- Unwiring reverses wiring only; it never deletes weft-side content.
- `internal/lyxdirs` is the single declarer of both `_lyx` and `.lyx`.

## Testing

**`internal/lyxdirs`** — no behaviour to test beyond the enforcement tests below; it is two consts.

**Enforcement test 1 — single declarer (TDD candidate).**
An AST-based test (parse each non-test Go file under `internal/` and `cmd/`, walk for `*ast.BasicLit` string values) asserting that no file outside `internal/lyxdirs` contains a string literal whose value is **exactly** `"_lyx"` or `".lyx"`.
Exact-value matching is what makes it implementable: `".lyx-anchor"` (`lyxcwd/anchor.go:41`) and `".lyx/"` are distinct literals and do not match, and the surviving `.lyx/` exclude entry is built as `lyxdirs.DotLyxDirName + "/"` rather than as a literal.
14 non-test files match today and must all be converted;
test files are out of the rule's scope (71 files match overall, mostly fixtures).
Model it on `cmd/lyx/boardguard_test.go`.

**Enforcement test 2 — no transients under `_lyx` (TDD candidate).**
A runtime test on a `lyxtest` synthetic hub rather than a static rule: drive each module's exported path constructors (`websterengine.Dir`/`PromptsDir`, `builderengine.Dir`, `perchengine.RunsDir`, `loomengine`'s status path, and treadle's run/scratch dirs) against a synthetic `Location`, then assert no returned `_lyx`-rooted path ends in `.lock`, equals `pause`, or resolves inside a `prompts` directory — and that each corresponding transient path *does* resolve under `.lyx`.
This checks actual behaviour instead of attempting the dataflow analysis a static rule would need.

**`internal/fabricengine`:**

- Junction lifecycle on a `lyxtest` synthetic hub (mirroring `junction_pattern_integration_test.go`): wiring creates the `.lyx` junction pointing at `<weft>/<AnchorRel>/.lyx`, seeds `.lyx` into the warp's `.git/info/exclude` **and** `.lyx/` into the weft's, and unwiring removes the junction and the warp entry.
  TDD candidate.
- Seeding order: after wiring and before any weft-git verb runs, writing a file into `.lyx` leaves `git status --porcelain` in the weft worktree clean.
  Regression test for the ordering hole.
- Unwire preserves weft content: after `Unwire`, both `<weft>/<AnchorRel>/_lyx` and `<weft>/<AnchorRel>/.lyx` still exist with their content, no `"lyx fabric unwire: clear _lyx"` commit was made, and `WeftContent` never reports `"cleared"`.
  Existing tests asserting the old clear-and-commit behaviour are rewritten.
  TDD candidate.
- `.lyx` content adoption: a pre-existing real `.lyx` directory holding files is moved into the weft target and replaced by a junction, idempotently;
  a name collision in the target aborts with both sides untouched;
  a pre-existing real `_lyx` or `_pattern` directory still produces the hard refusal.
  TDD candidate — the halves must be asserted together or the adoption branch will over-reach.
- Structural sets: `WiredNames` returns `_lyx` and `.lyx` even for a `fabric.yaml` whose `pathspec` names neither;
  a deployed `pathspec: _lyx _pattern` yields **no duplicate** `_lyx` in any of the three sets;
  the pathspec/commit-routing set contains `_lyx` but never `.lyx`;
  `classifyPaths` never routes a `.lyx` path to the weft side.
  TDD candidate.
- `weftPathspecFilter` with `--exclude-standard`: an entry matching only ignored files is filtered out and reported non-positive, so the commit is a clean no-op instead of a hard `git add` failure.
  TDD candidate — this one has a verified pre-fix failure to reproduce.
- `seedWeftArtifactExcludes` seeds `.lyx/` and no longer seeds the three cross-module patterns; idempotent on re-run.
- Reconcile/drift/health still converge with `.lyx` in the wired name-set.
- The split of `HubReservedNames` into slug-reservation versus junction-wiring sets: `.lyx` is refused as a worktree slug **and** survives into the wired junction set.
  TDD candidate — the naive one-list implementation fails exactly this pair.

**`internal/fabriccli`** — `lyx fabric sync` still commits `_lyx` content with `_lyx` gone from the template default (the regression guard for the `weft_verbs.go:100` edit), and never names `.lyx`.
Clone no longer writes a `.gitignore` entry;
the resulting host worktree has `.lyx` in `.git/info/exclude` and a clean `git status`.
`unwire` no longer reverts a `.gitignore` block (delete `TestUnwire_RevertsGitignore`).

**`internal/treadleengine`** — the scratch-dir seam: with no scratch dir set, locks and `pause` land in `runDir` (back-compat default); with one set, `run.lock`, `state.json.lock` and `pause` land there while `state.json` and round artifacts stay in `runDir`.
Mutual exclusion (`ErrBlockBusy`) and pause-clearing still work when the two directories differ.
TDD candidate.

**`internal/perchcli`** — the run handler passes the `.lyx`-anchored scratch dir, and the weft pathspec it builds still names only `_lyx`.

**`internal/websterengine` / `internal/builderengine` / `internal/loomengine`** — per-module path tests asserting each relocated artifact resolves under `.lyx` at the mirrored subpath, and that the durable files (`state.json`, `status.json`, reports) stay under `_lyx`.
Locking still serialises correctly when the lock file sits in a sibling tree from the file it guards.

**Anchor re-parenting** — for a synthetic hub with `AnchorRel != "."`, every relocated and every pre-existing `.lyx` consumer resolves under `AnchorPath()`, and there is exactly one `.lyx` root in the worktree.
Regression test for the two-roots bug.

**Leaf allowlists** — each amended `leaf_enforcement_test.go` / seam test still passes, and no package gains `internal/lyxdirs` in its allowlist without actually importing it.

**Cross-platform** — junction creation goes through `internal/fslink`, already covered by `fslink_test.go`; no new platform-specific assertions needed beyond keeping the new tests off hard-coded separators.
The adoption path's busy-directory behaviour is the one genuinely platform-divergent case and should be asserted through its error contract rather than by simulating open handles.

## Q&A log

- **Q:** Skal `.lyx` bli en weft-backed junction, eller forbli en vanlig host-katalog? **A:** Junction — `.lyx` skal fysisk ligge i weft og eksponeres fra warp via en junction som fabric lager.
- **Q:** Skal eksisterende ekte `.lyx`-kataloger migreres? **A:** Opprinnelig nei (hard feil). Revidert etter review-runde 1: ja, men **kun** for `.lyx`. Uten adopsjon hard-feiler første `reconcile` i hver eneste eksisterende worktree, og eneste botemiddel ville vært å slette levende daemon-state.
- **Q:** Gir det mening at `configengine` eier `.lyx`-navnet? **A:** Nei — eierskapet er arvet, ikke prinsipielt. Ny bladpakke `internal/lyxdirs` eier begge navn.
- **Q:** Hvordan flytter perch sine låser når treadle-motoren er geometri-blind? **A:** Treadle får en eksplisitt scratch-dir-inngang; perch-CLI-en oppgir `.lyx`-stien. Motoren blir fortalt, ikke utleder.
- **Q:** Skal treadles egen `pause`-flagg (ikke nevnt i briefen) også flytte? **A:** Ja — samme aldri-tracked klasse som webster og builder.
- **Q:** `.gitignore` eller `.git/info/exclude` på weft-siden? **A:** `.git/info/exclude`. Warp får aldri en committed `.gitignore` fordi den ville avslørt at LYX brukes i brukerens eget repo; weft er vår egen, så der er valget rent praktisk.
- **Q:** Hvor langt går sveipen etter feilplasserte transienter? **A:** Alle aldri-tracked artefakter under enhver modul-eid katalog, inkludert hver `*.lock` under `_lyx`, speilet til samme subpath under `.lyx`.
- **Q:** Hva med hub-nivå `.lyx` (`<hub>/.lyx/logs`)? **A:** Fabric anerkjenner den som geometri-element, slik den anerkjenner `<hub>/_board` som weft:main. Fortsatt ekte katalog, ingen junction.
- **Q:** Blir ikke en `.lyx`-fil i lista til `Fabric.Commit` bare ekskludert av `git add` selv? **A:** Filen blir aldri committet, men `git add` feiler hele invokasjonen (exit 1, ingenting staged). Derfor kreves både at `.lyx` holdes ute av pathspec-bygging og commit-ruting, **og** `--exclude-standard` på `weftPathspecFilter`.
- **Q (gap r1):** Hvilket anker bruker `.lyx`? **A:** `AnchorPath()`. `_lyx` og `.lyx` skal være mappesøsken; begge havner i anchor.
- **Q (gap r1):** Hvordan får en allerede utrullet `fabric.yaml` `.lyx` i seg? **A:** Den skal ikke. `_lyx` og `.lyx` blir strukturelle og injiseres i kode; `pathspec` beholdes kun for valgfrie kataloger.
- **Q (gap r1):** Én eier av weft-sidens exclude, og garantert rekkefølge? **A:** `seedWeftArtifactExcludes` er eneste eier; `WireJunctions` kaller den ved wiring, `ensureWeftLockDir` beholdes som selv-heling.
- **Q (gap r1):** Mekanismen for enforcement-testene? **A:** Test 1 AST-regel med eksakt literal-match på ikke-test-filer; test 2 kjøretidstest på syntetisk hub, siden en statisk regel ville krevd dataflyt-analyse.
- **Q (gap r2):** Burde ikke *alle* junctions stått i én yaml-liste, for ryddighetens skyld? **A:** Ideelt sett ja, men alt i lyx feiler om `_lyx` eller `.lyx` mangler — de må være der. Obligatorisk geometri kan ikke ligge i en liste man kan tømme. Ført opp som avvist alternativ.
- **Q (gap r2):** Skal denne tasken bygge verdi-nivå-migrering for `fabric.yaml`? **A:** Nei. `lyx config reconcile` propagerer allerede nye *nøkler*; verdi-migrering er et eget designproblem og den strukturelle beslutningen fjerner behovet.
- **Q (gap r2):** Hva skjer med weft-sidens `.lyx` ved `unwire`? **A:** Ingenting — og det avdekket at `unwire` i dag sletter weft-sidens `_lyx`, hvilket er feil. «Unwire» betyr det motsatte av «wire»: fjern junction-koblingen, ikke slett kildemappa. Rettes i denne tasken. Slettingen av weft-`_lyx` hører hjemme før en branch merges inn — et helt annet steg, og en egen task. `.lyx` trenger ikke røres, den forsvinner med worktreet.
- **Q (gap r2):** Skal repoer klonet av en eldre binær få den committede `.gitignore`-blokka ryddet? **A:** Nei, utenfor scope. Vi har omtrent ingen slike repoer — kun sandboxen, som re-klones uansett.
