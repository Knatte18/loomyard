# `loom` — fixer report, round 7 (`opus5-high-r7`)

Job 2 of the round described in `_mill/loom-review-opus5-high-r7.md`.
Every finding in that report is fixed — all severities, NITs included. Nothing was deferred.

## What was implemented

### F1 (MEDIUM) — `Attach`'s three reed-state gates now harvest a finished run instead of refusing

Commit `41430ca78`.

`Runner.Attach`'s `LoadState`-error, absent-state-file and `Status()`-error gates each consult the
new `soleFinishedCandidate` before reporting their refusal — the entry-side twin of round 5's
`finishedDespiteMechanismFailure`, and the same shape round 4's `classifyDeadlineExpiry` gave the two
deadlines.

- `soleFinishedCandidate(candidates, spec)` returns the one candidate whose persisted record still
  reads `runOutcomeRunning` while the spec's file contract is already satisfied, and reports whether
  exactly one exists. `allOutputFilesExist` is evaluated once rather than per candidate, because it
  reads the SPEC's file set — which every candidate set-matched to get there.
- The exactly-one rule is deliberate: two finished candidates fall through to the gate's own
  refusal, because reed is precisely the thing that cannot be consulted to tell them apart, and
  picking one silently is the hazard the whole mechanism exists to avoid.
- The `*Run` reconstruction block was lifted out of `Attach`'s tail into `reconstructAndWait`, so the
  four call sites (the ordinary one-attachable-match path plus the three harvests) cannot drift on
  `offset`, `deadline`, or `attached`.
- Each harvest logs a `logger.Warn` naming the gate and the cause before attaching, so an operator
  still learns reed is unhealthy even though the step advanced — preserving what
  `warnAttachCandidates` gave on the refusal path.

Folded into the same commit, because they are the same files describing the same behavior:

- **F2 (LOW)** — `attach.go`'s not-tracked comment claimed an unreadable `reed.json` "is repaired
  in-band by `lyx reed up`". It is not: `reedengine`'s `unreadableStateError` states that repairing
  is "deliberately not offered", and `up` itself refuses on a corrupt file. Rewritten to separate the
  ABSENT case (which `up`/`loom run`/`loom drive` really do recreate) from the UNREADABLE one (which
  nothing repairs, and whose real remedies are `lyx reed down` or deleting the file by hand), citing
  the live confirmation.
- **F3 (NIT)** — `doc.go`'s `Attach` summary described a two-signal answer that predates round 6.
  Rewritten as the three signals in their actual precedence, keeping the caller-level-file-existence
  prohibition intact and stating why the same question is safe inside `Attach`.
- **F4 (NIT)** — `attach.go`'s own file header said `Attach` "dispositions each match against reed's
  own liveness answer". Rewritten to state the two-fact precedence and point at the Completion Signal
  Invariant.

Docs in the same commit: `manifest/designs/loom.md`'s crash-recovery step 2 gains the paragraph
recording that "whatever `reed` now thinks" includes reed being unable to think at all, the three
gates, the live reproduction, and the exactly-one rule.

### R1 — the assigned residual: the Completion Signal Invariant, named and pinned

Commit `00fe3ae12`.

1. **`wait.go` file doc comment** gains a `# Completion Signal Invariant` section stating the rule
   once, listing the four negative answers and the three helpers that own the check, explaining why
   they are two different questions, recording all six instances with their rounds, and stating
   plainly why this is an invariant plus a tripwire rather than a `shape.go`-style ledger (no closed
   value enum; three types across two files). It also records the one structural precondition the
   rule rests on — no `*Run` with an empty `OutputFiles` can exist.
2. **`CONSTRAINTS.md`** gains a `## Completion Signal Invariant` entry cross-referencing that doc
   comment, so a reader following the repo's own stated discipline finds this rule where they find
   the Ref-Shape Registry Invariant. It says in its own text that the enforcement is a tripwire, not
   a completeness proof.
3. **`internal/shuttleengine/completionsignal_enforcement_test.go`** — two AST scans over `wait.go`
   and `attach.go`.

**The one correction I made to the brief's own sketch, because it was load-bearing.** The brief
describes the test as pinning "7 `allOutputFilesExist` call sites", but its own sabotage requirement
is that an 8th UNGUARDED negative-verdict return must make the test fail. A call-counting test cannot
do that — an unguarded return adds no call, so the count still reads correct. F1 is the proof: three
unguarded negative exits lived in `attach.go` the entire time the audited call-site count was
accurate. So the primary assertion pins **negative-verdict return sites** (what can grow unguarded),
and a second assertion pins the `allOutputFilesExist` call sites (what can be deleted, leaving the
return set unchanged). Neither subsumes the other and both are needed.

Design details worth naming:

- Keys are `"<function> [<markers>]"` with a count, never `file:line`. Line numbers drift on any edit
  above a site, which would make the test fail for reasons unrelated to the invariant and get it
  deleted. This key moves only when a return is added, removed, or changes which negative value it
  yields.
