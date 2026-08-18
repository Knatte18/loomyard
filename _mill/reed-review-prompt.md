# `reed` — independent review + fix (round 5 — scoped state-loss/corruption recovery pass)

> Filled instance of `crucible/review-prompt-template.md` for this crucible campaign's first module, rewritten fresh for round 5 (round 1's version is preserved in git history at commit `c0569063`; round 2's at `b38f58c3`; round 3's at `cdd8e107`; round 4's at `282abf12`).
> See `crucible/README.md` for the loop this prompt runs inside, and `_mill/reed-shuttle-HANDOFF.md` for campaign-wide state.

You are a senior engineer doing an INDEPENDENT review of the `reed` module (`internal/reedengine` + `internal/reedcli`, plus `internal/hubgeom`) in the loomyard repo, followed by FIXING what you find.
Work in the worktree at `/home/knatte/Code/loomyard/wts/reed-shuttle-crucible-hardening` (branch `reed-shuttle-crucible-hardening`).

**This round is DIFFERENT in shape from rounds 1–4: it is narrowly SCOPED, not a general safety pass.** Four general rounds have progressively hardened the told-geometry construction seam, the tmux-identity pre-flight, reap-root purity, and TmuxCmd discipline — re-reviewing those broadly would likely just re-walk ground four rounds have already covered. Round 4 surfaced a genuinely new, systematically-unprobed surface — **state-loss and state-corruption recovery** — and that is this round's entire mandate. See "Scope" below; do not wander outside it without a concrete reason tied to what you find inside it.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of reed's state-loss/corruption recovery behavior.
   Hunt for bugs by reading the code AND by driving the real substrate (real tmux, resolved on `PATH` — `tmux 3.6` is installed on this host) — this is where the defects hide.
2. FIX: after you have a findings list, implement the fixes one at a time, verify each against the real substrate, keep the whole test suite green, and update the docs in the same change as the fix they document.
   COMMIT after each individual fix lands green (see "Commit per fix" below).
   Do NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test, plus the live smoke/suite check if the finding needed one), and its doc update (if any) is included, COMMIT it — on the current branch, no push — before starting the next finding.
