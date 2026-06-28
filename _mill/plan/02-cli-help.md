# Batch: cli-help

```yaml
task: "Board fixes from sandbox run — payload keys, help, rerender"
batch: "cli-help"
number: 2
cards: 2
verify: go test ./internal/board/
depends-on: [1]
```

## Batch Scope

Delivers W1: a `Long` help block on every board leaf command documenting its payload
schema with an example, plus a test that pins the documented schema. Depends on batch 1
because the help text must describe the final command names and field names that batch 1
establishes (`set-status`, `status`, `slug`/`id`, `set_status`, no `phase`/`id_or_slug`/
`group`). Per the CLI/Cobra Invariant, these `Long` blocks ARE the discovery-path
documentation for the board CLI (there is no `docs/modules/board.md`), so they are the
primary doc deliverable. This batch only adds `Long` strings and a help test — no
behavior change.

## Cards

### Card 6: Add `Long` help to all seven board leaf commands

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/board/cli.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Add a `Long` field to each of the seven board leaf commands in
  `internal/board/cli.go`, each documenting required/optional fields and a concrete
  example JSON payload reflecting the post-batch-1 schema. The reverted draft in commit
  `c9d5c59` is a starting point but is WRONG in ways that must be corrected: it documented
  a `group` field (rejected), used `id_or_slug`/`phase` (removed), and listed `group` for
  upsert. Write the blocks against the final schema:
  - `upsert`: required `slug` (string); optional `title` (string), `brief` (string),
    `body` (string, full markdown / proposal), `depends_on` (array of slug strings),
    `isolated` (bool), `deferred` (bool), `status` (string). Note unknown keys are
    rejected. Example: `lyx board upsert '{"slug":"my-task","title":"My Task","brief":"Short summary"}'`.
  - `upsert-batch`: payload `{"tasks":[ … ]}`, each element the same fields as `upsert`
    (`slug` required); an absent/empty `tasks` is an error. Example:
    `lyx board upsert-batch '{"tasks":[{"slug":"t1","title":"One"},{"slug":"t2","title":"Two"}]}'`.
  - `set-status`: fields `slug` (string) OR `id` (integer) — exactly one; `status`
    (string, required; `null` clears). Example:
    `lyx board set-status '{"slug":"my-task","status":"active"}'` and the clear form
    `lyx board set-status '{"id":96,"status":null}'`.
  - `remove`: fields `slug` (string) OR `id` (integer) — exactly one. Example:
    `lyx board remove '{"slug":"my-task"}'`.
  - `get`: fields `slug` (string) OR `id` (integer) — exactly one; returns `{"task":null}`
    if not found (not an error). Example: `lyx board get '{"id":96}'`.
  - `merge`: fields `remove_slugs` (array of slug strings, optional), `upsert` (object,
    same fields as `upsert`, required), `set_status` (object `{slug|id, status}` validated
    like `set-status`, optional). Example:
    `lyx board merge '{"remove_slugs":["old"],"upsert":{"slug":"new","title":"New"},"set_status":{"slug":"new","status":"active"}}'`.
  - `set-deps`: fields `slug` (string, required), `depends_on` (array of slug strings,
    required — replaces the existing list wholesale; `[]` clears). Example:
    `lyx board set-deps '{"slug":"my-task","depends_on":["dep-a","dep-b"]}'`.
- **Commit:** `docs(board): document payload schema in leaf-command --help`

### Card 7: Help-schema test

- **Context:**
  - `internal/board/cli.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/board/help_test.go`
- **Deletes:** none
- **Requirements:** Create `internal/board/help_test.go` with a test that drives each
  board leaf command's `--help` through the `RunCLI` seam (e.g. `RunCLI(buf, []string{
  "set-status", "--help"})`) and asserts the `Long` output contains the documented field
  names for that command (e.g. `status` for set-status, `slug` and `id` for get/remove,
  `set_status` for merge, `depends_on` for set-deps, `tasks` for upsert-batch) and does
  NOT contain any removed token (`id_or_slug`, `phase`, `group`). Cover all seven leaf
  commands. Use the existing test conventions in `internal/board/cli_test.go` for driving
  `RunCLI` and capturing output.
- **Commit:** `test(board): pin documented --help payload schema`

## Batch Tests

`verify: go test ./internal/board/` runs the board package tests including the new
`help_test.go`. Scope is the single board package — the `Long` additions do not change
command names, so `cmd/lyx` help-tree/drift tests are unaffected. No `PYTHONPATH=` prefix
— Go project. The help test is the guardrail that the documented schema matches the
final field vocabulary and that no removed token leaks back into help text.
