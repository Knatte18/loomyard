# Batch: fabric API expand

```yaml
task: 'fabric: close the weft-visibility leak (slice 8)'
batch: 'fabric API expand'
number: 1
cards: 4
verify: go test -tags integration ./internal/fabricengine/
depends-on: []
```

## Batch Scope

Adds the four new `internal/fabricengine` API surfaces every downstream batch consumes — `Open`, `Ready`, `CommitResult.Committed()`, and `RefScanner` — as pure additions, TDD-first.
Nothing is unexported yet (`New`, `Fabric.Warp`, `Fabric.Weft` stay public until batch 04), so this batch compiles and passes standalone with zero call-site churn.
External interface for later batches: `Open(l *lyxcwd.Location) (*Fabric, error)`, `Ready(l *lyxcwd.Location) (bool, error)`, `(CommitResult).Committed() bool`, `NewRefScanner(l *lyxcwd.Location) *RefScanner` with `Matches(cmd string) bool`.
Batch-local decision: each new surface lives in its own new file (`open.go`, `ready.go`, `refscanner.go`) so batch 04's edits to `fabric.go` stay surgical.

## Cards

### Card 1: `Open(l)` constructor

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/lyxtest/lyxtest.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/open.go`
  - `internal/fabricengine/open_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Per decision `one-constructor-open`: add `func Open(l *lyxcwd.Location) (*Fabric, error)` in new `open.go`, implemented as `New(l.WorktreePath(), WeftWorktree(l))` for now (batch 04 retargets it to `newPaired`).
  `Open` performs no setup beyond what `New` already does (decision `open-does-not-wire`) and stat-validates both checkouts (decision `open-stats-both-sides`).
  Doc comment describes fabric one-repo-first: "Open returns a handle on the fabric repo for this worktree" — it must not explain the two-checkout mechanism (that stays in the package doc, an owner surface).
  Tests (TDD, write first, `//go:build integration` because they use `lyxtest.CopyPaired`): happy path returns a non-nil handle on a paired fixture;
  missing host worktree returns `*ErrMissingPath` naming the host path;
  missing sibling returns `*ErrMissingPath` naming the sibling, with the host checked first — the same contract `fabric_test.go:25,44` pins for `New` today, reached through a `*lyxcwd.Location`.
  Build each failure case by deleting one side of a `lyxtest.CopyPaired` fixture.
- **Commit:** `feat(fabricengine): add Open(l) location-based constructor`

### Card 2: `Ready(l)` presence probe

- **Context:**
  - `internal/fabricengine/fabric.go`
  - `internal/loomengine/preflight.go`
  - `internal/lyxtest/lyxtest.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/ready.go`
  - `internal/fabricengine/ready_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Per decision `ready-not-paired`: add `func Ready(l *lyxcwd.Location) (bool, error)` in new `ready.go`, replacing the probe `loomengine/preflight.go:105` currently does with `os.Stat(fabricengine.WeftWorktree(l))` (the preflight edit itself happens in card 5).
  Contract, all three outcomes tested: `(false, nil)` when the sibling worktree is absent (`errors.Is(err, fs.ErrNotExist)`);
  `(true, nil)` when present;
  `(false, err)` for any other stat failure (e.g. a permissions fault) — the third case is what pins the contract and must not be omitted.
  Doc comment answers loomengine's real question ("is fabric usable in this worktree") without naming a side.
  Tests tagged `//go:build integration` (fixture-backed via `lyxtest.CopyPaired`; simulate the error case with a non-directory path component or chmod where portable — if no portable simulation exists, cover the error branch with a unit-style test on a path whose parent is a regular file).
- **Commit:** `feat(fabricengine): add Ready(l) usability probe`

### Card 3: `CommitResult.Committed()`

- **Context:**
  - `internal/fabricengine/classify.go`
  - `internal/fabricengine/config.go`
  - `internal/lyxtest/lyxtest.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/fabricengine/commit.go`
- **Creates:**
  - `internal/fabricengine/committed_test.go`
  - `internal/fabricengine/committed_lyxonly_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Per decision `commit-result-committed-method`: add `func (r CommitResult) Committed() bool { return r.WarpCommitted || r.WeftCommitted }` to `commit.go`.
  The four raw fields stay exported (fabriccli prints them by design;
  batch 07's enforcement test polices non-owner reads).
  `committed_test.go` (untagged, no fixture): table-driven over the four `WarpCommitted`/`WeftCommitted` combinations.
  `committed_lyxonly_integration_test.go` (`//go:build integration`): pins the value-identity argument from the discussion — a `Commit` call whose file list is `ScopedPathspec(AnchorRel, ["_lyx"])`-scoped `_lyx`-only input never produces a warp commit (`WarpCommitted` stays false), so `Committed()` equals today's `WeftCommitted` for the three CLI callers.
- **Commit:** `feat(fabricengine): add CommitResult.Committed()`

### Card 4: `RefScanner`

- **Context:**
  - `internal/websterengine/audit.go`
  - `internal/websterengine/audit_test.go`
  - `internal/weftname/weftname.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/fabricengine/refscanner.go`
  - `internal/fabricengine/refscanner_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Per decision `refscanner-owns-both-halves`: add `type RefScanner struct` with `func NewRefScanner(l *lyxcwd.Location) *RefScanner` and `func (s *RefScanner) Matches(cmd string) bool` in new `refscanner.go`.
  The scanner absorbs both halves of `websterengine`'s current `weftReferencePattern(layout)`: the path half (the sibling worktree path via `WeftWorktree(l)` plus the `weftname.Suffix` sibling-name form) and the command-spelling half (`lyx(?:\.exe)?\s+(fabric|weft|warp)\b`).
  The regex is compiled once in `NewRefScanner` and reused per `Matches` call — never recompiled per command.
  Both halves stay hard-fail from the caller's perspective (decision `scanner-keeps-hard-fail`);
  `Matches` itself only reports a match.
  `refscanner_test.go` (untagged — build the `*lyxcwd.Location` synthetically the way `websterengine/audit_test.go`'s `fakeLayout` helper does, no git fixture): the three cases `audit_test.go` exercises today — a command containing the sibling worktree path, a command containing a `-weft` sibling name, a `lyx fabric`/`lyx weft`/`lyx warp` invocation — plus a clean command that must not match.
  This is the behavioural contract that must not regress when the regex moves packages (websterengine's own migration is card 12).
- **Commit:** `feat(fabricengine): add RefScanner for fabric-reference auditing`

## Batch Tests

`verify:` runs `go test -tags integration ./internal/fabricengine/`, which covers the four new test files (`open_integration_test.go`, `ready_integration_test.go`, `committed_test.go` + `committed_lyxonly_integration_test.go`, `refscanner_test.go`) and the package's existing suite, proving the additions break nothing.
`-tags integration` is required because three of the new test files are fixture-backed and integration-tagged per the Test Tier Purity Invariant.
