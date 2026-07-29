# Discussion: Give codeintel a persistent, session-long daemon

```yaml
task: Give codeintel a persistent, session-long daemon
slug: codeintel-daemon-persistence
status: discussing
parent: codeintel-v1
```

## Problem

A post-merge benchmark of `codeintel-v1` confirmed the LSP integration is precise (it resolved an interface-dispatched method call grep cannot solve by construction), but the *daemon* story is not real yet. `ensureServer` dispatches only to `ensureNative`, which spawns a disposable `-remote=auto` proxy per call and piggybacks on gopls's own convenience auto-daemon. A predecessor commit on this branch (`2574c14f`) already extended that native daemon's idle timeout to 10m and added a trust-signal note to `refs`/`definition` help — but native still means lyx does not own the daemon lifecycle. The already-built, integration-tested `ensureSupervised` strategy (state file + PID-liveness + protocol-version staleness) is dead code: never dispatched to for Go. Two smaller frictions from the same benchmark also remain: `refs`/`definition` JSON carries no completeness signal (so an agent re-verifies results the tool already resolved), and there is no exact file-scoped name lookup between fragile `file:line:col` column math and over-broad fuzzy `symbol` search.

**Why now:** the benchmark just proved the precision claim holds and pinpointed exactly these three remaining gaps; a lightweight before/after check (same interface-dispatch task, codeintel-specific tool calls dropped 12 → 3) already validated the direction. Items 1–2 (trust-signal docs, native 10m idle timeout) are confirmed present on this branch and are treated as done. This task delivers the remaining items 3–5. Fabric snapshot/tag coupling is explicitly deferred until the Fabric rewrite lands.

## Scope

**In:**

- **Item 3 — completeness/trust field.** Add a flat `"resolution":"complete"` marker to `refs`/`definition` JSON output, so a caller can programmatically trust a single call rather than re-verifying with grep. No extra LSP round-trips.
- **Item 4 — `--in-file <path>` exact lookup mode.** A new `--in-file` flag on `refs` and `definition` that resolves a bare symbol name to a position via a new `textDocument/documentSymbol` LSP method, exhaustively within one file — no fuzzy matching, no column math.
- **Item 5 — wire `ensureSupervised` live for Go.** Flip `ensureServer`'s single live dispatch arm from `ensureNative` to `ensureSupervised`, carry the existing 10m idle timeout onto the supervised daemon, keep `ensureNative` as an in-function fallback, and close the now-live wedged-daemon staleness gap with a bounded one-restart escalation. Needs a new `proc.KillPID` primitive.
- Docs updated in the same commit(s): `internal/codeintelengine/doc.go`, the `ensureserver.go` "exactly one live dispatch arm" comments (the decision they document is being reversed), and CLI `--help`/`Long` text for the new field and flag.

**Out:**

- **Items 1 & 2** (trust-signal docs, native `-remote.listen.timeout=10m`) — already shipped on this branch in `2574c14f`; not reopened.
- **Fabric snapshot/tag coupling to daemon invalidation** — explicitly deferred until the Fabric rewrite lands; do not start it.
- **A session-entry warm-up hook** (mill-spawn/mill-claim or lyx-first-invocation) — not built; supervised is lazy-and-persistent by construction (see Decision: daemon-start-trigger).
- **A formal benchmark harness / benchmark re-run** as a task deliverable — the manual before/after check already gave sufficient signal.
- **A registry `DaemonStrategy` field** — not added (see Decision: dispatch-discriminant).
- **`--in-file` on the `symbol` verb**, and **rich `interface (N impls)` resolution derivation** — both rejected (see Decisions).
- **A `documentSymbol` verb** — item 4 is a resolution *mode* for refs/definition, not a new CLI verb.

## Decisions

### item3-flat-trust-marker

- Decision: Item 3 adds a flat, constant `"resolution":"complete"` field to `refs`/`definition` result output. No extra LSP calls; the value is the same string on every successful lookup.
- Rationale: The observed pain was re-verification waste — a caller needs to know it can trust one call. A flat marker delivers that at zero added latency. Distinguishing interface-dispatch vs exact is informational polish the benchmark pain did not hinge on.
- Rejected: (a) Rich `"exact"` vs `"interface (N impls)"` derivation via an extra `textDocument/implementation` round-trip — adds latency/complexity to every lookup for polish. (b) Flat marker plus a `"count"` — deferred as unneeded; the array length already carries count.

