MILL_REVIEW_BEGIN
# Review: landing: parent-fabric resolution chain — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (model id claude-sonnet-5, per harness-reported identity)
reviewed_file: plan/
date: 2026-08-23
```

## Findings

### [NIT:consistency] Card 10 misdescribes the existing test's Location-building pattern
**Location:** batch 2 (`02-loomengine-scratch-dir.md`), card 10.
**Issue:** The card says to build the `*lyxcwd.Location` "the same way this file's existing tests do (a bare `&lyxcwd.Location{...}` over a `t.TempDir()`...)", but the actual existing tests in `config_test.go` (`TestLoomStatusFile_EqualsAnchorPathJoinedWithLoomStatusRel`, `TestLoomDriverLogAndBootstrapLock`) build the Location over a fixed synthetic path (`filepath.Join("home","user","repo-HUB")`), never `t.TempDir()` — no test in this file constructs a `Location` over a real temp directory.
**Fix:** Reword the card to describe the actual pattern (fixed fake path literal, no `t.TempDir()` call, no filesystem I/O), since this file's `TestLoadConfig_*` tests are the only ones using `t.TempDir()`, and those don't build a `Location` at all.

### [NIT:consistency] Card 15's "every scalar argument" phrasing doesn't clearly cover the `cfg` struct parameter
**Location:** batch 4 (`04-loomcli-landing-wiring.md`), card 15.
**Issue:** The instruction to set "every scalar argument set to a distinct non-zero value" doesn't clearly include the `cfg landingshed.Config` struct parameter (not scalar). If the implementer passes `landingshed.Config{}` (zero value), `landingDeps`'s correct assignment `Config: cfg` still leaves `Deps.Config` at its zero value, so the reflect-based drift guard would flag the `Config` field even though the code is correct — a spurious failure caused by test-data choice, not a code defect, but the card doesn't flag `cfg` as needing non-zero fields the way it explicitly flags `pushSkipped`.
**Fix:** Add an explicit instruction that `cfg` must be constructed with at least one non-zero field (e.g. `Squash: true` or a non-empty `Conflict`).

## Verdict

REQUEST_CHANGES
Two NIT-level documentation-accuracy gaps in batches 2 and 4; no BLOCKING findings.
MILL_REVIEW_END
