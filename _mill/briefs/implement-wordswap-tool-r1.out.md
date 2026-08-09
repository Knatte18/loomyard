All 5 of 5 cards committed, verify passes (`go test ./tools/wordswap/...` is green), and the working tree is clean with no uncommitted tracked changes.

{"status":"success","commit_sha":"3be81471cb02275fdea17b29e997ccd92b022e3c","session_id":"f52a7dc3-205e-421d-9030-85971e638e0b","cards_done":[1,2,3,4,5]}

Summary: 5 of 5 cards committed this turn. Built `tools/wordswap/` (a new package under `tools/`) implementing a case-preserving, whole-token, camel/snake-boundary-aware word-substitution tool with a reversibility safety invariant and two report buckets (AMBIGUOUS/SKIPPED). Key files:

- `/home/knatte/Code/loomyard/wts/fabric-host-to-warp-rename/tools/wordswap/main.go` — CLI flags (`-from`, `-to`, `-dry-run`, repeatable `-skip`), `processFile`, and the driving loop/report printer.
- `/home/knatte/Code/loomyard/wts/fabric-host-to-warp-rename/tools/wordswap/swap.go` — `caseForm`, `applyCase`, `boundaryBefore`/`boundaryAfter`, `swapText`, `span`/`revertSpansImpl`/`revertSpans`.
- `/home/knatte/Code/loomyard/wts/fabric-host-to-warp-rename/tools/wordswap/swap_test.go` — the full table-driven test suite written in card 2, before `swap.go` existed.

One deviation worth flagging for review: while implementing, I found the plan's card-2 API description was ambiguous about what `Occurrence.Text` should hold (the matched token vs. the full line), and resolved it in favor of "full line text" to match card 5's explicit `<path>:<line>: <line text>` report format — this only affects report-string content, not any pass/fail behavior, and all of card 2's pinned assertions still pass under this reading since its test inputs are single-line.
