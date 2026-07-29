# Batch: registry-and-state-foundations

```yaml
task: 'codeintel V1 — LSP-backed lookups (Go-only, CLI + EnsureServer)'
batch: registry-and-state-foundations
number: 1
cards: 4
verify: go test -count=1 ./internal/codeintelengine/... ./internal/hubgeometry/...
depends-on: []
```

## Batch Scope

Lands the two pieces of ground truth every later batch reads from: the
registry's new per-language daemon-strategy fields, and the `hubgeometry`
accessor that names where the codeintel daemon's runtime state lives. Zero
behavior change — no call site reads the new registry fields yet (that is
batch 5's `ensureServer` dispatch), and no call site writes to the new
state-file path yet (that is batch 4). This batch exists purely so those
later batches have stable, tested identifiers to build on.

The external interface the rest of the plan consumes: `Entry.PinnedVersion
string` and `Entry.HasNativeDaemon bool` (both yaml-tagged, on
`internal/codeintelengine.Entry`), and two new methods on
`hubgeometry.Layout` — `CodeintelDaemonStateFile(lang string) string` and
`CodeintelDaemonLock(lang string) string`.

## Cards

### Card 1: Add `PinnedVersion`/`HasNativeDaemon` to `Entry`, populate Go

- **Context:**
  - `internal/codeintelengine/load.go`
  - `internal/codeintelengine/detect.go`
- **Edits:**
  - `internal/codeintelengine/registry.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add two new fields to `Entry`: `PinnedVersion string`
  with tag `` `yaml:"pinned_version"` `` and `HasNativeDaemon bool` with
  tag `` `yaml:"has_native_daemon"` ``, placed after the existing
  `InstallHint` field. Update `builtins()`'s `"go"` entry to set
  `PinnedVersion: "v0.23.0"` and `HasNativeDaemon: true` — `v0.23.0` is
  not an arbitrary choice: it is the exact `gopls` version empirically
  verified during `_mill/discussion.md`'s `native-strategy-wire-compatibility`
  investigation (`gopls -remote=auto` installed via
  `go install golang.org/x/tools/gopls@latest` on 2026-07-29), so pinning
  it means the toolchain manager (batch 2) installs precisely the version
  this plan's own design was validated against. The other four entries
  (`python`, `csharp`, `typescript`, `rust`) get **no** literal added —
  leaving `PinnedVersion`/`HasNativeDaemon` unset in each entry's struct
  literal is correct and sufficient; Go zero-values them to `""`/`false`
  automatically, which is the "third, implicit no-daemon-strategy mode"
  `registry-scope`'s decision requires. Do **not** add either field to
  `validateEntry` — both are optional/Go-only in V1, and `load.go`'s
  `LoadRegistry` whole-entry-replace overlay semantics are unaffected by
  this change (a `servers.yaml` override for a language that omits the
  two new keys still decodes correctly, yaml.v3 leaves unset struct
  fields at their zero values on decode).
- **Commit:** `feat(codeintelengine): add PinnedVersion/HasNativeDaemon to registry Entry`

### Card 2: Extend registry_test.go coverage for the new fields

- **Context:**
  - `internal/codeintelengine/registry.go`
- **Edits:**
  - `internal/codeintelengine/registry_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend the existing test file (do not replace it) with
  three assertions: (1) `builtins()["go"].PinnedVersion == "v0.23.0"` and
  `builtins()["go"].HasNativeDaemon == true`; (2) for each of `"python"`,
  `"csharp"`, `"typescript"`, `"rust"`, `builtins()[lang].PinnedVersion ==
  ""` and `builtins()[lang].HasNativeDaemon == false` — table-driven,
  mirroring this file's existing style; (3) `validateEntry` accepts an
  `Entry` with both new fields at their zero values (i.e. the existing
  four non-Go entries continue to pass validation unchanged — this is a
  regression guard, not new behavior, so assert it against the actual
  `builtins()` entries rather than a hand-built fixture).
- **Commit:** `test(codeintelengine): cover PinnedVersion/HasNativeDaemon on the registry`

### Card 3: Add the codeintel daemon state-file/lock accessor to hubgeometry

- **Context:**
  - `internal/hubgeometry/loomstatus_test.go`
- **Edits:**
  - `internal/hubgeometry/hubgeometry.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add two new methods on `*Layout`, placed immediately
  after `LoomStatusLock` (same section of the file, same
  WorktreeRoot-anchoring rationale): `CodeintelDaemonStateFile(lang
  string) string` returning `filepath.Join(l.WorktreeRoot,
  dotLyxDirName, "codeintel", lang, "daemon.json")`, and
  `CodeintelDaemonLock(lang string) string` returning
  `filepath.Join(l.WorktreeRoot, dotLyxDirName, "codeintel", lang,
  "daemon.lock")`. Both are WorktreeRoot-anchored, not Cwd-anchored — the
  `supervised` daemon must be a worktree-wide singleton per language (two
  invocations from different subdirectories of one worktree share one
  `gopls`, never spawn two), exactly the reasoning `LoomStatusFile`'s doc
  comment already states for the same anchoring choice. Both live under
  `dotLyxDirName` (`.lyx`, the ephemeral/machine-bound directory), never
  `LyxDirName` (`_lyx`, weft-synced) — `.lyx`'s own doc comment already
  names reed's `reed.json`/`reed.lock` as exactly this kind of runtime
  daemon state, and a git-committed PID/socket file would be actively
  wrong (stale the instant it is committed). Write a godoc comment on each
  method following the file's existing convention (see `LoomStatusFile`/
  `LoomStatusLock` immediately above): state the returned path shape,
  the WorktreeRoot-anchoring rationale, and that per-`lang` scoping is
  what lets two different languages' daemons (a future Python
  `supervised` daemon alongside Go's `native` one) coexist without
  colliding on one shared state file.
- **Commit:** `feat(hubgeometry): add CodeintelDaemonStateFile/CodeintelDaemonLock accessors`

### Card 4: Test the new hubgeometry accessor

- **Context:**
  - `internal/hubgeometry/loomstatus_test.go`
  - `internal/hubgeometry/hubgeometry.go`
- **Edits:** none
- **Creates:**
  - `internal/hubgeometry/codeinteldaemon_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Mirror `loomstatus_test.go`'s exact pattern and file
  header style (pure path arithmetic, no spawning, untagged/Tier-1): a
  hand-built `&Layout{WorktreeRoot: ..., Cwd: <a different subdirectory>}`
  proving `CodeintelDaemonStateFile("go")` and `CodeintelDaemonLock("go")`
  both ignore `Cwd` and stay anchored to `WorktreeRoot`, asserting the
  exact expected path
  (`filepath.Join(l.WorktreeRoot, ".lyx", "codeintel", "go",
  "daemon.json")` / `"daemon.lock"`). Add a second pair of assertions with
  `lang = "python"` proving the two languages resolve to distinct,
  non-colliding paths (`.../codeintel/go/daemon.json` vs
  `.../codeintel/python/daemon.json`) — this is the one behavior specific
  to this accessor that `loomstatus_test.go`'s own tests have no
  equivalent of, since `LoomStatusFile`/`LoomStatusLock` are not
  parameterized.
- **Commit:** `test(hubgeometry): cover CodeintelDaemonStateFile/CodeintelDaemonLock`

## Batch Tests

`verify:` runs `go test -count=1 ./internal/codeintelengine/...
./internal/hubgeometry/...` — both packages this batch touches. No
integration tag needed: every card here is pure struct/path-arithmetic
work with no subprocess spawn, consistent with the Test Tier Purity
Invariant.
</content>
