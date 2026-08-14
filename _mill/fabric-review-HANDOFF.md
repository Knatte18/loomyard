# fabric crucible campaign — HANDOFF

Orchestrator's own state file. Refreshed after every round's verification. Never read by a round
agent (clean-room constraint — this file matches the banned `<module>-review-*` glob).

## Right now
Round 1 verified, closed. Round 2 (`opus-high-r2`) is RUNNING — Job 1 done (12 findings: 0
BLOCKING, 3 MEDIUM, 4 LOW, 5 NIT; review report `_mill/fabric-review-opus-high-r2.md`), Job 2 in
progress (M3, M1, M2 fixed and committed as of `97052a7a`; L1 next, working tree has uncommitted
edits to `remove.go`/`prune.go` — DO NOT touch, per hard rule 3, until it finishes or pauses).
Base commit for this campaign segment: `08520a1b` on branch `fabric-crucible-hardening`.

Round 2's headline finding: **M3, a real symlink-mediated containment bypass in the destruction
chokepoint** — a symlink planted at an intermediate path segment let a gated `remove --force`
delete files outside the hub, `ok:true`, exit 0. Reproduced live. Also root-caused round 1's
seeded residual (M1, re-graded MEDIUM — no racing needed, worse than seeded) and found `reconcile`
silently reporting success on a pair it failed to repair (M2). Full detail in the review report;
don't re-summarize it here, read the file when verifying.

## Operator instruction (2026-08-14) — more chokepoint testing in round 3 too
The operator wants round 3 (Fable High) to ALSO carry chokepoint adversarial testing as a named
focus — not just round 2's one-off assignment — plus whatever else the orchestrator or round 2
itself flags as deserving more scrutiny. Round 2's review report has a few natural candidates
worth carrying forward into round 3's seed once round 2 is verified:
- **M3's fix itself** — the symlink-resolution fix in `refuseUncontainedPath`/`pathAtOrBelow` is
  exactly the kind of security-shaped fix that deserves its own adversarial re-attack, not just a
  sabotage-proof of the regression test. Try OTHER symlink placements/shapes round 2 didn't
  (symlink loops, a symlink whose target itself doesn't exist yet at check time but does by act
  time, `..`-relative symlink targets) once round 2's fix is verified in.
- **N4's TOCTOU window** (dirtiness probe is check-then-act, not atomic) — round 2 explicitly
  could not construct a live repro and recorded it as CONFIRMED-by-source/PLAUSIBLE-as-an-event.
  A dedicated round with more budget on exactly this window (tight timing loop between the
  `git status --porcelain` probe and the executor's act) is a natural round 3 candidate.
  Round 2 judged closing it not worth this round's budget — round 3 could at least try harder to
  make it observable, even if the fix itself stays deferred pending an operator call.
- **Whatever round 2's OWN fixer report ends up flagging as deferred** (if anything — check once
  it's done) is worth folding in too.
- The orchestrator's own independent verification of round 2 (once it finishes) may surface its
  own candidates the same way it did after round 1 — fold those in as well before finalizing
  round 3's seed, same pattern as round 1 → round 2.

## CLOSED-AND-VERIFIED
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

## RESIDUAL currently seeded for round 2
**New finding, from the orchestrator's OWN independent verification pass (round 1 never found
this — it's exactly the kind of thing "never trust the round's own verdict" exists to catch):**
concurrent `unwire` racing something else that writes into `.lyx` can leave `.lyx` as a real,
populated directory instead of a junction, which `reconcile` then permanently refuses to
auto-heal.

- **Repro (independently reproduced by the orchestrator):** root-anchored hub, prime worktree, no
  `--subpath`. From the prime worktree:
  ```
  for i in 1 2 3 4; do
    ( lyx fabric unwire > race_unwire_$i.json 2>&1; lyx fabric reconcile > race_reconcile_$i.json 2>&1 ) &
  done; wait
  ```
  4× concurrent `(unwire; reconcile)` pairs racing each other on ONE hub — not `unwire` racing a
  separate dedicated process. `.lyx/logs/` ends up containing real trace-log files afterward
  (confirmed 3 existed), materializing `.lyx` as a real directory before the junction can be
  re-wired.
- **Exact error** on the next serial `reconcile`:
  `"error":"re-point junction: adopt <hub>/adv/.lyx into <hub>/adv-weft/.lyx: logs already exists
  at the weft target; an earlier adoption already ran — delete the warp-side copy at
  <hub>/adv/.lyx/logs and re-run \`lyx fabric reconcile\`"`. `lyx fabric pairs` self-diagnoses
  honestly: `"junction_healthy":false,"junction_reason":"warp .lyx is not a junction"` — not
  silently wrong, just stuck.
- **Root cause — UNCONFIRMED, round 2 must establish this itself, not assume it:** the working
  hypothesis is that some concurrent `lyx` invocation writes into `.lyx/logs/` during the window
  `unwire` has torn the junction down but `reconcile` hasn't re-wired it yet, and
  `seedLyxJunction`'s adoption logic then refuses to merge a real directory back into a junction.
  This was NOT verified against `internal/logger`'s actual write path (does a deployed binary log
  unconditionally, or only under `LYX_TRACE=1`/`testing.Testing()`? CONSTRAINTS.md's Live-Substrate
  Spawn Observability invariant only documents the `go test`-time gate) — round 2's first job on
  this finding is reading that write path before proposing a fix, not patching around a guessed
  mechanism.
