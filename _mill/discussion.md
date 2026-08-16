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
- `PerchProducer` — wraps `perchengine.Engine.Run`, maps `APPROVED`/`STUCK`/`PAUSED` onto Shed's vocabulary, reports an empty `OutputPointer` (gate producer).
- `WebsterProducer` — wraps `websterengine.Run`, maps its `RunResult.Outcome` and its five sentinel/typed errors onto Shed's vocabulary.
- Three narrow local seam interfaces (one per engine) with compile-time-proof lines, so every adapter is fakeable at tier 1.
- Context-cancellation handling for all three: entry check, a bridge into each engine's existing pause seam, and exit precedence of `ctx.Err()` over any verdict.
- Tier-1 (untagged) tests with fakes for all three seams.
- Docs in the same commit: a `doc.go` for the new package, corrections and a status update to `manifest/designs/shed.md`, a tree line and module bullet in `docs/overview.md`, and the roadmap move of Planned item 1 to Done.

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

- Decision: before starting a shuttle run, `SingleLLMProducer` renames every existing `Spec.OutputFiles` entry to a timestamped sibling (`<name>-<UTC-timestamp><ext>`, with a numeric suffix on collision), then starts the run.
- Rationale: `shuttleengine.Spec.validate` hard-rejects a pre-existing `OutputFiles` entry (`internal/shuttleengine/spec.go:136-143`) — the file contract's "done is bare file existence" rule means a stale file would classify a run done on its first turn end.
  Shed re-calls a producer **unconditionally** on every resume and every `OnStuck` bounce-back (`manifest/designs/shed.md:253-257`), so without this the adapter breaks permanently the second time it runs.
  Archiving rather than deleting reuses an existing, deliberately reusable in-repo pattern: `websterengine`'s `archiveStaleOutcome` (`outcome.go:77`) and `ArchiveStaleSummary` (`summary.go:77`), whose own doc comment names it "the same archive-never-refuse timestamp-rename discipline".
  The prior attempt stays inspectable for a human diagnosing a bounce loop.
- Rejected: deleting instead of archiving — loses the prior attempt's artifact for no gain.
  Rejected: returning `Done` when every output already exists — `shed.md:253-257` rejects exactly this shortcut at Shed level, and its reason applies identically here: after a bounce-back the previous attempt's output is still on disk, and existence alone cannot distinguish stale from fresh.

### Reattach out of scope — `SingleLLMProducer` is respawn-only

- Decision: `SingleLLMProducer` does not reattach to a live shuttle session.
  It archives stale outputs and respawns.
  The limitation is named explicitly in the package doc, and `manifest/designs/shed.md:255` is corrected in the same commit.
- Rationale: `shuttleengine` exposes no reattach entry point — `FindRun` (`internal/shuttleengine/rundir.go:150`) returns a `RunState` value and a directory, not a waitable `*Run`.
  Building one is new `shuttleengine` surface, which is scope growth into an already-shipped module and is not thin-wrapper work.
  `shed.md:255` currently claims "`SingleLLMProducer` wraps `shuttle`+`reed` and does this internally", referring to the full "live session / fresh output / respawn" three-case discipline.
  That claim would become false the moment this adapter ships, so correcting it is part of the task, not optional cleanup.
- Rejected: adding a `shuttleengine.Attach(guid) (*Run, error)` and wiring it — real scope growth, and no current caller needs the third case.

### Context cancellation — entry check, pause-seam bridge, exit precedence

- Decision: every adapter follows the same three-part discipline.
  (1) **Entry:** if `ctx.Err() != nil`, return it immediately without starting anything.
  (2) **Bridge:** wire `func() bool { return ctx.Err() != nil }` into the engine's existing pause seam where one exists — `perchengine.Options.PauseRequested` (`internal/perchengine/engine.go:43`, checked between rounds) and Webster's scratch-dir pause flag (`internal/websterengine/pause.go`: `RequestPause`/`PauseRequested`/`ClearPause`) — so a cancel mid-run drains to an orderly `PAUSED` rather than being invisible.
  (3) **Exit:** on return, `ctx.Err() != nil` wins over any verdict the engine produced and is returned as the non-nil error.
