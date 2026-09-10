# loom crucible campaign — handoff note

Campaign, thread A: confirm live that `centralize-glyph-shape-enum` and
`quarry-bump-v0-2-0-status-helpers` are genuinely behavior-preserving when driven through loom's
real built binary. **CONVERGED (rounds 1-2).**
Campaign, thread B/C: fix two pre-existing smoke-test failures, then independently review that fix
work plus a wider adversarial pass over loom's bootstrap/crash-recovery machinery. **STILL NOT
CONVERGED after round 7.** See `_mill/loom-crucible-orchestrator-kickoff.md` for the original
(thread-A-only) brief.

## Current state
**Thread A: CONVERGED**, unchanged since round 2, re-confirmed by round 6's light-touch pass.
**Thread B/C: NOT CONVERGED.** Five consecutive rounds (3, 4, 5, 6, 7) each found real defects, six of
them (r4-r7) sharing one recurring shape — a negative/terminal classification answering a question
using a proxy fact instead of the fact that actually owns the answer.

**After round 6, the operator asked whether this recurring shape warrants its own centralization
task, the way `centralize-glyph-shape-enum` (this campaign's original subject) centralized ~12
hand-rolled ref-shape checks into one AST-enforced registry.** A dedicated, skeptical investigation
(a fresh general-purpose agent, NOT a crucible round — deliberately, so it wouldn't just agree with
the orchestrator's framing) was spawned to answer this on evidence. Its independent conclusion,
written to `_mill/task-proposal-completion-signal-centralization.md` (read this file for full
reasoning — it is NOT a crucible round artifact and doesn't match the clean-room constraint's
exclusion pattern): **partial — the `shape.go` ledger mechanism doesn't transplant here** (no closed
value enum to build a flat map/AST-diff over; the five sites known at the time differ in return type,
caller identity, and guarding precondition in ways that would force an awkward shared checkpoint). It
recommended a lightweight alternative instead of a full mill task: a named "Completion Signal
Invariant" doc comment, a `CONSTRAINTS.md` paragraph, and a sabotage-proved tripwire test pinning the
known-audited negative-verdict-return-site count. The operator agreed this is normal crucible work,
not a separate task — "well ett og slett bare vanlig Crucible."

**Round 7 (`opus5-high-r7`, commit range `533c0a75c..c62cd1061`) executed exactly that residual, and
in doing so found the SIXTH instance the investigation's own search had missed** — because that
search scoped itself to sites returning a *verdict*, and this instance returns an *error* instead.
Independently verified by the orchestrator (file-scope diff, cold-state `go build`/`go vet`/
`go test -count=5`/`go test ./...`, live smoke suite 13/13 green including the new F1 reproduction
test, F5's de-race fix confirmed stable over 5 consecutive runs, and — critically — sabotage-proofing
both F1's fix and the new tripwire test in **both** mutation directions (unguarded new exit; deleted
guard), each restored to a byte-for-byte empty diff). All findings below are orchestrator-verified,
not merely round-self-reported.

- **F1 (MEDIUM, the sixth instance, reproduced live)** — `Attach`'s three reed-state gates
  (`attach.go:73,82,93`, pre-fix line numbers) sit AHEAD of round 6's `dispositionCandidate` guard and
  abandoned every candidate — including a `runOutcomeRunning` record whose every declared output file
  was already on disk — the moment `reed`'s own strand table was unreadable, absent, or wouldn't
  answer `Status()`. Fixed via `soleFinishedCandidate` (new in `attach.go`), consulted by all three
  gates before they report refusal, gated on exactly one `runOutcomeRunning` candidate with a
  satisfied file contract (two such candidates correctly falls through to the original refusal — reed
  is precisely what would be needed to pick between them). Reproduced live against the real built
  binary: a `Discussion-Write` with both output files on disk and `reed.json` removed under it
  (sanctioned `git clean -xdf` of `.lyx`) hard-failed with `"no reed state file"` before the fix,
  harvested as `OutcomeDone` after. New smoke test:
  `TestSmokeSingleLLM_HarvestsAFinishedRunWithReedStateGone`.
