MILL_REVIEW_BEGIN
# Review: Rename internal/paths to internal/hubgeometry — holistic

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-30
```

## Verdict

APPROVE

The plan is complete, accurate, and constraint-faithful. Verified against source: all 81 files referencing `internal/paths` (excluding `_mill/`) are covered across cards 1–11; no importer is missed. The two machine-enforced guard literals are correctly located and described — `enforcement_test.go` raw-primitive allowlist `pkgDir == "internal/paths"` (line 69) and geometry-literal allowlist (line 347), plus `codeguide_guard_test.go` filename skip `d.Name() == "paths.go"` (line 48). Package-clause classification is exactly right: white-box (`package paths`) for `paths.go`/`worktreelist.go`/`enforcement_test.go`/`codeguide_guard_test.go`, black-box (`package paths_test`) for the other five. Docs anchor handling is correct — `docs/overview.md` heading `## Path Invariants` (plural) and the live cross-doc link `loom.md` `#path-invariants` are both renamed in the same card (Card 10), avoiding a silent 404. "Path Invariant" prose lives only in `overview.md`/`CONSTRAINTS.md`/`CLAUDE.md`, all covered. The `weftengine` `Pathspec`/`pathspec` hits are correctly excluded as unrelated. DAG is acyclic (batch 2 depends on 1, files present), step numbering is unique/sequential 1–11, every card carries all required fields, all `Moves:` are well-formed backtick pairs with both batches carrying a `## Rename mechanic` section, and `verify` rightly adds `go vet -tags integration ./...` to compile the three build-tagged files that `go build`/untagged `go test` would skip.
MILL_REVIEW_END