Commit message format: `reed: fix <finding-id> — <one-line what/why>`.
Also commit `_mill/reed-review-r5.md` and `_mill/reed-review-r5-fixer-report.md` as you write or update them — they are NOT gitignored scratch, they are the campaign's durable record.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to `_mill/reed-review-r5.md` and committed — before you touch (edit, create, or delete) a single production or test file.
Do not fix findings as you go, even ones that look small and obviously right.
Write it down as a finding, keep reading, finish the review, save the file, THEN start Job 2.
(Round 2 and round 4 each recorded one legitimate exception: a finding surfacing during Job 2's own concurrent-verification sweep, which cannot by definition arrive during Job 1. If the same happens to you, record it with its provenance stated plainly — do not hide it in the fixer report.)

## Log as you go during Job 1 (BLOCKING)
Append your observations to `_mill/reed-review-r5.md`'s "What was tested" section immediately after each command/scenario returns.
Jot each finding into the file's findings section provisionally as you spot it.
COMMIT each meaningful append (`reed: review notes — <what you just appended>`).

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first.
Do NOT read any prior review or review-dialogue files before you have your own list.
Specifically do not open anything under `_mill/` matching `reed-review-*` — this is a FILENAME PATTERN, not a content judgment, so it covers rounds 1–4's review reports and fixer reports, AND the orchestrator's own running handoff note (`_mill/reed-shuttle-HANDOFF.md`) even though its name doesn't match the pattern literally — treat it as equally off-limits until your own findings list is complete. Do not open any of these out of curiosity, and do not act on anything you might glimpse in them even by accident.
Reading the design SPEC and the module docs is expected and required (those are not reviews).
AFTER your own findings are written, you MAY consult rounds 1–4's material (all 23 findings across four rounds CLOSED-AND-VERIFIED — see "Round context" below for exact commit shas) to confirm those previously-fixed behaviors have not regressed, and to re-evaluate anything deferred (nothing was deferred from any round).

## Scope — this is the whole mandate, not a floor to expand from

Round 4 fixed a permanent wedge (R4-F4/R4-F5) reachable when `.lyx/reed.json` disappears while its tmux session keeps running (a `git clean -xdf` in the worktree does this, and `.lyx` is never-tracked machine-local scratch by invariant — this is a SANCTIONED operator action, not misuse). It fixed exactly ONE loss mode: the file deleted wholesale. The orchestrator's independent verification confirmed that fix holds for that one mode, and separately confirmed via `hubgeom`/`lyxcwd` reading that R4-F3's `AnchorPath` backstop gap is real but currently unreachable in production (hub mode always supplies an absolute path).

Four concrete neighboring loss/corruption modes are named, untested by any of rounds 1–4, and are your primary hunting ground:

1. **`reed.json` truncated or corrupted mid-write** — a crash, a full disk, or a `kill -9` of the `lyx` process during `SaveState` leaves a partial or invalid-JSON file on disk. What does the NEXT `reed up`/`resume`/`status`/`add` do? `LoadState` (`internal/reedengine/state.go` or wherever it lives — find it) is the read path to scrutinize.
2. **A stale/older `reed.json` restored while panes have moved on** — e.g. a backup tool, a `git stash pop` that resurrects an old untracked-but-present copy (unusual but not impossible if `.lyx` was ever accidentally committed then later force-removed), or an operator hand-copying a `.lyx` directory between worktrees. The persisted `PaneID`s and `HeaderPaneID` no longer correspond to the panes that actually exist in the live tmux session. Does reconciliation (`reconcileApplyPersistLocked`, `reconcileLocked`) treat "recorded pane doesn't exist" and "recorded pane exists but is a DIFFERENT, unrelated live process" as the same case? Should it?
3. **`.lyx` deleted wholesale — including the LOCK file — while an operation is genuinely in flight.** Round 4 tested `reed.json` gone with the session idle between commands. This is narrower and nastier: what if the deletion races an in-progress `withOpLock`-protected operation? Does the lock file's absence let a second concurrent op start against inconsistent on-disk state? (You do not need real concurrent processes for every angle here — reasoning about the lock's fstat/create semantics plus a targeted repro is enough; say plainly which parts you reasoned about vs. drove live.)
4. **A `reed.json` written by one worktree, read after that worktree directory was renamed.** Round 3 (R3-F2) fixed the `Socket`/`Session` diagnostic fields going stale on rename — but the session/socket the ENGINE actually targets comes from told `Geometry`, freshly derived from the CURRENT worktree path on every invocation (per `hubgeom`), not from the stale file. Confirm that holds — i.e. that R3-F2 was a diagnostic-only staleness, not a targeting bug — and then ask: does anything ELSE in `ReedState` (strand records, `PaneID`s) become meaningless-but-silently-accepted after a rename, even though the socket/session targeting itself is fine?

For each of these: drive it live where you can (build the fixture, do the thing, observe what `reed` actually does), and where a live drive isn't practical (e.g. genuine process-level concurrency), reason from the code and say so explicitly rather than skipping it silently.

Also **re-verify R4-F4/R4-F5's fixes hold under scenario 2 and 3 above**, not just the exact one-file-deleted-while-idle case round 4 itself drove — those two fixes are the most directly adjacent code to this round's whole mandate, so confirm they generalize rather than assuming it.

## Explicitly OUT of scope for this round
- A general re-review of told-geometry field integrity, the tmux-identity pre-flight (`validateToldTmuxIdentity`), reap-root purity, or TmuxCmd discipline — all four rounds have hardened these repeatedly; do not re-walk them absent a concrete finding that specifically implicates one.
- `internal/shuttleengine`/`internal/shuttlecli` — a SEPARATE, later crucible campaign. Do not review or fix shuttle code this round even though it depends on reed.
- `internal/hubgeom`'s future siblings (wave 3, T6/T7) do not exist yet.
- The `layout`→`location` rename follow-up in `internal/burlercli`/`internal/shuttlecli`/`internal/perchcli`/`internal/webstercli` — round 2 (R2-F5) deliberately scoped this to `internal/reedcli` alone; not reed, do not touch.
- Windows-specific tmux/path behavior — this host is Linux; a Windows verification gap is expected and should be named, not driven.

