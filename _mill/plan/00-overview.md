# Plan: codeintel V1 — LSP-backed lookups (Go-only, CLI + EnsureServer)

```yaml
task: 'codeintel V1 — LSP-backed lookups (Go-only, CLI + EnsureServer)'
slug: codeintel-v1
approved: false
started: '20260729-064218'
parent: main
root: ""
verify: go vet ./...
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to
schedule batches. Every batch lives at `NN-<batch-slug>.md` in this
directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: registry-and-state-foundations
    file: 01-registry-and-state-foundations.md
    depends-on: []
    verify: go test -count=1 ./internal/codeintelengine/... ./internal/hubgeometry/...
  - number: 2
    name: toolchain-manager
    file: 02-toolchain-manager.md
    depends-on: [1]
    verify: go test -count=1 ./internal/codeintelengine/... ./cmd/lyx/...
  - number: 3
    name: lspclient-dial-transport
    file: 03-lspclient-dial-transport.md
    depends-on: []
    verify: go test -count=1 ./internal/codeintelengine/...
  - number: 4
    name: daemon-state-and-locking
    file: 04-daemon-state-and-locking.md
    depends-on: [1, 2]
    verify: go test -count=1 ./internal/codeintelengine/... ./internal/proc/... ./cmd/lyx/...
  - number: 5
    name: ensure-server-native
    file: 05-ensure-server-native.md
    depends-on: [2, 4]
    verify: go test -count=1 ./internal/codeintelengine/...
  - number: 6
    name: ensure-server-supervised
    file: 06-ensure-server-supervised.md
    depends-on: [3, 4, 5]
    verify: go test -count=1 ./internal/codeintelengine/... ./internal/proc/... ./cmd/lyx/...
  - number: 7
    name: wire-ensure-server-into-refs
    file: 07-wire-ensure-server-into-refs.md
    depends-on: [5, 6]
    verify: go test -count=1 ./internal/codeintelengine/...
  - number: 8
    name: definition-and-symbol-engine
    file: 08-definition-and-symbol-engine.md
    depends-on: [7]
    verify: go test -count=1 ./internal/codeintelengine/...
  - number: 9
    name: cli-definition-and-symbol
    file: 09-cli-definition-and-symbol.md
    depends-on: [8]
    verify: go test -count=1 ./internal/codeintelcli/... ./cmd/lyx/...
  - number: 10
    name: batch-mode-cli
    file: 10-batch-mode-cli.md
    depends-on: [9]
    verify: go test -count=1 ./internal/codeintelcli/...
  - number: 11
    name: finalize-docs-and-invariants
    file: 11-finalize-docs-and-invariants.md
    depends-on: [10]
    verify: go build ./... && go vet ./...
```

The DAG has two independent roots. Batch 1 (registry fields + the new
`hubgeometry` state-file/lock accessor) and batch 3 (the `lspClient`
dial-transport mode) touch disjoint files and share no dependency, so they
run in parallel. Batch 2 (the Go toolchain manager) needs batch 1's
`PinnedVersion` field and is where `internal/lock` first enters
`codeintelengine` (the `CONSTRAINTS.md` leaf-invariant amendment lands
there). Batch 4 (the daemon state file, the worktree-scoped spawn-race
lock, and the strategy-shared probe helper) needs both batch 1's
`hubgeometry` accessor and batch 2's now-allowlisted `internal/lock`
import. Batches 5 and 6 build the two `EnsureServer` strategies on top of
batch 4's shared machinery — `native` (batch 5) needs the toolchain
manager (batch 2); `supervised` (batch 6) needs the dial-transport mode
(batch 3) and is sequenced after `native` (batch 5) only because both
edit `internal/codeintelengine/ensureserver.go` and `supervised` is the
one adding the second dispatch arm to a file `native` first creates —
not because either strategy depends on the other's runtime behavior.
Batch 7 wires the finished `EnsureServer` into `References` and is the
single point every later batch (8, 9, 10) builds on. Batch 11 lands last
because its package-doc rewrites, `docs/overview.md` update, and the
`manifest/roadmap.md` Planned→Done flip can only be written truthfully
once the whole feature set (batch mode included) actually exists — see
"Documentation timing" below.

## Shared Decisions

