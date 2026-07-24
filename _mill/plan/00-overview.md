# Plan: gitrepo: generic, repo-agnostic git primitives

```yaml
task: 'gitrepo: generic, repo-agnostic git primitives'
slug: gitrepo
approved: true
started: '20260724-175205'
parent: main
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: core-repo
    file: 01-core-repo.md
    depends-on: []
    verify: go test -tags integration ./internal/gitrepo/
  - number: 2
    name: push
    file: 02-push.md
    depends-on: [1]
    verify: go test -tags integration ./internal/gitrepo/
  - number: 3
    name: snapshot
    file: 03-snapshot.md
    depends-on: [1]
    verify: go test -tags integration ./internal/gitrepo/
  - number: 4
    name: docs-lifecycle
    file: 04-docs-lifecycle.md
    depends-on: [1, 2, 3]
    verify: go build ./internal/gitrepo/
```

## Shared Decisions

### Decision: build on gitexec, never shell out directly

- **Decision:** every git invocation goes through `gitexec.RunGit(args []string, cwd string)
  (stdout, stderr string, exitCode int, err error)` with `cwd = r.path`. `gitrepo` never calls
  `exec.Command` itself. A single unexported helper on `Repo` (e.g. `run(args ...string)
  (stdout, stderr string, code int, err error)`) wraps `gitexec.RunGit` with `r.path` and is
  reused by every method.
- **Rationale:** `gitexec` stays the zero-dependency leaf; `gitrepo` is one of its consumers
  (discussion.md "gitexec stays a separate leaf layer"). Non-zero git exit is **not** a Go error
  in `gitexec` (err is nil, exitCode carries it); only spawn failures return non-nil err with
  exitCode -1 — every method interprets exitCode/stderr itself.
- **Applies to:** all batches

### Decision: exit-code / error interpretation posture

- **Decision:** methods that answer a yes/no or best-effort question swallow git failure into the
  safe answer (`SHAExists` → `false`; `SnapshotSHA` pre-read fetch failure → degrade to local ref).
  Methods that must produce a value return a typed error on genuine git failure (`CurrentSHA` →
  `ErrNoCommits` on an empty repo; `ChangedFilesSince` → error on a missing SHA). "Nothing to
  commit" is a **signal, not an error** (`StageAndCommit` third return `committed bool`).
- **Rationale:** matches the self-correcting staleness model in discussion.md — "when in doubt,
  rebuild" — while still surfacing genuine value-producing failures.
- **Applies to:** all batches

### Decision: explicit file lists, never `add -A`; commit and push are separate

- **Decision:** `StageAndCommit` always stages an explicit `git add -- <files...>`; `gitrepo` never
  wildcard-stages anywhere. `Push`/`PushCoalesced` are **push-only** — the commits are made
  beforehand via `StageAndCommit`. This keeps `add -A` out of the package entirely.
- **Rationale:** a wildcard stage risks silently committing an unrelated leftover file; separating
  commit from push lets `PushCoalesced` replace board's `sync.go` push loop without pulling in
  `add -A` (discussion.md "Push surface" + "Lock ownership").
- **Applies to:** core-repo, push

### Decision: hermetic, integration-tagged tests; one Tier-1 exception

- **Decision:** every git-spawning test file carries `//go:build integration` and lives in
  external package `gitrepo_test`; a single `testmain_test.go` (also `//go:build integration`)
  supplies `TestMain` calling `lyxtest.HermeticGitEnv()` before `m.Run()`. The one pure-logic test
  (`validSnapshotKey`) is **untagged**, in internal package `gitrepo`, and spawns no git — so it
  runs in Tier 1 and must contain none of the banned spawn tokens (`gitexec.RunGit`,
  `exec.Command`, `lyxtest.Copy*`).
- **Rationale:** Hermetic Git Test Environment Invariant + Test Tier Purity Invariant
  (CONSTRAINTS.md). Mirrors `internal/gitexec`'s `testmain_test.go` exactly. Real git against
  throwaway repos built with `lyxtest.MustRun(t, dir, "git", "init", ...)` under `t.TempDir()`.
- **Applies to:** all batches with tests (core-repo, push, snapshot)

### Decision: verify command shape (Go project)

- **Decision:** batch `verify:` uses the native Go runner with the integration tag —
  `go test -tags integration ./internal/gitrepo/` — no `PYTHONPATH=` prefix (that rule is
  Python-only). The docs batch uses `go build ./internal/gitrepo/`. Layout is non-nested
  (`hub == git_root`), so the plain-string form correctly implies `cwd: git_root`.
- **Rationale:** the integration tag gates the git-spawning tests; the docs batch only needs a
  compile check that `doc.go` still builds.
- **Applies to:** all batches

## All Files Touched

- `docs/overview.md`
- `internal/gitrepo/doc.go`
- `internal/gitrepo/gitrepo.go`
- `internal/gitrepo/gitrepo_test.go`
- `internal/gitrepo/keyvalidation_test.go`
- `internal/gitrepo/push.go`
- `internal/gitrepo/push_test.go`
- `internal/gitrepo/snapshot.go`
- `internal/gitrepo/snapshot_test.go`
- `internal/gitrepo/testmain_test.go`
- `manifest/designs/fabric.md`
- `manifest/designs/semantic-index.md`
- `manifest/roadmap.md`
