# PATTERN-leaf-packages

Each package below imports a closed set and never a feature package; a reverse import is never allowed.

## `internal/gitkit`

Imports only the standard library, `lyxcwd`, `weftname`, `configengine`, `lyxdirs` and `lyxtemp`, and `gofrs/flock` through `lyxtemp`.
- `gitkit.CopyRepo` is callable from `lyxcwd` alone; everyone else takes a hub from `hubforge`.
- Test git plumbing lives in `gitkit`; a package-local `_test.go` helper that shells out to git is a review flag.
  This is review discipline, not a scan.

## `internal/fabricvocab`

Imports only the standard library.
It holds only the vocabulary predicates and their data, since the owner-set row exempts the whole package from the bare-token rule.

## `internal/haltnote`

Imports only the standard library.

## `internal/modelspec`

Imports only the standard library, `configengine` and `gopkg.in/yaml.v3`.

## `internal/reviewbudget`

Imports only the standard library and `configengine`.
It declares the review budget key family loom and darn config build on, and imports neither engine.

## `internal/buildinfo`

Imports nothing at all, not even the standard library.
It exposes `Channel`, its two values `ChannelProduction` and `ChannelDev`, and the accessors `IsDev()` and `IsProduction()` only.

## `internal/standalonestate`

Imports only the standard library.
It never resolves a working directory, and `Derive` creates nothing on disk.

## `internal/segmentcolor`

Imports only the standard library.

## `internal/selfexec`

Imports only the standard library and `internal/logger`.
`internal/logger` is admitted because the helper logs its stat, exec and replace outcomes.

## `internal/pattern`

Imports only the standard library, `lyxdirs`, `stencilstore` and `stencil`.

## `internal/friction`

Imports only the standard library, `internal/logger`, `internal/stencil` and `internal/stencilstore`.
- `internal/logger` is admitted because the marker-absent helper logs rather than returning a bool for seven callers to duplicate.
  `internal/friction` already pulls `logger` transitively through `internal/stencilstore`, so the admission widens nothing in practice.

## `internal/durfmt`

Imports only the standard library.

## `internal/dotgit`

Imports only the standard library.
It is the sole `.git` parser: git dir, common dir, the repository test and the walk-up to a worktree root, in-process and spawning nothing.
It is its own package because `gitrepo`'s in-package test imports `gitkit`, which reaches `lyxcwd`, so `lyxcwd` importing `gitrepo` would close a cycle.

## `internal/pauserecord`

Imports only the standard library and `internal/state`, and `gofrs/flock` through it.
It is the one leaf every reader of a pause imports, and each reader is told its `Geometry` and derives no path of its own.

## `internal/lyxtemp`

Imports only the standard library and `internal/lock`, and `gofrs/flock` through it.
It is importable from production code, not a kit.

## `internal/fswatch`

Imports only the standard library, `github.com/fsnotify/fsnotify` and `internal/logger`.
- Every lyx process that waits on a file event goes through it, so fsnotify is imported nowhere else.
- `internal/logger` is admitted because fsnotify's error channel is drained and logged at Debug.
