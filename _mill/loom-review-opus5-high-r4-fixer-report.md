# `loom` — round-4 fixer report (opus5-high-r4)

Companion to [loom-review-opus5-high-r4.md](loom-review-opus5-high-r4.md), which is the review
itself: 38 findings (2 BLOCKING, 11 MEDIUM, 15 LOW, 10 NIT).
The review was written, saved and committed in full before any production or test file was touched.

One finding per commit, on the branch, never pushed.
Every fix landed with `CGO_ENABLED=1 go build ./...`, `go vet`, the untagged suite and the
`integration`-tagged suite green over the packages it touched.

## How the work was split

I wrote the review myself: four read-only auditor subagents read in parallel, but I verified every
finding in the code before recording it, and I drove all four live scenarios end to end personally.

For the fixes I wrote both BLOCKING findings and roughly two thirds of the rest myself, and
delegated three disjoint per-package groups to subagents under precise briefs (finding named,
commit-per-fix, a test that would have caught the bug, green gates before committing).
I then read every delegated production diff myself before the final gates — recorded below where
that reading changed the disposition.

**Orchestrator note:** the table below originally omitted seven rows (R4-07, R4-11, R4-23, R4-25,
R4-26, R4-33, R4-34) whose commits had already landed on the branch under this same round's session
before this report was last saved — a paperwork gap, not a missing fix.
The orchestrator's independent verification found the gap via `git log`, confirmed each of the seven
commits genuinely implements its finding (production diff + regression test, same session
attribution as every other round-4 commit), and completed the table to match the branch's actual
state.

## Implemented

