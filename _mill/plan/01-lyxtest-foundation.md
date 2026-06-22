# Batch: lyxtest-foundation

```yaml
task: "Optimise and slim the test suite"
batch: "lyxtest-foundation"
number: 1
cards: 4
verify: go test -tags integration ./internal/lyxtest/...
depends-on: []
```

## Batch Scope

Create the shared test-support package `internal/lyxtest` that all three package batches consume. It owns the git-fixture machinery: build the heavy template repos **once per test binary** (`sync.Once`), and hand each test an **isolated filesystem copy** with zero per-test git spawns. This is the root batch — batches 2, 3, and 4 depend on the public API defined here (see overview "Decision: lyxtest public API"). Build it first and prove the copy isolation + upstream-tracking preservation with the package's own integration-tagged tests. External interface consumed downstream: `MustRun`, `CopyHostHub`, `CopyPaired`, `CopyWeft`, and their fixture structs.

## Cards

### Card 1: lyxtest package skeleton + MustRun + doc.go

- **Context:**
  - `internal/worktree/testhelpers_test.go`
  - `internal/board/boardtest/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/lyxtest/lyxtest.go`
  - `internal/lyxtest/doc.go`
- **Deletes:** none
- **Requirements:** Create package `lyxtest` (normal package, not `_test`). Add `func MustRun(tb testing.TB, dir string, args ...string)` that runs `exec.Command(args[0], args[1:]...)` with `cmd.Dir = dir`, uses `CombinedOutput()`, calls `tb.Helper()` and `tb.Fatalf` on non-zero exit — semantically identical to the existing worktree `mustRun`. Add `doc.go` with a package comment explaining lyxtest is the shared git-fixture support package (template-built-once + per-test filesystem copy) for `worktree`/`weft`/`paths` tests, modelled on `boardtest/doc.go`'s convention note. `lyxtest.go` is the home for the builders and copy helpers added in cards 2–3.
- **Commit:** `feat(lyxtest): add lyxtest package skeleton with MustRun`

### Card 2: cached template builders

- **Context:**
  - `internal/worktree/testhelpers_test.go`
  - `internal/weft/sync_test.go`
  - `internal/paths/helpers_test.go`
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/lyxtest/lyxtest.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add unexported `sync.Once`-guarded template builders that each build a template ONCE per test binary into a stable temp location and return its path. Templates: (a) **host-hub** — `git init -b main` + `git config user.email/name` (or `git -c`/`GIT_*` env on the init commit) + a `README` + `add`/`commit`, plus a **bare** remote (`git init --bare`) with `git remote add origin <bare>` — **leave the bare empty (do NOT `push -u origin main`)**, matching the existing worktree/paths `addRemote` semantics where `Add` populates the bare via `push -u <newbranch>`; (b) **weft-prime** sibling matching worktree `newWeftRepo` (`<base>-weft` dir, `_lyx/config/placeholder`, init+commit) plus its own bare, **also left empty** (worktree weft tests pass `AddOptions{SkipPush:true}`, so the weft-prime bare is never pre-populated); (c) **weft-only** matching weft `newTestWeftRepo` (init+commit with `_lyx/config.yaml`) plus a bare WITH `git push -u origin main` — this is the only template that needs upstream tracking, because weft `Pull --ff-only`/`hasUnpushed` (`@{u}`) require it (matches the existing `addWeftRemote` which pushes). Replicate the exact repo shape each existing helper produces (read `testhelpers_test.go` `newTestRepo`/`newWeftRepo`, `weft/sync_test.go` `newTestWeftRepo`/`addWeftRemote`, `paths/helpers_test.go` `newTestRepo`). Builders must NOT depend on `t` — use `testing.TB` only where a failure must be reported, or panic on setup failure (document the choice). Honour the overview invariant: each `.git/config` has exactly one `origin` / one `url` line. **Enforcement-guard constraint:** `lyxtest.go` is non-test production code, so it must NEVER contain `os.Getwd` or the string `--show-toplevel` — `internal/paths/enforcement_test.go` fails the build otherwise. Use `paths.Resolve(hub)` (card 3) for any geometry; never shell out to `git rev-parse --show-toplevel`.
- **Commit:** `feat(lyxtest): build host/paired/weft templates once via sync.Once`

### Card 3: per-test copy helpers with origin url rewrite

- **Context:**
  - `internal/paths/paths.go`
  - `internal/worktree/testhelpers_test.go`
- **Edits:**
  - `internal/lyxtest/lyxtest.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add the public copy helpers `CopyHostHub(tb testing.TB) HostFixture`, `CopyPaired(tb testing.TB) PairedFixture`, `CopyWeft(tb testing.TB) WeftFixture`. Each calls the matching card-2 builder, recursively copies the cached template tree(s) into `tb.TempDir()` (a pure filesystem copy — no `exec`/git), and for any repo with a remote repoints `origin` by reading the copied `.git/config`, replacing the single `url = …` line under `[remote "origin"]` with the copied bare's path (text edit; assert exactly one match), and writing it back. Return fixture structs exposing the absolute paths named in the overview (`HostFixture{Hub, Bare}`, `PairedFixture{Container, Hub, Bare, WeftPrime, WeftBare, Layout}`, `WeftFixture{WeftPath, Bare}`); construct the `*paths.Layout` for `PairedFixture` via `paths.Resolve(hub)` so callers get geometry for free. Only `CopyWeft`'s repo carries upstream tracking (from template c); `CopyHostHub`/`CopyPaired` bares are empty by design. Copies must be fully independent (mutating one must not affect another or the template). **Enforcement-guard constraint:** never use `os.Getwd` or `--show-toplevel` in `lyxtest.go`; `paths.Resolve(hub)` is the only geometry primitive.
- **Commit:** `feat(lyxtest): add isolated per-test copy helpers`

### Card 4: lyxtest integration tests

- **Context:**
  - `internal/lyxtest/lyxtest.go`
  - `internal/board/boardtest/integration_test.go`
- **Edits:** none
- **Creates:**
  - `internal/lyxtest/lyxtest_test.go`
- **Deletes:** none
- **Requirements:** Create `lyxtest_test.go` with first line `//go:build integration`, blank line, then `package lyxtest`. Cover: (a) `CopyHostHub`/`CopyPaired`/`CopyWeft` each return a valid independent git repo — `git rev-parse HEAD` resolves in the copy; (b) `origin` url points at the copied bare, not the template's (assert via reading `.git/config` or `git remote get-url origin`); (c) upstream tracking survives — `git rev-parse @{u}` succeeds and `git rev-list --count @{u}..HEAD` is `0` on a fresh `CopyWeft`; (d) isolation — mutating one copy (e.g. add+commit a file) leaves a second copy unaffected; (e) `MustRun` runs a command and the helper exists. Use `t.Parallel()` on independent cases. These tests legitimately spawn git, hence the integration tag.
- **Commit:** `test(lyxtest): verify copy isolation and upstream preservation`

## Batch Tests

`verify: go test -tags integration ./internal/lyxtest/...` runs the new `lyxtest_test.go`. The untagged `go test ./internal/lyxtest/...` must also compile (the package has no untagged tests, so it builds clean). No equivalence-guardrail diff applies here — this is a new package, not a migration. Cards 2–3 are the TDD candidates; card 4's assertions define the contract (copy validity, origin rewrite, upstream preservation, isolation).
