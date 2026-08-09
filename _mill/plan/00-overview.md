# Plan: fabric: store the warp-URL binding in weft:main; fold bootstrap into clone (slice 10)

```yaml
task: 'fabric: store the warp-URL binding in weft:main; fold bootstrap into clone (slice 10)'
slug: fabric-warp-binding-in-weft
approved: true
started: '20260809-085424'
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
    name: warp-binding core
    file: 01-warp-binding-core.md
    depends-on: []
    verify: go test ./internal/fabricengine/ -run 'TestNormalizeWarpURL|TestResolveEffectiveWarpURL|TestWarpBindingReadWrite|TestWarpURLTransportIdentity'
  - number: 2
    name: probe and clone flip
    file: 02-probe-and-clone-flip.md
    depends-on: [1]
    verify: go build ./... && go test -tags integration ./internal/fabricengine/
  - number: 3
    name: cli surface
    file: 03-cli-surface.md
    depends-on: [2]
    verify: go build ./... && go test -tags integration ./internal/fabriccli/ && go test ./cmd/lyx/
  - number: 4
    name: clone integration tests
    file: 04-clone-integration-tests.md
    depends-on: [3]
    verify: go test -tags integration ./internal/fabricengine/
  - number: 5
    name: reconcile backfill
    file: 05-reconcile-backfill.md
    depends-on: [4]
    verify: go build ./... && go test -tags integration ./internal/fabricengine/ ./internal/fabriccli/
  - number: 6
    name: docs and sandbox suites
    file: 06-docs-and-sandbox.md
    depends-on: [5]
    verify: go test ./internal/lyxcwd/ ./cmd/lyx/
```

## Shared Decisions

_Cross-cutting decisions every batch inherits: naming conventions, error-handling posture, test frameworks, style/lint constraints.
One subsection per decision.
Batch-local decisions live in each batch file._

### Decision: record format and ownership

- **Decision:** the warp binding is a plain single-line file `.lyx-warp` at the board root (`<BoardDir>/.lyx-warp`, tracked at the root of `weft:main`), holding the warp URL plus a trailing newline and nothing else — no subpath, no YAML.
  Its filename constant and its read/write/normalize helpers live in a new `internal/fabricengine/warpbinding.go`;
  the file is committed onto `weft:main` by the existing `Bolt` board-commit call in the CLI layer, never by the engine.
- **Rationale:** exactly mirrors the shipped `.lyx-anchor` precedent (`internal/lyxcwd/anchor.go`), which is read with `os.ReadFile` + `strings.TrimSpace` and needs no YAML dependency.
  The Cwd Resolution Invariant bars the reader from `internal/lyxcwd`; the binding is fabric's own illusion plumbing, so it belongs in `fabricengine`.
- **Applies to:** all batches

### Decision: engine returns typed values, CLI owns the envelope

- **Decision:** every new engine function returns `(T, error)` and never touches `io.Writer`, exit codes, or the output envelope. `internal/fabriccli` maps results onto `output.Ok`/`output.Err`.
  `internal/fabricengine` must never import `internal/configsync` (documented `fabricengine → configsync → configreg → fabricengine` cycle), so the `Bolt` commit and push for both clone and reconcile stay CLI-side.
- **Rationale:** the CLI/Cobra Invariant and the existing `CloneResult` doc comment both pin this split.
- **Applies to:** all batches

### Decision: conflict-error wording names the remedy generically

- **Decision:** the differing-URL conflict error reads `recorded warp binding %s does not match %s; refusing to re-point. If the warp repo moved, edit .lyx-warp in the hub's _board worktree and commit.`
  The discussion's literal wording spells `<hub>/_board`; that interpolation is deliberately dropped because the conflict is raised by a pure resolver that runs *before* any hub directory exists and therefore has no hub path to name.
- **Rationale:** the whole point of the pre-hub probe is that a conflict aborts with zero residue, which means there is no `<hub>` to quote at that moment.
  The remedy text still names the exact file and the exact worktree, which is what the operator needs.
- **Applies to:** all batches

### Decision: absence is proved by `git ls-tree`, never by a failed read

- **Decision:** every "is this path in HEAD" question during the probe is answered with `git ls-tree HEAD --name-only -- <path>` (exit 0 with empty stdout means absent, exit 0 with the path echoed means present, nonzero is a hard error).
  `git show HEAD:<path>` runs only after `ls-tree` has confirmed presence, so any failure from it is a genuine error.
  This applies identically to `.lyx-warp` and to the old-order guard's `.lyx-anchor` check.
