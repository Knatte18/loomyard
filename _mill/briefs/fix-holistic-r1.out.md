All three NIT findings from the holistic review were evaluated per the mill-receiving-review decision tree and fixed (none qualified for pushback — no factual inaccuracy, no harm, no design conflict):

1. **`-c push.autoSetupRemote=true` undocumented in doc.go`** (findings 1 & 3, same fix) — added a sentence to the Push-surface section of `/home/knatte/Code/loomyard/wts/gitrepo/internal/gitrepo/doc.go` explaining the flag and its rationale (first push with no upstream still succeeds/establishes tracking, matching `hasUnpushed`'s contract).
2. **"no remote configured" error-passthrough scenario untested** — added one test per affected method against a repo with zero remotes configured at all (not merely no upstream tracking):
   - `TestPush_NoRemoteConfigured_SurfacesGitError` and `TestPushCoalesced_NoRemoteConfigured_SurfacesGitError` in `/home/knatte/Code/loomyard/wts/gitrepo/internal/gitrepo/push_test.go`
   - `TestSetSnapshotSHA_NoRemoteConfigured_SurfacesGitError` in `/home/knatte/Code/loomyard/wts/gitrepo/internal/gitrepo/snapshot_test.go`

   All three new tests confirm the wrapped error still carries git's own stderr unchanged, per discussion.md's Testing section requirement.

Commits made (baseline HEAD was `6f80f381f872ae322e54e105b7be528693d1b7e6`):
- `cb41017d` — docs: note push.autoSetupRemote flag in gitrepo package doc
- `6c90cc48` — test: cover no-remote-configured error passthrough for Push, PushCoalesced, SetSnapshotSHA

Verify commands run (all exit 0):
- `go test -tags integration ./internal/gitrepo/` (batches 1–3)
- `go build ./internal/gitrepo/` (batch 4)

Working tree is clean (`git status --porcelain --untracked-files=no` empty), HEAD (`6c90cc4854f3427ac5c726e8f5da8b8b1a701c36`) differs from the recorded baseline.

{"status":"success","commit_sha":"6c90cc4854f3427ac5c726e8f5da8b8b1a701c36","session_id":"96b2e824-6ef6-423d-b8a0-ec5eed582c88"}
