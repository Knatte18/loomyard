# Plan: Audit the remaining leaf and seam import invariants

```yaml
task: "Audit the remaining leaf and seam import invariants"
slug: "leaf-invariant-audit"
approved: true
started: "20260808-062516"
parent: "main"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: land-audit-corrections
    file: 01-land-audit-corrections.md
    depends-on: []
    verify: go build ./... && go test ./internal/lyxtest/... ./internal/modelspec/... ./internal/treadleengine/... ./internal/tokenvocab/... ./internal/pattern/... ./internal/shuttleengine/... ./internal/githubclient/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints._

### Decision: the audit's reasoning lives in a commit message, nowhere else

- **Decision:** the task creates no durable audit document — no `manifest/designs/` page, no `docs/` page, no `_raddle/` note, and no `KEEP`/`AUDITED`/date verdict lines inside `CONSTRAINTS.md`.
  The audit's findings are carried by card 1's commit **body**, which is mandatory content spelled out verbatim in that card's `Requirements:`.
- **Rationale:** `discussion.md` dies with the worktree by design, and `CONSTRAINTS.md` line 5 states the file holds rules only — no rationale, no incident narratives, no historical justification.
  A stored audit page also rots in the dangerous direction: "modelspec is genuinely leaf" becomes a confident false statement the moment someone adds an import, whereas the enforcement test carrying the same guarantee fails in CI instead of lying.
  A commit message rots harmlessly — nobody mistakes one for current truth.
  The branch's individual commits survive the squash-merge under the archive tag `/mill-merge` creates, so the body remains reachable after merge.
- **Applies to:** all batches

### Decision: no production behaviour changes anywhere in this task

- **Decision:** the only non-comment change in the whole plan is `internal/lyxtest/leaf_enforcement_test.go`'s enforcement mechanism.
  Every other edit is comment text or `CONSTRAINTS.md` prose.
  No production `.go` file outside a comment block is touched, and no new test case is added.
- **Rationale:** the audit concluded that no invariant needs removing or weakening — all seven survive.
  What it found is rule *text* that misdescribes what the rule enforces, plus one rule whose stated import set was never enforced.
  Widening beyond that turns a correction pass into a refactor.
- **Applies to:** all batches

### Decision: scoutengine is out of scope, everywhere

- **Decision:** do not edit `CONSTRAINTS.md`'s Scoutengine Leaf Invariant section, `internal/scoutengine/doc.go`, or `internal/scoutengine/leaf_enforcement_test.go`.
  Do not use scoutengine's enforcement test as the shape reference for the lyxtest conversion.
- **Rationale:** the parallel `scout-seam-conversion` task owns those files and is changing that test concurrently, so its shape is not stable at merge time.
  A same-file collision between the two tasks is exactly what this exclusion exists to prevent.
- **Applies to:** all batches

### Decision: the Fabric Vocabulary Invariant binds the treadle comment rewrites

- **Decision:** rewritten comments in `internal/treadleengine` must not introduce `weft` or `warp` as bare tokens, nor a fabric-sense `host` phrase (`host repo`, `host worktree`, `host branch`).
- **Rationale:** `internal/treadleengine` is not in the Fabric Vocabulary Invariant's owner set, and `TestEnforcement_FabricVocabulary` scans identifiers, string literals, **and comments** in production `.go` files under `internal/`.
  The bare word `fabric` is unpoliced, so the existing "fabric-blind and geometry-blind" phrasing is safe to keep;
  the risk is only in reaching for fabric vocabulary while rewriting.
  `internal/lyxtest` *is* in the owner set, so its `doc.go` edit is unconstrained here.
- **Applies to:** land-audit-corrections

### Decision: markdown and Go comment formatting follow the repo conventions

- **Decision:** `CONSTRAINTS.md` edits use semantic line breaks — one sentence per line, plus a break at internal independent-clause boundaries, never a fixed-column hard wrap.
  New Go doc-comment text matches the semantic-line-break reflow the repo ran in commit `99fccc55`.
- **Rationale:** `CLAUDE.md` mandates semantic line breaks in every `.md` file in the repo, and the `golang-comments` skill governs godoc formatting.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `internal/lyxtest/doc.go`
- `internal/lyxtest/leaf_enforcement_test.go`
- `internal/modelspec/leaf_enforcement_test.go`
- `internal/shuttleengine/seam_enforcement_test.go`
- `internal/treadleengine/doc.go`
- `internal/treadleengine/engine.go`
