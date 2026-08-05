MILL_REVIEW_BEGIN
# Review: fabric: shrink hubgeometry to the minimal illusion primitive (slice 7)

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: claude-opus-5 (best-effort self-assessment; matches the model ID reported by my environment)
reviewed_file: /home/knatte/Code/loomyard/wts/fabric-illusion-core/_mill/discussion.md
date: 2026-08-05
```

## Findings

### [GAP] Stale-sweep premise for `_board` is factually wrong
**Section:** `board-junction`, git-hygiene point (b)
**Issue:** The claim that `scanOnDiskJunctionNames` (`reconcile.go:325`) "lists **every** link" is false — `reconcile.go:332-341` skips every name in `hubgeometry.HubReservedNames()`, and that set is `{_board, _portals, _launchers, _raddle}` (`hubgeometry.go:317`), so `_board` is already invisible to the stale sweep and needs no known-name-set addition.
**Fix:** Restate (b) as "no change required — the reserved-name skip already protects the link", keep the regression test that asserts it, and note that this same skip is exactly why (c)'s explicit `Unwire` case *is* required.

### [GAP] `_board` is not covered by the drift/health checks as claimed
**Section:** `board-junction` ("covered by the same drift/health checks"); Testing ("repaired by reconcile when missing or mispointed")
**Issue:** `Healthy` (`drift.go:67-75`), `checkJunctionHealth` (`reconcile.go:267-272`) and `junctionRepointedDetail` (`reconcile.go:307-312`) all iterate `HostJunctionsHere(RepoWiredNames(l))`, i.e. the pathspec name-set the decision deliberately keeps `_board` out of — so nothing checks or repairs the link, and reconcile's re-wire (`reconcile.go:141`) never touches it.
**Fix:** Enumerate the named `_board` special case at each of those sites (health, repair, repointed-detail) the way clone/add/reconcile wiring is enumerated, or state explicitly that the link is wire-only and unmonitored.

### [GAP] `LyxDirName`'s destination contradicts the `_lyx` token owner
**Section:** `weft-junction-move` vs `config-path-move` / `enforcement-rewrite` map
**Issue:** `LyxDirName` is sent to `fabricengine`, but `configengine` needs `_lyx` (`config.go:23` `FindBaseDir`, plus the incoming `ConfigDir`) and `fabricengine/config.go:16` already imports `configengine` — so declaring it in `fabricengine` is a compile-time cycle; the token map meanwhile names `configengine` as `_lyx`'s owner and omits `fabricengine`'s own bare-name uses (`unwire.go:84`, `reconcile.go:218`, `weftgit.go:94-96`) and `ideengine/menu.go:53`.
**Fix:** Name one declaring owner (`configengine`) for `_lyx`, state that `fabricengine`/`ideengine` import it, and drop `LyxDirName` from the `weft-junction-move` constant list.

### [GAP] `Location.WorktreePath()` collides with `WorktreePath(slug)` in batch 1
**Section:** `location-struct`, `batching` (batch 1 vs batch 3)
**Issue:** `WorktreePath(slug)` is a method on the same type today (`hubgeometry.go:411`), so adding the no-arg `Location.WorktreePath()` in batch 1 while the slug variant only leaves in batch 3 is a duplicate-method compile error; its one non-`fabric*` production caller, `ideengine/spawn.go:20`, is listed in neither `seven-leak-fixes` nor `prime-and-list-move`.
**Fix:** Move or rename the slug variant in batch 1 alongside the accessor, and add `ideengine/spawn.go:20` to the call-site list with its `fabricengine` replacement named.

### [GAP] `ideengine/spawn.go` is not fabric-aware, so `primeName` has no source
**Section:** `prime-and-list-move`, rationale for the `vscode` parameter
**Issue:** The rationale asserts `PickColor`'s new `primeName` is "supplied by its sole caller `internal/ideengine/spawn.go:21`, which is already fabric-aware", but `spawn.go:6-11` imports only `hubgeometry` and `vscode`, and no file under `internal/ideengine`/`internal/idecli` imports `fabricengine` at all.
**Fix:** State where `Spawn` obtains the prime name in batch 1 — a new `fabricengine` import in `ideengine` (pulled forward from batch 4) or a new `Spawn`/`idecli` parameter — and correct the "already fabric-aware" claim.

### [NOTE] How `_board` reaches `seedGitExclude` is unspecified
**Section:** `board-junction`, git-hygiene points (a) and (c)
**Issue:** `seedGitExclude`/`unseedGitExclude` are reachable only from `WireJunctions`/`UnwireJunctions` and derive their names from `l.HostJunctions(slug, names)` (`junction.go:33,160,304,336`) — the same list that wires the mirror-pair junctions `_board` must not be in.
**Fix:** Name the call shape (e.g. a standalone `seedGitExclude(l, slug, []string{"_board"})`, as `reconcile.go:395` already does for stale removal) so the exclude seeding cannot be read as "add `_board` to `names`".

## Verdict

GAPS_FOUND
Five verifiable gaps: two wrong `_board` premises, an `_lyx` ownership cycle, and two batch-1 breakages.
MILL_REVIEW_END
