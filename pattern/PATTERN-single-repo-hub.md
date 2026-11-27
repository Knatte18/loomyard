# PATTERN-single-repo-hub

Running a hub as one repository: records on each code branch, the board on `lym-bolt`; merges, pull request views and code identity strip the records paths.

## Layout

Rule: Reading the layout: a worktree whose root carries `.lym-anchor` belongs to a one-repository hub, read through `lymcwd.OneRepository` and fabric's `oneRepositoryAt`. (test)

A one-repository hub keeps each task's records under `<anchor>/_lym` on its own code branch, and the board on the orphan branch `lym-bolt`.
No config key, flag or hub file records the layout.
`lymcwd` reads the marker from the working tree;
fabric reads it from git, and leaves out a two-repository records sibling and the Bolt checkout by name.

Held by `TestResolveWithin_OneRepository` in `internal/lymcwd`, `TestRecordsGeometry_OneRepository` in `internal/fabricengine`,
and `TestCloneAndWire_OneRepository` in `internal/fabriccli`.

## Records on the code branch

Rule: Committing a one-repository worktree's records: path-limited on the worktree's own branch, waiting out a held `index.lock`, refused in a prime off `lym.primeBranch`.

Fabric's commit primitives take only the paths `RecordsPathsAt` names.
A prime off the branch `lym.primeBranch` names refuses every records commit.
A records commit around an unsettled merge goes through `MergeStateOperation`, and an abort or continue of that merge settles it.

Held by `TestRecordsCommit_OneRepository` and `TestRecordsCommit_DuringMerge` in `internal/fabricengine`.

## Merges strip the records

Rule: Merging in a one-repository hub: every fabric merge restores the records paths of `RecordsPathsAt` to the receiving side and commits with the `MergeTrailerKey` trailer.

Records are per-branch and never a merge participant in either direction.
Pull request views and code identity strip the records paths too, which a later part builds.

Held by `TestMerge_OneRepository` in `internal/fabricengine`.

## Replay and the prime's push

Rule: Replaying or pushing in a one-repository hub: replay only records-only commits, with fork-point trailers; guard the prime's records push and owe it as `operator_unpushed`.

A diverged branch replays its records-only commits and stops, naming the commits, when a local commit changes a code path.
The prime's records push waits while the operator has unpushed commits on its branch,
and a prime whose branch refuses direct pushes records the failure as `protected`.
A pair pushes its one branch once, with no owed code tip.

Held by `TestReplay_OneRepository`, `TestPairPush_OneRepository` and `TestPrimePushGuard` in `internal/fabricengine`.

## The Bolt on `lym-bolt`

Rule: Placing the board: on the orphan branch `lym-bolt`, which no code branch merges.

A clone by one URL, or with `--single-repo`, makes the Bolt checkout on `lym-bolt`.
The Bolt's render-manifest `.gitignore` stays on that branch.

Held by `TestCloneAndWire_OneRepository` in `internal/fabriccli` and `TestBoardWrite_OneRepositoryHub` in `internal/boardengine`.
