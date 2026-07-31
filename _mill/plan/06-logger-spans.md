# Batch: logger-spans

```yaml
task: Diagnostic tracing (trace) on the logger module
batch: logger-spans
number: 6
cards: 4
verify: go test ./internal/logger/...
depends-on: [2, 5]
```

## Batch Scope

Adds explicit-parent spans in a new `internal/logger/span.go` per discussion.md's `explicit-span-parenting` decision: `StartSpan`/`Child`/`End` with span-scoped `Debug`/`Info`/`Warn` methods stamping `span=<dotted path>`, and the level split from that same decision (open/close at Debug, `End(err)` at Warn). Depends on batch 2 (a span's root sits under the process `TraceID()`) and batch 5 (span-scoped emit methods route through the same dual-handler fan-out `Debug`/`Info`/`Warn` do, so `span=` rides on whichever handler(s) accept the record).

## Cards

### Card 23: `Span` type — explicit parent, dotted path

- **Context:**
  - `internal/logger/trace.go`
- **Edits:** none
- **Creates:**
  - `internal/logger/span.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `internal/logger/span.go`:

  ```go
  type Span struct {
  	path string // dotted path, e.g. "burler.round.spawn"
  	err  error  // set by End, read by nothing outside End itself; kept for clarity
  }

  func StartSpan(name string, args ...any) *Span
  func (s *Span) Child(name string, args ...any) *Span
  func (s *Span) End(err error)
  ```

  `StartSpan(name, args...)` opens a root span under the process trace: `path = name`. `(s *Span) Child(name, args...)` returns a new `*Span` with `path = s.path + "." + name` — an explicit parent handle, no ambient package-level span stack (per `explicit-span-parenting`: "There is no ambient 'current span' global"). A `*Span` is a plain value only ever touched by the goroutine holding it — no locking of its own, per `concurrency-contract`'s note that spans hold no shared state. `End(err error)` is called via `defer sp.End(err)` at the caller's site; it does not "restore" any global (there is nothing to restore).
- **Commit:** `feat(logger): add explicit-parent Span type with dotted path construction`

### Card 24: Span-scoped emit methods and record levels

- **Context:** none
- **Edits:**
  - `internal/logger/span.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add span-scoped emit methods to `internal/logger/span.go`:

  ```go
  func (s *Span) Debug(msg string, args ...any)
  func (s *Span) Info(msg string, args ...any)
  func (s *Span) Warn(msg string, args ...any)
  ```

  Each stamps `span=<s.path>` (in addition to the `trace=` field the package-level `Debug`/`Info`/`Warn`, batch 5's Card 18, already stamp on every line) and otherwise routes through the same dual-handler fan-out as the package-level functions — an `Info` called via `sp.Info(...)` must reach the durable sink exactly like a package-level `Info` call does, carrying `span=` in addition to `trace=`.

  Record levels, per `explicit-span-parenting`'s "Levels of the span records themselves" paragraph: `StartSpan` and `Child` each emit their own open record **at Debug** (via the package-level `Debug`, stamped with the new span's own `span=` path); `End(nil)` emits its close record **at Debug**; `End(err)` with a non-nil `err` emits its close record **at Warn**, carrying the error as a field (e.g. `"err", err`). Consequence to preserve, not defeat: on a normal run, no span open/close record reaches the durable file — only a failing `End(err)` does — while the `span=` path still rides on every line emitted through `sp.Debug`/`sp.Info`/`sp.Warn` regardless of that line's own level.
- **Commit:** `feat(logger): add span-scoped emit methods with Debug-open/close, Warn-on-error levels`

### Card 25: Span nesting tests

- **Context:** none
- **Edits:** none
- **Creates:**
  - `internal/logger/span_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Untagged unit tests in `internal/logger/span_test.go`:
  - `StartSpan("a") → Child("b") → Child("c")` produces the dotted path `"a.b.c"` on the innermost span's emitted lines (assert via a captured-output buffer, `SetOutput`, following `logger_test.go`'s `withCapturedOutput` pattern).
  - `End` on a span restores nothing globally — there is no global state to restore; confirm by ending a child span and then emitting from a **sibling** span created independently, asserting the sibling's own `span=` path is unaffected (no corruption from the ended child).
  - An unended child span does not corrupt a sibling's path — construct two children of the same parent, never call `End` on the first, and confirm the second's emitted `span=` is still exactly its own dotted path.
  - `End(err)` with a non-nil error records the error (assert the close-record's captured line contains the error's text at Warn level, per Card 24).
- **Commit:** `test(logger): cover span nesting, sibling independence, and End(err)`

### Card 26: Span record level tests

- **Context:** none
- **Edits:**
  - `internal/logger/span_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add to `internal/logger/span_test.go` (created by Card 25), using `SetDurableSinkDir(t.TempDir())` (batch 4):
  - `StartSpan`/`Child`/`End(nil)`'s own open and close records are absent from the durable sink file (they emit at Debug, which never reaches the durable handler per batch 5's `Enabled` gate) — assert the durable file, once opened by some other Info+ activity in the test, contains none of the span open/close lines.
  - `End(err)` with a non-nil error **is** present in the durable file (it emits at Warn).
  - An `Info` emitted through a span-scoped method (`sp.Info(...)`) carries the `span=` path into the durable file even though that span's own open/close records did not — i.e. the causal structure survives via the `span=` field on ordinary Info/Warn lines, not via the open/close records themselves (this is the property discussion.md's `explicit-span-parenting` decision and its correction to `no-reader-cli`'s forward-compat property (2) both depend on).
- **Commit:** `test(logger): cover span record levels and durable-sink visibility of span= on Info/Warn lines`

## Batch Tests

`verify: go test ./internal/logger/...` runs `span_test.go` (Cards 25+26) alongside every prior batch's `internal/logger` tests.
</content>
