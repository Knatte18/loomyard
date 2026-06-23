# Batch: paths-host-junctions

```yaml
task: 'weft producers: _lyx/config, lyx config, codeguide'
batch: paths-host-junctions
number: 2
cards: 3
verify: go test -tags integration ./internal/paths/ ./internal/worktree/
depends-on: []
```

## Batch Scope

Centralize the host-worktree junction set in `internal/paths` (the sole geometry owner) and make
the `internal/worktree` seeder iterate it instead of hardcoding `_lyx`. The list has exactly one
entry (`_lyx`) today; this is purely a mechanism generalization so a future `_codeguide` entry is
a one-line addition — NO `_codeguide` is added here (would break
`internal/paths/codeguide_guard_test.go`). Behaviour is preserved: after the refactor, a paired
`lyx worktree add` still seeds exactly the `_lyx` junction and the `_lyx` `.git/info/exclude`
entry. Batch-local decision: the junction record carries three fields because the two seeding ops
consume different ones — junction creation uses `Link`+`Target`, exclude seeding uses `Name`.
This batch is independent of the other three.

## Cards

### Card 5: Add `HostJunction` type and `HostJunctions(slug)` to paths

- **Context:**
  - `internal/paths/codeguide_guard_test.go`
- **Edits:**
  - `internal/paths/paths.go`
  - `CONSTRAINTS.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `internal/paths/paths.go` add an exported struct
  `type HostJunction struct { Name, Link, Target string }` and a method
  `func (l *Layout) HostJunctions(slug string) []HostJunction` returning a single-element slice:
  `{Name: "_lyx", Link: l.HostLyxLink(slug), Target: l.WeftLyxDirFor(slug)}`. Do NOT reference
  `_codeguide` anywhere (the guard test in `internal/paths/codeguide_guard_test.go` forbids any
  `_codeguide` literal outside the geometry methods, and `paths.go` must not introduce one here).
  Append `HostJunctions(slug)` to the `Layout` method enumerations in `CONSTRAINTS.md` (the "For
  New Code" list, line ~19) and `docs/overview.md` (the geometry-methods list, line ~74).
- **Commit:** `feat(paths): add centralized HostJunctions list (_lyx only)`

### Card 6: Refactor seeders to iterate `HostJunctions`

- **Context:**
  - `internal/paths/paths.go`
  - `internal/worktree/add.go`
- **Edits:**
  - `internal/worktree/weft.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Rewrite `seedLyxJunction(l *paths.Layout, slug string) error` in
  `internal/worktree/weft.go` to loop over `l.HostJunctions(slug)`, applying the existing
  create-or-verify logic per junction using each record's `Link` and `Target` (the current
  per-junction body — `os.Lstat`, `fslink.IsLink`/`PointsTo` idempotency check, the
  "real _lyx predates weft" error, and `fslink.CreateDirLink`). Rewrite
  `seedGitExclude(l *paths.Layout, slug string) error` to append each junction's `Name` (looping
  over `l.HostJunctions(slug)`) to `.git/info/exclude`, preserving the line-exact idempotency
  check per name. The call sites in `internal/worktree/add.go` (steps 9 and 10) keep the same
  signatures. With one junction, output is byte-identical to today. Keep the existing function
  names `seedLyxJunction`/`seedGitExclude` (do NOT rename) so the `add.go` call sites stay
  unchanged and `add.go` remains read-only Context for this card.
- **Commit:** `refactor(worktree): seed host junctions from paths.HostJunctions`

### Card 7: Tests — HostJunctions geometry + seeder behaviour-preserved

- **Context:**
  - `internal/paths/paths.go`
  - `internal/worktree/weft.go`
  - `internal/worktree/weft_test.go`
  - `internal/paths/weft_test.go`
  - `internal/lyxtest/lyxtest.go`
- **Edits:**
  - `internal/paths/weft_test.go`
  - `internal/worktree/weft_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** (a) In `internal/paths/weft_test.go` add a table test asserting
  `HostJunctions(slug)` returns exactly one entry `{Name:"_lyx", Link: HostLyxLink(slug),
  Target: WeftLyxDirFor(slug)}` for both a prime-derived layout and a non-prime worktree layout,
  and an assertion that no entry's `Name` equals `_codeguide` (scope guard). (b) In
  `internal/worktree/weft_test.go` extend the existing seeder coverage (using the `CopyPaired`
  fixture, `//go:build integration`) to assert that after seeding, the `_lyx` junction exists and
  resolves to the weft `_lyx` target and `.git/info/exclude` contains the line `_lyx` — i.e. the
  refactor is behaviour-preserving.
- **Commit:** `test(paths,worktree): cover HostJunctions and seeder parity`

## Batch Tests

`verify: go test -tags integration ./internal/paths/ ./internal/worktree/` runs both the
untagged `HostJunctions` geometry table test and the integration-tagged seeder parity tests
(which need the `CopyPaired` git fixtures). The `_codeguide`-absent assertion plus the existing
`internal/paths/codeguide_guard_test.go` together prove no codeguide leakage. The seeder parity
test is the guard that iterating the one-entry list reproduces today's exact behaviour.
