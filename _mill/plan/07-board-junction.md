# Batch: board-junction

```yaml
task: 'fabric: shrink hubgeometry to the minimal illusion primitive (slice 7)'
batch: board-junction
number: 7
cards: 4
verify: go vet -tags "integration smoke scout" ./... && go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/lyxcwd/... ./cmd/lyx/...
depends-on: [6]
```

## Batch Scope

This batch wires the cwd-reachable `_board` junction: a link at `<AnchorPath>/_board` pointing at `<HubPath>/_board`, so an operator working inside a worktree can open, grep and read board data without knowing the hub path. It is exactly millhouse's own `.wiki` model, whose CLAUDE.md states that junction is "IDE/terminal convenience only, never a code path".

Three properties define it and every card here exists to hold one of them. It is **wire-only and unmonitored**: `Healthy`, `checkJunctionHealth` and `junctionRepointedDetail` are left untouched, because their reason strings feed loom preflight and a broken operator-convenience link must never be able to block real work. It is **unconditionally re-wired**: since nothing diagnoses the breakage, the repair cannot be conditioned on detection. And it is **for the human operator alone**: no lyx code path may read or write board data through it — every existing `BoardDir` consumer keeps resolving `<HubPath>/_board` directly and all board mutation continues through `internal/boardengine`.

Batch-local decision — `_board` is **not** added to `fabric.yaml`'s `pathspec`, and `filterHubReserved`, `ScopedPathspec` and `Topology.Add`'s reserved union are all left exactly as they are. Two facts make the pathspec route wrong rather than merely awkward. First, `pathspec` is dual-purpose: `fabriccli/weft_verbs.go:102` feeds the **raw unfiltered** `cfg.Dirs()` into `ScopedPathspec`, so adding `_board` would silently inject `<rel>/_board` into the weft *commit* pathspec — and `_board` is itself a weft worktree, not content to be committed from the warp side. Second, every pathspec junction is a warp↔weft **mirror pair** whose target is weft-worktree-level; this target is hub-level, so `hostJunctions` cannot derive it and every loop over the wired names would still need a special case.

## Cards

### Card 37: wire `_board` at clone and add

- **Context:**
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fslink/fslink.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `internal/fabricengine/clone.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/junction.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func wireBoardLink(l *lyxcwd.Location, slug string) error` to `internal/fabricengine/junction.go`: it creates a directory link at `filepath.Join(WorktreePath(l, slug), l.AnchorRel, BoardDirName)` targeting `BoardDir(l.HubPath)`, via `fslink.CreateDirLink`, and is idempotent — an already-correct link is a no-op costing one stat. The link is named `_board`, the same name as its target, **not** `.board`: there is no path collision because the two are always distinct directories, and in lyx's convention a leading `_` means durable and git-tracked while a leading `.` means ephemeral and machine-bound — the board data behind this link is durable, weft-tracked content, so `.board` would state the opposite of the truth. Call `wireBoardLink` at the same points the pathspec junctions are wired in clone and add, each as a **named special case** rather than a list entry. Also seed the name into the warp worktree's `.git/info/exclude` via a **standalone** `seedGitExclude(l, slug, []string{BoardDirName})` call — not by adding `_board` to the `names` slice `WireJunctions` passes down. `seedGitExclude` reads only `j.Name` off `hostJunctions(l, slug, names)`, so a one-element call works even though `_board` has no mirror-pair record; `reconcile.go:395` already uses exactly this shape for `unseedGitExclude` during stale removal.
- **Commit:** `feat(fabric): wire the operator-convenience _board junction at clone and add`

### Card 38: wire `_board` on reconcile, unconditionally with respect to junction health

