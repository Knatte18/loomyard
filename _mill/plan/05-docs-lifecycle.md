# Batch: docs-lifecycle

```yaml
task: 'Treadle: shared round-loop engine + perch rewrite'
batch: docs-lifecycle
number: 5
cards: 1
verify: go test ./...
depends-on: [4]
```

## Batch Scope

Closes the Documentation Lifecycle for the whole task: final package-header
passes on both engines, the shared docs map, the roadmap Planned→Done flip,
and deletion of the now-landed module-design doc. This batch runs last so
the docs describe the code as actually shipped across batches 1–4.

## Cards

### Card 14: documentation lifecycle for the landed Treadle

- **Context:**
  - `internal/treadleengine/run.go`
  - `internal/treadleengine/runner.go`
  - `internal/treadleengine/profile.go`
  - `internal/treadleengine/handoff.go`
  - `internal/treadleengine/targeting.go`
  - `internal/perchengine/adapter.go`
  - `internal/perchengine/config.go`
  - `manifest/designs/treadle.md`
  - `manifest/designs/hardener.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/treadleengine/doc.go`
  - `internal/perchengine/doc.go`
  - `docs/overview.md`
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:**
  - `manifest/designs/treadle.md`
- **Moves:** none
- **Requirements:**
  `internal/treadleengine/doc.go`: full package header absorbing
  `manifest/designs/treadle.md`'s key rationale before that file is
  deleted (module-design docs are deleted when their module lands; the
  package header becomes the source of truth — Documentation Lifecycle in
  `docs/overview.md`). Must cover: the generalized loop and what stays
  generic (retry/triage, stale-move, token naming, hydration assembly,
  gate, ladder, pause, lock, state), the attempt-level `RoundRunner` seam
  and why (Tenter headroom), the judge-maintained handoff (per-call files,
  lossless ledger + `covers_rounds`, two-layer fail-loud-parser /
  fail-safe-loop split, deterministic fallback to uncovered reviews,
  degrade-to-all-reviews), profile-gated pre-round targeting (fail-safe
  seed), name-parameterized diagnostics, geometry/weft-blindness, and the
  no-burler-import rule (pointing at the Treadle Runner-Seam Invariant in
  `CONSTRAINTS.md`).
  `internal/perchengine/doc.go`: final coherence pass — the package is now
  the burler-adapting configuration layer over treadleengine; every
  shipped invariant statement must match the as-built code after batches
  1–4 (handoff read-set, additive state fields, model-spec config keys);
  remove any remaining description of machinery that moved.
  `docs/overview.md`: add `internal/treadleengine/` to the internal-package
  tree listing with a one-line description; update the `perch` module
  bullet to name the treadleengine layering (behavior/CLI unchanged) and
  point at both package docs; leave the execution-stack table's `perch`
  row's builds-on note accurate (perch builds on burler AND treadleengine).
  `manifest/roadmap.md`: move the "Treadle: shared round-loop engine,
  combined with the perch rewrite" item from Planned to Done per the
  file's existing Done convention, linking the `internal/treadleengine`
  package documentation instead of the deleted design doc; scan the
  Someday `Tenter`/`Hardener` entries for links to
  `designs/treadle.md` and retarget them to the package doc (deleting a
  linked file must not leave dangling links — `manifest/designs/hardener.md`
  itself stays, it is not this task's module doc).
  Delete `manifest/designs/treadle.md` (git rm) in this same commit.
- **Commit:** `docs: absorb treadle design into package docs and close the roadmap item`

## Batch Tests

`verify:` is the full-module `go test ./...` — the task's terminal gate.
Justification for the unbounded scope: this is the last batch, the task's
changes span four packages plus repo-wide enforcement guards, and the
docs edits touch files (`CONSTRAINTS.md`-adjacent listings, overview) that
guard tests in `cmd/lyx` cross-check; one full run here replaces a
`done_gate` config change (which would trip the wiki-config-mutation
validator). The card itself is doc-only; no new runnable surface.
