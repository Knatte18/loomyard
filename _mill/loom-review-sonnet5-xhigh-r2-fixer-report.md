# `loom` crucible fixer report — glyph-hardening campaign, ROUND 2 (sonnet5-xhigh)

Companion to [`loom-review-sonnet5-xhigh-r2.md`](loom-review-sonnet5-xhigh-r2.md), which carries the findings
themselves, the live-run evidence, and the per-mechanism verdicts.

## Summary

7 findings: **all 7 fixed**, 0 deferred, 0 NOT-FIXED-THIS-ROUND. Every fix landed as its own commit, on the
current branch, verified green before the next was started. Nothing was pushed.

Unlike round 1, none of this round's findings were BLOCKING and none of them were observed to actually break,
corrupt, or mislead the live run this round drove — the headline result is that round 1's 20 fixes hold under
real contact with the real orchestrator. What this round found and fixed is residual doc/stencil drift the
glyph-alphabet doc pass missed, one genuine (but narrow) planglyph documentation gap surfaced by the live
run's own review round nearly mis-firing over it, one real Master-prompt coverage gap for an error shape round
1 introduced, and one narrow format-checker gap.

## What was implemented

Commits, in order, each self-contained and green at the point it landed:

| Commit | Finding | What changed |
|---|---|---|
| `06977e1ca` | F-doc1 | `loom-rubric-plan-review.md`: corrected the stale "sixteen"/"seventeen" mechanical-check count to "twenty-six"/"twenty-seven" (three occurrences), matching the current 27-check `loom-plan-spec.md`. |
| `7e18c5901` | F-doc3 | Same file: restored "symbol" wording in the Granularity bullet (three occurrences), matching `manifest/designs/loom.md`'s own text — a general card-shape principle that must not imply the glyph alphabet is active, since it also governs `language: none` plans. |
| `f2c3b4761` | F-doc2 | `contracts/recipes/loom-recipe.yaml`: corrected the same stale "sixteen" count in Plan-Burler's own embedded `fasit.instructions` field, a second live surface carrying the identical wrong numeral. |
| `c5cc083fb` | F-parse1 | `internal/planparser/validate.go`'s `checkRenamePairShape`: `rename-from-not-glyph` now also fires when a Rename pair's `Old` side is handle-shaped (`refKindHandle`), not only bare-symbol-shaped, with a message naming which wrong shape it saw. Previously a handle-shaped Old side slipped past this free, pre-quarry check and was only caught one layer later by `planglyph`'s `rename-old-unresolved`, with a less precise message. Updated `loom-plan-spec.md` check 17's own description to match. |
| `9cc70c595` | F-plan1 | `loom-plan-spec.md`'s "Plan: handles" section: added one paragraph stating explicitly that a Rename's `Old` side can never be a symbol the same plan creates, since `Plan-Validate`/`Plan-Revalidate` resolve the whole plan against the pre-execution tree — the real constraint this round's live `Plan-Write` session independently discovered and worked around, but nothing previously documented up front. |
| `2e4e32859` | F-plan2 | `internal/planglyph/doc.go`: added a canonical, enumerated list of every resolve-backed `Finding.Check` ID this package can raise (~15 IDs across `resolve.go`/`create.go`/`containment.go`/`handle.go`/`drift.go`/`scope.go`/`donecheck.go`), the parallel `planparser`'s 27 checks already have in `loom-plan-spec.md`. Added a cross-reference from `quarry-glyph-plan-alphabet.md`'s Related section. Placed in the package's own doc comment, per the Documentation Lifecycle (as-built detail lives beside the code, not in the landed `manifest/designs/` pointer doc), not in the design doc itself. |
| `0fcbfee98` | F-webster1 | `contracts/stencils/webster/webster-template-master.md`: added a new "A plan-drift refusal ends your run as stuck" section, modeled on the existing fabric-sync-error section, giving Master a scripted response to `begin-batch`'s `{"plan_drifted": true}` refusal — a structurally distinct outcome round 1's own F5-F7/F21 fix introduced with no matching prompt update. |

## Tests added or extended

Every code-shaped fix carries a test that fails on the unfixed code, verified by stashing the production
change and re-running (both sabotage-proofs shown below, not just claimed):

| Test | Package | Covers | Sabotage-proof |
|---|---|---|---|
| `TestValidate_RenamePairShape/symbol_rename_whose_old_side_is_a_plan:_handle_produces_rename-from-not-glyph` (new subtest) | `internal/planparser` | F-parse1 | Stashed `validate.go`'s fix alone: `countFor(findings, rename-from-not-glyph) = 0; want 1` — fails exactly as expected, restored, empty diff. |
| `TestMasterTemplate_StatesPlanDriftRefusalEndsRunAsStuck` (new) | `internal/websterengine` | F-webster1 | Stashed the stencil's new section alone: both `requireContains` assertions fail naming the missing text, restored, empty diff. |

The five pure-doc/stencil-text findings (F-doc1, F-doc2, F-doc3, F-plan1, F-plan2) needed no new test: none of
them changed program behavior, only prose an LLM reads or a human reads. Each was verified instead by:
re-running `go build`/`go vet`/`go test` over every touched package (all green — a stale numeral or word choice
in a markdown/YAML string cannot break Go compilation, but the surrounding files still needed to parse and the
stencil/recipe-loading tests still needed to pass), and, for the two doc edits carrying a new or existing
markdown link (`quarry-glyph-plan-alphabet.md`'s new bullet, `loom-plan-spec.md`'s existing cross-references),
`internal/lyxcwd`'s `TestEnforcement_MarkdownLinks` (its own `repo` subtest scans every `.md` file under
`manifest/`/`docs/` for real) — run and confirmed green after each of those edits.

