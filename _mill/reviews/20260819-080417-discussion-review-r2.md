MILL_REVIEW_BEGIN
# Review: fabric: merge-conflict primitive

```yaml
duration_s: 199.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-19
```

## Findings

### [NIT:consistency] Guard reason interpolates a branch name
**Demoted-from:** BLOCKING
**Section:** `weft-source-is-derived-and-must-exist` vs `safety-guards-are-aggregated-and-side-free`
**Issue:** The guards decision pins a closed reason set containing `"source branch is not fabric-managed"` and states "a reason string never interpolates a branch name or any other value", but the weft-source decision specifies the fixed reason `cannot merge %q: not a fabric-managed branch` — a different string that interpolates the branch.
**Fix:** Pick one spelling; if the branch must be reported it travels in a typed error field, as the guards decision already rules.

### [BLOCKING:design] Pinned signatures cannot reach the mapping geometry
**Section:** `conflicts-are-reported-as-unified-worktree-relative-paths` / `public-surface-shapes`
**Issue:** The mapping rule needs `AnchorRel` and the wired name set, but `Fabric` holds only `warp`/`weft`/`warpPath`/`weftPath` (`fabric.go:54-60`; `Open` discards the `*lyxcwd.Location`), and `WiredNames(baseDir)` (`junctionnames.go:271`) needs the hub board config base — so `MergeIn(source string)` / `Merge(source, opts)` as pinned have no route to either, nor a stated behaviour when that config read fails.
**Fix:** State where the merge path obtains `AnchorRel` and the wired-name set (handle field, a `ResolveWorktree` on `f.warpPath`, hub via `filepath.Dir(f.warpPath)` per `Fabric.ResetHard`'s precedent) and what a config-read failure returns.

### [NIT:scope] Only `Fabric.Commit` is guarded mid-merge
**Demoted-from:** BLOCKING
**Section:** `combined-lock-around-mutating-steps-only` (consequence) / Scope In
**Issue:** The resolution window is unbounded and lock-free, but only `Fabric.Commit` gains a merge-in-progress refusal; `Fabric.Pull` (`pull.go:205`, which hard-resets/re-anchors), `checkout.go`'s coordinated branch switch, `PushWeft`/sync, and `remove`/`cleanup` have no stated disposition while a record exists.
**Fix:** State, per existing mutating verb, whether it refuses on a live merge record or is deliberately left unguarded and why.

### [NIT:design] Abort's "provably merge-produced" rationale overclaims
**Section:** `weft-side-gated-reset-in-destroy-dot-go`
**Issue:** `force: true` is justified because the pre-merge guards required a clean pair, so the discarded dirt is "provably merge-produced" — but `verify-before-conclude-not-post-commit-rollback` puts build/test fixing inside the same window, so tracked edits unrelated to conflict resolution can exist at abort time.
**Fix:** Reword the rationale to "dirt accumulated since the merge started, which abort discards by definition" rather than claiming it is provably merge-produced.

### [NIT:design] `MergeStart` classification evidence unstated
**Section:** `public-surface-shapes` (gitrepo block)
**Issue:** A conflicted `git merge` exits non-zero, so `runChecked` returns `*gitexec.GitError`, and exit 1 covers both conflicts and some genuine failures; the discussion pins `runChecked` and unchanged raw-site counts without saying what evidence classifies the four outcomes.
**Fix:** Name the classification evidence (index/`MERGE_HEAD`/HEAD-movement probe, in `ancestry.go:29-38`'s `errors.As`+ExitCode style) so no plan writer reaches for the raw form and moves a pinned count.

## Verdict

REQUEST_CHANGES
Three blocking gaps: a contradicted guard reason, unreachable mapping geometry, unguarded sibling verbs.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
