# Batch: status-schema-migration

```yaml
task: 'loom: phase-machine scaffolding'
batch: status-schema-migration
number: 1
cards: 2
verify: go test ./internal/loomengine/... ./internal/lyxcwd/... ./cmd/lyx/... && go test -tags integration ./internal/loomengine/...
depends-on: []
```

## Batch Scope

This batch migrates `_lyx/loom/status.json` onto `shedengine.Status` and adds the one missing path accessor `Shed`'s run lock needs, so that batch 2's `internal/loomshed` has both a status file it can actually drive and a declared path for every told value its `Deps` takes.
It is one batch because both cards live entirely inside `internal/loomengine` plus the two `cmd/lyx` guard tests that pin its accessors, and because card 2's schema change is what forces every doc edit in it — the two cannot be reviewed apart from each other.

The external interface batch 2 consumes: `loomengine.LoomRunLock(l)` (new), the rewritten thin `loomengine.Status` product struct (which `loomshed.Seed` marshals into `shedengine.Status.Product`), and a check 4 that tolerates a `Preflight`-only history so a blocked row-1 run is resumable.

Batch-local decision, differing from nothing in `## Shared Decisions` but worth stating: `loomengine` gains a production import of `internal/shedengine`. That direction is legal — the Shed Producer-Seam Invariant constrains what `shedengine` may import, never who may import it.

## Cards

### Card 1: declare Shed's run-lock path as `loomengine.LoomRunLock`

- **Context:**
  - `internal/lyxdirs/dirs.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/loomengine/config.go`
  - `internal/loomengine/loomstatus_test.go`
  - `cmd/lyx/constructoranchoring_test.go`
  - `cmd/lyx/notransients_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `LoomRunLock(l *lyxcwd.Location) string` to `internal/loomengine/config.go`, immediately after the existing `LoomStatusLock`, returning `filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, "loom", "run.lock")`. It is the third path `Shed` is told, distinct from both `LoomStatusFile` and `LoomStatusLock`: `internal/state` acquires `StatusLockPath` with the blocking form, which is why `Shed.validate()` rejects `LockPath == StatusLockPath` outright, so one shared file would hang on the first persist rather than fail. Its doc comment states that, states the AnchorPath anchoring, and states the same `loom/` product-scoping reason `LoomStatusLock`'s own comment already gives. In `internal/loomengine/loomstatus_test.go`, add `TestLoomRunLock` and `TestLoomRunLock_UnanchoredEqualsWorktreePath`, modelled line-for-line on the existing `TestLoomStatusLock` and `TestLoomStatusLock_UnanchoredEqualsWorktreePath` pair, asserting the `run.lock` leaf under the `.lyx` tree. In `cmd/lyx/constructoranchoring_test.go`, add a `loomengine.LoomRunLock` row to the `.lyx` group of both `TestConstructorAnchoring_Unanchored` and `TestConstructorAnchoring_SubpathAnchored`, and to the `dotLyxConstructors` map in the latter's prefix-exclusion guard. In `cmd/lyx/notransients_test.go`, add a `loomengine.LoomRunLock` entry to `transientSet`. Do not add it to `durableSet` — it is never-tracked.
- **Commit:** `feat(loomengine): declare Shed's run-lock path as LoomRunLock`

### Card 2: migrate `_lyx/loom/status.json` onto `shedengine.Status`

- **Context:**
  - `internal/shedengine/status.go`
  - `internal/shedengine/run.go`
  - `internal/state/state.go`
  - `internal/loomengine/report.go`
  - `internal/preflight/doc.go`
