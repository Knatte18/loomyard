# Batch: engine-core

```yaml
task: "Build burler - the review+fix round worker"
batch: "engine-core"
number: 1
cards: 2
verify: go build ./... && go test ./internal/burlerengine/...
depends-on: []
```

## Batch Scope

Creates `internal/burlerengine` with its two pure, deterministic surfaces: the `Profile`
content contract with its fail-loud validation, and the review-file verdict parser. No
shuttle interaction yet — batch 2 consumes these types when it adds the prompt composer and
`Run`. External interface for the next batch: `Profile`, `FileSet`, `FixScope`, `RunOpts`,
`ErrClusterUnsupported`, `(*Profile).validate`, `Verdict`, `Severity`, `Finding`,
`ParseReview`.

## Cards

### Card 1: Profile types and validation

- **Context:**
  - `_mill/discussion.md`
  - `internal/shuttleengine/spec.go`
- **Edits:** none
- **Creates:**
  - `internal/burlerengine/profile.go`
  - `internal/burlerengine/profile_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create package `burlerengine` with the content contract from the
  discussion's profile-shape decision. Define `FileSet{Paths []string; Instructions
  string}`; `FixScope string` with constants `FixScopeOverlay FixScope = "overlay"` and
  `FixScopeSource FixScope = "source"`; `Profile{Target FileSet; Fasit FileSet; Rubric
  string; FixScope FixScope; ToolUse bool; ClusterN int; ReviewPath string;
  FixerReportPath string; PriorReviews []string; PriorFixerReports []string}`;
  `RunOpts{Model string; Effort string; Timeout time.Duration; Round string}`; and the
  sentinel `var ErrClusterUnsupported = errors.New("burler: cluster-N > 0 is not supported
  — cluster reviewers are gated on mux own-window anchoring (roadmap milestone 24); use
  cluster-N = 0")`. Implement `func (p *Profile) validate(worktreeRoot string) error`
  (unexported, pointer receiver, mirroring `shuttleengine.Spec.validate`'s
  normalize-in-place style): (1) resolve every entry of `Target.Paths`, `Fasit.Paths`,
  `PriorReviews`, `PriorFixerReports`, plus `ReviewPath` and `FixerReportPath`, to a
  cleaned absolute path written back in place (absolute entries kept verbatim, relative
  entries `filepath.Clean(filepath.Join(worktreeRoot, p))`); (2) `Target` and `Fasit` must
  each have at least one of `Paths` / non-whitespace `Instructions` (an empty Fasit
  degenerates the review to internal-consistency checking — say so in the error); (3)
  every resolved `Target.Paths`/`Fasit.Paths`/`PriorReviews`/`PriorFixerReports` entry
  must exist on disk (`os.Stat`; a file or a directory both count); (4) `Rubric` must be
  non-whitespace; (5) `FixScope` must be exactly `FixScopeOverlay` or `FixScopeSource` —
  anything else, empty included, is an error naming the two legal values (no silent
  default); (6) `ClusterN < 0` is an error; `ClusterN > 0` returns an error wrapping
  `ErrClusterUnsupported` (`%w`, so `errors.Is` matches); (7) `ReviewPath` and
  `FixerReportPath` must be non-empty. First-failure return order is (2)–(7) as listed;
  all messages prefixed `burler: `. `profile_test.go` is table-driven over: a fully valid
  profile (asserting in-place absolute resolution of every path field); each failure mode
  above; relative-vs-absolute path handling; a directory as a `Target.Paths` entry;
  `errors.Is(err, ErrClusterUnsupported)`.
- **Commit:** `burler: add Profile content contract with fail-loud validation`

### Card 2: Verdict, Finding, and the strict review-file parse

- **Context:**
  - `_mill/discussion.md`
  - `internal/yamlengine/reconcile.go`
- **Edits:** none
- **Creates:**
  - `internal/burlerengine/verdict.go`
  - `internal/burlerengine/verdict_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Implement the review-file contract from the discussion's
  review-file-format-and-parse decision. Define `Verdict string` with `VerdictApproved
  Verdict = "APPROVED"` and `VerdictBlocking Verdict = "BLOCKING"`; `Severity string` with
  `SeverityBlocking/SeverityMedium/SeverityLow/SeverityNit` = `"BLOCKING"/"MEDIUM"/"LOW"/
  "NIT"`; `Finding{ID string; Severity Severity; Location string; Summary string}` with
  yaml tags `id`, `severity`, `location`, `summary`. Implement `func ParseReview(content
  []byte) (Verdict, []Finding, error)`: the file must open with a `---` line and contain a
  closing `---` line (`---`-delimited YAML frontmatter over prose; CRLF tolerated); the
  header is unmarshalled with `gopkg.in/yaml.v3` (per the overview's
  yaml-strictness-split decision: unknown extra keys in the header or inside a finding are
  tolerated — do NOT use `KnownFields(true)` here). Enforce, fail-loud, each returning a
  `burler: `-prefixed error: missing/empty frontmatter or unparseable YAML; `verdict`
  absent or not exactly `APPROVED`/`BLOCKING` (case-sensitive); any finding with an
  empty/missing `id`, `severity`, `location`, or `summary`; a `severity` outside the
  four-value vocabulary; duplicate finding `id`s; `verdict: BLOCKING` with zero
  `BLOCKING`-severity findings; `verdict: APPROVED` with ANY `BLOCKING`-severity finding
  (a self-contradictory file is a reviewer-agent defect — no silent demotion). `APPROVED`
  with only MEDIUM/LOW/NIT findings is legal, as is either verdict with an absent or empty
  `findings` list only when the verdict is `APPROVED`. Prose below the closing `---` is
  ignored by the parser. `verdict_test.go` is table-driven over: happy APPROVED (no
  findings), happy APPROVED (NIT-only findings), happy BLOCKING (mixed severities incl.
  one BLOCKING), missing frontmatter, unclosed frontmatter, bad YAML, unknown verdict,
  lowercase `approved`, each missing-finding-key case, unknown severity, duplicate ids,
  BLOCKING-without-blocking-finding, APPROVED-with-BLOCKING-finding, unknown extra header
  key tolerated, CRLF content.
- **Commit:** `burler: add strict review-file verdict/findings parser`

## Batch Tests

`verify:` builds the whole module and runs `go test ./internal/burlerengine/...` — the two
new table-driven test files (`profile_test.go`, `verdict_test.go`) are the entire runnable
surface of this batch. Scoped to the new package; nothing outside it is touched.
