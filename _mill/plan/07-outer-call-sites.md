# Batch: outer-call-sites

```yaml
task: 'gitexec: add the checked entry point and migrate the call sites'
batch: outer-call-sites
number: 7
cards: 3
verify: go build ./... && go test ./internal/lyxcwd/... ./internal/fabriccli/... ./internal/websterengine/... ./internal/configcli/... ./internal/reedcli/... ./internal/idecli/... ./internal/loomengine/... && go test -tags integration ./internal/lyxcwd/... ./internal/fabriccli/... ./internal/websterengine/...
depends-on: [1]
```

## Batch Scope

This batch migrates the three production `gitexec.RunGit` sites outside `internal/fabricengine` and `internal/gitrepo`: one each in `internal/lyxcwd`, `internal/fabriccli`, and `internal/websterengine`.
It depends on batch 1 alone and shares no file with any fabric batch, so it can run as soon as `gitexec.Run` exists.
None of the three keeps a raw marker — each pins zero in batch 8's map, stated explicitly rather than omitted.

Two of the three are re-filings: an earlier draft had `lyxcwd` and `fabriccli` raw, and each was re-filed to checked by `the-raw-vs-checked-discriminator`, because each already reports its exec path separately from its exit path.
Both have pinned surfaces that must survive byte-for-byte, which is what makes `errors.As` the right tool rather than a merge: `errors.As` reproduces the exit-path surface exactly while the exec path keeps its own, distinct one.

## Cards

### Card 30: migrate lyxcwd's worktree-root probe

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/loomengine/preflight.go`
  - `internal/configcli/reconcile_test.go`
- **Edits:**
  - `internal/lyxcwd/lyxcwd.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Migrate `gitWorktreeRoot`'s `rev-parse --show-toplevel` call to `gitexec.Run` with `errors.As` recovery.
  The migrated shape is: a nil error trims, `filepath.FromSlash`es, and `filepath.Clean`s the stdout exactly as today;
  `errors.As(err, &gitErr)` returns the **bare** `ErrNotAGitRepo` sentinel, unwrapped and unwrapped-over;
  anything else returns the existing `fmt.Errorf("%w: %v", ErrNotAGitRepo, err)` form, with the sentinel keeping the `%w` verb and the error going in as `%v`.
  Both surfaces must survive byte-for-byte.
  `internal/loomengine/preflight.go` does `errors.Is(err, lyxcwd.ErrNotAGitRepo)`, and exact-string assertions in `internal/lyxcwd`, `internal/configcli`, `internal/reedcli`, and `internal/idecli` pin the bare-sentinel rendering — `internal/configcli/reconcile_test.go` asserts the CLI error is exactly `"not a git repository"`.
  Never `%w`-wrap the `GitError` over the sentinel: that breaks `errors.Is` at every one of those consumers.
  This site takes no `//gitexec:raw` marker.
  `internal/lyxcwd`'s import cap is stdlib plus `internal/gitexec`;
  `errors` is stdlib, so adding it is within the cap, but add nothing else.
- **Commit:** `refactor(lyxcwd): migrate the worktree-root probe to the checked form`

### Card 31: migrate fabriccli's current-branch probe

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/output/output.go`
- **Edits:**
  - `internal/fabriccli/fabric.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Migrate `runCheckout`'s `branch --show-current` call to `gitexec.Run` with `errors.As` recovery.
  The migrated shape is: a nil error trims the stdout into `branch` and falls through to the existing detached-HEAD check unchanged;
  `errors.As(err, &gitErr)` returns `output.Err(out, "usage: lyx fabric checkout <branch>")`, preserving the exit-path surface exactly;
  anything else returns `output.Err(out, err.Error())`, preserving the exec-path surface.
  Do not merge the two branches — the usage string is a distinct answer for "this checkout has no current branch to infer", not a diagnostic for "git could not run".
  This site takes no `//gitexec:raw` marker.
- **Commit:** `refactor(fabriccli): migrate the current-branch probe to the checked form`

### Card 32: migrate websterengine's status probe

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `cmd/lyx/rawgitmutation_test.go`
- **Edits:**
  - `internal/websterengine/gitwrap.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Migrate `dirty`'s `status --porcelain` call to `gitexec.Run`.
  Both of its branches are genuine failures with real messages, so this is a clean two-message merge under `default-merge-rule`: the single guard becomes `if err != nil { return false, fmt.Errorf("websterengine: git status --porcelain in %s: %w", worktree, err) }`, the exec-path duplicate is dropped, and the exit path's `strings.TrimSpace(stderr)` argument goes with it.
  Drop the now-unused `strings` import only if nothing else in the file uses it — the success path still calls `strings.TrimSpace(stdout)`, so it stays.
  Update the two comments in this file that name `gitexec.RunGit` — the file header and `dirty`'s own godoc — to name the checked entry point instead.
  This site takes no `//gitexec:raw` marker;
  its `cmd/lyx/rawgitmutation_test.go` allowlist entry is reworded in batch 8, not here.
- **Commit:** `refactor(websterengine): migrate the status probe to the checked form`

## Batch Tests

`verify:` runs `go build ./...`, then the Tier 1 suites of the three migrated packages plus the four whose tests pin `lyxcwd`'s sentinel surfaces (`internal/configcli`, `internal/reedcli`, `internal/idecli`, and `internal/loomengine`, the last for its `errors.Is` consumer), then the Tier 2 suites of the three migrated packages.
That widened Tier 1 scope is the point of this batch's verify: card 30's `errors.As` recovery is correct only if both the bare-sentinel rendering and the `%w: %v` exec-path rendering survive, and the four CLI suites are what pin the first of those.
`internal/websterengine`'s own suite covers the one migrated `gitwrap.go` site.
