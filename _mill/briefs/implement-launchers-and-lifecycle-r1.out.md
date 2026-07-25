All 3 of 3 cards from the batch are committed, matching their required `Commit:` messages. Card 16's roadmap.md content ended up split across two commits (f9fbf3c1, 15870158) due to a `git add` multi-pathspec error I caught and corrected via the mandatory pre-report self-check — the net diff is exactly what the card required, and `git status --porcelain` is now clean.

Summary of work:
- Card 14 (`b30194e9`): created `/home/knatte/Code/loomyard/wts/dev-test-binary/deploy-dev.cmd` and `/home/knatte/Code/loomyard/wts/dev-test-binary/deploy-dev` (mode 0755) launchers invoking `go run ./tools/deploy -dev`, and added a `.dev-bin/` line to `/home/knatte/Code/loomyard/wts/dev-test-binary/.gitignore`.
- Card 15 (`345799d2`): added the `## Dev/Prod Binary Separation` invariant section to `/home/knatte/Code/loomyard/wts/dev-test-binary/CONSTRAINTS.md`, before `## Documentation Lifecycle`.
- Card 16 (`f9fbf3c1` + `15870158`): deleted `/home/knatte/Code/loomyard/wts/dev-test-binary/manifest/designs/dev-test-binary.md` and moved the roadmap item from Planned to Done in `/home/knatte/Code/loomyard/wts/dev-test-binary/manifest/roadmap.md`, pointing at the new CONSTRAINTS.md invariant instead of the deleted doc; confirmed no dangling `designs/dev-test-binary.md` link remains in roadmap.md.

`verify: null` for this batch, so no test command was run; sanity-checked `go build ./tools/...` still succeeds with no output/errors.

{"status":"success","commit_sha":"15870158","session_id":"ad3bf4d8-a6b9-4617-afb1-fed90e8733e6"}
