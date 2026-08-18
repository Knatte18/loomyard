# `reed` — independent review + fix (round 6 — scoped: R5's new mechanism + wiring-test gaps)

> Filled instance of `crucible/review-prompt-template.md` for this crucible campaign's first module, rewritten fresh for round 6 (round 4's version preserved in git history at commit `282abf12`; round 5's at `a3d2dec7`).
> See `crucible/README.md` for the loop this prompt runs inside, and `_mill/reed-shuttle-HANDOFF.md` for campaign-wide state.

You are a senior engineer doing an INDEPENDENT review of the `reed` module (`internal/reedengine` + `internal/reedcli`, plus `internal/hubgeom`) in the loomyard repo, followed by FIXING what you find.
Work in the worktree at `/home/knatte/Code/loomyard/wts/reed-shuttle-crucible-hardening` (branch `reed-shuttle-crucible-hardening`).

**This round is narrowly SCOPED, not a general safety pass** — same posture as round 5. Round 5 found the campaign's second BLOCKING finding plus 7 others, all sharing one root cause (a persisted `PaneID` trusted absolutely across tmux session incarnations), and fixed it with a new mechanism: a `PaneGeneration` stamp (`internal/reedengine/generation.go`) probed via one `display-message` round trip on every state load, plus a pre-boot refusal when the recorded session is still alive under a different worktree name. That mechanism is now itself part of reed's hot path — every op that loads state pays its cost, and shuttle drives this path directly. This round has two concrete jobs; do not wander outside them.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of the two scoped areas below.
   Hunt for bugs by reading the code AND by driving the real substrate (real tmux, resolved on `PATH` — `tmux 3.6` is installed on this host) — this is where the defects hide.
2. FIX: after you have a findings list, implement the fixes one at a time, verify each against the real substrate, keep the whole test suite green, and update the docs in the same change as the fix they document.
   COMMIT after each individual fix lands green (see "Commit per fix" below).
   Do NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test, plus the live smoke/suite check if the finding needed one), and its doc update (if any) is included, COMMIT it — on the current branch, no push — before starting the next finding.
Commit message format: `reed: fix <finding-id> — <one-line what/why>`.
Also commit `_mill/reed-review-r6.md` and `_mill/reed-review-r6-fixer-report.md` as you write or update them — they are NOT gitignored scratch, they are the campaign's durable record.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to `_mill/reed-review-r6.md` and committed — before you touch (edit, create, or delete) a single production or test file.
Do not fix findings as you go, even ones that look small and obviously right.
Write it down as a finding, keep reading, finish the review, save the file, THEN start Job 2.
(Rounds 2 and 4 each recorded one legitimate exception: a finding surfacing during Job 2's own concurrent-verification sweep, which cannot by definition arrive during Job 1. If the same happens to you, record it with its provenance stated plainly — do not hide it in the fixer report.)

## Log as you go during Job 1 (BLOCKING)
Append your observations to `_mill/reed-review-r6.md`'s "What was tested" section immediately after each command/scenario returns.
Jot each finding into the file's findings section provisionally as you spot it.
COMMIT each meaningful append (`reed: review notes — <what you just appended>`).

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first.
Do NOT read any prior review or review-dialogue files before you have your own list.
Specifically do not open anything under `_mill/` matching `reed-review-*` — this is a FILENAME PATTERN, not a content judgment, so it covers rounds 1–5's review reports and fixer reports, AND the orchestrator's own running handoff note (`_mill/reed-shuttle-HANDOFF.md`) even though its name doesn't match the pattern literally — treat it as equally off-limits until your own findings list is complete. Do not open any of these out of curiosity, and do not act on anything you might glimpse in them even by accident.

**Exception, deliberately narrower than prior rounds':** you MUST read `internal/reedengine/generation.go` and its test file in full as part of ordinary code reading — that is the subject of this round, not a review report. What's off-limits is the *review/fixer report prose*, not the production/test code round 5 wrote.

Reading the design SPEC and the module docs is expected and required (those are not reviews).
AFTER your own findings are written, you MAY consult rounds 1–5's material (31 findings across five rounds CLOSED-AND-VERIFIED — see "Round context" below for exact commit shas) to confirm those previously-fixed behaviors have not regressed, and to re-evaluate anything deferred (nothing was deferred from any round).

## Scope — this is the whole mandate, two parts, not a floor to expand from

### Part A — probe the generation mechanism's own failure modes

Round 5 added `internal/reedengine/generation.go`: a `PaneGeneration` stamp probed via `display-message -p -t '=<session>:' '#{session_id}|#{pid}|#{session_created}'` on every state load, driving an adopt/clear/refuse decision, plus a pre-boot refusal (in `lifecycle.go`) when a recorded session is confirmed alive under a different worktree name. This is new machinery on a hot path. Round 5's own fixer report named three concrete things it did NOT probe — treat these as your floor, not your ceiling:

1. **The generation probe fails intermittently.** What happens when `display-message` returns a transient error, a malformed response, or times out — not "tmux is down" (already covered), but a flaky single failure on an otherwise-healthy server? `adoptPaneGenerationLocked`'s own doc comment states probe failure "fails open" (a deliberate trade) — verify that claim live, and ask whether "fails open" is the right choice for every one of its call sites, or only some.
2. **Two worktrees race the same socket through the new pre-boot refusal.** The refusal in `lifecycle.go` checks whether a recorded session is "still alive under a different worktree name" — construct an actual race (two near-simultaneous `up`/`resume` invocations against related state) and see whether the check's own read-then-decide window can itself be fooled, produce a false refusal, or fail to refuse when it should.
3. **Can the refusal wedge an operator who cannot reach tmux directly?** The refusal's whole point is to stop silently, so its ONLY remedy is manual (`kill-session`, then retry). Walk through what an operator with no tmux access (a remote CI-like environment, a sandboxed agent) sees and can actually do. Is the remedy always reachable via `lyx` itself, or does it sometimes require raw tmux the operator may not have?

### Part B — close the two wiring-level regression-test gaps the orchestrator's independent verification found

The orchestrator independently sabotage-proved all 8 of round 5's fixes. Two had NO regression test that caught the fix's *wiring* (the call site invoking the fix) being silently dropped — both the full hermetic suite and the full smoke suite stayed green with the call site removed, even though the underlying function's own unit test still passed:

1. **`clearConflictingPaneBindings`'s call site in `spawn.go`** (wired into `loadOrInitStateLocked` as part of R5-F3's BLOCKING fix). Partially mitigated today by an independent second layer (`removeDuplicatePaneCells` in the render path), but the reconcile-side layer itself has zero regression coverage at the wiring level.
2. **`noSessionMessage`'s readable/unreadable split, wired into `requireSessionLocked`** (R5-F8, NIT). No mitigation at all — NIT severity limits the blast radius, but the gap is real.

