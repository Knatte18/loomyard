# Discussion: Treadle: shared round-loop engine + perch rewrite

```yaml
task: 'Treadle: shared round-loop engine + perch rewrite'
slug: treadle
status: discussing
parent: main
```

## Problem

`perch` (`internal/perchengine` + `internal/perchcli`) is the shipped, tested round-loop
engine: it spawns a fresh `burlerengine` round per iteration, gates convergence
(llm-verdict / command / both), runs an ephemeral progress judge against a milestone-capped
round ladder, and persists per-round state for crash/pause resume. The future `Tenter`
module (behavior-review, see `manifest/designs/hardener.md`) needs exactly this loop with a
different round-runner — but perch hardwires "spawn a burlerengine round" instead of
exposing a seam. Additionally, perch's judge today reads **every prior round's** review
file on each call — unbounded O(N) context growth as rounds accumulate.

This task builds **Treadle** — the generalized round-loop engine (pluggable round-runner,
judge-maintained handoff, optional pre-round targeting) — and rewrites perch onto it **in
the same task**. Perch's existing behavior and tests are the differential test: if perch
still passes with identical behavior, Treadle has everything perch needs. Extracting the
engine without immediately proving its one real consumer would leave an untested
abstraction (the design docs' stated reason for combining; same pattern `fabric` used).
Full design rationale: `manifest/designs/treadle.md` (authoritative for this task),
`manifest/designs/hardener.md` (Tenter's needs — informs the interface, Tenter itself is
NOT built here), `manifest/designs/shed.md` (context only — the separate outer-FSM engine).

Why now: Treadle is Planned on `manifest/roadmap.md` as a prerequisite-shaped shared
engine; the handoff optimization is a real improvement to shipped perch behavior
independent of whether Tenter is ever built.

## Scope

**In:**

- New `internal/treadleengine` package: the generalized round loop (round loop, judge +
  triage machinery, gate execution, `state.json` persistence, pause seam, run-dir lock),
  a `RoundRunner` interface, the judge-maintained handoff/ledger, and profile-gated
  pre-round targeting.
- Rewrite `internal/perchengine` as a thin configuration layer over `treadleengine`
  (burlerengine adapted into the `RoundRunner` seam). Public Go API of `perchengine`
  stays byte-identical, so `perchcli`'s engine-facing code needs no changes — but
  `perchcli`'s profile-parsing schema (yaml tags in `run.go`), embedded help text, and
  test fixtures DO change as part of the model-spec migration below.
- Perch adopts the handoff: the judge's read-set becomes {handoff + reviews since last
  valid handoff} instead of {every prior review}.
- Model-spec migration of perch's operator config surface (a deliberate, fail-loud
  breaking change to config files — see Decisions): `perch.yaml` `judge_model` becomes a
  model-spec string and `judge_effort` is removed; profile keys `judge-model:`/`model:`
  become model-spec strings and `judge-effort:`/`effort:` are removed.
- Docs in the same commit(s): module docs, `docs/overview.md` module table if it changes,
  `manifest/roadmap.md` Planned→Done for this item, `manifest/designs/treadle.md`
  lifecycle per the Documentation Lifecycle (absorbed into the package header +
  overview.md on landing), `SANDBOX-PERCH-SUITE.md` profile updates, perchcli help text.

**Out:**

- `Tenter` / `Hardener` — Someday; only the *interface headroom* for them is in scope
  (round-runner seam, targeting capability), never their implementation.
- `Shed` — separate, independent task.
- Changing burler-round hydration (what reviewers read) — stays all prior
  reviews/fixer-reports/failed-gate outputs, unchanged.
- Any `treadle.yaml` config file — Treadle takes resolved data, owns no config file.
- Migrating other modules' configs to model-spec notation — only perch's surface moves.
- perch CLI verbs/flags/JSON output — unchanged.

## Decisions

### package-layout

