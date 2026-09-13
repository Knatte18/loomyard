# loom-step + self-report crucible campaign — orchestrator handoff

## Current state
Round 2 (`fable-high-r2`) complete and independently verified by the orchestrator. Findings are shrinking round over round (r1: 1 BLOCKING+4 MEDIUM+2 LOW+1 NIT; r2: 1 MEDIUM+2 LOW+1 NIT) but round 2 still found a real MEDIUM, so this is NOT yet a converged safety pass. Round 3 not yet spawned — waiting on the operator's model + effort pick, and on a decision about real-world leftovers round 2 created (see "Operator action needed" below) before spawning.

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
1. Get the operator's decision on the three leftovers above (or explicit "leave it, move on").
2. Ask the operator for round 3's model + effort pick. Sonnet is the one model in the Opus/Fable/Sonnet rotation not yet used — round 2's shrinking-findings trend (no BLOCKING, one MEDIUM) makes round 3 a reasonable candidate for the campaign's safety pass, but that's the operator's call on framing too.
3. Spawn `subagent_type: crucible-reviewer-<effort>` with `model: <pick>`, prompt: "Read `_mill/loom-review-prompt.md` and do exactly what it says." Tag it `<model>-<effort>-r3`.
