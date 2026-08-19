# Plan: loom: session bootstrap

```yaml
task: 'loom: session bootstrap'
slug: loom-session-bootstrap
approved: false
started: '20260819-180839'
parent: standalone-producers
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: fabric-origin-record
    file: 01-fabric-origin-record.md
    depends-on: []
    verify: go test ./internal/fabricengine/
  - number: 2
    name: fabric-add-and-launcher
    file: 02-fabric-add-and-launcher.md
    depends-on: [1]
    verify: go test ./internal/fabricengine/ && go test -tags integration ./internal/fabricengine/
  - number: 3
    name: loom-paths-and-seed-sentinel
    file: 03-loom-paths-and-seed-sentinel.md
    depends-on: []
    verify: go test ./internal/loomengine/ ./internal/loomshed/ ./cmd/lyx/
  - number: 4
    name: loomcli-core
    file: 04-loomcli-core.md
    depends-on: []
    verify: go test ./internal/loomcli/
  - number: 5
    name: loomcli-run-bootstrap
    file: 05-loomcli-run-bootstrap.md
    depends-on: [1, 2, 3, 4]
    verify: go test ./internal/loomcli/
  - number: 6
    name: registration-and-guards
    file: 06-registration-and-guards.md
    depends-on: [2, 5]
    verify: go test ./cmd/lyx/ ./internal/loomcli/
  - number: 7
    name: smoke-tests-and-roadmap
    file: 07-smoke-tests-and-roadmap.md
    depends-on: [6]
    verify: go vet -tags smoke ./internal/loomcli/ && go test ./internal/loomcli/
```

## Shared Decisions

_Cross-cutting decisions every batch inherits._

### Decision: no-lyx-directory-literals

- **Decision:** No file this plan touches may write the string `_lyx` or `.lyx` in path-construction context.
  Every such segment comes from `lyxdirs.LyxDirName` / `lyxdirs.DotLyxDirName`.
- **Rationale:** the Lyxdirs Single-Declarer Invariant, machine-enforced by `internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_GeometryLiterals`.
- **Applies to:** all batches

### Decision: path-ownership-stays-with-the-owning-module

- **Decision:** `internal/loomcli` constructs no path of its own.
  Every loom path comes from a `loomengine` accessor; the origin-record path and its anchor-relative form come from `fabricengine` accessors.
  `internal/fabricengine` performs every `AnchorRel` join for both sides of the origin record and for the weft commit pathspec.
- **Rationale:** the Cwd Resolution Invariant — a module's own durable subdirectory is that module's private relative-path constant, and `_lyx/fabric/` is fabric's.
- **Applies to:** all batches

### Decision: told-values-only-into-loomshed

- **Decision:** `internal/loomcli` resolves the `*lyxcwd.Location` once, in its `PersistentPreRunE`, and threads plain absolute strings into `loomshed.Deps`.
  `internal/loomshed` keeps its zero direct production imports of `internal/lyxcwd`.
- **Rationale:** the Told-Geometry Invariant, machine-enforced by `internal/loomshed/seam_enforcement_test.go`'s `TestToldGeometryInvariant_AllowlistOnly`.
- **Applies to:** batches 3, 4, 5

### Decision: one-envelope-and-the-two-handoff-exceptions

- **Decision:** every `RunE` this plan adds checks `clihelp.ShouldAbort(cmd.Context())` first, ahead of its own validation, and emits exactly one `internal/output` envelope.
  Two commands take the CLI/Cobra Invariant's narrow interactive-handoff exception on their tail only: `lyx loom status --watch` (self-displays, then blocks forever) and `lyx loom run` (hands stdio to `tmux attach-session`).
  Everything fallible in both stays pre-flight, on the envelope.
- **Rationale:** `clihelp.Abort` records an exit code but does not stop cobra running `RunE`, so an unguarded `RunE` writes a second envelope on top of the pre-run's.
- **Applies to:** batches 4, 5

### Decision: duplicated-cli-adapters-over-a-cli-to-cli-import

- **Decision:** `internal/loomcli` declares its own `runnerMasterStarter` (12 lines, adapting `*shuttleengine.Runner` to `websterengine.MasterStarter`) and its own `attachArgv` helper, rather than importing `internal/webstercli` or `internal/reedcli`.
- **Rationale:** both are unexported in their home packages, and a `<module>cli` importing another `<module>cli` has no precedent in this repo and would couple two independent cobra seams.
  Two small duplications are cheaper than that coupling.
- **Applies to:** batches 4, 5

### Decision: origin-record-records-both-its-write-and-its-commit