- **R1 (the assigned residual, closed)** — named "Completion Signal Invariant" section added to
  `wait.go`'s package doc (cross-referenced from `attach.go`'s own doc comment); a `CONSTRAINTS.md`
  paragraph naming all six now-known instances; a two-assertion AST tripwire test
  (`completionsignal_enforcement_test.go`) pinning BOTH the negative-verdict-*return-site* count
  (primary — catches an unguarded new exit) and the `allOutputFilesExist` *call-site* count
  (secondary — catches a deleted guard). **One correction to the brief was load-bearing**: the
  original brief specified a call-site-count-only tripwire, which round 7 itself proved cannot catch
  its own F1 (three unguarded exits existed the whole time the call-site count read correct) — it
  redesigned the primary assertion around return sites instead. The orchestrator independently
  sabotage-proved both directions on both assertions: an added unguarded exit trips
  `NegativeVerdictReturnSites` (not `FileContractCallSites`); a deleted guard call trips
  `FileContractCallSites` (not `NegativeVerdictReturnSites`) — confirming the two assertions are
  independent, neither vacuous, and correctly non-overlapping.
- **F5 (MEDIUM, pre-existing, not part of the recurring shape)** — the live smoke suite's own
  `TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed` killed the driver at an arbitrary moment
  then asserted the follow-up drive re-entered the same row — a real ~1-in-3 race, not a loom routing
  bug. Fixed by adding `waitForCurrentProducer`, which polls the status file until the driver has
  actually reached the row the test means to kill it on, before killing it. Orchestrator ran this test
  `-count=5` after the fix: 5/5 green (previously flaky at roughly this rate per the round's own
  reproduction on the pre-round tree).
- **F2 (LOW)** / **F3, F4 (NIT)** — doc-comment accuracy fixes in `attach.go`/`doc.go` (a stale claim
  that `reed` repairs an unreadable `reed.json` when it explicitly refuses to; a package-doc `Attach`
  summary and file-doc comment predating round 6/7's file-contract-first guard). Reviewed, consistent
  with the current code.

Full detail: `_mill/loom-review-opus5-high-r7.md` (review) and
`_mill/loom-review-opus5-high-r7-fixer-report.md` (fixer report).

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

### Round 7 (`opus5-high-r7`, commit range `533c0a75c..c62cd1061`) — found instance 6, closed the
assigned residual (R1), plus F2 (LOW)/F3/F4 (NIT) doc fixes and F5 (MEDIUM, pre-existing smoke flake,
unrelated to the recurring shape). Full detail in "Current state" above — orchestrator-verified
including sabotage-proofing of F1's fix and both directions of R1's new tripwire test. One process
note: the orchestrator's own pending handoff edit (this file, mid-refresh at the time) was
inadvertently swept into round 7's own final commit (`c62cd1061`) rather than committed separately by
the orchestrator in a clean tree — the exact Hard Rule 3 hazard the method warns about, materialized
for real this campaign. No content damage (the swept-in text was accurate and is superseded by this
same refresh), but a reminder that the hazard is real, not theoretical, and worth restating to future
round agents: commit only your own round's files, never a broad `git add -A`.

## Incidental finding, OUT OF loom's scope, not fixed — still outstanding
Fabric's `lyx fabric clone` names the weft primary branch after the weft bare repo's own HEAD rather
than the warp's primary branch name when they differ (originally hit in round 5, hit again in round
6). Not a loom finding. Worth a separate ticket through the normal mill flow if the operator wants it
tracked — nobody has opened one yet.

## RESIDUAL currently seeded
None (round 7's assigned residual, R1, is closed and verified; its one new finding, F1, is also
fixed). Round 8 needs a fresh operator decision on strategy — see "Next action".

## DEFERRED list
- The `AddStrand`/`run.json` crash-mid-registration race — operator-decision item, now agreed
  unclosable-by-reordering across THREE independent rounds (3, 4, 5; round 6 re-confirmed with no
  new angle — four rounds total). Detection tradeoff fully documented in `manifest/designs/loom.md`.
- The fabric weft-branch-naming bug — not loom's scope.

## Next action
Round 7 is independently verified (see "Current state") and does NOT qualify as convergence — it
both had an assigned residual (R1) and found a new sixth instance (F1). Get the operator's
model+effort pick for round 8. Six instances of the recurring shape across four rounds (r4-r7) is a
strong signal per `crucible/README.md`'s own fabric-campaign refinement ("when the tail starts
circling, stop reviewing and start counting") that another ad-hoc adversarial pass may keep finding
one-at-a-time instances indefinitely — worth raising with the operator again now that a sixth has
turned up in a residual-execution round rather than an adversarial one, though the tripwire test
(R1) is specifically meant to make a seventh instance impossible rather than merely findable, so the
open question is now "does the tripwire actually close this off" rather than "where is instance 7".
**Do not call thread B/C converged until a round with no assigned residual comes back with nothing
new.**
