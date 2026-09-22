# batten fixer report — sonnet-xhigh-r3

Companion to `_mill/batten-review-sonnet-xhigh-r3.md`. Two findings recorded, both fixed.

## What was implemented

### F1 [LOW] — `cancelErr` not consulted on every hard-error exit path

Files changed: `internal/battenshed/create.go`, `teardown.go`, `seamchild.go`, `innerrun.go`, `ctx.go`.

Added a `cancelErr(ctx, p.name)` check ahead of the returned hard error on the seven sites that follow a genuine seam call (a closure
invocation that may itself block or do I/O) with no such check: `create.go`'s and `teardown.go`'s `PrimeLock.Acquire` failure,
`seamchild.go`'s `ChildDriver` failure and `WriteSeed` non-refusal failure, and `innerrun.go`'s `ResolveStatus` failure and both
`ReadStatus` failures (pre-spawn and post-spawn). `innerrun.go`'s `StateBlocked`/`StatePaused`/`StateFailed` and unrecognized-state hard
errors were investigated and found NOT to need the change — both are reached by a `switch` immediately following an existing `cancelErr`
check with no seam call in between, so adding a second check there would be dead code the codebase's own pattern does not otherwise carry.
Corrected `ctx.go`'s own doc comment (both the file header and `cancelErr`'s own comment) to state the precise rule this fix establishes,
rather than the imprecise "every non-Done exit path" the code never actually matched.

Verification: `go build ./...`, `go vet`, `go test -count=5` all green before and after. Reverted the `create.go` fix temporarily and
confirmed the new regression test fails without it (`TestWorktreeCreate_CancelledDuringAcquireError`), then restored it and re-confirmed
green — proving the test would have caught this finding.

### F2 [NIT] — duplicated child-driver defaulting logic

Files changed: `internal/battencli/arm.go`, `wire.go`.

`wire.go`'s `SeedChild.ChildDriver` closure now calls `arm.go`'s `childDriverOf(seed)` instead of re-implementing the same two-line
defaulting check inline. Pure refactor, no behavior change (both implementations were already byte-identical). Updated both functions'
doc comments to state the sharing explicitly.

## Test commands run and results

```
go build ./...                                                                              — clean
go vet ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/...       — clean
go test -count=5 ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/... ./cmd/lyx/...   — ok (all four packages)
go test -tags integration -count=1 ./internal/battencli/...                                 — ok
```

All four commands were run and green after F1, again after F2, and a final time before this report was written. Also re-run once with F1's
`create.go` fix reverted, to confirm `TestWorktreeCreate_CancelledDuringAcquireError` fails without the fix (it does), then restored.

## New tests added (would have caught each finding)

- `TestWorktreeCreate_CancelledDuringAcquireError` (`create_test.go`)
- `TestWorktreeTeardown_CancelledDuringAcquireError` (`teardown_test.go`)
- `TestSeedChild_CancelledDuringChildDriverError`, `TestSeedChild_CancelledDuringWriteSeedError` (`seamchild_test.go`)
- `TestInnerRun_CancelledDuringResolveStatusError`, `TestInnerRun_CancelledDuringFirstReadStatusError`,
  `TestInnerRun_CancelledDuringSecondReadStatusError` (`innerrun_test.go`)
- `TestWire_ChildDriverDelegatesToChildDriverOf` (`wire_test.go`)

All seven are deterministic (no `time.Sleep` of any real duration; cancellation is triggered synchronously inside the faked seam closure,
never raced against a timer) and pass under `-count=5`.

## Post-fix live re-verification

Re-deployed the dev binary (`./deploy-dev`, `lyx @ 0955784d9`) against the same disposable fixture hub used for the review's own live
driving. Neither fix changes observable behavior in the normal (non-cancelled, non-race) path, so the full ~13.5-minute primary end-to-end
drive was not repeated (per the round's own "re-run only if a fix genuinely requires re-proving the whole path" guidance) — instead ran a
light live sanity cycle with the rebuilt binary against a fresh slug (`postfix-sanity-slug`): `Worktree-Create` → `Seed-Child` → (hand-set
child status to `done`, matching the same fixture technique used throughout the review) → `Run-Shed` → `Worktree-Teardown`, all `outcome:
"done"`, pair fully removed from disk. Also re-confirmed the Batten Bookend Invariant's non-prime refusal still fires correctly
(`lyx batten status <slug>` from the weft sibling, same refusal text as before the fix).

## Deferred items (none from this round's own findings — both F1 and F2 were fixed in full)

Re-affirming the pre-existing deferred items re-evaluated during the review (not new deferrals from this round's own findings; see the
review report's own "Deferred items" section for the full reasoning on each):
- R1-F9's recreate-from-branch half — blocked on a fabric capability that does not exist; cross-cutting, a different module's own change.
- R1-F6's `step`-mode pacing cost — a `shedengine` change, out of this round's scope.
- F6[R2]'s llm-driver trust-dialog hang — cross-module (`shuttleengine`/`loomcli`), not batten's own bug; could not be reproduced this
  round on a genuinely fresh worktree, recorded honestly as "not reproduced" rather than "fixed."
- The design doc's two consciously-shipped residuals (dead-strand detection; auto-teardown of a finished driver's strand/run-dir) — both
  re-confirmed accurate, one live-demonstrated this round; no code or doc change needed.

## Changed files (this round, both findings)

- `internal/battenshed/create.go`
- `internal/battenshed/create_test.go`
- `internal/battenshed/ctx.go`
- `internal/battenshed/innerrun.go`
- `internal/battenshed/innerrun_test.go`
- `internal/battenshed/seamchild.go`
- `internal/battenshed/seamchild_test.go`
- `internal/battenshed/teardown.go`
- `internal/battenshed/teardown_test.go`
- `internal/battencli/arm.go`
- `internal/battencli/wire.go`
- `internal/battencli/wire_test.go`
- `_mill/batten-review-sonnet-xhigh-r3.md`
- `_mill/batten-review-sonnet-xhigh-r3-fixer-report.md` (this file)

No `docs/overview.md` or `CONSTRAINTS.md` change: neither finding changes observable CLI behavior, a module boundary, or a cross-cutting
invariant — F1 only sharpens an already-internal error-message diagnosis under a race window narrow enough that no existing doc describes
it at that level of detail, and F2 is a pure internal refactor.
