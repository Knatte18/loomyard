# Batch: shedverbs-package

```yaml
task: "Shed-generic watchdog for ly-drive and loom's CLI verbs"
batch: "shedverbs-package"
number: 3
cards: 11
verify: go test ./internal/shedverbs/...
depends-on: []
```

## Batch Scope

This batch creates `internal/shedverbs`: the generic `run`, `step`, `status` (including `--watch`) and `pause` verb bodies, and nothing else.
It is the only genuinely new test surface in the task, so it is written test-first against a fake `*shedengine.Shed` built from `Stub` rows, with no LLM, no tmux and no git involved.

The package is a leaf by construction: it imports `cobra`, `clihelp`, `output`, `state` and `shedengine`, and it imports no `<module>cli`.
It derives no path, imports no resolver, and never calls `os.Getwd` or `lyxcwd` — every path reaches it told, through `Spec`.
That is what keeps `internal/shedcli`'s own imports acyclic in batch 5, and card 19 enforces it with a seam scan in the style of `internal/lifecycleshed`'s and `internal/loomrecipe`'s existing ones.

The external interface batches 4 and 5 consume is `shedverbs.Verbs(texts VerbTexts, spec *Spec) []*cobra.Command`, plus the `Spec`, `Hooks`, `VerbTexts` and `AbsentDisposition` types.
This batch creates no file any other batch edits and therefore carries no `depends-on` edge — it needs nothing from batch 1, because `Spec` carries a `BuildShed func() (*shedengine.Shed, error)` constructor rather than a `shedbuild.ShedPaths`.

Batch-local decision: the four verb bodies live in four files named after their verbs, mirroring `internal/loomcli`'s own `run.go`/`step.go`/`status.go`/`pause.go` layout, so a reader moving between the two sees the same shape.

Batch-local decision, and non-negotiable in every verb body below: **every** envelope write is recorded through `clihelp.SetExit(cmd.Context(), …)` wrapping the `output` helper's return, exactly as all six deleted bodies do (`clihelp.SetExit(ctx, output.Err(out, …))`, `clihelp.SetExit(ctx, output.ErrFields(out, …, fields))`, `clihelp.SetExit(ctx, output.Ok(out, envelope))`).
This is what makes the process exit code correct: `internal/clihelp/exec.go`'s `RunRootCtx` returns `es.code`, and `es.code` is written only by `SetExit` and `Abort` — `output.Err`/`ErrFields`/`Ok` merely *return* 1 or 0.
A body that writes the envelope without `SetExit` exits 0 on every refusal, silently breaking every caller that branches on the exit code, `ly-drive` included.

Batch-local decision: the helpers the two arming modules' own test tables reach are **exported** — `StepEnvelope`, `RenderStatusLine`, `UnavailableLine` and `PrintStatusLinesOnChange` — while everything else in the package stays unexported.
Those four are exported for a reason rather than for symmetry: `internal/loomcli`'s `step_test.go` and `status_test.go` pin the exact ten-key envelope and the exact rendered watch line for the `loom` label, which are loom's own shipped contract, and batch 4 retargets those tables onto these functions rather than deleting them.

## Cards

### Card 10: declare Spec, Hooks, VerbTexts and the package doc

- **Context:**
  - `internal/shedengine/shed.go`
  - `internal/shedengine/run.go`
  - `internal/shedengine/status.go`
  - `internal/loomcli/run.go`
  - `internal/loomcli/status.go`
  - `internal/loomcli/pause.go`
  - `internal/lifecyclecli/run.go`
  - `internal/lifecyclecli/status.go`