Add a test for each that specifically fails when the CALL SITE is removed (not just when the underlying function is broken) — e.g. an integration-shaped test that exercises the full `loadOrInitStateLocked`/`requireSessionLocked` path end-to-end with a fixture state that only a correctly-wired fix would handle right, mirroring how `TestRemoveStrand_NeverKillsAPaneOutsideThisSession` in round 5 already pins its call site rather than only the helper. Treat any other fix from rounds 1–5 you happen to notice has the same wiring-level gap as in scope too, but do not go hunting for it beyond what you notice incidentally while doing Part A/B — a broad audit of all prior rounds' wiring is explicitly OUT of scope for this round (see below).

## Explicitly OUT of scope for this round
- A general re-review of told-geometry field integrity, the tmux-identity pre-flight, reap-root purity, TmuxCmd discipline, or the four state-loss/corruption modes round 5 already probed (truncated file, stale bindings across rebirth, cross-worktree pane targeting, renamed worktree) — five rounds have hardened these; do not re-walk them absent a concrete finding that specifically implicates one.
- A broad wiring-level regression-test audit across all of rounds 1–5's fixes — Part B above names the two specific gaps found; fix those, note anything else you stumble on incidentally, but do not go looking for more.
- `internal/shuttleengine`/`internal/shuttlecli` — a SEPARATE, later crucible campaign. Do not review or fix shuttle code this round even though it depends on reed.
- `internal/hubgeom`'s future siblings (wave 3, T6/T7) do not exist yet.
- The `layout`→`location` rename follow-up in `internal/burlercli`/`internal/shuttlecli`/`internal/perchcli`/`internal/webstercli` — round 2 (R2-F5) deliberately scoped this to `internal/reedcli` alone; not reed, do not touch.
- Windows-specific tmux/path behavior — this host is Linux; a Windows verification gap is expected and should be named, not driven.
- `internal/fsx.AtomicWriteBytes`'s lack of fsync and `.tmp-*` litter after a `kill -9` — round 5 named these explicitly as NOT findings (shared across `internal/fsx`, a repo-wide decision, not reed's alone). Still out of scope; do not re-open them.

## Round context seeded from prior-round verification

