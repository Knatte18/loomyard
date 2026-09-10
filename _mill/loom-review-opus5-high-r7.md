# `loom` — independent review, round 7 (`opus5-high-r7`)

Clean-room round-7 review of the `loom` module per `_mill/loom-review-prompt.md`.
Written incrementally during Job 1 ("Log as you go"); the executive summary and final severity ordering were written last.

## Executive summary

**This round found something.** Thread B/C is NOT converged: a fifth consecutive round has turned up
a real defect of the same recurring shape, and it is the **sixth instance** — the one the
independent "should this be centralized?" investigation looked for and did not find, because it
scoped its search to sites that return a *verdict* and these three return an *error*.

**F1 (MEDIUM, CONFIRMED live):** `Runner.Attach`'s three reed-state gates
(`attach.go:64`, `:68`, `:76`) abandon every candidate — including one whose persisted record still
reads `outcome:"running"` and whose every declared output file is already on disk — without ever
consulting `allOutputFilesExist`. They sit UPSTREAM of round 6's `dispositionCandidate` guard, so
round 6's fix cannot reach them. Reproduced end to end against the real built binary (scenario B5):
the identical on-disk state that B2 harvests as `done` when reed's table is readable becomes a hard
`shedadapters: Discussion-Write (shuttle): shuttle attach: …` step failure when it is not, with the
finished, expensive LLM output left unharvested and the run directory left in place. That is exactly
the rework `manifest/designs/loom.md`'s crash-recovery step 2 promises will not happen.

**The round's assigned residual (the Completion Signal Invariant hardening) is also closed** — doc
comment, `CONSTRAINTS.md` paragraph, literal-count tripwire test, and the tripwire's own sabotage
proof — and the tripwire was written to catch F1's shape too, not only the verdict-returning shape
the brief's line-number hint describes (see R1 below for why that distinction was load-bearing).

**Top risks, ranked**

1. F1's failure mode is a *hard step failure*, not silent rework — it is loud, but it needs operator
   intervention to clear, and the trigger (reed's strand table unreadable or absent under a run whose
   agent already finished) is one reed's own documentation calls "not rare" and one `run.go:361`
   names as a sanctioned operator action.
2. The recurring shape has now produced six instances across four consecutive rounds. The
   investigation's "none found" conclusion was reached against a narrower definition of the class
   than the class actually has. That is worth carrying forward as a re-seed, not as convergence.
3. Thread A remains converged. 13 live `validate-plan` scenarios against a real hub, a real
   quarry-resolvable Go tree and real git history reproduced every behavior
   `quarry-glyph-plan-alphabet.md` specifies, including a genuinely `ambiguous` resolve and the
   `ErrQuarryUnavailable` disposition. No regression.

**Merge-readiness opinion.** After the fixes in this round's fixer report: **ready to merge.** The
branch is green (`go build`, `vet`, `-count=5`, full `go test ./...`, the 12-case live smoke suite),
F1 is fixed and its fix is proved live by re-running the scenario that reproduced it, and every
finding including the two NITs is closed. Whether thread B/C is *converged* is the orchestrator's
call, not mine — my input to it is that this round found a real defect, so the thread's own bar
("a round that comes back clean") is not met yet.

**Counts by severity:** 2 MEDIUM (F1, F5), 1 LOW (F2), 2 NIT (F3, F4), plus the assigned residual
(R1). All six closed in Job 2.

F5 was found during Job 2's verification pass rather than Job 1, and is recorded in full below with
that provenance stated. It is a pre-existing race in the live smoke suite's own
`TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed` — verified pre-existing by reproducing it
on the pre-round tree — that makes the gate accuse loom of a routing bug about one run in three.

## Scope assessment — plan-promised vs shipped

- **Thread A (`centralize-glyph-shape-enum`, `quarry-bump-v0-2-0-status-helpers`):** shipped ==
  promised. Every behavior `manifest/designs/quarry-glyph-plan-alphabet.md` specifies for the handle
  lifecycle, the resolve status policy, the Create inversion, both containment tiers and the
  infrastructure-error disposition was reproduced live and matched. Nothing deferred-that-should-be-v1;
  nothing shipped beyond scope. The `Ref-Shape Registry Invariant`'s claim that every ref-shape
  decision routes through `shape.go`'s ledger holds: all thirteen policies are complete over all four
  `refKind`s, and three of them were driven live on `refKindSymbol` with no panic.
