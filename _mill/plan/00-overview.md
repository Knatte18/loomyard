# Plan: Adopt quarry's glyph alphabet as the plan alphabet

```yaml
task: "Adopt quarry's glyph alphabet as the plan alphabet"
slug: "quarry-glyph-plan-alphabet"
approved: true
started: "20260906T104500Z"
parent: "main"
root: ""
verify: go build ./...
discussion_sha: 75468ceb4427355fea01ac52fb53e68b78344f02
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: quarry-dependency
    file: 01-quarry-dependency.md
    depends-on: []
    verify: go build ./...
  - number: 2
    name: planparser-alphabet
    file: 02-planparser-alphabet.md
    depends-on: [1]
    verify: go test ./internal/planparser/ ./internal/loomcli/ ./internal/loomshed/ ./internal/webstercli/ ./internal/loomrecipe/ && go test -tags integration ./internal/websterengine/
  - number: 3
    name: planparser-handles
    file: 03-planparser-handles.md
    depends-on: [2]
    verify: go test ./internal/planparser/ ./cmd/lyx/
  - number: 4
    name: planglyph
    file: 04-planglyph.md
    depends-on: [3]
    verify: go test ./internal/planglyph/ ./internal/planparser/ ./internal/lyxcwd/
  - number: 5
    name: gate-parity
    file: 05-gate-parity.md
    depends-on: [4]
    verify: go test ./internal/loomshed/ ./internal/loomcli/ ./internal/webstercli/ ./internal/websterengine/ ./internal/loomrecipe/ && go test -tags integration ./internal/websterengine/
  - number: 6
    name: quarry-cli
    file: 06-quarry-cli.md
    depends-on: [5]
    verify: go test ./internal/quarrycli/ ./internal/planglyph/ ./cmd/lyx/ ./internal/lyxcwd/
  - number: 7
    name: webster-drift
    file: 07-webster-drift.md
    depends-on: [6]
    verify: go test ./internal/planglyph/ ./internal/websterengine/ ./internal/webstercli/ ./cmd/lyx/ ./internal/lyxcwd/ && go test -tags integration ./internal/planglyph/ ./internal/websterengine/
```

## Shared Decisions

_Cross-cutting decisions every batch inherits._

### Decision: two-preconditions-outside-this-worktree

- **Decision:** two things this plan depends on are cut in the **quarry** repository, not here, and this worktree never touches quarry.
  (1) A real semver tag on quarry `main`, cut **after** quarry's own `glyph-unitpath` task merges, so one tag serves both preconditions.
  (2) The `Glyph.UnitPath() (string, bool)` accessor that tag must carry.
  Verified absent at planning time: quarry `main` carries only `archive/*` tags, and `glyph.Glyph` exposes `Lang`, `Unit`, `Owner`, `Name`, `Params` plus `IsSelf`/`String` — nothing maps a unit back to a disk path.
- **Rationale:** the worktree-isolation rule forbids editing another container's worktree, so these are preconditions this plan states rather than performs.
- **Applies to:** all batches. Card 1 names the tag; card 6 is the one card that consumes `UnitPath()` and is the one card that blocks if the accessor is not merged when the plan reaches it.

### Decision: glyph-conversion-chokepoint

- **Decision:** loomyard performs no glyph↔path conversion of its own.
  `glyph.Self(lang, path)` is the only path→glyph call, `Glyph.UnitPath()` is the only glyph→path call, and `glyph.Parse` plus `Glyph.String()` are the only glyph grammar.
  No `strings.TrimSuffix(s, "#")`, no reading `Glyph.Unit` as a disk path, no loomyard-side regex over a glyph string.
- **Rationale:** the Go-only strip rule is quarry's contract, so a loomyard copy of it would silently leak the Go assumption into a non-Go plan.
  Rejected outright, not deferred: a temporary loomyard-side helper implementing the trim until the accessor lands.
- **Applies to:** all batches. Recorded in `CONSTRAINTS.md` by card 15 and guarded by a constraint test in that same card.

### Decision: package-ownership-seam

- **Decision:** `internal/planparser` imports the pure `github.com/Knatte18/quarry/glyph` package only and stays a tier1-pure leaf.
  A new package `internal/planglyph` owns **every** `quarry.Repo` call (`Open`, `Resolve`, `DeltaGit`) and the package-level `quarry.Name`, plus the resolve-backed validation pass.
  `planglyph.ValidateFormat` and `planglyph.Validate` **call** `planparser.ValidateFormat`/`planparser.Validate` and add only resolve findings on top — no check is implemented twice.
