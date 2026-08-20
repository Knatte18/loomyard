# fabric merge — crucible campaign HANDOFF (orchestrator-only)

Off-limits to round agents: this file matches the `fabric-merge-review-*` pattern the round prompt declares unreadable.

**Last refreshed:** during round 2's Job 1, at commit `7e251055`. Written to disk, NOT committed — a round is live, so Hard Rule 3 keeps the orchestrator off `git add`/`git commit`. Commit this refresh once r2 finishes.

## What this campaign is

Mill task `fabric-merge-crucible-hardening` (wiki id 85), worktree `/home/knatte/Code/loomyard/wts/fabric-merge-crucible-hardening`, branch of the same name.
Crucible (`crucible/README.md`) run by hand against **only** the merge primitive shipped by `a2bf44e2` — `MergeIn` / `Merge` / `MergeContinue` / `MergeAbort` / `MergeInProgress` on `internal/fabricengine`, the `internal/gitrepo/merge.go` layer under it, and the `lyx fabric merge-in` / `lyx fabric merge` CLI surface.
Explicitly **not** the rest of fabricengine — the `crucible: follow-ups` slices 12–15 already hardened that.

Orchestrator-driven, not mill-go. The task's mill phase stays `discussing`; nothing in this campaign advances it.

## Operator's round plan (given up front, 2026-08-19)

Up to four rounds in the first instalment, model + effort fixed in advance:

| Round | Model | Effort | Tag | Status |
|---:|---|---|---|---|
| r1 | Opus | medium | `opus-medium-r1` | **done, verified** — 9 findings, 9 fixed; verification left 3 residuals |
| r2 | Opus | medium | `opus-medium-r2` | **done, verified** — 2 BLOCKING + 1 LOW + 1 NIT, all fixed, residual 1 closed; verification left 3 new residuals |
| r3 | Fable | medium | `fable-medium-r3` | **running** — residuals A–C |
| r4 | Opus | high | `opus-high-r4` | not started |

Hard Rule 2 (explicit effort pick required before every spawn) is satisfied for all four by that instruction.
Do not deviate from this list without the operator saying so.

## Operator norms observed so far

- **The operator pauses and resumes rounds on purpose.** Round 1 was stopped mid-Job-2 and restarted by hand, with an explicit "don't you worry about it". Hard Rule 6 applies: a `killed`/`stopped by user` notification is never a crash, never something to recover from, never something to report back as a problem. Note the state and keep waiting. The same round may notify several more times before it finishes for real.
- **Keep this handoff current at all times, not just after a verification.** The operator called this out directly. Refreshing the *file on disk* mid-round is required; only the `git add`/`git commit` of it waits for a clean tree. A refresh that only exists in orchestrator working context is exactly what a compaction destroys — which is the failure this file exists to prevent.

## Campaign-specific facts worth not rediscovering

- **fabric's live tier is `-tags integration`, not `-tags smoke`.** There are zero `//go:build smoke` files under `internal/fabricengine`, `internal/fabriccli`, `internal/gitrepo`. Any round claiming a "smoke" run for fabric has run nothing — reject that claim.
- **Not LLM-driving.** The substrate is real git repos. The generic protocol's N-concurrent amplifier is safe here and there is no EXECUTION BAN list. The real cost is wall-clock (~30 s per full `-tags integration` fabricengine run), not RAM.
- **No `manifest/designs/fabric.md`.** fabric's module doc is `internal/fabricengine/doc.go`; its "# The merge surface" section (~line 846) is the authoritative prose contract. Documentation Lifecycle updates land there.
- **SPEC lives only in git history.** `git show 3b800bc8:_mill/discussion.md` and `3b800bc8:_mill/plan/0*.md` (six batches); rejected alternatives at `967916ea:_mill/discussion-meta.md`. The working tree's `_mill/` was cleaned on merge.
- **`./deploy-dev`** is the POSIX deploy script (not `.cmd`) on this host.

## Baseline the orchestrator established before round 1 (all green, committed tree at `9115020a`)

- `go build ./...`
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...`
- `go test -count=1 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...`
- `go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — fabricengine 25.2 s, fabriccli 2.1 s, gitrepo 1.3 s