- **Edits:** none
- **Creates:**
  - `internal/shedverbs/doc.go`
  - `internal/shedverbs/spec.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** declare `type Spec struct` carrying every resolution-dependent value the four generic bodies read, all told and none derived:
  `StatusPath string`, `LockPath string`, `StatusLockPath string`;
  `BuildShed func() (*shedengine.Shed, error)`;
  `Hooks Hooks`;
  `StatusLabel string` (the `--watch` line's literal prefix, `loom` for the loom recipe);
  `DecodeErrPrefix string` (the told prefix for this spec's own `state.ReadJSONStrict` failure, `loom:` for loom and `lifecyclecli:` for lifecycle);
  `RunBusyMessage string` and `StepBusyMessage string` (the told `ErrShedBusy` treatments; the empty string means passthrough — the body reports `err.Error()` verbatim);
  `StepBusyKind string` (the refusal kind `step` maps `ErrShedBusy` onto);
  `AbsentStatus AbsentDisposition`;
  `PauseAbsentMessage string`;
  `EnsureStatusLockDir bool`.
  Declare `type AbsentDisposition struct` with a `Refuse bool` and a `RefuseMessage string`: `Refuse` true means `status` reports `RefuseMessage` on the error envelope (loom's case), `Refuse` false means it reports `found: false` plus `status_path` on the success envelope (lifecycle's case).
  Declare `type Hooks struct` with six nil-by-default function fields, each skipped when nil, documented individually:
  `PreRun func(ctx context.Context) error`;
  `PostRun func(ctx context.Context, result shedengine.Result, runErr error) map[string]any`;
  `PreStep func(ctx context.Context) (kind string, err error)`;
  `PostStep func(res shedengine.StepResult)`;
  `InterruptPolicyFor func(row string) string`;
  `StatusExtras func(st shedengine.Status) (map[string]any, error)`.
  `PostStep` runs after a successful `Shed.Step` and before the envelope is written, and exists so loom can keep calling `recordStepHandoff` at exactly that point: a completed step's persisted aftermath is byte-identical to a mid-run driver death, and that marker is the one thing letting the next run's entry observation tell the two apart, so it can move neither above the `Step` call nor below the envelope.
  It is filled by loom and left nil by lifecycle.
  `PostRun`'s result parameter is typed `shedengine.Result` — the type `func (s *Shed) Run(ctx context.Context) (Result, error)` actually returns — per the overview's `run-result-type-is-shedengine-Result` Shared Decision.
  Declare `type VerbTexts struct` carrying a `Use`, `Short` and `Long` string per verb (four sub-structs or twelve fields, the implementer's call), passed by value at construction time because cobra builds the tree before any pre-run fills the `Spec`.
  `PreRun` returns no envelope map: neither shipped filler produces one, and a second extras source would need a precedence rule against `PostRun`'s for no present benefit.
  Record on `PostRun`'s own field doc that it runs unconditionally after `Shed.Run` returns, including when `runErr` is non-nil and before the error envelope is written, because that placement is what preserves loom's `detectAndFileAnomalies` call — the hard-error arm would otherwise drop a whole failure class and lose the in-memory crash observation permanently.
  Record on `BuildShed`'s own field doc why the spec carries a constructor rather than a pre-built `*shedengine.Shed`: `loomcli`'s pre-flight assigns `c.env.Landing = landingDeps(…)` on its way through, immediately before `loomrecipe.New`, so a Shed built at arming time would carry a nil `Landing` and the landing rows would fail deep in the run.
  A nil `BuildShed` is legal for `status` and `pause`, which never call it, and an error for `run` and `step`.
  Write `doc.go` stating the package's contract: it owns the generic verb bodies, derives no path, imports no resolver and imports no `<module>cli`.
- **Commit:** `feat(shedverbs): declare the arming Spec, Hooks and VerbTexts types`

### Card 11: implement the generic run body

- **Context:**
  - `internal/shedverbs/spec.go`
  - `internal/loomcli/run.go`
  - `internal/lifecyclecli/run.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/errors.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
