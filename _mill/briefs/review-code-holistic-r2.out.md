MILL_REVIEW_BEGIN
# Review: Shed recipe: loader/builder — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-21
```

## Findings

None. Verified end-to-end across all four batches.

- `internal/shedbuild`'s five production files (`doc.go`, `recipe.go`, `parse.go`, `build.go`, `check.go`) match the package-layout Shared Decision exactly, and the import allowlist in `seam_enforcement_test.go` is exactly the four packages the plan pins, with `TestToldGeometryInvariant_AllowlistOnly` named per `CONSTRAINTS.md`.
- `Recipe`/`Row` field set, yaml tags, and doc comments match card 1; `Row.Config` stays `map[string]any`, `recipe.go` imports nothing.
- `Parse`/`Load` match card 2 verbatim: `KnownFields(true)`, `io.EOF` → `"shedbuild: recipe is empty"`, fixed-order shape checks, `Load`'s absolute-path-before-read rejection mirroring `internal/shedrecipe/paths.go`'s posture.
- `parse_test.go`/`load_test.go` cover every case card 3/4 enumerate, including the map-iteration-order determinism test and the reject-before-read relative-path case.
- `Build` (card 6) resolves via `shedrecipe.Lookup`, calls `ctor(row.Name, shedrecipe.Config(row.Config), env)`, copies all five `ProducerDef` fields, runs no `shedcheck` analysis — matches `shedengine.ProducerDef`'s real field set.
- `Check` (card 7) is the exact one-line forward the plan specifies, with the "why `Build` doesn't call this" rationale present.
- `fixture_test.go` (card 8) fills all nine told `Env` roots plus `Shuttle`/`Burler`/`WebsterRun`/`WebsterDeps`' four seams plus `Landing`, and `testLandingDeps` matches `internal/loomshed/fixture_test.go`'s and `internal/shedrecipe/coverage_guard_test.go`'s shape field-for-field.
- `build_test.go`/`build_engines_test.go`/`check_test.go` (cards 9-11) cover exactly what's required; `build_engines_test.go` drives off `shedrecipe.Names()` and its three-engine minimal-config map matches `entries_singlellm.go`/`entries_bouncer.go`/`entries_burler.go`'s real required keys.
- `testdata/loom-recipe.yaml` (card 12) reproduces `loomshed.New`'s thirteen rows, names, engines, and routing exactly, including the load-bearing empty `on_done` on `Finalize`.
- `equivalence_test.go` (card 13) builds a single paired fixture, uses a real `preflightshed.NewPreflight` on the loom side (same concrete type `preflightEntry` produces via the registry, satisfying the `reflect.TypeOf` row-1 comparison), gives `LockPath`/`StatusLockPath` distinct values, and asserts all five data fields, concrete type, and a zero-finding `Check` call.
- Docs batch (cards 14-18): `shed-recipe.md`'s banner/title/Segment-reversal paragraph/new "shipped" section and pieces-list entry all match; `shed.md`'s two corrected sentences point at `shed-recipe.md`'s corrected paragraph and state the field/rule are staying; `docs/overview.md`'s module-tree row and narrative sentence are both updated and the "see the package documentation" list now names `shedbuild`; `CONSTRAINTS.md` appends `internal/shedbuild` to the Told-Geometry machine-enforced list, updates "ten" → "eleven", and adds the one Shed-Recipe-Registry-Invariant sentence with no new invariant section; `roadmap.md` moves the item into Done adjacent to the engine-registry and validity-checker entries, in past-tense voice, and the remaining Planned item carries forward both the sole-parser-invariant note and the import-cycle constraint on the future consumer.
- No out-of-plan files: `internal/shedbuild/`'s fourteen files on disk are exactly the union in `00-overview.md`'s "All Files Touched" section.
- No global utility duplication: `fixture_test.go` is a deliberate fresh copy (not an import) of a same-package-incompatible sibling, as the plan itself requires and documents.

## Verdict

APPROVE
All four batches align with the plan, each other, and CONSTRAINTS.md; no findings.
MILL_REVIEW_END
