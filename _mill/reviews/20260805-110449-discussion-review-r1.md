MILL_REVIEW_BEGIN
# Review: fabric: shrink hubgeometry to the minimal illusion primitive (slice 7)

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.x-class model (exact build unknown; brief names it "opushigh")
reviewed_file: _mill/discussion.md
date: 2026-08-05
```

## Findings

### [GAP] lyxtest → fabricengine is an import cycle
**Section:** Decisions § leaf-invariant-updates
**Issue:** `internal/fabricengine/testmain_test.go:8,14` is `package fabricengine` (in-package) and imports `internal/lyxtest`; ~12 more in-package fabricengine test files do the same, so a production `lyxtest → fabricengine` import makes the fabricengine test binary uncompilable.
**Fix:** Decide the alternative — convert those test files to `package fabricengine_test`, or give `lyxtest` a private weft-suffix constant / injected paths instead of the fabricengine import — and record it before batch 1.

### [GAP] `_pattern` ownership contradicts moving PatternDir to internal/pattern
**Section:** Decisions § enforcement-rewrite vs § per-module-constructors
**Issue:** The ownership map assigns `_pattern` to `internal/fabricengine` only, while `PatternDir`/`PatternFile`/`PatternFileHere` move to `internal/pattern` (`pattern/pattern.go:63,86` consume them today) — and the Pattern Leaf Invariant forbids `pattern` importing `fabricengine`, which `leaf-invariant-updates` does not widen.
**Fix:** Add `internal/pattern` as a `_pattern` owner in the map, or state which package actually keeps the constructor.

### [GAP] `_board` in pathspec also changes the weft commit scope
**Section:** Decisions § board-junction
**Issue:** `pathspec` is dual-purpose: `fabriccli/weft_verbs.go:102` feeds the **raw unfiltered** `cfg.Dirs()` into `ScopedPathspec(l.RelPath, …)`, so adding `_board` injects `<rel>/_board` into the weft commit pathspec; only the junction half is filtered by `filterHubReserved`.
**Fix:** State how `ScopedPathspec` excludes `_board` (and whether `Topology.Add`'s reserved union is affected), not only `filterHubReserved`.

### [GAP] `_board` junction does not fit the junction record shape
**Section:** Decisions § board-junction
**Issue:** `HostJunctions`/`HostJunctionsHere` (`hubgeometry.go:565-591`) build every junction as Link=`<worktree>/<rel>/<name>`, Target=`<weftWorktree>/<rel>/<name>` with one name on both sides; a link named `.board` targeting `<HubPath>/_board` breaks both the name mapping and the target derivation, and every reconcile/health iteration over the wired names.
**Fix:** Specify the name/target exception (or drop `_board` from pathspec and wire it explicitly), including its effect on reconcile/drift/health checks.

### [GAP] "join onto AnchorPath()" is wrong for at least three constructors
**Section:** Decisions § constructor-anchoring
**Issue:** `HubLogsDir` is Hub-anchored (`hubgeometry.go:399`), and `WorktreeLogsDir`/`ScoutDaemonStateFile` use `dotLyxDirName` (`.lyx`, machine-local) not `_lyx` (`:365,406`); also the WorktreeRoot→AnchorPath change makes the stated "same absolute path before/after" test false for subpath-anchored repos.
**Fix:** Give a per-constructor base (AnchorPath / HubPath) and restate the before/after test as anchor-aware rather than identity.

### [GAP] Clone does call Resolve during steps 1–7
**Section:** Decisions § anchor-read-ownership (clone ordering fact)
**Issue:** `fabricengine/clone.go:112` calls `hubgeometry.Resolve(hostWorktreePath)` at step 5 for the post-checkout hook install, contradicting "today's code correctly does not call it" — the implementer is told to preserve a property that does not hold.
**Fix:** Correct the fact and state what that step-5 call becomes under the strict gate and `ResolveWithAnchor` (no anchor is known there yet).

### [GAP] Strict equality has no defined path-comparison semantics
**Section:** Decisions § strict-anchor-gate
**Issue:** The gate compares process cwd against `filepath.Join(HubPath, WorktreeName, AnchorRel)` where the worktree side comes from `git rev-parse --show-toplevel` (`hubgeometry.go:103-112`); symlinked paths, case-insensitive filesystems and separator normalization can make two identical directories compare unequal, and this now gates *every* invocation including unanchored repos where no gate ran before.
**Fix:** Name the comparison rule (Clean only, or EvalSymlinks, or case-folded on Windows) and cover it in the gate table test.

### [GAP] SiblingLayout's fate is stated two ways
**Section:** Scope (In) vs Technical context
**Issue:** Scope says "exactly three operations", while the post-shrink target keeps `SiblingLayout` (its only caller is `fabricengine/hostlayout.go:26`) and no decision gives its `Location`-era signature — under `HubPath`+`WorktreeName` it would take a name, not a `worktreeRoot`, and its `Cwd` field is explicitly called dishonest.
**Fix:** Decide whether `SiblingLayout` stays, moves into `fabricengine`, or becomes a `Location` derivation, and pin its signature.

### [GAP] Two doc surfaces missing from the update list
**Section:** Scope (In) / Constraints
**Issue:** `docs/shared-libs/hubgeometry.md` is a full module doc for the renamed package documenting the departing API, and `CONSTRAINTS.md:12` carries the same "joined onto `cwd` directly" wording that `constructor-anchoring` corrects only in `fabric-unified-view.md`; neither is listed.
**Fix:** Add both to batch 5's doc set (rename/rewrite or delete the shared-libs doc per the Documentation Lifecycle).

### [NOTE] internal/vscode gains a fabricengine dependency and a git spawn
**Section:** Decisions § prime-and-list-move
**Issue:** `vscode/color.go:47` imports only stdlib + `hubgeometry` today; sourcing the prime name from `fabricengine` pulls the fabric engine into a colour picker and reintroduces a `git worktree list` spawn per call.
**Fix:** Note the accepted dependency/spawn cost, or pass the prime name in from the already-fabric-aware caller.

### [NOTE] `.lyx` is absent from the token ownership map
**Section:** Decisions § enforcement-rewrite
**Issue:** `.lyx` is not policed today (`enforcement_test.go:224`), yet batch 2 spreads it to `logger`, `scoutengine` and `reedengine`; slice 9 then has to un-spread it.
**Fix:** Say explicitly that `.lyx` stays unpoliced this slice and is slice 9's problem.

## Verdict

GAPS_FOUND
A lyxtest import cycle plus unresolved `_board`, anchoring and gate semantics block plan writing.
MILL_REVIEW_END
