MILL_REVIEW_BEGIN
# Review: Harden the Path Invariant: close enforcement hole + fix geometry leaks

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-30
```

## Findings

### [NOTE] LYX_BOARD_PATH lingers as example in two shared-lib docs
**Section:** Scope (Out) / Technical context (docs)
**Issue:** After the board `path:` key and its env override are removed, `LYX_BOARD_PATH` no longer exists as a real key anywhere, yet `docs/shared-libs/yamlengine.md:34` and `docs/shared-libs/configengine.md:60` still use `${env:LYX_BOARD_PATH:-../_board}/sub` as the canonical `${env:}` illustration; the discussion's doc scope only lists `paths.md` + `CONSTRAINTS.md`.
**Fix:** Note in the plan that these two examples should swap to a generic env name (mechanism is unaffected, but the sample var is now dead) — or explicitly declare them out of scope.

## Verdict

APPROVE
All decisions are grounded; tree-scan feasibility (no unconverted geometry-literal sites outside the enumerated warp/lyxtest set) and extra-key tolerance both verify clean.
MILL_REVIEW_END
