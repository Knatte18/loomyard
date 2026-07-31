# Plan: fabric: fold snapshot-tracking into the Warp-SHA trailer

```yaml
task: 'fabric: fold snapshot-tracking into the Warp-SHA trailer'
slug: fabric-snapshot-trailer
approved: false
started: '20260731T091500Z'
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches. Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: retire-ref-mechanism
    file: 01-retire-ref-mechanism.md
    depends-on: []
    verify: go test -tags integration -count=1 ./internal/gitrepo/... ./cmd/lyx/...
  - number: 2
    name: commit-empty-primitive
    file: 02-commit-empty-primitive.md
    depends-on: [1]
    verify: go test -tags integration -count=1 ./internal/gitrepo/... ./cmd/lyx/...
  - number: 3
    name: snapshot-reader
    file: 03-snapshot-reader.md
    depends-on: []
    verify: go test -tags integration -count=1 -skip 'TestDiff_MergesWarpAndWeftSides|TestStatus_MergesUncommittedChangesBothSides_ExcludesWeftArtifacts' ./internal/fabricengine/...
  - number: 4
    name: empty-commit-rule
    file: 04-empty-commit-rule.md
    depends-on: [2, 3]
    verify: go test -tags integration -count=1 -skip 'TestDiff_MergesWarpAndWeftSides|TestStatus_MergesUncommittedChangesBothSides_ExcludesWeftArtifacts' ./internal/fabricengine/...
  - number: 5
    name: design-docs
    file: 05-design-docs.md
    depends-on: [1, 4]
    verify: go test -count=1 ./internal/fabricengine/... ./internal/gitrepo/... ./cmd/lyx/... && go test -tags integration -count=1 -skip 'TestDiff_MergesWarpAndWeftSides|TestStatus_MergesUncommittedChangesBothSides_ExcludesWeftArtifacts' ./internal/fabricengine/... ./internal/gitrepo/... ./cmd/lyx/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints. One subsection per decision. Batch-local decisions live in each batch file._

### Decision: trailer-is-truth-no-new-cache

- **Decision:** the `Snapshot: <tag>` git trailer on a weft commit is the sole source of truth for a snapshot baseline. The reader scans weft history on demand with one `git log` invocation and builds no index, no cache, and no new state file. `scanWarpSHATrailers` is **generalized** to capture the `Snapshot` field alongside `Warp-SHA`, never duplicated into a sibling scanner.
- **Rationale:** it is the same layering the correspondence index already rests on (trailer is truth, anything on top is a rebuildable cache), and a cache is explicitly optional in that layering. Snapshot reads are rare — a staleness check at a phase boundary, not a hot path — so an index would buy a second cache-invalidation surface for a consumer that does not exist yet. Generalizing keeps one git-log plumbing site, one copy of the `\x1f`/`\x1e` separator convention, and one copy of the unborn-HEAD tolerance.
- **Applies to:** all batches

### Decision: tags-force-a-weft-commit

- **Decision:** when `snapshotTags` is non-empty and no weft commit would otherwise land, `commitWeftLocked` lands an **empty** weft commit carrying the `Warp-SHA` + `Snapshot:` trailers. One rule, four triggering cases, no validation, no typed misuse error, no misuse-handling branch.
- **Rationale:** this single rule dissolves what looked like three separate problems. The unchanged-content case is a genuine correctness hole with no other fix (raddle regenerates against warp SHA X, output is byte-identical, no weft commit lands, so the baseline never advances and staleness reports drift forever). Once an empty commit fixes that, the same mechanism absorbs the caller-misuse cases with no extra code. A snapshot's whole meaning is "this warp SHA, recorded under this tag", and an empty weft commit records exactly that.
- **Applies to:** empty-commit-rule, commit-empty-primitive

### Decision: no-misuse-handling-code

- **Decision:** do not add validation, typed errors, or handling branches whose only job is to cope with a caller using the module incorrectly. Using the module correctly is the caller's obligation.
- **Rationale:** an explicit operator directive for this task. It is why `SnapshotWarpSHA` does not validate its `tag` argument against `snapshotTagPattern` (an unwritable tag simply never matches and reads as absent), why the tags-with-zero-weft-files case gets an empty commit rather than an `*ErrSnapshotNotRecorded`, and why the reader returns a dangling `Warp-SHA` raw instead of an `ErrStaleSHA`. The one place a refusal is still correct is `CommitEmpty`'s dirty-index pre-check — that is not misuse handling, it is refusing to silently commit somebody else's staged work.
- **Applies to:** all batches

### Decision: delete-outright-no-deprecation

- **Decision:** `internal/gitrepo/snapshot.go` and its whole test and documentation surface are deleted outright. No deprecation window, no speculative retention of `remoteName`/`isStrictDescendant`.
- **Rationale:** `SnapshotSHA`/`SetSnapshotSHA` have zero production callers outside `internal/gitrepo` itself, so a deprecation window would serve nobody while leaving two live mechanisms — exactly the duplication this task exists to end. `remoteName` and `isStrictDescendant` are used only by that file's own code paths and die with it.
- **Applies to:** retire-ref-mechanism

### Decision: grep-driven-comment-sweep

- **Decision:** stale comments are found by running greps, not by working down an enumerated list. The enumerations in these plan files are starting points and worked examples, explicitly not complete inventories. The mandatory sweep is `grep -rin snapshot internal/gitrepo/ cmd/lyx/` across production and test source, followed by separate greps for `remoteName` and `isStrictDescendant` (which carry no "snapshot" substring and are invisible to the first pass).
- **Rationale:** six discussion review rounds each found comment sites the previous round's "exhaustive" enumeration had missed; a case-insensitive `snapshot` grep finds 38 mentions in `internal/gitrepo`'s non-test source alone. Enumeration does not converge on this shape of change, and claiming a list is exhaustive gives false confidence.
- **Applies to:** retire-ref-mechanism, commit-empty-primitive

### Decision: same-commit-doc-obligation

- **Decision:** every card lands its documentation in its own commit — module doc (`doc.go`), `CONSTRAINTS.md`, and any godoc the card's change falsifies. The obligation explicitly covers **test-fixture type and field docs**, not just production godoc.
- **Rationale:** CLAUDE.md's Documentation Lifecycle and `CONSTRAINTS.md`'s own rule that a `gitexec` change comes with an updated invariant entry in the same commit. Test-fixture docs are called out because nothing fails to compile when they go stale, which makes them the easiest class to miss.
- **Applies to:** all batches

### Decision: test-tiering

- **Decision:** pure, git-free logic is tested in untagged Tier-1 files; anything needing real git history is `//go:build integration`. Every git-spawning package already has its `TestMain` calling `lyxtest.HermeticGitEnv()` — `internal/fabricengine/testmain_test.go` and `internal/gitrepo/testmain_test.go` both exist, so no new `TestMain` is needed.
- **Rationale:** `CONSTRAINTS.md`'s Test Tier Purity Invariant (an untagged file containing `gitexec.RunGit`, `exec.Command`, or `lyxtest.Copy` as a raw substring fails the guard — including in a comment or string literal) and the Hermetic Git Test Environment Invariant.
- **Applies to:** all batches

