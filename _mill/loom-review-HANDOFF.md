# loom crucible campaign — handoff note

Campaign, thread A: confirm live that `centralize-glyph-shape-enum` and
`quarry-bump-v0-2-0-status-helpers` are genuinely behavior-preserving when driven through loom's
real built binary. **CONVERGED (rounds 1-2).**
Campaign, thread B/C: fix two pre-existing smoke-test failures, then independently review that fix
work plus a wider adversarial pass over loom's bootstrap/crash-recovery machinery. **STILL NOT
CONVERGED after round 8.** See `_mill/loom-crucible-orchestrator-kickoff.md` for the original
(thread-A-only) brief.

## Current state
**Thread A: CONVERGED**, unchanged since round 2, re-confirmed by round 6's light-touch pass and
round 8's live spot-checks (Create-inversion both directions, `glyph-not-found`, a live handle
canonicalization rewrite proven on disk).
**Thread B/C: NOT CONVERGED.** Six consecutive rounds (3, 4, 5, 6, 7, 8) each found real defects. Six
of them (r4-r7) shared one recurring shape (a negative/terminal classification answering a question
using a proxy fact instead of the fact that actually owns the answer) — now closed AND structurally
tripwired (round 7's R1). **Round 8 was the first genuinely open, no-assigned-residual round, and it
still found two new, DIFFERENT-shaped defects** — so the standing convergence rule (a no-residual
round with nothing new) is still not met.

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

### Round 8 (`sonnet5-xhigh-r8`, commit range `45447a39b..4d4f3fe28`) — first genuine no-residual
round, deliberately steered away from `wait.go`/`attach.go` toward `run.go`'s `Start`/`finalize`,
`internal/loomengine`, `internal/loomcli`, `internal/loomshed`. Found TWO new, genuinely
DIFFERENT-shaped defects (neither is a seventh instance of the recurring completion-signal shape):

- **F1 (MEDIUM, found by full code trace, NOT live — disposed as a documented "Accepted residual",
  same pattern as the `AddStrand`/`run.json` one)** — a run classified `OutcomeDone` whose spec sets
  `ForkSubagents` and whose `AuditForks` call then fails (`finalize`, `wait.go:568-579`) leaves its
  strand and run directory alive forever: `run.state.Outcome` is already persisted to the terminal
  `"done"` sentinel BEFORE the audit runs (line 563-566), so the record can never again read
  `runOutcomeRunning` — the one value `dispositionCandidate` requires to consider a candidate
  attachable — and the strand is never removed from reed (the `cleaned` block that would do so sits
  AFTER the audit's early-return), so it never goes absent from reed's live set either, which is what
  `sweepOrphansOpportunistic` requires to sweep a directory. **Orchestrator independently traced all
  three claims directly in the code** (`wait.go:554-589`, `attach.go:381,388`,
  `run.go:372-387`) and confirms the finding is accurate: this really is a permanent leak for
  `burlerengine`'s cluster-fan rounds, with `websterengine`'s Master row escaping only by the
  accident of its own unconditional entry-time strand reclaim (which itself silently redoes
  already-finished work rather than reclaiming it cleanly). Genuinely a design decision, not a code
  fix: closing it needs a persisted reason field distinguishing an `AuditForks`-preserved run (meant
  to be reclaimed once diagnosed) from a `KeepPane`-preserved one (meant to stay alive indefinitely).
  Documented in `manifest/designs/loom.md`'s crash-recovery section as a third named "Accepted
  residual", matching the section's existing convention. Not live-reproduced (constructing a real
  `AuditForks` failure needs a genuine Claude Code fork-transcript layout, correctly judged out of
  this round's live-driving budget) — the orchestrator accepts the code-trace-only disposition as
  proportionate, the same standard the `AddStrand`/`run.json` residual was held to.
- **F2 (MEDIUM, code-fixed, orchestrator-verified)** — `validate-discussion`/`validate-plan` (the
  writer agents' own stencil-mandated pre-handoff self-checks) ran through the FULL eight-config
  `wire()` instead of the lightweight path `status`/`pause` already use for the identical,
  previously-live-observed hazard (a sibling process's config transiently broken mid-run). Fixed by
  extending the existing lightweight-wiring mechanism
  (`wireStatusPathsOnly`→`wireLightweight`, `verbReadsStatusOnly`→`verbUsesLightweightWiring`) to
  cover both verbs, filling exactly the four `c.env` path fields (`AnchorPath`, `WorktreeRoot`,
  `DecisionRecordPath`, `SupportLogPath`) those two verbs' own code actually reads — orchestrator
  confirmed via grep that `validate.go`'s `validateDiscussionCmd`/`validatePlanCmd` read no other
  `c.env` field. **Orchestrator independently sabotage-proved both regression tests, each in
  isolation**: reverting the `cli.go` switch-case addition fails `TestVerbUsesLightweightWiring`'s
  two new subtests exactly, with `Status`/`Pause`/`Run`/`Drive`/`UnknownVerb` staying green throughout
  (not vacuous); reverting the four-field fill in `wireLightweight` fails all four of
  `TestWireLightweight_FillsThePathsWithoutLoadingAnyConfig`'s new assertions exactly. Both restored
  to a byte-for-byte empty diff.

**Orchestrator's independent verification of round 8**: file-scope diff matched the round's own
report exactly (`cli.go`, `wiring.go`, `wiring_test.go`, `wiring_commitstatus_test.go`,
`manifest/designs/loom.md`, plus the two report files). Cold-state `go build`/`go vet`/full
`go test ./...` all green; live smoke suite 13/13 green (round 8 added no new smoke test — F2 is
unit-level, F1 is documentation-only, consistent with their claimed shapes). Both regression tests
sabotage-proved as described above.

## Incidental finding, OUT OF loom's scope, not fixed — still outstanding
Fabric's `lyx fabric clone` names the weft primary branch after the weft bare repo's own HEAD rather
than the warp's primary branch name when they differ (originally hit in round 5, hit again in round
6). Not a loom finding. Worth a separate ticket through the normal mill flow if the operator wants it
tracked — nobody has opened one yet.

## RESIDUAL currently seeded
None. Round 9 needs a fresh operator decision on strategy — see "Next action".

## DEFERRED list
- The `AddStrand`/`run.json` crash-mid-registration race — operator-decision item, now agreed
  unclosable-by-reordering across THREE independent rounds (3, 4, 5; round 6 re-confirmed with no
  new angle — four rounds total; round 8 did not touch it). Detection tradeoff fully documented in
  `manifest/designs/loom.md`.
- The `AuditForks`-failure orphan (round 8's F1) — operator-decision item, found by code trace, not
  live. Needs a persisted "reason this run is kept alive" field to distinguish an
  `AuditForks`-preserved run from a `KeepPane`-preserved one before it can be closed safely.
  Documented in `manifest/designs/loom.md` as a third named "Accepted residual". Only one round's
  worth of scrutiny so far (unlike the AddStrand/run.json residual's four), so worth an independent
  second look by a future round rather than treating it as settled this early.
- The fabric weft-branch-naming bug — not loom's scope.

## Next action
Round 8 is independently verified (see "Current state" and the round-8 entry above) and does NOT
qualify as convergence — it was the first genuinely open, no-assigned-residual round, and it still
found two new, differently-shaped defects (F1, F2). Get the operator's model+effort pick for round 9.

Worth naming explicitly: round 8 is genuine evidence in FAVOR of the tripwire strategy working as
intended — a full round of unconstrained adversarial reading, deliberately pointed away from
`wait.go`/`attach.go`, found zero instances of the recurring completion-signal shape and two
unrelated defects elsewhere instead. That is closer to what convergence looks like than any prior
round, even though it does not itself qualify (the bar is nothing new, not nothing-of-the-old-shape).
Round 9 should probably be another genuinely open no-residual pass, continuing to widen away from
`wait.go`/`attach.go` (now six-rounds-read and tripwired) and from `internal/loomcli/wiring.go`/
`cli.go` (round 8's F2 area, now fixed and tested) toward whatever surface has had the least
clean-room attention so far — raise this with the operator rather than assuming it.
**Do not call thread B/C converged until a round with no assigned residual comes back with nothing
new.**
