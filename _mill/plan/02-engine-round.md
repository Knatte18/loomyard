# Batch: engine-round

```yaml
task: "Build burler - the review+fix round worker"
batch: "engine-round"
number: 2
cards: 4
verify: go build ./... && go test ./internal/burlerengine/...
depends-on: [1]
```

## Batch Scope

Completes `internal/burlerengine`: the embedded prompt template that carries the round
discipline (plus the new CONSTRAINTS.md Review Round Invariant it machine-enforces), the
Go-side prompt composer, and the `Engine.Run` round driver over the `Shuttle` seam. After
this batch the library contract `{profile, prior-round files} → {verdict, review,
fixer-report}` is complete and unit-tested against a fake shuttle; batch 3 wraps it in a
CLI. External interface for later batches: `Shuttle`, `Engine`, `New`, `Result`,
`(*Engine).Run`.

## Cards

### Card 3: Embedded prompt template, its enforcement test, and the Review Round Invariant

- **Context:**
  - `_mill/discussion.md`
  - `docs/reviews/review-prompt-template.md`
  - `internal/stencil/stencil.go`
  - `internal/shuttleengine/template.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:**
  - `internal/burlerengine/review-prompt-template.md`
  - `internal/burlerengine/template.go`
  - `internal/burlerengine/template_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write the generic burler prompt template as a package asset embedded
  via `template.go` (`//go:embed review-prompt-template.md` into `var
  reviewPromptTemplate []byte`, following `internal/shuttleengine/template.go`'s shape).
  The template contains exactly the eight top-level markers from the overview's
  go-composes-template-blocks decision — `{{.target}}`, `{{.fasit}}`, `{{.rubric}}`,
  `{{.fix_scope_rules}}`, `{{.tool_use_rules}}`, `{{.prior_rounds}}`,
  `{{.review_path}}`, `{{.fixer_report_path}}` — and NO template conditionals. Static
  content (adapt the prose of `docs/reviews/review-prompt-template.md` and the
  discussion's prompt-template decision to a generic profile-driven round): (a) the two
  jobs, A-review then B-fix, in one agent; (b) the **Sequencing rule** as a BLOCKING
  section — job A complete and the review fully written to `{{.review_path}}` on disk
  before the agent touches (edits, creates, or deletes) a single target file; findings
  are recorded, never fixed on sight; (c) the **fix-everything rule** as a BLOCKING
  section — every recorded finding gets fixed in B, all severities including LOW and NIT
  ("severity affects how a finding is reported, not whether it gets fixed"); the only
  legitimate deferral is something the agent cannot do alone this round, stated
  explicitly with its reason in the fixer-report's deferred section; (d) the
  **review-file format contract** — `---`-delimited YAML frontmatter whose header carries
  `verdict: APPROVED | BLOCKING` and a `findings:` list where every entry has non-empty
  `id`, `severity` (one of `BLOCKING|MEDIUM|LOW|NIT`), `location`, `summary`; unique ids;
  BLOCKING verdict requires at least one BLOCKING finding; never write `APPROVED` while
  any finding is BLOCKING; prose review below the frontmatter with one
  `### [SEVERITY] <title>` block per finding carrying `**Location:** / **Issue:** /
  **Fix:**` lines; omit findings entirely when there are none; never invent findings to
  pad; (e) the **source-grounding rule** — never fabricate file contents; read the files;
  (f) the **fixer-report rule** — write `{{.fixer_report_path}}` unconditionally, every
  round, even when nothing was fixed (state "nothing fixed"), with a
  deferred-with-reason section; the run is not done until BOTH output files exist; (g)
  **never push, and never run git against `_lyx`/weft paths**. `template_test.go` has
  two tests: `TestTemplate_StatesRoundDiscipline` asserts the embedded bytes contain the
  load-bearing substrings `"Sequencing rule"`, `"before"` + `"touch"` in the same
  sentence (assert on a pinned phrase, e.g. `"fully written to"` and `"before you touch"`),
  `"not whether it gets fixed"`, `"never push"`, and `"nothing fixed"` — this is the
  machine half of the Review Round Invariant; and `TestTemplate_FillsWithAllMarkers`
  asserts `stencil.Fill(reviewPromptTemplate, values)` succeeds with all eight markers
  supplied and fails (error naming the marker) when any one is absent. In the SAME card,
  append a new short section `## Review Round Invariant` to `CONSTRAINTS.md`, after
  `## Weft Git Invariant` and before `## Sandbox Suite Coverage`, kept deliberately
  brief (≤10 lines, per operator instruction — existing entries tend to be too long):
  one review+fix round (burler now, hardener later) follows the round discipline —
  A-before-B (review fully on disk before any target edit), every recorded finding fixed
  in B including LOW/NIT, no self-grading (round N's fix is judged by round N+1's fresh
  review), commit-per-fix on host source and never push; **Enforced by**
  `internal/burlerengine/template_test.go` for the template's sequencing +
  fix-everything statements; the rest is a review obligation on prompt templates.
- **Commit:** `burler: embed round prompt template; add Review Round Invariant to CONSTRAINTS`

### Card 4: Prompt composer

- **Context:**
  - `_mill/discussion.md`
  - `internal/stencil/stencil.go`
- **Edits:** none
- **Creates:**
  - `internal/burlerengine/prompt.go`
  - `internal/burlerengine/prompt_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Implement `func composePrompt(p *Profile) (string, error)` in
  `prompt.go`, called only after `(*Profile).validate` (so all paths are absolute). It
  builds the eight marker values and returns `stencil.Fill(reviewPromptTemplate,
  values)`: `target` and `fasit` render as a markdown block of one backtick-wrapped
  absolute path per bullet (a directory entry is annotated "(a directory — everything
  under it)") followed by the `Instructions` text when non-empty; `rubric` is
  `p.Rubric` verbatim; `fix_scope_rules` switches on `p.FixScope` — `FixScopeSource`
  yields the commit-per-fix rules (write surface: the host working tree, whatever the
  findings require plus same-change tests/docs; commit each fix individually once green,
  message format `<module-or-target>: fix <finding-id> — <one-line what/why>`; never
  push), `FixScopeOverlay` yields the overlay rules (write surface: EXACTLY the target
  paths plus the two output files, nothing else; no git commands at all — the loop owner
  commits); `tool_use_rules` switches on `p.ToolUse` — true yields "drive the real
  substrate: build, run, test what you review", false yields "read-only analysis in job
  A: read files, run nothing"; `prior_rounds` is "This is the first round — no prior
  round files exist." when both `PriorReviews` and `PriorFixerReports` are empty,
  otherwise a block listing the prior review and fixer-report paths (bullets) plus the
  clean-room rule verbatim in spirit: form your OWN findings first; only AFTER your
  review is saved may you read the prior rounds' files, to confirm previously-fixed
  behaviors have not regressed and to re-evaluate their deferred items;
  `review_path`/`fixer_report_path` are the resolved absolute paths. `prompt_test.go`
  covers: all markers filled for a minimal valid profile (result contains both paths and
  the rubric text); `FixScopeSource` output contains "commit" and not the
  overlay-exclusive phrasing while `FixScopeOverlay` output contains "no git" and not
  "commit each fix"; `ToolUse` true/false swap their phrases; first-round vs
  prior-round `prior_rounds` blocks; directory annotation for a dir entry.
- **Commit:** `burler: compose round prompt from profile via stencil`

### Card 5: Shuttle seam, Engine, Run, and outcome mapping

- **Context:**
  - `_mill/discussion.md`
  - `internal/shuttleengine/spec.go`
  - `internal/shuttleengine/run.go`
  - `internal/shuttleengine/engine.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:** none
- **Creates:**
  - `internal/burlerengine/engine.go`
  - `internal/burlerengine/engine_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Implement the round driver in `engine.go`. Define `type Shuttle
  interface { Run(shuttleengine.Spec) (shuttleengine.Result, error) }` (satisfied by
  `*shuttleengine.Runner`); `type Engine struct` holding the `Shuttle` and the
  `*hubgeometry.Layout`; `func New(shuttle Shuttle, layout *hubgeometry.Layout) *Engine`;
  `type Result struct { Outcome shuttleengine.Outcome; Verdict Verdict; Findings
  []Finding; ReviewPath string; FixerReportPath string; SessionID string; StrandGUID
  string; LastAssistantMessage string }`; and `func (e *Engine) Run(p Profile, opts
  RunOpts) (Result, error)`. Run sequence: (1) `p.validate(e.layout.WorktreeRoot)` —
  error → `(Result{}, err)`; (2) `composePrompt(&p)`; (3) build
  `shuttleengine.Spec{Prompt: prompt, OutputFiles: []string{p.ReviewPath,
  p.FixerReportPath}, Model: opts.Model, Effort: opts.Effort, Timeout: opts.Timeout,
  Role: "burler", Round: opts.Round}` — `Interactive`, `Parent`, `Display`, `KeepPane`
  stay zero-valued (autonomous defaults per the discussion's run-tuning-off-profile
  decision); (4) `e.shuttle.Run(spec)` — error → `(Result{}, fmt.Errorf("burler: shuttle
  run: %w", err))`; (5) populate Result with `Outcome`, `SessionID`, `StrandGUID`,
  `LastAssistantMessage` from the shuttle result and the resolved
  `ReviewPath`/`FixerReportPath` from the profile; (6) if `Outcome !=
  shuttleengine.OutcomeDone` return `(Result, nil)` — asking/died/timeout are normal
  loop events, `Verdict` stays empty; (7) on done, `os.ReadFile(p.ReviewPath)` then
  `ParseReview` — either error → return the populated Result plus a wrapped error (a
  defaulted verdict could terminate a loop on a malformed round: fail loud); (8) set
  `Verdict`/`Findings`, return `(Result, nil)`. `engine_test.go` uses a same-package
  `fakeShuttle` whose `Run` records the received `Spec` and returns a scripted
  `shuttleengine.Result` after optionally writing scripted content to the spec's
  `OutputFiles` entries (mirroring the file contract). Cover: spec construction (prompt
  non-empty, OutputFiles exactly `[reviewPath, fixerPath]` resolved absolute and in that
  order, `Role == "burler"`, Model/Effort/Timeout/Round mapped, `Interactive == false`);
  `ClusterN: 1` → error matching `errors.Is(err, ErrClusterUnsupported)` and the fake
  never invoked; each non-done outcome → `Result.Outcome` set, empty `Verdict`, nil
  error, `LastAssistantMessage` carried for asking; done + valid BLOCKING review file →
  `VerdictBlocking` with parsed findings; done + valid APPROVED file → `VerdictApproved`;
  done + missing review file → error; done + malformed frontmatter → error carrying the
  parse failure; shuttle error → wrapped error.
- **Commit:** `burler: drive one A→B round through the Shuttle seam with strict verdict parse`

### Card 6: Package doc

- **Context:**
  - `_mill/discussion.md`
  - `docs/modules/burler.md`
  - `internal/shuttleengine/doc.go`
- **Edits:** none
- **Creates:**
  - `internal/burlerengine/doc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write the `burlerengine` package doc comment in `doc.go`, following
  `internal/shuttleengine/doc.go`'s register. This is the durable home of
  `docs/modules/burler.md`'s design content (that file is deleted in batch 5 per the
  Documentation Lifecycle), so fold in: what a burler is (ONE review+fix round — A-review
  then B-fix in a single agent — returning an invariant `{verdict, review file,
  fixer-report}` contract); A-before-B as a hard gate and why (a review finished after
  edits is post-hoc rationalization); fix-everything including NITs and why (unfixed nits
  loop instead of closing); no self-grading — the loop owner (perch, unbuilt) judges round
  N's fix with round N+1's fresh burler; the profile as the content contract vs `RunOpts`
  as caller-resolved run-tuning kept off the profile; the `{overlay, source}` FixScope
  write-surface/git split; weft-blindness (burler returns artifact paths; the weft commit
  belongs to the loop owner — Weft Git Invariant); `cluster-N > 0` rejected until mux
  own-window anchoring lands (roadmap milestone 24); why burler is a separate module from
  perch (LLM-heavy vs deterministic loop, fake-shuttle vs fake-burler test regimes,
  `perch → burler → shuttle` strict chain).
- **Commit:** `burler: write package doc folding in the module design`

## Batch Tests

`verify:` runs `go test ./internal/burlerengine/...` — template enforcement + fill tests
(card 3), prompt composer tables (card 4), and the fake-shuttle round-driver tables
(card 5) all live in this package. Scoped to the package this batch completes.
