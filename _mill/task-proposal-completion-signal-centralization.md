# Task proposal (investigation only) — should the "completion signal" defect class be centralized?

Independent investigation, requested by the operator after crucible rounds 4-6 found five instances
of what looked like one recurring defect shape in `internal/shuttleengine`.
This file is a standalone investigation deliverable, not a crucible round report — it does not
touch `_mill/loom-review-*` or `_mill/loom-review-HANDOFF.md`, and no code was changed to produce it.

## Verdict

**Partial — narrower than the `centralize-glyph-shape-enum` precedent, and not a fit for the same
mechanism.**

A real, one-shape defect class exists and was worth taking seriously (see Evidence).
But the ref-shape registry's mechanism — a shared map from a closed value enum to a per-site policy,
enforced by two AST-parsed sync meta-tests — does not transplant cleanly here, because the thing that
was actually missing in `internal/planparser` (a single shared source of truth for "what does this
`refKind` mean at this site") already existed here (`allOutputFilesExist`, present and correct since
before crucible round 4 ever started). The five bugs were never five divergent re-implementations of
"did this run finish" — they were five omissions to *call* the one existing, correct function before
returning a negative verdict.

That difference changes what "centralizing" should even mean:

- **Warranted**: a small, targeted regression safety net that makes it structurally harder for a
  *sixth* negative-outcome return to skip the check silently — see "If warranted" below for the
  concrete, deliberately narrow shape.
- **Not warranted**: a `shape.go`-style ledger/registry module, a new package, or a new mill task with
  its own design/plan step. The surface is two files, seven call sites, already fully closed by four
  independent effort-high/xhigh rounds, and I found no sixth instance on an independent read (see
  "6th instance" below). That is a half-day hardening commit, not a multi-day design effort.

## Evidence

### The five instances, read from the actual commits and current code

| # | Round | Site | Commit | Shape |
|---|---|---|---|---|
| 1 | r4 | `classifyStartupWindow` (startup deadline) | `401e86ad6` | routed through new `classifyDeadlineExpiry` |
| 2 | r4 | `Wait`'s run-deadline branch | `0cebc0b22` | routed through the same `classifyDeadlineExpiry` |
| 3 | r5 | `Wait`'s events-unreadable cap | `d94503be2` | routed through new `finishedDespiteMechanismFailure` |
| 4 | r5 | `Wait`'s reed-status-error cap | `d94503be2` (same commit, second call site) | same helper |
| 5 | r6 | `Attach`'s `dispositionCandidate` | `d52352305` | new inline top-of-function guard, no shared helper |

Read in full: `internal/shuttleengine/wait.go` (559 lines) and `internal/shuttleengine/attach.go`
(376 lines), current state, plus all four fix commits (`git show 401e86ad6`, `0cebc0b22`,
`d94503be2`, `d52352305`).

**Same underlying signal, same underlying function, at every site.** All five route to the same
seven-line primitive, `allOutputFilesExist(files []string) bool` (`wait.go:324`), which predates the
whole campaign — it was already being called correctly at `pollEventsTick` (`wait.go:273`) and at
`checkLivenessTick`'s not-tracked/not-live branches (`wait.go:350`, `wait.go:356`) before round 4 ever
started. Grepping the two files for `allOutputFilesExist(` today finds exactly **7 production call
sites** (`wait.go:273,350,356,450,480`; `attach.go:301,345`), all in these two files, all in
`internal/shuttleengine`. No eighth site exists yet, and no additional file in the package references
`OutputFiles` in a way that finalizes a verdict.

