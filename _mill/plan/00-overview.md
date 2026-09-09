# Plan: Centralize glyph ref-shape enumeration

```yaml
task: "Centralize glyph ref-shape enumeration"
slug: "centralize-glyph-shape-enum"
approved: false
started: "20260909-055817"
parent: "main"
root: ""
verify: go build ./...
discussion_sha: "b733f8d0c8594e24e52a600a0d1d0d1ef0954b87"
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: registry-core
    file: 01-registry-core.md
    depends-on: []
    verify: go test ./internal/planparser/ ./internal/planglyph/
  - number: 2
    name: exported-surface
    file: 02-exported-surface.md
    depends-on: []
    verify: go test ./internal/planparser/ ./internal/planglyph/
  - number: 3
    name: planparser-migration
    file: 03-planparser-migration.md
    depends-on: [1, 2]
    verify: go test ./internal/planparser/ ./internal/planglyph/
  - number: 4
    name: planglyph-migration
    file: 04-planglyph-migration.md
    depends-on: [2]
    verify: go test ./internal/planparser/ ./internal/planglyph/
  - number: 5
    name: status-family
    file: 05-status-family.md
    depends-on: [4]
    verify: go test ./internal/planparser/ ./internal/planglyph/
  - number: 6
    name: batch-coverage
    file: 06-batch-coverage.md
    depends-on: [4]
    verify: go test ./internal/planparser/ ./internal/planglyph/
  - number: 7
    name: boundary-enforcement
    file: 07-boundary-enforcement.md
    depends-on: [3, 4]
    verify: go test ./internal/planparser/ ./internal/planglyph/
```

## Shared Decisions

### Decision: registry-shape

- **Decision:** The registry is one new file, `internal/planparser/shape.go`, holding four things:
  the `disposition` type (three named values `dispKeep`/`dispSkip`/`dispFinding` declared `dispKeep disposition = iota + 1`, so the zero value is an unnamed "undeclared" that the lookup path fails closed on);
  the `refGate` type (a string naming one kind-gate) with one named `refGate` constant per gate and a package-level `ledger` of `map[refGate]map[refKind]disposition`;
  the lookup helper `lookup(g refGate, ref string) (refKind, disposition)` (classifies via `classifyRef`, panics on an undeclared disposition — never a silent skip);
  and the canonical `allRefKinds` slice plus the two relocated behavior-dispatch switches `diskPathForRef` and `refKindName`.
  The ledger unit is one kind-gate, not one function: a function with several independent gates registers one named policy per gate.
- **Rationale:** This is `_mill/discussion.md`'s Decision: kind-policy-ledger made concrete; the file boundary is what the enforcement scans exempt.
- **Applies to:** all batches

### Decision: exported-surface-placement

