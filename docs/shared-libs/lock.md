# `internal/lock`

Cross-process file locking, wrapping `github.com/gofrs/flock`. Coordinates the
short-lived Loomyard processes on a machine through the filesystem.

- **`AcquireWriteLock(lockPath)`** — exclusive; blocks until free.
- **`AcquireReadLock(lockPath)`** — shared; many readers at once, blocked only by an
  exclusive holder.

That is the whole surface: two primitives over a lock file. Each module decides
*which* lock files it needs and what they guard. board uses three (write / swap /
push) — all the same primitive over different files. The lib has no concept of what
is being protected.