- Both scans are AST-based, so a mention in a doc comment or string literal can never trip them.
- Every entry in the audited map carries a justification line, because "the count is 15" is not
  something a future reader can check without one. Sites that are honest noise (`readEventsFrom`'s
  I/O errors, `normalizeAttachSpec`'s argument validation, `Attach`'s two multiplicity refusals) are
  listed with their reason rather than hand-excluded — an exclusion list would need a second,
  unenforceable judgement about which `Errorf` "counts", and a human looking at a new one costs less.
- The audited `allOutputFilesExist` set is **8**, not 7: the seven rounds 4-6 left audited, plus
  `soleFinishedCandidate` from F1.

**The tripwire caught a real error while being written.** My first audited set said
`dispositionCandidate [verdictRespawnEligible]: 1`; the scan reported 2. It has two such returns (the
tracked-and-live-but-terminal record and the confirmed-dead-pane record). I had miscounted by hand,
and the test caught it — which is the most direct evidence available that it does what it claims.

### F5 (MEDIUM) — the standalone-drive smoke case no longer races the row it kills on

Commit `3b4d4483a`.

`TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed` killed the detached driver at an arbitrary
moment and then asserted the follow-up drive re-enters the same row. `waitForCurrentProducer` now
establishes that row first — polling the status file until it records `Discussion-Write` with the
machine still running — and an explicit `Fatalf` fires if the kill somehow landed elsewhere, so the
attribution assertions test attribution rather than racing it. The new helper's doc comment records
the failure it closes and that it predates this round.

## Sabotage proofs — every new test, and every prior round's

The brief asks for the tripwire to be sabotage-proved. I did that for every test this round touches,
and separately mutation-tested rounds 3-6's own regression tests to answer the brief's
test-coverage-soundness question. All mutations were applied in a scratch copy of the repo outside
the worktree (`<scratchpad>/mutant`), never to the worktree's own files.

