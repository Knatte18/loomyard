# fabric crucible campaign — HANDOFF

Orchestrator's own state file. Refreshed after every round's verification. Never read by a round
agent (clean-room constraint — this file matches the banned `<module>-review-*` glob).

## Right now
Round 1 and round 2 both verified, closed. Round 3 (`fable-high-r3`) seed is finalized in
`_mill/fabric-review-prompt.md` and about to be spawned: `subagent_type: crucible-reviewer-high`,
`model: fable`. Base commit for this campaign segment: `08520a1b`; round 2 landed 13 commits
`b0aa40b4`..`e49d81f7` on branch `fabric-crucible-hardening`, working tree clean at `e49d81f7`.

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

## RESIDUAL currently seeded for round 3
**New finding, from the orchestrator's OWN independent verification of round 2's M3 fix (round 2
never found this — it's the SAME "never trust the round's own verdict" pattern that has now fired
twice in a row: round 1 → round 2 caught the unwire/.lyx race; round 2 → orchestrator-verification
caught this):** a symlink-target-toggle TOCTOU defeats the destruction chokepoint's containment
check ~15-20% of the time, letting a gated `remove --force` delete files outside the hub. This is
NOT a regression of M3's fix (M3's original problem — never resolving symlinks at all — stays
fixed) — it is a NEW, narrower check-then-act gap inside that fix.

- **Mechanism (root-caused by the orchestrator's verification fork, re-confirm before fixing):**
  `refuseUncontainedPath` (`internal/fabricengine/ancestors.go`) and `pathAtOrBelow`
  (`internal/fabricengine/destroy.go`) each call `filepath.EvalSymlinks` at their own instant
  during the CHECK phase; if the symlink is dangling at that instant the fallback treats it as
  contained (correct for the legitimate not-yet-existing-target case). But `removePath`'s actual
  `os.Lstat`+`os.Remove` runs at a LATER, separate instant with no re-check — if the symlink has
  since been made live-and-escaping, the deletion proceeds through it, uncontained.
- **Repro:** real hub from local bare git remotes, deployed dev binary, symlink at
  `_launchers/<slug>` (or another intermediate path segment), a tight external loop toggling that
  symlink's target between absent and live-outside-the-hub concurrently with one `remove --force`
  call. Confirmed via the tool's OWN mutation record naming a path removed outside the hub. Hit
  rate ~15-20% across multiple independent runs — reproduce with dozens of attempts, not a
  handful.
- **Severity:** at least MEDIUM, seriously consider BLOCKING — `doc.go` calls containment "the one
  thing `--force` can never override"; this defeats exactly that, with real data loss outside the
  hub as the consequence, under real (if adversarially-timed) conditions.
- **Fix the right layer:** a second `EvalSymlinks` call immediately before act narrows the window
  but does not close it — same class of gap, smaller. Needs real design thought: capture the
  resolved path at check time and have the executor verify it still matches immediately before
  acting (open with `O_NOFOLLOW`, compare device/inode via `Lstat`, or similar), not just
  re-resolving a nominal path twice at two different instants. Full detail and specific attack
  shapes to try post-fix (symlink loops, `..`-relative targets, other `pathRequest` call sites) are
  in `_mill/fabric-review-prompt.md`'s "High-yield focus" section — read it there, not here.

## Primary emphasis for round 3 — chokepoint, third consecutive round, now with a CONFIRMED open defect
`internal/fabricengine/destroy.go` has now had two rounds' worth of dedicated adversarial budget
(round 1 re-verification + round 2 as primary target) and both a round's own review AND the
orchestrator's independent verification of that round's fix have each found a real containment
bypass in it — round 2 found M3 itself, the orchestrator's verification found the follow-on TOCTOU
above. This is no longer "probably clean, verify once more" — it is "this file has broken twice
under adversarial pressure, in the same function, and deserves continued dedicated attention until
a round produces a fix that survives independent re-attack." Round 3 (Fable High) carries the
chokepoint as PRIMARY target for a third consecutive round, per the operator's explicit
instruction, headlined by fixing and then re-attacking the CONFIRMED residual above.

## DEFERRED list
Empty — round 2 fixed all 12 of its own findings, nothing deferred.

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
1. `_mill/fabric-review-prompt.md` is already rewritten for round 3 (residual = the containment
   TOCTOU above, chokepoint re-affirmed PRIMARY for a third round, CLOSED-AND-VERIFIED updated
   with round 2, deferred/round-context sections updated). Commit it together with this HANDOFF
   update.
2. Spawn round 3: `subagent_type: crucible-reviewer-high`, `model: fable`, tag `fable-high-r3`.
3. Per the operator's 4-round plan: round 4 is Opus High (final safety pass, hard cap regardless
   of residual state — state limits honestly in the convergence verdict if anything is still open
   after round 4, per README's "state the limits" rule). If round 3 does not manage to produce a
   containment fix that survives independent re-attack, that itself is important information for
   round 4's seed and the eventual convergence verdict — do not treat "round 4 is the last one" as
   pressure to understate an unresolved chokepoint defect.
