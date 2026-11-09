# PATTERN-leaf-packages

Each package below imports a closed set and never a feature package; a reverse import is never allowed.

## `internal/gitkit`

Imports only the standard library, `lymcwd`, `weftname`, `configengine`, `lymdirs` and `lymtemp`.
It also reaches `gofrs/flock` through `lymtemp`.
- `gitkit.CopyRepo` is callable from `lymcwd` alone; everyone else takes a hub from `hubforge`.
- Test git plumbing lives in `gitkit`; a package-local `_test.go` helper that shells out to git is a review flag.
  This is review discipline, not a scan.

## `internal/fabricvocab`

Imports only the standard library.
It holds only the vocabulary predicates and their data.
That limit matters because the owner-set row exempts the whole package from the bare-token rule.

## `internal/haltnote`

Imports only the standard library.

## `internal/modelspec`

Imports only the standard library, `configengine` and `gopkg.in/yaml.v3`.

## `internal/agentconfig`

Imports only the standard library, `configengine` and `gopkg.in/yaml.v3`.
It owns `agents.yaml`, which shuttle, its engines and every spawning package read, so it imports none of them.

## `internal/lenses`

Imports only the standard library, `configengine` and `gopkg.in/yaml.v3`.
It owns the `lenses.yaml` module and the fan key grammar tapestry and darn share, and imports no engine.

## `internal/reviewbudget`

Imports only the standard library and `configengine`.
It declares the review budget key family tapestry and darn config build on, and imports neither engine.

## `internal/buildinfo`

Imports nothing at all, not even the standard library.
It exposes only `Channel` and its values `ChannelProduction` and `ChannelDev`.
The only accessors are `IsDev()` and `IsProduction()`.

## `internal/standalonestate`

Imports only the standard library.
It never resolves a working directory, and `Derive` creates nothing on disk.

## `internal/shellguard`

Imports only the standard library, `internal/agentname` and `internal/gateslot`.
- `internal/agentname` and `internal/gateslot` are admitted for environment-variable names.
  The raw go row's pattern names those variables.
  The `gateslot` admission departs from the header's "never a feature package" for the constants `gateslot` owns.
  Declaring each name once outweighs a dependency-free leaf.

## `internal/segmentcolor`

Imports only the standard library.

## `internal/selfexec`

Imports only the standard library and `internal/logger`.
`internal/logger` is admitted because the helper logs its stat, exec and replace outcomes.
It owns `LymBinary`, the one lookup of the running lym binary that every command pinned into tmux or a process goes through,
so every package that pins one can import it.

## `internal/pattern`

Imports only the standard library, `lymdirs`, `stencilstore` and `stencil`.

## `internal/friction`

Imports only the standard library, `internal/logger`, `internal/stencil` and `internal/stencilstore`.
- `internal/logger` is admitted because the marker-absent helper logs the absence.
  Returning a bool instead would make every caller duplicate that logging.
  `internal/friction` already reaches `logger` through `internal/stencilstore`, so the admission widens nothing.

## `internal/durfmt`

Imports only the standard library.

## `internal/consoletitle`

Imports only the standard library, so `fabricengine`, which learns nothing of the board, forms a workspace title with the same function the board's titles use.

## `internal/lensreport`

Imports only the standard library, so every lens review host and `planparser` can share the lens report naming.

## `internal/machinename`

Imports only the standard library.

## `internal/runproc`

Imports only the standard library, `internal/proc`, `internal/state`, `internal/logger` and `internal/lymcwd`.
`internal/lymcwd` is admitted for the one Bolt directory name `runproc.HubOwner` takes from it.
It is told the record directory and derives no path.

## `internal/sleepjournal`

Imports only the standard library.
It spawns no process: each caller runs `journalctl` itself and wraps the clause `Read` returns in its own note.

## `internal/dotgit`

Imports only the standard library.
It is the sole `.git` parser: git dir, common dir, the repository test and the walk-up to a worktree root.
It also parses a repository's worktree list:
each `worktrees/<id>/` entry's `gitdir`, `HEAD` and `locked` files,
the loose refs and `packed-refs` a `HEAD` resolves through,
and the `core.bare`, `core.ignorecase`, `extensions.refStorage` and `extensions.objectFormat` settings of its `config`.
It parses in-process and spawns nothing.
A repository whose ref storage is not `files` is refused with a named error, and its callers ask git instead.
It is its own package because `gitrepo`'s in-package test imports `gitkit`, which reaches `lymcwd`.
A `lymcwd` import of `gitrepo` would therefore close a cycle.

## `internal/toolfloor`

Imports only the standard library.
It holds the git and tmux version floor and the version-line parser, and spawns, caches and logs nothing.

## `internal/pauserecord`

Imports only the standard library and `internal/state`, and `gofrs/flock` through it.
It is the one leaf every reader of a pause imports.
Each reader is told its `Geometry` and derives no path of its own.

## `internal/lymtemp`

Imports only the standard library and `internal/lock`, and `gofrs/flock` through it.
It is importable from production code, not a kit.

## `internal/fswatch`

Imports only the standard library, `github.com/fsnotify/fsnotify` and `internal/logger`.
- Every lym process that waits on a file event goes through it, so fsnotify is imported nowhere else.
- `internal/logger` is admitted because fsnotify's error channel is drained and logged at Debug.
