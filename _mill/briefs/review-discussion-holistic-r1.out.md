MILL_REVIEW_BEGIN
# Review: .lyx hygiene -- relocate transients out of _lyx, fix .lyx junction geometry (slice 9)

```yaml
verdict: GAPS_FOUND
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-06
```

## Findings

### [GAP] `.lyx` anchor mismatch: WorktreePath vs AnchorPath
**Section:** Technical context → "Already correct (change only to adopt `lyxdirs.DotLyxDirName`)"
**Issue:** Every existing `.lyx` writer anchors at `l.WorktreePath()` (`logger/sink.go:37`, `shuttleengine/rundir.go` via `runDirRoot`, `scoutengine/daemonstate.go:43,50`, `burlerengine/engine.go:106`), but the proposed junction lands at `<worktree>/<AnchorRel>/.lyx` (`HostJunctions`, `junction.go:61-62`) and `_lyx` is `AnchorPath()`-anchored (`builderengine/state.go:33`, `perchengine/identity.go:33`) — so for any subpath-anchored repo (`AnchorRel != "."`) there are two distinct `.lyx` roots and those sites are *not* "already correct".
**Fix:** Decide which anchor `.lyx` uses, and state whether the five existing consumers re-anchor onto `AnchorPath()` as part of this slice.

### [GAP] Upgrade path for worktrees that already hold a real `.lyx`
**Section:** Decisions → `no-migration`
**Issue:** Every worktree in existence today has a real `.lyx` (logger/reed/shuttle/scout write it unconditionally), so after this change the first `WireJunctions`/`reconcile` hard-errors everywhere, and the stated remedy (delete it) destroys exactly the live reed/scout daemon state the rationale cites as too risky to touch automatically.
**Fix:** State the intended upgrade sequence for existing worktrees and whether the hard error is expected to block unrelated fabric verbs (reconcile/health/add) until the operator deletes.

### [GAP] Nothing adds `.lyx` to an already-deployed `fabric.yaml`
**Section:** Decisions → `dotlyx-is-a-weft-backed-junction`
**Issue:** `Config.Pathspec` is a single free-form string (`config.go:23,28`) with template default `pathspec: _lyx _pattern` (`internal/fabricengine/template.yaml:2`); an existing repo-wide `fabric.yaml` in `weft:main` keeps its old value, so `.lyx` is never wired while transients already target it — and migrating that value is "out" per Scope.
**Fix:** Say how existing repos acquire the `.lyx` pathspec entry (template default change plus documented manual edit, or an explicit exception to the no-migration rule).

### [GAP] Weft-side exclude seeding: two contradictory owners and no ordering guarantee
**Section:** Decisions → `weft-side-exclusion-via-git-info-exclude` vs Testing → `internal/fabricengine`
**Issue:** The decision seeds `.lyx/` from `seedWeftArtifactExcludes` via `ensureWeftLockDir` (a weft-*git*-verb choke point, `weftgit.go:39-54`), but the testing section asserts *wiring* seeds it; `WireJunctions` only seeds the warp side (`junction.go:91-103`), so scratch written into the weft worktree before the first weft-git verb shows as untracked dirt and trips Remove's dirty gate.
**Fix:** Name one owner and state that seeding is guaranteed to precede the first `.lyx` write after wiring.

### [GAP] Both enforcement tests are unimplementable as specified
**Section:** Testing → "Enforcement tests"
**Issue:** "no Go file outside `internal/lyxdirs` contains the literal `".lyx"`/`"_lyx"`" matches 71 files today, including sanctioned non-declarations (`lyxcwd/anchor.go:41` `".lyx-anchor"`, the `".lyx/"` exclude/gitignore entries, `template.yaml` prose, most `_test.go` fixtures); test 2 ("no path built on `lyxdirs.LyxDirName` ends in `.lock`/`pause`/`prompts`") needs dataflow analysis, not grep.
**Fix:** Define each test's concrete mechanism and exemption set (declaration-vs-literal discrimination, test-file scope) before plan writing.

### [NOTE] `neverCommittedNames` scope vs `Topology.Add`'s raw `Dirs()`
**Section:** Decisions → `never-committed-is-structural-not-configurable`
**Issue:** "kept out of every constructed pathspec" reads as filtering `Config.Dirs()`, which would silently undo the slug-reservation use the Technical context says must keep seeing `.lyx`.
**Fix:** State explicitly that filtering lives in `ScopedPathspec`/`classifyPaths`, never in `Dirs()`/`WiredNames`'s input.

### [NOTE] Sandbox suites and reference docs assert the old paths
**Section:** Scope → "Docs in the same commit"
**Issue:** `tools/sandbox/SANDBOX-BUILDER-SUITE.md:276`, `SANDBOX-WEBSTER-SUITE.md:128` and `docs/reference/builder-contract.md:166` name `_lyx/webster/prompts/*`, `*/builder/pause` and the exclude-layer mechanism this task deletes; none are in the docs list.
**Fix:** Add the sandbox suite files and `docs/reference/builder-contract.md` to the same-commit doc set.

## Verdict

GAPS_FOUND
Anchoring, upgrade path, config rollout, exclude ownership and both enforcement tests need resolution.
MILL_REVIEW_END
