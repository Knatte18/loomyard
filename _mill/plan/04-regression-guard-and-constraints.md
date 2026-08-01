# Batch: regression-guard-and-constraints

```yaml
task: 'fabric: audit and migrate all remaining direct git mutations onto Fabric'
batch: regression-guard-and-constraints
number: 4
cards: 1
verify: go test -run 'TestNoRawGitMutation|TestTierPurity_UntaggedTestsSpawnNothing|TestHermeticGitEnv' ./cmd/lyx/
depends-on: [2, 3]
```

## Batch Scope

Add the machine-enforced regression guard and reconcile the docs, in one commit (per the repo's Documentation Lifecycle rule — the guard is new cross-cutting infrastructure, so CONSTRAINTS.md lands with it). A new Tier-1 substring-scan test in the cmd/lyx package bans the two construction/call tokens `gitrepo.New(` and `gitexec.RunGit(` in the internal/websterengine package/the internal/builderengine package production source, file-allowlisting exactly the grandfathered read-only exemptions. The existing Test Tier Purity guard is told about the new guard file (which carries the banned tokens as its own scan data). CONSTRAINTS.md's Fabric Git Invariant is updated: the "Known gap, tracked" bullet closes (both instances migrated) and the enforcement note records the new machine check. Depends on batches 2 and 3 — the guard scans the live tree and only passes once both migrations have removed the raw tokens.

## Cards

### Card 10: Add the raw-git-mutation regression guard and reconcile Test Tier Purity + CONSTRAINTS

- **Context:**
  - `cmd/lyx/gitrepoboundary_test.go`
  - `cmd/lyx/hermeticenv_test.go`
  - `internal/websterengine/gitwrap.go`
  - `internal/websterengine/integration.go`
  - `internal/builderengine/gitquery.go`
  - `internal/builderengine/chain.go`
- **Edits:**
  - `cmd/lyx/tierpurity_test.go`
  - `CONSTRAINTS.md`
- **Creates:**
  - `cmd/lyx/rawgitmutation_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - Create `cmd/lyx/rawgitmutation_test.go` in `package main`, untagged (Tier 1 — its first non-empty line is the doc comment, NOT a `//go:build` constraint), modeled on `cmd/lyx/tierpurity_test.go`'s mechanics. It defines `TestNoRawGitMutation_WebsterBuilderProductionSource(t *testing.T)` which: resolves the module root via `exec.Command("go", "env", "GOMOD")` (skip cleanly if the go toolchain is absent or `GOMOD` is empty, exactly as `tierpurity_test.go` does); walks the two package subtrees the internal/websterengine package and the internal/builderengine package under the module root with `filepath.WalkDir`; for every `.go` file that is NOT a `_test.go` file, reads its source and flags it if it contains either raw substring `gitrepo.New(` or `gitexec.RunGit(`; normalizes each path to module-relative slash form (`filepath.ToSlash`) before any comparison; and skips flagging for files on a per-file allowlist. The allowlist is a `map[string]string` (path → reason) containing exactly `internal/websterengine/gitwrap.go` (reason: grandfathered read-only exemptions — `CurrentSHA` via `gitrepo.New` and `status --porcelain` via `gitexec.RunGit`) and `internal/builderengine/gitquery.go` (reason: grandfathered read-only exemptions — `HeadSHA`/`ChangedFiles`/`Dirty` via `gitexec.RunGit`). On any non-allowlisted flagged file, `t.Errorf` naming the file, the token, and pointing at CONSTRAINTS.md's Fabric Git Invariant. Include a vacuous-scan guard: `t.Fatalf` if fewer than 4 production `.go` files were scanned across the two packages (the walk is misconfigured otherwise).
  - The guard bans the two CONSTRUCTION/CALL tokens, not per-verb method names — per the discussion's `regression-guard` decision, a verb-name ban would both flag the correctly-migrated consumer code (which legitimately calls `.CheckoutDetached(`/`.ResetHard(` on the new interface) and miss the raw `gitexec.RunGit(` bypass that had no method token at all.
  - In `cmd/lyx/tierpurity_test.go`, add one entry to the `allowedSpawners` map: key `"cmd/lyx/rawgitmutation_test.go"`, reason noting it contains the banned `gitexec.RunGit`/`exec.Command` token strings as its own scan data (mirroring the existing `gitrepoboundary_test.go`/`ghguard_test.go` entries). Without this, the existing Test Tier Purity guard would flag the new file for containing `gitexec.RunGit` and `exec.Command`. No `hermeticenv_test.go` edit is needed: the cmd/lyx package's existing `TestMain` already satisfies the Hermetic Git Test Environment Invariant at package granularity (the same reason `gitrepoboundary_test.go` needs no `allowedNonHermetic` entry) — the batch `verify` runs `TestHermeticGitEnv` to confirm.
  - In `CONSTRAINTS.md`, under `## Fabric Git Invariant (warp + weft)`: (1) rewrite the "**Known gap, tracked:**" clause in the Module-ownership bullet — both instances (the internal/websterengine package's bisect/verify `CheckoutDetached`/`RestoreBranch` path and the internal/builderengine package's `ResetHard` chain-rollback) now dispatch through the internal/fabricengine package's warp-only methods (`Fabric.CheckoutDetached`/`RestoreBranch`/`CurrentBranch`/`ResetHard`); state the gap is closed and that these two packages' mutating-git non-bypass is now machine-checked. (2) In the "**Enforced by**" bullet, add the new guard alongside the existing boardengine machine-check: `cmd/lyx/rawgitmutation_test.go` (`TestNoRawGitMutation_WebsterBuilderProductionSource`) bans `gitrepo.New(`/`gitexec.RunGit(` in the internal/websterengine package/the internal/builderengine package production source (file-allowlisting `gitwrap.go`/`gitquery.go`'s read-only exemptions). Keep the general-case wording unchanged: every OTHER `fabricengine` caller remains a review obligation, not machine-checked. Write CONSTRAINTS.md prose as one line per paragraph/bullet (no hard-wrapping), matching the file's existing style.
  - Do not edit any production source in this batch — the guard must pass against the tree exactly as batches 2 and 3 left it.
- **Commit:** `test(cmd/lyx): guard websterengine/builderengine against raw git mutation; close the Fabric Git Invariant gap`

## Batch Tests

`verify: go test -run 'TestNoRawGitMutation|TestTierPurity_UntaggedTestsSpawnNothing|TestHermeticGitEnv' ./cmd/lyx/` runs the new guard (which passes only because batches 2+3 removed `gitrepo.New(` from `runlevel.go` and the mutating `gitexec.RunGit(` from `gitquery.go`), plus the Test Tier Purity guard (confirming the new file's `allowedSpawners` entry keeps it green) and the Hermetic Git Test Environment guard (confirming the new file needs no `allowedNonHermetic` entry). All three are Tier 1 — no `-tags integration`.

**Manual verification step (not a persistent test):** after this batch, re-run the task's own audit grep across the whole tree to confirm zero mutating git call sites remain outside the internal/fabricengine package/`internal/gitexec`/`internal/gitrepo` (beyond the documented read-only exclusions): `grep -rn "gitexec.RunGit\|gitrepo\.\|exec.Command.*\"git\"" internal/ cmd/ --include="*.go" | grep -v "_test.go"`, then eyeball that every remaining hit is either inside the three mechanism packages, a read-only verb (`rev-parse`/`status --porcelain`/`diff --name-only`/`CurrentSHA`), or `internal/fabriccli`. The new the cmd/lyx package guard mechanizes this going forward for the two migrated packages; the re-grep is the one-time whole-tree confirmation the task's step 4 asks for.
