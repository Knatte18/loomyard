MILL_REVIEW_BEGIN
# Review: fabric: clone doesn't commit written module configs — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: claude-sonnet-5 (Sonnet 5, "sonnet-5")
reviewed_file: plan/
date: 2026-08-26
```

## Findings

None. Cross-checked every card's Requirements against the current source (`internal/configengine/config.go`, `internal/configengine/config_test.go`, `docs/shared-libs/configengine.md`, `internal/hubforge/seed.go`, `hub.go`, `doc.go`, `internal/fabriccli/clone.go`, `pushbypass_integration_test.go`, `testmain_test.go`, `internal/fabricengine/clone.go`, `commitweftpaths.go`, `mutation.go`, `origin.go`, `internal/configsync/configsync.go`, `internal/configreg/configreg.go`, `internal/lyxdirs/dirs.go`, `internal/gitkit/gitkit.go`, `internal/preflight/preflight_integration_test.go`, `internal/preflightshed/preflight_integration_test.go`) and against `_mill/discussion.md`'s Decisions/Testing sections.

Verified specifically: `ConfigFileRel` placement (immediately after `ConfigFile`, config.go lines 53-56) and doc-section placement (between `ConfigFile` and `FindBaseDir` in configengine.md) match exactly; `SeedConfig`'s exact target commit line and the two preflight fixtures' exact `add`/`commit` pairs match card 2's quoted text verbatim; `fabricengine.CommitAnchoredPaths`/`CommitWeftPaths` exist with the signature and no-op/lock/record semantics card 3 describes; `l` is genuinely already in scope at the point card 3 inserts its new block; `CloneAndWire`'s doc comment and `clone.go`'s six-line header comment text match what card 3 says to extend; `hub.go`'s `WeftBase: res.WeftBase` precedent for the new `Mutations` field, and `doc.go`'s seeding-contract paragraph, both match card 3's quoted targets; `configreg.Modules()` returns exactly ten entries (nine non-`fabric`), matching every "nine modules" claim; `ScopedPathspec` anchor-joins as card 4's anchor-scoping test (test 3) expects; tracing `CloneAndWire`'s full mutation sequence confirms the new `KindCommitCreated` entry is genuinely the last entry appended, supporting card 4 test 5's "commit entry last" claim; `fabriccli_test`'s existing `TestMain`/external-package precedent (in `pushbypass_integration_test.go`) supports card 4's fixture-package choice without a Fabric-Fixture Invariant violation.

Batch Index DAG is acyclic, all `file:`/name references resolve, dependency ordering (`3` depends on `1,2`) matches the stated rationale and the measured fixture-failure ordering. Global step numbering (1-5) is sequential with no gaps. `## All Files Touched` is an exact union of the batches' `Edits:`/`Creates:` sets. Every card has non-empty `Creates`/`Edits`/`Context`/`Moves`/`Requirements`/`Commit`; all `Moves:` are `none`, correctly requiring no `## Rename mechanic` section. Every `Requirements:`-named identifier resolves to a file in that card's own `Context:` or `Edits:` list. Both integration test files added (card 4, card 5) are covered by batch 3's `-tags integration` verify scope. The overview's `docs-land-with-their-own-behaviour-change` Decision splits the discussion's "all in the same commit" doc list across cards 1/2/3 by which behaviour change actually drives each doc paragraph — a defensible, well-rationalized reading of CLAUDE.md's Documentation Lifecycle rule (doc lands with its behaviour change) rather than a misreading of the discussion's intent, and every doc target it names resolves to real, currently-matching prose in the source files.

## Verdict

APPROVE
Every card's claims verify against current source; DAG, numbering, and doc-lifecycle splits are all sound.
MILL_REVIEW_END
