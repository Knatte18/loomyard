# Batch: lyxtest-tidy

```yaml
task: "Move config templates home by removing the lyxtest->configreg edge"
batch: lyxtest-tidy
number: 3
cards: 2
verify: go vet -tags integration ./... && go test -tags integration ./...
depends-on: [2]
```

## Batch Scope

A pure-refactor cleanup of `internal/lyxtest`, isolated as the final batch so a regression is easy to
localize away from the behavior changes in batches 1–2. It extracts the duplicated git-fixture
boilerplate into shared helpers, audits for dead exported helpers, and resolves the `buildWeftOnly`
fixture-path asymmetry plus the raw-literal `_lyx` paths in `weft_integration_test.go` flagged by the
constraint rule. No fixture *content* changes beyond aligning paths; the full integration suite must
stay green throughout. Depends on batch 2.

## Cards

### Card 11: Extract shared git-fixture helpers; dead-helper audit

- **Context:**
  - `internal/lyxtest/doc.go`
- **Edits:**
  - `internal/lyxtest/lyxtest.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Extract the repeated git boilerplate in `buildHostHub`, `buildWeftPrime`, and
  `buildWeftOnly` into unexported helpers and call them from all three: `initRepo(dir string)` (`git
  init -b main` + `git config user.email`/`user.name`), `commitAll(dir, message string)` (`git add
  .` + `git commit -m`), and `initBareRemote(dir, remoteURL string)` (create bare via `git init
  --bare` + `git remote add origin`). Preserve the existing panic-on-error behavior (fixture
  construction errors are unrecoverable) and the exact command semantics (branch name `main`, user
  `Test`/`test@test.com`, the `push -u` that only `buildWeftOnly` performs stays in `buildWeftOnly`).
  Then audit exported `lyxtest` identifiers (`MustRun`, `SeedConfig`, `CopyHostHub`, `CopyPaired`,
  `CopyPairedLocal`, `CopyWeft`, the fixture structs) for callers across the repo; remove any
  exported helper with zero callers. Do not change any public signature that has callers.
- **Commit:** `refactor(lyxtest): extract initRepo/commitAll/initBareRemote, drop dead helpers`

### Card 12: Resolve buildWeftOnly asymmetry and the raw `_lyx` literals

- **Context:**
  - `internal/paths/paths.go`
- **Edits:**
  - `internal/lyxtest/lyxtest.go`
  - `internal/weft/weft_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** First confirm which consumers read `buildWeftOnly`'s literal `_lyx/config.yaml`
  path: `TestPushIntegration` (`weft_integration_test.go`) writes `filepath.Join(weftRepo, "_lyx",
  "config.yaml")` and commits pathspec `["_lyx"]`. Resolve the `buildWeftOnly` asymmetry (it writes a
  singular `_lyx/config.yaml` while `buildWeftPrime` uses `paths.ConfigDir(...)` under
  `_lyx/config/`) by **either** aligning `buildWeftOnly` to the `paths` helpers and updating that
  consumer in the same card, **or** adding a clear comment on `buildWeftOnly` explaining why its
  minimal `_lyx/config.yaml` differs (its consumers only need *some* tracked file under `_lyx`, not
  real config) — choose whichever keeps the suite green with least churn. Separately, replace the
  raw-literal `_lyx` paths in `weft_integration_test.go` with the helper: change
  `filepath.Join(fixture.WeftPrime, "_lyx", "placeholder")` to use `paths.LyxDirName`, and apply the
  same to the `_lyx/config.yaml` literal if `buildWeftOnly` is aligned. Do not alter the
  batch-1 `SeedConfig` call already present in `TestRunCLI_EnvMapToOption`.
- **Commit:** `refactor(lyxtest): align buildWeftOnly fixture path and _lyx literals to paths helpers`

## Batch Tests

`verify: go vet -tags integration ./... && go test -tags integration ./...`. No production code
changes in this batch, so `go build ./...` is covered by the test build. The full integration suite
is the justified scope because `lyxtest` is imported by nearly every integration test package, and
this batch refactors the fixture builders all of them depend on. Key tests: `TestPushIntegration` and
`TestRunCLI_EnvMapToOption` (weft, the `CopyWeft`/`CopyPaired` consumers touched here), plus the
broad fixture consumers in `worktree`, `configcli`, `ide`, `gitclone`, `paths` must all stay green.
