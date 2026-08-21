# crucible — a serial review+fix loop, a reusable hardening method

This directory holds **`crucible`** — the **manual, human-in-the-loop review method** we used to harden `reed` before merging it to `main`, plus the two prompts that drove it.
Named separately from the future, automated [`hardener`](../../manifest/designs/hardener.md) module this method is the hand-run prototype of (see below) — `crucible` is what you actually run today;
`hardener` is what it becomes once Go takes over the orchestrator role.
The method is **module-agnostic** — it is written down here so the modules built *on top of* reed (`shuttle` — see the `internal/shuttleengine` package documentation, `burler` (see the `internal/burlerengine` package documentation), [`hardener`](../../manifest/designs/hardener.md), [`loom`](../../manifest/designs/loom.md)) can reuse it instead of re-inventing it each time.

**The files here:**
- [`orchestrator-prompt.md`](orchestrator-prompt.md) — paste-ready prompt that bootstraps a thread into the **orchestrator** role (drives the loop, spawns rounds, independently verifies).
- [`review-prompt-template.md`](review-prompt-template.md) — module-agnostic skeleton for the **round agent** prompt (the reviewer-fixer a round spawns).
  The orchestrator fills it per module into `_mill/<module>-review-prompt.md` at run time and **commits it** (see "Commit deliverables continuously, not gitignored" below) — a module's state is stale the moment its review lands, so the file is rewritten and re-committed fresh each round, but every version that ever seeded a round stays in git history rather than being invisible.
- This README — the method itself (roles, loop, verification protocol) explained in prose.

> **This is the hand-executed prototype of the review-gate + `burler` (see the `internal/burlerengine` package documentation) round loop** (and the origin of the behavior-based [`hardener`](../../manifest/designs/hardener.md) concept). The automated engine — a fresh `burler` per round that does **A: review** then **B: fix**, with **no self-grading**, looped by a review gate with an **independent** progress check — is exactly this loop with the orchestrator role moved from a human+Claude pair into Go. This is how the method was originally run by hand; this doc remains the reference the engines were modeled on. If you change the method here, reconcile it with the `internal/shedadapters` and `internal/burlerengine` package documentation.
>
> **Text vs. behavior:** the review gate and `burler` automate the **text-based** form (read the artifact). [`hardener`](../../manifest/designs/hardener.md) (DRAFT) is the **behavior-based** form — *run* a live-substrate module in a sandbox — which is the harder campaign this directory actually documents for `reed`.

## When to use it

Reach for this before merging a **live-substrate module** — one whose real defects hide in composed, stateful, timing-sensitive behavior that a green `go test` does **not** prove (reed driving real tmux is the archetype;
anything driving real processes, sockets,
or an external tool qualifies).
For pure/logic modules a normal PR review is enough.
The tell that you need this loop: *"the unit tests pass but I don't trust it under load / crash / concurrency."*

## The two roles

- **Orchestrator** (a human operator + a steering Claude, i.e. *you* reading this).
  Owns the loop: seeds the prompt, spawns each round, **independently verifies** the round's work, re-seeds, rotates the model and effort tier, and decides when it has converged.
  The orchestrator does **not** edit the module code during a round — it stays off the worktree so it never collides with the round agent.