### Decision: two-test-packages-in-gitrepo

- **Decision:** `internal/gitrepo` has two test packages and helper names are duplicated across them deliberately. `gogit_test.go`, `keyvalidation_test.go`, and `snapshot_test.go` are `package gitrepo` (internal, can reach unexported identifiers); `oracle_test.go` and `parity_test.go` are `package gitrepo_test` (external). Both packages define their own `oracleCurrentSHA`, `oracleSnapshotSHA`, `oracleRemoteName`, and `oracleIsStrictDescendant`. Deleting one copy does not delete the other.
- **Rationale:** verified by reading the `package` line of each file. `gogit_test.go` must be internal because it calls the unexported `remoteName`/`isStrictDescendant` directly; `parity_test.go` is external and reaches only exported API. An implementer who assumes one copy will leave dead helpers behind in the other file.
- **Applies to:** retire-ref-mechanism

### Decision: known-pre-existing-windows-test-failures

- **Decision:** several packages are already red on this machine **before any change in this task**, and none of them are this task's to fix. Every `internal/fabricengine` verify command in this plan carries `-skip 'TestDiff_MergesWarpAndWeftSides|TestStatus_MergesUncommittedChangesBothSides_ExcludesWeftArtifacts'`, and no verify command in this plan is repo-wide. Do not attempt to fix any of these and do not treat them as regressions.
- **Rationale:** measured on the untouched worktree tip, both tiers. In the **integration** tier, `internal/fabricengine`'s `TestDiff_MergesWarpAndWeftSides` and `TestStatus_MergesUncommittedChangesBothSides_ExcludesWeftArtifacts` both assert with `containsPath(paths, filepath.Join("_lyx", "config.yaml"))`, which builds `_lyx\config.yaml` on Windows while go-git reports the path forward-slashed as `_lyx/config.yaml`, so the containment check can never match on this platform; `internal/traceengine` fails four more (a `.exe`-suffix path assertion, a temp-file locking race, and a `gopls` install needing the network). In the **untagged** tier, `internal/logger`, `internal/tracecli`, and `internal/traceengine` fail, and `internal/proc`'s `TestIsAlive_ExitedProcessIsNotAlive` is flaky. Crucially, the three package trees this task touches — `internal/gitrepo`, `internal/fabricengine`, and `cmd/lyx` — are **green** in the untagged tier and green in the integration tier apart from the two named `fabricengine` tests, so every batch verify in this plan starts from a clean baseline. Fixing any of the rest would widen this task's diff into code it has no other reason to touch, in areas another worktree may own.
- **Consequence the operator must know about:** `pipeline.done_gate` is configured as `go test ./...`, which is red on this machine for the three untagged-tier packages above. mill-go's done gate will therefore fail at the end of this task for reasons that have nothing to do with it. That is a config/environment matter for the operator to decide on — it is deliberately **not** handled inside this plan, because `mill-config.yaml` is a tracked hub-level file and changing it mid-task is exactly what the `wiki-config-mutation` validator check exists to stop.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens)._

- `CONSTRAINTS.md`
- `cmd/lyx/gitrepoboundary_test.go`
- `crucible/fabric-review-prompt.md`
- `crucible/gitrepo-review-prompt.md`
- `internal/fabricengine/commit.go`
- `internal/fabricengine/commit_integration_test.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/index.go`
- `internal/fabricengine/index_test.go`
- `internal/fabricengine/snapshot.go`
- `internal/fabricengine/snapshot_integration_test.go`
- `internal/fabricengine/weftgit.go`
- `internal/gitrepo/commitempty_integration_test.go`
- `internal/gitrepo/doc.go`
- `internal/gitrepo/gitrepo.go`
- `internal/gitrepo/gogit.go`
- `internal/gitrepo/gogit_test.go`
- `internal/gitrepo/keyvalidation_test.go`
- `internal/gitrepo/oracle_test.go`
- `internal/gitrepo/parity_test.go`
- `internal/gitrepo/push.go`
- `manifest/designs/fabric-unified-view.md`
- `manifest/designs/raddle.md`
- `manifest/roadmap.md`
