# `loom` — independent review, ROUND 4 (opus5-high-r4) — FINAL SAFETY PASS

> Clean-room round-4 review of the quarry-glyph-plan-alphabet surface (PR #230) plus the
> standalone-webster material (`#004`) merged into the same branch.
> Worktree: `/home/knatte/Code/loomyard/wts/crucible-loom-glyph-hardening`, branch
> `crucible-loom-glyph-hardening`, HEAD at review start `8e8c61b01`.

**STATUS: IN PROGRESS** — findings and test log appended incrementally (crash-resilience discipline).

## Executive summary

_(written last)_

## Findings

Severity counts: **2 BLOCKING, 11 MEDIUM, 13 LOW, 9 NIT — 35 total.**

### BLOCKING

#### R4-01 — a sanctioned plan rewrite in `RecordBatch` is never re-stamped when the same call blocks, wedging the run unrecoverably — CONFIRMED

`internal/websterengine/recordbatch.go:232`, `:240-242`, `:270-273`, `:289-291`, `:297`;
`internal/webstercli/recordbatch.go:117-126`.

`planglyph.BindHandles` (:232) and `planglyph.DetectDrift` (:270) each perform a
`planparser.RewriteRefs` **on disk** (`planglyph/handle.go:343`, `planglyph/drift.go:184`) and can
return a blocking finding — or a hard error — from the *same* call.
The two signals are independent: bind substitutions come from `delta.Created`, blocking drift comes
from `delta.Deleted` intersected with the still-pending plan.
`restampFingerprint` sits at :297, **after** every one of those blocking returns.
`webstercli/recordbatch.go` releases the lease and returns without `SaveState` on any error, so
`state.json` keeps fingerprint `F1` while the plan directory on disk is now `F2`.

Concrete scenario, batch 2 of a 3-card plan:
card 2 is a `Create` declaring a `plan:` handle, so `BindHandles` binds it and rewrites every card
file; the same fork also removed a helper card 3 still references, so `DetectDrift` returns
`plan-references-deleted-symbol` (blocking); `RecordBatch` returns `ErrCardNotDone` at :290 and the
restamp never runs.
Every subsequent `lyx webster begin-batch` now fails `ErrFingerprintMismatch`
(`beginbatch.go:210-212`), and so does `lyx webster run` (`runlevel.go:428-431`).
The advised recourse does not recover: `--fresh` re-inits state with zero batches, after which
`runlevel.go:466` calls `ValidateDispatch` with an **empty** completed set, so every already-built
`Create`/`Delete`/`Rename` card reports blocking — precisely what `ValidateDispatch`'s own doc
comment says the scoping exists to prevent — and `runlevel.go:476-482` refuses the run outright.
The run is unrecoverable short of hand-editing `state.json`.

Fix: restamp immediately after each rewriting call returns, regardless of findings, **and** persist
the state on the engine-error path in the CLI verb — the restamp is worthless if it is only ever in
memory on a path that never saves.

#### R4-02 — same class in `BeginBatch` and `Run`: `ValidateDispatch` canonicalizes handles, then the blocking return jumps over the restamp — CONFIRMED

`internal/websterengine/beginbatch.go:221-241`, `internal/websterengine/runlevel.go:466-491`,
`internal/webstercli/beginbatch.go:105-108`.

`planglyph.ValidateDispatch` → `resolvePass` → `CanonicalizeHandles` → `planparser.RewriteRefs`
(`planglyph/planglyph.go:173`), and `resolvePass` then *continues* to `statusFindings` /
`createFindings` / `resolveContainment` — so "rewrote the plan" and "reported a blocking finding"
co-occur routinely.
`beginbatch.go:234-236` returns `ErrPlanDrifted` before the restamp at :241.
`runlevel.go:476-482` has the identical shape.

`webstercli/beginbatch.go`'s comment — "BeginBatch mutates deps.State only on its success path, so
the lease is released with no SaveState … on every error branch below" — is factually wrong:
:241 mutates `State.PlanFingerprint` mid-call, and is then followed by eight more fallible steps
(`findBatch`, reports `MkdirAll`, the pre-existing-report refusal, `headSHA`, role lookup,
`RenderForkPrompt`, the prompt `WriteFile`, `removeStrandIfLive`, `Injector.Inject`).
Any of those failing discards a restamp whose on-disk cause persists.

At batch 1 this is recoverable via `--fresh`; at batch ≥2 it is not, and batch ≥2 is reachable —
`DetectDrift`'s exact-tier repair can rewrite a pending `Rename` card's `Old` side, which changes
the declaration `renameDeclSource` derives for that card's `New`-side handle, producing a
non-identity canonicalization at the *next* begin-batch.
This also makes `webster-template-master.md`'s promise that a plan-drift refusal "is fully resumable
later with `lyx webster run`" untrue on any refusal where canonicalization also fired.

### MEDIUM

#### R4-03 — `recover-batch` marks a batch terminal without any of `record-batch`'s mechanical checks — CONFIRMED

`internal/websterengine/recoverbatch.go:244-258` (`PersistRecoveryTerminal`) vs
`recordbatch.go:200-308`.

A recovery batch reaching `status: done` is marked `Terminal` on the strength of the recovery
strand's self-reported status plus the head-SHA cross-check alone.
It never runs `planglyph.DoneChecks`, `BindHandles`, `ScopeGuard`, `DetectDrift`, the dirty-worktree
check, or `restampFingerprint`.
The consequence specific to the glyph alphabet: a card recovered this way **never binds its
handles**, so every later card keeps referencing an unbound `plan:` handle for the rest of the
plan's life — and `DetectDrift`'s `targetCards` index keys on the raw ref, so drift against that
symbol becomes invisible to every later batch.
`contracts/stencils/webster/webster-template-master.md:105` tells Master "`recover-batch` returns a
terminal `status: done` → move on to the next batch", so this is a routine path, not an exotic one.

#### R4-04 — `Bouncer.Call`'s clear and replay branches archive/write with no attach probe, against the ladder both docs pin — CONFIRMED

`internal/shedadapters/bouncer.go:187-207` (clear) and `:222-225` (replay), vs
`internal/shedadapters/doc.go:158-161` and `manifest/designs/loom.md:349-352`.

`judgedVerdict(n)` is satisfied by verdict + ledger alone (`bouncer.go:249-275`), deliberately
excluding the focus file — but the judge spawn declares **three** outputs
(`bouncer.go:530-534`: verdict, ledger, `focus(n+1)`), so there is a real window in which the first
two are on disk, the third is not, and the judge agent is still alive.
`kill -9` the driver in that window:

- APPROVED → :202 `archiveRunDir` renames the run directory out from under the live judge, then
  falls through to `seedCall`, whose probe uses `OutputFiles: [focus(1)]` and therefore cannot match
  the live judge's persisted `[verdict(n), ledger(n), focus(n+1)]` — a second agent is spawned over
  a live one.
- BLOCKING → :223 `settle(ctx, n, false)` calls `ensureFocus(round+1)` (`:366`), writing a synthetic
  focus file at a path the live judge declared as an output, then returns `Stuck` to the Burler row,
  whose own probe also cannot match. Two agents in one segment again.

Both branches are outside the seed and judge passes the docs enumerate, and `doc.go:158` states flatly
"The probe always runs BEFORE the archive, in all three attaching adapters."
`bouncer_clear_test.go` has no live-agent case.

#### R4-05 — the Plan-Review rubric instructs the judge to hunt a state `Plan-Validate` already blocks — CONFIRMED

`contracts/stencils/loom/loom-rubric-plan-review.md:51`.

The rubric tells the judge that, "since the glyph alphabet's classification checks bind a card's flat
`Targets`/`Uses` exactly as `path-missing` does", a `Custom` card escapes `bare-symbol-target` and
`directory-target` too, "so a mistyped `Custom` card silently escapes four checks".
The premise is inverted.
`path-missing` is *group-scoped* on targets (spec row 26), which is exactly why `Custom` escapes it;
`checkBareSymbolTarget` (`internal/planparser/validate.go:388-414`) and `checkDirectoryTarget`
(`:424-453`) iterate `[][]string{c.Targets, c.Uses}` with **no group scoping at all**.
Spec row 9 says "under any card type" and row 26 closes it: `Custom` is exempt "from nothing else".
The state the reviewer is told to hunt is unreachable — `Plan-Validate` gates before `Plan-Bouncer`
and blocks on both IDs — so the instruction can only produce a hallucinated finding, in direct
contradiction of the same rubric's own "do not flag anything `Plan-Validate` already checks" rule.
`manifest/designs/loom.md:196`, the source this was transcribed from, still says **two** checks.

#### R4-06 — the Plan-Write stencil's worked skeleton is a card that fails `card-missing-field` — CONFIRMED

`contracts/stencils/loom/loom-template-plan.md:183-190`.

The skeleton hands `Plan-Write` an `**Edit:**` card carrying `**Intent:**` and no
`**ImpactSummary:**`, while the same file states the rule correctly 60 lines earlier (`:125`) and
`internal/planparser/validate.go:749-773` makes the omission a hard finding.
An implementation card copied from the one worked example the prompt supplies therefore blocks
`Plan-Validate`, which Stucks back to `Plan-Write`, burning a bounce, a full plan-directory rotation
and a fresh LLM spawn for a defect the prompt itself demonstrated.

#### R4-07 — `lyx webster validate` advertises parity with a gate `webster run` does not run — CONFIRMED

`internal/webstercli/validate.go:76-79` vs `internal/websterengine/runlevel.go:466`.

The help text says the verb runs "the SAME gate `lyx webster run` runs automatically before ever
forking an implementer", but the verb calls `planglyph.Validate` (whole plan, plus `plan-unapproved`)
while `Run` calls `ValidateDispatch(..., completedCards(...))` (pending-scoped, approval enforced
separately at `runlevel.go:352`).
Mid-run, every completed `Create` target now resolves, so the Create inversion fires
`create-already-exists`, and every completed `Delete`/`Rename`-old fires `glyph-not-found` — all
blocking.
The verb exits 1 over a plan `webster run` resumes without complaint: exactly the wedge
`runlevel.go:457-465` documents having fixed for `Run`, left unfixed on the verb that claims to be
the same gate.

#### R4-08 — a relative state-home env var puts the whole standalone state tree, trace logs included, inside the target repo — CONFIRMED (live repro)

`internal/standalonestate/standalonestate.go:52-92`; consumed at
`internal/webstercli/wiring.go:160,169` and `internal/burlercli/wiring.go:142,149`.

`derive` validates `target` for absoluteness but never validates the env-supplied base, so a relative
`XDG_STATE_HOME` / `LOCALAPPDATA` / `HOME` yields a relative `stateDir` that both CLIs resolve
against the process cwd — the operator's repository.
Reproduced: `cd <repo> && XDG_STATE_HOME=.relstate lyx burler run --profile /nonexistent.yaml`
created `<repo>/.relstate/lyx/<h8>/_lyx/stencils/…` (36 files) and
`<repo>/.relstate/lyx/<h8>/.lyx/logs/trace-….log` inside the repo — exactly the
untracked-logs-in-the-target-repo outcome `internal/standalonegeom/logsdir.go` exists to prevent.
`shuttleengine.NewDetachedRunner`'s own `IsAbs` check (`run.go:124`) is far too late: it only sets
`toldErr`, long after the directories were created.

#### R4-09 — `LYX_TRACE_ID` is adopted unvalidated and interpolated into a filename — CONFIRMED (live repro)

`internal/logger/trace.go:36-38`, `internal/logger/sink.go:112-117`, `internal/logger/retention.go:25`.

Three components disagree about the ID's alphabet and nothing enforces it.
`filepath.Join` cleans the composed name, so separators and `..` in the adopted value walk the write
out of the logs directory: `LYX_TRACE_ID='ci-run/../../pwned'` wrote `<stateDir>/.lyx/pwned-<pid>.log`,
one level above `.lyx/logs/`; a longer chain escapes the state directory entirely.
Independently, any adopted ID that is not exactly 16 lowercase hex produces a filename
`traceFilePattern` never matches, so `Sweep` skips it forever — neither the age bound nor the
newest-50 bound ever applies.
`MintOrAdoptAndExport` re-exports the value, so one bad value propagates to every child process.

#### R4-10 — standalone mode cannot run against a repository whose directory name contains `.` or `:` — CONFIRMED (live repro)

`internal/standalonegeom/reedgeom.go:29`.

`SessionName: filepath.Base(target) + "-" + hash8` takes the human-readable half raw, while
`reedengine.validateToldTmuxIdentity` refuses `.` and `:`.
Against a target named `my.repo` the run dies with "rename the worktree directory … so its name
carries no `.:`".
Hub worktree names are lyx-created and slug-shaped so this never bites there, but standalone's whole
premise is "point it at any plain checkout the operator already has", and a dot in a repo directory
name is routine (`foo.js`, `site.com`, `app.git`).
`reedengine.ServerName` already sanitizes its own human-readable half (`socketSafeBase` +
`truncateAtRuneBoundary`); the `hash8` suffix carries the identity, so sanitizing costs nothing.

#### R4-11 — standalone `recover-batch` spawns a cold strand but never boots the reed session — CONFIRMED

`internal/webstercli/recoverbatch.go:118` vs `internal/webstercli/run.go:101-106`.

The `#004` reed-bring-up fix wired `c.reedUp` into `run` only.
`recover-batch` is the other verb that calls `runner.Start` (a fresh cold recovery strand →
`reed.AddStrand`, which requires a live session) and it never calls `c.reedUp`.
After a standalone run ends `asking`/`died` and its session is gone,
`lyx webster recover-batch N --wait 8m` fails inside `AddStrand` with reed's "no reed session" error,
whose advised recourse (`lyx reed up`) is hub-only — precisely the impossible-recourse failure the
`#004` round filed, still open on this path.

#### R4-12 — `bisect` swallows `RestoreBranch`, leaving the worktree detached while the run reports success — CONFIRMED

`internal/websterengine/integration.go:111-113`.

`bisect` checks out card SHAs detached in the live worktree.
If `RestoreBranch` fails (a dirty tree left by a verify command, a lock, a ref problem) the error is
dropped, `BisectAndEscalate` returns nil, and the operator's worktree is silently left detached at a
mid-plan commit with no diagnostic anywhere.
Nothing on the next `lyx webster run` entry checks for or repairs a detached HEAD.

#### R4-13 — `BurlerProducer` advances past a round the Bouncer never judged — CONFIRMED

`internal/shedadapters/burler.go:266-270`, `internal/shedadapters/bouncer.go:281-287`.

`round := highestCompleteRound(runDir) + 1` derives the round purely from which review/fixer pairs
exist, with nothing recording whether the Bouncer ever *judged* the highest one.
Every `degrade` exit in `judgeCall` returns `Stuck` (an unreadable report, an unreadable template or
rubric, a prompt-fill failure, an attach-probe failure, an archive failure, a shuttle run error, a
non-`OutcomeDone` outcome, a verdict/ledger that did not parse), and the recipe routes that `Stuck`
to the Burler.
So one transient judge fault costs a full extra fixer round — a real LLM session — *and* breaks the
ledger chain: round N+1's `previousLedger` reads `ledger(N)`, finds it absent, logs
"previous ledger unreadable" and falls back to `"(none)"` (`bouncer.go:500-514`), silently resetting
the finding-identity carry-forward the judge uses to decide APPROVED-versus-BLOCKING across rounds.
If the judge fault is deterministic the segment burns its whole budget on fixer rounds no judge reads.

### LOW

#### R4-14 — three gates drop a non-`ErrQuarryUnavailable` planglyph error and can report the plan clean — CONFIRMED (latent)

`internal/loomshed/planvalidate.go:109`, `internal/loomcli/validate.go:126`,
`internal/webstercli/validate.go:97` all read
`if err != nil && errors.Is(err, planglyph.ErrQuarryUnavailable)`.
Every `resolvePass` error path is wrapped today, so the conjunct is currently redundant — but the
shape is the wrong default for a gate: the moment any planglyph path returns an unwrapped error,
`err` is dropped and, with an empty findings set, `planValidate.Call` returns `Done` with the plan
directory as its pointer, i.e. "the plan is clean" for a validator that failed.
That is verbatim the failure mode `internal/planglyph/repo.go:28-31` names as deliberately rejected.
`runlevel.go:467-475` handles the identical call correctly and is the model.

#### R4-15 — two `st.Batches` reads lack the nil-value guard every other site uses — CONFIRMED

`internal/websterengine/render.go:338-340`, `internal/webstercli/status.go:64-71`.
`completedCards`, `verifyEveryBatchDone`, `accumulatedCardSHAs`, `reclaimEntryTimeStrands` and
`predecessorDigestLine` all check `bs == nil`; these two do not.
A `state.json` containing `{"batches": {"3": null}}` panics `lyx webster status` and
`RenderProgress` — the latter inside `Run`'s Master prompt render, taking down the run rather than
surfacing a diagnosable error, against `LoadState`'s "fail loud, never guess" promise.

#### R4-16 — `webster.yaml`'s four numeric keys are neither validated nor defaulted — PLAUSIBLE

`internal/websterengine/config.go:52-76`.
`LoadOrTemplate` degrades to the template only on proven absence of the file, so a hand-written
`webster.yaml` carrying only `master:`/`recovery:` yields `RecoveryTimeoutMin == 0`,
`PollWaitS == 0`, `MasterTimeoutMin == 0`, `SelfFixCap == 0`, and `LoadConfig` validates only the
two model-spec strings.
`Elapsed > BatchTimeout` is then true on the first poll, so every recovery batch classifies
`dead`/`timeout` immediately with nothing reporting why.

#### R4-17 — `bisect` never evaluates the last SHA and blames the last card when nothing fails — CONFIRMED

`internal/websterengine/integration.go:116-129`.
If the integration failure is not attributable to any recorded card SHA, `lo` converges on
`len(shas)-1` and the last card is recorded as the offender with full confidence;
`AppendIntegrationFailure` then writes "SHA-bisect localized the failure to card X" into
`summary.md`, which is the PR-text source.

#### R4-18 — `runIntegrationStage` holds the state-mutation lease across an unbounded bisect — CONFIRMED

`internal/websterengine/runlevel.go:900-936` vs `state.go:83-87`, whose contract is explicit:
"never across a long block".
`AcquireStateMutation` uses the blocking `lock.AcquireWriteLock`, so any concurrent bracket verb (a
zombie Master — exactly what `ownerlessRunWarnings` exists to flag) blocks indefinitely with no
timeout and no diagnostic.

#### R4-19 — a `persist` failure on the producer-error path discards the real producer error — CONFIRMED

`internal/shedengine/run.go:186-194` (and the same shape at `:265-270`).
`persist` invokes `s.CommitStatus`, which in loom legitimately returns a git error
(`loomcli/wiring.go:145-147`); when it does, `drive.go:132-136` reports only the git error and the
actual producer failure that halted the run never reaches the operator's envelope.

#### R4-20 — `sink.go`'s lazy init runs entirely outside the mutex `SetDurableSinkDir*` holds — PLAUSIBLE

`internal/logger/sink.go:75-138` and `:200-209`.
`ensureDurableSink` reads `sinkDirOverride` and writes `sinkPath`/`sinkOK`/`sinkBytesWritten`/`header`
inside `sinkOnce.Do` with no lock, while `resetDurableSinkLocked` writes all of those *and*
reassigns `sinkOnce = sync.Once{}` under `sinkMu`.
A `SetDurableSinkDir*` concurrent with an in-flight first log record is a data race on the `Once`
itself, and the losing order leaves the pre-redirect `sinkPath` installed — and that redirect is the
mechanism keeping standalone logs out of the target repo.
Latent today (both CLIs redirect single-threaded in the cobra pre-run) and `-race` passes.

#### R4-21 — `ScopeGuard` never inspects `delta.Renamed`, so an out-of-band rename is operator-invisible — CONFIRMED

`internal/planglyph/scope.go:63-77`.
Quarry removes an exact-tier pair's constituents from `Created`/`Deleted` (stated in
`drift.go:74-77`), so a renamed symbol reaches neither of `ScopeGuard`'s two loops and it has no
`Renamed` loop of its own.
Combined with `DetectDrift`'s gate two, which logs an unreferenced rename at `Debug` only
(`drift.go:108`), a fork that renames a symbol nothing in the plan references produces no
operator-visible signal at all — even though a rename is exactly the "symbol touched outside the
batch's own target glyphs" this guard exists to report.

