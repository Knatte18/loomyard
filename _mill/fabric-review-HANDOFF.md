# fabric crucible campaign — HANDOFF

Orchestrator's own state file. Refreshed after every round's verification. Never read by a round
agent (clean-room constraint — this file matches the banned `<module>-review-*` glob).

## Right now — CAMPAIGN COMPLETE
Round 8 (`opus-medium-r8`), the operator's confirmed final round, has FINISHED and been
independently verified (fork `a05815de72264fe6c`). **The full convergence verdict is written to
`_mill/fabric-crucible-CONVERGENCE.md` (committed) — read that file for the campaign's complete
final state.** Summary: round 8's general unprimed sweep (no seeded target) found one genuinely
new defect class the prior 7 rounds' targeted chases had missed — `removeLaunchers` (delete side)
was never migrated to the TOCTOU-safe `removeContainedPath` machinery round 3 built, plus 3 LOW
findings (a deterministic out-of-container sweep bug, a swallowed-refusal dishonest-success bug in
the same shape as M2, and an Args-validation gap). All 4 fixed. Independent verification did what
round 8 itself could not — won M1's live race (250/3000 ≈ 8.3% pre-fix escapes, 0/3000 post-fix,
own in-process harness) — upgrading the campaign's last uncertain fix to fully live-proven both
ways. Two allowlist entries (both in `junction.go`) were flagged as structurally similar to
findings that were wrong before in this campaign, but NOT confirmed as live defects — named
honestly as open candidates for a hypothetical future round, not closed as safe and not treated as
proven unsafe either. This campaign is now DONE per the operator's explicit final-round
instruction; do not spawn round 9 without a new explicit operator instruction to do so.

