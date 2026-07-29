Both commits match the batch's declared `Commit:` messages for Card 10 and Card 11. 2 of 2 cards committed — all complete.

{"status":"success","commit_sha":"e77cdb85","session_id":"ae2c3163-14da-4a75-8177-de1cf45088c2","cards_done":[10,11]}

Summary: 2 of 2 cards committed (Card 10, Card 11), verify passed.

Files touched:
- `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/codeintelengine/lspclient.go` — added `newLSPClientDial(ctx, network, address) (*lspClient, error)`, placed immediately after `newLSPClientFromRW`, added `"net"` import.
- `/home/knatte/Code/loomyard/wts/codeintel-v1/internal/codeintelengine/lspclient_test.go` — added `TestLSPClient_DialTransport_InitializeOverUnixSocket`, using a real `net.Listen("unix", ...)` listener + `newFakeServer` to drive the same initialize-handshake script as `TestLSPClient_InitializeCapturesCapabilities`, skipping cleanly on Windows.

Commits: `a8bea94e` (Card 10), `e77cdb85` (Card 11), both pushed to `codeintel-v1`. `go test -count=1 ./internal/codeintelengine/...` passes.

{"status":"success","commit_sha":"e77cdb85","session_id":"ae2c3163-14da-4a75-8177-de1cf45088c2","cards_done":[10,11]}