**But the five ARE genuinely one shape, not five coincidences.** Each is a place that returns/finalizes
a *negative* answer to "did this run finish" — `OutcomeDied`, `OutcomeTimeout`, a mechanism-failure
`error`, or `verdictRespawnEligible` — using only a proxy fact (the clock, reed's liveness answer, a
retry counter) that sits one line away from the fact that actually settles the question. Every fix
commit message says this explicitly and each new fix cites the prior ones by name
(`401e86ad6`→`0cebc0b22`→`d94503be2`→`d52352305` each read "same shape as round X's Y, one exit type
over" or equivalent). This is not retrofitted framing — it is how each round's own reviewer found the
next instance, by asking "is there another exit like the ones already fixed."

**Where they genuinely differ (and why this matters for the mechanism question).**
Three structural differences separate the five sites, and none of them is cosmetic:

1. **Return shape.** `classifyDeadlineExpiry` returns a bare `Outcome`; `finishedDespiteMechanismFailure`
   returns `(Result, error, bool)` because its caller must keep control to build a `fmt.Errorf` when the
   contract is unsatisfied; `dispositionCandidate`'s guard returns `attachVerdict`, a third type
   entirely. A single shared function signature across all five sites is not possible without forcing
   at least two of the three call shapes through an awkward adapter.
2. **Caller identity.** `classifyDeadlineExpiry` and `finishedDespiteMechanismFailure` are methods on
   `*Run` — they run *inside* an already-constructed, already-live `Run`. `dispositionCandidate` is a
   free function that runs *before* any `*Run` exists at all — it is deciding whether to build one.
   The completion-signal check is answering a different question in the two contexts: "should this
   already-running Run's negative classification be overridden" versus "should a Run be reconstructed
   here instead of a fresh one spawned." Round 6's fixer report makes exactly this point when explaining
   why the crash-versus-bounce trap does not reopen: the safety argument for the `Attach`-side guard
   (`c.state.Outcome == runOutcomeRunning`) is not the same safety argument as the `Wait`-side ones
   (there is no "still running" gate needed inside `Wait`, because by construction `Wait` is only ever
   called on a `Run` that has not yet finalized).
3. **The guarding precondition differs.** `dispositionCandidate`'s guard additionally requires
   `c.state.Outcome == runOutcomeRunning` — a check the two `Wait`-side helpers never need, because
   inside `Wait` the run is by definition still in its `running` phase. Folding all three into one
   shared checkpoint would either (a) silently drop that extra precondition when reused from `Wait`
   (harmless there, since it's always true, but then the "one shared checkpoint" is carrying dead
   logic three-quarters of the time), or (b) require passing an extra bool/predicate into a
   "generic" checkpoint, which is exactly the kind of parameterized indirection that makes call sites
   harder to read for no enforcement benefit — the opposite of what the ref-shape ledger achieved.

So: genuinely one **defect class** (a real, nameable, recurring omission), but not one **call shape**
in the sense the ref-shape ledger needed. Question 1 in the brief asked me to say plainly if they
differ in ways that matter — they do, on exactly the axis (return shape, caller identity, guarding
precondition) that determines whether a shared function is achievable without contortion.

### Why the `shape.go` mechanism doesn't transplant

Re-read `internal/planparser/shape.go` and `shape_test.go` in full. The registry works because of a
specific structural fact: `refKind` is a **closed 4-value enum**, `ledger` is a **flat
`map[refGate]map[refKind]disposition`**, and the meta-tests exploit that flatness — `TestLedgerCompleteness`
simply ranges the map and asserts every gate's policy covers all four kinds;
`TestRefGateConstantsMatchLedger` AST-parses one const block and diffs its value set against the
map's key set. Both tests are cheap, mechanical, and have zero false-positive risk because the domain
being compared (a set of string constants) is exactly the same shape on both sides.

The "completion signal" invariant has no equivalent flat structure to check. There is no enum of
"exit kinds" analogous to `refKind` — the negative outcomes are `OutcomeDied`, `OutcomeTimeout`, an
untyped mechanism `error`, and `verdictRespawnEligible`, which live in different types
(`Outcome` is a string type in `engine.go`; `attachVerdict` is an int type in `attach.go`; the
mechanism failure is a bare Go `error`, not a value at all). A mechanical scan enforcing "every return
of one of these four things is preceded by a call to `allOutputFilesExist` (directly or via an
allow-listed helper)" would have to be a genuine control-flow/data-flow static analysis over the
function bodies — closer to writing a custom `go vet`/`x/tools/go/analysis` pass than to comparing two
flat sets. That is a materially higher-cost, higher-false-positive-risk mechanism than the ledger's
AST-diff, for a domain that is currently exactly 7 call sites in 2 files.

### 6th instance — none found

I read `internal/shuttleengine/run.go`'s `Start` (the one other place `OutputFiles`/`Outcome` matter),
`internal/shedadapters/singlellm.go`, `internal/shedadapters/burler.go`
(`Call` and `probeLiveRound`), `internal/shedadapters/bouncer.go` (`Call`, `awaitLiveJudge`,
`settle`, `seedCall`, `judgeCall`), and grepped `internal/loomengine`, `internal/loomcli`,
`internal/loomshed` for `allOutputFilesExist`/`OutputFiles` usage.

- `singlellm.go`'s `mapOutcome` and both `burler.go` call sites (`Call`'s own switch and
  `probeLiveRound`) consume `result.Outcome` **after** it has already passed through the now-hardened
  `shuttleengine.Runner.Attach`/`Wait` — they do not re-derive a completion verdict from scratch, so
  they are not additional instances of the same omission; they inherit the fix for free.
