This confirms the BLOCKING finding. Batch 2's verify (`go test ./cmd/lyx/`) runs `main_test.go`, whose `TestRunDispatchesToGitClone` invokes `run([]string{"git-clone"}, ...)`. After card 6 removes the `git-clone` case, that test hits `default` (unknown module → exit 1 but no output), so its assertion `strings.Contains(out.String(), "ok":false)` fails. Card 6 edits only `main.go` and does not update `main_test.go`; `main_test.go` edits are deferred to card 10 (batch 3) and only cover the `worktree list` line.

MILL_REVIEW_BEGIN
# Review: Introduce warp: the host↔weft-coordinated git module — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opushigh
reviewed_file: plan/
date: 2026-06-25
```

## Findings

### [BLOCKING] main_test.go TestRunDispatchesToGitClone left stale
**Location:** Batch 2, card 6 (and card 10)
**Issue:** Card 6 removes the `git-clone` dispatch case in `main.go` but no card touches `cmd/lyx/main_test.go`'s `TestRunDispatchesToGitClone`, which calls `run([]string{"git-clone"}, &out)` and asserts `out` contains `"ok":false`; after removal it hits `default` (exit 1, **no output**), so the assertion fails under batch 2 verify `go test ./cmd/lyx/`. Card 10's main_test edit only covers the `worktree list`→`warp list` line.
**Fix:** Add `cmd/lyx/main_test.go` to card 6's Edits and replace/retarget `TestRunDispatchesToGitClone` to invoke `run([]string{"warp", "clone"}, …)` (assert the usage/error envelope) so the batch-2 dispatch is actually exercised.

### [NIT] Unified RunCLI must resolve per-case, not at top
**Location:** Batch 2 card 4 / Batch 3 card 9
**Issue:** Old `worktree.RunCLI` calls `paths.Resolve`+`LoadConfig` at the top before its switch; `gitclone.RunCLI` (clone) deliberately does neither (clone runs outside a git repo). Folding both into one `warp.RunCLI` risks a mechanical top-of-function `Resolve` that breaks `clone`. Card 9 implies per-case resolution but never states the hazard.
**Fix:** In card 9 add an explicit note: `Resolve`/`LoadConfig` live inside the add/list/remove (and later checkout/status/reconcile/prune/cleanup) cases; `clone` bypasses them.

### [NIT] init activation slug derivation vs HostJunctions(slug) geometry
**Location:** Batch 4, cards 11/14
**Issue:** `WireJunctions(l, slug)` keys on `HostJunctions(slug)` = `HostLyxLink(slug)` (Hub/slug-based), but init must wire the *current* worktree whose host link is `HostLyxLinkHere()` (WorktreeRoot-based); these coincide only when `filepath.Base(WorktreeRoot)==slug`. Card 14 says "cwd-keyed" but does not name the slug source.
**Fix:** Specify in card 14 that init passes `filepath.Base(l.WorktreeRoot)` as the slug (or have card 11 expose a cwd/here-keyed variant), so a subpath/renamed worktree wires the correct link.

## Verdict

REQUEST_CHANGES
One stale dispatch test breaks batch-2 verify; two clarifications prevent foreseeable implementer errors.
MILL_REVIEW_END
