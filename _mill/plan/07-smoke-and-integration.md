# Batch: smoke-and-integration

```yaml
task: 'Seeded driver choice: ly-drive strand as the child''s driver'
batch: smoke-and-integration
number: 7
cards: 2
verify: go build ./... && go test -tags smoke ./internal/loomcli/... && go test -tags integration ./internal/loomcli/...
depends-on: [5]
```

## Batch Scope

This batch proves the two properties no Tier 1 test can reach: that a real reed session ends up with exactly one driver strand across repeated bootstraps, and that an llm bootstrap returns without waiting for its driver.
It is one batch because both exercise the same live substrate through the same stubbed provider binary, and because both depend on the whole path being open — which is why it sits after the refusal-lifting batch rather than beside the branch that introduced it.

Both cards run against a **stubbed provider binary**, never a real one.
Exercising the real thing end to end spawns a live, billed Claude session that runs for as long as the task takes, which is the reason the sandbox suite deliberately does not script this path at all; batch 8 records that disposition in prose.

Batch-local decision beyond the overview's: the re-entrancy property is asserted by **counting strands under the driver name**, not by inspecting the spawn predicate.
The predicate is already under test at Tier 1 with a four-row truth table; what those rows cannot prove is that reed, which has no upsert semantics on add, really ends up with one strand rather than two when the predicate says do-not-spawn on a second invocation.

## Cards

### Card 22: the smoke re-entrancy property

- **Context:**
  - `internal/loomcli/smoke_operatorstrand_test.go`
  - `internal/loomcli/bootstrap.go`
  - `internal/loomcli/start.go`
  - `internal/shedrun/seed.go`
  - `internal/reedengine/lifecycle.go`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/smoke_driverstrand_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/loomcli/smoke_driverstrand_test.go`, build-tagged for the smoke tier, covering three successive bootstraps of a worktree seeded with the llm driver, against a stubbed provider binary and with the terminal handover suppressed.
  Copy the strand-counting-by-display-name shape from `internal/loomcli/smoke_operatorstrand_test.go`, which is this package's existing model for exactly this kind of assertion.
  The three cases run in order against one worktree and each asserts the count of strands carrying the driver name.
  A first bootstrap leaves exactly one.
  A second, while the first driver's pane is still alive, leaves exactly **one** — this is the re-entrancy property, and it is the assertion no Tier 1 test can prove, because the predicate saying do-not-spawn and reed actually holding one strand are different facts.
  A third, after the stub's pane has exited, leaves exactly one again and **not** a corpse plus a live pane — which is the corpse-removal path, and the count is what distinguishes removing the dead strand before relaunching from simply adding a second one beside it.
  Drive the stub so its pane exits promptly in the third case rather than by waiting out a real timeout, and let the third bootstrap follow the second closely: a relaunch inside one second is the case the report path's random component exists for, and this test is where that collision would surface as a refused relaunch.
  Give the test package the hermetic git environment in its own entry point if this tier does not already have one in this package, per the Hermetic Git Test Environment Invariant.
  Never re-exec the test binary as the provider: the stub is a separate script or binary the test writes and points the engine at, per the Live-Substrate Spawn Observability invariant's clause on that.
- **Commit:** `test(loomcli): smoke the driver strand's re-entrancy across bootstraps`

### Card 23: the integration bootstrap

- **Context:**
  - `internal/loomcli/start.go`
  - `internal/loomcli/driverreport.go`
  - `internal/hubforge/hub.go`
  - `internal/shedrun/paths.go`
  - `internal/shedrun/seed.go`
  - `internal/shuttleengine/run.go`
  - `internal/loomcli/testmain_test.go`
  - `internal/loomcli/wiring_commitstatus_integration_test.go`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/integration_driverbootstrap_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/loomcli/integration_driverbootstrap_test.go`, build-tagged for the integration tier, covering one end-to-end llm bootstrap over a fixture hub built through the hub forge — never a hand-assembled hub, per the hubforge Fabric-Fixture Invariant.
  Seed the fixture worktree's own run with the llm driver, run the bootstrap with the terminal handover suppressed, and assert three things.
  The bootstrap **returns** rather than blocking: that is what makes an llm-driven child watchable by its parent at all, and it is the property the whole composition rests on, since the driver run is an ordinary shuttle run in every respect but one — the bootstrap never waits on it.
  The run's persisted state file exists under the run directory the handle reports.
  The report file exists at the path the spec named, written by a stubbed driver that writes one and exits.
  Assert the report's path sits under the run's ephemeral scratch directory rather than its durable one — the report is a per-machine record of one session's own narration, and the durable truth about the run is the status file beside it.
  Add no test entry point of your own: this package's existing one is untagged, so it compiles into the integration build alongside the tagged files and already supplies the hermetic git environment that tier needs — `internal/loomcli/wiring_commitstatus_integration_test.go` is the existing integration-tagged file relying on exactly that, and is the model for how this tier is already wired here.
  Do not assert anything about the driver run's deadline: nothing on this path reads it.
- **Commit:** `test(loomcli): cover the llm bootstrap end to end`

## Batch Tests

`verify: go build ./... && go test -tags smoke ./internal/loomcli/... && go test -tags integration ./internal/loomcli/...` runs both tagged tiers of the one package this batch adds files to, plus a whole-module build.
Neither tier is reached by an untagged run, so both tags are named explicitly; a verify listing only the untagged suite would compile neither new file and would pass against an empty batch.

The second and third smoke cases are the load-bearing ones and the reason this batch exists at all.
The first case — one bootstrap leaves one strand — passes against an implementation with no re-entrancy handling whatsoever.
The second is the only assertion anywhere in this task that a do-not-spawn verdict really produces one strand in reed rather than two, and the third is the only one that distinguishes corpse removal from a second add beside a dead entry, which is a distinction reed's own add cannot make for us because it has no upsert semantics.

The integration test's returns-rather-than-blocks assertion is load-bearing in a different way: every other test in this task drives the branch through a seam that returns immediately by construction, so none of them can catch a change that started waiting on the driver.
A bootstrap that waited would look correct in isolation and would deadlock every parent run that spawns an llm-driven child.
