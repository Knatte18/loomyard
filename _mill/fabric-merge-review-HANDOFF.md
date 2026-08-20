# fabric merge — crucible campaign HANDOFF (orchestrator-only, round 4+)

Off-limits to round agents: this file matches the `fabric-merge-review-*` pattern the round prompt declares unreadable.

**Last refreshed:** 2026-08-20, after round 6's verification.

## What this task is

Mill task `fabric-merge-crucible-round4`, worktree `/home/knatte/Code/loomyard/wts/fabric-merge-crucible-round4`, branch of the same name, based on `main` (the prior campaign's task-85 worktree was merged and torn down; `_mill/fabric-merge-review-*` history lives at `archive/fabric-merge-crucible-hardening~1`).

Continuation of the campaign task 85 (`fabric-merge-crucible-hardening`) ran and stopped after round 3, closing V1 (round 3's `sideConcludeAlreadyLanded` adoption arm — silent false success on an unrelated commit) and V2 (the AST closure test scoped to `mergeerrors.go` only), then continuing the crucible loop as ordinary rounds.

Orchestrator-driven, not mill-go. The task's mill phase stays `discussing`; nothing in this campaign advances it. **The orchestrator role for this task is Claude itself (no separate human operator steering per-round)** — the user gave the round plan up front and is not steering live; still apply every Hard Rule from `crucible/orchestrator-prompt.md`, especially never trusting a round's self-verdict.

## Operator's round plan (given 2026-08-20, this task)

Exactly four rounds, model + effort fixed in advance, UNLESS convergence is reached first:

| Round | Model | Effort | Tag | Status |
|---:|---|---|---|---|
| r4 | Opus | medium | `opus-medium-r4` | **done, independently verified** — 7 findings (1 BLOCKING, 2 MEDIUM, 2 LOW, 2 NIT), all fixed, all sabotage-proven, BLOCKING fix live-redriven in all 3 directions |
| r5 | Fable | high | `fable-high-r5` | **done, independently verified** — 7 findings (3 MEDIUM, 1 LOW, 3 NIT), all fixed, all sabotage-proven, both MEDIUMs live-redriven |
| r6 | Opus | high | `opus-high-r6` | **done, independently verified** — 9 findings (3 MEDIUM incl. 2 genuine NEW behavioral defects, 3 LOW, 3 NIT), all fixed, all sabotage-proven, both behavioral fixes verified (F1 live-redriven, F6 verified by inspection + sabotage since no Windows host exists) |
| r7 | Opus | medium | `opus-medium-r7` | **awaiting operator decision — see "Next action"** |

Round 4 hit a genuine watchdog stall (600s no-progress) after an earlier tool-use rejection; recovered by resuming the same agent from its transcript rather than spawning fresh — its commits were already intact per-fix. Not a deliberate operator stop; logged here per the method's "genuine stall" recovery path (same class as the reed campaign's round 1).

Round numbering continues from the prior campaign's r1–r3 (this campaign's round 4 was originally planned as `opus-high-r4` and never spawned; the operator has now replaced that single-round plan with this four-round rotation instead — the numbering stays continuous, the model/effort assignment does not match the original plan).
Hard Rule 2 (explicit effort pick required before every spawn) is satisfied for all four by this table.
Convergence = a safety-pass round (self-reports nothing new) AND the orchestrator's independent gates agree — see "Convergence" below. If reached before r7, stop there and report; do not spawn the remaining rounds.

## Baseline established before round 4 (all green, committed tree at `4471041a`)

- `go build ./...`
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...`
- `go test -count=1 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...`
- `go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — fabricengine 28.1 s, fabriccli 2.2 s, gitrepo 1.3 s

**V1 and V2 confirmed still present in the current tree before spawning r4** (read directly, not assumed from the prior HANDOFF):
- V1: `sideConcludeAlreadyLanded` (`internal/fabricengine/mergelifecycle.go:105-121`) has no second-parent / squash-vs-non-squash discriminator — matches the prior campaign's description exactly.
- V2: `TestMergeVocabulary_GuardReasonSetMatchesConstBlock` (`internal/fabricengine/mergevocab_test.go:50`) parses `"mergeerrors.go"` as a hardcoded filename — matches exactly.