### item3-field-placement

- Decision: The field sits at envelope level for single-arg calls (`{"ok":true,"references":[...],"resolution":"complete"}`) and per-entry for batch-mode `found` entries (alongside `status`). Applies to `refs` and `definition` only. `symbol`, and non-`found` outcomes (not_found / ambiguous / error), are untouched.
- Rationale: One field per result set, not repeated on every array item (every item would carry the same value — pure noise). `symbol` is inherently a fuzzy search where a "complete" marker is meaningless.
- Rejected: Per-item placement inside each reference/definition object (`{file,line,character,resolution}`) — redundant.

### item4-documentsymbol-mechanism

- Decision: Item 4 adds a new `textDocument/documentSymbol` method to `lspClient` and resolves `--in-file <path> <name>` by filtering that file's symbol tree for an exact name match. `documentSymbol` returns a hierarchical `DocumentSymbol[]`; the exact-name search must recurse into children (methods nested under types).
- Rationale: Item 4 exists specifically to escape `workspace/symbol`'s fuzzy matching and result-cap risk. Reusing that fuzzy call under the hood and post-filtering would reintroduce the exact failure mode: if gopls's fuzzy ranking doesn't surface (or caps out before) the target, the post-filter never sees it and silently reports not-found even though the symbol is in the file. `documentSymbol` is exhaustive per-file with no fuzzy ranking.
- Rejected: Reuse `workspace/symbol` + post-filter by file+name (no new LSP method, but inherits fuzziness/cap risk item 4 exists to eliminate).

### item4-flag-shape

- Decision: `--in-file <path>` is a flag on `refs` and `definition` only. When set, the positional argument is always the bare symbol name (never position-parsed, even if it looks like `file:line:col`). `<path>` is resolved against the process cwd by the CLI layer (the CLI owns path interpretation, exactly as `parseQuery` does today). Multiple same-name matches in the file → `ErrAmbiguousSymbol` (candidates listed with line/col); zero matches → `ErrSymbolNotFound`. The resolved position then feeds the normal `references`/`definition` LSP call — `--in-file` changes only the resolve step, not the lookup. Example: `lyx codeintel refs --in-file internal/foo/bar.go MyFunc`.
- Rationale: Mirrors the existing symbol-resolution error contract exactly, just file-scoped — no new error vocabulary. `symbol` doesn't get the flag because `symbol` *is* the fuzzy search; a file-restricted workspace search is a different feature outside item 4's motivation.
- Rejected: (a) `--in-file` on `symbol` too. (b) A new positional grammar (`path::name`) — adds a mini-grammar to parse/document for no benefit over a flag.

### item5-dispatch-supervised

- Decision: Flip `ensureServer`'s single live dispatch arm from `ensureNative` to `ensureSupervised` for Go. `ensureServer` still resolves the toolchain (as `ensureNative` does today) and passes the resolved binary as the `command` to `ensureSupervised`. `Entry` is unchanged — no registry `DaemonStrategy` field.
- Rationale: `ensureSupervised` is the only strategy where lyx itself owns the daemon lifecycle (state file, PID liveness, deterministic idle timeout) across separate `lyx` process invocations — the entire "session-long daemon" thesis. Only Go ever reaches `ensureServer` (sole `HasNativeDaemon==true` entry), so a strategy field would be machinery selecting among exactly one caller. This mirrors the original V1 "exactly one live dispatch arm" reasoning, with the live arm flipped.
- Rejected: Add a `DaemonStrategy` field to `Entry` — speculative generality for one caller.

### item5-daemon-start-trigger

- Decision: Daemon start is lazy on the first `codeintel` call — no session-entry hook.
- Rationale: `ensureSupervised` is already lazy-and-persistent by construction: the first `refs`/`definition` spawns a detached daemon that outlives that call and is found/reused by every later `lyx` invocation in the same worktree via the state file. A warm-up hook saves exactly one cold start at the cost of new cross-module coupling (mill↔lyx). YAGNI — and it moots the open "hook ownership" question, since there is no hook.
- Rejected: A mill-level (mill-spawn/mill-claim) or lyx-level (first-invocation-in-shell) warm-up hook.

