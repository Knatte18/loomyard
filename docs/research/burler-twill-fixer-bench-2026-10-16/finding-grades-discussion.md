# Finding grades: Discussion-Review perch (shed-tackler, base 9ea1f7474)

Code read at `9ea1f7474` only.
Checked there: `internal/shuttleengine/claudeengine/settings.go` (no deny covers Edit, Write, `git add` or `git commit`), `internal/battenshed/doc.go:160` (batten never pushes the prime), `internal/shedrun/paths.go` (driver lock, park marker, resume report, drive reports and tackler dirs already per-run-id; only `SettleRecordsFile` takes a friction dir), `internal/loomengine/config.go:166` (driver log at `.lyx/loom/driver.log`), `internal/battencli/wire.go:1073,1133,1141` (batten reads `shedrun.TacklerMarker` and `shedrun.DriverLock`; only the settle read goes through `LoomFrictionDir`), `internal/configreg/runsettings.go:56-64` (run_settings marshalled with `yaml.Marshal`, then `Migrate`), `internal/loomengine/template.yaml:36-39`.

## twb-btf-ks (arm s)

### 1. Round-1 dispositions

| Source | Sev | Title | Grade | Re-rated | Disposition | Right? |
|---|---|---|---|---|---|---|
| R1 F1 | B | Batten tamper removal has no stated bound | real | M | fixed as proposed (second branch); bound named as "stencil's write rule and the agent settings' denies" | wrong: the denies bound no write, so the stated bound is false (key KS) |
| R1 F2 | B | Migrate rewrite permanent, reaches run_settings | real | B | fixed as proposed (option a) | right |
| R1 F3 | M | Darn tackler settings source unnamed | real | M | fixed as proposed | right |
| R1 F4 | M | Halted-row-only resume has no seam | real | M | fixed as proposed | right |
| R1 F5 | L | D2 Bound line overstates hand-back | real | L | fixed as proposed | right |
| R1 F6 | L | D3 no-go-seeded premise false | real | L | fixed as proposed | wrong: the proposed text is itself false: "no run has driver records today" (drive reports exist and already sit at shedrun paths) and "adopted through the loop's waiter" (the old build holds the run lock in-process, so the new build exits busy) |
| R1 F7 | L | Acceptance misses D2/D7 behaviour | real | L | fixed as proposed | right |
| R1 F8 | L | Set tackler_timeout_min 0 meaning open | real | L | fixed as proposed (inherit) | right |
| R1 F9 | N | abandonedSession named twice | noise (two envelopes do carry it) | N | fixed as proposed | right (noise, not worse) |

### 2. Fork findings

None (arm s).

### 3. Round-2 review

| Source | Sev | Title | Grade | Re-rated | Class | Cause |
|---|---|---|---|---|---|---|
| R2 F1 | B | Design already landed on main (a52467411) | noise | - | setup | - |
| R2 F2 | M | D4 names the agent settings' denies as a write bound; none exists; batten-never-pushes unstated | real | M | A | proposal (R1 F1's fix text offered "plus the agent settings' denies") |

### 4. Own audit of fix.diff

| Source | Sev | Title | Class | Cause |
|---|---|---|---|---|
| audit | L | D3 "No run has driver records today ... the records move": drive reports, driver lock, park marker, resume report, tackler dirs already sit at shedrun paths; a plan writer could relocate them (the reference's round-2 F2) | A | proposal |
| audit | L | D4 old-build sentence: a new-build run "adopts" the old process's step through the loop waiter; the old build holds the run lock in-process with no loop pid file, so the new driver exits busy (the reference's round-2 F3) | A | proposal |
| audit | L | D5 migration keyed on "their old template comments" leaves the compare unspecified against reconcile's re-encoded spacing (same defect twb-btf-kt's round 2 reports) | A | proposal |

Key KS: R. D4's sentence, D4's Bound line and the acceptance line ("spawns under the same agent settings' denies as a loom tackler") all name the agent settings' denies as a bound on the tackler's prime writes, and no installed deny covers Edit, Write or git.

