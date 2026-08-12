# Plan: fabric: close the corrindex two-phase read-modify-write race (slice 15)

```yaml
task: 'fabric: close the corrindex two-phase read-modify-write race (slice 15)'
slug: 'fabric-corrindex-record-race'
approved: false
started: '20260812-102844'
parent: 'main'
root: ""
verify: go build ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: state-updatejson
    file: 01-state-updatejson.md
    depends-on: []
    verify: go test ./internal/state/...
  - number: 2
    name: corrindex-record-single-phase
    file: 02-corrindex-record-single-phase.md
    depends-on: [1]
    verify: go test ./internal/fabricengine/...
  - number: 3
    name: campaign-docs-fold
    file: 03-campaign-docs-fold.md
    depends-on: [2]
    verify: go test ./internal/lyxcwd/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: the fix shape is a state-level update primitive

- **Decision:** close `record()`'s two-phase window by adding `state.UpdateJSON` — a read-modify-write primitive holding one exclusive lock across read, mutate and atomic write — and routing `corrIndex.record` through it.
  Do **not** give `RebuildIndex` or `refreshCorrIndexAfterSwitch` the weft write lock.
- **Rationale:** `record()` takes no write lock in its own frame; `state.WriteJSON` acquires and releases `path+".lock"` internally, and `lock.AcquireWriteLock` calls `flock.New(lockPath)` fresh on every call, so a nested acquire from `corrindex.go` opens a second file descriptor on the same file and self-deadlocks.
  The weft-lock alternative rests on a whole-package deadlock claim every future caller must preserve, and would still leave `record()` two-phase against any writer that does not take the weft lock.
- **Applies to:** all batches

### Decision: `UpdateJSON` cannot be composed from `ReadJSON` + `WriteJSON`

- **Decision:** extract lock-free cores out of `ReadJSON` and `WriteJSON` and re-express all three functions on top of them.
  An implementation that acquires the lock and then calls the existing exported functions is banned.
- **Rationale:** both exported functions acquire `lockPath` internally, so that composition **hangs** rather than failing — the worst way to discover it, especially from inside a test that also holds the lock.
  `ReadJSON`'s and `WriteJSON`'s observable behaviour is unchanged by the extraction; only their bodies change.
- **Applies to:** state-updatejson

### Decision: this task closes one direction of the race, not both

- **Decision:** state the guarantee precisely everywhere it is written down (`corrindex.go`'s doc comment, `internal/state/doc.go`, `internal/fabricengine/doc.go`, the roadmap Done entry): `record()` is serialised against every other *write* to the index file, so it can no longer compose its payload from a superseded base.
  It is **not** serialised against `RebuildIndex`'s scan-to-write span.
- **Rationale:** `RebuildIndex` is itself two-phase — `scanWarpSHATrailers` reads git, then `state.WriteJSON` writes — and the scan is not under the index file's lock, so the interleaving *scan → `record()` writes → rebuild writes* still loses the recorded entry.
  Same shape, same LOW severity, same self-healing property: accepted by name, not overlooked.
  No prose in this task may imply the race is closed outright.
- **Applies to:** all batches

### Decision: no new `CONSTRAINTS.md` invariant

- **Decision:** add no invariant to `CONSTRAINTS.md`.
  The read-modify-write rule lands as a godoc package comment in a **new** `internal/state/doc.go`.
- **Rationale:** adoption stays at one consumer, so an invariant of the form "every locked-JSON read-modify-write goes through `UpdateJSON`" would be false on the day it lands, and a false invariant is worse than none.
  It lands in `doc.go` rather than `state.go` because `state.go`'s existing header block is separated from its `package state` clause by a blank line, making it a detached file comment that never appears in `go doc`.
- **Applies to:** state-updatejson

### Decision: Test Tier Purity — the new fabricengine test stays Tier-1

- **Decision:** the reproducing test in `internal/fabricengine/corrindex_test.go` stays untagged, uses explicit `t.TempDir()` paths, and spawns no git.
- **Rationale:** `corrindex.go`'s header comment records that keeping git out of that file is what lets its tests stay untagged Tier-1 under the Test Tier Purity Invariant.
  The test needs no goroutines either — it drives `record()` against an external write, which is deterministic.
- **Applies to:** corrindex-record-single-phase

### Decision: markdown prose uses semantic line breaks

- **Decision:** every `.md` file edited in this task keeps one sentence per line, breaking additionally at internal independent-clause boundaries.
  Never hard-wrap at a fixed column; never use trailing double-spaces or a backslash for a break.
- **Rationale:** `CLAUDE.md`'s markdown rule, which applies to all prose paragraphs and list items in the repo, not only newly written ones.
- **Applies to:** campaign-docs-fold

### Decision: repointed links name their target section in prose, never by anchor

- **Decision:** every reference repointed at `internal/fabricengine`'s package doc names the target section in prose — e.g. "see `internal/fabricengine`'s package doc, \"The destruction chokepoint\"" — and never appends a `#anchor` fragment to a `.go` target.
- **Rationale:** `internal/lyxcwd/docslink_test.go` skips the anchor check for non-`.md` targets, so a `doc.go#some-section` anchor would pass the checker while being a dead link on GitHub.
  A bare `.go` target is already a first-class case for that test, so it does not trip the Markdown Link Integrity Invariant.
- **Applies to:** campaign-docs-fold

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `internal/fabricengine/corrindex.go`
- `internal/fabricengine/corrindex_test.go`
- `internal/fabricengine/doc.go`
- `internal/state/doc.go`
- `internal/state/state.go`
- `internal/state/update_test.go`
- `manifest/designs/fabric-unified-view.md`
- `manifest/designs/fabric-windows-verification.md`
- `manifest/designs/gitexec-error-shape.md`
- `manifest/designs/lyxtest-real-hubs.md`
- `manifest/roadmap.md`