- **Rationale:** keeps `planparser` pure and preserves the `ValidateFormat`/`Validate` split the Gate Self-Check Parity Invariant depends on.
- **Applies to:** batches 2, 3, 4, 5, 7.

### Decision: told-geometry-for-planglyph

- **Decision:** `internal/planglyph` derives no path of its own and never imports `internal/lyxcwd`.
  Its validation entry points take `worktreeRoot string` (and, where the plan directory is needed, the anchor path) exactly as `planparser.Validate(plan, worktreeRoot)` does.
  The `lyx quarry` verb group probes tier 1 via `preflight.ResolveMode(cwd)` and operates on the current worktree only — no arbitrary-repo path flag.
- **Rationale:** `quarry.Open(root)` needs an absolute root, and `internal/planparser` is already on the bound-packages list carrying a plain `worktreeRoot string`, proving membership turns on deriving no paths rather than on carrying a `Geometry`.
  Refusing an arbitrary-repo flag keeps the planner's answers and the validator's `Resolve` on the same tree, which is the copied-verbatim guarantee.
- **Applies to:** batches 4, 6, 7.

### Decision: blocking-policy

- **Decision:** split by determinism.
  Done-check failures **block**: a `Create` target that still does not resolve, a `Delete` target that still resolves, a card-count mismatch on binding.
  Evidence-tier drift candidates and the glyph scope guard stay **informational**.
  An infrastructure error from `quarry.Open`/`Resolve`/`DeltaGit` is a third category that blocks at `Plan-Revalidate` and `begin-batch`, blocks the done-checks at `record-batch`, and degrades only the scope guard to informational.
  **Severity governs every gate verdict, at every boundary.** A findings set carrying no blocking finding is a pass: `Plan-Validate`/`Plan-Revalidate` report `Done`, `validate-plan` and `webster validate` emit a success envelope, and `begin-batch` dispatches — each still surfacing the informational findings for visibility.
- **Rationale:** mirrors quarry's own two-tier philosophy — block on what is mechanically asserted, stay informational on what quarry itself refuses to decide.
  Conflating an infrastructure error with a `not_found` payload would let a quarry outage silently mark every `Create` card done.
  The severity rule is load-bearing rather than tidy: `create-new-unit` fires on every plan that adds a brand-new package — including this task's own `internal/planglyph` and `internal/quarrycli` — and nothing about creating a package is something `Plan-Write` can fix, so bouncing on it would resubmit an unchanged plan until the segment's bounce budget escalated to a human over a condition that was never wrong.
- **Applies to:** batches 4, 5 and 7.

### Decision: no-new-rows-no-new-verbs

- **Decision:** this task adds **no** `internal/shedrecipe` registry entry and **no** new CLI verb beyond the `lyx quarry` group.
  `Plan-Revalidate` is the only `ShedProducer` row involved, and it is extended together with `validate-plan --require-approved` per the Gate Self-Check Parity Invariant.
  The two webster boundaries are `websterengine` functions called by the **already-existing** `begin-batch`/`record-batch` verbs.
- **Rationale:** `internal/shedrecipe`'s registry holds no `BeginBatch`/`RecordBatch` entries, so those two boundaries have verbs and no rows — there is no pair for parity to hold.
- **Applies to:** batches 5 and 7.

### Decision: go-native-verify-commands

- **Decision:** every `verify:` in this plan is a native Go command with no `PYTHONPATH=` prefix, scoped to the packages the batch touches.
  The overview's module-wide `verify: go build ./...` runs after each batch's own verify.
- **Rationale:** this is a Go repository; the `PYTHONPATH=` prefix rule is Python-project-specific.
  `go build ./...` is a cheap whole-module compile that catches a cross-package break at the batch that introduced it, while the hub's own `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) catches regressions outside every batch verify's scope at task end.
- **Applies to:** all batches.

### Decision: format-5-is-a-hard-break

- **Decision:** the plan format bumps to `5` and `checkFormatRecognized` accepts exactly that one version.
  A format-4 plan fails loud rather than half-parsing under the new alphabet.
- **Rationale:** the spelling of every symbol entry changes, and `plan:` handles plus `language:` are new grammar.
  There is nothing to migrate: `_lyx/plan/` is untracked weft content and nothing matches on `origin/main`, so the rewrite surface is the golden fixture, the spec's worked example, and the stencil.