### Decision: `EnsureServer` has exactly one live dispatch arm in V1

- **Decision:** `internal/codeintelengine/ensureserver.go` exposes
  `ensureServer(ctx context.Context, lang string, entry Entry, targetDir,
  worktreeRoot string, timeout time.Duration) (*lspClient, connKind,
  error)`, called by `References`/`Definition`/`Symbol` **only** when
  `entry.HasNativeDaemon` is `true` (Go, in V1 — see
  `registry-and-state-foundations`). For every other registry entry
  (Python/C#/TypeScript/Rust, `HasNativeDaemon` zero-valued), the caller
  never invokes `ensureServer` at all and keeps today's `newLSPClient` +
  manual `initialize` + `close()`/`kill()` code path byte-for-byte
  unchanged. `ensureServer`'s own body has exactly one branch in V1: it
  calls `ensureNative` (batch 5). It does **not** call `ensureSupervised`
  (batch 6) — no live V1 registry entry ever requests the supervised
  strategy, so that call would be dead code reachable by no test except
  one written to force it artificially.
- **Rationale:** this resolves an underspecified point in
  `_mill/discussion.md`'s `registry-scope` and Scope sections, which say
  `EnsureServer`'s two strategies are "selected by the registry's
  `HasNativeDaemon` field" without naming what a `false` value combined
  with "supervised, not legacy" would even mean — no such registry state
  exists in V1 (only `true` for Go and zero-valued-meaning-legacy for the
  other four). Branching *before* calling `ensureServer` for the
  zero-valued case, rather than folding a third "legacy" arm inside
  `ensureServer` itself, is the minimal-diff, lowest-regression-risk
  choice: it leaves the four already-working languages' code path
  completely untouched by this task, satisfying `registry-scope`'s "keep
  Python/C#/TypeScript/Rust working unregressed" requirement by
  construction rather than by a branch that must be tested to prove
  equivalence.
- **Applies to:** batches 5, 6, 7.

### Decision: `supervised` is proven standalone, not through `EnsureServer`'s dispatch

- **Decision:** `ensureSupervised` (batch 6) is a fully implemented,
  unit-tested, and integration-tested function callable directly, but it
  is not wired into `ensureServer`'s dispatch in V1 (see the decision
  above). Its integration test (`supervised_integration_test.go`, batch
  6) calls `ensureSupervised` directly against a plain `gopls`, proving
  the strategy's state-file, probe, and kill-and-restart behavior work
  before any language that actually needs it (a future `ty`/OmniSharp
  adapter) exists.
- **Rationale:** matches `_mill/discussion.md`'s Scope bullet verbatim —
  "`supervised` built and proven against a **plain** `gopls`... so the
  strategy nobody's production language uses in V1 is still exercised
  against a real server before any non-Go dependency exists." A dispatch
  path that no registry entry can reach is not a gap; wiring one in now
  would require inventing a fictitious registry state (a third
  `HasNativeDaemon`-adjacent field) `_mill/discussion.md` never
  specifies, which is out of scope for V1.
- **Applies to:** batch 6.

### Decision: `EnsureServer` performs its own `initialize` + probe internally; the caller must not double-initialize

- **Decision:** for both `native` and `supervised`, `ensureNative`/
  `ensureSupervised` perform the full readiness sequence internally —
  spawn-or-reuse, `initialize(ctx, rootURI)` (built from `targetDir`,
  exactly as `References` builds it today), and the shared `probe` helper
  (batch 4) — and hand back an **already-initialized** `*lspClient`.
  `References`/`Definition`/`Symbol` (batch 7, 8) must **not** call
  `client.initialize()` again on a connection that came from `ensureServer`
  — a second `initialize` request on an already-initialized LSP session is
  a protocol violation. The zero-valued/legacy path is unaffected: it
  still calls `client.initialize()` itself, exactly as `References` does
  today, because it never goes through `ensureServer`.
