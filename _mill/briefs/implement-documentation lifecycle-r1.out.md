All 6 cards (36-41) committed, matching all 6 `Commit:` messages declared in the batch file. 6 of 6 cards committed -- full completion, verified against the actual commit log, not an unqualified claim.

{"status":"success","commit_sha":"d742a88b7ff7aa8d13ef5a59211ad8fbf33bdc9e","session_id":"8312e9fd-f272-4b04-9b2e-609e0dcc1c2a","cards_done":[36,37,38,39,40,41]}

Summary of work (all paths absolute from `/home/knatte/Code/loomyard/wts/landing-publish-finalize-producers`):

- Card 36: folded durable content from the deleted design doc into `internal/mergeresolve/doc.go` and `internal/landingshed/doc.go`.
- Card 37: deleted `manifest/designs/landing.md`.
- Card 38: repointed all inbound links in `manifest/designs/loom.md`, `manifest/designs/shed.md`, `manifest/designs/raddle.md`, `manifest/designs/fabric-unified-view.md` (including dropping the stale "teardown" claim from `loom.md` and rewording the `fabric-unified-view.md` conflict-shape reference and `shed.md`'s status-banner prose).
- Card 39: moved the landing item to Done in `manifest/roadmap.md`, rewrote it to match what shipped, and corrected the `loom: phase-machine scaffolding` Done entry's row count/stub list.
- Card 40: added `internal/landingshed` and `internal/mergeresolve` to the module tree in `docs/overview.md`, corrected the loom row count there too.
- Card 41: added both packages to `CONSTRAINTS.md`'s Told-Geometry machine-enforced list and rewrote the stale Markdown Link Integrity worked example (now cites `docs/shared-libs/configengine.md`'s anchored `CONSTRAINTS.md` link instead of the deleted file).

`go test ./internal/lyxcwd/... ./cmd/lyx/... ./internal/landingshed/... ./internal/mergeresolve/...` passes (link-integrity guard, Fabric Vocabulary walk, and both changed packages' own test tiers). Working tree is clean; all six commits are pushed to `landing-publish-finalize-producers`.