- Decision: New `internal/treadleengine` package owns the generalized loop.
  `internal/perchengine` keeps its full exported surface (`Profile`, `Config`, `Engine`,
  `New`, `Run`, `Options`, `ErrBlockBusy`, `ProfileHash`, `DeriveRunID`, `ValidRunID`,
  `TerminalOutcome`, `PauseFlagPath`, `PauseFlagName`, `LoadConfig`, `Gate`, `GateMode`
  constants, `Outcome`/`StuckReason` vocabularies, `Result`/`RoundSummary`) as a thin
  layer that validates/resolves perch config and adapts burlerengine into Treadle.
- Rationale: matches the repo's `<module>engine` naming convention; keeps `perchcli` and
  every existing import untouched; the differential test stays meaningful.
- Rejected: renaming perchengine into treadleengine (rewires perchcli, widens diff);
  a sub-package under perchengine (misnames a shared engine Tenter will use).

### round-runner-seam

- Decision: The seam is **attempt-level**. `treadleengine` defines a `RoundRunner`
  interface that runs ONE attempt and reports: a shuttle-style outcome
  (done/asking/died/timeout), a generic verdict (approved/blocking), blocking-findings
  count, review path, fixer-report path, session id, last-assistant-message, and the
  runner's kept run dir. Treadle owns the generic machinery around it: the two-attempt
  retry, asking-triage, stale-artifact move-aside, round/attempt token naming
  (`roundToken`), artifact path derivation, and prior-hydration list assembly
  (`collectPriorHydration` semantics unchanged, including failed-gate feed-forward).
  Per-attempt input to the runner carries: run dir, round, attempt, artifact paths
  (review/fixer-report), prior reviews, prior fixer-reports, optional seed-prompt path
  (targeting), and tuning (model, effort, timeout).
- Rationale: retry/triage is generic infrastructure every future runner (Tenter) needs;
  it sits in the loop today (`run.go: runRound`) and moving it into runners would
  duplicate it and drag the triage judge call (Treadle's own fail-safe machinery) along.
- Rejected: round-level seam where the runner owns retries.

### no-burler-import

- Decision: `treadleengine` does NOT import `burlerengine`. It defines its own
  vocabulary (verdict enum, attempt-result struct). `perchengine` adapts
  `burlerengine.Result` onto it. If a type is ever genuinely shared, it gets
  **extracted out of burler** into shared ground — never imported downward.
- Rationale: the extraction exists to decouple the engine from one runner's types.
- Rejected: reusing `burlerengine.Verdict`/`Finding` in the interface.

### perch-adopts-handoff

- Decision: perch itself adopts the handoff — the judge's read-set changes from {all
  prior reviews + latest} to {latest valid handoff + reviews of rounds not covered by
  it}. Existing tests that pin the judge-input seam (all-reviews) are updated as part of
  the rewrite; external behavior/CLI unchanged.
- Rationale: this is the design doc's stated point — a genuine efficiency fix to shipped
  perch (kills O(N) judge context growth). Also the operator's stated reasoning: a
  long-lived accumulating LLM context buys nothing here — Claude's ~5-minute cache TTL
  is shorter than a typical burler round, so cross-round context caching never pays;
  a compact handoff + fresh spawn per call is strictly better.
- Rejected: opt-in capability perch doesn't use (ships the handoff untested by any real
  consumer — the exact "untested abstraction" this task exists to avoid).

### handoff-on-disk

- Decision: One handoff file **per judge call**: `round-<token>-handoff.md`, latest-wins.
  The handoff is produced by the **same judge call** that renders the verdict — no
  separate handoff spawn: the circling and milestone templates are both extended with
  handoff-maintenance instructions, a previous-handoff input marker, and a second
  output file (the handoff path joins the verdict path in the shuttle Spec's
  OutputFiles). The judge call reads the previous handoff file (path supplied by the
  loop from state.json) and writes a fresh one at this round's path. state.json's round
  record gains an optional `handoffPath` field. Pre-round targeting stays a separate
  template/call (see pre-round-targeting).
