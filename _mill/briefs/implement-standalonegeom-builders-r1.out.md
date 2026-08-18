All 5 cards (4-8) in batch `02-standalonegeom-builders.md` were completed and committed this turn, verify passed, and the working tree is clean.

{"status":"success","commit_sha":"4a4afa168abfced1f17b9171c42dd399792997a6","session_id":"83709310-e786-4604-8e3c-9bf3387d5a83","cards_done":[4,5,6,7,8]}

Summary: 5 of 5 cards committed (all of the batch's declared cards).

Files touched:
- `/home/knatte/Code/loomyard/wts/standalone-cli-entry/internal/standalonegeom/stencilsdir.go` (created, Card 4)
- `/home/knatte/Code/loomyard/wts/standalone-cli-entry/internal/standalonegeom/webstergeom.go` (edited, Card 5)
- `/home/knatte/Code/loomyard/wts/standalone-cli-entry/internal/standalonegeom/burlergeom.go` (created, Card 6)
- `/home/knatte/Code/loomyard/wts/standalone-cli-entry/internal/standalonegeom/perchgeom.go` (created, Card 7)
- `/home/knatte/Code/loomyard/wts/standalone-cli-entry/internal/standalonegeom/doc.go` (edited, Card 8)
- `/home/knatte/Code/loomyard/wts/standalone-cli-entry/internal/standalonegeom/standalonegeom_test.go` (edited, Card 8)

`go test ./internal/standalonegeom/...` passes with all 5 tests (TestBurlerGeometry, TestPerchGeometry, TestStencilsDir, TestReedGeometry, TestWebsterGeometry). `go build ./internal/standalonegeom/...` and `gofmt -l` on all touched files are clean. `git status --porcelain --untracked-files=no` shows no uncommitted tracked changes. All commits pushed to `origin/standalone-cli-entry`.

{"status":"success","commit_sha":"4a4afa168abfced1f17b9171c42dd399792997a6","session_id":"83709310-e786-4604-8e3c-9bf3387d5a83","cards_done":[4,5,6,7,8]}