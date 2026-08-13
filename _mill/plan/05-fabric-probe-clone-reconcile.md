# Batch: fabric-probe-clone-reconcile

```yaml
task: 'gitexec: add the checked entry point and migrate the call sites'
batch: fabric-probe-clone-reconcile
number: 5
cards: 4
verify: go build ./... && go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
depends-on: [4]
```

## Batch Scope

This batch migrates nineteen sites across the probe, clone, board-weft, and reconcile cluster: `warpprobe.go` (4), `clone.go` (6), `boardweft.go` (3), `reconcile.go` (6).
They belong together because two of the migration's three structural sub-changes live here — the `wrapProbeError` helper re-signature and `readBranch`'s prior-call-diagnostic composition — and both are read most easily beside the mixed `rev-parse` probes in the same files.
It depends on batch 4 only to keep the `internal/fabricengine` migration a single serial chain;
it shares no file with batch 4.

Batch-local decisions beyond `## Shared Decisions`:

- `wrapProbeError` keeps its `op` parameter and loses its `stderr` parameter.
  Feeding it `err.Error()` as the `stderr` argument would keep a parameter whose reason for existing is gone and stringify an error callers may want to `errors.As`.
- The mixed `rev-parse` probes in this batch's files take the checked form with `errors.As`, not the raw form.
  The design doc's classifier filed all eight `rev-parse` sites as pure predicates by looking only at `exitCode == 0`;
  the code shows each of these already separates an error-returning exec path from an answer-returning exit path, so a raw marker at any of them would have to claim "every exit code here is an answer", which is false.
- `readBranch`'s two downstream messages each cite the **earlier** `rev-parse --abbrev-ref HEAD` call's exit code — not one message, two.
  Both keep it, filled from the first call's recovered `*GitError`.

## Cards

### Card 23: re-signature wrapProbeError and migrate warpprobe.go

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/fabricengine/warpbinding_clone_integration_test.go`
- **Edits:**
  - `internal/fabricengine/warpprobe.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `wrapProbeError(weftURL, op, stderr string, cause error) error` to `wrapProbeError(weftURL, op string, cause error) error` and delete its internal stderr-vs-cause selection branch.
  The migrated body returns `fmt.Errorf("probe weft %s: %w", weftURL, cause)` when `cause` is non-nil, and `fmt.Errorf("probe weft %s: git %s failed", weftURL, op)` when it is nil.
  After migration every one of the seven call paths invokes the helper from inside an `err != nil` branch, so the nil-cause arm is unreachable by construction and `op` is used only there.
  Keep both, and say so in the godoc: the arm is deliberately defensive, guarding a future caller that reaches the helper without an error, and it is what keeps `op` meaningful rather than a parameter with no reader.
  Do not silently leave it looking like a live branch, and do not drop `op` from the signature to remove it — the two-parameter form is the shape this task settled on.
  Update its godoc, which currently describes choosing between git's trimmed stderr and a fallback naming the subcommand.
  Migrate all four `gitexec.RunGit` sites in `probeWeftBinding` and `probeTreeHasPath` to `gitexec.Run`.
  Six of the seven call paths into `wrapProbeError` are exec-path/exit-path pairs — the `clone`, `show`, and `ls-tree` sites — and each pair collapses into one call passing the single error.
  The seventh path is the unborn-HEAD `rev-parse --verify --quiet HEAD` check, which is a mixed probe: its exit path returns `warpProbeResult{Found: false, WeftLooksLikeWeft: true}` with a nil error while its exec path wraps a real error, so it takes `var gitErr *gitexec.GitError` recovery — `errors.As` succeeds means the unborn-HEAD answer, anything else means `wrapProbeError(weftURL, "rev-parse HEAD", err)`.
  The `"probe weft "` prefix that `internal/fabricengine/warpbinding_clone_integration_test.go` asserts on is preserved by this shape;
  do not edit that test.
- **Commit:** `refactor(fabricengine): re-signature wrapProbeError and migrate the probe sites`

