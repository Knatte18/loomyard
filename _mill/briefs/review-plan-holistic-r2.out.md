MILL_REVIEW_BEGIN
# Review: Producer-agnostic final-summary artifact + wire Finalize — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (Sonnet 5 per harness label)
reviewed_file: plan/
date: 2026-08-28
```

## Findings

No findings.

I cross-checked every batch/card against the current source: `websterengine/summary.go`, `summary_test.go`, `runlevel.go`, `geometry.go`, `integration_test.go`; `landingshed/deps.go`, `publish.go`, `finalize.go`, and all six of their test files; `loomcli/landingdeps.go`/`_test.go`; `shedrecipe/recipe.go`/`entries_simple_test.go`; `shedbuild/fixture_test.go`; `loomrecipe/fixture_test.go`; `webstercli/recordbatch.go`; `shedadapters/webster.go`/`doc.go`/`webster_test.go`; `discussionparser/doc.go`/`leaf_enforcement_test.go`; `webster-spec.md`, `roadmap.md`, `docs/overview.md`, `loom-status-spec.md`, `loom-plan-spec.md`; and `CONSTRAINTS.md`.

Every load-bearing quote and call-site claim in the plan (exact line text like recipe.go's "already carries fourteen fields," landingdeps_test.go's "a fifteenth field added later," webster-spec.md's "Finalize's PR-text source" and "dumps `summary.md` verbatim," docs/overview.md's kept-durable-contract-docs enumeration and its `webster-spec.md` Other-docs entry, roadmap.md's Planned entry and Maintenance convention text) matches the actual file content verbatim, including exact occurrence counts (e.g. the two `wantPath := websterengine.SummaryPath(dir)` sites in `webster_test.go`, the single `SummaryPath` call in `integration_test.go`, the field count of 15 in `landingshed.Deps`).

Structural checks: Batch Index DAG is acyclic, sequential, and every named `file:` exists; global step numbering (cards 1–12) has no gaps; `All Files Touched` is exactly the union of `Creates`/`Edits`/Move-targets across all 12 cards (verified item-by-item, 31/31); every card has `Moves: none` so no Rename mechanic section is required and none is missing; every `Shared Decision` (package-is-summaryparser, no-compat-wrappers, write-side-stays-in-websterengine, error-prefix-summaryparser, commitmessage-body-trim, told-final-summary-path, production-only-invariant-scope) is faithfully carried into the batch that implements it, with the production-only test-literal carve-out verified against the actual bare `"summary.md"` literals in `recordbatch_test.go`/`runlevel_test.go`/`smoke_test.go`. Sequencing is correct: card 5 (told-path swap) lands before card 6 (delete websterengine names) within batch 2, and batch 2 completes before batch 3 touches `finalize.go`, so no card commit leaves the tree uncompilable.

The one soft spot I considered was integration-test reachability: batch 2 edits `websterengine/integration_test.go` (build-tagged `integration`) but only typechecks it via `go vet -tags integration ./...`, never executes it under any batch's own `verify:`; batch 3's own tagged run covers `landingshed` only. This does not meet the BLOCKING criterion's letter (it names newly *added* integration tests, and this is a single-line mechanical rename inside an existing one), the change is behavior-preserving (`summaryparser.Path` is a byte-identical `filepath.Join` swap already unit-tested in batch 1), `go vet -tags integration` would catch any compile-level mistake, and the plan explicitly reasons about the gap and names its backstop (`pipeline.done_gate`'s repo-wide tagged run). Not raised as a finding.

## Verdict

APPROVE
Every claim verified against source; decisions, sequencing, and file-touch accounting are all internally consistent and accurate.
MILL_REVIEW_END
