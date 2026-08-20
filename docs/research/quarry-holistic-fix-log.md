# quarry holistic-fix log

This is a transient record for holistic-review fix rounds against the "Extract scout into its own
standalone repo" task, run after `06-lyx-removal` had already landed and the task's own
`docs/research/quarry-port-log.md` was closed.
Each round that lands quarry-side fixes appends its own `## Round N` section here, so a
quarry-only fix round still produces a task-worktree commit — the same rationale the closed port
log recorded under `two-repo-worktree-authorization` in `_mill/discussion.md`.

## Round 1 — holistic review r1 fixes

Fixed findings from `_mill/reviews/20260820-163952-code-review-r1.md`, all landed in
`/home/knatte/Code/quarry/wts/quarry` (repo `github.com/Knatte18/quarry`, branch `main`):

- **[BLOCKING:consistency] stale go.sum for the windows target** — `go mod tidy` against the
  final tree (including `internal/cli`) added the missing full `h1:` content hash for
  `github.com/inconshreveable/mousetrap` and listed it as an explicit indirect require.
  Verified `GOOS=windows go build ./...` and `go build ./...` both succeed under
  `-mod=readonly`.
- **[BLOCKING:scope] Loomyard/mill-internal residue in ported comments** — swept the `quarry/`
  package and `internal/cli` for `scoutcli`, `scoutengine package`, `modelspec`,
  `manifest/designs`, and bare `batch N` references the original `lyx`-substring sweep missed;
  rewrote each to describe the shipped architecture on its own terms.
- **[NIT:consistency] garbled cross-reference in `docs/port-equivalence.md`** — rewritten as a
  single valid pointer to quarry's own `docs/scout-multilang.md`.
- **[NIT:consistency] card 32's "grep -ric 'lyx'" zero-count claim** — pushed back, no code
  change: the 9 remaining hits are the README's card-2-mandated "Upgrading from `lyx scout`"
  section and similar deliberate, justified mentions, not residue.

While re-running this round's `verify:` gate, also fixed a pre-existing flaky test unrelated to
any review finding: `TestResolveConfigPath_UserConfigDirError`,
`TestResolveStateDir_UserCacheDirError`, and `TestRunCLIIn_TargetDirResolvesAgainstInjectedSeamCwd`
each mutate the package-level `userConfigDir`/`userCacheDir` seams in `internal/cli` but were
marked `t.Parallel()`, racing against each other and against every other seam-reading test in the
package. Removed `t.Parallel()` from all three, matching the rest of the package's convention.

### Quarry commit SHAs

```
9cab113 chore(deps): tidy go.mod/go.sum for the final internal/cli tree
9bc4272 docs(quarry): strip stale Loomyard/mill-internal cross-references from ported comments
72cf97d docs(quarry): fix garbled cross-reference in port-equivalence.md intro
3cd5064 test(cli): remove t.Parallel() from tests that mutate package-level path seams
```

`go -C /home/knatte/Code/quarry/wts/quarry test ./internal/...`, `go -C
/home/knatte/Code/quarry/wts/quarry test ./...`, and `go -C /home/knatte/Code/quarry/wts/quarry
test -tags lsp ./... -count=1` (with `gopls` on `$PATH`) all green, each re-run 5x with `-race` to
confirm the test-race fix held. `go test ./...` in this worktree is unaffected and stays green.
