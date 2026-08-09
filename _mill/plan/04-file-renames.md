# Batch: file-renames

```yaml
task: "Rename the fabric host vocabulary to warp, and name the composite repo Fabric"
batch: "file-renames"
number: 4
cards: 2
verify: go test ./internal/fabricengine/... && go test -tags integration ./internal/fabricengine/...
depends-on: [3]
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the lines that must change after the move (package or module declaration, imports, identifier retargeting, seam splits).
3. Use a full-file `Creates:` entry only for genuinely new files that have no predecessor.
4. Never write the relocated file from scratch and delete the original — that breaks git rename history and inflates review diffs.

## Batch Scope

Four filenames still carry the retired vocabulary after batch 3 has renamed everything *inside* them.
This batch renames the files themselves and repoints the five inbound markdown links to the one doc among them.

It is a separate batch, and a separate commit group, from the token sweep on purpose: merging the renames into batch 3 would bury four `git mv` operations inside a ~300-file mechanical diff and obscure both.
Kept apart, `git log --follow` still works on all four files and the rename diff is readable on its own.

Every file body edited here was already corrected by batch 3 card 9 — the file comments at `warpclean.go:1`, `warplayout.go:1` and `warpjunction_test.go:1` already name the *post*-rename filename, because that hand edit was what let batch 3's `wordswap` run reach exit zero.
So the three Go moves in card 11 are pure `git mv` with no follow-up edit at all.
Verify that rather than assuming it.

## Cards

### Card 11: rename the three fabric-geometry Go files

- **Context:**
  - `internal/fabricengine/drift.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `internal/fabricengine/hostclean.go` -> `internal/fabricengine/warpclean.go`
  - `internal/fabricengine/hostlayout.go` -> `internal/fabricengine/warplayout.go`
  - `internal/fabricengine/hostjunction_test.go` -> `internal/fabricengine/warpjunction_test.go`
- **Requirements:**
  Run the three `git mv` operations per the Rename mechanic above.
  Go does not bind filenames to package structure, so no import, no build tag and no package declaration changes as a result of these moves — the renamed test file keeps whatever `//go:build` constraint `hostjunction_test.go` carried, byte for byte.

  After the moves, confirm no file body still names an old filename: grep the repo for `hostclean.go`, `hostlayout.go` and `hostjunction_test.go` and expect zero hits outside `_mill/` and the four historical-record docs listed in the overview's exclusion decision.
  Batch 3 card 9 already rewrote the three self-referential file comments and `internal/fabricengine/drift.go` line 3's `(warpclean.go)` cross-reference, so this grep is a confirmation step;
  if it finds a live hit, fix that one line here rather than deferring it.
- **Commit:** `refactor(fabric): rename hostclean/hostlayout/hostjunction files to warp*`

### Card 12: rename the design doc and repoint its inbound links

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `manifest/roadmap.md`
  - `manifest/designs/fabric-unified-view.md`
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `manifest/designs/host-visibility.md` -> `manifest/designs/warp-visibility.md`
- **Requirements:**
  Run `git mv manifest/designs/host-visibility.md manifest/designs/warp-visibility.md` per the Rename mechanic above.

  Then repoint the five inbound references, changing **only** the link target and the doc's own name token on each line, and leaving the surrounding prose for batch 6 to reword:

  - `manifest/roadmap.md` line 44 — `Someday's \`host-visibility\`` becomes `Someday's \`warp-visibility\``.
  - `manifest/roadmap.md` line 81 — the bolded item title `**host-visibility: CLAUDE.local.md invisible in host's git history**` becomes `**warp-visibility: CLAUDE.local.md invisible in host's git history**`;
    leave the trailing `host's` occurrences on that line alone, they are prose that batch 6 rewords.
  - `manifest/roadmap.md` line 84 — `[designs/host-visibility.md](designs/host-visibility.md)` becomes `[designs/warp-visibility.md](designs/warp-visibility.md)`.
  - `manifest/roadmap.md` line 240 — the bare name `\`host-visibility\`` in the parenthesised list becomes `\`warp-visibility\``.
  - `manifest/designs/fabric-unified-view.md` line 203 — `[host-visibility.md](host-visibility.md)` becomes `[warp-visibility.md](warp-visibility.md)`.

  Do not reword any other `host` on those lines in this card — the split is deliberate, so the rename commit stays a rename and batch 6's judgment work stays reviewable on its own.
  The moved file's own line 1 heading (`# host-visibility — CLAUDE.local.md invisible in host's git history`) and its remaining prose are reworded in batch 6 card 19, not here.
- **Commit:** `docs(manifest): rename host-visibility.md to warp-visibility.md and repoint links`

## Batch Tests

`verify: go test ./internal/fabricengine/...` plus its integration-tagged twin covers the only package whose files move.
`internal/fabricengine/warpjunction_test.go` is integration-tagged, which is why both tiers run — the untagged pass alone would never compile the renamed test file, so a botched `git mv` that dropped its build constraint would go unnoticed.

The scope is narrow on purpose: a `git mv` inside one Go package cannot affect another package, since Go binds packages to directories and not to filenames.
The overview's module-wide `verify: go build ./...` still runs at the batch boundary and would catch a move that accidentally landed a file outside its package directory.

Card 12 touches only markdown and has no runnable surface;
its correctness gate is the grep in card 11 plus batch 6's own documentation pass, which re-reads `manifest/designs/warp-visibility.md` under its new name.
