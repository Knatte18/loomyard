# Batch: gitclone-command

```yaml
task: "ly-git-clone hub-creator (host, weft, board)"
batch: "gitclone-command"
number: 1
cards: 6
verify: go test -tags=integration ./internal/gitclone/ ./cmd/lyx/ ./internal/paths/
depends-on: []
```

## Batch Scope

Delivers the entire `lyx git-clone` feature: a new `internal/gitclone` package (URL-derivation
helpers, clone orchestration with strict-abort teardown, and a JSON `RunCLI` entry), its unit
and integration tests, and the `cmd/lyx/main.go` dispatcher wiring. One batch because it is a
single cohesive Go module plus a two-line dispatcher edit — a Sonnet session holds all of it
comfortably. The external interface is `gitclone.RunCLI(out, args)`, consumed by `main.go`.

Batch-local decisions (beyond `## Shared Decisions`): the orchestration core
(`cloneHub`) takes `cwd`, the three URLs, and returns `(hubPath, error)` so it is testable
without `Chdir`; a package-level `var removeAll = os.RemoveAll` is the seam that lets the
teardown-failure test force a removal error.

## Cards

### Card 1: package + URL-derivation helpers

- **Context:**
  - `internal/weft/weft.go`
- **Edits:** none
- **Creates:**
  - `internal/gitclone/gitclone.go`
- **Deletes:** none
- **Requirements:** Create package `gitclone` with a package-doc header comment (the durable
  design rationale lives here per the CONSTRAINTS documentation-lifecycle rule — no
  `docs/modules/*.md`): it bootstraps a fresh lyx Hub by cloning host + weft + board into
  `<cwd>/<name>-HUB/`, deterministic, with no junctions and no lyx activation (a dormant
  hub). Define unexported constants `hubSuffix = "-HUB"`, `weftSuffix = "-weft"`,
  `boardDirName = "_board"`. Implement `func deriveHostName(rawURL string) string`: return
  the host repo basename — the final path segment of `rawURL`, splitting on both `/` and `:`
  so the SCP form `git@github.com:user/repo.git` works, with a single trailing `.git`
  stripped; return `""` when no basename can be extracted. Implement
  `func deriveBoardURL(weftURL string) string`: strip a single trailing `.git` from `weftURL`
  if present, then append `.wiki.git` (so both `…/weft.git` and `…/weft` yield
  `…/weft.wiki.git`). Godoc each function per the golang-comments conventions.
- **Commit:** `feat(gitclone): add package with URL-derivation helpers`

### Card 2: unit tests for URL derivation

- **Context:**
  - `internal/output/output_test.go`
- **Edits:** none
- **Creates:**
  - `internal/gitclone/gitclone_test.go`
- **Deletes:** none
- **Requirements:** Same-package (`package gitclone`) table-driven tests. `TestDeriveHostName`
  covers `https://github.com/u/repo.git`→`repo`, `https://github.com/u/repo`→`repo`,
  `git@github.com:u/repo.git`→`repo`, and an empty/garbage input → `""`. `TestDeriveBoardURL`
  covers `https://github.com/u/weft.git`→`https://github.com/u/weft.wiki.git` and
  `https://github.com/u/weft`→`https://github.com/u/weft.wiki.git`. Use the `tt`/`t.Run`
  pattern and the `Func(input) = got; want want` error message format.
- **Commit:** `test(gitclone): unit-test URL derivation`

### Card 3: clone orchestration with strict-abort teardown

- **Context:**
  - `internal/git/git.go`
  - `internal/board/git.go`
  - `internal/paths/paths.go`
- **Edits:** none
- **Creates:**
  - `internal/gitclone/clone.go`
- **Deletes:** none
- **Requirements:** Declare the testability seam `var removeAll = os.RemoveAll`. Implement
  `func cloneHub(cwd, hostURL, weftURL, boardURL string) (hubPath string, err error)`:
  (1) `name := deriveHostName(hostURL)`; if `""` return an error "could not derive repo name
  from host URL <hostURL>". (2) `hubPath := filepath.Join(cwd, name+hubSuffix)`. (3) if
  `os.Stat(hubPath)` succeeds, return an error "hub already exists at <hubPath>" **without**
  removing it (we did not create it). (4) `os.MkdirAll(hubPath, 0o755)`. (5) clone host to
  `filepath.Join(hubPath, name)`; on failure call the teardown helper and return. (6) clone
  weft to `filepath.Join(hubPath, name+weftSuffix)`; on failure teardown and return. (7)
  resolve board URL: `board := boardURL`; if `board == ""` then `board = deriveBoardURL(weftURL)`;
  clone board to `filepath.Join(hubPath, boardDirName)`; on failure teardown and return. (8)
  return `hubPath, nil`. Add `func cloneRepo(url, dest, runFromDir string) error` wrapping
  `git.RunGit([]string{"clone", url, dest}, runFromDir)` — non-zero exit returns an error
  carrying stderr. Add `func teardownHub(hubPath string, cause error) error`: call
  `removeAll(hubPath)`; if it fails, return an error combining `cause` with "residual hub
  left at <hubPath>; remove it manually before retrying"; otherwise return `cause` unchanged.
  All git goes through `git.RunGit`; no raw `exec`; no `os.Getwd` (cwd is a parameter).
