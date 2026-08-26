# `loom` — crucible round 2 fixer report (tag `opus-high-r1`)

Companion to [`loom-review-opus-high-r1.md`](loom-review-opus-high-r1.md).
That file is the independent review — findings, evidence, and the live-driving account.
This one is the record of Job 2: what was implemented, what was deliberately not, and how each
change was verified.

## Summary

Fifteen findings. **Twelve fixed and committed one-per-finding; three recorded
NOT-FIXED-THIS-ROUND**, plus three half-findings inside F1, F2 and F12 carried the same way.

Every fix landed as its own commit on `loom-crucible-hardening-round2`, green before it was
committed. Nothing was pushed.

| Id | Severity | Status | Commit |
|---|---|---|---|
| F12 | BLOCKING | fixed | `b8fa9239` |
| F0 | BLOCKING | fixed | `f1b30b51` |
| F10 | MEDIUM | fixed | `b8606689` |
| F1 | MEDIUM | fixed (narrow half; merge half deferred) | `251929ad` |
| F2 | MEDIUM | fixed (message half; PATH half deferred) | `9f5b9b33` |
| F14 | MEDIUM | fixed | `e9bde678` |
| F13 | MEDIUM | **NOT-FIXED-THIS-ROUND** | recorded in the review |
| F4 | LOW | fixed | `84708880` |
| F8 | LOW | fixed | `c6a4efad` |
| F3 | LOW | fixed | `454ef357` |
| F6 | LOW | fixed | `643c1d11` |
| F9 | LOW | fixed | `a7314d79` |
| F7 | LOW | fixed | `9d1db0d8` |
| F11 | LOW | **NOT-FIXED-THIS-ROUND** | recorded in the review |
| F5 | NIT | fixed | `f30c2cf8` |

## What was implemented

### F12 (BLOCKING) — `Finalize` refused on loom's own status file

`Shed` rewrites `_lyx/loom/status.json` on every transition; it is tracked in the weft and
committed exactly once, at bootstrap; `fabricengine`'s merge guard refuses any tracked
modification on either side of the pair. So the last row of every run refused, deterministically,
after every session in the run had been paid for, with no `on_stuck` and no recovery but a human.

Both landing producers now commit it through a new injected loop-owner seam,
`landingshed.Deps.CommitStatus`, called immediately before each producer's merge — inside `Call`,
not once at bootstrap, because a resumed run persists again right before the producer runs (the
F10 interaction, handled deliberately). `internal/loomcli`'s `landingDeps` fills it with a closure
mirroring the existing `CommitDiscussion`/`CommitPlan` shape exactly, so `landingshed` stays
ignorant of where loom's status file lives. `Publish` gets it too: it performs its own merge-in
whenever a pull request is required and would hit the identical guard.

`manifest/designs/loom.md`'s cross-machine-resume claim is corrected in the same change — only
the seed and this checkpoint are ever committed, so that documented feature does not work today
and the doc no longer says it does.

Changed: `internal/landingshed/{deps,finalize,publish}.go`, `internal/loomcli/landingdeps.go`,
`manifest/designs/loom.md`, `internal/landingshed/commitstatus_test.go` (new).

### F0 (BLOCKING) — duplicate agents on a resume inside any review segment

`BurlerProducer.Call` and both of `Bouncer`'s spawn paths now probe `shuttleengine.Attach` with
the step's own `OutputFiles` before archiving anything, exactly as `SingleLLMProducer` already
did. No change to `burlerengine` was needed: it already declares `[ReviewPath, FixerReportPath]`
as its shuttle run's `OutputFiles`, which is the set `Attach` matches on. The archive moved onto
the respawn branch in both Bouncer paths, since archiving renames the very files a live agent is
about to write.

The seam is **required**, not optional, on `NewBurlerProducer` and in `burlerRoundEntry`, so a
wiring slip fails at construction rather than silently restoring the defect.
`internal/shedadapters/doc.go`'s Limitations paragraph, which recorded the gap as a deliberate
scope call, is retired and replaced with a section describing the probe every adapter now
performs; `manifest/designs/loom.md`'s crash-recovery ladder now names which producers implement
it and how, instead of stating it as an unqualified module-wide invariant.

