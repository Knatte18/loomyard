# loom-step + self-report crucible campaign — orchestrator handoff

## Current state
Round 3 (`sonnet-xhigh-r3`, the intended safety pass) complete and independently verified by the orchestrator. **It was NOT clean** — it found and fixed one real BLOCKING bug in territory neither prior round had ever driven to completion (`Publish`'s merged-PR detection). This confirms the method's own precedent (reed/fabric): the round right before the genuinely clean one is never actually clean. A round 4, framed as the real safety pass, is recommended next — not yet spawned, awaiting the operator's model + effort pick. All three rotation models (Opus r1, Fable r2, Sonnet r3) have now been used once; round 4 repeats one, operator's choice (the method suggests the most capable for a final safety pass).

## CLOSED-AND-VERIFIED

### Round 1 (commit range `4e0b3265c..4c875f690`) — see prior handoff version in git history for full detail
F-0 (BLOCKING), F-1/F-3/F-4 (MEDIUM), F-6 (LOW) personally sabotage-proved by the orchestrator; F-2/F-5/F-7/D-1/D-2 reviewed by diff. Live-fire confirmed real (#240, #241).

### Round 2 (commit range `9998aeb9d..0efef971d`)
Orchestrator independently reproduced, from a cold checkout, on the committed tree:
- `go build ./...`, `go vet` (ten packages), `go test -count=5` (ten packages + `./cmd/lyx/...`), `go test ./...` (whole repo) — all green.
- `go test -tags smoke ./internal/loomcli/... -run Smoke -v -count=1` — all green, zero real LLM subprocesses.
- `gh issue list` confirms the newest `Knatte18/loomyard` issues are still #240/#241 — **no third issue was filed this round.**

**Sabotage-proved personally:**
- **R2-F1 (MEDIUM)** `aaddede3e` — two independent sabotages, both failed exactly as claimed: (a) removed `entry.CleanStepHandoff` from `DetectCrashResume`'s exclusion — `TestDetectCrashResume/RunningWithHistory_CleanStepHandoff_None` failed `ok = true; want false`; (b) commented out the `recordStepHandoff` call in `step.go` — `TestSmokeStep_RecordsCleanHandoffMarkerMatchingPersistedStatus` failed "no clean-handoff marker … after a completed step".

**Reviewed by diff, not independently sabotage-proved** (low-risk: doc/skill text or a single log line): R2-F2 (skill+doc text), R2-F3 (one Info log line), R2-F4 (one log field name).

**Residual items round 1 seeded — all dispositioned by round 2, independently spot-checked against its review report's live-repro transcripts (procedure, exact commands, exact observed output all present and internally consistent):**
1. F-4 generalizes to a second `Bouncer` instance — **CLOSED**, reproduced live on `Plan-Bouncer` (kill-mid-seed, re-invoke, attach-not-abandon observed).
2. F-0's `halted` predicate pressed into its narrowest window (SIGSTOP before first persist) — **CLOSED as fine-by-design**; the one gap it surfaced (no log breadcrumb) became R2-F3.
3. `Publish`/`Finalize` — Publish driven live to a REAL pull request (`Knatte18/lyx-test#2`) and its designed awaiting-review halt; the merged-PR/Finalize leg blocked on a harness permission gap (agent could not merge the PR), left to the operator, hermetically covered in `internal/landingshed`. `RunDone`-triggered reflection driven live on a dedicated fixture — fired, judged, declined to file, archived correctly.
4. Spontaneous Tier-2 friction note — attempted honestly (a genuine stale-brief rough edge in the fixture's task board), the writer noticed it but judged it not worth a note. Not a defect; still unobserved.
5. F-6's live two-driver race — still accepted residual, cost-forbidden to reproduce.

**Docs updated in the same commits, confirmed via `git diff --stat`:** `manifest/designs/self-report-tier1.md`, `loom-step.md`, `plugins/ly/skills/ly-supervise/SKILL.md`. `docs/overview.md`/`CONSTRAINTS.md`/`manifest/roadmap.md` correctly untouched.

### Round 3 (commit range `06e4c8d1e..c35497f87`)
Orchestrator independently reproduced, from a cold checkout, on the committed tree:
- `go build ./...`, `go vet ./...`, `go test ./...` (whole repo) — all green.
- Independently confirmed the core factual claim via a live `gh api` call of my own (not just trusting the round's transcript): `gh api "repos/Knatte18/lyx-crucible-r3/pulls?state=all" --jq '...'` on the round's own real merged PR returned `{"merged":null,"merged_at":"2026-09-13T16:49:32Z",...}` — proving GitHub's List Pull Requests endpoint genuinely never populates `merged`, exactly as claimed.
- **Sabotage-proved F-R3-1 (BLOCKING) personally**: reverted `!pr.GetMergedAt().IsZero()` back to `pr.GetMerged()` in `internal/landingshed/publish.go`; `TestPublish_ClosedAndMergedPR_Done` failed `Call() outcome = "stuck"; want "done"`, exactly as claimed. Restored; `git diff --stat` empty; full `internal/landingshed` suite green after.
- `gh issue list` confirms the newest `Knatte18/loomyard` issues are still #240/#241 — **no third issue was filed.**
- Confirmed both disposable GitHub repos (`Knatte18/lyx-crucible-r3`, `-weft`) are archived (not deleted — same missing `delete_repo` OAuth scope round 2 also hit) and clearly labeled "safe to delete" in their descriptions. Low-urgency operator cleanup, unlike round 2's leftovers (see below) — these were never live/interactive infrastructure the operator uses.

**Findings this round:** 1 BLOCKING (F-R3-1), 0 MEDIUM/LOW/NIT — genuinely the first round to find nothing beyond the one new-territory defect, which is itself evidence the general envelope/anomaly/friction machinery is solid; the defect sat specifically in the one path (`Publish`'s merged-PR resume) neither round 1 nor round 2 ever drove to completion.

**Residual items closed this round, independently spot-checked against the review report's live-repro transcripts:**
1. `Publish`'s merged-PR resume — **root-caused, fixed, and re-verified live** against the exact real merged PR that exposed it. This was the single biggest remaining gap across the whole campaign.
2. `RunDone`-triggered friction reflection on a REAL walked-to-completion run (not a hand-built fixture like round 2 used) — **CLOSED**, reflection agent spawned for real, made a sensible judgment, archived correctly.
3. A second independent interrupted-and-resumed repro, this time a genuine mid-agent kill on `Webster-Burler` (a non-Discussion `*-Burler` row, a different code path than any prior round drove) — **CLOSED**, clean reattachment, zero double-spawn.
4. Spontaneous Tier-2 friction note — **third independent null result** across three different models; treated as informative rather than an open item now.
5. F-6's live two-driver race — still unreproduced after three rounds; remains the campaign's one accepted, honestly-documented residual.

## ⚠️ Operator action needed — real-world leftovers from round 2's live driving

Round 2 drove live against `~/Code/lyx-test-HUB/`, an **existing** sandbox hub the operator already uses interactively (its own `lyx-test` tmux session, attached by the operator mid-round, was correctly left untouched). This is different from round 1, which built a disposable fixture hub from scratch. Three real side effects need the operator's own decision — the orchestrator has NOT touched any of these:

1. **A real, open pull request**: [Knatte18/lyx-test#2](https://github.com/Knatte18/lyx-test/pull/2). Round 2 was permission-blocked from merging or closing it. Merge it to let a future round drive Publish's merged-PR/Finalize leg live, or close it to abandon that fixture task.
2. **`~/go/bin/lyx`** (outside this git worktree — a machine-wide binary) was overwritten with the round's freshly-built dev binary, because a stale pre-campaign build there was silently downgrading shared stencils (the same D-2 hazard round 1 documented). The pre-round original is preserved at `~/go/bin/lyx.bak-crucible-r2`. Restore it, or leave the dev binary in place — operator's call.
3. **Two leftover dummy task pairs** under `~/Code/lyx-test-HUB/`: `dummy-r2-greet` (blocked at Publish awaiting the PR above) and `dummy-r2-reflect` (a done-state fixture, safe to delete any time).

None of this touches `Knatte18/loomyard` or this crucible worktree/branch — it's all in the separate `lyx-test` sandbox repo and the operator's own machine state outside this repo.

## Live-fire self-report tracking

**Already captured — still do NOT re-trigger.** Unchanged from round 1.
- Issue #240 (Tier 1) — OPEN, real.
- Issue #241 (Tier 2) — OPEN, real.
- Closed: **not yet** — campaign has not converged. Close both, labeled as deliberate crucible test-fires, only at final hand-off.

## Next action
Round 3 found and fixed a real BLOCKING bug, so it does not itself count as the clean safety pass. Ask the operator for round 4's model + effort pick (repeat one of Opus/Fable/Sonnet — the method suggests the most capable, i.e. Opus, for a final safety pass, but it's the operator's call), re-seed `_mill/loom-review-prompt.md` as round 4's safety pass (residual: F-6's still-unreproduced live race is the only carried-forward item; everything else is now closed), then spawn `subagent_type: crucible-reviewer-<effort>` tagged `<model>-<effort>-r4`.

**Still outstanding, independent of round 4:**
- The three `lyx-test-HUB` leftovers from round 2 (open PR `Knatte18/lyx-test#2`, swapped `~/go/bin/lyx` binary, two leftover dummy pairs) still await the operator's decision.
- Round 3's two disposable, already-archived GitHub repos (`Knatte18/lyx-crucible-r3`, `-weft`) — low-urgency, safe-to-delete cleanup whenever convenient.

**If round 4 comes back clean** (nothing found): per the method, convergence is safety pass + orchestrator's gates + (for a live-substrate module) an operator-assisted check all agreeing. Propose to the operator that this is convergence, state the campaign's one honest residual (F-6's live race never reproduced across four rounds — accepted, documented, not blocking), and move to hand-off: close issues #240/#241 (labeled as deliberate crucible test-fires) and let the operator decide on push/merge.
**If round 4 finds something**: keep going — re-seed for round 5, rotate model again (repeating is now unavoidable since all three have been used).
