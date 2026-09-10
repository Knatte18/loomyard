# loom crucible campaign — handoff note

Campaign, thread A: confirm live that `centralize-glyph-shape-enum` and
`quarry-bump-v0-2-0-status-helpers` are genuinely behavior-preserving when driven through loom's
real built binary. **CONVERGED (rounds 1-2).**
Campaign, thread B/C: fix two pre-existing smoke-test failures, then independently review that fix
work plus a wider adversarial pass over loom's bootstrap/crash-recovery machinery. **STILL NOT
CONVERGED after round 6; round 7 IN FLIGHT.** See `_mill/loom-crucible-orchestrator-kickoff.md` for
the original (thread-A-only) brief.

## Current state
**Thread A: CONVERGED**, unchanged since round 2, re-confirmed by round 6's light-touch pass.
**Thread B/C: NOT CONVERGED.** Four consecutive rounds (3, 4, 5, 6) each found real defects, five of
them (r4-r6) sharing one recurring shape — a negative/terminal classification answering a question
using a proxy fact instead of the fact that actually owns the answer.

**After round 6, the operator asked whether this recurring shape warrants its own centralization
task, the way `centralize-glyph-shape-enum` (this campaign's original subject) centralized ~12
hand-rolled ref-shape checks into one AST-enforced registry.** A dedicated, skeptical investigation
(a fresh general-purpose agent, NOT a crucible round — deliberately, so it wouldn't just agree with
the orchestrator's framing) was spawned to answer this on evidence. Its independent conclusion,
written to `_mill/task-proposal-completion-signal-centralization.md` (read this file for full
reasoning — it is NOT a crucible round artifact and doesn't match the clean-room constraint's
exclusion pattern): **partial — the `shape.go` ledger mechanism doesn't transplant here** (no closed
value enum to build a flat map/AST-diff over; the five sites differ in return type, caller identity,
and guarding precondition in ways that would force an awkward shared checkpoint). It recommended a
lightweight alternative instead of a full mill task: a named "Completion Signal Invariant" doc
comment, a `CONSTRAINTS.md` paragraph, and a sabotage-proved tripwire test pinning the known-audited
negative-verdict-return-site count — estimated half a day, no new abstraction, no new package.

**Round 7 (`opus-high-r7`) was seeded with exactly that as its assigned residual, plus continued
light thread-C vigilance if time allowed. It is CURRENTLY RUNNING — Job 1 (review) is committed and
complete; Job 2 (fix) was in progress as of this handoff's last refresh.** Do NOT act on round 7's
findings until its own completion notification arrives AND the orchestrator has independently
verified it (file-scope diff, cold-state hermetic gates, sabotage-proof of every new/changed test) —
this handoff deliberately does not detail round 7's in-flight findings, per the operator's own
instruction, precisely so a context reset doesn't cause premature action on unverified work. If you
are picking this campaign up fresh: check `git log --oneline` on this branch for commits after
`533c0a75c` (the round-7 re-seed) to see how far round 7 actually got, and read
`_mill/loom-review-opus5-high-r7.md`/`-fixer-report.md` (if the fixer report exists yet) directly
rather than trusting this paragraph's staleness.

## CLOSED-AND-VERIFIED

### Thread A (rounds 1-2) — unchanged; re-confirmed by round 6's live `validate-plan` driving
(canonicalization, Create inversion, ambiguous, both not-found branches, both containment tiers,
rename-to gate, handle collision, infra-error disposition — no regression).

### Thread B, the three original production fixes (commit range `c0adce527..69886823e`)
`d0e5a0e7b`, `aba2c270a`, `69886823e` — independently re-verified correct by rounds 3, 4, 5, 6, and
the orchestrator each time.

### Round 3 (`sonnet5-xhigh-r3`) — closed the `Started`-gating coverage gap; documented the
`AddStrand`/`run.json` crash-mid-registration race as an accepted residual.

### Round 4 (`opus5-high-r4`) — found instances 1-2 of the recurring shape: `wait.go`'s two
deadline-expiry paths. Fixed via `classifyDeadlineExpiry`.

### Round 5 (`fable5-high-r5`) — found instances 3-4: `wait.go`'s two mechanism-failure caps. Fixed
via `finishedDespiteMechanismFailure`. Also fixed a separate MEDIUM (malformed-JSON status file
still refusing `lyx loom run` at the Seed step) and a NIT (doc mis-attribution).

### Round 6 (`fable5-xhigh-r6`, commit range `0ea41c1ae..b97eec1db`) — found instance 5, the first
OUTSIDE `wait.go`:
- **F1 (MEDIUM)** — `internal/shuttleengine/attach.go`'s `dispositionCandidate` classified a
  `run.json` still at `outcome:running` with every declared output file already on disk as
  `verdictRespawnEligible` whenever its pane was dead/untracked — the entry-side twin of the four
  `Wait`-side defects. A driver that crashed AFTER the agent finished its work had the finished
  output files archived and the whole (expensive) LLM step re-run on the next resume. **Reproduced
  live**: staged a finished Discussion-Write in a real fabric hub with a dead strand — before the
  fix, `lyx loom drive` archived the sentinel files and respawned; after, history gained
  `Discussion-Write:done` and the machine advanced. Fixed by classifying an
  `outcome==running && allOutputFilesExist` candidate as attachable before the liveness dispatch, so
  the reconstructed run's own (already-hardened) `Wait` harvests it. Proven safe against the
  crash-versus-bounce trap: a bounce leaves no `running` run.json (a completed run's `finalize`
  already removed its directory), so the new guard is unreachable from that path.
- Round 6 also re-confirmed all four `Wait`-side fixes (rounds 4-5), the `Started`-gating and
  `VerifySeedOwnership`/`CheckSeed` disposition-sharing claims (round 3), and the
  `AddStrand`/`run.json` "Accepted residual" judgment (rounds 3-5) with no new angle — three-round
  agreement now on that residual staying documented rather than code-fixed.
- Also tripped over round 5's known out-of-scope fabric weft-branch-naming bug again while building
  its hub fixture; worked around it (not loom's job to fix).

**Orchestrator's independent verification of round 6**: file-scope diff matched exactly (`attach.go`,
`attach_test.go`, `loom.md`), cold-state hermetic gates green repo-wide, **the new regression test
independently sabotage-proved**: reverted the new top-of-function guard in `dispositionCandidate` —
all four subtests of `TestAttach_RunningRecordSatisfiedFileContract_HarvestsNotRespawn` failed
exactly as claimed, while the negative-control test
(`TestAttach_RunningRecordUnsatisfiedFileContract_RespawnsOrErrors`) correctly stayed green
throughout (proving the new test isn't vacuously failing/passing) — restored to an empty diff. Smoke
suite 12/12 green (the fixer report said "13", a minor counting typo in its own text, not a
correctness issue — the actual run and content are what were verified).

## Incidental finding, OUT OF loom's scope, not fixed — still outstanding
Fabric's `lyx fabric clone` names the weft primary branch after the weft bare repo's own HEAD rather
than the warp's primary branch name when they differ (originally hit in round 5, hit again in round
6). Not a loom finding. Worth a separate ticket through the normal mill flow if the operator wants it
tracked — nobody has opened one yet.

## RESIDUAL currently seeded
None specific from round 6 (its one finding is fixed). See "Current state" above for the
recommended strategy shift for round 7 — a sweep rather than another ad-hoc adversarial pass.

## DEFERRED list
- The `AddStrand`/`run.json` crash-mid-registration race — operator-decision item, now agreed
  unclosable-by-reordering across THREE independent rounds (3, 4, 5; round 6 re-confirmed with no
  new angle — four rounds total). Detection tradeoff fully documented in `manifest/designs/loom.md`.
- The fabric weft-branch-naming bug — not loom's scope.

## Next action
**Wait for round 7's completion notification, then independently verify it** (file-scope diff
against commit `533c0a75c`, cold-state hermetic gates, sabotage-proof of every new/changed test,
smoke suite) before treating any of its findings as settled. Do not touch the module's code or
`git add`/commit anything while it is still running (Hard Rule 3).
Once round 7 is verified: update this handoff with its actual (verified) findings, decide whether
thread B/C is converged or needs another round (per the standing rule below), and get the operator's
model+effort pick for any further round.
**Do not call thread B/C converged until a round with no assigned residual comes back with nothing
new.**
