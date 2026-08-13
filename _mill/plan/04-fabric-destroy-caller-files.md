# Batch: fabric-destroy-caller-files

```yaml
task: 'gitexec: add the checked entry point and migrate the call sites'
batch: fabric-destroy-caller-files
number: 4
cards: 5
verify: go build ./... && go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
depends-on: [3]
```

## Batch Scope

This batch migrates the `gitexec.RunGit` sites still sitting in the six files batch 3 touched for their executor call sites: `add.go` (6), `checkout.go` (8), `weftwiring.go` (5), `cleanup.go` (1), `prune.go` (2), `remove.go` (1) — twenty-three sites in all.
It depends on batch 3 rather than running beside it precisely because it re-edits the same files;
the two are sequenced so no batch boundary has to reconcile concurrent edits to `add.go` or `checkout.go`.
It also plants two of the migration's five raw markers, in `weftwiring.go`.

Batch-local decisions beyond `## Shared Decisions`:

- `weftwiring.go`'s `weftRepoExists` and `weftBranchExists` are the only two raw sites left in `internal/fabricengine`, and their justification is the one raw class the invariant exists to name: the signature returns a plain `bool` with no error channel, so every outcome — exec failure included — must collapse into it.
  Migrating them with `err != nil → return false` would fold an exec failure into "the branch does not exist", which is the exact conflation the two-shape split exists to prevent, written at a site with no way to report it.
- `checkout.go`'s weft-branch capture in `rollbackSwitch`'s caller path is best-effort by design, written as a compound condition with no error return, because a detached or unborn weft HEAD has no branch name to roll back to.
  It migrates to the checked form as a plain simplification, collapsing two conditions into one.
- The four best-effort `worktree prune` discards are checked-form calls whose error is deliberately dropped;
  three of the four live in this batch's files.

## Cards

### Card 18: mark the two bool-returning predicates raw

- **Context:**
  - `internal/gitexec/gitexec.go`
- **Edits:**
  - `internal/fabricengine/weftwiring.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Leave the `gitexec.RunGit` call in `weftRepoExists` (the `rev-parse --is-inside-work-tree` probe) and the one in `weftBranchExists` (the `rev-parse --verify refs/heads/<branch>` probe) on the raw form, with their bodies unchanged, and add an adjacent `//gitexec:raw — <why>` marker at each.
  The justification must read, in substance, "bool-returning predicate: the signature has no error channel, so every outcome must collapse to a bool".
  Use the same `//gitexec:raw` token the `internal/gitrepo` markers use — one invariant, one searchable token.
  These two are the sites `internal/fabricengine`'s pinned raw count of 2 refers to.
- **Commit:** `docs(fabricengine): mark the two bool-returning predicates as deliberate raw sites`

### Card 19: migrate weftwiring.go's remaining three sites

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/fabricengine/destroy.go`
- **Edits:**
  - `internal/fabricengine/weftwiring.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Migrate the `gitexec.RunGit` sites in `createWeftWorktree`, `pushWeftBranch`, and the `worktree prune` call at the end of `removeWeftWorktree` to `gitexec.Run`.
  The first two are plain two-message merges and take `default-merge-rule` as written.
  The `worktree prune` site is not a discard here — it currently feeds `firstErr` — so it keeps that role: collapse `err != nil || exitCode != 0` to `if err != nil`, assign the real error to `firstErr` when `firstErr == nil`, and delete the synthesised `fmt.Errorf("git worktree prune failed with exit code %d", exitCode)` fallback.
  Do not touch the two marked raw sites from card 18.
- **Commit:** `refactor(fabricengine): migrate weftwiring.go's checked call sites`

