# Batch: webster-mechanism-helpers

```yaml
task: 'webster: rewrite for flat card list'
batch: webster-mechanism-helpers
number: 5
cards: 6
verify: go test -tags integration ./internal/websterengine/...
depends-on: [4]
```

## Batch Scope

Create the webster-LOCAL mechanism layer that replaces every non-plan, non-report helper
webster currently borrows from `internal/builderengine` — the first half of cutting the import
edge. This batch only ADDS new files to `internal/websterengine`; it does NOT yet edit the
existing verb files that still import `builderengine` (batch 7 retargets the call sites).
The new files compile alongside the old code and are exercised by their own tests. Disposition
per the frozen-builder-copy-not-move rule: the two git helpers RE-POINT to `internal/gitrepo`;
everything else is a webster-local copy (pure-Go primitives, or thin `shuttleengine` wrappers
webster inlines) — because every borrowed symbol has an in-tree builder caller and moving it
would break frozen builder. Export visibility is set by cross-package need: symbols
`internal/webstercli` calls are exported; engine-internal helpers stay unexported. The report
contract and digest/classify cluster are the second half (batch 6).

## Rename mechanic

_This batch performs no renames; all `Moves:` fields are `none`. Section retained only to note
that the mechanism layer is created fresh (copy), never `git mv`-moved out of builderengine
(which stays frozen)._

## Cards

### Card 15: git helpers re-pointed to gitrepo

- **Context:**
  - `internal/builderengine/gitquery.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitexec/gitexec.go`
  - `internal/websterengine/testmain_test.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/gitwrap.go`
  - `internal/websterengine/gitwrap_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `gitwrap.go` provide webster-internal (unexported) git helpers replacing `builderengine.HeadSHA`/`ChangedFiles`/`Dirty`: `headSHA(worktree string) (string, error)` re-points to `gitrepo.New(worktree).CurrentSHA()` (gitrepo maps unborn HEAD to `ErrNoCommits`, a superset of the old behavior); `changedFiles(worktree, sinceSHA string) ([]string, error)` calls `gitrepo.New(worktree).ChangedFilesSince(sinceSHA)` then applies webster's deterministic normalization on top (`filepath.ToSlash` each path and `sort.Strings`) because `gitrepo.ChangedFilesSince` uses `-z --no-renames` and does NOT sort/slash — webster's deviation compare wants the sorted, slash-normalized list; `dirty(worktree string) (bool, error)` is a webster-local copy over `gitexec.RunGit` (`git status --porcelain`, true iff trimmed output non-empty) because `gitrepo.Repo` exposes no porcelain/status method and adding one is out of scope. In `gitwrap_test.go` add `//go:build integration`-tagged tests (real git; reuse the package's existing hermetic `TestMain`) covering headSHA on a repo with commits, changedFiles returning a sorted slash-normalized set, and dirty true/false.
- **Commit:** `feat(websterengine): git helpers re-pointed to gitrepo (headSHA/changedFiles/dirty)`

### Card 16: fingerprint helper (webster-local copy)

- **Context:**
  - `internal/builderengine/fingerprint.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/fingerprint.go`
  - `internal/websterengine/fingerprint_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `fingerprint.go` add an unexported `fingerprint(planDir string) (string, error)` — a verbatim webster-local copy of `builderengine.Fingerprint`: SHA-256 over each `*.md` file in `planDir` as `name + NUL + contents + NUL`, filenames sorted lexically, returning lowercase hex; pure Go (`os.ReadDir`/`ReadFile`, `crypto/sha256`, `sort`). This is webster's own plan-fingerprint for the `--fresh` crash/resume guard (batch 7 wires it into state and runlevel). In `fingerprint_test.go` add Tier-1 tests (no git; `t.TempDir()` with a couple of `.md` files) asserting stability across reads and sensitivity to content and filename changes.
- **Commit:** `feat(websterengine): webster-local plan fingerprint helper`

### Card 17: pause helpers (webster-local copy, exported)

- **Context:**
  - `internal/builderengine/pause.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/pause.go`
  - `internal/websterengine/pause_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `pause.go` add webster-local copies of the four pause symbols, EXPORTED because `internal/webstercli` calls them (`pause.go`/`status.go`/`weft.go` retarget in batch 9): `const PauseFlagName = "pause"`; `RequestPause(websterDir string) error` (`os.MkdirAll` + `os.OpenFile O_CREATE`, idempotent); `PauseRequested(websterDir string) bool` (`os.Stat` presence); `ClearPause(websterDir string) error` (`os.Remove`, absent→nil). Add an unexported `pauseFlagPath(websterDir string) string` join helper. The pause flag lives under the webster dir (caller passes `hubgeometry.WebsterDir(...)`); do not construct `_lyx`/`webster` tokens here. In `pause_test.go` add Tier-1 tests (no git; `t.TempDir()`) for request→requested→clear idempotency.
- **Commit:** `feat(websterengine): webster-local pause flag helpers`

### Card 18: archive helpers (webster-local copy)

