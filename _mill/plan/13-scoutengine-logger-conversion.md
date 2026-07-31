# Batch: scoutengine-logger-conversion

```yaml
task: Diagnostic tracing (trace) on the logger module
batch: scoutengine-logger-conversion
number: 13
cards: 5
verify: go test ./internal/scoutengine/...
depends-on: [5, 10]
```

## Batch Scope

The most surgical adoption batch: widens `internal/scoutengine`'s machine-enforced leaf import allowlist to admit `internal/logger`, converts its six ad-hoc `fmt.Fprintf(os.Stderr, ...)` call sites (five in `lspclient.go`, one in `ensureserver.go`) to structured `logger.Warn` calls, and adds the missing spawn/teardown `logger.Info`/`Warn` calls at its supervised-daemon spawn point that CONSTRAINTS.md's Live-Substrate Spawn Observability entry already obliges scoutengine to have. Per discussion.md's `scoutengine-allowlist` decision, widening the allowlist without also adding this missing observability would be incoherent — both land in this batch.

`depends-on` includes batch 10 (not just batch 5) purely to serialize CONSTRAINTS.md edits: batch 10 also edits CONSTRAINTS.md's Treadle Runner-Seam Invariant entry, and the two batches are otherwise independent (neither's content depends on the other) — the edge exists only so mill-go never runs them in parallel against the same file.

## Cards

### Card 40: Widen the leaf allowlist

- **Context:** none
- **Edits:**
  - `internal/scoutengine/leaf_enforcement_test.go`
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/scoutengine/leaf_enforcement_test.go`:
  - Add `"github.com/Knatte18/loomyard/internal/logger": true` to the `allowedImports` map (currently lines 24-29, listing `hubgeometry`/`lock`/`proc`/`yaml.v3`).
  - Update the file's header comment (lines 1-8, currently "production code in internal/scoutengine imports ONLY the standard library, internal/hubgeometry, internal/lock, internal/proc, and gopkg.in/yaml.v3") to add internal/logger to that list.
  - Fix the failure message at line 106 (currently `"Scoutengine Leaf Invariant violated; imports outside the allowlist (stdlib + hubgeometry + lock + yaml.v3) found: %v"`) in the same edit: it is **already stale today**, omitting internal/proc even though internal/proc has long been a live `allowedImports` entry — fix it to read `"...(stdlib + hubgeometry + lock + proc + logger + yaml.v3)..."`, correcting the pre-existing omission and adding `logger` in one pass.

  In CONSTRAINTS.md's "Scoutengine Leaf Invariant" entry, add a new justification bullet for internal/logger, in the same style as the existing internal/lock/internal/proc bullets: state that `EnsureServer` spawns a detached, session-long daemon, making scoutengine a live-substrate spawn point that CONSTRAINTS' own Live-Substrate Spawn Observability entry already requires to log through internal/logger, and that widening costs nothing in real dependency surface since scoutengine already allows internal/hubgeometry (logger's only new transitive import) — the transitive set does not grow.
- **Commit:** `feat(scoutengine): widen the leaf allowlist to admit internal/logger`

### Card 41: Add a `lang` field to `lspClient`, set at production construction sites only

- **Context:**
  - `internal/scoutengine/ensureserver.go`
  - `internal/scoutengine/refs.go`
- **Edits:**
  - `internal/scoutengine/lspclient.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  `lspClient` (`internal/scoutengine/lspclient.go`, struct at line 142) has no `lang`/language-identifier field today. `newLSPClientFromRW(rwc io.ReadWriteCloser) *lspClient` (line 234) is called directly by name from six existing test files (`daemonstate_test.go`, `symbol_test.go`, `refs_test.go`, `ensureserver_test.go`, `supervised_test.go`, `lspclient_test.go` — confirmed by grep, dozens of call sites total) — **do not change its signature**, or every one of those call sites breaks for no benefit (those tests have no `lang` to supply and do not need one).

  Instead: add an unexported `lang string` field to the `lspClient` struct (zero-value `""` is an acceptable default for every test-constructed client). Leave all three constructors' signatures unchanged — `newLSPClient(command []string) (*lspClient, error)` (line 192), `newLSPClientFromRW(rwc io.ReadWriteCloser) *lspClient` (line 234), and `newLSPClientDial(ctx context.Context, network, address string) (*lspClient, error)` (line 258, itself calling `newLSPClientFromRW` internally at line 264). Set the field directly (`client.lang = lang`, same package) at each **production** construction call site where a language identifier is already in scope:
  - `internal/scoutengine/ensureserver.go:198` — `client, err := newLSPClient(argv)`.
  - `internal/scoutengine/ensureserver.go:363` — `if client, dialErr := newLSPClientDial(ctx, network, address); dialErr == nil { ... }`.
  - `internal/scoutengine/ensureserver.go:578` — `client, dialErr = newLSPClientDial(ctx, "unix", socketPath)`.
  - `internal/scoutengine/refs.go:107` — `client, err := newLSPClient(entry.Command)` (confirm `lang` or an equivalent language identifier is in scope in the enclosing function before setting it; if none is available at this specific call site, leave it unset there rather than inventing one — only the `ensureserver.go` sites are required).

  This is the prerequisite for Card 42's five stderr-site conversions in `lspclient.go`, which read `c.lang`.