## Round context seeded from prior-round verification

Rounds 1 (`opus-medium-r1`, 10 findings), 2 (`opus-high-r2`, 6 findings), 3 (`fable-high-r3`, 2 findings), and 4 (`opus-medium-r4`, 5 findings) are all CLOSED-AND-VERIFIED — full details in `_mill/reed-review-r{1,2,3,4}.md` / `-r{1,2,3,4}-fixer-report.md`, readable only AFTER your own findings list is complete (see "Clean-room review constraint" above).

The orchestrator independently re-verified round 4 from a cold state (forked sub-agent, full protocol): reran every hermetic gate (`go build`/`vet` clean; `-count=5` on reedengine/reedcli/hubgeom/cmd-lyx all `ok`; whole-repo `go test ./...` `ok`; `-tags integration` `ok`; `./deploy-dev` redeployed clean), full tmux-only smoke suite (23 PASS / 1 SKIP, matching the fixer report exactly), SABOTAGE-PROVED all five of round 4's fixes by reverting each in turn and confirming its exact regression test(s) failed at the intended assertion, then restoring and confirming an empty diff. Independently live-drove R4-F1 (backslash session-name refusal) and R4-F4 (scrubbed-state header recovery) on brand-new fixtures built from scratch, different names from round 4's own (`zetahub-HUB/core\prod` and `omegahub-HUB/svc-recover` respectively) — both held exactly as claimed. Independently confirmed, by reading `lyxcwd.go` rather than taking the report's word, that R4-F3's "hub mode cannot reach an unreachable AnchorPath" claim is structurally true (`git rev-parse --show-toplevel` always returns an absolute path). Found one real teardown gotcha along the way, carried forward below.

**IMPORTANT teardown correction, discovered during round 4 and confirmed during its independent verification:** `pgrep -x tmux` reports **zero even while a tmux server IS genuinely running**, because a tmux server's process `comm` is literally `tmux: server`, not `tmux`. Use `ps -eo comm | grep -cx 'tmux: server'` (must be `0` for a clean teardown) or `tmux -L <socket> ls` (must report `no server running`). Do not rely on `pgrep -x tmux` alone as teardown evidence — it will falsely read clean.

**CLOSED-AND-VERIFIED from round 1, do NOT re-litigate:**
- F1 (BLOCKING) — pane-child reaping was inert. Fixed via `proc.IsAlive` polling. Commit `d0bbbc82`.
- F2 (MEDIUM) — strand-pane `split-window` missing `-c AnchorPath`. Commit `a6bcb308`.
- F3 (MEDIUM) — `TestSmokeClaudeResumeRecallsCodeword` now pins `--model haiku`. Commit `61f0407d`.
- F4, F8 (LOW/NIT) — commit `0373f7fd`. F6, F7, F9, F10 (NIT) — commit `07883759`. F5 (LOW) — commit `ab6f8fdc`.

**CLOSED-AND-VERIFIED from round 2, do NOT re-litigate:**
- R2-F1 (BLOCKING) — `.`/`:` in a worktree name hung `reed up` 20s and stranded an untearable session. Fixed via `validateToldTmuxIdentity`. Commit `ae96397f`.
- R2-F2 (MEDIUM) — `Down`'s reap-root snapshot included dead panes. Fixed via `safeReapRoot`. Commit `530a1602`.
- R2-F6 (MEDIUM) — the capability probe bypassed `TmuxCmd`. Fixed. Commit `28c3aa0f`.
- R2-F3 (LOW) — a hub at filesystem root yields a `/`-containing socket key. Fixed. Commit `e9ad525f`.
- R2-F4, R2-F5 (NIT) — stale doc claims / retired `layout` vocabulary. Commits `3a40c8cb` / `eff5eda6`.

**CLOSED-AND-VERIFIED from round 3, do NOT re-litigate:**
- R3-F1 (MEDIUM) — the identity pre-flight missed tmux's vis-encode rewrite class (control chars, DEL, invalid UTF-8). Fixed. Commit `12ca3ea5`.
- R3-F2 (NIT) — `reed.json`'s `Socket`/`Session` diagnostic went stale after a worktree rename; now re-stamped on every load. Commit `7af91088`. **This round's scenario 4 above builds directly on this — confirm it, don't just cite it.**

