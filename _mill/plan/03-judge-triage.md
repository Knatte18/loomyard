# Batch: judge-triage

```yaml
task: "Build perch - the review gate loop"
batch: "judge-triage"
number: 3
cards: 3
verify: go test ./internal/perchengine/
depends-on: [2]
```

## Batch Scope

Perch's two ephemeral LLM utilities: the progress judge (two framings — per-round circling
check, milestone continuation gate) and the asking-triage call. Delivers the three embedded
stencil templates, the strict fail-loud verdict-file parsers, and the fail-safe spawn
wrappers over a package-local `Shuttle` seam. External interface for batch 4: the `Shuttle`
interface, `runCircling`, `runMilestone`, `runTriage`, and the verdict vocabularies
(`JudgeProgressing`/`JudgeCircling`/`JudgeContinue`/`JudgeStop`/`JudgeUncertain`,
`TriageRetry`/`TriageGiveUp`). Batch-local decision: the frontmatter splitter is a small
package-private copy of burlerengine's unexported `splitFrontmatter` (three checks,
CRLF-tolerant) rather than exporting burler's — the two parsers evolve independently.

## Cards

### Card 7: judge and triage prompt templates + embeds

- **Context:**
  - `internal/burlerengine/template.go`
  - `internal/burlerengine/review-prompt-template.md`
  - `internal/burlerengine/template_test.go`
  - `internal/stencil/stencil.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/perchengine/judge-circling-template.md`
  - `internal/perchengine/judge-milestone-template.md`
  - `internal/perchengine/triage-template.md`
  - `internal/perchengine/template.go`
  - `internal/perchengine/template_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Three markdown prompt templates in burlerengine's style: top-level
  `{{.X}}` stencil markers only, no `{{if}}`/`{{range}}`, a header comment naming every
  marker. All three instruct the agent to write EXACTLY ONE output file at
  `{{.verdict_path}}`: YAML frontmatter between `---` lines with `verdict:` and `rationale:`
  keys, then unconstrained prose. judge-circling-template.md (markers `round`,
  `prior_reviews`, `verdict_path`): the agent reads the listed prior review files, compares
  the newest round's findings against earlier rounds, and answers "is this block going in
  circles?" — verdict vocabulary exactly `PROGRESSING`, `CIRCLING`, or `UNCERTAIN`
  (uppercase); `CIRCLING` requires clear, citable evidence (the same underlying issue
  recurring across rounds, or a fix/break oscillation) named in the rationale; when in doubt
  answer `UNCERTAIN` — a false "circling" kills a converging block, a false "progressing"
  costs a few bounded rounds. judge-milestone-template.md (markers `round`, `hard_cap`,
  `prior_reviews`, `verdict_path`): the block has reached a soft cap at round `{{.round}}`
  (hard stop at `{{.hard_cap}}`); read the full review history and judge whether the
  trajectory justifies continuing — vocabulary exactly `CONTINUE`, `STOP`, `UNCERTAIN`;
  `STOP` requires clear evidence of stall or circularity; uncertain → `UNCERTAIN`. Both judge
  templates additionally instruct a `## Themes` section in the prose: a short human-facing
  overview of what KINDS of findings keep appearing per round, so an operator can eyeball
  overlap. triage-template.md (markers `round`, `question`, `verdict_path`): a review agent
  stopped mid-round and said `{{.question}}`; classify whether a fresh retry can plausibly
  proceed (`RETRY`) or the round profile itself is broken — missing context, contradictory
  instructions — so retrying would burn a round to hit the same wall (`GIVE_UP`); the
  rationale must restate the agent's blocker in one line. template.go: three `//go:embed`
  declarations mirroring burlerengine/template.go. template_test.go: for each template,
  assert `stencil.Fill` succeeds with all markers set and fails when one is missing; assert
  the templates state the load-bearing sentences (exact-vocabulary lists, clear-evidence
  requirement for CIRCLING/STOP, uncertain-fail-safe direction, Themes section, single
  output file) — mirroring burlerengine's `TestTemplate_StatesRoundDiscipline` style.
- **Commit:** `perch: add judge (circling, milestone) and triage prompt templates`

### Card 8: verdict-file parsers

- **Context:**
  - `internal/burlerengine/verdict.go`
  - `internal/perchengine/template.go`
