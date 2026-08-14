# fabric crucible campaign — HANDOFF

Orchestrator's own state file. Refreshed after every round's verification. Never read by a round
agent (clean-room constraint — this file matches the banned `<module>-review-*` glob).

## Right now
Round 4 (`fable-high-r4`) has FINISHED and been independently verified (fork `a8439474ef0d70b10`).
**The module is NOT yet fully mergeable — round 5 is warranted and being proposed to the operator.**
Round 4's F1, F3, F4 fixes are genuinely solid (sabotage-proven independently). But round 4's
"carried item 3" conclusion — that the two CREATE executors (`createExclusiveDir`/
`createGitWorktree`) have no symlink-directed-write exposure — is WRONG. Independent verification
found a live, reproducible escape: racing a symlink toggle against `Topology.Add`'s check-then-act
window (a MUCH wider window than M1's — `os.Stat` guard, then several git subprocess calls, then
the real `git worktree add`) causes `createGitWorktree` to write a real worktree through the
symlink to a location OUTSIDE the hub, ~1.7% hit rate across 240 trials (4/240). This is the same
shape as M3/M1 (nominal-path trust defeated by a racing symlink) but on the WRITE side instead of
the delete side — round 4's reasoning (`os.Mkdir` EEXIST, `os.Stat`-follow, git's lstat refusal)
was sound for STATIC symlink placement but was never subjected to an actual live timing attack,
the same rigor a "confirmed defect" claim would get. This is round 5's seeded residual — see below.
Base commit for this campaign segment: `08520a1b`; round 3 landed 5 commits
`8773625c`..`e0cb3dea` on branch `fabric-crucible-hardening`,
working tree clean.

**Operator instruction (2026-08-14) — round 4 model changed from Opus to Fable:** the original
fixed 4-round plan was r1 Opus/medium, r2 Opus/high, r3 Fable/high, r4 Opus/high (final safety
pass). The operator said "Kjør Fable High for R4 også. Den var god" — round 4 is **Fable/high**,
NOT Opus/high.

**Operator correction (2026-08-14) — round 4 is NOT a hard cap.** The orchestrator had been
describing round 4 as "the last round, hard cap, no round 5" throughout the round-4 seed and this
file. The operator corrected this directly: **"Det er ikke bestemt siste runde. Det er den siste
jeg konfigurerte. Vi kan fint kjøre flere."** Round 4 is only the last round explicitly
pre-configured at the campaign's start (the fixed model+effort schedule the operator gave up
front) — NOT a decided stopping point. If round 4 (or the orchestrator's independent verification
of it) surfaces something that warrants continued work, propose and run round 5+ rather than
forcing closure. Retire the "hard cap"/"no round 5" language everywhere it appears in this file
and in `_mill/fabric-review-prompt.md` — it does not reflect the operator's actual position.

**Historical context the operator gave for why this campaign exists at all (2026-08-14):** the
module's ORIGINAL 6-round crucible campaign (the one that ended at commit `79a72a38`, well before
this campaign started) itself concluded that fabric needed a dedicated destruction gate. At that
point the operator **interrupted crucible mid-campaign** and had `destroy.go` engineered as direct
work instead (slice 12, commit `3184cd5a`) — the chokepoint this entire current campaign has spent
three rounds hammering. That gate was therefore never itself the subject of an independent
crucible review+fix round until THIS campaign — which is precisely why the operator insisted so
strongly, from the very first message, that the chokepoint get dedicated adversarial rounds rather
than being assumed sound just because it was built in direct response to a prior campaign's
findings. This context also sets a precedent worth remembering for round 4 and beyond: if this
campaign's own findings point toward "this needs dedicated engineering work, not another review
round," interrupting crucible to go build that (the same move the operator made last time) is a
legitimate, previously-used outcome — not a failure of the method.

