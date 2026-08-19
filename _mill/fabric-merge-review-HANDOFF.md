# fabric merge — crucible campaign HANDOFF (orchestrator-only)

Off-limits to round agents: this file matches the `fabric-merge-review-*` pattern the round prompt declares unreadable.

**Last refreshed:** after round 1 stalled out mid-F1. Tree state as of commit `a20dc2ca` plus an uncommitted F1 diff.

## What this campaign is

Mill task `fabric-merge-crucible-hardening` (wiki id 85), worktree `/home/knatte/Code/loomyard/wts/fabric-merge-crucible-hardening`, branch of the same name.
Crucible (`crucible/README.md`) run by hand against **only** the merge primitive shipped by `a2bf44e2` — `MergeIn` / `Merge` / `MergeContinue` / `MergeAbort` / `MergeInProgress` on `internal/fabricengine`, the `internal/gitrepo/merge.go` layer under it, and the `lyx fabric merge-in` / `lyx fabric merge` CLI surface.
Explicitly **not** the rest of fabricengine — the `crucible: follow-ups` slices 12–15 already hardened that.

Orchestrator-driven, not mill-go. The task's mill phase stays `discussing`; nothing in this campaign advances it.

## Operator's round plan (given up front, 2026-08-19)

Up to four rounds in the first instalment, model + effort fixed in advance:

| Round | Model | Effort | Tag | Status |
|---:|---|---|---|---|
| r1 | Opus | medium | `opus-medium-r1` | **running** — Job 1 done and committed, Job 2 in progress |
| r2 | Opus | medium | `opus-medium-r2` | not started |
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

## Round 1 (`opus-medium-r1`) — live, Job 2 in progress

Seed commit `74ef8089` (prompt + pre-count + handoff).

**Job 1 is complete and committed.** Review report at `_mill/fabric-merge-review-opus-medium-r1.md` (~32 KB), built incrementally across five `review notes` commits, `125757f4` → `823bd1d6`, ending with "Job 1 complete". The Sequencing rule was respected — the report was committed before the first production edit.

Twelve live scenarios driven (L1–L12): fast-forward merge, merge-in conflict → sibling refusals → `--continue`, crash between the warp and weft `MergeStart`, unmappable weft conflict path, squash, foreign git merge state, merge onto a detached warp HEAD, source-ref resolution edges, hostile git config, concurrency (two interleaved hubs plus a sequential control), sibling verbs against a genuinely mid-merge pair, and CLI arity/flags/exit codes.
It also produced the enumerated mutating-entry-point table the prompt asked for.

**Nine findings, all self-marked CONFIRMED:**

| ID | Sev | Claim |
|---|---|---|
| F1 | BLOCKING | `MergeContinue` lands half a merge, then dead-ends forever, when the recorded attempt never reached one side |
| F2 | BLOCKING | no merge verb requires the checkouts to be on a branch |
| F3 | MEDIUM | `MergeResult.Committed` / `AlreadyUpToDate` do not describe what happened |
| F4 | MEDIUM | an operator's `merge.ff = only` breaks every non-fast-forward fabric merge |
| F5 | LOW | `Remove` can delete the pair that is a live merge's source |
| F6 | LOW | `doc.go` overstates the guarded sibling set |
| F7 | NIT | `MergeContinue`/`MergeAbort` break the "`Conflicts` is empty, never nil" contract |
| F8 | NIT | `merge --abort -m <msg>` silently accepts and ignores `-m` |
| F9 | NIT | docs — a conflicted merge is not distinguishable from a hard error by exit status |

Severities and CONFIRMED labels above are **the round's own claims, not verified**. Two of them (L3 crash-between-MergeStarts, L7 detached HEAD) are flagged in the report as CONFIRMED DEFECT from live driving; the rest still need the orchestrator's own reproduction.

**Fixes landed so far:** `a20dc2ca` — F2 (refuse a merge while either checkout has a detached HEAD). The round created a new `internal/fabricengine/mergecrucible_integration_test.go` for its regression tests.

### The stall — round 1 died mid-F1 (harness failure, NOT an operator stop)

