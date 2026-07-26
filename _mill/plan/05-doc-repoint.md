# Batch: D2 -- doc repoint

```yaml
task: 'fabric: cutover -- rewire consumers onto fabric, delete warp/weft'
batch: D2 -- doc repoint
number: 5
cards: 3
verify: go build ./...
depends-on: [4]
```

## Batch Scope

Delete the `manifest/designs/fabric.md` design doc (its rationale already lives durably in
`internal/fabricengine/doc.go`, and its own status banner prescribes deletion at cutover),
record a short **Done** entry in the roadmap, and repoint every other doc that links to
`fabric.md` at `internal/fabricengine/doc.go` so no link dangles. Depends on batch 4 (the
module is gone; the code is now the source of truth). Pure docs batch -- `verify: go build
./...` is a cheap smoke that no code was touched by accident. `docs/overview.md` gets its own
card because it needs a structural rewrite (module table + parallel-build banner), not just a
link swap.

## Cards

### Card 19: delete fabric.md, add roadmap Done entry

- **Context:**
  - `internal/fabricengine/doc.go`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:**
  - `manifest/designs/fabric.md`
- **Moves:** none
- **Requirements:** `git rm manifest/designs/fabric.md`. In `manifest/roadmap.md`, convert the
  existing fabric entry (currently linking to `manifest/designs/fabric.md`) into a short
  **Done** entry as plain text with NO link to the deleted file -- e.g. move it under the Done
  section with a one-line description ("fabric -- unified host<->weft git-coordination module
  replacing warp/weft; cut over and old modules deleted"). Per the repo rule, do not add
  bugfix/polish notes to the roadmap; this is the single completed-item move.
- **Commit:** `docs(roadmap): mark fabric done, delete fabric.md design doc`

### Card 20: repoint inbound fabric.md links to doc.go

- **Context:**
  - `internal/fabricengine/doc.go`
- **Edits:**
  - `manifest/designs/board-weft-storage.md`
  - `manifest/designs/raddle.md`
  - `manifest/designs/loom-finalize.md`
  - `manifest/designs/host-visibility.md`
  - `manifest/designs/codeintel-redesign.md`
  - `docs/reference/plan-format-v3.md`
  - `crucible/fabric-review-prompt.md`
  - `crucible/gitrepo-review-prompt.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In each file, replace every markdown link whose target is
  `manifest/designs/fabric.md` (relative forms vary per file: `fabric.md`,
  `designs/fabric.md`, `../manifest/designs/fabric.md`, etc.) with a link to the source of
  truth `internal/fabricengine/doc.go`, using the correct relative path from that file's
  location to the repo root. Keep the surrounding sentence intact -- only the link target
  (and, if the link text literally says "fabric.md", the visible text) changes. `crucible/`
  is durable review-prompt scaffolding, so it is in scope. This is a mechanical link swap;
  a script may drive it, but confirm each rewritten relative path actually resolves.
- **Commit:** `docs: repoint fabric.md links to internal/fabricengine/doc.go`

### Card 21: rewrite docs/overview.md onto fabric

- **Context:**
  - `internal/fabricengine/doc.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite every warp/weft reference to reflect the post-cutover world:
  remove the separate `warp` and `weft` rows from the module table and ensure `fabric` is
  present as the sole git-coordination module (adjust the stated module count if the table
  header carries one); delete the fabric "parallel-build" banner/callout and its inbound
  `fabric.md` link, replacing it with a one-line description of fabric as the shipped
  git-coordination module (link to `internal/fabricengine/doc.go` for rationale); and fix the
  intro prose that names warp/weft as live modules. Keep descriptions of the weft *repo/role*
  concept (fabric's weft sibling) accurate -- only the deleted *modules* are removed. Do not
  invent new sections.
- **Commit:** `docs(overview): rewrite warp/weft references onto fabric`

## Batch Tests

Pure docs batch: no runnable test surface. `verify: go build ./...` is a cheap guard that no
`.go` file was touched by mistake during the repoint. Link correctness (relative paths
resolve to `internal/fabricengine/doc.go`) is a review obligation, verified by the plan
reviewer and by card 20's own resolve check -- there is no automated markdown link checker in
this repo.
