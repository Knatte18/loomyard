# reed + shuttle crucible campaign — orchestrator handoff

Campaign: manual crucible review+fix loop on `internal/reedengine` (+ `internal/reedcli` + new `internal/hubgeom`) and `internal/shuttleengine` (+ `internal/shuttlecli`), bundled per the mill wiki task because shuttle drives its LLM agent runs through reed's tmux substrate. Motivated by wave-1 — three commits, `2b21ee57` (T1), `5b096ebd` (T2), `b98ee2ba` (T3, "shuttleengine + reedengine + tokenvocab told-geometry") — the last of which changed `reedengine.New`'s signature from `(cfg, *lyxcwd.Location)` to `(cfg, Geometry)`, moved `HubLogsDir` from `reedengine` to `fabricengine`, and introduced `internal/hubgeom` as the one-way told-geometry adapter. Must land before wave 3 (T6/T7). Orchestrator-driven, not mill-go — see `crucible/README.md` / `crucible/orchestrator-prompt.md`.

## Campaign structure (operator's call, 2026-08-18)
- **Reed first, standalone campaign** (running now). Shuttle is a **separate, later campaign** once reed converges — not interleaved.
- Round rotation for reed's campaign (operator-specified): **R1 Opus/Medium → R2 Opus/High → R3 Fable/High → R4 Opus/Medium**, used only as far as needed to converge (a safety pass may end the campaign earlier than R4; do not force all four if convergence is reached sooner).
- Shuttle's own campaign (not yet started) gets its own round-prompt file and its own rotation when reed converges — ask the operator for shuttle's rotation at that point.
- **Operator-mandated cost rule for shuttle specifically: any real `claude` process a round agent spawns while live-driving shuttle (every `lyx shuttle run` invocation) MUST use `--model haiku` (or the cheapest available), unless a specific finding genuinely requires testing model-specific behavior.** Already applied to reed's one claude-spawning test in round 1 (see F3 below).

## Wave-1 attribution (established during round 1 verification, 2026-08-18)
Wave-1 is three commits, not one: `2b21ee57` (T1), `5b096ebd` (T2), `b98ee2ba` (T3). Per-finding attribution, via `git log -S` on the exact code each finding touched:
- **T1 (`2b21ee57`)** touches no reed-related file at all (loomengine/planparser/webster only). Zero findings trace to it.
- **T2 (`5b096ebd`)** touched `internal/reedengine/config.go` (`LoadConfig` → `configengine.LoadOrTemplate`). This is the source of ONE of F5's three stale rows (the `reedengine.LoadConfig` "blocks standalone: yes" row in `producers-standalone.md`).
- **T3 (`b98ee2ba`)** is the source of F4, F6, F7, F8, F9, F10 (all LOW/NIT doc-and-dead-code fallout from the `Layout`→`Geometry` rename) and two of F5's three rows.
- **F1 (BLOCKING) and F3 (MEDIUM) are NOT wave-1-caused at all** — both trace via `git log -S` to `93ad5b01` ("mux -> reed rename"), reed's earliest history. They are old, previously-uncaught bugs the campaign happened to surface, not regressions wave-1 introduced.
- **F2 (MEDIUM) predates wave-1 too**, but wave-1's `geometry.go` doc comment turned an implicit, accidentally-correct derivation into an explicit contract ("AnchorPath is... the cwd every pane is spawned with") the code did not honor — wave-1 is what promoted a latent bug into a documented contract violation, even though it didn't write the buggy line itself.
- **Net: only 7 of round 1's 10 findings are wave-1-caused, and all 7 are LOW/NIT.** The one BLOCKING finding and one of the two MEDIUM findings are unrelated to the commit that motivated this campaign. Useful context for calibrating round 2+, not a reason to narrow scope — a safety pass should still look everywhere, per the prompt's own instruction.

## State right now
- **Round 1 (`opus-medium-r1`) — CLOSED AND INDEPENDENTLY VERIFIED.** 10 findings (1 BLOCKING, 2 MEDIUM, 2 LOW, 5 NIT), all fixed, none deferred. See "Round context seeded from prior-round verification" in `_mill/reed-review-prompt.md` (current version, round 2) for the full per-finding commit-sha list and the orchestrator's independent verification summary (cold-state gates, 3× concurrent smoke, sabotage-proof of both new regression tests, doc-fix spot-checks). Round 1's own prompt version is preserved in git history at commit `c0569063`; its review/fixer reports are `_mill/reed-review-r1.md` / `_mill/reed-review-r1-fixer-report.md` (both committed).
- Round running: **R2, reed, Opus / High**, tag `opus-high-r2` — a safety pass per the rotation (round 1 alone is never treated as convergence per the method; see `crucible/README.md`'s worked examples).
- Seed prompt: `_mill/reed-review-prompt.md` (rewritten fresh for round 2, committed).
- RESIDUAL currently seeded: none — safety pass, look for anything round 1 missed, especially the same shape as F1 (a liveness-check hazard elsewhere) or F2 (a missing `-c` at another spawn site).
- DEFERRED list: empty.
- Shuttle campaign: not started. Do not begin it until reed's campaign has converged (a safety pass + the orchestrator's independent gates + — since reed is a live-substrate module — ideally an operator-assisted visual check all agree).

## Next action
Wait for R2 (`opus-high-r2`) to complete, then the orchestrator independently verifies from a cold state (same protocol as round 1: rerun every gate, sabotage-prove every new regression test, spot-check doc fixes) before deciding whether reed has converged or needs R3 (Fable/High per the rotation). Never trust R2's own merge-readiness verdict.
