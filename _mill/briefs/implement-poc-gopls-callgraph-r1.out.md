{"status":"success","commit_sha":"2fa53f6283234b01ae3d160f97f5af6bcec42ed2","session_id":"5fc2ce0f-438f-4406-9774-7e1f2e5f93dd"}

Summary (2 of 2 cards committed):

- Card 4 (commit 4f61f4f6): tools/codeintel-poc/gopls.go — minimal Content-Length-framed LSP JSON-RPC client (initialize/initialized/textDocument/references/shutdown/exit) driving a held-open gopls subprocess for `gopls-refs` mode, plus `gopls-cli-refs` mode. Registered both in main.go dispatch.
- Card 5 (commit 2fa53f62): tools/codeintel-poc/callgraph.go — builds SSA program, computes transitive caller set for -algo=cha|rta|vta, seeded from cmd/lyx main.main + package inits + TestMain. Registered `callgraph` mode.

Note for batch 3: generic targets have no single call-graph node; transitiveCallers seeds from all nodes sharing Origin(). gopls binary not on $PATH in container (present at /home/knatte/go/bin/gopls) — batch 3 is the runtime-exercise point.

`go build ./tools/codeintel-poc/` passes; gofmt/go vet/golangci-lint clean on new code. 2 of 2 committed.
