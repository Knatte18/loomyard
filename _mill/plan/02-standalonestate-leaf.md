# Batch: standalonestate-leaf

```yaml
task: "lift the orchestrator preflight out of loomengine, plus the shared standalone-CLI foundations"
batch: "standalonestate-leaf"
number: 2
cards: 4
verify: go test ./internal/standalonestate/... ./internal/lyxcwd/... && go test -tags integration ./internal/standalonestate/...
depends-on: []
```

## Batch Scope

This batch lands `internal/standalonestate`, a second stdlib-only leaf owning the derivation from a target directory to a `hash8` and a per-OS state directory.
It is one batch because the package is one exported function over one unexported, fully injectable seam, and its tests are the whole point — without the seam, exactly one of the two `<state>` table rows would ever be exercised in CI, because `runtime.GOOS` is a compile-time constant.
Nothing consumes this package at the end of this task; `burlercli`, `perchcli` and `webstercli` wire it up in T7 and T8. That absence is expected, not an oversight.
This batch has no dependency on any other batch and shares no file with one.
No batch-local decisions differ from `## Shared Decisions` in the overview.

## Cards

### Card 5: create the internal/standalonestate leaf

- **Context:**
  - `internal/lyxcwd/anchor.go`
  - `internal/lyxdirs/doc.go`
  - `internal/hubgeom/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/standalonestate/standalonestate.go`
  - `internal/standalonestate/doc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Declare `package standalonestate`, importing only the standard library (`crypto/sha256`, `encoding/hex`, `errors` or `fmt`, `os`, `path/filepath`, `runtime`, `strings`) and nothing else.

  Export exactly one function:

  ```go
  func Derive(target string) (stateDir string, hash8 string, err error)
  ```

  `Derive` is a thin wrapper that fills the environment-shaped parameters of an unexported seam and calls it:

  ```go
  func derive(goos, localAppData, xdgStateHome, home, target string) (string, string, error)
  ```

  `Derive` reads `runtime.GOOS`, `os.Getenv("LOCALAPPDATA")` and `os.Getenv("XDG_STATE_HOME")`, and calls `os.UserHomeDir()` **only on the non-Windows branch** — on Windows it passes `""` for `home`, so a `UserHomeDir` failure can never surface as an error on a branch that never reads the value.

  `derive` behaviour, in order:

  1. Reject a relative target: call `filepath.IsAbs(target)` first and return an error naming the offending value when it is false.
     Never call `filepath.Abs` anywhere in this package — it resolves against the process working directory via `os.Getwd`, which the Cwd Resolution Invariant reserves to `internal/lyxcwd` alone, and which would make the supposedly pure seam depend on the host cwd of whatever test process ran it.
  2. Normalise the target: `filepath.EvalSymlinks`, falling back to `filepath.Clean(target)` alone when `EvalSymlinks` returns an error, then `filepath.Clean` the resolved value.
     An `EvalSymlinks` failure is explicitly **not** an error — it is the documented fallback for a target that does not exist on disk yet.
     These are the same semantics as the unexported `normalizePath` in `internal/lyxcwd/anchor.go`, reimplemented rather than imported because this package must stay stdlib-only.
  3. When `goos == "windows"`, lower-case the normalised string with `strings.ToLower` **before** hashing.
     This is the one intentional divergence from `internal/lyxcwd/anchor.go`'s `samePath`, which folds case at comparison time — hashing has no comparison step, so the fold must happen to the string that is hashed.
     Fold on Windows only, matching `lyxcwd`'s rule exactly rather than being marginally more correct on macOS at the cost of the two derivations disagreeing.
  4. `hash8` is `sha256.Sum256` over the resulting bytes, hex-encoded with `encoding/hex`, truncated to the first eight characters — exactly 8 lowercase hex characters.
  5. `stateDir` on the `goos == "windows"` branch is `filepath.Join(localAppData, "lyx", hash8)`, and an empty `localAppData` is an error.
     On every other branch it is `filepath.Join(xdgStateHome, "lyx", hash8)` when `xdgStateHome` is non-empty, and otherwise `filepath.Join(home, ".local", "state", "lyx", hash8)`, with an empty `home` at that point being an error.
  6. `derive` creates nothing on disk — no `os.MkdirAll`, no write of any kind.

  Return an error in exactly three cases and no others: the target is not absolute; `localAppData` is empty on the Windows branch; `xdgStateHome` is empty and `home` is also empty on the non-Windows branch.

  At the seam boundary the empty string means "unset": `derive` takes plain strings and deliberately cannot distinguish unset from set-to-empty, which is correct because neither an empty `%LOCALAPPDATA%` nor an empty `$XDG_STATE_HOME` is a usable directory.

  `internal/standalonestate/doc.go` carries the package doc comment, matching the vocabulary and tone of `internal/lyxdirs/doc.go` and `internal/hubgeom/doc.go`.
  It must record: that the package is a zero-non-stdlib-import leaf so standalone CLI packages can import it without cycle risk;
  that `target` must already be absolute and why (the cwd consultation belongs at the CLI argument-parsing boundary, not here);
  that normalisation exists so two spellings of the same directory hash identically, since otherwise two standalone runs against the same target would get different sockets, sessions and state directories;
  and that `hash8` collisions are accepted rather than handled — 32 bits means two distinct targets can in principle share one state directory, which is wrong-but-not-corrupting, and the fix if it ever matters is to widen `hash8` here, a one-line change because every consumer takes the value from `Derive` rather than re-deriving it.
  Do not use the tokens `weft` or `warp` anywhere in either file.
- **Commit:** `feat(standalonestate): add stdlib-only state-directory derivation leaf`

### Card 6: drive both platform rows through the seam

- **Context:**
  - `internal/standalonestate/standalonestate.go`
- **Edits:** none
- **Creates:**
  - `internal/standalonestate/standalonestate_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  An untagged, in-package (`package standalonestate`) test file calling the unexported `derive` directly — no `export_test.go`, since an in-package test reaches an unexported function without one.
  No build constraint, no process spawns, so the file stays tier 1.

  Cover, at minimum:

  - The Windows row: `derive("windows", "/localappdata", "", "", "/abs/target")` produces a `stateDir` equal to `filepath.Join("/localappdata", "lyx", hash8)`.
    Build the expected value with `filepath.Join` exactly as the implementation does, never a literal backslash string — `filepath.Join` takes its separator from the compile-time host, not from the injected `goos`, so a literal would pass only on Windows.
    What this row pins is that the Windows branch consults `localAppData`; the separator is left to `path/filepath`.
  - The non-Windows row with `xdgStateHome` set: the result is `filepath.Join(xdgStateHome, "lyx", hash8)`, and the same call with `localAppData` set to a distinctive value proves the non-Windows branch never consults it.
  - The `xdgStateHome`-unset fallback: an empty `xdgStateHome` with a non-empty `home` yields `filepath.Join(home, ".local", "state", "lyx", hash8)`.
  - An empty `localAppData` on the Windows branch returns an error.
  - An empty `xdgStateHome` **and** an empty `home` on the non-Windows branch returns an error.
  - The case fold: two spellings of one path differing only in case produce the **same** `hash8` under `goos == "windows"` and **different** hashes under `goos == "linux"`.
  - A relative target is rejected with an error, under both `goos` values.
    Since rejecting relative targets is precisely what keeps the seam independent of the host, assert in the same test that the outcome does not vary with the test process' working directory — the function must never consult it.
  - `hash8` is exactly 8 characters and every character is a lowercase hex digit.
  - Stability: the same input yields the same `hash8` across repeated calls, which is the property the whole resumability story rests on.
  - `derive` creates nothing on disk: point it at a path under `t.TempDir()`, call it, and assert the returned `stateDir` does not exist afterwards.
