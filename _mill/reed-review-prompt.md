# `reed` — independent review + fix (round 2 — safety pass)

> Filled instance of `crucible/review-prompt-template.md` for this crucible campaign's first module, rewritten fresh for round 2 (round 1's version is preserved in git history at commit `c0569063`).
> See `crucible/README.md` for the loop this prompt runs inside, and `_mill/reed-shuttle-HANDOFF.md` for campaign-wide state.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `reed` module (`internal/reedengine` + `internal/reedcli`, plus the new `internal/hubgeom` conversion layer it is now constructed through) in the loomyard repo, followed by FIXING what you find.
Work in the worktree at `/home/knatte/Code/loomyard/wts/reed-shuttle-crucible-hardening` (branch `reed-shuttle-crucible-hardening`).

## Your two jobs, in order
1. REVIEW: form your own independent judgment of reed's scope and correctness.
   Hunt for bugs by reading the code AND by driving the real substrate (real tmux, resolved on `PATH` — `tmux 3.6` is installed on this host) — this is where the defects hide.
2. FIX: after you have a findings list, implement the fixes one at a time, verify each against the real substrate, keep the whole test suite green, and update the docs in the same change as the fix they document.
   COMMIT after each individual fix lands green (see "Commit per fix" below).
   Do NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test, plus the live smoke/suite check if the finding needed one), and its doc update (if any) is included, COMMIT it — on the current branch, no push — before starting the next finding.
Commit message format: `reed: fix <finding-id> — <one-line what/why>`.
Also commit `_mill/reed-review-r2.md` and `_mill/reed-review-r2-fixer-report.md` as you write or update them — they are NOT gitignored scratch, they are the campaign's durable record.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to `_mill/reed-review-r2.md` and committed — before you touch (edit, create, or delete) a single production or test file.
Do not fix findings as you go, even ones that look small and obviously right.
Write it down as a finding, keep reading, finish the review, save the file, THEN start Job 2.