- **Commit:** `feat(gitclone): clone orchestration with strict-abort teardown`

### Card 4: RunCLI entry with JSON output

- **Context:**
  - `internal/weft/cli.go`
  - `internal/output/output.go`
  - `internal/paths/paths.go`
- **Edits:** none
- **Creates:**
  - `internal/gitclone/cli.go`
- **Deletes:** none
- **Requirements:** Implement `func RunCLI(out io.Writer, args []string) int`. Validate
  positional args: accept exactly 2 or 3 (`hostURL`, `weftURL`, optional `boardURL`);
  otherwise `output.Err(out, "usage: lyx git-clone <host-url> <weft-url> [board-url]")`.
  Obtain cwd via `paths.Getwd()`; on error `output.Err`. Call
  `cloneHub(cwd, host, weft, boardOrEmpty)`; on error `output.Err(out, err.Error())`; on
  success `output.Ok(out, map[string]any{"hub": hubPath, "host": host, "weft": weft,
  "board": resolvedBoardURL})` where `resolvedBoardURL` is the explicit board URL or the
  derived one. Include a short godoc on `RunCLI` noting the precondition that the board repo
  (default: the weft repo's wiki) must already exist, or the board clone — and thus the whole
  command — aborts.
- **Commit:** `feat(gitclone): RunCLI entry with JSON output`

### Card 5: integration tests

- **Context:**
  - `internal/weft/weft_integration_test.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/git/git.go`
  - `internal/paths/paths.go`
- **Edits:** none
- **Creates:**
  - `internal/gitclone/clone_integration_test.go`
- **Deletes:** none
- **Requirements:** File begins with `//go:build integration`, `package gitclone`. Add a local
  helper `makeBareRemote(t *testing.T, dir, name string) string` using `lyxtest.MustRun` to
  `git init --bare` a repo and seed one commit on branch `main` (init a working clone, commit a
  README, push) so a later `git clone` checks out a branch; return the bare repo path (used as a
  clone URL). Tests, all driving `cloneHub` with `t.TempDir()` as `cwd`:
  `TestCloneHub_HappyPath` (omit board URL; create the derived weft-wiki bare at
  `<dir>/<weft>.wiki.git`; assert `<name>-HUB/<name>`, `<name>-HUB/<name>-weft`,
  `<name>-HUB/_board` all exist and are git repos, the Hub root is **not** a git repo, and no
  `_lyx`/`_codeguide` were created); `TestCloneHub_GeometryRoundTrip` (`paths.Resolve` of the
  cloned host Prime yields `Hub == hubPath`, `PrimeName == name`, `WeftRepoRoot() ==
  filepath.Join(hubPath, name+"-weft")`); `TestCloneHub_ExplicitBoardURL` (pass an explicit
  board fixture; assert it is used, not the derived URL); `TestCloneHub_AbortIfExists`
  (pre-create `<cwd>/<name>-HUB`; assert `cloneHub` errors and leaves the pre-existing dir
  untouched); `TestCloneHub_StrictAbort` with `t.Run` subtests for host/weft/board each pointed
  at a non-existent remote — assert an error is returned **and** the Hub dir was fully removed;
  `TestCloneHub_TeardownFailure` (override `removeAll` to return an error, restore via
  `t.Cleanup`; trigger a clone failure; assert the returned error mentions the residual hub
  path and the Hub still exists). Use only local on-disk fixtures — no network.
- **Commit:** `test(gitclone): integration tests for clone, abort, teardown`

### Card 6: wire git-clone into the lyx dispatcher

- **Context:**
  - `internal/weft/cli.go`
- **Edits:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/main_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `cmd/lyx/main.go`, add the import
  `github.com/Knatte18/loomyard/internal/gitclone`, add `case "git-clone": return
  gitclone.RunCLI(out, moduleArgs)` to the `run` switch, and add a `git-clone` line to the
  Modules list in the package doc comment. In `cmd/lyx/main_test.go`, add
  `TestRunDispatchesToGitClone` that drives `run([]string{"git-clone"}, &out)` (too few args)
  and asserts exit code 1 plus `"ok":false` JSON on `out`, mirroring `TestRunDispatchesToWeft`.
- **Commit:** `feat(gitclone): wire git-clone into lyx dispatcher`

## Batch Tests

`verify: go test -tags=integration ./internal/gitclone/ ./cmd/lyx/ ./internal/paths/`.
Three focused packages, not the full tree:
- `./internal/gitclone/` — the new unit tests (Card 2) and integration tests (Card 5); the
  `-tags=integration` flag pulls in the tagged file while still running the untagged unit tests.
- `./cmd/lyx/` — `TestRunDispatchesToGitClone` plus the existing dispatcher tests, confirming
  the new `case` compiles and routes.
- `./internal/paths/` — runs `enforcement_test.go`, which scans the whole source tree (including
  the new `internal/gitclone` files) and fails if any banned `os.Getwd` / `rev-parse
  --show-toplevel` slipped in. Included specifically to guard the path-invariant decision.
