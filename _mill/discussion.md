# Discussion: self-report Tier 1: Go-detected structural anomalies

```yaml
task: 'self-report Tier 1: Go-detected structural anomalies'
slug: self-report-tier1
status: discussing
parent: main
```

## Problem

`lyx selfreport create` ships today as a manual-only verb: a human (or an agent a human is watching) decides that something went wrong and types the issue.
That means the class of friction lyx can detect *without* anyone watching — a driver process that died mid-producer and got resumed, a producer that escalated to a human because it had no bounce target, a review segment that burned its whole bounce budget, a review finding that keeps coming back round after round — is never filed at all unless somebody happens to be present and happens to notice.

loom already writes an exact record of every one of those events.
`_lyx/loom/status.json` (`shedengine.Status` + loom's `product` payload, pinned by `contracts/specs/loom-status-spec.md`) carries an append-only `history[]` of one entry per producer call, a `state` enum, and an `error` string naming the halt reason verbatim.
The bouncer segments additionally write a per-round *finding-identity ledger* (`round-<N>-bouncer-ledger.md`, with `key`/`rounds`/`status` per finding) whose path each Bouncer publishes as its own history entry's `output` field.
Go can therefore file these reports directly off its own history trail — deterministic, no LLM call, and strictly more complete than an LLM's approximate recall of its own session, since it reads an exact record instead of remembering one.

**Why now:** the design was settled with the operator on 2026-09-12 and split out of the former combined `designs/self-report.md` precisely so Tier 1 could build independently of Tier 2 and of `lyx loom step` — no code dependency in any direction, all three buildable in parallel.
Tier 1 is the cheap half: it costs no tokens, needs no session watching, and works identically under plain `lyx loom run` and under a future step-loop supervisor.

## Scope

**In:**

- A new pure, no-I/O anomaly detector in `internal/loomengine` (`anomaly.go`), told a `shedengine.Status`, an entry-time observation, and already-parsed ledger models, returning a slice of detected anomalies.
- Five detection triggers: crash-resume, escalation-to-human, bounce-budget exhaustion, producer hard-failure, and a review finding still open after N rounds.
- An exported ledger-path predicate and ledger-read accessor on `internal/shedadapters`, so the ledger's sole parser stays its sole parser — and the sole owner of "is this path one of mine" — while another package can consume the parsed model.
- Wiring in `internal/loomcli`'s `drive` verb: read the status file at entry, run the phase machine, re-read the status file, then detect and file — on both `shed.Run`'s success and error returns.
  The entry step is a non-blocking run-lock probe followed by the status read, in that order, and the probe's result rides along as part of the entry observation; it is itself guarded on the `selfreport` bool so a disabled run pays nothing.
  The detect-and-file step is one extracted, told-input function in `loomcli` that owns all three skips itself (`selfreport: false`, `ErrShedBusy`, cancelled context), so `drive`'s closure gains exactly one unconditional call and every branch stays Tier-1 testable.
- A filing pass with a pinned order — collapse duplicate titles, filter against the marker, file, record each title on its own success.
- An exported default-label accessor on `internal/selfreportengine`, so the `bug` label literal has one owner across the manual verb and the automatic path.
- Title and body rendering in `internal/loomengine`, as pure functions beside the detector: the detector's returned `Anomaly` value carries its own rendered title, and a sibling pure function renders the body from that value.
  `internal/loomcli` owns only the I/O half — the reads, the marker, the filing-pass ordering, and the `CreateIssue` call — and renders no text of its own.
- A machine-local filed-marker under `.lyx/loom/`, so a resumed run does not re-file an anomaly it already filed.
- A `selfreport` boolean key in `loom.yaml` (default `true`) plus its template entry, so filing can be switched off.
- Docs in the same commit: `manifest/designs/self-report-tier1.md` promoted from Planned to shipped shape, `docs/overview.md`'s selfreport bullet, `manifest/roadmap.md`'s Planned item moved to Done, and `contracts/specs/loom-status-spec.md` if the status schema is touched (it is not — see Decisions).

**Out:**

- Any change to `internal/shedengine`.
  Its Producer-Seam Invariant pins its imports to stdlib, `state`, and `lock`; it gains no anomaly concept, no injected anomaly closure, and no new `Status` field.
- Any change to the `_lyx/loom/status.json` schema.
  No new field, no `schema_version` — the spec deliberately omits versioning and this task does not reopen that.
- Tier 2 (per-agent friction notes) and `lyx loom step`.
  Independent items with no code dependency either direction.
- LLM judgment of any kind in the detection or filing path.
  Tier 1 is deterministic Go end to end.
- A new `lyx selfreport scan` verb, or any geometry wiring on `internal/selfreportcli`.
  That module is today the one seam module without `RunCLIIn` precisely because it references `lyxcwd` nowhere, and this task does not change that.
- Detection during a run.
  Filing happens once per `drive` invocation, after `shed.Run` returns.
- Closing or updating an already-filed issue.
  Filing is create-only.
- Any change to `lyx loom run`'s bootstrap, `lyx loom status`, or `lyx loom pause`.

## Decisions

### detection-site

- Decision: detection runs in `internal/loomcli`'s `drive` verb — a status read at drive's *entry* (before `shed.Run`), then a post-`Run` re-read of the status file plus the ledgers, then filing.
  `drive` is the only production process that ever calls `Shed.Run` (`lyx loom run` spawns `lyx loom drive` detached; see `internal/loomcli/run.go` step 3).
- Decision (error-path reachability, pinned): the detect-and-file step runs on **both** of `shed.Run`'s returns — the clean `Result` return *and* the `err != nil` return — with `errors.Is(err, shedengine.ErrShedBusy)` as the single skip.
  `drive.go`'s current tail returns an error envelope immediately on any non-nil `shed.Run` error (`internal/loomcli/drive.go:132-136`), so the step must be lifted above that early return rather than appended after the success envelope.
  The error envelope's text and `drive`'s exit code stay exactly as they are.
- Decision (the step is one extracted function, and it owns every skip): the whole detect-and-file step is one function in `internal/loomcli`, taking every input told — `cmd.Context()`, the `selfreport` config bool, the entry observation, the status path and status lock path, the marker path and its lock path, the `shed.Run` error, a ledger-read seam, and a filing seam — and returning nothing.
  `drive`'s cobra closure calls it **unconditionally** on one line immediately after `shed.Run` returns, passing the error through; every early return lives inside the function, not in the closure.
- Decision (the three skips, all inside that function, checked before anything is read):
  1. `cfg.Selfreport == false` — the knob (see `### on-by-default-with-a-knob`).
  2. `errors.Is(err, shedengine.ErrShedBusy)` — this process never owned the run lock.
  3. `ctx.Err() != nil` — the operator stopped the run.
     **Exception, and the only one:** a crash-resume detected from the entry observation is filed anyway, before this skip returns.
     Everything else the pass would have done is skipped.
- Rationale for putting the knob and the context check inside the function rather than guarding the call site: it keeps `drive`'s closure at one unconditional line with no branching to get wrong, and it puts all three skips in the same told-input table the branch tests already exercise, so each is a direct Tier-1 case rather than a condition only reachable through `reed.Up()` and `fabricengine.Open`.
  The context check is not redundant with `ErrShedBusy`: an operator Ctrl-C returns `RunPaused` with a **nil** error, so the busy check does not catch it — and `selfreportengine.CreateIssue` deliberately builds its 30s timeout from `context.Background()`, so it would not observe the cancellation either.
  Without this skip an operator stop could be followed by up to 30 seconds of network work per detected anomaly.
  For triggers 2–5 skipping loses nothing: a paused run resolved none of its anomalies, so the next `drive` detects the same ones and files them then.
  **For trigger 1 it would lose the anomaly permanently**, which is why the exception exists rather than an accepted imprecision: the cancellation arm persists `StatePaused`, so the next `drive`'s entry read sees `paused` — not `running` — and the crash observation, which lives only in this process's memory, is gone for good.
  That is the identical permanent loss this section already refuses to accept on the error path, and it would be inconsistent to accept it here.
  The cost of the exception is bounded and conditional: at most one `CreateIssue` call, and only when a crash actually preceded the operator's stop.
- Rationale for the extraction: `drive` builds its Shed inline via `loomrecipe.New` inside the closure with no seam, and reaching that line at all requires `reed.Up()` and `fabricengine.Open`, so a Tier-1 test cannot control what `shed.Run` returns — today's Tier-1 `cli_test.go` coverage can only reach `drive`'s early refusals.
  Putting the skip decision inside the extracted function makes that function the entire unit under test: all three branches (clean return, non-busy error, `ErrShedBusy`) are exercised by calling it directly with a stubbed filing seam, no tmux, no git, no real run.
  Leaving the skip in the closure would have put the one branch that most needs a regression test back behind the untestable wall.
- Decision (the entry observation is lock-probed, and the probe comes **first**): before reading the status file at entry, `drive` probes whether `loomengine.LoomRunLock` is currently held, using the same non-blocking `lock.TryAcquireWriteLock` + immediate `Release` shape `mustSpawnDriver`'s liveness probe already uses.
  The crash-resume trigger fires only when the probe reported **not held** *and* the subsequent read showed `running` with non-empty history.
  The order is load-bearing and must not be swapped: probe, then read.
- Rationale: `ErrShedBusy` does not close the two-driver race on its own, because `Run` takes the lock long after `drive` takes its entry observation — `reed.Up()`, `fabricengine.Open`, branch/origin/parent resolution and `loomrecipe.New` all sit in between.
  A healthy driver holding the lock during the observation can finish normally and release inside that window, after which `Run` short-circuits on `StateDone`, returns no `ErrShedBusy`, and a crash-resume would be filed for a run that never crashed.
  The probe closes that window because the lock is held for the whole of the other driver's `Run`, so "not held" is the statement the trigger actually needs.
  Probing first is what shrinks the residual to two adjacent syscalls *and* makes it self-correcting: `Shed` persists its terminal state **before** `Run` returns and the lock is released by the deferred release after, so a driver that finishes between the probe and the read has already written `done` — and the read that follows sees `done`, not `running`.
  Reading first and probing second would leave the multi-second window intact in a new place.
- Accepted residual, recorded as a third imprecision alongside trigger 1's two: a driver that dies (rather than finishing) is correctly detected, and a driver that is alive is correctly suppressed, but a crash that is *followed* by a newly-alive driver holding the lock at probe time is suppressed here.
  Nothing is lost — that driver took its own entry observation before acquiring the lock, so it is the process that files the crash.
- Decision (the detector reads the file, never `Result`): the final status handed to the detector comes from a fresh `state.ReadJSONStrict` of the status file after `Run` returns, not from `Result.Reason`/`Result.History`.
- Rationale: `shedengine` cannot host detection without breaking its Producer-Seam Invariant, and a purely post-hoc scan cannot see a crash-resume at all.
  A crashed driver leaves `state: running` behind, and the **first ordinary persist after the next producer call** (`Run`'s steps 5–6) overwrites it, so the crash signal exists only between the file's first read and that write.
  Step 3b is *not* what destroys it — 3b is explicitly conditional on `st.State != StateRunning`, so on a crash-resume, where the state already *is* `running`, it never fires at all.
  3b matters here for the opposite reason: it is why a resume from `paused`/`blocked`/`failed` is an ordinary human resume rather than a crash, which is what makes "`running` at entry" the discriminating observation in the first place.
  Taking the entry observation in `drive`, where the file is already read for `loomengine.VerifySeedOwnership`, costs one extra decode and no new architecture.
  Running on the error path is not optional: a producer hard error persists `state: "failed"` (`internal/shedengine/run.go`'s `callErr != nil` arm) and returns a non-nil error, so skipping that path would drop the whole failure class — and, worse, the entry-time crash observation lives only in this process's memory, so a crash-resume followed by a failing producer would be lost permanently, never re-detectable by any later `drive`.
  Reading the file rather than `Result` follows from the same decision: `Result` is explicitly meaningless when the returned error is non-nil (`shedengine.Result`'s own doc comment — every hard-error path returns an unpopulated `Result`), so the file is the only source that is valid on both paths.
  `ErrShedBusy` is the one skip because that return means this process never ran the machine at all and never owned the run lock, so anything it observed at entry belongs to a live driver, not to a crash.
- Rejected: running the step only on the clean-`Result` path, which is what the current code shape would give for free.
  It silently drops `state: failed` and permanently loses any crash-resume that precedes a producer failure.
  Also rejected: keeping `drive`'s success envelope as the only insertion point and re-deriving the halt reason from `Result` — meaningless on the error path, per above.
- Rejected: an injected `OnAnomaly func(...)` closure on `Shed` (precedent exists — `Shed.CommitStatus` is exactly that shape) fired in-lock at each transition.
  It would be exact and would fire mid-run, but it puts a network call inside the run's lifetime, hands the generic engine a product-flavoured concept, and buys nothing Tier 1 needs, since nobody is watching for a mid-run issue anyway.
  Also rejected: a standalone `lyx selfreport scan` verb as the only mechanism — it reintroduces the "somebody has to be present" problem the task exists to remove, and would force geometry wiring onto the deliberately geometry-free `selfreportcli`.

### detector-package-and-purity

- Decision: the detector is `anomaly.go` in `internal/loomengine`, a pure function over told inputs with no I/O and no spawns — `DetectAnomalies(entry EntryObservation, final shedengine.Status, ledgers []LedgerObservation) []Anomaly` (exact signature is the plan's call).
  Every ledger file is read by the caller (`loomcli`) and handed in already parsed.
- Rationale: `loomengine` already imports `shedengine` and already houses exactly this shape — `coherence.go`'s `checkCoherence` is a pure, no-I/O, exhaustively table-tested Tier-1 validator over a decoded `shedengine.Status`, and says so in its own file comment.
  Keeping the detector pure is also what keeps `loomengine` free of a `shedadapters` import: the ledger models cross the boundary as data, not as a read call.
- Rejected: a new `internal/loomanomaly` package — a second home for the same validator shape with no separation gained.
  Also rejected: putting the detector in `internal/selfreportengine` — that would make the geometry-free, loom-agnostic selfreport module depend on loom's status schema and the bouncer's file contracts, inverting the dependency direction the module was built with.

### ledger-access

- Decision: `internal/shedadapters` gains **two** exported surfaces — a path predicate ("is this path one of my ledger files") and a read accessor returning an exported model carrying the round number and each entry's key, rounds, and status.
  `loomcli` calls both; the detector consumes the model and parses nothing.
- Decision (round-agreement, pinned): the exported accessor **inherits** the fail-closed check `recordedVerdict` already applies — it derives the round from the filename through the package's own path helper and rejects the file when the frontmatter's `round` disagrees.
  A rejection is not an error the caller handles specially; it lands in skip #3 (warn, no anomaly).
- Rationale: `parseLedger` itself deliberately does not check filename/frontmatter agreement — the in-package caller does, and its comment names the failure precisely: the judge wrote the wrong round's claim into a file that landed at the right path, which is exactly the shape to fail closed on rather than trust.
  Inheriting it matters here specifically because `### filing-pass-order`'s "keep the highest `round`" collapse keys on the frontmatter value: a file whose frontmatter disagrees with its own name would silently win or lose that comparison against a value nothing corroborates.
  Deriving the round inside the accessor (rather than making the caller pass it) also keeps round extraction from the filename inside the package that owns the filename.
- Rationale: `internal/shedadapters/bouncerfiles.go` already declares itself the owner of the three bouncer file contracts and their strict, fail-loud parsers, and `round.go` already owns their filename shapes.
  Exporting an accessor keeps that ownership intact — the same posture the repo already pins for other formats (Planparser Sole-Parser, Discussionparser Sole-Parser, Summaryparser Sole-Parser) — and exporting the predicate alongside it is what stops the *caller* from having to re-declare either the filename convention or the recipe's row names in order to recognise a ledger pointer (see `### ledger-discovery`).
  This is a **new production import edge** — `loomcli` → `shedadapters` exists today only in `internal/loomcli/smoke_attachprobe_test.go`, not in production — and it introduces no cycle: `shedadapters` imports `burlerengine`, `logger`, `shedengine`, `shuttleengine`, `stencil`, `stencilstore`, `summaryparser`, `websterengine`, and no `loom*` package at all.
- Rejected: duplicating a minimal ledger parser in the consuming package — a second reader of a format with a documented owner, guaranteed to diverge.
  Also rejected: extracting the ledger format into its own sole-parser package — real churn across `shedadapters`' four bouncer files for a format with exactly one producer and, after this task, two consumers.

### trigger-list-and-thresholds

- Decision: five triggers.
  1. **crash-resume** — the entry-time read observed `state: "running"` with a non-empty `history`: the previous process left the file mid-run without writing a terminal state.
     The kind is named for its dominant cause, not asserted as its only one — see the accepted imprecision below.
  2. **escalation-to-human** — final `state: "blocked"` with `error == "stuck with no OnStuck target"`.
     Eight rows escalate this way (`Preflight`, `Loom-Preflight`, `Discussion-Write`, `Plan-Write`, `Batchifier`, `Webster`, `Publish`, `Finalize`).
  3. **bounce-budget exhausted** — final `state: "blocked"` with `error == "bounce budget exhausted"`.
  4. **producer hard-failure** — final `state: "failed"`, with `error` carried verbatim.
  5. **recurring review finding** — a ledger entry whose `status` is `open` and whose `rounds` list has three or more entries.
- Rationale: the first four are read straight off fields `Shed` writes verbatim, with the two blocked reasons being exact string literals `internal/shedengine/run.go` sets (`reason := "stuck with no OnStuck target"` and `reason := "bounce budget exhausted"`).
  Trigger 4 exists because the error-path decision above makes it reachable: `Run`'s `callErr != nil` arm persists `state: "failed"` and returns the error, and a run the engine itself classified as an engine-level failure a human must resolve is squarely the structural anomaly this tier files.
  It is distinguished from the two blocked triggers by `state` alone and matches no `error` literal, since the text there is whatever the producer returned.
  The fifth is the design doc's "N repeated review rounds on the same finding", and the bouncer ledger is the only place finding *identity* exists — `status.json`'s `history[]` records producer and outcome, never a finding.
- Accepted imprecision in trigger 1, recorded rather than engineered away, in both directions:
  - **Over-match.** `state: "running"` with non-empty history is not exclusively a dead driver.
    `Run`'s step-1 and step-2 hard errors — an unreadable or undecodable status file, a state outside the five-value enum, or a `current_producer` naming no row in the list — all return without persisting anything and leave `running` behind.
    The `current_producer`-not-found case is genuinely reachable: it is what a recipe row rename produces, which `contracts/recipes/loom-recipe.yaml`'s own header warns breaks resume for any in-flight task.
    Every one of those is a structural anomaly worth filing anyway, so the trigger's *value* holds even where its label is imprecise; the body carries `state`, `current_producer`, and `error` verbatim, which is what lets a reader tell them apart.
  - **Under-match.** A crash during the *very first* producer call leaves `running` with an empty `history` and is invisible to this trigger.
    That is forced, not chosen: `contracts/specs/loom-status-spec.md`'s fresh seed is exactly `current_producer: "Preflight"`, `state: "running"`, empty `history` — byte-identical to a crash at `Preflight`, so no predicate over this file can separate them.
    Dropping the non-empty-history condition would file a crash-resume issue for every ordinary first `drive` of every task.
  Threshold three, because every review segment carries `max_bounces: 5` and the Bouncer's own seed call permanently consumes one unit: firing at three reports the recurrence while the segment is still alive, rather than only after it has already degenerated into trigger 3.
- Rejected: dropping the ledger read and reporting a per-segment round *count* from `history[]` alone.
  It is cheaper but answers a different question — "this segment took many rounds" rather than "this specific finding survived many rounds" — and the design doc names the latter.
  Also rejected: a minimal two-trigger set (the two blocked reasons only), which files nothing for the silent-crash case that motivates the whole tier.

### ledger-discovery

- Decision: ledger files are discovered by walking the final status's `history[]`, offering **every** non-empty `output` value to `shedadapters`' exported ledger-path predicate, and reading each distinct path the predicate accepts through the exported accessor.
  The discriminator is therefore the predicate, owned by `shedadapters` — never a row-name list in the caller, and never a filename pattern re-declared in the caller.
- Decision (three skips, each for its own reason, none of them fatal):
  1. An **empty** `output` is skipped before the predicate is consulted.
  2. A non-empty `output` the predicate **rejects** is skipped and never read, so a fail-loud parser is never handed a non-ledger file.
  3. A path the predicate accepts but whose read or parse **fails** — including a file that no longer exists — is skipped with a `logger.Warn` and contributes no anomaly.
- Accepted imprecision, recorded rather than engineered around (the fourth case, alongside the three skips): a stale `history[].output` path may resolve not to *nothing* but to a **different generation's live file**.
  `archiveRunDir` (reached from `Bouncer.Call`'s clear-and-re-seed when a settled generation is re-entered) renames the whole run directory aside, recreates it empty, and restarts round numbering at 1; `archiveStaleOutputs` stamps an individual round's judge outputs aside before a fresh spawn.
  Either way a later generation can write a new `round-1-bouncer-ledger.md` at exactly the path an older history entry still names.
  Two consequences, both accepted:
  - **Provenance is loose, content is not.** Discovery reads whatever file currently lives at an accepted path, so an old entry may surface a newer generation's finding.
    The harm is bounded: the finding read is a real, currently-open one, and the newest generation's ledger is reachable from the newest history entries too, so collapse-by-title (see `### filing-pass-order`) reduces both paths to the one issue.
  - **The ≥3 threshold counts rounds within the current generation**, since a clear restarts numbering at 1.
    A finding that survived two generations at two rounds each does not trip it.
    That is a deliberate under-report, consistent with the rest of the tier: file what the record proves, never what it merely suggests.
- Rejected: adding a generation discriminator to the trigger-5 title.
  Nothing in `status.json` or in the ledger format carries a generation counter, so one would have to be derived by globbing the archive directories `archiveRunDir` leaves behind — real complexity, in the caller, over a naming convention `shedadapters` owns, to sharpen provenance on an anomaly whose content is already correct.
- Rationale: `Bouncer.settle` sets `shedengine.OutputPointer{Path: ledgerPath(b.cfg.RunDir, round)}` and returns it on **both** the `APPROVED`/`Done` and the `BLOCKING`/`Stuck` branch, and `Shed.Run` persists `output.Path` into the history entry — so the trail from `status.json` to finding identity already exists on disk and needs no geometry derivation and no knowledge of each segment's `run_subdir` recipe key.
  Each skip closes a real case, not a hypothetical one:
  - `Bouncer.seedCall` returns `Stuck` with an explicitly empty pointer, so every segment has at least one history entry with no ledger behind it.
  - **A non-empty `output` is not evidence of a ledger.** `SingleLLMProducer.mapOutcome` returns `shedengine.OutputPointer{Path: spec.OutputFiles[0]}` on its `Done` branch (`internal/shedadapters/singlellm.go`), so the `Discussion-Write`, `Plan-Write`, and `Webster` rows all publish their own artifact paths into `history[].output` too.
    Without a discriminator the detector would hand `decision-record.md` to a parser that fails loud on it.
  - Ledger files are ephemeral by construction — they live under `loomengine.LoomReviewsDir` (`.lyx/loom/reviews/`), which that accessor's own doc comment states is never committed — so a `history[].output` path from an earlier machine, or from before a `.lyx` wipe, legitimately points at nothing.
  Making all three skips non-fatal is the same failure posture the rest of this task takes: a diagnostics side-channel never fails a loom run.
- Rejected: a row-name check in the caller (`strings.HasSuffix(producer, "-Bouncer")`, or a literal list of the three Bouncer row names).
  Row names are `contracts/recipes/loom-recipe.yaml` identities, and the recipe's own header warns that a rename there without a matching constant rename breaks resume — a name list in `loomcli` would be a third place to keep in sync.
  Also rejected: a filename check in the caller (matching `round-%d-bouncer-ledger.md`).
  That is exactly the string `internal/shedadapters/round.go`'s `ledgerPath` owns, and re-declaring it in the consumer is the divergence the sole-parser posture exists to prevent — which is why the predicate is exported from `shedadapters` instead.
  Also rejected: deriving ledger paths from `loomengine.LoomReviewsDir` joined with each segment's `run_subdir` and globbing rounds — that duplicates recipe config knowledge and would silently go wrong on a recipe edit.

### one-issue-per-anomaly

- Decision: one GitHub issue per distinct anomaly **occurrence**, not one per task per kind.
  The title carries a discriminator so two genuinely separate occurrences of the same kind are two issues — and, because the triggers differ in whether they are re-observed at all, the discriminator is **not** the same for all of them:
  - **Triggers 2–4** (the three halt kinds, each re-observed on every resume until fixed): `loom anomaly: <kind> — <slug> — <producer>#<success-count>`, where `<producer>` is the final `current_producer` and `<success-count>` is the number of `history[]` entries whose `producer` is that name and whose `outcome` is `done` — that is, how many times this row had previously succeeded.
  - **Trigger 1** (crash-resume, observed at most once per crash): `loom anomaly: crash-resume — <slug> — <producer>@<history-length>`, using the **entry-time** `current_producer` and `len(history)`.
  - **Trigger 5** (recurring finding): `loom anomaly: recurring-finding — <slug> — <bouncer-row> — <ledger-key>`, where `<bouncer-row>` is the `producer` field of the history entry that published the ledger path.
    Trigger 5 is the deliberate exception to this section's per-occurrence premise: it is **identity-deduped, not occurrence-deduped**.
    One issue per `(row, key)` pair per task, for the lifetime of the task — a key that recurs after being marked `resolved`, including across an `archiveRunDir` generation boundary, is suppressed by the marker rather than filed again.
    That is intended, not an oversight: the issue's subject is "this finding keeps coming back", so a second issue about the same finding coming back again adds a thread, not information, and the first issue is still the right place for it.
    The plan must not read the per-occurrence premise as applying here.
- Decision: the label list for the automatic path is `selfreportengine`'s exported default (see `### label-ownership`), not a literal re-declared in `loomcli`.
- Rationale: the title must be deterministic because it is the dedupe key (see `### dedupe-marker`), and one anomaly per issue is what lets each be closed on its own merits.
  Per-occurrence granularity is the right choice because the *whole* point of the marker is to suppress re-filing **the same halt** on every resume, and a once-per-task-per-kind title would overshoot that into suppressing a *second, unrelated* halt: a task that escalates at `Discussion-Write`, gets fixed, and later escalates at `Webster` must file twice.
  The discriminator has to satisfy two things at once — **stable** across every re-observation of one unresolved occurrence, and **distinct** for a genuinely later occurrence — and `len(history)` satisfies only the second:
  `StateBlocked` deliberately does not short-circuit `Run`'s step 1 (only `StateDone` does), so every resume of an unfixed escalation re-calls the halting producer, `appendHistory()` appends another `stuck` entry, and the blocked persist lands at a longer `len(history)`.
  A length-keyed title would therefore file a brand-new issue on every single resume — the exact failure the marker exists to prevent, reintroduced by the discriminator.
  **Counting that producer's own `done` entries is stable under precisely that append.** A resume of an unresolved halt appends only `stuck` entries for that producer, so the count does not move; it moves only when the row genuinely succeeds, which is what makes a *later* halt at the same row after an intervening success a distinct occurrence.
  It stays well-defined for trigger 4 too, where `appendHistory` skips entirely (a producer that returned an error and no outcome appends nothing).
  The same reasoning is why trigger 1 keeps a *positional* discriminator instead: a crash-resume is observed at most once, since `Run` overwrites the state as it proceeds, so stability under re-observation is not a property it needs, and `len(history)` gives it the finer distinctness that separates a crash at one point in the run from a crash at another.
  Trigger 5 needs the Bouncer row name because a ledger `key` is an LLM-authored short finding identity scoped to its own segment's run directory (`contracts/stencils/bouncer/bouncer-template-judge.md`), with nothing making it unique across the `discussion`, `plan`, and `webster` segments — two unrelated findings that happen to pick the same key would otherwise collapse into one issue and permanently suppress the second.
  The row name is read from the history entry's own `producer` field, so it is data, not recipe knowledge re-declared in the caller.
- Accepted limitation, stated rather than engineered around: two crashes at the *identical* entry-time history position collapse into one issue.
  They are the same unresolved condition re-crashing, and a crash-loop that never advances surfaces through the driver log and through whichever halt trigger fires once it stops crashing.
- Rejected: the bare `loom anomaly: <kind> — <slug>` title of the first draft.
  Combined with a title-keyed marker it permanently suppresses every later distinct anomaly of the same kind in the same task — which reads as "detected once, nothing since" and is indistinguishable from a healthy run.
  Also rejected: `<producer>@<history-length>` for the halt triggers, for the instability above.
  Also rejected: `episodeStuckCount` of the halting producer as the discriminator — it counts exactly the entries a resume appends, so it drifts on every resume for the same reason `len(history)` does.
  Also rejected: a timestamp as the discriminator, which makes the resume case file a fresh issue every time, defeating the marker outright.
  Also rejected: one aggregated issue per `drive` invocation listing every anomaly found — it mixes unrelated causes into one thread that can never be cleanly closed, and its title cannot be deterministic without becoming meaningless.

### filing-pass-order

- Decision: the filing pass is four ordered steps over the detector's returned slice, and the plan implements them in this order:
  1. **Collapse by title.** Reduce the slice to one anomaly per distinct title, keeping the occurrence with the highest `round` when the duplicates are trigger-5 anomalies and the first occurrence otherwise.
  2. **Filter against the marker.** Drop every title the marker already holds.
  3. **File**, one `CreateIssue` per surviving anomaly, in the slice's deterministic order.
  4. **Record**, adding a title to the marker immediately after that title's own `CreateIssue` succeeds — never in advance, and never in one batch at the end.
- Rationale: step 1 is not defensive tidying, it is required by the ledger format's own carry-forward rule.
  The judge prompt carries an `open` entry forward losslessly into every later round's ledger, and discovery reads every accepted ledger path in `history[]` — so from round 3 onward a single recurring finding appears in several ledger files at once and would produce one identically-titled anomaly per file.
  Without the collapse the first would file and the rest would be silently swallowed by the marker mid-pass, which happens to give the right issue count for the wrong reason and breaks the moment filing order or marker timing changes.
  Keeping the highest-`round` occurrence is what makes the issue body carry the most complete `rounds` list, since carry-forward means the latest ledger has the fullest record.
  Step 4's per-title ordering is what makes the failure posture work: a `CreateIssue` that fails leaves its title unrecorded and therefore retried next run, while its already-filed siblings stay recorded.
- Rejected: reading only the highest-round ledger per run directory.
  It also removes the duplication, but it makes correctness depend on correctly identifying "the run directory" and on the carry-forward rule actually holding — and `parseLedger` explicitly does **not** enforce carry-forward, so an incomplete carry-forward would silently drop findings the collapse approach still reports.
  Also rejected: batching the marker write to the end of the pass, which loses every already-filed title if the process dies mid-pass and re-files all of them next run.

### label-ownership

- Decision: `internal/selfreportengine` gains an exported default-label accessor (a `DefaultLabels()` function or an exported `bug` constant — the plan picks the spelling), and `internal/selfreportcli/cli.go`'s existing `labels = []string{"bug"}` fallback is switched to use it in the same change.
  The automatic filing path in `loomcli` calls the same accessor.
- Rationale: the `bug` default is today a literal inside `internal/selfreportcli/cli.go`'s `runCreate`, not in the engine — so a second caller of `CreateIssue` that wants the same default would have to re-declare the string, leaving two owners of one convention.
  Moving it to the engine gives it one owner reachable from both callers, and it is a pure refactor: the manual verb's observable behaviour (omit all `--label` flags, get `bug`) is unchanged.
- Rejected: `loomcli` declaring its own `[]string{"bug"}` literal.
  One line, but it is the second declaration of a default that already exists, and the two would drift the first time either is reconsidered.
  Also rejected: having `loomcli` import `selfreportcli` to reach the existing literal — a cli-to-cli import, against the CLI/Cobra Invariant's `<module>cli` → `<module>engine` direction.

### dedupe-marker

- Decision: a machine-local filed-marker JSON file at `.lyx/loom/selfreport-filed.json`, written through `internal/state`'s locked/atomic typed primitive with its own sibling lock file, holding the set of already-filed anomaly titles.
  A new `loomengine` accessor exposes the path (and the lock path), beside the existing `LoomScratchDir`.
  Dedupe granularity is therefore whatever the title shape encodes — per-occurrence, per `### one-issue-per-anomaly` — and the marker itself stays a dumb string set with no parsing of its own.
- Rationale: without a marker, every resume of a blocked run re-files the same escalation issue, which would make the feature worse than useless.
  `.lyx` is the repo's declared home for never-tracked state at the mirrored subpath of the `_lyx` content it relates to (Durable-vs-Ephemeral State Invariant), and `loomengine` already exposes four accessors under exactly that directory (`LoomStatusLock`, `LoomRunLock`, `LoomDriverLog`, `LoomBootstrapLock`).
  Losing the marker — a fresh clone, a `fabric` re-wire — costs at most one duplicate issue, which is the cheapest failure mode available.
- Rejected: a durable marker under `_lyx/loom/`, which would need a weft commit at a point in `drive` that owns no commit boundary, and would drag the Fabric Git Invariant into a diagnostics path.
  Also rejected: network dedupe (search GitHub for an open issue with the same title before filing) — a second fallible API call per anomaly per run, non-deterministic, and it silently stops working the moment someone closes the issue without fixing the cause.

### on-by-default-with-a-knob

- Decision: on by default, disabled by setting a new `selfreport: false` key in `loom.yaml`.
  The key is added to `internal/loomengine/template.yaml` in the same change, and the default in the template is `true`.
- Decision (consultation site, pinned): the resolved bool is a **told input of the extracted detect-and-file function**, not a guard around its call — `drive`'s closure reads it off the already-loaded config and passes it in, keeping that call unconditional.
- Decision (what `false` skips): **everything, including the entry step.**
  `drive` guards the entry lock-probe-and-read on the same bool, and the extracted function returns before reading the status file, before any ledger read, and before touching the marker — not merely before `CreateIssue`.
  This is the one place the closure does branch on the knob, and it is deliberate: leaving the entry step unconditional would make `selfreport: false` still pay for a lock probe and an extra status decode on every `drive`, which is exactly the cost the rationale below claims it avoids.
- Rationale: detection with filing disabled has no observable effect anywhere — nothing is written, nothing is logged as an anomaly — so performing the reads would be pure cost.
  Making it a told input rather than a call-site guard keeps all three skips (`selfreport`, `ErrShedBusy`, done context) in one place and in the same Tier-1 test table, instead of putting one of them back behind `drive`'s untestable closure.
- Rationale: the design doc's own argument is that Tier 1 costs nothing and needs no session watching, so there is no reason to gate it behind opt-in.
  A knob is still required, because a run in CI, or against a fork, must be able to stop filing issues into the hardcoded `Knatte18/loomyard` target.
  `loom.yaml` is where loom's run-wide knobs already live, and `loomengine.LoadConfig` uses `configengine.Load` (strict) — so the key must exist in the template or every existing config file fails to load.
- Rejected: default-off opt-in, which leaves the common case — an unattended run nobody is watching — filing nothing.
  Also rejected: no knob at all, which makes any fork or CI run a spammer of the upstream issue tracker.

### failure-posture

- Decision: every failure in the detect-and-file path degrades to `logger.Warn` and is never allowed to change `drive`'s outcome, its exit code, or its envelope.
  A failed `CreateIssue` leaves that anomaly's title *out* of the marker, so the next `drive` invocation retries it.
  A failed marker read is treated as an empty marker; a failed marker write is warned and nothing else.
- Rationale: this is a diagnostics side-channel on a long autonomous run.
  A GitHub outage, an unresolvable token, or a rate limit must never turn a completed loom run into a reported failure.
  The repo already has this posture written down in the adjacent code: `Bouncer.runSeedSpawn` "degrades every failure to a warning", and `selfreportengine.CreateIssue` already classifies token-unresolvable, network, and API-rejection cases distinctly for exactly this kind of caller.
  Not writing the marker on a filing failure is what makes the retry happen without any retry loop.
- Rejected: surfacing the failure on `drive`'s envelope, which would make an unrelated network problem indistinguishable from a real orchestration failure in the driver log an operator reads to diagnose a halt.

### issue-body-content

- Decision: a Go-rendered markdown body carrying: the anomaly kind and a one-line statement of what was detected; the task slug and parent branch from `product`; the final `state`, `current_producer`, and `error` verbatim; the relevant `history[]` slice rendered as producer/outcome/at rows; and, for the recurring-finding trigger, the Bouncer row name plus the ledger entry's key, its `rounds` list, and its `status`.
  Rendering is owned by `internal/loomengine`, as a pure function beside the detector — same package as the title, so both are table-testable in Tier 1 and neither lives in the I/O layer.
- Rationale: the issue has to be actionable by someone reading it cold, with no access to the worktree — the ledger and status files are under `.lyx`/`_lyx` in a worktree that may already be torn down.
  Everything named here is already in hand at filing time and needs no extra read.
- Rejected: title-only issues, which would require the reader to still have the worktree.
  Also rejected: embedding the whole `status.json`, which buries the signal and can be large.

## Technical context

**The status file.** `_lyx/loom/status.json` is `shedengine.Status` (`internal/shedengine/status.go`) plus loom's `product` payload (`internal/loomengine/status.go`), pinned by `contracts/specs/loom-status-spec.md`.
Fields that matter here:

- `state` — the five-value `shedengine.State` enum (`running`, `paused`, `done`, `blocked`, `failed`).
- `error` — the halt reason, set verbatim by `Shed.Run`.
- `current_producer` — which row the run is at.
- `history[]` — `[]shedengine.HistoryEntry{Producer, Outcome, Output, At}`, one entry per producer call.
  It is **budget-bearing, not only a log**: `episodeStuckCount` derives every producer's per-producer, episode-scoped bounce budget by counting its own `stuck` entries since its own most recent `done`.
  It must never be truncated or compacted.
- `product` — `{slug, parent, start_sha}`.

Reads go through `state.ReadJSONStrict[shedengine.Status](statusPath, statusLockPath)`, which uses `DisallowUnknownFields` at the shell level; `product` is decoded separately afterwards.
`internal/loomengine/seed.go` and `VerifySeedOwnership` are the existing in-repo examples of that two-step read, and `drive` already calls `VerifySeedOwnership`.

**The two blocked reasons are exact literals.** In `internal/shedengine/run.go`'s `outcome == Stuck` arm:

- `def.OnStuck == ""` → `reason := "stuck with no OnStuck target"`, persisted as `StateBlocked`.
- `episodeStuckCount(st.History, def.Name) >= effectiveMaxBounces(def, s.MaxBounces)` → `reason := "bounce budget exhausted"`, persisted as `StateBlocked`.

Both strings also land in `Result.Reason` — but the detector reads the **file**, not `Result`, per the `### detection-site` decision: `Result` is documented as meaningless whenever `Run` returns a non-nil error, and the detect-and-file step runs on that path too.
A third `error` value exists and matters: the `callErr != nil` arm persists `state: "failed"` with the producer's own error text, which is trigger 4 and matches no literal.

**Why crash-resume is not post-hoc detectable.** A crashed driver leaves `state: running` behind, because it died between two persists.
So `state == "running"` at the *first* read of a fresh `drive` process, with non-empty `history`, is the crash signal — and it is gone as soon as `Run`'s first ordinary persist for the next producer call (steps 5–6) rewrites the file.
`Run`'s step 3b is the *other* half of why that observation discriminates, and it is easy to misread: it is explicitly conditional (`if st.State != StateRunning`), persisting `StateRunning` with an empty error before calling the producer so the file and the status strand stop describing a paused/blocked/failed run that is in fact already spawning.
On a crash-resume it therefore never fires — the state is already `running` — and on a human resume from `paused`/`blocked`/`failed` it fires and normalises the state.
That is exactly what leaves `running`-at-entry meaning "nobody wrote a terminal state", rather than "a human is resuming".

**Concurrency.** Two concurrent drivers cannot both proceed: `Shed.Run` holds `LockPath` (`loomengine.LoomRunLock`) non-blocking for the whole run and returns `shedengine.ErrShedBusy` otherwise, and `drive` treats that as an ordinary error envelope.
`lyx loom run`'s `mustSpawnDriver` uses that same held lock as its liveness probe, built on `lock.TryAcquireWriteLock` plus an immediate `Release` — never held by the probe itself, since holding it would make the predicate indistinguishable from the driver it is checking for.

**`ErrShedBusy` alone does not make the entry observation safe.** `Run` acquires the lock inside itself, but `drive` takes its entry observation far earlier — `reed.Up()`, `fabricengine.Open`, branch/origin/parent resolution and `loomrecipe.New` all sit between them.
A healthy driver holding the lock at observation time can therefore finish normally and release before this process reaches `Run`, which then short-circuits on `StateDone` and returns no `ErrShedBusy` at all.
The crash-resume trigger needs its own guard; see `### detection-site`'s lock-probe decision.

**The bouncer ledger.** `internal/shedadapters/bouncerfiles.go` owns three file contracts: verdict, finding-identity ledger, focus.
The ledger's parsed model is `ledgerFile{Round int, Entries []ledgerEntry, Prose string}` with `ledgerEntry{Key string, Rounds []int, Status string}`; `parseLedger` is fail-loud and pins `status` to exactly `open` or `resolved`, `round` to a positive int, and every `rounds` element to a positive int.
Paths come from `ledgerPath(runDir, round)` in `internal/shedadapters/round.go`, which formats `round-%d-bouncer-ledger.md` — its siblings are `round-%d-bouncer-verdict.md` and `round-%d-focus.md`, so the exported predicate must accept only the ledger spelling.
Note `judgeOutputs` in the same file declares all three as one judge pass's `OutputFiles` set and warns that the spawn and every probe must name byte-identical paths — another reason the filename shape stays inside this package.
Carry-forward of entries across rounds is stated in the judge prompt and deliberately **not** enforced by `parseLedger` — so a `rounds` list of length ≥ 3 is a claim the judge made, not something the parser guarantees, and the detector must treat a short or missing list as "no anomaly" rather than as corruption.

**Where ledgers live.** `runDir` resolves under `loomengine.LoomReviewsDir` = `.lyx/loom/reviews/`, joined with each segment's `run_subdir` (`discussion`, `plan`, `webster`).
They are ephemeral by construction — `internal/loomengine/config.go`'s `LoomReviewsDir` doc says so explicitly, and the recipe's `Webster-Bouncer` comment repeats it.
The detector must therefore tolerate a ledger path in `history[].output` that no longer exists on disk.

**Not every `history[].output` is a ledger.** `SingleLLMProducer.mapOutcome` (`internal/shedadapters/singlellm.go`) returns `OutputPointer{Path: spec.OutputFiles[0]}` on its `Done` branch, so `Discussion-Write`, `Plan-Write`, and `Webster` publish their own artifacts into `output` as well.
`BurlerProducer` publishes `roundReviewPath(runDir, n)` — `round-%d-review.md` — into `output` on its `Stuck` returns, and `roundFixerReportPath` (`round-%d-fixer-report.md`) is its sibling in the same directory.
Together with `round-%d-bouncer-verdict.md` and `round-%d-focus.md`, that is four same-directory files sharing the ledger's own `round-%d-` prefix.
This is why ledger discovery needs the `shedadapters`-owned predicate rather than an "is it non-empty" test, and why the predicate's rejection set is asserted against the real path helpers.

**A blocked run's history grows on every resume.** `Run`'s step 1 short-circuits on `StateDone` only — `StateBlocked` and `StateFailed` deliberately fall through so the loop re-calls `current_producer`, which is how a human resumes.
Both blocked arms call `appendHistory()` before persisting, so each resume of an unfixed escalation appends another `stuck` entry for the same producer and the blocked persist lands at a longer `len(history)` and a higher `episodeStuckCount`.
The one stable quantity across those appends is the producer's own `done` count, which is why the halt triggers' title discriminator is built on it.
The hard-failure arm is the exception that also stays stable: `appendHistory` returns early when `outcome == ""`, appending nothing at all.

**Run directories are archived and re-seeded.** `Bouncer.Call` re-entering a settled generation logs a `Warn` and calls `archiveRunDir`, which renames the run directory aside and recreates it empty so rounds restart at 1; `archiveStaleOutputs` (`internal/shedadapters/archive.go`) stamps an individual round's outputs aside before a fresh spawn.
A commit-seam failure followed by a resume takes exactly this path, per `Call`'s own comment — so it is a live case, not a corner.
Consequence for discovery: a path in `history[].output` can resolve to a later generation's file rather than to nothing.

**Ledger entries carry forward.** `contracts/stencils/bouncer/bouncer-template-judge.md` instructs the judge to carry every `open` entry losslessly into each later round's ledger, and its `key` is an LLM-authored short finding identity scoped to that segment's own run directory — not unique across the three segments.
`parseLedger` enforces neither property (its own doc comment says carry-forward is "stated in the judge prompt and deliberately not enforced here"), so the filing pass must collapse duplicates itself and the title must carry the segment.

**The three review segments.** `contracts/recipes/loom-recipe.yaml` has seventeen rows.
`Discussion-Bouncer`/`Discussion-Burler` (segment `Discussion-Review`), `Plan-Bouncer`/`Plan-Burler` (`Plan-Review`), `Webster-Bouncer`/`Webster-Burler` (`Webster-Review`) — each pair with `max_bounces: 5` and a mutual `on_stuck`.
The Bouncer is the row that publishes the ledger pointer; the Burler row never returns `Done` at all.

**The filing primitive.** `selfreportengine.CreateIssue(title string, body *string, labels []string) (url string, number int, err error)`.
`targetRepo` is the hardcoded `"Knatte18/loomyard"` constant; `NewGitHubClient` is an exported, swappable package variable (`var NewGitHubClient = githubclient.New`) — that is the seam the tests use, and it is what makes the filing path testable with no network.
The whole call is bounded by a 30s `createIssueTimeout`, and errors are already classified three ways (token-unresolvable via `githubclient.ErrTokenUnresolvable`, API rejection via `*github.ErrorResponse`, everything else as unreachable).
`internal/selfreportcli/cli.go` shows the existing caller shape, including the `nil` body pointer convention: `nil` means "omit the body field entirely", which this task will not use — every anomaly issue gets a body.
The `bug` default is a literal in that file's `runCreate` (`labels = []string{"bug"}` when no `--label` is given), **not** in the engine — which is what the `### label-ownership` decision moves.

**The drive verb.** `internal/loomcli/drive.go` (147 lines) is the whole insertion site.
Current order: `ShouldAbort` → stat the status path → `VerifySeedOwnership` → `reed.Up()` → `fabricengine.Open` + branch/origin/parent resolution → build `c.env.Landing` → `loomrecipe.New` → `shed.Run(ctx)` → **`if err != nil { output.Err; return }`** → `output.Ok` envelope with `outcome`, `halted_producer`, `reason`, `history_length`.
The entry observation goes next to `VerifySeedOwnership` (the file is already being read there).
The detect-and-file step must be inserted **above** that `err != nil` early return, so it runs before either envelope is emitted, and must alter neither envelope's text nor `drive`'s exit code.
The `ErrShedBusy` skip is the one condition that bypasses it.

**Config.** `internal/loomengine/config.go`'s `Config` mirrors `loom.yaml`'s seven keys today; `LoadConfig` uses `configengine.Load` (**strict**, per the Config Strictness Invariant — loom is in the strict set) with `ConfigTemplate()`, and validates the three model-specs plus rejects negative timeouts.
The new `selfreport` key must be added to both `Config` and `internal/loomengine/template.yaml`, with a comment in the template matching the existing style.
Note the boolean-default trap: a `bool` field's zero value is `false`, so "default true" must come from the template's own `selfreport: true` line (which strict load always supplies), exactly as `discussion_interactive: false` does today.

**Two guard tests every new `.lyx` accessor must be registered in.** `cmd/lyx/notransients_test.go` and `cmd/lyx/constructoranchoring_test.go` both walk loom's path constructors by name (`loomengine.LoomStatusLock`, `LoomDriverLog`, `LoomBootstrapLock`, …).
`LoomDriverLog`'s own doc comment states why it exists as an accessor at all: "cmd/lyx's transient guard walks constructors, not call sites."
The new filed-marker accessor (and its lock accessor) must be added to both tests in the same change or the guard will not cover them.

## Constraints

From `CONSTRAINTS.md`, the ones this task is bounded by:

- **Shed Producer-Seam Invariant** — `internal/shedengine` imports only stdlib, `state`, `lock`.
  This is the invariant that forces detection out of the engine.
  Not to be relaxed.
- **Told-Geometry Invariant** — an engine is handed the absolute paths it operates on and derives none of its own; no direct `internal/lyxcwd` import.
  `loomengine` is not in the bound-package list (it owns path accessors taking a `*lyxcwd.Location`), but the detector itself must stay told-input-only: no path derivation, no reads.
- **Cwd Resolution Invariant** — a module's own durable subdirectory is its own constant joined onto `AnchorPath()`.
  The new marker accessor follows `LoomDriverLog`'s exact shape: `filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, loomDirName, "<file>")`.
- **Durable-vs-Ephemeral State Invariant** — every never-tracked file lives under `.lyx`, at the mirrored subpath of its `_lyx` content, and no engine derives its own `.lyx` path (each module exposes a scratch accessor beside its durable one).
  The filed-marker is never-tracked, so `.lyx/loom/`, via a `loomengine` accessor.
- **Lyxdirs Single-Declarer Invariant** — no production file outside `internal/lyxdirs` names the `_lyx`/`.lyx` literals in path-construction context.
  Use `lyxdirs.DotLyxDirName`.
- **CLI / Cobra Invariant** — `<module>cli` imports `<module>engine`; engine never imports cli/cobra; non-empty `Short` on every command; errors are JSON via `internal/output`; every `RunE` checks `clihelp.ShouldAbort` first.
  This task adds no command and no flag, so no help-tree change.
  It does add two new production cross-module edges — `loomcli` → `selfreportengine` and `loomcli` → `shedadapters` — and **neither needs a `CONSTRAINTS.md` entry**, settled here rather than deferred: the invariant's "Package naming" clause governs the `<module>cli` ↔ `<module>engine` *naming* pair, and its two listed deviations (`stencilcli` → `internal/stencilstore`, `quarrycli` → `internal/planglyph`) are both cases where a cli's engine-side counterpart is not named `<module>engine` at all.
  It is not a ledger of every cross-module import: production `loomcli` already imports `burlerengine`, `websterengine`, `reedengine`, `fabricengine`, `landingshed`, `batcher`, `shedrecipe`, `shuttleengine`, and `planparser` (`internal/loomcli/wiring.go`, `cli.go`) with no entry for any of them.
  `loomcli` still imports `loomengine`, so its naming pair is intact and the invariant is satisfied unchanged.
- **Config Strictness Invariant** — loom is in the strict (`configengine.Load`) set.
  A new `loom.yaml` key without a matching template entry breaks every existing config.
- **Live-Substrate Spawn Observability** — not triggered: this task starts no OS process.
- **Test Tier Purity Invariant** — untagged test files perform no expensive spawns; no `gitexec.Run`/`RunGit`, `exec.Command`, `gitkit.Copy*`, `hubforge.NewHub` outside `integration`/`smoke`-tagged files.
  Every test this task adds is Tier 1 and must stay so.
- **GitHub Auth Invariant** — all GitHub authentication goes through `internal/githubclient`; no other production package shells out to `gh`.
  Satisfied by going through `selfreportengine.CreateIssue`, which is already the only door.
- **Documentation Lifecycle** — see `docs/overview.md#documentation-lifecycle`.

From `CLAUDE.md`:

- Docs land in the same commit: the module doc in `manifest/designs/` (here `self-report-tier1.md`), `docs/overview.md` if the module table or execution stack changes, `CONSTRAINTS.md` for any new cross-cutting invariant.
- `manifest/roadmap.md` moves on completing a planned item — which this is, so the Planned entry moves.
- **Markdown Link Integrity** (`CONSTRAINTS.md`): every inline markdown link under `manifest/` or `docs/` must resolve, file part and `#anchor`.
  Editing `self-report-tier1.md` and `roadmap.md` puts this task inside that gate.
- Markdown: semantic line breaks, one sentence per line, no fixed-column hard-wrap.
- Build prerequisite: `CGO_ENABLED=1` and a C compiler on `PATH` (Quarry CGO Requirement Invariant).

Discovered during discussion, and load-bearing:

- **`ErrShedBusy` does not close the two-driver race for the crash-resume trigger.**
  `Run` takes the run lock long after `drive` takes its entry observation, so a healthy driver can finish and release inside that window and `Run` then short-circuits on `StateDone` with no busy error at all.
  The closure is a non-blocking run-lock probe taken **before** the entry read, plus the ordering that makes a driver finishing in between show up as `done` rather than `running`.
- **`history[]` must never be read destructively or compacted.**
  It stores every producer's bounce budget — and, after this task, the `<history-length>` half of every anomaly title's dedupe discriminator depends on that same append-only guarantee.
- **A ledger path from `history[].output` may not exist on disk**, the seed call's history entry has an empty `output`, and a non-empty `output` may name a producer artifact rather than a ledger.
  All three are normal, none is corruption.
- **`drive`'s current shape drops the whole error path.** The detect-and-file step has to be inserted above `drive.go`'s existing `if err != nil` early return, not appended after the success envelope, or `state: failed` and any crash-resume preceding a producer failure are lost permanently.
- **`shedengine.Result` is meaningless on a non-nil error return**, so the final status must be re-read from the file rather than taken from `Result`.

## Testing

Tier 1 for every behavioural assertion, and nothing that spawns.
The single permitted exception is the thin `drive` call-site placement assertion below, which may land in the existing `smoke`-tagged `loomcli` suite if in-process reach proves impractical — every detection, rendering, dedupe, and branch case stays Tier 1 either way, which is what the extracted-function decision buys.

**`internal/loomengine` — the detector (TDD candidate, write the table first).**
`coherence_test.go` is the model to follow: a table over hand-built `shedengine.Status` values asserting the exact set of detected anomalies.
Cases that must be covered:

- Clean `state: done` run, non-empty history, no ledger anomalies → empty result.
- Entry observation `running` + non-empty history → crash-resume.
- Entry observation `running` + **empty** history → no crash-resume (that is a fresh seed at `Preflight`, exactly the shape `contracts/specs/loom-status-spec.md`'s worked seed example carries).
- Entry observation `paused`/`blocked`/`failed` → no crash-resume (an ordinary human resume, which is what step 3b's conditional write exists for).
- Entry observation `running` + non-empty history but the run-lock probe reported **held** → no crash-resume.
  This is the r5 finding's case, so assert it directly: a live driver's run must never be reported as a crash.
- Final `blocked` with `error == "stuck with no OnStuck target"` → escalation.
- Final `blocked` with `error == "bounce budget exhausted"` → budget exhaustion.
- Final `blocked` with some other `error` → neither of the two blocked triggers (no over-matching on `state` alone).
- Final `failed` with arbitrary `error` text → producer hard-failure, and **not** classified as either blocked trigger.
- Title discriminators, and this is the case the r3 review caught, so it must be asserted directly rather than implied: take a blocked status, then build the *resumed* shape of the same unresolved halt by appending one more `stuck` entry for the same producer, and assert the two yield **byte-identical titles**.
  A length-based discriminator passes every other test in this list and fails only this one.
- Title discriminators, distinctness side: two escalations at different producers in one status history → different titles; an escalation at a producer that had previously succeeded (one `done` entry for it) versus one that had not → different titles.
- Trigger-5 titles: two ledger entries with the **same** `key` reached via history entries naming different Bouncer rows → two distinct titles, not one.
- Trigger-5 identity-dedupe, asserted as intended behaviour so it is not later read as a bug: the same `(row, key)` pair recurring after a `resolved` round yields the **same** title, and the marker therefore suppresses it.
- Ledger entry `status: open` with `rounds` length 3 → recurring finding; length 2 → not; length 5 → one anomaly, not three.
- Ledger entry `status: resolved` with a long `rounds` list → no anomaly.
- Multiple independent anomalies in one status → all reported, deterministically ordered (pin the order; an unordered result cannot be table-asserted).
- Anomaly titles are deterministic: the same input twice yields byte-identical titles.

**`internal/loomengine` — the marker accessor.**
Assert the exact path shape (`.lyx/loom/<file>`) against a fixture `*lyxcwd.Location`, mirroring how `LoomDriverLog` is asserted today, and register the new accessor in `cmd/lyx/notransients_test.go` and `cmd/lyx/constructoranchoring_test.go`.

**`internal/shedadapters` — the exported predicate and ledger accessor.**
The existing unexported `parseLedger` tests already cover the parse grammar; the new tests cover the exported surface only.
Accessor: a well-formed ledger file round-trips into the exported model, a malformed one reports failure rather than a half-filled model, an absent file reports failure without panicking, and — the round-agreement case — a file at `ledgerPath(dir, 4)` whose frontmatter says `round: 3` reports failure rather than returning a model claiming round 3.
Predicate (TDD candidate — it is the discriminator three rejected alternatives hinged on): accepts `ledgerPath(dir, n)` output for several `n`, and rejects, for the same `n`, its four same-directory `round-%d-` siblings — `verdictPath` (`round-%d-bouncer-verdict.md`), `focusPath` (`round-%d-focus.md`), `roundReviewPath` (`round-%d-review.md`), and `roundFixerReportPath` (`round-%d-fixer-report.md`) — plus a plain `decision-record.md`, a `_lyx/plan` directory path, and the empty string.
The two Burler filenames are the nearest false-accept risk of the whole set: they share the `round-%d-` prefix, live in the same `runDir`, and `BurlerProducer` publishes `roundReviewPath` into `history[].output` on its own `Stuck` returns (`internal/shedadapters/burler.go`), so a loose predicate would hand a review file to the ledger parser.
Assert the predicate against the real `ledgerPath`/`verdictPath`/`focusPath`/`roundReviewPath`/`roundFixerReportPath` helpers rather than against hand-typed filenames, so a future filename change cannot pass the test while breaking discovery.
Use a `t.TempDir()` fixture file for the accessor — file I/O is not an expensive spawn and stays Tier 1.

**`internal/selfreportengine` + `internal/selfreportcli` — the label move.**
Assert the exported default is exactly `["bug"]`, and that `selfreportcli`'s existing no-`--label` behaviour is unchanged — `cli_test.go` already covers that case, so this is a refactor the existing test must keep passing, not new coverage.

**Two seams, two boundaries — which one each assertion binds to.**
The told filing seam passed into the extracted function is the **`loomcli` unit boundary**: every branch, filing-pass-order, collapse, marker, and config assertion binds there, counting calls without reaching the engine at all.
`selfreportengine.NewGitHubClient` is the **engine boundary**: only the title/body/label rendering and the error-classification assertions bind there, where the actual `CreateIssue` argument shapes are observable.
No test binds both.

**The filing path (TDD candidate) — engine boundary.**
Swap `selfreportengine.NewGitHubClient` — the package variable that exists for exactly this — and assert: one `Issues.Create` call per detected anomaly; the rendered title matches the deterministic shape; the body contains the slug, the halt reason, and (for a recurring finding) the ledger key and rounds; the label list is the `bug` default.
Then the posture cases: a `CreateIssue` error leaves that title out of the marker and does not propagate; an anomaly whose title is already in the marker is not filed again; a marker read failure is treated as an empty marker; a marker write failure does not propagate.

**Marker round-trip.**
Write, read back, confirm the set survives; confirm a second detect-and-file over the identical status files nothing.
This is the regression test for the "every resume re-files the same issue" failure the marker exists to prevent — it deserves its own named test, not a clause inside another.
Its counterpart is equally load-bearing and equally deserves its own name: a status whose history has advanced to a *second*, distinct halt files a new issue despite the marker already holding the first halt's title.
That is the over-suppression failure the title discriminator exists to prevent, and a test that only covers the collapse direction would pass on a design that files exactly once per task forever.

**Ledger discovery against a mixed history.**
A single `history[]` containing: an empty `output` (the Bouncer seed call), a `Discussion-Write` entry whose `output` is `decision-record.md`, a Burler entry whose `output` is a `round-%d-review.md`, a Bouncer entry whose `output` is a real ledger file, and a Bouncer entry whose ledger path no longer exists.
Assert exactly one ledger is read, neither the producer artifact nor the Burler review file is ever opened, and no skip produces an anomaly or an error.
Add the generation case alongside it: a history entry whose ledger path now holds a *different* generation's file (round numbering restarted) is read as ordinary content, contributes its anomaly, and raises no error — the accepted imprecision recorded in `### ledger-discovery`, pinned as behaviour so a later reader does not mistake it for a bug.

**Carry-forward collapse (TDD candidate).**
Three ledger files for one segment — rounds 3, 4, and 5 — each carrying the same `open` key with a progressively longer `rounds` list, all reachable from `history[]`.
Assert exactly **one** trigger-5 anomaly survives the filing pass, that its body carries the round-5 entry's `rounds` list (the highest-round occurrence wins), and that `CreateIssue` is called exactly once.
Assert this at the filing-pass level rather than by observing the marker absorb the duplicates mid-pass — the collapse must be why there is one issue, not a side effect of filing order.

**Config.**
`selfreport` absent from a `loom.yaml` → strict load supplies the template's `true`.
`selfreport: false` → the extracted function returns before doing anything: assert zero told-filing-seam calls **and** zero ledger-seam reads, which is what distinguishes "skips everything" from "detects then declines to file".
Assert the entry step is skipped too, so a disabled run performs no lock probe and no extra status decode.
Follow `config_test.go`'s existing shape for the load half.

**`internal/loomcli` — the extracted detect-and-file function.**
These three cases are tests of the extracted function called **directly**, which is the whole reason the `ErrShedBusy` skip was pushed down into it (see `### detection-site`): the function takes every input told and holds the one branch, so no test needs to control what `shed.Run` returns, and none needs tmux, git, or a real run.
`internal/loomcli/wiring_test.go` and `wiring_commitstatus_test.go` are the precedent for exercising a told-input `loomcli` seam this way.
Five cases are mandatory, one per branch:

- the run error is `nil` → detect-and-file runs; the stubbed filing seam records the expected calls.
- the run error is non-nil and **not** `ErrShedBusy` → detect-and-file still runs, same expectation.
- the run error is `shedengine.ErrShedBusy` → nothing is read, nothing is detected, nothing is filed.
- `selfreport` is `false` → same total-inaction expectation, asserted on both seams (see Config below).
- the told context is already cancelled **and the entry observation carries no crash-resume** → total inaction.
- the told context is already cancelled **and the entry observation does carry a crash-resume** → exactly one filing-seam call, for that anomaly alone, and nothing else.
  This is the r6 finding's case and the asymmetry is the whole point: a crash-resume is unrecoverable after a paused persist, every other trigger is re-detected next run.
  Pair these with the complement: a **nil** run error and a live context file everything, so the skip is shown to key on the context rather than on the paused outcome.

The middle case is the regression test for the reachability defect; without it the natural implementation (append after the success envelope) passes every other test in this list.

**`internal/loomcli` — the `drive` call site.**
Separate from the three above, and deliberately thinner: assert only that the entry observation is taken before `shed.Run` and that the extracted function is called unconditionally on the one line after it, with `drive`'s envelope keys (`outcome`, `halted_producer`, `reason`, `history_length`) and its error-envelope text unchanged.
If reaching that line in-process proves to need `reed.Up()` or `fabricengine.Open`, this one assertion moves to the existing `smoke`-tagged `loomcli` suite rather than dragging the three branch cases with it — they stay Tier 1 regardless, which is the point of the extraction.

## Q&A log

- **Q:** Does gating on `ErrShedBusy` keep a second `drive` from filing a false crash-resume? **A:** [auto-pick] No — it does not close that race, and the wording claiming it did is removed. The closure is a non-blocking run-lock probe taken **before** the entry read. **Why:** `Run` acquires the lock long after `drive`'s entry observation, with `reed.Up()`, `fabricengine.Open`, parent resolution and `loomrecipe.New` in between, so a healthy driver can finish and release inside that window and `Run` then short-circuits on `StateDone` with no busy error at all. The lock is held for the whole of the other driver's `Run`, so "not held" is the statement the trigger needs; and probing *before* reading makes the residual self-correcting, since `Shed` persists its terminal state before the lock is released, so a driver finishing in between shows up as `done` rather than `running`. Residual, recorded: a crash followed by a newly-alive driver is suppressed here, and filed by that driver instead — it took its own entry observation before acquiring the lock.
- **Q:** Does the new exported ledger accessor keep the filename/frontmatter round-agreement check? **A:** [auto-pick] Yes, inherited and fail-closed, landing in the ordinary warn-and-skip path. **Why:** `parseLedger` deliberately omits the check and its in-package caller applies it, naming the failure as a judge writing the wrong round's claim into a rightly-named file. It matters doubly here because the filing pass's "keep the highest `round`" collapse keys on the frontmatter value, so a disagreeing file would win or lose that comparison on a number nothing corroborates.
- **Q:** Where does detection run — post-hoc in `drive`, an injected `shedengine` closure, or a standalone scan verb? **A:** [auto-pick] Post-hoc scan in `drive`, plus a lock-probed entry-time status read for crash-resume. **Why:** `shedengine`'s Producer-Seam Invariant pins its imports to stdlib/`state`/`lock`, and `drive` is the only production process that ever calls `Shed.Run`; crash-resume is the one signal a purely post-hoc read cannot see, because the first ordinary persist after the next producer call overwrites the `running` a dead driver left behind. (Step 3b is conditional and does *not* fire on a crash-resume; it is what makes a human resume from `paused`/`blocked`/`failed` distinguishable from one.)
- **Q:** Which package owns the detector? **A:** [auto-pick] `anomaly.go` in `internal/loomengine`, pure over told inputs. **Why:** `loomengine` already imports `shedengine` and already houses exactly this shape in `coherence.go` — a pure, no-I/O, exhaustively table-tested Tier-1 validator over a decoded `shedengine.Status`; keeping it pure also keeps `loomengine` free of a `shedadapters` import.
- **Q:** How does the detector see finding identity? **A:** [auto-pick] `shedadapters` gains an exported ledger accessor *and* an exported ledger-path predicate; `loomcli` reads and hands the parsed models in. **Why:** `bouncerfiles.go` already declares itself owner of the bouncer file contracts and their strict parsers, and `round.go` owns their filename shapes, matching the repo's other sole-parser invariants. This is a new production edge (`loomcli` imports `shedadapters` only in `smoke_attachprobe_test.go` today) and introduces no cycle, since `shedadapters` imports no `loom*` package.
- **Q:** Which triggers, at which thresholds? **A:** [auto-pick] Five — crash-resume, escalation-to-human, bounce-budget exhausted, producer hard-failure, and a ledger entry still `open` after ≥3 rounds. **Why:** the first four read exact fields `Shed.Run` writes verbatim (the two blocked reasons are exact literals; `failed` matches on `state` alone since its `error` is whatever the producer returned). The fifth is the design doc's "same finding" clause, which only the ledger can answer. Threshold 3 against `max_bounces: 5` (one unit of which the Bouncer's seed call permanently consumes) fires while the segment is still alive rather than only after it has degenerated into the exhaustion trigger.
- **Q:** How are ledger files found, given that non-Bouncer rows also publish a non-empty `output`? **A:** [auto-pick] Every non-empty `output` is offered to `shedadapters`' exported ledger-path predicate; only accepted paths are read. **Why:** `Bouncer.settle` publishes the ledger path on both the `Done` and `Stuck` branches and `Shed.Run` persists it, so the trail exists on disk — but `SingleLLMProducer.mapOutcome` publishes producer artifacts the same way, so "non-empty" is not a discriminator. Both candidate discriminators (row names, filename shape) are knowledge another package owns, so the predicate is exported from the owner rather than re-declared in the caller. Three skips, all non-fatal: empty pointer (`seedCall`), predicate rejection, and read/parse failure including an already-wiped ephemeral ledger. The predicate's sharpest job is rejecting the Burler's own `round-%d-review.md` and `round-%d-fixer-report.md`, which share the `round-%d-` prefix and the same run directory and are published into `output` the same way.
- **Q:** Does the detect-and-file step run when `shed.Run` returns an error? **A:** [auto-pick] Yes, on both returns — and the step is inserted above `drive.go`'s existing `if err != nil` early return, with three skips (`selfreport: false`, `ErrShedBusy`, done context) all living inside the extracted function. **Why:** the `callErr != nil` arm persists `state: "failed"`, so the natural "append after the success envelope" shape would drop that whole class — and because the entry-time crash observation lives only in this process's memory, a crash-resume followed by a failing producer would be lost permanently, unrecoverable by any later `drive`. It also forces the final status to be re-read from the file, since `Result` is documented meaningless on an error return.
- **Q:** One issue per anomaly, or one aggregate per run — and per task or per occurrence? **A:** [auto-pick] One per *occurrence*, with a per-trigger discriminator: `<producer>#<success-count>` for the three halt triggers, `<producer>@<history-length>` for crash-resume, and `<bouncer-row> — <ledger-key>` for the recurring finding. **Why:** the marker exists to suppress re-filing the same halt on resume, but a bare `<kind> — <slug>` title overshoots into suppressing every later distinct halt in the same task. The discriminator must be stable across re-observations *and* distinct for a later occurrence, and `len(history)` fails the first half: `StateBlocked` does not short-circuit `Run`'s step 1, so every resume of an unfixed halt appends another `stuck` entry and would mint a new title — a fresh issue per resume. Counting the halting row's own `done` entries is invariant under exactly that append and moves only on a genuine success, so it separates "same unresolved halt" from "halted here again after succeeding". Crash-resume keeps the positional form because it is observed at most once and needs no stability, only finer distinctness. Trigger 5 needs the row name because a ledger `key` is LLM-authored and scoped to one segment, so the same key in two segments would otherwise collapse.
- **Q:** Does the per-occurrence premise apply to trigger 5 too? **A:** [auto-pick] No — trigger 5 is deliberately identity-deduped: one issue per `(bouncer-row, ledger-key)` pair for the task's lifetime, including across an `archiveRunDir` generation boundary. **Why:** the issue's subject is already "this finding keeps coming back", so a second issue about the same finding recurring adds a thread rather than information, and the existing issue is the right place for it. Recorded explicitly so the plan does not read the section's general premise as covering it.
- **Q:** Carry-forward means one recurring finding appears in every later round's ledger — what stops that filing N identical issues? **A:** [auto-pick] A pinned four-step filing pass: collapse by title (highest `round` wins), filter against the marker, file, then record each title immediately after its own success. **Why:** without an explicit collapse the duplicates are absorbed by the marker mid-pass, which yields the right issue count for the wrong reason and breaks as soon as filing order or marker timing changes. Highest-round-wins is what makes the body carry the fullest `rounds` list, since carry-forward makes the latest ledger the most complete. Per-title recording is what makes a failed `CreateIssue` retry next run without re-filing its siblings. Reading only the highest-round ledger was rejected because `parseLedger` does not enforce carry-forward, so an incomplete one would silently drop findings.
- **Q:** Two of the three mandatory `drive`-wiring tests need control over `shed.Run`'s return, which Tier 1 cannot reach — how is that resolved? **A:** [auto-pick] Extract the whole detect-and-file step into one told-input `loomcli` function that owns the `ErrShedBusy` skip itself; `drive` calls it unconditionally on one line. **Why:** `drive` builds its Shed inline via `loomrecipe.New` with no seam and needs `reed.Up()` plus `fabricengine.Open` to reach that line, so the branch that most needs a regression test — detect-and-file on a non-busy error — was unwritable. Pushing the skip down makes the function the entire unit under test, so all three branches are direct Tier-1 calls; only a thin placement assertion may fall back to the existing `smoke` suite.
- **Q:** Who owns the `bug` label default for the automatic path? **A:** [auto-pick] `selfreportengine` gains an exported default, and `selfreportcli`'s existing literal switches to it. **Why:** the default lives today as a literal in `selfreportcli`'s `runCreate`, so a second `CreateIssue` caller would re-declare the string and leave two owners of one convention. Moving it to the engine is a pure refactor with no behaviour change to the manual verb, and it avoids a cli-to-cli import.
- **Q:** How is re-filing on every resume prevented? **A:** [auto-pick] A machine-local `.lyx/loom/selfreport-filed.json` marker via `internal/state`, keyed by title. **Why:** `.lyx` is the declared home for never-tracked state at the mirrored subpath, and `loomengine` already exposes four accessors in that exact directory. A durable marker would need a weft commit at a boundary `drive` does not own; network dedupe adds a fallible call per anomaly per run and breaks the moment someone closes an issue without fixing the cause. Marker loss costs at most one duplicate issue.
- **Q:** Who consults the `selfreport` knob, and does `false` skip detection too or only filing? **A:** [auto-pick] It is a told input of the extracted detect-and-file function (so that call stays unconditional), and `false` skips **everything** — including the entry lock-probe-and-read, which `drive` does guard on the same bool. No status read, no ledger read, no marker touch. **Why:** detection with filing off has no observable effect, so the reads would be pure cost — and leaving the *entry* step unconditional would have kept paying a lock probe plus a status decode on every `drive` despite that rationale. Keeping the knob inside the function for the pass itself puts all three skips in one told-input test table rather than putting one back behind `drive`'s untestable closure.
- **Q:** An operator Ctrl-C returns `RunPaused` with a nil error, and `CreateIssue` builds its 30s timeout from `context.Background()` — does a stopped run still file? **A:** [auto-pick] No: a done context is a third skip alongside `ErrShedBusy` — with one exception, a detected crash-resume, which is filed anyway before the skip returns. **Why:** without the skip an operator stop is followed by up to 30 seconds of network work per anomaly, since `CreateIssue` never observes the cancellation. For triggers 2–5 nothing is lost, because a paused run resolved none of its anomalies and the next `drive` re-detects them. Trigger 1 is different and needs the exception: the cancellation arm persists `StatePaused`, so the next `drive` sees `paused` rather than `running` and the in-memory crash observation is gone permanently — the same loss this design already refuses to accept on the error path. The exception costs at most one `CreateIssue`, and only when a crash actually preceded the stop.
- **Q:** A stale ledger path can resolve to a *later generation's* live file, since `archiveRunDir` recreates the run directory and restarts rounds at 1 — what is the disposition? **A:** [auto-pick] Accepted imprecision, recorded, with no generation discriminator added. **Why:** the content read is a real currently-open finding and collapse-by-title reduces the duplicate reachable from newer history entries, so the harm is provenance, not correctness. The second consequence is stated plainly instead: the ≥3 threshold counts rounds within the current generation, so a finding surviving two generations at two rounds each does not trip it — a deliberate under-report. Deriving a generation counter would mean globbing archive directories in the caller, over a naming convention `shedadapters` owns.
- **Q:** On by default, or opt-in? **A:** [auto-pick] On by default, with `selfreport: false` in `loom.yaml` to disable. **Why:** the tier's whole argument is that it costs nothing and needs no watcher, so opt-in would leave the unattended case filing nothing — but a fork or CI run must be able to stop filing into the hardcoded upstream target. Strict config means the key must ship in `template.yaml`, whose `selfreport: true` line is what makes the default true despite `bool`'s `false` zero value.
- **Q:** What happens when GitHub is unreachable? **A:** [auto-pick] `logger.Warn` and continue; the run's outcome, exit code, and envelope are untouched, and the title stays out of the marker so the next run retries. **Why:** a diagnostics side-channel must never turn a completed loom run into a reported failure; `Bouncer.runSeedSpawn` sets the same precedent, and omitting the marker write gives a retry with no retry loop.
- **Q:** What goes in the issue body? **A:** [auto-pick] Anomaly kind, slug and parent, final `state`/`current_producer`/`error` verbatim, the relevant history slice, and the ledger key/rounds/status for a recurring finding. **Why:** the reader will not have the worktree — the status file is under `_lyx` and the ledgers under `.lyx`, in a worktree that may already be torn down — and every field named is already in hand at filing time.
- **Q:** Do the new `loomcli` → `selfreportengine` and `loomcli` → `shedadapters` edges need recording as CLI/Cobra Invariant deviations? **A:** [auto-pick] No, and this is settled here rather than handed to the plan. **Why:** the invariant's two listed deviations (`stencilcli` → `stencilstore`, `quarrycli` → `planglyph`) are both cases where a cli's engine-side counterpart is *not named* `<module>engine`; the clause governs that naming pair, not every cross-module import. Production `loomcli` already imports `burlerengine`, `websterengine`, `reedengine`, `fabricengine`, `landingshed`, `batcher`, `shedrecipe`, `shuttleengine`, and `planparser` with no entry for any of them, and it still imports `loomengine`, so its naming pair is intact. No `CONSTRAINTS.md` change; nothing is handed forward.
