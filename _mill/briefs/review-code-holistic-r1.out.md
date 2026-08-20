MILL_REVIEW_BEGIN
# Review: preflight: split into two Shed rows -- a generic one, and loom's own — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-08-20
```

## Findings

### [BLOCKING:scope] fabricengine doc comments still describe the deleted composite
**Location:** `internal/fabricengine/doc.go:142` and `internal/fabricengine/drift.go:3`
**Issue:** Card 28's sweep was scoped to a grep for the literal string `loomengine.Preflight` and fixed exactly four sites, but two further production comments describe the same now-deleted mechanism under a different phrase and were missed: `doc.go:142` says a missing junction makes "loom's preflight fail `CheckJunction`, set its `check3BlocksSeed` flag, and block the run" — `check3BlocksSeed` was `runCheck4`'s internal derivation, deleted whole with `internal/loomengine/preflight.go` in card 24, and no such flag exists anywhere in the codebase post-task. `drift.go:3` says "Healthy and Clean (warpclean.go) are wired into the loom preflight via internal/loomengine" — after the split, `Healthy`/`Clean` are wired into `internal/preflight.CheckResolved`, consumed by `internal/preflightshed`, and `internal/loomengine` no longer calls either function at all.
**Fix:** Reword both to name `internal/preflight`/`preflightshed` as the actual wiring point and drop the `check3BlocksSeed` reference, the same way card 28's four sites were repointed at `preflight.CheckResolved`.

### [NIT:consistency] Migrated junction test keeps the source package's test name
**Location:** `internal/preflight/preflight_integration_test.go:266` (`func TestPreflight_JunctionBroken`)
**Issue:** Every other test in this file follows the `TestCheckResolved_*`/`TestCheck_*`/`TestPredicates_*`/`TestResolveMode` naming already established here (including the two tests card 22 migrated in the same batch, `TestCheckResolved_ConfigLoadFailed` and `TestCheckResolved_MissingOptionalJunctionIsAJunctionFault`). This one test instead kept the deleted `internal/loomengine` suite's own name, `TestPreflight_JunctionBroken`, which card 21 cited only as the shape to restructure into, not a name to carry over.
**Fix:** Rename to `TestCheckResolved_BrokenJunction` (the name card 21 itself uses to refer to this test) to match the file's own convention.

## Verdict

REQUEST_CHANGES
Two stale fabricengine doc comments describe deleted preflight mechanics; a migrated test also keeps the wrong name.
MILL_REVIEW_END
