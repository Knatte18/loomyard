# Batch: hook-and-launcher

```yaml
task: 'Introduce warp: the host↔weft-coordinated git module'
batch: hook-and-launcher
number: 8
cards: 4
verify: go build ./... && go test ./internal/warp/
depends-on: [7]
```

## Batch Scope

Add the proactive drift detection point and the friction-asymmetry shortcut. The **post-checkout git hook** is an embedded POSIX `sh` script installed into the repo's common `.git/hooks/post-checkout` (idempotent, non-clobbering — chains an existing hook); it fires after a raw `git checkout`/`switch`, identifies the worktree it ran in, resolves the deterministic `<base>-weft` sibling, and warns when host and weft branches diverge. The **launcher shortcut** makes warp's per-worktree launcher generation emit a `warp-checkout.cmd` that invokes `lyx warp checkout`.

Batch-local decisions: the hook is installed at `warp clone` and `warp add` time. It never hard-blocks (principle 6). On git-for-Windows the hook runs under the bundled bash, so the script body is POSIX `sh`. The launcher shortcut lives entirely in warp's launcher code — no `internal/ide` change (the `ide menu` is a worktree picker, not an action menu).

## Cards

### Card 27: post-checkout hook — embedded script + install primitive

- **Context:**
  - `internal/warp/drift.go`
  - `internal/warp/weftwiring.go`
  - `internal/paths/paths.go`
  - `internal/gitexec/gitexec.go`
  - `internal/fslink/fslink.go`
- **Edits:** none
- **Creates:**
  - `internal/warp/hook.go`
  - `internal/warp/post-checkout.sh`
- **Deletes:** none
- **Requirements:** Create `internal/warp/post-checkout.sh` — a POSIX `sh` script (`go:embed`-ed) that, on checkout, determines the current worktree (cwd / `git rev-parse --show-toplevel`), computes the deterministic `<base>-weft` sibling, compares the two branches, and prints a non-blocking warning ("host/weft out of sync — run `lyx warp reconcile`") when they differ; exit 0 always. Create `internal/warp/hook.go` with `func InstallPostCheckoutHook(l *paths.Layout) error`: resolve the repo's common hooks dir (`gitexec` `rev-parse --git-common-dir` + `hooks/post-checkout`), write the embedded script idempotently, and be **non-clobbering** — if a user `post-checkout` already exists and is not warp's, chain it (back up / invoke the prior hook then warp's check) rather than overwrite; mark warp's content with a sentinel comment so re-install is idempotent. Make the script executable where applicable.
- **Commit:** `feat(warp): embedded post-checkout drift-warning hook + installer`

### Card 28: Install the hook at clone and add

- **Context:**
  - `internal/warp/hook.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/warp/clone.go`
  - `internal/warp/add.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Call `InstallPostCheckoutHook` from the clone path (after the host worktree exists) and from `Add` (after the host worktree is created). Installation failure is non-fatal to the operation — log/report it but do not abort the clone/add (the hook is belt-and-suspenders). Keep `Add`'s all-or-nothing rollback semantics unchanged (a hook-install failure does not trigger pair rollback).
- **Commit:** `feat(warp): install post-checkout hook at clone and add`

### Card 29: Launcher checkout shortcut

- **Context:**
  - `internal/warp/portals.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/warp/launchers.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/warp/launchers.go` `writeLaunchers`, emit an additional per-worktree launcher `warp-checkout.cmd` (alongside the existing `ide.cmd`/`ide-menu.cmd`) whose body invokes `lyx warp checkout` (matching the existing `.cmd` generation style; Windows-only, no-op elsewhere as today). Ensure `removeLaunchers` still cleans the whole launcher dir (it uses `os.RemoveAll`, so the new file is covered).
- **Commit:** `feat(warp): emit warp-checkout launcher shortcut`

### Card 30: hook and launcher tests

- **Context:**
  - `internal/warp/hook.go`
  - `internal/warp/launchers.go`
  - `internal/warp/launchers_test.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/warp/launchers_test.go`
- **Creates:**
  - `internal/warp/hook_test.go`
- **Deletes:** none
- **Requirements:** `hook_test.go`: `InstallPostCheckoutHook` writes the script into the common hooks dir; re-install is idempotent (sentinel detected, no duplication); an existing non-warp hook is chained, not clobbered; the validation criterion — cwd-based worktree identification resolves the correct `<base>-weft` sibling for both a prime (`<PrimeName>-weft`) and a child (`<slug>-weft`) worktree — is asserted (drive a real checkout in an integration-tagged case and assert the warning fires on divergence). Extend `launchers_test.go` to assert `warp-checkout.cmd` is emitted with the `lyx warp checkout` invocation and removed by `removeLaunchers`. Seed config via `warp.ConfigTemplate()` at the call site.
- **Commit:** `test(warp): post-checkout hook install/chain and launcher shortcut`

## Batch Tests

`verify: go build ./... && go test ./internal/warp/`. `hook_test.go` is the new TDD surface and carries the shared-`.git/hooks` validation criterion (correct `<base>-weft` resolution for prime and child). The extended `launchers_test.go` covers the shortcut emission. Integration-tagged cases drive a real `git checkout` to confirm the hook warns on drift without blocking.
