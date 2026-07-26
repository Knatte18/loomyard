# Batch: treadle-extraction

```yaml
task: 'Treadle: shared round-loop engine + perch rewrite'
batch: treadle-extraction
number: 1
cards: 5
verify: go test ./internal/treadleengine/... ./internal/perchengine/... ./internal/perchcli/... ./cmd/lyx/...
depends-on: []
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move
   (package declaration, imports, identifier retargeting, seam splits).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks
   git rename history and inflates review diffs.

## Batch Scope

Extracts perch's round loop into the new generalized `internal/treadleengine`
package (pluggable `RoundRunner` seam, own vocabulary, no burlerengine import)
and rewrites `internal/perchengine` as a thin adapter over it, with a
byte-identical exported Go surface so `internal/perchcli` compiles untouched.
No behavior change whatsoever in this batch: the judge still reads all prior
reviews (handoff lands in batch 2), no targeting (batch 3), no config changes
(batch 4). The external interface batch 2 consumes is treadleengine's loop
(`Engine.Run`, `RoundRunner`, `judgeInputs`, the moved `roundRecord`/
`artifactPaths` machinery).

Batch-local note on compile atomicity: cards 1–3 form one atomic refactor.
Card 1's commit compiles standalone (a new leaf package). Card 2's commit is
the extraction — after it, the whole module compiles and `perchcli` is
untouched, but the in-package perchengine test files that reference moved
unexported symbols do not compile until card 3 relocates them. That
intermediate state is confined to cards 2→3 within this batch; `verify:` runs
at batch end. This is the honest atomic unit for an extraction — there is no
smaller sequence of fully-green commits that preserves `git mv` history.

## Cards

### Card 1: treadleengine skeleton and vocabulary

- **Context:**
  - `internal/perchengine/doc.go`
  - `internal/perchengine/engine.go`
  - `internal/perchengine/profile.go`
  - `internal/perchengine/result.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/engine.go`
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/verdict.go`
  - `manifest/designs/treadle.md`
  - `_mill/discussion.md`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/treadleengine/doc.go`
  - `internal/treadleengine/runner.go`
  - `internal/treadleengine/profile.go`
  - `internal/treadleengine/engine.go`
  - `internal/treadleengine/result.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create package `treadleengine` with its generalized
  vocabulary; this card must compile standalone (no dependency on code that
  moves in card 2).
  `runner.go`: `type Verdict string` with `VerdictApproved Verdict = "APPROVED"`
  and `VerdictBlocking Verdict = "BLOCKING"`; `type AttemptInput struct` with
  fields `RunDir string`, `Round int`, `Attempt int`, `RoundToken string`,
  `ReviewPath string`, `FixerReportPath string`, `PriorReviews []string`,
  `PriorFixerReports []string`, `Model string`, `Effort string`,
  `Timeout time.Duration` (a `SeedPath` field is added in batch 3, not here);
  `type AttemptResult struct` with fields `Outcome shuttleengine.Outcome`,
  `Verdict Verdict`, `BlockingCount int`, `ReviewPath string`,
  `FixerReportPath string`, `SessionID string`, `LastAssistantMessage string`,
  `RunDir string`; `type RoundRunner interface { RunAttempt(AttemptInput)
  (AttemptResult, error) }`. Document on `RoundRunner` that one call runs ONE
  attempt of one round; the engine owns retry, triage, stale-move, token
  naming, and hydration assembly (the discussion's attempt-level seam).
  `profile.go`: treadle's own `type GateMode string` with constants
  `GateLLMVerdict GateMode = "llm-verdict"`, `GateCommand GateMode = "command"`,
  `GateBoth GateMode = "both"`; `type Gate struct { Mode GateMode; Command
  []string; Timeout time.Duration }` (exactly these three fields — perch's
  exported `Gate` stays a distinct identical struct, so no extra field may
  appear here); `type Profile struct` with fields `ProfileHash string`
  (caller-computed block identity, stamped into state.json verbatim),
  `Gate Gate`, `GateDir string` (absolute cwd for the gate command —
  caller-supplied, keeping treadle geometry-blind), `RoundCaps []int`
  (resolved, non-empty, strictly increasing — treadle does NO default
  resolution), `JudgeModel string`, `JudgeEffort string`, `Model string`,
  `Effort string`, `Timeout time.Duration`. Give `Profile` a fail-loud
  `validate()` covering only structural invariants (non-empty strictly
  increasing `RoundCaps`; legal `Gate.Mode` with the same
  command-emptiness cross-checks `perchengine.Profile.validate` applies
  today; non-negative timeouts; non-empty `ProfileHash`; non-empty `GateDir`
  when `Gate.Mode` runs a command), with every message prefixed by the
  engine name (see `engine.go`).
  `engine.go`: `type CommandRunner func(argv []string, dir string, timeout
  time.Duration) ([]byte, bool, error)` (doc text carried over from
  perchengine's); `type Options struct { PauseRequested func() bool;
  RunCommand CommandRunner }`; `type Engine struct` holding `name string`,
  `runner RoundRunner`, `shuttle Shuttle`, plus the two Options fields
  stored verbatim; `func New(name string, runner RoundRunner, shuttle
  Shuttle, opts Options) *Engine`. Declare a small unexported helper (e.g.
  `func (e *Engine) errf(format string, args ...any) error`) that prefixes
  errors with `e.name + ": "` — the shared-decision
  name-parameterized-diagnostics mechanism. The `Shuttle` interface itself
  moves here from perch in card 2; this card may declare it (`type Shuttle
  interface { Run(shuttleengine.Spec) (shuttleengine.Result, error) }`) so
  the card compiles, with card 2 deleting perch's copy.
  `result.go`: treadle-owned `type Outcome string`
  (`OutcomeApproved "APPROVED"`, `OutcomeStuck "STUCK"`,
  `OutcomePaused "PAUSED"`), `type StuckReason string` (`StuckHardCap
  "hard-cap"`, `StuckMilestoneStop "milestone-stop"`, `StuckCircling
  "circling"`), `type RoundSummary struct` mirroring perchengine's
  `RoundSummary` field-for-field except `Verdict Verdict` (treadle's own
  vocabulary instead of `burlerengine.Verdict`), and `type Result struct {
  Outcome Outcome; StuckReason StuckReason; RoundsRun int; Rounds
  []RoundSummary }`. `doc.go`: a real package header describing the
  generalized round loop, the RoundRunner seam, and geometry/weft-blindness
  (full doc-lifecycle polish happens in batch 5; this must already be a
  correct, self-standing package doc).
- **Commit:** `treadle: add treadleengine package skeleton and round-runner vocabulary`

### Card 2: extract the loop into treadleengine and rewire perchengine

- **Context:**
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/profile.go`
  - `internal/burlerengine/verdict.go`
  - `internal/shuttleengine/spec.go`
  - `internal/treadleengine/runner.go`
  - `internal/treadleengine/profile.go`
  - `internal/treadleengine/result.go`
  - `internal/perchengine/profile.go`
  - `internal/perchengine/result.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/treadleengine/engine.go`
  - `internal/perchengine/engine.go`
  - `internal/perchengine/doc.go`
  - `internal/perchengine/template.go`
- **Creates:**
  - `internal/treadleengine/template.go`
  - `internal/perchengine/adapter.go`
  - `internal/perchengine/identity.go`
- **Deletes:** none
- **Moves:**
  - `internal/perchengine/run.go` -> `internal/treadleengine/run.go`
  - `internal/perchengine/judge.go` -> `internal/treadleengine/judge.go`
  - `internal/perchengine/judgeverdict.go` -> `internal/treadleengine/judgeverdict.go`
  - `internal/perchengine/state.go` -> `internal/treadleengine/state.go`
  - `internal/perchengine/gate.go` -> `internal/treadleengine/gate.go`
  - `internal/perchengine/roundfiles.go` -> `internal/treadleengine/roundfiles.go`
  - `internal/perchengine/judge-circling-template.md` -> `internal/treadleengine/judge-circling-template.md`
  - `internal/perchengine/judge-milestone-template.md` -> `internal/treadleengine/judge-milestone-template.md`
  - `internal/perchengine/triage-template.md` -> `internal/treadleengine/triage-template.md`
- **Requirements:** The extraction commit. Treadle side, per moved file
  (surgical edits, `package treadleengine`, drop every `burlerengine`
  import):
  `run.go`: `Engine.Run(p Profile, runDir string) (Result, error)` keeps
  today's loop structure verbatim — pause-flag clear semantics, `ProfileHash`
  handling replaced by using `p.ProfileHash` directly (the caller computes
  it; treadle just validates non-empty and stamps it), `p.validate()`
  (treadle's, card 1), run.lock acquisition with `ErrBlockBusy` (sentinel
  message becomes the un-prefixed `block is already running`; the wrap adds
  `e.name` so the composed text is byte-identical to today's), state
  load/resume, the resume-past-hard-cap finalization, the round loop with
  gate execution (cwd from `p.GateDir` instead of `layout.WorktreeRoot`),
  convergence, and the stuck ladder. `runRound` drives
  `e.runner.RunAttempt` with a fully-populated `AttemptInput` (run dir,
  round, attempt, `roundToken(round, attempt)`, this attempt's
  review/fixer-report paths from `artifactPaths`, the hydration lists, and
  `p.Model`/`p.Effort`/`p.Timeout`), branching on
  `AttemptResult.Outcome` exactly as today (done / asking-with-triage /
  died-timeout retry, second-consecutive-failure error text unchanged
  modulo name prefix). `roundOutcome.Findings` is replaced by carrying
  `AttemptResult.BlockingCount` straight into the record (delete
  `countBlockingFindings` from the loop — the runner reports the count).
  `collectPriorHydration`, `collectJudgeReviews`, `isMilestoneRung`,
  `resultFromState` move as-is (types retargeted to treadle vocabulary).
  `judge.go`: keep `Shuttle` here (delete card 1's temporary declaration if
  one was made in `engine.go` — exactly one declaration survives),
  `judgeInputs`, `runCircling`, `runMilestone`, `runJudgeCall`, `runTriage`
  unchanged except Warn labels built from the engine name (pass the name
  in — e.g. thread it through the call sites — so today's exact
  `perch: circling judge failed...` strings are preserved when name is
  `"perch"`).
  `judgeverdict.go`: moves verbatim (exported `ParseJudgeVerdict`/
  `ParseTriageVerdict`, `JudgeVerdict`/`TriageVerdict` and constants,
  unexported `judgeFraming`/`splitFrontmatter`), with one pinned change:
  the parsers are package-level pure functions with no engine name in
  scope, so their error strings switch from the literal `perch: ` prefix
  to the neutral `treadle: ` prefix, and the moved `judgeverdict_test.go`
  (card 3) updates ONLY those prefix expectations. This does not weaken the
  differential bar: parser errors never surface through perch's public API
  — judge.go swallows them into Warn + fail-safe fallback.
  `state.go`: `runState`/`roundRecord` JSON schema byte-identical (see
  shared decision state-json-compatibility); `loadOrInitState`, `saveState`,
  `moveStaleArtifacts`, `moveStaleIfExists`, `fileExists`,
  `PauseFlagPath`, `PauseFlagName`, `clearPauseFlag`, `TerminalOutcome`
  (returns treadle's `Outcome`) move; `ProfileHash`, `DeriveRunID`,
  `ValidRunID`, `sanitizeSlug` do NOT move — they are extracted to
  `internal/perchengine/identity.go` (rename-plus-extraction: the moved
  file keeps the state machinery, the extraction keeps perch's identity
  functions; delete them from the moved file). Error strings
  name-parameterized where they carry `perch: ` today (`loadOrInitState`'s
  "started with a different profile" and "already finished" messages must
  stay byte-identical when name is `"perch"` — thread the name through or
  make these methods on `*Engine`).
  `gate.go`: `execGateCommand`, `writeGateOutput`, `converged` move;
  `converged`'s `verdict burlerengine.Verdict` parameter becomes treadle's
  `Verdict`.
  `roundfiles.go`: `roundToken`, `roundArtifactPaths`, `artifactPaths`
  move; `buildRoundProfile` does NOT move — it is extracted to
  `internal/perchengine/adapter.go` (delete from the moved file).
  `template.go` (create, treadle): `//go:embed` the three moved `.md`
  templates into `judgeCirclingTemplate`/`judgeMilestoneTemplate`/
  `triageTemplate` package vars, mirroring the embed pattern; the moved
  template `.md` files themselves are content-unchanged in this batch.
  Perch side:
  `internal/perchengine/engine.go` (edit): keep `Burler`,
  `CommandRunner` (perch-owned type, converted to treadle's at wiring),
  `Options`, `Engine`, `New` with today's exact signatures
  (`New(burler Burler, shuttle Shuttle, cfg Config, layout
  *hubgeometry.Layout, opts Options)`); perch's `Shuttle` becomes a type
  alias `type Shuttle = treadleengine.Shuttle` so existing fakes satisfy
  it unchanged. Add the thin `func (e *Engine) Run(p Profile, runDir
  string) (Result, error)`: compute `ProfileHash(p)` (identity.go),
  `p.validate(e.cfg)` (unchanged, stays in `profile.go` untouched),
  construct the burler adapter (adapter.go) closing over `p`'s content
  fields, build `treadleengine.Profile` from the resolved perch Profile
  (`GateDir: e.layout.WorktreeRoot`, Gate converted field-for-field),
  construct `treadleengine.New("perch", adapter, e.shuttle,
  treadleengine.Options{...})` and delegate, then map
  `treadleengine.Result` onto perch's `Result`/`RoundSummary`
  (`Verdict: burlerengine.Verdict(...)`). MkdirAll of runDir stays
  wherever it lives after the move (treadle's Run keeps it — perch must
  not duplicate it).
  `internal/perchengine/adapter.go` (create): unexported adapter type
  implementing `treadleengine.RoundRunner` over the `Burler` seam:
  `RunAttempt` maps `AttemptInput` onto `burlerengine.Profile` via the
  relocated `buildRoundProfile` logic — whose post-extraction signature is
  pinned here, since its old `roundArtifactPaths` parameter type stays
  unexported in treadleengine: `buildRoundProfile(p Profile, reviewPath,
  fixerReportPath string, priorReviews, priorFixerReports []string)
  burlerengine.Profile` (content fields from the perch Profile; the two
  path strings and hydration lists sourced from `AttemptInput`'s
  `ReviewPath`/`FixerReportPath`/`PriorReviews`/`PriorFixerReports`) — and
  `burlerengine.RunOpts{Model, Effort, Timeout, Round: input.RoundToken}`,
  then maps `burlerengine.Result` onto `AttemptResult` (including
  `BlockingCount` via the relocated `countBlockingFindings` over
  `Result.Findings` — move that helper here).
  `internal/perchengine/identity.go` (create): `ProfileHash`, `DeriveRunID`,
  `ValidRunID`, `sanitizeSlug` (moved out of state.go, doc comments
  intact); delegations `func TerminalOutcome(runDir string) (Outcome, bool,
  error)` (calls treadle's, converts Outcome), `func PauseFlagPath(runDir
  string) string`, `const PauseFlagName = treadleengine.PauseFlagName`;
  `var ErrBlockBusy = treadleengine.ErrBlockBusy` (same instance so
  `errors.Is` matches for perchcli); type aliases `type JudgeVerdict =
  treadleengine.JudgeVerdict`, `type TriageVerdict =
  treadleengine.TriageVerdict` and aliased constants `JudgeProgressing`,
  `JudgeCircling`, `JudgeContinue`, `JudgeStop`, `JudgeUncertain`,
  `TriageRetry`, `TriageGiveUp`.
  `internal/perchengine/template.go` (edit): drop the three judge/triage
  template embeds (they moved); keep `ConfigTemplate`.
  `internal/perchengine/doc.go` (edit): minimal accuracy pass only — add a
  paragraph stating the loop now lives in `internal/treadleengine` and
  perchengine is the burler-adapting configuration layer; do not restructure
  (batch 5 owns the full doc pass). `internal/perchengine/profile.go` and
  `result.go` are deliberately untouched.
- **Commit:** `treadle: extract round loop from perchengine into treadleengine`

### Card 3: relocate the moved machinery's tests

- **Context:**
  - `internal/treadleengine/run.go`
  - `internal/treadleengine/state.go`
  - `internal/treadleengine/roundfiles.go`
  - `internal/perchengine/testmain_test.go`
  - `internal/lyxtest/hermetic.go`
- **Edits:**
  - `internal/perchengine/run_test.go`
- **Creates:**
  - `internal/treadleengine/testmain_test.go`
  - `internal/perchengine/identity_test.go`
  - `internal/perchengine/adapter_test.go`
- **Deletes:** none
- **Moves:**
  - `internal/perchengine/judge_test.go` -> `internal/treadleengine/judge_test.go`
  - `internal/perchengine/judgeverdict_test.go` -> `internal/treadleengine/judgeverdict_test.go`
  - `internal/perchengine/state_test.go` -> `internal/treadleengine/state_test.go`
  - `internal/perchengine/roundfiles_test.go` -> `internal/treadleengine/roundfiles_test.go`
  - `internal/perchengine/gate_test.go` -> `internal/treadleengine/gate_test.go`
  - `internal/perchengine/gate_lingering_test.go` -> `internal/treadleengine/gate_lingering_test.go`
  - `internal/perchengine/smoke_judge_test.go` -> `internal/treadleengine/smoke_judge_test.go`
  - `internal/perchengine/template_test.go` -> `internal/treadleengine/template_test.go`
- **Requirements:** Move the eight test files exercising moved unexported
  machinery into `package treadleengine` via `git mv` with surgical edits
  only: package declaration, and identifier retargeting where the moved
  production code changed (e.g. `converged`'s verdict parameter type).
  `judgeverdict_test.go` additionally updates ONLY the error-prefix
  expectations from `perch: ` to `treadle: ` per card 2's pinned
  parser-prefix resolution. Header-comment corrections on moved files are
  licensed alongside the package-declaration/identifier edits wherever the
  move falsifies the header's own claims — concretely:
  `smoke_judge_test.go`'s "This file stays in package perchengine ..."
  rationale updates to name treadleengine, and `roundfiles_test.go`'s
  header drops its `buildRoundProfile` claim once that test is extracted
  out below.
  Two of the moved files contain sub-tests for functions that card 2 kept
  in perchengine — those tests move BACK out (rename-plus-extraction on the
  test side, so the bulk of each file keeps its `git mv` history):
  `state_test.go`'s `TestProfileHash`, `TestDeriveRunID`, and
  `TestValidRunID` (their subjects live in `perchengine/identity.go`) are
  extracted into the new `internal/perchengine/identity_test.go`, and
  `roundfiles_test.go`'s `TestBuildRoundProfile_FieldMapping` (its subject
  lives in `perchengine/adapter.go`, and it builds a perch-shaped
  `Profile`) is extracted into the new
  `internal/perchengine/adapter_test.go` — in `package perchengine`. The
  three identity tests move verbatim; the extracted
  `TestBuildRoundProfile_FieldMapping` keeps its assertion structure but
  its FIXTURE is necessarily rewritten: the current body calls
  `artifactPaths` (unexported, now treadle-side) to build a
  `roundArtifactPaths` argument, and `buildRoundProfile`'s post-extraction
  signature (pinned in card 2) takes plain
  `reviewPath`/`fixerReportPath` strings — construct those as literal
  path strings and update the call accordingly. What remains in the moved
  `state_test.go` (`TestLoadOrInitState`, `TestSaveState_ReadJSONRoundTrip`,
  `TestTerminalOutcome`, `TestMoveStaleArtifacts`, `TestPauseFlag`) and
  `roundfiles_test.go` (`TestRoundToken`, `TestArtifactPaths`) compiles in
  `package treadleengine` against the moved machinery.
  Cross-package test-helper fallout (licensed by the differential-test-bar
  decision's mechanical-package-split clause; each helper is a few lines,
  duplicated verbatim): (a) `writeFile` is defined in the moving
  `state_test.go` but called by the staying `run_test.go` — duplicate it
  into a perchengine-side test file (e.g. `run_test.go` itself);
  (b) `stringSlicesEqual` is defined in the moving `roundfiles_test.go`
  but called by `run_test.go` — duplicate it perchengine-side likewise;
  (c) `intSlicesEqual` is defined in the staying `profile_test.go` but
  called by the moved `state_test.go` — duplicate it into the moved
  `treadleengine/state_test.go`. Assertion bodies stay untouched in all
  cases; only helper availability is restored. Preserve every
  build tag (`//go:build integration` on gate_lingering, `//go:build smoke`
  on smoke_judge) as the file's first line. `judge_test.go`'s Warn-label
  and fail-safe assertions must keep passing with the name-parameterized
  labels (name `"perch"` is not in play inside treadle-only tests — where a
  test constructs the machinery directly, pass an explicit name and update
  only the expected prefix, keeping assertion structure intact; prefer
  passing `"perch"` to keep strings verbatim).
  `internal/treadleengine/testmain_test.go` (create): `TestMain` calling
  `lyxtest.HermeticGitEnv()` before `m.Run()`, mirroring
  `internal/perchengine/testmain_test.go` (the moved smoke test spawns git
  via lyxtest fixtures — the Hermetic Git guard requires this).
  `internal/perchengine/run_test.go` (edit, mechanical only): the file
  stays in perchengine as the differential heart. Replace its uses of
  moved unexported symbols: (a) `readRunState` unmarshals into a test-local
  mirror struct (declare test-local `runState`/`roundRecord` types in this
  file with the same JSON tags — the on-disk schema is a pinned contract,
  so a test-side mirror is legitimate); (b) the `artifactPaths(runDir, 2,
  1).Review` call becomes an inline
  `filepath.Join(runDir, "round-2-review.md")`; (c) receive the duplicated
  `writeFile`/`stringSlicesEqual` helpers per the helper-fallout list
  above. Assertion bodies and test names untouched.
  `internal/perchengine/testmain_test.go` stays.
- **Commit:** `treadle: relocate loop machinery tests to treadleengine`

### Card 4: runner-seam enforcement test and CONSTRAINTS entry

- **Context:**
  - `internal/modelspec/leaf_enforcement_test.go`
  - `internal/treadleengine/run.go`
  - `internal/treadleengine/judge.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:**
  - `internal/treadleengine/seam_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `seam_enforcement_test.go`: an allowlist-only import
  guard in the style of `internal/modelspec/leaf_enforcement_test.go`
  (`TestRunnerSeamInvariant_AllowlistOnly` or equivalent): every production
  (non-test) file in `internal/treadleengine` may import only stdlib,
  `internal/lock`, `internal/logger`, `internal/state`,
  `internal/stencil`, `internal/shuttleengine`, and `gopkg.in/yaml.v3` —
  no `internal/hubgeometry` (the engine is geometry-blind and no moved
  file imports it; extending the allowlist is a deliberate future edit,
  not a pre-grant) — and importing `internal/burlerengine` or any
  `internal/*cli` package fails the test with a message naming the
  offending file and import. `CONSTRAINTS.md`: add a `## Treadle
  Runner-Seam Invariant` section in the file's established shape
  (statement, rationale bullet, **Enforced by** bullet naming the test):
  treadleengine never imports burlerengine or any feature/cli package;
  round runners adapt onto treadle's `RoundRunner` vocabulary in their own
  packages; a type genuinely needed by both is extracted out of burler into
  shared ground, never imported downward.
- **Commit:** `treadle: enforce runner-seam import allowlist and record the invariant`

### Card 5: fake-runner parity tests for the generalized loop

- **Context:**
  - `internal/treadleengine/run.go`
  - `internal/treadleengine/runner.go`
  - `internal/treadleengine/profile.go`
  - `internal/treadleengine/engine.go`
  - `internal/treadleengine/result.go`
  - `internal/treadleengine/state.go`
  - `internal/treadleengine/roundfiles.go`
  - `internal/perchengine/run_test.go`
- **Edits:** none
- **Creates:**
  - `internal/treadleengine/engine_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** New unit tests driving `treadleengine.Engine.Run` with a
  scripted fake `RoundRunner` and fake `Shuttle` (mirroring
  `internal/perchengine/run_test.go`'s fakeBurler/queuedShuttle style,
  adapted to the runner seam). Deliberately scoped to what the perch
  differential suite CANNOT prove — that the loop works against a
  non-burler runner and that the seam contract holds: (a) `AttemptInput`
  population per attempt (round/attempt numbers, `RoundToken` letter
  suffix on retry, artifact paths, hydration lists accumulating across
  rounds including a failed gate file, tuning passthrough of
  Model/Effort/Timeout); (b) retry semantics at the seam — died/timeout
  retried once with identical hydration, second consecutive non-done is a
  name-prefixed hard error, asking triggers triage whose RETRY re-attempts
  and GIVE_UP errors; (c) name-parameterization — an engine constructed
  with a non-perch name (e.g. `"tenter"`) produces errors and the busy
  sentinel wrap carrying that prefix, and `errors.Is(err, ErrBlockBusy)`
  still matches; (d) ladder + gate parity smoke — one hard-cap STUCK case,
  one milestone STOP case, one `GateCommand` convergence case via a fake
  `CommandRunner` observing `GateDir` as its cwd argument; (e) profile
  validation fail-loud cases (empty ProfileHash, non-increasing caps,
  illegal gate mode). Untagged file: no spawning (Test Tier Purity) — the
  fake CommandRunner is an in-process func.
- **Commit:** `treadle: add fake-runner parity tests for the generalized loop`

## Batch Tests

`verify:` runs the four affected package trees: `internal/treadleengine`
(moved + new tests), `internal/perchengine` (the differential suite over the
thin layer), `internal/perchcli` (proves the exported surface did not
shift), and `cmd/lyx` (the repo's enforcement guards — geometry literals,
tier purity, hermetic git, help tree — which the new/moved files must
satisfy, including the new seam-enforcement test's own package). Scope
justification: the batch touches no other package; `go test ./...` full
breadth is deferred to batch 5's final gate.
