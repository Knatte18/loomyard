// Package scoutengine finds every reference to a symbol name or an
// explicit source position, shows a symbol's definition, and searches
// workspace symbols by name (`lyx scout refs|definition|symbol
// <symbol|file:line:col>`) in a target project, across whichever of five
// languages (Go, Python, C#, TypeScript, Rust) the project is written in. It
// generalizes the Go-only, in-process go/packages/go/types approach the
// scout spike (wiki task codeintel-spike, #008) recommended for Go alone
// into a uniform LSP ("Language Server Protocol") path that works for every
// supported language, including Go, trading the spike's sub-millisecond
// in-process query cost for one LSP round trip per query — a deliberate
// scope trade the scout multilang research (wiki task
// codeintel-multilang, #009) records in full. V1 (this package's current
// shape) goes further for Go specifically: it wires a full `EnsureServer`
// daemon lifecycle — toolchain-managed install, spawn-or-reuse, health
// probing — so Go's language server is warm across calls rather than
// cold-spawned every time. The other four languages (Python, C#,
// TypeScript, Rust) keep the original cold-spawn-per-call design entirely
// unchanged: each call still launches its server fresh, initializes it, and
// tears it down at the end of that one call. This comment covers only the
// resulting design, not the alternatives considered.
//
// # The engine/CLI split
//
// scoutengine is the engine half of an engine/CLI seam: it returns typed
// Go results and typed errors and never imports internal/output, cobra, or
// any internal/*cli package — no io.Writer, no exit codes, no output
// envelope. internal/scoutcli is the sole consumer that maps engine
// results/errors onto the internal/output JSON envelope (output.Ok/output.Err),
// exactly the CLI/Cobra Invariant's "engine returns (T, error), cli emits
// the envelope" split every other lyx module follows. Beyond that negative
// rule there is no import allowlist: scoutengine draws on the shared
// infrastructure layer as freely as any other engine module, which keeps it
// cycle-free and importable by any future consumer (e.g. webster)
// without charging rent on each new dependency. CONSTRAINTS.md's "Scout
// Engine-Seam Invariant" records the rule; the package's seam enforcement
// test enforces it.
//
// # The generalized LSP client
//
// The LSP client (lspclient.go) speaks exactly eight methods over stdio
// JSON-RPC framing (Content-Length-prefixed messages): initialize,
// initialized, textDocument/references, textDocument/definition,
// textDocument/documentSymbol, workspace/symbol, shutdown, exit. No
// callHierarchy, no implementation — the spike's call-hierarchy
// recommendation (build it on TypesInfo.Uses/Defs, never syntactic
// *ast.CallExpr pattern-matching) does not translate to a language-agnostic
// LSP client at all, since LSP callers must accept whatever
// callHierarchy/incomingCalls a given server implements; that
// generalization is explicitly deferred (see Scope boundaries below).
//
// Every request phase — initialize, the workspace/symbol resolver call, and
// textDocument/references or textDocument/definition — is bounded by its
// own context.WithTimeout(ctx, opts.Timeout) deadline (--timeout, default
// 30s). A phase that times out returns ErrServerTimeout (naming the stalled
// phase) and the caller hard-kills the subprocess (cmd.Process.Kill() via
// the client's kill()) rather than attempting the graceful shutdown/exit
// handshake, which could itself re-block on a server that is already
// unresponsive. A phase that completes normally instead closes the server
// via the graceful handshake (close()) — except under the EnsureServer
// supervised strategy, whose connections are never closed at all; see "The
// EnsureServer seam" below for why. This mirrors the spike's own
// timeout-closes-the-hang-failure-mode framing, generalized from "gopls
// hangs on initialize" to any of the request phases on any server.
//
// workspace/symbol is the name → position resolver: given a bare symbol
// name (no explicit position), References/Definition issue workspace/symbol
// (via resolvePosition, refs.go) and require exactly one candidate — zero is
// ErrSymbolNotFound, more than one is ErrAmbiguousSymbol (carrying every
// candidate as a formatted file:line:col string so the caller can
// disambiguate without a second broader search). A server that does not
// advertise workspaceSymbolProvider in its initialize response fails this
// path immediately with ErrResolverUnsupported rather than attempting the
// call and getting an empty or undefined result. An explicit file:line:col
// position bypasses this resolver entirely.
//
// A second, narrower resolve mode (--in-file, Query.InFile) exists
// alongside workspace/symbol for the case a caller already knows which file
// a symbol lives in: resolvePosition issues textDocument/documentSymbol for
// that one file instead and searches its hierarchical result exhaustively
// (collectInFileMatches, descending into every symbol's Children) for exact
// name matches. Unlike workspace/symbol, this resolver does no fuzzy or
// project-wide matching — it is scoped to exactly the one named file — and
// gates on the server advertising documentSymbolProvider rather than
// workspaceSymbolProvider, failing with ErrResolverUnsupported the same way
// when it does not. The zero/one/many candidate mapping to
// ErrSymbolNotFound/success/ErrAmbiguousSymbol otherwise mirrors
// workspace/symbol's exactly.
//
// Position conversion (position.go) is the one place caller-facing 1-based
// line/byte-column positions (file:line:col as parsed from a CLI argument)
// cross into LSP's wire format: 0-based line, UTF-16 code-unit column. The
// conversion re-reads the target file because the byte column and the UTF-16
// offset only coincide on a pure-ASCII line — any multi-byte rune before the
// target column would otherwise misalign the position handed to the server.
// A workspace/symbol candidate's own returned position, by contrast, is
// already in LSP's wire shape and is used as-is with no round trip through
// the byte-column type, avoiding exactly that misalignment hazard on the
// resolver path.
//
// # The language-server registry
//
// The registry (registry.go, load.go, template.go/template.yaml) mirrors
// internal/modelspec's registry shape end to end:
//
//   - Built-ins (builtins()): a pinned, default-free Registry (a
//     map[string]Entry) for the five supported languages, each entry naming
//     its detection Markers, whether Match requires "all" or "any" of them,
//     the launch Command argv, an operator-facing InstallHint, and — Go's
//     entry only in V1 — PinnedVersion and HasNativeDaemon (see "The
//     EnsureServer seam" and "Go toolchain manager" below).
//     BuiltinRegistry() exposes this to any caller (the CLI uses it when no
//     lyx-hub overlay base is resolvable).
//   - Optional servers.yaml overlay (LoadRegistry(baseDir)): loaded via
//     configengine.ConfigFile(baseDir, "servers") — never hand-joined, per the
//     Cwd Resolution Invariant. An absent file is not an error (built-ins
//     suffice); a present file decodes with yaml.Decoder.KnownFields(true)
//     (an unknown field anywhere is a loud error naming the offending entry
//     and file path) and every present entry whole-replaces its built-in
//     counterpart — no field-level merge, so an override can never silently
//     mix a stale built-in default with a new one.
//   - Embedded seed (ConfigTemplate()): template.yaml, embedded at build
//     time, documents every built-in at its default values plus how to add a
//     new language or override one, ready for lyx config's
//     materialize/reconcile flow the way models.yaml's template already
//     works — scout does not wire that flow itself (the accessor exists
//     so a future lyx config integration is a template lookup away, not a
//     new design).
//   - Detection precedence (detect.go): a fixed order (go, rust, csharp,
//     typescript, python, pinned as a slice — map iteration is unordered) so
//     a project matching more than one language's markers (e.g. a Go module
//     vendoring a TypeScript frontend) resolves deterministically to the
//     earlier language. --lang bypasses precedence entirely, looking the
//     override up directly in the registry (an unknown override names every
//     valid language in its error).
//
// # The EnsureServer seam
//
// ensureserver.go implements ensureServer(ctx, lang, entry, targetDir,
// anchorRoot, timeout) (*lspClient, connKind, error): given a registry
// entry whose HasNativeDaemon field is true, it resolves, spawns or dials,
// and hands back an already-initialized, already-probed connection ready
// for immediate use. entry.HasNativeDaemon is the gate that decides whether
// a language ever calls into this machinery at all — in V1 that is Go
// alone. Python, C#, TypeScript, and Rust all leave HasNativeDaemon at its
// zero value and never call ensureServer; their callers keep using
// newLSPClient + a manual client.initialize + close()/kill(), byte-for-byte
// the same code path this package has always run, completely untouched by
// V1.
//
// Two strategies implement the seam: ensureNative (native, Go's production
// path — spawn gopls -remote=auto, a disposable local proxy subprocess;
// gopls itself dedups and owns the real shared daemon behind it, kept warm
// via an explicit -remote.listen.timeout override — see daemonIdleTimeout
// in ensureserver.go — sized for an agent's own reasoning gaps between
// calls, not gopls's 1-minute human-editing-rhythm default) and
// ensureSupervised (supervised — lyx owns a state file, an advisory
// spawn-race lock, a deterministic socket path, and detached-spawn/restart
// logic for a language server with no shared-daemon mode of its own).
// ensureServer dispatches Go to ensureSupervised as its live V1 strategy: it
// resolves the toolchain once, then attempts supervised, falling back to
// ensureNative on any supervised error (a toolchain-resolution failure
// itself never reaches the fallback, since it is returned before
// ensureSupervised is ever attempted). ensureNative remains fully built,
// unit-tested, and integration-tested as this fallback — its own dedicated
// integration test still drives it directly, proving the -remote=auto
// proxy path independently of the supervised dispatch above it.
//
// Connection teardown differs by connKind, and getting this wrong is a
// protocol-correctness bug, not a style choice:
//
//   - connKindNative: safe to close()/kill(), exactly like the legacy path.
//     What ensureNative hands back is lyx's own disposable -remote=auto
//     proxy subprocess for this one call, not the shared daemon behind it —
//     closing it ends only this session, never gopls's real shared
//     instance.
//   - connKindSupervised: never close() or kill() it. The connection is a
//     dial into a daemon lyx spawned to outlive this call — the entire
//     point of the supervised strategy — so the LSP graceful-shutdown
//     handshake close() would send (shutdown+exit) is meaningless network
//     chatter at best (the daemon is meant to keep serving other callers)
//     and a needless RPC round trip lyx has no reason to spend. The dialed
//     socket's file descriptor is reclaimed by the OS when this one-shot
//     process exits a moment later.
//   - connKindLegacy: unchanged from before this task — the caller closes
//     the real server subprocess it directly owns, since it never went
//     through ensureServer at all.
//
// A wedged daemon — live but hung, or never finished binding its listen
// socket, neither of which daemonStale's PID-plus-protocol-version check
// alone detects — no longer strands every caller indefinitely: see
// ensureSupervised's own doc comment for the re-dial-under-lock-then-
// one-restart escalation that recovers it, now that Go's registry entry
// dispatches here as a live V1 caller.
//
// # Go toolchain manager
//
// toolchain.go resolves (and, on a cold cache, installs) the Go language
// server. $PATH is never consulted for Go: resolveGoToolchain installs a
// pinned gopls version — registry.go's builtins()["go"].PinnedVersion, an
// exact version, not "latest" — into os.UserCacheDir()/lyx/tools/go/<version>,
// and ensureNative always launches that resolved binary, never whatever
// "gopls" happens to resolve to on the operator's PATH. The install itself
// is fenced by its own advisory lock (goToolchainInstallLock), a lock
// distinct from the daemon spawn-race lock ensureSupervised uses — an
// install-in-progress and a daemon-spawn-in-progress are unrelated races
// that must not serialize on each other.
//
// This cache root is deliberately outside the Cwd Resolution Invariant's
// scope: it is machine-global, not worktree/hub geometry, so this file
// hand-joins os.UserCacheDir() directly rather than routing through
// internal/lyxcwd. That is deliberate, not an oversight — a pinned
// gopls binary is shared across every worktree on the machine, which is the
// entire point of pinning-and-caching it once rather than per worktree.
//
// # Daemon state and concurrency
//
// daemonstate.go implements the supervised strategy's runtime state: a JSON
// state file plus a paired advisory lock per (anchorRoot, lang), resolved
// via this package's own DaemonStateFile/DaemonLock at
// .lyx/scout/<lang>/ — never _lyx/. That distinction matters: .lyx/ is
// ephemeral, machine-bound runtime state (per the Cwd Resolution Invariant's
// durable-vs-ephemeral split), and a live daemon's PID, socket path, and
// spawn time mean nothing on another machine or after this one is
// rebooted, so this state must never be git-committed the way _lyx/'s
// config lives.
//
// A recorded daemon is stale, forcing a fresh spawn rather than a reuse,
// under a two-part check (daemonStale): its PID is no longer alive
// (proc.IsAlive), or its recorded ProtocolVersion does not match this
// binary's supervisedProtocolVersion. That protocol version is lyx's own
// wire-compatibility marker for the supervised daemon protocol itself —
// bumped when a future lyx change to that protocol needs it — and must not
// be confused with gopls's own version, which registry.Entry.PinnedVersion
// pins separately.
//
// The daemon's socket path is a deterministic function of (anchorRoot,
// lang), never randomly chosen at spawn time. This is what makes
// stale-socket cleanup across restarts simple: a fresh spawn can always
// remove-if-exists the one predictable path before binding, with no
// separate bookkeeping needed to remember what the previous socket was
// called.
//
// A caller that keeps losing the spawn race, or keeps finding a state that
// reads healthy but never becomes dialable, is bounded by
// ErrServerSpawnTimeout: ensureSupervised's whole retry loop is wrapped in a
// deadline, and a losing caller that never observes a healthy, dialable
// daemon before that deadline expires gets this error rather than blocking
// indefinitely.
//
// # The typed error vocabulary
//
// Every engine failure mode is a distinct sentinel or data-carrying error
// type (errors.go), each satisfying errors.Is against a package-level
// sentinel regardless of its concrete field values: ErrNoLanguage (no
// registry entry's markers matched under the target directory),
// ErrServerNotFound (the entry's Command[0] binary is absent on $PATH, or —
// for Go — the toolchain manager could not resolve/install the pinned
// binary; carries InstallHint), ErrSymbolNotFound (workspace/symbol
// resolved the queried name to zero candidates), ErrAmbiguousSymbol
// (resolved to more than one candidate; carries every candidate as
// file:line:col), ErrResolverUnsupported (the launched server does not
// advertise workspaceSymbolProvider), ErrServerTimeout (a phase's deadline
// expired; names the stalled phase and the timeout), and
// ErrServerSpawnTimeout (the supervised strategy's bounded spawn-race retry
// gave up without ever observing a healthy daemon). ErrSymbolNotFound and
// ErrAmbiguousSymbol exist as their own distinct types specifically so
// internal/scoutcli can tell "confirmed absent" apart from "found, but
// ambiguous" without parsing error strings — exit codes and the rest of
// that CLI-level contract are internal/scoutcli's concern, not this
// package's; see its own package documentation for the full shape.
//
// # Scope boundaries — what this package deliberately does not do
//
//   - No in-process go/packages arm. The spike's recommended
//     sub-millisecond, zero-false-positive Go-only path is not wired here;
//     scout always goes through LSP, including for Go (gopls), trading
//     peak Go-only precision/speed for uniform multi-language coverage.
//   - No call hierarchy, no implementation. Only textDocument/references,
//     textDocument/definition, and the workspace/symbol resolver are wired.
//     The spike's call-hierarchy fix (TypesInfo.Uses/Defs-based, not
//     AST-pattern-based) does not generalize to a language-agnostic LSP
//     client, and implementation was never in this package's rubric.
//   - No lyx config reconcile wiring for servers.yaml yet. ConfigTemplate()
//     exists and mirrors models.yaml's shape, but scout is not yet added
//     to lyx config reconcile's seed-only module list; an operator who wants
//     an overlay writes _lyx/config/servers.yaml by hand today.
//   - Symbol does not share resolvePosition's ambiguity-collapsing
//     behavior. Unlike References/Definition, Symbol (symbol.go) never
//     collapses multiple workspace/symbol candidates into an
//     ErrAmbiguousSymbol failure — returning every match is the whole point
//     of a symbol search, not an error state needing disambiguation. See
//     symbol.go's own doc comment for the full rationale.
package scoutengine