- `burler.go` and `bouncer.go`'s round-recovery logic (`highestCompleteRound`, `roundComplete`,
  `judged`) use a **different** completion signal entirely — on-disk round-artifact pairs, checked
  directly by the producer, independent of `Attach` — which round 6's own reading confirmed
  ("these rows self-recover finished-round artifacts without Attach"). This is a deliberately
  different mechanism (the design's "crash-versus-bounce trap" specifically forbids a producer-level
  bare file-existence check being read as a run-outcome verdict on its own, for `SingleLLMProducer`'s
  two rows — see `singlellm.go`'s own doc comment on `prepareFreshSpawn`), not an unaudited
  fifth/sixth copy of the same bug shape.
- `internal/loomengine`'s `VerifySeedOwnership`/`CheckSeed` and `internal/loomshed`'s `Seed` answer a
  structurally different question (is this status file's *encoding* trustworthy enough to act on),
  never "did an agent's run finish" — no `OutputFiles`/`allOutputFilesExist` involvement at all. This
  confirms the brief's suspicion should be checked, not assumed: the pattern does **not** recur in
  `internal/loomengine`/`internal/loomcli`/`internal/loomshed`, because those layers' completion
  questions are answered by genuinely different signals (decode success, seed ownership, or
  round-artifact completeness), not by the shuttle-level file contract.

I did not find a sixth instance of the exact shape (a negative shuttle-run verdict finalized without
consulting `allOutputFilesExist`) anywhere in the reachable surface I read. This is some evidence
*against* urgency (the surface looks genuinely closed now, confirmed independently across rounds 4,
5, and 6, each re-spot-checking the prior rounds' fixes and finding them holding), though it does not
prove no seventh caller will ever be added to `wait.go`/`attach.go` in the future.

## If warranted — the narrow version I'd actually recommend

Not a new mill task with its own design/plan step. This is small enough for either a single targeted
hardening commit (a few hours) or a finding inside one more crucible round, whichever the operator
prefers procedurally. The concrete shape:

**Title**: Pin the completion-signal call sites in `internal/shuttleengine` against silent regrowth.

**Problem statement**: `internal/shuttleengine/wait.go` and `attach.go` currently have exactly 7
production call sites that must consult `allOutputFilesExist` before finalizing a negative run
verdict, all audited and correct as of round 6. Nothing currently stops a future edit — a new retry
cap, a new liveness branch, a new attach-side special case — from adding an 8th negative-verdict
return that forgets the check, the same way each of the five fixed instances was added at a different
time by a different author (or generation) without the omission being caught until an adversarial
crucible round found it live.

**Enforcement mechanism (concrete, deliberately lightweight)**:
1. A single named doc-comment section — "Completion Signal Invariant" — added to `wait.go`'s package
   doc comment (it already documents the file's other cross-cutting rules at the top), naming the rule
   once: *any code path in this package that finalizes `OutcomeDied`, `OutcomeTimeout`, a
   mechanism-failure `error`, or `verdictRespawnEligible` must first consult `allOutputFilesExist`
   over the run's `OutputFiles`, directly or via `classifyDeadlineExpiry`/
   `finishedDespiteMechanismFailure`.* This is the analogue of `checkLivenessTick`'s own doc comment,
   which already states a version of this rule informally — promoting it to a named, cross-referenced
   invariant is what CONSTRAINTS.md is for.
2. One CONSTRAINTS.md entry (a short paragraph, not a new invariant category) cross-referencing the
   five fixed instances and the doc-comment location, so a future reader searching CONSTRAINTS.md (the
   repo's own stated first stop before writing or reviewing code) finds this rule the same way they
   find the Ref-Shape Registry Invariant.
3. **A literal-count regression test**, not an AST-diffed ledger: a test in `internal/shuttleengine`
   that greps/AST-walks `wait.go` and `attach.go` for return statements yielding the four negative
   constants/types (`OutcomeDied`, `OutcomeTimeout`, `verdictRespawnEligible`, and the three
   `fmt.Errorf` mechanism-failure returns in `Wait`) and asserts the count matches today's known-audited
   count (a handful of named line ranges), failing loudly — "a new negative-verdict return site
   appeared; confirm it consults allOutputFilesExist and update this test's expected set" — rather than
   silently passing. This is a tripwire, not a completeness proof: it cannot verify a *new* site
   correctly calls the check, only that a human was forced to look at it. That is a real difference from
   `TestLedgerCompleteness`, which mechanically proves completeness because the domain (4 `refKind`
   values) is exhaustively enumerable up front; here the "domain" is however many return statements a
   future diff adds, which is not enumerable in advance the same way.

**Call sites that would need touching**: none of the five existing fixes are touched — this is
additive documentation plus one new test. The only files touched: `internal/shuttleengine/wait.go`
(doc comment), `internal/shuttleengine/attach.go` (doc comment, optionally), `CONSTRAINTS.md` (one
paragraph), and one new or extended `_test.go` file.

**Honest scope/size estimate**: half a day, including writing and sabotage-proving the tripwire test
(add an 8th unguarded negative return in a scratch copy, confirm the test catches it, revert). This
is not a "day of focused work" full task and does not need its own design/plan step; it fits as a
single crucible-round finding or a standalone small commit.

**Explicitly OUT of scope**:
- A `shape.go`-style ledger/registry module. There is no enum of "exit kinds" to build one from (see
  "Why the mechanism doesn't transplant" above), and forcing one into existence would add a layer of
  indirection (a map lookup keyed by some invented "exit-site identifier") that makes the five call
  sites *harder* to read for a static-analysis guarantee weaker than what the existing doc comments
  plus a tripwire test already give.
- Touching `internal/loomengine`/`internal/loomcli`/`internal/loomshed` — confirmed above that the same
  defect shape does not recur there; their completion questions are answered by different, unrelated
  signals.
- Re-opening the `AddStrand`/`run.json` "Accepted residual" (the crash-mid-registration race) — a
  separate, already-analyzed, structurally different (two-independent-stores) problem that three
  rounds have already agreed is an operator tradeoff, not a completion-signal omission.
- Building any new production abstraction (a `CompletionChecker` type, an interface, a wrapper) — the
  existing `allOutputFilesExist` primitive is already correct and already shared; nothing about its
  implementation needs to change.

## Sixth instance found

None. See "6th instance — none found" above for what was checked and why the two most plausible
neighboring areas (`shedadapters`' round-recovery logic, `loomengine`/`loomshed`'s seed-integrity
gates) are not additional instances of this shape.
