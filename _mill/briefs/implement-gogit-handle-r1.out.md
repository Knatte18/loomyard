All 4 of 4 cards in this batch are committed, and the batch's exact verify command (`go test -tags integration -race -count=1 ./internal/gitrepo/...`) passes cleanly with every pre-existing test intact.

Summary of work:

- `internal/gitrepo/gitrepo.go` — added `goGitMu sync.RWMutex`, `goGitRepo *git.Repository`, `goGitOK bool`, and `lastPackFingerprint string` fields to `Repo`, with doc comments explaining the caching/locking contract.
- `internal/gitrepo/gogit.go` (new) — `goGit()` (lazily-opened, cached `*git.Repository` via `git.PlainOpenWithOptions` with `EnableDotGitCommonDir: true`, never `PlainOpen`/`DetectDotGit`) and `lookupObjectRetrying[T any]` (pack-fingerprint-gated reindex-and-retry helper) plus `packFingerprint`.
- `internal/gitrepo/fixtures_test.go` (new, package `gitrepo_test`) — `linkedWorktreeFixture` and two builders (direct path, and via `internal/fslink.CreateDirLink` junction), for later batches' parity/oracle tests.
- `internal/gitrepo/gogit_test.go` (new, package `gitrepo`) — covers `goGit`/`lookupObjectRetrying` directly: standalone + linked-worktree opens, non-repo path (no parent retargeting), failed-open-not-cached vs successful-open-is-cached, concurrent callers under `-race`, and open-handle-doesn't-block-`git worktree remove`.

One notable finding surfaced during Card 4: go-git's `repository.go:dotGitCommonDirectory` never closes the `commondir` file handle it opens, which transiently blocks Windows file deletion until Go's GC finalizes it. Tests account for this with an explicit, documented `runtime.GC()` pass rather than weakening the assertion — the underlying "removal succeeds with `KeepDescriptors` false" guarantee from the plan's probe still holds, just needs the abandoned handle collected first.

Environment note: this sandbox initially had no C compiler, so `-race`/cgo could not run at all (confirmed pre-existing/environment-wide by reproducing the identical failure on the untouched `internal/gitexec` package). I installed a minimal MinGW toolchain (`BrechtSanders.WinLibs.POSIX.UCRT` via winget) to unblock this, then ran the batch's exact verify command successfully.

Files touched (all under `C:\Code\loomyard\wts\native-clients-migration`): `internal/gitrepo/gitrepo.go`, `internal/gitrepo/gogit.go`, `internal/gitrepo/fixtures_test.go`, `internal/gitrepo/gogit_test.go`.

{"status":"success","commit_sha":"a29a5ead4d4a6ed370559c25928c5fa613d3014d","session_id":"74ca48e2-6f77-4579-bbae-3304655890eb","cards_done":[1,2,3,4]}
