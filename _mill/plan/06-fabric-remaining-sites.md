# Batch: fabric-remaining-sites

```yaml
task: 'gitexec: add the checked entry point and migrate the call sites'
batch: fabric-remaining-sites
number: 6
cards: 3
verify: go build ./... && go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
depends-on: [5]
```

## Batch Scope

This batch finishes `internal/fabricengine`, migrating the twelve sites in the eight files no earlier batch touched: `index.go` (3), `pull.go` (3), `dirtiness.go` (1), `gitexclude.go` (1), `hook.go` (1), `status.go` (1), `weftgit.go` (1), `worktreelist.go` (1).
It carries the package's one content-sniff site and one of the seven mixed `rev-parse` probes;
everything else here is a plain two-message merge.
After this batch, `internal/fabricengine` has exactly two `gitexec.RunGit` sites left, both marked raw in batch 4, which is the count batch 8's guard pins.

Batch-local decision beyond `## Shared Decisions`: `index.go`'s unborn-HEAD check inspects **stderr content**, not the exit code, to decide answer-versus-failure.
It gets strictly better under the change, because the string it inspects and the diagnostic it falls through to become the same value instead of a stderr string that has to stay in scope alongside an exit code.

## Cards

### Card 27: migrate index.go, including the unborn-HEAD content sniff

- **Context:**
  - `internal/gitexec/gitexec.go`
- **Edits:**
  - `internal/fabricengine/index.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Migrate the three `gitexec.RunGit` sites in `weftGitDir`, `warpSeq`, and `scanWarpSHATrailers` to `gitexec.Run`.
  In `scanWarpSHATrailers`, move the `strings.Contains(stderr, "does not have any commits yet")` sniff onto the recovered error: `var gitErr *gitexec.GitError` plus `errors.As(err, &gitErr) && strings.Contains(gitErr.Stderr, "does not have any commits yet")` returns `(nil, nil)`, keeping the unborn-HEAD-is-an-empty-history answer, and anything else returns the failure with `%w` of the error in place of the discarded stderr.
  Keep the existing comment explaining why an unborn HEAD is not a scan failure.
  The other two sites are plain two-message merges under `default-merge-rule`.
- **Commit:** `refactor(fabricengine): migrate index.go and move its unborn-HEAD sniff onto GitError`

### Card 28: migrate pull.go's three sites

- **Context:**
  - `internal/gitexec/gitexec.go`
- **Edits:**
  - `internal/fabricengine/pull.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Migrate the three `gitexec.RunGit` sites in `weftHasUpstream`, `warpUpstreamSHA`, and `patternResidueCommits` to `gitexec.Run`.
  `weftHasUpstream` is a mixed `rev-parse` probe and takes `errors.As` recovery: its exit path answers "no upstream is configured" while its exec path returns a real error, so `errors.As(err, &gitErr)` selects the answer and anything else propagates the error.
  The other two are plain two-message merges under `default-merge-rule`.
  Leave the `errors.Is(err, gitrepo.ErrNoCommits)` handling in this file untouched — it reads a `gitrepo` sentinel, not a git exit code, and the migration does not reach it.
- **Commit:** `refactor(fabricengine): migrate pull.go's call sites to the checked form`

### Card 29: migrate the six single-site files

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/fabricengine/worktreelist_test.go`
- **Edits:**
  - `internal/fabricengine/dirtiness.go`
  - `internal/fabricengine/gitexclude.go`
  - `internal/fabricengine/hook.go`
  - `internal/fabricengine/status.go`
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/worktreelist.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Migrate the single `gitexec.RunGit` site in each of `worktreeDirty` (`internal/fabricengine/dirtiness.go`), `resolveGitExcludePath` (`internal/fabricengine/gitexclude.go`), `InstallPostCheckoutHook` (`internal/fabricengine/hook.go`), `detectWarpPollution` (`internal/fabricengine/status.go`), `entryMatchesWeft` (`internal/fabricengine/weftgit.go`), and `List` (`internal/fabricengine/worktreelist.go`) to `gitexec.Run`.
  All six are plain two-message merges under `default-merge-rule`: the exit-path message wins with `%w` of the error, the exec-path message is dropped, and any `(git exit %d)` or `exited %d` fragment goes with its `exitCode` argument.
  `internal/fabricengine/worktreelist_test.go` asserts that `List`'s error against a non-repository contains `"not a git repository"`;
  the merged form still satisfies that, because `GitError.Error()` renders git's own stderr, but verify it rather than assume it and do not edit that test.
- **Commit:** `refactor(fabricengine): migrate the remaining single-site files to the checked form`

## Batch Tests

`verify:` runs `go build ./...`, then `go test ./internal/fabricengine/...` and `go test -tags integration ./internal/fabricengine/...`.
The Tier 1 run matters more here than in the preceding fabric batches: `internal/fabricengine/worktreelist_test.go`'s non-repository assertion is untagged and is the one test in this batch that pins a merged error string, so a merge that dropped git's stderr instead of wrapping the error fails immediately.
The Tier 2 run covers `worktreeDirty`, `detectWarpPollution`, `InstallPostCheckoutHook`, and the index and pull paths against real repositories, including `scanWarpSHATrailers`'s unborn-HEAD answer — the one site in this batch where a mis-transcribed sniff converts an empty history into a hard failure.
After this batch, re-run the raw-site regeneration grep over `internal/fabricengine` and confirm exactly two `gitexec.RunGit` sites remain, both carrying their batch 4 markers.
