Batch docs-and-cross-verification — card 14 complete (Builder-authored: measurement + docs).

Deliverable: docs/benchmarks/test-suite-timing.md gained the dated 2026-06-21 block (two tiers, before/after timings, equivalence-guardrail outcome, parallel-safety note); "Reducing wall-clock" commentary updated.

Plan amended per operator decision: -race dropped from verify (no CGO/C compiler in dev env; parallelism is safe by construction via isolated per-test lyxtest copies). -race may run opportunistically in CI.

Cross-verification (the updated verify):
- go test ./... -count=1 : PASS (offline whole-repo), wall ~27.6s (was ~82s).
- go test -tags integration -count=1 ./internal/{lyxtest,weft,worktree,paths}/... : PASS (worktree 30.6s, weft 21.5s, paths 4.05s, lyxtest fast).

{"status":"success","commit_sha":"c05e26056dfeba93cc274103350d251f92d5e0cb","session_id":"40e0eb56-aa2b-41b0-b837-86a461a642d4"}
