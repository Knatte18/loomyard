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

### [BLOCKING] Card 18 prescribes a `Config.Dirs() == nil` assertion that cannot pass
**Location:** batch 4, card 18 (`internal/fabricengine/template_test.go`)
**Issue:** `Config.Dirs()` is `strings.Fields(c.Pathspec)`. For `Pathspec == ""`, Go's `strings.Fields("")` returns a non-nil, zero-length `[]string{}` (built via `make([]string, 0)`, whose data pointer is the non-nil `zerobase` sentinel), not `nil`. `Config.Dirs() == nil` therefore evaluates `false`, so a test written exactly as instructed ("proving an empty `pathspec` yields `Config.Dirs() == nil`") fails on every run, independent of any code change in this batch. Card 17's supporting claim ("`strings.Fields("")` returns nil either way") is the same error; it doesn't affect card 17's actual YAML-value choice (harmless there) but is the root of card 18's bad instruction.
**Fix:** Change the card-18 requirement to assert `len(Config.Dirs()) == 0` (or `reflect.DeepEqual(Config.Dirs(), []string{})`) instead of `== nil`, and correct card 17's `strings.Fields` claim to describe an empty, non-nil slice rather than nil. No production-code behavior is affected — `pathspecNames`/`junctionNames`'s downstream range loops work identically over nil or empty slices — only the test assertion and its rationale are wrong.

### [NIT] Card 21's rationale for the retained `no_raddle_names` sub-test misdescribes `HostJunctions`
**Location:** batch 4, card 21 (`internal/fabricengine/hostjunction_test.go`)
**Issue:** Card 21 justifies keeping the sub-test with "it is the only assertion that `HostJunctions` respects the hub-reserved block set at all." `HostJunctions`' own doc comment (and the plan's own batch-3 card 9 description) states it "takes names as a plain slice and does no sourcing of its own" — it performs no `HubReservedNames`/`filterHubReserved` filtering, so there is nothing for this sub-test to actually pin about `HostJunctions` respecting the reserved-name block; that property is instead covered by `TestFilterHubReserved`/`TestIsReservedHubName` in `junctionnames_test.go`. Re-pointing the sub-test at a still-reserved name and keeping this stated rationale would land an inaccurate justification comment in the codebase.
**Fix:** Reword card 21's rationale to something accurate (e.g. "a regression guard that the caller-supplied `names` slice a health/status call site builds never happens to include a still-reserved token") rather than claiming `HostJunctions` itself enforces reservation.

## Verdict

REQUEST_CHANGES — one BLOCKING test-assertion defect (Go `strings.Fields` nil-vs-empty) in batch 4 card 18/17; everything else independently verified accurate.
MILL_REVIEW_END