## Campaign-specific facts carried forward (do not rediscover)

- fabric's live tier is `-tags integration`, not `-tags smoke`. Zero `//go:build smoke` files under `internal/fabricengine`, `internal/fabriccli`, `internal/gitrepo`.
- Not LLM-driving. Substrate is real git repos. N-concurrent amplifier is safe but NOT required — the merge bar is single-instance correctness (deliberate scoping carried from the prior campaign).
- `./deploy-dev` is the POSIX deploy script (not `.cmd`) on this host.
- Hub recipe: `GIT_CONFIG_GLOBAL` with `[init] defaultBranch = main` BEFORE the first `git init`, bare warp + bare weft, `lyx fabric clone`. A merge source must be a fabric-managed pair (branch exists on both warp and weft).
- `internal/fabricengine/doc.go`'s "# The merge surface" section (~line 846–960) is the authoritative prose contract.
- SPEC lives only in git history: `git show 3b800bc8:_mill/discussion.md` + `3b800bc8:_mill/plan/0*.md`; rejected alternatives at `967916ea:_mill/discussion-meta.md`.
- Full round 1–3 history (findings, sabotage proofs, live-drive recipes): `git show archive/fabric-merge-crucible-hardening~1:_mill/fabric-merge-review-HANDOFF.md`.

## Round 4 (`opus-medium-r4`) — COMPLETE, independently verified

Seed commit `9e6d6e5c`. Round commits `505d50a3` → `142c99a3` (8 commits): review report, F2 (V2), F1 (V1), F3, F5, F4+F6, sandbox F20 coverage, fixer report.
Reports: `_mill/fabric-merge-review-opus-medium-r4.md`, `_mill/fabric-merge-review-opus-medium-r4-fixer-report.md`.
Self-verdict READY TO MERGE. **The orchestrator's independent verification agrees — this is the first round in either campaign instalment whose self-verdict held up completely.**

Findings: BLOCKING 1 (R4-F1 = V1) · MEDIUM 2 (R4-F2 = V2, R4-F3) · LOW 2 (R4-F4, R4-F5) · NIT 2 (R4-F6, R4-F7). All fixed, nothing deferred as unfixed. Two genuine coverage gaps stated plainly rather than claimed closed: Windows path behaviour (still unexecuted, still needs a Windows host) and round 2's 45-row per-site adjudication (not re-walked, not required this round).

### What the orchestrator verified itself

**Gates, from cold on the committed tree, run twice (once mid-verification after sabotage/revert cycles, once final) — all green:**
`go build ./...`; `go vet ./...`; `go test -count=5` across `fabricengine`/`fabriccli`/`gitrepo`/`cmd/lyx`; `go test -tags integration -count=1 -timeout 30m` (fabricengine ~47-50s, fabriccli ~3.5-4.4s, gitrepo ~1.9-2.6s).

**Sabotage proofs — every one run by the orchestrator, watched to fail at the intended assertion, then restored to an empty diff:**

| # | Mechanism sabotaged | Test(s) | Result |
|---|---|---|---|
| S1 | `sideConcludeAlreadyLanded` reverted to the ambiguous HEAD-moved-only read (R4-F1) | `TestMergeContinue_UnrelatedCommitWhileRecordLive_IsNeverAdopted`, `TestMergeContinue_SquashConcludeLandedByHand_IsNeverAdopted` | both failed at `committed true, error <nil>; want *ErrMergeIncomplete` |
| S2 | a `mergeReason*` constant declared in `mergeguards.go` instead of `mergeerrors.go` (R4-F2) | `TestMergeVocabulary_GuardReasonSetMatchesConstBlock`, `TestMergeVocabulary_GuardReasonSetIsDeclaredInOneFile` | both failed correctly; first sabotage attempt (const before `package`) was a build break and was redone after the import block, per the method's "a build break is not a proof" rule |
| S3 | `Merge`'s weft write lock moved back to after the pre-merge sync (pre-fix order, R4-F3) | `TestMerge_PreMergeSyncRunsInsideTheWriteLock` | failed at `Stat(...) while the lock was held = <nil>; want not-exist` |

