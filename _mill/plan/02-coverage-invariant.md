# Batch: coverage-invariant

```yaml
task: "Expand the sandbox suite: subfolder init, weft, warp, config reconcile + coverage invariant"
batch: "coverage-invariant"
number: 2
cards: 1
verify: go test ./cmd/lyx/...
depends-on: [1]
```

## Batch Scope

This batch delivers the machine-enforced "Sandbox Suite Coverage" invariant: a
new Go test that fails the build when a registered lyx module has neither a
`**Covers:**` scenario tag nor an allowlist exclusion, plus the CONSTRAINTS.md
entry recording the invariant. It depends on batch 1 because the test parses the
`**Covers:**` lines that batch adds to `SANDBOX-SUITE.md` — after batch 1 the
covered set is `{board, config, init, weft, warp}` and the test passes. The test
creation and the CONSTRAINTS.md edit live in a single card / single commit to
honour the repo's Documentation Lifecycle rule ("record new cross-cutting
invariants in CONSTRAINTS.md in the same commit").

## Cards

### Card 4: Add the Sandbox Suite Coverage test and CONSTRAINTS.md invariant

- **Context:**
  - `_mill/discussion.md`
  - `cmd/lyx/registration_test.go`
  - `cmd/lyx/longlist_test.go`
  - `cmd/lyx/main.go`
  - `tools/sandbox/SANDBOX-SUITE.md`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:**
  - `cmd/lyx/sandbox_coverage_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `cmd/lyx/sandbox_coverage_test.go` in `package main`.
  It must:
  (a) Build the live cobra root via `newRoot()` and collect `registered` = the
  set of `child.Name()` over `root.Commands()`, skipping the cobra infrastructure
  commands `"help"` and `"completion"` — copy the exact skip pattern from
  `cmd/lyx/longlist_test.go`'s `TestLongList_NamesEveryRegisteredModule`. Do not
  hand-maintain a module list.
  (b) Resolve the repo root from the test file's own location via
  `runtime.Caller(0)` + **three** `filepath.Dir` walk-ups (this file lives at
  `cmd/lyx/sandbox_coverage_test.go`: `cmd/lyx/…` → `cmd/lyx` → `cmd` → repo
  root), matching the *code* at `cmd/lyx/registration_test.go:71`
  (`filepath.Dir(filepath.Dir(filepath.Dir(testFile)))`) — note that file's
  own comment saying "two" is stale; the code is authoritative. Then read
  `tools/sandbox/SANDBOX-SUITE.md` from disk under that root. Parse `covered` =
  the set of whitespace/comma-separated module tokens from every line matching
  the `**Covers:**` prefix. Because S0/S1/S5 carry no `Covers:` line, every token
  parsed is expected to be a bare registered-module name — no parenthesized-token
  stripping is needed.
  (c) Declare `excluded` as a `map[string]string` allowlist with exactly three
  entries and their reasons: `"muxpoc"` (PoC, slated for replacement by the mux
  module), `"ide"` (side-effect heavy: `spawn` opens a real VS Code window,
  `menu` is an interactive stdin picker), `"selfreport"` (`create` files a real
  GitHub issue).
  (d) Assert 1 (coverage): for every module `m` in `registered`, fail with a
  message naming `m` unless `m` is in `covered` or is a key of `excluded` —
  mirror `registration_test.go`'s style of naming the exact offending module in
  the error, not a generic failure.
  (e) Assert 2 (drift guard): for every token in `covered` and every key in
  `excluded`, fail (naming the token) unless it is a member of `registered` —
  this catches typos in `**Covers:**` lines and stale allowlist/tag entries after
  a module rename or removal.
  (f) Include a sanity sub-test (mirroring `registration_test.go`'s
  `discovered_non_empty`) asserting both `registered` and `covered` are non-empty,
  so a silently-broken parse or root-resolution cannot produce a vacuous pass.
  Follow the file-header-comment and godoc conventions of the sibling
  `cmd/lyx/*_test.go` files.
  Then edit `CONSTRAINTS.md`: add a new `## Sandbox Suite Coverage` invariant
  section (place it after the `## CLI / Cobra Invariant` section, before
  `## Documentation Lifecycle`), following the existing invariants' format —
  short authoritative prose stating that every registered lyx module must have a
  `**Covers:**` scenario tag in `tools/sandbox/SANDBOX-SUITE.md` or be on the
  test's exclusion allowlist, and an "**Enforced by**" line pointing at
  `cmd/lyx/sandbox_coverage_test.go`. Mention the allowlist members
  (`muxpoc`, `ide`, `selfreport`) and note that adding a new module requires
  either a tagged scenario or a new allowlist entry with a reason — the same
  "exists ⇒ registered" discipline as the CLI/Cobra registration guard.
- **Commit:** `test(lyx): enforce Sandbox Suite Coverage invariant`

## Batch Tests

`verify: go test ./cmd/lyx/...` — runs the new `sandbox_coverage_test.go`
alongside the existing `cmd/lyx` guard tests (`registration_test.go`,
`longlist_test.go`, `helptree_test.go`, etc.). After batch 1's `**Covers:**`
tags exist, the new test's coverage and drift-guard assertions pass; the scope
is `./cmd/lyx/...` because that is the only package this batch adds code to
(the CONSTRAINTS.md edit is docs with no runnable surface).
