# PATTERN-lyx-temp-root

A temp file or directory lyx or its tests create lives in the process run directory of `internal/lyxtemp`, never directly in the system temp directory.

- The root is `<system temp>/lyx-<uid>` off Windows and `<system temp>\lyx` on Windows, where the system temp directory is already per user.
  It is created 0700 and refused unless it is a real directory owned by the user; a refused root creates nothing.
- Each process has one run directory below the root, `os.MkdirTemp(root, "r")`, holding a lock file its process holds until it removes the directory.
  `lyxtemp.MkdirTemp` and `lyxtemp.CreateTemp` create below it.
- A process whose system temp directory is already a run directory under a lyx root takes that root as its own and makes its run directory beside the parent's, so nesting never pushes a tmux socket path past the `sun_path` limit.
  No code hands a child a `TMPDIR` that is not a run directory.
- `tmuxkit.Main` removes the run directory after a test binary's run, and `lyx`'s `main` removes it on normal exit.
- `Root.Sweep` removes the run directories of dead owners: a child past the creation grace of 10 minutes whose lock the sweep can take, or that has none.
  It never reads inside a live run directory and never touches an unprefixed entry, a file or a link.
- Bound: a killed process skips its removal, and its directory waits for the next start's sweep once the OS has released the lock.
- The scan in `cmd/lyx/baretemp_test.go` fails a call `os.MkdirTemp` or `os.CreateTemp` whose first argument is `""` or `os.TempDir()` in any Go file outside `internal/lyxtemp`, production or test.
  `t.TempDir()` and a dir naming any other directory stay allowed.
- The scan's allowlist is keyed `<file>:<call>` and admits one call with a reason naming the obstacle.
  An admitted site still removes what it creates on normal exit; what the entry lets through is that one site's directory left in the system temp directory after a kill.
- Bound: the scan sees the static shape only, so a directory held in a variable that holds `""` or the system temp directory passes.
