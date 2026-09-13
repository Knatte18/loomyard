# loom (loom-step + self-report Tier 1 + Tier 2) — independent review, round 2

Reviewer: crucible-reviewer-high (Fable 5), 2026-09-13.
Clean-room pass: no `_mill/loom-review-r1*` or `_mill/loom-review-HANDOFF.md` material read before the findings list below was complete.

## Executive summary

(to be written at the end of Job 1)

## Scope assessment (plan-vs-shipped)

Read pass complete (design docs `loom-step.md`, `self-report-tier1.md`, `self-report-tier2.md`, `loom.md`; code per the prompt's "What to read" list; `ly-supervise` SKILL.md; S8; overview/roadmap/CONSTRAINTS spot-checks):

- `lyx loom step`: ten-key envelope shipped exactly as documented (`stepEnvelope`, closed-set test `TestStepEnvelope_KeySetIsExactlyTen`); five-kind error vocabulary shipped and closed-set-tested; early busy probe + bootstrap-stage mapping present; no Tier-1/Tier-2 firing from step, documented in all three places (loom-step.md "Neither self-report tier fires from step", tier1.md exemption paragraph, SKILL.md "Self-report" section). Matches spec.
- Tier 1: five anomaly kinds shipped with the documented title discriminators (halt kinds keyed on producer+done-count, crash-resume on producer@history-length, recurring-finding on row+key); four-step filing pass (collapse/filter/file/record-immediately) as documented; every failure degrades to Warn. `selfreport` knob gates before any read. Matches spec.
- Tier 2: friction leaf + frictionengine shipped as documented; directive roles (4), marker-absent warning, non-clobbering NotePath, EnsureDir; reflection with non-blocking LoomFrictionLock, archive-on-clean-return-only; `drive` fires on RunDone|RunBlocked only; shared bootstrap owns clear-on-first-seed/ensure-on-re-entry for both verbs. Matches spec.
- `ly-supervise` skill text matches current code behavior on all checked claims (five kinds, one-retry-on-producer-only, interrupt_policy read off status, "no orphan" claim backed by the four adapters' probes incl. the re-bounce probe).
- No shipped-beyond-scope surface found; `Plan-Sweep` absent as designed (out of scope).

## Code findings (severity-ranked)

Provisional entries (appended as spotted; finalized after the live pass):

- P-1 (provisional, NIT) `internal/loomcli/drive.go:236` — `reflectFriction`'s MkdirAll-failure warning says "could not create the friction lock's directory" but logs `"dir", c.frictionDir`; the directory actually being created is `filepath.Dir(loomengine.LoomFrictionLock(...))` (the loom scratch dir), so a diagnosing operator is pointed at the wrong path. Log-field mismatch only. CONFIRMED by reading.
- P-2 (provisional, LOW) `internal/loomcli/run.go:205-211` — the handshake logs an Info breadcrumb on `awaitRunLockChildDied` but nothing on `awaitRunLockHalted`, the disposition Tier 2 introduced. An operator diagnosing a resume that proceeded while the driver never took the lock (post-run bookkeeping, or the narrow wedge case below) has zero log evidence which path the bootstrap took. Suggested fix: symmetric Info log on the halted disposition. CONFIRMED by reading (asymmetry is in the code).
- P-4 (provisional, MEDIUM) `internal/loomcli/selfreport.go` (observeEntry) + `internal/loomengine/anomaly.go:109` (DetectCrashResume) — **a healthy step-driven task mid-walk is byte-identical to a crash signature, and the next `drive` files a spurious public GitHub issue for it.** A completed `lyx loom step` persists `state: running` with `current_producer` = next row and a non-empty history, and holds no run lock between steps (Step releases per call) — exactly DetectCrashResume's trigger (Observed, !RunLockHeld, StateRunning, HistoryLength>0). An operator legitimately switching from the supervised step loop to `lyx loom run`/`drive` mid-task (the skill's own hand-back flow invites this) gets a `crash-resume` issue filed into Knatte18/loomyard with `selfreport: true` (the shipped default), for a task in which nothing crashed. The Preflight-fresh-seed false positive got its own exclusion (history==0); this sibling did not. CONFIRMED by trace against the live walk's own status file (state running, lock free, history 9, between steps). Suggested fix: `step` records a machine-local step-handoff marker (history length + state) in the ephemeral tree after each completed step; `observeEntry`/detection suppresses crash-resume when the observed entry matches the recorded clean handoff. A killed-mid-producer step never updates the marker, so a genuine step-crash still files.
- P-5 (provisional, LOW) `plugins/ly/skills/ly-supervise/SKILL.md` + `manifest/designs/loom-step.md` — a friction note written during a step-driven run is silently dropped when the run completes under the supervisor: `step` never reflects (by design), the skill's Self-report section owns the reporting responsibility but never tells the supervisor WHERE notes live (`.lyx/loom/friction/`), and on the next task's first seed the shared bootstrap clears the directory (round 1's F-1, correct). loom-step.md's "a later run/drive reflection can still aggregate them" sentence covers only the resumed-run case, not a task that completes under step and merges. Suggested fix: one instruction in the skill's stopping/self-report flow to check `.lyx/loom/friction/` for notes before handing back, plus a clause in loom-step.md naming the completes-under-step case.
- P-3 (provisional, analysis pending live repro) `internal/loomcli/run.go:184-190` — the `halted` predicate reads the persisted state once per poll; on ANY resume of a task whose status file already reads non-`running` (blocked/paused/done — every ordinary resume), the very first poll returns true and the handshake proceeds ~100ms after spawn, before the driver has done anything. Wedged-spawn detection is therefore lost on every resume, not only during a reflection window. To be assessed live (SIGSTOP a freshly-spawned driver against an old blocked status file).

## Docs & operability findings

(in progress)

## What was tested

Appended incrementally, in order, as each command/scenario returned.

### Hermetic

- `go build ./...` — OK (exit 0).
- `go vet ./internal/loomcli/... ./internal/loomengine/... ./internal/loomshed/... ./internal/loomrecipe/... ./internal/friction/... ./internal/frictionengine/... ./internal/selfreportengine/... ./internal/selfreportcli/... ./internal/shedadapters/... ./internal/websterengine/...` — OK (exit 0).
- `go test -count=5 <the ten packages + cmd/lyx>` — all ok (loomcli, loomengine, loomshed, loomrecipe, friction, frictionengine, selfreportengine, selfreportcli, shedadapters, websterengine, cmd/lyx), exit 0.
- `go test ./...` (whole repo) — exit 0, no failures.

### Live substrate — bench setup

- Host had a STALE production `lyx` at `~/go/bin/lyx` (built Sep 8, pre-campaign) — the exact two-lyx stencil-rewrite hazard `manifest/designs/loom.md` documents. Backed it up (`~/go/bin/lyx.stale-crucible-r2`) and installed the freshly built dev binary at the same path so exactly one `lyx` is reachable from the hub; restore at teardown.
- `./deploy-dev` — built and deployed `lyx @ d8824d96e` to `.dev-bin/lyx`.
- Bench: the existing `~/Code/lyx-test-HUB` container (legacy suffix, resolves structurally per the Hub Suffix Invariant). An operator-owned `lyx reed attach` terminal on session `lyx-test` appeared at 16:29 and was left untouched.
- `lyx stencil sync` — refreshed 7 stale hub stencils (incl. `loom-template-discussion`/`loom-template-plan`, which carry the friction marker) and committed to the board repo.
- `lyx board upsert` — created task `dummy-r2-greet` whose brief/body reference a helper name (`FormatGreeting`) that does not exist on `main` — a genuine stale-brief rough edge for the spontaneous-friction probe, not a doctored prompt.
- `lyx fabric add dummy-r2-greet` — pair created and pushed.
- `_lyx/config/loom.yaml` overwritten per the cost declaration: discussion/plan/review `sonnet[effort=low]`, `selfreport: false`, `friction: haiku`, `friction_timeout_min: 10`.
- Checked: strict `configengine.Load` ERRORS on the stale 7-key `loom.yaml` vintage ("missing keys ... run lyx config reconcile") — a pre-Tier-1 pair can never silently default `selfreport: true`. Not a finding.

### Live substrate — step walk (dummy-r2-greet)

- `lyx loom status` / `lyx loom pause` on the never-bootstrapped pair: both named their own remedy ("no status file ... run \"lyx loom run\"") — F-3 holds live on this exact path.
- Step 1-4: `Preflight` stuck×4 (blocked, "stuck with no OnStuck target") — correctly refusing on my own uncommitted weft `loom.yaml` edit (`worktree-clean: M _lyx/config/loom.yaml` at Warn on stderr). Envelope fidelity: `continue:false`, `state:blocked`, `next:Preflight`, `next_interrupt_policy:reinvoke`, history_length incrementing per stuck.
- `lyx fabric sync` committed the config; next step: `Preflight` done → `Loom-Preflight` (history 5), then `Loom-Preflight` done → `Discussion-Write` (history 6), `continue:true` both. Resume-from-blocked re-call worked exactly as designed.
- `Discussion-Write` step (sonnet[effort=low], real agent in reed pane `discussion::39fd7c16`, status strand `loom-status` added by step's own bootstrap): done, output `_lyx/discussion/decision-record.md`, next `Discussion-Validate`, history 7. Both discussion files written.
- **Spontaneous-friction probe, observation 1:** the writer NOTICED the stale-brief rough edge — support-log.md records "No `FormatGreeting` helper exists yet, despite the task brief phrasing this as 'extend'" — but chose to absorb it into the discussion rather than write a friction note (`.lyx/loom/friction/` stayed empty). Genuine model judgment, not forced; recorded honestly.
- `Discussion-Validate`: done, next `Discussion-Bouncer`, history 8.
- Confirmed loom's Burler rounds are single-agent (no cluster fan wired in loomrecipe) — per-round live cost bounded to one review-fix agent + one judge.
