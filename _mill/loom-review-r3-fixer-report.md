# `loom` review round 3 — fixer report

> Fixes applied for `_mill/loom-review-r3.md`'s findings.
> Agent: `sonnet5-xhigh-r3` (crucible-reviewer-xhigh, model Sonnet 5).

## Findings fixed

### F-R3-1 (BLOCKING) — `Publish` can never detect its own merged PR

**Root cause:** `internal/landingshed/publish.go`'s `Call` resolved the existing PR via
`client.PullRequests.List(...)` (GitHub's List Pull Requests REST endpoint) and then branched on
`pr.GetMerged()`. That endpoint never populates the `merged` boolean field on any item it returns —
only the single-PR Get endpoint does — so `pr.GetMerged()` was unconditionally `false` regardless of
the PR's real state. Confirmed live against a real GitHub repository (see the review report's
"Live driving" section for the exact `gh api` output proving the field-shape mismatch, and the exact
`lyx loom step` repro against a genuinely merged PR that misreported "the pull request was closed
without being merged").

**Fix:** `internal/landingshed/publish.go` — changed the switch's merged-PR case from
`case pr.GetMerged():` to `case !pr.GetMergedAt().IsZero():`. `merged_at` IS populated by the List
endpoint (confirmed in the same live `gh api` call), so this is the correct signal to read from a
List response. Added a code comment at the call site explaining the root cause and pointing at this
round's finding, so a future reader does not reintroduce `pr.GetMerged()` here.

**Test added/extended:**
- `internal/landingshed/publish_test.go`'s `TestPublish_ClosedAndMergedPR_Done` — the mock's scripted
  List response previously hand-set `"merged":true`, a JSON shape the real API never produces from
  this endpoint; this is what let the bug ship undetected through two prior crucible rounds. Changed
  the mock to the REAL List-endpoint shape: no `"merged"` key at all, only `"merged_at"` set to a
  real timestamp. Added a doc comment on the test explaining why this exact shape matters.
- `TestPublish_ClosedAndUnmergedPR_StuckDistinctFromOpen` and `TestPublish_OpenPR_StuckNoCreate` —
  updated their mocks the same way (dropped the unrealistic `"merged":false` in favor of no key at
  all, matching what List actually returns for a PR that either never merged or is still open), so
  the whole file's mocks consistently reflect the real API contract rather than the code's own
  assumption about it.
- **Sabotage-proved**: reverted just the `publish.go` one-line fix (via `git stash push` on that file
  alone, keeping the corrected test in place) and re-ran `TestPublish_ClosedAndMergedPR_Done` — it
  now correctly FAILS (`Call() outcome = "stuck"; want "done"`), proving the new test would have
  caught the original bug. Restored the fix (`git stash pop`) and re-ran to confirm green again.

**Docs:** no design doc (`manifest/designs/loom-step.md` / `self-report-tier1.md` /
`self-report-tier2.md` / `loom.md`) makes any claim about the GitHub List-vs-Get field shape this bug
turned on, so none needed updating — this was purely an implementation/test defect, not a
documented-but-wrong behavior. `CONSTRAINTS.md` carries no invariant this touches either.

**Live re-verification (the specific ask this round's mission named):**
1. Redeployed the fixed binary (`./deploy-dev`).
2. Rolled the SAME disposable fixture task (`greet-lib`, the same real merged PR
   `Knatte18/lyx-crucible-r3#1` I opened and merged for real earlier in this round, still merged on
   GitHub throughout) back to `current_producer: "Publish"`, `state: "running"` in its own
   `_lyx/loom/status.json` — a targeted rollback of the run's OWN orchestration pointer, not a
   fabrication of the PR's real, already-merged state on GitHub, which never changed — and restored
   `landing.yaml`'s `require_pr_to_base` to its shipped default `["main"]` for this task (I had
   relaxed it earlier, before the fix existed, specifically so I could keep exercising `Finalize`/
   `RunDone` without being blocked by this exact bug — see the review report's "Live driving" section
   for that disclosed methodological note).
3. Re-invoked `lyx loom step`: `{"outcome":"done", "next":"Finalize", ...}` — `Publish` now correctly
   recognizes the real merged PR and advances. A further `step` call ran `Finalize` again cleanly to a
   terminal `done`.

This closes the round-context's "genuinely open" item 1 (`Publish`'s merged-PR resume) with the bug
found, fixed, and the fix independently re-verified against the exact real external state that
exposed it in the first place — not merely against a corrected mock.

## Findings deliberately deferred

None. The one finding this round produced (F-R3-1) is fully fixed and verified.

## Test commands run + results

- `go build ./...` — clean, both before and after the fix.
- `go vet ./...` — clean.
- `go test ./internal/landingshed/...` — all tests pass, including the two corrected mocks and the
  sabotage-proof cycle described above.
- `go test ./...` (whole repo) — all packages `ok` or `[no test files]`, no regressions.
- `go test -tags smoke ./internal/loomcli/... -run Smoke -v -count=1` — all 13 functions pass
  (providerless/hermetic per the cost declaration; unaffected by this fix, run as a regression check).
- Live: real `lyx loom step` invocations against a disposable fixture hub (two fresh private GitHub
  repos, `Knatte18/lyx-crucible-r3` + `-weft`, built via `lyx fabric clone`, never the operator's
  `lyx-test-HUB`), driving a real dummy task (`greet-lib`) through the WHOLE real phase machine with
  cheap models (`sonnet[effort=low]` for discussion/plan/review/webster, `haiku[effort=low]` for
  friction, `selfreport: false` throughout) — see the review report's "Live driving" section for the
  full walk, the two interrupted-and-resumed repros, the RunDone friction reflection confirmation, and
  the exact `gh api`/`lyx loom step` transcripts proving F-R3-1 and its fix.

## Changed files

- `internal/landingshed/publish.go` — the one-line logic fix plus an explanatory comment.
- `internal/landingshed/publish_test.go` — corrected three mocked List-response bodies to the real
  API shape and added an explanatory doc comment on `TestPublish_ClosedAndMergedPR_Done`.
- `_mill/loom-review-r3.md` — this round's review report (committed incrementally throughout).
- `_mill/loom-review-r3-fixer-report.md` — this file.

## Teardown

Disposable fixture hub (`r3fixture/` under this session's scratchpad), the two disposable GitHub
repos (`Knatte18/lyx-crucible-r3`, `Knatte18/lyx-crucible-r3-weft`), and every tmux/reed/driver
process this round started are torn down after this report is committed — see the final chat message
for the exact teardown confirmation. The operator's own `lyx-test-HUB` and `~/go/bin/lyx` were never
touched.
