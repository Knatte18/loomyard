# Plan: Scout owns its own lyxcwd-based geometry accessors (drop Options.AnchorRoot threading)

```yaml
task: "Scout owns its own lyxcwd-based geometry accessors (drop Options.AnchorRoot threading)"
slug: "scout-lyxcwd-accessors"
approved: false
started: "20260808-082459"
parent: "main"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: location-threading
    file: 01-location-threading.md
    depends-on: []
    verify: go build ./... && go vet -tags scout ./internal/scoutengine/... && go test ./internal/scoutengine/... ./internal/scoutcli/... ./internal/lyxcwd/... ./cmd/lyx/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: anchor-stays-worktreepath

- **Decision:** `DaemonStateFile`/`DaemonLock` join onto `l.WorktreePath()`, never `l.AnchorPath()`.
  Both carry a `// TODO(dotlyx):` marker naming them as candidates for the `WorktreePath` → `AnchorPath` migration when `.lyx` gets a single owner.
- **Rationale:** `cmd/lyx/constructoranchoring_test.go` pins a three-group anchoring table and asserts these two accessors stay byte-identical at a nested `AnchorRel`, grouped with `logger.WorktreeLogsDir` as `.lyx`-ephemeral.
  Switching to `AnchorPath()` breaks `TestConstructorAnchoring_SubpathAnchored` and reverses a recorded decision another task owns.
  The CONSTRAINTS bullet mandating `AnchorPath()` is scoped to a module's *durable*-storage subdirectory;
  `.lyx` is ephemeral, so it does not bind here.
- **Applies to:** all batches

### Decision: field-named-layout

- **Decision:** the `scoutengine.Options` field holding the `*lyxcwd.Location` is named `Layout`, not `Location` and not `WorktreeRoot`.
  Local variables and parameters carrying it are named `layout` (production) or `l` (tests, where a fixture `l` already exists).
- **Rationale:** one vocabulary across the repo — `websterengine.RunDeps.Layout`, `buildercli`/`webstercli`'s `c.layout`, `perchengine`'s `layout` field.
  The rename also clears a Cwd Resolution Invariant violation: `root` is reserved for the git worktree root, which `WorktreeRoot` does not hold out-of-hub.
- **Applies to:** all batches

### Decision: out-of-hub-synthesized-location

- **Decision:** when `lyxcwd.Resolve(cwd)` fails, `scoutcli` synthesizes the `Location` by hand from the absolute target directory as `&lyxcwd.Location{HubPath: filepath.Dir(abs), WorktreeName: filepath.Base(abs), AnchorRel: "."}`, where `abs` is `filepath.Abs(targetDir)`.
  The synthesized value is a fiction outside `WorktreePath()`: `HubPath` is merely the parent of the target directory and names no real hub, `RepoName` is left zero, and `AnchorPath()` is meaningless because `AnchorRel` was assumed rather than read from a `.fabric-anchor` marker.
  It is therefore contractually consumed by `DaemonStateFile`/`DaemonLock` alone and must never be widened to feed a caller that reads `AnchorPath()`, `HubPath`, or `RepoName`.
- **Rationale:** `WorktreePath()` is `filepath.Join(HubPath, WorktreeName)`, so this yields exactly `abs` — byte-identical to today's `resolveWorktreeRoot` fallback, with no new failure modes and no new git spawns.
  Hand-building a `Location` outside `internal/lyxcwd` has only two production precedents, both inside the geometry owner itself, so the limit must be stated in `resolveLocation`'s doc comment where the next person to reach for this branch will see it.
  **Scope of the byte-identity claim:** asserted for non-root directories only.
  A filesystem or volume root is a degenerate case where the `Dir`/`Base`-then-`Join` round trip is not obviously an identity, and it was not verified.
  Do not add a special case for it and do not claim byte-identity there — `--target-dir /` names no buildable project, and the pre-existing fallback has the same shape.
- **Applies to:** all batches