- **Rationale:** `_mill/discussion.md`'s `state-file-location-and-content`
  decision spells out `native`'s path as "spawn the subprocess... →
  `initialize` → the probe... → use the connection if the probe passes"
  — `initialize` and the probe both happen *inside* the `EnsureServer`
  step, not after it. For `supervised`, every dial (including a caller
  reusing an already-running daemon spawned by an earlier, separate `lyx`
  invocation) is win its own new LSP session on gopls's multi-session
  daemon (the same session-multiplexing property
  `native-strategy-wire-compatibility` verified empirically for
  `-remote=auto`), so it needs its own fresh `initialize` too — the
  daemon process persisting across calls does not mean any one session
  persists across calls.
- **Applies to:** batches 5, 6, 7.

### Decision: connection teardown differs by `connKind` — resolves an internal tension in `_mill/discussion.md`

- **Decision:** `ensureserver.go` defines
  `type connKind int` with `connKindNative` and `connKindSupervised`.
  `References`/`Definition`/`Symbol`'s deferred teardown (batch 7) branches
  on it:
  - **`connKindNative`:** call `close()` on a normal completion,
    `kill()` on a timeout — byte-for-byte the same teardown logic
    `References` already runs today. This is safe because what
    `ensureNative` hands back wraps a **disposable local `-remote=auto`
    proxy subprocess** lyx itself spawned for this one call, not the
    shared background daemon — closing it ends only this session
    (confirmed empirically: two independent client processes got
    sequential, non-merged session IDs on one shared daemon). The shared
    daemon itself is never something lyx holds a direct handle to under
    `native`; there is no "daemon connection" for native to protect,
    only its own throwaway proxy.
  - **`connKindSupervised`:** call **neither** `close()` nor `kill()`.
    The `*lspClient` wraps a **dial** to a daemon lyx spawned to
    *outlive this call* (that is the entire point of `supervised`); the
    LSP graceful-shutdown handshake `close()` sends
    (`shutdown`+`exit`) is meaningless network chatter at best here (the
    daemon is meant to keep serving other callers) and it is a needless
    RPC round trip lyx has no reason to spend. lyx is a one-shot process
    (Loomyard's own "one command does its work, writes JSON, exits"
    principle); the dialed socket's file descriptor is reclaimed by the
    OS when this process exits a moment later, and `lspClient.readLoop`'s
    own doc comment already documents this exact bounded,
    process-lifetime-scoped leak as an accepted cost.
  - **Legacy/zero-valued path (never touches `ensureServer`):** unchanged
    — `close()`/`kill()` exactly as today, since it owns the real server
    subprocess directly.
- **Rationale:** `_mill/discussion.md`'s `ensure-server-call-site`
  decision states "never close the daemon-owned connection" as a rule
  that the round-5 NOTE says applies to "supervised/native-only", but
  `native-lifecycle-and-probe-failure` separately and explicitly says
  native's connection **is** torn down via the existing `close()`/
  `kill()` logic. Read literally together these two decisions
  contradict each other. This decision resolves the contradiction by
  distinguishing what "the connection" actually *is* under each
  strategy: for `native` it is lyx's own disposable proxy (safe to
  close); for `supervised` it is a dial straight into an
  externally-persistent daemon (not safe to run a shutdown handshake
  against). Both of `_mill/discussion.md`'s decisions are correct once
  read through this lens — `native-lifecycle-and-probe-failure` is the
  more specific, empirically-grounded one on this exact point, and this
  Shared Decision makes the resolution explicit and testable rather than
  leaving it for batch 7's implementer to guess.
- **Applies to:** batches 5, 6, 7.

### Decision: `internal/proc` is also added to the Codeintelengine Leaf Invariant allowlist