## Log as you go during Job 1 (BLOCKING)
Append your observations to `_mill/reed-review-r2.md`'s "What was tested" section immediately after each command/scenario returns.
Jot each finding into the file's findings section provisionally as you spot it.
COMMIT each meaningful append (`reed: review notes — <what you just appended>`).

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first.
Do NOT read any prior review or review-dialogue files before you have your own list.
Specifically do not open anything under `_mill/` matching `reed-review-*` — this is a FILENAME PATTERN, not a content judgment, so it covers round 1's review report (`reed-review-r1.md`), round 1's fixer report (`reed-review-r1-fixer-report.md`), AND the orchestrator's own running handoff note (`_mill/reed-shuttle-HANDOFF.md`) even though its name doesn't match the pattern literally — treat it as equally off-limits until your own findings list is complete. Do not open any of these out of curiosity, and do not act on anything you might glimpse in them even by accident.
Reading the design SPEC and the module docs is expected and required (those are not reviews).
AFTER your own findings are written, you MAY consult `_mill/reed-review-r1.md` and `_mill/reed-review-r1-fixer-report.md` (round 1's material, all ten findings F1–F10 CLOSED-AND-VERIFIED — see the "Round context" section below for the exact commit shas) to confirm those previously-fixed behaviors have not regressed, and to re-evaluate anything deferred (nothing was deferred from round 1).

## What to read
- Code: `internal/reedengine/**` (especially the new `geometry.go`, and every file `git show b98ee2ba -- internal/reedengine` touched: `lifecycle.go`, `lock.go`, `header.go`, `strand.go`), `internal/reedcli/**`, `internal/hubgeom/**` (new conversion package), `cmd/lyx` integration.
- The wave-1 commit that changed reed's construction seam: `git show b98ee2ba` — read it in full for `internal/reedengine/*`, `internal/hubgeom/*`, `internal/fabricengine/junctionnames.go`. This is the diff that motivated this campaign; understand it before judging what it might have broken.
- Docs: `docs/overview.md` (reed's bullet, execution-stack section), `manifest/roadmap.md`, `CONSTRAINTS.md` (Cwd Resolution Invariant especially — `hubgeom` is the new told-geometry seam and must not violate it), `README.md`.
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md`.
- Design intent: no dedicated `manifest/designs/reed.md` exists — `docs/overview.md`'s reed bullet (~line 280) plus `internal/reedengine/doc.go`'s package doc are the authoritative scope statement. Read both.
- `tools/sandbox/SANDBOX-REED-SUITE.md` — for SCENARIO IDEAS only (a `**Covers:** reed` tag already satisfies the Sandbox Suite Coverage invariant; you do not need to extend the launcher machinery, only run scenarios yourself directly).

## Mission (assess on two axes, be adversarial)
1. Scope — is reed's scope right post wave-1? Did the told-geometry refactor (`Engine.New(cfg, Geometry)` replacing `Engine.New(cfg, *lyxcwd.Location)`) silently drop or change any observable behavior?
2. Correctness — bugs, races, error handling, edge cases; concentrate on the areas below. Also assess docs accuracy and operability.

## High-yield focus — where reed's real bugs live post wave-1 (drive these, do not just read them)
This campaign exists because wave-1 (commit `b98ee2ba`) changed reed's constructor signature from `New(cfg, *lyxcwd.Location)` to `New(cfg, Geometry)`, moved `HubLogsDir` out of `reedengine` into `fabricengine`, and introduced `internal/hubgeom` as the one-way told-geometry adapter. Every invariant below is specific to that change — a green `go test` proves the mechanical field renames compile and unit-pass, not that the new construction seam is safe under real use.

- **Told-geometry field integrity.** `reedengine.New` validates NOTHING about the `Geometry` it receives — the doc comment on `geometry.go` states this explicitly: "populating every field with a usable absolute path... is entirely the caller's obligation." Before wave-1, every one of `SocketKey`/`SessionName`/`AnchorPath`/`WorktreeRoot`/`LogsDir`/`RepoName`/`HubPath` was DERIVED from one single `*lyxcwd.Location`, so they could never disagree about which hub/worktree they meant. Now they are seven independently-settable fields. `hubgeom.ReedGeometry` is the only production populator today and looks correct by inspection — but is it actually IMPOSSIBLE, end-to-end through the real CLI, for two of these fields to end up describing different worktrees/hubs (e.g. a stale `AnchorRel`, a `Location` resolved against the wrong cwd, a race between resolving `layout` and calling `hubgeom.ReedGeometry`)? Drive `lyx reed` commands from a genuinely surprising cwd/anchor combination (a subpath-anchored worktree per `AnchorRel`, a hub with more than one worktree live at once) and confirm the socket, session, and log dir reed actually touches are the ones you expect — not silently the wrong worktree's.
- **HubLogsDir ownership move.** `fabricengine.HubLogsDir(hub)` is now the sole source of truth (`reedengine` no longer has its own function of that name). Confirm the real boot path — `lyx reed up` against a FRESH hub whose `_board/.lyx` scratch tree does not exist yet — still creates the logs dir correctly on the very first boot, not just in the moved unit test's synthetic fixture.
- **hubgeom one-way told direction.** `internal/hubgeom`'s own doc.go states reed must never import hubgeom (the told direction is one-way: hubgeom → reedengine, never back). Confirm this holds by inspection (`go list -deps` or grep) — a violation here would be a real design regression, not just a lint nit.
- **Every invariant the original reed hardening campaign closed must be RE-VERIFIED against the new construction path — do not assume it still holds just because it was fixed once against the old `*lyxcwd.Location`-based `Engine`:**
  - `down` must reap every pane **child** process, not just the pane itself — repro: start a pane running a long-lived child, `lyx reed down`, then confirm no orphaned child PIDs survive.
  - Crash/rebirth — a hub whose tmux **server** died out from under it must rebuild cleanly on the next command, never error out or act on dead state — repro: kill the tmux server (`tmux -L <socket> kill-server`, socket from `lyx reed status`) while a hub is live, then run any `lyx reed` verb.
  - Cross-instance/cross-worktree scope boundary — `remove`/`down` in one worktree must never reap panes belonging to a different worktree's hub — repro: two hubs live at once (two worktrees under the same hub, or two hubs), tear one down, assert the other's panes/strands are untouched. This is the invariant most directly at risk from a `Geometry` field mix-up above — a wrong `SocketKey` or `WorktreeRoot` could make reed operate against the wrong hub's tmux server entirely.
  - Mid-operation-failure orphan — an operation interrupted between two steps must leave no half-linked state a later run trips on, and must report honestly what it did and did not complete.
  - `lyx reed attach` and `header --blocking` (the two registered interactive-handoff exceptions, per `CONSTRAINTS.md`'s CLI/Cobra Invariant) still resolve the right socket/session under the new `Geometry`-based construction.

## Explicitly OUT of scope for this round
- `internal/shuttleengine`/`internal/shuttlecli` — a SEPARATE, later crucible campaign (its own prompt, its own round numbering), spawned only after reed's campaign converges. Do not review or fix shuttle code in this round even though it depends on reed.
- `internal/hubgeom`'s future siblings (`BurlerGeometry`, `PerchGeometry`, `WebsterGeometry` — wave 3, T6/T7) do not exist yet; do not review for their absence.
- Windows-specific tmux/path behavior — this host is Linux; a Windows verification gap is expected and should be named, not driven.

## Round context seeded from prior-round verification
**Safety pass.** Round 1 (`opus-medium-r1`) found and fixed 10 findings (1 BLOCKING, 2 MEDIUM, 2 LOW, 5 NIT) — full details in `_mill/reed-review-r1.md` / `_mill/reed-review-r1-fixer-report.md`, readable only AFTER your own findings list is complete (see "Clean-room review constraint" above). The orchestrator independently re-verified all of it from a cold state afterward — re-ran every hermetic gate, ran the live smoke suite, ran a 3× concurrent smoke sweep (tmux-only tests) with zero corruption markers, and — critically — SABOTAGE-PROVED both of round 1's new regression tests by reverting each fix in turn and confirming the corresponding test actually failed at the intended assertion before restoring (both did; both diffs came back empty on restore). Two of round 1's doc fixes (F5, F9) were also spot-checked against the current file content and are accurate.

**CLOSED-AND-VERIFIED, do NOT re-litigate:**
- F1 (BLOCKING) — pane-child reaping (`waitProcessExit`) was inert because `os.Process.Wait()` on a non-child pid returns instantly; fixed by polling `proc.IsAlive` on a deadline. Commit `d0bbbc82`. Orchestrator sabotage-proof: reverting to the old `Wait()`-based implementation made `TestSmokeDownForceKillsSighupImmunePaneChildren` fail with a leaked pid, exactly as round 1's fixer report claimed.
- F2 (MEDIUM) — strand-pane `split-window` was missing `-c AnchorPath`, so a pane's cwd came from the invoking client, not the told anchor; fixed. Commit `a6bcb308`. Orchestrator sabotage-proof: reverting the `-c` flag made `TestSmokeStrandPaneSpawnsAtToldAnchorNotProcessCwd` fail at the expected assertion.
- F3 (MEDIUM) — `TestSmokeClaudeResumeRecallsCodeword` now pins `--model haiku` on both the launch and `--continue` lines, behind a named `smokeClaudeModel` constant. Commit `61f0407d`. Orchestrator confirmed the constant is genuinely wired into both invocations and re-ran the test (10.09s, haiku).
- F4, F8 (LOW/NIT — dead `socketName` alias deleted, `server.go`'s stale `Location` comment corrected) — commit `0373f7fd`.
- F6, F7, F9, F10 (NIT — stale `layout`/`HubLogsDir` vocabulary cleared from comments) — commit `07883759`.
- F5 (LOW — `manifest/designs/producers-standalone.md`'s stale reed/tokenvocab rows corrected) — commit `ab6f8fdc`.

**Attribution note, for context only (not something to re-litigate):** this campaign was framed around wave-1 commit `b98ee2ba` alone, but wave-1 is actually three commits (`2b21ee57` T1, `5b096ebd` T2, `b98ee2ba` T3). Investigation found T1 touched nothing reed-related; T2 caused one of F5's three stale rows (the `reedengine.LoadConfig` degrade-to-template row); `b98ee2ba` caused the rest of F4–F10. Neither F1, F2, nor F3 — the three findings with real behavioral consequence — trace to ANY wave-1 commit; `git log -S` on each shows they trace back to `93ad5b01` ("mux -> reed rename"), reed's earliest history. Do not read this as license to skip driving the told-geometry invariants below — F2 (a pre-existing bug) was only CAUGHT because wave-1's `geometry.go` doc comment turned an implicit derivation into an explicit contract the code didn't honor. A safety pass exists precisely because a clean first round proves less than it feels like it proves.

**Your mission this round:** a genuinely independent clean-room pass. Do NOT assume round 1 was exhaustive. Specifically:
- Re-drive the told-geometry field-integrity scenarios in the High-yield focus list below with YOUR OWN fixture choices (different worktree layout, different hub shape) — round 1's fixture was a 3-worktree hub with one subpath-anchored worktree; try something round 1 didn't (e.g. a worktree whose `AnchorRel` is deeper, or a hub mid-`fabric reconcile`).
- Look specifically for anything in the same SHAPE as F1/F2 (a `Wait`/liveness assumption elsewhere in the package that might share F1's root cause; another spawn/split site that might share F2's missing-`-c` pattern) — `grep` every `tmux.output(...)`/`tmux.run(...)` call site in `internal/reedengine` for a missing `-c` where one of the other three sites has it, and every process-liveness check for the same `os.Process.Wait()`-on-non-child hazard F1 exposed.
- Honestly confirm merge-readiness if you find nothing — "no new defects, ship it" is the expected, valuable outcome of a safety pass, not a failure to find something.

State the **merge bar** so you calibrate: correctness in the NORMAL single-instance flow is the gate; an N×-concurrent stress suite (if the orchestrator later asks for one) is a diagnostic amplifier, not a merge blocker on its own.

## Live-substrate cost declaration
`LLM-DRIVING: PARTIAL.` Reed's smoke suite is overwhelmingly real-tmux-only and cheap (a stray pane costs nothing) — `-run Smoke` broadly is fine for those. ONE exception: `internal/reedcli/smoke_resume_test.go`'s `TestSmokeClaudeResumeRecallsCodeword` launches exactly ONE real `claude` subprocess per invocation (it needs a logged-in claude CLI; it will self-skip if none is configured). That is not a fan/cluster test — one invocation spawns exactly one process, never more — so it does not carry the multi-process RAM-exhaustion risk this method's N-concurrent step was built to guard against, but it does carry a real token/wall-clock cost and needs an operator-authenticated `claude`.
- **You MUST pass `--model haiku` (or the cheapest available model) for the real `claude` process this test launches, and for any real `claude` process you spawn yourself in an ad-hoc scenario (e.g. manually `lyx reed add` a strand that runs `claude`) — this is an explicit operator instruction for this campaign, not a suggestion.** Only deviate if a specific finding genuinely requires testing model-specific behavior, and say so explicitly if you do.
- Do not run this one test as part of a bare, broad `-run Smoke` sweep casually many times — run it deliberately, by its exact name, when you actually intend to exercise the claude-resume path, same as every other scenario's cost discipline below.
- The generic "N× CONCURRENT full smoke suites" gate (see the verification protocol in `crucible/README.md`) is fine to run for reed's tmux-only tests as usual, but exclude `TestSmokeClaudeResumeRecallsCodeword` from any concurrent sweep — do not run N copies of a real claude-spawning test concurrently without first asking the orchestrator.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/...`
- `go test -count=5 ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/... ./cmd/lyx/...`

Live smoke (real substrate, behind the `smoke` build tag):
- `go test -tags smoke ./internal/reedcli/... -run Smoke -v -count=1` is fine as written for the tmux-only tests.
- For `TestSmokeClaudeResumeRecallsCodeword` specifically: run it by exact name (`-run TestSmokeClaudeResumeRecallsCodeword`), deliberately, with the `--model haiku` discipline above — do not fold it into a careless broad sweep.
- tmux is resolved via `PATH` on this host (`/usr/bin/tmux`, 3.6). No pwsh/Windows tooling involved — this is a Linux host.

Live driving — YOU drive it directly, no launcher (PRIMARY — where the bugs surface):
- Deploy the current source as the dev binary: `./deploy-dev` (POSIX script at repo root). **FOOTGUN:** live driving runs the DEPLOYED snapshot — re-run `./deploy-dev` after EVERY source change or you validate a stale binary.
- Do NOT invoke `sandbox-reed-suite.cmd`. Run the real `lyx reed` CLI commands yourself, directly, foreground, waiting for each to return — walk the High-yield focus list above (and `tools/sandbox/SANDBOX-REED-SUITE.md`'s scenarios for extra ideas).
- The list above is a FLOOR — devise and run MANY more adversarial scenarios of your own (combine verbs in orders nothing has tried; chase anything the code makes you suspicious of), especially around the `Geometry` field-integrity invariant above, which is genuinely novel post wave-1.
- "Headless" means "no human required", not "no time/token cost to me." A real tmux/claude scenario takes real wall-clock time — that is expected and budgeted for.

TEARDOWN DISCIPLINE (critical): if you start any tmux server/session, tear it down. At the end, confirm ZERO stray tmux processes: `pgrep -af tmux || echo "zero tmux"` (this is a Linux host — no `tasklist`). Leave no stray state. Be honest about what you could NOT verify and why.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/traced) vs PLAUSIBLE (looks wrong, unverified).
For scope: plan-promised vs shipped; flag deferred-that-should-be-v1 and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings you record get fixed in Job 2 — including every NIT.
The only legitimate reason to leave a finding unfixed is that fixing it genuinely requires something you cannot do alone this round — say so explicitly in the fixer report's deferred section.

## Deferred items from the prior round — RE-EVALUATE these
None — round 1 deferred nothing (all 10 findings were fixed, per its fixer report's "Deferred: Nothing").

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
1. Structured review report → `_mill/reed-review-r2.md`, committed incrementally per "Log as you go" above.
2. Fixer report → `_mill/reed-review-r2-fixer-report.md`, committed (folding into a fix commit is fine).
3. Final chat message: concise executive summary + counts by severity + the two report paths + an explicit merge-readiness verdict. Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive the real substrate), produce your independent findings, then implement and verify the fixes.
