MILL_REVIEW_BEGIN
# Review: Collapse _pattern into _lyx, and un-reserve _raddle as a hub-level name — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (Anthropic)
reviewed_file: plan/
date: 2026-08-08
```

## Findings

### [BLOCKING] Card 18 misdescribes template_test.go's pathspec fixture and gives a wrong target value
**Location:** batch 4 / card 18 (`internal/fabricengine/template_test.go`)
**Issue:** Card 18 states that `internal/configcli/configcli_integration_test.go` line 67 AND `internal/fabricengine/template_test.go` both hold a `{"_lyx", "_pattern"}` expectation, and that the latter becomes "the routing set `{_lyx}`". Verified against source: `TestConfigTemplate_PathspecResolvesToPattern` actually holds a single-element `want := []string{"_pattern"}` (not a two-element `{"_lyx","_pattern"}`), and it asserts the *raw resolved* `ConfigTemplate()` pathspec value, not `pathspecNames()`'s routing set. `_lyx` is never read from the `pathspec:` key at all — template.yaml's own trailing comment says so explicitly ("_lyx and .lyx are structural and injected in code ..., never read from here") — so directing the implementer to assert this raw value equals `{_lyx}` produces an assertion that contradicts the template's own documented semantics; the correct post-change value is empty (`[]string{}`), consistent with the card's own later instruction to assert `len(cfg.Dirs()) == 0`.
**Fix:** Correct Card 18 to say `TestConfigTemplate_PathspecResolvesToPattern`'s `want` becomes an empty slice (and rename the test off "...ResolvesToPattern"), and drop the inaccurate "routing set `{_lyx}`" characterization for this file; keep the `{_lyx, .lyx}` guidance only for the `configcli_integration_test.go` `WireJunctions` call it actually applies to.

### [BLOCKING] No card actually fixes the dangling link to the deleted design doc
**Location:** batch 7 / cards 34 and 37 (`manifest/roadmap.md`)
**Issue:** `manifest/roadmap.md` line 41 reads `See [designs/pattern.md](designs/pattern.md).`, a real relative link to the file card 37 deletes. Card 34 is the only card that edits `manifest/roadmap.md` (fixing lines 37 and 39's PATTERN paths), but its Requirements never mention line 41. Card 37 instructs "grep the repo for links to `manifest/designs/pattern.md` and fix any that exist," but Card 37's own `Edits:` is `none` — fixing a link necessarily requires editing whatever file contains it, which Card 37 cannot do per its own field. The result: after batch 7, `manifest/roadmap.md` still links to a file that no longer exists.
**Fix:** Add "line 41's link to `designs/pattern.md`, retargeted to `internal/pattern`'s package godoc" to Card 34's Requirements (the natural owner, since it already edits this file), or add `manifest/roadmap.md` to Card 37's `Edits:` list.

## Verdict

REQUEST_CHANGES
Two grounded plan-text defects — a misdescribed test fixture with a wrong target value, and a card that must fix a link but declares no edits — need correcting before implementation.
MILL_REVIEW_END
