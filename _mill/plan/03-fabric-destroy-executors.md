# Batch: fabric-destroy-executors

```yaml
task: 'gitexec: add the checked entry point and migrate the call sites'
batch: fabric-destroy-executors
number: 3
cards: 5
verify: go build ./... && go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
depends-on: [1]
```

## Batch Scope

This batch re-signatures the three git-spawning gate executors in `internal/fabricengine/destroy.go` from `(exitCode int, stderr string, err error)` to `error`, and updates all nine production call sites plus the `export_test.go` seam and its one test consumer.
It is one batch because a re-signature does not compile until every caller moves with it.
It is deliberately **only** the executor call sites: the other `gitexec.RunGit` sites living in the same six caller files migrate in batch 4, so this batch's diff stays readable as one shape change.

The two hardest sites are here, and they are not merges.
`remove.go`'s `removeWarpWorktreeDir` and `prune.go`'s `removeStalePair` use the old `exitCode != 0` branch as **control flow**: it re-probes worktree registration and, when the worktree is still registered, performs a fallback destructive `removePath`.
Collapsing those two branches under the default merge rule either drops the fallback — so `lyx fabric remove` and `prune` stop cleaning up a worktree git itself declined to remove — or runs the fallback on the exec-failure path, routing an exec-level failure or a gate refusal raised *before git ran* into a destructive primitive, which is a Fabric Destruction Chokepoint Invariant violation reached by a message-merging rule.

Batch-local decisions beyond `## Shared Decisions`:

- The recording predicate inside each executor changes from `err == nil && exitCode == 0` to `err == nil`, and must not become unconditional — the Mutation Record Invariant requires the append to follow an observed effect.
- The gate's own pipeline errors (`*destructiveRefusal` from `checkPathRequest` / `checkBranchRequest`) are returned before any git spawn and are unaffected: a refusal is still not a `*GitError`, so every `errors.As(err, &refusal)` handler and every `surfaceRefusal` call keeps working unchanged.
- At the two shape-(D) sites, the messages inside the preserved sub-branches keep their `(git exit %d)` fragment and their stderr, filled from the recovered `gitErr.ExitCode` and `gitErr.Stderr`.
  This is the prior-call-diagnostic treatment the discussion states for `prune.go`, applied identically to `remove.go`'s sibling message: under shape (D) the branch was never a message merge, so its two sub-branch strings are preserved verbatim with their values re-sourced from the recovered error rather than rewritten.
- No banned destructive argument slice leaves `destroy.go`: `{"worktree", "remove"}`, `{"branch", "-D"}`, and `createdToken{` all stay inside it, so `cmd/lyx/destructiveguard_test.go` is unaffected — confirm that by running it, do not assume it.

## Cards

### Card 13: re-signature the three gate executors

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/fabricengine/mutation.go`
- **Edits:**
  - `internal/fabricengine/destroy.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `removeGitWorktree(rec *Mutations, req pathRequest, repoDir string)` to return a single `error`, `deleteBranch(rec *Mutations, req branchRequest)` to return a single `error`, and `createGitWorktree(rec *Mutations, repoDir string, addArgs []string, target string)` to return `(createdToken, error)`.
  Each body calls `gitexec.Run` instead of `gitexec.RunGit`, discards the stdout return, and returns the resulting error unchanged — do not wrap it, because every call site builds its own message from it.
  Each recording predicate becomes `if err == nil`.
  In `createGitWorktree`, the early return on failure becomes `return createdToken{}, err`, and the explicit `tok createdToken` named first result may lose its name now that no `exitCode int` follows it — either spelling is acceptable, but the godoc paragraph explaining why the name was needed must go, because its premise is gone.
  Rewrite all three executor godocs: delete the `(exitCode, stderr, err)` return-shape descriptions, the "appends … only when err is nil AND exitCode is zero" recording-predicate wording, and `createGitWorktree`'s named-return rationale.
  Update the file header's "Recording contract" paragraph, which states the same nonzero-exit-with-nil-error reasoning, to describe the new single-error predicate.
  These are the godocs a future reader of the destructive chokepoint reads first, so they must describe the code as it now is.
  Do not change any check-pipeline function, any refusal type, or any non-git executor in this file.
- **Commit:** `refactor(fabricengine): re-signature the three gate executors to return error`

