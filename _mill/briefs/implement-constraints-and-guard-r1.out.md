40 chars + newline, full SHA confirmed. All 5 cards committed (5 of 5 cards complete — cards 21, 22, 23, 24, 25 all committed with real content commits).

{"status":"success","commit_sha":"114d70d7f4e571a68321f87d08b40e075a07151c","session_id":"75b8ba39-b19c-433e-be04-c5e01f7cb97a","cards_done":[21,22,23,24,25]}

Summary of work:

All 5 cards in batch `07-constraints-and-guard.md` are complete (5 of 5 committed).

- Card 21: Rewrote `CONSTRAINTS.md`'s Fabric Vocabulary Invariant and `docs/overview.md` line 80/81 to state the Fabric/warp/weft/host rule as one coherent rule, dropping `tools/`/`sandbox/` from the owner set. Commit `b000571a`.
- Card 22: Renamed remaining retired-identifier citations in `CONSTRAINTS.md` (Cwd Resolution, Fabric Git, Review Round invariants). Commit `98ff72e8`.
- Card 23 + 24 (combined, same file): Tightened `internal/lyxcwd/enforcement_test.go`'s host-phrase guard to fire inside owner dirs in both the Go walk and the `.md` walk, rewrote the falsified doc comments/sub-test, and added sub-tests proving the tightened behavior. Commit `9e05a2c9`.
- Card 25: Ran the repo-wide completeness grep. Found one genuine miss not on the plan's exclusion list — `manifest/designs/raddle.md:63` still said "the host (warp) code SHA" / "the last host commit" (a stray leftover next to an already-added `(warp)` clarification). Since this file wasn't in the batch's declared scope, followed the out-of-scope-file protocol: stopped, added `manifest/designs/raddle.md` to card 25's Context/Edits in `_mill/plan/07-constraints-and-guard.md`, committed the plan edit (`7504d13b`), then made the fix (`114d70d7`). Also confirmed `docs/shared-libs/configengine.md:66`'s `${env:HOST:-localhost}` is the same benign machine-sense example as the allowed `yamlengine.md:40` line (an enumeration omission, not a real hit — left untouched), and that `tools/wordswap/*` hits are the tool's own subject matter.

Verify: `go test ./...` and `go test -tags integration ./...` both pass in full.

Files touched: `/home/knatte/Code/loomyard/wts/fabric-host-to-warp-rename/CONSTRAINTS.md`, `/home/knatte/Code/loomyard/wts/fabric-host-to-warp-rename/docs/overview.md`, `/home/knatte/Code/loomyard/wts/fabric-host-to-warp-rename/internal/lyxcwd/enforcement_test.go`, `/home/knatte/Code/loomyard/wts/fabric-host-to-warp-rename/manifest/designs/raddle.md`, `/home/knatte/Code/loomyard/wts/fabric-host-to-warp-rename/_mill/plan/07-constraints-and-guard.md`.

Working tree is clean; no uncommitted tracked changes.

{"status":"success","commit_sha":"114d70d7f4e571a68321f87d08b40e075a07151c","session_id":"75b8ba39-b19c-433e-be04-c5e01f7cb97a","cards_done":[21,22,23,24,25]}
