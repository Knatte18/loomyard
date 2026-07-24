# codeintel — multi-language redesign (Someday, deprioritized)

> **Status: Design exists, not scheduled.** Full four-layer architecture designed during the
> vacation-time discussion. **Deprioritized because it isn't required for a first working `loom
> run`** — not abandoned. This is a *different, larger* design than what's actually shipped
> today: the current `internal/codeintelengine` (see its package doc) is a single-language
> (Go-only), daemon-free, no-toolchain-manager implementation. This doc describes the future
> redesign; it does not describe what exists.

## Motivation (unchanged from the original proposal)

Webster forks (and the planner) currently discover "what does this symbol touch" via text
search (Grep/Glob) plus manual reading — imprecise (false positives from name collisions,
silent misses) and expensive (every false-positive hit costs a full LLM round-trip). A working
codeintel replaces this with fast, deterministic, compiler-derived lookups, and is what makes
[plan-format v3](plan-format-v3.md)'s symbol fields (`creates-symbols`/`edits-symbols`/
`reads-symbols`) trustworthy enough to write into a card at all — without it, they degrade to
guesses (see plan-format-v3.md's resolution of this exact machine-mismatch problem).

**What codeintel is not:** not a semantic/conceptual index ("what have we written that's
thematically similar" — see [semantic-index.md](semantic-index.md), a separate, further-out idea,
not part of this proposal); not a
replacement for raddle (raddle answers "where does this belong and why," codeintel answers "what
exactly is affected"); not a DAG builder itself (it provides raw reference/definition facts;
mechanical DAG-derivation is webster's own logic — see [plan-format-v3.md](plan-format-v3.md)).

## Four-layer architecture

### 1. Toolchain manager

Owns installation and pinning of the underlying language-server binaries.

- Checks whether the correct **pinned version** exists in a codeintel-owned cache directory
  (e.g. `~/.cache/lyx/tools/<lang>/<version>/`); installs deterministically if missing
  (`go install ...@<pinned-version>`, or a direct prebuilt-release download) — never relies on
  the host already having the language's own toolchain.
- **Hard constraint: prefer self-contained, runtime-free binaries** over anything needing an
  external runtime on the host. This ruled out the official `roslyn-language-server` and
  `csharp-ls` (both require the .NET SDK) in favor of OmniSharp-Roslyn's self-contained builds.
- Pins an **exact** version, not "latest" — unlike editor extensions optimizing for one
  interactive user tolerating drift, codeintel needs the same input to produce the same output
  across machines and over time.

### 2. Daemon/supervisor

Owns process lifecycle for each running language-server instance. Modeled directly on the
existing wiki-daemon pattern (`millhouse/plugins/mill/scripts/wiki/_client.py`), ported to Go,
generalized to be **language-parameterized**:

- **State file per `(language, worktree-root)`** — not a single global file, since multiple
  language servers may be live at once.
- **Auto-spawn on demand** — a health check *is* the "start if not running" path, no separate
  check step.
- **Two-part staleness check**: a recorded process is trusted only if (a) the PID is alive *and*
  (b) it actually answers (a cheap real LSP call, e.g. `workspace/symbol` with an empty query) —
  not PID-liveness alone, since an LSP server can hang without the process dying.
- **Detached spawn** — survives the spawning process exiting (`start_new_session=True` on Unix,
  `CREATE_BREAKAWAY_FROM_JOB` on Windows), no `systemd`/OS-service dependency.
- **Version-forced restart** — if the client's compiled-in protocol/tool version doesn't match
  what's recorded in the state file, kill and respawn.
- **Not reused from the wiki-daemon:** its bespoke line-delimited JSON-over-TCP wire protocol —
  codeintel's daemon speaks real LSP (JSON-RPC 2.0, `Content-Length` framing) to the underlying
  server; only the lifecycle/supervision layer is shared.

### 3. LSP client

`initialize`/`initialized` handshake, `textDocument/references`, `textDocument/definition`,
`callHierarchy/*`. Standard for the great majority of servers (gopls, ty, rust-analyzer,
OmniSharp-Roslyn), with per-adapter escape hatches for known non-standard behavior (e.g.
Roslyn's own official server needs a `solution/open` call after `initialize`; OmniSharp doesn't
have this requirement, another reason it's preferred).

### 4. Language registry

Maps `language → {binary, pinned version, CLI flags, protocol quirks, install method,
has_native_daemon}`. **v1 scope: Go only (`gopls`)** — covers loomyard's own codebase; no other
language is a proven, immediate need. Documented future candidates:

- **Go → `gopls`.** Pure Go binary, no external runtime. **Has a native shared-daemon mode**
  (`gopls -remote=auto`, confirmed in production use by Anthropic's own official `gopls-lsp`
  Claude Code plugin) — for Go specifically, codeintel's daemon layer can likely be a thin
  wrapper delegating to gopls's own remote mode rather than reimplementing full supervision.
- **Python → `ty`** (Astral, Rust-based, self-contained), preferred over Pyright
  (Node-dependent, fails the runtime-free filter). No known native shared-daemon mode —
  codeintel's own daemon wrapper carries the full weight here; `ty` markets itself as fast even
  cold, so measure whether a full daemon is even necessary before building one for this language.
- **C# → OmniSharp-Roslyn** (self-contained platform builds), preferred over the official
  `roslyn-language-server` and `csharp-ls` (both require the .NET SDK).

### Public interface

```go
type Location struct {
    File   string
    Line   int
    Column int
}

func References(symbol string) ([]Location, error)
func Definition(symbol string) (Location, error)
```

Consumers never need to know whether they're talking to `gopls` or OmniSharp.

## The name-resolution path: `workspace/symbol`

Given a bare symbol name (no explicit position), the engine issues `workspace/symbol` and
requires exactly one candidate — zero is "not found," more than one is "ambiguous" (every
candidate returned as a formatted `file:line:col` string so the caller can disambiguate without
a second broader search). A server that doesn't advertise `workspaceSymbolProvider` fails this
path immediately rather than attempting the call and getting an undefined result. An explicit
`file:line:col` position bypasses this resolver entirely.

## Feedback from external review (folded in)

- **Design-lock now, implementation later, and that's fine as a deliberate split:** the
  four-layer architecture looked heavy to support gopls alone (which has its own
  `-remote=auto`), but given codeintel is explicitly multi-language (Go, Python, C#), the
  daemon/supervisor layer is necessary infrastructure, not premature generalization — Python and
  C# servers don't share gopls's native daemon behavior. **Lock now:** the registry format and
  the `References`/`Definition` public interface, across all three planned languages, so the
  registry never needs a breaking shape change later. **Defer:** build and test the Go (`gopls`)
  adapter first; let Python/C# adapters wait until there's a concrete second consumer, as long as
  the registry format already has room for them.
- **Per-language snapshot keys, not one shared `codeintel` key** (see
  [fabric.md](fabric.md#consumer-boundaries-avoid-re-coupling-codeintel-and-raddle)) — use
  `codeintel-go`/`codeintel-py`/`codeintel-cs` so one language's daemon downtime can't block or
  falsely-advance tracking for the others.
- **Tag decisions as "ported from Millhouse" vs. "new for this rewrite"** when consolidating —
  several assumptions here (grep-based search miss rate, LSP cold-start cost) may already be
  observed facts from months of Millhouse production use, not open questions for the Go rewrite.
  This distinction changes how much scrutiny each decision still needs before being treated as
  settled.

## Known limitations

- **Cannot resolve symbols that don't exist yet** — a structural limit, not a bug. Mitigated at
  the webster/plan-format level by plan-internal name matching for not-yet-existing symbols (see
  [plan-format-v3.md](plan-format-v3.md)), not by codeintel itself.
- Reduced precision around generics, reflection, and heavy dynamic-dispatch patterns (DI
  containers, `dynamic` in C#) — worth explicit measurement per language before trusting
  codeintel as a hard collision judge, especially for C#.
- No cross-worktree cache sharing — each active worktree needs its own loaded/type-checked view.
- Cold-start cost is real and version/repo-size-dependent — should be measured empirically
  (`codeintel-spike` wiki task reportedly already has this data for the Go-only in-process arm).

## Consumers and usage pattern

- **Planner:** verifies symbol names against the real codebase before writing
  `edits-symbols`/`reads-symbols` into a card (see [plan-format-v3.md](plan-format-v3.md)).
- **Webster forks:** conditional prompt injection — only when a card has declared
  `edits-symbols` *and* the relevant language's codeintel daemon is confirmed reachable. Put the
  instruction in the card/task prompt itself, near the relevant field, not in a static
  CLAUDE.md — context proximity to the decision point matters more than instruction placement in
  general system-level files.
- Two consumer interfaces on one daemon: a **Go API** for webster's own orchestration (direct
  function calls, no protocol overhead), and an **MCP server** for Claude agents that need
  dynamic tool discovery. Prefer this self-built/self-pinned path over Claude Code's built-in
  `ENABLE_LSP_TOOL` flag for production use — that path is explicitly documented upstream as
  "raw," undocumented, and subject to change, conflicting with this project's determinism
  requirements; fine for quick interactive experimentation only.

## Related

- [plan-format-v3.md](plan-format-v3.md) — the symbol fields this module makes trustworthy.
- [fabric.md](fabric.md) — per-language snapshot-key notification.
- `internal/codeintelengine` package doc — the current, simpler, shipped implementation this
  design eventually redesigns (not superseded yet — no work has started on this redesign).