| # | mutation | result |
|---|---|---|
| S1 | `soleFinishedCandidate` never fires (reverts F1) | FAIL — all three subtests of `TestAttach_ReedStateUnavailable_HarvestsFinishedRun` |
| S2 | drop `soleFinishedCandidate`'s exactly-one rule | FAIL — `…_StillRefusesWithoutAFinishedRun/two_running_records` |
| S3 | drop `soleFinishedCandidate`'s `runOutcomeRunning` gate | FAIL — `…_StillRefusesWithoutAFinishedRun/already_terminal_record` |
| S4 | revert F1, run the new SMOKE test on the real substrate | FAIL with the exact live envelope: `shuttle attach: no reed state file at …/.lyx — an absent strand table is not evidence any of the 1 matching run dir(s) are dead` |
| A | add an 8th UNGUARDED `return run.finalize(OutcomeDied, "")` to `Wait` | tripwire FAILS, naming it: `Wait [OutcomeDied]: found 1, audited 0` |
| B | add an unguarded `return verdictRespawnEligible` in a new `attach.go` function (F1's own shape) | tripwire FAILS, naming it: `sabotageNewExit [verdictRespawnEligible]: found 1, audited 0` |
| C | delete round 6's `dispositionCandidate` guard | the OTHER assertion FAILS: `dispositionCandidate: found 0, audited 1` |
| — | all sabotage removed | tripwire passes again |
| M1 | `classifyDeadlineExpiry` always returns `expired` (reverts round 4) | FAIL — both r4 regression tests |
| M2 | `finishedDespiteMechanismFailure` never fires (reverts round 5) | FAIL — both r5 regression tests |
| M3 | `dispositionCandidate`'s guard deleted (reverts round 6) | FAIL — `TestAttach_RunningRecordSatisfiedFileContract_HarvestsNotRespawn` |
| M4 | `started := run.attached` alone (reverts round 3's gap fix) | FAIL — `TestRun_Wait_AttachedButNeverStarted_StartupProbeStillRuns` |
| M5 | `VerifySeedOwnership` escalates `state.ErrDecode` (reverts `69886823e`) | FAIL — `TestVerifySeedOwnership` |

Sabotage A is the brief's own item 4, done exactly as specified: an 8th unguarded negative-verdict
return added in a scratch copy, the test confirmed to fail and name it, the addition removed, the
test confirmed to pass again.

## Tests added or changed

| file | what |
|---|---|
| `internal/shuttleengine/attach_test.go` | `TestAttach_ReedStateUnavailable_HarvestsFinishedRun` (3 cases: unreadable / absent `reed.json`, erroring `Status()`) and `TestAttach_ReedStateUnavailable_StillRefusesWithoutAFinishedRun` (3 cases: unsatisfied contract, already-terminal record, two running records). Each harvest case drives the reconstructed run all the way to `OutcomeDone` and asserts the run dir was cleaned up, not merely that `found` came back true |
| `internal/shuttleengine/completionsignal_enforcement_test.go` | NEW — `TestCompletionSignal_NegativeVerdictReturnSites`, `TestCompletionSignal_FileContractCallSites` |
| `internal/loomcli/smoke_attachprobe_test.go` | NEW test `TestSmokeSingleLLM_HarvestsAFinishedRunWithReedStateGone` (real hub, real reed session, real tmux pane, real `run.json`, ZERO provider subprocesses), plus `waitForOutputFile`, plus a rewritten file header covering both defects this file now guards |
| `internal/loomcli/smoke_test.go` | `waitForCurrentProducer` helper; `TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed` establishes its row before killing |

## Changed files

Production: `internal/shuttleengine/attach.go`, `internal/shuttleengine/wait.go`,
`internal/shuttleengine/doc.go`.
Tests: `internal/shuttleengine/attach_test.go`,
`internal/shuttleengine/completionsignal_enforcement_test.go`,
`internal/loomcli/smoke_attachprobe_test.go`, `internal/loomcli/smoke_test.go`.
Docs: `CONSTRAINTS.md`, `manifest/designs/loom.md`.
Reports: `_mill/loom-review-opus5-high-r7.md`, `_mill/loom-review-opus5-high-r7-fixer-report.md`.

`manifest/roadmap.md` deliberately untouched — this is a hardening pass, and the repo's own rule is
that the roadmap moves only for completing or adding a planned item.

## Commits (this branch, nothing pushed)

```
88f655bd9  loom: review notes — r7 opened, environment preflight + hermetic gates green
917d4c5d9  loom: review notes — F1 recorded …, F2/F3 doc-accuracy
544de3ce7  loom: review notes — thread A live-driven (13 validate-plan scenarios + real-delta drift)
f399b91f2  loom: review notes — F1 REPRODUCED LIVE (B5), round 6's fix re-confirmed live (B2), F2 disproved live (B4)
13e116d95  loom: review report — r7 Job 1 COMPLETE
41430ca78  loom: fix F1 (r7) — Attach's three reed-state gates harvest a finished run instead of refusing
00fe3ae12  loom: fix R1 (r7) — name the Completion Signal Invariant and pin it with a sabotage-proved tripwire
3b4d4483a  loom: fix F1 smoke guard + F5 (r7) — live-substrate guard for the reedless harvest, and de-race the standalone-drive case
```

## Verification — exact commands and results

Hermetic, run after the last fix:

```
go build ./...                                                          -> clean
go vet <the nine in-scope packages>                                     -> clean
go vet -tags smoke ./internal/loomcli/                                  -> clean
gofmt -l internal/shuttleengine/ internal/loomcli/                      -> no output
go test -count=5 <the nine in-scope packages + ./cmd/lyx/...>            -> all ok
go test ./...                                                           -> all ok (full repo)
```

Live smoke, real substrate, rebuilt binary:

```
go test -tags smoke ./internal/loomcli/... -run Smoke -count=1   x3      -> ok, 13/13 PASS, 0 SKIP, all three runs
```

Live driving of F1's own fix, real built binary rebuilt first
(`CGO_ENABLED=1 go build -o <scr>/bin/lyx ./cmd/lyx`), re-running the exact scenario that reproduced
the defect (review report scenario B5) against the same real hub:

```
lyx loom drive, crashed-but-finished run dir present, reed.json held absent
```

Before the fix: `{"error":"shedadapters: Discussion-Write (shuttle): shuttle attach: shuttle: attach:
no reed state file at …/.lyx …","ok":false}` — step hard-failed, run dir untouched, output files
un-archived.

After the fix, the durable trace sink reads:

```
shuttle: attach: harvesting a finished run despite an absent reed state file
shuttle: run attached
shuttle: cleanup: remove strand failed (non-fatal)
shuttle: run finished
loomshed: discussion artifacts failed validation
```

The finished run was harvested as `done` and the machine advanced to `Discussion-Validate` (which
correctly rejected the fixture's stub artifacts — that is the fixture's content, not a defect).

F5's fix, verified against the exact combination that failed 1-in-3:

```
go test -tags smoke ./internal/loomcli/ -run 'BurlerRound|SingleLLM_Harvests|AdvancesMachineFromExistingSeed' -count=1   x4  -> ok, 4/4
```

## Teardown

- `lyx reed down` on the live-driving fixture's session. Re-probing every socket under
  `/tmp/tmux-1000` afterwards: **0 live tmux servers**. `pgrep -af "lyx loom drive"`: **0**.
- The scratch git fixtures (bare remotes, the wired `demo3-HUB`, the mutation-testing copy of the
  repo) all live under this session's scratchpad, outside the worktree, and hold no live processes.
- Honest note, not a defect: `/tmp/tmux-1000` accumulates one socket FILE per hub fixture the smoke
  suite builds (101 after this round's runs). tmux does not unlink a socket when its server exits, so
  these are inert leftovers of servers that were correctly torn down — every one of them probes dead.

## Deferred

**Nothing.** All six items (F1, F2, F3, F4, F5, R1) are fixed, tested, and committed.

Two things were deliberately NOT done, both because the brief says so rather than because they were
skipped:

- The `AddStrand`/`run.json` "Accepted residual" was not re-opened. Four rounds agree it is
  structurally unclosable by reordering, and my own driving surfaced no new angle on it.
- The fabric weft-branch-naming bug (rounds 5-6) is not fixed — out of loom's scope. I tripped over
  it a third time while building this round's hub fixture and recorded the exact reproduction in the
  review report, per the brief's instruction to report it if driving trips over it.