Green baseline is the starting condition, not evidence. The whole point of this campaign is that these gates pass and the surface is still not trusted under crash and concurrency.

## Pre-count

`_mill/fabric-merge-review-PRECOUNT.md`, written before round 1 was spawned, never shown to a round.
Six counted classes, each with its blind spots named. Expect to be corrected by a round; that is the round working.
**Not yet checked against r1's numbers** — do that during verification, and read r1's enumeration table (report line ~297) against pre-count classes 3 and 6 in particular.

## Round 1 (`opus-medium-r1`) — COMPLETE, and independently verified

Seed commit `74ef8089`. Round commits `125757f4` → `a6a88502`.
It survived one deliberate operator pause/restart and one genuine watchdog stall (recovered by resuming the same agent from its transcript — see the stall note above).

Job 1: twelve live scenarios (L1–L12) on ~20 purpose-built scratch hubs, the mutating-entry-point enumeration, and the plan-vs-shipped scope pass. Report committed incrementally, before the first production edit — the Sequencing rule held.
Job 2: nine findings, nine fixed, nothing deferred. It also repaired four pre-existing integration tests that were pinning F3's defect as correct.

Self-verdict was READY TO MERGE. That verdict is not the gate; what follows is.

### What the orchestrator verified itself

**Gates, from cold on the committed tree — all green, tags named:**
`go build ./...`; `go vet` on the three packages; `go test -count=5` hermetic across `fabricengine`/`fabriccli`/`gitrepo`/`cmd/lyx`; `go test -tags integration -count=1` across the three packages (fabricengine 44.0 s, fabriccli 4.3 s, gitrepo 2.3 s).

**Sabotage proofs — every new test watched to fail at its intended assertion, then the code restored to an empty diff.** Eight new tests, seven mechanisms:

| Mechanism sabotaged | Test | Result |
|---|---|---|
| F1 `mergeAttemptIncompleteReason` → never fires | `TestMergeCrucible_ContinueRefusesAttemptThatNeverReachedBothSides` | failed correctly |
| F2 `detachedHeadReason` → returns nil | `TestMergeCrucible_DetachedHeadRefused` (both subtests) | failed correctly |
| F3 `landedConcludeCommit` → `true` | `TestMergeCrucible_ResultFlagsDescribeWhatHappened` | failed correctly |
| F3 `bothSidesAlreadyUpToDate` → `false` | — | **STAYED GREEN — see residual 1** |
| F4 `--ff` pin removed | `TestMergeStart_HostileMergeFFConfig` (both subtests) | failed correctly |
| F5 `mergeSourceInFlight` → `false` | `TestMergeCrucible_RemoveRefusesAPairSomeOtherMergeIsConsuming` | failed correctly |
| F7 `Conflicts: mergeNoConflicts` dropped | `TestMergeCrucible_ConflictsIsEmptyNeverNil` | failed correctly |
| F8 `-m`+`--abort` rejection disabled | `TestRunCLI_MergeRejectsFlagsItWouldOtherwiseIgnore` | failed correctly |
| F8 rejection broadened to `--continue` | `TestRunCLI_MergeContinueAcceptsMessage` | failed correctly |

The first sabotage attempt on F2 (`if warpDetached || weftDetached` → `if false`) broke the build on unused variables and was inconclusive; it was redone by neutering the return instead. A build break is not a proof — redo the sabotage rather than counting it.

**Both BLOCKING fixes re-driven live**, on a purpose-built hub, against the freshly deployed binary:
- F2: warp detached, weft detached, and `merge --squash` as the strongest mode — all three refused with `checkout is not on a branch`, empty mutation record, no record written, neither branch moved.
- F1: the crash window reconstructed by hand (`fabric-merge.json` with `warp_outcome:"staged"`, `weft_outcome:""`). `--continue` refused with `merge attempt did not reach both sides`, zero mutations, nothing landed; `--abort` then restored both sides to the recorded pre-merge SHAs, deleted the record, and left a clean worktree. The reconstruction turned out stricter than intended — git had actually fast-forwarded where the record claimed `staged`, and the guard still fired on the empty weft outcome.