Changed: `internal/shedadapters/{burler,bouncer,doc}.go`, `internal/shedrecipe/entries_burler.go`,
`manifest/designs/loom.md`, plus `internal/shedadapters/attachprobe_test.go` (new),
`internal/loomcli/smoke_attachprobe_test.go` (new), and updates to three existing test files.

### F10 (MEDIUM) — a resumed run described itself as halted while already spawning

`shedengine.Run` now persists `StateRunning` with an empty error after the pause check passes and
before the producer call, and only when the state just read is not already `running` — so every
step after a resume keeps the loop's one-persist-per-iteration shape.
`manifest/designs/shed.md`'s step list documents it.

Changed: `internal/shedengine/run.go`, `manifest/designs/shed.md`,
`internal/shedengine/run_pause_test.go`.

### F1 (MEDIUM) — an emptied list was unrepresentable

`yamlengine.MissingKeys` now requires a list-valued key to be PRESENT rather than at least as long
as the template's own list. Every non-sequence leaf keeps the exact-match rule, and the reconcile
MERGE is untouched. `CONSTRAINTS.md`'s Config Strictness Invariant states the rule and records the
merge's own truncation as a known gap.

Changed: `internal/yamlengine/reconcile.go`, `CONSTRAINTS.md`,
`internal/yamlengine/reconcile_test.go`.

### F2 (MEDIUM) — the stencil-downgrade warning named no remedy

The dev-mode refusal now says producers will read the OLDER on-disk copy and names
`lyx stencil sync`, with the stencil's path. `manifest/designs/loom.md` records the underlying
PATH hazard and the operator-side mitigation.

Changed: `internal/stencilstore/reconcile.go`, `manifest/designs/loom.md`,
`internal/stencilstore/reconcile_test.go`.

### F14 (MEDIUM) — loom's own smoke suite shipped broken

The fixture is provider-free by construction, `AdvancesMachineFromExistingSeed` no longer requires
a provider session to complete, the died-child case is re-aimed at the disposition that ships and
renamed `TestSmokeBootstrap_DiedDriverProceedsToHandoverAndLogsWhy`, and the leaking test
registers a teardown.

Changed: `internal/loomcli/smoke_test.go`.

### F4, F8 (LOW) — two silent faults

`Batchifier` and the `Webster` row now log the `batcher.Active` error they discarded; the
`Bouncer` logs the approved generation it discards before archiving it.

Changed: `internal/loomshed/{batchifier,webster}.go`, `internal/shedadapters/bouncer.go`,
`internal/loomshed/gatefindings_test.go`, `internal/shedadapters/bouncer_clear_test.go`.

### F3, F5, F6, F7, F9 (LOW/NIT) — five accuracy fixes

- F3: both places asserting the Bouncer shares `BurlerProducer`'s pair predicate now state what it
  actually tests and why the asymmetry is safe today.
- F5: `burlerRoundFileSet`'s two dead parameters now qualify its errors.
- F6: `status --watch`'s `Long` text, its flag help, and `docs/overview.md` now describe
  print-on-change.
- F7: `docs/overview.md`'s `loom.yaml` key list names `review`/`review_timeout_min`.
- F9: `lyx loom --help` no longer claims the shed engine "already drives Hardener", and its phase
  summary names the review segments.

## What was deliberately NOT fixed, and why

Each is recorded in full in the review report, with its reproduction, for the orchestrator to
spin into its own task.

1. **F13 (MEDIUM) — loom's status file is a merge subject on the landing merge.** Every available
   fix is a durable design decision about how per-task orchestration state coexists with a tree
   the task merges into and out of: a `merge=ours` driver seeded via `.gitattributes` plus repo
   git config, dropping the file from git and finding another carrier for cross-machine resume, or
   excluding it from the landing merge's pathspec. Each reaches fabric, loom's seeding, and the
   status-file contract at once. **This one interacts with F12's fix and must be read with it** —
   see the review's own note.