- Rationale: none of the three engines take a `context.Context` — `shuttleengine.Runner.Run(Spec)`, `perchengine.Engine.Run(p, runDir, scratchDir, stencilsDir)`, and `websterengine.Run(deps, opts)` are all synchronous and ctx-free — yet `ShedProducer`'s second unenforceable obligation is that cancellation surfaces as a non-nil error, never as `Stuck` (`internal/shedengine/producer.go:26-32`).
  Shed's routing predicate is the context's own state (`ctx.Err() != nil`), which routes to the clean `state: "paused"` exit rather than `failed`.
  The bridge is reuse of shipped seams, not new surface.
- Rejected: entry and exit checks only, with no bridge — a cancel would be invisible until the producer finished on its own, potentially hours.
  Rejected: running the engine in a goroutine and `select`ing on `ctx.Done()` — abandons a live engine mid-write, leaking tmux panes and racing `state.json`; the one option of the three that risks the real hazard.

### `SingleLLMProducer` outcome mapping

- Decision: `OutcomeDone` → `Done`; `OutcomeAsking` → `Stuck`; `OutcomeDied` and `OutcomeTimeout` → non-nil error.
- Rationale: `shuttleengine` classifies four outcomes, all returned with a nil error (`internal/shuttleengine/engine.go:14-26`), and `burlerengine.Run` already treats the three non-done ones as "normal loop events, not errors" (`internal/burlerengine/engine.go:91-95`).
  `asking` is a genuine producer verdict — the agent could not finish from its input — so bouncing to the upstream producer that wrote that input is exactly what `OnStuck` is for.
  `died`/`timeout` are engine-level infrastructure failures: `OnStuck` would bounce to an *upstream* producer, which is nonsense, whereas an error makes Shed write `state: "failed"` and the next run re-calls **this same** producer — the correct recovery.
- Rejected: mapping all three non-done outcomes to `Stuck` — spends bounce budget on a dead pane and routes an infrastructure failure to an unrelated producer.
  Rejected: making the died/timeout mapping depend on whether `OnStuck` is set — the adapter would have to know its own `ProducerDef`, which it does not and should not.

### Perch adapter — outcome mapping and empty `OutputPointer`

- Decision: `OutcomeApproved` → `Done` with an **empty** `OutputPointer`; `OutcomeStuck` → `Stuck` (the `StuckReason` surfaced in the returned detail, not as a third verdict); `OutcomePaused` → error (see the pause Decision below).
- Rationale: `shed.md:29` names a review gate as the canonical **gate producer** — pass/fail only, no output pointer — whose empty pointer makes it re-run on resume as "a cheap idempotent re-check", which is exactly right for a gate.
  The per-round review files stay discoverable through perch's own `state.json` round history (`internal/perchengine/result.go:40-59`), so nothing is lost by not naming one in Shed's `history[].output`.
- Rejected: reporting the last round's `ReviewPath` as the pointer — more observable, but declares the gate an artifact producer, contradicting shed.md's own classification and making the pointer's meaning inconsistent with `SingleLLMProducer`'s.
  Rejected: making the pointer configurable per instantiation — a knob with no caller.

### Webster adapter — `Fresh` fixed false, error mapping mirrors the shuttle rule

- Decision: `RunOptions.Fresh` is fixed `false` and is not configurable on the adapter.
  `RunResult.Outcome` `done` → `Done`; `stuck` → `Stuck` (with `StuckReason` in the detail); `paused` → error (see the pause Decision below).
  `*MasterAskingError` → `Stuck`; `*MasterDiedError`, `*MasterTimeoutError`, `ErrRunBusy`, and `ErrFingerprintMismatch` → non-nil error.
- Rationale: same rule as the `SingleLLMProducer` mapping, applied to Webster's error-typed equivalents (`internal/websterengine/runlevel.go:179-235`) — asking is a verdict, died/timeout are infrastructure.
  `Fresh: true` is the destructive fingerprint-mismatch escape: it archives `state.json` and the reports dir and clears the prompts dir (`runlevel.go:140-146`, `clearRenderedPrompts` at `runlevel.go:243`).
  That must stay an explicit human act via `lyx webster run --fresh`, never something Shed triggers automatically on a resume.
  `ErrFingerprintMismatch` means the plan changed under a running Webster — a bounce-back cannot fix it, a human must.