**The BLOCKING fix (R4-F1/V1) re-driven live by the orchestrator, freshly deployed binary (`142c99a3`), three fresh hubs, all three directions:**

- **Adversarial** (hub2): conflicted `merge-in feat`, `git merge --abort` + one unrelated commit, then `lyx fabric merge --continue` → `{"error":"...merge conclude did not finish...","ok":false}`, record retained, `git merge-base --is-ancestor <feat-source> HEAD` confirms `feat` NOT merged. `merge --abort` afterwards also refuses (`merge conclude already landed`) — both verbs consistent, honestly stuck. Exactly what the round claimed.
- **Legitimate** (hub3): conflicted `merge-in feat2`, resolved by hand, `git commit --no-edit` while `MERGE_HEAD` still live, then `merge --continue` → `committed:true` naming the hand-landed SHA, HEAD unmoved (the hand-landed commit itself, no second commit fabricated), record cleared.
- **Squash** (hub4): `merge feat3 --squash` with a failing `pre-commit` hook, hand-landed squash conclude via plain `git commit` (confirmed one parent), `merge --continue` → refuses (`merge conclude did not finish`), record retained (`squash: true`, `warp_committed: ""`), HEAD left at the hand-landed commit untouched.

Docs (`doc.go`'s "# The merge surface") read as accurate against the new code — the claim-vs-refusal asymmetry, the parentage evidence, squash non-adoption, and the corrected plain-git recovery instructions are all present and match what was driven live.

### RESIDUAL — what verification left standing

**None found.** This is the first round in either campaign instalment to survive independent verification with zero residual. One thing to hand to round 5 rather than treat as closed: round 4's own flagged judgement call — `parents[0] == start` makes adoption refuse a legitimate recovery if the operator did anything else on the branch first before hand-landing the conclude. Safe direction, documented, but an ergonomics tradeoff a fresh reviewer should independently re-examine rather than inherit as settled.

### Honest limits of this verification

- Windows path behaviour in `weftPathVisible`/`unifyConflictPaths` remains unexecuted by every round and the orchestrator, on every campaign. Linux host, reasoned about, never driven.
- The N-way concurrent amplifier was not run — not required for this module (see the round prompt's "Merge bar" section).
- Scratch hubs (`hub1`–`hub4`, plus the round's own `h1`–`h4`) live under the session scratchpad, outside the repo. `git status` in the worktree is clean throughout.

## Round 5 (`fable-high-r5`) — COMPLETE, independently verified

Seed commit `b9dd4174`. Round commits `1ca62d61` → `66a04e43` (11 commits): review report (built incrementally), sabotage-check log (exposed F7), then F1/F7/F3/F5/F6/F4/F2 fixes in that order, sandbox coverage extension, fixer report.
Reports: `_mill/fabric-merge-review-fable-high-r5.md`, `_mill/fabric-merge-review-fable-high-r5-fixer-report.md`.
Self-verdict: NOT merge-ready yet (not because of open defects — none — but because a round seeded "no known residual" still found 7 real things, so no clean safety pass has landed yet). **The orchestrator's independent verification agrees with both halves of that: all 7 fixes hold up, and this is correctly not being called convergence.**

Findings: MEDIUM 3 (F1 non-ASCII conflict paths, F2 lifecycle-guard TOCTOU, F7 adoption-arm parentage proof gap) · LOW 1 (F3) · NIT 3 (F4, F5, F6). All fixed, nothing deferred as unfixed.

**F7 is the campaign's signature finding repeating: round 4's OWN new parentage-evidence clauses (`parents[0]==start` and source-SHA membership) had zero test coverage** — the existing adversarial test's fixture is a one-parent commit, refused by the parent-count check alone, so neither clause could fail its own test if deleted. Round 5 found this by re-sabotaging round 4's already-shipped fix, not by reviewing new code — the same "sabotage a test, not a fix" shape as first-instalment residual A/V2.

### What the orchestrator verified itself

**Gates, from cold on the committed tree, run twice (mid-verification and final) — all green:**
`go build ./...`; `go vet ./...`; `go test -count=5` across all four packages; `go test -tags integration -count=1 -timeout 30m` (fabricengine ~31s, fabriccli ~2.3-2.6s, gitrepo ~1.4-1.6s).

**Sabotage proofs — every one run by the orchestrator, watched to fail at the intended assertion, then restored to an empty diff:**

| # | Mechanism sabotaged | Test(s) | Result |
|---|---|---|---|
| S1 | `parents[0] != start` clause deleted from `sideConcludeAlreadyLanded` (F7) | `TestMergeContinue_MergeOfSourceOntoWrongBase_IsNeverAdopted` | failed at `want no KindMergeCommitted — a wrong-base merge is a merge of a different base`; the sibling wrong-source test correctly stayed green (doesn't exercise this clause) |
| S2 | source-SHA membership loop deleted from `sideConcludeAlreadyLanded` (F7) | `TestMergeContinue_MergeOfWrongSourceOntoStart_IsNeverAdopted` | failed at `want no KindMergeCommitted — a merge of some other branch is not this merge's conclude`; the sibling wrong-base test correctly stayed green |
| S3 | `MergeAbort`'s lock acquisition moved back before the record load + guard (pre-F2 order) | `TestMergeAbort_ConcludeLandingWhileWaitingForLock_RefusesInsteadOfResetting` | failed at `want *fabricengine.ErrNoMergeInProgress` — the destructive TOCTOU race reproduces exactly as F2's finding describes |
| S4 | `ConflictedFiles` reverted to `git diff --name-only --diff-filter=U` (no `-z`, F1) | `TestMergeConflictedFiles_NonASCIIPathIsRawNeverQuoted`, `TestMergeIn_NonASCIIConflictPaths_ReportedRawNotQuotedNotUnmergeable` | both failed, reproducing the exact pre-fix bug: C-quoted path returned, then self-aborts as `*ErrUnmergeableState` for an in-tree conflict |

**Both MEDIUM behavioral fixes re-driven live by the orchestrator**, freshly deployed binary, fresh isolated hub (the shared scratch warp/weft-bare pair from V1 verification had accumulated branch names across many ad-hoc hub clones and collided — switched to a dedicated fresh bare pair, `warp-bare2`/`weft-bare2`, for this round's live checks):

- **F1** (hub8): add/add conflict on `_lyx/ä-note.md` on both sides → `merge-in` reports `"conflicts":["_lyx/ä-note.md"]` — raw, correctly mapped, no self-abort. Resolved by hand (edit through the warp `_lyx` junction, `git add` in the weft checkout — the junction means the edit alone doesn't stage it, matching how the operator would actually work), `merge --continue` → `committed:true`, record cleared, resolved content landed. Full end-to-end proof, not just the negative (no-longer-self-aborts) half.
- **F2**: verified by sabotage only (S3 above) — a real cross-process race is exactly what the round's five deterministic external-lock-hold tests exist to make reproducible without timing luck; re-deriving the race by hand outside a test harness would be strictly less rigorous than what was already sabotage-confirmed.

### RESIDUAL — what verification left standing

**None found.** Both MEDIUM fixes and the LOW/NITs hold up under independent sabotage and (for F1) live re-drive.

### Honest limits of this verification

- Windows path behaviour remains unexecuted — two instalments running now, still Linux-only.
- F2 was verified by sabotage of the deterministic tests, not by an independently-authored race reproduction — judged sufficient (see above) but noting the distinction from F1's full live re-drive.
- Scratch hubs (`hub5`–`hub8`) live under the session scratchpad, outside the repo. `hub5`/`hub6`/`hub7` hit branch-name collisions against a shared bare pair reused across many verification passes across this session — not a fabric defect, an artifact of scratch reuse; noted so a future orchestrator doesn't waste time on it. `git status` in the worktree is clean throughout.

## Round 6 (`opus-high-r6`) — COMPLETE, independently verified

Seed commit `3e7d4f56`. Round hit a transient Cloudflare/API error (522, retryable) mid Job-1 after its baseline-gates commit had already landed — resumed the same agent, per the reed campaign's "genuine stall" recovery path (not an operator stop). Round commits `f5ec8a17` → `c3e9746e` (12 commits): baseline gates, sabotage-sweep + live-driving log, complete review (9 findings), F1–F9 fixes one per commit, fixer report.
Reports: `_mill/fabric-merge-review-opus-high-r6.md` (16-row sabotage sweep table + full live-driving log), `_mill/fabric-merge-review-opus-high-r6-fixer-report.md`.
Self-verdict READY TO MERGE, with an explicit caveat the round wrote itself: **"this is the third consecutive round to find real material in the immediately preceding round's own shipped work; I would not read this round's clean gates as evidence of convergence."** The orchestrator's independent verification agrees with the findings and fixes; it also agrees with that caveat.

Findings: MEDIUM 3 (F1, F2, F3) · LOW 3 (F4, F5, F6) · NIT 3 (F7, F8, F9). All fixed, nothing deferred as unfixed. **Two of these are genuinely NEW behavioral defects, not proof-quality gaps in existing tests** — the first time since round 4 that a round found something beyond "a mechanism has no test."

- **F1 — the octopus-merge adoption bug (MEDIUM, CONFIRMED live).** Round 4's parentage-evidence fix (`sideConcludeAlreadyLanded`) checked `len(parents) < 2` — a *lower* bound — and scanned all remaining parents for the source SHA. An operator who discards a staged merge and runs `git merge <recorded-source> <unrelated-branch>` in one command builds a genuine 3-parent octopus whose first parent is the recorded start and whose second is the recorded source — satisfying the loose check. Fabric silently adopted it as its own conclude, recorded correspondence, deleted the record, and left the unrelated branch's content on the pair with nothing in any `merge_staged` entry accounting for it. Fixed to exact equality: `len(parents) != 2 || parents[0] != start || parents[1] != sourceSHA`. This survived round 4's own fix, round 5's independent re-review, AND two full orchestrator verification passes (this task's round-4 and round-5 verifications) before round 6 found it — three-for-three rounds and two-for-two orchestrator passes missed it at the loose arity.
- **F6 — Windows path separator bug (MEDIUM, PLAUSIBLE — traced, not driven; no Windows host exists in this campaign).** `weftPathVisible` compared git's always-forward-slash conflict paths against `anchorRel`, an OS-separator path from `lyxcwd.ValidateAnchorRel`. On Windows, a multi-segment anchor (e.g. `apps/backend`) arrives as `apps\backend`, breaking every weft-side conflict-path prefix match under that anchor and self-aborting the whole merge. Fixed with `filepath.ToSlash` (provably identity on Linux, so cannot regress this host) rather than a naive `strings.ReplaceAll`, which would corrupt a Linux directory legitimately named with a backslash. This upgrades the campaign's long-standing "Windows untested" caveat from a blanket unknown to a traced, fixed, line-level defect — still never executed on real Windows, five rounds and two instalments running.

### What the orchestrator verified itself

**Gates, from cold on the committed tree, run twice (mid-verification and final) — all green:**
`go build ./...`; `go vet ./...`; `go test -count=5` across all four packages; `go test -tags integration -count=1 -timeout 30m` (fabricengine ~33s, fabriccli ~2.3-2.7s, gitrepo ~1.4-1.7s).

**Sabotage proofs — every one run by the orchestrator, watched to fail at the intended assertion, then restored to an empty diff:**

| # | Mechanism sabotaged | Test | Result |
|---|---|---|---|
| S1 | `sideConcludeAlreadyLanded` reverted to the loose "≥2 parents, source anywhere" check (F1) | `TestMergeContinue_OctopusMergeCarryingTheSource_IsNeverAdopted` | failed at `want no KindMergeCommitted — an octopus is not a conclude fabric can make` |
| S2 | `weftPathVisible`'s `filepath.ToSlash` replaced with naive `strings.ReplaceAll(anchorRel, "\\", "/")` (F6) | `TestMergePaths_UnifyConflictPaths/single_anchor_segment_containing_a_backslash_is_not_split` | failed at `unified = []; want [weird\name/_lyx/foo.md]` — confirms `ToSlash` (not a blanket replace) is genuinely load-bearing for the Linux legitimate-backslash-in-name case |

First sabotage attempt on S2 (deleting the `filepath` import along with the call) was a build break — redone by keeping the import alive with a no-op reference, per the method's "a build break is not a proof" rule.

**F1 re-driven live by the orchestrator**, freshly deployed binary, fresh isolated hub (`hub10` — `hub9`'s first attempt produced a *different* octopus shape where git's redundant-ancestor elimination silently dropped the recorded start as a parent, because the "unrelated" branch was built as a descendant of the start commit instead of a properly divergent third line; rebuilt with the unrelated branch off the pair's own root commit to get the exact 3-parent, start-first shape the round's own reproduction shows):

- Built the exact scenario: warp side stages cleanly (`warp_outcome: staged`), weft conflicts, record survives with `warp_committed: ""`. Discarded the staged warp merge (`git merge --abort`), then `git merge --no-edit <recorded-source> <unrelated-branch>` → genuine octopus, parents `[start, source, unrelated]` confirmed via `git log --format=%P`.
- Resolved the weft-side conflict (masks nothing — `MergeContinue`'s conflict-precondition guard would otherwise refuse before ever reaching the warp-side adoption logic) and re-ran `merge --continue` on the fixed binary → `merge conclude did not finish; run MergeContinue again`, record retained, `warp_committed` still empty. The octopus commit is NOT adopted; fabric does not claim it as its own conclude.

**F6 verified by inspection + sabotage only** — no Windows host exists in this campaign (stated plainly, same as every prior round). The fix is provably a no-op on Linux by construction (`filepath.ToSlash` is identity when the OS separator is already `/`), which the round's own reasoning states and which cannot itself be falsified on this host; the sabotage above confirms the *correct implementation choice* (ToSlash vs. a naive replace) is load-bearing, which is the strongest verification available without a Windows machine.

### RESIDUAL — what verification left standing

**None found.** Both behavioral fixes (F1 live, F6 by the strongest verification available) and all seven other findings hold up.

### Honest limits of this verification

- Windows path behaviour remains unexecuted on real Windows — three rounds now know the exact defect shape (F6), but none has run it. This is the sharpest named gap in the whole campaign at this point: a traced, fixed, but never-executed defect.
- F6 was not live-redriven (cannot be, headlessly, on this host) — verified by inspection + the ToSlash-vs-ReplaceAll sabotage instead, which the orchestrator judges the strongest verification available, not equivalent to a live drive.
- Scratch hubs (`hub9`, `hub10`) live under the session scratchpad, outside the repo. `git status` in the worktree is clean throughout.

## Round 7 (`opus-medium-r7`) — NOT YET SEEDED, awaiting operator decision

This is the last round in the operator's original fixed 4-round plan (Opus medium → Fable high → Opus high → **Opus medium**). Three consecutive rounds (r4, r5, r6) have each found and fixed real material — r6's own self-assessment explicitly declines to read its clean gates as convergence evidence, and the orchestrator agrees. The operator raised this trend directly mid-campaign ("Den finner jo masse").

Two live options, not yet decided:
1. **Run r7 as originally planned, then stop regardless of outcome** (the plan was "4 rounds unless convergence" — convergence has not been reached, but the plan was also bounded at 4, not open-ended).
2. **Run r7, and if it also finds real material, extend beyond 4 rather than stopping on a hard count** — since the trend (BLOCKING → MEDIUM-heavy → MEDIUM-heavy-with-2-new-behavioral-defects) does not show the severity/novelty curve flattening the way the reed and first-instalment fabric campaigns did before they converged.

## Next action

**Ask the operator which of the two options above before spawning round 7** — do not spawn reflexively. If they confirm the plan as originally stated (run r7, stop after regardless), seed and spawn `opus-medium-r7` per the standard loop. If they want the extend-on-signal option, say so explicitly in the seed prompt's round-context section so a genuinely fresh round 8 (if needed) has the right frame.
