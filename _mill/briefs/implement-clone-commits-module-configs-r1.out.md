All 3 cards (Card 3, Card 4, Card 5) of this batch are committed, and both halves of the batch's `verify:` command pass cleanly. No tracked modifications remain uncommitted.

Summary of work:
- `internal/fabriccli/clone.go`: `CloneAndWire` now accumulates `configengine.ConfigFileRel(r.Module)` for every applied module during the `ReconcileAll` recording loop, then commits `relPaths` via `fabricengine.CommitAnchoredPaths` on the weft primary branch with no push. Doc comments (file header and `CloneAndWire`'s own) updated to name the new commit step.
- `internal/hubforge/hub.go` / `doc.go`: added a `Mutations fabricengine.Mutations` field to `Hub`, populated verbatim from `res.Mutated()`; doc comments updated to describe the post-clone clean/committed state.
- `internal/fabriccli/cloneconfigcommit_integration_test.go` (new): 5 integration tests proving weft-prime cleanliness, pair inheritance, anchor scoping, single-commit shape, and mutation-record shape — verified via a temporary revert-and-restore TDD check (all 5 failed pre-fix, all 5 passed post-fix, `clone.go` left byte-identical).
- `internal/hubforge/seed_test.go` (new): regression test proving `SeedConfig` tolerates a byte-identical redundant seed and still applies a genuinely different one.

Commit SHAs: `c0958def` (card 3), `dcda74cc` (card 4), `22d179f7` (card 5, = HEAD).

{"status":"success","commit_sha":"22d179f773d82acf4faa2061a073df90521c8f58","session_id":"c2f8c84e-7762-4ff4-a622-9c2961b12126","cards_done":[3,4,5]}