2. **F11 (LOW) — the mandated card shape is forced onto `Custom`.** Every fix is a plan-format
   contract change (a second type label, a new type, or per-target classification in
   `checkPathMissing`) reaching `planparser`, the plan stencil, the Plan-Review rubric,
   `plan-card-format.md`, and webster's consumption of `Targets`.
3. **F1's second half — `lyx config reconcile --apply` truncates a list longer than the
   template's.** Fixing it means changing the sequence merge model for every module's config in a
   shared leaf package, with its own test matrix. The narrow half that unblocks loom (an emptied
   list now LOADS) is fixed.
4. **F2's second half — a spawned agent should reach the binary that spawned it.** Belongs in the
   shuttle/reed spawn layer, outside this module. Note for whoever takes it: prepending `.dev-bin`
   to the agent's environment PATH is **not** sufficient, measured — the agent's shell re-sources
   the user profile and re-orders PATH back (`PATH=<dev-bin>:$PATH bash -lc 'command -v lyx'`
   resolves to `~/.local/bin/lyx`).
5. **F12's second half — continuous status durability.** A git commit per producer transition is a
   `Shed` persistence-policy decision with real cost; it is what the documented cross-machine
   resume actually needs.

Nothing was left unfixed for being small or low-severity. Every NIT and LOW finding is closed.

## Verification

### Hermetic gates — green after every commit, and at the end

```
go build ./...                                                    -> ok
go vet <the nine module packages>                                 -> ok, no diagnostics
go vet ./...                                                      -> ok, no diagnostics
go vet -tags smoke ./internal/loomcli/...                          -> ok
go test -count=5 <the nine module packages> ./cmd/lyx/...          -> 10 packages ok, zero FAIL
go test ./...                                                      -> whole repo, zero FAIL
```

### Sabotage proofs — every new guard was shown to detect

A green test that never executes the code is indistinguishable from a fix, so each new guard was
neutered and re-run:

- **F0:** forcing the attach probe's `found`/`attached` to false failed exactly the three
  attach-branch hermetic cases and nothing else, and failed the new smoke test too. Restored,
  re-verified green.
- **F10:** disabling the resume write failed
  `TestRun_ResumeWritesRunningBeforeCallingTheProducer` and left every other shedengine case
  passing. Restored, re-verified green.
- **F14:** proven pre-existing rather than self-inflicted by checking the tree out at `980f2d48`
  and reproducing both failures there.

### Live-substrate verification — the real CLI, driven directly

Re-deployed with `deploy-dev` before each block. Operator attach commands for anything started:
`<worktree>/.dev-bin/lyx reed status` and `... reed attach`, run from
`/home/knatte/Code/loomyard/live-r2/hub2/tinytool2-HUB/greet-suffix`.

- **F12 — from `stuck` to `done`, nothing else changed.** With the fixture rewound to blocked at
  `Finalize` and the status file tracked-dirty (the exact state that produced
  `merge-in failed: fabricengine: merge preconditions failed: worktree dirty` during the pipeline
  run), `lyx loom drive` now returns `{"halted_producer":"Finalize","outcome":"done"}`, and the
  weft carries `79a3df4 loom: status checkpoint for greet-suffix` immediately before the merge
  commit.
- **F0 — the same crash, one agent instead of two.** A real `Webster-Burler` round was staged and
  driven (`burler:2:b2a2c230`, pane `%17`, a real provider session), the driver hard-killed
  mid-round, and `lyx loom run` used to resume. Before the fix this produced two live agents on
  two rows; now:
  ```
  BEFORE  %17 2161859 claude
  $ kill -9 <driver>; lyx loom run
  AFTER   strands: loom-status (%0), burler:2:b2a2c230 (%17, live)   <- the SAME one
          %17 2161859 claude                                          <- same pane, same pid
          new driver 2162299 alive and waiting
  ```
