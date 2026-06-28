# Batch: warp-features

```yaml
task: "CLI help & error ergonomics from sandbox run"
batch: "warp-features"
number: 3
cards: 3
verify: go test ./internal/warp/... ./cmd/lyx/...
depends-on: [2]
```

## Batch Scope

Delivers the warp-specific ergonomics: `clone --reset` + `Long` (W2/W3), `add` `Long`
documenting the fork point (W6), and the `status` → `pairs` rename plus clarified `list`
help (W7), with the pinned `cmd/lyx/helptree_test.go` updated to match. Depends on batch 2
because it co-edits `internal/warp/warp.go` (which batch 2 gave a group `RunE`). Reuse the
reverted `c9d5c59` draft as a starting reference for the clone pieces (`git show c9d5c59` —
the `runCloneWithReset` helper and the clone `Long`); the warp parts of that draft were
reverted only for review, not because they were wrong. Batch-local decision: the rename is a
clean break — no `status` alias is kept.

## Cards

### Card 12: warp clone --reset flag + Long (W2/W3)

- **Context:**
  - `internal/clihelp/exec.go`
  - `internal/warp/warp_test.go`
- **Edits:**
  - `internal/warp/warp.go`
  - `internal/warp/clone.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - In `clone.go`, add `func runCloneWithReset(out io.Writer, args []string, reset bool) int`
    that, when `reset` is true, derives the hub path (`deriveHostName(args[0])` +
    `hubSuffix`, joined onto `paths.Getwd()`), and if that hub dir exists removes it via the
    existing `removeAll` seam before delegating to `runClone(out, args)`. On a non-reset call
    it simply calls `runClone`. Mirror the argument-count guard `runClone` already enforces
    (emit a usage error via `output.Err` for the wrong arg count). Reuse the reverted draft's
    body as reference.
  - In `warp.Command()`, replace the inline `clone` subcommand literal with a
    `cloneCmd` variable (closure pattern, like `removeCmd`): set `Use:`
    `"clone [--reset] <host-url> <weft-url> [board-url]"`, keep `Short`, add a `Long`
    describing the three-repo hub layout (host prime / `<host>-weft` weft prime / `_board`
    board passenger), the derived board URL default (`<weft-url>.wiki.git` when omitted), the
    `lyx init` follow-up, the `--reset` idempotent-reclone behavior, and a concrete example.
    Register `cloneCmd.Flags().Bool("reset", false, "remove an existing hub before cloning (idempotent re-clone)")`
    and a `RunE` (via `clihelp.WrapRun`) that reads `reset` from the flag set and calls
    `runCloneWithReset`. Add `cloneCmd` via `cmd.AddCommand`.
- **Commit:** `feat(warp): add clone --reset and document clone`

### Card 13: warp add Long, status→pairs rename, list clarify (W6, W7)

- **Context:**
  - `internal/warp/add.go`
  - `cmd/lyx/helptree_test.go`
- **Edits:**
  - `internal/warp/warp.go`
  - `cmd/lyx/helptree_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:**
  - `add` (W6): add a `Long` to the `add` subcommand stating the new pair is forked from the
    branch currently checked out in the worktree you run `lyx warp add` from (that worktree's
    `HEAD`) — explicitly not `main` and not prime's branch — and that it errors on a detached
    or unborn `HEAD`. (Behavior already implemented in `add.go` step 6b; this is doc only.)
  - `status` → `pairs` (W7): rename the `status` subcommand's `Use:` to `"pairs"` and rename
    the handler `runStatus` → `runPairs` (and its call site). Update the subcommand `Short`
    to describe it as the full host↔weft pair geometry view. Keep its output identical.
  - `list` clarify (W7): sharpen the `list` subcommand's `Short` (and add a short `Long`) to
    say it lists host worktrees only and to point at `lyx warp pairs` for full pair geometry.
  - `helptree_test.go`: update the warp `wantSubs` set, replacing `"status"` with `"pairs"`.
  - Do not touch the group `RunE` added in batch 2.
- **Commit:** `feat(warp): rename status to pairs, document add and list`

### Card 14: warp feature tests

- **Context:**
  - `internal/warp/clone.go`
  - `internal/warp/warp.go`
  - `internal/warp/add.go`
- **Edits:**
  - `internal/warp/warp_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add tests (follow existing `warp_test.go` patterns, driving via
  `warp.RunCLI`):
  - `clone --reset` removes a pre-existing hub directory then proceeds to clone: inject the
    `removeAll` seam (as existing clone tests do) to assert the hub path was passed to it
    when `--reset` is set, and assert it is NOT called without `--reset`.
  - `warp clone --help` output contains the `--reset` flag and a non-empty `Long` (assert
    substrings like `--reset` and `_board` / `hub`); optionally assert via `--json` that the
    flags list is non-empty.
  - `warp pairs` runs the former status handler (assert it reaches the same resolution path /
    same error or output as the old `status`), and `warp status` is gone — `warp status`
    now returns the JSON `ok:false` `unknown subcommand` envelope (mounted behavior is in
    cmd/lyx; here in isolation it is the Cobra "unknown command" JSON envelope — assert
    `ok:false` and exit 1).
  - `warp add --help` `Long` contains the fork-point wording (assert a substring such as
    `worktree you run` / `HEAD`).
- **Commit:** `test(warp): cover clone --reset, pairs rename, add/list help`

## Batch Tests

`verify: go test ./internal/warp/... ./cmd/lyx/...` exercises the warp package (clone
`--reset`, the `pairs` rename, add/list help) and `cmd/lyx` (the updated `helptree_test.go`
pinned set and the drift guard, which stays green because every renamed/clarified command
keeps a non-empty `Short`). The mounted `warp status → unknown subcommand` behavior is
already covered by batch 2's `cmd/lyx/unknown_subcommand_test.go`; this batch's helptree
update keeps that test consistent with the new subcommand set.
