{"status":"success","commit_sha":"3d6d552efbd98a97cb0f6db03267240cdfc25eb5","session_id":"c7e928a5-cc21-4bb2-a390-a1442b6baeee","cards_done":[20,21,22,23]}

## Summary

All 4 cards in batch `retire-poc-and-measure` are complete (4 of 4 — genuinely all done, verified against the commit log since the batch-start commit `c0cb814c`).

**Card 20 (Falsification pass, verification-only, no commit):** Temporarily reverted each migrated read in `/home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients/internal/gitrepo/gitrepo.go`, `push.go`, and `gogit.go` to a plausible-but-wrong shape, confirmed the parity suite failed, then restored the correct implementation (verified zero residual diff via `git diff`):
- `ChangedFilesSince` via `Tree.Diff` instead of `object.DiffTree` → failed `TestChangedFilesSince_Parity_Rename`.
- `hasUnpushed` seeding `seenExternal` with only the upstream tip → failed `TestHasUnpushed_Parity/Behind` and the linked-worktree `HasUnpushed_StrictlyBehind` case.
- `SHAExists`'s `commitByHash` under a bare object lookup with no commit-type peel → failed `TestSHAExists_Parity_TreeOrBlobSHA`.
- The fingerprint gate replaced with a per-`*Repo` call counter → failed `TestSHAExists_MixedBackend_CrossInstanceReindexSeesWriteFromOtherRepo`.

**Card 21 (commit `4b877736`):** Measured `hasUnpushed` against the CLI spawn on a real local clone of this checkout's own 268-commit history (20 trials/case). Equal-hash shortcut: go-git ~6x faster (239µs vs 1.42ms). Walking path (the one `PushCoalesced` actually hits): go-git ~9.4x **slower** (20.4ms vs 2.16ms) — triggering the reversal criterion. Reverted `hasUnpushed` in `/home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients/internal/gitrepo/push.go` to its CLI form verbatim, documented the measurement in its godoc, and deleted the now-meaningless go-git parity cases from `/home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients/internal/gitrepo/gogit_test.go` and `/home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients/internal/gitrepo/parity_test.go`. Discovered the plan misattributed one deleted test's file location, so I stopped, extended `/home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients/_mill/plan/05-retire-poc-and-measure.md`'s card-21 `Edits:` to add `parity_test.go`, and committed that plan edit first (`a6d15b92`) before touching the file.

**Card 22 (commit `3d6d552e`):** Deleted `internal/gitnativepoc/` (all 8 files) after confirming no non-test file imports it, every migrating read was lifted into `gitrepo`, `write.go` contributes nothing (all its methods stay CLI-bound), and every harness case worth keeping lives in `gitrepo`'s parity files.

**Card 23 (verification-only, no commit):** Confirmed via `git log main..HEAD -- <consumer files>` that no consumer of `gitrepo` (`boardengine/sync.go`, `fabricengine/{fabric,weftgit}.go`, `websterengine/{gitwrap,integration,runlevel}.go`, `fabriccli/spawn.go`) needed an edit across batches 1–5. Read `websterengine/integration.go` closely — its `CurrentBranch → CheckoutDetached → RestoreBranch` bisect flow matches `gitrepo`'s unchanged signatures exactly. Ran `go test -tags integration ./... -count=1`: `boardengine`, `fabricengine`, `websterengine`, and `fabriccli` all pass. Other failures (`cmd/lyx` tierpurity guard, `buildercli`/`webstercli` weft-gitignore tests, `builderengine` template text, `reedengine` Windows-path test) are confirmed pre-existing and out of scope — zero same-task commits touch any of those files, and the tierpurity failure is explicitly deferred to batch 8 ("guards") per the plan's own DAG.

Batch verify (`go test -tags integration -race -count=1 ./internal/gitrepo/...`) passes. Working tree is clean of tracked in-scope modifications.