- **Commit:** `feat(scoutengine): add an unexported lang field to lspClient, set at production construction sites`

### Card 42: Convert `lspclient.go`'s five stderr writes

- **Context:** none
- **Edits:**
  - `internal/scoutengine/lspclient.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Convert the five ad-hoc `fmt.Fprintf(os.Stderr, ...)` calls in `internal/scoutengine/lspclient.go` to `logger.Warn`, using Card 41's new `c.lang` field plus whatever else is in scope at each site:
  - `lspclient.go:596` (inside `close()`, shutdown request failure) — `logger.Warn("scoutengine: lsp shutdown request", "lang", c.lang, "err", err)`.
  - `lspclient.go:599` (inside `close()`, exit notification failure) — `logger.Warn("scoutengine: lsp exit notification", "lang", c.lang, "err", err)`.
  - `lspclient.go:604` (inside `close()`, `c.cmd.Wait()` failure) — `logger.Warn("scoutengine: lsp process exit", "lang", c.lang, "err", err)`.
  - `lspclient.go:627` (inside `kill()`, `c.cmd.Process.Kill()` failure, guarded by the existing nil-check at line 623 so `c.cmd.Process.Pid` is safe to read here) — `logger.Warn("scoutengine: kill lsp process", "lang", c.lang, "pid", c.cmd.Process.Pid, "err", err)`.
  - `lspclient.go:630` (inside `kill()`, `c.cmd.Wait()` failure after kill, same nil-check guard) — `logger.Warn("scoutengine: lsp process exit after kill", "lang", c.lang, "pid", c.cmd.Process.Pid, "err", err)`.

  Leave `lspclient.go:211`'s `cmd.Stderr = os.Stderr` untouched — it is direct passthrough of the spawned LSP server's own stderr, not a diagnostic write, and is explicitly out of scope per discussion.md's Scope → Out and the `adoption-scope` decision.
- **Commit:** `feat(scoutengine): convert lspclient.go's five stderr diagnostics to logger.Warn`

### Card 43: Convert `ensureserver.go`'s wedged-daemon kill

- **Context:** none
- **Edits:**
  - `internal/scoutengine/ensureserver.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Convert `internal/scoutengine/ensureserver.go:465`'s `fmt.Fprintf(os.Stderr, "scoutengine: kill wedged supervised daemon pid %d for %q: %v\n", escalationState.PID, lang, err)` to `logger.Warn("scoutengine: kill wedged supervised daemon", "pid", escalationState.PID, "lang", lang, "err", err)` — all three values (`escalationState.PID`, `lang`, `err`) are already in scope at this site (guarded by the existing `if escalationFound { if err := proc.KillPID(...); err != nil { ... } }` block at lines 463-467).
- **Commit:** `feat(scoutengine): convert the wedged-daemon-kill stderr write to logger.Warn`

### Card 44: Missing spawn observability

- **Context:** none
- **Edits:**
  - `internal/scoutengine/ensureserver.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add the spawn-side observability CONSTRAINTS.md's Live-Substrate Spawn Observability entry already requires for a live-substrate spawn point, per discussion.md's `scoutengine-allowlist` decision's "The spawn-logging obligation is IN this task" paragraph. Immediately after the supervised daemon's `cmd.Start()` succeeds (`ensureserver.go:520`, `cmd := exec.Command(argv[0], argv[1:]...)` built from `supervisedArgv(command, socketPath)` at line 519), add `logger.Info("scoutengine: spawned supervised daemon", "lang", lang, "pid", cmd.Process.Pid, "socket", socketPath)` — naming lang, pid, and socket path as discussion.md's decision text specifies verbatim.

  **There is no separate "normal teardown" call site to add** — confirmed by reading `ensureserver.go` in full: the function's own doc comment (lines 303-308) states the daemon "ends on its own: its own idle timeout... or a future restart's stale-socket cleanup finding it already dead," and `refs.go`'s `teardownConnection`'s `connKindSupervised` branch (line 146-151) is a deliberate bare `return` — the daemon must outlive the call, so scoutengine's own code never observes a clean exit event to log. Discussion.md's "both halves of the lifecycle" (spawn + teardown) is therefore satisfied by this card's spawn `Info` plus Card 43's already-converted wedged-kill `Warn` at `ensureserver.go:465` — the wedged-kill IS the only teardown event scoutengine's own code can ever see; do not invent a clean-teardown call site that does not exist.
- **Commit:** `feat(scoutengine): add spawn observability at the supervised daemon's cmd.Start()`

## Batch Tests

`verify: go test ./internal/scoutengine/...` — this must pass `TestLeafInvariant_AllowlistOnly` with `internal/logger` now allowlisted (Card 40) and the existing daemon-lifecycle/lspclient test suite unaffected (instrumentation only; Card 41 deliberately leaves every constructor signature unchanged, so none of the six existing test files that call `newLSPClientFromRW` directly need any edit).
</content>
