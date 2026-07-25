All 5 cards from the batch file's `## Cards` section are committed, matching the declared `Commit:` messages exactly. Final commit sha: `93e5457f`.

{"status":"success","commit_sha":"93e5457f","session_id":"d900e741-9cf6-4e54-b424-0f514235fe8d","cards_done":[1,2,3,4,5]}

## Summary

All 5 of 5 cards in batch `01-planparser-core` were committed, and the batch's verify command (`go test ./internal/planparser/...`) passes cleanly. Card-count self-check: compared `git log 34c2c346..HEAD --oneline` against the batch file's `### Card N:` `Commit:` messages — all 5 match exactly, one commit per card.

Files created:
- `/home/knatte/Code/loomyard/wts/webster-rewrite/internal/planparser/doc.go` — package doc (sole `_lyx/plan/` reader, type model summary)
- `/home/knatte/Code/loomyard/wts/webster-rewrite/internal/planparser/plan.go` — `Plan`, `Card`, `MovePair` struct model
- `/home/knatte/Code/loomyard/wts/webster-rewrite/internal/planparser/parse.go` — `ParsePlan`, overview/Card-Index parsing, per-card body parsing (typed file-op fields, Depends-on, Commit, verify:)
- `/home/knatte/Code/loomyard/wts/webster-rewrite/internal/planparser/normalize.go` — `normalizeCardPath` (the `root:`/`//` three-case rule), `normalizeCard`
- `/home/knatte/Code/loomyard/wts/webster-rewrite/internal/planparser/sections.go` — extraction of `## Shared Decisions` / `## Rename mechanic` / `## verify:` into `Plan`
- `/home/knatte/Code/loomyard/wts/webster-rewrite/internal/planparser/parse_test.go`, `normalize_test.go`, `sections_test.go` — Tier-1 table-driven tests
- `/home/knatte/Code/loomyard/wts/webster-rewrite/internal/planparser/testdata/goodplan/*.md` — golden fixture, verified byte-identical to `docs/reference/plan-format-v3.md`'s Worked example via a scripted diff

One deliberate design decision worth flagging for reviewers: `ParsePlan` does **not** fail loud on an absent `format:`/`approved:` key in the overview frontmatter (unlike the frozen v2 parser) — those default to their Go zero values (`0`/`false`) and are left for batch 2's `Validate`'s `format-unrecognized`/`plan-unapproved` checks, per the v3 spec's explicit split of those checks into the validator. Only a missing/undecodable frontmatter block (not individual absent keys) is a `ParsePlan` failure. This is documented in `parseOverviewFrontmatter`'s doc comment and covered by `TestParsePlan_Overview_MissingFormatOrApprovedIsNotFailLoud`.
