All 3 cards (4, 5, 6) committed, matching the batch's declared card count of 3. Verify passed. Working tree is clean.

{"status":"success","commit_sha":"ce72456bcba180e001eee87579fefe02eea301bd","session_id":"a5d9f4f2-7fb3-4369-9e3d-069bfd23b38d","cards_done":[4,5,6]}

Summary: All 3 cards of batch `02-config-strictness-guard.md` committed (3 of 3 — full completion, verified by counting commits since batch-start commit `ce5e5089`).

- `/home/knatte/Code/loomyard/wts/standalone-docs-and-invariants/cmd/lyx/configstrictness_test.go` (new) — Card 4, commit `8b9f07fd`: `TestConfigStrictness_PinnedCallSiteSets`, a pinned-set guard following `gitrepoboundary_test.go`'s style, asserting the two `configengine.Load(`/`configengine.LoadOrTemplate(` call-site package sets match `CONSTRAINTS.md`'s pinned sets exactly.
- `/home/knatte/Code/loomyard/wts/standalone-docs-and-invariants/cmd/lyx/tierpurity_test.go` — Card 5, commit `02ba9036`: added `"cmd/lyx/configstrictness_test.go"` to `allowedSpawners`.
- `/home/knatte/Code/loomyard/wts/standalone-docs-and-invariants/CONSTRAINTS.md` — Card 6, commit `ce72456b`: flipped the Config Strictness Invariant's **Enforced by** bullet from review-obligation/candidate-guard text to naming the shipped guard.

`verify: go test ./cmd/lyx/...` passed (`ok github.com/Knatte18/loomyard/cmd/lyx`). Working tree has no uncommitted tracked changes. All commits pushed to `standalone-docs-and-invariants`.

{"status":"success","commit_sha":"ce72456bcba180e001eee87579fefe02eea301bd","session_id":"a5d9f4f2-7fb3-4369-9e3d-069bfd23b38d","cards_done":[4,5,6]}