#### R4-22 — `validate()` rejects only a self-referencing `OnDone`, not a longer `OnDone` cycle — CONFIRMED (latent)

`internal/shedengine/validate.go:73-86`.
Its own rationale — "Done routing consumes no bounce budget, so a self-referencing `OnDone` is a
statically certain infinite loop" — applies verbatim to `A.OnDone = B; B.OnDone = A`, which validate
accepts. `Shed.Run` would spin forever, one history entry and one git commit per hop, with no budget
to stop it.
The shipped `loom-recipe.yaml` has no such cycle, so this is a validator gap rather than a live defect.

#### R4-23 — `--plan-dir`/`--stencils-dir` are stored verbatim and may be relative — CONFIRMED

`internal/webstercli/wiring.go:109-114,165-188`, `internal/burlercli/wiring.go:145-146`.
A relative value is resolved by the CLI process against *its* cwd, but the spawned pane's cwd is the
target (standalone) or the anchor (hub), so one string names two directories.
In standalone it also defeats the `standalonePlanDirOverridden` comparison at `wiring.go:183`, so
`--plan-dir ./plan` naming the default location still trips `run`'s refusal.
Every other told path in this codebase is required absolute.

#### R4-24 — `Derive` symlink-normalizes the identity but `ReedGeometry` does not — PLAUSIBLE

