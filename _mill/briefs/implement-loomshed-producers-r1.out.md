Task: implement batch 2 (`loomshed-producers`) of the `loom-phase-machine-scaffolding` plan — 8 cards (3-10) creating `internal/loomshed`.

All 8 cards completed and committed. Verify passed: `go test ./internal/loomshed/... ./internal/lyxcwd/... ./cmd/lyx/...` — clean. Working tree clean, no uncommitted tracked changes.

Commits (parent `cc9ebf37`, batch-start marker):
- `bfa242d8` — feat(loomshed): create the package with its cancellation helper and import guard (doc.go, ctx.go, ctx_test.go, seam_enforcement_test.go, CONSTRAINTS.md, docs/overview.md)
- `5e49bbab` — feat(loomshed): add the stub producer backing the seven not-yet-built rows (stub.go, stub_test.go, manifest/designs/loom.md)
- `a247948e` — feat(loomshed): build the Discussion-Validate producer (discussionvalidate.go, discussionvalidate_test.go)
- `9dee8ab9` — feat(loomshed): build the Plan-Validate producer (planvalidate.go, planvalidate_test.go)
- `bfc3c15e` — feat(loomshed): build the Batchifier gate and the lazy Webster wrapper (batchifier.go, webster.go, batchifier_test.go, webster_test.go, manifest/designs/loom.md)
- `ce3e38eb` — feat(loomshed): wire in Preflight behind NewPreflightProducer (preflight.go, no untagged test per Test Tier Purity)
- `ffb0f2c6` — feat(loomshed): assemble loom's 12-row producer list behind Deps and New (loomshed.go, loomshed_test.go)
- `a3e4e18d` — feat(loomshed): add Seed, the production writer for the initial status file (seed.go, seed_test.go)

8 of 8 cards committed. All in scope — no files touched outside the batch's declared `Context:`/`Edits:`/`Creates:` lists.