| Finding | Severity | Fix | Commit |
|---|---|---|---|
| R4-01 | BLOCKING | `RecordBatch` re-baselines the plan fingerprint immediately after `BindHandles` and after `DetectDrift`, ahead of every refusal between them; `webstercli`'s record-batch verb persists that re-baseline on the engine-error path via a new narrow `persistPlanFingerprintRebaseline` that writes only when the fingerprint actually changed | `01553ff7a` |
| R4-02 | BLOCKING | Same for `BeginBatch` (restamp right after `ValidateDispatch`) and `Run` (new `restampAndSaveFingerprint`, which persists its own); `webstercli`'s begin-batch verb persists on error too, and its factually-wrong "mutates State only on its success path" comment is corrected | `8da3ff89b` |
| R4-03 | MEDIUM | The post-batch mechanical pass is extracted into one shared `postBatchChecks` both terminal paths call, so `recover-batch` runs the done-checks, handle binding, scope guard, drift detection and re-baseline it used to skip entirely; a recovery `BatchState` now inherits the stuck fork's own `StartSHA` so the delta spans the whole bracket | `ffe622938` |
| R4-04 | MEDIUM | `Bouncer.Call` probes for a live judge on the judge spec's own three `OutputFiles` before either the clear or the replay branch acts; an attach harvests the judgment as this call's own settle, a not-found probe leaves both branches acting on unchanged state | `73a25ae78` |
| R4-05 | MEDIUM | The Plan-Review rubric no longer tells the judge that `Custom` escapes `bare-symbol-target` and `directory-target` — it escapes only the two group-scoped checks, and states so | `162af6ae2` |
| R4-06 | MEDIUM | The Plan-Write stencil's worked skeleton carries `**ImpactSummary:**`, so the one example the prompt supplies no longer models a card `card-missing-field` blocks | `7c87359ba` |
| R4-07 | MEDIUM | `lyx webster validate` now scopes its gate the same way `webster run` scopes its own, so validate no longer advertises parity with a check run never actually performs | `a204b4c24` |
| R4-08 | MEDIUM | `standalonestate.derive` validates the environment-supplied base: a relative `XDG_STATE_HOME` is ignored per the XDG spec, a relative `LOCALAPPDATA` or home is a loud error, and the returned `stateDir` is asserted absolute | `5f7388bd9` |
| R4-09 | MEDIUM | An adopted `LYX_TRACE_ID` is validated against the minted 16-lowercase-hex alphabet in both entry points, so it can neither escape the logs directory through `filepath.Join`'s cleaning nor make the retention sweep permanently blind | `1503d3fc6` |
| R4-10 | MEDIUM | New `reedengine.SanitizeSessionName` (owned by the package that owns the tmux identity rules) sanitizes standalone's session name, so a repo directory containing `.` or `:` no longer makes standalone mode unusable | `37d507b65` |
| R4-11 | MEDIUM | Standalone `recover-batch` boots its own reed session before spawning the recovery strand, instead of spawning a strand with no session for it to attach to | `e955d56d7` |
| R4-12 | MEDIUM | `bisect`'s deferred `RestoreBranch` error is surfaced rather than dropped, so a failed restore no longer leaves the operator on a detached mid-plan HEAD while the run reports success | `b4813d58c` |
| R4-13 | MEDIUM | `BurlerProducer` advances past the highest complete round only when that round carries a parsing verdict AND ledger; otherwise it hands control back to the Bouncer, spawning nothing, so a degraded judge no longer costs a whole fixer session and silently resets the ledger chain | `7d2a20d07` |
| R4-14 | LOW | All three planglyph gates fail on ANY validator error, not only the quarry-named one — the `errors.Is` conjunct let anything else be dropped and the plan reported clean | `21d67d13e` |
| R4-15 | LOW | The two `st.Batches` reads that dereferenced a present-but-nil entry are guarded, matching every other reader | `225d770fd` |
| R4-16 | LOW | `webster.yaml`'s four numeric knobs are validated at load, so a partial config fails naming the key instead of classifying every recovery batch dead on its first poll | `119904e58` |
| R4-17 | LOW | `bisect` verifies the LAST sha before searching and reports no offender when it passes, so an unattributable integration failure no longer names a card by arithmetic in the PR text; the single-element case is no longer special-cased either | `b4813d58c` |
| R4-18 | LOW | The integration stage splits into read / localize / record, with the bisect running unleased through a new `LocalizeIntegrationFailure` — the lease is no longer held across a block that runs the plan's whole verify command once per step | `24d4ff9ac` |
| R4-19 | LOW | Both failed terminals in `shedengine.Run` join the persist error with the producer error instead of replacing it | `3ede78ca4` |
| R4-20 | LOW | The durable sink's lazy arm runs under `sinkMu` (a plain guarded bool, not a `sync.Once` that is reassigned under that same mutex), closing a real data race on the `Once` value that could leave the pre-redirect sink path installed | `1c2043896` |
| R4-21 | LOW | `ScopeGuard` inspects `delta.Renamed`, so a rename touched outside the batch's declared targets is reported — quarry removes an exact pair's constituents from Created and Deleted, so it reached neither existing loop | `2b0d2dca1` |
| R4-23 | LOW | `--plan-dir`/`--stencils-dir` are resolved against cwd once, at the wiring boundary, instead of being stored verbatim and possibly relative | `c5e4641a8` |
| R4-24 | LOW | New exported `standalonestate.Normalize` gives the geometry builders the same symlink-resolved spelling `hash8` is computed from, so identity and session name cannot disagree across two spellings of one repo | `74be4ccee` |
| R4-25 | LOW | The "state dir nested in the target" refusal now fires at the wiring boundary, before the wrong cause could be blamed | `3bb2a4a42` |
| R4-26 | LOW | The standalone target is normalized to its repository root, once, at the CLI boundary, instead of defaulting to cwd with no git-root normalization | `af468b9d4`, doc `ea0f03584` |
| R4-27 | NIT | `CanonicalizeHandles` length-guards `quarry.Name`'s positional answer, reporting a mismatch as an infrastructure error instead of panicking before the echo check can run | `87d9a01f3` |
| R4-28 | NIT | `formatPlanFindings` calls `Finding.Error()` instead of re-deriving its layout, making its own "described identically" promise hold by construction | `39b88160e` |
| R4-29 | NIT | `loom.md` names a live check ID (`depends-on-order` is a format-3 ghost) and states which checks `Custom` does not escape | `6e8c3b990` |
| R4-30 | NIT | A producer that returned an error with no outcome records no history entry at all. Worse than a nit in the end: `loomengine`'s coherence check rejects any `history[].outcome` outside `{done, stuck}`, so an ordinary hard failure at either Preflight row wrote a value that made the status file permanently un-resumable. `shed.md`'s own append rule updated to name both no-verdict exceptions | `e02078dff`, `3be1d7159` |
| R4-31 | NIT | `prosa-symbol-target`'s detail names the shape rule the entry failed rather than asserting it is a symbol | `55e5eab55` |
| R4-32 | NIT | Plan-Write and Discussion-Write name a missing commit seam instead of nil-panicking on the Done path | `0394b97f1` |
| R4-33 | NIT | `webstercli/wiring.go`'s comment now states the sink redirect's real placement obligation (it does not in fact run before `resolveStandaloneTarget`/`standalonestate.Derive`), and pins it with a test | `75729758a` |
| R4-34 | NIT | Master's failure ladder gets a `card_not_done` rung, and a new test pins ladder coverage of every refusal flag `record-batch` can emit | `a70174e84` |
| R4-35 | NIT | The integration stage's warnings are logged on the loud done-over-a-failed-suite return instead of being discarded with the zero `RunResult` | `24d4ff9ac` |
| R4-36 | LOW | The entry-time reclaim logs the live agent it kills, per the Live-Substrate Spawn Observability rule | `260bf9dcb` |
| R4-37 | LOW | A failed reed liveness probe fails the reclaim instead of being read as "not live", which had left a leftover agent running beside its replacement. The pinned test asserting the old behaviour is updated with the reasoning for overturning it | `260bf9dcb` |