- **Severity:** LOW-MEDIUM. No data/work lost, self-diagnoses honestly via `pairs`, has a stated
  manual remedy — but is permanently non-self-healing without operator intervention once it
  happens, even though the trigger (racing `unwire` against itself/reconcile) is a deliberately
  adversarial scenario, not a single realistic operator action.
- **Fix the right layer:** once the writer is confirmed, the real fix is almost certainly making
  the window unreachable (serialize `unwire`'s junction teardown against whatever writes `.lyx/logs`,
  or make the adoption logic in `seedLyxJunction` merge a same-shaped `logs` directory instead of
  refusing) rather than just improving the error message — though a clearer remedy message is
  still worth doing regardless, per "fix every finding including NITs".

## Primary emphasis for round 2 — the destruction chokepoint has never itself been through crucible
`internal/fabricengine/destroy.go` — the chokepoint consolidating ~28 destructive call sites
behind one gate (containment → ownership → dirtiness → force) — was built in slice 12, **after**
the prior 6-round adversarial campaign had already finished (see git log: `79a72a38` is the
campaign, `3184cd5a` slice 12 building the chokepoint comes after). It was direct engineering work
in response to that campaign's findings, never itself the target of an independent review+fix
round. Round 1 re-verified its properties (since the gitexec migration touched `destroy.go`
directly) and the orchestrator's own independent pass tried harder to break it — both came back
clean on the core chokepoint properties (containment/ownership/dirtiness/force ordering, allowlist
completeness), so this is NOT a residual to close. But per the operator's explicit instruction,
round 2 should make the chokepoint the PRIMARY adversarial target in its own right — not a
re-verification side-note — since two rounds finding it clean is good evidence but is not the same
as one round having been assigned to genuinely try to break it as its main mission with a full
round's worth of adversarial budget (concurrent destructive races beyond what's been tried,
symlink/junction trickery, TOCTOU windows between a check and its executor).

## DEFERRED list
Empty.

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
1. Rewrite `_mill/fabric-review-prompt.md`'s "Round context seeded" section: residual = the
   unwire/.lyx race above (with the explicit instruction to confirm the write-path mechanism
   before fixing it), High-yield focus re-ordered so the destruction chokepoint is PRIMARY, the
   CLOSED-AND-VERIFIED list from this file so F1-F7 and the three migration commits aren't
   re-litigated. Commit the re-seed.
2. Spawn round 2: `subagent_type: crucible-reviewer-high`, `model: opus`, tag `opus-high-r2`.
3. Per the operator's 4-round plan: round 3 is Fable High, round 4 is Opus High (final safety
   pass, hard cap regardless of residual state — state limits honestly in the convergence verdict
   if anything is still open after round 4, per README's "state the limits" rule).