**Pre-count reconciliation:** r1's enumeration reported 27 mutating entry points, 4 guarded, 1 genuine gap. Pre-count class 6 had the spec's 4 guarded verbs and predicted that a round enumerating every entry point would exceed 4 — it did, and F5 is that excess. The round moved in the correct direction and the orchestrator's number needed no correction. Classes 1, 2, 4 and 5 were not contested.

### RESIDUAL — three things verification left standing, now seeded for r2

1. **`bothSidesAlreadyUpToDate()` has zero test coverage.** Hardwired to `return false`, the entire suite — hermetic and `-tags integration`, both packages — stays green. And the subtest that looks like it covers the flag (`…/SecondCallReportsAlreadyUpToDateNotCommitted`) is caught by the **pre-lock** probe at `merge.go:122`, a different return site with a hardcoded `true`. Textbook refinement 5: a green run that never entered the code path. The fix itself reads correct; it is the proof that is missing.
2. **The post-record error-return class was never enumerated.** r1 brushed against it repeatedly (F1 lives in it) but never counted it. Pre-count class 3 has the orchestrator's own hand-read: 27 sites, of which 12 leave the record live *after* a conclude-commit already landed — where `MergeAbort` restores from pre-merge SHAs and would discard committed work. That class is countable and unclosed, which is exactly what refinement 2 says to switch a round to.
3. **`mergeReasonNoMergeInProgress` is dangling** — declared in the closed reason set at `mergeerrors.go:24`, referenced only by the vocabulary tests that pin the set, produced by no code path. This was noted in pre-count class 4 and deliberately withheld; r1 did not find it.

### DEFERRED — carried into r2's prompt for re-evaluation

- The unlocked guard window between `mergeRecordExists()` and `saveMergeState`. r1 reasoned about it, could not reproduce it, and correctly declined to record it.
- `CheckoutDetached`/`RestoreBranch` abandoning a merge already in progress. r1 logged it as webster's problem on the argument that F2 closes the harmful direction. r2 is asked to re-examine that argument, since F2 stops a merge *starting* while detached but not a detach *during* a merge.

### Honest limits of this verification

- The two verification hubs under the session scratchpad could not be deleted — `rm -rf` is refused by this session's sandbox. They are outside the repo, in an ephemeral session directory, and `git status` is clean. Not cleaned, and said so rather than worked around.
- One stray `lyx reed header --blocking` process is running, and it belongs to the **`reed-shuttle-crucible-hardening`** worktree, not this one. Left alone under worktree isolation. Do not reap it.
- Windows path behaviour in `weftPathVisible`/`unifyConflictPaths` remains unexecuted, by both the round and the orchestrator. Out of scope, Linux host, reasoned about rather than driven — the same named gap the previous fabric campaign carried to the end.
- The N-way concurrent amplifier was not run. The merge bar is single-instance correctness and the surface is not tmux-shaped; this is a deliberate omission, not an oversight.

## Round 2 (`opus-medium-r2`) — COMPLETE, and independently verified

Re-seed commit `a5700c41`. Round commits `aa7fcd49` → `39208500` (9 commits). Sequencing rule held: the whole review was committed before the first production edit.
Reports: `_mill/fabric-merge-review-opus-medium-r2.md` (394 lines), `-fixer-report.md` (187 lines).
Self-verdict READY. That verdict is not the gate; what follows is.

Findings: BLOCKING 2 (R1, R2) · LOW 1 (R5) · NIT 1 (R3) · withdrawn-on-evidence 1 (R4). All fixed, nothing deferred. Plus residual 1 closed with a new test.

### What the orchestrator verified itself

**Gates, from cold on the committed tree — all green, tiers named:**
`go build ./...`; `go vet` on the three packages; `go test -count=5` hermetic across `fabricengine`/`fabriccli`/`gitrepo`/`cmd/lyx`; `go test -tags integration -count=1 -timeout 40m` (fabricengine 43.6 s, fabriccli 4.5 s, gitrepo 2.5 s).

**Sabotage proofs — every one run by the orchestrator, watched to fail at the intended assertion, then restored to an empty diff:**

