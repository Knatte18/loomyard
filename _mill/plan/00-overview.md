# Plan: loom's status file can conflict on the landing merge

```yaml
task: "loom's status file can conflict on the landing merge"
slug: "loom-status-file-merge-conflict"
approved: true
started: "20260826-175733"
parent: "main"
root: ""
verify: go vet ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: untrack-status-file
    file: 01-untrack-status-file.md
    depends-on: []
    verify: go test ./cmd/lyx/... ./internal/loomengine/... ./internal/loomcli/... ./internal/landingshed/... ./internal/loomshed/... && go vet -tags smoke ./internal/loomcli/...
  - number: 2
    name: text-references
    file: 02-text-references.md
    depends-on: [1]
    verify: go test ./contracts/... ./internal/loomengine/... ./internal/shedengine/... ./internal/loomshed/... ./internal/lyxcwd/...
  - number: 3
    name: docs
    file: 03-docs.md
    depends-on: [1]
    verify: go test ./internal/lyxcwd/... ./cmd/lyx/...
  - number: 4
    name: regression-coverage
    file: 04-regression-coverage.md
    depends-on: [1]
    verify: go test -tags integration ./internal/loomcli/...
```

## Shared Decisions

### Decision: the destination path is `.lyx/loom/status.json`, built from `lyxdirs.DotLyxDirName`

- **Decision:** `loomengine.LoomStatusFile` re-roots from `lyxdirs.LyxDirName` to `lyxdirs.DotLyxDirName`, keeping the `loomDirName` + `loomStatusFileName` tail unchanged, so the status file lands beside `LoomStatusLock`, `LoomRunLock`, `LoomDriverLog`, and `LoomBootstrapLock` — all of which already resolve under `.lyx/loom/`.
  The two segment constants stay declared in `internal/loomengine/config.go` and move nowhere.
- **Rationale:** `CONSTRAINTS.md`'s Durable-vs-Ephemeral State Invariant puts every never-tracked file under `.lyx` at the mirrored subpath, its Lyxdirs Single-Declarer Invariant forbids a hand-built `.lyx` literal, and its Cwd Resolution Invariant keeps the module's own subpath constants in the module.
  All three are satisfied by re-rooting the one constructor.
- **Applies to:** all batches

### Decision: after the move `_lyx/loom/` ceases to exist, and that is conforming

- **Decision:** no `_lyx`-rooted path under `loom/` remains, and no placeholder is created to keep the mirrored pair symmetric.
- **Rationale:** the mirrored-subpath rule constrains where a never-tracked file lives, not whether tracked content must exist beside it, so it is satisfied vacuously.
  `loomengine` still exposes a durable `_lyx` path in `DiscussionDir`, so the module is not leaving the `_lyx` tree — only its `loom/` subdirectory is.
- **Applies to:** all batches

### Decision: directory creation is already covered; no new `MkdirAll` is needed

- **Decision:** neither `loomshed.Seed` nor any caller gains a directory-creation step for the new location.
- **Rationale:** `internal/state`'s `UpdateJSON` already `MkdirAll`s the status file's own parent, and `loomshed.Seed` already `MkdirAll`s the status *lock*'s parent before calling it.
  After the move both parents are the same `.lyx/loom/` directory, so the existing pair of calls covers the new path without change — the seed at `lyx loom run` step 2 no longer depends on step 4's bootstrap-lock `MkdirAll` running first.
- **Applies to:** untrack-status-file

### Decision: the smoke suite's seed helper drops its commit entirely rather than committing the origin record

- **Decision:** `seedAndCommitStatus` in `internal/loomcli/smoke_test.go` becomes a seed-only helper, and `TestSmokeBootstrap_CleanlinessOrderingAfterSeedCommit` asserts the pair is clean after the seed with the weft commit count **unchanged**, rather than incremented by one commit whose changed-file set is the origin record.
- **Rationale:** substituting `fabricengine.OriginRecordRel()` for the removed status pathspec would assert a commit that never happens — `fabricengine`'s own `Topology.Add` already commits the origin record at add.go step 10c, so a second commit of that already-clean, already-tracked path is a no-op (`committed == false`) and produces no new commit for the count or changed-file assertions to observe.
  The seed-only shape also states the stronger post-move fact directly: seeding now writes solely under `.lyx`, which is structurally never committed, so the pair stays clean with no commit needed at all.
