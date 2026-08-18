# `shuttle` — independent review + fix (round 4 — close R2-F11 + three named joint-composition scenarios)

> Filled instance of `crucible/review-prompt-template.md` for shuttle's campaign, rewritten fresh for round 4 (round 1's version preserved in git history at commit `33a14555`; round 2's at `fa93d9f5`; round 3's at `06719ef2`).
> See `crucible/README.md` for the loop this prompt runs inside, and `_mill/reed-shuttle-HANDOFF.md` for campaign-wide state.

You are a senior engineer doing an independent review + fix pass on the `shuttle` module (`internal/shuttleengine` + `internal/shuttleengine/claudeengine` + `internal/shuttlecli`) in the loomyard repo.
Work in the worktree at `/home/knatte/Code/loomyard/wts/reed-shuttle-crucible-hardening` (branch `reed-shuttle-crucible-hardening`).

**This round is scoped differently from rounds 1-3.** It is NOT a fresh open-ended clean-room sweep. The operator capped the remaining campaign at **up to two more rounds, this one and possibly one more, then shuttle is done regardless of outcome** (short of a BLOCKING finding). Round 4's mandate is two specific, named items:

1. **Close R2-F11 properly** — a real design+validate step, done as this round's dedicated work.
2. **Three joint-composition scenarios round 3 explicitly named as untested successors** — not a new open hunt, these three specifically.

Anything else you notice at the normal bar gets recorded/fixed too (small findings fixed inline, all severities including NIT; a genuinely new LARGE finding named NOT-FIXED-THIS-ROUND) — but do not go looking for an unrelated fresh full-module sweep. Three prior rounds (21 findings, all fixed) already did that; this round's job is these two focus items.

## Your two jobs, in order
1. REVIEW: form your own independent judgment on the two focus items below (plus anything else you notice along the way).
2. FIX: after your findings list for this round is complete, implement fixes one at a time, verify each against the real substrate, keep the whole test suite green, update docs in the same change as the fix they document.
   COMMIT after each individual fix lands green (see "Commit per fix" below).
   Do NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test, plus the live smoke/live-TUI check if the finding needed one), and its doc update (if any) is included, COMMIT it — on the current branch, no push — before starting the next finding.
Commit message format: `shuttle: fix <finding-id> — <one-line what/why>`.
Also commit `_mill/shuttle-review-r4.md` and `_mill/shuttle-review-r4-fixer-report.md` as you write or update them.

## Focus item 1 — close R2-F11 (this is the round's headline deliverable)

`internal/shuttleengine`'s `sendVerified` (the mechanism `Interrupt`/`Send`/`Inject` use to confirm delivery) verifies by counting occurrences of a needle string in the tmux-captured viewport before and after sending. Round 2 partially fixed a false-negative shape (re-baseline when the count drops below baseline, commit `cdb22bf8`) but named a residual, LOW-severity, previously-classified-LARGE gap: **when an earlier occurrence scrolls off-viewport at the exact moment the new occurrence arrives, the count stays unchanged — indistinguishable from "nothing was delivered" using a count alone.**

The operator, asked directly whether this really warrants a separate mill-wiki task, concluded no: the fix is contained to one function/file, doesn't reach outside the module — the earlier LARGE classification was argued on RISK (replacing a live-proven detector, failing silently if the replacement is wrong), not on SIZE per Hard Rule 5's actual text. **Your job this round is to close it as a proper, validated fix — not a rushed inline swap, but also not spun into a separate task.**

