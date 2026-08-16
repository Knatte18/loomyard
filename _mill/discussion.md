# Discussion: Shed engine adapters: SingleLLMProducer, perch, Webster

```yaml
task: 'Shed engine adapters: SingleLLMProducer, perch, Webster'
slug: shed-adapters
status: discussing
parent: main
```

## Problem

`internal/shedengine` shipped as a skeleton: it walks one flat, ordered producer list, honoring resume, crash-recovery, and pause at producer granularity, and it drives every entry through one seam — `ShedProducer.Call(ctx) (Outcome, OutputPointer, error)`.
Nothing satisfies that seam yet.
Shed can be exercised only against throwaway stub producers in its own tests, so no real engine — not `shuttle`, not `perch`, not `Webster` — can currently appear in a Shed producer list.

**Why now:** Shed's skeleton is Done on `manifest/roadmap.md`, and the adapters are Planned item 1 — the immediate next step, and a hard prerequisite for the separate later `loom: Discussion-phase producers` task, which needs `SingleLLMProducer` for `Discussion-Write` and the perch adapter for `Discussion-Review`.
This task builds only the generic wrappers.
It is deliberately mechanical and deterministic: pure Go, no prompt design, nothing product-specific — consistent with doing all mechanical work before any LLM-prompt work.

The adapter count scales with the number of distinct **engines**, never with the number of producers (`manifest/designs/shed.md`'s "Engine adapters" section).
Three engines are in play today, so three adapters:

- **`SingleLLMProducer`** — one generic, reusable `ShedProducer` for the simple single-agent-spawn case, over the shipped `shuttleengine.Spec` → `shuttle.Run` pattern.
  Two concrete producers configuring this same type is one adapter instantiated twice, not two adapters.
- **The perch adapter** — one adapter reusable by *every* review-gate producer regardless of which artifact it reviews, wrapping perch's gate loop (burler rounds until `APPROVED`/`STUCK`).
- **The Webster adapter** — its own adapter for Webster's black-box multi-spawn engine; the granularity rule is one adapter per such engine, not one per producer that uses it.

## Scope

**In:**

- A new package `internal/shedadapters` holding all three `ShedProducer` implementations.
- `SingleLLMProducer` — wraps a caller-supplied `shuttleengine.Spec` source, archives stale output files, runs one shuttle spawn, maps the four shuttle outcomes onto Shed's verdict vocabulary.
- `PerchProducer` — resolves its per-`Call` run identity (the run-id Decision), builds its engine through a caller-supplied factory so the ctx bridge is installable, maps `APPROVED`/`STUCK`/`PAUSED` onto Shed's vocabulary, and reports an empty `OutputPointer` (gate producer).
- `WebsterProducer` — wraps `websterengine.Run` with `Fresh` fixed `false`, maps its `RunResult.Outcome` and its errors (one exception, then a default) onto Shed's vocabulary, and reports `SummaryPath` on `Done`.
- Two narrow local seam interfaces plus a func-typed Webster seam, each with a compile-time-proof line, so every adapter is fakeable at tier 1.
- Context-cancellation handling for **all three**: an entry check, and an exit rule where `ctx.Err()` overrides every result *except* a genuine success verdict, which is returned as `Done` so finished work is never discarded. Plus a mid-run pause-seam bridge for **perch only** — Webster and `SingleLLMProducer` are deliberately bridgeless, each bounded by its own timeout (see the cancellation Decisions).
- Tier-1 (untagged) tests with fakes for all three seams.
- Docs in the same commit — the exact edit list is pinned in the "Doc set" Decision below: a `doc.go` for the new package, five named corrections in `manifest/designs/shed.md`, three edits in `docs/overview.md`, and three in `manifest/roadmap.md`.

**Out:**

- **No loom wiring of any kind.** No producer definitions, no producer list, no `loomengine` changes, no CLI changes. That is the separate later `loom: Discussion-phase producers` task.
- **No new prompt content.** No new stencil, no stencil edits, no prompt templating inside the adapters.
- **No live-session reattach.** `SingleLLMProducer` is respawn-only (see the "Reattach out of scope" Decision).
- **No new `shuttleengine`/`perchengine`/`websterengine` surface.** The adapters consume the shipped APIs as they stand; if an adapter appears to need new engine surface, that is a signal the mapping is wrong, not a licence to widen an engine.
- **No `Finalize` producer.** Genuinely new code, its own later task (`manifest/designs/finalize.md`).
- **No mechanical Go-function producer adapter.** A plain Go function already satisfies `ShedProducer` directly; there is nothing to build.
- **No changes to `internal/shedengine`.** Its seam is shipped and correct; the adapters adapt onto it, never the reverse.

## Decisions

### Package layout — one `internal/shedadapters` package

- Decision: all three adapters live in one new package, `internal/shedadapters`, together with their three seam interfaces and any shared context/pause helper.
- Rationale: mirrors the shipped precedent one level down — `internal/perchengine/adapter.go` holds `burlerAdapter`, the `treadleengine.RoundRunner` implementation, so the adapter lives in the **caller** of the seam-owning engine, not in the engine itself.
  Applied here, the caller of Shed holds the wrappers, not `shedengine`.
  `CONSTRAINTS.md`'s Shed Producer-Seam Invariant already forbids `shedengine` from importing them ("producers adapt onto the package's own `ShedProducer` seam in their own packages").
- Rejected: putting each adapter beside its engine (`perchengine/shedproducer.go`, `websterengine/shedproducer.go`) — `perchcli` and `webstercli` both ship as standalone CLIs, so this would make two shipped-CLI engines depend on `shedengine` for a consumer neither of them has.
  Rejected: three separate packages (`shedllm`/`shedperch`/`shedwebster`) — three near-empty packages and no shared home for the common ctx/pause helper.

### Prompt composition — a caller-supplied `Spec` source, never adapter-side templating

- Decision: `SingleLLMProducer` is parameterized by a caller-supplied `shuttleengine.Spec` source — a `func() (shuttleengine.Spec, error)` (or an equivalent one-method seam), evaluated per `Call`.
  The adapter never reads a stencil, never templates, never composes prompt text.
  The brief's "Input-format pointer, Output-format pointer, one instruction file" parameterization is realized by whatever the product's `Spec` builder does, not by the adapter.
- Rationale: this is not a new pattern being introduced — it already ships.
  `loomengine.DiscussionSpec` (`internal/loomengine/discussion.go:19`) is exactly `func DiscussionSpec(layout *lyxcwd.Location, stencilsDir string, cfg Config, reg modelspec.Registry, slug string, autonomous bool) (shuttleengine.Spec, error)`, and `internal/loomengine/prompt.go`'s `composePrompt` already reads `loom-template-discussion` via `stencilstore.Read` and substitutes slug/paths/mode rules.
  Keeping composition on the product side honors the out-of-scope "no new prompt content" rule strictly and keeps the Stencil Ownership Invariant's call-time-read discipline where it already lives.
- Rejected: the adapter composing from a stencil name plus the two pointers — needs a new generic stencil, which is new prompt content and explicitly out of scope, and could not perform loom's existing substitution anyway.
  Rejected: the adapter reading a named stencil verbatim as `Prompt` and appending pointer paths — no new stencil, but also no substitution, so loom's own discussion prompt still could not use it.

### Stale output files — archive, then respawn

- Decision: `SingleLLMProducer` **requires every `Spec.OutputFiles` entry to be an absolute path** and returns an error naming the offending entry when one is relative.
  Given that, before starting a shuttle run it renames every existing entry to a timestamped sibling (`<name>-<UTC-timestamp><ext>`, with a numeric suffix on collision), then starts the run.
  The timestamp comes from an **injected clock seam** — `NewSingleLLMProducer` takes a `now func() time.Time`, and a nil value selects `time.Now`.
  The `OutputPointer` it reports on `Done` is `Spec.OutputFiles[0]`, the first entry.
- Rationale: `shuttleengine.Spec.validate` hard-rejects a pre-existing `OutputFiles` entry (`internal/shuttleengine/spec.go:136-143`) — the file contract's "done is bare file existence" rule means a stale file would classify a run done on its first turn end.
  Shed re-calls a producer **unconditionally** on every resume and every `OnStuck` bounce-back (`manifest/designs/shed.md:253-257`), so without this the adapter breaks permanently the second time it runs.
  Archiving rather than deleting reuses an existing, deliberately reusable in-repo pattern: `websterengine`'s `archiveStaleOutcome` (`outcome.go:77`) and `ArchiveStaleSummary` (`summary.go:77`), whose own doc comment names it "the same archive-never-refuse timestamp-rename discipline".
  The prior attempt stays inspectable for a human diagnosing a bounce loop.
  **Why absolute entries are a precondition rather than something the adapter resolves:** `Spec.validate` resolves relative entries against the worktree root, but it runs *inside* `Runner.Start` — after the archive step (`internal/shuttleengine/spec.go:115-134`).
  An adapter that archived relative entries would resolve them against its own process cwd, which the Cwd Resolution Invariant forbids it from even reading.
  Requiring absolute entries keeps the adapter geometry-blind, and costs the caller nothing: `loomengine.DiscussionSpec` and `burlerengine` both already build absolute `OutputFiles`.
  **Why the first entry is the pointer:** both shipped `Spec` builders already order the primary artifact first — `burlerengine` emits `[ReviewPath, FixerReportPath]` (`internal/burlerengine/engine.go:136`) and `loomengine.DiscussionSpec` emits `[decisionRecordPath, supportLogPath]` (`internal/loomengine/discussion.go:43`).
  A first-entry rule needs no new field on `Spec`, no extra constructor argument, and no per-producer configuration; it is documented as a convention a `Spec` source honors by ordering.
  **An empty `OutputFiles` is not this adapter's error to raise, but it must not be indexed either:** `Spec.validate` already rejects an empty list inside `Runner.Start` (`internal/shuttleengine/spec.go:119-121`), so the seam returns that error and the adapter propagates it.
  The adapter still guards the `Done` branch rather than indexing `OutputFiles[0]` blindly — an empty list there would panic, and a panic inside a long unattended Shed run is exactly what `shedengine`'s own nil-`Producer` validation exists to avoid.
  **Why an injected clock:** both cited precedents take exactly this seam — `archiveStaleOutcome(websterDir string, now func() time.Time)` (`internal/websterengine/outcome.go:77`) and `ArchiveStaleSummary` (`summary.go:77`) — and `firstFreeArchivePath` is unexported, so the collision helper is re-implemented locally anyway.
  Without the seam, the collision-suffix path can only be tested by hoping two archive calls land inside the same wall-clock second; with it, a fake clock returning a fixed instant makes that test deterministic.
  This is the one place `manifest/designs/shed.md:231`'s "no injectable clock" rule does not apply — that rule governs `Shed`'s own `history[].at`, a field Shed writes and tests assert structurally, not an adapter-side filename whose collision behavior is the thing under test.
- Rejected: silently resolving a relative entry — needs a worktree root the adapter must not resolve.
  Rejected: an explicit primary-output constructor argument or index — duplicates a value `Spec` already carries, and lets the two disagree.
  Rejected: reporting every output file — `OutputPointer` holds exactly one `Path`.
  Rejected: deleting instead of archiving — loses the prior attempt's artifact for no gain.
  Rejected: returning `Done` when every output already exists — `shed.md:253-257` rejects exactly this shortcut at Shed level, and its reason applies identically here: after a bounce-back the previous attempt's output is still on disk, and existence alone cannot distinguish stale from fresh.

### Reattach out of scope — `SingleLLMProducer` is respawn-only

- Decision: `SingleLLMProducer` does not reattach to a live shuttle session.
  It archives stale outputs and respawns.
  The limitation is named explicitly in the package doc, and both places `manifest/designs/shed.md` states the claim — `:255` and `:261` — are corrected in the same commit (see the "Doc set" Decision).
- Rationale: `shuttleengine` exposes no reattach entry point — `FindRun` (`internal/shuttleengine/rundir.go:150`) returns a `RunState` value and a directory, not a waitable `*Run`.
  Building one is new `shuttleengine` surface, which is scope growth into an already-shipped module and is not thin-wrapper work.
  `shed.md:255` currently claims "`SingleLLMProducer` wraps `shuttle`+`reed` and does this internally", referring to the full "live session / fresh output / respawn" three-case discipline.
  That claim would become false the moment this adapter ships, so correcting it is part of the task, not optional cleanup.
- Rejected: adding a `shuttleengine.Attach(guid) (*Run, error)` and wiring it — real scope growth, and no current caller needs the third case.

### Context cancellation — entry check, exit precedence, and a bridge only where one is installable

- Decision: every adapter shares two parts of the discipline unconditionally.
  (1) **Entry:** if `ctx.Err() != nil`, return it immediately without starting anything.
  (2) **Exit:** on return, `ctx.Err() != nil` wins over the engine's result **except when the engine reached a genuine success verdict** — shuttle `OutcomeDone`, perch `APPROVED`, Webster `done`.
  A success verdict is returned as `Done` even under a cancelled context; every other result under a cancelled context (a `Stuck`-equivalent verdict, a `PAUSED`, or any engine error) is replaced by the ctx error.
  The third part — a mid-run bridge that lets a cancel reach the running engine — is installed for **perch only**, because perch is the only one of the three whose pause seam is a callback the adapter can supply.
  The per-engine mechanics and their accepted consequences are the three Decisions immediately below.
- Rationale: none of the three engines take a `context.Context` — `shuttleengine.Runner.Run(Spec)`, `perchengine.Engine.Run(p, runDir, scratchDir, stencilsDir)`, and `websterengine.Run(deps, opts)` are all synchronous and ctx-free — yet `ShedProducer`'s second unenforceable obligation is that cancellation surfaces as a non-nil error, never as `Stuck` (`internal/shedengine/producer.go:26-32`).
  Shed's routing predicate is the context's own state (`ctx.Err() != nil`), which routes to the clean `state: "paused"` exit rather than `failed`.
  Entry and exit checks alone satisfy that obligation in every case; the bridge is a responsiveness improvement, available only where an engine already exposes a callback for it.
  **Why a success verdict survives cancellation — otherwise the exit check throws away finished work.** Since shuttle and Webster carry no mid-run bridge, a cancel lets the engine run to completion.
  If the exit check then converted a genuine `Done` into the ctx error, Shed would append **no** history entry and leave `current_producer` unchanged, so the next `Call` would archive the freshly written, valid output and respawn the entire session — paying for the same LLM run twice and discarding a completed artifact, for what was an operator stop.
  Returning the `Done` instead loses nothing: Shed records the history entry, advances `current_producer`, and its own top-of-loop check (`shed.md`'s step 3, `ctx.Err()` alongside `pause_requested`) then exits `paused` on the very next iteration, before any further producer is called.
  The run still stops immediately at the operator's request; it simply keeps the work already paid for.
  **This does not weaken the seam obligation.** `producer.go:26-32` requires that cancellation never be reported as `Stuck` — the hazard being that Shed cannot distinguish a cancelled `Stuck` from a genuine one and would spend bounce budget on an operator stop (`shed.md:37`).
  A completed `Done` is not cancellation being reported at all; it is a finished producer, and it consumes no bounce budget.
  Every path where the hazard is live — a `Stuck`-equivalent verdict, a `PAUSED`, an engine error — still yields the ctx error, unchanged.
- Rejected: converting a completed `Done` into the ctx error for uniformity — throws away a finished artifact and a paid-for LLM session to express something Shed's own next loop iteration expresses anyway.
  Rejected: running the engine in a goroutine and `select`ing on `ctx.Done()` — abandons a live engine mid-write, leaking tmux panes and racing `state.json`; the one option that risks the real hazard.
  Rejected: forcing a uniform bridge onto all three by inventing the missing mechanism — for Webster and shuttle that means writing an operator-owned flag file or driving a pane, both rejected in their own Decisions below.

### Perch cancellation bridge — installed via a construction-time seam

- Decision: `PerchProducer`'s seam is an engine **factory**, not a bare runner: a `func(pauseRequested func() bool) PerchRunner` supplied by the caller, where `PerchRunner` is the narrow `Run(Profile, runDir, scratchDir, stencilsDir string) (Result, error)` interface.
  `Call` invokes the factory once per call with `func() bool { return ctx.Err() != nil }`, then runs the returned engine.
  The caller's factory closes over the burler, shuttle, config, and layout it already holds and returns `perchengine.New(burler, shuttle, cfg, layout, perchengine.Options{PauseRequested: bridge})`.
- Rationale: `PauseRequested` is a **construction-time** field of `perchengine.Options`, consumed by `perchengine.New` (`internal/perchengine/engine.go:41-67`).
  It cannot be installed through a `Run(...)` seam over an already-constructed engine, so a seam that took a built `*perchengine.Engine` would make the bridge unbuildable and the corresponding test unwritable.
  A factory keeps the told-not-derived discipline intact — the adapter is told how to build its engine and still constructs no geometry, resolves no config, and knows nothing about burler or shuttle — while making the bridge both installable and fakeable at tier 1.
  Responsiveness is round-granular: `PauseRequested` is checked between rounds, and a round is one burler spawn, which is the correct granularity for an orderly drain.
- Rejected: `PerchProducer` constructing the engine itself from burler/shuttle/cfg/layout — widens what the adapter is told from one seam to four collaborators, and drags `burlerengine` into `shedadapters`' import set for no gain.
  Rejected: dropping the perch bridge — perch is the one engine where the bridge costs nothing but a closure, and its round loop can run for a long time.

### Perch run identity — a run-id that advances only past a terminal block

- Decision: `PerchProducer` is told a `runDirBase`, a `scratchDirBase`, and a `runIDPrefix` (validated with `perchengine.ValidRunID`), never a single fixed `runDir`.
  Its run-id has **two** varying segments, not one: `<prefix>-<profileHash[:8]>-<N>`, where `profileHash` is `perchengine.ProfileHash(p)` over the told `Profile`.
  Per `Call` it computes that hash, then resolves the current attempt **within that hash's namespace**: starting from the highest existing `<prefix>-<hash8>-<N>` on disk (N starting at 1), it calls `perchengine.TerminalOutcome(runDir, scratchDir)`;
  a **non-terminal** block is reused verbatim, so perch resumes its own in-flight rounds, and a **terminal** block advances to `<prefix>-<hash8>-<N+1>`, a fresh directory.
  `runDir` is `filepath.Join(runDirBase, runID)` and `scratchDir` is `filepath.Join(scratchDirBase, runID)` — the exact pairing `perchcli` already uses (`internal/perchcli/pause.go:63,80`).
  The attempt number is discovered from disk on every `Call`, never held in adapter memory, so a process restart resolves the same attempt.
  **The scan rule:** read `runDirBase`'s directory entries, keep the directories whose names match `<prefix>-<hash8>-<N>` for **this** `Call`'s `hash8`, with `N` a positive decimal integer and no leading zeros, and take the highest `N`; no match (or an absent base directory) starts at `N = 1`.
  Entries carrying a different `hash8` are ignored, never adopted and never deleted.
  **Scratch dir first:** before probing a candidate, the adapter `os.MkdirAll`s `filepath.Join(scratchDirBase, runID)`.
  **Error disposition:** a `TerminalOutcome` probe error against an existing scratch dir — a genuinely unreadable or corrupt `state.json`, as distinct from a missing one, which returns `("", false, nil)` — propagates as the adapter's own error, failing the `Call`. So does a `ReadDir` failure on the base and a `MkdirAll` failure on the scratch dir.
  An absent scratch dir is explicitly **not** an error: it is created, and the probe then reports non-terminal.
- Rationale: `treadleengine.loadOrInitState` refuses a terminal run dir outright — `if existing.Outcome != "" { return ... "this block already finished (%s)" }` (`internal/treadleengine/state.go:126-128`) — and its own doc comment says "treadle never re-opens a finished block", with the CLI's documented remedy being a fresh `--run-id`.
  A `PerchProducer` told one fixed `runDir` would therefore work exactly once: after the gate returned `APPROVED` or `STUCK`, Shed's unconditional re-call — a crash resume, or the `OnStuck` bounce-back this adapter exists to serve — would get an error and Shed would write `state: "failed"` instead of re-running the gate.
  That is the same stale-terminal-state hazard already solved for `SingleLLMProducer` (archive-then-respawn) and solved inside Webster itself (`Run` archives its own stale outcome and summary, `internal/websterengine/runlevel.go:440-446`), left unsolved for the one adapter whose bounce loop is this task's motivating use case.
  Advancing only past a **terminal** block is what keeps both properties: perch's own crash-resume survives (an interrupted block is re-entered, not abandoned mid-ladder), and a completed block never blocks the next attempt.
  **Why the profile hash is in the id — treadle has a *second* refusal branch, and the N-counter alone does not answer it.** Immediately after the terminal check, `loadOrInitState` also refuses a **non-terminal** block whose recorded `ProfileHash` differs from the one it was handed: `"run dir %s was started with a different profile; use a fresh --run-id"` (`internal/treadleengine/state.go:130-132`).
  A rule that reused any non-terminal `<prefix>-<N>` verbatim would wedge that producer permanently the first time an operator edits `perch.yaml` mid-bounce-loop — a round-caps or judge-model change after a failed gate is exactly what an operator does — because every later `Call` would rescan to the same non-terminal `N` and re-error, with no advancement path and no remedy expressible from Shed.
  Putting the hash in the id dissolves the branch rather than handling it: a changed `Profile` yields a different `hash8`, so the scan opens a fresh namespace at `N = 1` and the old block is never re-opened under the wrong profile.
  This is not an invention — it is the shipped convention. `perchcli`'s own `deriveBlockRunID` calls `perchengine.DeriveRunID(profilePath, hash)`, which is literally `<sanitized-basename>-<hash[:8]>` (`internal/perchengine/identity.go:57-63`);
  the adapter's `runIDPrefix` plays the basename's role, and the `-<N>` suffix is what the standalone CLI does not need because it has no unconditional re-caller behind it.
  The stale-namespace directories left behind are the same artifact `perchcli` leaves when a profile changes, inspectable and never reaped by this adapter.
  `TerminalOutcome` exists for exactly this caller-side question and is already re-exported by `perchengine` and consumed by `perchcli` (`pause.go:94`).
  **Why the scratch dir must be created before probing, not merely told:** `perchengine.TerminalOutcome` reaches `treadleengine.TerminalOutcome`, which takes its read lock at `<scratchDir>/state.json.lock` (`internal/treadleengine/state.go:147-150`), and `lock.AcquireReadLock` opens the lock file without creating its parent (`internal/lock/lock.go:44-50`) — so the probe errors outright when the directory is absent.
  `perchcli` already does exactly this `os.MkdirAll` for the same reason, with a comment saying `TerminalOutcome` "needs the directory to exist before it can even acquire its read lock" (`internal/perchcli/pause.go:80-84`).
  This matters here specifically because `runDirBase` lives under the tracked, fabric-synced `_lyx` tree while `scratchDirBase` lives under the never-tracked `.lyx` sibling (the Durable-vs-Ephemeral State Invariant): a `<prefix>-<hash8>-<N>` run dir with **no** scratch sibling is a perfectly normal state after a fresh clone or on a second machine.
  Without the `MkdirAll`, the stated propagate-the-error rule would convert that ordinary state into a permanent producer failure.
  This is not path derivation — the same join the run itself uses, onto a told base.
  Propagating a genuine probe error rather than treating it as non-terminal is the fail-loud reading: treating a corrupt `state.json` as "not finished" would hand it straight to `loadOrInitState`, which refuses it again with a less specific message.
- Rejected: minting a fresh run dir on every `Call` — discards perch's in-flight round state on a crash resume, throwing away exactly the expensive internal progress a bespoke producer's own recovery exists to protect.
  Rejected: archiving or deleting the terminal run dir in place and reusing the id — destroys the previous attempt's round history, which is the audit trail a human reads when diagnosing a bounce loop; treadle's own remedy is a new id, not a cleared one.
  Rejected: a caller-supplied `func() (runDir, scratchDir string, err error)` source evaluated per `Call` — pushes the terminal-vs-in-flight policy onto a caller that does not exist yet, shipping an adapter whose correctness depends on an unenforced contract.
  Rejected for the hash branch: advancing `N` on a hash-mismatch error instead of namespacing by hash — it would work, but it detects the mismatch only by *provoking the error*, leaving a stale non-terminal block permanently shadowed under a lower `N` and making the run-dir sequence depend on failure history rather than on identity.
  Rejected: reading the recorded `profileHash` out of `state.json` to compare it — `treadleengine.runState` is unexported, so production code would hand-parse a private schema, which is exactly the coupling `TerminalOutcome` exists to avoid.

### Webster cancellation — no bridge, bounded by Master's own timeout

- Decision: `WebsterProducer` installs **no** mid-run bridge. Entry and exit checks only.
  A cancel during a Webster producer is not observed until Master reaches a terminal outcome or its own `MasterTimeoutMin` wall-clock deadline elapses.
  At that point a `done` is returned as `Done` (the work is kept, and Shed pauses at its next loop top), while any other result becomes the ctx error and Shed routes to `paused` directly.
  This consequence is stated in the package doc, not left implicit.
- Rationale: Webster exposes no pause callback. Its pause is an on-disk flag file under the scratch dir, written by `RequestPause` and polled by the batch loop (`internal/websterengine/pause.go`, `beginbatch.go`).
  Bridging into it would require a ctx-watching goroutine writing that flag — which (a) is the operator's own channel, so writing it from an adapter conflates the two channels the ctx-only pause Decision exists to keep separate, (b) races `Run`'s own `ClearPause` calls, and (c) risks leaving the flag set after a cancelled run, permanently pausing the next invocation.
  The bound is real rather than open-ended: Webster's run is capped by its own `MasterTimeoutMin` config key, which the product sets.
- Rejected: the ctx-watching flag writer — the three hazards above, each concrete.
  Rejected: adding a callback seam to `websterengine` — new engine surface, out of scope.

### `SingleLLMProducer` cancellation — no bridge, bounded by `Spec.Timeout`

- Decision: `SingleLLMProducer` installs **no** mid-run bridge. Entry and exit checks only.
  A cancel during a shuttle run is not observed until the run reaches a terminal outcome or its `Spec.Timeout` deadline elapses.
  At that point an `OutcomeDone` is returned as `Done` (the written output files are kept rather than archived and respawned), while `asking`/`died`/`timeout` become the ctx error.
  This consequence is stated in the package doc.
- Rationale: `shuttleengine` has no pause seam of any kind — `Runner.Run(Spec)` (`internal/shuttleengine/run.go:174`) is start-and-wait, and the only mid-run levers are `Run.Interrupt`/`Run.Send`, which stop or redirect a **turn** rather than cancelling a run.
  The wait is bounded by a deadline the caller sets on every `Spec`: `Spec.Timeout`, defaulting to `cfg.RunTimeoutMin` minutes (`internal/shuttleengine/spec.go:82-91, 152-155`).
  So the accepted consequence is "up to one producer's configured timeout", not "potentially hours with no bound" — which is what makes entry/exit-only correct here and inadequate in the abstract.
- Rejected: `Start` + a ctx watcher calling `Interrupt`, then `Wait` — `Interrupt` ends the current turn without ending the run or the session (`run.go:206-216`), so the run continues and later classifies `asking` or `timeout` anyway; it also requires a live, input-ready pane (`requireReadyAgentPane`), which a cancelled run may not have. More moving parts, same bound, new failure modes.
  Rejected: shortening `Spec.Timeout` on cancellation — the deadline is already fixed inside the running engine; there is nothing to shorten.

### Every adapter is told its producer name

- Decision: each `New...` constructor takes a `name string` — the same value the caller will register in its `shedengine.ProducerDef.Name`.
  It is used for exactly two things: a log field on every message the adapter emits, and the text of every error it returns.
  It is never compared, parsed, or used for control flow, and the adapter never validates it against Shed's list (which it cannot see).
- Rationale: `ShedProducer.Call(ctx)` carries no identity (`internal/shedengine/producer.go:30-32`), and `ProducerDef.Name` lives on Shed's side of the seam, so an adapter has no way to learn who it is.
  Without a told name, a `Stuck` log line or a `state: "failed"` error string from a producer list containing two `SingleLLMProducer` instances is unattributable — and two instances of that one type is the explicitly expected shape (`manifest/designs/shed.md`: "one adapter, instantiated twice").
  The duplication with `ProducerDef.Name` is accepted: it is one string a caller passes twice at wiring time, and the alternative (widening `ShedProducer` to pass a name into `Call`) is a change to a shipped seam this task must not touch.
- Rejected: dropping identity from logs and error text — makes the two-instance case indistinguishable in exactly the situation the operator is reading the log to resolve.
  Rejected: widening `ShedProducer.Call` to carry the name — modifies `shedengine`, which this task's scope forbids.

### `StuckReason` surfaces through the log, never through the seam

- Decision: when the perch or Webster adapter maps a `STUCK`/`stuck` outcome onto `Stuck`, it emits the engine's `StuckReason` via `logger.Warn` (with the told producer name and the engine name) and returns `(Stuck, OutputPointer{}, nil)`.
  `StuckReason` never rides `OutputPointer.Path` and never becomes a non-nil error.
- Rationale: `ShedProducer.Call` returns exactly `(Outcome, OutputPointer, error)` (`internal/shedengine/producer.go:30-32`), and Shed's `Stuck` branch requires a nil error, so the seam has no detail channel — leaving it as "the returned detail" would have been a phrase with no implementation.
  `OutputPointer.Path` is the wrong carrier: Shed persists it verbatim into `history[].output` as an artifact path a human opens, and the perch Decision above pins it empty precisely because a gate produces no artifact.
  The log is the honest channel, and the underlying reason stays durably readable in each engine's own `state.json` round history regardless.
- Decision (same channel, the asking paths): the two `asking → Stuck` mappings log their own detail the same way, since the pointer is empty and the error nil there too.
  `SingleLLMProducer` logs `shuttleengine.Result.LastAssistantMessage` (set only for `OutcomeAsking`, `internal/shuttleengine/run.go:41,47`) together with `SessionID`, `StrandGUID`, and `RunDir`;
  `WebsterProducer` logs `*MasterAskingError`'s `Message`, `SessionID`, and `RunDir` (`internal/websterengine/runlevel.go:186-194`).
  Both at `logger.Warn`, both carrying the told producer name.
- Rationale for the asking paths: the agent's question *is* the account of why the producer could not finish, and the run dir is kept precisely so a human can inspect it.
  Discarding it would leave an operator with a `Stuck` in `history[]` and nothing to read — the identical gap the `StuckReason` decision closes for the gate outcomes.
- Rejected: putting `StuckReason` in `OutputPointer.Path` — overloads a path field with prose, breaking Shed's own documented meaning for it.
  Rejected: returning `Stuck` alongside a non-nil error — Shed treats a non-nil error as an engine-level failure and never reads the outcome, so the verdict would be discarded.
  Rejected: dropping `StuckReason` entirely — it is the only account of *why* a gate gave up, and a human debugging a bounce loop needs it.

### `SingleLLMProducer` outcome mapping

- Decision: `OutcomeDone` → `Done` with `OutputPointer{Path: OutputFiles[0]}`; `OutcomeAsking` → `Stuck` with an **empty** `OutputPointer`; `OutcomeDied` and `OutcomeTimeout` → non-nil error.
  The empty pointer on the `asking` path is deliberate and matches perch's: `asking` means the run ended without writing its output files (`internal/shuttleengine/engine.go:16-17`), so naming a path that does not exist would put a dead link into Shed's persisted `history[].output`.
- Rationale: `shuttleengine` classifies four outcomes, all returned with a nil error (`internal/shuttleengine/engine.go:14-26`), and `burlerengine.Run` already treats the three non-done ones as "normal loop events, not errors" (`internal/burlerengine/engine.go:91-95`).
  `asking` is a genuine producer verdict — the agent could not finish from its input — so bouncing to the upstream producer that wrote that input is exactly what `OnStuck` is for.
  `died`/`timeout` are engine-level infrastructure failures: `OnStuck` would bounce to an *upstream* producer, which is nonsense, whereas an error makes Shed write `state: "failed"` and the next run re-calls **this same** producer — the correct recovery.
- Rejected: mapping all three non-done outcomes to `Stuck` — spends bounce budget on a dead pane and routes an infrastructure failure to an unrelated producer.
  Rejected: making the died/timeout mapping depend on whether `OnStuck` is set — the adapter would have to know its own `ProducerDef`, which it does not and should not.

### Perch adapter — outcome mapping and empty `OutputPointer`

- Decision: `OutcomeApproved` → `Done` with an **empty** `OutputPointer`; `OutcomeStuck` → `Stuck` with an empty `OutputPointer` (the `StuckReason` logged, per the `StuckReason` Decision above, never a third verdict); `OutcomePaused` → error (see the pause Decision below).
- Rationale: `shed.md:29` names a review gate as the canonical **gate producer** — pass/fail only, no output pointer — whose empty pointer makes it re-run on resume as "a cheap idempotent re-check", which is exactly right for a gate.
  The per-round review files stay discoverable through perch's own `state.json` round history (`internal/perchengine/result.go:40-59`), so nothing is lost by not naming one in Shed's `history[].output`.
  **One caveat on shed.md's "cheap idempotent re-check" phrasing:** it does not hold literally here.
  Because the run-identity Decision advances to `<prefix>-<hash8>-<N+1>` past a terminal block, a re-call after `APPROVED` runs a fresh burler ladder, not a cheap re-check.
  That cost is accepted: it is the direct consequence of treadle never re-opening a finished block, and the alternative — reusing the terminal dir — is an error, not a cheaper re-check.
  What the empty pointer still buys is the property that matters: Shed never stats an artifact to decide control flow for a gate, and a gate's verdict is always re-derived rather than inferred from a file that may be stale.
- Rejected: reporting the last round's `ReviewPath` as the pointer — more observable, but declares the gate an artifact producer, contradicting shed.md's own classification and making the pointer's meaning inconsistent with `SingleLLMProducer`'s.
  Rejected: making the pointer configurable per instantiation — a knob with no caller.

### Webster adapter — `Fresh` fixed false, error mapping mirrors the shuttle rule

- Decision: `RunOptions.Fresh` is fixed `false` and is not configurable on the adapter.
  `RunResult.Outcome` `done` → `Done`; `stuck` → `Stuck` (with `StuckReason` logged, per the `StuckReason` Decision above); `paused` → error (see the pause Decision below).
  The `Done` pointer is `websterengine.SummaryPath(deps.WebsterDir)` (`internal/websterengine/summary.go:27`); every non-`Done` path carries an **empty** pointer.
  For errors the rule is stated as a **default with one exception**, not an enumeration: `*MasterAskingError` (matched via `errors.Is(err, ErrMasterAsking)`) → `Stuck` with an empty `OutputPointer`; **every other non-nil error** → non-nil error, unwrapped and returned.
  That default covers the named sentinels — `*MasterDiedError`, `*MasterTimeoutError`, `ErrRunBusy`, `ErrFingerprintMismatch`, `ErrNilBatcher` — and equally the unnamed ones `Run` also returns: plan-validation refusal (`runlevel.go:335`), the zero-batches refusal (`:347`), and `MkdirAll`/run-lock failures (`:309-321`).
- Rationale: same rule as the `SingleLLMProducer` mapping, applied to Webster's error-typed equivalents (`internal/websterengine/runlevel.go:179-235`) — asking is a verdict, died/timeout are infrastructure.
  `Fresh: true` is the destructive fingerprint-mismatch escape: it archives `state.json` and the reports dir and clears the prompts dir (`runlevel.go:140-146`, `clearRenderedPrompts` at `runlevel.go:243`).
  That must stay an explicit human act via `lyx webster run --fresh`, never something Shed triggers automatically on a resume.
  `ErrFingerprintMismatch` means the plan changed under a running Webster — a bounce-back cannot fix it, a human must.
  **The three outcome values are matched as string literals, and an unrecognised one is an error.** `outcomeDone`/`outcomeStuck`/`outcomePaused` are unexported (`internal/websterengine/outcome.go:24-28`), and the no-new-engine-surface rule forbids exporting them, so `shedadapters` compares `RunResult.Outcome` against its own `"done"`/`"stuck"`/`"paused"` literals with no compile-time link to webster's constants.
  The duplication is named rather than hidden: the `switch` carries a `default:` branch returning an error that quotes the unrecognised value, and a test row drives it, so a webster-side rename surfaces as a failing test instead of a silently mis-mapped verdict.
  This is safe in one direction by construction — `parseOutcome` already rejects any value outside the three (`outcome.go:62-66`), so webster can never hand the adapter a fourth value; only a *rename* of an existing one is the live risk, which is exactly what the `default:` branch catches.
  **Why `SummaryPath` is the pointer:** `RunResult` carries no path of its own (`runlevel.go:150-166`), but `summary.md` is Webster's human-readable account of the whole run and is guaranteed present on `done` — `ParseSummary` is a hard requirement there, and `RunResult.SummaryTitle` is "always populated for `Outcome == outcomeDone`".
  `WebsterDir` is already told via `RunDeps`, so the adapter derives no geometry to name it.
  The pointer is empty on `stuck`/`asking` for the same reason it is empty on shuttle's `asking` path: summary parsing is explicitly best-effort on the non-done outcomes, so the file may not exist and a named path could be a dead link in `history[].output`.
- Rejected for the pointer: `OutcomePath(deps.WebsterDir)` — `outcome.yaml` is Webster's internal machine handoff, not the artifact a human opens.
  Rejected: an empty pointer on `done` too — Webster is a producer with a real artifact, unlike a gate, so declaring it pointerless would misclassify it.
- Rejected: exposing `Fresh` on the adapter — a field whose only safe value is `false`.
  Rejected: mapping `ErrFingerprintMismatch` to `Stuck` — routes a plan/state divergence to an unrelated producer instead of a human.

### Pause reaches the adapters through `ctx` only

- Decision: the adapters' pause bridge is fed **solely** by `ctx.Err() != nil`.
  No adapter takes a caller-supplied `PauseRequested func() bool`, and no adapter reads Shed's status file.
- Rationale: one channel, one meaning — a `PAUSED` return from perch or Webster can then only mean cancellation, and Shed's own `ctx.Err()` predicate routes it to the clean `paused` exit rather than `failed`.
  A product wanting a mid-producer pause cancels the context it handed to `Shed.Run`.
- Accepted consequence, stated rather than discovered: **`lyx perch pause --run-id <...>` is a silent no-op against an adapter-driven run dir.**
  The pause-flag check is `perchcli`'s own closure, built in its `run` verb (`internal/perchcli/run.go:295-298`), not engine behaviour — `perchengine.Options.PauseRequested` is whatever its constructor is handed, and the adapter hands it the ctx bridge instead.
  So the shipped pause verb writes a flag nothing reads, reports success, and treadle clears it at the next `Run` entry.
  This is the direct cost of "one channel, one meaning", and it belongs in the package doc so an operator is not left believing a pause was accepted.
  The remedy is the product's own pause path (cancel the context handed to `Shed.Run`), never the standalone perch verb.
- Rejected: `ctx` plus a caller-supplied `PauseRequested` — more faithful to how a future `lyx loom pause` might work, but it creates a `PAUSED`-with-healthy-`ctx` case that Shed can only record as `failed`.
  Rejected: the adapter reading Shed's status file directly — couples every adapter to Shed's on-disk schema and its `StatusLockPath` discipline, which is exactly what the told-not-derived design avoids.

### `PAUSED` with a healthy context returns an error

- Decision: when an engine returns its `PAUSED` outcome and `ctx.Err() == nil`, the adapter returns a non-nil error whose text names it as an out-of-band pause and identifies the engine.
- Rationale: under the ctx-only pause decision this path is unreachable through Shed's own cancellation, but it remains reachable via Webster's independent operator flag file (`lyx webster pause` → `websterengine.RequestPause`), and via any future direct use of perch's pause seam.
  Shed writes `state: "failed"` with that text; a human reads it, and the next run re-calls the same producer, since `failed` deliberately does not short-circuit (`shed.md:73`).
  Semantically imperfect but recoverable and honest.
- Rejected: returning `Stuck` — spends bounce budget and routes to an unrelated producer for what was an operator action.
  Rejected: returning `Done` with an empty pointer — silently advances the run past a producer that never finished; actively unsafe.

### Seam interfaces — narrow local interfaces with compile-time proofs

- Decision: `shedadapters` declares its own narrow seam interface per engine that needs one, each with a `var _ Seam = (*concrete)(nil)` compile-time-proof line.
  Two interfaces are needed: `Run(shuttleengine.Spec) (shuttleengine.Result, error)`, satisfied by `*shuttleengine.Runner`, and `Run(perchengine.Profile, string, string, string) (perchengine.Result, error)`, satisfied by `*perchengine.Engine`.
  `PerchProducer` does not hold the second one directly — it holds the factory `func(pauseRequested func() bool) PerchRunner` the perch-bridge Decision pins, which returns it (a compile-time proof line still pins `*perchengine.Engine` against the interface).
  Webster's `Run` is already a free function, so a func-typed seam suffices there and no interface is declared.
- Rationale: follows the two shipped precedents exactly — `burlerengine.Shuttle` with `var _ Shuttle = (*shuttleengine.Runner)(nil)` (`internal/burlerengine/engine.go:20-25`) and `perchengine.Burler` with `var _ Burler = (*burlerengine.Engine)(nil)` (`internal/perchengine/engine.go:26-32`).
  This is what makes every adapter testable at tier 1 with a fake — no tmux, no real engine, no git.
- Rejected: depending on the concrete types — only the Webster one would be fakeable, leaving the other two adapters untestable without live substrate.

### Construction — `New(...)` constructors with unexported fields

- Decision: each adapter is built by a `New...(...)` constructor and holds unexported fields.
- Rationale: the adapters are engine-shaped, not config-shaped — they hold live seams a caller has already constructed, and there is no field a human hand-edits.
  Matches both `perchengine.New` and `burlerengine.New`.
- Rejected: exported-field structs mirroring `shedengine.Shed` — visually symmetric with the thing they plug into, but `Shed`'s explicit reason for that shape does not apply to a wrapper over live seams.
  That reason is stated in two places: `internal/shedengine/shed.go:6-9` ("there is no `New` constructor, which would leave a bare struct literal as a second, unvalidated door") and `manifest/designs/shed.md:168` ("would create a second, unvalidated way to build one").
  Both describe a validated field set a human configures — which the adapters, holding seams a caller has already built, are not.

### Type names

- Decision: `SingleLLMProducer`, `PerchProducer`, `WebsterProducer`.
- Rationale: `SingleLLMProducer` is pinned verbatim in `manifest/designs/shed.md` and in the task brief; the other two follow one obvious pattern and read cleanly package-qualified (`shedadapters.PerchProducer`).
- Rejected: `SingleLLM`/`Perch`/`Webster` — shorter, but drops the name the design doc already pins.
  Rejected: `PerchGateProducer` — encodes today's gate classification into the name, a rename cost if perch is ever used non-gate.

### Doc set — the exact edits, named line by line

- Decision: this commit carries exactly these doc edits.
  **`internal/shedadapters/doc.go`** — the as-built contract: the three adapters, the mapping tables, the told-name and clock seams, the perch run-id scheme, the success-verdict-survives-cancellation rule, and the three named limitations (no reattach; no mid-run bridge for shuttle or Webster, with their respective bounds; and `lyx perch pause` being a silent no-op against adapter-driven run dirs).
  **`manifest/designs/shed.md`** — five corrections: `:38`'s "three of the four planned adapters — `perch`, `Webster`, and a bespoke multi-spawn engine — own their own error taxonomies and are not designed yet", now false for two of the three; the `:3` status banner (the adapters are no longer Planned); `:255`'s claim that `SingleLLMProducer` performs the full three-case live-session discipline; `:261`'s identical reattach claim in the "What `Shed` does not provide" list; and `:278`'s description of `SingleLLMProducer` as "parameterized by an Input-format pointer, an Output-format pointer, and one instruction file", which the caller-supplied-`Spec`-source Decision supersedes — reworded to say the parameterization lives in the caller's `Spec` source.
  **`docs/overview.md`** — a tree line beside `internal/shedengine` (line 228), a module bullet beside the `shed` entry (line 292), and the correction of `:294`'s "the three engine adapters ... remain Planned".
  **`manifest/roadmap.md`** — three edits: Planned item 1 (lines 12-14) moves to Done; the existing Done entry for the Shed skeleton (lines 196-199), which currently asserts the three adapters "remain their own Planned item above" and justifies shed.md's survival by that Planned item; and `:16`'s "wired via the `perch` adapter above", whose "above" dangles once Planned item 1 leaves the Planned section — reworded to point at the shipped package instead.
  The Done entry's two claims — that the adapters remain Planned, and that shed.md survives *because* of that Planned item — both become false in this commit, so shed.md's Documentation-Lifecycle survival rationale is restated there on its own footing: the doc remains the authoritative narrative of Shed's generic mechanism, independent of any Planned item.
- Rationale: CLAUDE.md requires a task that adds a module or introduces cross-cutting infrastructure to update its docs in the same commit, and the roadmap moves on completing a planned item — which this is.
  Naming every line explicitly rather than saying "update shed.md" is what stops a partial edit from leaving a claim that is false the moment the package ships; four of the five shed.md corrections are exactly such claims, and the roadmap Done entry and `docs/overview.md:294` are two more.
- Rejected: correcting only `shed.md:255` — `:261` states the same reattach claim in different words, so fixing one and not the other leaves the doc self-contradicting.
  Rejected: leaving the roadmap Done entry alone — it forward-references a Planned item this commit deletes.
  Rejected: deferring the doc set to a follow-up — CLAUDE.md's same-commit rule exists precisely to prevent that.

## Technical context

**The seam being implemented** — `internal/shedengine/producer.go`:

```go
type Outcome string
const (Done Outcome = "done"; Stuck Outcome = "stuck")

type OutputPointer struct{ Path string } // "" = no artifact (gate or terminal producer)

type ShedProducer interface {
    Call(ctx context.Context) (Outcome, OutputPointer, error)
}
```

Two obligations Shed cannot enforce bind every implementation (`producer.go:26-32`, `shed.md:34-38`): return **exactly** `Done` or `Stuck` (any other value with a nil error is an engine-level failure, not a third verdict), and surface context cancellation as a **non-nil error**, never as `Stuck`.

**How Shed routes what an adapter returns** (`shed.md:82-86`) — this is what makes the mapping decisions above load-bearing:

- non-nil `error` with a healthy ctx → `state: "failed"`, halt; the next run re-calls the **same** producer.
- non-nil `error` with `ctx.Err() != nil` → `state: "paused"`, `RunPaused`, nil error; no history entry, `current_producer` unchanged.
- `Stuck` → route to this producer's `OnStuck` target if named and bounce budget remains, else `state: "blocked"`.
- `Done` → advance to the next entry.
- anything else with a nil error → `state: "failed"` naming the offending value.

**The three engines, as they actually ship:**

| Engine | Entry point | Terminal vocabulary |
| --- | --- | --- |
| shuttle | `(*shuttleengine.Runner).Run(Spec) (Result, error)` — `internal/shuttleengine/run.go:174` | `OutcomeDone`/`OutcomeAsking`/`OutcomeDied`/`OutcomeTimeout`, all with nil error (`engine.go:14-26`) |
| perch | `(*perchengine.Engine).Run(p Profile, runDir, scratchDir, stencilsDir string) (Result, error)` — `internal/perchengine/engine.go:82` | `OutcomeApproved`/`OutcomeStuck`/`OutcomePaused` + `StuckReason` (`result.go:16-38`) |
| webster | `websterengine.Run(deps RunDeps, opts RunOptions) (RunResult, error)` — `internal/websterengine/runlevel.go:308` | `RunResult.Outcome` ∈ `done`/`stuck`/`paused` + `StuckReason`/`BatchesDone`/`SummaryTitle`; typed errors `*MasterAskingError`/`*MasterDiedError`/`*MasterTimeoutError` (each `Unwrap`-able to `ErrMasterAsking`/`ErrMasterDied`/`ErrMasterTimeout`) and sentinels `ErrRunBusy`/`ErrFingerprintMismatch`/`ErrNilBatcher` |

None of the three takes a `context.Context`.

**Told, never derived.** Every adapter receives already-resolved absolute paths and already-constructed engines from its caller.
No adapter calls `lyxcwd`, constructs a `_lyx` path, or resolves geometry — the same told-not-derived discipline `shedengine`, `treadleengine`, and `perchengine` already hold.
Concretely: every constructor is told a **producer name** (log fields and error text only);
`PerchProducer` is additionally told an engine factory plus its `Profile`, its `runDirBase`/`scratchDirBase`/`stencilsDir` (all resolved today by `perchcli`), and a `runIDPrefix`;
`WebsterProducer` is told its fully populated `RunDeps`;
`SingleLLMProducer` is told a `Spec` source that yields absolute `OutputFiles` and a `now func() time.Time` clock (nil selects `time.Now`).
The perch factory does not weaken this: the adapter is told *how to build* its engine and still resolves no path, reads no config, and names no collaborator of its own.

**Precedents to follow, by name:**

- `internal/perchengine/adapter.go` — `burlerAdapter`, the shipped example of exactly this shape one level down: a small unexported struct closing over a seam, one method, mapping one vocabulary onto another.
- `internal/burlerengine/engine.go:20-25` and `internal/perchengine/engine.go:26-32` — the narrow-local-seam-plus-compile-time-proof idiom.
- `internal/websterengine/outcome.go:77` (`archiveStaleOutcome`) and `internal/websterengine/summary.go:77` (`ArchiveStaleSummary`) — the timestamp-rename archive discipline, including collision handling via `firstFreeArchivePath`.
- `internal/loomengine/discussion.go:19` (`DiscussionSpec`) — the shipped `Spec`-source shape `SingleLLMProducer` consumes.

**Gotchas discovered during exploration:**

- `Spec.validate` mutates its receiver: it rewrites `OutputFiles` in place with resolved absolute paths and defaults `Timeout`/`Display.Anchor` (`spec.go:115-162`).
  It runs **inside** `Runner.Start`, i.e. after the adapter's archive step — which is why absolute entries are a precondition the adapter enforces rather than something it resolves (see the stale-output Decision).
- `perchengine.Options.PauseRequested` is checked **only between rounds**, so the ctx bridge gives round-granularity responsiveness, not instant cancellation. That is the correct granularity — a round is one burler spawn.
- `perchengine.Engine`'s pause callback is fixed at construction (`New`, `engine.go:41-67`), which is what forces the factory seam rather than a bare runner seam. A plan that reintroduces a bare `*perchengine.Engine` seam silently drops the bridge.
- Webster's pause flag is a file under its scratch dir, written by `RequestPause` and cleared by `ClearPause`, and `Run` clears it on its own paths. This is the operator's channel, and the Webster Decision above deliberately leaves it untouched.
- `websterengine.Run` refuses with `ErrNilBatcher` when `RunDeps.Batcher` is nil (`runlevel.go:338-340`); the adapter must not paper over a caller's incomplete `RunDeps`.
- `shedadapters` may import `internal/logger` freely — the Shed Producer-Seam Invariant's import allowlist binds `internal/shedengine` only. The `StuckReason` Decision depends on this.

## Constraints

From `CONSTRAINTS.md`:

- **Shed Producer-Seam Invariant** — `internal/shedengine` imports only stdlib, `internal/state`, `internal/lock`.
  This task must not add an import to `shedengine`; `internal/shedengine/seam_enforcement_test.go` (`TestProducerSeamInvariant_AllowlistOnly`) fails the build otherwise.
- **Cwd Resolution Invariant** — no adapter calls `os.Getwd`, `git rev-parse --show-toplevel`, or resolves a per-module subdirectory. Paths are told.
- **Lyxdirs Single-Declarer Invariant** — no adapter may write the literals `_lyx` or `.lyx` in path-construction context.
  The only paths any adapter builds are a run-id leaf joined onto a told base (`Join(runDirBase, runID)` / `Join(scratchDirBase, runID)`) and an archive sibling beside a told output file — no token, no geometry.
- **Durable-vs-Ephemeral State Invariant** — two placements this task must honor.
  The archived stale-output files land beside the original output (durable `_lyx` content the product chose), not in a location the adapter invents.
  The perch scratch dir the adapter creates is `Join(scratchDirBase, runID)`, the mirrored `.lyx` sibling of `Join(runDirBase, runID)` — the exact pairing `perchcli` already uses, so the mirrored-subpath rule holds by construction rather than by the adapter reasoning about it.
- **CLI / Cobra Invariant** — `shedadapters` is a support package, not a cobra module: no `Command()`, no `RunCLI`, no cobra import, no registration in `newRoot()`.
- **Test Tier Purity Invariant** — untagged test files must not call `gitexec.Run`, `exec.Command`/`exec.CommandContext`, `gitkit.Copy*`, or `hubforge.NewHub`, and must not `time.Sleep` ≥ 1s with a constant duration. All tests here are untagged and fake-driven, so this holds by construction.
- **Live-Substrate Spawn Observability** — the adapters start no OS process themselves (the wrapped engines do, and already log), so this invariant does not engage. Do not add a spawn path.
- **Markdown Link Integrity** — the `manifest/designs/shed.md` and `docs/overview.md` edits must keep every inline link's file part and `#anchor` resolving (`internal/lyxcwd/docslink_test.go`).
- **Documentation Lifecycle** plus `CLAUDE.md`'s task-completion rule — the module doc, `docs/overview.md`, and the roadmap move all land in this same commit.

Discovered during discussion:

- **No new engine surface.** `shuttleengine`, `perchengine`, and `websterengine` are consumed exactly as they ship. Needing to widen one is a signal the mapping is wrong.
- **No new cross-cutting invariant is expected** from this task; if the plan finds one, it lands in `CONSTRAINTS.md` in the same commit.
- **Worktree discipline** (`CLAUDE.md`) — all work stays in this worktree on branch `shed-adapters`; never push to `main` from here.

## Testing

All tests are **tier 1, untagged**, driven by fakes for the three seams — no tmux, no git, no live provider.
The three adapters are pure mapping code over an injected seam, which is exactly the shape table-driven tests suit.

**`SingleLLMProducer` — the TDD candidate.** Its behavior is fully determined before a line of it exists: four outcome rows, the archive step, and the three ctx checks. Write the tests first.

- Outcome mapping table: `done`→(`Done`, `OutputFiles[0]` as the pointer, nil error); `asking`→(`Stuck`, **empty** pointer, nil error); `died`→non-nil error; `timeout`→non-nil error. Assert the error text names the outcome and the told producer name, since that text is what lands in Shed's persisted `error` field.
- A seam error from `Run` propagates as a non-nil error, distinct from the died/timeout mapping.
- An empty `OutputFiles` never panics: the fake seam returns `OutcomeDone` with an empty list and the adapter returns an error, not an index panic.
- Archive-then-respawn against a real `t.TempDir()`, with a **fake clock returning a fixed instant** so the filenames are deterministic: a pre-existing output file is renamed to the expected timestamped sibling and the original path is free when the fake seam is invoked; a second archive under the same fixed instant takes the numeric collision suffix; a missing output file is a no-op, not an error.
- A nil `now` selects `time.Now` — assert the constructor accepts nil and still archives (filename asserted by shape, not by literal).
- The `Spec` source returning an error surfaces without the seam ever being called.
- ctx: already-cancelled at entry → error, seam never invoked. Cancelled during the call (the fake seam cancels, then returns): `OutcomeDone` → `Done` with its pointer, **not** the ctx error, and the output files are left un-archived; `OutcomeAsking` → the ctx error, not `Stuck`.
- `OutputPointer` on `Done` is `OutputFiles[0]` — asserted against a multi-entry `Spec` so the first-entry convention is pinned, not incidental.
- A relative `OutputFiles` entry is rejected with an error that names the entry, and the seam is never invoked.
- No bridge is installed: the fake seam is handed nothing but the `Spec` — no callback, no cancellation channel.

**`PerchProducer`.**

- Mapping table: `APPROVED`→(`Done`, empty `OutputPointer`); `STUCK`→(`Stuck`, empty pointer); `PAUSED` with healthy ctx→non-nil error naming the out-of-band pause.
- The empty `OutputPointer` on the `Done` path is asserted explicitly — it is a decision, not an omission, and a future reader must see it pinned.
- `Stuck` returns an empty `OutputPointer` and a **nil** error for all three `StuckReason` values — the reason goes to the log, not the seam, so the assertion is on the seam's shape, not on log text.
- ctx: entry check (factory never invoked); the bridge is actually installed — the fake factory captures the `pauseRequested` callback it was handed and the test asserts it reports `false` before cancellation and `true` after. Under a cancelled ctx, a returned `APPROVED` still maps to `Done` (a finished gate is not discarded), while a returned `STUCK` or `PAUSED` becomes the ctx error.
- The factory is invoked once per `Call`, so a second `Call` with a fresh ctx gets a fresh bridge rather than a stale closure over the first ctx.
- **Run-id advancement, against a real `t.TempDir()`** — the rows that pin the second-`Call` fix.
  Seeding mechanism, stated explicitly because `treadleengine.runState` is unexported and written through `state.WriteJSON`: hand-write the JSON at `<runDir>/state.json` against its `json` tags — `{"outcome": "APPROVED"}` for a terminal block, `{"outcome": ""}` (or the field omitted, since it is `omitempty`) for an in-flight one.
  Rows (writing `H` for this `Profile`'s `hash8`): a terminal `<prefix>-H-1` makes the next `Call` run against `<prefix>-H-2`; a non-terminal `<prefix>-H-1` is reused; an empty base starts at `<prefix>-H-1`; `<prefix>-H-1` and `<prefix>-H-2` both present with `-2` terminal advances to `<prefix>-H-3` (highest-N, not first-gap).
- **Profile-hash namespacing** — the row that pins the second refusal branch away. With a non-terminal `<prefix>-H1-1` on disk, a `Call` carrying a *different* `Profile` resolves to `<prefix>-H2-1`, leaving `H1`'s directory untouched: no reuse, no deletion, no error. Assert `hash8` equals `perchengine.ProfileHash(p)[:8]` so the id is verifiably derived, not merely different.
- **Missing scratch sibling** — a `<prefix>-H-N` run dir whose `filepath.Join(scratchDirBase, runID)` does not exist resolves normally: the adapter creates it, the probe reports non-terminal, and the `Call` proceeds. This is the default shape of a `t.TempDir()` fixture and the real shape after a fresh clone, so it is asserted rather than assumed.
- A corrupt `state.json` (unparseable bytes, scratch dir present) fails the `Call` with a propagated error — the other half of the error-disposition rule.
- The resolved `runDir`/`scratchDir` pair handed to the factory's engine is `Join(base, runID)` for each base respectively — asserted, since a mismatched pair would put treadle's state lock in the wrong tree.
- An invalid `runIDPrefix` is rejected at construction via `perchengine.ValidRunID`, before any directory is touched.
- A seam error propagates unchanged.

**`WebsterProducer`.**

- Mapping table over `RunResult.Outcome`: `done`→(`Done`, `SummaryPath(WebsterDir)` as the pointer); `stuck`→(`Stuck`, empty pointer); `paused` with healthy ctx→error.
- Error mapping table: `*MasterAskingError`→(`Stuck`, empty pointer, nil error), matched via `errors.Is(err, ErrMasterAsking)` rather than string comparison; `*MasterDiedError`, `*MasterTimeoutError`, `ErrRunBusy`, `ErrFingerprintMismatch`, `ErrNilBatcher`→non-nil error.
- The default-with-one-exception rule holds for an error matching no sentinel at all: a plain `errors.New` from the fake seam maps to a non-nil error, never to `Stuck`.
- `RunOptions.Fresh` is `false` on every call the adapter makes — assert it from the fake, since this is a safety property, not a default.
- ctx: entry check (the seam is never invoked); under a cancelled ctx a returned `done` still maps to `Done` with its `SummaryPath` pointer, while `stuck`/`paused`/any error becomes the ctx error.
- No bridge is installed: assert the adapter never writes Webster's pause flag — a `t.TempDir()` scratch dir stays free of it across a cancelled call, so the operator's channel is provably untouched.

**Compile-time coverage.** A `var _ shedengine.ShedProducer = (*SingleLLMProducer)(nil)` line per adapter, plus the `var _ Seam = (*concrete)(nil)` proofs, so a drift in either direction is a build failure rather than a test failure.

**Not tested here:** Shed's own loop behavior. `internal/shedengine`'s `run_routing_test.go`, `run_pause_test.go`, and `run_persist_test.go` already prove routing, pause, and persistence against stub producers; re-driving a real `Shed` over a fake-engine producer list would re-test Shed, not the adapters.

## Q&A log

- **Q:** Where do the three adapters live — one `internal/shedadapters` package, beside each engine, or three separate packages? **A:** One `internal/shedadapters` package. `perchcli`/`webstercli` are both shipped standalone CLIs, so making their engines depend on `shedengine` is a real cost; `perchengine/adapter.go` confirms the adapter belongs in the seam-caller.
- **Q:** How does `SingleLLMProducer` get its prompt — a caller-supplied `Spec` source, adapter-side templating, or a verbatim-stencil hybrid? **A:** Caller-supplied `Spec` source. `loomengine.DiscussionSpec` already returns exactly that shape, so this reuses a shipped pattern rather than introducing one.
- **Q:** How is `Spec.validate`'s rejection of pre-existing output files handled across Shed's unconditional re-calls? **A:** Archive-then-respawn, reusing webster's timestamp-rename discipline. Returning `Done` on existing outputs is rejected for the same reason `shed.md:253-257` rejects it at Shed level.
- **Q:** Is live-session reattach in scope? **A:** No — respawn only. No public shuttle reattach API exists, and `shed.md:255` overclaims what the adapter will do, so that line is corrected in the same commit.
- **Q:** How is context cancellation handled given no engine takes a ctx? **A:** Entry check for all three, plus an exit rule where the ctx error overrides everything except a genuine success verdict (refined in r6 — see that entry below); a mid-run bridge for perch only (its pause seam is a callback), with Webster and shuttle bridgeless and bounded by `MasterTimeoutMin` and `Spec.Timeout` respectively. The goroutine+`select` alternative is rejected for abandoning a live pane mid-write.
- **Q:** How do shuttle's four outcomes map? **A:** `done`→`Done`, `asking`→`Stuck`, `died`/`timeout`→error. Asking is a verdict worth bouncing upstream; died/timeout are infrastructure, where `failed` + same-producer re-call is the right recovery.
- **Q:** What `OutputPointer` does the perch adapter report? **A:** Empty — a review gate is shed.md's canonical gate producer, and the empty pointer's re-run-on-resume behavior is correct for a gate.
- **Q:** Is `RunOptions.Fresh` configurable on the Webster adapter? **A:** No, fixed `false`. It is the destructive fingerprint-mismatch escape and must stay an explicit human act.
- **Q:** Where does a pause request reach the adapters from? **A:** `ctx` only. One channel, one meaning — a `PAUSED` return can then only mean cancellation, which Shed routes to `paused` rather than `failed`.
- **Q:** What happens on `PAUSED` with a healthy ctx (reachable via Webster's own flag file)? **A:** Return an error naming it as an out-of-band pause. `Stuck` would spend bounce budget on an operator action; `Done` would silently advance past an unfinished producer.
- **Q:** Concrete engine types or narrow local seam interfaces? **A:** Narrow local interfaces with compile-time proofs, per `burlerengine.Shuttle`/`perchengine.Burler`. Only Webster's free-func `Run` needs no interface.
- **Q:** `New(...)` constructors or exported-field structs like `shedengine.Shed`? **A:** `New(...)` with unexported fields. `Shed`'s no-constructor rule is about a human-configured validated field set; these wrap already-built live engines.
- **Q:** Test strategy? **A:** Tier 1, fakes for all three seams, table-driven mapping tests. An integration test over a real `Shed` would re-test Shed's already-proven loop.
- **Q:** Which docs land in this commit? **A:** See the "Doc set" Decision — package `doc.go`, five named `manifest/designs/shed.md` corrections (`:3`, `:38`, `:255`, `:261`, `:278`), three `docs/overview.md` edits (tree line, module bullet, `:294`), and three `manifest/roadmap.md` edits (Planned item 1 → Done, the Done entry at `:196-199`, and `:16`'s dangling "above").
- **Q:** (review r6 gap) `loadOrInitState` has a *second* refusal branch — a non-terminal block whose `ProfileHash` differs — so reusing a non-terminal run dir wedges the producer permanently the first time an operator edits `perch.yaml` mid-loop. Disposition? **A:** [auto-pick] put the profile hash in the run-id: `<prefix>-<ProfileHash(p)[:8]>-<N>`, scanning within that hash's namespace. **Why:** it dissolves the branch instead of handling it, and it is the shipped convention — `perchengine.DeriveRunID` is literally `<basename>-<hash[:8]>`.
- **Q:** (review r6 gap) With no mid-run bridge, a cancel lets shuttle/Webster finish; the exit check then converts a genuine `Done` into the ctx error, so Shed records nothing and the next `Call` archives the valid output and respawns. Accept the discard, or let `Done` win? **A:** [auto-pick] a genuine success verdict (`OutcomeDone`/`APPROVED`/`done`) survives cancellation and is returned as `Done`; every other result under a cancelled ctx becomes the ctx error. **Why:** Shed records the entry, advances, and pauses at its own next loop top — the run still stops immediately, but a finished artifact and a paid-for LLM session are not thrown away. The seam obligation forbids reporting cancellation as `Stuck`, which this never does.
- **Q:** (review r6 nit) Webster's three outcome values are unexported, so mapping means hardcoding literals. **A:** [auto-pick] name the duplication and add a `default:` branch that errors on an unrecognised value, with a test row. **Why:** `parseOutcome` already rejects a fourth value, so only a rename is the live risk — which the `default:` catches as a failing test rather than a mis-mapped verdict.
- **Q:** (review r6 nit) `lyx perch pause` writes a flag the adapter's engine never reads. **A:** [auto-pick] state it as an accepted consequence in the decision and the package doc. **Why:** it is the direct cost of the ctx-only pause channel, and an operator must not be left believing a pause was accepted.
- **Q:** (review r5 gap) `TerminalOutcome` takes a read lock inside `scratchDir`, and `lock.AcquireReadLock` never creates its parent — so the probe errors when the never-tracked scratch sibling is absent, which is normal after a clone. How is that handled? **A:** [auto-pick] the adapter `os.MkdirAll`s `Join(scratchDirBase, runID)` before probing, exactly as `perchcli/pause.go` already does, and the propagate-the-error rule is scoped to a genuinely corrupt `state.json`. **Why:** `runDirBase` is tracked `_lyx` and `scratchDirBase` is never-tracked `.lyx`, so run-dir-without-scratch is an ordinary state that must not become a permanent producer failure.
- **Q:** (review r4 gap) What `OutputPointer` does `WebsterProducer` report on `Done`? **A:** [auto-pick] `websterengine.SummaryPath(deps.WebsterDir)`; empty on every non-`Done` path. **Why:** `summary.md` is the human-readable artifact and is guaranteed present on `done`, while `outcome.yaml` is an internal machine handoff and summary parsing is best-effort on the other outcomes.
- **Q:** (review r3 gap) `treadleengine` refuses a terminal run dir, so a `PerchProducer` told one fixed `runDir` breaks on its second `Call` — the bounce-back this adapter exists to serve. How is perch's run identity resolved? **A:** [auto-pick] told a `runDirBase`/`scratchDirBase`/`runIDPrefix`; per `Call`, reuse the current `<prefix>-<N>` while `perchengine.TerminalOutcome` says non-terminal, advance to `<prefix>-<N+1>` once it is terminal. **Why:** it is treadle's own documented remedy (a fresh run-id), and advancing *only* past a terminal block preserves perch's in-flight crash-resume, which minting fresh every call would destroy. **Superseded in r6:** the id gained a profile-hash segment, so the scheme is `<prefix>-<hash8>-<N>` and the scan is scoped to one hash namespace.
- **Q:** Type names? **A:** `SingleLLMProducer`, `PerchProducer`, `WebsterProducer` — the first is pinned verbatim in shed.md, the other two follow it without encoding today's classification into the name.
- **Q:** (review r1 gap) Where does `StuckReason` go, given `Call` returns only `(Outcome, OutputPointer, error)` and the `Stuck` branch requires a nil error? **A:** [auto-pick] `logger.Warn`, with the empty `OutputPointer` preserved. **Why:** `OutputPointer.Path` is an artifact path Shed persists verbatim and a human opens; overloading it with prose breaks its documented meaning, and a non-nil error would make Shed discard the verdict entirely.
- **Q:** (review r1 gap) `perchengine.Options.PauseRequested` is fixed at construction and cannot be installed through a `Run(...)` seam over a built engine — how is the perch bridge installed? **A:** [auto-pick] the seam becomes an engine factory, `func(pauseRequested func() bool) PerchRunner`, invoked once per `Call`. **Why:** it is the only shape that makes the bridge both installable and fakeable without the adapter learning about burler, shuttle, or config.
- **Q:** (review r1 gap) Does the Webster adapter install a ctx bridge, and if so how? **A:** [auto-pick] no bridge; entry/exit checks only, bounded by `MasterTimeoutMin`. **Why:** the only mechanism available is writing the operator's own pause flag from a goroutine, which conflates the two pause channels, races `Run`'s own `ClearPause`, and can leave Webster paused for the next invocation.
- **Q:** (review r2 gap) The log call and the error text both name "the producer", but `Call(ctx)` carries no identity — where does it come from? **A:** [auto-pick] each `New...` takes a `name string`, the same value the caller registers in `ProducerDef.Name`, used only for log fields and error text. **Why:** the expected two-instances-of-one-type shape makes an unattributed `Stuck` log useless, and widening `ShedProducer.Call` would modify a shipped seam this task must not touch.
- **Q:** (review r2 gap) Where does the archive timestamp come from, and how is the collision-suffix path tested deterministically? **A:** [auto-pick] an injected `now func() time.Time` seam (nil selects `time.Now`), with a fixed-instant fake in the collision test. **Why:** both cited webster precedents take exactly this seam, and without it the collision test can only hope two calls land in the same wall-clock second.
- **Q:** (review r1 gap) `shuttleengine` has no pause seam — what is the accepted mid-run cancellation consequence for `SingleLLMProducer`? **A:** [auto-pick] entry/exit only, bounded by `Spec.Timeout` (defaulting to `cfg.RunTimeoutMin`). **Why:** the bound is a real, product-set deadline rather than an open-ended wait, and the `Start`+`Interrupt` alternative ends a turn rather than a run while adding a live-pane precondition.