| # | Mechanism sabotaged | Test | Result |
|---|---|---|---|
| S1 | `bothSidesAlreadyUpToDate` → `return false` | `TestMergeCrucible_DerivedAlreadyUpToDateIsReadFromTheRecord` | failed at `AlreadyUpToDate = false; want true`, and it was the **ONLY** failure in the whole integration fabricengine tier — residual 1's proof gap inverted and closed |
| S2 | R1's `mergeHeadPresent` probe neutered | `TestMergeStart_EmptyResultTree_…` + `TestMergeCrucible_EmptyResultMergeIsConcludedNotAbandoned` | gitrepo test failed at `outcome = 3; want 0`; pair-level test failed at seven assertions covering every documented harm, including the `ErrForeignMergeState` bricking |
| S3 | R2's abort guard disabled (`if false && …`) | `TestMergeCrucible_AbortRefusesAnAttemptWhoseConcludeLanded` | both arms failed at `error = <nil>; want *MergeGuardError` |
| S3b | guard returns the correct error but the reset still runs | same test | failed at the **destruction** assertions — zero-mutations and `warp HEAD … want the landed conclude-commit still in place`. Proves those assertions are load-bearing, not decorative; S3 alone could not show this because `assertSoleGuardReason` t.Fatals first |
| S4 | R2's predicate narrowed to the recorded SHA alone | same test | **only** the `InvisibleConcludeTheRecordNeverLearnedAbout` arm failed — the second clause is load-bearing and the two arms are not testing the same thing |
| S5 | a tenth constant added to the closed guard-reason set | `TestMergeVocabulary_GuardReasonSetIsClosedAndSideFree` | **STAYED GREEN — see residual A.** The closure test cannot detect an added member |

**Both BLOCKING fixes re-driven live** on freshly built hubs against the freshly deployed binary (`a057bc35`):

- **R1** (`vhub1`): `merge-in` of an empty-result non-ancestor merge → `already_up_to_date:false, committed:true`, four honest mutations, **no `MERGE_HEAD` on either side**, both HEADs advanced onto two-parent merge commits, record deleted. The bricking claim re-driven specifically: `--abort` and `--continue` now both report `no merge in progress`, not `ErrForeignMergeState`.
- **R2** (`vhub3`): warp conflict resolved by hand, refusing `pre-commit` hook in the weft → `merge --continue` lands the warp conclude and returns `ErrMergeIncomplete` with the record retained; `merge --abort` then refuses with `merge preconditions failed: merge conclude already landed`, **zero mutations**, conclude commit intact, record retained, the operator's resolution still in the tree; hook removed → `merge --continue` skips warp, commits only weft, deletes the record, both sides carrying their merge commit.
  My first R2 fixture (`vhub2`) was wrong and proved nothing: the weft merge fast-forwarded, so the conclude skipped that side and the hook never ran. Fixed by diverging the weft trunk too. Recorded because a fixture that silently degrades is exactly the failure this campaign is about.

**Enumeration reconciliation.** The orchestrator re-ran round 2's awk sweep over the same eight functions and reproduced its raw total **exactly: 94**. The per-site adjudication (41 in-class, 17 destructive) was NOT re-derived by hand — see residual C. It matters less than it looks: R2's guard reads the record and the two HEADs at the top of `MergeAbort`, so it is count-independent.

### RESIDUAL — what verification left standing, now seeded for r3

