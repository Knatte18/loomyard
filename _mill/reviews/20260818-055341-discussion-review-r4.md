MILL_REVIEW_BEGIN
# Review: lift the orchestrator preflight out of loomengine, plus the shared standalone-CLI foundations

```yaml
duration_s: 250.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [NIT:consistency] standalonestate: in-package tests vs export_test.go
**Demoted-from:** BLOCKING
**Section:** `### standalonestate-is-pure-derivation-with-an-injectable-seam` + `## Testing`
**Issue:** The decision says `derive` is "re-exported through `export_test.go`" and calls it "the only new `export_test.go` in this task", while Testing says "Untagged **in-package** tests over the injected `derive` seam" — an in-package test calls `derive` directly, so the shim has no consumer and is exactly the dead code the `preflight-tests-are-an-external-test-package` decision rejects ("every seam its tests need is already exported, so a shim would be dead code").
**Fix:** Pick one: external `package standalonestate_test` + `export_test.go`, or in-package tests and no shim; state it in both places.

### [NIT:consistency] "any other Resolve error" is narrower than stated
**Section:** `### preflight-exposes-four-entry-points` / `## Technical context` Gotchas
**Issue:** `lyxcwd.gitWorktreeRoot` (`internal/lyxcwd/lyxcwd.go:162`) wraps `ErrNotAGitRepo` around an *exec-level* failure too (`fmt.Errorf("%w: %v", ErrNotAGitRepo, err)`), so the surviving comment at `internal/loomengine/preflight.go:48-50` ("e.g. the git subprocess itself failed to spawn") is already false; the residual `(Report{}, err)` branch is reachable only via the anchor gate (`ErrCwdOutsideAnchor`/`ErrStaleAnchorMarker`).
**Fix:** Note this in Gotchas so the lifted comment and `internal/preflight/doc.go` do not ship the false characterisation into the new shared package.

### [NIT:scope] `stencilstore.ModeFor` ships with no named test
**Section:** `## Scope` (In) / `## Testing`
**Issue:** `ModeFor(dev bool) Mode` is a new exported symbol on a third owned file, and Gotchas pins its required semantics (`ModeFor(false) == ModeProduction`, the `iota` zero value at `internal/stencilstore/stencilstore.go:142`), but Testing names test surfaces only for preflight, loomengine, buildinfo, standalonestate and `cmd/lyx`.
**Fix:** Add a one-line untagged `stencilstore` test row pinning both mappings.

### [NIT:design] ldflags repoint has no drift detection
**Section:** `## Technical context` Gotchas / `## Testing`
**Issue:** The discussion correctly flags that Go's linker does not error on an unmatched `-X`, so a stale `tools/deploy/main.go:62` path yields a dev binary behaving as production — yet no test or verify step detects the mismatch, and `tools/deploy` is outside every existing guard walk.
**Fix:** Either name a cheap guard pinning `tools/deploy`'s `-X` string against `internal/buildinfo`'s import path, or state explicitly that the same-commit rule is the only protection.

## Verdict

APPROVE
One self-contradiction on standalonestate's test shape; everything else verified against source.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
MILL_REVIEW_END
