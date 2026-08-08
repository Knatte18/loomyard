MILL_REVIEW_BEGIN
# Review: Collapse _pattern into _lyx, and un-reserve _raddle as a hub-level name — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5
reviewed_file: plan/
date: 2026-08-08
```

## Findings

### [BLOCKING] Card 9 contradicts itself on line 199's pattern.DirName
**Location:** Batch 3 / Card 9 (`internal/fabricengine/hostjunction_test.go`)
**Issue:** The card's retarget-to-`"_extra"` line list explicitly includes line 199 (`pattern.DirName` inside the `no_raddle_names` sub-test), but the same card also says "Leave the `no_raddle_names` sub-test at lines 191-202 untouched in this batch — it is batch 4's subject," and line 199 falls inside that range. Verified against the actual file: line 199 is `junctions := HostJunctions(loc, "slug", []string{"_lyx", pattern.DirName})`, squarely inside the sub-test body (192-205).
**Fix:** Resolve the conflict explicitly — either drop line 199 from the retarget list (the whole sub-test, including its junction-name literal, waits for batch 4), or state that only the `_raddle` assertion is deferred and line 199's junction-name input retargets now.

### [BLOCKING] Card 29's `_raddle` comment-block instruction is self-contradictory
**Location:** Batch 6 / Card 29 (`internal/lyxcwd/enforcement_test.go`)
**Issue:** Requirements say to delete "the comment block at lines 277-280 ... outright" as part of removing `_raddle`'s row, then immediately say to "Rewrite the surviving `_portals`/`_launchers` comment so it no longer mentions `_raddle`." Verified against the source: lines 277-280 are one shared comment block covering `_portals`, `_launchers`, AND `_raddle` together (feeding rows 281-283) — it cannot be both deleted outright and rewritten-and-kept.
**Fix:** State plainly that lines 277-280 are rewritten (drop only the `_raddle` clause, keep the `_portals`/`_launchers` rationale intact), not deleted; only the `"_raddle": {...}` map row (283) and the separate `_pattern`-only comment block (293-298, confirmed `_pattern`-exclusive) are deleted outright.

### [BLOCKING] `pattern.DirName`/`PathspecFile` referenced without `internal/pattern/pattern.go` in Context
**Location:** Batch 3 cards 9, 10, 13, 14, 15, 16; Batch 4 card 22; Batch 5 card 26
**Issue:** Each card's Requirements names `pattern.DirName` or `pattern.PathspecFile` — constants declared in `internal/pattern/pattern.go` — but that file is absent from every one of these cards' `Context:` lists (confirmed by re-reading each card's Context block). Per the Context-completeness rule, the implementer may only read files in Context, so the constant's declaring file is cold-start-unreachable there.
**Fix:** Add `internal/pattern/pattern.go` to `Context:` for cards 9, 10, 13, 14, 15, 16, 22, and 26.

### [NIT] Card 8 overcounts `pattern.DirName` occurrences in `pull_integration_test.go`
**Location:** Batch 2 / Card 8
**Issue:** After handling `wantPath`, Card 8 says "Retarget the other two `pattern.DirName` uses in this file," but a grep of the actual file shows only one other occurrence (line 254: `patternDir := filepath.Join(weftFixture.WeftPath, pattern.DirName)`), not two.
**Fix:** Correct "the other two" to "the other one (line 254)."

### [NIT] Card 40 has two off-by-one line citations
**Location:** Batch 7 / Card 40 (`internal/fabriccli/fabric.go`, `weft_verbs.go`)
**Issue:** `fabric.go`'s pollution-scan sentence actually begins at line 191 ("The pollution scan likewise covers..."), not the cited line 189 (189 is still part of the preceding `junction_healthy` sentence); `weft_verbs.go`'s "pathspec names only `_pattern`" mention is at line 94, not the cited line 93.
**Fix:** Shift both citations by one line so the implementer lands on the exact sentence.

### [NIT] Card 13's `_extra`-seeding fallback may not be achievable for `TestRunCLI_CloneEndToEnd`
**Location:** Batch 3 / Card 13
**Issue:** The card's fallback — "seed that config with `pathspec: _extra`" before the loop asserts — presumes an overridable pre-existing `fabric.yaml`, but `TestRunCLI_CloneEndToEnd` clones from a genuinely empty bare weft (zero commits, confirmed via `makeCLICloneWeftBare`), so there is no existing weft:main config to seed before `clone` runs; with batch 4's now-empty template default, a bare clone wires no optional junction at all.
**Fix:** Say explicitly how to seed it — e.g. wire `_extra` after `clone` completes (via `WireJunctions`/`reconcile` against a config written post-clone) rather than before, so the mechanism isn't left to implementer discovery.

## Verdict

REQUEST_CHANGES
Three genuine contradictions/gaps (cards 9, 13-context-family, 29) need resolving before implementation; line-level grounding is otherwise excellent.
MILL_REVIEW_END
