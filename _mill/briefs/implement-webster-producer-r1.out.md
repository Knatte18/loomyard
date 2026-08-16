Both cards (8 and 9) committed and match the batch file's Commit: messages. 2 of 2 cards committed — all complete.

{"status":"success","commit_sha":"90b4406ab71e300f32526a7720b4d9f133f73a11","session_id":"0f89475b-2048-4d93-acd7-09d3c8badf75","cards_done":[8,9]}

Summary: implemented both cards of batch 03-webster-producer.

- `/home/knatte/Code/loomyard/wts/shed-adapters/internal/shedadapters/webster.go` (new): `WebsterRunner` func-typed seam with `var _ WebsterRunner = websterengine.Run` proof, `WebsterProducer` struct with unexported `name`/`run`/`deps` fields, `NewWebsterProducer` constructor defaulting nil `run` to `websterengine.Run`, local outcome literals `websterOutcomeDone`/`Stuck`/`Paused`, and `Call` implementing entry-check → run with `Fresh: false` → asking-error-as-Stuck special case (via `errors.Is`/`errors.As`) → outcome mapping with `cancelErr` consulted on every non-done exit path.
- `/home/knatte/Code/loomyard/wts/shed-adapters/internal/shedadapters/webster_test.go` (new): fake `WebsterRunner`-driven table tests covering outcome mapping (done/stuck/paused/unrecognized), error mapping (asking via `errors.Is`, died/timeout/busy/mismatch/nil-batcher, plain unmatched error), the `Fresh: false` safety property, context rows (already-cancelled, cancelled-during-run for done/stuck/paused/error), and the no-bridge assertion via `websterengine.PauseRequested`.

Both cards committed as `09b182c4` and `90b4406a`, pushed to `shed-adapters`. `go test ./internal/shedadapters/...` passes (32 test functions including prior batches' package-foundation-and-singlellm and perch-producer suites). Working tree is clean; no plan-file edits were needed — every file touched was already declared in the batch's Context/Creates lists.

{"status":"success","commit_sha":"90b4406ab71e300f32526a7720b4d9f133f73a11","session_id":"0f89475b-2048-4d93-acd7-09d3c8badf75","cards_done":[8,9]}