# Plan: Shed: outer phase-FSM skeleton

```yaml
task: 'Shed: outer phase-FSM skeleton'
slug: shed
approved: true
started: '20260815-093520'
parent: main
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: package-skeleton
    file: 01-package-skeleton.md
    depends-on: []
    verify: go test ./internal/shedengine/...
  - number: 2
    name: run-loop
    file: 02-run-loop.md
    depends-on: [1]
    verify: go test ./internal/shedengine/... && go test -run 'TestTierPurity_|TestHermeticGitEnv_' ./cmd/lyx/
  - number: 3
    name: pause-and-resume-scenarios
    file: 03-pause-and-resume-scenarios.md
    depends-on: [2]
    verify: go test ./internal/shedengine/... && go test -run 'TestTierPurity_|TestHermeticGitEnv_' ./cmd/lyx/
  - number: 4
    name: persistence-and-hard-error-scenarios
    file: 04-persistence-and-hard-error-scenarios.md
    depends-on: [2]
    verify: go test ./internal/shedengine/... && go test -run 'TestTierPurity_|TestHermeticGitEnv_' ./cmd/lyx/
  - number: 5
    name: seam-invariant
    file: 05-seam-invariant.md
    depends-on: [2]
    verify: go test ./internal/shedengine/...
  - number: 6
    name: docs-reconciliation
    file: 06-docs-reconciliation.md
    depends-on: [2]
    verify: go test -run 'TestEnforcement_MarkdownLinks|TestDocsLink' ./internal/lyxcwd/
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: the discussion's Decisions section is authoritative over `manifest/designs/shed.md`

- **Decision:** where `_mill/discussion.md`'s Decisions section and the current text of `manifest/designs/shed.md` disagree, the discussion wins, and `shed.md` is corrected to match in batch 6.
  The known divergences are enumerated in the discussion's `docs-and-roadmap` decision, but that list is explicitly non-exhaustive.
- **Rationale:** `shed.md` is a design sketch written before this discussion ran; the discussion closed real gaps in it (the two-value `Outcome` contract against its own four-value prose, the step-6 routing target, the two-writes-vs-one-persist split, the missing `StatusLockPath`, the missing `Status`/`HistoryEntry`/`Activity` Go declarations).
  Implementing `shed.md` verbatim would ship several of those bugs.
- **Applies to:** all batches

### Decision: `internal/treadleengine` is the structural model, copied deliberately rather than by analogy

- **Decision:** every structural choice that has a treadle equivalent copies treadle's shipped shape: `type Outcome string` with string constants (`internal/treadleengine/result.go`), the `(Result, error)` entrypoint and terminal-`Outcome`-plus-reason `Result` (`internal/treadleengine/result.go`), the non-blocking run-lock acquire with a sentinel busy error (`internal/treadleengine/run.go`), the one-method seam interface (`internal/treadleengine/runner.go`), and the allowlist seam-enforcement test (`internal/treadleengine/seam_enforcement_test.go`).
- **Rationale:** `Shed` is the sibling generic engine one level up from treadle; matching its shapes keeps the two readable side by side and avoids inventing a second vocabulary for the same jobs.
- **Applies to:** all batches

### Decision: `Shed` is told all three of its paths and derives none

- **Decision:** `Shed` carries `StatusPath`, `LockPath`, and `StatusLockPath` as exported string fields, all caller-supplied.
  `internal/shedengine` never imports `internal/lyxcwd`, never names the `_lyx`/`.lyx` literals, and never joins a convention-relative constant of its own.
  The only filesystem paths it constructs are `filepath.Dir` of the two lock paths, passed to `os.MkdirAll` so the told path is usable.
- **Rationale:** the Cwd Resolution Invariant and the Lyxdirs Single-Declarer Invariant in `CONSTRAINTS.md`, plus the whole reason `Shed` is generic across products that will not share one geometry.
  Batch 5's seam-enforcement test is what keeps this from eroding.
- **Applies to:** all batches

### Decision: error style — plain wrapped `fmt.Errorf`, one exported sentinel

- **Decision:** `ErrShedBusy` is the only exported error value in the package.
  Every other failure is a `fmt.Errorf` carrying a distinct, `shedengine: `-prefixed message; tests assert on a distinct substring of that message, never on a sentinel identity.
- **Rationale:** the design pins exactly one sentinel (`ErrShedBusy`, mirroring `treadleengine.ErrBlockBusy`) because exactly one caller-side branch needs it — "another run holds the lock, try later".
  Inventing a wider error taxonomy nothing branches on is YAGNI.
- **Applies to:** batches 1, 2, 3, 4

### Decision: tests are Tier 1, untagged, in-package, against a real status file

- **Decision:** every test file in `internal/shedengine` is untagged, declares `package shedengine`, and drives a real status file under `t.TempDir()` through `internal/state`.
  No mocked persistence, no `TestMain`, no `exec.Command`, no git, no `time.Sleep` of one second or more.
- **Rationale:** the Test Tier Purity Invariant plus the discussion's `tier1-fake-producer-tests` decision — the `internal/state` round-trip is the thing most likely to break, so mocking it would make the crash-recovery test prove nothing.
  A `TestMain` calling `gitkit.HermeticGitEnv()` would be actively wrong here: no test in this package spawns git, and adding one would imply otherwise (the Hermetic Git Test Environment Invariant is not engaged).
- **Applies to:** batches 1, 2, 3, 4, 5

### Decision: a `history` entry is appended on every producer return except the cancellation path

- **Decision:** step 5 appends a `HistoryEntry` for the call that just returned in all of: `Done`, `Stuck`, an unrecognised `Outcome`, and a non-nil `error` with a healthy `ctx`.
  It is skipped in exactly one case — a non-nil `error` with `ctx.Err() != nil`, which routes to the pause exit.
  On the error path the entry's `outcome` field records whatever `Outcome` value the producer returned, which may be the empty string.
- **Rationale:** `shed.md`'s step 5 is unconditional ("append ... persist") and step 6 routes on the already-appended result; the discussion's `ctx-cancellation-as-pause` decision states the cancellation carve-out as an explicit exception ("No `history` entry is appended on the cancellation path"), which only reads as an exception if the error path otherwise appends.
  The discussion's `unrecognised-outcome-is-an-engine-error` decision independently pins the append for the unrecognised case ("The `history` entry is still appended, recording the literal value received").
- **Applies to:** batches 2, 3, 4, 6

### Decision: steps 5 and 6 are one `state.UpdateJSON` call per iteration, routing computed first

- **Decision:** each loop iteration computes its route (next `current_producer`, next `state`, next `error`, whether to clear `pause_requested`) entirely in memory, then commits all of it — including the `history` append and the recomposed `activity` — in a single `state.UpdateJSON` mutate.
  The mutate returns an error when `UpdateJSON` reports `found == false`, aborting the write.
- **Rationale:** the discussion's `reread-and-merge-persist` decision.
  Two writes would let a crash between them re-call the finished producer and append a duplicate `history` entry; a missing `found` guard would let `Shed` silently seed a status file it swore never to create.
- **Applies to:** batches 2, 3, 4, 6

### Decision: docs land in this task's squash-merge commit, in their own batch

- **Decision:** the `CONSTRAINTS.md` invariant (batch 5) and the `manifest/designs/shed.md` / `docs/overview.md` / `manifest/roadmap.md` pass (batch 6) are separate batches rather than folded into the code batches.
- **Rationale:** CLAUDE.md's task-completion rule requires the module doc, `docs/overview.md`, and `CONSTRAINTS.md` "in the same commit" as the code.
  This task squash-merges to `main`, so every batch lands as one commit there — the rule is satisfied at the granularity that reaches `main`.
  Splitting them out keeps the `shed.md` whole-document reconciliation (the single largest and most judgment-heavy piece of work in this task) from competing for attention with the Go implementation.
- **Applies to:** batches 5, 6

### Decision: `manifest/designs/shed.md` survives this task rather than being deleted

- **Decision:** `shed.md` is reconciled and re-bannered, not deleted, even though the skeleton it describes now ships.
- **Rationale:** the Documentation Lifecycle's delete-on-landing rule applies to a design doc whose module has landed whole.
  `shed.md` also describes the three engine adapters, which remain their own Planned roadmap item and whose entry links to this doc — deleting it would break both that link and the Markdown Link Integrity invariant.
  The discussion's `docs-and-roadmap` decision pins a status-banner update, not a deletion.
- **Applies to:** batch 6

### Decision: Go doc comments follow the repo's existing engine-package style

- **Decision:** every new `.go` file opens with a file-level comment naming what the file holds and why, in the shape `internal/treadleengine/result.go` and `internal/treadleengine/runner.go` use.
  `doc.go` carries the package doc with `# `-prefixed headings, in the shape `internal/treadleengine/doc.go` uses.
  Every exported identifier carries a doc comment starting with its own name.
- **Rationale:** the `golang:golang-comments` skill's rules plus the shipped in-repo precedent; the Documentation Lifecycle makes the package doc the durable home for `Shed`'s rationale once `shed.md` stops being the only description.
- **Applies to:** batches 1, 2, 5

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `CONSTRAINTS.md`
- `docs/overview.md`
- `internal/shedengine/activity.go`
- `internal/shedengine/activity_test.go`
- `internal/shedengine/doc.go`
- `internal/shedengine/errors.go`
- `internal/shedengine/producer.go`
- `internal/shedengine/run.go`
- `internal/shedengine/run_pause_test.go`
- `internal/shedengine/run_persist_test.go`
- `internal/shedengine/run_routing_test.go`
- `internal/shedengine/seam_enforcement_test.go`
- `internal/shedengine/shed.go`
- `internal/shedengine/status.go`
- `internal/shedengine/status_test.go`
- `internal/shedengine/testsupport_test.go`
- `internal/shedengine/validate.go`
- `internal/shedengine/validate_test.go`
- `manifest/designs/shed.md`
- `manifest/roadmap.md`