- **Edits:** none
- **Creates:**
  - `internal/shedverbs/run.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** implement `runCmd(texts VerbTexts, spec *Spec) *cobra.Command` whose `RunE` checks `clihelp.ShouldAbort(cmd.Context())` first, then in order: calls `spec.Hooks.PreRun` when non-nil and reports a non-nil error on the error envelope without starting the run;
  calls `spec.BuildShed()` and reports a non-nil error on the error envelope;
  calls `shed.Run(cmd.Context())`;
  calls `spec.Hooks.PostRun(ctx, result, runErr)` when non-nil, unconditionally and before any error envelope is written;
  then, when `runErr` is non-nil, reports it on the error envelope — using `spec.RunBusyMessage` in place of `err.Error()` when `errors.Is(runErr, shedengine.ErrShedBusy)` and `RunBusyMessage` is non-empty, and `err.Error()` verbatim otherwise — and returns;
  then, on success, builds the envelope `{"outcome": string(result.Outcome), "halted_producer": result.HaltedProducer, "reason": result.Reason, "history_length": len(result.History)}` merged with `PostRun`'s returned map, and reports it with `output.Ok`.
  `PostRun`'s map is merged into the success envelope only and discarded when `runErr` is non-nil, since an error envelope carries no extras;
  `PostRun` is still called on that path, for its side effects, which is the whole reason it runs there.
  A nil `BuildShed` is an error on this verb: report a clear message naming the verb rather than panicking on a nil call.
  Record every envelope write through `clihelp.SetExit(cmd.Context(), …)` per this batch's own scope decision — both the error envelopes above and the success one.
  Take the `Use`/`Short`/`Long` strings from `texts`, never from a literal in this file.
- **Commit:** `feat(shedverbs): implement the generic run verb body`

### Card 12: implement the generic step body and the closed refusal-kind set

- **Context:**
  - `internal/shedverbs/spec.go`
  - `internal/loomcli/step.go`
  - `internal/shedengine/run.go`
  - `internal/shedengine/errors.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
