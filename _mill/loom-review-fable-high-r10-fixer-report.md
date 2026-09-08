# loom glyph-plan-format surface — round 10 fixer report (fable-high-r10)

Companion to `_mill/loom-review-fable-high-r10.md`. One row per finding, filled in as each fix lands, never reconstructed at the end.

## Fix table

| Finding | Severity | Status | Commit | Tests added/extended | Files changed |
|---|---|---|---|---|---|
| F1 | MEDIUM | fixed | (this commit) | `TestDoneCheckVerdicts_UnreadableStatusFailsClosed` (new), `TestDoneCheckVerdicts_Rules` (+4 ambiguous rows) | `internal/planglyph/donecheck.go`, `internal/planglyph/doc.go`, `internal/planglyph/resolve.go`, `internal/planglyph/donecheck_test.go` |
| F2 | MEDIUM | pending | — | — | — |
| F3 | MEDIUM | pending | — | — | — |
| F4 | MEDIUM | pending | — | — | — |
| F5 | LOW | pending | — | — | — |
| F6 | LOW | pending | — | — | — |
| F7 | LOW | pending | — | — | — |

## Deferred (with reasons)

- OBS-1, OBS-2, OBS-3 — remain deferred/accepted per the review report's "Deferred items" section (re-evaluated this round; unchanged disposition).

## Verification

- Gate commands re-run after every fix; final full-suite run recorded at the bottom once all fixes land.