- **Applies to:** untrack-status-file

### Decision: `shedengine`'s caller obligation stops naming a side, rather than flipping to "ephemeral"

- **Decision:** `internal/shedengine/doc.go`'s "Told, never derived" paragraph and `internal/shedengine/shed.go`'s `StatusPath` field comment stop asserting which side of the durable/ephemeral line a caller's status file must land on.
- **Rationale:** `Shed` is generic and told its paths, so the obligation it states is the caller's.
  Flipping the assertion to "ephemeral" would mislead the next product the same way in the other direction.
- **Applies to:** text-references

### Decision: the regression test lives in `internal/loomcli`, integration-tagged

- **Decision:** the two-sequential-landings regression test is a new integration-tagged file in package `loomcli`, not an addition to `internal/landingshed`'s or `internal/fabricengine`'s own integration files.
- **Rationale:** the test needs both loom's own status-file path and `landingshed.Finalize`, and `internal/loomcli` is the one layer that legitimately imports both — it already imports `loomengine`, `loomshed`, `landingshed`, `fabricengine`, and `hubforge`.
  Placing it in `internal/landingshed` would make a generic landing package's test depend on one product's path constructors.
  Package `loomcli`'s existing untagged `TestMain` in `internal/loomcli/testmain_test.go` already calls `gitkit.HermeticGitEnv()`, so the new file needs no `TestMain` of its own and satisfies the Hermetic Git Test Environment Invariant.
- **Applies to:** regression-coverage

### Decision: `pipeline.done_gate` is left as configured

- **Decision:** no change to `pipeline.done_gate`, which is already `go test ./... && go test -tags integration ./...`.
- **Rationale:** the repo-wide test command already covers both tiers this task touches.
  `golangci-lint` is not installed on this machine, so appending it would make the gate fail on tooling absence rather than on code.
- **Applies to:** all batches

### Decision: `manifest/roadmap.md` and `CONSTRAINTS.md` do not move

- **Decision:** neither file is edited.
- **Rationale:** this is a bugfix, not a planned roadmap item completing or being added, per the task-completion rule.
  No new cross-cutting invariant is created — the existing Durable-vs-Ephemeral State Invariant already covers and machine-enforces the destination.
- **Applies to:** all batches

## All Files Touched

- `cmd/lyx/constructoranchoring_test.go`
- `cmd/lyx/notransients_test.go`
- `contracts/recipes/loom-recipe.yaml`
- `contracts/specs/loom-status-spec.md`
- `contracts/stencils/discussiontemplate_test.go`
- `contracts/stencils/loom/loom-rubric-webster-review.md`
- `contracts/stencils/loom/loom-template-discussion.md`
- `docs/overview.md`
- `internal/fabricengine/commitweftpaths.go`
- `internal/landingshed/deps.go`
- `internal/landingshed/finalize.go`
- `internal/landingshed/publish.go`
- `internal/loomcli/landing_integration_test.go`
- `internal/loomcli/landingdeps.go`
- `internal/loomcli/landingdeps_test.go`
- `internal/loomcli/run.go`
- `internal/loomcli/smoke_test.go`
- `internal/loomengine/config.go`
- `internal/loomengine/config_test.go`
- `internal/loomengine/loomstatus_test.go`
- `internal/loomengine/report.go`
- `internal/loomengine/status.go`
- `internal/loomshed/seed.go`
- `internal/shedengine/doc.go`
- `internal/shedengine/shed.go`
- `manifest/designs/fabric-unified-view.md`
- `manifest/designs/loom.md`
- `manifest/designs/self-report.md`
- `manifest/designs/shed.md`
- `tools/sandbox/SANDBOX-CORE-SUITE.md`