`internal/standalonestate/standalonestate.go:60-65` vs `internal/standalonegeom/reedgeom.go:24-31`.
`Derive` runs `EvalSymlinks` before hashing, so a symlinked and a real spelling of the same repo share
one `hash8`, one socket and one `reed.json` — but `ReedGeometry` builds `SessionName`/`PaneCwd` from
the un-normalized `target`, so the two spellings produce different session names on the same socket
sharing one state file.
reed's foreign-session guard catches it, so this fails loudly rather than corrupting, but with advice
that does not fit ("the worktree directory was renamed … or a `.lyx` directory was copied here").

#### R4-25 — the "state dir inside the target" refusal fires late and blames the wrong cause — CONFIRMED

`internal/shuttleengine/run.go:130-132`.
Standalone against a dotfiles repo rooted at `$HOME` (or any `XDG_STATE_HOME` under the target) puts
`stateDir` inside `worktreeRoot`, so the containment guard fires — correctly — but its message blames
"a subpath-anchored hub geometry handed to the wrong constructor", which is not what happened, offers
no recourse (the only lever is `XDG_STATE_HOME`, never mentioned), and fires only after
`webstercli/run.go:101` has booted a tmux server and `websterengine.Run` has taken `run.lock`.

#### R4-26 — the standalone target defaults to cwd with no git-root normalization — CONFIRMED

