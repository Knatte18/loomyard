# Batch: typed Healthy reason and Clean reword

```yaml
task: 'fabric: close the weft-visibility leak (slice 8)'
batch: 'typed Healthy reason and Clean reword'
number: 2
cards: 2
verify: go test -tags integration ./internal/fabricengine/ ./internal/loomengine/
depends-on: [1]
```

## Batch Scope

The task's only behavioural-surface change: `Healthy`'s reason becomes typed (`HealthReason`), `loomengine/preflight.go` switches on the typed cause instead of substring-matching, preflight adopts `Ready(l)`, loomengine's `CheckID`s rename, and `Clean`'s reason strings reword.
This is one batch because `Healthy`'s signature change breaks `loomengine` and five `package fabricengine_test` files in the same compile unit — card 5 is deliberately atomic.
Classification outcomes must stay provably identical (same `CheckID` for the same underlying condition);
the equivalence tests in card 5 are the safety net.
External interface for later batches: `Healthy(l) (bool, HealthReason, error)`, `HealthCause` constants, `loomengine.CheckFabricReady`/`CheckFabricSync`.

## Cards

### Card 5: `HealthReason` + preflight typed switch + `Ready` adoption + `CheckID` renames

- **Context:**
  - `internal/fabricengine/ready.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/config.go`
  - `internal/fabricengine/junction.go`
  - `internal/lyxtest/lyxtest.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/fabricengine/drift.go`
  - `internal/loomengine/preflight.go`
  - `internal/loomengine/report.go`
  - `internal/loomengine/status.go`
  - `internal/loomengine/preflight_integration_test.go`
  - `internal/fabricengine/junction_pattern_integration_test.go`
  - `internal/fabricengine/reconcile_stale_removal_test.go`
  - `internal/fabricengine/config_driven_junctions_integration_test.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/boardjunction_integration_test.go`