- **Applies to:** batches 2 and 3.

## All Files Touched

- `CLAUDE.md`
- `CONSTRAINTS.md`
- `README.md`
- `cmd/lyx/constraintchokepoint_test.go`
- `cmd/lyx/helptree_test.go`
- `cmd/lyx/main.go`
- `cmd/lyx/seamsignature_test.go`
- `cmd/lyx/spawnobservability_test.go`
- `cmd/lyx/tierpurity_test.go`
- `contracts/specs/loom-plan-spec.md`
- `contracts/stencils/loom/loom-rubric-plan-review.md`
- `contracts/stencils/loom/loom-template-plan.md`
- `contracts/stencils/webster/webster-body-implementer.md`
- `docs/overview.md`
- `go.mod`
- `go.sum`
- `internal/loomcli/parity_test.go`
- `internal/loomcli/validate.go`
- `internal/loomcli/validate_test.go`
- `internal/loomrecipe/fixture_test.go`
- `internal/loomshed/gatefindings_test.go`
- `internal/loomshed/planvalidate.go`
- `internal/loomshed/planvalidate_test.go`
- `internal/planglyph/containment.go`
- `internal/planglyph/containment_test.go`
- `internal/planglyph/create.go`
- `internal/planglyph/create_test.go`
- `internal/planglyph/delta.go`
- `internal/planglyph/delta_integration_test.go`
- `internal/planglyph/doc.go`
- `internal/planglyph/donecheck.go`
- `internal/planglyph/donecheck_integration_test.go`
- `internal/planglyph/drift.go`
- `internal/planglyph/drift_integration_test.go`
- `internal/planglyph/handle.go`
- `internal/planglyph/handle_test.go`
- `internal/planglyph/planglyph.go`
- `internal/planglyph/planglyph_test.go`
- `internal/planglyph/repo.go`
- `internal/planglyph/repo_test.go`
- `internal/planglyph/resolve.go`
- `internal/planglyph/resolve_test.go`
- `internal/planglyph/scope.go`
- `internal/planglyph/scope_test.go`
- `internal/planglyph/testmain_test.go`
- `internal/planparser/amendment.go`
- `internal/planparser/amendment_test.go`
- `internal/planparser/approve_test.go`
- `internal/planparser/classify.go`
- `internal/planparser/classify_test.go`
- `internal/planparser/containment.go`
- `internal/planparser/containment_test.go`
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
- `internal/planparser/rewrite_test.go`
- `internal/planparser/sections_test.go`
- `internal/planparser/testdata/goodplan/00-overview.md`
- `internal/planparser/testdata/goodplan/01-json-row-type.md`
- `internal/planparser/testdata/goodplan/02-json-flag.md`
- `internal/planparser/testdata/goodplan/03-json-emission.md`
- `internal/planparser/testdata/goodplan/04-legacy-rows-delete.md`
- `internal/planparser/testdata/goodplan/05-rowmapper-rename.md`
- `internal/planparser/testdata/goodplan/06-helppins-move.md`
- `internal/planparser/testdata/goodplan/07-json-docs.md`
- `internal/planparser/validate.go`
- `internal/planparser/validate_test.go`
- `internal/quarrycli/cli.go`
- `internal/quarrycli/cli_test.go`
- `internal/quarrycli/expand.go`
- `internal/quarrycli/glyphs.go`
- `internal/quarrycli/resolve.go`
- `internal/quarrycli/toc.go`
- `internal/quarrycli/verbs_test.go`
- `internal/webstercli/beginbatch.go`
- `internal/webstercli/cli_test.go`
- `internal/webstercli/recordbatch.go`
- `internal/webstercli/validate.go`
- `internal/websterengine/beginbatch.go`
- `internal/websterengine/beginbatch_test.go`
- `internal/websterengine/recordbatch.go`
- `internal/websterengine/recordbatch_test.go`
- `internal/websterengine/runlevel.go`
- `internal/websterengine/runlevel_test.go`
- `manifest/designs/loom.md`
- `manifest/designs/plan-card-format.md`
- `manifest/designs/quarry-glyph-plan-alphabet.md`
- `manifest/roadmap.md`
- `tools/deploy/main.go`
- `tools/sandbox/SANDBOX-CORE-SUITE.md`
- `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
