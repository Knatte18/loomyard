MILL_REVIEW_BEGIN
# Review: Rename internal/config to internal/configengine

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-28
```

## Findings

### [NOTE] config.md body has an un-enumerated bare `config` token
**Section:** Scope (docs: `config.md`) / Decision full-stale-reference-sweep
**Issue:** `docs/shared-libs/config.md:22` uses a bare `` `config` `` as the package/actor ("`config` errors with `not initialized…`"); the per-file scope bullet only directs `internal/config` → `internal/configengine`, so a literal replace skips this token — the same bare-token class that warranted an explicit decision for `roadmap.md:31`.
**Fix:** Enumerate `config.md:22` and state the intended form (bare `` `configengine` `` or reworded prose), mirroring the roadmap-31 decision; the word-boundary verification grep already commits to catching it.

## Verdict

APPROVE
Scope, decisions, and r1 gaps fully resolved; one residual bare-token enumeration noted, non-blocking.
MILL_REVIEW_END