- **Creates:**
  - `internal/fabricengine/healthreason_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Per decision `healthy-typed-reason`, all parts in ONE commit (the signature change is atomic across both packages):
  (a) In `drift.go`: add `type HealthReason struct { Cause HealthCause; Detail string }` and `type HealthCause string` with the five constants `CauseBranchMismatch`, `CauseConfigLoadFailed`, `CauseJunctionMissing`, `CauseNotAJunction`, `CauseJunctionPointsElsewhere`.
  Change `Healthy(l *lyxcwd.Location) (ok bool, reason string, err error)` to return `(ok bool, reason HealthReason, err error)`;
  ok path returns the zero `HealthReason{}`.
  Replace the five reason strings exactly per the discussion's replacement table: branch mismatch `"fabric out of sync: on %s (want %s)"` (second branch name deliberately dropped);
  config load `"junction check unavailable: cannot load fabric.yaml: %v"`;
  `"%s junction missing"`;
  `"%s is not a junction"`;
  `"%s junction points elsewhere"`.
  The config-load failure stays a Cause, not a promoted error return.
  Two `drift.go` comments become factually false and must be corrected in this same card (a staleness fix, not a vocabulary exemption — `drift.go` keeps warp/weft words as an owner file, but it may not keep a wrong statement): `Healthy`'s doc comment at `:18-25` documents `Returns (true, "", nil)` and must describe the `HealthReason` return instead;
  and the note at `:61-66` ending "the reason string must keep the substring `junction`" documents a dependency this card removes and is deleted.
  (b) In `loomengine/preflight.go`: replace the `strings.HasPrefix(reason, "host on ")` / `strings.Contains(reason, "junction")` classification at lines 117-141 with a switch on `reason.Cause` — `CauseBranchMismatch` → `CheckFabricSync`;
  the other four causes → `CheckJunction` with `check3BlocksSeed` set;
  `report.addFailure` prints `reason.Detail` verbatim.
  Remove the now-dead comment warning that rewording the reason strings breaks classification.
  Replace `preflight.go:105`'s `os.Stat(fabricengine.WeftWorktree(l))` with `fabricengine.Ready(l)`: absence records the check failure and sets `check3BlocksSeed` exactly as today;
  a non-nil error hard-returns `Report{}, err` as `preflight.go:106-108` does today.
  Reason string `"weft not paired"` → `"fabric not ready"`.
  (c) In `report.go`: rename `CheckWeftPairing` → `CheckFabricReady` (value `"weft-pairing"` → `"fabric-ready"`) and `CheckWeftSync` → `CheckFabricSync` (value `"weft-sync"` → `"fabric-sync"`).
  Grep confirmed the only production consumers of the old values are `report.go` and `preflight.go` themselves;
  re-verify with `grep -rn '"weft-pairing"\|"weft-sync"' internal cmd` before committing (they are strings — the compiler cannot find stragglers).
  (d) Migrate the five `package fabricengine_test` files that read `Healthy`'s third return as a string: `junction_pattern_integration_test.go:417-426` asserts the typed `Cause` (and `Detail` where the junction name matters);
  `reconcile_stale_removal_test.go:343-351` and `config_driven_junctions_integration_test.go:120-125` replace `strings.Contains(reason, "unavailable")` with `Cause != CauseConfigLoadFailed`;
  `reconcile_stale_registration_test.go:487` and `boardjunction_integration_test.go:162-167` format `reason.Detail` in failure messages, and `boardjunction_integration_test.go:167`'s `(true, "")` assertion becomes `(true, HealthReason{})`.
  (e) New `healthreason_integration_test.go` (`//go:build integration`): one case per cause asserting the typed value — all five, config-load failure included.
  (f) In `preflight_integration_test.go`: equivalence tests asserting each of the five causes maps to the same `CheckID` as today — branch mismatch → `CheckFabricSync`, the other four individually → `CheckJunction` with `check3BlocksSeed` — enumerated per cause, never one "junction-ish" case;
  line 310's sibling-worktree removal keeps driving the not-present branch via the lyxtest fixture field, not `fabricengine.WeftWorktree`.
  (g) Reword every `weft`/`warp`/fabric-sense-`host` comment in the loomengine files this card edits (`preflight.go` ~12, `report.go` ~8 including `report.go:21`'s "host worktree" → "the worktree", `status.go` 1) per decision `comment-fidelity`;
  `drift.go`'s comments are owner-package and keep their vocabulary.
- **Commit:** `refactor(fabricengine,loomengine): typed HealthReason, Ready adoption, fabric-named CheckIDs`

### Card 6: `Clean` reason reword

- **Context:**
  - `internal/loomengine/preflight.go`
  - `internal/lyxtest/lyxtest.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/fabricengine/hostclean.go`
- **Creates:**
  - `internal/fabricengine/cleanreason_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Per decision `clean-reworded-reason`: `Clean` keeps its `(ok bool, reason string, err error)` signature — only the wording changes, because nothing branches on the string (loomengine passes it straight to `report.addFailure` under `CheckWorktreeClean`).
  `hostclean.go:38-43`'s `"host: %s"` / `"weft: %s"` sides become `"uncommitted code changes: %s"` and "uncommitted state changes under `_lyx`: %s" (backticks literal in the Go string), still joined with `"; "` when both sides are dirty.
  New `cleanreason_integration_test.go` (`//go:build integration`, fixture-backed): three shapes — code-side only, state-side only, and both (the joined case `hostclean.go:44-47` produces).
  No existing test covers this string;
  loomengine prints it verbatim to an operator, so all three shapes are asserted.
  `hostclean.go`'s own comments are owner-package and keep their vocabulary.
- **Commit:** `refactor(fabricengine): reword Clean reasons as code-side/state-side`

## Batch Tests

`verify:` runs `go test -tags integration ./internal/fabricengine/ ./internal/loomengine/` — covers the new `healthreason_integration_test.go` and `cleanreason_integration_test.go`, the five migrated `fabricengine_test` files, and the loomengine equivalence tests in `preflight_integration_test.go`.
The equivalence tests are the gate for the task's only behavioural-surface change: same underlying condition → same `CheckID` as before this batch.
