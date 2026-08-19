# fabric merge — crucible campaign HANDOFF (orchestrator-only)

Off-limits to round agents: this file matches the `fabric-merge-review-*` pattern the round prompt declares unreadable.

**Last refreshed:** after the orchestrator's independent verification of round 1, at commit `a6a88502`.

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
| r2 | Opus | medium | `opus-medium-r2` | **next** — prompt re-seeded with the 3 residuals |
| r3 | Fable | medium | `fable-medium-r3` | not started |
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

## Next action

Spawn round 2: `Agent` → `subagent_type: crucible-reviewer-medium`, `model: opus`, tag `opus-medium-r2`, prompt = read `_mill/fabric-merge-review-prompt.md` and do exactly what it says.
The prompt is already re-seeded with the three residuals above, the CLOSED-AND-VERIFIED list, and the two deferred items.
Then stay off the tree and off `git add`/`git commit`, keep refreshing this file on disk, and verify independently again.