- **Edits:** none
- **Creates:**
  - `internal/shedverbs/step.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** move the five refusal-kind constants and the `stepKinds` slice out of `internal/loomcli/step.go` and declare them here, exported, as the closed vocabulary: `KindBusy = "busy"`, `KindUnseeded = "unseeded"`, `KindOwnership = "ownership"`, `KindBootstrap = "bootstrap"`, `KindProducer = "producer"`, plus a `StepKinds` slice listing exactly those five.
  Carry across the existing doc comment explaining why the set is closed and why the supervisor skill's one-retry rule applies to `KindProducer` alone.
  Do not delete the loom-side constants in this card — batch 4 card 22 removes them, once `loomcli` has been rearmed onto these.
  Declare **exported** `StepEnvelope(res shedengine.StepResult, nextPolicy, statusFile string) map[string]any` returning exactly the ten keys `internal/loomcli/step.go`'s own `stepEnvelope` returns, with the same derivations: `producer`, `outcome`, `output`, `next`, `state`, `reason`, `continue` derived as `res.State == shedengine.StateRunning`, `history_length`, `next_interrupt_policy`, `status_file`.
  Carry across that function's doc comment, including the reason `continue` is derived in Go rather than left to the caller — a thin external supervisor skill never carries its own copy of the `State` vocabulary.
  Implement `stepCmd(texts VerbTexts, spec *Spec) *cobra.Command` whose `RunE` checks `clihelp.ShouldAbort` first, then calls `spec.Hooks.PreStep` when non-nil and, on a non-nil error, reports it with `output.ErrFields` carrying the hook's own returned `kind` on the envelope's `kind` field;
  then calls `spec.BuildShed()`, reporting a build error with `kind: KindBootstrap`;
  then calls `shed.Step(ctx)`, mapping an `errors.Is(err, shedengine.ErrShedBusy)` onto `spec.StepBusyKind` with `spec.StepBusyMessage` when non-empty and `err.Error()` verbatim otherwise, and any other error onto `kind: KindProducer` with `err.Error()`;
  then, on success, calls `spec.Hooks.PostStep(res)` when non-nil and then reports `output.Ok(out, StepEnvelope(res, nextPolicy, spec.StatusPath))` where `nextPolicy` is `spec.Hooks.InterruptPolicyFor(res.Next)` when the hook is non-nil and the empty string when it is nil.
  `PostStep` runs only on the success path and only before the envelope is written — never on an error path, since the marker records a clean handoff and a step that failed produced none.
  A recipe with no policy table therefore yields an empty `next_interrupt_policy`, which is the caller's "no entry" signal and never a third policy word.
  Record every envelope write through `clihelp.SetExit(cmd.Context(), …)` per this batch's own scope decision, wrapping `output.ErrFields` on each refusal and `output.Ok` on success.
  This body owns nothing above `shed.Step`: the run-lock probe, the `MkdirAll`, the seed-and-commit bootstrap and the status-strand work all belong to loom's `PreStep` and stay in `loomcli`.
- **Commit:** `feat(shedverbs): implement the generic step verb body and the closed kind set`

### Card 13: implement the generic status body's one-shot half

- **Context:**
  - `internal/shedverbs/spec.go`
  - `internal/loomcli/status.go`
  - `internal/lifecyclecli/status.go`
  - `internal/shedengine/status.go`
  - `internal/state/state.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/loomcli/sharedbootstrap.go`
- **Edits:** none
- **Creates:**
  - `internal/shedverbs/status.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** implement `statusCmd(texts VerbTexts, spec *Spec) *cobra.Command` registering two flags — `--watch` (bool, default false) and `--interval` (duration, default `time.Second`) — and no others, because those are the only two the generic body itself reads.
  `RunE` checks `clihelp.ShouldAbort` first, then performs `ensureStatusLockDir` when `spec.EnsureStatusLockDir` is true and skips it entirely when false, reporting a failure on the error envelope.
  That `MkdirAll` of the status lock's ephemeral parent is load-bearing and crucible-fixed: the status file is durable under `_lyx` while its lock is ephemeral under `.lyx`, `internal/lock` opens with `O_CREATE` and never creates a parent, and nothing creates that directory before a bootstrap — so without it both verbs failed inside lock acquisition on a never-bootstrapped pair and reported a raw "no such file or directory" instead of their own remedy.
  It travels as a told boolean rather than an unconditional generic step because lifecycle's `status` is read-only and creating its per-slug directory as a side effect of reading it would be a new, unasked-for write.
  Then read `state.ReadJSONStrict[shedengine.Status](spec.StatusPath, spec.StatusLockPath)` and, on a decode error, report `spec.DecodeErrPrefix + " decode status file " + spec.StatusPath + ": " + err.Error()` on the error envelope — the told prefix applies to this body's own `ReadJSONStrict` failure alone.
  On `!found`, apply `spec.AbsentStatus`: when `Refuse` is true report `RefuseMessage` on the error envelope, and when false report `output.Ok` with exactly the two keys `found: false` and `status_path: spec.StatusPath`.
  This disposition short-circuits *before* both the generic core and `StatusExtras`, so neither contributes a key to an absent-file envelope;
  passing a zero `shedengine.Status` through `StatusExtras` is explicitly rejected, because it would add keys to an envelope that carries neither today.
  On `found`, build the four-key generic core `{"current_producer": st.CurrentProducer, "state": string(st.State), "error": st.Error, "activity": st.Activity}`, call `spec.Hooks.StatusExtras(st)` when non-nil, report a non-nil hook error verbatim on the error envelope with no re-prefixing — the hook owns its whole string — and otherwise merge the returned map onto the core and report it with `output.Ok`.
  Declare an unexported `ensureStatusLockDir(prefix, statusLockPath string) error` in this file performing `os.MkdirAll(filepath.Dir(statusLockPath), 0o755)` and, on failure, returning `<prefix> create the status lock's directory <dir>: <err>` composed from the told `spec.DecodeErrPrefix`.
  Reusing that told prefix reproduces `internal/loomcli/sharedbootstrap.go`'s existing `loom: create the status lock's directory %s: %w` text byte-for-byte for loom's spec, rather than inventing a second prefix field for one string.
  Carry that function's existing doc comment across: the status file is durable under `_lyx` while its lock is ephemeral under `.lyx`, `internal/lock` opens with `O_CREATE` and never creates a parent, nothing creates the ephemeral directory before a bootstrap has run, and without this both verbs' carefully-worded "no status file … run \"lyx loom start\"" messages were unreachable on the one path they exist for.
  Keep its closing note that this creates a directory and reads nothing, so it cannot resurrect a deleted status file or mask a genuine absence — `found` still answers that question.
  Batch 4, card 23 deletes `loomcli`'s own copy once its two callers are gone, so this doc comment is where that history lives afterwards.
  Record every envelope write through `clihelp.SetExit(cmd.Context(), …)` per this batch's own scope decision.
  The `--watch` branch is card 14's; leave a call site for it between the absent-file disposition and the core-envelope assembly, matching `internal/loomcli/status.go`'s own ordering.
- **Commit:** `feat(shedverbs): implement the generic status verb's one-shot envelope`