### Card 14: the two shape-(D) call sites — control flow, not a merge

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/fabricengine/destroy.go`
- **Edits:**
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/prune.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `removeWarpWorktreeDir` (`internal/fabricengine/remove.go`), keep the existing `errors.As(err, &refusal) && !isRegisteredLinkedWorktree(l, target)` branch first and unchanged — it reports the gate's pre-git refusal with its own pre-existing message.
  Below it, add `var gitErr *gitexec.GitError` and bail with the existing `fmt.Errorf("run git worktree remove for %s: %w", target, err)` whenever `!errors.As(err, &gitErr)`: git never ran, or the gate refused before it could, so destroy nothing.
  Only inside the `errors.As` success branch does the old exit-path logic run: the `!isRegisteredLinkedWorktree` message, then the `fallbackReq` `removePath` call with its `*destructiveRefusal` pass-through and its "fallback removal failed" wrapper.
  Both preserved messages keep their exact current text, with `%d` filled from `gitErr.ExitCode` and `%s` from `strings.TrimSpace(gitErr.Stderr)`.
  Apply the identical split in `removeStalePair` (`internal/fabricengine/prune.go`): `!errors.As(err, &gitErr)` sets `pe.Error` from the error and returns false without destroying anything;
  the `errors.As` success branch keeps the `isRegisteredLinkedWorktreeIn` re-probe, its "git refused to remove weft worktree" message filled from `gitErr.ExitCode` and `gitErr.Stderr`, and the `removePath` fallback whose failure message keeps its `(git exit %d)` fragment filled from `gitErr.ExitCode` — that `%d` cites the `worktree remove` call while the `%v` reports the `removePath` fallback's error, so it is two failures in one string, not a duplicate of anything.
  Do not touch either file's other `gitexec.RunGit` sites in this card;
  the best-effort `worktree prune` discards in both files migrate in batch 4.
- **Commit:** `refactor(fabricengine): split the destructive fallback on errors.As, not on a merge`

### Card 15: the shape-(A) and shape-(B) executor call sites

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/fabricengine/destroy.go`
- **Edits:**
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/add.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `removeWeftWorktree` (`internal/fabricengine/weftwiring.go`), the `removeGitWorktree` and `deleteBranch` call sites are already unified as `err != nil || exitCode != 0`;
  collapse each to `if err != nil`, assigning the real error to `firstErr` when `firstErr == nil`.
  The synthesised `fmt.Errorf("git worktree remove failed with exit code %d", exitCode)` and `fmt.Errorf("git branch -D failed with exit code %d", exitCode)` fallbacks disappear entirely, because the branch now always has a real error to carry.
  Do not touch this file's `worktree prune` call in this card.
  In `rollbackAdd` (`internal/fabricengine/add.go`), the `removeGitWorktree` and `deleteBranch` call sites keep their `surfaceRefusal` branch first and unchanged, and their `else if err != nil || exitCode != 0` branch collapses to `else if err != nil` with the same disappearance of the synthesised exit-code fallbacks.
  In `Add` (`internal/fabricengine/add.go`), migrate the `createGitWorktree` call to the two-value form and apply `default-merge-rule` to it: it is a plain two-message merge, so the exit-path message wins with `%w` of the error and the exec-path message is dropped.
  Do not touch `add.go`'s six `gitexec.RunGit` sites in this card — they migrate in batch 4.
- **Commit:** `refactor(fabricengine): collapse the unified executor call sites onto the error return`

### Card 16: the remaining two executor call sites

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/fabricengine/destroy.go`
- **Edits:**
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/checkout.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `deleteWeftBranch` (`internal/fabricengine/cleanup.go`), the failure sink is `entry.Error`, a plain `string` field, so `%w` is unavailable and must not be reached for — a `%w` inside `fmt.Sprintf` renders as `%!w(…)` rather than failing the build.
  Merge the two assignments into one `entry.Error = fmt.Sprintf(...)` using `%v` of the error, keeping the exit-path message's wording and dropping its `(git exit %d)` fragment together with the `exitCode` argument, exactly as elsewhere.
  These are display-only report fields, never consumed via `errors.Is`/`errors.As`, which is why `%v` suffices rather than widening the field to `error`.
  In `rollbackSwitch` (`internal/fabricengine/checkout.go`), the `deleteBranch` call becomes `if err := deleteBranch(rec, req); err != nil {` and everything inside stays as it is — the `errors.As(&refusal)` handler and its `logger.Warn` are unchanged, because a gate refusal on a best-effort path is never allowed to vanish silently.
  Do not touch `checkout.go`'s eight `gitexec.RunGit` sites in this card — they migrate in batch 4.
- **Commit:** `refactor(fabricengine): merge the executor's string sink and simplify the rollback call`

### Card 17: the export_test.go seam and its consumer

- **Context:**
  - `internal/fabricengine/destroy.go`
- **Edits:**
  - `internal/fabricengine/export_test.go`
  - `internal/fabricengine/destructivegaps_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `DeleteBranchForTest` returns `deleteBranch(...)` directly, so the executor re-signature changes this seam's own signature to `error`.
  Update its declaration and its godoc, then update both call sites in `internal/fabricengine/destructivegaps_integration_test.go`: the refusal assertion drops its two discarded returns, and the clean-deletion assertion drops the `exitCode` and `stderr` bindings it currently reports in its failure message, asserting on the returned error alone.
  Keep both tests asserting exactly what they assert today — that the gate refuses in both force modes with the expected message substring and deletes nothing, and that an orphan branch is deleted cleanly.
  This is an in-scope consequence of the re-signature: the Checked-Call Invariant exempts test files from the *marker* requirement, which is not a claim that no test file changes.
- **Commit:** `test(fabricengine): follow the executor re-signature through the export_test seam`

## Batch Tests

`verify:` runs `go build ./...`, then `go test ./internal/fabricengine/...` (Tier 1) and `go test -tags integration ./internal/fabricengine/...` (Tier 2).
The Tier 2 run is the real net here.
`internal/fabricengine/destructivegaps_integration_test.go` covers the re-signatured `DeleteBranchForTest` seam directly, and `internal/fabricengine/warpforward_integration_test.go` asserts that a dirtiness-gate refusal stays distinguishable from a failure — which is exactly what the new `err == nil` recording predicate and the surviving `errors.As(&refusal)` paths must preserve.
The destructive-fallback split in card 14 has no dedicated unit test;
its regression net is the remove and prune integration coverage in the same suite, which is why this batch runs the full package at both tiers rather than a narrower selection.
`go build ./...` is included because the re-signature's blast radius is compile-checked, and a missed caller must fail here rather than at batch 4.
