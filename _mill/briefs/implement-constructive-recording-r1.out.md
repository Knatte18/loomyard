{"status":"success","commit_sha":"223f79bd84dea86e62ddf75c932f0c717eedeff2","session_id":"192d532e-5ea9-4463-967e-70f5c7b274c4","cards_done":[17,18,19,20,21,22]}

All 6 cards of batch `constructive-recording` are committed (6 of 6 — verified via `git log <start>..HEAD` against the batch's `## Cards` headings), and the batch's `verify:` command passes:

- `go test ./internal/fabricengine/ ./internal/fabriccli/` → ok
- `go test -tags integration -run TestMutationRecord ./internal/fabricengine/` → ok

Commits (newest first), all pushed to `fabric-mutation-record-envelope`:
- `223f79bd` test(fabricengine): assert the record survives Remove's refusal and Add's rollback (card 22)
- `e777116d` feat(fabricengine): record the launcher and clone construction sites (card 21)
- `f17d8d86` feat(fabricengine): record worktree_switched and repo_advanced (card 20)
- `cc53be59` feat(fabricengine): record commit_created and branch_pushed on the commit and push paths (card 19)
- `9466942d` feat(fabricengine): record worktree, branch and push mutations on the add path (card 18)
- `5397e322` feat(fabricengine): record link_created and file_written at their success sites (card 17)

Key files touched: `internal/fabricengine/{portals,junction,gitexclude,gitexclude_integration_test,weftgit,add,reconcile,unwire,checkout,pull,commit,spawn,coalesce,launchers,clone,weftwiring}.go`, plus the new `internal/fabricengine/mutation_record_integration_test.go`.

**Card 21's required derivation (completion note)** — grep of `internal/fabricengine` and `internal/fabriccli` for `os.WriteFile(`, `os.MkdirAll(`, `os.Mkdir(`, `os.Rename(`, `fslink.CreateDirLink(`, `cloneRepo(`, classifying every production (non-`_test.go`) hit:

- **Bucket 1 — recorded here or by an earlier card:** `junction.go` (all `fslink.CreateDirLink` sites, the `os.Rename` adoption loop — card 17), `portals.go` (`fslink.CreateDirLink` — card 17), `launchers.go` (`MkdirAll(launcherDir)`, both script `WriteFile`s, menu `WriteFile` — card 21), `clone.go` (`.lyx` `MkdirAll`, both `cloneRepo` calls, `.lyx-anchor` `WriteFile` — card 21), `warpbinding.go`'s `WriteFile` (recorded at its two callers, `clone.go` and `reconcile.go` — card 21), `destroy.go`'s `os.Mkdir` (already recorded by the gate, batch 4).
- **Bucket 2 — covered by an enclosing recorded root:** `junction.go:149` (`MkdirAll(target)` materialising a junction's weft-side target, under an already-recorded weft worktree), `launchers.go:145` (`MkdirAll(filepath.Dir(menuPath))`, the menu launcher's implicit ancestor — explicitly conceded by the Shared Decision), `weftgit.go:55` (`MkdirAll(.weft)`, explicitly named bucket 2 in the Shared Decision's own text).
- **Bucket 3 — invisible to the manifest (`.git` metadata):** `hook.go` (all `WriteFile`/`MkdirAll` under `.git/hooks/`), `gitexclude.go` (`MkdirAll`/`Rename` are the write mechanism *inside* `mutateGitExclude`; the resulting file is recorded at the `seedGitExclude`/`unseedGitExclude` caller level per card 17, not at this raw-primitive level).
- **Out of scope for this derivation:** `internal/fabricengine/fabrictest/*.go` (`states.go`, `hub.go`, `verbs.go`) — these are test-fixture-construction helpers, not fabric verb implementation; they simulate external/operator state for integration tests and have no recorder to thread through. No hits found in `internal/fabriccli` production code at all.

No pre-existing tests were weakened, skipped, or removed; the untagged suite and the new tagged file both pass.
