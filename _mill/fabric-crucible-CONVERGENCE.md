# `fabric` — crucible campaign convergence verdict

8 rounds, 2026-08-14, orchestrated in worktree `fabric-crucible-hardening`.
Every round independently verified from a cold state by the orchestrator (a fresh fork with no
access to the round's own reasoning) before the next round was seeded — no round's self-report was
ever taken on trust.
This file is the campaign's closing statement: what is fixed and how strongly, what remains open,
and what the process itself demonstrated.

## Why this campaign happened

`fabric`'s destruction chokepoint (`internal/fabricengine/destroy.go`) was built in slice 12, as
direct engineering work responding to fabric's *original* 6-round crucible campaign — which had
itself concluded the module needed a dedicated destructive-operation gate, and was interrupted
mid-campaign so that gate could be built instead of continuing review rounds.
That means the chokepoint had never itself been the subject of an independent crucible review —
it was the *product* of a prior campaign's findings, never the *target* of one.
This campaign started specifically to close that gap, and the chokepoint's containment guarantees
(does a gated operation ever act outside the boundary it claims to respect?) became its central
thread across all 8 rounds.

## Round-by-round

| Round | Model/effort | Scope | Findings | Verdict |
|---|---|---|---|---|
| 1 | Opus/medium | Broad review of the recent gitexec migration + fixture inversion | 7 (0 BLOCKING/MEDIUM, 3 LOW, 4 NIT) — doc/message drift | All fixed, independently confirmed |
| 2 | Opus/high | Chokepoint as primary target, first time | 12 (0 BLOCKING, 3 MEDIUM, 4 LOW, 5 NIT) — **M3: containment never resolved symlinks** | Fixed; independent verification found a follow-on TOCTOU M3's own fix left open |
| 3 | Fable/high | The TOCTOU independent verification found after round 2 | 1 MEDIUM (M1) — check-then-act gap in M3's fix | Fixed via `os.Root` (Go 1.26); first fix to survive one round of independent re-attack, but not yet the create-side |
| 4 | Fable/high | Chokepoint graduated to spot-check; broad module sweep | 4 LOW + 1 NIT | Fixed; but independent verification found round 4 wrongly cleared a create-side symlink-write gap as "not a defect" |
| 5 | Fable/high | The create-side gap round 4 missed | 1 MEDIUM + 2 LOW + 1 NIT | Fixed via a crypto-random staging path + `os.Root.Rename`; independent verification found the fix defended only against a *guessing* adversary, not an *observing* one (inotify attack, 8/8 escapes) |
| 6 | Fable/high | The staging-path observability gap | 1 MEDIUM (+ 2 NIT, one reversed) | Fixed via pre/post fail-closed checks bound to the act; **first sub-fix in this whole chain to survive fully independent re-attack** (70 live trials, independently-built tool, 0 escapes) |
| 7 | Fable/high | Full write-side containment audit (operator-scoped, after verification found `writeLaunchers` had zero protection) | 1 MEDIUM + 3 LOW | Fixed (`writeLaunchers` + a newly-found sibling, `createPortal`); built a permanent write-side guard test; **first round with a fully clean independent verification — no counter-evidence, no new sibling** |
| 8 | Opus/medium | General final sweep, no seeded target, operator's explicit last round | 1 MEDIUM + 3 LOW | Fixed (`removeLaunchers` — the delete-side sibling nobody had migrated); independent verification won the one live race round 8 itself could not |

## The central story: containment, defeated and re-fixed five times before it held

The chokepoint's core promise, in its own words: *"a containment failure... can never be
overridden by a flag."*
Across rounds 2 through 8, that promise was tested and found wanting, repeatedly, in a chain where
each fix closed one door and independent verification found another door the fix itself had left
open, or a sibling door nobody had checked:

1. **M3 (round 2):** the containment check never resolved symlinks at all. A symlink planted at an
   intermediate path segment let `remove --force` delete files outside the hub while reporting
   success.
2. **M1 (round 3):** M3's fix resolved symlinks, but at a different instant than the actual
   deletion — a check-then-act window. Closed via `os.Root`, making resolution and the act one
   atomic operation.
3. **Create-side gap (round 4→5):** the delete side was now sound, but the *create* side
   (`createGitWorktree`/`createExclusiveDir`, reached via `add`) had an analogous, wider window.
   Round 4 investigated this and wrongly concluded it was safe. Round 5 found and fixed it — with a
   crypto-random staging path.
4. **Staging-observability gap (round 5→6):** "crypto-random" only defeats an adversary who has to
   *guess*. An adversary who can *watch* the hub (which every adversary in this threat model
   already can, by definition) can observe the staging path's creation via `inotify` and win 100%
   of the time — worse than the bug it replaced. Round 6 closed this properly: fail-closed checks
   bound to the act, not secrecy of a path inside a directory the adversary already controls.
5. **Write-side siblings (round 6→7):** independent verification, sweeping beyond its assigned
   scope, found `writeLaunchers` — a *completely separate* call site on the same `add` path — had
   *no containment protection whatsoever*, exploitable with a static symlink and no timing attack
   at all. Round 7 fixed it, found a second sibling (`createPortal`) the same way, and built the
   write-side's first-ever guard test so this class of gap gets caught automatically going forward.
6. **Delete-side sibling (round 7→8):** the pattern repeated one more time, in the opposite
   direction — `removeLaunchers` (delete side) had never been migrated to the `os.Root` machinery
   that fixed `removePath`/`removeLink` back in round 3. Round 8's unprimed general sweep found it;
   independent verification then won the live race round 8 itself could not (250/3000 ≈ 8.3%
   escapes pre-fix, 0/3000 post-fix), giving this last fix the strongest evidence bar of the whole
   campaign.

**What finally held:** round 6's delete-side fix and round 7's write-side fixes are the only two
links in this chain that survived a FULLY independent re-attack — built with a fresh tool, not
reusing the round's own harness — with zero counter-evidence. Round 8's fix for the delete-side
sibling has since joined them, live-proven both broken and fixed by independent verification.

## What is now closed, and how strongly

- **Delete-side containment** (`removePath`, `removeLink`, `removeLaunchers`'s directory removal)
  — all route through `removeContainedPath`/`os.Root`. Survived independent re-attack with real
  live trial volume in rounds 3, 6, and 8 (the last with a won live race at ~8.3% pre-fix hit
  rate).
- **Create-side containment** (`createGitWorktree`, `createExclusiveDir`, and the four
  weft/board/reconcile sites routing through the same helper) — via `containedWorktreeAdd`'s
  pre/post fail-closed checks. Survived a fully independent re-attack in round 6 (70 live trials,
  0 escapes).
- **Write-side containment** (`writeLaunchers`, `createPortal`) — via `os.Root`. Survived
  independent live re-attack in round 7, plus an independently re-derived write-primitive
  inventory that matched the new guard test's allowlist exactly.
- **Two permanent machine guards now exist that did not exist before this campaign:**
  `destructiveguard_test.go` (delete-side raw-primitive allowlist, pre-existing but re-verified
  repeatedly) and `TestNoUncontainedWrite_FabricengineProductionSource` (write-side, built in
  round 7). Between them, any future raw filesystem primitive added to `internal/fabricengine`
  outside the containment helpers now fails a test automatically — this is arguably the single
  most durable outcome of the whole campaign, independent of any individual fix.
- **Ownership predicates, `createdToken` unforgeability, `--force`-answers-dirtiness-only, and
  concurrent-race combinations** (4× concurrent `remove --force`, `remove` vs `reconcile`, `prune`
  vs `add`) — re-derived and re-driven across rounds 2 and beyond, held throughout.
- **The gitexec migration, fixture-dependency inversion, and `t.Parallel` unblock** — the three
  commits that motivated this campaign's start — drove live in round 1, no shape-of-migration
  defect found, only doc drift (fixed).

## What remains open — stated honestly, not glossed over

- **Windows path/junction behavior.** Permanent, never-executed gap on every round of this
  campaign — the host was Linux throughout. `os.Root`'s Windows junction semantics are a stdlib
  contract nobody here ran. This was true from round 1 and is still true now.
- **N4's dirtiness-probe TOCTOU** (`checkPathDirtiness`'s `git status --porcelain` probe is a
  check, the executor's act happens later — a theoretical check-then-act gap). Two independent
  attempts at a live repro failed (round 2, and the orchestrator's own verification of round 2);
  round 3 traced the reachable paths and found they're all either pre-checked or bypass the probe
  via `force`; round 4's independent verification read that reasoning and found no weak link.
  Settled as real-in-theory, no-demonstrated-exploit, documented in `destroy.go`'s own header.
- **Two `junction.go` allowlist entries, flagged but NOT confirmed as live defects**, found during
  round 8's independent verification while re-reading both guards' allowlist reasoning through the
  lens round 8 itself suggested (an entry that explains *why* a raw primitive is safe is worth
  re-reading as a finding candidate, not settled reasoning — exactly the pattern that produced
  M1 and L1 in round 8):
  - `adoptDotLyxContent`/`mergeAdoptionTree` (destructive guard): raw, nominal `os.Remove` on
    `.lyx`-junction-adoption paths, justified only by "the OS refuses on a non-empty directory" —
    the same blast-radius-limiting-≠-containment reasoning shape that was insufficient for M1 and
    L1 before their fixes.
  - `seedLyxJunction`'s `os.MkdirAll` (write guard): justified as "race-only, not statically
    pre-plantable" — the same reasoning shape round 4 used to wrongly clear the create-side gap.
  Neither was independently re-attacked (time-boxed at the end of an 8-round campaign, not a
  capability limit) — these are the most credible starting candidates if the operator chooses to
  continue this campaign at some future point, not proven-unsafe code today.
- **Windows aside, everything else this campaign specifically targeted is closed with live
  evidence, not merely reasoned about.**

## What the process itself demonstrated

Every round from 1 through 7 had *something* corrected by independent verification — a missed
defect, a fix that didn't close what it claimed to, an overstated test-coverage claim, or a wrongly
cleared "not a defect" conclusion. Round 8 is the first (and last) round where independent
verification *strengthened* the round's own result rather than correcting it. That pattern — seven
consecutive rounds each catching something the previous round's own confidence missed — is not
evidence the module is now perfect. It is strong evidence that the specific properties this
campaign hammered hardest (the chokepoint's containment guarantees, in both directions, plus the
write-side call sites nobody had audited before) are now sound, verified by a method that has
repeatedly proven able to detect when they were not.

The chain also shows something about audits versus point-fixes: every reactive point-fix in this
campaign (rounds 3, 5, 6, 8's M1) closed exactly the gap it targeted and nothing more — leaving a
sibling gap for the *next* round to find by accident. The one deliberately broad audit (round 7,
explicitly scoped that way after the pattern had repeated twice) found not only its assigned target
but a second, previously-unknown sibling in the same pass, and left behind the two guard tests that
are now the campaign's most durable asset. Round 8's own unprimed general sweep then repeated that
lesson once more, from the opposite direction — finding the one remaining sibling audits number 2
(round 7) had not been scoped to look for.

## Merge-readiness

**MERGEABLE**, with the limits stated above carried forward honestly rather than claimed closed.
