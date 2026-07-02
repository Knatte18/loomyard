Clean tree, follow-up fix committed and pushed. The race in TestWithOpLock_SerializesConcurrentCalls (internal/muxengine/lock_test.go) is resolved: the final select now waits deterministically on secondErr. go test ./internal/muxengine/... -count=1 passes; 5 repeated runs consistently green. Only the test file touched; lock.go unchanged.

{"status":"success","commit_sha":"e38ca50e05fcb7dbad2164a3b00e41547966a542","session_id":"2afa96bf-ca46-4b53-b4cf-a3dc39d5d65a"}