- **Rationale:** a missing path and a broken repo both make `git show` exit nonzero, so a failed read cannot classify itself; without an explicit presence probe a network outage would be reported as `unbound weft`.
- **Applies to:** batch 2, batch 4

### Decision: URL comparison is normalized, transport swaps still differ

- **Decision:** `normalizeWarpURL` strips one trailing `/`, then one trailing `.git`, then lowercases a recognised `<scheme>://<host>` prefix only.
  A string with no `<scheme>://` prefix (every local filesystem fixture path, including a Windows drive letter) keeps its case byte-for-byte;
  the two trailing strips still apply to it.
  A transport swap (`git@github.com:u/r.git` against `https://github.com/u/r`) normalizes to different values and is therefore a conflict, both at clone and as a reported divergence at reconcile.
- **Rationale:** trailing `/` and `.git` are spelling noise; a transport swap is a real difference that would leave the record describing something other than what was cloned.
  One normalizer serves both clone and reconcile so the two verbs can never disagree about what "same URL" means.
- **Applies to:** all batches

### Decision: the DAG is deliberately linear

- **Decision:** every batch depends on exactly its predecessor, even where a narrower dependency would be technically sufficient.
  Batch 4 is the clearest case: its cards exercise `CloneHub` through the options struct delivered by batch 2 and touch nothing under `internal/fabriccli`, so `depends-on: [2]` would be equally correct and would let batches 3 and 4 schedule independently.
- **Rationale:** all batches execute in this one worktree on this one branch, so parallel batches would interleave commits on a shared history for no benefit at this size — six batches, none long-running.
  The linear chain also means each batch's verify runs against a tree that already contains every earlier batch, which is what makes the narrow per-batch verify scopes safe.
- **Applies to:** all batches

### Decision: docs land in this task, commits squash on merge

- **Decision:** the documentation updates live in their own final batch rather than being scattered across the code batches.
- **Rationale:** `CLAUDE.md` requires docs in the same commit as the code; mill merges a task branch to `main` as a single squash commit, so every batch in this plan lands as one commit on `main` and the rule is satisfied.
  Keeping the doc edits together avoids six partial rewrites of the same design-doc section.
- **Applies to:** batch 6

### Decision: test tier and hermetic-env placement

- **Decision:** every new test that spawns git lives in a file whose first non-empty line is `//go:build integration`, in `internal/fabricengine` or `internal/fabriccli` (both already carry a `TestMain` calling `lyxtest.HermeticGitEnv()`).
  The only untagged new test file is `internal/fabricengine/warpbinding_test.go`, which is pure and must not name `gitexec.RunGit`, `exec.Command`, or `lyxtest.Copy*` even inside a comment or string literal.
- **Rationale:** Test Tier Purity Invariant (raw substring match) and Hermetic Git Test Environment Invariant.
- **Applies to:** batch 1, batch 2, batch 3, batch 4, batch 5

### Decision: probe minimality degrades silently under local fixtures

- **Decision:** the probe clone passes `--depth 1 --filter=tree:0 --no-checkout --single-branch`.
  Against a local bare repo path — every fixture in this repo's test suite — git ignores `--filter` and `--depth` and performs an ordinary hardlinked clone, emitting warnings on stderr.
  The implementation must key off the exit code alone and must never treat those warnings as a failure.
- **Rationale:** partial clone needs `uploadpack.allowFilter` on the server; correctness is unaffected when it is unavailable, only minimality is.
- **Applies to:** batch 2, batch 4

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically._

- `CONSTRAINTS.md`
- `docs/overview.md`
- `docs/sandbox-hub.md`
- `internal/fabriccli/cli_test.go`
- `internal/fabriccli/clone.go`
- `internal/fabriccli/fabric.go`
- `internal/fabricengine/boardjunction_integration_test.go`
- `internal/fabricengine/clone.go`
- `internal/fabricengine/clone_adopt_test.go`
- `internal/fabricengine/clone_test.go`
- `internal/fabricengine/doc.go`
- `internal/fabricengine/export_test.go`
- `internal/fabricengine/reconcile.go`
- `internal/fabricengine/warpbinding.go`
- `internal/fabricengine/warpbinding_clone_integration_test.go`
- `internal/fabricengine/warpbinding_reconcile_integration_test.go`
- `internal/fabricengine/warpbinding_test.go`
- `internal/fabricengine/warpprobe.go`
- `manifest/designs/fabric-unified-view.md`
- `manifest/roadmap.md`
- `tools/sandbox/SANDBOX-CORE-SUITE.md`
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- `tools/sandbox/main.go`