**CLOSED-AND-VERIFIED from round 4, do NOT re-litigate (but scenarios 2/3 above ask you to re-verify these GENERALIZE):**
- R4-F1 (MEDIUM) — the identity pre-flight missed tmux's third rewrite class: backslash doubling. An exhaustive printable-ASCII sweep now proves `.`, `:`, `\`, plus control/DEL/invalid-UTF-8 is the complete rewrite surface of tmux 3.6. Commit `621165ce` (+ `b3c74d54` message fix).
- **R4-F4 (MEDIUM) — the direct ancestor of this round's mandate.** A lost `.lyx/reed.json` while the tmux session stayed up left the header pane permanently unsplittable (`no space for new pane` against a 1-row band), wedging `up`/`resume` forever while `status` reported the session healthy. Fixed by retrying the header split once behind a `select-layout even-vertical` re-tile. Commit `9ec33880`.
- **R4-F5 (LOW) — surfaced verifying R4-F4's fix.** Pane adoption guessed which of several untracked panes was idle, sometimes typing a strand's command into a pane already busy running `header --blocking`, then reporting a nonexistent process `live:true`. Fixed by narrowing adoption to the sole-alive-non-header-pane case. Commit `be746b85`.
- R4-F3 (LOW) — the told-geometry backstop covered 2 of 7 fields; `AnchorPath` non-absolute now refused too. Commit `cb935402`. Currently unreachable in production (confirmed above); still worth having as a backstop.
- R4-F2 (NIT) — `ServerName` now bounds its readable half so a long hub basename can't overflow `sun_path`. Commit `b3f58a2b`.

**Attribution note, for context only:** none of round 4's findings trace to a wave-1 commit (`2b21ee57`/`5b096ebd`/`b98ee2ba`) — `ServerName`, the identity pre-flight's ancestry, and `ensureHeaderPaneLocked` all predate wave-1's seam swap. Not a reason to narrow this round's scope.

**Your mission this round:** this is the campaign's fifth round, beyond the originally planned four-round rotation — the operator chose to spend it specifically because round 4 surfaced a new, under-explored defect class (state-loss/corruption recovery) rather than a variant of an already-closed one. Model/effort: Opus / High.
- Work through the four scenarios in "Scope" above, live-driving what you can, reasoning explicitly about what you can't.
- R4-F5's false `live:true` is the concrete reason this class of bug matters beyond hygiene: it is the one symptom an orchestrating caller (shuttle, a future campaign) cannot defend against from the outside. Weight findings that produce a false-healthy report higher than ones that merely fail loudly.
- **This round's outcome decides whether the campaign ends here.** If the four scenarios turn up nothing real, say so plainly and explicitly recommend convergence — that is a valuable, expected outcome of a scoped pass, not a failure. If you find something real, fix it with the same rigor as every prior round.

State the **merge bar** so you calibrate: correctness in the NORMAL single-instance flow is the gate; deliberately induced state loss/corruption is exactly this round's mandate (unlike prior rounds' concurrency stress, which was a diagnostic amplifier, not the gate itself) — a defect here IS a normal-flow defect, because `git clean -xdf` and process crashes are ordinary operator/environment events, not adversarial misuse.

## Live-substrate cost declaration
`LLM-DRIVING: PARTIAL.` Reed's smoke suite is overwhelmingly real-tmux-only and cheap (a stray pane costs nothing) — `-run Smoke` broadly is fine for those. ONE exception: `internal/reedcli/smoke_resume_test.go`'s `TestSmokeClaudeResumeRecallsCodeword` launches exactly ONE real `claude` subprocess per invocation (it needs a logged-in claude CLI; it will self-skip if none is configured).
- **You MUST pass `--model haiku` (or the cheapest available model) for the real `claude` process this test launches, and for any real `claude` process you spawn yourself in an ad-hoc scenario — this is an explicit operator instruction for this campaign, not a suggestion.** Only deviate if a specific finding genuinely requires testing model-specific behavior, and say so explicitly if you do.
- Do not run this one test as part of a bare, broad `-run Smoke` sweep casually many times — run it deliberately, by its exact name.
- This round's scope is unlikely to need it at all (it targets state-loss recovery, not claude-resume behavior) — skip it entirely unless a finding specifically requires it.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/...`
- `go test -count=5 ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/... ./cmd/lyx/...`