### Card 14: implement the status --watch tail

- **Context:**
  - `internal/shedverbs/spec.go`
  - `internal/loomcli/status.go`
  - `internal/shedengine/status.go`
  - `internal/state/state.go`
- **Edits:**
  - `internal/shedverbs/status.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** add **exported** `RenderStatusLine(label string, st shedengine.Status) string` composing exactly one line: the told `label`, then the state, then `" | now "` and `st.Activity.Now`, then `" | last "` and `st.Activity.Last` only when non-empty, then `" | wait "` and `st.Activity.Wait` only when non-empty.
  For `label` of `loom` this must render byte-identically to `internal/loomcli/status.go`'s existing unexported `renderStatusLine`, whose format is `loom %s | now %s` plus the two optional tails.
  Add **exported** `UnavailableLine(label string) string` composing `<label> status unavailable (status file transiently unreadable)`, and compute it ONCE outside the poll loop rather than per poll.
  That is required, not stylistic: the tail dedupes on printed text, so a line recomposed per poll that ever differed by a byte would turn a transient fault into its own flood, which is the whole thing the dedupe exists to prevent.
  Add **exported** `PrintStatusLinesOnChange(out io.Writer, poll func() string, sleep func(), polls int)` with the same body and the same semantics as `internal/loomcli/status.go`'s own: print only when the polled line differs from the last printed one, and treat a non-positive `polls` as "poll forever", which is what the production call passes.
  Carry across that function's doc comment explaining why suppressing an unchanged line is the whole point — a producer call lasts minutes while the tail polls every second, so printing unconditionally fills tmux's scrollback and buries the one line an operator needs.
  Wire the `--watch` branch into `statusCmd`'s `RunE` at the call site card 13 left: after the absent-file disposition has already short-circuited, so a `--watch` against an absent status file returns the told disposition immediately instead of entering the tail.
  On a never-run slug, `lyx lifecycle status --watch` therefore exits immediately rather than waiting;
  that is the decided behaviour, not an accident.
  Inside the poll closure, a read failure or a `!found` returns the precomputed unavailable line and must not terminate the tail and must not write an envelope — the pane is expected to survive the driver rewriting the file underneath it.
- **Commit:** `feat(shedverbs): implement the status --watch tail with a told label`

### Card 15: implement the generic pause body

- **Context:**
  - `internal/shedverbs/spec.go`
  - `internal/shedverbs/status.go`
  - `internal/loomcli/pause.go`
  - `internal/shedengine/status.go`
  - `internal/state/state.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
