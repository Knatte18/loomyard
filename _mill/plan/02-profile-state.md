# Batch: profile-state

```yaml
task: "Build perch - the review gate loop"
batch: "profile-state"
number: 2
cards: 3
verify: go test ./internal/perchengine/
depends-on: [1]
```

## Batch Scope

The deterministic data layer of the engine: the perch `Profile` (burler content fields +
perch-owned gate/caps/tuning keys) with fail-loud validation and default resolution, the
`Result` contract (`APPROVED | STUCK | PAUSED` + stuck reason + per-round summaries), the
per-round artifact naming and `burlerengine.Profile` builder, and the on-disk block state
(`state.json` via `internal/state`) with run-id/profile-hash identity, resume classification,
stale-partial handling, and the pause flag file. External interface for batch 3/4:
`Profile`, `Gate`, `GateMode`, `Result`, `Outcome`, `StuckReason`, `RoundSummary`,
`roundArtifactPaths`, `buildRoundProfile`, `runState`, `loadOrInitState`, `saveState`,
`ProfileHash`, `DeriveRunID`, `PauseFlagPath`.

## Cards

### Card 4: Profile, Gate, defaults, validate

- **Context:**
  - `internal/burlerengine/profile.go`
  - `internal/perchengine/config.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/perchengine/profile.go`
  - `internal/perchengine/profile_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** profile.go defines the perch-side content contract. `type GateMode string`
  with constants `GateLLMVerdict = "llm-verdict"`, `GateCommand = "command"`, `GateBoth =
  "both"`. `type Gate struct { Mode GateMode; Command []string; Timeout time.Duration }`.
  `type Profile struct` embedding the burler content fields by value — `Target
  burlerengine.FileSet`, `Fasit burlerengine.FileSet`, `Rubric string`, `FixScope
  burlerengine.FixScope`, `ToolUse bool`, `ClusterN int` — plus perch-owned fields: `Gate
  Gate`, `RoundCaps []int`, `JudgeModel string`, `JudgeEffort string`, `Model string`,
  `Effort string` (burler round tuning, uniform across rounds per the discussion Decision
  "Run-tuning v1"). Method `func (p *Profile) validate(cfg Config) error`, fail-loud with
  `"perch: "` prefixes, checking ONLY perch-owned fields (the burler content fields are
  validated by `burlerengine.Profile.validate` inside the first round's `Engine.Run` — state
  this in a comment): resolve defaults first — empty `RoundCaps` takes `cfg.RoundCaps`, still
  empty takes package constant `defaultRoundCaps = []int{5, 8, 10}`; empty `JudgeModel` takes
  `cfg.JudgeModel`, still empty takes `defaultJudgeModel = "haiku"`; empty `JudgeEffort`
  takes `cfg.JudgeEffort`; zero `Gate.Timeout` takes `defaultGateTimeout = 10 *
  time.Minute`. Then validate: `RoundCaps` entries all >= 1 and strictly increasing;
  `Gate.Mode` must be one of the three constants (no silent default, mirroring FixScope's
  posture in burlerengine); `GateCommand`/`GateBoth` require non-empty `Gate.Command`;
  `GateLLMVerdict` requires empty `Gate.Command`; negative `Gate.Timeout` rejected.
  profile_test.go: table-driven validate tests covering every rule, the three-level default
  resolution chains (profile > Config > built-in) for RoundCaps and JudgeModel, and the
  one-element-array (plain hard cap) acceptance.
- **Commit:** `perch: add Profile with gate/caps validation and default resolution`

### Card 5: Result contract + round artifact paths + round-profile builder

- **Context:**
  - `internal/burlerengine/profile.go`
  - `internal/burlerengine/verdict.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/perchengine/result.go`
  - `internal/perchengine/roundfiles.go`
  - `internal/perchengine/roundfiles_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** result.go: `type Outcome string` with `OutcomeApproved = "APPROVED"`,
  `OutcomeStuck = "STUCK"`, `OutcomePaused = "PAUSED"`; `type StuckReason string` with
  `StuckHardCap = "hard-cap"`, `StuckMilestoneStop = "milestone-stop"`, `StuckCircling =
  "circling"`; `type RoundSummary struct { Round int; Attempts int; Verdict
  burlerengine.Verdict; BlockingCount int; ReviewPath, FixerReportPath, JudgePath, GatePath
  string; JudgeVerdict string; GatePassed *bool }` (empty/nil fields mean "did not occur
  this round"); `type Result struct { Outcome Outcome; StuckReason StuckReason; RoundsRun
  int; Rounds []RoundSummary }` — PAUSED is an operational exit (resumable, not judged);
  StuckReason is set only when Outcome is OutcomeStuck. roundfiles.go: `func
  roundToken(round, attempt int) string` — `"3"` for attempt 1, `"3b"` for attempt 2, `"3c"`
  for attempt 3...; `type roundArtifactPaths struct { Review, FixerReport, Judge, Gate,
  Triage string }` and `func artifactPaths(runDir string, round, attempt int)
  roundArtifactPaths` producing `round-<token>-review.md`, `round-<token>-fixer-report.md`,
  `round-<token>-judge.md`, `round-<token>-gate.md`, `round-<token>-triage.md` inside runDir;
  `func buildRoundProfile(p Profile, paths roundArtifactPaths, priorReviews,
  priorFixerReports []string) burlerengine.Profile` mapping the content fields 1:1 and
  setting `ReviewPath`/`FixerReportPath` from paths and
  `PriorReviews`/`PriorFixerReports` from the accumulated prior lists (prior gate-output
  files are appended to priorReviews by the loop — the builder just passes the slices
  through). roundfiles_test.go: token/path shape table and a buildRoundProfile field-mapping
  test (every content field mapped, loop-owned fields set, operator-owned prior lists never
  invented by the builder).
- **Commit:** `perch: add Result contract, round artifact naming, and round-profile builder`

### Card 6: block state on disk — identity, resume classification, pause flag

- **Context:**
  - `internal/state/state.go`
  - `internal/perchengine/result.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/perchengine/state.go`
  - `internal/perchengine/state_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** state.go persists block progress via `state.WriteJSON` / `state.ReadJSON`
  at `<runDir>/state.json` with lock path `<runDir>/state.json.lock`. `type runState struct
  { ProfileHash string; RoundCaps []int; Rounds []roundRecord; Outcome string; StuckReason
  string }` where empty `Outcome` means in-progress; `type roundRecord struct { Round int;
  Attempts int; ShuttleOutcome string; Verdict string; BlockingCount int; ReviewPath,
  FixerReportPath, JudgePath, GatePath, TriagePath string; JudgeVerdict string; GatePassed
  *bool; SessionID string }`. `func ProfileHash(p Profile) (string, error)` — sha256 hex of
  the canonical `json.Marshal` of the (already default-resolved) Profile; works identically
  for a CLI-decoded profile and a loom-supplied struct. `func DeriveRunID(profilePath string,
  hash string) string` — `<sanitized-basename-without-ext>-<first-8-hex-of-hash>` (sanitize
  to lowercase alphanumerics and dashes). `func loadOrInitState(runDir string, hash string,
  caps []int) (runState, resumeInfo, error)` classifying: no state.json → fresh (write
  initial state); unfinished state with matching hash → resume at `len(Rounds)+1` (or re-run
  the last round when its record is marked incomplete — a round record is appended only on
  completion, so an interrupted round simply has no record); unfinished with hash mismatch →
  fail-loud error `"perch: run dir %s was started with a different profile; use a fresh
  --run-id"`; terminal Outcome non-empty → fail-loud error `"perch: this block already
  finished (%s)"`. `func moveStaleArtifacts(runDir string, round, attempt int) error` —
  renames any existing artifact file for the incoming round/attempt token by appending
  `.stale` (with a numeric suffix if `.stale` already exists), so shuttle's
  no-pre-existing-output-file rule always holds on resume. `func saveState(runDir string, s
  runState) error`. `const PauseFlagName = "pause"` and `func PauseFlagPath(runDir string)
  string` (exported — perchcli's pause verb and run wiring both use it); `func
  clearPauseFlag(runDir string) error` removing the flag if present (called at Run entry so a
  resumed block does not instantly re-pause). state_test.go: table-driven classification
  tests (fresh / resume / hash mismatch / terminal), stale-artifact renaming incl. the
  double-`.stale` collision case, and a WriteJSON/ReadJSON round-trip of runState.
- **Commit:** `perch: add block state persistence, run identity, resume classification, pause flag`

## Batch Tests

`verify:` runs `go test ./internal/perchengine/`: validate/default tables (card 4), artifact
naming + builder mapping (card 5), state classification/stale/pause helpers (card 6). All
pure Go on temp dirs — no LLM, no git, no psmux.