Round 3's finding: **M1, the containment TOCTOU seeded from the orchestrator's own verification
of round 2's M3 fix** — fixed via `removeContainedPath`, a new helper routing the two arbitrary-
path executors (`removePath`, `removeLink`) through Go 1.26's `os.Root`, rooted at the gate's
container, so containment resolution and the actual removal happen as one atomic operation instead
of two separately-timed syscalls. **This is the first fix in the campaign to survive independent
adversarial re-attack**: the orchestrator's verification (fork `ab4252fa952a6c97d`) confirmed via
the Go 1.26 stdlib's own documented `os.Root` semantics that the window is genuinely closed (not
narrowed), then live-attacked it — 160 trials of the original toggle-race repro (0 escapes),
symlink loops (refused via ELOOP), `..`-relative targets (refused) — nothing got through. One
accuracy issue, not a functional defect: the fixer report overstated what its integration test
proves (it's redundant with round 2's M3 check-phase fix, not a distinct guard on M1's
executor-level fix) — carried into round 4's seed as item 1 to correct.
Verification also flagged three more items, all folded into round 4's seed: (2) a cosmetic
honesty gap on symlink-loop refusals (`ok:true` while silently dropping one unremoved entry), (3)
an unconfirmed symlink-directed-write angle on the two CREATE executors
(`createExclusiveDir`/`createGitWorktree`) — never attacked by any round yet, and (4) round 2's
"inert leftover directory" — still open, still never independently re-driven live by anyone.
N4's dirtiness-probe TOCTOU is now SETTLED as an accepted, documented residual (two independent
attempts at a live repro failed; round 3 traced the reachable paths and confirmed no weak link;
the orchestrator's verification of round 3 read that reasoning and found it sound) — treat it like
the Windows-path limit going forward, state it, do not re-chase it.

Given three rounds' worth of adversarial pressure on the chokepoint with the last fix surviving
independent re-attack, round 4's seed broadens scope: the chokepoint graduates from primary target
to spot-check status, and the round's main Job-1 budget goes to a comprehensive final sweep of the
WHOLE module (the way round 1 did) plus the four carried items above — this is the last scheduled
round, nothing catches what it misses.

Round 2's headline finding: **M3, a real symlink-mediated containment bypass in the destruction
chokepoint** — a symlink planted at an intermediate path segment let a gated `remove --force`
delete files outside the hub, `ok:true`, exit 0. Reproduced live, fixed (resolve via
`filepath.EvalSymlinks`), independently re-verified. Also root-caused round 1's seeded residual
(M1, re-graded MEDIUM, fixed) and found `reconcile` silently reporting success on a pair it failed
to repair (M2, fixed). 12/12 findings fixed, nothing deferred. Full detail in the review report.

