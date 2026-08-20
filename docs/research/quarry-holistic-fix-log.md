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

## Round 2 — holistic review r2 fixes

Fixed findings from the round-2 holistic review, both landed in
`/home/knatte/Code/quarry/wts/quarry` (repo `github.com/Knatte18/quarry`, branch `main`):

- **[BLOCKING] `lspclient.go`'s slog Warn-level handler governs only 2 of 7 ported log call
  sites** — routed the remaining five `slog.Warn` call sites in `lspclient.go` through
  `defaultLogHandler`, matching `ensureserver.go`'s convention so every warn-level log call in the
  package is governed by the same handler seam.
- **[NIT:consistency] `docs/port-equivalence.md`'s comparison-count claim was stale** — corrected
  the stated comparison count from 26 to 27 to match the actual number of envelope comparisons the
  document describes.

### Quarry commit SHAs

```
80c2ccc fix(quarry): route all lspclient.go Warn calls through defaultLogHandler
21835aa docs(quarry): fix comparison count in port-equivalence.md (26 -> 27)
```

`go -C /home/knatte/Code/quarry/wts/quarry test ./internal/...`, `go -C
/home/knatte/Code/quarry/wts/quarry test ./...`, and `go -C /home/knatte/Code/quarry/wts/quarry
test -tags lsp ./... -count=1` (with `gopls` on `$PATH`) all green. `go test ./...` in this
worktree is unaffected and stays green.

## Round 3 — holistic review r3 fixes

Fixed findings from `_mill/reviews/20260820-171211-code-review-r3.md`, all `[NIT:consistency]`.
Three landed in `/home/knatte/Code/quarry/wts/quarry` (repo `github.com/Knatte18/quarry`, branch
`main`); this section itself is the fourth finding's fix, landed in this worktree:

- **[NIT:consistency] `toolchain.go`'s header still called the cache "scout-owned"** — reworded to
  "quarry-owned" to match the renamed cache path segment and package identity.
- **[NIT:consistency] `registry.go`'s doc comment cited the dropped `ConfigTemplate` symbol** —
  reworded the parenthetical to point at `docs/servers.yaml.example` instead.
- **[NIT:consistency] `resolveContext`'s `cwd` parameter was dead** — dropped `cwd` from
  `resolveContext`'s signature, its four call sites in `internal/cli/cli.go`, and the now-unused
  `cwd` locals in `internal/cli/resolve_test.go`'s nine subtests.
- **[NIT:consistency] this log had no Round 2 entry** — added the `## Round 2` section above, per
  this file's own stated convention.

### Quarry commit SHA

```
a510c87 fix: address round-3 holistic review NIT-consistency findings
```

`go -C /home/knatte/Code/quarry/wts/quarry build ./...`, `go -C /home/knatte/Code/quarry/wts/quarry
test ./internal/cli/...`, `gofmt -l .`, and `go vet ./...` all clean. `go test ./...` in this
worktree is unaffected and stays green.
