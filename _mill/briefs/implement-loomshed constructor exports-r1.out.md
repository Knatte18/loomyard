1 of 1 cards committed — this batch has exactly 1 card, and it matches the commit message. All complete.

Batch "loomshed constructor exports" (batch 1, 1 of 1 card) is complete. I renamed the six unexported loomshed producer constructors to exported forms returning `shedengine.ShedProducer`, keeping their concrete types unexported:

- `/home/knatte/Code/loomyard/wts/shed-recipe-engine-registry/internal/loomshed/stub.go` — `newStub` → `NewStub`
- `/home/knatte/Code/loomyard/wts/shed-recipe-engine-registry/internal/loomshed/batchifier.go` — `newBatchifier` → `NewBatchifier`
- `/home/knatte/Code/loomyard/wts/shed-recipe-engine-registry/internal/loomshed/discussionvalidate.go` — `newDiscussionValidate` → `NewDiscussionValidate`
- `/home/knatte/Code/loomyard/wts/shed-recipe-engine-registry/internal/loomshed/loompreflight.go` — `newLoomPreflight` → `NewLoomPreflight` (godoc's stale "constructor is unexported" paragraph replaced)
- `/home/knatte/Code/loomyard/wts/shed-recipe-engine-registry/internal/loomshed/planvalidate.go` — `newPlanValidate` → `NewPlanValidate`
- `/home/knatte/Code/loomyard/wts/shed-recipe-engine-registry/internal/loomshed/webster.go` — `newWebsterProducer` → `NewWebsterProducer`
- `/home/knatte/Code/loomyard/wts/shed-recipe-engine-registry/internal/loomshed/loomshed.go` — six call sites in `New`'s `producers` slice updated
- Seven test files updated (call sites only, no assertion/fixture/name changes): `stub_test.go`, `batchifier_test.go`, `discussionvalidate_test.go`, `loompreflight_test.go`, `planvalidate_test.go`, `webster_test.go`, `resume_test.go`

`sequence_test.go` and `loomshed_test.go` were left untouched as required. `seam_enforcement_test.go` was left untouched (no new imports). `go build ./internal/loomshed/...` and `go test ./internal/loomshed/...` both pass, `gofmt -l` is clean. Commit `217a1c1bb263b31e40738bfe8c9327d87ed23b73` pushed to `shed-recipe-engine-registry`.

{"status":"success","commit_sha":"217a1c1bb263b31e40738bfe8c9327d87ed23b73","session_id":"d233ec3d-06fb-4c62-b7da-d74f07f41920","cards_done":[1]}
