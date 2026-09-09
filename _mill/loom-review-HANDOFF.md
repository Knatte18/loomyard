# loom crucible campaign — handoff note

Campaign, thread A: confirm live that `centralize-glyph-shape-enum` and
`quarry-bump-v0-2-0-status-helpers` are genuinely behavior-preserving when driven through loom's
real built binary. **CONVERGED (rounds 1-2).**
Campaign, thread B/C: fix two pre-existing smoke-test failures, then independently review that fix
work plus a wider adversarial pass over loom's bootstrap/crash-recovery machinery. **STILL NOT
CONVERGED after round 6** — four consecutive rounds (3, 4, 5, 6) have each found real defects. See
`_mill/loom-crucible-orchestrator-kickoff.md` for the original (thread-A-only) brief.

## Current state
**Thread A: CONVERGED**, unchanged since round 2, re-confirmed by round 6's light-touch pass.
**Thread B/C: NOT CONVERGED.** The campaign's core defect class ("a terminal/negative classification
answers a question using only a proxy fact, ignoring the fact that actually owns the answer, one
line away") has now been found in FIVE call sites across three rounds: r4 (2, both in `Wait`'s
deadline paths), r5 (2, both in `Wait`'s mechanism-failure caps), r6 (1, in `Attach`'s
`dispositionCandidate` — the first instance OUTSIDE `Wait`). **Recommendation for round 7: switch
strategy from another broad adversarial pass to an exhaustive SWEEP** — per
`crucible/README.md`'s own fabric-campaign refinement ("when the tail starts circling, stop
reviewing and start counting"), five instances of one shape found by four rounds of ad-hoc adversarial
reading is exactly the signal to stop hoping a fifth round spots instance #6 live, and instead
enumerate EVERY place in the bootstrap/crash-recovery surface that finalizes a terminal/negative
outcome, in a table, with a reason for each row (checks the completion signal / doesn't need to /
genuinely doesn't and is a new finding). This was discussed with the operator; see whether it was
acted on before assuming another generic safety-pass round is still the right shape.

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
Discuss with the operator whether round 7 should be a sweep (enumerate every terminal/negative-outcome
call site across the bootstrap/crash-recovery surface — `Wait`, `Attach`, `Start`,
`internal/loomengine`, `internal/loomcli`, `internal/loomshed` — in a table, with a reason per row)
rather than another generic adversarial safety pass, per the fabric campaign's own "stop reviewing,
start counting" refinement. If the operator agrees, seed the review prompt accordingly and pick a
model — a sweep is well-suited to a systematic, lower-creativity task, so effort tier doesn't need to
be the highest available; if the operator prefers another ad-hoc pass instead, that's their call to
make, not a default to fall back to silently.
**Do not call thread B/C converged until a round with no assigned residual — sweep or ad-hoc —
comes back with nothing new.**
