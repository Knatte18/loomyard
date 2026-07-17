# Conflict Resolution Brief

Your sole job is to resolve git conflict markers in the listed files, stage each resolved file, and report success. Do NOT commit. Do NOT run `git merge --continue` — the SKILL does that after receiving `{"status":"success"}`.

## Task intent

These excerpts describe what THIS branch is trying to accomplish. When the merge introduces a parent-side change that conflicts with this branch's intent, the resolution preserves THIS branch's intent. In particular: if a file appears under a batch's `Deletes:` list and the merge introduces a modified version of that file from the parent, the resolution is to delete the file (your branch's intent overrides). Stage the deletion with `git -C /home/knatte/Code/loomyard/wts/codeintel-multilang rm <file>`.

### From discussion.md

# Discussion: Extend codeintel lookup to non-Go languages via LSP

```yaml
task: Extend codeintel lookup to non-Go languages via LSP
slug: codeintel-multilang
status: discussing
parent: main
```

## Problem

`codeintel-spike` (wiki #008) built a throwaway harness that measured structured
reference lookup two ways on this repo: an in-process arm on `go/packages`+`go/types`
(Go-only) and a `gopls-refs` arm that drives `gopls` as a subprocess over LSP JSON-RPC
on stdin/stdout. Its verdict was "adopt in-process `go/packages` for references." But
`lyx` is routinely pointed at target repos that are **not** Go, and the in-process arm
is a dead end for those — `go/packages`/`go/types` simply does not exist for Python, C#,
TypeScript, or Rust. LSP, by contrast, is a *protocol*: every mainstream language ships a
server implementing the same `textDocument/references` method. The `gopls-refs` arm's
LSP-client plumbing is therefore the part of the spike worth generalizing.

**Why now:** #008 has landed (commit `25cc401b`, archive tag `archive/codeintel-spike`),
so its cost/precision numbers exist as a starting point. Nothing production actually
consumes codeintel yet — the spike's "adopt now" recommendation was never built, so this
task is the *first* production codeintel code, built directly on the language-general LSP
path rather than the Go-only one.

## Scope

**In:**

- A new production module: `internal/codeintelengine` (LSP client + language-server
  registry + language detection + references query) and `internal/codeintelcli` (a
  `lyx codeintel refs …` cobra verb), wired into `cmd/lyx/main.go`.
- A **generalized stdio LSP client** — the recovered `gopls.go` plumbing (JSON-RPC
  Content-Length framing, `initialize`/`initialized` handshake, `textDocument/references`,
  UTF-16 position conversion, `shutdown`/`exit`) decoupled from "the binary is `gopls`."
- A **per-language server registry** mapping project markers → launch command, in the
  `internal/modelspec` mould (pinned Go built-ins + optional operator-editable
  `servers.yaml` overlay + embedded seed template). Built-in entries for Go, Python, C#,
  TypeScript, Rust.
- **Language detection** from project markers at the target-repo root.
- A `workspace/symbol` name→position resolver so callers can query by symbol *name*
  (positions are otherwise the only LSP-native input).
- **Empirical re-measurement** of #008's references precision/cost for: Go (gopls, parity
  check), Python (pyright **and** pylsp), C# (csharp-ls). Written up in
  `docs/research/codeintel-multilang.md`.
- Docs: module doc under `docs/modules/`, `docs/overview.md` module-table/stack update,
  any new invariant recorded in `CONSTRAINTS.md` — all in the same commit as the code
  (per CLAUDE.md task-completion rule).

**Out:**

- **The in-process `go/packages`+`go/types` arm is NOT built here.** Go targets are served
  through the same LSP client via `gopls` (`go.mod → gopls`). #008's "adopt in-process
  `go/packages` now" becomes a *separate later optimization task*; nothing regresses
  because that arm was never in production.
- **Call hierarchy and interface-implementation** (`callHierarchy/*`,
  `textDocument/implementation`). Only `textDocument/references` is implemented and
  measured, for exact parity with #008's `refs` arm. These are a documented follow-up.
- **Transitive impact / callgraph** (CHA/RTA/VTA) — Go-only and already Deferred by #008.
- **A lyx-owned install/pin story for server binaries.** Servers are assumed present on
  `$PATH`; provisioning is a documented open follow-up (see Decisions →
  `server-provisioning`).
- **Wiring codeintel into `builder`/`webster`/`burler`.** This task ships the callable
  verb + library; consumers adopt it later.
- **Symbol-name resolution as a first-class contract.** `workspace/symbol` is a
  convenience resolver, not a guaranteed-precise API; the benchmark hand-picks positions.