`internal/webstercli/wiring.go:261-269`, `internal/burlercli/wiring.go:192-200`.
`preflight.ResolveMode` returns `ModeStandalone` for a plain repo's *subdirectory*, and
`resolveStandaloneTarget` then takes that subdirectory as the target, so the same repository yields
different `hash8` values, state directories, reed sessions and plan directories depending on which
directory the operator happened to be in.
Webster fails loudly; burler silently reviews profile paths rooted at the subdirectory.

### NIT

- **R4-27** — `internal/planglyph/handle.go:210-235` indexes `sources[i]` while ranging over
  `nameResults` with no length guard; a `quarry.Name` returning more results than declarations
  panics rather than reporting. CONFIRMED.
- **R4-28** — `internal/loomshed/planvalidate.go:27-37` re-implements `planglyph.Finding.Error()`
  byte-for-byte while its own doc comment promises the two renderings are identical; the sibling
  `discussionvalidate.go:73-79` calls `f.Error()`. CONFIRMED.
- **R4-29** — `manifest/designs/loom.md:43` names the check ID `depends-on-order`, which exists
  nowhere in the repo (a format-3-era ID; format 5 retired `**Depends-on:**` into
  `card-retired-label`). CONFIRMED.
- **R4-30** — `internal/shedengine/run.go:150-159` persists `Outcome: ""` into `history[]` on every
  error exit, though `producer.go:14-17` declares `""` illegal, and `composeActivity` renders it as
  a dangling `"Plan-Bouncer → "`. CONFIRMED.
