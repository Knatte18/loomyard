# fabric crucible campaign — HANDOFF

Orchestrator's own state file. Refreshed after every round's verification. Never read by a round
agent (clean-room constraint — this file matches the banned `<module>-review-*` glob).

## Right now
Rounds 1, 2, and 3 all verified, closed. Round 4 (`fable-high-r4`) seed is finalized in
`_mill/fabric-review-prompt.md` and about to be spawned:
`subagent_type: crucible-reviewer-high`, `model: fable`. Base commit for this campaign segment:
`08520a1b`; round 3 landed 5 commits `8773625c`..`e0cb3dea` on branch `fabric-crucible-hardening`,
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

## RESIDUAL currently seeded for round 4
No single headline defect this time — four bounded, specific items (all detailed in full in
`_mill/fabric-review-prompt.md`'s "High-yield focus" section, not repeated here):
1. M1's fixer report overstated its integration test's coverage (redundant with M3, not a
   distinct guard on M1's executor-level fix) — correct the claim or add a genuine regression test.
2. Cosmetic honesty gap: a symlink-loop refusal during `remove` reports `ok:true` while silently
   leaving that one launcher entry unremoved.
3. Unconfirmed: do the two CREATE executors (`createExclusiveDir`/`createGitWorktree`) have a
   symlink-directed-write angle, never attacked by any round yet?
4. Round 2's "inert leftover directory" — still open, still never independently re-driven live.

## Primary emphasis for round 4 — chokepoint graduates to spot-check, broaden to the whole module
Three consecutive rounds hammered the chokepoint; the round 3 fix survived independent adversarial
re-attack (first one in the campaign to do so) — see CLOSED-AND-VERIFIED above. This earns
"closed, watch for regressions" status. Round 4's Job-1 budget goes to a comprehensive sweep of
the WHOLE module (the way round 1 did), plus the four carried items above. (This was seeded as
"the last scheduled round" before the operator's 2026-08-14 correction above — round 4 is only
the last round pre-configured at campaign start, not a decided stopping point; a round 5+ is on
the table if warranted.)

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
1. `_mill/fabric-review-prompt.md` is already rewritten for round 4 (chokepoint graduated to
   spot-check, broad whole-module final sweep as primary, four carried items detailed,
   CLOSED-AND-VERIFIED updated with round 3, deferred/round-context sections updated). Commit it
   together with this HANDOFF update.
2. Spawn round 4: `subagent_type: crucible-reviewer-high`, `model: fable`, tag `fable-high-r4` —
   Fable per the operator's 2026-08-14 override, NOT the original plan's Opus.
3. Round 4 was the last round pre-configured at campaign start — NOT a decided stopping point
   (operator correction, 2026-08-14: "Det er ikke bestemt siste runde... Vi kan fint kjøre flere").
   After round 4's own independent verification, assess honestly whether the campaign should
   continue (round 5+) or wrap up: write a status verdict either way, stating what's fixed and
   confirmed (the chokepoint's containment property, now survived two rounds of independent
   re-attack across M3 and M1) and what remains open (Windows path/junction behavior — never
   executed, permanent gap on this Linux host; N4's dirtiness-probe TOCTOU — settled as a
   real-in-theory, no-demonstrated-exploit, documented residual; anything round 4 itself leaves
   open). If anything round 4 surfaces looks like it needs dedicated engineering work rather than
   another review round — the same fork the operator took after the ORIGINAL 6-round campaign,
   which is why `destroy.go` exists and why this campaign exists — say so explicitly and propose
   it, rather than defaulting to "run another round" or "declare done."
