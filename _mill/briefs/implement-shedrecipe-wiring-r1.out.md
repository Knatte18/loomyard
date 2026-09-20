All 6 cards for this batch are complete and committed, both verify sub-commands pass, `go vet ./...` is clean, and the working tree has no uncommitted tracked changes (only the untracked brief file remains, which is out of scope).

Summary of work:
- `/home/knatte/Code/loomyard/wts/producer-gates/internal/shedrecipe/entries_gate.go` (new) — `resolveGateSpec`, the shared "gate"/"gate_attempts" Config resolver.
- `/home/knatte/Code/loomyard/wts/producer-gates/internal/shedrecipe/entries_discussionwrite.go`, `entries_planwrite.go` — wired through `shedadapters.NewSingleLLMProducerGated`.
- `/home/knatte/Code/loomyard/wts/producer-gates/internal/shedrecipe/entries_burler.go` — `RunOpts.Gate` wired via `resolveGateSpec`.
- `/home/knatte/Code/loomyard/wts/producer-gates/contracts/recipes/loom-recipe.yaml` — the four gated rows (Discussion-Write, Plan-Write, Discussion-Burler, Plan-Burler) declare `gate`/`gate_attempts`; Webster-Burler documented as deliberately ungated.
- `/home/knatte/Code/loomyard/wts/producer-gates/internal/shedrecipe/entries_discussionwrite_test.go`, `entries_planwrite_test.go`, `entries_burler_test.go` — gate/gate_attempts coverage across all three entries.
- `/home/knatte/Code/loomyard/wts/producer-gates/internal/loomcli/smoke_gate_test.go` (new) — the live-substrate gate re-prompt smoke test.
- `/home/knatte/Code/loomyard/wts/producer-gates/internal/loomrecipe/resume_test.go` — `TestBounceRouting_StuckContinuesAtDeclaredTarget` and `TestBounceRouting_BudgetExhaustionBlocks` adapted for Discussion-Write's own new gate (this fallout was discovered mid-batch, so I stopped, added `internal/loomrecipe/resume_test.go` to Card 25's `Edits:` list in `_mill/plan/04-shedrecipe-wiring.md`, committed that plan edit first, then made the code fix).
- `/home/knatte/Code/loomyard/wts/producer-gates/_mill/plan/04-shedrecipe-wiring.md` — plan edit recording the above.

Card-count self-check: 6 of 6 cards committed (cards 22–27), all verified against `git log 1a01d9d8016b561a82139cc20df6dd26acaad662..HEAD --oneline`, matching each card's declared `Commit:` message. All complete.

Note: a broader, non-required `go test ./...` sweep surfaced three pre-existing failures outside this batch's scope and verify command — `cmd/lyx`'s hermetic/tier-purity checks flagging `internal/shuttleengine/gate_test.go` (from batch 1's commit `40693044a`), and `internal/lyxcwd`'s fabric-vocabulary scan flagging `internal/loomshed/discussionwrite.go`, `planwrite.go`, and `internal/shedadapters/singlellm.go` (from batch 2's commits). Confirmed via `git log main..HEAD -- <path>` that all of these predate batch 4 and are outside this batch's declared file scope, so I left them untouched.

{"status":"success","commit_sha":"373aef1d198f9931879d36b45a5ac99b3b48a14c","session_id":"5e81325f-34eb-4a05-a64b-73f2cdc23a71","cards_done":[22,23,24,25,26,27]}