- **Decision:** The exported cross-package surface is the handle vocabulary only, and it lives in `internal/planparser/handle.go` (the handle grammar's owner), implemented with raw `strings.HasPrefix`/`strings.TrimPrefix`/concat against `HandlePrefix` — legal there because that file is exempt from the `plan:`-op scan, and deliberately without `classifyRef`/`refKind` (that file is NOT exempt from the `refKind` scan).
  `refKind` and `classifyRef` stay unexported.
- **Rationale:** Keeps the grammar in its declared owner and avoids widening the classifier's visibility (discussion Decision: registry-home-and-exported-surface).
- **Applies to:** exported-surface, planparser-migration, planglyph-migration, boundary-enforcement

### Decision: ast-enforcement-idiom

- **Decision:** Every source-scanning meta-test is AST-based (stdlib `go/parser`/`go/ast`, never raw text), scans production files only (`_test.go` skipped), resolves the repo root from `runtime.Caller(0)`, and carries no build tag — exactly the idiom of `internal/cliwire/bannedecl_enforcement_test.go`.
  Each matcher gets a seeded self-test: parse a synthetic in-test source string containing a deliberate violation and assert the matcher fires, then assert zero hits in the real tree.
- **Rationale:** A raw-text grep fails on an untouched tree (banned tokens live in comments today); the cliwire precedent already decides the mechanism (discussion review r4).
- **Applies to:** registry-core, status-family, batch-coverage, boundary-enforcement

### Decision: behavior-preservation

- **Decision:** Pure refactor except two named hardenings (batch 5's `containment.go` Status-half guard, batch 6's drift per-key guard), each landing with its own new regression tests.
  Every existing test's assertions stay untouched.
  Sanctioned behavior-neutral exceptions, all compile-driven: `internal/planparser/classify_test.go`'s wrapper test is rewired to `classifyRef`/`IsHandleRef`; `internal/planglyph/handle_test.go`'s `draftHandleIdentifier` table moves verbatim to planparser as `HandleIdentifier`'s test; comment-only updates wherever a comment names a deleted symbol.
  Any further behavior defect discovered mid-migration is recorded as a finding for a follow-up task, not fixed opportunistically.
- **Rationale:** The crucible regression suite is the proof that no shape handling silently changed under the registry — that proof only works if the suite is untouched (discussion Decision: behavior-preservation).
- **Applies to:** all batches

### Decision: docs-with-registry

- **Decision:** `CONSTRAINTS.md`'s new "Ref-Shape Registry Invariant" and the one-sentence registry addition to `manifest/designs/quarry-glyph-plan-alphabet.md`'s "The package-ownership seam" section land in the same commit as `internal/planparser/shape.go` (batch 1 card 1).
  `contracts/specs/loom-plan-spec.md` is not involved (classification behavior is identical); `docs/overview.md` is not involved (no new module); `manifest/roadmap.md` is not involved (this task is not a roadmap Planned item).
- **Rationale:** Documentation Lifecycle rule — docs land in the same commit as the code they describe; the discussion pins "same commit as the registry".
- **Applies to:** registry-core

### Decision: verify-scope

- **Decision:** Every batch's `verify:` is `go test ./internal/planparser/ ./internal/planglyph/` — the two touched packages' untagged unit suites (integration-tagged planglyph tests spawn quarry and are excluded by default).
  The overview-level module-wide `verify:` is `go build ./...`, catching cross-package compile breaks from export-surface changes.
  The configured `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) covers the repo-wide and integration surface at Handoff.
- **Rationale:** Per-batch scoping keeps the many-times-per-batch verify fast while the done gate still catches everything (CGO note: building requires `CGO_ENABLED=1` and a C compiler per the Quarry CGO Requirement Invariant).
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `internal/planglyph/chokepoint_enforcement_test.go`
- `internal/planglyph/containment.go`
- `internal/planglyph/containment_test.go`
- `internal/planglyph/create.go`
- `internal/planglyph/donecheck.go`
- `internal/planglyph/drift.go`
- `internal/planglyph/drift_test.go`
- `internal/planglyph/handle.go`
- `internal/planglyph/handle_test.go`
- `internal/planglyph/planglyph.go`
- `internal/planglyph/resolve.go`
- `internal/planglyph/status_enforcement_test.go`
- `internal/planparser/classify.go`
- `internal/planparser/classify_test.go`
- `internal/planparser/containment.go`
- `internal/planparser/doc.go`
- `internal/planparser/glyphref.go`
- `internal/planparser/glyphref_test.go`
- `internal/planparser/handle.go`
- `internal/planparser/handle_test.go`
- `internal/planparser/normalize.go`
- `internal/planparser/normalize_test.go`
- `internal/planparser/parse.go`
- `internal/planparser/parse_test.go`
- `internal/planparser/plan.go`
- `internal/planparser/rewrite.go`
- `internal/planparser/shape.go`
- `internal/planparser/shape_enforcement_test.go`
- `internal/planparser/shape_test.go`
- `internal/planparser/validate.go`
- `internal/planparser/validate_test.go`
- `manifest/designs/quarry-glyph-plan-alphabet.md`
