# PATTERN-tracked-file-locks

A lock file is machine-local, so it never sits beside a tracked file.

- The lock of a file in `_lym` or the Bolt checkout lives under that checkout's `.lym/locks/`, and its path comes from `lock.TrackedFileLockPath` (a `_lym` file, mirroring the file's path) or `lock.TrackedLockPath` (a Bolt file, by name), never `path + ".lock"` or `".swaplock"`.
- A push lock lives in the git dir.
- No `*.lock` or `*.swaplock` ignore in `.git/info/exclude` or a `.gitignore` hides a stray lock, so one shows in `git status`.
- A test refuses a lock path built beside a file under `_lym`, or in a Bolt checkout, which is a `_bolt` directory holding a `.git` entry.
