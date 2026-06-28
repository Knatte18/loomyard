# Batch: payload-contract

```yaml
task: "Board fixes from sandbox run — payload keys, help, rerender"
batch: "payload-contract"
number: 1
cards: 5
verify: go test ./internal/board/ ./cmd/lyx/
depends-on: []
```

## Batch Scope

Delivers the full board payload-contract overhaul: the `set-phase`→`set-status` /
`phase`→`status` rename, the `slug`-or-`id` lookup contract on the single-target
commands, error-on-missing for mutations, strict unknown-key rejection on every write and
lookup payload (upsert chokepoint + CLI-boundary shapes), and the `merge` `set_status`
object shape. This is one batch because every change lives in the four board source files
`cli.go`/`board.go`/`store.go`/`task.go` (plus the `cmd/lyx` help-tree pin) and they share
one mental model — the JSON payload contract. The external interface the next batch
(`cli-help`) consumes is the final set of command names and payload field names defined
here. Batch-local decision: the store's existing `any` type-switch resolvers
(`Store.GetTask`/`RemoveTask`/`SetPhase`) are REUSED — the CLI boundary builds the right
`any` value (Go `string` for slug, JSON-number for id) from the validated payload, so the
store resolution logic does not need rewriting, only the `SetPhase`→`SetStatus` rename and
its new error-on-missing.

## Cards

### Card 1: Rename `set-phase`→`set-status` and `phase`→`status`

- **Context:**
  - `_mill/discussion.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/board/cli.go`
  - `internal/board/board.go`
  - `internal/board/store.go`
  - `cmd/lyx/helptree_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Rename the cobra subcommand from `set-phase` to `set-status`: change
  `setPhaseCmd` `Use: "set-phase [json-payload]"` → `Use: "set-status [json-payload]"` and
  its `Short` to `"Set or clear the status of a task"` in `internal/board/cli.go`. Rename
  the facade method `Board.SetPhase` → `Board.SetStatus` in `internal/board/board.go` and
  the store method `Store.SetPhase` → `Store.SetStatus` in `internal/board/store.go`
  (update all call sites, including `Store.MergeTasks` which calls `s.SetPhase`). The
  payload key for the status value is renamed from `phase` to `status` in the
  `set-status` RunE struct (`Phase *string \`json:"phase"\`` → `Status *string
  \`json:"status"\``); the `merge` payload's `set_phase` element is handled in Card 5. Do
  NOT rename the on-disk `Task.Status` field or its `json:"status,omitempty"` tag — it is
  already `status`. Update the pinned board subcommand set in
  `cmd/lyx/helptree_test.go` (the `wantSubs` list for the `board` module) replacing
  `"set-phase"` with `"set-status"`. The `set-status` command must keep a non-empty
  `Short` so `cmd/lyx/drift_test.go`'s `TestDriftGuard_AllCommandsHaveShort` stays green.
- **Commit:** `refactor(board): rename set-phase to set-status, phase key to status`

### Card 2: `slug`-or-`id` lookup contract on `get`/`set-status`/`remove`

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/board/cli.go`
  - `internal/board/board.go`
  - `internal/board/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Replace the `id_or_slug` payload key on the `get`, `remove`, and
  `set-status` RunE handlers in `internal/board/cli.go` with a lookup that accepts exactly
  one of `slug` (non-empty string) or `id` (JSON integer). Implement a shared helper in
  cli.go (e.g. `resolveLookup(raw []byte, extraKeys ...string) (any, map[string]any,
  error)`) that: unmarshals the payload into a `map[string]any`; rejects any key outside
  the allowed set (`{slug, id}` for get/remove, `{slug, id, status}` for set-status —
  pass the extra allowed keys in); detects presence by map-key membership (so `id:0` is a
  valid distinct-from-absent lookup, since `store.nextID()` assigns 0 to the first task);
  errors if neither or both of `slug`/`id` are present; and returns the resolved selector
  as an `any` — a Go `string` when `slug` is present, or the JSON number (`float64`) when
  `id` is present. Pass that `any` to the existing `Board.GetTask`/`Board.RemoveTask`/
  `Board.SetStatus`, whose store-level type switches already handle `string`/`int`/
  `float64`. `Board.GetTask`/`RemoveTask`/`SetStatus` keep their `idOrSlug any` parameter
  shape (rename the param for clarity if desired) — no store resolution rewrite. Add
  `internal/board/cli_test.go` cases: `get`/`remove`/`set-status` succeed with
  `{"slug":...}`; succeed with `{"id":N}`; `get '{"id":0}'` resolves the first-created
  task (ID 0); error with neither key; error with both keys; cover the int-vs-float64
  JSON-number path.
- **Commit:** `feat(board): accept slug or id on get/set-status/remove`