Live smoke (real substrate, behind the `smoke` build tag):
- `go test -tags smoke ./internal/reedcli/... -run Smoke -skip ClaudeResume -v -count=1` is fine as written for the tmux-only tests.
- tmux is resolved via `PATH` on this host (`/usr/bin/tmux`, 3.6). No pwsh/Windows tooling involved.

Live driving — YOU drive it directly, no launcher (PRIMARY — where the bugs surface):
- Deploy the current source as the dev binary: `./deploy-dev` (POSIX script at repo root). **FOOTGUN:** live driving runs the DEPLOYED snapshot — re-run `./deploy-dev` after EVERY source change or you validate a stale binary.
- Do NOT invoke `sandbox-reed-suite.cmd`. Run the real `lyx reed` CLI commands yourself, directly, foreground, waiting for each to return.
- The four scenarios in "Scope" are a FLOOR for this round — if driving them surfaces a fifth related loss mode, follow it.
- "Headless" means "no human required", not "no time/token cost to me." A real tmux scenario takes real wall-clock time — that is expected and budgeted for.

TEARDOWN DISCIPLINE (critical): if you start any tmux server/session, tear it down. At the end, confirm ZERO stray tmux processes using `ps -eo comm | grep -cx 'tmux: server'` = 0 and/or `tmux -L <socket> ls` = "no server running" for every socket you used — **`pgrep -x tmux` alone is NOT sufficient evidence; it falsely reads clean while a server runs** (see the teardown correction above). Leave no stray state. Be honest about what you could NOT verify and why.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/traced) vs PLAUSIBLE (looks wrong, unverified).

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings you record get fixed in Job 2 — including every NIT.
The only legitimate reason to leave a finding unfixed is that fixing it genuinely requires something you cannot do alone this round — say so explicitly in the fixer report's deferred section.

## Deferred items from prior rounds — RE-EVALUATE these
None — no round 1–4 deferred anything (all four fixer reports' "Deferred: Nothing").

## Fixing — after the review
- Fix EVERY finding from your review, all severities including NIT.
- Load the code-quality guidance (`/code-quality` skill) AND `golang:golang-build`/`golang:golang-testing`/`golang:golang-comments` before editing — ALL of them, not code-quality alone.
- For every bug you fix, add or extend a test that would have caught it. For a live-only defect, add a `//go:build smoke` test.
- MAKE SMOKE TESTS DETERMINISTIC — poll on actual state transitions with a deadline, never sleep a fixed amount.
- If `tools/sandbox/SANDBOX-REED-SUITE.md` needs extending because a review surfaces a live/visual behavior it doesn't cover, extend it (keep `sandbox_coverage_test.go` green). Otherwise note the new scenario in your fixer report.
- Keep `go build`/`vet`/`test` green after every change. Then RE-DEPLOY (`./deploy-dev`) and re-run every live scenario yourself, directly.
- Update `docs/overview.md` (reed's bullet) / `internal/reedengine/doc.go` / `CONSTRAINTS.md` IN THE SAME change if invariants or scope move. Do NOT add bugfix/hardening notes to `manifest/roadmap.md`.
- Tear down all tmux state; confirm zero stray processes. COMMIT each fix as you finish it. Do NOT push.
- Report the changed files and how you verified each fix.

## Deliverables
1. Structured review report → `_mill/reed-review-r5.md`, committed incrementally per "Log as you go" above.
2. Fixer report → `_mill/reed-review-r5-fixer-report.md`, committed (folding into a fix commit is fine).
3. Final chat message: concise executive summary + counts by severity + the two report paths + an explicit merge-readiness verdict, AND an explicit convergence recommendation (say clearly whether you believe reed is done or whether you'd want another look, and why). Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive the real substrate), produce your independent findings, then implement and verify the fixes.
