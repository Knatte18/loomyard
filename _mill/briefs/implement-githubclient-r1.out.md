Card-count self-check: 7 of 7 cards committed (24–30), matching all `Commit:` messages declared in `_mill/plan/06-githubclient.md`'s `## Cards` section. All complete.

Summary of work:

- **Card 24** (`build: add go-github v75 dependency`) — added `github.com/google/go-github/v75 v75.0.0` as a direct dependency in `go.mod`/`go.sum`; `go-git` stayed pinned at v5.19.1.
- **Card 25** (`feat(githubclient): add non-blocking token resolution chain`) — `internal/githubclient/token.go`: env-var → cache → bounded `gh auth token` shell-out chain behind an injectable seam, returning the source alongside the token.
- **Card 26** (`feat(githubclient): add machine-global token cache...`) — `internal/githubclient/cache.go`, `cache_windows.go`, `cache_other.go`: atomic temp-file-then-rename cache with Windows DACL hardening applied before rename.
- **Card 27** (`feat(githubclient): add authenticating RoundTripper...`) — `internal/githubclient/transport.go`: `authRT` sets/clones/replays with `GetBody` rewind and env-source 401 short-circuit.
- **Card 28** (`feat(githubclient): add authenticated client constructors`) — `internal/githubclient/githubclient.go`: `New()`/`NewWithBaseURL()`; folded in the `go mod tidy` pass that settled `go-querystring` as an indirect dependency once real usage existed.
- **Card 29** (`test(githubclient): enforce leaf import allowlist`) — `internal/githubclient/leaf_enforcement_test.go`.
- **Card 30** (`test(githubclient): cover token chain, cache, and 401 replay`) — `internal/githubclient/githubclient_test.go` (built `//go:build windows`, permitted as "untagged" Tier 1 per CONSTRAINTS.md's platform-constraint carve-out, since it asserts the Windows DACL directly). While writing this test I found and fixed a genuine `net/http.RoundTripper` contract violation in `transport.go` (returning a non-nil response alongside a non-nil error); I added `transport.go` to card 30's `Edits:` list via a `plan:` commit before applying the fix, per the brief's scope-extension protocol.

Verify: `go test -race -count=1 ./internal/githubclient/...` passes (13/13 test functions green). Note: this sandbox had no C compiler initially, so `-race` failed at the tool level (`CGO_ENABLED` requires cgo) — confirmed pre-existing/environmental by reproducing the identical failure against the already-committed `internal/gitrepo` package; self-fixed by locating an already-installed WinLibs GCC toolchain via `winget` and prepending it to `PATH` for this session, after which the full `-race` verify command passed cleanly.

Files touched (all under `C:\Code\loomyard\wts\native-clients-migration`): `go.mod`, `go.sum`, `internal/githubclient/token.go`, `internal/githubclient/cache.go`, `internal/githubclient/cache_windows.go`, `internal/githubclient/cache_other.go`, `internal/githubclient/transport.go`, `internal/githubclient/githubclient.go`, `internal/githubclient/leaf_enforcement_test.go`, `internal/githubclient/githubclient_test.go`, `_mill/plan/06-githubclient.md`.

{"status":"success","commit_sha":"1b29d86f","session_id":"a7f0765e-de94-4bb2-8acf-10053a2c131e","cards_done":[24,25,26,27,28,29,30]}