### Card 3: Error on missing target + require `status` key on `set-status`

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/board/store.go`
  - `internal/board/cli.go`
  - `internal/board/store_test.go`
  - `internal/board/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Change `Store.SetStatus` (renamed in Card 1) in
  `internal/board/store.go` so that when no task matches the id/slug it returns an error
  `fmt.Errorf("task not found: %v", idOrSlug)` instead of the current silent `return nil`
  (remove the "SetStatus is idempotent: no error for missing task" no-op). In
  `internal/board/cli.go`, make the `set-status` handler require the `status` key to be
  present in the payload map: an absent `status` key is an error (`missing required field:
  status`), while an explicit `"status":null` clears the status (decode into the map first
  — Card 2's helper — then read `status` only when the key is present). Update the
  existing `internal/board/store_test.go` test that asserts `SetPhase`/`SetStatus` is a
  silent no-op on a missing task to assert the new "task not found" error (intended
  contract change). Add `internal/board/cli_test.go` cases: `set-status '{"slug":"x"}'`
  (no status) errors with `missing required field: status`;
  `set-status '{"slug":"x","status":null}'` succeeds and clears the status;
  `set-status` on a non-existent slug/id errors with "task not found".
- **Commit:** `fix(board): error on missing set-status target, require status key`

### Card 4: Upsert chokepoint allowlist + fold `group` rejection

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/board/store.go`
  - `internal/board/task.go`
  - `internal/board/store_test.go`
  - `internal/board/task_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add an upsert-fields allowlist at the store chokepoint so create,
  patch, and merge-upsert are all covered uniformly. In `internal/board/store.go`,
  validate the incoming `fields map[string]any` keys against the allowed set
  `{slug, title, depends_on, isolated, deferred, brief, body, status}` at the top of
  `Store.UpsertTask`, for each task in `Store.UpsertTasksBatch`, and for the `upsert` map
  in `Store.MergeTasks` — before the `NewTask`/`ApplyPatch` JSON round-trip — returning a
  clear error naming the offending key (e.g. `fmt.Errorf("unknown field: %q", k)`; for
  the key `phase` add the hint `(did you mean "status"?)`). Fold the existing `group`
  rejection (`internal/board/task.go` `NewTask`/`ApplyPatch`, which currently returns
  "group key is not allowed…") into this single allowlist so there is one error path: the
  allowlist rejection covers `group` (and `id`, `phase`) automatically. Remove the now-
  redundant explicit `group` checks from `task.go` only if the allowlist fully subsumes
  them (every `NewTask`/`ApplyPatch` caller now passes through the store allowlist — they
  do). `status` IS in the allowed set, so `upsert '{"slug":"x","status":"active"}'` sets
  the status (the W11 fix). Add `internal/board/store_test.go` cases: upsert/upsert-batch/
  merge-upsert with a stray `phase` key error; with a typo key (`titel`) error; `group`
  still errors via the allowlist; a payload with only allowed keys (including `status`)
  succeeds and persists `status`. Update `internal/board/task_test.go` if it asserts the
  old `group`-specific error message.
- **Commit:** `feat(board): reject unknown upsert fields at the store chokepoint`

### Card 5: CLI-boundary strict shapes + `merge` `set_status` object

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/board/cli.go`
  - `internal/board/board.go`
  - `internal/board/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add strict-key/shape validation at the cli.go RunE boundary for the
  remaining payloads (decode into `map[string]any` first to detect unknown keys):
  (a) `set-deps` accepts only `{slug, depends_on}` with `depends_on` required — an absent
  `depends_on` errors (`missing required field: depends_on`), an explicit `[]` clears the
  list; a stray key (e.g. `depends`) errors. (b) `upsert-batch` accepts only the top-level
  wrapper key `{tasks}` — a typo'd wrapper (`taks`) errors, and an absent or empty `tasks`
  array errors (no silent `count:0`). (c) `merge` accepts only top-level keys
  `{remove_slugs, upsert, set_status}` — a stale top-level `set_phase` errors. The `merge`
  status step changes from the positional `set_phase: [id_or_slug, phase]` to an object
  `set_status: {"slug"|"id": …, "status": …}` validated identically to `set-status`
  (reuse Card 2's lookup helper with `status` allowed, exactly-one-of `slug`/`id`,
  `status` key required). Update `Board.MergeTasks` in `internal/board/board.go`: replace
  the `setPhase *[2]any` parameter with a resolved status-update selector (e.g. a small
  struct carrying the `any` id/slug selector and the `*string` status, or `nil` when the
  `set_status` object is omitted); `Store.MergeTasks` then calls the renamed
  `Store.SetStatus` (which now errors on missing target). Add `internal/board/cli_test.go`
  cases: `merge` with `set_status:{"slug":"x","phase":"done"}` errors (inner validated
  like set-status); `merge` with stale top-level `set_phase` errors; `set-deps` with a
  stray key errors; `set-deps '{"slug":"x"}'` (no depends_on) errors;
  `set-deps '{"slug":"x","depends_on":[]}'` succeeds and clears; `upsert-batch '{"taks":[…]}'`
  errors; `upsert-batch` with absent/empty `tasks` errors.
- **Commit:** `feat(board): strict payload shapes for set-deps, upsert-batch, merge`

## Batch Tests

`verify: go test ./internal/board/ ./cmd/lyx/` covers the board package unit tests
(`cli_test.go`, `store_test.go`, `task_test.go`, `board_test.go`) edited across cards 2–5,
and the `cmd/lyx` package tests (`helptree_test.go` pin updated in card 1, `drift_test.go`
status-guard) affected by the command rename. Both packages are in the batch's blast
radius, so the scope is the two packages, not a single file. No `PYTHONPATH=` prefix — Go
project. Key scenarios: the B1/B2 regression (slug + id lookups succeed; `id:0` resolves
the first task), error-on-missing for mutations, `get` still returns `task:null` on a
valid-but-absent target, and unknown-key rejection on every write/lookup payload.
