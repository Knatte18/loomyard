# PATTERN-coverage-map-reader

Reading the coverage maps across packages or finding a diff's changed functions goes only through `internal/testreach`.
The index of the shared maps is written only by the prime's map run, and a report step's review note is used only when it matches HEAD's tree.

## The reader

Rule: Reading the coverage maps of more than one package, or finding the functions a diff changes: only through `internal/testreach`. (test)

`internal/testreach` runs no git: its callers hand it the file contents of each side.
It alone answers which tests reach a function, and it alone matches the functions of two file sets.

- Every new reader of one package's map calls `testreach.ReadPackageMap`, with its pass-over rules for a build failure, a map naming an index outside its tables and a map that does not parse.
- The single-package readers that call `testcover.ReadMap` themselves stay, each pinned by file:
  the map run's own rewrite, the redundancy step's judged package and `lym gate redundant`'s package.
- Enforced by `cmd/lym/coveragemapreader_test.go`.
  A non-test file outside `internal/testcover` and `internal/testreach` that calls `testcover.ReadMap` fails it unless pinned,
  and a pinned file that no longer holds the call fails it too.
- Bound: the scan sees the call by its `testcover` qualifier.
  A map read through a renamed import, or through a path that bypasses `testcover`'s functions, passes it.

## The index

Rule: The index of the shared maps: written only by the prime's `lym gate test --map` after its maps; read by every reader that finds its stamp current. (test)

The index is a reverse table from function to reaching tests, a cache file beside the shared maps.
It is stamped with the set of map files in the directory and each one's size and modification time.

- A reader whose index is absent, unreadable or stamped differently builds the same table in memory from the maps and goes on.
  The index is a cache, so its absence refuses nothing (`PATTERN-lym-files`).
- A task worktree's map run writes none: its maps are few and the reader reads them whole.
- Bound: a map rewritten at the same size within one modification-time tick keeps its stamp,
  so the index can serve its old pairs until the next map run; every answer is advisory.
- Bound: after a map run that ended before its index write, as a moved HEAD ends it,
  every reader builds the table in memory until a map run writes the index again.

## The review note

Rule: A report step's review note: used by a review only when the report matches HEAD's tree.

The redundancy report, and the reach report that follows it, record the tree they judged.
Code-Review's rubric reads such a report only when it matches the reviewed HEAD's tree,
and otherwise reads a line saying the report is absent or stale, never a verdict from it.
A report written in the prime, which has no parent branch, is marked so and never matches a review's HEAD.
Held by review alone; no test enforces it.