- **Edits:** none
- **Creates:**
  - `internal/perchengine/judgeverdict.go`
  - `internal/perchengine/judgeverdict_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** judgeverdict.go: `type JudgeVerdict string` with constants
  `JudgeProgressing = "PROGRESSING"`, `JudgeCircling = "CIRCLING"`, `JudgeContinue =
  "CONTINUE"`, `JudgeStop = "STOP"`, `JudgeUncertain = "UNCERTAIN"`; `type TriageVerdict
  string` with `TriageRetry = "RETRY"`, `TriageGiveUp = "GIVE_UP"`. `type judgeFraming
  string` with `framingCircling` / `framingMilestone` selecting the allowed vocabulary
  ({PROGRESSING, CIRCLING, UNCERTAIN} vs {CONTINUE, STOP, UNCERTAIN}). `func
  ParseJudgeVerdict(content []byte, framing judgeFraming) (JudgeVerdict, string, error)` and
  `func ParseTriageVerdict(content []byte) (TriageVerdict, string, error)` — both split
  frontmatter via a package-private `splitFrontmatter` copied from burlerengine's (same three
  fail-loud checks: opening `---`, closing `---`, non-empty header; CRLF-tolerant), decode
  `verdict:` and `rationale:` keys (unknown extra keys tolerated, like burler's review
  header), and enforce fail-loud with `"perch: "` prefixes: verdict exactly one of the
  framing's vocabulary (case-sensitive), rationale non-empty. Second return value is the
  rationale. judgeverdict_test.go: table-driven — every legal verdict per framing, wrong
  framing's vocabulary rejected, lowercase rejected, missing/empty rationale rejected,
  missing/unclosed/empty frontmatter rejected, CRLF content accepted.
- **Commit:** `perch: add strict judge and triage verdict-file parsers`

### Card 9: fail-safe judge/triage spawners over the Shuttle seam

- **Context:**
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/smoke_round_test.go`
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/run.go`
  - `internal/logger/logger.go`
  - `internal/stencil/stencil.go`
  - `internal/perchengine/judgeverdict.go`
  - `internal/perchengine/template.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/perchengine/judge.go`
  - `internal/perchengine/judge_test.go`
  - `internal/perchengine/smoke_judge_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** judge.go: `type Shuttle interface { Run(shuttleengine.Spec)
  (shuttleengine.Result, error) }` with the compile-time proof `var _ Shuttle =
  (*shuttleengine.Runner)(nil)` (burlerengine's seam pattern verbatim). `type judgeInputs
  struct { Round int; HardCap int; PriorReviews []string; VerdictPath string; Model, Effort
  string }`. Three functions, each composing the stencil-filled prompt, building
  `shuttleengine.Spec{Prompt, OutputFiles: []string{VerdictPath}, Model, Effort, Role,
  Round}` (Role `"judge"` for both judge framings, `"triage"` for triage; Round is
  `strconv.Itoa(inputs.Round)`), running it through the seam, reading and parsing the verdict
  file: `func runCircling(sh Shuttle, in judgeInputs) (JudgeVerdict, string)`, `func
  runMilestone(sh Shuttle, in judgeInputs) (JudgeVerdict, string)`, `func runTriage(sh
  Shuttle, round int, question, verdictPath, model, effort string) (TriageVerdict, string)`.
  ALL are fail-safe, never returning an error: any failure — stencil fill, shuttle Run error,
  non-done Outcome, verdict file read, parse — logs `logger.Warn("perch: <call> failed,
  <fail-safe consequence>", ...)` with round and cause, and returns the safe default
  (`JudgeProgressing` for circling, `JudgeContinue` for milestone, `TriageRetry` for triage)
  with an empty rationale. `PriorReviews` are rendered into the `prior_reviews` marker as a
  newline-separated absolute-path list (the judge agent reads the files itself — its input is
  self-contained, no memory between calls). judge_test.go: fake Shuttle covering — happy path
  per function (fake writes a valid verdict file, spec assertions: Role, Model/Effort
  passthrough, OutputFiles = [VerdictPath]); each fail-safe branch (Run error, non-done
  outcome, missing file, unparseable file) returns the safe default and does not error.
  smoke_judge_test.go: `//go:build smoke` opt-in test in package `perchengine_test` following
  burlerengine's smoke_round_test.go gating (skip when no claude binary resolves): spawn a
  real judge over two tiny fixture review files written by the test and assert the verdict
  file parses. Note: judge.go builds prompts via a small local fill helper around
  `stencil.Fill`; the burler round itself is NOT touched by this card.
- **Commit:** `perch: add fail-safe judge and triage spawners over the shuttle seam`

## Batch Tests

`verify:` runs `go test ./internal/perchengine/`: template marker/statement tests (card 7),
parser tables (card 8), fake-shuttle spawner + fail-safe branch tests (card 9). The smoke
test is excluded by its build tag and never runs in `verify:`.
