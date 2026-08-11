MILL_REVIEW_BEGIN
# Review: fabric: accumulate the result envelope from mutations, not control flow (slice 14) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Anthropic Claude, Opus-class (harness reports claude-opus-5); best-effort self-assessment
reviewed_file: plan/
date: 2026-08-11
```

## Findings

### [BLOCKING:design] Omission direction fires on ancestor create/prune
**Location:** batch 7 card 29 (coverage rule) × batch 5 card 21 / batch 4 card 13
**Issue:** The coverage set is subtree-downward only, but fabric creates and destroys *ancestors* of every recorded root: `fslink.CreateDirLink` MkdirAlls `filepath.Dir(link)` (portals.go:43 comment), `writeLaunchers` MkdirAlls `_launchers/<anchorRel>/<slug>`, and `removePortal`/`removeLaunchers` call `pruneEmptyAncestors` (portals.go:70, launchers.go:204) — the harness already had to permit `filepath.Dir(portal)`/`filepath.Dir(launcher)` for exactly this (verbs.go:341-346). Those `_portals/<anchor>` and `_launchers/<anchor>` add/remove diffs are covered by no record entry, so the omission direction fails on correct behaviour in the Add, Remove and Prune cells.
**Fix:** State a coverage or recording rule for implicitly-created/pruned ancestor directories (record them, or make the coverage rule account for a recorded root's created/pruned ancestor chain).

### [BLOCKING:design] `removeLink` records an already-absent link
**Location:** batch 4 card 12 (`removeLink` bullet)
**Issue:** `fslink.Remove` returns nil when the target is absent (fslink.go:40-42) and `checkPathRequest` deliberately passes an absent target as an idempotent no-op (destroy.go:511-514, naming removePortal/removeJunctionRecords). "Record on a nil error from `fslink.Remove`" therefore appends `link_removed` for a removal that never happened — the exact lie of commission the plan's record-only-on-observed-effect Shared Decision forbids, and a commission-direction failure in batch 7 for any cell whose junction was already gone.
**Fix:** Give `removeLink` the same observed-effect predicate `removePath` gets (probe presence before the primitive; record only when something was actually removed), and add the absent-target case to card 16's table.

### [BLOCKING:design] The `"."` exemption leaves the reset teardown uncovered
**Location:** batch 7 card 29 (hub-root exemption) × batch 4 card 14 (`resetHub`)
**Issue:** `resetHub` removes the whole hub through `removePath` at `target = hubPath` (clone.go:579-590), so the only possible entry for that teardown is the `"."`-targeted one card 29 skips in both directions — and if `rec` is still nil there (card 14 explicitly allows it) nothing is recorded at all. `TestCloneHubReset` captures before/after at `fixture.ResetHubPath`, so every path the old hub carried and the new clone does not surfaces as an uncovered `ChangeRemoved` in the unfiltered diff for the `CloneHubReset/RealHub` cell.
**Fix:** Resolve the reset teardown's coverage explicitly (e.g. record the teardown at a non-`"."` target, or exempt the reset column from the omission direction with the reason stated); note `hubPath` is in fact derived at clone.go:153/198, immediately before both `resetHub` calls, so card 14's "if the implementer finds it derivable" hedge is already answered by the source.

### [BLOCKING:scope] `ReconcileAll` writes N config files, not one
**Location:** batch 6 card 25 ("the two `configsync` calls as `KindFileWritten` on the config path each wrote")
**Issue:** `configsync.ReconcileAll(baseDir, apply)` returns `[]Result` — one per registered module (configsync.go:77-80) — so "the config path each wrote" under-specifies by an unbounded factor. A single entry per call leaves every other materialised config file as an uncovered addition, which card 25 itself says the omission direction will fire on.
**Fix:** Name the rule: one `KindFileWritten` per `Result` that reports a write (or one entry at the config directory), and say which `Result` field carries the path.

### [NIT:consistency] Stale card cross-references
**Location:** batch 6 cards 24, 25, 26; overview `## Shared Decisions` (covering-root decision)
**Issue:** Cards 24/25/26 all route output through "the card-22 helpers" — the envelope helpers are card 23; card 22 is batch 5's engine test card. The covering-root Shared Decision cites "the oracle's coverage rule (batch 7 card 30)"; the coverage rule is card 29, and card 30 is `VerbCase.Run`.
**Fix:** Repoint both references to card 23 and card 29.

### [NIT:consistency] Batch 4's Batch Tests contradicts its own verify
**Location:** batch 4, `## Batch Tests` (first sentence vs. the next)
**Issue:** The paragraph quotes `verify: … && go vet -tags integration ./internal/fabricengine/...` and then immediately states "The chained vet is `go vet -tags integration ./...` — module-wide, not the package-scoped form"; the authoritative yaml header and the overview Batch Index both say `./...`.
**Fix:** Correct the quoted verify string to the module-wide form.

## Verdict

REQUEST_CHANGES
Oracle coverage rules and one gate predicate fail on correct behaviour as specified.
MILL_REVIEW_END