- Rationale: "edited in place" in the design means *bounded, not appended* — per-round
  files satisfy boundedness while reusing the existing crash machinery verbatim:
  stale-artifact move-aside, per-round records, no torn in-place writes, and a full
  audit trail of ledger evolution.
- Rejected: a single literal `handoff.md` edited in place (crash between handoff rewrite
  and state save leaves an inconsistent pair; the stale-move machinery cannot apply to a
  file that must survive across rounds).

### handoff-format-and-ledger

- Decision: One markdown file per handoff: strict YAML frontmatter carrying the
  **lossless finding-identity ledger** (per finding: identity/title, rounds-seen list,
  status open/resolved) plus a `covers_rounds` list naming exactly which rounds' reviews
  the handoff has absorbed; below the frontmatter, a **distilled prose narrative**.
  Go parses the frontmatter fail-loud at read (mirroring `ParseJudgeVerdict`); the
  carry-forward rule (every previous ledger entry must reappear, as open or resolved,
  never dropped) is enforced at prompt level.
- Rationale: mirrors the shipped judge-verdict pattern (strict YAML over prose); honors
  the design doc's hard constraint — "distill the prose, but keep the key-ledger
  lossless" — a prose-only summary that quietly drops a recurring finding breaks
  circling-detection silently. Go-side *semantic* key matching stays out: doc.go
  documents key-canonicalization in Go as deliberately rejected; the judge stays the
  holistic decider.
- Rejected: Go-maintained ledger (reintroduces rejected canonical-key machinery);
  free-form handoff (violates the lossless-ledger constraint outright).

### handoff-failure-fallback

- Decision: The handoff is an optimization with a deterministic fallback, never a
  correctness dependency. A judge call whose handoff output is missing or unparseable
  logs a `logger.Warn` (the judge's existing fail-safe posture, never an error, never
  STUCK). The NEXT judge call computes its read-set as {last **valid** handoff +
  reviews of all rounds not in its `covers_rounds`}; with no valid handoff at all it
  degrades to exactly today's all-reviews behavior. The `covers_rounds` field is also
  what closes the judge-gap hole: rounds where no judge runs (round 1; approved-verdict
  rounds; rounds immediately after an approved round) still get their reviews fed to the
  next judge call, because they are not in any handoff's coverage.
- Rationale: preserves the shipped fail-safe posture (a judge infrastructure failure is
  NEVER an engine error); keeps resume of old-format blocks working with zero migration.
- Rejected: failing the round on handoff-write failure.

### pre-round-targeting

- Decision: A third judge framing (alongside circling and milestone), profile-gated,
  using the same `runJudgeCall`-shaped machinery and fail-safe posture: read the latest
  handoff, decide what to target, write `round-<token>-seed.md`. The capability flag
  lives on **Treadle's own per-block input struct** (`treadleengine`'s profile type — a
  distinct type from perch's byte-identical `perchengine.Profile`; exact field name is
  mill-plan's call, e.g. a `PreRoundTargeting bool`); perch's adapter leaves it
  zero-valued, so perch never exercises it. The per-attempt runner
  input carries the optional seed path. Fail-safe: on any targeting failure the round
  runs without a seed (Warn logged), exactly like a judge miss. state.json's round
  record gains an optional `seedPath` field.
