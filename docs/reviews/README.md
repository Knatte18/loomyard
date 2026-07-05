# Serial review+fix loop — a reusable hardening method

This directory holds the **manual, human-in-the-loop review method** we used to harden `mux`
before merging it to `main`, plus the two prompts that drove it. The method is
**module-agnostic** — it is written down here so the modules built *on top of* mux
([`shuttle`](../modules/shuttle.md), [`review`](../modules/review.md), [`loom`](../modules/loom.md))
can reuse it instead of re-inventing it each time.

**The files here:**
- [`orchestrator-prompt.md`](orchestrator-prompt.md) — paste-ready prompt that bootstraps a thread
  into the **orchestrator** role (drives the loop, spawns rounds, independently verifies).
- [`review-prompt-template.md`](review-prompt-template.md) — module-agnostic skeleton for the
  **round agent** prompt (the reviewer-fixer a round spawns).
- [`mux-review-prompt.md`](mux-review-prompt.md) — the fully-worked `mux` instance of that template.
- This README — the method itself (roles, loop, verification protocol) explained in prose.

> **This is the hand-executed prototype of the [`review`](../modules/review.md) module.** The
> automated gate engine described there — a fresh Handler per round that does **A: review** then
> **B: fix**, with **no self-grading** and an **independent** progress check — is exactly this loop
> with the orchestrator role moved from a human+Claude pair into Go. Until `review` lands, this is
> how we run it by hand; when it lands, this doc is the reference the engine was modeled on. If you
> change the method here, reconcile it with `modules/review.md`.

## When to use it

Reach for this before merging a **live-substrate module** — one whose real defects hide in composed,
stateful, timing-sensitive behavior that a green `go test` does **not** prove (mux driving real
psmux is the archetype; anything driving real processes, sockets, or an external tool qualifies).
For pure/logic modules a normal PR review is enough. The tell that you need this loop: *"the unit
tests pass but I don't trust it under load / crash / concurrency."*

## The two roles

- **Orchestrator** (a human operator + a steering Claude, i.e. *you* reading this). Owns the loop:
  seeds the prompt, spawns each round, **independently verifies** the round's work, re-seeds, rotates
  the model, and decides when it has converged. The orchestrator does **not** edit the module code
  during a round — it stays off the worktree so it never collides with the round agent.
