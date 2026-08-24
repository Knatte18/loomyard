Both commits match the batch's two `Commit:` messages exactly. 2 of 2 cards committed, all complete.

Summary: implemented both cards of batch 1 (sequencing core) — created `internal/websterengine/sequence.go` (the `Cycle` type, `SequenceBatches`, and its edge-derivation/Tarjan-SCC/Kahn-ordering helpers) and `internal/websterengine/sequence_test.go` (Tier 1 coverage). `go test ./internal/websterengine/...` passes, gofmt/goimports/golangci-lint are clean on the new files (one pre-existing, unrelated errcheck finding remains in `runlevel.go:321`, untouched by this batch). Both commits are pushed.

{"status":"success","commit_sha":"1a132ff5b9088f96e2b4793c85d5060473f0822a","session_id":"66df0333-d368-4004-8bc9-1c3283147abb","cards_done":[1,2]}
