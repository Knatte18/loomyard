MILL_REVIEW_BEGIN
# Review: burlerengine + perchengine told-geometry

```yaml
duration_s: 220.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Anthropic) — Opus-class model, as identified by the harness "Opus 5"
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [NIT:design] lyxcwd removal has no machine guard, only a grep step
**Section:** Testing → Verify **Issue:** The `internal/lyxcwd`-absent property for `burlerengine`/`perchengine` production imports rests on a one-off manual grep; the discussion never decides whether a leaf/seam enforcement test should pin it, though wave 2 tightened exactly such a test for `internal/pattern` (`internal/pattern/leaf_enforcement_test.go`) while `reedengine` — which still names `lyxcwd` in `server.go`/`geometry.go`/`doc.go` — got none. **Fix:** State the disposition explicitly (add a guard now, or defer to T8/T10 with the reason), so a plan writer does not have to guess.

### [NIT:decision] shedadapters doc comment left as "may need a wording touch"
**Section:** Scope → Out, and Technical context (`internal/shedadapters/perch.go`) **Issue:** The named artifact's disposition is conditional ("may need a wording touch", "check the wording survives"); the comment at `perch.go:34` names only `perchengine.New` and `Options.PauseRequested`, no geometry, so it is unaffected. **Fix:** Record it as "no change" rather than leaving a conditional edit in the inventory.

## Verdict

APPROVE
Decisions are complete and every cited file, line, and count verified against the tree.
MILL_REVIEW_END
