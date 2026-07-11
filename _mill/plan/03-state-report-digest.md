# Batch: state-report-digest

```yaml
task: "Build builder - the batch-implementation loop"
batch: "state-report-digest"
number: 3
cards: 5
verify: go test ./internal/builderengine/...
depends-on: [1]
```

## Batch Scope

The durable run state (`state.json` with plan fingerprint, per-batch records, chain
anchors), the fail-loud batch-report parser, the git query layer (start-SHA capture,
diff-vs-SHA, dirty check), the digest distillation (report + diff + scope → the pinned
terse field set), and chain membership + rollback. External interface consumed later:
`State`/`BatchState`, `LoadState`/`SaveState`, `ParseReport`, `Distill`, `Digest`,
`HeadSHA`/`ChangedFiles`/`Dirty`, `ChainMembers`/`RestartChain`.

## Cards

### Card 12: run state

- **Context:**
  - `_mill/discussion.md`
  - `internal/hubgeometry/hubgeometry.go`
- **Creates:**
  - `internal/builderengine/state.go`
  - `internal/builderengine/state_test.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `State` (json tags): `RunGUID string`, `PlanFingerprint string`,
  `CurrentBatch int` (0 = none in flight), `Batches map[int]*BatchState` keyed by batch
  number, `ChainStartSHAs map[int]string` keyed by chain-end batch number (the anchor =
  host HEAD immediately before the lowest member's first spawn). `BatchState`:
  `Slug string`, `StartSHA string`, `Role string`, `StrandGUID string`,
  `ShuttleRunDir string`, `EventsPath string`, `SpawnedAt string` (RFC3339 UTC),
  `Terminal bool`, `Status string`. `LoadState(builderDir string) (*State, error)` — absent file returns
  `(nil, nil)`, unreadable/malformed JSON is a wrapped error (fail loud, never
  guessed); `SaveState(builderDir string, st *State) error` — MkdirAll + atomic write
  (temp file + rename) of `state.json` inside builderDir. Callers pass
  `hubgeometry.BuilderDir(...)`; state.go itself never constructs `_lyx` paths. Tests:
  round-trip, absent-file nil, corrupt-file error.
- **Commit:** `feat(builder): durable run state (state.json)`

### Card 13: batch-report parser

- **Context:**
  - `docs/modules/plan-format.md`
- **Creates:**
  - `internal/builderengine/report.go`
  - `internal/builderengine/report_test.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Types mirroring plan-format.md's batch-report schema: `Report`
  with `Batch string`, `Status string` (`done|stuck`), `Tests string`
  (`green|red|skipped`), `StuckReason string` (yaml `stuck_reason`, empty for the YAML
  `null`), `OutOfScope []OutOfScopeEntry` (`out_of_scope`); `OutOfScopeEntry` with
  `Path string`, `Why string`. `ParseReport(path string) (*Report, error)`:
  `yaml.Decoder.KnownFields(true)`, then vocabulary checks — unknown `status`/`tests`
  value, empty `batch`, `status: stuck` with empty `stuck_reason`, or an
  `out_of_scope` entry missing `path`/`why` are each distinct wrapped errors. The
  burler verdict-parse discipline: an unparseable report is a loud error, never a
  guessed digest. Tests: the two worked examples from plan-format.md parse exactly;
  each rejection case.
- **Commit:** `feat(builder): fail-loud batch-report parser`

### Card 14: git query layer

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `_mill/discussion.md`
- **Creates:**
  - `internal/builderengine/gitquery.go`
  - `internal/builderengine/gitquery_test.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Thin helpers over `gitexec.RunGit`, all taking an explicit
  `worktree string` cwd: `HeadSHA(worktree) (string, error)` via `rev-parse HEAD`;
  `ChangedFiles(worktree, sinceSHA string) ([]string, error)` via
  `diff --name-only <sinceSHA>..HEAD` (slash-normalized paths, sorted);
  `Dirty(worktree) (bool, error)` via `status --porcelain` non-empty;
  `ResetHard(worktree, sha string) error` via `reset --hard <sha>` (consumed by
  card 16 only). Non-zero git exit codes wrap stderr into the error. Tests build a
  scratch repo in `t.TempDir()` (`git init`, config user.name/email, commits via
  `gitexec.RunGit`) and exercise all four.
- **Commit:** `feat(builder): git query layer over gitexec`

### Card 15: digest distillation

- **Context:**
  - `_mill/discussion.md`
  - `docs/modules/plan-format.md`
- **Creates:**
  - `internal/builderengine/digest.go`
  - `internal/builderengine/digest_test.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `Digest` — the pinned terse contract (json tags exactly):
  `Batch string` (`batch`), `Status string` (`status`: `running|done|stuck|dead`),
  `Tests string` (`tests`, omitempty), `StuckReason string` (`stuck_reason`,
  omitempty), `OutOfScope []OutOfScopeEntry` (`out_of_scope`, omitempty),
  `DriftUnreported []string` (`drift_unreported`, omitempty), `FilesChanged int`
  (`files_changed`), `Dirty bool` (`dirty`), `DeadReason string` (`dead_reason`,
  omitempty: `asking|timeout|died`), `ElapsedS int` (`elapsed_s`, omitempty — running
  snapshots only). No prose fields, no file lists beyond the drift paths — the
  mill-go-bloat lesson is the contract. `Distill(report *Report, changed []string,
  scope []string, dirty bool) Digest` (`out_of_scope` is read from
  `report.OutOfScope` — never passed separately): scope comparison
  uses prefix semantics (a scope entry that is a directory covers everything under it;
  match on `/`-separated path boundaries, not raw string prefixes); a changed file
  outside every scope entry and not named by any `out_of_scope` entry lands in
  `DriftUnreported` (sorted); `FilesChanged = len(changed)`. Tests: in-scope,
  dir-prefix scope, justified out-of-scope, unreported drift, boundary case
  (`internal/foo` must not cover `internal/foobar`), dirty flag pass-through.
- **Commit:** `feat(builder): digest distillation with scope-drift computation`

### Card 16: chain membership and rollback

- **Context:**
  - `docs/modules/plan-format.md`
  - `_mill/discussion.md`
- **Creates:**
  - `internal/builderengine/chain.go`
  - `internal/builderengine/chain_test.go`
- **Edits:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `ChainMembers(plan *Plan, chainEnd int) []int` — every batch number
  whose `ChainEnd` equals chainEnd, plus chainEnd itself, sorted ascending; empty when
  chainEnd names no chain. `ChainEndFor(plan *Plan, batch int) int` — the chain-end a
  batch belongs to (its own `ChainEnd`, or `batch` itself when other batches name it),
  0 when the batch is chainless. `RestartChain(worktree string, st *State, plan *Plan,
  chainEnd int, reportsDir string) error`: verify `st.ChainStartSHAs[chainEnd]` is
  recorded (error if absent), `ResetHard` to it, delete the chain members' report
  files from reportsDir (named `NN-<slug>.yaml`), reset those members' `BatchState`
  entries and `CurrentBatch` in st (caller saves). The recorded SHA is the ONLY reset
  target — no caller-supplied SHA parameter exists, per the discussion's
  correctness-by-tool-design decision. Tests: scratch repo with a recorded anchor —
  commits after the anchor vanish on restart, reports removed, chainless/unrecorded
  cases error.
- **Commit:** `feat(builder): chain membership and Go-owned chain rollback`

## Batch Tests

`verify:` runs the builderengine suite: state round-trip, report parse accept/reject
tables, git helpers against scratch repos, digest scope/drift matrix, chain rollback
end-to-end in a scratch repo.
