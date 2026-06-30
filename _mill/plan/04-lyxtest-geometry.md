# Batch: lyxtest-geometry

```yaml
task: 'Harden the Path Invariant: close enforcement hole + fix geometry leaks'
batch: lyxtest-geometry
number: 4
cards: 1
verify: go test ./internal/lyxtest/...
depends-on: [1]
```

## Batch Scope

Converts the geometry construction in the `lyxtest` support library. `internal/lyxtest/lyxtest.go`
is a non-`*_test.go` library file, so the production-only enforcement scan (batch 5) will read and
flag its `base+"-weft"` joins; routing them through `paths.WeftSiblingPath` closes the leak and
keeps the enforcement allowlist at `internal/paths` only. This is a single-package, single-file
change that depends only on batch 1 and shares no files with batches 2 or 3. It is its own batch
because `lyxtest` is a distinct leaf module from warp/board and its conversion is independent.

## Cards

### Card 17: Route lyxtest weft fixture paths through paths

- **Context:**
  - `internal/paths/paths.go`
  - `internal/lyxtest/lyxtest_test.go`
- **Edits:**
  - `internal/lyxtest/lyxtest.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace the three `filepath.Join(<parent>, base+"-weft")` sites (~lines 185,
  475, 541) with `paths.WeftSiblingPath(<parent>, base)`, where `<parent>` is the existing local
  (`tmpDir` / `tempContainer`). Add the `internal/paths` import if not already present — the
  lyxtest Leaf Invariant explicitly permits importing `internal/paths`. Do NOT convert the
  `base+"-weft-bare"` sites (~lines 207, 481): `-weft-bare` is a fixture suffix, not a geometry
  token, and is not flagged under whole-token matching. Leave the `"_lyx"` comment (~line 240)
  untouched. Paths produced must be byte-identical.
- **Commit:** `refactor(lyxtest): route weft fixture paths through paths.WeftSiblingPath`

## Batch Tests

`verify: go test ./internal/lyxtest/...` compiles `lyxtest` plus `lyxtest_test.go` and confirms the
import and conversions are correct. `lyxtest` is a test-support library consumed by warp/weft
integration suites; those dependents are exercised by batch 5's repo-wide `go test ./...`, which is
the cross-package parity backstop. No new assertions are needed — identical fixture paths mean the
existing dependent suites are the parity proof.
