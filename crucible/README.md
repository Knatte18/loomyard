# crucible — a serial review+fix loop, a reusable hardening method

This directory holds **`crucible`**, a manual, human-in-the-loop review method.
It hardens a module before the module merges to `main`.
The directory also holds the prompts and agent profiles that drive the method.
`crucible` is the hand-run prototype of the automated `hardener` module (the board's `hardener` note).
The automated module replaces the human orchestrator with Go.
The method is module-agnostic, so modules built on top of reed (`shuttle`, `burler`, `hardener`, `loom`) can reuse it.
The `internal/shuttleengine`, `internal/burlerengine` and `internal/loomengine` package documentation describe those modules.

**The files here:**
- [`orchestrator-prompt.md`](orchestrator-prompt.md) — paste-ready prompt for the **orchestrator** role.
  It bootstraps a thread into that role.
  The orchestrator drives the loop, spawns rounds and independently verifies.
- [`review-prompt-template.md`](review-prompt-template.md) — module-agnostic skeleton for the **round agent** prompt.
  The round agent is the reviewer-fixer a round spawns.
  The orchestrator fills the template per module into `_mill/<module>-review-prompt.md` at run time and commits it.
  See "Why deliverables are committed continuously, not gitignored" below.
  A module's state is stale once its review lands, so the file is rewritten and re-committed each round;
  every version that seeded a round stays in git history.
- [`agents/`](agents/) — the effort-tiered `crucible-reviewer-<effort>` round-agent profiles.
  They live here, not in `.claude/agents/`, so they cost no context in sessions that never run crucible;
  copy them into `.claude/agents/` before a run and remove them after.
- This README — the method itself (roles, loop, verification protocol) explained in prose.

> **This is the hand-executed prototype of the review-gate + `burler` round loop.**
> The automated engine runs a fresh `burler` per round that does **A: review** then **B: fix**, with **no self-grading**.
> A review gate loops it with an **independent** progress check.
> That is this loop with the orchestrator role moved from a human+Claude pair into Go.
> The engines are modeled on this document: reconcile any change to the method with the `internal/shedadapters` and `internal/burlerengine` package documentation.
>
> **Text vs. behavior:** the review gate and `burler` automate the **text-based** form (read the artifact).
> `hardener` (DRAFT) is the **behavior-based** form: it *runs* a live-substrate module in a sandbox, which is the campaign this directory documents.

## When to use it

Reach for this before merging a **live-substrate module**.
Its real defects hide in composed, stateful, timing-sensitive behavior that a green `go test` does **not** prove.
Reed driving real tmux is the archetype; anything driving real processes, sockets or an external tool qualifies.
For pure/logic modules a normal PR review is enough.
The tell that you need this loop: *"the unit tests pass but I don't trust it under load / crash / concurrency."*

## The two roles

- **Orchestrator** (a human operator + a steering Claude, i.e. *you* reading this).
  Owns the loop: seeds the prompt, spawns each round, **independently verifies** the round's work and re-seeds.
  It also rotates the model and effort tier and decides when the loop has converged.
  The orchestrator does **not** edit the module code during a round.
  It stays off the worktree so it never collides with the round agent.
- **Round agent**: a fresh, **clean-room** sub-agent spawned per round.
  It is an effort-tiered `crucible-reviewer-<effort>` Agent, *not* a fork.
  A fork would inherit the orchestrator's context and destroy independence.
  It does two jobs in order.
  **A — review**: form its own findings by reading the code *and* driving the real substrate.
  **B — fix**: implement, test, update docs.
  One agent does both because the review context is already loaded, so the fix is cheap.
  **The order is not advisory — it is a hard gate.**
  Job A must be fully written to its review-report file on disk before the agent touches any production or test file.
  Fixing findings before the report is saved turns the "review" into a post-hoc rationalization of edits already made.
  That defeats the point of an independent judgment.
  Every per-module review prompt must state this explicitly.
  See the "Sequencing rule" in [`review-prompt-template.md`](review-prompt-template.md).

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
   The prompt lives at `_mill/<module>-review-prompt.md` and is committed.
   The orchestrator fills it from [`review-prompt-template.md`](review-prompt-template.md).
   It carries a *"round context seeded from prior-round verification"* section.
   Each round rewrites that section with the residual the last verification found.
   Once the module is clean, the section flips to a **safety pass**:
   "no known residual; confirm merge-readiness or find what every prior round missed".
2. **Spawn.**
   Spawn one fresh `subagent_type: crucible-reviewer-<effort>` Agent with a `model:` override.
   The operator picks the effort for the round.
   Tell it **only** to read the prompt file and do exactly what it says.
   Tag it `<model>-<effort>-r<N>`.
   Tell it to **commit each individual fix as it lands**, with a message identifying the finding it closes.
   Tell it to **never push**.
   See "Commit per fix" in [`review-prompt-template.md`](review-prompt-template.md).
   It writes two deliverables under `_mill/`: `<module>-review-<tag>.md` and `<module>-review-<tag>-fixer-report.md`.
   It commits each as soon as it is written or meaningfully updated, not batched to round-end.
   See "Why deliverables are committed continuously, not gitignored" below.
3. **Verify — the part that actually catches residuals.**
   See the protocol below.
   The round's own verdict is **never** the gate.
   In the reed campaign rounds 3, 4 and 5 each self-reported "merge-ready".
   Each left a residual that the orchestrator's independent verification caught.
4. **Re-seed + rotate.**
   The round's fixes are already committed one by one, not in a single wrap-up commit from the orchestrator.
   Re-seed the prompt with whatever verification found.
   Spawn the next round with a **different** model and/or effort tier.

### Why commit per fix, not one commit for the whole round

A round agent's session can be killed mid-fix by a corrupted terminal or a lost connection, with no self-report.
If its fixes sit in one uncommitted working-tree diff, the orchestrator must reverse-engineer which ones landed clean.
Committing after each fix makes `git log` show exactly which findings are done.
A fix is committed once build, vet and test are green, plus the live check if the finding needed one.
A finding with no commit is not done.

### Why log incrementally during Job 1 too, not just commit fixes in Job 2

Commit-per-fix protects Job 2 (fixing) from a mid-work crash.
Job 1, the review, needs the same protection, especially when live-substrate driving takes real wall-clock minutes.
A round killed before it writes its review report leaves the orchestrator with no account of what was tried.
Hence the round agent follows the "Log as you go" section of [`review-prompt-template.md`](review-prompt-template.md).
It appends the review report's What-was-tested section and provisional findings to disk.
It does so immediately after each command or scenario.
A round that dies at 95% leaves a 95%-complete account on disk.

### Why deliverables are committed continuously, not gitignored

Logging as you go protects the account within one running session.
A gitignored file is gone when its worktree is torn down.
Short-lived task worktrees are torn down routinely on merge, and persistent notes go in git.
The review report, the fixer report and the handoff note are the campaign's record.
Losing them is the failure "log as you go" exists to prevent.
Every crucible deliverable therefore lives under `_mill/` (a worktree's git-tracked task directory), not `.scratch/`.
It is **committed as soon as it is written or meaningfully updated**:
the review report after each logged test, scenario or finding;
the fixer report after each fix (folding into that fix's commit is fine);
the re-seeded prompt each time the orchestrator rewrites it;
the handoff note each time it is refreshed.
A killed session then costs at most the most recent update.

### Why rotate the model

Different models miss different things and fixate on different risks.
Rotating Opus / Fable / Sonnet across rounds is a cheap diversity lens.
A bug one model reads past, another trips over.
Convergence across *different* models is far stronger evidence than N passes from one.

### The effort axis

Reasoning effort is a second, independent knob.
The round agent's `subagent_type: crucible-reviewer-<effort>` selects it.
It works alongside the model rotation, not instead of it, and the operator picks both per round.
`orchestrator-prompt.md`'s "Model + effort selection" section holds the rationale and the full tier list.
The list includes the shipped tier names.

### Why independent verification is non-negotiable

A round agent that just fixed something is motivated to declare it fixed.
This is the self-grading hazard the review gate and `burler` design against.
The defences are A-before-B in burler and a fresh burler per round under the gate.
See the `internal/burlerengine` package documentation for burler.
The orchestrator re-runs the gates from a cold state, on the committed tree, and believes only what it observes.
"No self-grading" is the load-bearing discipline of the whole method.

## The verification protocol (orchestrator, every round)

**Before running any of this, know what class of module you're verifying.**
This protocol was designed on `reed`, where a smoke test costs a real tmux pane.
Such a test is cheap to run broadly and N times concurrently.
A module whose smoke tests drive a real LLM round (burler, loom) differs.
A single test function can spawn several simultaneous real provider sessions; a cluster/fan round spawns one per lens.
Step 3's N concurrent copies of the whole suite multiply that.
A bare `-run Smoke` with N concurrent copies on such a module runs every smoke test in the package.
That can exhaust the host's RAM with real `claude` processes.
Before step 2, work out from the module's smoke test source how many real LLM subprocesses a bare `-run Smoke` spawns.
If it is more than one for an LLM-driving module, replace `-run Smoke` with the exact test name you mean to run.
Also skip step 3's concurrency for that module, or scale it down radically.
See the per-module review prompt's "Live-substrate cost declaration" section, which every instantiation must fill in.

Run from the module's worktree root.
Adjust package paths per module.

```sh
# 1. Static + hermetic — must be green throughout
go build ./...
go vet ./internal/<module>engine/... ./internal/<module>cli/...
go test -count=5 ./internal/<module>engine/... ./internal/<module>cli/... ./cmd/lym/...

# 2. Live serial smoke (real substrate: `LYM_TEST_TIER=2` for a tmux/pty module, `LYM_TEST_TIER=3` for an LLM-driving one)
# For an LLM-driving module, use LYM_TEST_TIER=3 and replace -run Smoke with the ONE exact test name you mean to run.
LYM_TEST_TIER=2 go test ./internal/<module>cli/... -run Smoke -v -count=1
#    -> scan output for FAIL and for substrate-specific corruption markers
#       (reed: "being used by another process" / "TempDir RemoveAll" / "did not start")

# 3. THE decisive gate — N× CONCURRENT full smoke suites — TMUX-ONLY MODULES LIKE REED ONLY.
#    Do NOT run this step against an LLM-driving module without first computing the real
#    process count it would produce (tests-matched × subprocesses-per-test × N) and getting
#    the operator to confirm that count is acceptable — it is not a default step for those modules.
#    A quiet serial pass is NOT proof; concurrency + CPU saturation is the amplifier
#    that surfaces teardown races and leaked substrate state. Compile once, run N copies.
go test -c -o "$SCRATCH/smoke.test.exe" ./internal/<module>cli/...
# the compiled binary reads the tier at run time, not at compile time
export LYM_TEST_TIER=2
for i in 1 2 3; do ( "$SCRATCH/smoke.test.exe" -test.run Smoke -test.count=1 -test.v \
    > "$SCRATCH/smoke_$i.txt" 2>&1; echo "run$i rc=$?" ) & done; wait
grep -hiE 'being used by another process|TempDir RemoveAll|did not start|FAIL' "$SCRATCH"/smoke_*.txt \
    || echo "no markers"

# 4. ZERO stray substrate state at teardown (reed: no leftover tmux servers)
tasklist | grep -i tmux || echo "zero tmux"   # must be zero
```

**Reading the result.**
Green static+hermetic+serial-smoke establishes *correctness in the normal single-instance flow*.
That is the **merge bar**.
The N× concurrent suite is a **diagnostic amplifier**, not the merge gate.
It drove the real fixes, but a timeout under an artificial N-suite CPU peg is *not* a defect.
Merge on: serial-clean + zero-stray-state + a couple of concurrent rounds with zero corruption markers.
An artificial stress peg must not block a correct module.

## Driving the real substrate — the round agent does it itself, directly

Static and hermetic tests can't see the real bugs.
The round agent exercises the real substrate by calling the module's own CLI **directly, with its own tool calls**.
The per-module review prompt's "High-yield focus" invariant list guides it.
See [`review-prompt-template.md`](review-prompt-template.md).

**Do not have the round agent invoke a `sandbox-<module>-suite.cmd` launcher.**
The `tools/sandbox/` launcher machinery hands a scenario doc to a SEPARATE, context-free interactive `claude` session.
That machinery is `SANDBOX-<MODULE>-SUITE.md`, `suite.go`, `main.go` and the `.cmd` wrapper.
The spawned session is a naive black-box tester with no source knowledge.
It is useful for a *human operator* dogfooding the CLI.
See [sandbox-howto.md](../docs/sandbox-howto.md).
It is meaningless for a round agent to spawn on top of itself.
The round agent already has full source knowledge and its own tool calls,
and the spawned session has no attached console of its own to inherit in this context anyway.
A round that reads "launch the suite" as "invoke the launcher" skips all live driving.
It silently substitutes code-tracing.
The round agent runs the real CLI commands itself (`lym <module> <verb>`, foreground, waiting for each to return).
When the module rides reed/shuttle, this spawns real substrate underneath.
The substrate is real tmux panes and real interactive `claude` sessions.
That is expected and required, not something to avoid.
None of it needs an attached TTY of its own: a tmux pane is a real pty regardless of whether anyone is watching it.

A maintained `tools/sandbox/SANDBOX-<MODULE>-SUITE.md` is built for human-operator dogfooding.
If the module has one, the round agent MAY read it for scenario ideas.
It must execute every scenario with its own tool calls, never via the launcher.
**Building a new dedicated suite file and launcher wiring is NOT a prerequisite for running this method on a new module.**
That machinery serves the sandbox suite coverage invariant that the `tools/sandbox` command doc states.
The invariant is a separate requirement for every *registered* module.
This hardening method does not need to stand it up for itself.

Reusable rules that bit us and are worth carrying to any module's live driving:

- **Deploy-first footgun.**
  Live driving runs the **deployed** binary, not your working tree.
  Re-run `deploy-dev.cmd` (`deploy-dev` on POSIX) after **every** source change.
  Otherwise you validate a stale binary and draw a false PASS/FAIL.
  When in doubt, re-deploy.
- **Spawned agent panes run the binary that spawned them.**
  Reed prepends the spawning binary's directory to every strand pane's `PATH` and exports `LYM_BIN` to its absolute path,
  so an agent pane's bare `lym …` resolves to the same build driving the round — no PATH setup needed.
  For a round agent, the deploy-first footgun above concerns the binary the round is *started* from;
  the panes underneath it inherit that choice rather than falling back to the operator's production install.
  Without this, agents inside a child worktree could run a different `PATH`-resolved build than the one under test.
  That build could rewrite config the tested binary cannot load, with nothing reporting the mismatch.
- **Cost/time is not a reason to skip live driving.**
  A real substrate session, a real implementer doing real work, takes real wall-clock minutes, not seconds.
  That is a budget fact, not grounds to fall back to code-tracing.
  Reserve "cannot verify headlessly" for a genuine environment gap or an actual human-eyeball need.
  A visual `lym reed attach` confirmation is an example.
  It is never a blanket cost or turn-budget excuse.
- **The high-yield focus list is a floor, not a ceiling.**
  The round agent is expected to hand-roll many more adversarial scenarios beyond what that list names:
  crash/rebirth, cross-worktree scope, dead-but-present state, mid-op-failure orphans and rapid churn.
- **Teardown discipline.**
  If you start any substrate server/session, tear it down and confirm zero stray processes at the end.
  "No stray state" is itself an invariant under test.
- **Grow whatever scenario record you keep with the module.**
  If a maintained SANDBOX-suite file exists for the module, extend it when a round surfaces behavior it doesn't cover.
  Keep `sandbox_coverage_test.go` green.
  A bug found live should leave behind a live regression test, whether or not a suite file exists.
  Its first statement is `tiergate.Require(t, 2)`, or `tiergate.Require(t, 3)` when it spawns a real LLM.

## Instantiating this for a new module

1. Fill every `<PLACEHOLDER>` in a copy of [`review-prompt-template.md`](review-prompt-template.md).
   Write the copy to `_mill/<module>-review-prompt.md` and commit it.
   The fill-ins are what to read, the high-yield focus list, the exact test commands and the substrate-teardown check.
   The high-yield focus list is where *this* module's bugs live.
2. Confirm the module already satisfies sandbox suite coverage.
   Coverage means a `**Covers:** <module>` tag somewhere under `tools/sandbox/*SUITE.md`.
   That invariant is independent of this method.
   Do NOT build a new dedicated suite file or launcher just to satisfy this hardening loop.
   The round agent drives the real CLI directly, whether or not a dedicated suite file exists.
   See "Driving the real substrate" above.
3. Run the loop: seed → spawn (rotate model + effort) → independently verify → re-seed.
   Repeat until a safety pass finds nothing and your gates agree.
   Then do any operator-assisted step the harness can't reach headlessly, and merge.
   For reed that step is the visual `attach` test in a real TTY.

## Worked example — the reed campaign (the evidence this works)

Seven serial rounds rotated models and chipped down one bug class each.
Failure severity fell monotonically until it hit zero:

| Round | Model | Effort | What it closed |
|------:|-------|--------|----------------|
| R3 | Opus  | n/a | `down` reap of pane children (left `remove`/churn leaking) |
| R4 | Fable | n/a | shared `descendantClosurePIDs`/`reapPaneChildren` seam for `down`+`remove`; dash-leading cmd escape; anchor validation (residual under concurrency) |
| R5 | Opus  | n/a | traced the real hub holder via PEB cwd; closed the tmux-**server** leak with saturation-tolerant deadlines (residual = pure timeout-under-saturation) |
| R6 | Fable | n/a | **F1** zero-pane zombie (empty-layout apply destroyed every pane) + **F11** positional select-layout reaping a tracked pane — two *new product* bugs prior rounds missed; plus hardening (F5/F6) and harness (F2/F3/F4) |
| R7 | Opus  | n/a | safety pass — **no new defects**; independently confirmed merge-ready |

R3, R4 and R5 each self-reported "merge-ready" and each was wrong.
The orchestrator's independent verification caught every residual.
R6 was the first round to survive verification.
R7, a belt-and-suspenders safety pass, agreed: clean.
So did the orchestrator's gates and a live operator-assisted `attach` test.
That convergence is the bar this method is built to reach.
It comprises the round verdict, independent gates and live operator sign-off, across rotated models.

### Why fix every finding, including NITs — not just BLOCKING/MEDIUM

A prompt that requires fixing only higher-severity findings lets NIT/LOW findings re-surface or vanish in later rounds.
The round count rises.
Fixing everything a round finds, at all severities, in the same round keeps the count down.
The template therefore requires fixing every recorded finding, including NITs;
severity affects how a finding is reported, not whether it gets fixed.

**This is a severity rule, not a size rule: a large finding is a separate case.**
A NIT still gets fixed inline, in this round.
A finding whose fix is LARGE does not belong in a round's commit-per-fix loop, whatever its severity label.
Large means a subsystem or feature addition, or a cross-cutting refactor reaching outside the module.
Record it like any other finding and mark it NOT-FIXED-THIS-ROUND.
The orchestrator spins it into its own mill-wiki task, so a hardening round does not grow into feature work.
See `orchestrator-prompt.md`'s Hard Rule 5 for the full instruction.

## Worked example — the fabric campaign (where the method was refined)

Six serial rounds against `fabric` after the slice 1–10 v2 rewrite.
81 findings, 9 BLOCKING, 8 of them data-loss.

| Round | Model | Effort | What it did |
|------:|-------|--------|-------------|
| R1 | Opus  | high   | 23 findings; self-reported "ready to merge" and the operator rejected that outright |
| R2 | Fable | high   | 17 findings; `pull` destroying uncommitted code-side work via `ResetHard` while returning `ok:true` |
| R3 | Opus  | high   | 6 findings; hostile-slug `remove`, plus a 40-cell dirty-worktree matrix |
| R4 | Opus  | medium | **three graded sweeps instead of a fourth review** — closed two defect classes with counted evidence |
| R5 | Opus  | medium | 15 findings, 2 BLOCKING, in three regions no prior round had ever driven |
| R6 | Fable | high   | two graded sweeps; **no BLOCKING and no MEDIUM** — first round to meet the bar |

Every round self-reported some form of "all green, all fixed".
**Six for six, the orchestrator's independent verification found something material that the round's report missed.**
One such find, after R3, was a BLOCKING data-loss bug in a file R3 had been editing.

### The refinements this campaign produced

**1. Pre-count the ground truth BEFORE spawning the round — and record the count's blind spots next to the number.**
When a round must enumerate a class, the orchestrator counts it first, into a file the round never sees.
A total *below* the pre-count is the truncation signal; a total *above* it is the correct direction.
Without this, "I enumerated 28 sites" is unfalsifiable.

The blind-spot half matters as much as the number.
A grep is only as good as its pattern.
An `os.RemoveAll` pattern cannot see calls routed through a seam,
and a line count cannot tell code from a comment that mentions the same identifier.
**Twice in this campaign the round corrected the orchestrator's count rather than the reverse.**
That is the round working, not failing.
Write down what the pattern cannot see.
A later exact match is then not mistaken for agreement when it should have been a correction.

*Measured effect.*
Across this campaign the pre-count **detected zero truncations**: every round met or exceeded every number.
It also caused one wrong conclusion.
The orchestrator had predicted that a specific blind spot would force R6's `os.WriteFile` total above the pre-count,
and when R6 reported an exact match, accused it of adopting the number rather than counting.
R6 had decomposed the total correctly (8 call sites plus 3 comment mentions); the prediction was what was wrong.

What the pre-count demonstrably DID deliver differed from its stated purpose.
It forced rounds to **explain deltas instead of reporting totals**.
R5 wrote out why its 388 err-check blocks were more accurate than the orchestrator's 304.
The simpler grep could not see 108 inline `if x, err := f(); err != nil` lines.
It also got the orchestrator's own numbers corrected twice.

Keep doing it: it is cheap and it makes reports checkable.
Do not claim it catches truncation on this evidence.
A deterrent cannot be measured by how often it fires,
and this campaign cannot establish whether no round truncated *because* it knew it would be counted.

**2. When the tail starts circling, stop reviewing and start counting.**
By R3 the findings were still real but were repeating two or three recognisable shapes.
A fourth broad review would most likely have produced a fourth variant.
R4 was therefore given *sweeps* instead.
A sweep enumerates every site in a class.
It presents a table with a row for every site, **including the ones judged correct, with the reason**.
It states the total and the enumeration method as reproducible shell.

The sweep found a defect at a site the orchestrator's own worked example did not point at.
That is the whole argument for counting a class rather than fixing its instances.
It was also **cheaper** than a broad review, at 428k tokens against 535k.
One file read covers many table rows, and the enumeration is scripted rather than performed per site by the model.

**3. Derive each round's assignment from the previous round's residue — never "review it again".**
R4 became sweeps because R3 was circling.
R5 became adversarial work on three regions nobody had driven, because R4's sweeps had closed the countable classes.
R6 became two new sweeps: R5's two BLOCKING findings were interesting as *shapes* rather than as bugs,
and both shapes were unenumerated.
Each round's yield came from a region the previous round's *method* could not reach, not from luck.

**4. Grade an adversarial round on evidence of execution, not on argument.**
A race reasoned about but never made to happen is not a finding.
Require the interleaved run behind every concurrency claim.
Require a **strictly sequential control** of the identical sequence.
Require a **reproduction on a second independent hub**.
The control turns "the file got corrupted" into "the file got corrupted *because of concurrency*";
the second hub turns an anecdote into a finding.

**5. Prove the scenario actually reached the code before believing a clean result.**
This is the sharpest lesson of the campaign, and both the orchestrator and a round fell into it.

While verifying R5's `.git/info/exclude` fix, the orchestrator ran a six-way concurrency storm that stayed green.
It stayed green *with the lock and the atomic write both sabotaged*.
The scenario had never entered the write path:
sibling worktrees kept every exclusion, so the read-modify-write short-circuited on "nothing changed" and never wrote.
R6 then reproduced the same mistake independently:
its two `.git/info/exclude` table rows were marked "driven — correct" on the strength of concurrent verbs.
On an established hub, those verbs do not touch that file at all.

**A green run that never executed the code is indistinguishable from a fix.**
Reporting one as the other is the worst error available to a verifier.
The check is cheap:
sabotage the mechanism and confirm the scenario now fails, or instrument the write and confirm it happened.
If the scenario stays green under sabotage, the scenario is unproven — not the code.

**6. Read the neighbouring code while preparing each sabotage.**
The single highest-yield habit the campaign produced.
Reverting a hunk to watch a test fail means reading closely around it.
That is how the orchestrator found a BLOCKING data-loss bug in `prune` that three dedicated review rounds had missed.
Three rounds of looking for bugs did not find it;
ten minutes of reading one file closely enough to neuter a single line did.

**7. Seed cost rules — with an explicit guardrail against under-driving.**
Rounds cost 300–535k tokens each, roughly 70% of it tool results rather than reasoning.
Cost rules cut the cost of the largest round by 40% with no loss of coverage.
They said to re-verify narrowly and never read a verbose command's full output back.
They said to read large files with offset/limit, batch independent shell work, and script repetitive fixtures.

They must ship with the guardrail, verbatim in the prompt:
**this is about removing waste, never about driving less**.
They also need a "what is NOT waste" list.
The list names the independent fixture per destructive scenario and the sabotage proofs.
It also names the live reproduction of every BLOCKING finding and the full hermetic gates at the end.
A round that saves tokens by reading code instead of driving it has failed, and its findings get rejected.

**8. Never characterise a round's work without reading its "what was tested" section.**
The orchestrator once told the operator that one round "drove live where the other read code".
That was false and had not been checked: both had driven live, and the actual difference was much narrower.
Check before summarising;
a wrong characterisation of a round misdirects every round after it.

### What a converged round looks like, and what it still does not prove

R6 met the bar: no BLOCKING, no MEDIUM, gates reproduced by the orchestrator, working tree provably untouched.

R6's scope was two enumerated classes, so its zero says those two classes are clean, not that the module is.
Its own method failed to demonstrate itself on the two calibration rows (see refinement 5),
so its zero is weaker evidence than a zero from a method proven able to detect.
After six rounds the campaign also still carried a named, never-executed gap:
Windows path behaviour, unreachable from a Linux host and reasoned about rather than driven in every round.

State those limits in the convergence verdict.
A campaign that ends by claiming more than it proved teaches the next campaign to do the same.