- **Context:**
  - `internal/builderengine/runlevel.go`
  - `internal/builderengine/outcome.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/archive.go`
  - `internal/websterengine/archive_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `archive.go` add webster-local copies (unexported) of the archive primitives with a webster-owned timestamp const `const archiveTimestampFormat = "20060102T150405Z"`: `firstFreeArchivePath(candidate func(suffix string) string) (string, error)` (first non-existent of `candidate("")`, `candidate("-1")`, … via `os.Stat`); `archiveStateFile(websterDir string, now func() time.Time) (string, error)` (rename webster's `state.json` → `state-<stamp>.json`, absent→`("",nil)`); `archiveReportsDir(reportsDir string, now func() time.Time) error` (rename the reports dir wholesale to `<dir>-<stamp>` and recreate empty). Use webster's own state filename (`state.json` under the webster dir — reuse the package's existing state-path constant/helper from `state.go` if present, otherwise a local const) rather than importing builder's name. In `archive_test.go` add Tier-1 tests (no git; `t.TempDir()`, injected `now`) covering collision suffixing, absent-file no-op, and reports-dir recreate.
- **Commit:** `feat(websterengine): webster-local archive helpers`

### Card 19: outcome contract (webster-local copy)

- **Context:**
  - `internal/builderengine/outcome.go`
  - `internal/websterengine/archive.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/outcome.go`
  - `internal/websterengine/outcome_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `outcome.go` add webster's own Master-outcome contract (unexported/engine-scoped, since only `runlevel.go`/`summary.go` consume it in-package): `type outcome struct { Outcome string; StuckReason string; BatchesDone int }` with yaml tags; the vocabulary consts `outcomeDone = "done"`, `outcomePaused = "paused"`, `outcomeStuck = "stuck"`; `parseOutcome(path string) (*outcome, error)` doing a strict `yaml.Decoder.KnownFields(true)` decode with vocabulary + `stuck⇒reason` validation, wrapped `websterengine:`-prefixed errors; and `archiveStaleOutcome(websterDir string, now func() time.Time) (string, error)` renaming `outcome.yaml`→`outcome-<stamp>.yaml` (absent→`("",nil)`) via the batch-18 `firstFreeArchivePath`. This is a rewrite-anyway contract: webster defines its own shape; do not import builder's `Outcome`/`ParseOutcome`. In `outcome_test.go` add Tier-1 tests (no git; `t.TempDir()`) for a valid done/paused/stuck parse, a strict-decode rejection of an unknown key, the stuck-without-reason rejection, and stale-outcome archive.
- **Commit:** `feat(websterengine): webster-local Master outcome contract`

### Card 20: strand/spawn seam helpers (webster-local, exported where cli-facing)

- **Context:**
  - `internal/builderengine/poll.go`
  - `internal/builderengine/spawn.go`
  - `internal/builderengine/runlevel.go`
  - `internal/shuttleengine/mux.go`
  - `internal/shuttleengine/engine.go`
  - `internal/websterengine/audit.go`
- **Edits:** none
- **Creates:**
  - `internal/websterengine/strand.go`
  - `internal/websterengine/strand_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `strand.go` add webster-local copies of the strand/spawn seam symbols, inlining direct `shuttleengine` calls (no builder import). EXPORTED (called by `internal/webstercli`): `StrandLive(mux shuttleengine.MuxOps, guid string) (bool, error)` (`mux.Status()`, scan `.Strands` for guid, return its `.Live`; absent→`(false,nil)`); `TurnEnded(eventsPath string, engine shuttleengine.Engine) (bool, error)` (read the events file, `engine.ParseEvents`, true iff any `Event.Kind == shuttleengine.EventStop`; missing file→`(false,nil)`); the spawn seam interfaces `type Starter interface { Start(shuttleengine.Spec) (*shuttleengine.Run, error) }` and `type OrchestratorStarter interface { StartOrchestrator(shuttleengine.Spec) (OrchestratorHandle, error) }` plus its companion `type OrchestratorHandle interface { StrandGUID() string; Wait() (shuttleengine.Result, error) }` (Go structural typing means `*shuttleengine.Runner` still satisfies `Starter`; webstercli assigns the runner to these seams). Engine-internal (unexported): `removeStrandIfLive(mux shuttleengine.MuxOps, guid string) error` (call `StrandLive`; if live, `mux.RemoveStrand(guid, false)`; a `StrandLive` error is treated as not-live; a failed removal of a live strand propagates). Confirm the exact `shuttleengine` type/const names against the package before writing. In `strand_test.go` add Tier-1 tests (no git; fake `shuttleengine.MuxOps`/`Engine` doubles like the existing engine tests) for StrandLive present/absent/live-false, TurnEnded stop/no-stop/missing-file, and removeStrandIfLive live/not-live/removal-error.
- **Commit:** `feat(websterengine): webster-local strand/spawn seam helpers`

## Batch Tests

`verify: go test -tags integration ./internal/websterengine/...` — the `-tags integration`
flag runs the git-spawning `gitwrap_test.go` (card 15) alongside the Tier-1 tests for
fingerprint/pause/archive/outcome/strand (cards 16–20, all `t.TempDir()`/fake-based, no git).
The package's existing hermetic `TestMain` neutralizes the operator gitconfig. New files are
additive and not yet called by the still-`builderengine`-importing verb files; those call sites
retarget in batch 7.