- **Edits:**
  - `internal/loomengine/status.go`
  - `internal/loomengine/coherence.go`
  - `internal/loomengine/preflight.go`
  - `internal/loomengine/coherence_test.go`
  - `internal/loomengine/preflight_integration_test.go`
  - `contracts/specs/loom-status-spec.md`
  - `internal/shedengine/doc.go`
  - `manifest/designs/shed.md`
  - `manifest/designs/loom.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite `loomengine.Status` in `internal/loomengine/status.go` as the thin product payload carried in `shedengine.Status.Product`: exactly `Slug string` (`json:"slug"`), `Parent string` (`json:"parent"`), and `StartSha *string` (`json:"start_sha"`). Delete `loomengine.HistoryEntry` — the shell's history is `shedengine.HistoryEntry` now, and nothing else in the repo names the old type. Rewrite `checkCoherence` in `internal/loomengine/coherence.go` to the signature `checkCoherence(shed shedengine.Status, product Status) []Failure`, and delete the now-unused `validPhases` and `validStages` maps. It keeps reporting through the same two check IDs: `CheckSeedIncoherent` for content violations, `CheckHalfFinished` for the fresh-start invariants. Its rules become: `product.Slug` and `product.Parent` are mandatory, an empty string counting as absent exactly as today; `shed.CurrentProducer` must equal `"Preflight"`, since that is the only way check 4 is ever reached; `shed.State` must be one of the five `shedengine` members and must not be `shedengine.StateDone`, a finished run; `shed.Error` is tolerated at any value including non-empty, because it is the previous halt's reason a human resumes after reading; `shed.Activity` is never validated at all, because `Shed` recomposes it mechanically on every persist and validating it would assert `Shed`'s arithmetic against itself; every `shed.History[i].Outcome` must be one of `shedengine.Done` or `shedengine.Stuck`; every `shed.History[i].At` must be RFC3339 UTC via the existing `isRFC3339UTC` helper, which stays. The fresh-start check replaces today's `len(s.History) != 0` test with a narrower one: a history entry naming any producer other than `"Preflight"` is a `CheckHalfFinished` failure, while entries naming `"Preflight"` itself are tolerated. This narrowing is the whole point of the rewrite and needs its own comment stating why: `shedengine.Run` appends a history entry before persisting `StateBlocked`, including on the `OnStuck: ""` path, so a `Stuck` at row 1 leaves one `Preflight` entry behind, and the old test would then fail `CheckHalfFinished` on every subsequent resume attempt, forever. `product.StartSha` and `shed.PauseRequested` keep their existing fresh-start treatment: a non-nil `StartSha` or a true `PauseRequested` is still a `CheckHalfFinished` failure. In `internal/loomengine/preflight.go`, change `runCheck4`'s read from `state.ReadJSONStrict[Status]` to `state.ReadJSONStrict[shedengine.Status]`, then unmarshal the shell's `Product` field into a `Status` and pass both to `checkCoherence`. A `Product` that fails to unmarshal is a determined `CheckSeedIncoherent` verdict, not an infra error; an absent or null `Product` decodes to the zero `Status`, whose empty `Slug`/`Parent` the mandatory-field rules then report — do not special-case it. Everything else in `runCheck4` — the `check3BlocksSeed` derivation, the `os.IsNotExist` branch, the `state.ErrDecode` branch, the vanished-between-stat-and-read synthesized error, the `MkdirAll` of the lock's parent — stays byte-identical. Rewrite `internal/loomengine/coherence_test.go` as a table over the new signature: each mandatory product field empty in turn; each `shedengine` `State` member tolerated except `StateDone`; a non-empty `Error` tolerated; `CurrentProducer` naming something other than `Preflight`; an invalid `history[].outcome`; a non-RFC3339 and a non-UTC `history[].at`; a set `StartSha`; a true `PauseRequested`. It must also carry an explicitly named regression test for the retry deadlock: a history of only `Preflight` entries passes the fresh-start check, and a history containing any entry naming a later producer fails it. In `internal/loomengine/preflight_integration_test.go`, retarget every seed writer onto the new on-disk shape — `seedValidStatus` writes a `shedengine.Status` with `CurrentProducer: "Preflight"`, `State: shedengine.StateRunning`, empty history, and a `Product` carrying the `Status` payload; the two `TestPreflight_SeedHalfFinished` cases become a non-`Preflight` history entry and a set `StartSha` respectively. Do not weaken any assertion in that file — every test must still assert the same check set it asserts today. Then update, in this same commit, every doc the change falsifies. Rewrite `contracts/specs/loom-status-spec.md`'s schema, per-field notes, validation checklist, and both worked examples against the `shedengine.Status` shape, with loom's three fields shown inside `product`, and delete its trailing stale-content note, which this rewrite resolves. Per the Producer Pointer-Rule Invariant, point at `internal/shedengine`'s own package documentation for the shell's field semantics rather than restating them; the doc's own content is the loom-specific half — what lives in `product`, and what check 4 additionally requires. Rewrite the `# Divergence from loom's status schema` paragraph in `internal/shedengine/doc.go`: the two schemas no longer diverge, loom's fields live in `product`, and the sentences claiming reconciliation "is loom's own later rewiring work" and that "a Shed-written file would still fail loom's coherence check" are both false and must go. This doc-comment edit adds no import, so the Shed Producer-Seam Invariant is untouched; make no other change anywhere in `internal/shedengine`. In `manifest/designs/shed.md`, rewrite the `**product carries no compatibility claim for loom's shipped schema.**` paragraph and the seed/external-writer sentence just above it that depends on it, so neither claims the old top-level `phase`/`stage`/`narration` shape. In `manifest/designs/loom.md`, reword the State-&-contracts bullet that calls the status file the source of truth for "current phase, current review stage" onto `current_producer` plus the history trail, and reword the following bullet's "human-readable *current-activity* narration" onto Shed's mechanically-composed `activity` — its `now:`/`last:`/`wait:` example survives verbatim, so only the retired term changes, not the described behaviour. In `docs/overview.md`, reword the `_lyx/` bullet's "current phase, review round, verdict history" parenthetical onto the new vocabulary. Do not touch `contracts/specs/webster-spec.md` or `manifest/designs/self-report.md`, whose references are pointers that assert nothing about the file's shape, and do not touch the `loom-status-spec.md` mention in the kept-contract-docs bullet of `docs/overview.md`, which stays true.
- **Commit:** `refactor(loomengine): migrate loom status.json onto shedengine.Status`

## Batch Tests

`verify:` runs `go test ./internal/loomengine/... ./internal/lyxcwd/... ./cmd/lyx/...` plus a second, integration-tagged pass over `./internal/loomengine/...`.

- `./internal/loomengine/...` untagged covers the rewritten `coherence_test.go` — the whole table plus the named Preflight-retry-deadlock regression test — and `loomstatus_test.go`'s new `LoomRunLock` pair.
- The `-tags integration` pass is required, not optional: card 2 edits `internal/loomengine/preflight_integration_test.go`, which carries `//go:build integration` and is invisible to the untagged run. Without it, a broken seed rewrite would pass verify and surface only at the done gate.
- `./cmd/lyx/...` covers the two accessor guards card 1 edits (`constructoranchoring_test.go`, `notransients_test.go`) and, unedited but load-bearing here, `tierpurity_test.go` and `hermeticenv_test.go`.
- `./internal/lyxcwd/...` covers `docslink_test.go`, the Markdown Link Integrity guard over the five docs card 2 rewrites.
