MILL_REVIEW_BEGIN
# Review: invariants and docs for the told-geometry rule — holistic

```yaml
verdict: APPROVE
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 4.5 (Claude Agent SDK context identifies the underlying model as "Sonnet 5" / claude-sonnet-5, but I cannot independently confirm beyond what the harness states)
reviewed_file: plan/
date: 2026-08-19
```

## Verification performed

Read all five batch files and the overview, then cross-checked every load-bearing factual claim
against source: `internal/preflight/predicates.go` (ResolveMode's five-way outcome table),
`internal/preflight/preflight.go`/`doc.go` (Check/CheckResolved's PrimeName/Clean/Ready/Healthy
calls), `internal/lyxcwd/lyxcwd.go`/`anchor.go` (Resolve's four validation sub-points),
`internal/hubgeom/{doc,hubgeom,webstergeom}.go` and `internal/standalonegeom/*.go` (constructor
names, adapter direction, StencilsDir asymmetry), `internal/burlerengine/geometry.go` (Geometry's
two fields), `internal/buildinfo/doc.go` (byte-exact quoted sentence), all six named
leaf/seam-enforcement test files (tokenvocab, pattern, buildinfo, standalonestate, shedengine,
treadleengine) plus shuttleengine/scoutengine's seam tests (confirmed neither mentions lyxcwd),
the transitive lyxcwd-reachability claims (logger imports lyxcwd; stencilstore and shuttleengine
import logger), the eight review-obligation packages (confirmed none imports lyxcwd directly, and
none carries an enforcement/seam test), `internal/{batcher,stencilstore,shedadapters}` (confirmed
no direct lyxcwd import), `cmd/lyx/gitrepoboundary_test.go` (confirmed `diffMethodSets`,
`gitrepoBoundaryMinScannedFiles`, and the `go env GOMOD`/`t.Skip` shapes the new guard is asked to
follow), `cmd/lyx/tierpurity_test.go`'s `allowedSpawners` map (confirmed 14 existing entries),
`docs/overview.md` (confirmed the `#package-naming` broken link at line 29 and the
`internal/hubgeom/`→`internal/preflight/` diagram lines with no `standalonegeom` line between
them), `internal/lyxcwd/docslink_test.go`'s `docsLinkAllowlist` and `docsLinkSlug` (confirmed both
allowlist entries verbatim and that "Told-Geometry Invariant" slugs to
`told-geometry-invariant`), `manifest/roadmap.md` (confirmed exactly five `producers-standalone.md`
references — one Planned, four Done — and that `## Planned` holds exactly one item), the config.go
files for all eight named packages (confirmed the `Load`/`LoadOrTemplate` split matches the pinned
sets exactly), and `docs/shared-libs/README.md` (confirmed it omits preflight/hubgeom/standalonegeom,
consistent with the plan's own scoping rationale for leaving it untouched).

No divergence found between any plan claim and the tree it describes.

## Structural checks

Batch Index DAG: 5 batches, batch 1 has no deps, batches 2–5 each depend on `[1]` only, no cycle,
every `file:` present. Global step numbering: cards 1–16, sequential, matches each batch's
`cards:` count (3+3+5+4+1=16). All Files Touched (10 entries) is exactly the union of every
batch's `Edits:`/`Creates:` targets, Deletes/Move-source correctly excluded (no Moves anywhere in
this plan). Every card carries Context/Edits/Creates/Deletes/Moves/Requirements/Commit.
Requirements throughout name exact identifiers, functions, and file paths rather than vague
prose, and every named identifier's home file is present in that card's Context or Edits list.
No production Go behavior is proposed anywhere (doc.go prose and one new `_test.go` only), matching
the Shared Decision. No platform/harness behavior claims about Claude Code appear in this plan.

No findings.

## Verdict

APPROVE
Every load-bearing claim verified against source; DAG, scope, and card completeness are all sound.
MILL_REVIEW_END
