{"status":"success","commit_sha":"99d79ebb4a8a031b32cdff70aa8a0d834d798668","session_id":"1e6268ca-0f3b-41f7-89a8-532b40027dcb","cards_done":[1,2,3,4]}

All 4 cards in batch `01-gitrepo-primitives.md` are committed and pushed (4 of 4 — all complete):

1. **Card 1** (`00e7a674`, fixed in `99d79ebb`): Added `(*Repo).Fetch() error` to `internal/gitrepo/pull.go`, new `internal/gitrepo/fetch_integration_test.go`. During verify, discovered the plan's assumed no-remote error behavior for bare `git fetch` doesn't match real git 2.53 semantics (fetch with zero remotes is a silent no-op, unlike `pull`), so I corrected the integration test to assert the true no-op success and added a genuine error-path test against an unreachable remote URL instead.
2. **Card 2** (`0947ec1c`): Added `(*Repo).IsAncestor(sha, ref string) (bool, error)` in new `internal/gitrepo/ancestry.go`, split tests into `ancestry_test.go` (Tier-1, untagged) and `ancestry_integration_test.go` (tagged integration).
3. **Card 3** (`16d95a81`): Promoted `hasUnpushed` → exported `HasUnpushed` in `internal/gitrepo/push.go`; fixed `internal/gitrepo/gogit.go`'s locking-discipline comment (removed the always-wrong `hasUnpushed` entry from the go-git-lookup list rather than case-renaming it); updated stale comment mentions in `gogit_test.go`, `push_test.go`, `internal/boardengine/sync.go`.
4. **Card 4** (`9eab5492`): Updated `cmd/lyx/gitrepoboundary_test.go`'s pinned method set, `CONSTRAINTS.md`'s gitrepo Client Boundary Invariant, and `internal/gitrepo/doc.go`.

All three verify scopes pass: `go test ./internal/gitrepo/`, `go test -tags integration ./internal/gitrepo/ -run 'TestFetch|TestIsAncestor|TestPush'`, and `go test -run TestGitrepoBoundary_PinnedRunCallSites ./cmd/lyx/`. Working tree is clean (no uncommitted tracked changes).

Key files touched:
- `/home/knatte/Code/loomyard/wts/fabric-rebase-reconcile/internal/gitrepo/pull.go`
- `/home/knatte/Code/loomyard/wts/fabric-rebase-reconcile/internal/gitrepo/fetch_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-rebase-reconcile/internal/gitrepo/ancestry.go`
- `/home/knatte/Code/loomyard/wts/fabric-rebase-reconcile/internal/gitrepo/ancestry_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-rebase-reconcile/internal/gitrepo/ancestry_integration_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-rebase-reconcile/internal/gitrepo/push.go`
- `/home/knatte/Code/loomyard/wts/fabric-rebase-reconcile/internal/gitrepo/gogit.go`
- `/home/knatte/Code/loomyard/wts/fabric-rebase-reconcile/internal/gitrepo/gogit_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-rebase-reconcile/internal/gitrepo/push_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-rebase-reconcile/internal/boardengine/sync.go`
- `/home/knatte/Code/loomyard/wts/fabric-rebase-reconcile/cmd/lyx/gitrepoboundary_test.go`
- `/home/knatte/Code/loomyard/wts/fabric-rebase-reconcile/CONSTRAINTS.md`
- `/home/knatte/Code/loomyard/wts/fabric-rebase-reconcile/internal/gitrepo/doc.go`

{"status":"success","commit_sha":"99d79ebb4a8a031b32cdff70aa8a0d834d798668","session_id":"1e6268ca-0f3b-41f7-89a8-532b40027dcb","cards_done":[1,2,3,4]}
