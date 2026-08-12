40 chars + newline, correct. Full SHA: `4f22c75326cb59a07ee3ab04f3ab461986116d8f`.

All 5 cards (12–16) of batch `gate-auto-recording` committed and verified: `go test ./internal/fabricengine/ ./internal/fabriccli/` passes, `go vet -tags integration ./...` passes clean, and working tree is clean.

Key files touched:
- `/home/hanf/Code/loomyard/wts/fabric-mutation-record-envelope/internal/fabricengine/destroy.go` — the 8 gate executors now take a leading `rec *Mutations` and self-record on observed effect only.
- `/home/hanf/Code/loomyard/wts/fabric-mutation-record-envelope/internal/fabricengine/{portals,launchers,remove,prune,cleanup,weftwiring,reconcile}.go` — removal helpers threaded.
- `/home/hanf/Code/loomyard/wts/fabric-mutation-record-envelope/internal/fabricengine/{junction,unwire,clone}.go`, `/home/hanf/Code/loomyard/wts/fabric-mutation-record-envelope/internal/fabriccli/clone.go` — junction/unwire/clone helpers threaded; `WireJunctionsWith` added alongside a nil-recorder `WireJunctions` wrapper.
- `/home/hanf/Code/loomyard/wts/fabric-mutation-record-envelope/internal/fabricengine/{add,checkout,pull}.go` — `rollbackAdd`, `rollbackSwitch`, and `Pull`'s `ResetHard` calls threaded.
- `/home/hanf/Code/loomyard/wts/fabric-mutation-record-envelope/internal/fabricengine/destroy_test.go` — new `TestGate_RecordOnlyOnObservedEffect` table tests.
- `/home/hanf/Code/loomyard/wts/fabric-mutation-record-envelope/_mill/plan/04-gate-auto-recording.md` — plan extended to include the previously-undeclared `destructivegaps_integration_test.go` (discovered dependency), committed before editing it.

{"status":"success","commit_sha":"4f22c75326cb59a07ee3ab04f3ab461986116d8f","session_id":"202aa53e-e5ac-407a-abf9-239fa0082e68","cards_done":[12,13,14,15,16]}
