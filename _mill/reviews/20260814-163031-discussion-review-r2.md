MILL_REVIEW_BEGIN
# Review: Move <hub>/.lyx into <hub>/_board

```yaml
duration_s: 252.5
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-08-14
```

## Findings

### [BLOCKING:design] seed-excludes-at-clone premise is false; its test is vacuous
**Section:** `### seed-excludes-at-clone` + Testing (clone-ordering coverage)
**Issue:** The stated window ("`_board/.lyx` reads as untracked dirt to the board dirty-gate") does not exist as described: `CloneHub` creates the directory EMPTY and git never reports an empty directory, and `WireJunctionsWith` (`internal/fabricengine/junction.go:110`) already seeds `.lyx/` into the weft COMMON gitdir at clone time via `internal/fabriccli/clone.go:86` — which the discussion itself notes covers every linked weft worktree, `_board` included. Consequently the named TDD assertion ("`git status` in `_board` reports clean") passes identically with and without the new call, so it cannot make the ordering load-bearing.
**Fix:** Restate the real exposure — `internal/fabriccli/clone.go:59`'s stage-all `Bolt.Commit` on `res.BoardDir` runs BEFORE wiring, so a future non-empty `_board/.lyx` would be committed onto `weft:main` — and specify an assertion that fails without the call, e.g. the weft common-gitdir exclude carries `.lyx/` at the instant `CloneHub` returns, before any wiring.

### [NIT:consistency] New reed→fabric edge falsifies the Treadle Runner-Seam Invariant text
**Demoted-from:** BLOCKING
**Section:** `### hub-scratch-constructor` / `## Constraints`
**Issue:** `internal/treadleengine/runner.go:14` → `internal/shuttleengine` → `internal/shuttleengine/reed.go:11` → `internal/reedengine` → (new) `internal/fabricengine`. CONSTRAINTS' Treadle Runner-Seam Invariant states the root pre-run seeding "is what keeps `internal/fabricengine` off treadle's stack"; that sentence becomes false. The guard is direct-imports-only so nothing fails, but the Constraints list never names this invariant at all.
**Fix:** State the disposition explicitly — accept the transitive edge and reword that CONSTRAINTS sentence in the implementation commit, or take the rejected `reedcli`-injection alternative.

### [NIT:scope] Stale-prose inventory named as complete but is not
**Demoted-from:** BLOCKING
**Section:** `### Prose and scenario surfaces naming the old paths` ("Beyond code, three...")
**Issue:** Four further texts go false and are named only generically or not at all: `docs/overview.md:111` + `:114` (hub tree showing `.lyx/` at hub level) and `:117` ("`_board`, `_portals`, `_launchers`, and `.lyx` are hub geometry"); `internal/fabricengine/slug.go:4`, a production doc comment naming `<Hub>/.lyx` as existing hub geometry; `manifest/designs/fabric-unified-view.md:71` ("`HubLogsDir` alone joins onto `Location.HubPath`, deliberately hub-anchored") and `:148` ("`<hub>/.lyx` shipped as a new hub-level geometry element").
**Fix:** Extend the inventory to these sites, or state that "update `docs/overview.md`/the design doc" is deliberately unenumerated and drop the "three texts" completeness claim.

### [NIT:decision] Disposition of the emptied helpers unstated
**Section:** `### slug-reservation-simplified`
**Issue:** With the append gone, `hubSlugReservedNames()` is byte-identical to `HubReservedNames()`; the discussion does not say whether the wrapper survives, and its justifying doc comment (`internal/fabricengine/junctionnames.go:154-155`, "would collide with the hub-level `<hub>/.lyx`") must be rewritten or deleted either way. `slugReservedNames(cfg)`, noted as having no production caller, likewise gets no keep/delete verdict.
**Fix:** State keep-or-fold for both, and that the `junctionnames.go` comment is rewritten with the change.

### [NIT:scope] Uncontained-write allowlist reason goes stale
**Section:** `## Constraints` (Fabric Write-Side Containment, "Untouched here")
**Issue:** `cmd/lyx/uncontainedwrite_test.go:72-74` justifies `clone.go`'s raw `os.MkdirAll` as "the hub `.lyx` directory ... written into a hub just minted by `createExclusiveDir`"; after the relocation the write lands inside the `containedWorktreeAdd`-minted `_board`, so the reason text no longer describes the code. The guard still passes, so nothing catches it.
**Fix:** Add that allowlist reason string to the update inventory.

## Verdict

REQUEST_CHANGES
Ordering rationale rests on a false premise; a constraint sentence and prose inventory need disposition.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
