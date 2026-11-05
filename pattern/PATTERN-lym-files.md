# PATTERN-lym-files

Placing a file, temp dir, lock or link: tracked under `_lym`, never-tracked under `.lym`, named only by `lymdirs`; temp under `lymtemp`, lock paths via `lock`, links via `fslink`.

## Durable and ephemeral state

Rule: Adding file: tracked under `_lym`, never-tracked under `.lym`, same subpath, bar heddle's rendered files and `.heddle` at a warp worktree's anchor; only `lymdirs` names either, bar a `tools/` script's variable block.

Held by review alone; no test enforces it.

Every never-tracked file lives under `.lym`, at the mirrored subpath of the `_lym` content it relates to.
`_lym` holds tracked content only.

- A lock file is never-tracked state: the lock of a tracked file sits under its checkout's `.lym/locks/`, mirroring the file's path, never beside the file (see Tracked-file locks below).
- Nothing a run, the orch or a landing depends on lives only under `.lym`: decisions, landings, stop reports and board state are always in git (board, records, decision record), and never-tracked state such as the creel journal is a delivery path or cache that a fresh machine starts without.
- The two are siblings under `AnchorPath()`: in a hub the sibling is `BoltDir(hub)`, standalone it is `standalonestate.Derive`.
- No engine derives its own `.lym` path; each module exposes a scratch accessor beside its durable one.
- A module's local config override sits in the mirrored directory, `.lym/config/<module>.local.yaml`, never at the mirrored file path `.lym/config/<module>.yaml`, which is the editor staging copy's.
- A hub-wide module's machine override sits under the board dir's `.lym/config/`, as `.lym/config/<module>.machine.yaml`.
- The split is structural (`fabricengine.structuralCommittedDirs` and `structuralNeverCommittedDirs`), never read from `fabric.yaml`'s `pathspec`.
- Weft content is per-branch and is never a merge participant in either direction.
- `internal/logger`'s durable trace sink arms its cwd-anchored fallback only inside a worktree lym owns (`_lym` present at the anchor).
  A plain checkout gets no sink rather than a `.lym` lym does not own.
- Heddle's rendered instruction files (`CLAUDE.local.md`, `AGENTS.override.md`) and the `.heddle` temporary directory sit at a warp worktree's anchor, outside `.lym`, untracked through the warp repository's `info/exclude` lines fabric seeds from its exclude-list key under the anchor.
  The files sit beside the repository's own instruction files in the sessions' working directory, and the temporary directory beside them, so a rename onto them never crosses a filesystem.

### Single declarer

Only `internal/lymdirs` names `_lym` or `.lym` in a path.
The one exception is a `tools/` script's variable block.

## Temp root

Rule: Creating temp file or dir: under `internal/lymtemp`'s run dir, never `os.TempDir()`. (test)

A temp file or directory lym or its tests create lives in the process run directory of `internal/lymtemp`, never directly in the system temp directory.

- The root is `<system temp>/lym-<uid>` off Windows and `<system temp>\lym` on Windows, where the system temp directory is already per user.
  It is created 0700 and refused unless it is a real directory owned by the user; a refused root creates nothing.
- Each process has one run directory below the root, `r` plus four random lower-case letters or digits, created 0700 with `os.Mkdir`, a taken name retried up to a bound and then refused naming the root, holding a lock file its process holds until it removes the directory.
  `lymtemp.MkdirTemp` and `lymtemp.CreateTemp` create below it.
- A process whose system temp directory is already a run directory under a lym root takes that root as its own and makes its run directory beside the parent's, so nesting never pushes a tmux socket path past the `sun_path` limit.
  No code hands a child a `TMPDIR` that is not a run directory.
- `tmuxkit.Main` removes the run directory after a test binary's run, and `lym`'s `main` removes it on normal exit.
- `Root.Sweep` removes the run directories of dead owners: a child past the creation grace of 10 minutes whose lock the sweep can take, or that has none.
  It never reads inside a live run directory and never touches an unprefixed entry, a file or a link.
- Creel's per-hub socket `creel-<shortname>-<h>.sock` is a long-lived file directly in the root beside the run directories, and `Root.Sweep` touches no unprefixed entry, so it never removes it.
- Bound: a killed process skips its removal, and its directory waits for the next start's sweep once the OS has released the lock.
- The scan in `cmd/lym/baretemp_test.go` fails a call `os.MkdirTemp` or `os.CreateTemp` whose first argument is `""` or `os.TempDir()` in any Go file outside `internal/lymtemp`, production or test.
  `t.TempDir()` and a dir naming any other directory stay allowed.
- The scan's allowlist is keyed `<file>:<call>` and admits one call with a reason naming the obstacle.
  An admitted site still removes what it creates on normal exit; what the entry lets through is that one site's directory left in the system temp directory after a kill.
- Bound: the scan sees the static shape only, so a directory held in a variable that holds `""` or the system temp directory passes.

## Tracked-file locks

Rule: Tracked-file lock: path via `lock`. (test)

A lock file is machine-local, so it never sits beside a tracked file.

- The lock of a file in `_lym` or the Bolt checkout lives under that checkout's `.lym/locks/`, and its path comes from `lock.TrackedFileLockPath` (a `_lym` file, mirroring the file's path) or `lock.TrackedLockPath` (a Bolt file, by name), never `path + ".lock"` or `".swaplock"`.
- A push lock lives in the git dir.
- No `*.lock` or `*.swaplock` ignore in `.git/info/exclude` or a `.gitignore` hides a stray lock, so one shows in `git status`.
- A test refuses a lock path built beside a file under `_lym`, or in a Bolt checkout, which is a `_bolt` directory holding a `.git` entry.

## Filesystem links

Rule: Creating a filesystem link: only `internal/fslink`'s `CreateDirLink` or `CreateFileLink`; no `os.Symlink` or `os.Link` outside `internal/fslink`. (test)

Every filesystem link lym creates goes through `internal/fslink`'s `CreateDirLink` or `CreateFileLink`, which own the platform difference between link kinds.

- `CreateDirLink` links a directory, a junction on Windows and a symlink elsewhere.
- `CreateFileLink` links a file as a symlink, and refuses on Windows, where a file symlink needs elevated privileges.
- The scan in `cmd/lym/fslinkonly_test.go` parses every production Go file of the module, `tools/` included, and reports each `os.Symlink` or `os.Link` call, through whatever name the file binds `os` to, outside `internal/fslink`; a dot import of `os` binds no name, so the scan reports the import line itself.
- The scan's allowlist is keyed `<file>:<call>` and admits one call with a reason naming the obstacle to going through fslink.
  `tools/deploy/main.go` is admitted for `os.Symlink`: it copies a plugin-tree symlink verbatim, keeping its raw target without reading whether it names a file or a directory.
- Bound: test files are not scanned, because tests plant links as hostile or foreign fixtures that fslink's refusals would block.
- The entry claims nothing about how a link behaves on Windows: fslink's Windows path has never been executed, and `internal/fabricengine`'s package documentation records what settling it takes.
