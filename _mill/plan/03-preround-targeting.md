# Batch: preround-targeting

```yaml
task: 'Treadle: shared round-loop engine + perch rewrite'
batch: preround-targeting
number: 3
cards: 2
verify: go test ./internal/treadleengine/... ./internal/perchengine/... ./internal/perchcli/... ./cmd/lyx/...
depends-on: [2]
```

## Batch Scope

Adds the optional pre-round targeting capability to treadleengine: a third
judge framing (alongside circling and milestone), gated by a
`treadleengine.Profile` flag, that reads the latest valid handoff before a
round and writes a seed file the round-runner may consume. Perch's adapter
leaves the flag zero-valued, so perch behavior is unchanged — this batch is
pure interface headroom for the future Tenter, proven by treadle-level fake
tests only. Fail-safe throughout: any targeting failure means the round
runs without a seed (Warn), exactly like a judge miss.

## Cards

### Card 10: targeting framing, seed threading, and profile gate

- **Context:**
  - `internal/treadleengine/judge.go`
  - `internal/treadleengine/handoff.go`
  - `internal/treadleengine/judge-circling-template.md`
  - `internal/shuttleengine/spec.go`
  - `_mill/discussion.md`
  - `manifest/designs/hardener.md`
- **Edits:**
  - `internal/treadleengine/profile.go`
  - `internal/treadleengine/runner.go`
  - `internal/treadleengine/run.go`
  - `internal/treadleengine/state.go`
  - `internal/treadleengine/roundfiles.go`
  - `internal/treadleengine/template.go`
- **Creates:**
  - `internal/treadleengine/targeting.go`
  - `internal/treadleengine/targeting-template.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `profile.go`: add `PreRoundTargeting bool` to `treadleengine.Profile`
  (zero value = off; perch's adapter never sets it — no perchengine change
  in this batch).
  `runner.go`: add `SeedPath string` to `AttemptInput`, documented as
  optional — empty when targeting is off or produced no seed; a runner MAY
  ignore it (perch's burler adapter does, per the
  burler-hydration-unchanged decision).
  `roundfiles.go`: `roundArtifactPaths` gains a `Seed` field named
  `round-<token>-seed.md`. The seed is round-scoped, not attempt-scoped:
  the loop resolves the seed path ONCE per round at attempt 1's token
  (`round-3-seed.md`, never `round-3b-seed.md`) before any attempt runs,
  and threads that same path into every attempt's `AttemptInput.SeedPath`
  alongside the equally round-scoped `PriorReviews`/`PriorFixerReports` —
  it is never recomputed per attempt via `artifactPaths(round, attempt)`.
  `state.go`: `roundRecord` gains `SeedPath string
  `json:"seedPath,omitempty"`` (additive); `moveStaleArtifacts` includes
  the seed path.
  `targeting-template.md`: the ONE new template of this task. Same
  single-clean-room-agent shape as the judge templates (HTML comment
  header, no stencil conditionals). Markers: `{{.round}}`,
  `{{.previous_handoff}}`, `{{.seed_path}}`. Framing: "you are a pre-round
  targeting judge — read the handoff's ledger and prose and write a short,
  concrete targeting brief for the NEXT round's runner: which open ledger
  findings to prioritize, what to leave alone". Output: write EXACTLY ONE
  file at `{{.seed_path}}` — free-form prose brief, no frontmatter (it is
  runner input, not machine-parsed).
  `template.go`: embed `targeting-template.md` into a
  `targetingTemplate []byte` var.
  `targeting.go`: `runTargeting(sh Shuttle, name string, round int,
  previousHandoffPath, seedPath, model, effort string) (string, bool)`
  (signature may vary; behavior pinned): fail-safe like `runTriage` — fill
  the template, run a shuttle Spec (`Role: "targeting"`, `OutputFiles:
  [seedPath]`, judge model/effort), require `OutcomeDone` and a
  non-empty seed file on disk; every failure path logs `logger.Warn`
  (label `targeting judge`, name-prefixed, round, cause) and returns
  not-ok — never an error. No verdict parse: the seed is free prose.
  `run.go`: when `p.PreRoundTargeting` is true, run targeting once per
  round BEFORE attempt 1, only when card 7's `latestValidHandoff(rounds)`
  helper yields a handoff — that helper is the designated shared walk (it
  takes only the completed round records, so it composes pre-round where
  no current-round review exists yet); no handoff — e.g. round 1 — skips
  targeting silently, no Warn: nothing to target from. On success, thread the seed path into every
  attempt's `AttemptInput.SeedPath` for that round (retries reuse it) and
  set `record.SeedPath`; on failure, the round runs with an empty
  `SeedPath`. Targeting never affects convergence, the ladder, or judge
  verdicts.
- **Commit:** `treadle: add profile-gated pre-round targeting with fail-safe seed files`

### Card 11: targeting tests and template pins

- **Context:**
  - `internal/treadleengine/targeting.go`
  - `internal/treadleengine/run.go`
  - `internal/treadleengine/runner.go`
  - `internal/treadleengine/profile.go`
- **Edits:**
  - `internal/treadleengine/engine_test.go`
  - `internal/treadleengine/template_test.go`
  - `internal/treadleengine/state_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `engine_test.go`: (a) flag off (zero value) → no targeting Spec is ever
  issued and every `AttemptInput.SeedPath` is empty — the perch-parity
  guarantee; (b) flag on with a valid prior handoff → a targeting Spec runs
  before the round, the fake shuttle writes the seed, `AttemptInput.SeedPath`
  carries it for attempt 1 AND a retry attempt of the same round,
  `state.json`'s record carries `seedPath`; (c) flag on, no handoff yet
  (round 1) → no targeting call, no Warn-required behavior asserted, round
  proceeds; (d) fail-safe — targeting shuttle error / non-done outcome /
  missing seed file → round runs with empty `SeedPath`, block continues,
  no error; (e) stale-artifact move-aside covers a leftover seed file on
  re-run.
  `template_test.go`: fill test supplying the three targeting markers, and
  pinned statements: exactly-one-output-file, the read-the-handoff
  instruction, and the free-form (no frontmatter) output rule.
  `state_test.go`: round-trip the additive `seedPath` field; legacy records
  without it still load.
- **Commit:** `treadle: cover targeting gate, threading, and fail-safe paths`

## Batch Tests

`verify:` scope unchanged from batches 1–2. The perchengine/perchcli suites
run to prove the zero-value flag changes nothing for perch (differential
bar); treadle fakes prove the capability itself. Template pins run under
`internal/treadleengine`.
