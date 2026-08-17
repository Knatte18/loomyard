# Batch: tokenvocab-plain-fields

```yaml
task: "shuttleengine + reedengine + tokenvocab told-geometry"
batch: "tokenvocab-plain-fields"
number: 2
cards: 2
verify: go test ./internal/tokenvocab/... ./internal/reedengine/...
depends-on: []
```

## Batch Scope

This batch converts `tokenvocab.Ctx` from a `*lyxcwd.Location` holder to two plain string fields, tightens the package's leaf-enforcement allowlist to match, and rewords the Tokenvocab Leaf Invariant in `CONSTRAINTS.md`.
It is one batch because `internal/tokenvocab` has exactly one consumer in the repo — `internal/reedengine/header.go` — so the conversion is fully contained and needs no other package's cooperation.
It is a root batch, parallel-safe with batch 1: batch 1 touches `internal/reedengine/lifecycle.go`, this batch touches `internal/reedengine/header.go`, and the two share no file.

The external interface batch 3 consumes is `tokenvocab.Ctx{RepoName, HubPath string}`, which `reedengine.Engine.HeaderText` populates from `e.geom` once `Geometry` exists.

Batch-local decision beyond `## Shared Decisions`: `header.go` still reads `e.layout.RepoName` / `e.layout.HubPath` after this batch.
The `*lyxcwd.Location` leaves `internal/tokenvocab` here; it leaves `internal/reedengine` in batch 3.

## Cards

### Card 4: Convert `tokenvocab.Ctx` to two plain fields

- **Context:**
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/tokenvocab/render.go`
  - `internal/stencil/stencil.go`
- **Edits:**
  - `internal/tokenvocab/tokenvocab.go`
  - `internal/tokenvocab/doc.go`
  - `internal/tokenvocab/tokenvocab_test.go`
  - `internal/tokenvocab/leaf_enforcement_test.go`
  - `internal/reedengine/header.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/tokenvocab/tokenvocab.go`, replace `Ctx`'s single `Layout *lyxcwd.Location` field with two plain string fields, `RepoName` and `HubPath`, each with its own doc comment naming the token it feeds.
  Change the `registry` entries so `repo` resolves `c.RepoName` and `hub` resolves `c.HubPath`, and delete the `github.com/Knatte18/loomyard/internal/lyxcwd` import — that import is the file's only non-stdlib dependency today, so the file ends with no import block at all.
  `Token`, `Build` and `internal/tokenvocab/render.go` keep their present shape; do not touch `Render`.
  In `internal/tokenvocab/doc.go`, reword the sentence describing the registry so it no longer says the two tokens are "resolved from lyxcwd.Location", and reword the leaf-invariant paragraph so the allowed set reads stdlib plus `internal/stencil`.
  In `internal/tokenvocab/leaf_enforcement_test.go`, delete the `"github.com/Knatte18/loomyard/internal/lyxcwd": true` entry from `allowedImports`, leaving only the `internal/stencil` entry, and update the file's header comment and the `t.Errorf` failure message, both of which name `lyxcwd` as allowed.
  In `internal/tokenvocab/tokenvocab_test.go`, replace every `Ctx{Layout: layout}` construction with `Ctx{RepoName: ..., HubPath: ...}` carrying distinct literal values, drop the `lyxcwd` import and the `*lyxcwd.Location` fixtures, and keep every existing assertion's intent: the per-token resolve table, the `repo` token case, the `Build` map case, the two `Render` cases, and the registry-coverage case.
  In `internal/reedengine/header.go`, replace `ctx := tokenvocab.Ctx{Layout: e.layout}` with `ctx := tokenvocab.Ctx{RepoName: e.layout.RepoName, HubPath: e.layout.HubPath}`.
  Do not change `internal/reedengine/header_test.go` here — its assertions read `e.layout.HubPath` / `e.layout.RepoName`, which still resolve, and batch 3 rewrites them onto `Geometry`.
- **Commit:** `refactor(tokenvocab): replace Ctx.Layout with plain RepoName and HubPath fields`

### Card 5: Reword the Tokenvocab Leaf Invariant

- **Context:**
  - `internal/tokenvocab/tokenvocab.go`
  - `internal/tokenvocab/leaf_enforcement_test.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Under the `## Tokenvocab Leaf Invariant` heading, change the statement sentence so the allowed import set reads stdlib and `internal/stencil`, dropping `internal/lyxcwd`.
  Leave the reverse-import bullet and the **Enforced by** bullet as they are — the enforcing test and its name have not changed.
  Follow the repo's semantic-line-break rule: one sentence per line, no fixed-column hard wrap.
  Change no other invariant in this file; the Treadle Runner-Seam reword is card 9's.
- **Commit:** `docs(constraints): tighten the Tokenvocab Leaf Invariant to stdlib plus stencil`

## Batch Tests

`verify:` runs the untagged suites of `internal/tokenvocab` and `internal/reedengine`.

- `internal/tokenvocab/tokenvocab_test.go` — the converted `Ctx` fixtures must keep asserting exactly what they assert today: that `repo` resolves to the repo name, `hub` to the hub path, that `Build` returns both keys, that `Render` fills a good template and errors on an unknown top-level token, and that the registry-coverage test still enumerates every token.
- `internal/tokenvocab/leaf_enforcement_test.go` — `TestLeafInvariant_AllowlistOnly` is the machine assertion that `internal/lyxcwd` is gone from the package's production imports.
  Tightening the allowlist and removing the import land in one card deliberately: splitting them would push a knowingly-red commit into the batch.
  The implementer should still work TDD-style inside the card — tighten `allowedImports` first, run the test, watch it fail on `tokenvocab.go`, then remove the import and watch it pass.
- `internal/reedengine/...` — the only reed change here is `header.go`'s `Ctx` construction, so `header_test.go`'s four existing cases are the regression gate: the empty-template default still renders the hub path, a configured template still renders the repo name, an unknown token still errors, and a good template still validates.