- Rejected: exposing `Fresh` on the adapter — a field whose only safe value is `false`.
  Rejected: mapping `ErrFingerprintMismatch` to `Stuck` — routes a plan/state divergence to an unrelated producer instead of a human.

### Pause reaches the adapters through `ctx` only

- Decision: the adapters' pause bridge is fed **solely** by `ctx.Err() != nil`.
  No adapter takes a caller-supplied `PauseRequested func() bool`, and no adapter reads Shed's status file.
- Rationale: one channel, one meaning — a `PAUSED` return from perch or Webster can then only mean cancellation, and Shed's own `ctx.Err()` predicate routes it to the clean `paused` exit rather than `failed`.
  A product wanting a mid-producer pause cancels the context it handed to `Shed.Run`.
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
  Two are needed (`shuttleengine.Runner`'s `Run(Spec) (Result, error)` and `perchengine.Engine`'s `Run(Profile, string, string, string) (Result, error)`); Webster's `Run` is already a free function, so a func-typed seam suffices there.
- Rationale: follows the two shipped precedents exactly — `burlerengine.Shuttle` with `var _ Shuttle = (*shuttleengine.Runner)(nil)` (`internal/burlerengine/engine.go:20-25`) and `perchengine.Burler` with `var _ Burler = (*burlerengine.Engine)(nil)` (`internal/perchengine/engine.go:26-32`).
  This is what makes every adapter testable at tier 1 with a fake — no tmux, no real engine, no git.
- Rejected: depending on the concrete types — only the Webster one would be fakeable, leaving the other two adapters untestable without live substrate.

### Construction — `New(...)` constructors with unexported fields

- Decision: each adapter is built by a `New...(...)` constructor and holds unexported fields.
- Rationale: the adapters are engine-shaped, not config-shaped — they hold live seams a caller has already constructed, and there is no field a human hand-edits.
  Matches both `perchengine.New` and `burlerengine.New`.
- Rejected: exported-field structs mirroring `shedengine.Shed` — visually symmetric with the thing they plug into, but `Shed`'s explicit reason for that shape (a validated field set a human configures, where a constructor "would create a second, unvalidated way to build one", `internal/shedengine/shed.go:6-9`) does not apply to a wrapper over live seams.

### Type names

- Decision: `SingleLLMProducer`, `PerchProducer`, `WebsterProducer`.
- Rationale: `SingleLLMProducer` is pinned verbatim in `manifest/designs/shed.md` and in the task brief; the other two follow one obvious pattern and read cleanly package-qualified (`shedadapters.PerchProducer`).
- Rejected: `SingleLLM`/`Perch`/`Webster` — shorter, but drops the name the design doc already pins.
  Rejected: `PerchGateProducer` — encodes today's gate classification into the name, a rename cost if perch is ever used non-gate.

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
Concretely: `PerchProducer` is told its `Profile` and its `runDir`/`scratchDir`/`stencilsDir` (resolved today by `perchcli`); `WebsterProducer` is told its fully populated `RunDeps`; `SingleLLMProducer` is told a `Spec` source that already yields absolute `OutputFiles`.

**Precedents to follow, by name:**

- `internal/perchengine/adapter.go` — `burlerAdapter`, the shipped example of exactly this shape one level down: a small unexported struct closing over a seam, one method, mapping one vocabulary onto another.
- `internal/burlerengine/engine.go:20-25` and `internal/perchengine/engine.go:26-32` — the narrow-local-seam-plus-compile-time-proof idiom.
- `internal/websterengine/outcome.go:77` (`archiveStaleOutcome`) and `internal/websterengine/summary.go:77` (`ArchiveStaleSummary`) — the timestamp-rename archive discipline, including collision handling via `firstFreeArchivePath`.
- `internal/loomengine/discussion.go:19` (`DiscussionSpec`) — the shipped `Spec`-source shape `SingleLLMProducer` consumes.

**Gotchas discovered during exploration:**

- `Spec.validate` mutates its receiver: it rewrites `OutputFiles` in place with resolved absolute paths and defaults `Timeout`/`Display.Anchor` (`spec.go:115-162`).
  It runs **inside** `Runner.Start`, after the adapter's archive step, so the adapter must resolve `OutputFiles` itself (relative to the worktree root) before archiving, or require the `Spec` source to yield absolute entries.
  Requiring absolute entries is simpler and matches what `loomengine.DiscussionSpec` already produces.