- **Decision:** batch 4 amends `CONSTRAINTS.md`'s Codeintelengine Leaf
  Invariant a second time (batch 2 already added `internal/lock`) to also
  allow `internal/proc`, and adds `proc.IsAlive(pid int) bool` — a new
  exported, platform-split function in `internal/proc` (implemented in
  both `proc_linux.go` and `proc_windows.go`, tested in both
  `proc_linux_test.go` and `proc_windows_test.go`) — for the daemon
  state file's PID-liveness half of its two-part staleness check (batch
  4). Batch 6 then reuses the same already-widened allowlist for
  `proc.Detach`/`proc.DetachBreakaway` (the `detached-spawn-windows`
  decision's fix) with no second `CONSTRAINTS.md` edit needed.
- **Rationale:** the PID-liveness check `daemonStale` (batch 4) needs is
  platform-different in the same way `internal/proc`'s existing
  `HideWindow`/`Detach` split already is — on Unix, `os.FindProcess`
  always trivially succeeds and only `process.Signal(syscall.Signal(0))`
  actually queries the OS; on Windows, `os.Process.Signal` does not
  reliably support a signal-0 liveness probe at all, but `os.FindProcess`
  itself opens a real process handle and fails when the PID is gone.
  Rather than inventing a second, codeintelengine-private
  `//go:build linux`/`//go:build windows` split for a primitive that is
  squarely "a cross-OS process fact," this decision extends
  `internal/proc` — the package that already owns exactly this class of
  primitive — and reuses the exact allowlisting precedent
  `CONSTRAINTS.md`'s own **GitHub Auth Invariant** entry sets for this
  identical question ("`internal/proc` is on that list because...
  `internal/proc` is itself stdlib-only, allowlisting it does not widen
  the leaf's real dependency surface or weaken the leaf property" —
  `internal/proc`'s own production imports are `os/exec` and `syscall`
  only). Doing the widening once, in batch 4, rather than twice (once for
  `IsAlive`, again in batch 6 for `Detach`), also means
  `_mill/discussion.md`'s Constraints section — which names only
  `internal/lock` and says process spawning "stays stdlib, no amendment
  needed there" — is corrected in exactly one place rather than two.
  `_mill/discussion.md`'s `detached-spawn-windows` decision separately
  names `proc.DetachBreakaway` as an implementation option for the
  supervised spawn path, which only makes sense if `codeintelengine`
  calls into `internal/proc` at all — this decision is what makes that
  call legal under the Leaf Invariant.
- **Applies to:** batches 4, 6.

### Decision: `verify:` commands are native Go, never `PYTHONPATH=`-prefixed

- **Decision:** every `verify:` in this plan (overview and per-batch) is a
  bare `go build`/`go vet`/`go test` invocation. No `PYTHONPATH=` prefix.
- **Rationale:** this is a Go repository; the `PYTHONPATH=`-prefix rule in
  the mill-plan skill is scoped to Python/mill projects specifically, and
  every prior plan in this repo's own history (see
  `_mill/plan` on the `native-clients-migration` and `pattern-wiring`
  branches) uses bare `go test`/`go vet` commands.
- **Applies to:** all batches.

## All Files Touched

- `CONSTRAINTS.md`
- `cmd/lyx/helptree_test.go`
- `cmd/lyx/hermeticenv_test.go`
- `cmd/lyx/tierpurity_test.go`
- `docs/overview.md`
- `internal/codeintelcli/cli.go`
- `internal/codeintelcli/cli_test.go`
- `internal/codeintelengine/daemonstate.go`
- `internal/codeintelengine/daemonstate_test.go`
- `internal/codeintelengine/definition.go`
- `internal/codeintelengine/definition_test.go`
- `internal/codeintelengine/doc.go`
- `internal/codeintelengine/ensureserver.go`
- `internal/codeintelengine/ensureserver_integration_test.go`
- `internal/codeintelengine/ensureserver_test.go`
- `internal/codeintelengine/errors.go`
- `internal/codeintelengine/leaf_enforcement_test.go`
- `internal/codeintelengine/lspclient.go`
- `internal/codeintelengine/lspclient_test.go`
- `internal/codeintelengine/probe.go`
- `internal/codeintelengine/refs.go`
- `internal/codeintelengine/refs_test.go`
- `internal/codeintelengine/registry.go`
- `internal/codeintelengine/registry_test.go`
- `internal/codeintelengine/supervised_integration_test.go`
- `internal/codeintelengine/supervised_test.go`
- `internal/codeintelengine/symbol.go`
- `internal/codeintelengine/symbol_test.go`
- `internal/codeintelengine/toolchain.go`
- `internal/codeintelengine/toolchain_integration_test.go`
- `internal/codeintelengine/toolchain_test.go`
- `internal/hubgeometry/codeinteldaemon_test.go`
- `internal/hubgeometry/hubgeometry.go`
- `internal/proc/isalive_test.go`
- `internal/proc/proc_linux.go`
- `internal/proc/proc_windows.go`
- `internal/proc/proc_windows_test.go`
- `manifest/roadmap.md`
</content>