- **Edits:** none
- **Creates:**
  - `internal/shedverbs/pause.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** implement `pauseCmd(texts VerbTexts, spec *Spec) *cobra.Command` whose `RunE` checks `clihelp.ShouldAbort` first, performs `ensureStatusLockDir(spec.DecodeErrPrefix, spec.StatusLockPath)` when `spec.EnsureStatusLockDir` is true, matching card 13's two-argument signature, then calls `state.UpdateJSON(spec.StatusPath, spec.StatusLockPath, …)` with a mutate closure that returns `errors.New(spec.PauseAbsentMessage)` when `!found` and otherwise sets `cur.PauseRequested = true` and returns `cur`.
  Report any error on the error envelope and, on success, report `output.Ok` with exactly the one key `status_file: spec.StatusPath`.
  Record both writes through `clihelp.SetExit(cmd.Context(), …)` per this batch's own scope decision.
  `pause` never calls `BuildShed` — a nil one is legal here, exactly as on `status`.
- **Commit:** `feat(shedverbs): implement the generic pause verb body`

### Card 16: assemble the Verbs constructor

- **Context:**
  - `internal/shedverbs/spec.go`
  - `internal/shedverbs/run.go`
  - `internal/shedverbs/step.go`
  - `internal/shedverbs/status.go`
  - `internal/shedverbs/pause.go`
- **Edits:** none
- **Creates:**
  - `internal/shedverbs/verbs.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** declare `func Verbs(texts VerbTexts, spec *Spec) []*cobra.Command` returning the four commands in the order `run`, `step`, `status`, `pause`.
  Document why the two arguments have different lifetimes, per the overview's `build-time-texts-run-time-spec` Shared Decision: cobra builds the whole tree before any `PersistentPreRunE` runs, so help text must exist at construction time while resolved paths and hooks cannot, and the arming module fills the pointed-to `Spec` in its own pre-run.
  Document that a module needing more than the two flags the generic bodies read decorates the returned command itself before adding it to its own subtree — `loomcli` registers `--parent` on the `step` command it got back, and `lifecyclecli` sets `Args: cobra.ExactArgs(1)` — and that hooks reach those values by closure over the module's own receiver, never through a `shedverbs` parameter.
  `Verbs` registers no flag beyond `status`'s `--watch` and `--interval`, and declares no `Args` constraint of its own.
  This package exposes no `Command()`/`RunCLI` seam: it is not a CLI module and is not counted in the CLI/Cobra Invariant's module tally.
- **Commit:** `feat(shedverbs): add the Verbs constructor returning the four subcommands`

### Card 17: test the run body's hook ordering

- **Context:**
  - `internal/shedverbs/spec.go`
  - `internal/shedverbs/run.go`
  - `internal/shedverbs/verbs.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/errors.go`
  - `internal/shedrecipe/entries_simple.go`
  - `internal/clihelp/exec.go`
  - `internal/loomcli/parity_test.go`
- **Edits:** none
- **Creates:**
  - `internal/shedverbs/testsupport_test.go`
  - `internal/shedverbs/run_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `testsupport_test.go`, build the shared fake: a helper returning a `*shedengine.Shed` over `Stub` rows from the registry, told a `t.TempDir()`-anchored status path, run lock and status lock, plus a helper that executes one built command through `clihelp` against a `bytes.Buffer` and decodes the emitted JSON envelope into a `map[string]any`.
  No LLM, no tmux and no git may be involved, and no test here may construct a real hub — this package is tier 1.
  In `run_test.go`, cover: a successful run's four-key envelope;
  an `ErrShedBusy` run with an empty `RunBusyMessage` reporting `err.Error()` verbatim and with a filled one reporting the told text instead;
  a producer hard error reaching the error envelope;
  each of `PreRun` and `PostRun` nil, and each filled;
  a `PreRun` returning a non-nil error reporting it on the error envelope and leaving `BuildShed` uncalled, asserted by a call counter on the constructor closure.
  Give the `PostRun`-runs-on-the-error-path case its own named test, because it is the property most easily lost in the extraction: assert that `PostRun` was called with the non-nil `runErr`, that it was called *before* the error envelope was written, and that its returned map does not appear on that envelope.
  Assert the before-the-envelope ordering by having the `PostRun` filler write into the same buffer the envelope is written to and checking the buffer's ordering, rather than by asserting on a call counter alone, which cannot distinguish "called before" from "called after".
- **Commit:** `test(shedverbs): cover the run body's hook ordering and busy treatments`