- **F10 — proven from the durable record.** With the fixture rewound to `blocked` at `Finalize`,
  the status snapshot committed from *inside* `Finalize` reads `state: "running"`, `error: ""`,
  `activity.wait: ""`, `current_producer: "Finalize"` — the four properties the hermetic test
  asserts, confirmed against a real git commit.
- **F1 — an emptied list loads.** With `require_pr_to_base: []`, `lyx loom drive` reaches its own
  `no status file` refusal instead of the loader's `missing keys: require_pr_to_base[0]`.
- **F2 — the new message appears in a real driver log:**
  `stencilstore: dev build does not refresh an untouched stencil; producers will read the OLDER
  on-disk copy -- run "lyx stencil sync" to force-refresh it ... path=...`
- **F6, F7, F9 — re-read from the deployed binary's own `--help` output**, all three corrected.

### Live smoke — every named test, one exact function at a time

Never a bare `-run Smoke`. The banned `internal/burlerengine/smoke_cluster_test.go` tests were not
run.

```
TestSmokeBootstrap_BringsUpSessionStrandAndDriver                        ok   1.14s
TestSmokeBootstrap_SecondInvocationDoesNotSpawnASecondDriver             ok   1.18s
TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed                 ok  61.47s
TestSmokeDriveStandalone_RefusesOnNeverSeededPair                        ok   0.75s
TestSmokeDriveStandalone_FailureBeforeFirstPersistLeavesNonEmptyLog      ok   0.92s
TestSmokeFabricAdd_RunLauncherExistsThenGoneAfterRemove                  ok   0.17s
TestSmokeBootstrap_CleanlinessOrderingAfterSeedCommit                    ok   0.18s
TestSmokeBootstrap_OriginRecordSelfHealsAfterCrashBetweenWriteAndCommit  ok   1.03s
TestSmokeBootstrap_ConcurrentSpawnHandshakeYieldsOneDriver               ok   1.06s
TestSmokeBootstrap_DiedDriverProceedsToHandoverAndLogsWhy                ok   1.24s   (renamed, F14)
TestSmokeBurlerRound_AttachesToALiveRoundInsteadOfRespawning             ok   3.38s   (new, F0)
TestSmokeBurlerRoundToyFixture (internal/burlerengine)                   ok  33.55s   (1 real session)
```

### Teardown

Zero stray tmux servers and the provider process count back to the host's own baseline of two
(the operator's long-running sessions, not mine) after the full sweep. Two tmux servers leaked
mid-round by the pre-existing test bug in F14 were killed by hand and that leak is now fixed.

The live fixture hub is left in place under `/home/knatte/Code/loomyard/live-r2/` rather than
deleted, since it holds this round's evidence — the completed run's status file, the plan, the
review artifacts, and the merged parent branch. It is outside every worktree and can be removed
whenever the orchestrator is done reading it.

## Changed files

Production:
`internal/landingshed/{deps,finalize,publish}.go`,
`internal/loomcli/{landingdeps,status,cli}.go`,
`internal/loomshed/{batchifier,webster}.go`,
`internal/shedadapters/{burler,bouncer,doc}.go`,
`internal/shedengine/run.go`,
`internal/shedrecipe/entries_burler.go`,
`internal/stencilstore/reconcile.go`,
`internal/yamlengine/reconcile.go`.

Docs: `manifest/designs/loom.md`, `manifest/designs/shed.md`, `docs/overview.md`,
`CONSTRAINTS.md`. `manifest/roadmap.md` deliberately untouched — this round added no planned
milestone.

Tests: `internal/landingshed/commitstatus_test.go` (new),
`internal/shedadapters/attachprobe_test.go` (new),
`internal/loomcli/smoke_attachprobe_test.go` (new, `//go:build smoke`),
plus `internal/loomcli/smoke_test.go`, `internal/loomshed/gatefindings_test.go`,
`internal/shedadapters/{burler,singlellm,bouncer_clear}_test.go`,
`internal/shedengine/run_pause_test.go`, `internal/shedrecipe/entries_burler_test.go`,
`internal/stencilstore/reconcile_test.go`, `internal/yamlengine/reconcile_test.go`.
