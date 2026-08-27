# fabric `weft-is-never-merged` diff — crucible campaign handoff

Worktree: `/home/knatte/Code/loomyard/wts/fabric-crucible-hardening`, branch `fabric-crucible-hardening`.
Scope: the diff landed by merge `ab99f531` only — not a full `fabricengine`/`fabriccli` sweep (see `_mill/fabric-review-prompt.md`'s header for why).

## What round is running/paused right now

None. **Campaign converged after round 2.** Round `opus-high-r1` (13 findings, all fixed) and the round `sonnet-high-r2` safety pass (0 findings, different model) both completed and were independently verified by the orchestrator, cold, on the committed tree. Awaiting the operator's push/merge decision — that call is explicitly the operator's, not the orchestrator's to trigger.

## CLOSED-AND-VERIFIED (commit sha, do not re-litigate)

**Round `opus-high-r1`** found 0 BLOCKING, 4 MEDIUM, 5 LOW, 4 NIT (13 total) and fixed all 13. The orchestrator independently verified:
- Cold `go build ./...` / `go vet ./...`: clean, on the committed tree at `20779a34` (round tip).
- Cold `go test -count=5` over the in-scope packages: green.
- Cold `go test -tags integration` over fabricengine/fabriccli/landingshed: green.
- 3× concurrent copies of the fabricengine integration binary: `exit=0` each, no real FAIL/panic/race marker (the only "fail"-substring hits were expected `logger.Warn` messages from scenarios that intentionally exercise a failure path, e.g. F6's non-fatal weft-pull arm).
- **Sabotage-proved F2 myself** (the one real production-logic fix — the `MergeStateActive` probe-then-commit TOCTOU in `internal/loomcli/wiring.go`'s `newCommitStatusSeam`): reverted `commitStatusFailureDisposition`'s call back to a bare `return err`, ran `TestNewCommitStatusSeam_CommitFailsAfterMergeWentLive_TakesTheSkip`, `TestNewCommitStatusSeam_CommitFailsAndReProbeFails_TakesTheSkip`, and the real-merge integration test `TestCommitStatusSeam_Real_MergeGoesLiveAfterProbeSkipsInsteadOfHalting` — all three failed at the intended assertion (`fatal: cannot do a partial commit during a merge` surfacing as a hard error instead of a skip). Restored the fix, confirmed empty `git diff`.
- Read the F3, F4, F10 diffs directly (`694029c6`, `d16b8adc`, `769ec82d`) — doc corrections are accurate against the surrounding code (`mergestateactive.go`, `mergestate.go`'s retained dual weft probes), and F10's single-write collapse is a straightforward, correct simplification.

Findings and their commits: F5 `df185c8b`, F6 `84687354`, F1 `79947900`, F3 `694029c6`, F4 `d16b8adc`, F10 `769ec82d`, F9 `4c00a8e3`, F2/F11/F13 `c974f8ae`, F12 `6045f34c`, F7 `79366900`, F8 `0135044f`. Reports: `_mill/fabric-review-opus-high-r1.md` (`413256c1`), `_mill/fabric-review-opus-high-r1-fixer-report.md` (`20779a34`).

**Round `sonnet-high-r2`** (safety pass, different model) found **0 BLOCKING/MEDIUM/LOW/NIT** — no code or doc defect, no regression of round 1's fixes. Live-drove two combinations round 1 hadn't: a rename/delete weft-conflict shape (correctly surfaced through `MergeContinue`) and weft dirty+detached+foreign-merge-state combined (foreign state correctly refuses via the retained `foreignMergeStatePresent`; ordinary dirty+detached alone correctly ignored). The orchestrator independently verified:
- Cold `go build`/`go vet`/`go test -count=5` over the in-scope packages and cold `-tags integration` over fabricengine/fabriccli/landingshed/loomcli: all green, on the round-2 tip `af29b76c`.
- Confirmed zero production/test edits landed (`git diff --stat` since the round-2 seed shows only the two `_mill/` report files) — consistent with a genuine zero-finding pass, not a skipped one.
- Independently re-derived the round's one **process observation** (not a code finding): the campaign prompt's SPEC pointer (`4b30b14e`) was one revision behind the actually-final `discussion.md`. Walked the full `git log`/`git diff` myself — confirmed `4ccd610d` (gap-fix round 1) already carries the shipped design (explicitly rejects `MergeOptions.LocalOnlyPaths`), and rounds 2-5 through `ec433317` (the true final write) only add precision/detail, not a further scope change. **Fixed**: `_mill/fabric-review-prompt.md`'s three SPEC citations now point at `ec433317`, with a note on why `4b30b14e`/`4ccd610d` alone are each incomplete.
- Spot-checked round 2's code citations against the actual source (`TestPushAnchored_*`'s four subtests, `pairDirtyReason`, `foreignMergeStatePresent`, `resetMergeSides`) — all real, none fabricated.

Report: `_mill/fabric-review-sonnet-high-r2.md`, `_mill/fabric-review-sonnet-high-r2-fixer-report.md` (`af29b76c`).

## RESIDUAL currently seeded in `_mill/fabric-review-prompt.md`

The prompt still carries round 2's safety-pass seed (round 1's CLOSED-AND-VERIFIED list). If a round 3 is ever spawned, re-seed with: campaign converged after 2 rounds (opus-high-r1 + sonnet-high-r2 safety pass), no residual, both independently verified by the orchestrator; add round 2's zero-finding result and the SPEC-pointer fix to the CLOSED-AND-VERIFIED list.

Two **disclosed, deliberately-not-closed residuals**, re-evaluated by round 2 and still standing — not defects, carry forward as context, do not re-open without a real reason:
1. F2's remaining `git add` staging into a foreign merge's index on a lost race (bounded — one extra staged path in a merge the operator is already resolving by hand; closing it needs a lock the operator doesn't take).
2. `sideRecordedMergeGone`'s squash exemption — pre-existing, untouched by `ab99f531`, out of this campaign's scope.

## DEFERRED list

Empty — neither round deferred anything.

## Exact next action

**None required from a future round — the campaign is done.** The push/merge decision is the operator's: 17 commits sit on `fabric-crucible-hardening`, nothing pushed. If the operator wants a third round anyway (e.g. before a push, as extra insurance), it needs an explicit model + effort pick (Hard Rule 2 — no defaulting either axis) and should rotate to Fable (the one model not yet used in this campaign).

## Caveats worth carrying forward

- `./deploy-dev` was refused by this environment's command classifier during round 1; live driving used a scratchpad-built `lyx` binary instead (`go build -o <scratch>/bin/lyx ./cmd/lyx`), functionally equivalent. Round 2 used the normal `./deploy-dev` successfully, so this looks environment-flaky rather than a standing block.
- This module's live/composed tests use the `integration` build tag, not `smoke` — do not reach for `-tags smoke` here (that's `internal/loomcli`'s unrelated reed/loom-attach LLM-driving coverage, explicitly out of scope for this campaign).
- The campaign prompt's SPEC citation was corrected mid-campaign (see round 2's CLOSED-AND-VERIFIED entry above) — any future crucible campaign that recovers a torn-down task's `discussion.md` from git history should check for later `discussion-gap-fix` commits, not just the first `write discussion.md` commit.