## Decisions

### deliverable-is-production-module

- Decision: Ship a real `internal/codeintel{engine,cli}` module (generalized LSP client +
  registry + CLI verb), with the multi-language measurement as validation — **not** another
  throwaway spike.
- Rationale: #008 already answered "is this feasible." The open question is a general,
  production-shaped capability. No production codeintel exists to extend, so this is the
  first production landing, built on the language-general path from the start.
- Rejected: (a) another throwaway harness + research-only doc like #008 — redundant, delays
  a capability the webster doc says is useful to `builder` implementers *today*; (b) a
  hybrid where only the registry ships as code — leaves the LSP client (the actually
  reusable part) as throwaway.

### uniform-lsp-path-defer-goarm

- Decision: Go targets flow through the **same** LSP client as every other language, via a
  registry entry `go.mod → gopls`. The in-process `go/packages` fast-path is explicitly
  deferred to a separate task.
- Rationale: One code path to build, test, and measure. `gopls` is a valid LSP server for
  Go, so uniformity costs nothing at the transport layer. The task's whole point is
  language-generality; adding a Go-only second arm doubles surface and drags #008's
  separate adopt-decision into this task.
- Rejected: Build both arms now (task body's literal "Go arm stays as a fallback"). "Stays"
  is aspirational — there is no existing production Go arm to keep. The fast-path is a real
  future optimization, but it is not this task.

### lsp-client-surface

- Decision: The generalized LSP client core is **position-in → references-out** — a pure,
  language-agnostic transport implementing exactly `initialize`, `initialized`,
  `textDocument/references`, `shutdown`, `exit`, plus `workspace/symbol` solely as a
  name→position resolver. No `callHierarchy`, no `implementation`.
- Rationale: `textDocument/references`-only gives exact measurement parity with #008's
  `refs` arm, so Python/C# numbers are directly comparable to Go's. `workspace/symbol` is
  the only LSP-native way to turn a symbol *name* into a position across languages (the Go
  harness used `go/packages` for this, which does not generalize), and builder/webster
  callers think in names, not `file:line:col`.
- Rejected: (a) references-only with no resolver (caller must supply `file:line:col`) —
  unusable for name-based callers; (b) grep-based name→position — reintroduces the textual
  imprecision LSP exists to remove; (c) full impact surface (references + callHierarchy +
  implementation) — unmeasured surface, larger client, defers the honest first deliverable.
- **Deadline / cancellation contract** (a *slow*/hung fault, distinct from the fast typed
  errors elsewhere): the engine's references entry point takes a `context.Context` and every
  call is bounded by a deadline — a `--timeout <dur>` flag on the verb (sensible default,
  e.g. 30s, given #008 measured multi-second warm-ups and rust-analyzer/csharp-ls can index
  a large solution for far longer). On deadline expiry the engine cancels the in-flight LSP
  request, tears down the server subprocess, and returns a typed `ErrServerTimeout` naming
  the phase that stalled (`initialize` vs `references` vs `workspace/symbol`); `codeintelcli`
  emits it as `output.Err`. **Timeout teardown hard-kills the subprocess**
  (`cmd.Process.Kill()`), *not* the recovered client's graceful `shutdown`/`exit` handshake —
  on `ErrServerTimeout` the server is by definition unresponsive, so the graceful path could
  re-block on the same fault; the graceful `close` handshake is only for the normal
  (non-timeout) end of a run. Without this, a server that launches but hangs on `initialize`
  (the fault #008 flagged as the realistic slow case) would block the verb forever — no fast
  typed error covers it.
- Recovered reference: the existing client is preserved at commit `3b4dcf86`
  (`tools/codeintel-poc/gopls.go`; recover with `git show 3b4dcf86:tools/codeintel-poc/gopls.go`).
  Its `lspClient`, `call`/`notify`, `references`, `toLSPPosition`/`utf16Length`, and
  `close` are already ~90% language-agnostic. The only Go couplings to strip: the hardcoded
  `exec.LookPath("gopls")` (→ registry-supplied launch command) and the
  `loadPackages`+`resolveSymbol` symbol→position step (→ `workspace/symbol`, or a
  caller-supplied position).

### language-server-registry

- Decision: A registry mapping project markers → launch command, modelled on
  `internal/modelspec`: pinned Go built-ins in `builtins()`, an optional operator-editable
  `servers.yaml` overlay loaded via `hubgeometry.ConfigFile(base, "servers")`, an embedded
  `template.yaml` seed, whole-entry replacement on overlay, loud errors naming unknown
  keys. Absent overlay is **not** an error (built-ins suffice).
- Built-in entries (marker(s) → command; install-hint carried per entry):
  - `go.mod` → `gopls` (install: `go install golang.org/x/tools/gopls@latest`)
  - `pyproject.toml` / `setup.py` / `setup.cfg` → `pyright-langserver --stdio`
    (install: `npm install -g pyright`)
  - `.sln` / `.csproj` → `csharp-ls` (install: `dotnet tool install --global csharp-ls`)
  - `package.json` + `tsconfig.json` → `typescript-language-server --stdio`
    (install: `npm install -g typescript-language-server typescript`)
  - `Cargo.toml` → `rust-analyzer` (install: via rustup component)
- Rationale: Proven in-repo pattern (modelspec), operator can add/repoint a server without
  a recompile, and the built-ins cover the languages the task names. Per-entry install-hint
  reproduces #008's gopls behaviour (name the one command that unblocks the arm).
- Rejected: hardcoding the marker→command map in Go with no overlay — every new server or
  binary-path change would need a recompile, unlike every other lyx registry.
- **TypeScript and Rust entries ship unmeasured.** The built-in registry includes
  `typescript-language-server` and `rust-analyzer` as marker→command config, but the
  measurement matrix (below) validates only Go, Python (pyright+pylsp), and C#. The TS/Rust
  entries ship as **unverified registry configuration** — correct marker→launch-command
  wiring, no live-server benchmark — so a plan writer must not schedule benchmark validation
  for them. They become measurable later through the same verb with no code change; only the
  empirical numbers are out of this task's scope.
- **Entry overlay schema** (richer than modelspec's Engine/Model/Defaults): each entry
  carries `markers` (list of filenames), `match` (closed vocab: exactly `all` | `any` —
  validated with a loud error naming the offending entry, exactly like modelspec's engine
  vocab), `command` (non-empty launch argv), and `install_hint` (non-empty). The overlay is
  decoded under `yaml.Decoder.KnownFields(true)`, so an unknown field or an out-of-vocab
  `match` value is a loud error, never a silent default. mill-plan pins the exact YAML field
  names against this list rather than inventing them.
- Note: `pylsp` (the second Python server measured) is a benchmark alternative, not the
  default Python built-in — the default is `pyright` (more precise). The registry may carry
  an alt-server mechanism or the measurement can point the client at pylsp directly; a
  detail for mill-plan, not load-bearing for the registry contract. If an alt-server *field*
  is chosen over pointing the client at pylsp directly, it must ride the **same overlay and
  validation path** as primary entries — decoded under `yaml.Decoder.KnownFields(true)`,
  carrying its own install-hint, validated for a known/non-empty launch command — never a
  side-channel that bypasses the registry's loud-error validation.

### language-detection

- Decision: Detect the target language by scanning the target-repo root for the registry's
  marker files. Each registry entry declares its marker-match as **all-of** (AND — e.g.
  TypeScript needs `package.json` AND `tsconfig.json`) or **any-of** (OR — e.g. C# `.sln`
  OR `.csproj`; Python `pyproject.toml` OR `setup.py` OR `setup.cfg`; Go `go.mod`; Rust
  `Cargo.toml`). Entries are tested in a **fixed precedence order** —
  `[go, rust, csharp, typescript, python]` (most structurally-specific / least-ambiguous
  markers first) — and the first entry whose marker-condition is satisfied wins. A
  `--lang <name>` flag overrides detection outright for polyglot repos. No entry matched →
  the engine returns a typed `ErrNoLanguage` naming the markers searched (the CLI emits it
  as `output.Err`).
- Rationale: Mirrors the registry's own marker set; no separate config. A pinned precedence
  list + per-entry AND/OR semantics + a `--lang` escape hatch makes polyglot repos fully
  predictable rather than order-of-filesystem-walk dependent.
- Rejected: shelling out to a language-guesser (e.g. linguist) — heavy external dependency
  for what a handful of marker checks answers. Rejected: leaving precedence unpinned — a
  polyglot repo (a Go service with a `package.json` tooling shim) would resolve
  nondeterministically.

### server-provisioning

- Decision: Server binaries are assumed present on `$PATH`. A missing binary makes the
  engine return a typed `ErrServerNotFound` carrying that server's install command (from
  the registry entry), which `codeintelcli` emits as `output.Err`. lyx does **not**
  install/pin servers itself in this task.
- Rationale: Exactly #008's gopls behaviour. A cross-platform install/pin story per server
  is arguably its own task and out of scope here.
- Rejected: lyx-owned install/pin now — large cross-platform surface (npm, dotnet tool,
  rustup, go install all differ), premature before the capability has consumers.
- Follow-up (documented, not built): a lyx-owned server install/pin story, mirroring the
  "external dependency to vet and pin" caution in `websterv2_extension.md` §5.

### cli-verb

- Decision: Expose `lyx codeintel refs <symbol|file:line:col> [--target-dir <path>]
  [--lang <name>]` via `internal/codeintelcli` (`Command()` + `RunCLI`) with domain logic in
  `internal/codeintelengine`. Registered in `cmd/lyx/main.go` `newRoot()` and the root
  `Long` module list.
- Rationale: Matches every module's Cobra seam (CLI/Cobra Invariant), gives the benchmark a
  real driver (as #008 drove its harness via a command), and is the "expose as a Go verb
  any session can call" the webster doc envisions. `--json` output via the `output`
  envelope, one JSON object per line.
- **Name-resolution contract** (the `<symbol>` form, resolved via `workspace/symbol`):
  exactly-one candidate → proceed to references. Zero candidates → engine returns a typed
  `ErrSymbolNotFound` naming the queried symbol and target. Multiple candidates → engine
  returns a typed `ErrAmbiguousSymbol` listing every candidate's `file:line:col` so the
  caller can re-issue the query with the precise `file:line:col` form. The `file:line:col`
  form bypasses resolution entirely. (`workspace/symbol` precision is best-effort, per Scope
  → Out; the contract is about *how ambiguity is surfaced*, not about guaranteeing a unique
  match.) `codeintelcli` maps each typed error to `output.Err`.
- **Resolver-capability signal** (distinct from a genuine no-match): a server that does not
  advertise `workspaceSymbolProvider` in its `initialize` capabilities (or under-populates
  it) would otherwise return zero candidates and masquerade as `ErrSymbolNotFound`. The
  engine inspects the `initialize` response and, when the capability is absent, returns a
  distinct typed `ErrResolverUnsupported` for the `<symbol>` form — telling the caller "this
  server can't resolve names, use `file:line:col`" rather than "the symbol doesn't exist."
  The `file:line:col` form is unaffected (it needs no resolver).
- Rejected: library-only `internal/codeintel` with tests as the only driver — nothing
  exercises it end-to-end, and the measurement would need a bespoke harness instead of the
  shipping verb. Rejected: silently picking the first of multiple `workspace/symbol`
  candidates — hides ambiguity and yields a wrong-symbol reference set with no signal.

### engine-cli-layering

- Decision: `codeintelengine` returns typed Go errors and typed result values (`(T, error)`)
  and imports **no** `io.Writer`/exit-code/output machinery; `codeintelcli` is the sole
  layer that maps those to the `internal/output` JSON envelope (`output.Ok`/`output.Err`).
  The engine leaf allowlist is therefore stdlib + `hubgeometry` + `gopkg.in/yaml.v3` —
  **not** `internal/output` (exactly as `internal/modelspec`'s leaf excludes it).
- Rationale: Required by the CLI/Cobra Invariant ("engine returns `(T, error)` with no
  cobra/`io.Writer`/exit codes; cli imports engine, engine never imports cli"). Keeping
  `output` out of the engine is also what lets the engine stay a cycle-free leaf importable
  by builder/webster later.
- Rejected: the engine calling `output.Err` directly — mixes the presentation envelope into
  the domain kernel, violates the invariant, and drags an `io.Writer`/exit-code dependency
  into a package meant to return values. (This corrects an internal inconsistency in an
  earlier draft of this discussion that both listed `output` in the leaf allowlist and had
  engine-level decisions emit `output.Err`.)

### measurement-matrix

- Decision: Re-run #008's references precision/cost measurement across:
  - **Go / gopls** — parity check: confirm the generalized client reproduces #008's
    `gopls-refs` numbers on this repo (proves the generalization changed no behaviour).
    No sudo (`go install`).
  - **Python / pyright** — strict type-inference server (the proposal's named default).
  - **Python / pylsp** — jedi-heuristic server; measured *alongside* pyright to expose the
    per-server precision spread within one dynamically-typed language (the proposal's
    point-3 caution turned into a number).
  - **C# / csharp-ls** — Roslyn-based, the mature statically-typed contrast to fuzzy Python.
- Method: mirror #008 exactly — hand-pick benchmark symbols per target repo, establish
  ground truth by grep + manual false-match exclusion, compare the client's reported count
  and position list against it. Record warm-up vs steady-state cost separately (held-open
  server, spawned once, N queries). Raw JSON to `.scratch/codeintel/` (gitignored).
- Rationale: Delivers the exact mature-vs-fuzzy contrast requested, plus a within-Python
  precision spread, without assuming Go's numbers transfer.
- Rejected: (a) Go-parity only / defer all non-Go — fails the task's core deliverable (a
  real non-Go datapoint); (b) Python-only — loses the mature-Roslyn contrast; (c) toy
  fixtures instead of real repos — unrepresentative reference counts.

### benchmark-target-repos

- Decision: For each non-Go language, clone one **mid-size, real, partially-typed**
  project (permissive licence, enough fan-in for interesting reference counts) into
  `.scratch/codeintel/targets/<lang>/` and hand-pick symbols there. Go is measured against
  this repo (loomyard), as #008 did. Exact repo choice is the implementer's, recorded in
  the write-up for reproducibility; criteria: not a toy (unrepresentative), not huge
  (load-time noise dominates), a realistic mix of typed/untyped code so the
  precision-per-typing-discipline story is honest.
- Rationale: Keeps the measurement reproducible and representative; `.scratch/` is
  gitignored so cloned targets never enter the product diff (matches #008's
  measurement-artifacts-to-scratch decision).
- Rejected: vendoring a fixture into the repo (bloats the tree, unrepresentative); measuring
  against lyx's own (Go-only) tree for non-Go arms (impossible).

### measurement-writeup

- Decision: The findings land in `docs/research/codeintel-multilang.md`, same house style
  as `docs/research/codeintel-spike.md` (verdict up front, cost table, precision table per
  symbol vs ground truth, per-language honesty notes, caveats).
- Rationale: `docs/research/` is where #008's spike write-up lives; this is its direct
  continuation. The module's own design doc goes under `docs/modules/` (task-completion
  rule); the research doc is the measurement record, kept distinct.
- Rejected: folding measurement into the module doc — conflates durable design with a
  point-in-time measurement.

## Technical context

- **Recovered client:** commit `3b4dcf86` (last before the spike's revert `d4dcb31c`), also
  reachable via tag `archive/codeintel-spike`. `git show 3b4dcf86:tools/codeintel-poc/gopls.go`.
  The `main.go`/`gopackages.go`/`callers.go`/`callgraph.go` siblings are Go-specific and
  are **not** carried forward (except `gopackages.go`'s symbol-resolution idea, replaced by
  `workspace/symbol`).
- **Registry pattern to mirror:** `internal/modelspec/{load.go,registry.go,template.go,
  template.yaml}`. `LoadRegistry(baseDir)` reads `hubgeometry.ConfigFile(baseDir, "…")`,
  absent-file-is-not-error, `yaml.Decoder.KnownFields(true)`, whole-entry overlay,
  `builtins()` fallback, `ConfigTemplate()` embed accessor. Keep the new engine a **leaf**
  (stdlib + `hubgeometry` + `gopkg.in/yaml.v3`; **not** `internal/output` — see the
  `engine-cli-layering` decision) and add a `leaf_enforcement_test.go` like
  modelspec/tokenvocab if it will be imported widely.
- **CLI seam:** `internal/*cli` exposes `Command() *cobra.Command` and
  `RunCLI(out io.Writer, args []string) int = clihelp.Execute(Command(), out, args)`; wired
  in `cmd/lyx/main.go newRoot()`. Look at a small pair (e.g. `weftcli`/`weftengine`) for the
  idiom. Errors/results via `internal/output` (`output.Ok`/`output.Err`), `--json`.
- **Geometry:** the target-repo root is where the server is launched. Resolve the *current
  working directory* via `hubgeometry.Getwd()` (raw `os.Getwd`/`git rev-parse` are banned
  outside hubgeometry/`cmd/lyx`). An explicit `--target-dir <path>` flag is a plain path
  argument — the geometry-token ban (`_board`, `-weft`, `_lyx`, …) concerns lyx-internal
  tokens, not arbitrary target-repo paths, so target paths need no hubgeometry token API.
- **LSP wire subtleties already solved in the recovered client** (carry them forward, do
  not re-derive): Content-Length framing with CRLF; answering server-initiated requests
  (`client/registerCapability`, `workspace/configuration`) with an empty result so the
  server does not block; UTF-16 code-unit position conversion (`token.Position` byte column
  ≠ LSP character offset for non-ASCII lines); `includeDeclaration: true` on the references
  request (the CLI-form default of `false` was #008's off-by-one).
- **Toolchain reality on this dev machine:** only `go` and `python3` present; Ubuntu 26.04
  strips `ensurepip`. Installs for the measurement (implementation phase, mill-go): `gopls`
  via `go install` (no sudo); `nodejs`+`npm` then `npm i -g pyright` (sudo); `python3-pip`
  then `pip install --user python-lsp-server` (sudo apt for pip); a `.NET SDK` then
  `dotnet tool install --global csharp-ls` (sudo). Operator has approved sudo installs on
  request. Network to `bootstrap.pypa.io` and the Go proxy is available.

## Constraints

From `CONSTRAINTS.md` (this task must satisfy):

- **Hub Geometry Invariant** — cwd resolution via `hubgeometry.Getwd()`; no raw `os.Getwd`
  / `git rev-parse` in the new packages. Target-repo paths are plain paths (no lyx geometry
  tokens involved).
- **CLI / Cobra Invariant** — `codeintelcli` exposes `Command()`+`RunCLI`; `codeintelengine`
  returns `(T, error)` with no cobra/`io.Writer`/exit codes and never imports cobra/cli;
  every command has a non-empty `Short` (self-discoverable ones a `Long` with examples);
  errors via the `output` JSON envelope; parent group sets `RunE = clihelp.GroupRunE`.
  Update the pinned sets in `cmd/lyx/{drift,helptree,registration,longlist}_test.go` in the
  **same commit** as registration.
- **Sandbox Suite Coverage** — add a scenario tagged `**Covers:** codeintel` to a
  `tools/sandbox/*SUITE.md`, **or** add `codeintel` to `excludedModules` with a reason.
  `refs` on a small target is sandbox-friendly, so a real scenario is preferred.
- **Test Tier Purity Invariant** — any test that spawns a language-server subprocess
  (`exec.Command`) or clones a target must be `//go:build integration`-tagged; untagged
  tier-1 tests stay offline/spawn-free. Unit-test the registry parse/resolve, language
  detection, and LSP framing (against a fake in-memory server) untagged; gate live-server
  tests behind the integration tag.
- **Hermetic Git Test Environment Invariant** — any new git-spawning test package needs a
  `TestMain` calling `lyxtest.HermeticGitEnv()`.
- **Leaf invariant (new)** — if `codeintelengine` (or a split-out registry package) is to be
  importable by builder/webster without cycles, keep it a leaf and add a
  `leaf_enforcement_test.go`; record the invariant in `CONSTRAINTS.md` in the same commit.
- **Documentation Lifecycle / task-completion** — module doc in `docs/modules/`,
  `docs/overview.md` module-table + execution-stack update, new invariant in `CONSTRAINTS.md`,
  research write-up in `docs/research/` — all same-commit-as-code where they describe shipped
  behaviour.

## Testing

- **Registry (`load`/`resolve`) — TDD candidate.** Unit tests mirroring
  `modelspec/{load_test,registry_test}.go`: built-ins resolve with no overlay; overlay
  whole-entry replacement; unknown-field / bad-marker / missing-command loud errors naming
  the offending entry + path; absent overlay is not an error. Untagged, offline.
- **Language detection — TDD candidate.** Table test over synthetic marker trees
  (`.scratch`-style temp dirs, no git spawn): each marker set → expected language;
  multi-marker precedence; no-marker → loud error. Untagged.
- **LSP framing — TDD candidate.** Drive `lspClient` against an in-process fake server
  (a pair of `io.Pipe`s or a scripted reader/writer) that speaks Content-Length frames:
  assert the `initialize` handshake, that a server-initiated request gets an empty-result
  reply, correct `textDocument/references` params (`includeDeclaration: true`), UTF-16
  position conversion on a non-ASCII line, and clean `shutdown`/`exit`. Untagged (no real
  subprocess).
- **Live server integration (`//go:build integration`).** Per available server, launch it
  against a small fixture/target and assert a known symbol's reference set — gated so tier-1
  stays offline. `HermeticGitEnv` if the package spawns git.
- **CLI verb.** `RunCLI` seam test: `codeintel refs` output shape through the `output`
  envelope, error envelope on missing server / unresolved symbol / no markers. Help-tree /
  drift / registration / longlist pinned-set updates.
- **Measurement is not a unit test.** The precision/cost numbers are produced by running the
  verb against the target repos and hand-verifying ground truth; the result is the
  `docs/research/codeintel-multilang.md` write-up, not a CI assertion.

## Q&A log

- **Q:** Is the deliverable a measurement spike (like #008) or production code? **A:** Production module (`internal/codeintel{engine,cli}`), with the multi-language measurement as validation. No production codeintel exists yet, so this is the first landing, on the language-general path.
- **Q:** Which non-Go language(s) to measure? **A:** Python **and** C# — a mature-Roslyn vs fuzzy-dynamically-typed contrast.
- **Q:** How does a non-Go query specify the target symbol (the Go harness used `go/packages`)? **A:** (delegated) Client core is position-in→references-out; add `workspace/symbol` as a name→position resolver; the benchmark hand-picks positions to isolate references precision from resolution noise.
- **Q:** Which LSP methods does the client implement? **A:** `textDocument/references` only (for exact #008 parity), plus `workspace/symbol` purely as the name resolver. No callHierarchy/implementation.
- **Q:** Build the in-process `go/packages` Go arm now, or serve Go via gopls over LSP? **A:** Uniform LSP path (`go.mod → gopls`); defer the in-process arm to a separate optimization task.
- **Q:** Which C# server? **A:** csharp-ls (razzmatazz/csharp-language-server) — Roslyn precision, freely licensed, standalone `dotnet tool`. Rejected: MS official Roslyn LSP (licence scoped to VS/VS Code), OmniSharp (maintenance-only). Markers `.sln`/`.csproj`.
- **Q:** Server-binary provisioning? **A:** Assume pre-installed on `$PATH`; loud error with the per-server install command. lyx-owned install/pin is a documented follow-up.
- **Q:** CLI verb or library-only? **A:** (delegated) Thin `lyx codeintel refs` verb via `codeintelcli`/`codeintelengine`.
- **Q:** How to handle the measurement given the machine had no toolchains and Ubuntu 26.04 strips `ensurepip`? **A:** Operator can sudo-install. Full matrix: Go/gopls (parity, no sudo) + Python/pyright + Python/pylsp (precision spread within Python) + C#/csharp-ls.
- **Q:** What target repos to measure against? **A:** (delegated) One mid-size, real, partially-typed project per language cloned into `.scratch/codeintel/targets/`; Go measured against loomyard as #008 did; exact repos recorded in the write-up.
- **Q:** (review r1 gap) The engine leaf allowlist listed `internal/output`, but `output.Err/Ok` are CLI-layer (io.Writer + exit code) and the CLI/Cobra invariant bars the engine from importing them — how is the layer split resolved? **A:** Engine returns typed errors/`(T, error)`; `codeintelcli` alone maps them to the `output` envelope. `output` dropped from the engine leaf allowlist (matching modelspec). See the `engine-cli-layering` decision.
- **Q:** (review r2 gap) Every failure mode was a fast typed error, but a server that launches then hangs on `initialize`/`references` (rust-analyzer/csharp-ls indexing) has no deadline — the verb blocks forever. **A:** The `refs` entry point takes a `context.Context`; a `--timeout` flag (default ~30s) bounds every call; expiry cancels the request, tears down the subprocess, and returns typed `ErrServerTimeout` naming the stalled phase. See `lsp-client-surface` → Deadline / cancellation contract.


### From _mill/plan/00-overview.md


```yaml
task: Extend codeintel lookup to non-Go languages via LSP
slug: codeintel-multilang
approved: true
started: 20260717-172437
parent: main
root: ""
verify: null
```

### From _mill/plan/01-codeintelengine-core.md


```yaml
task: Extend codeintel lookup to non-Go languages via LSP
batch: codeintelengine-core
number: 1
cards: 7
verify: go test ./internal/codeintelengine/...
depends-on: []
```



- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/errors.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/registry.go`
- **Deletes:** none
- **Edits:**
  - `internal/codeintelengine/registry.go`
- **Creates:**
  - `internal/codeintelengine/load.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/template.go`
  - `internal/codeintelengine/template.yaml`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/detect.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/registry_test.go`
  - `internal/codeintelengine/load_test.go`
  - `internal/codeintelengine/detect_test.go`
- **Deletes:** none
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:**
  - `internal/codeintelengine/leaf_enforcement_test.go`
- **Deletes:** none

### From _mill/plan/02-lsp-client-and-refs.md


```yaml
task: Extend codeintel lookup to non-Go languages via LSP
batch: lsp-client-and-refs
number: 2
cards: 5
verify: go test ./internal/codeintelengine/...
depends-on: [1]
```



- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/position.go`
  - `internal/codeintelengine/position_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/lspclient.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/lspclient_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/refs.go`
  - `internal/codeintelengine/refs_test.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/codeintelengine/refs_integration_test.go`
- **Deletes:** none

### From _mill/plan/03-cli-wiring-and-docs.md


```yaml
task: Extend codeintel lookup to non-Go languages via LSP
batch: cli-wiring-and-docs
number: 3
cards: 4
verify: go test ./internal/codeintelcli/... ./cmd/lyx/...
depends-on: [2]
```



- **Edits:** none
- **Creates:**
  - `internal/codeintelcli/cli.go`
- **Deletes:** none
- **Edits:** none
- **Creates:**
  - `internal/codeintelcli/cli_test.go`
- **Deletes:** none
- **Edits:**
  - `cmd/lyx/main.go`
  - `cmd/lyx/sandbox_coverage_test.go`
  - `cmd/lyx/helptree_test.go`
  - `internal/codeintelcli/cli_test.go`
  - `internal/codeintelengine/detect_test.go`
  - `internal/codeintelengine/lspclient_test.go`
  - `internal/codeintelengine/position_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `docs/overview.md`
- **Creates:**
  - `docs/modules/codeintel.md`
- **Deletes:** none

### From _mill/plan/04-measurement-and-writeup.md


```yaml
task: Extend codeintel lookup to non-Go languages via LSP
batch: measurement-and-writeup
number: 4
cards: 1
verify: go test -tags integration ./internal/codeintelengine/...
depends-on: [3]
```



- **Edits:** none
- **Creates:**
  - `docs/research/codeintel-multilang.md`
- **Deletes:** none

## Conflicting files

- `cmd/lyx/main.go`
- `cmd/lyx/helptree_test.go`

## Instructions

For each file listed above:

1. Read the file and locate every conflict block (`<<<<<<<`, `=======`, `>>>>>>>`).
2. Understand both sides of the conflict — what each branch intended.
3. Write a resolution that preserves the intent of both sides. When both sides modify **different, non-overlapping parts** of the same conflict region — for example, different columns of one table row, different keys of one object, or disjoint lines of a prose block — **combine both edits** into a single resolved structure. Do NOT pick one side wholesale just because the region overlaps syntactically; picking one side wholesale is correct only when the two changes are genuinely mutually exclusive (e.g. the same key is renamed to two different values). Worked example: if `ours` changes column A and `theirs` changes column B of the same table row, the resolution keeps both column changes in a single row — it does not discard either.
4. Run `git -C /home/knatte/Code/loomyard/wts/codeintel-multilang add <file>` to stage the resolved file.
5. For modify/delete (DU) conflicts: if Task intent above lists this file under a batch's `Deletes:`, run `git -C /home/knatte/Code/loomyard/wts/codeintel-multilang rm <file>` instead of editing; that stages the intentional deletion.
6. For UD conflicts — files this branch **modified** that the parent branch **deleted**: do not silently keep the modification. Instead:
   a. Run `git log --diff-filter=D --oneline MERGE_HEAD -- <file>` to find the deletion commit on the parent.
   b. Run `git show <deletion-commit>` to inspect context.
   c. If the deletion commit message mentions a replacement file (e.g. "replaced by", "moved to", "consolidated into"), or the commit also adds a file in the same directory with overlapping content: stage the deletion — `git -C /home/knatte/Code/loomyard/wts/codeintel-multilang rm <file>`.
   d. If detection is inconclusive: report `{"status":"stuck","stuck_type":"logic","reason":"modify/delete conflict on <file>: cannot determine if parent deletion is a replacement -- operator must decide"}` and halt. Do NOT silently keep the modification.

Never use `git checkout --ours` or `git checkout --theirs` — they silently discard one side of the conflict.

## Report

Your last output line MUST be a bare JSON object (no code fence, no backticks):

On success (nothing discarded):

{"status":"success"}

On success with discarded content — if you had to drop content from one side (e.g. two sides made mutually exclusive changes and only one could survive), list each dropped item:

{"status":"success","discarded":["<short description of what was dropped from which side>"]}

An empty or absent `discarded` field means nothing was lost. If anything was discarded, you MUST list it; an empty list when content was actually dropped is a protocol violation. The `mill-merge-in` frontend reads this field and surfaces any losses to the operator before continuing, rather than silently running `git merge --continue`.

If you cannot resolve one or more conflicts:

{"status":"stuck","stuck_type":"logic","reason":"<one-line description of what you could not resolve>"}

Anything other than this JSON object on the last line is a protocol violation; the merge-in dispatcher treats that as stuck_type: logic with reason "no structured report" — your work is lost. Do not wrap the JSON in a code fence; do not add commentary after it.

## Tools

Available: Read, Edit, Write, Bash, Grep, Glob. Use `git -C /home/knatte/Code/loomyard/wts/codeintel-multilang` for any git commands; do not `cd`. Worktree cwd is `/home/knatte/Code/loomyard/wts/codeintel-multilang`.
