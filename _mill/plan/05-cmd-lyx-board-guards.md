# Batch: cmd/lyx: board git-import guard + drift/help-tree/registration coverage

```yaml
task: 'board: move storage to weft:main'
batch: 'cmd/lyx: board git-import guard + drift/help-tree/registration coverage'
number: 5
cards: 3
verify: go test ./cmd/lyx/... ./internal/fabriccli/... && go test -tags integration ./cmd/lyx/... ./internal/fabriccli/...
depends-on: [2, 4]
```

## Rename mechanic

Not applicable — this batch contains no `Moves:` entries.

## Batch Scope

This batch closes the machine-checked gap `_mill/discussion.md`'s "Machine-checked guard for board's git-import boundary" decision calls for, and updates the one `cmd/lyx` pinned test list that needs a hand-edit for this task's CLI surface changes. Research done during planning (a background fork, confirmed against the live source) established that `cmd/lyx/registration_test.go`, `longlist_test.go`, `drift_test.go`, and `hermeticenv_test.go` are all either fully self-discovering (live cobra-tree/import walks with no pinned literal to edit) or already satisfied by batch 3's existing `internal/fabricengine`/`internal/boardengine` `TestMain` setup — none of them need edits for this task. `cmd/lyx/sandbox_coverage_test.go` is module-granularity and `board`/`fabric` are already tagged `**Covers:**` in `tools/sandbox/SANDBOX-CORE-SUITE.md`/`SANDBOX-FABRIC-SUITE.md` — only those suite files' prose needs updating, which is batch 6's job, not this batch's Go code. This batch depends on batch 2 (fabric clone's 2-arg signature, for Card 27's new test) and batch 4 (the `notes`/`promote-note` CLI surface, for Card 26's pinned-list update).

## Cards

### Card 25: new guard — `internal/boardengine` never imports raw git or shells out to it

- **Context:**
  - `cmd/lyx/ghguard_test.go`
  - `cmd/lyx/gitrepoboundary_test.go`
  - `internal/pattern/leaf_enforcement_test.go`
  - `internal/boardengine/spawn.go`
- **Edits:** none
- **Creates:**
  - `cmd/lyx/boardguard_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** New file `boardguard_test.go`, `package main` (same package as every other `cmd/lyx` guard test). Add `TestBoardGuard_NoRawGitImportOrShellOut`, combining two checks scoped to `internal/boardengine`'s non-test `.go` files only (walk `filepath.Join(filepath.Dir(goMod), "internal", "boardengine")`, resolving `goMod` via `go env GOMOD` exactly as `ghguard_test.go`/`gitrepoboundary_test.go` do — skip cleanly via `t.Skip` when the go toolchain is absent or no module is found, mirroring both). Skip the `boardtest` subdirectory entirely (`filepath.SkipDir` when `d.Name() == "boardtest"`) — it is a sibling package of integration tests that legitimately spawn git via `lyxtest.CopyWeft`, not production code this guard's import/shell-out ban applies to. Check 1 (import ban, mirroring `internal/pattern/leaf_enforcement_test.go`'s `go/parser` `ImportsOnly` AST walk): parse each file's imports and fail if either `"github.com/Knatte18/loomyard/internal/gitrepo"` or `"github.com/Knatte18/loomyard/internal/gitexec"` appears. Check 2 (shell-out ban, mirroring `ghguard_test.go`'s `lineHasBannedGHSpawn` pattern exactly, generalized from `"gh"` to `"git"`): scan each file line by line and fail if a line contains BOTH `` `"git"` `` and one of `exec.Command`/`exec.CommandContext` — this same-line co-occurrence requirement (not a bare substring ban) is what lets `internal/boardengine/spawn.go`'s legitimate, git-free `exec.Command(exe, "board", "--board-path", abs, "sync")` self-relaunch call pass cleanly, since that line never mentions `"git"`. Vacuous-scan floor: `scanned < 5` (mirrors `gitrepoBoundaryMinScannedFiles`'s convention for a similarly-sized package — `internal/boardengine` has 8 non-test `.go` files as of this task's completion: `board.go`, `config.go`, `layer.go`, `render.go`, `spawn.go`, `store.go`, `sync.go`, `task.go`). On failure, name the CONSTRAINTS.md Weft Git Invariant in the error message, matching every other guard's style (`ghguard_test.go`'s "see CONSTRAINTS.md's GitHub Auth Invariant" precedent).
- **Commit:** `test(cmd/lyx): add board-git-import-boundary guard (Weft Git Invariant board carve-out)`

### Card 26: `helptree_test.go` — pin `notes`/`promote-note` into board's subcommand list

- **Context:**
  - `internal/boardcli/cli.go`
- **Edits:**
  - `cmd/lyx/helptree_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `TestHelpTree_VerbModuleSubcommands`'s `tests` table, the `board` entry's `wantSubs` slice (currently `{"upsert", "upsert-batch", "set-status", "remove", "get", "list", "list-full", "merge", "set-deps", "rerender", "sync"}`) gains two entries: `"notes"` (the new subcommand group's own name — this table checks direct children of `board`, not grandchildren, so `notes`'s own 9 children are not separately listed here) and `"promote-note"`.
- **Commit:** `test(cmd/lyx): pin notes and promote-note into board's help-tree subcommand list`

### Card 27: `fabric clone` argument-count coverage

- **Context:**
  - `internal/fabriccli/clone.go`
- **Edits:**
  - `internal/fabriccli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `TestRunCLI_CloneRequiresExactlyTwoArgs`, `//go:build integration` (same file, same build tag as every existing test in `cli_test.go`), no fixture needed (`runCloneWithReset`'s `len(args) != 2` check runs before any git spawn — a `t.TempDir()` + `t.Chdir` is sufficient, mirroring `TestRunCLI_UnknownSubcommand`'s minimal setup, not `setupCLIRepo`'s full `lyxtest.CopyHostHub` fixture). Assert `fabriccli.RunCLI(&out, []string{"clone", "https://example.com/host"})` (1 arg) exits 1 with a JSON error envelope whose `error` field contains `"usage: lyx fabric clone"` (the updated 2-arg usage message from batch 2's Card 7); assert `fabriccli.RunCLI(&out, []string{"clone", "https://example.com/host", "https://example.com/weft", "https://example.com/board"})` (3 args, the old board-url form) also exits 1 with the same usage-message substring — locking in that the 3rd positional argument this task removes is now rejected, not silently accepted or ignored.
- **Commit:** `test(fabriccli): cover fabric clone's 2-arg requirement`

## Batch Tests

`go test ./cmd/lyx/... ./internal/fabriccli/...` plus a second `-tags integration` pass over the same two package paths (Card 27's new test lives in `internal/fabriccli/cli_test.go`, which is `//go:build integration`-tagged and invisible to the plain, untagged run). Card 25 is the load-bearing addition: it is the only mechanical enforcement that `internal/boardengine`'s git-routing swap (batch 3, Card 13) does not regress back to a raw `gitrepo`/`gitexec` call in some future change. Card 26 keeps `helptree_test.go` in sync with the CLI surface batch 4 added. Card 27 locks in the argument-count contract batch 2 changed.
