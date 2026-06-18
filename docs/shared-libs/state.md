# `internal/state`

Generic locked typed JSON I/O for persistent state. No fixed schema — callers own
what the fields mean and where the file lives. Built on [`internal/fsx`](fsx.md)
(atomic writes) and [`internal/lock`](lock.md) (cross-process coordination).

The `.lyx/` directory is the **gitignored runtime-state dir** — it holds
machine-local data only (never config, never anything portable across machines).
Modules like mux write their own state files there (e.g. `.lyx/mux-state.json`)
and define their own schemas; `internal/state` provides the read/write plumbing.

- **`WriteJSON[T](path, v)`** — acquires an exclusive write lock on `path + ".lock"`,
  marshals `v` to indented JSON, and writes it atomically via `fsx.AtomicWriteBytes`.
  The lock is released via defer.
- **`ReadJSON[T](path)`** — acquires a shared read lock on `path + ".lock"`, reads
  the file, and unmarshals it into type `T`. Returns `(zero, false, nil)` if the
  file does not exist; `(zero, false, err)` on other errors; `(value, true, nil)`
  on success. The lock is released via defer. Corruption (unmarshal errors) is not
  swallowed.