### Card 20: migrate add.go's six sites

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/fabricengine/destroy.go`
- **Edits:**
  - `internal/fabricengine/add.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Migrate all six `gitexec.RunGit` sites in `Add` and `rollbackAdd` to `gitexec.Run`.
  Two of the six need shapes `default-merge-rule` does not cover, and both are in `Add`.
  The `rev-parse --verify refs/heads/<warp>` warp-branch existence probe is a mixed probe: its exit path is an answer ("the branch already exists") while its exec path returns a real error, so recover with `var gitErr *gitexec.GitError` and treat only `errors.As(err, &gitErr)` as the answer, letting anything else return the error.
  The `rev-parse --abbrev-ref HEAD` parent-branch probe is a **compound** guard and must not be merged: its exit branch today is `exitCode != 0 || strings.TrimSpace(stdout) == "HEAD"`, and the second disjunct fires on a *successful* git call with a nil error — a detached HEAD — so there is no error to wrap there at all.
  Merging it mechanically would both misattribute an unrelated exec-level failure to "detached HEAD or unborn branch" and be unwritable for the success arm.
  The migrated shape keeps the two conditions apart: a non-nil error that `errors.As` does not recover as `*GitError` returns the existing `fmt.Errorf("rev-parse abbrev-ref HEAD: %w", err)`;
  a non-nil error that it does recover returns the existing detached-HEAD-or-unborn-branch message;
  and a nil error still falls through to the separate `strings.TrimSpace(stdout) == "HEAD"` check, which returns that same message.
  Both operator-visible strings survive unchanged.
  The remaining four sites in this file are plain two-message merges taking `default-merge-rule`.
  The `worktree prune` site in `rollbackAdd` currently feeds `firstErr`;
  collapse it to `if err != nil` and delete the synthesised exit-code fallback, exactly as card 19 does for the sibling site.
  Every remaining site here is a plain two-message merge and needs no special handling beyond `default-merge-rule`.
  Do not reword any of this file's error messages beyond what the merge itself requires — the wording at these sites was already settled per-site by an earlier campaign, and re-litigating it is out of scope.
- **Commit:** `refactor(fabricengine): migrate add.go's call sites to the checked form`

### Card 21: migrate checkout.go's eight sites

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/fabricengine/destroy.go`
- **Edits:**
  - `internal/fabricengine/checkout.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Migrate all eight `gitexec.RunGit` sites in `Checkout`, `switchOrForkWeft`, and `rollbackSwitch` to `gitexec.Run`.
  The two-message merge sites take `default-merge-rule`, including every `(git exit %d)` fragment deletion — this file carries the `"warp switch to branch %q failed (git exit %d): %s"` shape the decision names as its worked example, which becomes `fmt.Errorf("warp switch to branch %q failed: %w", branch, err)`.
  The best-effort weft-branch capture, currently written `if out, _, code, werr := gitexec.RunGit(…); werr == nil && code == 0 {`, becomes `if out, err := gitexec.Run(…); err == nil {`, collapsing both conditions into one.
  In `rollbackSwitch`, the two `switch` calls are deliberate best-effort probes gating a `rec.Append`: each becomes `if _, err := gitexec.Run(…); err == nil { rec.Append(…) }`, which preserves the record-only-on-observed-effect rule exactly.
  Leave the `deleteBranch` call and its `errors.As(&refusal)` / `logger.Warn` handler as batch 3 left them.
- **Commit:** `refactor(fabricengine): migrate checkout.go's call sites to the checked form`

### Card 22: migrate cleanup.go, prune.go, and remove.go's remaining sites

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/fabricengine/destroy.go`
- **Edits:**
  - `internal/fabricengine/cleanup.go`
  - `internal/fabricengine/prune.go`
  - `internal/fabricengine/remove.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Migrate the `gitexec.RunGit` site in `listWeftBranches` (`internal/fabricengine/cleanup.go`) to `gitexec.Run` as a plain two-message merge under `default-merge-rule`.
  Migrate the three best-effort `worktree prune` discards — two in `removeStalePair` (`internal/fabricengine/prune.go`) and one in `removeWarpWorktreeDir` (`internal/fabricengine/remove.go`) — to the form `_, _ = gitexec.Run(…)`.
  Delete the two `//nolint:errcheck` comments, which both sit on the `prune.go` pair and enforce nothing because this repo runs no `golangci-lint` — the `remove.go` site already discards via the bare `_, _, _, _ =` form and has no such comment to remove.
  Give each of the three sites its own comment stating why discarding is correct there: a failed prune leaves a stale registration the next reconcile or prune re-reports, and it must not turn a completed removal into an error.
  These three are not raw sites and must never carry a `//gitexec:raw` marker — they use the checked form and drop its error, so a raw marker would be false and would inflate the pinned counts batch 8 asserts.
  Leave the shape-(D) executor call sites in both files exactly as batch 3 left them.
- **Commit:** `refactor(fabricengine): migrate the remaining cleanup, prune, and remove sites`

## Batch Tests

`verify:` runs `go build ./...`, then `go test ./internal/fabricengine/...` and `go test -tags integration ./internal/fabricengine/...`.
The whole package is the right scope: twenty-three sites across six files, spread over `Add`, `Checkout`, `Cleanup`, `Prune`, and `Remove`, and the package's real behavioural coverage lives entirely behind the `integration` tag.
No test asserts on the merged wording at any of these sites, so what the suite proves is that the control flow around each merge is still correct — in particular that `rollbackAdd`, `rollbackSwitch`, and `removeWeftWorktree` still report their first error, and that the record entries `rollbackSwitch`'s two `switch` probes gate are still appended only on an observed effect.
