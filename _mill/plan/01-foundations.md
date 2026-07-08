# Batch: foundations

```yaml
task: "Build perch - the review gate loop"
batch: "foundations"
number: 1
cards: 3
verify: go test ./internal/burlerengine/ ./internal/hubgeometry/ ./internal/perchengine/ ./internal/configreg/
depends-on: []
```

## Batch Scope

Everything the perch loop depends on but that lives outside the loop itself: the burler seam's
missing `RunDir` passthrough (plus correcting two stale comments the perch design amendment
obsoleted), the Hub-Geometry-owned accessor for the perch runs area under `_lyx`, and the
perch config module (`perch.yaml`: `judge_model`, `judge_effort`, `round_caps`) registered in
`configreg`. External interface for later batches: `burlerengine.Result.RunDir`,
`hubgeometry.PerchRunsDir`, and `perchengine.Config` / `perchengine.LoadConfig` /
`perchengine.ConfigTemplate`.

## Cards

### Card 1: burlerengine RunDir passthrough + stale comment fixes

- **Context:**
  - `internal/shuttleengine/run.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/verdict.go`
  - `internal/burlerengine/doc.go`
  - `internal/burlerengine/engine_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `RunDir string` field to `burlerengine.Result` (engine.go),
  documented as the 1:1 passthrough of `shuttleengine.Result.RunDir` — the kept shuttle run
  dir a caller surfaces when a round dies/times out. Populate it in `Engine.Run` where
  `SessionID`/`StrandGUID` are already copied from `shuttleResult` (before the non-done early
  return, so non-done outcomes carry it too). Correct the stale design references from before
  the verdict-judge amendment (see `_mill/discussion.md` Decision "Verdict-judge model"):
  in verdict.go, the `Finding` doc comment and the `ParseReview` doc line calling finding IDs
  "perch's future cycle-detection keys" — reword to say IDs stay unique/fail-loud for
  cross-round hydration and audit, and that perch judges progress holistically via a verdict
  judge, not key-based cycle detection; in doc.go, the same claim inside "# What a round
  returns" AND the package-intro paragraph's "the loop, the cap, cycle detection" description
  of perch (reword to name the loop, the milestone cap ladder, and the progress judge). Extend the existing fake-shuttle `Engine.Run` test(s) in engine_test.go to assert
  the `RunDir` passthrough for both a done and a non-done outcome.
- **Commit:** `burler: surface shuttle RunDir on Result; retire stale cycle-key comments`

### Card 2: hubgeometry.PerchRunsDir accessor

- **Context:**
  - `internal/hubgeometry/enforcement_test.go`
- **Edits:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/hubgeometry/hubgeometry_unit_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func PerchRunsDir(baseDir string) string` to hubgeometry.go,
  returning `filepath.Join(baseDir, LyxDirName, "perch")`, placed alongside
  `ConfigDir`/`ConfigFile` with a doc comment stating: this is the base for perch block run
  dirs (`<PerchRunsDir>/<run-id>/`); it lives under `_lyx` so run artifacts are weft-synced
  via the host `_lyx` junction; per the Hub Geometry Invariant no other package may construct
  this path. Add a unit test next to the existing `ConfigDir`/`ConfigFile` tests in
  hubgeometry_unit_test.go asserting the joined shape. No enforcement pinned-set update is
  needed — `TestEnforcement_GeometryLiterals` allowlists `internal/hubgeometry` itself and
  `"perch"` is not a pinned geometry token (confirm by reading enforcement_test.go, listed as
  Context).
- **Commit:** `hubgeometry: add PerchRunsDir accessor for perch block run dirs`

### Card 3: perch config module + configreg registration

- **Context:**
  - `internal/muxengine/config.go`
  - `internal/muxengine/config_test.go`
  - `internal/shuttleengine/config.go`
  - `internal/shuttleengine/template.go`
  - `internal/shuttleengine/template.yaml`
  - `internal/configengine/config.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/configreg/configreg.go`
  - `internal/configreg/configreg_test.go`
- **Creates:**
  - `internal/perchengine/config.go`
  - `internal/perchengine/template.go`
  - `internal/perchengine/template.yaml`
  - `internal/perchengine/config_test.go`
  - `internal/perchengine/doc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create package `perchengine` with its config module, copying the
  muxengine/shuttleengine pattern exactly. config.go: `type Config struct` with fields
  `JudgeModel string` (yaml `judge_model`), `JudgeEffort string` (yaml `judge_effort`),
  `RoundCaps []int` (yaml `round_caps`); `func LoadConfig(baseDir, module string) (Config,
  error)` delegating to `configengine.Load(baseDir, module, []byte(ConfigTemplate()))` with
  the same not-initialized rewrap and a `"unmarshal perch config"` error wrap. template.go:
  `//go:embed template.yaml` + `func ConfigTemplate() string`. template.yaml: commented
  template with defaults `judge_model: haiku`, `judge_effort: ""` (empty = provider default),
  `round_caps: [5, 8, 10]`, each key documented (judge model/effort serve both the progress
  judge and the asking-triage call; round_caps is the default milestone ladder — strictly
  increasing, last entry is the hard cap — overridable per profile). doc.go: a concise
  package header naming perch as the deterministic gate loop over burler rounds (batch 5
  expands it into the full durable design header when `docs/modules/perch.md` is deleted).
  config_test.go: mirror the shape of muxengine's config test (template parses into Config;
  defaults match the documented values; not-initialized error path). Register the module in
  `configreg.Modules()` — insert `{"perch", perchengine.ConfigTemplate}` between `mux` and
  `shuttle` (alphabetical source order) with the matching import — and add `"perch"` at the
  same position in configreg_test.go's pinned `want` list.
- **Commit:** `perch: add config module (judge_model, judge_effort, round_caps) and register in configreg`

## Batch Tests

`verify:` runs the four touched packages' suites: burlerengine (RunDir passthrough +
existing round tests), hubgeometry (new accessor test + both enforcement guards, proving no
pinned-set update was needed), perchengine (new config tests), configreg (pinned Names list
including perch). No LLM, no fixtures beyond what the existing suites already use.
