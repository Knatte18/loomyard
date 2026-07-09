# Batch: linux-suffix-rename

```yaml
task: Rename _other.go platform files to _linux.go
batch: linux-suffix-rename
number: 1
cards: 4
verify: GOOS=windows go build ./... && GOOS=linux go build ./... && GOOS=linux go vet ./...
depends-on: []
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (here: the
   leading `//go:build !windows` build-tag line + its trailing blank line, and the enumerated
   comment rewords). Do not reformat, reorder, or otherwise touch unrelated lines.
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor — there
   are none in this batch.
4. Never write the relocated file from scratch and delete the original — that breaks git rename
   history and inflates review diffs.

## Batch Scope

This batch renames the four `_other.go` platform-seam files (plus proc's test) to `_linux.go`,
drops their `//go:build !windows` tags so the filename suffix supplies the `GOOS=linux`
constraint, rewords "non-Windows"/"POSIX" comments to "Linux", and fixes the one stale
`proc_other.go` reference in `docs/roadmap.md`. It is a single batch because every change is the
same mechanical, behaviour-preserving rename pattern across four sibling packages, sharing one
set of Shared Decisions; total context is a handful of tiny files. There is no external interface
for a later batch to consume — this is the whole task. Batch-local decision: the `docs/roadmap.md`
reference fix is co-located with the proc rename (Card 2) because that roadmap line names
`proc_other.go` specifically.

## Cards

### Card 1: Rename fslink_other.go → fslink_linux.go

- **Context:**
  - `internal/fslink/fslink_windows.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/fslink/fslink_other.go` -> `internal/fslink/fslink_linux.go`
- **Requirements:** After `git mv`, delete the leading `//go:build !windows` line (line 1) **and
  its immediately-following blank line** from `internal/fslink/fslink_linux.go`, so the file now
  begins with `package fslink`. Make no other change — this file contains no "non-Windows"/"POSIX"
  prose to reword. The `//go:build windows` tag on the `fslink_windows.go` sibling is left
  untouched.
- **Commit:** `refactor(fslink): rename fslink_other.go to fslink_linux.go, drop build tag`

### Card 2: Rename proc_other.go + test → _linux.go, reword prose, fix roadmap ref

- **Context:**
  - `internal/proc/proc_windows.go`
- **Edits:**
  - `docs/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/proc/proc_other.go` -> `internal/proc/proc_linux.go`
  - `internal/proc/proc_other_test.go` -> `internal/proc/proc_linux_test.go`
- **Requirements:**
  - After `git mv` of both proc files, delete the leading `//go:build !windows` line + its
    trailing blank line from **both** `internal/proc/proc_linux.go` and
    `internal/proc/proc_linux_test.go`.
  - In `internal/proc/proc_linux.go`, apply exactly these four comment rewrites (preserve the
    em-dash `—` verbatim):
    - `// proc_other.go — non-Windows process control primitives.` → `// proc_linux.go — Linux process control primitives.`
    - `// On non-Windows platforms, HideWindow is a no-op (there are no console windows).` → `// On Linux, HideWindow is a no-op (there are no console windows).`
    - `// HideWindow is a no-op on non-Windows platforms (no console windows to suppress).` → `// HideWindow is a no-op on Linux (no console windows to suppress).`
    - `// On non-Windows, Setsid is the equivalent of Windows CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW:` → `// On Linux, Setsid is the equivalent of Windows CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW:`
  - `internal/proc/proc_linux_test.go` needs only the tag drop — no prose rewrites.
  - In `docs/roadmap.md` (around line 113, the text `build-tagged \`proc_windows.go\` / \`proc_other.go\``), change the token `proc_other.go` to `proc_linux.go`. Leave `proc_windows.go` and all surrounding prose unchanged.
- **Commit:** `refactor(proc): rename proc_other.go+test to _linux.go, reword prose, fix roadmap ref`

### Card 3: Rename launch_other.go → launch_linux.go, reword prose

- **Context:**
  - `internal/vscode/launch_windows.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/vscode/launch_other.go` -> `internal/vscode/launch_linux.go`
- **Requirements:** After `git mv`, delete the leading `//go:build !windows` line + its trailing
  blank line from `internal/vscode/launch_linux.go`. Then apply exactly these two comment rewrites:
    - `// Launch returns an error on non-Windows platforms (POSIX).` → `// Launch returns an error on Linux.`
    - `// VS Code launch is a Windows-only feature; POSIX systems are not supported.` → `// VS Code launch is a Windows-only feature; Linux is not supported.`
  The `Launch` function body (`return ErrUnsupported`) is unchanged — behaviour-preserving.
- **Commit:** `refactor(vscode): rename launch_other.go to launch_linux.go, reword prose`

### Card 4: Rename spawnattach_other.go → spawnattach_linux.go, reword prose

- **Context:**
  - `internal/muxpoccli/spawnattach_windows.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/muxpoccli/spawnattach_other.go` -> `internal/muxpoccli/spawnattach_linux.go`
- **Requirements:** After `git mv`, delete the leading `//go:build !windows` line + its trailing
  blank line from `internal/muxpoccli/spawnattach_linux.go`. Then apply exactly these two comment
  rewrites (preserve the em-dash `—` verbatim):
    - `// spawnattach_other.go — psmux attach for non-Windows.` → `// spawnattach_linux.go — psmux attach for Linux.`
    - `// Blocks until the user detaches (normal for non-Windows interactive use).` → `// Blocks until the user detaches (normal for Linux interactive use).`
  The `spawnAttach` function body is unchanged — behaviour-preserving.
- **Commit:** `refactor(muxpoccli): rename spawnattach_other.go to spawnattach_linux.go, reword prose`

## Batch Tests

`verify:` runs `GOOS=windows go build ./...`, `GOOS=linux go build ./...`, and
`GOOS=linux go vet ./...` — all three currently pass at baseline and must stay green. Together they
cover the batch's whole risk surface: the windows build exercises the untouched `_windows.go`
siblings, the linux build compiles the renamed `_linux.go` files (proving the tag drop left a valid
`GOOS=linux` constraint via the suffix), and `GOOS=linux go vet` typechecks the renamed
`proc_linux_test.go` (test files are excluded from `go build` but compiled by `vet`). The command is
git-root-relative (non-nested layout), so the plain-string form is correct; no `PYTHONPATH=` prefix
(Go project, not Python).

Manual / one-time confirmations (NOT part of the automated `verify:` gate, which must exit 0):

- `GOOS=darwin go build ./...` must now **fail** with "build constraints exclude all Go files" for
  the four renamed packages — this failure is the deliverable (honest unsupported-platform build),
  not a regression.
- Existing tests pass on the Windows dev host (`go test ./...`); the renamed `proc_linux_test.go`
  only builds on Linux and is simply excluded on Windows.
- Post-implementation grep confirms zero remaining `_other` references in `.go` files and under
  `docs/`, and zero remaining "non-Windows"/"POSIX" strings in the five renamed files.
