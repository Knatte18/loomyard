# Batch: preflight-prereqs

```yaml
task: 'loom: Preflight phase (precondition validation)'
batch: preflight-prereqs
number: 1
cards: 3
verify: go test ./internal/state/ ./internal/hubgeometry/ && go test -tags integration -run TestHostClean ./internal/warpengine/
depends-on: []
```

## Batch Scope

Three small, independent helper additions across existing packages that `loomengine.Preflight`
(batch 2) depends on: a strict, read-only JSON reader with distinguishable read/decode errors
(`internal/state`), a WorktreeRoot-anchored `_lyx/status.json` path accessor plus its lock-path
sibling (`internal/hubgeometry`), and a host-worktree cleanliness check that treats untracked
files as dirty (`internal/warpengine`). They are grouped as one batch because each is a single
small function that logically exists only to serve Preflight, and none depends on another. The
external interface batch 2 consumes: `state.ReadJSONStrict[T]` + `state.ErrRead`/`state.ErrDecode`,
`(*hubgeometry.Layout).LoomStatusFile()`/`.LoomStatusLock()`, and `warpengine.HostClean`.

## Cards

### Card 1: state.ReadJSONStrict + read/decode sentinels

- **Context:**
  - `internal/lock/lock.go`
  - `internal/fsx/fsx.go`
- **Edits:**
  - `internal/state/state.go`
- **Creates:**
  - `internal/state/strict_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/state/state.go` add two exported sentinels
  `var ErrRead = errors.New("state: read failed")` and
  `var ErrDecode = errors.New("state: decode failed")`, and a new generic
  `func ReadJSONStrict[T any](path, lockPath string) (T, bool, error)`. It mirrors `ReadJSON`
  (acquire `lock.AcquireReadLock(lockPath)`, defer `Release`, return `(zero, false, nil)` on
  `os.IsNotExist`) with three differences: (1) it does **not** call `os.MkdirAll` — a read must
  not create directories; (2) it decodes strictly via
  `d := json.NewDecoder(bytes.NewReader(data)); d.DisallowUnknownFields()` then `d.Decode(&v)`
  instead of `json.Unmarshal`; (3) it wraps the `os.ReadFile` failure as
  `fmt.Errorf("%w: %v", ErrRead, err)` and the decode failure as
  `fmt.Errorf("%w: %v", ErrDecode, err)` so callers classify via `errors.Is`. The
  `lock.AcquireReadLock` failure is returned wrapped as today (neither sentinel — it is a third,
  infra mode the caller escalates). Godoc the sentinels and the read-only / strict / classification
  behaviour. `strict_test.go` (untagged, temp-file only, no git) covers: valid decode →
  `(v, true, nil)`; unknown field → `errors.Is(err, ErrDecode)`; malformed JSON →
  `errors.Is(err, ErrDecode)`; missing file → `(zero, false, nil)`; and confirms no directory is
  created for a missing parent (read-only).
- **Commit:** `feat(state): add strict read-only ReadJSONStrict with ErrRead/ErrDecode sentinels`

### Card 2: hubgeometry LoomStatusFile + LoomStatusLock accessors

- **Context:**
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/hubgeometry/hubgeometry.go`
- **Creates:**
  - `internal/hubgeometry/loomstatus_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/hubgeometry/hubgeometry.go` add two `*Layout` methods:
  `func (l *Layout) LoomStatusFile() string { return filepath.Join(l.WorktreeRoot, LyxDirName, "status.json") }`
  and `func (l *Layout) LoomStatusLock() string { return filepath.Join(l.WorktreeRoot, LyxDirName, "status.json.lock") }`.
  Both are **WorktreeRoot-anchored**, deliberately NOT built on `LyxDir()` (which is `Cwd`-anchored
  — hubgeometry.go:319 — and would misread the seed from a worktree subdirectory). Godoc each,
  stating the WorktreeRoot anchoring and its rationale. `loomstatus_test.go` (untagged, pure path,
  no spawn) asserts each equals `filepath.Join(layout.WorktreeRoot, hubgeometry.LyxDirName, "status.json"[".lock"])`
  for a hand-built `Layout` (do not hardcode the `_lyx` literal — use the exported `LyxDirName`
  constant, respecting the Hub Geometry Invariant), including a case where `Cwd != WorktreeRoot`
  to prove the accessor ignores `Cwd`.
- **Commit:** `feat(hubgeometry): add WorktreeRoot-anchored LoomStatusFile/LoomStatusLock accessors`

### Card 3: warpengine.HostClean

- **Context:**
  - `internal/warpengine/add.go`
  - `internal/warpengine/drift.go`
  - `internal/gitexec/gitexec.go`
  - `internal/warpengine/testmain_test.go`
- **Edits:** none
- **Creates:**
  - `internal/warpengine/hostclean.go`
  - `internal/warpengine/hostclean_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a package-level
  `func HostClean(l *hubgeometry.Layout) (clean bool, reason string, err error)` in
  `internal/warpengine/hostclean.go` (package-level like `PairInSync` in `drift.go`, not a
  `*Worktree` method, since Preflight calls it standalone). It runs
  `gitexec.RunGit([]string{"status", "--porcelain"}, l.WorktreeRoot)` — **bare `--porcelain`, so
  untracked files count as dirty** (deliberately stricter than `add.go`'s
  `--untracked-files=no`). Returns `(false, "", err)` on spawn error or a non-zero `exitCode`
  (wrap with context); `clean = strings.TrimSpace(stdout) == ""`; when dirty, `reason` is the
  trimmed porcelain output so the operator sees which paths are dirty. Godoc it, noting the
  untracked-is-dirty policy and the host-repo-is-unrestricted rationale (Weft Git Invariant).
  `hostclean_test.go` is `//go:build integration`-tagged (it spawns git); the package's existing
  `testmain_test.go` already provides the `HermeticGitEnv` `TestMain`. Name the top-level test
  `TestHostClean` (the batch `verify` selects it via `-run TestHostClean`). Cover: a clean
  committed worktree → `(true, "", nil)`; a tracked-modified file → `(false, <reason>, nil)`; and
  an **untracked-only** file → `(false, <reason>, nil)` (the strict-policy case). Build fixtures
  with a lightweight `git init` + commit via `lyxtest.MustRun`, mirroring existing warpengine
  integration tests.
- **Commit:** `feat(warpengine): add HostClean host-worktree cleanliness check (untracked = dirty)`

## Batch Tests

`verify: go test ./internal/state/ ./internal/hubgeometry/ && go test -tags integration -run TestHostClean ./internal/warpengine/`

- The first invocation runs `internal/state` and `internal/hubgeometry` untagged (Tier 1): it
  covers Card 1's `strict_test.go` and Card 2's `loomstatus_test.go`, and also re-runs
  `internal/hubgeometry`'s `TestEnforcement_GeometryLiterals` guard, confirming Card 2 introduced
  no banned `_lyx` literal.
- The second invocation runs only Card 3's `TestHostClean` in `internal/warpengine` with the
  `integration` tag (via `-run`), so the batch does not pay for warpengine's full, slow
  clone/worktree integration suite while still exercising the new git-spawning helper under the
  hermetic `TestMain`.
