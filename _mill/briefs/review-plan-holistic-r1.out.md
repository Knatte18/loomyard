MILL_REVIEW_BEGIN
# Review: Shed recipe: loader/builder — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-21
```

## Findings

### [BLOCKING:design] Doc claims four disk-touching constructors, only three
**Location:** Batch 1 Card 1 (`shedbuild/doc.go`) and Batch 2 Card 6 (`shedbuild/build.go`) **Issue:** Requirements state "four registry constructors reach disk at construction time (two create their resolved run directory, two eagerly probe a stencil)," but the source shows only three distinct constructors touch disk: `bouncerEntry` (`internal/shedrecipe/entries_bouncer.go`) does BOTH — `os.MkdirAll` on the run dir and `shedadapters.NewBouncer`'s eager rubric-stencil probe — while `burlerRoundEntry` only creates a run dir and `singleLLMEntry` only probes a stencil. **Fix:** Reword both cards' Requirements (which become shipped `doc.go`/`build.go` godoc) to say three constructors produce four disk-touching effects, naming Bouncer as the one performing both.

### [BLOCKING:scope] Card 7 Context omits shedcheck.Check's defining file
**Location:** Batch 2 Card 7 (`internal/shedbuild/check.go`) **Issue:** Requirements mandate the literal call `shedcheck.Check(producers, r.Entry, r.Terminals)`, but Card 7's Context lists only `internal/shedcheck/doc.go` and `internal/shedcheck/finding.go` — neither states Check's parameter order or types; the real signature (`Check(producers []shedengine.ProducerDef, entry string, terminals []string) []Finding`) lives in `internal/shedcheck/check.go`, which is not in Context on any card and not in the plan's own manifest. **Fix:** Add `internal/shedcheck/check.go` to Card 7's Context.

### [NIT:consistency] Duplicate-key rejection misattributed to KnownFields(true)
**Location:** Batch 1 Card 2 and Card 3 **Issue:** Requirements say `KnownFields(true)` "already rejects a repeated key at either level"; in `gopkg.in/yaml.v3@v3.0.1` (`decode.go`), duplicate-mapping-key detection is gated by the decoder's `uniqueKeys` field, which defaults to `true` unconditionally and is unrelated to `KnownFields` — the tested behavior is real and correct, only the stated cause is wrong. **Fix:** Reword the rationale to cite yaml.v3's default duplicate-key detection rather than `KnownFields(true)`.

## Verdict

REQUEST_CHANGES
Two BLOCKING findings: a doc-comment miscount and a Context gap on Card 7; otherwise the plan is thorough and well-grounded.
MILL_REVIEW_END
