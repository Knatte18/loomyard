Both cards' commit subjects match the batch file's `## Cards` `Commit:` messages exactly: 2 of 2 cards committed.

{"status":"success","commit_sha":"4a729b2f","session_id":"6b026116-ee79-4b72-9e91-455d3fadca21"}

Summary: 2 of 2 cards committed for batch `03-burler-config-module`. Verify command `go test ./internal/burlerengine/ ./internal/configreg/` passes. Working tree clean (no uncommitted tracked changes).

Files touched:
- `/home/knatte/Code/loomyard/wts/burler-fork-cluster/internal/burlerengine/config.go` (new — Config, Lens, ConfigTemplate, LoadConfig, maxClusterN, ResolveFan)
- `/home/knatte/Code/loomyard/wts/burler-fork-cluster/internal/burlerengine/config_test.go` (new)
- `/home/knatte/Code/loomyard/wts/burler-fork-cluster/internal/burlerengine/template.yaml` (new — nine lenses, `standard`/`full` fans)
- `/home/knatte/Code/loomyard/wts/burler-fork-cluster/internal/configreg/configreg.go` (edited — registered `burler` as seed-only)
- `/home/knatte/Code/loomyard/wts/burler-fork-cluster/internal/configreg/configreg_test.go` (edited — updated `TestNames`/`TestModules_SeedOnly` pins)

Commits: `da3cb47b` (Card 6), `4a729b2f` (Card 7), both pushed to `burler-fork-cluster`.

{"status":"success","commit_sha":"4a729b2f","session_id":"6b026116-ee79-4b72-9e91-455d3fadca21"}
