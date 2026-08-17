Implemented batch 3 (docs-and-invariant), 2 of 2 cards.

- Card 8 — rewrote `docs/shared-libs/configengine.md`'s Layout/Resolution-model/Key-properties/What-it-returns/LyxDirName/Load sections and added a sibling `LoadOrTemplate` section; fixed the LyxDirName-declarer claim, the FindBaseDir error text, the fictional typed-wrapper callers, the five/six-step mismatch, and the `lyx init` remedy text. Commit `73a2c0ba`.
- Card 9 — added `## Config Strictness Invariant` to `CONSTRAINTS.md` (rule, membership predicate, two pinned sets, own-loader third class, typed-absence note, T7/T10 watch item, guard blind spot, Enforced-by with the future grep guard's shape). Commit `1b46264f`.

2 of 2 cards committed, confirmed against `git log 69844817..HEAD --oneline`. `go test ./internal/lyxcwd/...` passes (`TestEnforcement_MarkdownLinks` resolves the new inline link cleanly). Working tree clean. Both commits pushed to `origin/config-template-fallback`.

{"status":"success","commit_sha":"1b46264fb008d3e1a9e7a095ddf7a0fd458e7a80","session_id":"7477a117-9533-478b-8c59-b974c9b9a398","cards_done":[8,9]}