- **Commit:** `test(standalonestate): drive both platform rows through the derive seam`

### Card 7: pin symlink normalisation against a real filesystem

- **Context:**
  - `internal/standalonestate/standalonestate.go`
- **Edits:** none
- **Creates:**
  - `internal/standalonestate/symlink_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  An in-package (`package standalonestate`) test file whose first non-empty line is `//go:build integration`.
  It needs a real filesystem to create a symlink, and tagging keeps tier 1 clean even though a symlink is a filesystem operation rather than a git spawn.
  This package spawns no git and builds no `gitkit`/`hubforge` fixture, so it needs no `TestMain` — the Hermetic Git Test Environment Invariant does not engage here.

  Build a real directory under `t.TempDir()`, create a symlink pointing at it with `os.Symlink`, and assert that `derive` produces the **same** `hash8` for the symlink path and for its real target — the property that stops two spellings of one directory getting two different state directories.
  Skip the test with `t.Skip` when `os.Symlink` fails with a permission error, so a Windows host without Developer Mode does not fail the suite.
- **Commit:** `test(standalonestate): pin symlink normalisation against a real filesystem`

### Card 8: enforce the Standalonestate leaf invariant mechanically

- **Context:**
  - `internal/tokenvocab/leaf_enforcement_test.go`
- **Edits:** none
- **Creates:**
  - `internal/standalonestate/leaf_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Copy the structure of `internal/tokenvocab/leaf_enforcement_test.go` into `package standalonestate`: a `TestLeafInvariant_AllowlistOnly` that locates its own package directory via `runtime.Caller(0)`, walks it with `filepath.WalkDir`, skips directories and any file that is not a non-`_test.go` `.go` file, parses each with `go/parser` using `parser.ImportsOnly`, and fails on any import that is neither stdlib (no `.` in the first path segment) nor present in an `allowedImports` map.
  Declare `allowedImports` as an **empty** `map[string]bool{}` — `internal/standalonestate` has no permitted non-stdlib import — and word the failure message so it names the Standalonestate Leaf Invariant and reports the offending relative paths.
  Untagged, with no build constraint.
- **Commit:** `test(standalonestate): enforce stdlib-only leaf invariant`

## Batch Tests

`verify:` runs `go test ./internal/standalonestate/... ./internal/lyxcwd/...` followed by `go test -tags integration ./internal/standalonestate/...`.

The untagged run covers `standalonestate_test.go` (both platform rows, both error rows, the case fold, the relative-target rejection, the `hash8` shape and stability, and the creates-nothing assertion) and `leaf_enforcement_test.go`.
The tagged run is required because this batch creates `symlink_integration_test.go`, which carries `//go:build integration` and is invisible to a plain `go test` — Go does not compile a build-constrained file at all unless its tag is enabled, so without the second invocation the symlink normalisation assertion would silently never run.
`./internal/lyxcwd/...` is included for `TestEnforcement_FabricVocabulary` and `TestEnforcement_GeometryLiterals`, the repo-wide walks that are the only thing catching a vocabulary or geometry-literal slip in the new production files.
