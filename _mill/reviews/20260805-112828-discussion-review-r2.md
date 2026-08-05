MILL_REVIEW_BEGIN
# Review: fabric: shrink hubgeometry to the minimal illusion primitive (slice 7)

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: claude-opus-5 (self-assessed)
reviewed_file: _mill/discussion.md
date: 2026-08-05
```

## Findings

### [GAP] DotEnv move creates a configengine↔envsource cycle
**Section:** `### config-path-move`
**Issue:** The sole production caller of `DotEnv` is `internal/envsource/envsource.go:22`, and `internal/configengine/config.go:15` already imports `envsource` — moving `DotEnv` into `configengine` makes `configengine → envsource → configengine` a compile-time cycle; `envsource` is also absent from the decision's call-site list.
**Fix:** Decide `DotEnv`'s destination against that cycle (e.g. leave it with `envsource`, which already owns the `.env` format) and record `envsource` as a call site.

### [GAP] Wrong leaf allowlist widened for the config-path move
**Section:** `### leaf-invariant-updates` / `### config-path-move`
**Issue:** `internal/scoutengine/load.go:23` calls `hubgeometry.ConfigFile` in production, so it must import `configengine` after the move — but the Scoutengine Leaf allowlist is not widened; conversely `internal/tokenvocab` production uses only `Layout` (`tokenvocab.go:12`), never a config path, yet its allowlist is widened.
**Fix:** Widen the Scoutengine Leaf Invariant to include `internal/configengine` and drop the unjustified `tokenvocab` widening, or state the `tokenvocab` consumer that needs it.

### [GAP] Batch 1's Location reshape breaks batch 2–4 consumers
**Section:** `### batching` vs `### location-struct` / `### prime-and-list-move`
**Issue:** Batch 1 replaces `Layout{Cwd, WorktreeRoot, Hub, RelPath, Prime, Repo}` with the four-field `Location`, but `l.Prime` consumers (`loomengine/preflight.go:67`, `vscode/color.go:47`, plus in-module `hubgeometry.go:466,472`) are scheduled for batch 4 and the ~30 in-module constructors reading `Cwd`/`WorktreeRoot` for batch 2 — batch 1 cannot compile, contradicting "each batch must leave `go build ./...` and the full suite green".
**Fix:** State either that batch 1 rewrites every field consumer in place (making batches 2/4 pure relocation) or that `Location` keeps transitional fields, and note that `LauncherSpawnRel`/`PrimeName` need a prime argument once `Prime` leaves the struct.

### [GAP] Guard rewrite scheduled last, but batches 1 and 3 trip it
**Section:** `### enforcement-rewrite` / `### batching`
**Issue:** `enforcement_test.go:420` allowlists the literal directory `internal/hubgeometry`, and the token set at `:224` includes `-weft`, `_board`, `-HUB`, `_portals`, `_launchers`, `_raddle` — so the batch-1 package rename plus `weftname`'s `-weft` const, and batch 3's `_portals`/`_launchers`/`_raddle` consts landing in `fabricengine`, all fail the guard well before batch 5.
**Fix:** Split the guard work — allowlist-path update in batch 1 and per-token owners added as each batch moves them — or drop the green-per-batch claim for the guard test explicitly.

### [GAP] `.board` junction has no stated consumer or git hygiene
**Section:** `### board-junction`
**Issue:** The decision fixes the name, slice and non-pathspec wiring but never names who reads `<AnchorPath>/.board` (all ten `BoardDir` callers keep using the hub path), and says nothing about keeping the new link out of warp git (`seedGitExclude`/`unseedGitExclude`, `junction.go:328,273`) or how `Unwire`'s `scanOnDiskJunctionNames` treats it — leaving the design doc's third open item (`fabric-unified-view.md:42`) unresolved.
**Fix:** Name the consumer that justifies the link and state its warp-exclude and unwire behaviour alongside the wiring points.

### [NOTE] `deriveRepo` moves to fabricengine as dead code
**Section:** `### prime-and-list-move` vs `### reponame-derivation`
**Issue:** `RepoName` is redefined as `TrimSuffix(Base(HubPath), "-HUB")`, so `deriveRepo(prime, worktreeRoot)` has no remaining consumer, yet it is listed as moving into `fabricengine`.
**Fix:** Say it is deleted rather than moved, or name the `fabricengine` caller.

### [NOTE] Out-of-package `Layout` struct literals unaccounted for
**Section:** `### location-struct`
**Issue:** `scoutengine/ensureserver.go:300` builds `&hubgeometry.Layout{WorktreeRoot: worktreeRoot}`; `Location` has no such field, and the discussion inventories field *readers* but not synthetic constructions.
**Fix:** Note that synthetic `Layout` literals must be re-expressed as `HubPath`+`WorktreeName` (or the constructor take a plain path) and assign that to a batch.

### [NOTE] Untagged strict-gate test needs an extracted gate
**Section:** `## Testing`
**Issue:** The gate table test is specified as pure string math over `(cwd, anchorRel, worktreePath)`, but the gate lives inside `Resolve`, which spawns git (`hubgeometry.go:103`); only the comparison helper is pinned as a named function.
**Fix:** State that the gate itself is an independently callable helper taking those three values, not just the path comparator.

## Verdict

GAPS_FOUND
Two import/allowlist errors, two batch-ordering contradictions, and an underspecified `.board` junction.
MILL_REVIEW_END
