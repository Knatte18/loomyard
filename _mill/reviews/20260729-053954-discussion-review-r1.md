MILL_REVIEW_BEGIN
# Review: codeintel V1 — LSP-backed lookups (Go-only, CLI + EnsureServer)

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewed_file: _mill/discussion.md
date: 2026-07-29
```

## Findings

### [GAP] supervised reconnect transport unspecified for stdio gopls
**Section:** Decisions › state-file-location-and-content; Scope; Testing
**Issue:** `supervised` must reconnect across separate `lyx` processes, but plain `gopls` speaks only stdio (lspclient.go is stdio-only, `-listen`/`-remote` excluded); the state file names a socket address yet nothing specifies which long-lived process owns gopls's stdio and serves that socket, so the "kill-and-confirm-restart against plain gopls" proof — the central layer-2 deliverable — is not buildable as written.
**Fix:** Specify the supervised transport: a lyx-owned proxy/wrapper daemon that holds gopls stdio and exposes the socket, plus whether `lspClient` needs a socket-transport variant.

### [NOTE] internal/lock vs bespoke lock left explicitly unresolved
**Section:** Decisions › concurrency-locking
**Issue:** The heading "decides" `internal/lock` but then flags the third-party `gofrs/flock` dependency-widening of `codeintelengine`'s leaf as an open trade-off "for mill-plan/review to weigh," so the CONSTRAINTS.md leaf-amendment scope is not actually settled.
**Fix:** Resolve the choice (accept the widened leaf, or the stdlib `O_EXCL` alternative) before the plan writes the invariant amendment.

### [NOTE] toolchain path vs registry Command[0] composition implicit
**Section:** Decisions › toolchain-manager-authority; native-strategy-wire-compatibility
**Issue:** Registry `Command` for go is bare `["gopls"]` ($PATH lookup), but the toolchain decision says never consult $PATH for Go and always use the cached absolute binary; how the cached path replaces `entry.Command[0]` and where `-remote=auto` is appended is not stated.
**Fix:** State that EnsureServer/toolchain resolves the absolute cached binary and builds the native argv, overriding `entry.Command[0]` for Go.

### [NOTE] toolchain cache-dir location and platform handling undecided
**Section:** Decisions › toolchain-manager-authority; design layer-1
**Issue:** The cache dir is only illustrative ("e.g. `~/.cache/lyx/tools/go/<version>/`"); a raw home-dir path outside hubgeometry's geometry tokens, with no XDG_CACHE_HOME / Windows convention decided.
**Fix:** Pin the concrete cache-root resolution and confirm it is legitimately outside the Hub Geometry Invariant's scope.

### [NOTE] EnsureServer loser retry-exhaustion path unspecified
**Section:** Decisions › concurrency-locking
**Issue:** Losers "poll the state file until the winner's PID+probe succeeds (bounded retry)" but the behavior when the bounded retry exhausts (winner never comes up) — hard error vs. fallback cold-spawn — is not stated.
**Fix:** Name the terminal failure mode when the spawn-race winner never produces a healthy server.

## Verdict

GAPS_FOUND
One unbuildable-as-written gap: the supervised reconnect transport for stdio gopls.
MILL_REVIEW_END
