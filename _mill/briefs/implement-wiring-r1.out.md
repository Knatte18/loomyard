All 5 cards in batch 07-wiring (cards 22, 23, 24, 25, 26) are implemented, committed, and pushed. The batch verify command (`go test ./internal/loomcli/... ./internal/webstercli/... ./internal/burlercli/...`) passes, and `go build ./...` confirms no cross-package breakage.

Key files touched:
- /home/knatte/Code/loomyard/wts/self-report-tier2/internal/loomcli/wiring.go
- /home/knatte/Code/loomyard/wts/self-report-tier2/internal/loomcli/cli.go
- /home/knatte/Code/loomyard/wts/self-report-tier2/internal/loomcli/run.go
- /home/knatte/Code/loomyard/wts/self-report-tier2/internal/loomcli/drive.go
- /home/knatte/Code/loomyard/wts/self-report-tier2/internal/loomcli/wiring_test.go
- /home/knatte/Code/loomyard/wts/self-report-tier2/internal/loomcli/friction_test.go
- /home/knatte/Code/loomyard/wts/self-report-tier2/internal/webstercli/wiring.go
- /home/knatte/Code/loomyard/wts/self-report-tier2/internal/webstercli/cli.go
- /home/knatte/Code/loomyard/wts/self-report-tier2/internal/webstercli/beginbatch.go
- /home/knatte/Code/loomyard/wts/self-report-tier2/internal/webstercli/recoverbatch.go
- /home/knatte/Code/loomyard/wts/self-report-tier2/internal/webstercli/run.go
- /home/knatte/Code/loomyard/wts/self-report-tier2/internal/webstercli/wiring_test.go
- /home/knatte/Code/loomyard/wts/self-report-tier2/_mill/plan/07-wiring.md (extended card 26's Edits to include internal/loomcli/run.go, needed to extract a testable friction seam there — committed separately as a plan edit before the code change, per protocol)

One notable design decision I made: rather than inline the reflection/seed-clear logic directly in runCmd/driveCmd's RunE closures, I extracted ensureFrictionDirAfterSeed (run.go), and shouldReflectFriction/reflectFriction (drive.go) as package-level/method seams, matching the existing bootstrap_test.go pattern (mustSpawnDriver, awaitRunLock) so the new tests could drive every branch at Tier 1 without a real Shed or cobra invocation.
