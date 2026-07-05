No tracked modifications remain dirty. All three cards are committed cleanly on top of the batch-start commit.

{"status":"success","commit_sha":"31907f6","session_id":"010567a4-2004-4207-8258-45180d2c7a93"}

Summary of work (batch `07-docs-lifecycle`, all 3 cards committed):

- **Card 25** (`8a57295`): Deleted `docs/modules/shuttle.md` and `docs/modules/mux.md`; retargeted every inbound link across `docs/modules/loom.md`, `docs/modules/review.md`, `docs/modules/README.md`, `docs/research/mux-exploration.md`, `docs/research/mux-hooks-exploration.md`, `docs/research/mux-proposal.md`, `docs/reviews/README.md`, `docs/reviews/mux-review-prompt.md` — module-level mentions now point at `docs/overview.md#modules`/`#execution-stack-orchestration-layers`, deep behavior mentions point at the `internal/muxengine` / `internal/shuttleengine` (+`claudeengine`) package documentation.
- **Card 26** (`49d7a60`): Added a `shuttle` entry to `docs/overview.md`'s module table (between mux and loom) and marked shuttle ✅ in the execution-stack diagram; flipped roadmap.md milestone 10 to Done, updated the build-order spine and "immediate front" paragraph (now names `review`), and repointed the remaining `modules/mux.md` links in roadmap.md's milestone 9/13/14/18 at the `internal/muxengine` package docs.
- **Card 27** (`31907f6`): Added the "Shuttle Provider-Seam Invariant" section to `CONSTRAINTS.md`, and fixed its pre-existing dangling `docs/modules/mux.md` reference (in the CLI/Cobra Invariant's attach-exception note) to point at the `internal/muxcli` attach command's godoc/`Long` and `docs/overview.md#modules`.

Verified zero dangling `mux.md`/`shuttle.md` references anywhere in the repo outside `_mill/` after all edits. `verify: null` for this batch (pure documentation batch), so no test command was run.
