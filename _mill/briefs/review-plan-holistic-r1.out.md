MILL_REVIEW_BEGIN
# Review: reed: AddStrand and attach self-heal a cold worktree — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Claude, Anthropic)
reviewed_file: plan/
date: 2026-09-19
```

## Findings

### [BLOCKING:design] Card 8's literal `seedReedConfig` recipe collides with `_lyx` already existing
**Location:** batch 3 (comment-sweep), card 8
**Issue:** Card 8 has the first of its two `wire()` calls run `wireStandalone` → `cliwire.ResolveStandalone` (`internal/cliwire/standalone.go:108-109,120-123`), which seeds `<stateDir>/_lyx/stencils` and `<stateDir>/_lyx/specs` via `stencilstore.Reconcile`; `Reconcile`'s `seedGitattributes` (`internal/stencilstore/reconcile.go:149-159`) unconditionally `os.MkdirAll`s that baseDir, so `<stateDir>/_lyx` already exists on disk by the time the card's "between the two calls" step runs. `seedReedConfig`'s literal recipe (`internal/reedengine/contract_integration_test.go:34-38`) calls `os.Mkdir(lyxDir, 0o755)` — not `MkdirAll` — and `t.Fatalf`s on any error, so transplanting that recipe verbatim into this new context fails immediately with "mkdir _lyx: file exists" before the config is ever written.
**Fix:** Have card 8 direct the new helper to tolerate an already-existing `_lyx` (e.g. `os.MkdirAll`, or stat-then-skip) instead of an unqualified transplant of `seedReedConfig`'s `os.Mkdir` step.

### [NIT:consistency] Card 10 asks to carry a Covers: tag that M1 does not have
**Location:** batch 4 (docs-and-suites), card 10
**Issue:** `tools/sandbox/SANDBOX-REED-SUITE.md`'s M1 scenario carries no `**Covers:**` tag today (only M2/M26/M27 do — confirmed against the file and against `cmd/lyx/sandbox_coverage_test.go`'s module-level, not scenario-level, coverage check), so "carry the Covers: tag across the split" and the Batch Tests section's "a scenario's coverage tag must survive the split" both describe a tag that is not there to carry.
**Fix:** Drop the Covers-tag-carrying instruction for the M1 split, or note that "reed" module coverage already survives the split independently via M2/M26/M27.

## Verdict

REQUEST_CHANGES
Card 8's config-seeding recipe will fail on an already-existing `_lyx` dir; one M1 Covers-tag claim is inaccurate but harmless.
MILL_REVIEW_END
