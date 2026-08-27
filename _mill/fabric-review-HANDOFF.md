# fabric `weft-is-never-merged` diff — crucible campaign handoff

Worktree: `/home/knatte/Code/loomyard/wts/fabric-crucible-hardening`, branch `fabric-crucible-hardening`.
Scope: the diff landed by merge `ab99f531` only — not a full `fabricengine`/`fabriccli` sweep (see `_mill/fabric-review-prompt.md`'s header for why).

## What round is running/paused right now

None. Round `opus-high-r1` completed and was independently verified by the orchestrator. Awaiting the operator's decision: safety-pass round 2 (different model), or stop here and hand to the operator for the merge/push decision.

## CLOSED-AND-VERIFIED (commit sha, do not re-litigate)

Round `opus-high-r1` found 0 BLOCKING, 4 MEDIUM, 5 LOW, 4 NIT (13 total) and fixed all 13. The orchestrator independently verified:
- Cold `go build ./...` / `go vet ./...`: clean, on the committed tree at `20779a34` (round tip).
- Cold `go test -count=5` over the in-scope packages: green.
- Cold `go test -tags integration` over fabricengine/fabriccli/landingshed: green.
- 3× concurrent copies of the fabricengine integration binary: `exit=0` each, no real FAIL/panic/race marker (the only "fail"-substring hits were expected `logger.Warn` messages from scenarios that intentionally exercise a failure path, e.g. F6's non-fatal weft-pull arm).
- **Sabotage-proved F2 myself** (the one real production-logic fix — the `MergeStateActive` probe-then-commit TOCTOU in `internal/loomcli/wiring.go`'s `newCommitStatusSeam`): reverted `commitStatusFailureDisposition`'s call back to a bare `return err`, ran `TestNewCommitStatusSeam_CommitFailsAfterMergeWentLive_TakesTheSkip`, `TestNewCommitStatusSeam_CommitFailsAndReProbeFails_TakesTheSkip`, and the real-merge integration test `TestCommitStatusSeam_Real_MergeGoesLiveAfterProbeSkipsInsteadOfHalting` — all three failed at the intended assertion (`fatal: cannot do a partial commit during a merge` surfacing as a hard error instead of a skip). Restored the fix, confirmed empty `git diff`.
- Read the F3, F4, F10 diffs directly (`694029c6`, `d16b8adc`, `769ec82d`) — doc corrections are accurate against the surrounding code (`mergestateactive.go`, `mergestate.go`'s retained dual weft probes), and F10's single-write collapse is a straightforward, correct simplification.

Findings and their commits: F5 `df185c8b`, F6 `84687354`, F1 `79947900`, F3 `694029c6`, F4 `d16b8adc`, F10 `769ec82d`, F9 `4c00a8e3`, F2/F11/F13 `c974f8ae`, F12 `6045f34c`, F7 `79366900`, F8 `0135044f`. Reports: `_mill/fabric-review-opus-high-r1.md` (`413256c1`), `_mill/fabric-review-opus-high-r1-fixer-report.md` (`20779a34`).

## RESIDUAL currently seeded in `_mill/fabric-review-prompt.md`

None rewritten yet — the prompt still reads as the round-1 seed. If a round 2 is spawned, rewrite its "Round context seeded from prior-round verification" section to a **safety pass** (no known residual; round 1 converged and was independently verified clean; do a genuinely independent clean-room pass with a different model, or confirm merge-readiness) and list the round-1 findings above under CLOSED-AND-VERIFIED so round 2 does not re-open them.

Two **disclosed, deliberately-not-closed residuals** from round 1's own fixer report — not defects, carry forward as context, do not re-open without a real reason:
1. F2's remaining `git add` staging into a foreign merge's index on a lost race (bounded — one extra staged path in a merge the operator is already resolving by hand; closing it needs a lock the operator doesn't take).
2. `sideRecordedMergeGone`'s squash exemption — pre-existing, untouched by `ab99f531`, out of this campaign's scope.

## DEFERRED list

Empty — round 1 deferred nothing (fixed all 13 findings, including every NIT).

## Exact next action

Ask the operator: safety-pass round 2 with a different model (Fable or Sonnet — round 1 was Opus/high), or stop the campaign here and surface merge-readiness for the operator's own push/merge decision. Do not spawn round 2 without an explicit model + effort pick (Hard Rule 2 — no defaulting either axis).

## Caveats worth carrying forward

- `./deploy-dev` was refused by this environment's command classifier during round 1; live driving used a scratchpad-built `lyx` binary instead (`go build -o <scratch>/bin/lyx ./cmd/lyx`), functionally equivalent. If this repeats in round 2, it is expected, not a new problem.
- This module's live/composed tests use the `integration` build tag, not `smoke` — do not reach for `-tags smoke` here (that's `internal/loomcli`'s unrelated reed/loom-attach LLM-driving coverage, explicitly out of scope for this campaign).