Round 7 (`fable-high-r7`) has FINISHED and been independently verified (fork `a896b8ed43fb2c180`).
**Clean result — first round in the whole campaign where independent verification found NO
counter-evidence and NO new sibling gap.** Round 7 ran the operator-scoped full write-side
containment audit and found two real defects: F1 `writeLaunchers` (the confirmed zero-race gap
that triggered this round) and a SECOND, previously-unknown-to-anyone gap, F2 `createPortal`
(a `_portals`-container symlink let a portal junction get planted outside the hub). Both fixed via
`os.Root`, both independently re-attacked live and held. F3 built a durable write-side guard test
(`TestNoUncontainedWrite_FabricengineProductionSource`, modeled on the delete-side
`destructiveguard_test.go`) — independent verification re-derived the write-primitive inventory
from scratch by grepping `internal/fabricengine/` independently and it matched the guard's
allowlist EXACTLY, no mismatch, nothing missed; also confirmed the guard's detection mechanism
actually works by injecting a throwaway uncontained write and watching it get caught. F4 (a
doc-accuracy correction on `createExclusiveDir`'s overstated guarantee) had its "not exploitable
at this call site" claim independently scrutinized — the highest-risk kind of claim given round
4's history of an identical-sounding claim being wrong — and this one held up: `DeriveWarpName`
structurally cannot inject a path separator, so the gap can only ever land on the leaf, never an
intermediate ancestor, at its sole call site.

**Independent verifier's own assessment: no concrete evidence surfaced to justify a mandatory
round 8.** The module appears genuinely MERGEABLE on the write-side containment property this
round targeted.

**Operator decision (2026-08-14): "Kjør en SISTE runde som tester generelle ting. Den bør ikke
finne mye. Opus medium."** Round 8 is the operator's explicitly stated LAST round of this
campaign — not "last configured," genuinely last this time. Model/effort: Opus/medium (a
deliberate step down from the high-effort rounds that chased specific hard-to-find TOCTOUs —
appropriate for a general confidence sweep, not a targeted hunt). No seeded residual — general
final pass, explicit operator expectation that it should not find much. Seed and spawn now.

**Round 6's fix (the good news):** rejected the seed's own hypothesis (relocate staging outside
the hub) after testing it — found that breaks `os.Root.Rename` across a mount boundary (EXDEV,
architecturally real per POSIX `rename(2)`, independently confirmed sound) and regresses a
different-UID posture (plausible by the permission-bit reasoning, not independently re-derived —
would need a genuine second-UID test environment). Instead kept round 5's two-level staging
design and added `stagedWorktreeContained` — `os.Root.Lstat`-based checks BOTH before and after
`os.Root.Rename`, failing closed (cleaning up any escape) on either check. Independent
verification mapped every gap in the timeline (git write → pre-check → rename → post-check →
cleanup → `git worktree repair` → return) and built its OWN inotify-based attack tool (not reusing
round 6's harness) — 70 live trials against the real deployed binary, **0 escapes, 0 false
success, 0 debris.** NIT-F3's reversal (round 5's F2 WARN-log test actually already sabotage-proves
itself, contrary to what round 5's verification found) was independently confirmed correct too.

**The new finding (the bad news) — round 7's likely seed, pending operator direction:**
`internal/fabricengine/launchers.go`'s `writeLaunchers` (the CREATE-side counterpart of
`removeLaunchers`, called on every `add`) writes to `<hub>/_launchers/<AnchorRel>/<slug>` via
plain `os.MkdirAll`+`os.WriteFile` — no `refuseUncontainedPath`, no `os.Root`, nothing. A static
symlink planted at that path BEFORE running `add` (no timing, no race, no observation) makes `add`
write `ide.sh`/`fabric-checkout.sh` OUTSIDE the hub while reporting `ok:true` — M3's exact
false-success shape, strictly EASIER to exploit than everything this campaign has spent five
rounds on, on a call path nobody has audited before.

**Operator decision (2026-08-14): round 7 = full write-side audit, Fable/high.** Not scoped to
`writeLaunchers` alone — the round must grep for every hub-relative write call site under
`internal/fabricengine` missing containment, fix all of them, not just the one the verification
happened to find. Seed and spawn round 7 now.

Verification also flagged a related, only
theoretical (not live-reproduced) residual: `add.go`'s later steps after `containedWorktreeAdd`
returns (`InstallPostCheckoutHook`, `createPortal`, `writeLaunchers`, `WireJunctionsWith`,
`wireBoardLink`) all trust the earlier containment check without re-verifying — exactly the design
pattern that produced the `writeLaunchers` bug. Verification's explicit recommendation: round 7
should NOT fix `writeLaunchers` in isolation — it should grep for every `os.MkdirAll`/
`os.WriteFile`/`os.Create` call site under `internal/fabricengine` writing to a hub-relative path
without going through `refuseUncontainedPath`/`os.Root`, given the pattern that every "fixed" gap
in this campaign has had an unaudited sibling.

Round 5 correctly reproduced and fixed the create-side gap seeded from round 4's verification
(`createGitWorktree`'s symlink-directed-write escape via `Topology.Add`) — but the FIX itself
(`containedWorktreeAdd`: git writes to a crypto-random `os.Root`-created staging path inside the
hub, then `os.Root.Rename`s it into place) only defends against a GUESSING adversary, not an
OBSERVING one. Independent verification built an inotify watcher on the staging parent directory,
caught the staging path's creation the instant it happened (before git had written anything into
it), planted a symlink there, and hit **8/8 (100%)** across two batches — MORE reliable than the
original bug's ~1.7%. This affects the shared helper, so F3's four "sibling" sites (which correctly
route through the same helper) inherit the same exposure — not per-site bugs, one shared-mechanism
bug. `os.Root.Rename`'s own claimed semantics (refuses a destination symlink) checked out fine
against Go 1.26 docs — the flaw is earlier, in the staging path's observability, not in the final
move. Round 5's own re-attack (1200 trials, 0 escapes) only re-tried the ORIGINAL attack shape
(toggle a symlink at the final target) — it never attacked the NEW surface its own fix introduced
(the staging directory's observable creation). Same category of blind spot as round 4's "carried
item 3" mistake, one level more subtle: round 5 DID run a timing attack, just not the right one.
This is round 6's seeded residual — see below.
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
**Round 8 (`opus-medium-r8`, the campaign's FINAL round) — FULLY closed, strongest evidence bar of
any round.** 1 MEDIUM (M1, `removeLaunchers` routed through `removeContainedPath` — the delete-side
sibling round 7's `writeLaunchers` fix never covered) + 3 LOW (L1 `pruneEmptyAncestors` deterministic
out-of-container sweep, L2 a swallowed-refusal dishonest-success bug — same shape as round 2's M2,
L3 Args-validation on every verb), 6 commits `696979e2`..`32e32cb8`. Independently verified (fork
`a05815de72264fe6c`): gates green cold; L1/L2/L3 sabotage-proven exactly as claimed (L1 confirmed
genuinely deterministic, not a race); M1's two "behavioral" tests don't actually sabotage-prove the
routing change (only the mechanical guard-allowlist test does — a precision correction to the fixer
report's framing, not a functional defect). **Independent verification WON M1's live race where
round 8 itself could not**: round 8 tried 60 CLI-subprocess trials and failed to hit it, honestly
reporting the fix as CONFIRMED-by-trace rather than live; independent verification built its own
in-process toggler and got 250/3000 (~8.3%) live escapes against the PRE-fix code, 0/3000 against
the POST-fix code — M1 is now the strongest-evidenced fix in the whole campaign, live-proven broken
AND live-proven closed. L2 re-attacked live in round 8's exact scenario, confirmed genuine; one
minor already-accepted-class edge case noted (empty-target absent-target short-circuit, round 2's
documented guard-strength limit, not a new live defect). L3 confirmed live. A targeted re-read of
both guards' allowlist reasoning (round 8's own suggested lens, since M1/L1 both came from exactly
this kind of stale reasoning) flagged two `junction.go` entries — `adoptDotLyxContent`/
`mergeAdoptionTree` (destructive guard) and `seedLyxJunction`'s "race-only, not statically
pre-plantable" claim (write guard) — as structurally matching the reasoning shape that was wrong
three separate times this campaign (M1, the create-side gap, the staging-observability gap). NOT
independently re-attacked (time-boxed) — named as open candidates for a hypothetical future round,
not confirmed defects. See `_mill/fabric-crucible-CONVERGENCE.md` for the full campaign writeup.

**Round 7 (`fable-high-r7`) — FULLY closed, first round with a clean independent verification (no
counter-evidence, no new sibling found).** 1 MEDIUM (F1, `writeLaunchers` routed through `os.Root`)
+ 3 LOW (F2 `createPortal`'s container-symlink gap, fixed the same way; F3 the new write-side
guard test + CONSTRAINTS.md invariant; F4 a doc-accuracy correction on `createExclusiveDir`), 6
commits `7e7583a8`..`6d600fc5`. Independently verified (fork `a896b8ed43fb2c180`): re-derived the
write-primitive inventory from scratch by independently grepping `internal/fabricengine/` —
matched the guard test's allowlist EXACTLY, nothing missed; sabotage-proved F1/F2's tests AND
confirmed the guard test's OWN detection mechanism works (injected a throwaway uncontained write,
watched it get caught); live-re-attacked F1 (leaf + container symlink) and F2 (container symlink,
plus independently confirmed the leaf vector was already refused by pre-existing fslink behavior)
against the real deployed binary — fails closed in every case, no regression to the happy path.
F4's "not exploitable at this call site" claim — the highest-risk kind of claim given round 4's
history of an identically-shaped claim being wrong — was independently scrutinized, not just
repeated: confirmed `DeriveWarpName` structurally cannot inject a path separator, so the gap can
only ever land on the leaf (already protected), never an intermediate ancestor, at its sole call
site. Independent verifier's own conclusion: no concrete evidence to justify a mandatory round 8.

**Round 6 (`fable-high-r6`) — FULLY closed, the create-side chain's fix finally holds.** 1 MEDIUM
(F1, `containedWorktreeAdd`'s pre/post fail-closed `stagedWorktreeContained` checks around
`os.Root.Rename`) + NIT-F2 (folded into F1) + NIT-F3 (reversed to "not a finding" — round 5's F2
WARN-log test already sabotage-proved itself, independently confirmed correct), 5 commits
`d58d61b8`..`4050c5af`. Independently verified (fork `a54217f6d5db7273a`, using an
independently-built inotify attack tool, not reusing round 6's own harness): full timeline mapped
(git write → pre-check → rename → post-check → cleanup → `git worktree repair` → return), 70 live
trials against the real deployed binary — 0 escapes, 0 false success, 0 debris. Round 6's
rejection of the seed's own hypothesis (relocate staging outside the hub) was independently
assessed as sound: the EXDEV/mount-boundary claim is architecturally real (POSIX `rename(2)`),
the different-UID regression claim is plausible by the permission-bit reasoning though not
independently re-derived (would need a second-UID test environment). **First sub-fix in the
M3→M1→create-side→staging-observability chain to survive a fully independent re-attack.**
One theoretical (not live-reproduced) residual noted: `add.go`'s steps AFTER
`containedWorktreeAdd` returns (`InstallPostCheckoutHook`, `createPortal`, `writeLaunchers`,
`WireJunctionsWith`, `wireBoardLink`) trust the earlier containment check without re-verifying —
the same design pattern that produced round 7's seeded residual below, worth remembering if a
future round is hunting for more siblings.

**Round 5 (`fable-high-r5`) — PARTIALLY closed, see round 6's residual below for what is NOT
closed.** 1 MEDIUM (F1) + 2 LOW (F2, F3) + 1 NIT (F4), 8 commits `6034464d`..`88fe81fb`.
Independently verified (fork `afd8fb60fc7bd525e`): gates green cold; F1's regression test
(`TestContainedWorktreeAdd_RefusesSymlinkedTarget`) and F4's (`TestAddRollback_...WarnsLog`)
both sabotage-proven genuinely. `os.Root.Rename`'s specific claimed semantics (refuses a
destination symlink) checked out against actual Go 1.26 docs — that part of the design is sound.
**But the overall F1 fix does NOT close the window — it substitutes one exposure for a worse
one.** The staging path's "crypto-random, so unguessable" defense only stops a GUESSING
adversary; independent verification built an inotify watcher on the staging parent, caught the
staging directory's creation the instant it happened, planted a symlink there, and hit 8/8 (100%)
— more reliable than the original bug. F2/F3 (the sibling `createExclusiveDir` fix and the four
newly-gated weft/board/reconcile sites) correctly route through the same shared helper, so they
inherit the identical exposure — not per-site bugs, one shared-mechanism bug. F2/F3 also have no
dedicated regression test of their own (minor, secondary test-architecture gap). Round 5's own
1200-trial re-attack only re-tried the ORIGINAL attack shape; it never attacked the NEW surface
its own fix introduced. **This is round 6's seeded residual, not closed work — see below.**

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

## RESIDUAL — none seeded for round 8 (operator's choice, general final sweep)
Round 7 closed the write-side audit residual and it is now CLOSED-AND-VERIFIED above. Round 8 is
the operator's confirmed, explicitly final round of this campaign — "Kjør en SISTE runde som
tester generelle ting. Den bør ikke finne mye. Opus medium." No seeded target; round 8's seed
(`_mill/fabric-review-prompt.md`) is a general confidence sweep with an explicit instruction to
report honestly rather than manufacture findings. Optional (not required) spot areas named in the
seed if round 8 wants somewhere specific to look: the `add.go` post-`containedWorktreeAdd` steps
never individually audited beyond `writeLaunchers`/`createPortal` (`InstallPostCheckoutHook`,
`WireJunctionsWith`, `wireBoardLink`).

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

## Exact next action — NONE, campaign complete
`_mill/fabric-crucible-CONVERGENCE.md` is the final deliverable, committed. If the operator asks
for further work on `fabric`, that is a NEW instruction, not a continuation of this campaign's
standing plan — do not assume round 9 is wanted just because the module could theoretically absorb
more scrutiny forever. If the operator does ask to continue, treat it the same as every scope
change in this campaign: confirm the seed/target explicitly (the two `junction.go` allowlist
entries named in CLOSED-AND-VERIFIED above are the most credible starting candidates) before
spawning.

If the operator says continue, rewrite `_mill/fabric-review-prompt.md` for round 8 using one of
the candidate directions in "RESIDUAL" above (or a direction the operator specifies), commit, spawn
`subagent_type: crucible-reviewer-high`, `model: fable` (or whatever model/effort the operator
specifies — do not assume Fable/high silently forever, confirm if it's been a while since it was
last explicitly stated), tag `fable-high-r8`.