- **Context:**
  - `internal/fabricengine/drift.go`
  - `internal/fabricengine/junction.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `internal/fabricengine/reconcile.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Call `wireBoardLink(hostLayout, slug)` in the pair loop body **next to `applyStaleRemoval`** (`reconcile.go:152`), inside the `weftWorktreeExists` branch — **not** inside the `if !junctionHealthy` block at `reconcile.go:137-150` where `WireJunctions` lives. This placement is not obvious and is the point of the card: `checkJunctionHealth` (`:135`) only ever inspects the pathspec name-set, which `_board` is deliberately outside, so a tree whose *only* broken link is `_board` reports `junctionHealthy == true`, takes the `else` at `:148-150` and is recorded `AlreadyHealthy`. Wiring inside the unhealthy branch would therefore never run in exactly the case it exists for. Unconditional placement is what makes wire-only-and-unmonitored coherent. Surface a wiring failure as a `pr.Detail` note, never as a `pr.Error` or a changed `pr.Action` — a convenience link must not be able to downgrade a reconcile verdict. The **missing-weft branch is excluded on purpose**: `reconcileMissingWeft` (`:164-197`) wires no junctions at all today — the recreate path adopts the weft worktree and returns `WeftRecreated`, the raw-adopt path creates a dormant weft branch and returns `RawAdopted`, and neither calls `WireJunctions`. `_board` follows that existing convention; hoisting it above the `weftWorktreeExists` check would produce a worktree with a board link but no working junctions, a half-wired state that reads as healthier than it is and one `Healthy` cannot correct for. The deferral is already the documented contract on that path — `createDormantWeftForRawHost`'s operator message at `:189` says to re-run reconcile to wire it — and the second run takes the `weftWorktreeExists` branch, where both the pathspec junctions and `_board` are wired together.
- **Commit:** `feat(fabric): re-wire the _board junction on every reconciled pair`

### Card 39: remove `_board` explicitly on unwire

- **Context:**
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/fabricengine/unwire.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `Unwire` must remove the `_board` link as an **explicitly named case**, with a matching standalone `unseedGitExclude(l, slug, []string{BoardDirName})`. It cannot be picked up generically: `unwire.go:49` enumerates names through `scanOnDiskJunctionNames`, which skips every name in `HubReservedNames()` (`reconcile.go:332-341`) — a set that already contains `_board`. That same skip is why **no change is required** to reconcile's stale sweep: the link is already invisible to it and cannot be removed as stale. Both facts are one consequence of the reserved-name skip — the generic machinery neither removes the link by accident nor removes it on purpose. Removing a `_board` link that is absent is not an error. Surface the removal in `UnwireVerbResult` alongside the existing fields rather than silently.
- **Commit:** `feat(fabric): remove the _board junction on unwire`

### Card 40: `_board` junction tests

- **Context:**
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/drift.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabriccli/weft_verbs.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/boardjunction_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** New integration-tagged test file with a `TestMain` calling `lyxtest.HermeticGitEnv()` if the package does not already have one. Assert: the link exists at `<AnchorPath>/_board` pointing at `<HubPath>/_board` after clone and after add; reconcile re-creates it when missing or mispointed, **constructing that scenario with only the `_board` link broken and every pathspec junction intact**, so the pair is reported `AlreadyHealthy` and the re-wire is still observed — that is what proves the wiring sits outside the `!junctionHealthy` branch, and it is the assertion that would have caught the placement error; the name appears in the warp worktree's `.git/info/exclude` after wiring and is gone after `Unwire`; a reconcile run immediately after wiring does **not** remove the link, pinning the stale-sweep protection, which comes from `scanOnDiskJunctionNames`'s `HubReservedNames` skip and is inherited rather than designed-in — it would break silently if that set changed; `_board` appears in **neither** `filterHubReserved`'s output **nor** `ScopedPathspec`'s, guarding against someone "simplifying" the special case back into the pathspec; a **deliberately broken** `_board` link leaves `Healthy` reporting healthy and `checkJunctionHealth` reporting ok, pinning the wire-only-and-unmonitored decision and guarding loom preflight against being blocked by a link nothing depends on; and after a reconcile pass returning `WeftRecreated` or `RawAdopted` the link is **absent**, while after the immediately following pass it is present.
- **Commit:** `test(fabric): cover the _board junction's wiring, exclusion and non-monitoring`

## Batch Tests

`verify` runs the repo-wide tagged type-check plus the `fabricengine`, `fabriccli`, `lyxcwd` and `cmd/lyx` suites. The new coverage in card 40 is integration-tagged because it needs a real clone and real filesystem links, so it is reached by the tagged `go vet` at every batch boundary and by `-tags integration` runs, not by the untagged `go test`.

The two assertions worth naming separately are the negative ones, because they pin decisions rather than behaviour: the broken-link-still-reports-healthy row is what keeps a convenience artifact from blocking loom preflight, and the absent-from-`filterHubReserved`-and-`ScopedPathspec` row is what stops a later "simplification" from folding `_board` back into a list that also feeds the weft commit scope.
