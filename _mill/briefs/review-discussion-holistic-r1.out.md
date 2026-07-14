MILL_REVIEW_BEGIN
# Review: Reduce git spawns in warpengine integration tests

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-14
```

## Findings

### [GAP] Byte-for-byte guarantee assumes all worktrees are hub siblings
**Section:** Decisions/sibling-layout-method; Technical context (byte-for-byte)
**Issue:** `SiblingLayout` sets `Hub: l.Hub`, but `Resolve(root)` sets `Hub = filepath.Dir(root)`; these diverge for any worktree root that is not a direct child of `l.Hub` (e.g. a raw `git worktree add /elsewhere`, which `Reconcile`'s raw-adoption path explicitly handles). Since `WeftWorktree()`/`WeftLyxDir()`/`WeftRepoRoot()` all derive from `Hub`, the junction-health verdict and weft-add target would change for such an entry, breaking the "byte-for-byte unchanged" claim.
**Fix:** State the hub-sibling precondition explicitly and decide the non-sibling path — either guard (`filepath.Dir(root) != l.Hub` ⇒ fall back to `Resolve`) or document non-sibling worktrees as out of scope; add an equivalence-test case pinning the intended behavior.

### [NOTE] New tests must be integration-tagged and hermetic
**Section:** Testing (equivalence test, spawn-count guard)
**Issue:** The equivalence test calls `Resolve` (spawns git) and the guard runs `Status`/`Reconcile` under a fixture hub; an untagged placement trips the Test Tier Purity guard. `hubgeometry` also has an untagged `hubgeometry_unit_test.go` a plan writer could wrongly target.
**Fix:** Note that both new tests go in `//go:build integration` files; both packages already carry a `HermeticGitEnv` TestMain, so no new infra is needed.

### [NOTE] SiblingLayout contract: input must be a worktree root, not a subpath
**Section:** Decisions/sibling-layout-method
**Issue:** `RelPath: "."` is only correct when `worktreeRoot` is an actual worktree root; a subpath argument would silently mis-derive `RelPath` (unlike `Resolve`, which computes it).
**Fix:** Document the "must be a resolved worktree root" precondition in the method godoc; note callers only pass `List` roots.

## Verdict

GAPS_FOUND
One conditional-equivalence gap undermines the byte-for-byte guarantee for non-sibling worktrees.
MILL_REVIEW_END