### item5-native-fallback

- Decision: `ensureNative` is kept as an in-function fallback. Supervised falls back to native on genuinely supervised-specific failures: spawn failure, repeated lost spawn race / spawn timeout (`ErrServerSpawnTimeout`), dial failure, and a finalize/probe failure *after* a successful dial. It does **not** fall back on toolchain-resolution failure — that is surfaced directly.
- Rationale: Toolchain-resolution failure means the gopls binary itself is unavailable; native needs that exact same resolved binary, so falling back there guarantees an identical failure at doubled latency. The supervised-specific failures are legitimately recoverable via native's different `-remote=auto` code path — real insurance, not wasted latency. Keeping native also keeps item 2's just-shipped 10m-timeout work meaningful rather than reverting it.
- Rejected: (a) Remove `ensureNative` entirely (loses a tested strategy, reverts item 2). (b) Fall back on *any* supervised error including toolchain-resolution (guaranteed-identical failure at 2× latency). (c) Fall back only on spawn failures (too narrow — a wedged daemon that survives the Q12 restart would still error out).

### item5-supervised-idle-timeout

- Decision: The supervised daemon (`command serve -listen=unix;<socket>`) gets an explicit `-listen.timeout` set to the same 10-minute value item 2 applied to native. Reuse the existing `nativeDaemonIdleTimeout` constant, renamed to a shared name (it is no longer native-only).
- Rationale: Supervised currently leaves `serve -listen.timeout` unconfigured (gopls default). Wiring supervised live without setting it would regress the very idle-timeout property item 2 established. One shared constant keeps native and supervised consistent.
- Rejected: A separate/different timeout for supervised — no reason to diverge; a divergence would be surprising.

### item5-wedged-daemon-escalation

- Decision: Close the now-live wedged-daemon gap with a bounded one-restart escalation. On the reconnect path, when a caller finds a non-stale ("healthy"-reading) state but dial-or-finalize fails against it, escalate: take the existing spawn lock, re-check staleness under the lock, and if the state still reads non-stale, force-kill the recorded PID and respawn once — treating "healthy-reading but unreachable" as stale-for-this-call. One restart per call, still bounded by the overall deadline. The slow-first-bind case is deliberately unaffected: that is the *spawner's* own step-6 bounded dial retry, not a reconnecting caller, so it never triggers this escalation.
- Rationale: `daemonStale` only checks PID liveness + protocol version. `doc.go` accepted this gap explicitly *because* supervised had no live V1 dispatch path; Decision `item5-dispatch-supervised` voids that justification. Without escalation, a wedged-but-alive daemon reading "healthy" forever strands every future caller on `ErrServerSpawnTimeout` with no automatic recovery — worse than the 60s-native problem the predecessor fixed. Reusing the existing spawn-race lock to serialize the forced respawn is the minimal-diff move.
- Rejected: (a) Overwrite state and let the wedged process idle out — now meaningfully worse with the 10m timeout: a zombie lingers up to 10 minutes instead of a clean immediate kill. (b) A heartbeat / dial-failure-count field in `daemonStale` — adds write contention to rarely-touched state and centralizes logic in a currently pure check for no clear benefit. (c) Accept the gap as-is — leaves the live trap open.

### item5-killpid-primitive

- Decision: Add `proc.KillPID(pid int) error` to `internal/proc` (cross-platform via stdlib `os.FindProcess` + `Process.Kill()`; on Windows `Kill` calls `TerminateProcess`, on Unix it sends SIGKILL). Accept the PID-reuse risk as-is, consistent with the existing `daemonStale` PID-liveness trust — no identity/cmdline guard.
- Rationale: The escalation only force-kills after confirming the recorded PID is alive *and* its socket is unreachable, on a PID lyx itself wrote to its own state file — the reuse window is narrow, and the existing design already trusts that PID for the liveness half. `internal/proc` is already allowlisted in the Codeintelengine Leaf Invariant (for `IsAlive`/`Detach`), and `KillPID` is stdlib-only, so it neither changes the invariant nor widens the leaf's real dependency surface.
- Rejected: (a) An identity guard (verify the process is really the gopls daemon before killing) — adds a platform-specific process-inspection primitive for a narrow window. (b) No kill-by-PID (would force the weaker "overwrite and idle out" escalation rejected above).

