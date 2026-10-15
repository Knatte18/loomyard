# PATTERN-shed-driver

`internal/sheddriver` is the one Go driver of shed runs: it steps a run to its stops, decides each stop by a closed rule table, spawns the tackler for a stop the table does not settle, and hands back what it cannot.
No `<module>cli` and no producer runs a step-until-stop loop of its own.
That second clause is review discipline, not a scan, as `PATTERN-shed-verb-set`'s first clause is.

## Told geometry

- `sheddriver` is a told-geometry engine: it takes every path from its caller and reaches recipe behaviour only through `Deps` and `TacklerDeps`.
- It imports no `lyxcwd`, `loomengine`, `loomshed` or `internal/*cli` package, and its other imports are a closed allowlist.
- `shedrun` is on that list for the driver-kind vocabulary and its validator only, since every `shedrun` path constructor takes a `*lyxcwd.Location` the package can never hold.
- Enforced by `internal/sheddriver/seam_enforcement_test.go`.

## Seams a recipe fills

The arming module fills `Deps` and `TacklerDeps`; the optional groups are the recipe-specific part.

- `Arbiter`, `Verify`, `HandOver` and `Tamper` are optional groups on `Deps`, and `Watched`, `CommitRecords` and `WriteFrictionNote` optional closures.
- `TacklerDeps.Snapshot` and `GotoRoutedFrom` shape the tackler's tamper comparison and the rows its resume may name.
- A nil group or closure only removes rows or agents from the rule table.
  A stop a removed row would have caught meets the later rows unchanged, so it ends in the tackler under its spawn budget, a re-step under the repair cap, or a hand-back.
  No unwired seam adds a resume path.

## Outside it

Batten's InnerRun wait polls a child run it does not step, so it is not a step loop and stays in `internal/battenshed`.
