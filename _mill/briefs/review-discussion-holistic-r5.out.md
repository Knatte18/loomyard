MILL_REVIEW_BEGIN
# Review: codeintel V1 — LSP-backed lookups (Go-only, CLI + EnsureServer)

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: /home/knatte/Code/loomyard/wts/codeintel-v1/_mill/discussion.md
date: 2026-07-29
```

## Findings

### [NOTE] Stale unix-socket cleanup on supervised restart
**Section:** supervised-reconnect-transport / state-file-location-and-content
**Issue:** On the kill-and-restart path, a `gopls serve -listen=unix;<path>` respawn binding the same recorded path fails with EADDRINUSE if the stale socket file still exists on disk (a unix socket bind errors on a pre-existing path even with no listener).
**Fix:** State whether restart unlinks the old socket file first or always picks a fresh lyx-chosen path — either resolves it; implementer's call, but naming which avoids an ambiguous failure mode.

### [NOTE] Zero-valued (no-daemon) mode teardown not spelled out
**Section:** registry-scope / ensure-server-call-site
**Issue:** `ensure-server-call-site` forbids closing the daemon-owned connection, and `native-lifecycle-and-probe-failure` covers native's per-call subprocess teardown, but the implicit zero-valued path (Python/C#/TS/Rust) relies on "exactly today's shipped behavior" (spawn→call→close) without restating that its connection IS closed by the caller.
**Fix:** One sentence confirming the zero-valued path retains today's caller-owned close/kill, so the "never close daemon-owned conn" rule is understood as supervised/native-daemon-only.

## Verdict

APPROVE
Zero GAPs; all load-bearing source claims verified, two minor NOTEs recorded.
MILL_REVIEW_END