Tally: round-1 wrong 2; fork findings n/a; fork dispositions wrong n/a; round-2 findings real 1 / wrong 0 / noise 1; class A by review 1 (proposal 1 / execution 0), by audit 3 (proposal 3 / execution 0); class B 0; setup 1.

## twb-btf-kt (arm t)

### 1. Round-1 dispositions

| Source | Sev | Title | Grade | Re-rated | Disposition | Right? |
|---|---|---|---|---|---|---|
| R1 F1 | B | Batten tamper removal has no stated bound | real | M | fixed as proposed (second branch), dropping the denies: only bound is the stencil's write rule, stated as prompt text | right |
| R1 F2 | B | Migrate rewrite permanent, reaches run_settings | real | B | fixed as proposed (option a), all four comments replaced | right |
| R1 F3 | M | Darn tackler settings source unnamed | real | M | fixed as proposed | right |
| R1 F4 | M | Halted-row-only resume has no seam | real | M | fixed as proposed | right |
| R1 F5 | L | D2 Bound line overstates hand-back | real | L | fixed as proposed, plus D2's opening sentence | right |
| R1 F6 | L | D3 no-go-seeded premise false | real | L | fixed differently: only settle records and driver log move, in-flight effect stated; old-build sentence rewritten to busy exit after self-check | right |
| R1 F7 | L | Acceptance misses D2/D7 behaviour | real | L | fixed as proposed, build-change case narrowed to blocked | right |
| R1 F8 | L | Set tackler_timeout_min 0 meaning open | real | L | fixed as proposed (inherit) | right |
| R1 F9 | N | abandonedSession named twice | noise | N | fixed differently: names both envelopes once | right |

### 2. Fork findings

| Source | Sev | Title | Grade | Re-rated | Disposition | Right? |
|---|---|---|---|---|---|---|
| claims | M | Old-build sentence: run lock "serializes" steps and a detached step is adopted; it refuses (busy) | real | L | fixed as proposed | right |
| way-forward | M | Same sentence: new driver exits busy, no serialized hand-off | real (dup of claims) | L | merged, fixed as proposed | right |
| consistency 1 | M | D8 and its criterion still say the tackler marker and driver lock move | real | L | fixed as proposed | right |
| consistency 2 | L | D3's records sentence reads as a move list | noise | - | fixed as proposed | right (noise, not worse) |
| consistency 3 | L | In-flight effect on batten's publish-retry read unstated | real | L | fixed as proposed (D8 one-time extra Publish retry) | right |

### 3. Round-2 review

| Source | Sev | Title | Grade | Re-rated | Class | Cause |
|---|---|---|---|---|---|---|
| R2 F1 | M | Migration comment match unspecified against reconcile's re-encoded spacing | real | L | A | proposal (R1 F2 option a's comment marker) |

### 4. Own audit of fix.diff

No further class-A defect.
Note, not counted: D3's "Go-seeded runs exist, ... live loom runs" rests on the bench pair's own seed under a later prod (setup), and at this base the hub's `loom.yaml` holds `driver_kind: llm`; the sentence's conclusion (which records move) holds either way.

Key KS: A. D4 and its Bound line name the stencil's write rule as the only bound and call it prompt text, and the acceptance line checks only the rendered rule. This came from the fixer's own fix, not from a fork.

Tally: round-1 wrong 0; fork findings real 4 (distinct 3) / wrong 0 / noise 1; fork dispositions wrong 0; round-2 findings real 1 / wrong 0 / noise 0; class A by review 1 (proposal 1 / execution 0), by audit 0; class B 0; setup 0.

## Rules adopted

- Real means the finding's premise holds at the base and changing the record changes what a plan writer would build or check. A premise that holds but changes nothing the plan does is noise.
- A finding that comes from the bench environment (a later main or prod, the bench pair's own seeds) is graded noise with class `setup`.
- A fix that applies a flawed proposed text is a wrong disposition, and the defect it brings in is class A, cause `proposal`.
- A dispute or own change is right when its evidence holds at the base.
- Fork duplicates count once in the distinct-real count.
