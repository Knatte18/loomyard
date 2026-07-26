# Batch: judge-handoff

```yaml
task: 'Treadle: shared round-loop engine + perch rewrite'
batch: judge-handoff
number: 2
cards: 4
verify: go test ./internal/treadleengine/... ./internal/perchengine/... ./internal/perchcli/... ./cmd/lyx/...
depends-on: [1]
```

## Batch Scope

Adds the judge-maintained handoff to treadleengine and lets perch adopt it:
the progress judge's read-set changes from {every prior review} to {latest
valid handoff + reviews of rounds it does not cover}, killing the O(N)
judge-context growth. The handoff is produced by the SAME judge call that
renders the verdict (extended circling/milestone templates, a second output
file), one file per judge call (`round-<token>-handoff.md`, latest-wins),
with a strict-YAML lossless finding ledger + `covers_rounds` over distilled
prose. Failure is always fail-safe: the parser is fail-loud (two-layer split
mirroring `ParseJudgeVerdict`), the loop swallows parse errors into Warn +
fallback read-set — never a propagated error, never STUCK; with no valid
handoff at all the read-set degrades to exactly today's all-reviews
behavior. Burler-round hydration is deliberately untouched
(`collectPriorHydration` semantics unchanged). External interface for batch
3: the `judgeReadSet` walk and the `roundRecord.HandoffPath` field.

## Cards

### Card 6: handoff format and fail-loud parser (TDD)

- **Context:**
  - `internal/treadleengine/judgeverdict.go`
  - `internal/treadleengine/judgeverdict_test.go`
  - `_mill/discussion.md`
  - `manifest/designs/treadle.md`
- **Edits:** none
- **Creates:**
  - `internal/treadleengine/handoff.go`
  - `internal/treadleengine/handoff_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** TDD the pure parser first (write `handoff_test.go`
  cases, then `handoff.go`). File format — strict `---`-delimited YAML
  frontmatter over unconstrained prose:
  frontmatter keys `covers_rounds` (non-empty list of positive ints — the
  rounds whose reviews this handoff has absorbed) and `ledger` (list, MAY
  be empty, of entries with `key` — short stable finding identity, non-empty
  string; `rounds` — non-empty list of positive ints the finding was seen
  in; `status` — exactly `open` or `resolved`). Go types: `type Handoff
  struct { CoversRounds []int; Ledger []LedgerEntry; Prose string }`,
  `type LedgerEntry struct { Key string; Rounds []int; Status string }`.
  `func ParseHandoff(content []byte) (Handoff, error)`: reuse
  `splitFrontmatter` (same three fail-loud checks, CRLF-tolerant), strict
  `treadle: `-prefixed errors for missing/unclosed/empty frontmatter,
  invalid YAML, empty `covers_rounds`, non-positive round numbers, a ledger
  entry with empty `key`, empty `rounds`, or a status outside
  {open, resolved}. Document the two-layer posture on the function: the
  parser NEVER silently defaults (mirrors `ParseJudgeVerdict`); the loop
  (card 7) swallows the error into the judge's fail-safe Warn + fallback,
  never STUCK. The ledger is deliberately not semantically matched in Go
  (no key canonicalization — the judge stays the holistic decider); Go
  only validates shape. Test coverage: happy path incl. prose extraction
  and empty ledger; every fail-loud rule; CRLF content.
- **Commit:** `treadle: add fail-loud handoff frontmatter parser`

### Card 7: handoff lifecycle in the round loop and judge calls

- **Context:**
  - `internal/treadleengine/handoff.go`
  - `internal/treadleengine/judgeverdict.go`
  - `internal/shuttleengine/spec.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/treadleengine/run.go`
  - `internal/treadleengine/judge.go`
  - `internal/treadleengine/state.go`
  - `internal/treadleengine/roundfiles.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `roundfiles.go`: `roundArtifactPaths` gains a `Handoff` field;
  `artifactPaths` names it `round-<token>-handoff.md`.
  `state.go`: `roundRecord` gains `HandoffPath string
  `json:"handoffPath,omitempty"`` (additive — see shared decision
  state-json-compatibility); `moveStaleArtifacts` includes the handoff
  path in its move-aside list.
  `handoff.go` or `run.go` (implementer's choice of file, named functions
  required): the newest-valid-handoff walk is its own helper —
  `latestValidHandoff(rounds []roundRecord) (path string, h Handoff, ok
  bool)` — walking completed rounds newest-to-oldest for a record with a
  non-empty `HandoffPath` whose file reads and `ParseHandoff`s cleanly; an
  unreadable/unparseable recorded handoff logs a `logger.Warn` (fail-safe
  posture, naming round and cause) and the walk continues to the next
  older one; no hit → ok false. (Batch 3's pre-round targeting reuses this
  helper — it must not depend on a current-round review existing.) On top
  of it, `judgeReadSet(rounds []roundRecord, currentReviewPath string)
  (readSet []string, prevHandoffPath string)` replaces
  `collectJudgeReviews` at both judge call sites: with a valid handoff,
  `readSet` = reviews of every completed round whose number is NOT in its
  `covers_rounds` (in round order) plus `currentReviewPath`, and
  `prevHandoffPath` = that handoff's path; with none, exactly today's
  behavior: all completed rounds' reviews + current, empty
  `prevHandoffPath`. This walk is what closes the judge-gap hole: rounds
  where no judge ran have no `HandoffPath` and are absent from every
  `covers_rounds`, so their reviews are always fed to the next judge call.
  `judge.go`: `judgeInputs` gains `PreviousHandoffPath string` and
  `HandoffPath string`; `runCircling`/`runMilestone` add stencil values
  `previous_handoff` (the path, or the literal `(none)` when empty) and
  `handoff_path`; the shuttle `Spec.OutputFiles` becomes
  `[verdict_path, handoff_path]` for both framings (triage unchanged).
  `run.go`: both judge call sites pass the read-set walk's outputs and the
  round's `Paths.Handoff`; after a judge call with `judgeOK` true, the loop
  stats + `ParseHandoff`-validates the handoff file — valid: set
  `record.HandoffPath`; missing or invalid: `logger.Warn` (naming the
  framing label, round, cause) and leave the field empty — never an error,
  never STUCK, and the verdict handling is unaffected. A judge call whose
  verdict failed (`judgeOK` false) records no handoff either.
