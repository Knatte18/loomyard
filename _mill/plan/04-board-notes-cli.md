# Batch: boardcli: notes CLI surface + promote-note command

```yaml
task: 'board: move storage to weft:main'
batch: 'boardcli: notes CLI surface + promote-note command'
number: 4
cards: 5
verify: go test ./internal/boardcli/... && go test -tags integration ./internal/boardcli/...
depends-on: [3]
```

## Rename mechanic

Not applicable — this batch contains no `Moves:` entries.

## Batch Scope

This batch wires batch 3's new `Board` methods (`UpsertNote`, `SetNoteStatus`, `RemoveNote`, `MergeNotes`, `SetNoteDeps`, `UpsertNotesBatch`, `GetNote`, `ListNotesBrief`, `ListNotesFull`, `PromoteNote`) into `internal/boardcli`'s cobra tree: a new `notes` subcommand group mirroring the existing 9 task verbs, plus a new top-level `promote-note` command. It also fixes the help text `_mill/discussion.md` flags as stale the moment this task ships (`board.yaml`'s renamed config keys, `Home.md`/`_Sidebar.md` no longer produced). This batch depends on batch 3 for every `Board` method it calls; it does not touch `internal/boardengine` itself. The external interface this batch hands to batch 5 (`cmd/lyx`): the CLI surface's final shape (`lyx board notes <verb>`, `lyx board promote-note`), which batch 5's help-tree/registration/guard coverage checks against.

## Cards

### Card 20: `notes` subcommand group

- **Context:**
  - `internal/boardengine/board.go`
- **Edits:**
  - `internal/boardcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `notesCmd := &cobra.Command{Use: "notes", Short: "manage not-yet-claimable manifest entries (notes.json)", RunE: clihelp.GroupRunE}` parent command (mirrors the root `board` command's own `GroupRunE` wiring, so bare `lyx board notes` lists its subcommands and an unknown `lyx board notes bogus` emits a JSON error envelope). Add nine child commands, each a direct mirror of its existing task-verb counterpart with the same `Use`/flag/payload shape, `Short`/`Long` text swapping "task"/"tasks.json" for "note"/"notes.json" (e.g. `upsertCmd`'s `Short: "Create or update a single task"` becomes `notesUpsertCmd`'s `Short: "Create or update a single note"`), and a `RunE` body byte-for-byte identical to its counterpart's (same JSON parsing, same `resolveLookup`/`output*` helper calls) except the one line invoking `b.<Verb>Task(...)`/`b.<bare-verb>(...)` is replaced with `b.<Verb>Note(...)`/`b.<bare-verb>Notes(...)`: `notesUpsertCmd` (`Use: "upsert [json-payload]"` → `b.UpsertNote(fields)`), `notesUpsertBatchCmd` (`Use: "upsert-batch [json-payload]"` → `b.UpsertNotesBatch(notes)`), `notesSetStatusCmd` (`Use: "set-status [json-payload]"` → `b.SetNoteStatus(selector, status)`), `notesRemoveCmd` (`Use: "remove [json-payload]"` → `b.RemoveNote(selector)`), `notesGetCmd` (`Use: "get [json-payload]"` → `b.GetNote(selector)`), `notesListCmd` (`Use: "list"` → `b.ListNotesBrief()`), `notesListFullCmd` (`Use: "list-full"` → `b.ListNotesFull()`), `notesMergeCmd` (`Use: "merge [json-payload]"` → `b.MergeNotes(removeSlugs, upsertFields, setStatusPtr)`), `notesSetDepsCmd` (`Use: "set-deps [json-payload]"` → `b.SetNoteDeps(slug, dependsOn)`). Every payload-shape validation (`resolveLookup`'s allowed-key enforcement, `upsert-batch`'s wrapper-key/empty-array checks, `merge`'s top-level/inner `set_status` key checks, `set-deps`'s required-`depends_on` check) stays byte-for-byte identical to its task-verb counterpart — only which `Board` method receives the parsed result changes. Register the nine children on `notesCmd` via `notesCmd.AddCommand(...)`, then `cmd.AddCommand(notesCmd)` alongside the existing top-level `AddCommand(...)` call.
- **Commit:** `feat(boardcli): add notes subcommand group mirroring the task verb set`

### Card 21: `promote-note` command

- **Context:**
  - `internal/boardengine/board.go`
  - `internal/boardcli/cli.go`
- **Edits:**
  - `internal/boardcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a top-level `promoteNoteCmd := &cobra.Command{Use: "promote-note [json-payload]", Short: "Move a note from notes.json into tasks.json"}` with a `Long` documenting the `{slug|id}` lookup payload (identical shape to `get`/`remove`: exactly one of `slug` or `id`, per `resolveLookup`'s existing contract) and stating the move is atomic and idempotent-on-retry (per `Board.PromoteNote`'s doc comment from batch 3, Card 16). `RunE`, via `clihelp.WrapRun`: require `len(args) > 0` (json payload required, matching `get`/`remove`'s existing error shape); call `resolveLookup([]byte(args[0]))` (no extra keys — same call shape as `remove`/`get`) to obtain `selector`; call `b.PromoteNote(selector)`; on error, `outputError`; on success, `outputSuccessWithTask(out, task)` (matching `get`'s/`upsert`'s success envelope shape: `{"ok":true,"task":{...}}`). Register via `cmd.AddCommand(promoteNoteCmd)` at the top level (a sibling of `notesCmd`, not nested under it — `promote-note` moves an entry OUT of `notes.json`, it is not itself a notes-scoped verb).
- **Commit:** `feat(boardcli): add promote-note command`

### Card 22: fix stale help text (config keys, rerender, package docs)

- **Context:** none
- **Edits:**
  - `internal/boardcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Update `cli.go`'s file-header doc comment: "Command() returns the root "board" command with 11 subcommands." becomes accurate to the new count (11 existing top-level + `notes` group + `promote-note` = 13 top-level entries — state it as "13 subcommands (including the notes group and promote-note)" rather than hand-counting leaves); "Configuration resolution happens once in a PersistentPreRunE: the config file (home, sidebar, proposal_prefix) is loaded..." becomes "...the config file (readme, design_prefix) is loaded...". In the root `cmd`'s own `Long` field: change "The config file (_lyx/config/board.yaml) controls non-geometry settings: home,\nsidebar, and proposal_prefix filenames." to "The config file (_lyx/config/board.yaml) controls non-geometry settings: readme\nand design_prefix filenames."; add one sentence noting the two-store model, e.g. "Task verbs operate on tasks.json (claimable); the notes subcommand group mirrors the same verb set over notes.json (not yet claimable); promote-note moves an entry from one to the other." — placed after the existing `--board-path`/detached-sync-child sentence. In `rerenderCmd`: change `Short` from `"Rebuild Home.md and _Sidebar.md from tasks.json"` to `"Rebuild the combined README from tasks.json and notes.json"`.
- **Commit:** `docs(boardcli): fix stale help text for config renames and the two-store model`

### Card 23: mechanical test-literal updates for the config rename

- **Context:**
  - `internal/boardengine/config.go`
- **Edits:**
  - `internal/boardcli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `seedCwd` (used by every integration-tagged test in this file): change the written config content from `"home: Home.md\nsidebar: _Sidebar.md\nproposal_prefix: proposal-\n"` to `"readme: Home.md\ndesign_prefix: proposal-\n"` (keeping the same string VALUES — `"Home.md"`/`"proposal-"` — so `TestCLIRerender`'s existing `homePath := filepath.Join(hubgeometry.BoardDir(filepath.Dir(cwd)), "Home.md")` assertion needs no change), and update `seedCwd`'s doc comment (currently "seeds with all template keys (home, sidebar, proposal_prefix; path: is not a template key)") to name the new keys. Apply the identical content-string change to `TestCLIBoardPathResolution`'s own duplicated `configContent` literal (this test seeds its own config directly rather than calling `seedCwd`, per its existing two-level Hub/worktree fixture setup).
- **Commit:** `test(boardcli): update seeded board.yaml content for readme/design_prefix rename`

### Card 24: new tests — notes CRUD, promote-note, help schema

- **Context:**
  - `internal/boardcli/cli_unit_test.go`
  - `internal/boardcli/cli_test.go`
- **Edits:**
  - `internal/boardcli/help_test.go`
- **Creates:**
  - `internal/boardcli/notes_test.go`
  - `internal/boardcli/promotenote_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** New file `notes_test.go`, `//go:build integration`, `package boardcli_test` (mirrors `cli_test.go`'s package/build-tag/`seedCwd`/`runCLI` reuse — `seedCwd`/`runCLI` are defined in `cli_test.go`/`cli_unit_test.go`, same package, directly callable). Add a `TestCLINotesContract` table-driven test mirroring `TestCLIContract`'s shape, covering the happy path for `notes upsert` (`args = []string{"notes", "upsert", `{"slug":"foo-note","title":"Foo Note"}`}`, asserts exit 0, `task` field present, and — distinctively — that `notes.json` (not `tasks.json`) was written in the board dir), `notes list` (asserts a `tasks` field... actually the JSON envelope key stays `"tasks"` even for notes output, matching `ListNotesBrief`'s reuse of the `BriefTask` shape — confirm this against `outputListBrief`'s existing envelope key before asserting), `notes get`, `notes set-status`, `notes remove` (asserts the note is gone from `notes.json` and `tasks.json` is untouched). New file `promotenote_test.go`, same build tag/package: a test that seeds a note via `notes upsert`, calls `promote-note '{"slug":"..."}'`, asserts exit 0 and the returned `task.slug` matches, then asserts `notes get` for that slug now returns `task:null` while `get` (the task verb) returns the promoted task with matching fields. Also add a `promote-note` error-path case: calling it with a slug that was never a note errors with a message containing `"note not found"`. In `help_test.go`: extend `runHelp`'s signature from `runHelp(t *testing.T, verb string) string` to `runHelp(t *testing.T, args ...string) string` (backward-compatible — every existing single-verb call site like `runHelp(t, "upsert")` still resolves as a one-element `args` slice), changing its body to `boardcli.RunCLI(&buf, append(args, "--help"))`. Add new `TestHelpSchema_LeafCommands` table entries: `{name: "notes upsert", args: []string{"notes", "upsert"}, mustContain: [...same field list as "upsert"...]}` and equivalently for `notes set-status`/`notes remove`/`notes get`/`notes merge`/`notes set-deps` (reusing each entry's existing task-verb `mustContain` list verbatim — the payload shapes are identical), plus a `promote-note` entry asserting its help mentions `slug`/`id`.
- **Commit:** `test(boardcli): cover notes CRUD, promote-note, and their --help schema`

## Batch Tests

`go test ./internal/boardcli/...` (both untagged `cli_unit_test.go` and integration-tagged `cli_test.go`/`help_test.go`/`notes_test.go`/`promotenote_test.go`). Card 24's new `notes_test.go`/`promotenote_test.go` are the primary coverage for Cards 20-21's new CLI surface; `help_test.go`'s extended table locks in the documented payload schema for every new leaf command, matching the existing pattern's own stated purpose ("pins the documented payload schema visible via --help").
