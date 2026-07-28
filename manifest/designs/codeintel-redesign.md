# codeintel — multi-language code-intelligence (Planned)

> **Status: Planned — V1 is Go-only, architected for multi-language.** Promoted from Someday (2026-07) after a design pass that settled three things the earlier sketch left open: the **consumer surface** (a `lyx codeintel` CLI plus an in-process Go API — *no MCP*), the **daemon contract** (a single `EnsureServer` seam with two swappable spawn strategies behind it), and the **V1 scope** (build and prove *both* strategies against `gopls`, populate the registry for Go only, lock its format so the other languages drop in without a breaking change). This doc supersedes the earlier four-layer/MCP write-up. It is **independent of the rest of the Planned queue** — it reads code, drives a language server, returns locations, with no dependency on board / native-clients / fabric-v2 / loom — so it can be built now, in parallel. It is a *different, larger* design than what ships today: the current `internal/codeintelengine` (see its package doc) is a single-language (Go), daemon-free, no-toolchain-manager implementation; V1 **extends** it, it does not replace it wholesale.

## Motivation

Webster forks (and the planner, and reviewers) currently discover "what does this symbol touch" via text search (Grep/Glob) plus manual reading — imprecise (false positives from name collisions, silent misses) and expensive (every false-positive hit costs a full LLM round-trip). A working codeintel replaces this with fast, deterministic, compiler-derived lookups, and is what makes [plan-format v3](../../docs/reference/plan-format-v3.md)'s symbol fields (`creates-symbols`/`edits-symbols`/`reads-symbols`) trustworthy enough to write into a card at all — without it, they degrade to guesses (see `internal/websterengine`'s package documentation for the resolution of this exact machine-mismatch problem).

The payoff is not lyx-runtime CPU speed. It is (a) planner/implementer/reviewer stop gissing "where is this defined / used" and stop paying an LLM round per false grep hit — fewer wasted agent rounds, less token spend, shorter wall-clock; and (b) the plan-format-v3 symbol fields become trustworthy enough to exist.

**What codeintel is not:** not a semantic/conceptual index ("what have we written that's thematically similar" — see [semantic-index.md](semantic-index.md), a separate, further-out idea); not a replacement for raddle (raddle answers "where does this belong and why," codeintel answers "what exactly is affected"); not a DAG builder itself (it provides raw reference/definition facts; mechanical DAG-derivation is webster's own logic — see `internal/websterengine`'s package documentation); and — settled in this design pass — **not an LSP server.** lyx never *serves* LSP. It *consumes* published language-server binaries (gopls first) through an embedded LSP client. "codeintel" = lyx gains code-intelligence powered by LSP, not lyx becomes a language server.

## The two consumer entry points — one engine, no MCP

Everything sits on one `codeintelengine`; only the entry differs:

- **Go-orchestrated code** (webster's own DAG-derivation) calls the engine **in-process** — `codeintelengine.References(...)` / `Definition(...)`, direct function calls, no subprocess, no protocol. Webster does *not* shell out to its own CLI.
- **LLM agents** (planner, webster forks, implementer, reviewer) call **`lyx codeintel references|definition|symbol <query>`** — a thin Cobra command over the same engine.

**Why a CLI and not an MCP server.** The query surface is 2–3 fixed calls, so MCP's value (dynamic tool discovery + typed schemas) is near zero, while its cost (server registration, per-worktree config, connection lifecycle, a Claude-Code-specific mechanism to maintain) is not. A bash CLI is one code path (fits the CLI/Cobra invariant — `Command()`/`RunCLI`, `Short`, help-tree tests), engine-neutral (works for any provider, unlike MCP — see `CLAUDE.md`'s provider-agnostic-engines rule), and needs no new agent skill (agents already parse `file:line:col` from grep; swapping the grep call for a `lyx codeintel` call is zero new learning). On the deployment target (Linux) the per-call latency delta between a subprocess and an MCP message is a few ms of framework overhead — invisible against multi-second LLM turns and against the LSP query itself, which is identical for both paths. MCP would not even remove the daemon (a shared warm server is needed across ephemeral agents regardless), so it buys no throughput and no architectural saving. MCP can be revisited later *only if* dynamic discovery ever becomes worth it; it is not in V1.

**Hard constraint on the CLI:** it is a *thin client over the warm daemon* — it must never boot the language server itself. A cold server per invocation loses to grep; the whole "faster than grep" claim rests on the server being warm when the agent asks (see `EnsureServer` below). The CLI is just another client of the daemon, side by side with the in-process Go API.

