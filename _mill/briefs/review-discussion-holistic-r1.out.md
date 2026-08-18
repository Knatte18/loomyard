MILL_REVIEW_BEGIN
# Review: scoutengine told-geometry (optional uniformity pass)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: /home/knatte/Code/loomyard/wts/scout-told-geometry/_mill/discussion.md
date: 2026-08-18
```

## Findings

### [BLOCKING:design] Hub-mode branch has no named automated evidence
**Section:** § Testing + § Constraints ("Zero behavioural change")
**Issue:** The sole acceptance property is byte-identical paths "in hub mode and out-of-hub mode", but every named test covers only the out-of-hub branch (`scoutdaemon_test.go` is pure path math on told strings; the reshaped `lookupContext` test is explicitly out-of-hub; `constructoranchoring_test.go` exercises the constructors, never `lookupContext`'s `anchorRoot = layout.AnchorPath()` derivation) — the hub branch is left to the manual `lyx scout refs` smoke, which the same section says must never be the only evidence.
**Fix:** Either name a test that pins the hub branch of `lookupContext` (a hub-fixture/integration test), or state explicitly why the constructor rows plus the untested one-line derivation are sufficient evidence for that half of the property.

### [NIT:consistency] Precedent file cited for the bare-string shape is wrong
**Section:** § Technical context → "Precedent to copy"
**Issue:** `internal/websterengine/geometry.go` holds the eight-field `Geometry` struct, not the bare-`anchorRoot` free functions; `Dir(anchorRoot string)` and `ReportsDir(anchorRoot string)` are declared in `internal/websterengine/state.go:41/49`. As written the pointer sends an implementer to the shape this task's Decisions section rejects.
**Fix:** Cite `internal/websterengine/state.go` for the free-function shape and keep `geometry.go`/`burlerengine/geometry.go` cited only for the doc-comment wording.

### [NIT:consistency] "Keep the existing chdir setup" misdescribes the test being reshaped
**Section:** § Testing → `internal/scoutcli` TDD candidate
**Issue:** `TestLookupContext_OutsideHubReturnsSynthesizedLocationAndBuiltinRegistry` (`internal/scoutcli/cli_test.go:584-607`) never chdirs — it passes two `t.TempDir()` values as the `cwd`/`dir` arguments; the chdir-into-a-non-git-temp-dir setup lives in the `RunCLI_*_NoLanguageError` tests (lines 82, 165, 205, 798).
**Fix:** Reword to "keep the existing explicit non-git temp-dir arguments", so the rewrite does not introduce a process-wide chdir the current test does not have.

### [NIT:design] Misuse failure mode changes silently, unstated
**Section:** § Decisions → "`Options.Layout` becomes `Options.AnchorRoot string`"
**Issue:** Today a caller omitting `Layout` panics at `l.AnchorPath()` (`refs.go:52` documents "required and must be non-nil", and nothing validates it); after the change an empty `AnchorRoot` yields the cwd-relative path `.lyx/scout/<lang>/daemon.json` and writes daemon state wherever the process stands. The decision says "the engine validates nothing" but never states this loud-to-silent shift.
**Fix:** Record the consequence in the decision (accepted, matching converted siblings) so a later reader does not read "zero behavioural change" as covering the misuse path too.

### [NIT:scope] `producers-standalone.md` left describing a shape that no longer exists
**Section:** § Scope (Out) + § Decisions → documentation
**Issue:** `manifest/designs/producers-standalone.md:198` describes scout's synthesized `Location` as a live "deliberate, documented fiction" and `:641`'s T9 Files list is already stale (`constructoranchoring_test.go` rows are 102-103/160-161/181-182, and `cmd/lyx/notransients_test.go` is absent) — the doc stays in-tree until T10 deletes it, while the task's own Note forbids exactly this for Go comments. T10's Files list also claims "the `doc.go` of each converted package", overlapping this task's doc.go decision.
**Fix:** State the disposition explicitly (accepted staleness bounded by T10's deletion, and doc.go touched twice by design) rather than leaving it implied.

## Verdict

REQUEST_CHANGES
Scope and decisions verify against source; hub-mode evidence for the acceptance property is missing.
MILL_REVIEW_END