- **R4-31** — `internal/planparser/validate.go:914-921` — `checkProsaSymbolTarget` reports
  "targets the symbol %q" for a ref that is a bare directory or an unparseable path, not a symbol.
  CONFIRMED.
- **R4-32** — `internal/loomshed/planwrite.go:53` and `discussionwrite.go:32` accept a nil `commit`
  seam and nil-panic on the Done path; every neighbouring constructor validates its seams, and the
  registry entries guard it, so this is unreachable in production but out of line with the
  codebase's own stated posture (`shedadapters/singlellm.go:159-161`). CONFIRMED.
- **R4-33** — `internal/webstercli/wiring.go:156-159` claims "The redirect runs before any other
  statement in this function"; `resolveStandaloneTarget` and `standalonestate.Derive` both precede
  it. CONFIRMED.
- **R4-34** — `internal/webstercli/recordbatch.go:120-123` emits `{"card_not_done": true}`, but
  `webster-template-master.md`'s failure ladder has no branch for it, so Master falls through to
  generic error handling for a refusal with a specific meaning. CONFIRMED.
- **R4-35** — `internal/websterengine/runlevel.go:652-655,954-956` — on the `outcomeDone` loud
  return, `runIntegrationStage`'s warnings are returned alongside an error and discarded by the
  caller, including the "could not be localized because this mode has no fabric repo" notice that is
  the operator's only explanation. CONFIRMED.