### Card 18: test the step, status, watch and pause bodies

- **Context:**
  - `internal/shedverbs/spec.go`
  - `internal/shedverbs/step.go`
  - `internal/shedverbs/status.go`
  - `internal/shedverbs/pause.go`
  - `internal/shedverbs/verbs.go`
  - `internal/shedverbs/testsupport_test.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/run.go`
  - `internal/loomcli/step_test.go`
  - `internal/loomcli/status_test.go`
  - `internal/lifecyclecli/status.go`
- **Edits:** none
- **Creates:**
  - `internal/shedverbs/step_test.go`
  - `internal/shedverbs/status_test.go`
  - `internal/shedverbs/pause_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** in `step_test.go`, assert the ten-key envelope's key set is exactly closed at ten and no larger, mirroring the existing closure test in `internal/loomcli/step_test.go`;
  assert `continue` is true only for `shedengine.StateRunning`;
  assert `next_interrupt_policy` is empty when `InterruptPolicyFor` is nil and is threaded through when the hook is filled;
  assert the five refusal kinds are exactly `StepKinds` and no larger;
  assert a `PreStep` returning `(kind, err)` reaches the envelope's `kind` field with that exact kind and leaves `BuildShed` uncalled;
  assert `PostStep` is called with the returned `shedengine.StepResult` on the success path, before the envelope is written, and is not called on either the `PreStep`-error path or the `Step`-error path.
  In `status_test.go`, drive both told absent-file dispositions — the refusing form, asserting the told message on the error envelope, and the `found: false` form, asserting the success envelope carries exactly the two keys `found` and `status_path` and no core key and no extras key;
  assert `StatusExtras` merging onto the four-key core;
  assert a `StatusExtras` error reaches the error envelope verbatim with no added prefix;
  assert the decode-error prefix is the told one;
  assert the told lock-dir boolean's effect both ways, over a status-lock path whose parent directory does not yet exist: with `EnsureStatusLockDir` true, the parent is created and the read goes on to produce the told absent-file disposition;
  with it false, the parent is still absent afterwards and the read fails in lock acquisition instead.
  That failing arm is the assertion, not an accident: `internal/lock`'s `AcquireReadLock` is `flock.New(path).RLock()`, which creates the lock file but never its parent, and `state.ReadJSONStrict` adds no `MkdirAll` of its own — so a skipped `MkdirAll` is observable exactly there.
  Drive the reachable `found: false` and refusal dispositions separately, over a status-lock path whose parent already exists, which is the state both shipped consumers are in whenever those dispositions actually fire.
  For the `--watch` tail, drive `PrintStatusLinesOnChange` through a finite `polls` count with an injected sleep and no wall-clock wait, exactly as `printStatusLinesOnChange` and `awaitRunLock` are driven today;
  assert change-only printing;
  assert the told label appears in the rendered line;
  assert the composed unavailable line is byte-identical across polls for a given label, which is the dedupe property;
  assert a `--watch` against an absent status file returns the told disposition immediately instead of entering the tail.
  In `pause_test.go`, assert `PauseRequested` is set on the persisted status;
  assert the told absent-file message is reported for each of the two shipped wordings;
  assert the success envelope carries exactly the one key `status_file`.
- **Commit:** `test(shedverbs): cover step, status, watch and pause against a fake Shed`

### Card 19: enforce the no-resolver, no-modulecli seam

- **Context:**
  - `internal/lifecycleshed/seam_enforcement_test.go`
  - `internal/loomrecipe/seam_enforcement_test.go`
  - `internal/shedverbs/spec.go`
  - `internal/shedverbs/run.go`
  - `internal/shedverbs/step.go`
  - `internal/shedverbs/status.go`
  - `internal/shedverbs/pause.go`
  - `internal/shedverbs/verbs.go`
  - `internal/shedverbs/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/shedverbs/seam_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** add an AST-walking scan in the style of `internal/lifecycleshed/seam_enforcement_test.go`: parse every non-test `.go` file in this package and assert each non-stdlib import path is an entry in an allowlist naming exactly `internal/clihelp`, `internal/output`, `internal/state`, `internal/shedengine` and `github.com/spf13/cobra`.
  Assert separately, and by name so a violation is reported explicitly rather than only implied by the allowlist's absence, that no production import path is `internal/lyxcwd` and that no production import path matches the `<module>cli` shape — an `internal/` path whose final segment ends in `cli`.
  Add a second assertion that no production file in this package references `os.Getwd`, matching the no-resolver clause the new Shed Verb-Set Invariant carries in batch 6;
  a selector-expression walk is enough, since the package imports no shell runner through which `git rev-parse` could reach it.
  The allowlist is deliberately a membership list rather than a bare `lyxcwd` denylist, mirroring the two existing scans' own reasoning: it catches the excluded import and anything else that would drag geometry resolution in, with no list maintenance beyond a genuine new dependency.
  `shedverbs` importing cobra while not being a `<module>cli` package is deliberate: the rule that matters is that an *engine* never imports cli/cobra, and `shedverbs` is not an engine.