## Deliberately NOT fixed

None. All 7 findings were fixed this round.

## Exact commands run, and results

- `CGO_ENABLED=1 go build ./...` — clean, run after every individual fix.
- `CGO_ENABLED=1 go vet` over every touched package, run after every individual fix — clean throughout.
- Per-fix targeted `go test` runs (see the table above) — all green, plus each fix's own sabotage-proof.
- Full hermetic suite, count=5, after all seven fixes landed:
  `CGO_ENABLED=1 go test -count=5 ./internal/loomengine/... ./internal/loomcli/... ./internal/loomrecipe/...
  ./internal/loomshed/... ./internal/shedengine/... ./internal/shedadapters/... ./internal/shedrecipe/...
  ./internal/shedbuild/... ./internal/hubgeom/... ./internal/planparser/... ./internal/planglyph/...
  ./cmd/lyx/...` — 12 packages `ok`, 0 FAIL.
- Full integration suite after all seven fixes:
  `CGO_ENABLED=1 go test -tags integration ./internal/planglyph/... ./internal/planparser/...
  ./internal/websterengine/... ./internal/loomcli/... ./internal/loomshed/...` — 5 packages `ok`, 0 FAIL.
- Whole-repo safety net: `CGO_ENABLED=1 go test ./...` — 81 packages `ok`, 0 FAIL, 0 panic.
- `internal/lyxcwd`'s `TestEnforcement_MarkdownLinks` — green after both markdown-link-bearing doc edits.

## Live re-verification — deliberately not re-run in full, with reasoning stated

None of the 7 fixes changed a code path this round's own live `lyx loom run` exercised differently than it
already did: the five doc/stencil-text findings have zero effect on program behavior (they are prose an LLM
or a human reads, not data any Go code parses or asserts on beyond the tests added above), and F-parse1's
widened `rename-from-not-glyph` only fires on a handle-shaped Rename `Old` side — a shape the live run's own
plan never carried (its real `Plan-Write` session correctly avoided producing a `Rename` card at all, per
F-plan1). F-webster1's new Master-prompt section is inert unless `begin-batch` actually returns
`plan_drifted: true`, which did not occur live this round either. Re-running the full ~30-45-minute live
pipeline again would therefore re-confirm the exact same clean pass the review report already recorded, not
exercise anything the fixes changed. Judged disproportionate given the round's live-substrate cost
declaration and the fact that this round already produced one complete, successful live pass through
`Webster-Review` before any of these findings were even found.

## Changed files

Production/contract:

- `contracts/stencils/loom/loom-rubric-plan-review.md` — F-doc1, F-doc3 (two separate commits, same file)
- `contracts/recipes/loom-recipe.yaml` — F-doc2
- `internal/planparser/validate.go` — F-parse1 (`checkRenamePairShape`)
- `contracts/specs/loom-plan-spec.md` — F-parse1 (check 17 description), F-plan1 (new "Plan: handles" paragraph)
- `internal/planglyph/doc.go` — F-plan2 (canonical check-ID list)
- `manifest/designs/quarry-glyph-plan-alphabet.md` — F-plan2 (Related-section cross-reference)
- `contracts/stencils/webster/webster-template-master.md` — F-webster1 (new plan-drift section)

Tests:

- `internal/planparser/validate_test.go` — new subtest for F-parse1
- `internal/websterengine/template_test.go` — new test for F-webster1

Review record (this campaign's own durable artifacts, committed incrementally throughout):

- `_mill/loom-review-sonnet5-xhigh-r2.md`
- `_mill/loom-review-sonnet5-xhigh-r2-fixer-report.md` (this file)

`manifest/roadmap.md` was not touched — this is a hardening pass, and the roadmap moves only on completing or
adding a planned item. No new cross-cutting invariant was introduced, so `CONSTRAINTS.md` is unchanged.
`docs/overview.md` was not touched — no module table or execution-stack change.

## Live-substrate teardown

The sandbox hub's tmux session (`lyx-lyx-test-HUB-d919e29a`) was torn down (`tmux ... kill-server`) once Job 2
no longer needed it. Confirmed via `ps aux` afterward: no `lyx loom drive`, no `claude` process with a cwd
under `/home/knatte/Code/lyx-test-HUB`, no tmux server on that socket. The two processes remaining in a
broad `ps aux | grep -iE 'tmux|lyx|claude'` scan (a bare `tmux` server dated `sep.02`, and mill's own
`wiki._server`) both predate this round and are unrelated to it.

**Operator follow-up still open, unrelated to this repo's own git state:** PR #1 on
`github.com/Knatte18/lyx-test` (`glyph-demo-greet` -> `main`) remains open, awaiting a human merge/close
decision — see the review report's "How far the real run got" section for the full context. This is a real
GitHub artifact in the disposable sandbox hub, not something in this worktree's own commit history.

## Merge-readiness

**Merge-ready.** All 7 findings fixed, all severities including NIT, all gates green cold, both code-shaped
fixes independently sabotage-proofed. Round 1's 20 fixes are confirmed to hold under a real `lyx loom run`,
hub mode, real LLM sessions throughout, a plan authored by a real `Plan-Write` session and carried through
`Webster-Review` with zero glyph-surface defects blocking it anywhere — the residual this campaign's own
handoff note named as round 2's mission is closed.
