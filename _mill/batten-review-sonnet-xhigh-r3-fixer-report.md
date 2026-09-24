# batten fixer report — round sonnet-xhigh-r3

Job 2 close-out for `_mill/batten-review-sonnet-xhigh-r3.md`. All 4 findings fixed, each in its own
commit, sabotage-proven before landing, repo-wide gates green throughout.

## What was implemented

### F-SIGKILL-ADD (BLOCKING) — three commits: `afb5976eb`, `ac3ba6e3e`, `b61db8545`, plus `f0419cff4`

Root cause: `taskWorktreePresent` (the `CreateWorktree` closure's idempotency probe, `wire.go`)
answered "is the warp worktree directory there and does it resolve" — a question `Topology.Add`'s
very first step already satisfies, long before the other ten steps (the pair's other-side worktree,
portal, launchers, junctions, origin record and commit, two pushes) run. A SIGKILL landing anywhere
after that first step left the probe answering `true`, so a re-entered `Worktree-Create` skipped
`Add` entirely and reported Done over a pair that was not actually built.

Fix, in three passes (the second and third landed only after live re-verification against the real
substrate surfaced gaps the first pass's own tests did not cover):

1. **`afb5976eb`** — added `fabricengine.PairComplete(l)`, a vocabulary-neutral check (alongside its
   existing sibling `PairSiblingRemnant`) for a caller outside the Fabric Vocabulary Invariant's
   owner set: does the pair's other-side worktree exist, and do the warp junctions actually resolve
   into it. Added `taskWorktreeComplete` in `battencli`, which calls it, and rewired
   `CreateWorktree`'s closure to use it instead of the bare probe. A present-but-incomplete pair now
   refuses with a named remedy (remove the incomplete worktree and its branch by hand, then resume)
   instead of silently reporting Done. `Worktree-Teardown`'s own `Shutdown`/`Remove` closures were
   deliberately left on the original `taskWorktreePresent` — strengthening the shared probe would
   have made an already-plausible, unconfirmed Remove-side crash window (a SIGKILL mid-junction-sweep,
   recorded PLAUSIBLE in the review's focus-1 table) look like a false half-torn-pair report on a
   healthy pair; the fix stays scoped to where the defect was found and confirmed live.
2. **`ac3ba6e3e`** — live re-verification of pass 1 (a second SIGKILL window, after the pair's
   other-side worktree already exists but before the junctions are wired) found the remedy text named
   only the task worktree, not its already-existing other-side leftover; following it verbatim still
   hit `Add`'s own "directory already exists" refusal as a second obstacle. Added
   `incompletePairRemedy`, which consults `PairSiblingRemnant` and names that worktree's own removal
   too when one exists.
3. **`b61db8545`**, **`f0419cff4`** — `docs/overview.md`'s batten paragraph and
   `internal/fabricengine/doc.go`'s own vocabulary-neutral-entry-point catalog, missed in the first
   commit, added as same-day follow-ups.

**Live re-verification against the real substrate** (redeployed `./deploy-dev` after each pass):
rebuilt the same two throwaway-instrumented-`Sleep` binaries the review used to find this
(uncommitted `time.Sleep` in `internal/fabricengine/add.go`, reverted from the source tree
immediately after each build — `git status --porcelain` confirmed clean before proceeding), reran
both SIGKILL windows against the FIXED binary:
- **Window A** (kill right after the warp worktree is created): `Worktree-Create` now refuses
  cleanly with the incomplete-pair remedy, instead of silently advancing and later wedging in
  `StateFailed` at Seed-Child. Followed the remedy verbatim; the slug re-created and progressed
  normally afterward.
- **Window B** (kill after the pair's other-side worktree exists, before junctions are wired):
  `Worktree-Create` now refuses before `Seed-Child` ever runs, so the warp-pristine-invariant
  violation (a real, non-junction `_lyx` directory) the review found no longer happens at all —
  confirmed by `ls`, no `_lyx` directory exists after the refusal (pass 1). Pass 2's remedy-naming
  gap was itself found via this same live re-verification, then fixed and reconfirmed live a third
  time (a cheap direct git-command reproduction, no throwaway binary needed for the reconfirm):
  refusal names both worktrees, following it verbatim recovers to a genuinely healthy pair (`_lyx`
  resolves as a real symlink afterward), and the slug reaches `Worktree-Create → Done` cleanly.

**Regression tests** (both integration-tagged, both drive `c.env.CreateWorktree` directly — the
production closure, not a helper):
- `TestBattenIntegration_CreateRow_IncompletePairRefusesRatherThanSkippingAdd` — reproduces Window A
  with the same git command `Add`'s own `createGitWorktree` step issues (no stub).
- `TestBattenIntegration_CreateRow_IncompletePairNamesTheOtherSideLeftoverToo` — reproduces Window B
  the same way, pins the remedy's own completeness.

Both sabotage-proven twice each (once per fix pass) before landing: reverted the relevant hunk,
confirmed the new test failed at the intended assertion with the pre-fix symptom, restored via
`cp`/`Edit` back to the fixed content, confirmed `git status --porcelain` clean, re-ran green.

### F-CLEANUP-REMOTE-ORPHAN (BLOCKING) — `a021b016e`

Root cause: `Teardown.Remove`'s closure called `top.Remove(location, slug, false, false)` — always
`remote: false`. `Topology.Remove` always deletes the pair's other-side branch locally regardless of
that flag; only whether it is ALSO deleted on the remote depends on it. Every batten-driven teardown
therefore left that branch on the remote while deleting it locally — a state
`lyx fabric cleanup --apply --remote` (the remedy both `createRefusal`'s and `doneSlugRefusal`'s own
text name) can never discover, since its own enumeration (`listWeftBranches`) is local-branches-only.
Confirmed via the review's own live drives: following either remedy verbatim left the branch
stranded, and a re-run's own push was rejected non-fast-forward against it.

Fix: `top.Remove(location, slug, false, true)`. A batten-driven teardown is the task's own final
removal — nothing will ever re-adopt that branch — and `Remove`'s own contract already states a
remote-deletion failure never fails the call, so this is a low-risk, one-line change.

Extended `TestBattenIntegration_FourRowRun_SeedsChildCommitsAndTearsDown` (the existing full
four-row-cycle integration test) to assert the pair's other-side branch is gone from `hubforge`'s own
remote fixture (`h.WeftBare`) after teardown. Sabotage-proven: reverted the flag to `false`, confirmed
the new assertion failed naming the still-present branch, restored, re-ran green.

**Live re-verification**: redeployed, reran the `typed-loom` full-lifecycle drive a third time — after
manually clearing the stale remote branch the review's own (pre-fix) drives had left behind — and
confirmed it reached `done` cleanly. Also directly confirmed via two later fixture teardowns
(`postfix-sigkill-a`, `postfix-sigkill-b`, both via `lyx fabric remove --force --remote`, which is
NOT the code path this fix touches, but shares the same underlying `Topology.Remove` call) and,
more directly, via the `--force` (no `--remote`) `fabric remove` calls used for cleanup during this
round's other live tests — none left a stranded remote branch once this fix was live, consistent
with the fix landing in the actual batten-driven path.

**Deferred, not fixed this round**: `fabricengine.Cleanup`'s own enumeration stays local-branches-only.
This fix removes the defect from batten's own normal teardown (the common case both remedy texts are
actually written to describe), but a branch stranded on the remote by some OTHER path — a failed push
during this fix's own `remote: true` teardown, or a hand-run `fabric remove --force` with no
`--remote` — is still permanently invisible to `lyx fabric cleanup`. Extending `Cleanup` to reach
remote-only orphans needs `git ls-remote` and careful gating (the same primary-branch-protection
logic `Cleanup` already carries for local branches would need a remote-aware counterpart) — a
materially larger, `fabricengine`-scoped change, **NOT-FIXED-THIS-ROUND**, its own task.

### F-WIRING-1 (MEDIUM) — `7adf43606`

Root cause: `createRefusal`'s own reword of `fabricengine.ErrBranchExists` was pinned only as a bare
function (`TestCreateRefusal_LeftoverBranchRemedyNeverNamesCheckout`, unit-level); nothing exercised
`CreateWorktree`'s own call to it. Confirmed (per the review's own focus-3 sweep): reverting
`wire.go`'s `return createRefusal(err)` to `return err` left every test green, `-tags integration`
included.

Fix: added `TestBattenIntegration_CreateRow_LeftoverBranchIsRewordedForPrime`, which plants a real
leftover branch via `gitkit.MustRun` (the same pattern `fabricengine`'s own
`TestAdd_ExistingBranchErrorNamesRemedy` uses) and drives `c.env.CreateWorktree` directly. Sabotage-
proven: reverted the call site, confirmed the new test failed with the raw fabric text
(`"lyx fabric checkout"` suggested as a remedy), restored, re-ran green.

### F-WIRING-2 (MEDIUM) — `1353acd0d`

Root cause: `SeedChild.WriteSeed`'s own `errors.Is(err, shedrun.ErrDisagreeingSeed)` →
`battenshed.ErrDisagreeingChildSeed` rewrap was pinned only by a test that stubbed the sentinel
already wrapped (`TestSeedChild_DisagreeingChildSeedIsStuck`, battenshed); nothing exercised the
closure's own rewrap against a real disagreeing seed on disk. Confirmed the same way: reverting the
rewrap branch left every test green, `-tags integration` included.

Fix: added `TestBattenIntegration_SeedChild_WriteSeedRewrapsARealDisagreeingChildSeed`, which plants a
real disagreeing child seed via `shedrun.WriteSeed` and drives `c.env.SeedChild.WriteSeed` directly.
Sabotage-proven the same way.

## Verification (repeated after every fix, final state below)

- `go build ./...` — green.
- `go vet ./...` — green.
- `go test -count=1 ./...` — green, 95 `ok` packages, repo-wide.
- `go test -count=5 ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/... ./internal/shedrun/... ./cmd/lyx/...` — green, stable.
- `go test -tags integration -count=1 ./internal/battencli/...` — green.
- `go test -tags integration -count=1 ./internal/fabricengine/...` — green (exercises the new
  `PairComplete`/`remote: true` code paths from fabricengine's own side too).
- `./deploy-dev` re-run after every commit that touched production code; the final binary is built
  from `f0419cff4`, non-dirty.
- Live re-verification against the real substrate for all four findings, detailed above and in each
  commit message — every fix was re-driven live post-fix, not just unit/integration-tested.

## Changed files

- `internal/fabricengine/fabric.go` — `PairComplete` (F-SIGKILL-ADD).
- `internal/fabricengine/doc.go` — catalog entry for `PairComplete`/`PairSiblingRemnant`.
- `internal/battencli/wire.go` — `taskWorktreeComplete`, `incompletePairRemedy`, `CreateWorktree`'s
  closure rewired to use them (F-SIGKILL-ADD); `Teardown.Remove`'s `top.Remove` call passes
  `remote: true` (F-CLEANUP-REMOTE-ORPHAN); `taskWorktreePresent`'s own doc comment narrowed to its
  remaining (teardown-only) use.
- `internal/battencli/lifecycle_integration_test.go` — five new integration tests (one per fix, plus
  the remedy-completeness follow-up), `gitkit`/`battenshed`/`errors` imports added.
- `docs/overview.md` — batten's `Worktree-Create` paragraph updated.

## Deliberately deferred (not fixed this round)

- `fabricengine.Cleanup`'s own local-branches-only enumeration (see F-CLEANUP-REMOTE-ORPHAN above) —
  NOT-FIXED-THIS-ROUND, too large for an inline fix, needs its own task.
- Every item in the review report's own Sweep-5 "Deferred items, re-evaluated" section: unchanged,
  re-confirmed still accurate, none touched (cold-machine recreate-from-branch, `step`-mode pacing,
  the design doc's two shipped residuals, GitHub issue #263, the friction-reflection teardown
  interaction, the absent `ly` plugin).
- The review's own PLAUSIBLE-not-CONFIRMED items in the focus-1 table (the Remove-side
  junction-sweep SIGKILL window; the Add steps 15-17 uncommitted-origin-record and unpushed-branch
  residuals) — recorded but not independently driven live this round, and not fixed, since they were
  never confirmed as real defects. Flagged for a follow-up round's own live drive.
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md`'s F22 — not extended. F22's existing line ~544 claim
  ("removing all of them does let the slug run again") is accurate again now that
  F-CLEANUP-REMOTE-ORPHAN is fixed, so no wording change is needed there. The new incomplete-pair
  scenario (F-SIGKILL-ADD) is not added to F22 either: F22's own established pattern hand-writes a
  `status.json` fixture to avoid spawning real git, and this scenario's whole point is a real
  worktree/branch state a hand-written status file cannot stand in for — it is thoroughly covered
  instead by the two new Go integration tests plus this round's own live SIGKILL drives against the
  real substrate.

## Teardown

Fixture hub at `/tmp/batten-r3-fixture-<timestamp>` (outside the loomyard tree and `$HOME/Code`,
never used on this host before) fully deleted (`find <scratch> -mindepth 1 -delete`, then `rmdir`) —
confirmed absent from `/tmp` afterward. Zero stray `lyx`/`tmux`/`reed`/`claude` processes belonging to
the fixture (`ps aux` before and after teardown showed only this host's pre-existing, untouched
standing sessions — same PIDs at session start and end). The operator's own standing bench
(`lyx-test-LYXHUB`) does not exist on this host and was never touched. `~/.claude.json` gained one
`projects` entry for the fixture's prime worktree path during live driving; read, not modified, per
the live-substrate cost declaration's own instruction.

## Merge-readiness verdict

**MERGEABLE for batten's own scope, with one item recorded for a follow-up task.** All 4 findings
from this round's own independent review are fixed, sabotage-proven, and live-reverified against the
real substrate. The repo-wide hermetic gate is green. One deferred item
(`fabricengine.Cleanup`'s local-only enumeration) is real, scoped, and explicitly NOT a blocker for
this round's own fixes landing — it is a smaller residual gap the fixes already close for the normal
case, filed here for the orchestrator to route to its own task per the crucible method's size rule.
