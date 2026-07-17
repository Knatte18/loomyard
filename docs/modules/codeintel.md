# codeintel: multi-language reference lookup over LSP

> **Status: ✅ Implemented (v1 scope).** Per the [documentation
> lifecycle](../overview.md#documentation-lifecycle), the durable design rationale below
> is expected to fold into the `internal/codeintelengine` package header and this file
> deleted once that migration happens; until then it is the module's design reference.

## What it is

`codeintel` is a lyx module (`lyx codeintel refs <symbol|file:line:col>`) that finds every
reference to a symbol name or an explicit source position in a target project, across
whichever of five languages (Go, Python, C#, TypeScript, Rust) the project is written in.
It generalizes the Go-only, in-process `go/packages`/`go/types` approach the
[codeintel spike](../research/codeintel-spike.md) (#008) recommended for Go alone into a
uniform **LSP** ("Language Server Protocol") path that works for every supported
language, including Go, trading the spike's sub-millisecond in-process query cost for one
LSP round trip per query — a deliberate scope trade the
[multilang research doc](../research/codeintel-multilang.md) records in full; this file
covers only the resulting design, not the alternatives considered.

## The engine/CLI split

`internal/codeintelengine` is a **leaf package**: it returns typed Go results and typed
errors and imports nothing beyond stdlib, `internal/hubgeometry`, and `gopkg.in/yaml.v3`
— no `io.Writer`, no exit codes, no `internal/output`. `internal/codeintelcli` is the
**sole** consumer that maps engine results/errors onto the `internal/output` JSON
envelope (`output.Ok`/`output.Err`), exactly the CLI/Cobra Invariant's "engine returns
`(T, error)`, cli emits the envelope" split every other lyx module follows (see
`internal/modelspec` for the shape this package mirrors most directly). This keeps
`internal/codeintelengine` cycle-free and importable by any future consumer (e.g.
`builder` or `webster`) the same way `internal/modelspec` already is.

## The generalized LSP client

`internal/codeintelengine`'s LSP client (`lspclient.go`) speaks exactly six methods over
stdio JSON-RPC framing (`Content-Length`-prefixed messages): `initialize`, `initialized`,
`textDocument/references`, `workspace/symbol`, `shutdown`, `exit`. No `callHierarchy`, no
`implementation` — the spike's call-hierarchy recommendation (build it on
`TypesInfo.Uses`/`Defs`, never syntactic `*ast.CallExpr` pattern-matching) does not
translate to a language-agnostic LSP client at all, since LSP callers must accept
whatever `callHierarchy/incomingCalls` a given server implements; that generalization is
explicitly deferred (see Scope boundaries below).

Every request phase — `initialize`, the `workspace/symbol` resolver call, and
`textDocument/references` — is bounded by its own `context.WithTimeout(ctx,
opts.Timeout)` deadline (`--timeout`, default 30s). A phase that times out returns
`ErrServerTimeout` (naming the stalled phase) and the caller hard-kills the subprocess
(`cmd.Process.Kill()` via the client's `kill()`) rather than attempting the graceful
`shutdown`/`exit` handshake, which could itself re-block on a server that is already
unresponsive. A phase that completes normally instead closes the server via the graceful
handshake (`close()`). This mirrors the spike's own timeout-closes-the-hang-failure-mode
framing, generalized from "gopls hangs on initialize" to any of the three phases on any
server.

`workspace/symbol` is the **name → position resolver**: given a bare symbol name (no
explicit position), the engine issues `workspace/symbol` and requires exactly one
candidate — zero is `ErrSymbolNotFound`, more than one is `ErrAmbiguousSymbol` (carrying
every candidate as a formatted `file:line:col` string so the caller can disambiguate
without a second broader search). A server that does not advertise
`workspaceSymbolProvider` in its `initialize` response fails this path immediately with
`ErrResolverUnsupported` rather than attempting the call and getting an empty or
undefined result. An explicit `file:line:col` position bypasses this resolver entirely.

Position conversion (`position.go`) is the one place caller-facing 1-based
line/byte-column positions ("file:line:col" as parsed from a CLI argument) cross into
LSP's wire format: 0-based line, UTF-16 code-unit column. The conversion re-reads the
target file because the byte column and the UTF-16 offset only coincide on a pure-ASCII
line — any multi-byte rune before the target column would otherwise misalign the position
handed to the server. A `workspace/symbol` candidate's own returned position, by
contrast, is already in LSP's wire shape and is used as-is with no round trip through the
byte-column type, avoiding exactly that misalignment hazard on the resolver path.

## The language-server registry

The registry (`registry.go`, `load.go`, `template.go`/`template.yaml`) mirrors
`internal/modelspec`'s registry shape end to end:

- **Built-ins** (`builtins()`): a pinned, default-free `Registry` (a
  `map[string]Entry`) for the five supported languages, each entry naming its detection
  `Markers`, whether `Match` requires `"all"` or `"any"` of them, the launch `Command`
  argv, and an operator-facing `InstallHint`. `BuiltinRegistry()` exposes this to any
  caller (the CLI uses it when no lyx-hub overlay base is resolvable).
- **Optional `servers.yaml` overlay** (`LoadRegistry(baseDir)`): loaded via
  `hubgeometry.ConfigFile(baseDir, "servers")` — never hand-joined, per the Hub Geometry
  Invariant. An absent file is not an error (built-ins suffice); a present file decodes
  with `yaml.Decoder.KnownFields(true)` (an unknown field anywhere is a loud error naming
  the offending entry and file path) and every present entry **whole-replaces** its
  built-in counterpart — no field-level merge, so an override can never silently mix a
  stale built-in default with a new one.
- **Embedded seed** (`ConfigTemplate()`): `template.yaml`, embedded at build time,
  documents every built-in at its default values plus how to add a new language or
  override one, ready for `lyx config`'s materialize/reconcile flow the way
  `models.yaml`'s template already works — codeintel does not wire that flow itself
  (out of this batch's scope; the accessor exists so a future `lyx config` integration is
  a template lookup away, not a new design).
- **Detection precedence** (`detect.go`): a fixed order (`go, rust, csharp, typescript,
  python`, pinned as a slice — map iteration is unordered) so a project matching more
  than one language's markers (e.g. a Go module vendoring a TypeScript frontend)
  resolves deterministically to the earlier language. `--lang` bypasses precedence
  entirely, looking the override up directly in the registry (an unknown override names
  every valid language in its error).

## The typed error vocabulary

Every engine failure mode is a distinct sentinel or data-carrying error type (see
`errors.go`), each satisfying `errors.Is` against a package-level sentinel regardless of
its concrete field values:

| Error | Meaning |
|---|---|
| `ErrNoLanguage` | no registry entry's markers matched under the target directory |
| `ErrServerNotFound` | the entry's `Command[0]` binary is absent on `$PATH`; carries `InstallHint` |
| `ErrSymbolNotFound` | `workspace/symbol` resolved the queried name to zero candidates |
| `ErrAmbiguousSymbol` | `workspace/symbol` resolved to more than one candidate; carries every candidate as `file:line:col` |
| `ErrResolverUnsupported` | the launched server does not advertise `workspaceSymbolProvider` |
| `ErrServerTimeout` | a phase's deadline expired; names the stalled phase and the timeout |

`internal/codeintelcli` maps every one of these (and any other engine error) uniformly to
`output.Err(err.Error())` with exit 1 — no error needs a distinct exit code, since the
message text alone is the actionable signal a caller (human or the future measurement
harness) needs.

## The `lyx codeintel refs` verb

`lyx codeintel refs <symbol|file:line:col> [--target-dir DIR] [--lang LANG] [--timeout
DURATION]` is the only verb this module ships. The parent `codeintel` command carries
`RunE: clihelp.GroupRunE` (a bare `lyx codeintel` lists `refs`; an unknown subcommand
emits a JSON error), matching every other lyx module group.

`refs`' `RunE`:

1. Resolves `--target-dir` (default: `hubgeometry.Getwd()` — never raw `os.Getwd`, per
   the Hub Geometry Invariant).
2. Parses the single positional argument into a `codeintelengine.Query`: an explicit
   `file:line:col` position when the argument has that shape (scanned from the right, so
   a Windows drive-letter path still parses correctly), otherwise a bare symbol name.
3. Resolves the `servers.yaml` overlay base: `hubgeometry.Resolve(cwd)` on success loads
   `codeintelengine.LoadRegistry(layout.Cwd)` — the resolved `Layout`'s **`Cwd`** field,
   exactly as `internal/buildercli/cli.go` anchors `modelspec.LoadRegistry(layout.Cwd)`,
   since `ConfigFile(baseDir, "servers")` resolves `<baseDir>/_lyx/config/servers.yaml`
   and passing `Layout.Hub` would silently miss every overlay. When `cwd` is not inside a
   lyx hub at all, this degrades to `codeintelengine.BuiltinRegistry()` rather than
   failing the lookup — codeintel works with zero configuration outside any hub.
4. Calls `codeintelengine.References(ctx, opts)` and maps the result
   (`[]{file,line,character}`) to `output.Ok`, or any error to `output.Err`.

## Scope boundaries — what this module deliberately does not do

- **No in-process `go/packages` arm.** The spike's recommended sub-millisecond,
  zero-false-positive Go-only path is not wired here; `codeintel` always goes through
  LSP, including for Go (`gopls`), trading peak Go-only precision/speed for uniform
  multi-language coverage. See the [multilang research doc](../research/codeintel-multilang.md)
  for the full trade-off record.
- **No call hierarchy, no `implementation`.** Only `textDocument/references` and the
  `workspace/symbol` resolver are wired. The spike's call-hierarchy fix
  (`TypesInfo.Uses`/`Defs`-based, not AST-pattern-based) does not generalize to a
  language-agnostic LSP client, and `implementation` was never in this batch's rubric.
- **No lyx-owned server install/pin story.** `InstallHint` surfaces the operator command
  to install a missing server binary (e.g. `go install golang.org/x/tools/gopls@latest`);
  lyx does not install, version-pin, or manage language-server binaries itself — that is
  the same boundary `internal/modelspec` draws around model binaries/API keys.
- **No `lyx config` reconcile wiring for `servers.yaml` yet.** `ConfigTemplate()` exists
  and mirrors `models.yaml`'s shape, but this batch does not add codeintel to `lyx config
  reconcile`'s seed-only module list; an operator who wants an overlay writes
  `_lyx/config/servers.yaml` by hand today.

The original design reasoning that motivated generalizing lookup beyond Go in the first
place lives in `docs/modules/websterv2_extension.md` — named here in prose rather than as
a link, because that doc lives on `main`, not this task branch, and a relative link here
would dangle at this branch's HEAD.