- **Commit:** `test(shedverbs): enforce the no-resolver and no-modulecli import seam`

### Card 20: confirm the package builds clean against the live tree

- **Context:**
  - `internal/shedverbs/doc.go`
  - `internal/shedverbs/spec.go`
  - `internal/shedverbs/run.go`
  - `internal/shedverbs/step.go`
  - `internal/shedverbs/status.go`
  - `internal/shedverbs/pause.go`
  - `internal/shedverbs/verbs.go`
  - `internal/shedverbs/seam_enforcement_test.go`
  - `cmd/lyx/drift_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** run `go build ./internal/shedverbs/...` and `go vet ./internal/shedverbs/...` and confirm both exit zero, then run the batch's own `verify:` command and confirm every test in the package passes.
  Confirm by inspection that no file in the package declares a `Command()` or a `RunCLI` function, since `shedverbs` is not a CLI module and must not be counted in the CLI/Cobra Invariant's module tally.
  Confirm by grep that every `output.Err`, `output.ErrFields` and `output.Ok` call in this package is wrapped in a `clihelp.SetExit(...)` call — an unwrapped one exits 0 on a refusal, which no test asserting envelope *content* would catch.
  Confirm every command returned by `Verbs` carries a non-blank `Short`, which `cmd/lyx/drift_test.go` will enforce across the whole tree once batch 5 registers the subtree.
  This card changes no file;
  it is the gate that the package is self-contained before batch 4 arms it.
- **Commit:** none

## Batch Tests

`verify:` runs this package alone, which is correct scope: the batch creates one new package and edits nothing outside it, so no other package's behaviour can have changed.

The suite is the whole new test surface the task introduces: `run_test.go` (hook ordering, the named `PostRun`-on-the-error-path property, both busy treatments), `step_test.go` (the ten-key closure, `continue`'s derivation, the five-kind closure, `PreStep`'s kind), `status_test.go` (both absent-file dispositions, `StatusExtras` merging and its un-re-prefixed error, the told decode prefix, the told lock-dir boolean's presence and absence, and the `--watch` tail driven through a finite `polls` count), `pause_test.go` (the flag, both told refusals, the single-key envelope), and `seam_enforcement_test.go` (the no-resolver, no-`<module>cli` allowlist scan).

Everything runs against a fake `*shedengine.Shed` built from `Stub` rows over a `t.TempDir()`, so the package stays tier 1 — no hub, no git, no tmux, no LLM — and needs no `-tags integration` chain.