**Batch mode:** the CLI accepts several symbols in one call (`lyx codeintel references Foo Bar Baz`) to amortize process startup across lookups — cheap insurance against Windows' expensive process creation, and unnecessary but harmless on Linux.

**Agent wiring** is prompt-injection near the decision point, not a static CLAUDE.md line — put the "you may call `lyx codeintel`" instruction next to the relevant field (webster: beside `edits-symbols`, only when the language's daemon is confirmed reachable). The reviewer/implementer anchor (a treadle/burler round has no `edits-symbols` field to hang it on) is an **open integration point**, resolved in the later consumer-wiring slice, not V1.

## `EnsureServer` — the one layer-2 contract

A single function is the whole lifecycle layer to everything above it:

```go
EnsureServer(lang, worktreeRoot) -> LSPConn
```

CLI, Go API, and the layer-3 protocol calls all go through it and never reason about lifecycle. Its body is four steps:

1. **Toolchain** (layer 1): is the binary present at the pinned version? Install deterministically if missing.
2. **Spawn strategy** — selected by the registry's `has_native_daemon` flag (see below).
3. **Probe (shared by both strategies):** does the server *answer*? A cheap `workspace/symbol` with an empty query. This runs regardless of strategy — even `gopls -remote=auto` can hand back a connection to a hung shared instance, so PID-liveness alone is never trusted.
4. Return a warm `LSPConn`.

Steps 2+3 *are* the doc's old "a health check is the start-if-not-running path": every CLI call invokes `EnsureServer` blindly; on the warm path that is only the probe (cheap), on the cold path it spawns. That is exactly what keeps the thin-CLI-over-warm-daemon model true. **The two strategies collapse under one contract — they differ only in who spawns:**

- **`native`** (Go / `gopls`): run `gopls -remote=auto`; gopls itself dedups and spawns the shared instance. Confirmed in production use by Anthropic's own official `gopls-lsp` Claude Code plugin. We build no supervisor for Go in production.
- **`supervised`** (Python / `ty`, C# / OmniSharp — no native shared-daemon mode): **we** own it — a **state file per `(language, worktree-root)`** (not one global file, since multiple language servers may be live), auto-spawn on the health check, **two-part staleness** (PID alive *and* answers), **detached spawn** that survives the spawning process (`start_new_session=True` on Unix, `CREATE_BREAKAWAY_FROM_JOB` on Windows — no `systemd`/OS-service dependency), and **version-forced restart** when the client's compiled-in protocol version doesn't match the state file.

This layer is modeled on the existing wiki-daemon pattern (`millhouse/plugins/mill/scripts/wiki/_client.py`), ported to Go and generalized to be language-parameterized. **Not reused from it:** the wiki-daemon's bespoke line-delimited JSON-over-TCP wire protocol — codeintel speaks real LSP (see layer 3); only the lifecycle/supervision shape is shared.

## Proving layer 2 in a Go-only V1 — supervise plain `gopls`

The risk in a Go-only V1 is that the `supervised` strategy — the one we actually own — is never exercised, because production Go takes the `native` path. A daemon interface designed but never run is exactly the kind that turns out mis-shaped the day OmniSharp is built against it. Mitigation, at near-zero cost, since gopls is already installed: **V1 builds `supervised` and tests it against a plain `gopls`** (a bare `gopls` process *we* spawn, state-file, probe, and restart — not `-remote=auto`). Production Go still uses `native`; but layer 2 and the strategy-selection seam are proven working against a real LSP server before any non-Go dependency exists. Adding `ty`/OmniSharp later is then a registry entry + adapter quirks against machinery already known to run.

## The LSP client (layer 3)

The shared protocol layer, identical across all languages: `initialize`/`initialized`, `textDocument/references`, `textDocument/definition`, `callHierarchy/*`, `workspace/symbol`. JSON-RPC 2.0 with `Content-Length` framing. Per-adapter escape hatches for known non-standard behavior (e.g. Roslyn's official server needs a `solution/open` after `initialize`; OmniSharp doesn't — one reason it's preferred). Because layer 3 is shared and the `EnsureServer` seam hides the strategy, **the CLI surface is identical across languages**: `lyx codeintel references Foo` looks the same whether the backend is gopls or OmniSharp — the engine routes by the worktree's registered language. (V1 is implicitly Go; cross-language routing from a bare name is a concern only once a second language has a real consumer.)

## Toolchain manager (layer 1)

Owns installation and pinning of the underlying language-server binaries.

- Checks whether the correct **pinned version** exists in a codeintel-owned cache directory (e.g. `~/.cache/lyx/tools/<lang>/<version>/`); installs deterministically if missing (`go install ...@<pinned-version>`, or a direct prebuilt-release download) — never relies on the host already having the language's own toolchain.
- **Hard constraint: prefer self-contained, runtime-free binaries.** This ruled out the official `roslyn-language-server` and `csharp-ls` (both require the .NET SDK) in favor of OmniSharp-Roslyn's self-contained builds, and Pyright (Node-dependent) in favor of `ty` (Astral, Rust, self-contained).
- Pins an **exact** version, not "latest" — unlike editor extensions optimizing for one interactive user tolerating drift, codeintel needs the same input to produce the same output across machines and over time.

## Language registry (layer 4)

Maps `language → {binary, pinned version, CLI flags, protocol quirks, install method, has_native_daemon}`. **The format is locked now, all fields, across the three planned languages** (Go / `gopls`, Python / `ty`, C# / OmniSharp-Roslyn) so it never needs a breaking shape change later — even though **V1 populates Go only**. This is the "easy to add a language" contract made concrete: adding a language = **one registry entry + an optional protocol-quirk hook + a `has_native_daemon` choice** (which strategy `EnsureServer` picks). No change to the client, the CLI, or the engine API. That contract — not the gopls adapter — is the deliverable.

## Name resolution and exit codes

Given a bare symbol name (no explicit position), the engine issues `workspace/symbol` and interprets the result as a small, deterministic contract, surfaced through the CLI's exit code:

- **exactly one candidate → found.** Exit `0`; the location is printed as `file:line:col`.
- **zero candidates → not found.** Exit `1`; empty stdout.
- **more than one → ambiguous.** Exit `2`; *every* candidate printed as `file:line:col` on stdout, so the caller disambiguates without a second broader search (still one precise answer set vs. N grep hits).

A server that doesn't advertise `workspaceSymbolProvider` fails this path immediately rather than attempting the call and getting an undefined result. An explicit `file:line:col` position bypasses the resolver entirely. The in-process Go API returns the same three cases as typed values rather than exit codes.

## V1 scope — what lands

- The `EnsureServer` contract, with **both** spawn strategies coded.
- `native` (`gopls -remote=auto`) as the production Go path.
- `supervised` built and **tested against a plain `gopls`** (state-file + probe + restart), proving layer 2 and the strategy seam.
- Layer 3 LSP client (shared).
- The registry format locked (all fields), Go populated.
- `lyx codeintel references|definition|symbol` CLI + the in-process Go API, both over `EnsureServer`.
- The exit-code contract above.
- **Windows works** (detached spawn on both OSes) but is **not optimized for** — subprocess-spawn cost is a dev-only concern; Linux is the deployment target and process spawn there is cheap.

**Deferred, explicitly:** the `ty` (Python) and OmniSharp (C#) adapters — registry entries + adapter quirks against the proven machinery, when a concrete second consumer exists. And the **consumer wiring** (planner/webster/reviewer prompt-injection, the reviewer anchor) is its own later integration slice — V1 delivers the engine + CLI + Go API and is tested against loomyard's own Go codebase, not against a live producer.

## Fabric / snapshot integration

Per-language snapshot keys, not one shared `codeintel` key (see [`internal/fabricengine`](../../internal/fabricengine/doc.go)) — `codeintel-go`/`codeintel-py`/`codeintel-cs` — so one language's daemon downtime can't block or falsely-advance tracking for the others. This concerns raddle/tracking, not the live-query CLI path (which reflects the daemon's current view of the worktree); V1's interactive queries need no snapshot machinery.

## Known limitations

- **Cannot resolve symbols that don't exist yet** — a structural limit, not a bug. Mitigated at the webster/plan-format level by plan-internal name matching for not-yet-existing symbols (see `internal/websterengine`'s package documentation, the dead-DAG-seam section), not by codeintel itself. This bites the planner consumer most, the reviewer/implementer least.
- Reduced precision around generics, reflection, and heavy dynamic-dispatch patterns (DI containers, `dynamic` in C#) — worth explicit measurement per language before trusting codeintel as a hard collision judge, especially for C#.
- No cross-worktree cache sharing — each active worktree needs its own loaded/type-checked view.
- Cold-start cost is real and version/repo-size-dependent — should be measured empirically; the warm-daemon design exists precisely so ephemeral agents don't pay it.

## Related

- [plan-format-v3.md](../../docs/reference/plan-format-v3.md) — the symbol fields this module makes trustworthy.
- [`internal/fabricengine`](../../internal/fabricengine/doc.go) — per-language snapshot-key notification.
- [semantic-index.md](semantic-index.md) — the separate, further-out conceptual-search idea codeintel is explicitly *not*.
- `internal/codeintelengine` package doc — the current, simpler, shipped implementation V1 extends (Go-only, daemon-free).