1. **The closed-set "closure" test does not verify closure.** `TestMergeVocabulary_GuardReasonSetIsClosedAndSideFree` compares two lists both hand-maintained inside the test; nothing reads the const block. Adding a tenth constant to the set leaves the whole hermetic tier green (S5). Its doc comment claims the opposite. Found by sabotaging a *test* rather than a fix — the shape to look for from here on.
2. **Four states where `MergeContinue` is stuck** (round 2's rows 27/28/30/31): the conclude lands but `CurrentSHA`/`saveMergeState` fails, so the record never learns. R2's guard stops the abort from destroying them; round 2 deliberately declined to make the conclude idempotent, leaving plain git as the only recovery. Defensible, accepted, not closed.
3. **Round 2's 45-row per-site adjudication is unchecked.** Method reproduced, judgement not.

### Honest limits of this verification

- Windows path behaviour in `weftPathVisible`/`unifyConflictPaths` remains unexecuted by every round and by the orchestrator. Linux host, out of scope, reasoned about rather than driven.
- The N-way concurrent amplifier was not run. Deliberate: the merge bar is single-instance correctness.
- The verification hubs under the session scratchpad (`vhub1`, `vhub2`, `vhub3`) could not be deleted — `rm -rf` is refused by this session's sandbox. They are outside the repo in an ephemeral session directory; `git status` in the worktree is clean. Not cleaned, and said so rather than worked around.
- One stray `lyx reed header --blocking` process belongs to the **`reed-shuttle-crucible-hardening`** worktree. Left alone under worktree isolation. Do not reap it.

## Round 3 (`fable-medium-r3`) — RUNNING

Re-seed commit `ab8dd9f9` (prompt rewritten for residuals A–C + handoff carrying r2's full verification record).
Spawned as `crucible-reviewer-medium` on **Fable / medium**, per the operator's fixed rotation. No round commits yet at the time of writing.

Identify this round by its round tag and by `git log`, never by an internal agent ID — those are ephemeral and mean nothing in a fresh session. If you are a new orchestrator and need to reach the running agent, use `ListAgents`; if it is gone, read `_mill/fabric-merge-review-fable-medium-r3.md` (committed incrementally) to see how far it got and pick up from there rather than restarting the round.

Its assignment is residuals A–C, which are stated in full in the "RESIDUAL" section above and in the round prompt. Residual A is the primary and is the interesting one: it was found by sabotaging a *test* rather than a fix.

**Model-rotation note.** This is the first non-Opus round of the campaign. Rotation is the point — a different model fails differently — but calibrate the verification accordingly and do not soften any step of the protocol for it. In particular, re-derive its sabotage proofs yourself rather than reading its table; that has now caught something in both prior rounds.

## Next action

**While r3 runs:** stay off the tree and off `git add`/`git commit` entirely. Keep refreshing this file on disk. Do not read the round's raw transcript. A `killed`/`stopped by user` notification is the operator being deliberate — note it and keep waiting. A `failed` notification with a watchdog/stall reason is a genuine harness death: resume the same agent from its transcript, and if a round stalls a **second** time switch to a narrow targeted fixer rather than resuming a third time.

**When r3 finishes,** run the same protocol that has now caught something in both rounds:

1. Commit the handoff refresh once the tree is the orchestrator's again.
2. Read r3's "What was tested" in full before characterising its work at all.
3. Gates from cold, tiers named in your own record. Never accept a green claim that does not name its tag.
4. **Sabotage-prove every new test yourself**, including the companion direction. Sabotage the *tests* too, not only the fixes — that is what found residual A. A build break is not a proof; redo it a different way.
5. Re-drive every BLOCKING fix live on a freshly built hub against a freshly deployed binary (`./deploy-dev`). The recipe, written out rather than pointed at — the session scratchpad is shared and round agents overwrite files there, so a path reference rots:
   - Export `GIT_CONFIG_GLOBAL` (a file carrying `[init] defaultBranch = main` plus a `user.name`/`user.email`) **before the first `git init`**. Get this wrong and the weft bare defaults to `master`, so the pair is `main`/`master-weft` and every fabric verb fails on an invalid `main-weft` reference.
   - `git init --bare` a warp and a weft; `git init` a seed, commit into it, push it to the warp bare as `main`.
   - From an empty work dir: `lyx fabric clone <weft-bare> <warp-bare>`. That yields `<work>/<warpname>-HUB/` containing the prime pair `warp-bare` + `warp-bare-weft`.
   - The prime pair is enough for every merge scenario — do not bother with `lyx fabric add`, which needs cwd inside a repo and adds nothing here.
   - Sandbox constraints: inline `cd <dir> && …` is refused, so put the `cd` inside a script file or use `env -C`; `rm -rf` is refused, so scratch hubs cannot be cleaned up and that must be stated honestly rather than worked around.
   **Check every fixture for silent degradation** before trusting a scenario — assert the precondition the scenario depends on, the way the good tests do with `t.Fatal`.
6. Re-seed from whatever verification leaves standing — derived from the residue, never "review it again" — and spawn r4 as **Opus / high**, tag `opus-high-r4`.

**Convergence bar:** a safety pass that finds nothing, the orchestrator's own gates, and the sabotage proofs all agreeing, across rotated models. State the limits in the verdict rather than claiming more: Windows path behaviour, the N-way amplifier, and any class whose zero came from a method never shown able to detect.
