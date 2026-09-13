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
