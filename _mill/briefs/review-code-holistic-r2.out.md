MILL_REVIEW_BEGIN
# Review: Adopt quarry's glyph alphabet as the plan alphabet — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-09-06
```

## Findings

No findings. All three prior non-blocking items were checked against the current source and are resolved, not merely re-asserted:

- `internal/planparser/validate.go`'s doc comment (lines 64-78) now correctly states "plan-unapproved at position three" and the 26/27 check counts, matching the actual `validate()` dispatch order and the 27-entry enumeration in the package comment.
- `internal/planparser/doc.go:98` now reads "the plan format's 27 validation checks", matching `Validate`'s true count.
- `manifest/designs/quarry-glyph-plan-alphabet.md:3` now reads "Status: Done", consistent with `manifest/roadmap.md`'s Done-section entry (card 39 landed correctly).
- `contracts/specs/loom-plan-spec.md` carries no stale "format 4" description of the current grammar; the one remaining "format-4" mention (line 464) is an accurate historical pointer to `plan-card-format.md`'s own rewrite, not a live contradiction.

None of these are escalated; no new evidence contradicts the prior round's non-blocking judgment.

## Verification performed

Read all seven batch files, the overview, and cross-checked the shipped source for: the five-rule shape classifier (`classify.go`) and its glyph/handle/path/symbol ordering; canonicalization ordering and the file-extension gate (`normalize.go`); the `plan:` handle grammar and consistency checks (`handle.go`, `validate.go`); `RewriteRefs`/`AppendAmendment` as the plan's second/third write paths; the `internal/planglyph` package composition (`ValidateFormat`/`Validate` calling into `planparser`'s pure functions, never reimplementing a check); the resolve status policy and Create-group inversion (`resolve.go`, `create.go`); handle canonicalization and binding (`handle.go`'s `CanonicalizeHandles`/`BindHandles`); the two containment tiers (`planparser/containment.go`, `planglyph/containment.go`); the `ErrQuarryUnavailable` infrastructure-error disposition threaded consistently through `loomshed/planvalidate.go`, `loomcli/validate.go`, `webstercli/validate.go`, and `websterengine/runlevel.go`; the severity (blocking vs. informational) gate applied identically across all four plan-checking surfaces per the blocking-policy Shared Decision; `internal/quarrycli`'s four verbs delegating to `internal/planglyph` with no facade import and no re-shaping of quarry's own renderers; the `lyx quarry` cobra registration (`cmd/lyx/main.go`, `seamsignature_test.go` — confirmed 12 `RunCLI` / 11 `RunCLIIn` entries, matching `CONSTRAINTS.md`'s "eleven of twelve"); the `DeltaGit` call site and its `logger.Info` spawn observability (`planglyph/delta.go`); and the record-batch sequencing of done-checks, binding, scope-guard degradation, and drift detection (`websterengine/recordbatch.go`).

One documented, well-justified deviation from the plan's literal prose was found and is not a defect: `internal/planglyph/handle.go`'s `CanonicalizeHandles` and `internal/planglyph/drift.go`'s `DetectDrift` carry one extra parameter each (`results []quarry.ResolveResult` and `worktreeRoot string`, respectively) beyond the signatures card 19/36's prose states. Both additions are necessary for the described behavior (deriving a Rename declaration from the batched Resolve answer; revalidating after an exact-tier rewrite) and are called out explicitly in each function's own doc comment as a deviation from the batch prose's literal signature, with a stated reason. This is the implementer correctly resolving an internal inconsistency in the plan's own two descriptions of the same mechanism, not a scope or design defect.

All cross-batch contracts checked out: `planparser`'s exports (`HandlePrefix`, `HandleUnit`, `RewriteRefs`, `AppendAmendment`, `Plan.SurfaceRefs`, `Plan.Language`) are consumed by `planglyph` exactly as batch 4 onward requires; the Gate Self-Check Parity Invariant's pairs were moved together (producer + CLI verb + webster's own `Run` gate) in batch 5; no new `shedrecipe` row or CLI verb was added for the two webster boundaries, per the no-new-rows-no-new-verbs Shared Decision.

## Verdict

APPROVE
Implementation is faithful to the plan across all seven batches; no blocking or new findings.
MILL_REVIEW_END
