Batch `docs-and-sandbox-preconditions` (batch 3) complete: 2 of 2 cards committed.

- Card 8 (`580673fe9`): added the "Spawned agent panes run the binary that spawned them" bullet to `crucible/README.md`'s live-driving footgun list (naming the fable-high-r2 mismatch), and extended `docs/sandbox-howto.md`'s "What the suite does" section to state that panes an agent spawns inherit binary resolution from reed, complementary to the launcher's own `.dev-bin` PATH prepend.
- Card 9 (`24c0c306d`): extended item 1 of both `tools/sandbox/SANDBOX-REED-SUITE.md` and `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md` pre-conditions with live `command -v lyx`/`LYX_BIN` checks; shuttle's item 1 also gained the third `claude`-resolution check with its own rationale sentence. `SANDBOX-REED-WATCH-SUITE.md` left untouched per the batch's scope. No `**Covers:**` tags touched.

Verify: `go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks` and `go test ./cmd/lyx/ -run TestSandboxCoverage_AllModulesCoveredOrExcluded` both pass. Working tree clean (`git status --porcelain --untracked-files=no` empty).

Files touched: `/home/hanf/Code/loomyard/wts/lyx-bin-pane-path/crucible/README.md`, `/home/hanf/Code/loomyard/wts/lyx-bin-pane-path/docs/sandbox-howto.md`, `/home/hanf/Code/loomyard/wts/lyx-bin-pane-path/tools/sandbox/SANDBOX-REED-SUITE.md`, `/home/hanf/Code/loomyard/wts/lyx-bin-pane-path/tools/sandbox/SANDBOX-SHUTTLE-SUITE.md`.

{"status":"success","commit_sha":"24c0c306d7627602bc57c9812841f3a99a6eba5a","session_id":"3269a2ce-932d-4d01-943e-dfac5c703af2","cards_done":[8,9]}