- `perchengine.Options.PauseRequested` is documented as checked **only between rounds**, so the ctx bridge gives round-granularity responsiveness there, not instant cancellation. That is the correct granularity — a round is one burler spawn.
- Webster's pause flag is a file under its scratch dir, written by `RequestPause` and cleared by `ClearPause`.
  A ctx bridge that writes this flag must also clear it on the way out, or a cancelled run would leave Webster permanently paused for the next invocation.
  An alternative worth weighing at plan time: pass no bridge for Webster and rely on entry/exit checks alone, since Master is a long single spawn whose pause is already operator-driven.
- `websterengine.Run` refuses with `ErrNilBatcher` when `RunDeps.Batcher` is nil (`runlevel.go:338-340`); the adapter must not paper over a caller's incomplete `RunDeps`.

## Constraints

From `CONSTRAINTS.md`:

- **Shed Producer-Seam Invariant** — `internal/shedengine` imports only stdlib, `internal/state`, `internal/lock`.
  This task must not add an import to `shedengine`; `internal/shedengine/seam_enforcement_test.go` (`TestProducerSeamInvariant_AllowlistOnly`) fails the build otherwise.
- **Cwd Resolution Invariant** — no adapter calls `os.Getwd`, `git rev-parse --show-toplevel`, or resolves a per-module subdirectory. Paths are told.
- **Lyxdirs Single-Declarer Invariant** — no adapter may write the literals `_lyx` or `.lyx` in path-construction context. The adapters should not construct such paths at all.
- **Durable-vs-Ephemeral State Invariant** — the archived stale-output files land beside the original output (which is durable `_lyx` content the product chose), not in a new location the adapter invents.
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

- Outcome mapping table: `done`→(`Done`, pointer set, nil error); `asking`→(`Stuck`, nil error); `died`→non-nil error; `timeout`→non-nil error. Assert the error text names the outcome and the producer, since that text is what lands in Shed's persisted `error` field.
- A seam error from `Run` propagates as a non-nil error, distinct from the died/timeout mapping.
- Archive-then-respawn against a real `t.TempDir()`: a pre-existing output file is renamed to a timestamped sibling and the original path is free when the fake seam is invoked; a second archive in the same timestamp second still succeeds (collision suffix); a missing output file is a no-op, not an error.
- The `Spec` source returning an error surfaces without the seam ever being called.
- ctx: already-cancelled at entry → error, seam never invoked; cancelled during the call (fake seam cancels, then returns `OutcomeDone`) → the ctx error wins over the `Done` verdict.
- `OutputPointer`: which of a multi-entry `OutputFiles` becomes the pointer must be pinned by a test — the decision (first entry, or a separately named primary) belongs to mill-plan, but it must not be left implicit.

**`PerchProducer`.**

- Mapping table: `APPROVED`→(`Done`, empty `OutputPointer`); `STUCK`→(`Stuck`, empty pointer); `PAUSED` with healthy ctx→non-nil error naming the out-of-band pause.
- The empty `OutputPointer` on the `Done` path is asserted explicitly — it is a decision, not an omission, and a future reader must see it pinned.
- `StuckReason` reaches the caller-visible surface (whatever detail channel the adapter uses) for all three `StuckReason` values.
- ctx: entry check; the pause bridge is actually installed into `Options.PauseRequested` (assert the fake engine observes a bridge that reports `true` once the ctx is cancelled); exit precedence over a returned verdict.
- A seam error propagates unchanged.

**`WebsterProducer`.**

- Mapping table over `RunResult.Outcome`: `done`→`Done`; `stuck`→`Stuck`; `paused` with healthy ctx→error.
- Error mapping table: `*MasterAskingError`→`Stuck`; `*MasterDiedError`, `*MasterTimeoutError`, `ErrRunBusy`, `ErrFingerprintMismatch`, `ErrNilBatcher`→non-nil error. Match via `errors.Is` against the exported sentinels, not string comparison.
- `RunOptions.Fresh` is `false` on every call the adapter makes — assert it from the fake, since this is a safety property, not a default.
- ctx: entry check and exit precedence; whichever pause-bridge decision the plan lands on (flag file with cleanup, or entry/exit only), its cleanup behavior is asserted — a cancelled run must not leave Webster's pause flag set.

