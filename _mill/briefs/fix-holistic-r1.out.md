Summary of work:

**Finding processed** (via mill-receiving-review decision tree):
- `[NIT] wordswap's byte-offset scan assumes ASCII-stable strings.ToLower` (`tools/wordswap/swap.go:117-136`) — VERIFY: accurate, the byte-offset desync risk for non-byte-length-preserving lowercase mappings (e.g. U+0130 Ä°) is real. HARM CHECK: no functionality broken, no design conflict, no destabilization from documenting the constraint. Action: FIX — added the ASCII-only precondition to the package doc comment in `/home/knatte/Code/loomyard/wts/fabric-host-to-warp-rename/tools/wordswap/main.go`, per the review's own suggested minimal remediation ("Worth a follow-up guard... or an explicit ASCII-only precondition documented in the package doc"). Did not touch the tested scan control flow itself, since the review confirmed zero actual instances in the ~100 swept files this task covers.

**Commit:** `a9b9e147ecea5879171b38a16e3fa1ce4fadcd7b` — "tools/wordswap: document ASCII-only precondition for offset scan", pushed to `fabric-host-to-warp-rename`.

**Verify:** ran every non-null `verify:` command from all 7 batch plan files (`01`-`07`) in order — `go test ./tools/wordswap/...`, the batch-2 package subset, `go test ./...` + `-tags integration ./...` (batch 3 and again batch 7), `go test ./internal/fabricengine/...` (+integration), `go test ./cmd/lyx/... ./internal/fabriccli/... ./tools/sandbox/...`, and `go test ./cmd/lyx/... ./internal/lyxcwd/...`. All green, no FAIL lines.

**Pre-report check:** baseline HEAD was `0438bfe8ca204d38e272d7994c2e11ff57a757b5`; final HEAD `a9b9e147ecea5879171b38a16e3fa1ce4fadcd7b` differs. `git status --porcelain --untracked-files=no` is clean (no uncommitted tracked changes).

{"status":"success","commit_sha":"a9b9e147ecea5879171b38a16e3fa1ce4fadcd7b","session_id":"b6200f19-b735-40b8-927e-62923b7e6b7d"}

{"status":"success","commit_sha":"a9b9e147ecea5879171b38a16e3fa1ce4fadcd7b","session_id":"b6200f19-b735-40b8-927e-62923b7e6b7d"}
