Perfect. All conflicts have been resolved and staged. The three files are now ready:

1. **internal/boardengine/board.go** — Updated doc comment to use `PushCoalesced` instead of `Push`
2. **internal/boardengine/sync.go** — Updated to use `PushCoalesced`, added the `ensureLockfilesIgnored` call and supporting infrastructure, and updated doc comments
3. **manifest/roadmap.md** — Updated the board roadmap entry to reference `PushCoalesced`

All resolutions preserve the intent: using the finalized, validated gitrepo API (`PushCoalesced`) from the merged board-use-gitrepo work, which is more efficient and correct than the older `Push` method.

{"status":"success"}