**Compile-time coverage.** A `var _ shedengine.ShedProducer = (*SingleLLMProducer)(nil)` line per adapter, plus the `var _ Seam = (*concrete)(nil)` proofs, so a drift in either direction is a build failure rather than a test failure.

**Not tested here:** Shed's own loop behavior. `internal/shedengine`'s `run_routing_test.go`, `run_pause_test.go`, and `run_persist_test.go` already prove routing, pause, and persistence against stub producers; re-driving a real `Shed` over a fake-engine producer list would re-test Shed, not the adapters.

## Q&A log

- **Q:** Where do the three adapters live — one `internal/shedadapters` package, beside each engine, or three separate packages? **A:** One `internal/shedadapters` package. `perchcli`/`webstercli` are both shipped standalone CLIs, so making their engines depend on `shedengine` is a real cost; `perchengine/adapter.go` confirms the adapter belongs in the seam-caller.
- **Q:** How does `SingleLLMProducer` get its prompt — a caller-supplied `Spec` source, adapter-side templating, or a verbatim-stencil hybrid? **A:** Caller-supplied `Spec` source. `loomengine.DiscussionSpec` already returns exactly that shape, so this reuses a shipped pattern rather than introducing one.
- **Q:** How is `Spec.validate`'s rejection of pre-existing output files handled across Shed's unconditional re-calls? **A:** Archive-then-respawn, reusing webster's timestamp-rename discipline. Returning `Done` on existing outputs is rejected for the same reason `shed.md:253-257` rejects it at Shed level.
- **Q:** Is live-session reattach in scope? **A:** No — respawn only. No public shuttle reattach API exists, and `shed.md:255` overclaims what the adapter will do, so that line is corrected in the same commit.
- **Q:** How is context cancellation handled given no engine takes a ctx? **A:** Entry check, bridge `ctx.Err()` into each engine's existing pause seam, exit precedence of the ctx error over any verdict. The goroutine+`select` alternative is rejected for abandoning a live pane mid-write.
- **Q:** How do shuttle's four outcomes map? **A:** `done`→`Done`, `asking`→`Stuck`, `died`/`timeout`→error. Asking is a verdict worth bouncing upstream; died/timeout are infrastructure, where `failed` + same-producer re-call is the right recovery.
- **Q:** What `OutputPointer` does the perch adapter report? **A:** Empty — a review gate is shed.md's canonical gate producer, and the empty pointer's re-run-on-resume behavior is correct for a gate.
- **Q:** Is `RunOptions.Fresh` configurable on the Webster adapter? **A:** No, fixed `false`. It is the destructive fingerprint-mismatch escape and must stay an explicit human act.
- **Q:** Where does a pause request reach the adapters from? **A:** `ctx` only. One channel, one meaning — a `PAUSED` return can then only mean cancellation, which Shed routes to `paused` rather than `failed`.
- **Q:** What happens on `PAUSED` with a healthy ctx (reachable via Webster's own flag file)? **A:** Return an error naming it as an out-of-band pause. `Stuck` would spend bounce budget on an operator action; `Done` would silently advance past an unfinished producer.
- **Q:** Concrete engine types or narrow local seam interfaces? **A:** Narrow local interfaces with compile-time proofs, per `burlerengine.Shuttle`/`perchengine.Burler`. Only Webster's free-func `Run` needs no interface.
- **Q:** `New(...)` constructors or exported-field structs like `shedengine.Shed`? **A:** `New(...)` with unexported fields. `Shed`'s no-constructor rule is about a human-configured validated field set; these wrap already-built live engines.
- **Q:** Test strategy? **A:** Tier 1, fakes for all three seams, table-driven mapping tests. An integration test over a real `Shed` would re-test Shed's already-proven loop.
- **Q:** Which docs land in this commit? **A:** Package `doc.go`, `manifest/designs/shed.md` corrections (the `:255` reattach overclaim and the `:3` status banner), a `docs/overview.md` tree line and module bullet, and the `manifest/roadmap.md` move of Planned item 1 to Done.
- **Q:** Type names? **A:** `SingleLLMProducer`, `PerchProducer`, `WebsterProducer` — the first is pinned verbatim in shed.md, the other two follow it without encoding today's classification into the name.