- Rationale: general capability, not a Tenter special case (design doc's requirement);
  separate call keeps "verdict on the past" and "targeting the future" prompts clean;
  the post-round judge doesn't run every round anyway, while targeting is per-round.
- Rejected: folding targeting into the post-round judge call as a mode flag.

### perch-api-and-identity-stability

- Decision: `perchengine`'s exported Go surface stays byte-identical — same types, same
  fields, same JSON encoding of `Profile`. `state.json` schema changes are strictly
  additive (`handoffPath`, `seedPath` — omitempty); a resumed old block's records simply
  lack handoff coverage, which the fallback rule already handles (judge reads all
  uncovered reviews). **Resume across the config migration is guaranteed only when the
  migrated config resolves to the identical `Profile` field values the block was started
  with.** `ProfileHash` marshals the Profile as supplied (`state.go:95`), and the
  migrated CLI unpacks the *resolved* effort into `Profile.JudgeEffort`/`Effort` at
  block load — so a block started under the old keys with empty effort ("provider
  default") hashes differently once a seeded `models.yaml` default effort applies, and
  `loadOrInitState` refuses the resume with the existing fail-loud "started with a
  different profile; use a fresh `--run-id`" error (`state.go:195`). That fail-loud
  outcome is accepted, not worked around: in-flight blocks at migration time are rare
  and short-lived, the error is loud, and the recovery (fresh `--run-id`) is trivial.
  An old-format `perch.yaml`/profile also fails strict validation before any resume
  starts — same accepted posture. There is no way to re-express "provider default"
  effort once a registry default is seeded (`modelspec` rejects an empty bracket
  value); that is fine — the registry default IS loomyard's chosen default.
- Rationale: the differential-test bar for the Go API; no state-version machinery
  exists today, the fallback covers handoff-coverage, and the fail-loud profile-hash
  mismatch already handles the migration edge safely.
- Rejected: letting the API drift; a state-version bump that refuses cross-rewrite
  resume; hashing pre-resolution spec strings to preserve resume (changes ProfileHash
  semantics for every caller and still cannot map old split-key hashes); a one-off
  state migration/rehash mechanism (YAGNI — machinery for a dev-tool edge the loud
  error handles).

### config-and-modelspec-migration

- Decision: Treadle owns no config file and no model-spec parsing — it receives resolved
  plain data ((model, effort) string pairs and profile/settings structs) and maps them
  1:1 onto `shuttleengine.Spec`, exactly as `modelspec`'s package doc prescribes for
  spawn sites. `perch.yaml` keeps being owned/loaded by `perchengine.LoadConfig`.
  **Perch's operator file surface migrates to model-spec notation in this task** (fixing
  an acknowledged oversight — perch predates modelspec; loom/webster/builder already use
  it): `judge_model: <model-spec>` (e.g. `sonnet[effort=medium]`), `judge_effort`
  removed; profile `judge-model:`/`model:` become model-spec strings, `judge-effort:`/
  `effort:` removed. Grammar is validated at load via `modelspec.Parse`; resolution runs
  once at block creation via `modelspec.LoadRegistry(baseDir)` + `Registry.Resolve`,
  then unpacks into the unchanged `Profile.Model/Effort/JudgeModel/JudgeEffort` fields.
  A bare alias (e.g. `sonnet`) picks up the operator-configured default effort from the
  seeded `models.yaml` registry entry (`Defaults: {effort: ...}`); built-ins are
  deliberately default-free, so loomyard's own defaults override the provider's in one
  place, never baked into Go. Bracket params other than `effort` fail loud — as an
  **explicit perch-layer check on `Resolved.Params` after resolution** (only `effort`
  accepted), NOT a modelspec guarantee: `version` is in `modelspec`'s known-params set
  (`modelspec.go:116`), so `Parse` accepts `sonnet[version=x]` and `Resolve` merges it
  silently; perch (which has no Version field to thread it into) must reject it itself
  rather than drop it. Old config files carrying the removed keys fail strict
  validation with a clear message — a deliberate, fail-loud breaking config change; no
  deprecated-fallback dual schema.
- Rationale: operator decision — the split keys were an oversight; the repo's
  established, pinned contract is `docs/reference/model-spec.md`. The Profile struct is
  unchanged, and a migrated config that resolves to the same field values keeps the
  same `ProfileHash`; where resolution changes the values (seeded default effort), the
  resume consequence is accepted fail-loud — see perch-api-and-identity-stability.
- Rejected: a shared `treadle.yaml` (YAGNI); Treadle resolving model-specs internally
  (needless registry/baseDir dependency + split→string→split round-trip); deprecated
  fallback keys (contradicts strict fail-loud validation); deferring the migration to a
  separate task (superseded by operator's call).

### burler-hydration-unchanged

- Decision: What the round-runner (burler) reads stays exactly as shipped: all prior
  reviews + prior fixer-reports + failed-gate outputs (`collectPriorHydration`
  semantics, including the pass-gate-never-fed rule). The handoff bounds only the
  JUDGE's read-set.
- Rationale: design doc scopes the handoff to the judge; changing reviewer inputs is a
  real review-quality behavior change that would contradict the acceptance bar. If ever
  wanted, that is a later, separate decision.
- Rejected: feeding rounds {handoff + latest}.

## Technical context

- `internal/perchengine/doc.go` — the authoritative shipped contract; every invariant it
  documents must survive: milestone-ladder exactness (rungs judge-gated, last entry
  unconditional hard cap, one-element ladder degenerates), the three `GateMode`s and
  their convergence semantics, judge fail-safe posture (UNCERTAIN normal; any
  infrastructure failure degrades to progressing/CONTINUE with Warn, never persisted as
  a real verdict), burler-verdict-based judge triggers (never round 1, never after an
  approved round for circling; milestone replaces circling on rungs; approved rounds run
  no judge), pause seam checked only at round boundaries + pause-flag clearing rules,
  run-dir locking (`run.lock`, `ErrBlockBusy` sentinel semantics), weft-blindness and
  geometry-blindness (caller-supplied absolute runDir; layout only for gate cwd),
  `profile > perch.yaml > built-in` resolution applied once at block creation, identity
  hash over the profile as supplied, resolved ladder stamped into state.json.
- The loop to generalize lives in `internal/perchengine/run.go` (round loop, stuck
  ladder, `runRound` retry/triage, hydration collectors), `engine.go` (Burler/
  CommandRunner seams, Options), `judge.go` (Shuttle seam, `runJudgeCall`, `runTriage` —
  all fail-safe), `judgeverdict.go` (strict YAML-frontmatter parsing — the pattern the
  handoff parser mirrors), `gate.go` (exec runner incl. timeout/orphaned-pipe handling),
  `state.go` (locked state.json I/O, fresh/resume/refuse classification,
  `moveStaleArtifacts`, pause flag), `roundfiles.go` (roundToken, artifact paths,
  `buildRoundProfile`), `profile.go` (validate + default resolution), `result.go`,
  `config.go`. Judge/triage prompt templates: `judge-circling-template.md`,
  `judge-milestone-template.md`, `triage-template.md` (via `internal/stencil`).
- `internal/burlerengine`: `Result` carries Outcome/Verdict/Findings/paths/SessionID/
  LastAssistantMessage/RunDir — the material the perch-side adapter maps onto Treadle's
  attempt-result type. `Profile.PriorReviews`/`PriorFixerReports` entries must exist on
  disk (fail-loud) — hydration lists Treadle assembles must respect that.
- `internal/perchcli` consumes exactly: `Config, DeriveRunID, Engine, ErrBlockBusy,
  Gate, GateMode, LoadConfig, New, Options, PauseFlagPath, Profile, ProfileHash,
  TerminalOutcome, ValidRunID` — the surface that must not shift. Its `run.go` parses
  profile YAML (currently split `judge-model`/`judge-effort`/`model`/`effort` keys — the
  file-schema migration lands here) and embeds help text with profile examples
  (help-accuracy review obligation).
- `internal/modelspec`: `Parse` (grammar), `LoadRegistry(baseDir)` (models.yaml overlay
  over default-free builtins sonnet/opus/haiku/fable), `Registry.Resolve` (bracket param
  > registry default), unpack `Resolved.Params["effort"]` → `Spec.Effort`. Leaf package,
  importable without cycles. Loom's `discussion.go`/`config.go` show the established
  Parse-at-load / Resolve-at-use pattern.
- `internal/shuttleengine.Spec` has separate `Model`/`Effort` fields — Treadle's plain
  (model, effort) pairs map straight onto it. Judge/triage calls go through a
  package-local `Shuttle` interface (`Run(Spec) (Result, error)`) — Treadle gets the
  equivalent seam.
- Existing perchengine tests pin internal seams that legitimately change: judge
  `PriorReviews` = all-reviews assertions (`run_test.go:240`, `judge_test.go`), and
  they pin burler hydration incl. failed-gate feed-forward (`run_test.go:665,806`) —
  the latter must keep passing unchanged.
- Templates move/generalize: circling/milestone/triage templates become Treadle's. The
  circling and milestone templates are **extended in place** with handoff-maintenance
  instructions (previous-handoff input marker, handoff output path — same call, second
  output file); exactly one NEW template is added, for pre-round targeting. Template
  content changes affecting pinned statements have tests (`template_test.go`) — the
  extended circling/milestone templates get updated/new pinned statements (incl. the
  ledger carry-forward rule) and the targeting template gets its own.
- `perchengine/template.yaml` is perch.yaml's strict config template (judge_model,
  judge_effort, round_caps) — the `judge_effort` line is removed in the migration, and
  the strict-template/`configreg` registration update is **required** (perch.yaml stays
  configreg-validated; strict validation is what makes old files fail loud).

## Constraints

From `CONSTRAINTS.md` (all apply; the machine-enforced ones fail `go test`):

- **Hub Geometry Invariant** — `treadleengine` must stay geometry-blind like perchengine:
  caller-supplied absolute runDir, no `_lyx` path construction, no geometry tokens.
- **CLI / Cobra Invariant** — `perchcli`'s seam is untouched, but help accuracy is a
  review obligation: the model-spec migration changes profile/config examples in
  `Short`/`Long` text and they must match the new schema. Errors stay on the JSON
  envelope.
- **Modelspec Leaf Invariant** — perchengine may import modelspec (allowed direction);
  never the reverse.
- **lyxtest Leaf / Test Tier Purity / Hermetic Git** — new treadleengine tests follow the
  existing tier discipline (untagged tests spawn nothing; git-spawning test packages need
  `lyxtest.HermeticGitEnv` TestMain).
- **Shuttle Provider-Seam Invariant** — Treadle's judge/triage calls stay
  provider-invariant through the Shuttle seam; no Claude specifics.
- **Weft Git Invariant** — Treadle (like perchengine today) never touches weft git;
  block-exit weft commits remain the loop owner's job (perchcli).
- **Sandbox Suite Coverage** — perch stays covered by `SANDBOX-PERCH-SUITE.md`; its
  profiles must migrate to the new model-spec keys. No new CLI module is added, so no
  new coverage entry.
- **Review Round Invariant** — round-discipline statements in prompt templates keep
  their pinned tests passing.
- **Documentation Lifecycle** — on landing: package header for treadleengine,
  `docs/overview.md` updates, `manifest/designs/treadle.md` absorbed/deleted per
  lifecycle, roadmap Planned→Done. New cross-cutting invariants (if any emerge — e.g. a
  treadleengine leaf/seam rule) recorded in `CONSTRAINTS.md` in the same commit.

## Testing

- **Differential bar (the acceptance test):** the existing `perchengine` and `perchcli`
  test suites pass unchanged, with two knowing exceptions: (1) tests pinning the judge's
  internal read-set seam (all-prior-reviews) update to the handoff contract; (2) tests/
  fixtures carrying the old split model/effort config keys update to model-spec strings.
  Burler-hydration pins, ladder exactness, gate modes, pause, locking, resume, and CLI
  behavior tests must pass without modification.
- **New `treadleengine` unit tests** (fake RoundRunner + fake Shuttle, mirroring the
  existing fake-Burler/fake-Shuttle style): loop parity (ladder rungs/hard cap, three
  gate modes, pause at round boundary, run-lock busy, resume classification,
  stale-artifact move-aside), retry + asking-triage paths, handoff lifecycle (written on
  judge rounds, previous-handoff threading, `covers_rounds` correctness across
  judge-skipped rounds, fail-loud parse, fallback to uncovered-reviews read-set,
  no-valid-handoff degrades to all-reviews), pre-round targeting (profile-gated, seed
  path threaded to runner, fail-safe on targeting failure), additive state.json fields
  and old-state resume.
- **TDD candidates:** the handoff frontmatter parser (fail-loud, mirrors
  `ParseJudgeVerdict` — pure function, ideal TDD); `covers_rounds` read-set computation
  (pure function over round records); model-spec unpacking/validation in the perch
  config/profile layer (including bare-alias default-effort resolution and the
  unsupported-bracket-param rejection).
- **Template tests:** the extended circling/milestone templates and the new targeting
  template get pinned-statement tests in the style of `template_test.go` where they
  encode discipline (e.g. the ledger carry-forward rule).
- **Integration/smoke:** existing tagged perch integration tests remain the end-to-end
  proof; extend only where the handoff artifact needs an end-to-end existence check.

## Q&A log

- **Q:** Package layout for Treadle? **A:** New `internal/treadleengine`; perchengine
  keeps its full public API as a thin layer; perchcli untouched.
- **Q:** Where does the round-runner seam cut? **A:** Attempt-level; Treadle owns
  retry/triage/stale-move/token naming.
- **Q:** Does treadleengine import burlerengine? **A:** No — own vocabulary; if
  something is needed from burler, extract it out of burler rather than import it.
- **Q:** Does perch adopt the handoff? **A:** Yes. Operator rationale: a long-lived
  accumulating LLM context buys nothing — Claude's ~5-min cache TTL is shorter than a
  burler round, so caching never pays; compact handoff + fresh spawns is better.
- **Q:** Handoff on disk? **A:** Per-judge-call files `round-<token>-handoff.md`,
  latest-wins, reusing stale-move crash machinery; not a single in-place file.
- **Q:** Handoff format? **A:** Strict YAML frontmatter (lossless finding ledger +
  `covers_rounds`) over distilled prose; Go parses fail-loud; carry-forward
  prompt-enforced; no Go key-canonicalization.
- **Q:** Handoff write/parse failure? **A:** Fail-safe Warn; next judge call reads last
  valid handoff + uncovered reviews; no valid handoff → today's all-reviews behavior.
- **Q:** Pre-round targeting shape? **A:** Third judge framing, profile-gated, seed file
  per round, fail-safe skip; perch's profile doesn't enable it.
- **Q:** Perch public API? **A:** Byte-identical exported surface; ProfileHash stable.
- **Q:** Old in-flight blocks? **A:** state.json changes are additive only; resume
  works when the profile resolves to identical values. (Clarified: state.json =
  per-block `<runDir>/state.json` under `_lyx/perch-runs/<run-id>/`, written via
  `internal/state`.)
- **Q:** (review gap r1) Resume across the config migration when resolved values
  change? **A:** Accept fail-loud non-resume — the existing "different profile; use a
  fresh `--run-id`" error fires; no rehash/migration machinery, no pre-resolution
  hashing.
- **Q:** (review gap r2) "perchcli compiles untouched" vs the migration? **A:** Reword
  to the precise claim: engine-facing Go API unchanged; perchcli's profile-parsing
  schema, help text, and fixtures DO change. (Operator delegated: recommended option
  auto-applies for all review findings from round 2 on.)
- **Q:** Config layer? **A:** No treadle.yaml; Treadle takes resolved data. Then
  operator identified perch's split `judge_model`+`judge_effort` as an oversight:
  perch's file surface migrates to the established model-spec notation
  (`sonnet[effort=medium]`) in this task — split keys removed, fail-loud on old files,
  no deprecated fallback. Bare alias → operator-configured default effort from seeded
  `models.yaml` (loomyard's defaults override the provider's, defined in one place).
  Unsupported bracket params (e.g. `version`) fail loud. Profile struct unchanged, so
  ProfileHash equivalence and resume survive.
- **Q:** Does the handoff also bound what burler rounds read? **A:** No — judge only;
  reviewer hydration unchanged (behavior-preservation).
- **Q:** Testing strategy? **A:** Existing suites as differential bar (two knowing
  exception groups: judge read-set pins, config-key fixtures); new treadleengine unit
  tests with fakes; TDD on the pure parsers/computations.
