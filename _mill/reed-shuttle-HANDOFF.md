# reed + shuttle crucible campaign — orchestrator handoff

Campaign: manual crucible review+fix loop on `internal/reedengine` (+ `internal/reedcli` + new `internal/hubgeom`) and `internal/shuttleengine` (+ `internal/shuttlecli`), bundled per the mill wiki task because shuttle drives its LLM agent runs through reed's tmux substrate. Motivated by wave-1 (commit `b98ee2ba`, "shuttleengine + reedengine + tokenvocab told-geometry"): changed `reedengine.New`'s signature from `(cfg, *lyxcwd.Location)` to `(cfg, Geometry)`, moved `HubLogsDir` from `reedengine` to `fabricengine`, introduced `internal/hubgeom` as the one-way told-geometry adapter. Must land before wave 3 (T6/T7). Orchestrator-driven, not mill-go — see `crucible/README.md` / `crucible/orchestrator-prompt.md`.

## Campaign structure (operator's call, 2026-08-18)
- **Reed first, standalone campaign** (this is what's running now). Shuttle is a **separate, later campaign** once reed converges — not interleaved, because shuttle builds on reed's current shape and a residual in reed would contaminate shuttle's review.
- Round rotation for reed's campaign (operator-specified): **R1 Opus/Medium → R2 Opus/High → R3 Fable/High → R4 Opus/Medium**, used only as far as needed to converge (a safety pass may end the campaign earlier than R4; do not force all four if R1–R3 already converge).
- Shuttle's own campaign (not yet started) will get its own round-prompt file and its own rotation when reed converges — ask the operator for shuttle's rotation at that point; do not assume it reuses reed's tags.
- **Operator-mandated cost rule for shuttle specifically: any real `claude` process a round agent spawns while live-driving shuttle (i.e. every `lyx shuttle run` invocation) MUST use `--model haiku` (or the cheapest available), unless a specific finding genuinely requires testing model-specific behavior.** This also applies to reed's one claude-spawning test (`TestSmokeClaudeResumeRecallsCodeword`) — already encoded in `_mill/reed-review-prompt.md`'s Live-substrate cost declaration.

## State right now
- Round running: **R1, reed, Opus / Medium**, tag `opus-medium-r1`.
- Seed prompt: `_mill/reed-review-prompt.md` (committed).
- CLOSED-AND-VERIFIED: nothing yet — first round.
- RESIDUAL currently seeded: none — full clean-room review, focused on the told-geometry `Geometry` field-integrity invariant (see the prompt's High-yield focus list).
- DEFERRED list: empty.
- Shuttle campaign: not started. Do not begin it until reed's campaign has converged (safety pass + orchestrator's independent gates agree).

## Next action
Wait for R1 (`opus-medium-r1`) to complete, then the orchestrator independently verifies from a cold state (see `crucible/README.md`'s verification protocol) before re-seeding or spawning R2. Never trust R1's own merge-readiness verdict.