**The orchestrator's own independent verification of round 2 (agent `ac379077d25106fce`, complete)
found something round 2 itself missed: a SECOND, more severe containment bypass in the same fixed
functions** — a TOCTOU distinct from M3. `refuseUncontainedPath`/`pathAtOrBelow` each call
`filepath.EvalSymlinks` at their own instant during the check phase; the executor's actual
`Lstat`/`Remove` runs at a later, different instant with no re-check. Racing a symlink's target
between absent and live-and-escaping during a single `remove --force` defeats containment ~15-20%
of the time — confirmed via the tool's own mutation record showing a path removed outside the hub.
This is now round 3's PRIMARY seeded residual (a CONFIRMED, open, unfixed defect — not a
hypothesis). Everything else round 2 claimed was independently confirmed to hold: gates green
cold, all 9 new tests sabotage-proven (including N5's follow-up fix, via compile-dependency), the
M2-fix's self-caught regression (8/8 remove-vs-reconcile races) confirmed genuinely re-fixed.
N4's dirtiness-probe TOCTOU attempt did not improve on round 2's own PLAUSIBLE-only grading —
still open, still unproven live, folded into round 3 as a secondary target.

## Operator instruction (2026-08-14) — more chokepoint testing in round 3 too — ACTED ON
The operator's instruction ("mer chokepoint-testing i neste Round også, og evt annet som du eller
R2 mener bør testes mer") is now baked into `_mill/fabric-review-prompt.md`'s round-3 seed:
chokepoint stays PRIMARY target for a third consecutive round, headlined by the new CONFIRMED
containment TOCTOU above (round 3's Job-1 task: reproduce it themselves, root-cause-confirm, fix
with real design thought — not just a second EvalSymlinks call — then adversarially re-attack
their own fix, including symlink loops/`..`-relative targets/other pathRequest call sites round 2
didn't explicitly cover). N4's TOCTOU is folded in as a secondary target with instructions to try
harder than both round 2 and the orchestrator's verification managed. Round 2's fixer report had
nothing deferred, so nothing else to fold in from that source. The "inert leftover directory"
post-freeze observation is folded in as a low-priority spot-check only.

## CLOSED-AND-VERIFIED
**Round 4 (`fable-high-r4`) — PARTIALLY closed, see round 5's residual below for what is NOT
closed.** 4 LOW (F1-F4) + 1 NIT (F5), 6 commits `7f49049d`..`f19bc1d6`. Independently verified
(fork `a8439474ef0d70b10`): gates green cold; F1 (`applyStaleRemoval` false-convergence report)
and F4 (Add's dir-exists error names the cleanup remedy) sabotage-proven genuinely — reverting the
production hunk fails the test at the claimed assertion. F3 (correcting round 3's fixer report's
overstated M1-integration-test-coverage claim) independently traced through the actual code
(`containmentPath`→`resolveAncestorSymlinks`) and confirmed the correction is itself accurate.
F2 (surfacing `rollbackAdd`'s swallowed warp-branch-deletion refusal via a WARN log) is confirmed
to fire live and real, but its "regression test" doesn't actually sabotage-prove the log line —
reverting the whole production diff still leaves the test green, since it only asserts
pre-existing behavior. Low-priority test-coverage gap, not a functional defect; not urgent, but
should not be re-described as sabotage-proven without this caveat. F5 (ELOOP-swallow, no code
change) — reasoning holds.
**Carried item 3's "not a defect" verdict is WRONG — this is NOT closed, it's round 5's seeded
residual.** See below. Round 4's live-driving on this specific item was reasoning-plus-static-probe
only, never an actual timing attack — the same gap in rigor the whole campaign has repeatedly
found between "looks safe" and "survives an adversarial race."

**Round 3 (`fable-high-r3`)** — 1 MEDIUM (M1, the containment TOCTOU below), fixed via
`removeContainedPath` (Go 1.26 `os.Root`, routing `removePath`/`removeLink`), 5 commits
`8773625c`..`e0cb3dea`. Independently verified by the orchestrator (fork `ab4252fa952a6c97d`):
confirmed the fix genuinely closes the window (checked against Go 1.26 stdlib `os.Root` semantics
directly, not just the round's claim), sabotage-proved the hermetic regression test, and
adversarially re-attacked live — 160 trials of the original toggle-race (0 escapes), symlink loops
(refused), `..`-relative targets (refused). **First fix in this campaign to survive independent
adversarial re-attack.** One accuracy issue (not functional): the fixer report overstated the
integration test's coverage of M1 — carried into round 4 as item 1 to correct. N4's TOCTOU is now
SETTLED as an accepted, documented residual, not open work. Merge-readiness: MERGEABLE, confirmed.

**Round 2 (`opus-high-r2`)** — 12 findings (0 BLOCKING, 3 MEDIUM: M1 stuck-reconcile/logger-sink,
M2 dishonest reconcile success, M3 containment-check-never-resolves-symlinks; 4 LOW: L1 dropped
`--force`, L2 vacuous gate on absent targets, L3 `entries:null`, L4 dangling-HEAD clone; 5 NIT),
all fixed, 13 commits `b0aa40b4`..`e49d81f7` (ends in the fixer report commit). Independently
verified by the orchestrator (fork `ac379077d25106fce`) from a cold state: build/vet/test and
live-integration gates re-run and green; ALL 9 new regression tests sabotage-proven (production
hunk reverted, confirmed the test fails at the exact assertion claimed, restored, confirmed empty
diff — N5's follow-up fix proven via compile-time dependency instead, an even stronger check); the
M2-fix's self-caught 8/8 `remove`-vs-`reconcile` regression confirmed genuinely re-fixed, not just
claimed. M1 closes round 1's seeded `unwire`/`.lyx` residual — confirmed. Ownership predicates,
`createdToken` unforgeability, `--force`-answers-dirtiness-only, the raw-primitive inventory, and
the concurrent-race combinations round 2 drove (4×`remove --force`, `remove` vs `reconcile`,
`prune` vs `add`) were re-derived/re-driven by round 2 and hold.
**Caveat — M3's fix is NOT fully closed:** the ORIGINAL M3 finding (containment never resolved
symlinks) is closed, but the orchestrator's own verification found a SECOND bypass in the same
fix — see "RESIDUAL currently seeded for round 3" below. Do not describe M3 as fully closed to a
future round; describe the original finding as closed and the follow-on TOCTOU as open.
Do not re-litigate anything else from round 2's 12 findings unless a later round finds a genuine
regression.

**Round 1 (`opus-medium-r1`)** — 7 findings (0 BLOCKING, 0 MEDIUM, 3 LOW, 4 NIT), all fixed, 8
commits (`fff4bdc6` F4, `509a3f4a` F5, `7297e8d2` F7, `aed410ba` F1, `22bcdac0` F2, `1bea8e09` F6,
`33d67a6c` F3, `d5cb6a83` F1-cont). Independently verified by the orchestrator from a cold state:
build/vet/test and live-integration gates re-run and green; 3 of the 4 new regression tests
sabotage-proven independently (production hunk reverted, confirmed the test fails at the intended
assertion, restored, confirmed empty diff) — `TestWorktreeDirty_ErrorNamesGitCommandOnce` (F1),
`TestRemove_RefusalNamesStrandedPortalTeardown` (F3), and
`TestStageAndCommit_PathspecMissMarkerSurvivesTheErrorChain`; the fourth
(`TestGitError_ErrorOmitsDir`) is a doc-only NIT with no revertible code change, correctly skipped.
Pinned raw-site count (fabricengine 2, gitrepo 3) independently re-derived from source, matches.
Do not re-litigate any of F1-F7 unless a later round finds a genuine regression.

**Do NOT re-litigate:** the gitexec migration (`74e6a6bb`) itself — round 1 drove every mixed
error-recovery site live and found no shape-of-migration-failure defect, only the doc drift F1-F7
already closed. The fixture-inversion (`f4ce0188`) and `t.Parallel` unblock (`16c0cfcc`) — both
independently confirmed delivered as intended, drove real scenarios against both. Treat these
three commits as settled unless a later round's own independent driving turns up something new.

## RESIDUAL currently seeded for round 5 (proposed to operator, not yet confirmed)
**New finding, from the orchestrator's OWN independent verification of round 4's "carried item 3"
conclusion (round 4 investigated this and got it WRONG — same "never trust the round's own
verdict" pattern, fifth time it's fired in this campaign):** the two CREATE executors
(`createExclusiveDir`, and specifically `createGitWorktree` via `Topology.Add`) have a live,
reproducible symlink-directed-write escape — the write-side twin of M3/M1's delete-side
containment bypasses.

- **Mechanism:** `add.go`'s `os.Stat(target)` guard runs, then several git subprocess calls
  execute (list remotes, stat weft target, check weft branch, rev-parse HEAD), THEN
  `createGitWorktree`'s real `git worktree add` finally runs — a multi-syscall, multi-subprocess
  check-then-act window, much wider than M1's original nanosecond-scale gap. A symlink toggled at
  the target path during that window can make `git worktree add` write a complete real worktree
  through it to a location OUTSIDE the hub.
- **Repro (independently reproduced):** a goroutine toggling a symlink at the Add target (pointing
  to a genuinely-outside-the-hub temp dir) on/off every 20µs, racing one `Topology.Add` call.
  240 trials (6 batches of 40): **4 escapes (~1.7%)** — a full worktree (`.git`, `README`, source
  dirs) written outside the hub; fabric's own subsequent `wire junctions` step then fails with a
  `chdir` error on the nominal in-hub path once the racer removes the symlink, so the nominal path
  and the actual write location have diverged — same shape as M3, opposite direction.
- **Why round 4 got this wrong:** they tested only STATIC symlink placement (already live before
  the call) and reasoned from `os.Mkdir` EEXIST semantics, `os.Stat`-follow behavior, and git's own
  lstat refusal — sound for the non-racing case, but never subjected to an actual timing attack.
  Their stated threat model ("no concurrent fabric writer is expected at the unique slug path")
  mis-scoped the threat: it's an external adversarial symlink-planter, exactly the model this
  campaign has used for M3 and M1 throughout.
- **Severity:** likely MEDIUM, same class as M1 — real data can be written to an unintended
  location outside the hub under adversarial timing, though (unlike M1) nothing inside the hub is
  destroyed; grade it once reproduced and root-caused independently by round 5.
- **Fix the right layer:** likely the same shape as M1's fix — route `createGitWorktree`/
  `createExclusiveDir` through the same `os.Root`-rooted containment machinery
  `removeContainedPath` already uses for `removePath`/`removeLink`, so creation and the
  containment check happen as one atomic operation the way removal now does. Round 5 should
  verify this is actually the right generalization, not assume it.

**Not yet an operator-approved seed** — the operator's own position ("Det er ikke bestemt siste
runde... Vi kan fint kjøre flere") means round count isn't fixed, but round 5 itself should be
proposed and confirmed, not launched unilaterally, since it's beyond the originally-agreed 4-round
schedule. See "Exact next action" below.

## Primary emphasis for round 5 (proposed) — the create-side containment gap, chokepoint's write-side twin
The delete-side containment property (`removePath`/`removeLink`) has now survived two rounds of
independent adversarial re-attack. The create-side has never been attacked with live timing
pressure until the orchestrator's verification of round 4 did it — and it broke. Round 5's Job-1
task, if approved: reproduce the above independently, root-cause it the way M1 was root-caused,
fix it with the same rigor (a fix that provably CLOSES the window, not narrows it, verified against
the language's actual concurrency/filesystem-API semantics the way M1's `os.Root` fix was), then
adversarially re-attack the fix itself before declaring it closed — the same discipline that made
M1 the first fix in this campaign to survive re-attack.

## DEFERRED list
Empty — round 2 fixed all 12 of its own findings, round 3 fixed its one finding, nothing deferred
by either. (Round 3's deliberate call to leave N4 as an accepted residual is not a deferral — it's
a considered, independently-confirmed-sound judgment call, see CLOSED-AND-VERIFIED above.)

## Method-doc hygiene, already done (do not redo)
`crucible/*.md` and all five `.claude/agents/crucible-reviewer-*.md` now point at `_mill/`
(committed) instead of `.scratch/` (gitignored), with a commit-continuously rule, a rule banning
orchestrator `git add`/`git commit` while a round is live (an incident during round 1 — see git
log `74bca030`), and a clean-room-leak fix explicitly naming the HANDOFF note as off-limits to a
round agent even though it doesn't read like a "review" (round 1 partially, briefly acted on this
file's content before self-correcting). All committed. Round 1's own deliverables were relocated
from `.scratch/` to `_mill/` after the fact (commit `eea90e7a`) since it was seeded before the
convention changed — round 2 onward is seeded directly at `_mill/` from the start.

## Exact next action
1. Round 4 finished and was independently verified (fork `a8439474ef0d70b10`). Its F1/F3/F4 fixes
   hold; F2 has a minor test-coverage caveat (not urgent); its "carried item 3" conclusion is
   WRONG — a real, live, reproducible create-side symlink-write escape exists. This has been
   reported to the operator with a recommendation to run round 5, seeded with that escape as the
   primary residual (see above). **Awaiting operator confirmation before spawning round 5** — do
   not spawn unilaterally, this is beyond the originally-agreed 4-round schedule.
2. Once confirmed: rewrite `_mill/fabric-review-prompt.md` for round 5 (residual = the create-side
   TOCTOU above, CLOSED-AND-VERIFIED updated with round 4's partial closure, deferred/round-context
   sections updated), commit, then spawn round 5. Model/effort: ask the operator, or default to
   Fable/high given round 3 and round 4's strong results at that tier — but this is the operator's
   call, not a default to assume silently.
3. After round 5's own independent verification: assess again whether to continue or wrap up, same
   honest-either-way judgment as after round 4. If a future round's finding looks like it needs
   dedicated engineering work rather than another review round — the fork the operator took after
   the ORIGINAL 6-round campaign, which is why `destroy.go` exists and why this campaign exists —
   say so explicitly and propose it, rather than defaulting to "run another round" or "declare
   done."
