14 commits, one per card (cards 9-22, 14 cards). All match the batch's declared 14 cards.

{"status":"success","commit_sha":"427c28a26e99ef5d87abba192e82208c2b880faa","session_id":"8ea70cee-5678-4b27-ac29-f7abee593066","cards_done":[9,10,11,12,13,14,15,16,17,18,19,20,21,22]}

Card-count self-check: 14 of 14 cards committed (cards 9 through 22), matching the batch's declared 14 cards exactly, verified by comparing commit subjects since the "mill-go: start batch" commit against each card's `Commit:` message. All three verify packages (`internal/shedrecipe`, `internal/loomrecipe`, `internal/loomcli`) pass, gofmt/goimports show no drift on touched files, and `git status --porcelain --untracked-files=no` shows no dirty tracked files.

Relevant files touched:
- `/home/knatte/Code/loomyard/wts/loom-discussion-write-producer/internal/shedrecipe/recipe.go`
- `/home/knatte/Code/loomyard/wts/loom-discussion-write-producer/internal/shedrecipe/entries_discussionwrite.go`
- `/home/knatte/Code/loomyard/wts/loom-discussion-write-producer/internal/shedrecipe/entries_discussionwrite_test.go`
- `/home/knatte/Code/loomyard/wts/loom-discussion-write-producer/internal/shedrecipe/registry.go`
- `/home/knatte/Code/loomyard/wts/loom-discussion-write-producer/internal/shedrecipe/registry_test.go`
- `/home/knatte/Code/loomyard/wts/loom-discussion-write-producer/internal/shedrecipe/fixture_test.go`
- `/home/knatte/Code/loomyard/wts/loom-discussion-write-producer/contracts/recipes/loom-recipe.yaml`
- `/home/knatte/Code/loomyard/wts/loom-discussion-write-producer/internal/loomrecipe/fixture_test.go`
- `/home/knatte/Code/loomyard/wts/loom-discussion-write-producer/internal/loomrecipe/shape_test.go`
- `/home/knatte/Code/loomyard/wts/loom-discussion-write-producer/internal/loomrecipe/coverage_guard_test.go`
- `/home/knatte/Code/loomyard/wts/loom-discussion-write-producer/internal/loomrecipe/resume_test.go`
- `/home/knatte/Code/loomyard/wts/loom-discussion-write-producer/internal/loomrecipe/sequence_test.go`
- `/home/knatte/Code/loomyard/wts/loom-discussion-write-producer/internal/loomcli/wiring.go`
- `/home/knatte/Code/loomyard/wts/loom-discussion-write-producer/internal/loomcli/wiring_test.go`

{"status":"success","commit_sha":"427c28a26e99ef5d87abba192e82208c2b880faa","session_id":"8ea70cee-5678-4b27-ac29-f7abee593066","cards_done":[9,10,11,12,13,14,15,16,17,18,19,20,21,22]}