## Technical context

All engine work is in `internal/codeintelengine` (a leaf package — see Constraints); all CLI work is in `internal/codeintelcli`. Key files:

- `internal/codeintelengine/ensureserver.go` — `ensureServer` (the dispatch seam being flipped), `ensureNative` / `nativeArgv` / `nativeDaemonIdleTimeout` (item 5 fallback + shared timeout constant), `ensureSupervised` (the strategy being wired live; its doc comment names the wedged-daemon gap this task closes), `finalizeConnection`, `rootURIFor`, `connKind` (native/supervised/legacy teardown rules).
- `internal/codeintelengine/daemonstate.go` — `daemonState`, `readDaemonState`/`writeDaemonState` (atomic temp+rename), `daemonStale` (PID + protocol-version check; the escalation treats "healthy-reading but unreachable" as stale-for-this-call without changing this pure function), `supervisedProtocolVersion`.
- `internal/codeintelengine/refs.go` — `References`, `acquireConnection` (dispatches to `ensureServer` when `HasNativeDaemon`), `teardownConnection`, the shared `lookup` pipeline, and `resolvePosition` (item 4 adds a new resolve branch here for the `--in-file` name→position path; the position-based and workspace/symbol branches already live here). `Query{Symbol, Pos}` gains a third form for the in-file case; `Options` already threads `WorktreeRoot` for supervised.
- `internal/codeintelengine/definition.go` — `Definition`, a thin `lookup` wrapper (item 4's `--in-file` and item 3's field flow through the shared pipeline, so both verbs get them together).
- `internal/codeintelengine/lspclient.go` — the LSP client. Existing methods: `initialize`, `supportsWorkspaceSymbol`, `references`, `definition`, `workspaceSymbol`, `close`, `kill`. Item 4 adds a `documentSymbol` method here, following the exact `call(ctx, phase, method, params)` plumbing the others use. `symbolInformation`/`lspLocation`/`lspPosition` are the existing wire types; `documentSymbol` needs a `DocumentSymbol` wire type (hierarchical, with `children`).
- `internal/codeintelengine/symbol.go` — `Symbol`, `SymbolMatch{Name,Kind,File,Line,Character}` (the shape `--in-file` ambiguity candidates and documentSymbol matches can mirror).
- `internal/codeintelcli/cli.go` — `refsCommand`/`definitionCommand`/`symbolCommand`, the `--target-dir`/`--lang`/`--timeout` flags (item 4 adds `--in-file` to refs+definition), `parseQuery`/`parsePosition` (item 4 must bypass position-parsing when `--in-file` is set), `emitLookupResult`/`referenceFields`/`classifyLookupError`/`runBatch` (item 3 adds the `"resolution":"complete"` field in `emitLookupResult` and `classifyLookupError`'s `found` branch), and the exit-code contract (0 found / 1 not-found / 2 ambiguous / batch worst-status).
- `internal/proc/proc_linux.go` + `proc_windows.go` — `IsAlive`/`Detach`/`DetachBreakaway`/`HideWindow` today; item 5 adds `KillPID` in both files.
- `internal/hubgeometry` — `Layout.CodeintelDaemonStateFile(lang)` / `CodeintelDaemonLock(lang)` already resolve the supervised state/lock paths; the socket path is `filepath.Join(filepath.Dir(statePath), "daemon.sock")`.

Gotchas:

- The `ensureserver.go` and `doc.go` comments assert "EnsureServer has exactly one live dispatch arm" and name the wedged-daemon gap as acceptable "because supervised has no live V1 dispatch path." Both statements become false with this task — update them in the same commit (Documentation Lifecycle / CLI-help review obligation).
- `resolvePosition`'s workspace/symbol branch deliberately uses a candidate's own LSP location as-is (0-based line / UTF-16 char) to avoid a byte-column round-trip through `Position`. The new `--in-file` documentSymbol branch should follow the same discipline — use the symbol's LSP range start directly, do not round-trip through the byte-column `Position` type.
- Force-kill has only a PID (from the state file), not a `cmd` handle — hence `proc.KillPID`, distinct from `lspClient.kill()` which kills a spawned `*exec.Cmd`.

## Constraints

From `CONSTRAINTS.md`:

- **Codeintelengine Leaf Invariant.** `internal/codeintelengine` production code imports only stdlib, `internal/hubgeometry`, `internal/lock`, `internal/proc`, and `gopkg.in/yaml.v3`. The engine returns typed `(T, error)` and never touches `io.Writer`/exit codes/the output envelope; `internal/codeintelcli` is the sole envelope-mapping layer. `codeintelcli → codeintelengine` is the only allowed direction. `proc.KillPID` stays within this invariant (proc already allowlisted, KillPID is stdlib-only). Enforced by `internal/codeintelengine/leaf_enforcement_test.go`.
- **CLI / Cobra Invariant.** Every command carries a non-empty `Short`; self-discoverable commands carry a `Long` with concrete examples. `--in-file` changes observable behaviour on `refs`/`definition`, so their `Long`/`Short` and any completeness-note help text must be re-read and confirmed accurate against the changed code (help-accuracy is a review-blocking obligation). No new module, so `helptree`/`registration`/`longlist` pinned sets are unchanged. Errors stay JSON via the `output` envelope.
- **Hub Geometry Invariant.** All cwd/geometry and `_lyx`/config paths go through `internal/hubgeometry` — the supervised state/lock/socket paths already do (`Layout.CodeintelDaemonStateFile`/`CodeintelDaemonLock`); no raw geometry tokens in `codeintelengine`.
- **Test Tier Purity Invariant.** Untagged test files must spawn nothing (no `exec.Command`, no git, no fixture copies). Any test that spawns a real gopls or a real daemon must be `//go:build integration`. New unit tests (decision-logic helpers, JSON shape, documentSymbol parse via fake transport, `--in-file` query construction) stay untagged and process-free.
- **Documentation Lifecycle.** Docs land in the same commit as the behaviour change: `doc.go`, the `ensureserver.go` dispatch/gap comments, and CLI help text. `manifest/roadmap.md` does **not** move (this is completing planned work items, but the roadmap convention here is for planned-item completion — confirm at plan time whether a roadmap entry exists for this task; bugfix/hardening/polish alone does not move it). No new cross-cutting invariant is introduced, so no `CONSTRAINTS.md` edit is required.

## Testing

Per the Test Tier Purity Invariant, decision logic and pure mappings are unit-tested (untagged, process-free); anything needing a real gopls or real daemon is `//go:build integration`.

- **Item 3 (resolution field) — unit.** Assert `emitLookupResult` emits `"resolution":"complete"` on a successful single-arg refs/definition envelope and omits it on not_found/ambiguous/error; assert `classifyLookupError`'s `found` batch entry carries it and other statuses don't; assert `symbol` output is unchanged. No LSP needed. TDD candidate.
- **Item 4 (`documentSymbol` + `--in-file`).**
  - Unit (fake transport): `lspClient.documentSymbol` parses a hierarchical `DocumentSymbol[]` response, recursing into children; exact-name filter returns the right range; multiple matches and zero matches map to the ambiguous / not-found engine errors. TDD candidate.
  - Unit (CLI): `--in-file` forces bare-name interpretation (an arg shaped like `file:line:col` is not position-parsed); relative `<path>` is resolved against cwd; the flag is absent from `symbol`.
  - Integration (`//go:build integration`, real gopls): end-to-end `--in-file` resolve → refs/definition against a fixture Go module, including a same-name-in-two-types ambiguity case.
- **Item 5 (supervised dispatch, fallback, wedged restart, KillPID).**
  - Unit: factor the **fallback-trigger classification** (which error → fall back to native vs surface) and the **wedged-escalation decision** ("non-stale state + dial/finalize failure → force restart under lock") into pure helpers and unit-test them exhaustively with fake transport + fake state. This is where the bugs live. TDD candidates.
  - Unit: `proc.KillPID` — cross-platform behaviour on a live child process (kills it) and a non-existent PID (returns an error, does not panic).
  - Unit: `nativeArgv`/supervised argv include the shared `-listen.timeout=10m` flag (argv-shape assertion, no spawn).
  - Integration (`//go:build integration`): happy-path supervised dispatch — a live `refs` spawns a detached daemon, writes the state file, and a second call reuses it (state file address stable, PID alive). Rely on the existing `supervised_integration_test.go` coverage for the strategy internals.
  - **Not built:** a dedicated integration test that artificially wedges a live daemon to exercise the forced restart end-to-end — high-cost, flaky, low marginal value once the escalation *decision* logic is unit-covered.

## Q&A log

- **Q:** Is the task scope items 3–5 only, with items 1–2 done and no benchmark-harness deliverable? **A:** Yes — 1–2 confirmed on-branch (`2574c14f`); a lightweight manual before/after check (codeintel tool calls 12 → 3) already validated the fix; a formal harness is unjustified scope creep.
- **Q:** For item 5, dispatch Go to supervised, carry the 10m idle timeout onto the daemon, and keep native as fallback? **A:** Yes — supervised is the only lyx-owned-lifecycle strategy (the session-long thesis); native stays as tested fallback and keeps item 2's work meaningful.
- **Q:** What triggers daemon start — lazy first-call or a session-entry hook? **A:** Lazy first-call, no hook — supervised is already lazy-and-persistent via the state file; a hook only saves one cold start at the cost of mill↔lyx coupling. Moots the hook-ownership open question.
- **Q:** Now that supervised is live, fix the wedged-daemon staleness gap? **A:** Yes, must-do now — bounded one-restart escalation (force-kill recorded PID + respawn under the spawn lock) treating healthy-reading-but-unreachable as stale-for-this-call. The `doc.go`-accepted "no production caller yet" justification is voided by going live.
- **Q:** What does the item-3 completeness field carry? **A:** A flat `"resolution":"complete"` marker, no extra LSP round-trips — kills re-verification waste at zero latency; interface-vs-exact is unneeded polish.
- **Q:** Item 4 resolution mechanism — new `documentSymbol` method or reuse fuzzy `workspace/symbol`? **A:** Add `textDocument/documentSymbol` (exhaustive per-file, no fuzzy ranking) — reusing the fuzzy call under the hood would reintroduce the exact miss/cap failure item 4 exists to eliminate.
- **Q:** Item 4 flag shape / which verbs / ambiguity semantics? **A:** `--in-file <path>` flag on refs+definition only; positional is always the bare name (never position-parsed) when set; `<path>` resolved against cwd; multiple matches → `ErrAmbiguousSymbol`, zero → `ErrSymbolNotFound`; `symbol` unchanged.
- **Q:** Item 5 dispatch discriminant — flip the single arm or add a registry `DaemonStrategy` field? **A:** Flip `ensureServer`'s single live arm to supervised; native becomes the in-function fallback; `Entry` unchanged — a strategy field would select among exactly one caller.
- **Q:** When does supervised fall back to native? **A:** On spawn/dial/spawn-timeout failures and on a finalize/probe failure after a successful dial; NOT on toolchain-resolution failure (native needs the same binary — falling back guarantees identical failure at 2× latency).
- **Q:** Wedged-escalation mechanism — kill the PID or just overwrite state? **A:** Force-kill the recorded PID and respawn once under the spawn lock; option 2 (idle out) is worse with the 10m timeout (a zombie lingers up to 10 min); a heartbeat field adds needless state churn.
- **Q:** Add `proc.KillPID`, and how careful about PID reuse? **A:** Add `proc.KillPID` (stdlib `os.FindProcess`+`Kill`); accept the PID-reuse risk as-is, consistent with the existing `daemonStale` PID-liveness trust — no identity guard for a narrow window.
- **Q:** Where does the item-3 field sit in the JSON? **A:** Envelope-level for single-arg, per-entry for batch `found` entries, on refs/definition only — never per-item-in-array (redundant), never on `symbol`.
- **Q:** How far to push testing on the hard item-5 paths? **A:** Factor fallback-trigger and wedged-escalation decision logic into pure helpers unit-tested exhaustively; integration-test only happy-path supervised dispatch; no dedicated artificially-wedged-daemon integration test.