- **Thread B/C (bootstrap / crash recovery):** shipped **falls short of** promised, in one place.
  `manifest/designs/loom.md:352-357` promises that a matched `"running"` record whose output files are
  all on disk "is a run that FINISHED, harvested as `done` whatever `reed` now thinks of its pane —
  a dead pane, a strand `reed` no longer tracks, a cleared pane binding". `Attach` delivers that for
  every case where reed *answers*, and delivers the opposite (a hard refusal) for the three cases
  where reed cannot be *read at all* — which are strictly weaker evidence than the ones the doc
  already says do not matter. F1 closes that gap; the doc gains the same sentence in the same commit.
- **The layering rule** the brief states ("every layer must answer only the question it owns") is
  otherwise honoured throughout the area I read: `mustSpawnDriver`, `awaitRunLock`,
  `dispositionForHandshake`, `resolveStatusStrandAction` (`internal/loomcli/bootstrap.go`),
  `CheckSeed`/`VerifySeedOwnership` (`internal/loomengine/seed.go`), and `sweepOrphansOpportunistic`
  (`run.go:348-397`) each answer exactly one question and each documents why. F1 is the exception,
  not a pattern of exceptions.

## Status

Job 1 COMPLETE. Findings below are final; Job 2's work is recorded in
`_mill/loom-review-opus5-high-r7-fixer-report.md`.

## What was tested

(Appended incrementally. Exact commands + observed results.)

### Environment preflight

```
git branch --show-current   -> crucible-loom-refshape-registry
git status --short          -> clean
which tmux                  -> /usr/bin/tmux          (present; smoke tests will not skip-as-pass)
go version                  -> go1.26.0 linux/amd64
which gcc                   -> /usr/bin/gcc           (cgo build prerequisite satisfied)
```

### Hermetic gates

```
go build ./...                                 -> BUILD_OK (clean)
go vet ./internal/loomengine/... ./internal/loomcli/... ./internal/loomshed/... \
       ./internal/planparser/... ./internal/planglyph/... ./internal/websterengine/... \
       ./internal/webstercli/... ./internal/shuttleengine/...
                                               -> clean, no diagnostics
go test -count=5 <the nine in-scope packages + ./cmd/lyx/...>
                                               -> EXIT=0, all ok
```

### Live smoke suite (real substrate, real tmux, zero provider subprocesses)

```
which tmux -> /usr/bin/tmux (present, so no test could skip-as-pass)
go test -tags smoke ./internal/loomcli/... -run Smoke -v -count=1  -> EXIT=0
```

