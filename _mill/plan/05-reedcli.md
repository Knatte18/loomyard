# Batch: reedcli

```yaml
task: 'lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency'
batch: 'reedcli'
number: 5
cards: 7
verify: go vet -tags integration ./... && go vet -tags smoke ./... && go test -tags integration ./internal/reedcli/...
depends-on: [4]
```

## Batch Scope

This batch migrates `internal/reedcli`'s twenty `Copy*` sites and twenty-one `SeedConfig` sites — the largest single-package block outside `fabricengine`, and the one the discussion names as the clearest stand-in-hub offender: `cli_integration_test.go` does `CopyPaired` → `SeedConfig(fixture.Hub)` → `t.Chdir(fixture.Hub)`, chdir-ing into a directory that is not a hub at all.
`internal/reedcli` sits outside `internal/fabriccli`'s dependency set, so its in-package test files import `hubforge` directly with no file moves.

Batch-local decision: six of the seven files are `//go:build smoke` and are compile-checked but never executed here, so only four of the twenty sites get runtime proof in this batch.
That is called out in `## Batch Tests` rather than papered over.
It is also why `cli_integration_test.go` is migrated first, in its own card: it is the one file whose migration this batch can actually prove.

## Cards

### Card 28: Migrate reedcli's integration suite

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/gitkit/gitkit.go`
- **Edits:**
  - `internal/reedcli/cli_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace all four `gitkit.CopyPaired(t)` calls with `hubforge.NewHub(t, ".")` and retarget the fixture fields per the overview's mapping table.
  The `t.Chdir(fixture.Hub)` calls become `t.Chdir(h.PrimeWorktree())`: the old fixture's `Hub` field was a git repo standing in for a hub, whereas `h.Path` is the `<name>-HUB` container and is not a repo, so chdir-ing there would resolve a different `lyxcwd.Location` than the test intends.
  Keep the `t.Chdir` calls themselves — removing them is the follow-up task `hubforge-parallel-chdir`, not this one.
  Apply the `SeedConfig` triage to all four `gitkit.SeedConfig(t, fixture.Hub, …)` calls: every one of them today writes config into a directory no production code reads from, so none can survive unchanged.
  Each becomes either a deletion (when the YAML is the module's registered template, already materialized by `fabriccli.CloneAndWire`) or `hubforge.SeedConfig(t, h, …)`.
  This card is where the pattern for the six smoke files is settled;
  run it green before starting card 29.
- **Commit:** `test(reedcli): build the integration fixtures with hubforge.NewHub`

### Card 29: Migrate smoke_lifecycle_test.go

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/reedcli/cli_integration_test.go`
- **Edits:**
  - `internal/reedcli/smoke_lifecycle_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace all six `gitkit.CopyPaired(t)` calls with `hubforge.NewHub(t, ".")`, retarget the fixture fields per the overview's mapping table, and apply the `SeedConfig` triage to all six `gitkit.SeedConfig(` calls, following exactly the resolution card 28 reached for the same shapes.
  This file is `//go:build smoke` and is compile-checked only.
- **Commit:** `test(reedcli): build the lifecycle smoke fixtures with hubforge.NewHub`

### Card 30: Migrate smoke_teardown_test.go

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/reedcli/cli_integration_test.go`
- **Edits:**
  - `internal/reedcli/smoke_teardown_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace all five `gitkit.CopyPaired(t)` calls with `hubforge.NewHub(t, ".")`, retarget the fixture fields per the overview's mapping table, and apply the `SeedConfig` triage to all five `gitkit.SeedConfig(` calls.
  This file asserts on teardown of reed's own strands, not on hub teardown;
  `hubforge`'s junction removal runs as a `tb.Cleanup` after the test body, so it cannot interfere with what this file observes — but any assertion here that counts files or lists directories under the fixture root will move, because a real hub is ~155 files against the old template's ~36.
  Re-express such assertions against the real shape rather than deleting them.
  This file is `//go:build smoke` and is compile-checked only.
- **Commit:** `test(reedcli): build the teardown smoke fixtures with hubforge.NewHub`

### Card 31: Migrate smoke_debuglog_test.go and smoke_resume_test.go

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/reedcli/cli_integration_test.go`
- **Edits:**
  - `internal/reedcli/smoke_debuglog_test.go`
  - `internal/reedcli/smoke_resume_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace the two `gitkit.CopyPaired(t)` calls in each file with `hubforge.NewHub(t, ".")`, retarget the fixture fields per the overview's mapping table, and apply the `SeedConfig` triage to the two `gitkit.SeedConfig(` calls in each.
  Both files are `//go:build smoke` and are compile-checked only.
- **Commit:** `test(reedcli): build the debuglog and resume smoke fixtures with hubforge.NewHub`

### Card 32: Migrate smoke_attach_test.go

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/reedcli/cli_integration_test.go`
- **Edits:**
  - `internal/reedcli/smoke_attach_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Replace the single `gitkit.CopyPaired(t)` call with `hubforge.NewHub(t, ".")`, retarget the fixture fields per the overview's mapping table, and apply the `SeedConfig` triage to the single `gitkit.SeedConfig(` call.
  This file is `//go:build smoke` and is compile-checked only.
- **Commit:** `test(reedcli): build the attach smoke fixture with hubforge.NewHub`

### Card 33: Migrate smoke_test.go's seeding

- **Context:**
  - `internal/hubforge/hub.go`
  - `internal/hubforge/seed.go`
  - `internal/gitkit/gitkit.go`
- **Edits:**
  - `internal/reedcli/smoke_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  This file has no `Copy*` call — it has one `gitkit.SeedConfig(` call and three `gitkit.MustRun(` calls.
  Apply the `SeedConfig` triage to the seeding call.
  Whether it becomes a deletion, a `hubforge.SeedConfig` or stays on `gitkit.SeedConfig` depends on what directory it is handed: if the base is a `Hub` obtained from a helper in another file in this package, it retargets to `hubforge.SeedConfig(t, h, …)`;
  if the base is a plain repo this file builds itself with `MustRun`, `gitkit.SeedConfig` remains correct and the call stays exactly as it is.
  The three `gitkit.MustRun(` calls stay on `gitkit` unchanged in either case.
  This file is `//go:build smoke` and is compile-checked only.
- **Commit:** `test(reedcli): triage smoke_test.go's config seeding`

### Card 34: Confirm reedcli holds no stand-in hub

- **Context:**
  - `internal/reedcli/cli_integration_test.go`
  - `internal/reedcli/smoke_attach_test.go`
  - `internal/reedcli/smoke_debuglog_test.go`
  - `internal/reedcli/smoke_lifecycle_test.go`
  - `internal/reedcli/smoke_resume_test.go`
  - `internal/reedcli/smoke_teardown_test.go`
  - `internal/reedcli/smoke_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Verification-only gate, no diff.
  Confirm by grep that `internal/reedcli` contains zero occurrences of `gitkit.CopyPaired`, `gitkit.CopyPairedLocal`, `gitkit.CopyWeft` and `gitkit.CopyWarpHub`, and that every surviving `gitkit.SeedConfig(` call in the package is handed a plain repo this package builds itself rather than a hub accessor.
  Confirm that no `t.Chdir` call in the package was removed and none gained a `t.Parallel` neighbour — both are out of scope and belong to the follow-up task `hubforge-parallel-chdir`.
  If any check fails, fix it under the card that owns the file rather than here.
- **Commit:** none

## Batch Tests

`verify:` compile-checks the repo under both tags, then runs `internal/reedcli`'s integration suite — which covers exactly one of the seven migrated files, `cli_integration_test.go`, and four of the twenty migrated `Copy*` sites.

The other sixteen sites live in `//go:build smoke` files that spawn live tmux sessions and LLM agents, so they are compile-checked under `go vet -tags smoke ./...` and never executed.
This is the batch's honest coverage limit: sixteen call sites get type-checked, not run, and the repo-wide `done_gate` does not close the gap either, since it runs no `smoke`-tagged tests.
Card 34's grep gate is what stands in for runtime proof that no stand-in-hub shape survives in this package.
