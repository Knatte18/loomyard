# Plan: Give codeintel a persistent, session-long daemon

```yaml
task: "Give codeintel a persistent, session-long daemon"
slug: "codeintel-daemon-persistence"
approved: false
started: "20260729-154426"
parent: "codeintel-v1"
root: ""
verify: null
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches. Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: proc-killpid
    file: 01-proc-killpid.md
    depends-on: []
    verify: go test ./internal/proc/...
  - number: 2
    name: engine-documentsymbol-infile
    file: 02-engine-documentsymbol-infile.md
    depends-on: []
    verify: go test ./internal/codeintelengine/...
  - number: 3
    name: cli-resolution-buildoptions-infile
    file: 03-cli-resolution-buildoptions-infile.md
    depends-on: [2]
    verify: go test ./internal/codeintelcli/...
  - number: 4
    name: engine-supervised-flip
    file: 04-engine-supervised-flip.md
    depends-on: [1, 2, 3]
    verify: go test ./internal/codeintelengine/...
  - number: 5
    name: engine-integration-tests
    file: 05-engine-integration-tests.md
    depends-on: [4]
    verify: go test -tags integration -run=^$ ./internal/codeintelengine/...
```

## Shared Decisions

_Cross-cutting decisions every batch inherits. Batch-local decisions live in each batch file._

### Decision: engine/CLI/proc layering stays intact

- **Decision:** `internal/codeintelengine` stays a leaf that returns typed `(T, error)` — no `io.Writer`, no exit codes, no `internal/output`, no cobra. `internal/codeintelcli` remains the sole envelope-mapping layer (`output.Ok`/`output.Err`, exit codes, batch worst-status). `internal/proc` stays stdlib-only (`os`, `os/exec`, `syscall`). `proc.KillPID` (batch 1) is stdlib-only and does not widen the Codeintelengine Leaf Invariant's allowlist (`proc` is already allowlisted for `IsAlive`/`Detach`). Item 3's `"resolution":"complete"` field is emitted only in the CLI layer (`emitLookupResult`/`classifyLookupError`); the engine result types (`Reference`, `SymbolMatch`) are unchanged.
- **Rationale:** These are machine-enforced invariants (`leaf_enforcement_test.go`, `TestLeafInvariant_AllowlistOnly`); violating one fails `go test`. Keeping the trust-marker in the CLI keeps the engine envelope-free.
- **Applies to:** all batches

### Decision: test tiers and per-batch verify scope