All 12 cases PASS, none SKIP: `TestSmokeBurlerRound_AttachesToALiveRoundInsteadOfRespawning` (4.09s),
`TestSmokeBootstrap_BringsUpSessionStrandAndDriver`, `…_SecondInvocationDoesNotSpawnASecondDriver`,
`TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed` (the assertion commit `aba2c270a`
rewrote), `…_RefusesOnNeverSeededPair`, `…_FailureBeforeFirstPersistLeavesNonEmptyLog`,
`TestSmokeBootstrap_MalformedStatusProceedsToHandoverAndLogsWhy` (the `state.ErrDecode` fix's guard),
`TestSmokeFabricAdd_RunLauncherExistsThenGoneAfterRemove`, `…_CleanlinessOrderingAfterSeedCommit`,
`…_OriginRecordSelfHealsAfterCrashBetweenWriteAndCommit`, `…_ConcurrentSpawnHandshakeYieldsOneDriver`,
`…_DiedDriverProceedsToHandoverAndLogsWhy`.
Teardown confirmed: `pgrep -af tmux` afterwards matched only my own probe command line, and
`tmux ls` answered "error connecting to /tmp/tmux-1000/default (No such file or directory)" — zero
stray tmux servers.

### Thread A — live driving through the REAL built binary (regression spot-check)

I built a real fabric hub end to end from the CLI rather than reusing a Go fixture helper:

```
CGO_ENABLED=1 go build -o <scr>/bin/lyx ./cmd/lyx
git init --bare -b main <scr>/remotes/demo3.git ; …/demo3-weft.git   (warp pushed, weft empty)
lyx fabric clone <scr>/remotes/demo3-weft.git <scr>/remotes/demo3.git   -> ok:true, hub demo3-HUB wired
lyx fabric add r7task                                                    -> ok, pair r7task / r7task-weft
```

then planted a real quarry-resolvable Go tree in the warp worktree (`internal/rows` with `Row`,
`MapRow`, `Helper`; `internal/out` with `Ok`; later `dup_a.go`/`dup_b.go` declaring `Dup` twice) plus
a real `_lyx/plan/`, and drove `lyx loom validate-plan` for each scenario. Every command was run
foreground from the worktree root and its full JSON envelope read.

| # | scenario | observed | verdict |
|---|---|---|---|
| A0 | well-formed plan, Create handle declared + referenced, Rename to-side a `plan:` handle | `{"ok":true,"plan_dir":…}` for both `validate-plan` and `--require-approved` | matches spec |
| A1 | `Create` targeting an already-existing symbol (`plan:internal/rows#Helper`) | `create-already-exists/1-…[blocking]: Create target "plan:internal/rows#Helper" already resolves found` | Create inversion correct |
| A2 | `Create` into a brand-new unit (`plan:internal/brandnew#Thing`) | `create-new-unit/…[informational]`, envelope `ok:true` with the finding surfaced under `findings` | matches spec's informational rule |
| A3 | reference a member that does not exist (`internal/out#NoSuchSymbol`) | `glyph-not-found/…[blocking]: …'s unit exists but the member is missing — check for a misspelled member` | `not_found` + `Unit: found` branch correct |
| A4 | `Rename` to-side spelled as a glyph, not a handle | `rename-to-not-handle/…[blocking]: … has a new side that is a glyph, not a plan: handle` (+ the consequent `handle-dangling`) | `gateRenameTo` `dispFinding` correct |
| A5 | genuinely AMBIGUOUS member (`Dup` declared in two files of one unit) | `glyph-ambiguous/…[blocking]: target "internal/rows#Dup" is ambiguous among candidates: internal/rows#Dup (internal/rows/dup_a.go), internal/rows#Dup (internal/rows/dup_b.go)` | real `ambiguous` status, listed candidates — the quarry-v0.2.0 `Status.Known()` call site driven for real |
| A6 | `Create` inversion over that same ambiguous target | `create-already-exists/…[blocking]: … already resolves ambiguous among existing declarations: …dup_a.go, …dup_b.go` | matches the spec's explicit ambiguous-is-create-already-exists rule |
| A7 | resolve-backed containment: card 2 edits `internal/rows#Helper`, card 1 edits `//internal/rows/rows.go` | `containment-file-overlap/2-…[blocking]: card 2's member glyph physically overlaps card 1-…'s own file self glyph naming "internal/rows/rows.go"` | tier 2 correct |
| A8 | syntactic containment: card 1 `internal/rows#`, card 2 `//internal/rows/rows.go` | `containment-unit-overlap/2-…[blocking]: card 2's file self glyph … overlaps card 1-…'s own self glyph naming that file's whole unit "internal/rows"` | tier 1 correct |
| A9 | quarry infrastructure error (`chmod 000 internal/rows`, so quarry cannot read the tree) | `{"error":"loom: quarry could not answer validating plan at …: planglyph: quarry could not answer: resolve: engine: read .gitignore …: permission denied","ok":false}` | `ErrQuarryUnavailable` disposition correct — reported as a gate failure, NOT as "plan is not yet valid", so a quarry outage cannot silently pass a `Create` card |
| A10 | bare package-qualified symbols on both Rename sides and on an Edit | three `bare-symbol-target[blocking]` + `rename-to-not-handle … is a bare symbol` + `rename-from-not-glyph … is a bare symbol`; **no panic** | `refKindSymbol`'s `dispFinding` entries in `gateBareSymbolTarget`/`gateRenameTo`/`gateRenameFrom` exercised live |
| A12 | real `Prosa:` group under `language: none` with a glyph, a bare symbol and a path | `prosa-symbol-target[blocking]` for the glyph and the symbol, none for the path | `gateProsaPathOnly` (path `dispKeep`, others `dispFinding`) exercised live |
| A13 | the same Prosa group under `language: go` | `bare-symbol-target` + two `prosa-symbol-target` | the glyph-enabled branch of the same check |

**Fail-closed `lookup` — answered.** The prompt asks whether a symbol-shaped ref reaching a gate
with no entry for `refKindSymbol` actually panics live. It cannot, and that is correct by
construction, not an untested gap: every one of the thirteen policies in `internal/planparser/shape.go`'s
`ledger` declares all four kinds explicitly (verified by reading the map, lines 107-186), and
`TestLedgerCompleteness` plus the two AST sync meta-tests make that mechanical. The panic in
`lookupIn` (`shape.go:196-198`) guards the addition of a FIFTH `refKind`, which is exactly what its
own doc comment claims. A10/A12 drove `refKindSymbol` through three `dispFinding` gates live with no
panic and correct findings, which is the reachable half of the question.

### Thread A — DetectDrift against real git deltas

`lyx webster record-batch <NN>` was NOT reachable for a live drive: `RecordBatch` reaches
`planglyph.DetectDrift` only behind the bracket-discipline check, an incremental fork audit over real
provider transcripts, and the per-card done-checks (`internal/websterengine/recordbatch.go:87`,
`:273-330`), so driving it through the CLI would have required a genuine LLM-driven webster batch —
which this campaign's cost declaration forbids and which its design intent says the mechanics under
test do not need. Stating that explicitly rather than skipping silently, per the prompt.

What I drove instead is the same code against the same real substrate one layer in — the
`//go:build integration` suite, which builds REAL git fixture repos and calls the REAL
`quarry.Repo.Resolve`/`Delta`:

```
go test -tags integration ./internal/planglyph/... \
  -run 'DetectDrift|Delta|Containment|Create|Resolve|Handle|Done' -count=1 -v
```

60 cases, all PASS. The ones that carry this round's regression question: 
`TestDetectDrift_RealDeltaGateOneRecognizesDeclaredRename` and
`TestDetectDrift_RealDeltaExactTierRepairsUndeclaredRename` (both build a real git rename commit and
run the real `Delta`), `TestDetectDrift_RenameMatchingCardPairProducesNoFindingNoAmendment` (the
`renameCardPairs` regression a prior round hit for real — still closed),
`TestDetectDrift_DeletedAndStillReferencedProducesBlockingFinding` (evidence tier stays blocking and
never repairs), and the whole `TestCreateFindings_*`/`TestCanonicalizeHandles_*`/`TestBindHandles_*`
families.

### Thread B/C — live driving through the REAL built binary (the round's main event)

Same real hub, with both `shuttle.yaml` (`claude: /nonexistent/lyx-r7-has-no-provider`,
`startup_timeout_s: 2`) and `loom.yaml` (`discussion_timeout_min: 1`) patched exactly the way
`providerlessShuttleConfig`/`fastDeadlineLoomConfig` do, so ZERO real provider subprocesses ran.
Every command was foreground and its envelope + the durable trace sink
(`.lyx/logs/trace-*.log`) read.

| # | scenario | observed |
|---|---|---|
| B0 | `lyx loom drive` on a never-seeded pair | `{"error":"loom: no status file at …/_lyx/loom/status.json; run \"lyx loom run\" first to bootstrap this task"}` — the drive-may-not-seed refusal, correct |
| B1 | `lyx loom run` — real bootstrap | seeded `_lyx/loom/status.json`, created `.lyx/reed.json`, brought up a real tmux session (socket `lyx-demo3-HUB-21a7e62b`), added the `loom-status` strand live, spawned the detached driver, then failed the terminal handover with `open terminal failed: not a terminal` (expected in a non-tty). Driver ran and halted at `Preflight` on `worktree-clean` — a genuine, correctly-diagnosed halt |
| B1b | clean the pair, `lyx loom drive` again | reached `Discussion-Write`, started a REAL shuttle run (`shuttle: run started`), the providerless launch failed, and it classified `died` at `startup_timeout_s` — a real `.lyx/shuttle/<id>/run.json` on disk with `outcome:"died"`, `started:false` |
| **B2** | **round 6's fix, live**: put that record back to `outcome:"running"`, write both declared output files, re-drive | trace sink: `shuttle: run attached` → `shuttle: cleanup: remove strand failed (non-fatal)` → `shuttle: run finished`. **The crashed-but-finished run was harvested rather than respawned over** — round 6's `dispositionCandidate` guard confirmed working end to end on the real substrate, including the untracked-strand path |
| B3 | same, with `reed.json` deleted on a 50 ms cadence during the drive | the delete landed AFTER `Attach` had already harvested; the machine then bounced through `Discussion-Validate` and respawned. Recorded because it is what made the timing requirement for B5 explicit |
| B4 | crashed-but-finished run + TRUNCATED `reed.json`, `lyx loom drive` | `{"error":"reed state file …/.lyx/reed.json is unreadable: state: decode failed: unmarshal state: unexpected end of JSON input — … Either run \"lyx reed down\" … or delete …"}`. **`drive` refuses at its own `reed.Up()`**, which is upstream of `Attach` — so F1's `LoadState`-*error* gate is shielded from `lyx loom drive`/`lyx loom run` by `Up()`. It also directly disproves F2's claim: `up` does not repair the file, it refuses on it |
| **B5** | **F1, live**: same crashed-but-finished state, `reed.json` held absent for the whole drive (the `git clean -xdf .lyx` / lost-strand-table case `run.go:361` names as a sanctioned operator action) | `{"error":"shedadapters: Discussion-Write (shuttle): shuttle attach: shuttle: attach: no reed state file at …/.lyx — an absent strand table is not evidence any of the 1 matching run dir(s) are dead; check \"lyx reed status\"","ok":false}` — **the step hard-failed and the finished, expensive output was never harvested.** `ls .lyx/shuttle` afterwards still shows `aaaa1111…` untouched and both discussion files still at their canonical (un-archived) paths: nothing advanced, nothing was cleaned up |

B2 and B5 are the same on-disk state, differing only in whether reed's strand table is readable.
B2 harvests; B5 hard-fails. That is F1, reproduced: **CONFIRMED, live, against the real built
binary.**

## Findings

(Appended provisionally as spotted.)

### F1 — `Attach`'s three reed-state gates abandon a finished run without consulting the file contract (the SIXTH instance of the recurring shape)

**Severity: MEDIUM. CONFIRMED — reproduced live against the real built binary** (scenario B5 under
"What was tested"; contrast with B2, the identical on-disk state with a readable reed table).

`internal/shuttleengine/attach.go:64`, `:68`, `:76`.

Round 6 put the file-contract-first guard at the TOP of `dispositionCandidate` (`attach.go:301`).
But `dispositionCandidate` is called at `attach.go:84`, which is downstream of three earlier
gate returns that abandon every candidate before any disposition is computed:

| line | gate | returns |
|---|---|---|
| 64 | `reedengine.LoadState` errored (unreadable/truncated `reed.json`) | `(Result{}, false, err)` |
| 68 | `LoadState` reported the state file ABSENT | `(Result{}, false, err)` |
| 76 | `r.reed.Status()` errored (incl. reed's foreign-session refusal) | `(Result{}, false, err)` |

All three answer "has reed's own bookkeeping gone wrong", never "did this run finish" — the exact
sentence `finishedDespiteMechanismFailure`'s own doc comment (`wait.go:456-478`) uses for round 5's
two `Wait`-side caps. This is the same defect shape, one layer earlier, on the entry side:
`Attach`'s twin of round 5's fix, exactly as round 6's `dispositionCandidate` guard was `Attach`'s
twin of round 4's.

**Failure scenario (inputs/state → wrong behavior).**
1. A driver crashes after its agent wrote every declared output file but before any `Wait`
   classified the run, so `run.json` is stranded at `outcome:"running"` with all `OutputFiles` on
   disk — the exact state round 6's guard exists to harvest, and the state
   `manifest/designs/loom.md`'s crash-recovery step 2 promises is harvested as `done`.
2. reed's own strand table then becomes unreadable (`kill -9`/full disk/power loss during a reed
   write — reed's `unreadableStateError` documents this as "not rare"), or absent (a
   `git clean -xdf` of `.lyx`, which `run.go:361` itself names as "a sanctioned operator action
   under the Durable-vs-Ephemeral State Invariant"), or `reed.Status()` refuses (reed's
   foreign-session refusal after a renamed worktree or a copied `.lyx` — the very cases
   `errStrandNotTracked`'s doc comment enumerates).
3. The next resume's `SingleLLMProducer.Call` probes `Attach` first (`singlellm.go:118`). `Attach`
   collects the one matching candidate, then hits one of the three gates and returns an error.
4. `singlellm.go:119-124` turns that into a hard producer error
   (`shedadapters: <row> (shuttle): shuttle attach: …`), which fails the whole Shed step. The
   finished, expensive LLM output sitting on disk is never harvested; the step needs operator
   intervention instead of self-healing.

The `Wait`-side twin of exactly this is already fixed and documented: `loom.md:351` records round 5
reproducing "a run whose every declared output file was on disk, its `reed.json` truncated mid-run
… abandoned with a mechanism error rather than classified `done`". `Attach` still does that.

**Why this is not one of the five closed instances.** The five are `wait.go`'s two deadline paths,
`wait.go`'s two mechanism-failure caps, and `attach.go:301`'s `dispositionCandidate` guard. All
three sites here are *upstream* of `dispositionCandidate` and none of them is reachable from
`Wait` at all. Grepping `allOutputFilesExist(` still finds exactly the audited 7 sites; these three
gates are the ones that were never counted because they return an `error`, not a verdict.

**Suggested fix.** The direct twin of `finishedDespiteMechanismFailure`: before each of the three
gate returns, check whether a candidate's file contract already answers the question, and attach it
instead of erroring. The check needs no reed read at all (round 6's own comment: "a run that
FINISHED whatever reed now thinks of its pane"), and the reconstructed run's `Wait` then harvests
it as `OutcomeDone` through round 5's own `finishedDespiteMechanismFailure` even with reed still
broken. Keep it conservative: only when exactly ONE candidate carries `outcome:"running"` with a
satisfied contract; otherwise fall through to today's error unchanged, so every healthy-reed path
stays byte-identical.

### F2 — `attach.go`'s not-tracked comment claims reed repairs an unreadable `reed.json`; reed explicitly refuses to

**Severity: LOW. CONFIRMED** (both sides read).

`internal/shuttleengine/attach.go:332-335` justifies the age escape with:

> An absent or unreadable reed.json is repaired in-band by "lyx reed up", or simply by
> "lyx loom run", which calls reed.Up() itself.

That is true for ABSENT and false for UNREADABLE. `internal/reedengine/state.go:158-160` states the
opposite in as many words — "Repairing the file automatically is deliberately not offered: every
repair reed could perform amounts to discarding the strand table" — and
`unreadableStateError`'s own comment (`state.go:145-148`) records that reed refuses "EVERY verb that
loads state — up, resume, status, add, remove, even attach" on a corrupt file. So the one remedy
this comment offers an operator for the unreadable case does not exist; reed's actual remedies are
`lyx reed down` or deleting the file by hand.

This matters beyond tidiness: it is the sentence a future reader would rely on to conclude the
unreadable case self-heals — which is precisely the wrong conclusion for F1's scenario.

### F3 — the package doc's `Attach` summary predates the file-contract-first guard

**Severity: NIT. CONFIRMED.**

`internal/shuttleengine/doc.go:58-63` still describes `Attach` as answering its question "on
live-agent evidence plus the persisted `RunState.Outcome`" and frames those as "a deliberate
two-line-of-defence framing". Since round 6 there is a third, higher-precedence line of defence —
the file contract, consulted at `attach.go:301` BEFORE the liveness dispatch — and F1 adds two more
places it is consulted. A reader who takes `doc.go` as the package's summary of `Attach` gets a
description that no longer matches `attach.go`. `attach.go`'s own file doc comment (`attach.go:1-5`)
has the same gap.

### F4 — `attach.go`'s file doc comment describes only the liveness half of the disposition

**Severity: NIT. CONFIRMED.**

`internal/shuttleengine/attach.go:1-5` says Attach "scans the run-dir root for a matching `run.json`,
dispositions each match against reed's own liveness answer, and — on exactly one live match —
reconstructs a `*Run`". Since round 6 the first thing `dispositionCandidate` does is NOT consult
reed's liveness answer, and after F1 two more paths reach a `*Run` with no reed answer at all.
Same class as F3, different file; recorded separately because they are separate edits.

### F5 — `TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed` races the row it kills the driver on

**Severity: MEDIUM. CONFIRMED — reproduced roughly 1 run in 3, and reproduced on the PRE-round tree,
so it predates this round's own changes.**

`internal/loomcli/smoke_test.go`'s `TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed` —
one of the three files thread B puts under review, since commit `aba2c270a` rewrote this exact
test's assertion.

**Found during Job 2's verification pass, not Job 1.** Recorded here anyway, clearly marked, rather
than only in the fixer report: it is a finding about the code under review, and a reader of this
report should not have to reconstruct it from a commit message.

The test bootstraps a driver with `lyx loom run`, kills it immediately, and then asserts that the
follow-up `lyx loom drive` re-enters the SAME row — via `current_producer` unchanged and history
length unchanged. But nothing establishes which row the driver was on when it was killed. When the
kill lands while the driver is still at `Loom-Preflight` (a fast row), the follow-up `drive`
legitimately completes that row and advances to `Discussion-Write`, so both assertions fire:

```
smoke_test.go:550: current_producer changed from "Loom-Preflight" to "Discussion-Write";
                   want it unchanged -- the failure is attributed to the same row, not routed onward
smoke_test.go:553: history length changed from 1 to 2;
                   want unchanged -- a producer call that reached no verdict records no history entry
```

Both messages accuse loom of a routing bug that is not there. That is the worst shape a gate can
have in a campaign like this one: a future round re-running the live suite either chases a phantom
regression or, having learned to dismiss it, misses a real one.

**Reproduction and provenance.** Running the test alone passes; running it after another
tmux-and-hub test in the same package fails intermittently — 1 of 3 on my host. Verified NOT caused
by this round by rebuilding the pre-round tree (`git show 6bec8892d:` for the five files this round
touches, this round's new test file removed) and reproducing the same failure there with only
pre-existing tests: `TestSmokeBurlerRound_AttachesToALiveRoundInsteadOfRespawning` followed by this
one, 1 failure in 3 runs, same two assertions.

**Suggested fix.** Establish the precondition instead of racing it: poll the status file until it
records `current_producer: Discussion-Write` with the machine still running, and only then kill.
Weakening the assertions is the wrong direction — the test's own doc comment explains at length why
the history-length claim is the durable one, and that claim is correct once the row is pinned.

### R1 — the round's assigned residual: the Completion Signal Invariant is nowhere named

**Not a defect — the pre-scoped hardening this round was assigned.** Recorded here so the round's
own report is complete.

The rule "any code path in `internal/shuttleengine` that finalizes `OutcomeDied`, `OutcomeTimeout`,
a mechanism-failure `error`, or `verdictRespawnEligible` must first consult `allOutputFilesExist`"
exists today only as prose scattered across five doc comments
(`checkLivenessTick`'s "a satisfied file contract wins over every negative answer",
`classifyDeadlineExpiry`'s, `finishedDespiteMechanismFailure`'s, `dispositionCandidate`'s, and
`wait.go`'s file header). It is not named, not in `CONSTRAINTS.md`, and nothing mechanical notices
when a new negative-verdict exit is added without it. Six instances across four rounds is the
evidence that this matters.

**Audited call-site set, verified against the current files before writing anything** (the brief
asks for this explicitly, since the line numbers drift): `allOutputFilesExist(` appears at exactly
seven production call sites — `wait.go:273` (`pollEventsTick`), `:350` and `:356`
(`checkLivenessTick`'s not-tracked and not-live branches), `:450` (`classifyDeadlineExpiry`),
`:480` (`finishedDespiteMechanismFailure`); `attach.go:301` (`dispositionCandidate`) and `:345`
(`leftoverThenAgeVerdict`). All seven confirmed present at those exact lines.

**One correction to the brief's own sketch, and it is load-bearing.** The brief describes the
tripwire as asserting "the count matches today's audited set (7 `allOutputFilesExist` call sites)",
but its own sabotage requirement is to "add an 8th, unguarded negative-verdict return site … confirm
the test fails and names it". A test that counts `allOutputFilesExist` calls **cannot** fail on an
unguarded new negative-verdict return, because an unguarded return adds no such call — it would be
exactly the tripwire-that-does-not-trip the brief warns is "the single most on-theme mistake this
round could make". The test therefore tracks **negative-verdict return sites** (the thing that can
grow unguarded) as its primary assertion, and pins the seven `allOutputFilesExist` sites as a
*second* assertion (which catches the opposite mutation — a guard being deleted). Both are needed;
neither subsumes the other. F1 is itself the proof: it is three negative exits that never called
`allOutputFilesExist`, so a call-counting test would have reported "7, as expected" while the bug
was live.

## Docs & operability findings

- F2, F3 and F4 above are comment/doc-accuracy findings; all three are fixed in Job 2 alongside
  their code.
- `manifest/designs/loom.md:352-357` (crash-recovery step 2) describes `Attach`'s file-contract-first
  rule entirely inside the matched-record disposition, with no mention that three reed-state gates
  sit ahead of it and refuse before it runs. Updated in the same change as F1.
- **Operability, positive:** the durable trace sink made every one of the B-series scenarios legible
  without any added instrumentation — `shuttle: run attached` / `run started` / `run finished` at
  Info, and the orphan-sweep and cleanup failures at Warn, were exactly what was needed to tell B2's
  harvest apart from B3's respawn. The Live-Substrate Spawn Observability invariant is paying for
  itself.
- **Operability, negative (F1's second-order cost):** when `Attach` refuses at a reed gate, nothing
  ever removes the matching run directory — `sweepOrphansOpportunistic` runs only inside `Start`,
  which the refusal never reaches (`attach.go:330-335` says this in as many words about a different
  branch). So every subsequent resume re-reads the same directory and re-refuses identically until
  the operator applies reed's own out-of-band remedy. Confirmed in B5: `ls .lyx/shuttle` after the
  refusal still showed the candidate directory untouched.

## Test-coverage soundness — did rounds 3-6's own regression tests actually trip?

The brief asks whether any of rounds 3-6's fixes carry a test that would still pass if the fix were
reverted (round 3's F1 was that shape). I answered it by mutation testing rather than by reading:
the whole repo was copied to a scratch tree (`<scratchpad>/mutant`, `.git` excluded), each fix
reverted there one at a time, and the package suite re-run.

| mutation | result |
|---|---|
| M1 — `classifyDeadlineExpiry` always returns `expired` (reverts round 4) | FAIL: `TestRun_Wait_StartupDeadline_SatisfiedFileContractWinsOverDied`, `TestRun_Wait_RunDeadline_SatisfiedFileContractWinsOverTimeout` |
| M2 — `finishedDespiteMechanismFailure` never fires (reverts round 5) | FAIL: `TestRun_Wait_StatusFailureCap_SatisfiedFileContractWins`, `TestRun_Wait_EventsUnreadableCap_SatisfiedFileContractWins` |
| M3 — `dispositionCandidate`'s guard deleted (reverts round 6) | FAIL: `TestAttach_RunningRecordSatisfiedFileContract_HarvestsNotRespawn` |
| M4 — `started := run.attached` alone (reverts round 3's `d0e5a0e7b` gap fix) | FAIL: `TestRun_Wait_AttachedButNeverStarted_StartupProbeStillRuns` |
| M5 — `VerifySeedOwnership` escalates `state.ErrDecode` (reverts `69886823e`) | FAIL: `TestVerifySeedOwnership` |

**All five trip.** No prior round shipped a test that would survive its own fix being reverted; the
lesson from round 3's F1 has been applied consistently. Every file was restored from its `.orig`
copy immediately after each run, and the scratch tree is outside the worktree, so no production or
test file in the repo was touched during Job 1.

## Independent spot-checks of the CLOSED items (brief asks for minutes, not a re-derivation)

- **`VerifySeedOwnership` vs `CheckSeed` disposition-sharing claim (round 3, "confirmed sound").**
  Independently re-checked: the line both draw is *decode-failure is not an infrastructure error,
  everything else is* — `seed.go:84-88` (CheckSeed: `errors.Is(rerr, state.ErrDecode)` → a
  `CheckSeedIncoherent` verdict, else `return Report{}, rerr`) and `seed.go:148-153`
  (VerifySeedOwnership: same predicate → `nil`, else `return err`). They act differently on the
  decode side (a recorded verdict vs a pass) because they answer different questions, which is the
  point; the error/not-error line itself is identical. Claim holds.
- **`RunState.Started`'s best-effort persistence (round 3).** `wait.go:391-394` persists inside the
  `StartupReady` arm and degrades a save failure to `logger.Warn`, matching `finalize`'s own Outcome
  write. B1b's real run.json on disk carried `"started": false` for a launch that never came up, and
  M4 proves the gating is guarded. Sound.
- **The `AddStrand`/`run.json` "Accepted residual" (four-round settled).** Not re-opened, per the
  brief. Nothing in my own driving surfaced a new angle on it.
- **The `Seed`/`state.ErrDecode` malformed-JSON fix (round 5).**
  `TestSmokeBootstrap_MalformedStatusProceedsToHandoverAndLogsWhy` passed in the live smoke run.
- **The fabric weft-branch-naming bug (rounds 5-6, out of scope).** Tripped over it a third time
  while building this round's hub: a weft remote whose default branch is `master` while the warp's is
  `main` produces a weft prime on `master-weft`, and `lyx fabric add <slug>` then fails with
  `fatal: invalid reference: main-weft`, rolling the pair back. Recorded only because the brief says
  to report it if driving trips over it; NOT fixed, not loom's scope.

## What could NOT be verified, and why

- **`lyx webster record-batch <NN>` as a live entry point to `DetectDrift`.** `RecordBatch` reaches
  `DetectDrift` only past the bracket-discipline check, an incremental fork audit over real provider
  transcripts, and the per-card done-checks, so driving it needs a genuine LLM-driven webster batch.
  That is banned by this campaign's cost declaration and its design intent says it is not needed.
  Driven instead through the `//go:build integration` suite, which builds real git rename commits and
  calls the real `quarry.Delta`/`Resolve` (60 cases, all pass). Named here rather than skipped
  silently, per the brief.
- **Windows path behavior.** Unreachable from this Linux host. A named, never-executed gap — not a
  claim of coverage.
- **F1's `reed.Status()`-failure gate (`attach.go:76`) specifically.** The `st == nil` gate
  (`attach.go:68`) was reproduced live; forcing `Status()` itself to error live would need reed's
  foreign-session refusal, which I could not construct deterministically from the CLI in this
  fixture. The three gates are structurally identical (all three return `(Result{}, false, err)`
  from the same block, all three ahead of `dispositionCandidate`), the fix covers all three, and the
  fix's new hermetic tests cover each gate independently.
- `manifest/designs/loom.md:352-357` (crash-recovery step 2) describes `Attach`'s file-contract-first
  rule but places it entirely inside the matched-record disposition, with no mention that three
  reed-state gates sit ahead of it. It needs the F1 fix recorded in the same change.