- **Round agent** — a fresh, **clean-room** sub-agent spawned per round (a `general-purpose` Agent,
  *not* a fork — a fork would inherit the orchestrator's context and destroy independence). It does
  two jobs in order: **A — review** (form its own findings by reading the code *and* driving the
  real substrate), then **B — fix** (implement, test, update docs). One agent does both because the
  review context is already loaded, so the fix is cheap.

## The loop

```
        ┌─────────────────────────────────────────────────────────────┐
        │  1. SEED the prompt with the current known state             │
        │  2. SPAWN a fresh clean-room round agent (rotate the model)  │
        │        A — review (independent findings, drive real substrate)│
        │        B — fix (implement + test + docs, do NOT commit)       │
        │  3. ORCHESTRATOR independently VERIFIES (never trust the      │
        │        round's own "merge-ready" verdict)                     │
        │  4. COMMIT the round's work; RE-SEED with what verification   │
        │        found; go to 2 with the next model                    │
        └───────────────────────── until converged ───────────────────┘
```

1. **Seed.** The prompt (`<module>-review-prompt.md`, instantiated from
   [`review-prompt-template.md`](review-prompt-template.md)) carries a *"round context seeded from
   prior-round verification"* section. Each round rewrites it with the residual the last
   verification found — or, once clean, flips it to a **safety pass** ("no known residual; confirm
   merge-readiness or find what every prior round missed").
2. **Spawn.** One fresh `general-purpose` Agent with a `model:` override, told **only** to read the
   prompt file and do exactly what it says, tagged `<model>-r<N>`, told **not** to commit. It writes
   two deliverables under `.scratch/` (gitignored): `<module>-review-<tag>.md` and
   `<module>-review-<tag>-fixer-report.md`.
3. **Verify — the part that actually catches residuals.** See the protocol below. The round's own
   verdict is **never** the gate: in the mux campaign rounds 3, 4, and 5 each self-reported
   "merge-ready" and each left a residual the orchestrator's independent verification caught.
4. **Commit + re-seed + rotate.** Commit the round's partial fix (a clean base for the next round),
   honestly labeled if incomplete. Re-seed the prompt with the new finding. Spawn the next round
   with a **different** model.

### Why rotate the model

Different models miss different things and fixate on different risks. Rotating Opus / Fable / Sonnet
across rounds is a cheap diversity lens: a bug one model reads past, another trips over. Convergence
across *different* models is far stronger evidence than N passes from one.

### Why independent verification is non-negotiable

A round agent that just fixed something is motivated to declare it fixed — the same self-grading
hazard the [`review`](../modules/review.md) module designs against. The orchestrator re-runs the
gates from a cold state, on the committed tree, and believes only what it observes. "No self-grading"
is the load-bearing discipline of the whole method.

## The verification protocol (orchestrator, every round)

Run from the module's worktree root. Adjust package paths per module.

```sh
# 1. Static + hermetic — must be green throughout
go build ./...
go vet ./internal/<module>engine/... ./internal/<module>cli/...
go test -count=5 ./internal/<module>engine/... ./internal/<module>cli/... ./cmd/lyx/...

# 2. Live serial smoke (real substrate, behind the `smoke` build tag)
go test -tags smoke ./internal/<module>cli/... -run Smoke -v -count=1
#    -> scan output for FAIL and for substrate-specific corruption markers
#       (mux: "being used by another process" / "TempDir RemoveAll" / "did not start")

# 3. THE decisive gate — N× CONCURRENT full smoke suites.
#    A quiet serial pass is NOT proof; concurrency + CPU saturation is the amplifier
#    that surfaces teardown races and leaked substrate state. Compile once, run N copies.
go test -c -tags smoke -o "$SCRATCH/smoke.test.exe" ./internal/<module>cli/...
for i in 1 2 3; do ( "$SCRATCH/smoke.test.exe" -test.run Smoke -test.count=1 -test.v \
    > "$SCRATCH/smoke_$i.txt" 2>&1; echo "run$i rc=$?" ) & done; wait
grep -hiE 'being used by another process|TempDir RemoveAll|did not start|FAIL' "$SCRATCH"/smoke_*.txt \
    || echo "no markers"

# 4. ZERO stray substrate state at teardown (mux: no leftover psmux servers)
tasklist | grep -i psmux || echo "zero psmux"   # must be zero
```

**Reading the result.** Green static+hermetic+serial-smoke establishes *correctness in the normal
single-instance flow* — that is the **merge bar**. The N× concurrent suite is a **diagnostic
amplifier**, not the merge gate: it drove the real fixes, but a timeout under an artificial N-suite
CPU peg is *not* a defect. Merge on: serial-clean + zero-stray-state + a couple of concurrent rounds
with zero corruption markers. (This distinction was agreed with the operator during the mux
campaign; keep it — don't let an artificial stress peg block a correct module.)

## The live sandbox suite (the substrate-driving vehicle)

Static and hermetic tests can't see the real bugs; a maintained **live-driving suite** is how the
round agent (and you) exercise the real substrate. For mux that is
[`tools/sandbox/SANDBOX-MUX-SUITE.md`](../../tools/sandbox/SANDBOX-MUX-SUITE.md) (scenarios M0–Mn),
run through the harness documented in [sandbox-howto.md](../sandbox-howto.md). Reusable rules that
bit us and are worth carrying to any module's suite:

- **Deploy-first footgun.** The suite runs the **deployed** binary, not your working tree. Re-run
  `deploy.cmd` after **every** source change or you validate a stale binary and draw a false
  PASS/FAIL. When in doubt, re-deploy.
- **The suite is a floor, not a ceiling.** M0–Mn is the minimum. The round agent is expected to
  hand-roll many more adversarial scenarios (crash/rebirth, cross-worktree scope, dead-but-present
  state, mid-op-failure orphans, rapid churn) beyond what the suite codifies.
- **Teardown discipline.** If you start any substrate server/session, tear it down and confirm zero
  stray processes at the end. "No stray state" is itself an invariant under test.
- **Grow the suite with the module.** When a round surfaces a live behavior the suite doesn't cover,
  add it as a new scenario in the same change (keep the coverage guard green — for mux,
  `sandbox_coverage_test.go`). A bug found live should leave behind both a `//go:build smoke`
  regression test *and*, where visual/manual, a suite scenario.

## Instantiating this for a new module

1. Copy [`review-prompt-template.md`](review-prompt-template.md) to
   `docs/reviews/<module>-review-prompt.md` and fill every `<PLACEHOLDER>` (what to read, the
   high-yield focus list = where *this* module's bugs actually live, the exact test commands, the
   substrate-teardown check).
2. Stand up a live suite under `tools/sandbox/SANDBOX-<MODULE>-SUITE.md` (mux's is the worked
   pattern) and wire its coverage guard.
3. Run the loop: seed → spawn (rotate model) → independently verify → re-seed → repeat until a
   safety pass finds nothing and your gates agree. Then do any operator-assisted step the harness
   can't reach headlessly (for mux: the visual `attach` test in a real TTY), and merge.

## Worked example — the mux campaign (the evidence this works)

Seven serial rounds, models rotated, one bug class chipped down each round; failure severity
degraded monotonically until it hit zero:

| Round | Model | What it closed |
|------:|-------|----------------|
| R3 | Opus  | `down` reap of pane children (left `remove`/churn leaking) |
| R4 | Fable | shared `descendantClosurePIDs`/`reapPaneChildren` seam for `down`+`remove`; dash-leading cmd escape; anchor validation (residual under concurrency) |
| R5 | Opus  | traced the real hub holder via PEB cwd; closed the psmux-**server** leak with saturation-tolerant deadlines (residual = pure timeout-under-saturation) |
| R6 | Fable | **F1** zero-pane zombie (empty-layout apply destroyed every pane) + **F11** positional select-layout reaping a tracked pane — two *new product* bugs prior rounds missed; plus hardening (F5/F6) and harness (F2/F3/F4) |
| R7 | Opus  | safety pass — **no new defects**; independently confirmed merge-ready |

R3, R4, and R5 each self-reported "merge-ready" and each was wrong — the orchestrator's independent
verification is what caught every residual. R6 was the first round to survive verification; R7 (a
belt-and-suspenders safety pass) and the orchestrator's gates *and* a live operator-assisted `attach`
test all agreed: clean. That convergence — round verdict + independent gates + live operator sign-off,
across rotated models — is the bar this method is built to reach.