## What was tested

### Hermetic battery (cold, at review start)

HEAD at start of the hermetic battery: `e32f853b4` (the orchestrator advanced HEAD from
`8e8c61b01` to `e32f853b4` while I was reading; both are docs-only commits).

| Command | Result |
|---|---|
| `CGO_ENABLED=1 go build ./...` | PASS |
| `CGO_ENABLED=1 go vet` over the 16 in-scope package trees | PASS (empty output) |
| `CGO_ENABLED=1 go test -count=5` over the same set + `./cmd/lyx/...` | PASS — 18 `ok`, exit 0 |
| `CGO_ENABLED=1 go test -tags integration ./internal/planglyph/... ./internal/planparser/... ./internal/websterengine/... ./internal/loomcli/... ./internal/loomshed/... ./internal/webstercli/... ./internal/burlercli/...` | PASS — exit 0 |

### PATH `lyx` agreement with HEAD

`which lyx` → `/home/knatte/go/bin/lyx`.
`CGO_ENABLED=1 go run ./tools/deploy` reported `Building lyx @ e32f853b4 (dirty) -> /home/knatte/go/bin/lyx`
and the binary's sha256 was **byte-identical before and after** the redeploy
(`1a7f93592713d0a0646e0a4a31d42c2f77962e8cef2c3fe17431b7a3382ae88c`),
so the installed binary already reflected current HEAD.
(The `(dirty)` suffix is this review file itself, untracked at the time.)

### Substrate state at review start

`ps aux | grep -iE 'tmux|lyx|claude'` — no lyx/loom/webster driver processes of mine.
Three leftover `tmux -L lyx-<hash>` server processes from earlier untagged/integration test runs
(`TestRunCLIIn_StandalonePreRun_ReachesRunsOwnValidationGate` temp dirs) were already present
before I started; recorded here and revisited at teardown.

### Live scenarios

_(appended as run)_
