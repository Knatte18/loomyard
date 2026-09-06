# Review: Adopt quarry's glyph alphabet as the plan alphabet

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-09-06
```

## Findings

### [NIT:decision] Semver tag precondition has no owner or timing
**Demoted-from:** BLOCKING
**Section:** `quarry-version-pin`
**Issue:** The decision requires a quarry semver tag that does not exist (verified: no non-archive tags), cutting it is a quarry-side operator action this worktree's own Scope-Out forbids, yet unlike `quarry-unitpath-precondition` no owner, timing, or tracking is stated — and the sequencing question is undecided: if `glyph-unitpath` merges before the tag is cut (likely — it is mill-quick-sized and already spawned), one tag satisfies both the initial `go.mod` require and the disk-check cards' bump; otherwise two tag moments exist.
**Suggested fix:** Add a Status line mirroring `quarry-unitpath-precondition`: the operator cuts the tag on quarry main (preferably after `glyph-unitpath` merges, so one tag serves both preconditions), and the initial `go.mod` card names the existing tag as its precondition.

### [NIT:design] Resolve-backed pass's composition rule unstated
**Section:** `package-ownership`
**Issue:** The decision gives `planglyph` "the resolve-backed validation pass" but never states the operator's rider that this pass *calls* the pure `ValidateFormat`/`Validate` and never duplicates any of their checks — without it the two entry points can drift and gate parity becomes convention instead of construction.
**Suggested fix:** Add one sentence: the resolve-backed entry point composes (calls) the pure validation and adds only resolve findings on top; no check is implemented twice.

### [NIT:design] cgo retreat path dropped from the record
**Section:** `cgo-posture`
**Issue:** Option 2 (second cgo-only binary from the same module) is rejected flatly, but the operator's decision framed it as the *documented retreat* behind the `planglyph` seam — payable if Windows toolchain pain ever materializes, a mechanical swap inside one package no call site notices — and that reversibility record is what makes accepting cgo safe rather than dogmatic.
**Suggested fix:** Reword the rejection: "not chosen now; recorded as the named retreat — the planglyph seam makes swapping in-process calls for a spawned `cmd/lyxq` a one-package refactor if the toolchain cost ever materializes."

## Verdict

APPROVE
Decisions faithfully match all operator answers and hard rules; one external precondition (the semver tag) lacks owner and sequencing.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 0._