### Card 24: migrate clone.go's six sites

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/fabricengine/warpprobe.go`
- **Edits:**
  - `internal/fabricengine/clone.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Migrate all six `gitexec.RunGit` sites in `suffixWeftPrimaryBranch`, `bornWeftPrimaryBranch`, and `cloneRepo` to `gitexec.Run`.
  Two of them are mixed `rev-parse` probes and take `errors.As` recovery rather than a merge: the remote weft primary-ref probe in `suffixWeftPrimaryBranch`, and the unborn-branch verify in `bornWeftPrimaryBranch`.
  At each, the exit path is an answer and the exec path already returns a real error, so `errors.As(err, &gitErr)` selects the answer branch and anything else propagates the error.
  Every other site in this file is a plain two-message merge under `default-merge-rule`, including the deletion of any `(git exit %d)` or `exited %d` fragment together with its `exitCode` argument.
  Preserve `cloneRepo`'s existing error wording apart from the merge itself — the clone failure message is what operators see on a bad URL.
- **Commit:** `refactor(fabricengine): migrate clone.go's call sites to the checked form`

### Card 25: migrate boardweft.go's three sites

- **Context:**
  - `internal/gitexec/gitexec.go`
- **Edits:**
  - `internal/fabricengine/boardweft.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Migrate all three `gitexec.RunGit` sites in `ensureBoardWorktree` to `gitexec.Run`.
  The `rev-parse --verify --quiet refs/heads/<warp>` local weft-branch probe is a mixed probe and takes `errors.As` recovery: its exit path answers "the branch is not there yet", which is the orphan-create path this function exists to support, while its exec path returns a real error.
  The other two sites are plain two-message merges under `default-merge-rule`.
  Do not change the orphan-create control flow — only the shape of the calls and the messages the merge collapses.
- **Commit:** `refactor(fabricengine): migrate boardweft.go's call sites to the checked form`

### Card 26: migrate reconcile.go, including readBranch's composed diagnostic

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/fabricengine/destroy.go`
- **Edits:**
  - `internal/fabricengine/reconcile.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Migrate all six `gitexec.RunGit` sites in `reconcileWarpBinding`, `reconcileMissingWeft`, `adoptWeftWorktree`, `createDormantWeftForRawWarp`, and `readBranch` to `gitexec.Run`.
  The `worktree prune` site in `reconcileMissingWeft` is one of the four best-effort discards: write it `_, _ = gitexec.Run(…)` and give it its own comment stating why discarding is correct there.
  It is not a raw site and must not carry a `//gitexec:raw` marker.
  In `readBranch`, both calls migrate and the earlier call's `*GitError` stays bound so its exit code survives into the two downstream messages that cite it.
  The migrated shape is: a nil error from the `rev-parse --abbrev-ref HEAD` call returns the trimmed stdout;
  a failure that `errors.As` does **not** recover as `*GitError` returns `fmt.Errorf("rev-parse: %w", err)`;
  otherwise the `branch --show-current` fallback runs, and its own failure likewise splits — a non-`*GitError` returns `fmt.Errorf("branch --show-current: %w", err)`, while a `*GitError` returns a message citing the first call's `gitErr.ExitCode` alongside the fallback's own error, preserving today's combined diagnostic.
  The no-current-branch-set message keeps citing the first call's `gitErr.ExitCode` the same way.
  Neither call may be left raw: every exit in `readBranch` is a failure once the fallback is exhausted, so a marker there could not honestly claim otherwise.
  The remaining sites are plain two-message merges under `default-merge-rule`.
- **Commit:** `refactor(fabricengine): migrate reconcile.go and preserve readBranch's composed diagnostic`

## Batch Tests

`verify:` runs `go build ./...`, then `go test ./internal/fabricengine/...` and `go test -tags integration ./internal/fabricengine/...`.
The load-bearing coverage is `internal/fabricengine/warpbinding_clone_integration_test.go`, which asserts the `"probe weft "` prefix survives `wrapProbeError`'s re-signature in all three of its failure scenarios, and the clone and reconcile integration coverage in the same suite, which drives `ensureBoardWorktree`'s orphan-create path and `readBranch`'s unborn-branch fallback — the two places where a mis-transcribed `errors.As` recovery converts an answer into a hard failure.
The Tier 1 run is included because `internal/fabricengine`'s untagged tests cover the pure-parsing helpers around these call sites, and a signature slip in `wrapProbeError` must not wait for the tagged run to surface.