- **Round agent** — a fresh, **clean-room** sub-agent spawned per round (an effort-tiered `crucible-reviewer-<effort>` Agent, *not* a fork — a fork would inherit the orchestrator's context and destroy independence).
  It does two jobs in order: **A — review** (form its own findings by reading the code *and* driving the real substrate), then **B — fix** (implement, test, update docs).
  One agent does both because the review context is already loaded, so the fix is cheap.
  **The order is not advisory — it is a hard gate.**
  Job A must be fully written to its review-report file on disk before the agent touches any production or test file;
  fixing findings as it spots them (instead of after the report is saved) turns the "review" into a post-hoc rationalization of edits already made, which defeats the whole point of an independent judgment.
  Every per-module review prompt must state this explicitly (see the "Sequencing rule" in [`review-prompt-template.md`](review-prompt-template.md)) — this was missing from the template until shuttle's round 1 interleaved the two jobs.

## The loop

```
        ┌───────────────────────────────────────────────────────────────────────┐
        │  1. SEED the prompt with the current known state                      │
        │  2. SPAWN a fresh clean-room round agent (rotate model + effort)      │
        │        A — review (independent findings, drive real substrate)        │
        │        B — fix (implement + test + docs, commit each fix as it lands) │
        │  3. ORCHESTRATOR independently VERIFIES (never trust the              │
        │        round's own "merge-ready" verdict)                             │
        │  4. RE-SEED with what verification found; go to 2 with the            │
        │        next model + effort tier                                       │
        └─────────────────────────── until converged ───────────────────────────┘
```

1. **Seed.**
   The prompt (`_mill/<module>-review-prompt.md`, the orchestrator's filled instance of [`review-prompt-template.md`](review-prompt-template.md), committed) carries a *"round context seeded from prior-round verification"* section.
   Each round rewrites it with the residual the last verification found — or, once clean, flips it to a **safety pass** ("no known residual;
   confirm merge-readiness or find what every prior round missed").
2. **Spawn.**
   One fresh `subagent_type: crucible-reviewer-<effort>` Agent (the operator's pick this round) with a `model:` override, told **only** to read the prompt file and do exactly what it says, tagged `<model>-<effort>-r<N>`, told to **commit each individual fix as it lands** (message identifying the finding it closes — see "Commit per fix" in [`review-prompt-template.md`](review-prompt-template.md)) but **never push**.
   It writes two deliverables under `_mill/`, **committing each as soon as it is written or meaningfully updated** — not batched to round-end: `<module>-review-<tag>.md` and `<module>-review-<tag>-fixer-report.md`.
   See "Commit deliverables continuously, not gitignored" below.
3. **Verify — the part that actually catches residuals.**
   See the protocol below.
   The round's own verdict is **never** the gate: in the reed campaign rounds 3, 4, and 5 each self-reported "merge-ready" and each left a residual the orchestrator's independent verification caught.
4. **Re-seed + rotate.**
   The round's fixes are already committed one-by-one (per-fix commits, not a single wrap-up commit from the orchestrator — see below).
   Re-seed the prompt with whatever verification found.
   Spawn the next round with a **different** model and/or effort tier.

### Why commit per fix, not one commit for the whole round

A round agent's session can be killed by something entirely outside the method's control — a corrupted terminal, a lost connection — mid-fix, with no self-report at all.
If its fixes sit as one uncommitted working-tree diff, the orchestrator has to reverse-engineer, finding by finding, which ones actually landed clean.
Committing after each individual fix (green build/vet/test, plus the live check if the finding needed one) turns that same crash into something the orchestrator can just read: `git log` on the branch shows exactly which findings are done, and anything with no commit is unambiguously not done — no guesswork.
This happened for real on shuttle's round 2: the operator's terminal broke mid-fix, the round had produced a review and several real fixes, but with no commits and no fixer report, the orchestrator had to independently re-derive which fixes were actually complete from a raw diff before it could safely continue.

### Why log incrementally during Job 1 too, not just commit fixes in Job 2

Commit-per-fix (above) protects Job 2 — fixing — from a mid-work crash.
Until this was noticed, Job 1 — the review itself, especially the live-substrate driving that takes real wall-clock minutes — had no equivalent protection: a round could run every hermetic and live-driving check, form a complete picture, and be killed before writing a single byte of its review report, leaving the orchestrator with nothing — not even a partial account of what was tried.
This happened for real on a burler campaign's first round: killed mid-review, before `.scratch/burler-review-*.md` existed at all, with no commits either — the orchestrator had zero evidence to read, not even which live scenarios had already been run.
The fix, now in [`review-prompt-template.md`](review-prompt-template.md)'s "Log as you go" section: the round agent appends the review report's What-was-tested section (and provisional findings) to disk immediately after each command/scenario, in real time, rather than holding it all in working context to compose in one shot at the end.
A round that then dies at 95% leaves a 95%-complete account on disk, not an empty `.scratch/` directory.

### Why deliverables are committed continuously, not gitignored

Logging as you go (above) protects against losing the account *within* a single running session — but a file that only ever exists on disk, gitignored, is still gone the moment the worktree it lives in is torn down, which for this repo (per `CLAUDE.md`'s "Persistent notes go in git, not file-memory") happens routinely on merge for every short-lived mill task worktree.
`.scratch/`'s old convention — write there, never commit, treat it as ephemeral — was correct for the seed prompt's own staleness (a module's state IS stale the moment its review lands) but wrong for the review report, the fixer report, and the handoff note: those are the campaign's actual record, and losing them to a worktree teardown is the same failure "log as you go" exists to prevent, just at a longer time horizon.
The fix: every crucible deliverable lives under `_mill/` (a worktree's normal, git-tracked task directory) instead of `.scratch/`, and gets **committed as soon as it is written or meaningfully updated** — the review report after each logged test/scenario or finding, the fixer report after each fix (folding into that fix's own commit is fine), the re-seeded prompt each time the orchestrator rewrites it, the handoff note each time it's refreshed.
This is the same "commit per fix" discipline extended to the paperwork: a killed session should never cost more than the single most recent update, for the review record exactly as much as for the code.

### Why rotate the model

Different models miss different things and fixate on different risks.
Rotating Opus / Fable / Sonnet across rounds is a cheap diversity lens: a bug one model reads past, another trips over.
Convergence across *different* models is far stronger evidence than N passes from one.

### The effort axis

Reasoning effort is a second, independent knob a round agent's `subagent_type: crucible-reviewer-<effort>` spawn selects, alongside — not instead of — the model rotation above: the operator picks both per round.
See `orchestrator-prompt.md`'s "Model + effort selection" section (including its enumeration of the shipped tier names) for the rationale and the full tier list;
it is not repeated here.

### Why independent verification is non-negotiable

A round agent that just fixed something is motivated to declare it fixed — the same self-grading hazard the review gate and `burler` (see the `internal/burlerengine` package documentation) design against (A-before-B in burler;
a fresh burler per round under the gate).
The orchestrator re-runs the gates from a cold state, on the committed tree, and believes only what it observes.
"No self-grading" is the load-bearing discipline of the whole method.

## The verification protocol (orchestrator, every round)

**Before running any of this, know what class of module you're verifying.**
This protocol was designed and validated on `reed`, where a smoke test only ever costs a real tmux pane — cheap to run broadly, cheap to run N times concurrently.
A module whose smoke tests drive a real LLM round (burler, loom) is a different animal entirely: a single test function can spawn several simultaneous real provider sessions (a cluster/fan round spawns one per lens), and step 3's "N concurrent copies of the WHOLE suite" multiplies that further.
Applying this protocol to an LLM-driving module exactly as written below — bare `-run Smoke`, N concurrent copies — is what caused a real incident: it matched and ran every smoke test in the package (including cluster-fan tests nobody intended to run that round), spawning enough real `claude` processes simultaneously to exhaust the host's RAM in minutes.
Before step 2, work out (from the module's own smoke test source, not from assumption) how many real LLM subprocesses a bare `-run Smoke` would spawn;
if it's more than one process for an LLM-driving module, replace `-run Smoke` with the exact test name you actually mean to run, and skip or radically scale down step 3's concurrency for that module (see the per-module review prompt's own "Live-substrate cost declaration" section, which every instantiation must fill in).

Run from the module's worktree root.
Adjust package paths per module.

```sh
# 1. Static + hermetic — must be green throughout
go build ./...
go vet ./internal/<module>engine/... ./internal/<module>cli/...
go test -count=5 ./internal/<module>engine/... ./internal/<module>cli/... ./cmd/lyx/...

# 2. Live serial smoke (real substrate, behind the `smoke` build tag)
# For an LLM-driving module, replace -run Smoke with the ONE exact test name you mean to run.
go test -tags smoke ./internal/<module>cli/... -run Smoke -v -count=1
#    -> scan output for FAIL and for substrate-specific corruption markers
#       (reed: "being used by another process" / "TempDir RemoveAll" / "did not start")

# 3. THE decisive gate — N× CONCURRENT full smoke suites — TMUX-ONLY MODULES LIKE REED ONLY.
#    Do NOT run this step against an LLM-driving module without first computing the real
#    process count it would produce (tests-matched × subprocesses-per-test × N) and getting
#    the operator to confirm that count is acceptable — it is not a default step for those modules.
#    A quiet serial pass is NOT proof; concurrency + CPU saturation is the amplifier
#    that surfaces teardown races and leaked substrate state. Compile once, run N copies.
go test -c -tags smoke -o "$SCRATCH/smoke.test.exe" ./internal/<module>cli/...
for i in 1 2 3; do ( "$SCRATCH/smoke.test.exe" -test.run Smoke -test.count=1 -test.v \
    > "$SCRATCH/smoke_$i.txt" 2>&1; echo "run$i rc=$?" ) & done; wait
grep -hiE 'being used by another process|TempDir RemoveAll|did not start|FAIL' "$SCRATCH"/smoke_*.txt \
    || echo "no markers"

# 4. ZERO stray substrate state at teardown (reed: no leftover tmux servers)
tasklist | grep -i tmux || echo "zero tmux"   # must be zero
```

**Reading the result.**
Green static+hermetic+serial-smoke establishes *correctness in the normal single-instance flow* — that is the **merge bar**.
The N× concurrent suite is a **diagnostic amplifier**, not the merge gate: it drove the real fixes, but a timeout under an artificial N-suite CPU peg is *not* a defect.
Merge on: serial-clean + zero-stray-state + a couple of concurrent rounds with zero corruption markers. (This distinction was agreed with the operator during the reed campaign;
keep it — don't let an artificial stress peg block a correct module.)

## Driving the real substrate — the round agent does it itself, directly

Static and hermetic tests can't see the real bugs;
the round agent exercises the real substrate by calling the module's own CLI **directly, with its own tool calls**, guided by the per-module review prompt's "High-yield focus" invariant list (see [`review-prompt-template.md`](review-prompt-template.md)).

**Do not have the round agent invoke a `sandbox-<module>-suite.cmd` launcher.** `tools/sandbox/`'s launcher machinery (`SANDBOX-<MODULE>-SUITE.md` + `suite.go`/`main.go`/the `.cmd` wrapper) exists to hand a scenario doc to a SEPARATE, context-free interactive `claude` session — a naive black-box tester with no source knowledge, useful for a *human operator* dogfooding the CLI (see [sandbox-howto.md](../sandbox-howto.md)), but meaningless for a round agent to spawn on top of itself: the round agent already has full source knowledge and its own tool calls,
and the spawned session has no attached console of its own to inherit in this context anyway.
Builder's `opus-r1` round (2026-07) made exactly this mistake — it read "launch the suite" as "invoke the launcher", judged that operator-assisted/cost-bearing, and as a result skipped ALL live driving for an entire round, silently substituting pure code-tracing.
The fix: the round agent runs the real CLI commands itself (`lyx <module> <verb>`, foreground, waiting for each to return).
This spawns real substrate underneath when the module rides reed/shuttle (real tmux panes, real interactive `claude` sessions) — that is expected and required, not something to avoid.
None of it needs an attached TTY of its own: a tmux pane is a real pty regardless of whether anyone is watching it.

If the module already has a maintained `tools/sandbox/SANDBOX-<MODULE>-SUITE.md` (built for the separate human-operator dogfooding use case), the round agent MAY read it for scenario ideas, but must execute every scenario with its own tool calls — never via the launcher.
**Building a new dedicated suite file + launcher wiring is NOT a prerequisite for running this method on a new module.**
That machinery serves `CONSTRAINTS.md`'s Sandbox Suite Coverage invariant — a separate, pre-existing requirement for every *registered* module — not something this hardening method needs to stand up for itself.

Reusable rules that bit us and are worth carrying to any module's live driving:

- **Deploy-first footgun.**
  Live driving runs the **deployed** binary, not your working tree.
  Re-run `deploy-dev.cmd` (`deploy-dev` on POSIX) after **every** source change or you validate a stale binary and draw a false PASS/FAIL.
  When in doubt, re-deploy.
- **Cost/time is not a reason to skip live driving.**
  A real substrate session (a real implementer/agent doing real work) takes real wall-clock minutes, not seconds — that is a budget fact, not grounds to fall back to code-tracing.
  Reserve "cannot verify headlessly" strictly for a genuine environment gap or an actual human-eyeball need (e.g. a visual `lyx reed attach` confirmation) — never a blanket cost/turn-budget excuse.
- **The high-yield focus list is a floor, not a ceiling.**
  The round agent is expected to hand-roll many more adversarial scenarios (crash/rebirth, cross-worktree scope, dead-but-present state, mid-op-failure orphans, rapid churn) beyond what that list names.
- **Teardown discipline.**
  If you start any substrate server/session, tear it down and confirm zero stray processes at the end.
  "No stray state" is itself an invariant under test.
- **Grow whatever scenario record you keep with the module.**
  If a maintained SANDBOX-suite file already exists for the module, extend it when a round surfaces a live behavior it doesn't cover (keep `sandbox_coverage_test.go` green).
  A bug found live should leave behind a `//go:build smoke` regression test regardless of whether a suite file exists.

## Instantiating this for a new module

1. Fill every `<PLACEHOLDER>` in a copy of [`review-prompt-template.md`](review-prompt-template.md) and write it to `_mill/<module>-review-prompt.md`, then commit it: what to read, the high-yield focus list = where *this* module's bugs actually live, the exact test commands, the substrate-teardown check.
2. Confirm the module already satisfies `CONSTRAINTS.md`'s Sandbox Suite Coverage invariant (a `**Covers:** <module>` tag somewhere under `tools/sandbox/*SUITE.md`).
   That invariant is pre-existing and independent of this method — do NOT build a new dedicated suite file or launcher just to satisfy this hardening loop;
   the round agent drives the real CLI directly (see "Driving the real substrate" above) whether or not a dedicated suite file exists.
3. Run the loop: seed → spawn (rotate model + effort) → independently verify → re-seed → repeat until a safety pass finds nothing and your gates agree.
   Then do any operator-assisted step the harness can't reach headlessly (for reed: the visual `attach` test in a real TTY), and merge.

## Worked example — the reed campaign (the evidence this works)

Seven serial rounds, models rotated, one bug class chipped down each round;
failure severity degraded monotonically until it hit zero:

| Round | Model | Effort | What it closed |
|------:|-------|--------|----------------|
| R3 | Opus  | n/a | `down` reap of pane children (left `remove`/churn leaking) |
| R4 | Fable | n/a | shared `descendantClosurePIDs`/`reapPaneChildren` seam for `down`+`remove`; dash-leading cmd escape; anchor validation (residual under concurrency) |
| R5 | Opus  | n/a | traced the real hub holder via PEB cwd; closed the tmux-**server** leak with saturation-tolerant deadlines (residual = pure timeout-under-saturation) |
| R6 | Fable | n/a | **F1** zero-pane zombie (empty-layout apply destroyed every pane) + **F11** positional select-layout reaping a tracked pane — two *new product* bugs prior rounds missed; plus hardening (F5/F6) and harness (F2/F3/F4) |
| R7 | Opus  | n/a | safety pass — **no new defects**; independently confirmed merge-ready |

R3, R4, and R5 each self-reported "merge-ready" and each was wrong — the orchestrator's independent verification is what caught every residual.
R6 was the first round to survive verification;
R7 (a belt-and-suspenders safety pass) and the orchestrator's gates *and* a live operator-assisted `attach` test all agreed: clean.
That convergence — round verdict + independent gates + live operator sign-off, across rotated models — is the bar this method is built to reach.

### Why fix every finding, including NITs — not just BLOCKING/MEDIUM

The reed campaign above took seven rounds to converge.
In retrospect, the operator's experience with an earlier review setup (millhouse's own) points at a likely contributor: when a round's prompt only required fixing higher-severity findings and let NIT/LOW findings sit as "reported but not fixed," round count went up — unfixed NITs don't just stay static, they re-surface (or silently vanish) across subsequent rounds instead of ever closing, adding rounds that should not have been needed.
Round count dropped sharply once the instruction changed to fix everything a round finds, all severities, in the same round.
This is why the shuttle instance of this method (and the template, going forward) requires fixing every recorded finding — including NITs — not just the BLOCKING/MEDIUM ones;
severity affects how a finding is reported, not whether it gets fixed.

**This is a severity rule, not a size rule — a large finding is a separate case.** A NIT still gets fixed inline, in this round; but a finding whose fix is genuinely LARGE (a subsystem/feature addition, a cross-cutting refactor reaching outside the module) does not belong in a round's commit-per-fix loop at all, regardless of its severity label. See `orchestrator-prompt.md`'s Hard Rule 5 for the full instruction: record it like any other finding, mark it NOT-FIXED-THIS-ROUND, and the orchestrator spins it into its own mill-wiki task instead of letting a hardening round grow into feature work.

## Worked example — the fabric campaign (where the method was refined)

Six serial rounds against `fabric` after the slice 1–10 v2 rewrite.
81 findings, 9 BLOCKING, 8 of them data-loss.

| Round | Model | Effort | What it did |
|------:|-------|--------|-------------|
| R1 | Opus  | high   | 23 findings; self-reported "ready to merge" and the operator rejected that outright |
| R2 | Fable | high   | 17 findings; `pull` destroying uncommitted warp work via `ResetHard` while returning `ok:true` |
| R3 | Opus  | high   | 6 findings; hostile-slug `remove`, plus a 40-cell dirty-worktree matrix |
| R4 | Opus  | medium | **three graded sweeps instead of a fourth review** — closed two defect classes with counted evidence |
| R5 | Opus  | medium | 15 findings, 2 BLOCKING, in three regions no prior round had ever driven |
| R6 | Fable | high   | two graded sweeps; **no BLOCKING and no MEDIUM** — first round to meet the bar |

Every round self-reported some form of "all green, all fixed".
**Six for six, the orchestrator's independent verification found something material the round's own report did not surface** — including, after R3, a BLOCKING data-loss bug in a file R3 had been editing.

### The refinements this campaign produced

**1. Pre-count the ground truth BEFORE spawning the round — and record the count's blind spots next to the number.**
When a round is asked to enumerate a class, the orchestrator counts the same class first, into a file the round never sees.
A total *below* the pre-count is the truncation signal; a total *above* it is the correct direction.
Without this, "I enumerated 28 sites" is unfalsifiable.

The blind-spot half matters as much as the number.
A grep is only as good as its pattern: an `os.RemoveAll` pattern cannot see calls routed through a seam, and a line count cannot tell code from a comment mentioning the same identifier.
**Twice in this campaign the round corrected the orchestrator's count rather than the reverse** — which is the round working, not failing.
Write down what the pattern cannot see, so a later exact match is not mistaken for agreement when it should have been a correction.

*Measured effect, stated honestly.*
Across this campaign the pre-count **detected zero truncations** — every round met or exceeded every number.
It also caused one wrong conclusion: the orchestrator had predicted a specific blind spot would force R6's `os.WriteFile` total above the pre-count, and when R6 reported an exact match, accused it of adopting the number rather than counting.
R6 had decomposed the total correctly (8 call sites plus 3 comment mentions); the prediction was what was wrong.

What the pre-count demonstrably DID deliver was different from its stated purpose: it forced rounds to **explain deltas instead of just reporting totals** (R5 wrote out why its 388 err-check blocks is more accurate than the orchestrator's 304 — 108 inline `if x, err := f(); err != nil` lines the simpler grep cannot see), and it got the orchestrator's own numbers corrected twice.

Keep doing it — it is cheap and it makes reports checkable.
But do not claim it catches truncation on this evidence.
A deterrent cannot be measured by how often it fires, and whether no round truncated *because* it knew it would be counted is not something this campaign can establish.

**2. When the tail starts circling, stop reviewing and start counting.**
By R3 the findings were still real but were repeating two or three recognisable shapes.
A fourth broad review would most likely have produced a fourth variant.
R4 was therefore given *sweeps* instead: enumerate every site in a class, present a table with a row for every site **including the ones judged correct, with the reason**, and state the total plus the enumeration method as reproducible shell.

The sweep found a defect at a site the orchestrator's own worked example did not point at — which is the whole argument for counting a class rather than fixing its instances.
It was also **cheaper** than a broad review (428k tokens vs 535k), because one file read covers many table rows and the enumeration is scripted rather than performed per-site by the model.

**3. Derive each round's assignment from the previous round's residue — never "review it again".**
R4 became sweeps because R3 was circling.
R5 became adversarial work on three regions nobody had driven, because R4's sweeps had closed the countable classes.
R6 became two new sweeps because R5's two BLOCKING findings were interesting as *shapes* rather than as bugs, and both shapes were unenumerated.
Each round's yield came from a region the previous round's *method* could not reach — not from luck.

**4. Grade an adversarial round on evidence of execution, not on argument.**
A race reasoned about but never made to happen is not a finding.
Require the interleaved run behind every concurrency claim, a **strictly sequential control** of the identical sequence, and a **reproduction on a second independent hub**.
The control is what turns "the file got corrupted" into "the file got corrupted *because of concurrency*"; the second hub is what turns an anecdote into a finding.

**5. Prove the scenario actually reached the code before believing a clean result.**
This is the sharpest lesson of the campaign, and both the orchestrator and a round fell into it.

While verifying R5's `.git/info/exclude` fix, the orchestrator ran a six-way concurrency storm that stayed green — and then stayed green *with the lock and the atomic write both sabotaged*.
The scenario had never entered the write path: sibling worktrees kept every exclusion, so the read-modify-write short-circuited on "nothing changed" and never wrote.
R6 then reproduced the same mistake independently: its two `.git/info/exclude` table rows were marked "driven — correct" on the strength of concurrent verbs that, on an established hub, do not touch that file at all.

**A green run that never executed the code is indistinguishable from a fix.**
Reporting one as the other is the worst error available to a verifier.
The check is cheap: sabotage the mechanism and confirm the scenario now fails, or instrument the write and confirm it happened.
If the scenario stays green under sabotage, the scenario is unproven — not the code.

**6. Read the neighbouring code while preparing each sabotage.**
The single highest-yield habit the campaign produced.
Reverting a hunk to watch a test fail means reading closely around it, and that is how the orchestrator found a BLOCKING data-loss bug in `prune` that three dedicated review rounds had missed.
Three rounds of looking for bugs did not find it; ten minutes of reading one file closely enough to neuter a single line did.

**7. Seed cost rules — with an explicit guardrail against under-driving.**
Rounds cost 300–535k tokens each, roughly 70% of it tool results rather than reasoning.
Cost rules (re-verify narrowly, never read a verbose command's full output back, read large files with offset/limit, batch independent shell work, script repetitive fixtures) cut the cost of the largest round by 40% with no loss of coverage.

They must ship with the guardrail, verbatim in the prompt:
**this is about removing waste, never about driving less** — and a "what is NOT waste" list naming the independent fixture per destructive scenario, the sabotage proofs, the live reproduction of every BLOCKING finding, and the full hermetic gates at the end.
A round that saves tokens by reading code instead of driving it has failed, and its findings get rejected.

**8. Never characterise a round's work without reading its "what was tested" section.**
The orchestrator once told the operator that one round "drove live where the other read code".
That was false and had not been checked — both had driven live, and the actual difference was much narrower.
Check before summarising; a wrong characterisation of a round misdirects every round after it.

### What a converged round looks like, and what it still does not prove

R6 met the bar: no BLOCKING, no MEDIUM, gates reproduced by the orchestrator, working tree provably untouched.

It is worth recording what that does *not* establish.
R6's scope was two enumerated classes, so its zero says those two classes are clean — not that the module is.
Its own method failed to demonstrate itself on the two calibration rows (see refinement 5), so its zero is weaker evidence than a zero from a method proven able to detect.
And after six rounds the campaign still carried a named, never-executed gap: Windows path behaviour, unreachable from a Linux host and reasoned about rather than driven, every single round.

State those limits in the convergence verdict.
A campaign that ends by claiming more than it proved teaches the next campaign to do the same.