Rounds 1 (`opus-medium-r1`, 10 findings), 2 (`opus-high-r2`, 6 findings), 3 (`fable-high-r3`, 2 findings), 4 (`opus-medium-r4`, 5 findings), and 5 (`opus-high-r5`, 8 findings) are all CLOSED-AND-VERIFIED — full details in `_mill/reed-review-r{1,2,3,4,5}.md` / `-r{1,2,3,4,5}-fixer-report.md`, readable only AFTER your own findings list is complete (see "Clean-room review constraint" above; note the explicit exception for `generation.go` itself).

The orchestrator independently re-verified round 5 from a cold state (forked sub-agent, full protocol, deliberately more thorough than rounds 2–4's given round 5's severity): all gates green matching round 5's own claims exactly (`build`/`vet` clean, `-count=5` all `ok`, whole-repo `go test ./...` and `-tags integration ./...` `ok`, smoke suite 22 top-level PASS + 4 subtests / 1 SKIP / 0 FAIL). **All eight of round 5's fixes sabotage-proved** — reverted in turn, each failed at its intended assertion, restored to an empty diff. **Independently live-drove R5-F3, R5-F4, and R5-F5 on brand-new fixtures**, names never used by round 5 itself (`iotahub-HUB/{wt-north,wt-south}`, `kappahub2-HUB/wt-before→wt-after`) — all three held exactly as claimed, including a live re-confirmation that R5-F4's fix is a genuinely independent defense layer from R5-F5's refusal (constructed a case that isolates them: a *valid*, non-stale generation stamp whose `PaneID` still points at a sibling worktree's pane — R5-F5's refusal does not fire here, so R5-F4's session-membership filter is what actually stops it). This also reconfirmed R3-F2 live (rename targeting is derived fresh from the current path, not the stale file).

**The two wiring-test gaps in Part B above are the orchestrator verification's own finding, not round 5's** — round 5 did not know about them; they surfaced specifically from sabotaging the wiring call sites rather than the underlying functions during independent verification.

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
- R3-F1 (MEDIUM) — the identity pre-flight missed tmux's vis-encode rewrite class. Fixed. Commit `12ca3ea5`.
- R3-F2 (NIT) — `reed.json`'s `Socket`/`Session` diagnostic re-stamped on every load. Commit `7af91088`. Re-confirmed live by both round 5 and the orchestrator's round-5 verification: diagnostic-only staleness, not a targeting bug — engine targeting is derived fresh from the current worktree path every time.

**CLOSED-AND-VERIFIED from round 4, do NOT re-litigate:**
- R4-F1 (MEDIUM) — identity pre-flight now refuses all three of tmux's rewrite classes (`.`/`:`, vis-encode, backslash-doubling) — proven exhaustive by a printable-ASCII sweep. Commit `621165ce` + `b3c74d54`.
- R4-F4 (MEDIUM) — header-split retry behind an even-vertical re-tile, fixing a permanent wedge when `.lyx/reed.json` is lost while tmux stays up. Commit `9ec33880`. Re-confirmed generalizing under round 5's scenarios (held against a 1-row band with a corrupt table).
- R4-F5 (LOW) — pane adoption narrowed to the sole-alive-non-header case. Commit `be746b85`. Re-confirmed: adoption itself never misfires, though its *symptom* (false `live:true`) turned out reachable via a different route, which round 5's R5-F2/R5-F3 closed.
- R4-F3 (LOW) — `AnchorPath` backstop. Commit `cb935402`.
- R4-F2 (NIT) — `ServerName` bounded. Commit `b3f58a2b`.

**CLOSED-AND-VERIFIED from round 5, do NOT re-litigate (these ARE the subject of this round's Part A/B, not up for re-opening as a class):**
- R5-F3 (BLOCKING) — a strand `PaneID` colliding with `HeaderPaneID`, or two strands sharing one `PaneID`, made `select-layout` emit a duplicate pane number; tmux silently destroys panes it has no cell for. Fixed via `removeDuplicatePaneCells` (render path structurally cannot emit a duplicate) + `clearConflictingPaneBindings` (sanitizes the loaded table). Commit `24c475a0`.
- R5-F2 (MEDIUM) — pane bindings from a previous tmux server incarnation were trusted as live. Fixed via the `PaneGeneration` stamp. Commit `80909e4c`.
- R5-F5 (MEDIUM) — a renamed worktree's `resume` silently double-launched every strand against an orphaned old session. Fixed via the pre-boot foreign-session refusal, built on R5-F2's stamp. Commit `2c7e88ac`.
- R5-F4 (MEDIUM) — a persisted `PaneID` was spent as a tmux target across worktree boundaries on the shared per-hub socket. Fixed via session-membership filtering. Commit `ab36dcb6`.
- R5-F1 (MEDIUM) — a corrupt/`null` `reed.json` failed opaquely or silently lost the strand table. Fixed via `UnmarshalJSON` rejecting `null` + `unreadableStateError`. Commit `c4ff17cb`.
- R5-F8 (NIT) — misleading "0 strands persisted" for an unreadable file. Fixed via `noSessionMessage`'s split. Commit `788dd562`.
- R5-F6 (LOW) — `.lyx` deletion mid-op voided lock exclusion silently. Fixed via post-op lock-identity detection. Commit `c95de3db`.
- R5-F7 (LOW) — R4-F3's own regression guard was disarmable by stale litter. Fixed via self-cleaning. Commit `5e145a71`.

**Attribution note, for context only:** none of round 5's findings trace to a wave-1 commit (`2b21ee57`/`5b096ebd`/`b98ee2ba`) — the bindings-trust-PaneID root cause predates wave-1 entirely. Not a reason to narrow this round's scope.

**Your mission this round:** Model/effort: Opus / High. This is the sixth round, well beyond the originally planned four-round rotation — the operator is spending it specifically to close the residual risk in the mechanism round 5 just built, and to close two named regression-test gaps, not to find a new general class of bug.
- Work through Parts A and B above.
- **If Part A turns up nothing real and Part B's two tests land clean, say so plainly and explicitly recommend convergence.** That is a valuable, expected outcome of a scoped closing pass, not a failure. Five rounds have now found real defects each time except round 3 — a genuinely clean round 6 on a narrow, well-defined mandate is meaningfully different evidence than "the rotation ran out."
- If you find something real in Part A, fix it with the same rigor as every prior round. Weight anything that produces a false-healthy report or crosses a worktree boundary highest, per the pattern rounds 4–5 established.

State the **merge bar** so you calibrate: correctness in the NORMAL single-instance flow is the gate. Part A's race scenario (item 2) is a deliberate exception — round 5 showed that scenarios requiring "coincidences" are still real findings when the coincidences are ordinary (server rebirth, worktree rename), so do not wave off a race finding as unrealistic without checking how ordinary its preconditions actually are.

## Live-substrate cost declaration
`LLM-DRIVING: PARTIAL.` Reed's smoke suite is overwhelmingly real-tmux-only and cheap (a stray pane costs nothing) — `-run Smoke` broadly is fine for those. ONE exception: `internal/reedcli/smoke_resume_test.go`'s `TestSmokeClaudeResumeRecallsCodeword` launches exactly ONE real `claude` subprocess per invocation (it needs a logged-in claude CLI; it will self-skip if none is configured).
- **You MUST pass `--model haiku` (or the cheapest available model) for the real `claude` process this test launches, and for any real `claude` process you spawn yourself in an ad-hoc scenario — this is an explicit operator instruction for this campaign, not a suggestion.**
- This round's scope is unlikely to need it at all (it targets the generation mechanism and wiring-test coverage, not claude-resume behavior) — skip it entirely unless a finding specifically requires it.

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
- The three items under Part A are a FLOOR for that half — if driving them surfaces a fourth related failure mode in the generation mechanism, follow it.
- "Headless" means "no human required", not "no time/token cost to me." A real tmux scenario takes real wall-clock time — that is expected and budgeted for.

TEARDOWN DISCIPLINE (critical): if you start any tmux server/session, tear it down. At the end, confirm ZERO stray tmux processes using `ps -eo comm | grep -cx 'tmux: server'` = 0 and/or `tmux -L <socket> ls` = "no server running" for every socket you used — **`pgrep -x tmux` alone is NOT sufficient evidence; it falsely reads clean while a server runs.** Leave no stray state. Be honest about what you could NOT verify and why.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/traced) vs PLAUSIBLE (looks wrong, unverified).

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings you record get fixed in Job 2 — including every NIT.
The only legitimate reason to leave a finding unfixed is that fixing it genuinely requires something you cannot do alone this round — say so explicitly in the fixer report's deferred section.

## Deferred items from prior rounds — RE-EVALUATE these
None — no round 1–5 deferred anything (all five fixer reports' "Deferred: Nothing").

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
1. Structured review report → `_mill/reed-review-r6.md`, committed incrementally per "Log as you go" above.
2. Fixer report → `_mill/reed-review-r6-fixer-report.md`, committed (folding into a fix commit is fine).
3. Final chat message: concise executive summary + counts by severity + the two report paths + an explicit merge-readiness verdict, AND an explicit convergence recommendation (say clearly whether you believe reed is done or whether you'd want another look, and why). Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive the real substrate), produce your independent findings, then implement and verify the fixes.