- **Decision:** New unit tests are untagged and process-free (decision-logic helpers, JSON-shape, `documentSymbol` parse via the fake transport, `--in-file`/`buildOptions` query construction). Any test that spawns a real `gopls` or a real daemon is `//go:build integration`, skip-gated on `exec.LookPath("gopls")`, and excluded from the plain `go test` gate — matching the existing `*_integration_test.go` files. `internal/proc` tests may spawn (the package is allowlisted as a directory prefix in `cmd/lyx/tierpurity_test.go`'s `allowedSpawners`), so `killpid_test.go` needs no allowlist edit. Each batch's `verify:` is scoped to its own package; the repo-wide `go test ./...` `done_gate` (already configured) is the final cross-package gate mill-go runs before marking the task done.
- **Rationale:** Test Tier Purity Invariant keeps Tier 1 offline/fast; the pure decision helpers (fallback-trigger, wedged-escalation) are exactly where item 5's bugs live and are unit-covered without a real daemon.
- **Applies to:** all batches

### Decision: docs land in the same commit as the behaviour change

- **Decision:** Doc surfaces are updated in the batch that changes the behaviour they describe: `internal/codeintelengine/doc.go` (method list + resolver mode in batch 2; dispatch arm + wedged-gap limitation in batch 4), the `ensureserver.go` "exactly one live dispatch arm" comment and `refs.go` `Options.WorktreeRoot` comment (batch 4), the `supervised_integration_test.go` "no live path to it in V1" and `ensureserver_integration_test.go` header comments (batch 5, alongside that batch's integration tests), refs/definition `--help`/`Long` text for the new field and flag (batch 3), and the `docs/overview.md` codeintel module-table clause that currently frames native as Go's wired strategy (batch 4). `manifest/roadmap.md` is deliberately **not** touched: per the project's roadmap convention it moves only on planned-item section transitions, not hardening/completion passes, and the discussion decided this explicitly — the durable native-vs-supervised rationale lives in `doc.go`, which batch 4 updates. There is no `manifest/designs/codeintel-*.md` to update (the design doc was deleted on landing; `doc.go` is the durable home). No new cross-cutting invariant, so no `CONSTRAINTS.md` edit. No new module or subcommand (`--in-file` is a flag), so the `helptree`/`registration`/`longlist` pinned sets are unchanged.
- **Rationale:** Documentation Lifecycle + CLI/Cobra help-accuracy are review-blocking obligations; stale strategy prose in `doc.go`/`overview.md` after the flip would be a defect.
- **Applies to:** batches 2, 3, 4, 5

### Decision: LSP wire positions are used as-is on the resolve path

- **Decision:** The `--in-file` `documentSymbol` resolve branch uses the matched symbol's LSP range (`SelectionRange.Start`) directly as the wire position, never round-tripping through the byte-column `Position`/`toLSPPosition` type — identical to the discipline `resolvePosition`'s existing workspace/symbol branch already follows.
- **Rationale:** A byte-column round-trip misconverts any line with a multi-byte rune before the symbol; the server already returns a correct 0-based-line / UTF-16-char position.
- **Applies to:** batch 2

### Decision: `daemonStale` stays a pure PID+protocol check

- **Decision:** The wedged-daemon reachability logic (re-dial under the lock, kill+respawn only if the fresh dial also fails) lives in the escalation path inside `ensureSupervised`, **not** in `daemonStale`. `daemonStale` remains `!proc.IsAlive(s.PID) || s.ProtocolVersion != supervisedProtocolVersion`, unchanged, so its existing `daemonstate_test.go` coverage stays valid.
- **Rationale:** Folding reachability into `daemonStale` would add write contention to rarely-touched state and centralize logic in a currently-pure check; re-dial-under-lock is what distinguishes a wedged daemon from a freshly-respawned healthy one under concurrency.
- **Applies to:** batch 4

### Decision: Supervised daemon anchoring outside a lyx hub

- **Decision:** When `hubgeometry.Resolve(cwd)` fails (the supported "outside a lyx hub" path where the CLI degrades to `BuiltinRegistry()`), the CLI resolves `worktreeRoot` to the **absolute `--target-dir`** rather than leaving it `""`. So the supervised daemon anchors its state/lock/socket at `<abs target dir>/.lyx/codeintel/go/` — a deterministic, absolute location stable across cwds for the same `--target-dir` — instead of a cwd-relative `.lyx/codeintel/go/` path.
- **Rationale:** Under native (pre-flip) an empty `worktreeRoot` was inert. Once batch 4 flips Go to supervised, `Layout{WorktreeRoot: ""}.CodeintelDaemonStateFile("go")` resolves to a **relative** path anchored to the process cwd, so two invocations against the same `--target-dir` from different cwds would spawn/collide different daemons and litter `.lyx/` wherever the shell happens to be — defeating the session-long-daemon goal for a first-class supported mode. Anchoring at the absolute target dir restores per-target stability with a one-line CLI fallback; it writes the same ephemeral, machine-local `.lyx/codeintel/` artifacts it already writes inside a hub, just rooted at the target project. Implemented in batch 3 (card 8), before the flip goes live.
- **Applies to:** batch 3 (and the property it protects is what batch 4's flip relies on)

## All Files Touched

_Full union of every `Creates:` / `Edits:` / `Moves:` **target** path across every batch, sorted alphabetically._

- `docs/overview.md`
- `internal/codeintelcli/cli.go`
- `internal/codeintelcli/cli_test.go`
- `internal/codeintelengine/doc.go`
- `internal/codeintelengine/ensureserver.go`
- `internal/codeintelengine/ensureserver_integration_test.go`
- `internal/codeintelengine/ensureserver_test.go`
- `internal/codeintelengine/lspclient.go`
- `internal/codeintelengine/lspclient_test.go`
- `internal/codeintelengine/refs.go`
- `internal/codeintelengine/refs_integration_test.go`
- `internal/codeintelengine/refs_test.go`
- `internal/codeintelengine/supervised_integration_test.go`
- `internal/codeintelengine/supervised_test.go`
- `internal/proc/killpid_test.go`
- `internal/proc/proc_linux.go`
- `internal/proc/proc_windows.go`