- **Commit:** `treadle: bound the judge read-set with a per-call handoff and coverage fallback`

### Card 8: extend the circling and milestone templates for handoff maintenance

- **Context:**
  - `internal/treadleengine/handoff.go`
  - `internal/treadleengine/judge.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/treadleengine/judge-circling-template.md`
  - `internal/treadleengine/judge-milestone-template.md`
  - `internal/treadleengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend BOTH templates in place (no new template; the
  handoff rides the same judge call). Each gains: a `{{.previous_handoff}}`
  input marker section ("read the previous handoff at this path; `(none)`
  means this is the first handoff"); an instruction that `{{.prior_reviews}}`
  lists only reviews not yet absorbed by that handoff; and a second-output
  section: write `{{.handoff_path}}` as strict `---`-delimited YAML
  frontmatter (`covers_rounds`, `ledger` — exact key names and entry shape
  from card 6) over a distilled prose narrative. Spell out, as BLOCKING
  rules in the template: (a) the lossless carry-forward rule — every ledger
  entry from the previous handoff MUST reappear, as `open` or `resolved`,
  never dropped; (b) `covers_rounds` = the previous handoff's
  `covers_rounds` plus the round number of every review file read this
  call (including the current round's); (c) distill the prose, but keep the
  key ledger lossless; (d) write EXACTLY TWO files — the verdict file and
  the handoff file. Keep every OTHER existing pinned statement (fail-safe
  direction, verdict vocabulary, frontmatter strictness) intact — with the
  one deliberate exception that (d) supersedes: the circling and milestone
  templates' current "EXACTLY ONE" output-file wording is replaced by the
  two-file rule.
  `template_test.go`: replace the stale `EXACTLY ONE` `requireContains`
  pins in the circling and milestone sub-tests with the new two-file pin
  (the triage sub-test's `EXACTLY ONE` pin is untouched — triage still
  writes one file); extend the fill tests to supply the new markers
  (stencil.Fill requires all markers; no conditionals allowed in
  templates); add pinned-statement assertions for (a), (b), and (d) on
  both templates, in the file's existing pinned-substring style.
- **Commit:** `treadle: extend judge templates with handoff maintenance instructions`

### Card 9: handoff lifecycle tests and perch read-set pin update

- **Context:**
  - `internal/treadleengine/run.go`
  - `internal/treadleengine/judge.go`
  - `internal/treadleengine/handoff.go`
  - `internal/treadleengine/state.go`
  - `internal/treadleengine/roundfiles.go`
- **Edits:**
  - `internal/treadleengine/engine_test.go`
  - `internal/treadleengine/state_test.go`
  - `internal/perchengine/run_test.go`
  - `internal/perchengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `engine_test.go` (treadle): scripted-shuttle loop tests for the handoff
  lifecycle — (a) a judge round writes/records the handoff only when the
  shuttle fake actually produced a valid handoff file; (b) the next judge
  call's Spec carries `previous_handoff` = the recorded path and its
  read-set lists only uncovered reviews (assert via the recorded prompt/
  Spec contents, the same technique the existing tests use for
  `prior_reviews`); (c) `covers_rounds` correctness across judge-skipped
  rounds — a round with no judge call is fed to the NEXT judge call;
  (d) invalid-handoff fallback — a recorded handoff whose content is
  corrupted falls back (Warn, older-or-all-reviews read-set), the block
  continues, never STUCK from handoff machinery; (e) no-valid-handoff
  degrades to the exact all-reviews list.
  `state_test.go` (treadle): round-trip the additive `handoffPath` field;
  a legacy record WITHOUT the field loads and resumes cleanly (old-state
  resume).
  `internal/perchengine/run_test.go`: update ONLY the judge read-set pin
  assertions (the sanctioned exception (b) in shared decision
  differential-test-bar): where the test currently asserts the judge Spec/
  prompt lists every prior round's review, assert the handoff contract
  instead (first judge call: all reviews, no previous handoff; subsequent:
  handoff + uncovered). The scripted shuttle fake must now also produce
  handoff files where a scenario depends on coverage — concretely:
  `queuedShuttle`'s scripted-call entry gains a `handoffContent string`
  field, and its `Run` writes it to `Spec.OutputFiles[1]` when non-empty
  (today it writes only the verdict file at `OutputFiles[0]`).
  Burler-hydration assertions (including failed-gate
  feed-forward) must remain byte-identical and passing.
  `internal/perchengine/doc.go`: update the verdict-judge section — the
  judge reads {latest valid handoff + uncovered reviews}, degrading to
  all-reviews; state.json records an optional per-round `handoffPath`;
  posture unchanged (fail-safe, never STUCK).
- **Commit:** `perch: adopt the judge handoff read-set and update the read-set pins`

## Batch Tests

`verify:` scope identical to batch 1 (same four package trees): treadle
tests prove the handoff lifecycle with fakes; the perchengine suite proves
perch's adoption differentially (hydration, ladder, gate, pause, resume
pins untouched); `cmd/lyx` guards re-check the templates' package and tier
purity.