- **Decision:** `fabricengine.WriteOrigin` appends exactly one `KindFileWritten` after the write observably succeeds, and `CommitWeftPaths` takes a `*Mutations` recorder and appends exactly one `KindCommitCreated` when — and only when — it actually landed a commit. `Topology.Add` threads its own recorder through both; a caller with no verb outcome of its own passes a throwaway recorder and discards it.
- **Rationale:** the Mutation Record Invariant exists so a consumer can tell "no error was returned" apart from "something was actually mutated" without parsing prose, and `AddResult` — which embeds `MutationRecord` and is surfaced verbatim in the CLI envelope — would otherwise under-report a real commit this task newly lands on the weft branch.
  Both existing commit sites in the package already record that kind at their own success sites, so recording here is the established pattern rather than a new one.
- **Deviation, stated rather than hidden:** the discussion sketches `CommitWeftPaths` with no recorder parameter.
  This plan adds one.
  The decision's substance — positive-pathspec only, the weft write lock taken inside the helper, and no push — is unchanged; only the recorder is added, for the invariant reason above.
- **The oracle cannot check this half, and the plan does not pretend it can:** the live-state mutation oracle classifies the commit kind as a git-state kind and exempts it from the commission direction entirely, so no matrix cell can catch a missing or spurious commit entry.
  Its only guard is the explicit integration assertion in card 8.
  What the oracle does cover is the write entry: the omission direction is satisfied because the pair's own worktree-creation entries already cover every path under both worktree roots, including the junction-side second path for the same file, and the commission direction because the write entry's target is outside the git metadata directory and does produce a manifest change.
  Card 9 verifies that half against the real oracle and states the exemption for this half.
- **Applies to:** batches 1, 2, 5

### Decision: hub-only-resolution-for-loom

- **Decision:** `internal/loomcli`'s pre-run resolves via `lyxcwd.Resolve`, the way `internal/reedcli` does — never `preflight.ResolveMode`'s degrade-to-standalone path.
- **Rationale:** loom needs a wired fabric (its own Preflight validates warp/weft pairing and sync) and a real weft sibling to commit its seed into, so there is no meaningful standalone mode.
  This is also why `RefMatcher` is `fabricengine.NewRefScanner(loc)` and never `websterengine.NeverMatches`.
- **Applies to:** batches 4, 5

### Decision: go-test-verify-scoping

- **Decision:** each batch's `verify:` names only the packages that batch touches, and any batch editing a `//go:build integration` test file carries `-tags integration` on a second `&&`-chained invocation.
- **Rationale:** Go project, so no `PYTHONPATH=` prefix applies.
  `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) is the repo-wide backstop at Handoff.
- **Applies to:** all batches

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/constructoranchoring_test.go`
- `cmd/lyx/helptree_test.go`
- `cmd/lyx/main.go`
- `cmd/lyx/notransients_test.go`
- `cmd/lyx/sandbox_coverage_test.go`
- `cmd/lyx/seamsignature_test.go`
- `contracts/specs/loom-status-spec.md`
- `docs/overview.md`
- `internal/fabricengine/add.go`
- `internal/fabricengine/commitweftpaths.go`
- `internal/fabricengine/commitweftpaths_test.go`
- `internal/fabricengine/destroy_containment_toctou_integration_test.go`
- `internal/fabricengine/launcher_content_test.go`
- `internal/fabricengine/launchers.go`
- `internal/fabricengine/launchers_containment_integration_test.go`
- `internal/fabricengine/origin.go`
- `internal/fabricengine/origin_integration_test.go`
- `internal/fabricengine/origin_test.go`
- `internal/fabricengine/portallauncher_test.go`
- `internal/loomcli/bootstrap.go`
- `internal/loomcli/bootstrap_test.go`
- `internal/loomcli/cli.go`
- `internal/loomcli/cli_test.go`
- `internal/loomcli/drive.go`
- `internal/loomcli/pause.go`
- `internal/loomcli/run.go`
- `internal/loomcli/seedinput.go`
- `internal/loomcli/seedinput_test.go`
- `internal/loomcli/smoke_test.go`
- `internal/loomcli/status.go`
- `internal/loomcli/status_test.go`
- `internal/loomcli/testmain_test.go`
- `internal/loomcli/wiring.go`
- `internal/loomcli/wiring_test.go`
- `internal/loomengine/config.go`
- `internal/loomengine/config_test.go`
- `internal/loomshed/seed.go`
- `internal/loomshed/seed_test.go`
- `manifest/designs/loom.md`
- `manifest/roadmap.md`
- `tools/sandbox/SANDBOX-CORE-SUITE.md`