## Withdrawn after implementation

**R4-22 (LOW, `shedengine.validate` rejects only a self-referencing `OnDone`) — NOT a real finding.**
Implementing it broke `TestRun_OnDoneRoutesBackward`, which was the tell.
`manifest/designs/shed.md` states the decision outright and gives its reason:
a multi-producer `Done` cycle "is not statically infinite the way a self-referencing `OnDone` is —
any member may still exit via its own `OnStuck` — so a rule rejecting it would reject legitimate
backward jumps along with genuine mistakes", and the unbounded case is accepted explicitly a few
lines later with three stated reasons.
The general check does exist, at the right layer: `internal/shedcheck`'s `done-cycle` finding, which
generalises the length-1 rule at authoring time without breaking legitimate backward routing at run
time.
The change was reverted rather than forced. I verified this rejection myself against `shed.md` and
`internal/shedcheck` rather than taking it on report.

## Not fixed this round

**R4-38 (PLAUSIBLE, unreproduced) — the `stencilseed: commit seeded stencils failed` WARN.**
Observed once, live, on the first `lyx` invocation inside a freshly created worktree: a `git add` of
exactly the two stencils this campaign changed failed with `fatal: not a git repository`, which
means git ran with a cwd that was not the board.
`<hub>/_board` is a valid git repo and `fabricengine.BoardDir` composes correctly, so I could not
attribute it, and it did not reproduce on any later invocation.
It degrades to a WARN and the seeded files were already committed, so nothing was lost.
Recording it rather than guessing at a fix: it lives in `cmd/lyx/stencilseed.go` and
`internal/fabricengine`, outside the loom/glyph surface this campaign scopes, and a fix built on an
unreproduced symptom would be a guess. It wants its own task with a reproduction first.

## Tests added

Every fix above carries one, in the tier its subject belongs to. The load-bearing ones:

- **R4-01** — `TestRecordBatch_RestampsFingerprintEvenWhenDriftBlocks` (integration): one delta
  carrying both an exact-tier rename the pending card references (so `RewriteRefs` runs) and a
  deletion it also references (so the call blocks), asserting the plan file really was rewritten AND
  the fingerprint moved off its pre-call value. **Sabotage-proofed**: with the restamp moved back
  past the refusal the test fails with exactly the wedge message.
- **R4-02** — `TestBeginBatch_RestampsFingerprintEvenWhenPlanDrifts` (integration): a plan whose
  first card declares a draft handle spelled differently from what `quarry.Name` computes (so
  canonicalization rewrites it) and whose second names an unresolvable glyph (so the call blocks).
  **Sabotage-proofed** the same way.
- **R4-03** — `TestRecoverBatch_TerminalRunsTheSamePostBatchChecksAsRecordBatch` (a recovery
  reporting done over an unlanded Create target must be refused and stay non-terminal) and
  `TestRecoverSpawn_InheritsTheStuckForksStartSHA`.
- **R4-04** — five new attach-probe cases, including one asserting the live judge's own focus file
  survives byte-identical rather than being archived or overwritten.
- **R4-08 / R4-09 / R4-10 / R4-20 / R4-24** — each **verified against the pre-fix code**: the
  trace-ID test placed `pwned-<pid>.log` outside the logs directory, `go test -race` reported a real
  `WARNING: DATA RACE` on the sink's `sync.Once`, and the symlink test showed two session names on
  one socket.
- **R4-17** — `TestBisectAndEscalate_UnattributableFailureBlamesNoCard` against a real scratch repo.
  **Sabotage-proofed**: without the fix it names `03-batch3`.

## Changed files

Production: `internal/websterengine/{recordbatch,beginbatch,recoverbatch,runlevel,integration,fingerprint,render,strand,config,state,doc}.go`,
`internal/webstercli/{cli,recordbatch,beginbatch,recoverbatch,status,validate,wiring}.go`,
`internal/burlercli/{recoverbatch,wiring}.go`,
`internal/shedadapters/{bouncer,burler,bouncerfiles,round,doc}.go`,
`internal/shedengine/run.go`,
`internal/planglyph/{scope,handle,repo}.go`,
`internal/planparser/validate.go`,
`internal/loomshed/{planvalidate,planwrite,discussionwrite}.go`,
`internal/loomcli/validate.go`,
`internal/logger/{sink,trace}.go`,
`internal/standalonestate/standalonestate.go`,
`internal/standalonegeom/{reedgeom,doc}.go`,
`internal/reedengine/server.go`.

Contracts and docs: `contracts/stencils/loom/{loom-rubric-plan-review,loom-template-plan}.md`,
`contracts/stencils/webster/webster-template-master.md`,
`manifest/designs/{loom,shed}.md`.

Tests: the matching `_test.go` file beside each of the above, plus the new
`internal/standalonegeom/reedgeom_symlink_integration_test.go`.