What "properly" means here:
- Design a replacement or supplement to the count-only check that is **position-aware** — e.g. checking the needle appears in the tail region of the captured viewport relative to where the baseline capture ended, or attaching a per-send unique sentinel (a short random/incrementing marker embedded in what's sent, distinguishable from any prior occurrence) and waiting for THAT specific instance rather than a generic count. Pick whichever approach you can actually validate live; state your reasoning for the choice.
- **Validate LIVE against a real Claude TUI under genuine viewport churn** — not just a hermetic test with a synthetic/fake pane buffer. You need to actually reproduce viewport scroll (e.g. a long-running agent producing enough output to scroll the needle off) and confirm the new check gets it right where the old count-only check would have been ambiguous or wrong. A hermetic test alone is not sufficient evidence for this finding; the whole point is that the old mechanism's failure mode only shows up under real terminal scroll dynamics.
- Because this replaces a live-proven guarantee whose failure direction is silent (a caller believes delivery succeeded when it didn't), be conservative: prefer a fix that provably NARROWS the false-negative window over one that changes the check's fundamental semantics in a way you cannot fully validate live in the time available. If you cannot get full confidence in a complete fix, it is legitimate to land a real, tested IMPROVEMENT (e.g., the sentinel approach for the specific case you can validate) and honestly name what residual (if any) remains — do not overclaim "fully closed" if it isn't.
- Add a regression test. If the live-TUI failure mode cannot be made deterministic/hermetic, say so explicitly and document why, same as the existing precedent for live-only defects in this campaign (`smoke`-tagged tests where practical).

## Focus item 2 — three joint-composition scenarios (round 3's named untested successors)

Same driving style as round 3: start a real, live `lyx shuttle run` (`--model haiku`), let it reach a genuinely in-progress state, THEN trigger the reed-side event, observe what shuttle does.

1. **Subpath-anchored worktree geometry under joint stress.** Every fixture across rounds 1-3 has had `AnchorRel = "."` (the worktree root itself is the anchor). Construct a fixture where the anchor is a SUBDIRECTORY of the worktree root (`AnchorRel` is a real non-trivial relative path) and re-run at least one of round 3's mid-flight joint scenarios (e.g. `reed.json` deleted mid-run, or a worktree rename) against it. Does `validateToldPaths`' containment check, the run-dir root, and the fork-audit's transcript-directory derivation all still resolve correctly when anchor ≠ worktree root? This is the scenario shape `validateToldPaths` (R1-F3) was specifically built to guard against a SWAP of, but it has never been driven live with a genuinely non-trivial anchor.
2. **A `PaneGeneration` mismatch that reed CLEARS rather than refuses, with a live shuttle run attached.** Reed's own `PaneGeneration` mechanism (`generation.go`) has two outcomes for a stale/foreign binding: REFUSE (round 3 tested this shape via the foreign-session refusal) and CLEAR (reed silently drops the stale binding and treats the strand as gone, rather than erroring). Construct the CLEAR path specifically — read `generation.go` to find the exact condition that triggers clear-not-refuse — while a shuttle run is live and polling. Does shuttle's `Wait` (post-R3-F1 fix) correctly classify this as the "reed no longer tracks this strand" mechanism failure, or does the CLEAR path produce some third shape neither R3-F1's fix nor the original `died` classification anticipated?
3. **Concurrent `Interrupt` and `Wait` genuinely racing against a reed that is mid-refusal** — not sequential (issue one, wait for its result, issue the other), actually concurrent: start a live run, get reed into its foreign-session-refusal state (rename the worktree, same trigger as round 3 scenario 1), then fire `lyx shuttle interrupt <guid>` and let `Wait`'s own in-process liveness poll land at roughly the same moment (you may need to script this — e.g. background both, or reduce `PollIntervalMS` via a test config to shrink the race window). Does anything double-fire, double-log, or produce an inconsistent pair of results (e.g. `Wait` and `interrupt` disagreeing about whether the run is alive, dead, or refused)?

Judge each the same as any finding: severity, CONFIRMED/PLAUSIBLE, small vs. large. A clean result (shuttle correctly composes with reed here too, no defect) is a valuable, explicit result to record — do not manufacture a finding to have something to report. If a scenario reveals what looks like a genuine REED defect, name it as an OUT-OF-CAMPAIGN finding (reed's campaign is closed, do not fix reed here) but do not suppress it.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE for both focus items — full review report SAVED to `_mill/shuttle-review-r4.md` and committed — before you touch (edit, create, or delete) a single production or test file. Exception: focus item 1 (R2-F11) is a KNOWN, already-named finding from round 2, not something to rediscover — you may go straight to designing/prototyping candidate fixes for it as part of Job 1's investigation (to find out what's actually validatable live), but do not COMMIT a production fix until Job 1's report is otherwise complete and committed.
Do not fix focus item 2's findings as you go, even ones that look small and obviously right.

## Log as you go during Job 1 (BLOCKING)
Append your observations to `_mill/shuttle-review-r4.md`'s "What was tested" section immediately after each command/scenario returns. Jot each finding into the file's findings section provisionally as you spot it. COMMIT each meaningful append (`shuttle: review notes — <what you just appended>`).

## Clean-room review constraint (narrower than prior rounds — read this carefully)
This round is NOT clean-room in the rounds-1-3 sense, because its two focus items are already named by the operator, not things you're expected to discover blind. You MAY and SHOULD read:
- `_mill/shuttle-review-r2.md`'s R2-F11 section and `_mill/shuttle-review-r3.md` (all of it — this round builds directly on round 3's named successors) before starting.
- `_mill/reed-shuttle-HANDOFF.md` for full campaign state.
- Reed's current code/docs in depth, same as round 3 (`doc.go`, `CONSTRAINTS.md`, and specifically `generation.go` for focus item 2).

If, while doing this work, you notice something else that looks like a genuine new finding unrelated to the two focus items, record and (if small) fix it at the normal bar — but do not go hunting for more.

## What to read
- `internal/shuttleengine/run.go` (`sendVerified`, `Interrupt`/`Send`/`Inject`), `wait.go` (post-R3-F1 `checkLivenessTick`), `internal/shuttlecli` for the CLI verbs.
- `internal/reedengine/generation.go` in full — the CLEAR vs. REFUSE distinction for focus item 2.
- `internal/reedengine/doc.go`, `CONSTRAINTS.md`.
- `_mill/shuttle-review-r2.md` (R2-F11), `_mill/shuttle-review-r3.md` (all findings + "Not verified this round" section).

## Live-substrate cost declaration (BLOCKING)
`LLM-DRIVING: YES.`
- **`--model haiku` for every real `claude` process, no exceptions**, same standing rule as every round.
- Focus item 1 needs enough live runs to actually reproduce viewport scroll — budget up to 4-5 real `claude` processes for this alone (you may need several attempts to reliably produce scroll churn; a long-running, high-output prompt helps).
- Focus item 2's three scenarios: up to one real `claude` process per scenario (3 total), same single-process-at-a-time discipline as rounds 1-2 (no concurrent-agent budget this round — round 3 already spent its authorized 2-simultaneous exception on scenario 4; this round has no equivalent authorization).
- Record baseline `claude`/tmux process counts BEFORE driving anything; verify against that baseline after every scenario and at the end.

## What to TEST — do not just read, EXERCISE it
Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/...`
- `go vet -tags smoke ./internal/shuttlecli/...`
- `go test -count=5 ./internal/shuttleengine/... ./internal/shuttleengine/claudeengine/... ./internal/shuttlecli/... ./cmd/lyx/...`

Live smoke: `go test -tags smoke ./internal/shuttlecli/... -run <ExactTestName> -v -count=1` — one at a time, by exact name.

Live driving — YOU drive it directly:
- Deploy first: `./deploy-dev`. Re-run after EVERY source change.
- Real `lyx shuttle`/`lyx reed` commands, foreground, interleaved as each scenario requires.

TEARDOWN DISCIPLINE (critical): `ps -eo comm | grep -cx 'tmux: server'` = 0 (NOT `pgrep -x tmux` alone) AND zero new stray `claude` processes beyond your recorded baseline, after EVERY scenario and again at the end.

## How to judge each finding
`file:line`, concrete failure scenario, severity (BLOCKING/MEDIUM/LOW/NIT), suggested fix, CONFIRMED/PLAUSIBLE, and SIZE (small = fix inline; large = name it, mark NOT-FIXED-THIS-ROUND — though note focus item 1 is explicitly NOT to be deferred again; see below).

**R2-F11 is not eligible for NOT-FIXED-THIS-ROUND again.** The operator's decision this round was specifically to close it here rather than open a new task. If, after genuine investigation, you conclude a full close is not achievable in this round, land the best validated improvement you can and state clearly what residual risk (if any) remains — do not simply re-defer it a second time without a strong, explicit reason and operator-visible flag in the report.

## Fixing — after the review
- Fix every small finding, all severities including NIT.
- Load `/code-quality` AND `golang:golang-build`/`golang:golang-testing`/`golang:golang-comments` before editing.
- For every bug you fix, add or extend a test.
- MAKE SMOKE TESTS DETERMINISTIC — poll on actual state transitions, never sleep a fixed amount.
- Extend `tools/sandbox/SANDBOX-SHUTTLE-SUITE.md` if this review surfaces a live/visual behavior it doesn't cover.
- Keep `go build`/`vet`/`test` green after every change. RE-DEPLOY and re-run every live scenario yourself.
- Update `docs/overview.md` / `internal/shuttleengine/doc.go` / `CONSTRAINTS.md` IN THE SAME change if invariants or scope move.
- Tear down all tmux/claude state; confirm zero new stray processes. COMMIT each fix as you finish it. Do NOT push.

## Deliverables
1. Structured review report → `_mill/shuttle-review-r4.md`, committed incrementally.
2. Fixer report → `_mill/shuttle-review-r4-fixer-report.md`, committed.
3. Final chat message: concise executive summary + what happened with R2-F11 specifically (fully closed / improved-with-stated-residual, and why) + the three scenario results + counts by severity + any other findings + merge-readiness verdict + your own read on whether shuttle's campaign is ready to close after this round or whether one more narrowly-scoped round is warranted (the operator has capped the remaining campaign at one more round after this one, at most). Do not paste the whole reports.

Begin with focus item 1's investigation (read `sendVerified`, round 2's R2-F11 writeup, prototype and live-validate candidate fixes) and focus item 2's three scenarios, log findings as you go, then implement and verify fixes.
