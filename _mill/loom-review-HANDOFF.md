# loom-step + self-report crucible campaign — orchestrator handoff

## Current state
Round 2 (`fable-high-r2`) complete and independently verified by the orchestrator. Findings are shrinking round over round (r1: 1 BLOCKING+4 MEDIUM+2 LOW+1 NIT; r2: 1 MEDIUM+2 LOW+1 NIT). Operator decided: round 2 did NOT establish convergence (found a real MEDIUM), so round 3 is spawned as an explicit **safety pass** — `sonnet-xhigh-r3` — per the operator's pick. Re-seeded `_mill/loom-review-prompt.md` accordingly, instructing it to build its OWN disposable fixture hub rather than reuse the operator's `~/Code/lyx-test-HUB` (to avoid a third round of real-world leftovers on top of the two still pending the operator's decision — see below, still unresolved).

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
Round 3 (`sonnet-xhigh-r3`, safety pass) is spawned. Wait for its notification, then verify independently exactly as rounds 1/2 were verified (cold rebuild/vet/test, sabotage-prove every new/changed test, confirm no fourth GitHub issue, confirm it built its own fixture hub rather than touching `lyx-test-HUB`).

**Still outstanding, independent of round 3:** the three `lyx-test-HUB` leftovers from round 2 (open PR `Knatte18/lyx-test#2`, swapped `~/go/bin/lyx` binary, two leftover dummy pairs) still await the operator's decision — raise again once round 3 completes if not addressed sooner.

**If round 3 comes back clean** (no BLOCKING/MEDIUM, ideally nothing at all): per the method, convergence is safety pass + orchestrator's gates + (for a live-substrate module) an operator-assisted check all agreeing. Propose to the operator that this is convergence, note the campaign's stated residuals honestly (F-6's live race never reproduced across three rounds; Publish/Finalize's merged-PR leg's live status depends on what round 3 managed to drive), and move to hand-off: close issues #240/#241 (labeled as deliberate crucible test-fires) and let the operator decide on push/merge.
**If round 3 finds something**: re-seed for a round 4, rotating model again (all three of Opus/Fable/Sonnet will have been used by then — repeat one, operator's choice).
