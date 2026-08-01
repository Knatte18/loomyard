MILL_REVIEW_BEGIN
# Review: fabric: audit and migrate all remaining direct git mutations onto Fabric

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.8 (claude-opus-4-8)
reviewed_file: _mill/discussion.md
date: 2026-08-01
```

## Findings

### [GAP] Guard tokens collide with sanctioned Fabric calls
**Section:** Decisions / regression-guard + fabric-api-naming
**Issue:** `fabric-api-naming` names the Fabric methods after the verb only (`CheckoutDetached`/`RestoreBranch`/`ResetHard`), so the migrated consumer still writes `repo.CheckoutDetached(sha)` etc. against the interface — textually identical to the raw `gitrepo` call; the substring guard banning `.CheckoutDetached(`/`.RestoreBranch(`/`.ResetHard(` would flag the very post-migration code it is meant to permit.
**Fix:** Resolve the tension — the guard cannot distinguish receiver by substring; either detect the banned *construction* (`gitrepo.New`) minus grandfathered read sites, or scope by AST/type, and state which.

### [GAP] Guard misses raw gitexec mutating verbs
**Section:** Decisions / regression-guard
**Issue:** builderengine's actual bypass is `gitexec.RunGit([]string{"reset","--hard",sha}, worktree)` (gitquery.go:76), a raw call with no `.ResetHard(` method token; a regression reintroducing that exact form would pass the method-dot-token guard, so the guard does not cover the form of the bug this task found.
**Fix:** Ban the raw gitexec mutating-verb arg patterns (`"reset","--hard"`, `"checkout","--detach"`, `"push"`, …) too, or otherwise cover raw `gitexec.RunGit` mutation without breaking the read-only `HeadSHA`/`ChangedFiles`/`dirty` uses.

### [NOTE] Warp-only ops gain a weft-exists precondition
**Section:** Decisions / fabric-handle-construction
**Issue:** `fabricengine.New` stat-checks both warp and weft exist (fabric.go:57); routing warp-only bisect/chain-restart through it makes a warp-only operation fail when the weft worktree is absent/broken — a new production failure mode noted only for tests, not production.
**Fix:** Confirm/state that a real hub always has a paired weft, or handle a missing-weft `New` error on this warp-only path.

### [NOTE] builderengine gains SHA validation silently
**Section:** Technical context (validation claim)
**Issue:** builderengine's `ResetHard` (gitquery.go:76) does NOT call `validSHA`; migrating onto `Fabric.ResetHard`→`gitrepo.ResetHard` adds `ErrInvalidSHA`, so "no new validation needed" understates a (benign) behavior change.
**Fix:** Note the added validation explicitly; chain-start SHA is always valid so impact is nil, but record it.

### [NOTE] New methods invert fabric.go's stated convention
**Section:** Decisions / fabric-api-naming
**Issue:** fabric.go's package doc says only genuinely cross-repo ops get a Fabric method and repo-specific/uncoordinated ops go through `f.Warp.X()`; the four new warp-only methods are exactly the category the doc says NOT to add as methods.
**Fix:** Reconcile and update fabric.go's package/`Fabric` doc in the same commit to reflect the illusion-preserving warp-only method carve-out.

## Verdict

GAPS_FOUND
The regression-guard design is self-contradictory and misses the raw-gitexec bypass it targets.
MILL_REVIEW_END