The round agent failed with `Agent stalled: no progress for 600s (stream watchdog did not recover)`.
This is a different animal from the operator's deliberate pause/restart earlier in the same round: that one is Hard Rule 6 territory and must never be "recovered" from, this one is a genuine harness death and is the orchestrator's to handle.
Tell them apart by the notification status — `killed` / `stopped by user` is the operator, `failed` with a watchdog reason is not.

**Do NOT stash, revert, reset or "tidy" the uncommitted F1 diff.** It is the round's work and it is nearly complete.

Uncommitted working tree at the moment of the stall (F1 in flight), and the commit-per-fix discipline is exactly what makes this readable:

| File | Change |
|---|---|
| `internal/fabricengine/mergelifecycle.go` | +31 — new `mergeAttemptIncompleteReason(st)`, wired into `MergeContinue` and aggregated with the unresolved-conflicts reason; doc comments on `concludeMergeSides` and `MergeContinue` updated |
| `internal/fabricengine/mergeerrors.go` | +1 — new closed-set member `mergeReasonAttemptIncomplete = "merge attempt did not reach both sides"` |
| `internal/fabricengine/mergevocab_test.go` | +3 — the pinned closed-set assertion updated in step, as that test's own failure message demands |
| `internal/fabricengine/mergecrucible_integration_test.go` | +61 — F1's regression test |
| `internal/fabricengine/doc.go` | +9 — module-doc update for the new refusal |

Read on its own terms the diff is coherent and complete — production change, closed-vocabulary update, regression test, and the same-commit doc update the Documentation Lifecycle requires. What is missing is the commit itself, and any evidence that the gates were run against it. **Nothing here has been verified by the orchestrator, and the F1 test has not been watched to fail.** Treat "looks complete" as a reading of the diff, not as a green light.

**Findings NOT yet started when it died:** F3, F4, F5, F6, F7, F8, F9 — seven of the nine. No fixer report exists yet (`_mill/fabric-merge-review-opus-medium-r1-fixer-report.md` was never created).

**CLOSED-AND-VERIFIED:** nothing yet — no orchestrator verification has run.

**RESIDUAL currently seeded in `_mill/fabric-merge-review-prompt.md`:** none; the prompt still carries the round-1 "first round, no prior residual" seed.

**DEFERRED list:** empty.

## Next action

**Round 1 is dead and did not finish. The immediate decision is how to close it out — that decision is the operator's and has been put to them.** The options, cheapest first:

1. **Resume the same agent** (its task id is still resumable via a message). It holds the whole review context, so F1's commit and the seven untouched findings are cheap for it. Risk: it stalled once already.
2. **Spawn a narrow, targeted fixer** — explicitly sanctioned by `orchestrator-prompt.md`'s decision step 5 for exactly this shape: brief it to read the existing review report plus the current diff and log, then finish and commit whatever is left. NOT a fresh full review round (Hard Rule 4).
3. **Close r1 where it stands**, commit nothing further of its work, and roll the seven open findings into r2's seed as the residual.

Do not pick one of these unilaterally after the operator has answered otherwise.

Once round 1 is closed out one way or the other, verify in this order:
1. Read r1's "What was tested" section in full before characterising its work at all (fabric-campaign rule 8).
3. Run the gates from cold on the committed tree: `build`, `vet`, hermetic `-count=5`, and full `-tags integration`. Name the tag in the record.
4. For every new test r1 added, reproduce the not-false-green proof independently: revert the production hunk, watch the test fail at the intended assertion, restore, confirm an empty diff. Read the neighbouring code while preparing each sabotage — that habit is what found a BLOCKING bug three rounds had missed in the last campaign.
5. Re-drive both BLOCKING fixes (F1, F2) live, in their strongest mode, on a fresh hub.
6. Check r1's enumeration total against pre-count classes 3 and 6; expect and welcome a correction of the orchestrator's numbers.
7. Re-seed the prompt with whatever verification leaves standing — derived from the residue, never "review it again" — and spawn r2 as Opus / medium, tag `opus-medium-r2`.