### Decision: no-nil-layout-check

- **Decision:** `Options.Layout` is documented as required.
  No nil check is added anywhere in `scoutengine`.
- **Rationale:** matches the `websterengine.RunDeps.Layout` precedent, which never nil-checks either.
  A nil deref is the correct immediate signal for a caller that skipped a required field, and `scoutcli` — the only production caller — now always supplies one.
  Known limit, deliberately accepted: `Layout` is dereferenced only inside `ensureSupervised`, reached only when `entry.HasNativeDaemon` is true (Go alone in V1), so the fail-fast is language-conditional rather than universal.
- **Applies to:** all batches

### Decision: byte-identical-except-one-delta

- **Decision:** every resolved path must be byte-identical before and after this change, in-hub and out-of-hub, with exactly one intended exception: out-of-hub `assert-no-callers`, whose daemon-state path moves from process-cwd-relative to `filepath.Abs(targetDir)`-rooted.
- **Rationale:** the task is a signature refactor, not a behaviour change.
  If any expected value in any of the nine *existing* test files has to change, the implementation drifted — treat it as a signal, not a test to update.
  No existing test covers the out-of-hub `assert-no-callers` path, which is why the one intended delta touches no existing expected value.
  Do not read the byte-identical rule as grounds to back that delta out;
  closing it is the point of card 5.
- **Applies to:** all batches

### Decision: never-touch-lspclient

- **Decision:** `internal/scoutengine/lspclient.go` is never edited and never gains a `lyxcwd` import.
- **Rationale:** it is under a file-scoped import guard (stdlib plus `internal/logger` only, `lspclient_guard_test.go`) that keeps the ported stdio LSP client liftable back out of lyx.
  It has nothing to do with daemon geometry.
- **Applies to:** all batches

### Decision: out-of-scope-files

- **Decision:** `CONSTRAINTS.md`, `manifest/roadmap.md`, and `docs/overview.md` are not touched.
- **Rationale:** this adds no new cross-cutting invariant — it is an application of the existing Cwd Resolution Invariant.
  No module is added, the execution stack is unchanged, and no CLI surface changes, so no Documentation Lifecycle trigger fires.
  The `.lyx`-anchoring rule specifically must *not* be written into `CONSTRAINTS.md`: it is temporary, the dotlyx-hygiene task is chartered to reverse it, and a second source would contradict `constructoranchoring_test.go`.
- **Applies to:** all batches

### Decision: stale-citation-left-alone

- **Decision:** `internal/scoutengine/ensureserver.go:1-2` cites `manifest/designs/scout-redesign.md`, which does not exist in the tree.
  Leave that citation exactly as it is.
- **Rationale:** pre-existing rot unrelated to this refactor.
  Fixing it here would widen a pure signature change.
  Card 3 rewords the `worktreeRoot` half of that same header sentence and must not touch the citation half.
- **Applies to:** all batches

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically (Move **source** paths are excluded — they disappear, like `Deletes:` tokens).
Cards are the source of truth;
this section is the input `_plan_validate.py`'s `all-files-touched-mismatch` check cross-references against the derived union of every card's `Edits:`/`Creates:`/Move-target paths, to catch drift between the hand/agent-maintained list here and that derived union._

- `cmd/lyx/constructoranchoring_test.go`
- `internal/scoutcli/cli.go`
- `internal/scoutcli/cli_test.go`
- `internal/scoutengine/daemonstate.go`
- `internal/scoutengine/doc.go`
- `internal/scoutengine/ensureserver.go`
- `internal/scoutengine/ensureserver_integration_test.go`
- `internal/scoutengine/ensureserver_test.go`
- `internal/scoutengine/refs.go`
- `internal/scoutengine/refs_integration_test.go`
- `internal/scoutengine/scoutdaemon_test.go`
- `internal/scoutengine/supervised_integration_test.go`
- `internal/scoutengine/supervised_scout_test.go`
- `internal/scoutengine/supervised_test.go`
