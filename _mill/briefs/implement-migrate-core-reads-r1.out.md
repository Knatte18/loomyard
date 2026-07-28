All 4 cards in this batch (10–13) are implemented, committed, and pushed as a single combined commit (all four necessarily touch the same file, `internal/gitrepo/gitrepo.go`, matching the plan's stated allowance for a combined commit when cards share a file).

Summary of changes to `internal/gitrepo/gitrepo.go`:
- **Card 10 (`CurrentSHA`)**: now reads `Head()` through the cached go-git handle under `RLock`; unborn-HEAD detection is typed (`plumbing.ErrReferenceNotFound`) rather than stderr-matched.
- **Card 11 (`SHAExists`)**: peels to a commit via a new `commitByHash` helper routed through `lookupObjectRetrying` (batch-1's fingerprint-gated helper). `commitByHash` decodes full-length hashes directly (so a miss surfaces the storer's raw `plumbing.ErrObjectNotFound`, which the fingerprint gate needs) and only falls back to `ResolveRevision`'s prefix expansion for abbreviated hashes (verified empirically against the pinned go-git v5.19.1 that `ResolveRevision` alone always translates a miss into `ErrReferenceNotFound`, which would make the fingerprint gate unreachable — confirmed with a standalone probe program before writing this).
- **Card 12 (`CurrentBranch`)**: written from scratch on an unresolved `Reference(plumbing.HEAD, false)` read (never `Head()`, which resolves and would wrongly succeed on a detached HEAD), holding `RLock` explicitly since it touches no object.
- **Card 13 (`ChangedFilesSince`)**: diffs via `object.DiffTree` directly (never `Tree.Diff`/`DiffTreeWithOptions` with `DefaultDiffTreeOptions`, both of which enable rename detection since go-git v5.1.0 — verified against source), using a new `treeForRev` helper built on `commitByHash`.

Verification: `go build ./...`, `gofmt -l`, `go vet ./...` all clean; `go test -tags integration -race -count=1 ./internal/gitrepo/...` passes, including the previously-tautological `TestSHAExists_Parity_TreeOrBlobSHA` and `TestChangedFilesSince_Parity_Rename` cases that are now live differential assertions against the CLI oracle.

Card-count self-check: batch declares 4 cards (10, 11, 12, 13); commit `90dfd1a4` (range start `34f0e7f8`) addresses all 4 — 4 of 4 cards committed, all complete.

Files touched: `/home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients/internal/gitrepo/gitrepo.go`

{"status":"success","commit_sha":"90dfd1a4","session_id":"fc60b44b-a6d3-4a52-9019-68eab18a4663","cards_done":[10,11,12,13]}
