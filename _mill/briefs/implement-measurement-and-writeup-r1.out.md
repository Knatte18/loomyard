{"status":"success","commit_sha":"2e87570c0655774a4bcff25e8ec05c34d7751d42","session_id":"8866c0ad-8867-4afe-924c-2c49a3eafa08"}

1 of 1 cards committed (Card 17), verify `go test -tags integration ./internal/codeintelengine/...` passed (live-gopls integration test confirmed executed, not skipped).

Work: measured multi-language `lyx codeintel refs` across Go/gopls and Python/pylsp arms (3 runs each) against real permissively-licensed target repos (psf/requests, restsharp/RestSharp; gitignored). Wrote docs/research/codeintel-multilang.md in the house style with verdict, cost/precision tables, per-language honesty notes. Go arm 0 FN/0 FP once ground truth scoped correctly (documented gopls -tags limitation); pylsp 37% recall / 0 FP documented. pyright and csharp-ls recorded as pending operator install per graceful-degradation clause. Cleaned up temporary _lyx/config/servers.yaml overlay.
