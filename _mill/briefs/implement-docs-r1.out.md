4 of 4 cards committed (cards 10, 11, 12, 13), matching the batch's declared 4 cards. Verify (`go build ./... && go test ./...`) passed cleanly.

{"status":"success","commit_sha":"939d7889e2b13f50bbbcd5a0d068dd42ea835b85","session_id":"87ae4143-317a-42fd-865e-3c63aa8c81b6","cards_done":[10,11,12,13]}

Summary of work in this batch (03-docs.md):

- `/home/knatte/Code/loomyard/wts/pattern-directive-stencils/internal/pattern/doc.go` — corrected the "Why the pointer stays relative" claim and added a new "The stencil read path" subsection; extended the header/opening-paragraph summaries. Commit `b2389e27`.
- `/home/knatte/Code/loomyard/wts/pattern-directive-stencils/manifest/designs/pattern-directive-stencils.md` — flipped status to Built, corrected steps 3 (fail-loud, not fail-silent) and 4 (two call sites are map-literal hoists, not plumbing-free), added the missing banner-strip step, corrected the "Related" bullet and the Test migration section to reference the stripped body rather than whole-file bytes; kept the file per the recorded Shared Decision. Commit `797cb834`.
- `/home/knatte/Code/loomyard/wts/pattern-directive-stencils/manifest/roadmap.md` — moved the PATTERN directive stencils item from Planned to Done, pointing at `internal/pattern`'s package documentation, dropped the stale scheduling sentence, no renumbering. Commit `3d671220`.
- `/home/knatte/Code/loomyard/wts/pattern-directive-stencils/tools/sandbox/SANDBOX-CORE-SUITE.md` — updated "fifteen" to "eighteen" registered stencils (verified against `stencils/stencils.go`'s 18 registry entries; confirmed no other stale count existed in the file). Commit `939d7889`.

All 4 cards declared in this batch are committed. `go build ./...` and `go test ./...` both passed cleanly with no failures. Working tree is clean (no uncommitted tracked changes); the only untracked file is the brief itself (`_mill/briefs/implement-docs-r1.md`), out of scope.

{"status":"success","commit_sha":"939d7889e2b13f50bbbcd5a0d068dd42ea835b85","session_id":"87ae4143-317a-42fd-865e-3c63aa8c81b6","cards_done":[10,11,12,13]}