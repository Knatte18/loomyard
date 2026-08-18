# `reed` — independent review + fix (round 3 — second safety pass)

> Filled instance of `crucible/review-prompt-template.md` for this crucible campaign's first module, rewritten fresh for round 3 (round 1's version is preserved in git history at commit `c0569063`; round 2's at `b38f58c3`).
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
Also commit `_mill/reed-review-r3.md` and `_mill/reed-review-r3-fixer-report.md` as you write or update them — they are NOT gitignored scratch, they are the campaign's durable record.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to `_mill/reed-review-r3.md` and committed — before you touch (edit, create, or delete) a single production or test file.
Do not fix findings as you go, even ones that look small and obviously right.
Write it down as a finding, keep reading, finish the review, save the file, THEN start Job 2.
(Round 2 recorded one legitimate exception: a finding surfacing during Job 2's own concurrent-verification sweep, which cannot by definition arrive during Job 1. If the same happens to you, record it with its provenance stated plainly, same as round 2 did — do not hide it in the fixer report.)

## Log as you go during Job 1 (BLOCKING)
Append your observations to `_mill/reed-review-r3.md`'s "What was tested" section immediately after each command/scenario returns.
Jot each finding into the file's findings section provisionally as you spot it.
COMMIT each meaningful append (`reed: review notes — <what you just appended>`).

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first.
Do NOT read any prior review or review-dialogue files before you have your own list.
Specifically do not open anything under `_mill/` matching `reed-review-*` — this is a FILENAME PATTERN, not a content judgment, so it covers round 1's review report (`reed-review-r1.md`), round 1's fixer report (`reed-review-r1-fixer-report.md`), round 2's review report (`reed-review-r2.md`), round 2's fixer report (`reed-review-r2-fixer-report.md`), AND the orchestrator's own running handoff note (`_mill/reed-shuttle-HANDOFF.md`) even though its name doesn't match the pattern literally — treat it as equally off-limits until your own findings list is complete. Do not open any of these out of curiosity, and do not act on anything you might glimpse in them even by accident.
Reading the design SPEC and the module docs is expected and required (those are not reviews).
AFTER your own findings are written, you MAY consult the round 1 and round 2 material (all sixteen findings across both rounds CLOSED-AND-VERIFIED — see the "Round context" section below for the exact commit shas) to confirm those previously-fixed behaviors have not regressed, and to re-evaluate anything deferred (nothing was deferred from either round).

## What to read
- Code: `internal/reedengine/**` (especially `geometry.go`, `server.go`'s `validateToldTmuxIdentity`, `strand.go`'s `safeReapRoot`/`sessionReapRoots`, `probe.go`, and every file `git show b98ee2ba -- internal/reedengine` touched: `lifecycle.go`, `lock.go`, `header.go`, `strand.go`), `internal/reedcli/**`, `internal/hubgeom/**` (new conversion package), `cmd/lyx` integration.
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

- **Told-geometry field integrity.** `reedengine.New` validates NOTHING about the `Geometry` it receives beyond what round 2 added at `withOpLock`'s pre-flight (`validateToldTmuxIdentity`, which checks `SessionName`/`SocketKey` are tmux-safe strings — it does NOT check that the seven fields are mutually consistent about which hub/worktree they mean). `hubgeom.ReedGeometry` is the only production populator today and builds all seven fields from one already-resolved `*lyxcwd.Location`, which round 2 confirmed structurally rules out cross-field disagreement. Re-confirm this holds under a fixture round 2 didn't try (see "Your mission this round" below).
- **The pre-flight chokepoint itself.** Round 2 added `validateToldTmuxIdentity` at `withOpLock` specifically to refuse tmux-unsafe `SessionName`/`SocketKey` values before any tmux round trip. Is its character set actually complete? tmux's session-name grammar was probed for a handful of characters (`. : / \ space $ % # @ -` `_`) on tmux 3.6 — are there others (unicode, control characters, a purely-whitespace name, an empty string after `filepath.Base`) that slip through and still get silently mangled?
- **Reap-root purity elsewhere.** Round 2's `safeReapRoot`/`sessionReapRoots` fixed `Down` taking descendant-closure roots from dead panes. Is there a THIRD reap-root snapshot site anywhere in the package that predates both `alivePanePIDs` and `sessionReapRoots` and was missed by both rounds' sweeps?
- **The `TmuxCmd` discipline.** Round 2's R2-F6 found `probeCapabilityLocked` was the one call site bypassing the socket-scoped `TmuxCmd` and hitting the operator's global default socket. Grep every remaining `exec.Command`/raw tmux invocation in the package (not just `e.tmux.output`/`e.tmux.run` call sites) for another site that similarly bypasses `-L <socket>` — a probe, a version check, a diagnostic path nobody has driven under load yet.
- **HubLogsDir ownership move.** `fabricengine.HubLogsDir(hub)` is now the sole source of truth (`reedengine` no longer has its own function of that name). Confirm the real boot path — `lyx reed up` against a FRESH hub whose `_board/.lyx` scratch tree does not exist yet — still creates the logs dir correctly on the very first boot.
- **hubgeom one-way told direction.** `internal/hubgeom`'s own doc.go states reed must never import hubgeom (the told direction is one-way: hubgeom → reedengine, never back). Confirm this holds by inspection (`go list -deps` or grep).
- **Every invariant the original reed hardening campaign closed must be RE-VERIFIED against the current construction path:**
  - `down` must reap every pane **child** process, not just the pane itself.
  - Crash/rebirth — a hub whose tmux **server** died out from under it must rebuild cleanly on the next command.
  - Cross-instance/cross-worktree scope boundary — `remove`/`down` in one worktree must never reap panes belonging to a different worktree's hub.
  - Mid-operation-failure orphan — an operation interrupted between two steps must leave no half-linked state a later run trips on.
  - `lyx reed attach` and `header --blocking` still resolve the right socket/session.

## Explicitly OUT of scope for this round
- `internal/shuttleengine`/`internal/shuttlecli` — a SEPARATE, later crucible campaign (its own prompt, its own round numbering), spawned only after reed's campaign converges. Do not review or fix shuttle code in this round even though it depends on reed.
- `internal/hubgeom`'s future siblings (`BurlerGeometry`, `PerchGeometry`, `WebsterGeometry` — wave 3, T6/T7) do not exist yet; do not review for their absence.
- The `layout`→`location` rename follow-up in `internal/burlercli`, `internal/shuttlecli`, `internal/perchcli`, `internal/webstercli` — round 2 (R2-F5) deliberately scoped this to `internal/reedcli` alone and left the other four as a named follow-up. They are not reed; do not touch them this round either unless a new finding specifically requires it.
- Windows-specific tmux/path behavior — this host is Linux; a Windows verification gap is expected and should be named, not driven.

## Round context seeded from prior-round verification
**Second safety pass.** Round 1 (`opus-medium-r1`, 10 findings) and round 2 (`opus-high-r2`, 6 findings) are both CLOSED-AND-VERIFIED — full details in `_mill/reed-review-r1.md`/`-fixer-report.md` and `_mill/reed-review-r2.md`/`-fixer-report.md`, readable only AFTER your own findings list is complete (see "Clean-room review constraint" above).

The orchestrator independently re-verified round 2 from a cold state: reran every hermetic gate (`build`/`vet`/`test -count=5` on the reed packages, `go test ./...` whole-repo, the integration-tagged gate), ran the full tmux-only smoke suite (18 PASS, 1 expected SKIP), SABOTAGE-PROVED all four of round 2's new regression tests by reverting each fix in turn (`validateToldTmuxIdentity`'s call site, `sessionReapRoots`'s dead-pane filter, `probeCapabilityLocked`'s `TmuxCmd` routing) and confirming each failed at the intended assertion — all four did, matching the fixer report's claims exactly — then restored all three files and confirmed an empty diff. The orchestrator also independently live-drove R2-F1's headline fix on its own fresh fixture (a dot-named worktree, `svc.v2`, under a deep three-segment anchor): `reed up` refused in 0.009s with the exact actionable message the fixer report quotes, versus the pre-fix 20s hang and stray untearable session, with zero tmux processes started. Both doc fixes (R2-F4, R2-F5) were spot-checked against current file content and are accurate. Round 1's findings were re-verified as not regressed via round 2's own re-drive of the behavioral ones (F1/F2/F3) plus a fresh read of `producers-standalone.md` (F5).

**CLOSED-AND-VERIFIED from round 1, do NOT re-litigate:**
- F1 (BLOCKING) — pane-child reaping was inert (`os.Process.Wait()` on a non-child pid). Fixed via `proc.IsAlive` polling. Commit `d0bbbc82`.
- F2 (MEDIUM) — strand-pane `split-window` missing `-c AnchorPath`. Fixed. Commit `a6bcb308`.
- F3 (MEDIUM) — `TestSmokeClaudeResumeRecallsCodeword` now pins `--model haiku`. Commit `61f0407d`.
- F4, F8 (LOW/NIT) — commit `0373f7fd`. F6, F7, F9, F10 (NIT) — commit `07883759`. F5 (LOW) — commit `ab6f8fdc`.

**CLOSED-AND-VERIFIED from round 2, do NOT re-litigate:**
- R2-F1 (BLOCKING) — a worktree directory name containing `.`/`:` makes `reed up` hang 20s, fail opaquely, and strand an untearable session on the shared hub server (tmux silently rewrites `.`/`:` to `_` and exits 0). Fixed by `validateToldTmuxIdentity` at the `withOpLock` chokepoint, refusing loudly before any tmux round trip. Commit `ae96397f`. Orchestrator sabotage-proof and independent live re-drive both confirmed above.
- R2-F2 (MEDIUM) — `Down`'s reap-root snapshot included dead panes, risking a SIGKILL of an unrelated process tree that recycled a corpse's pid. Fixed via the shared `safeReapRoot` predicate. Commit `530a1602`. Orchestrator sabotage-proof confirmed above.
- R2-F6 (MEDIUM, surfaced during Job 2's concurrent sweep) — the capability probe bypassed `TmuxCmd` and hit the operator's global default socket, starting a stray server and racing concurrent invocations (2 failures in 9 suite runs). Fixed by routing through `e.tmux`. Commit `28c3aa0f`. Orchestrator sabotage-proof confirmed above.
- R2-F3 (LOW) — a hub resolving to the filesystem root yields a socket key containing `/`, which tmux cannot open and reports nothing about. Fixed. Commit `e9ad525f`.
- R2-F4 (NIT) — `ConfigTemplate`'s doc claimed a `claude` key reed's template does not have. Fixed. Commit `3a40c8cb`. Orchestrator spot-check confirmed above.
- R2-F5 (NIT) — `reedcli` named the resolved `*lyxcwd.Location` `layout`, reviving vocabulary wave-1 retired. Fixed (`reedcli` only; the other four call sites are a named follow-up, out of scope — see above). Commit `eff5eda6`. Orchestrator spot-check confirmed above.

**Attribution note, for context only (not something to re-litigate):** wave-1 is three commits (`2b21ee57` T1, `5b096ebd` T2, `b98ee2ba` T3). T1 touched nothing reed-related. T2 caused one of round 1's F5 rows. `b98ee2ba` caused the rest of round 1's F4–F10. None of round 1's F1/F2/F3, nor any of round 2's six findings, trace to a wave-1 commit — they are all pre-existing defects the campaign's scrutiny surfaced, not regressions wave-1 introduced. This is not license to narrow scope: a safety pass looks everywhere, and round 2 is proof a "clean-looking" module still had a BLOCKING defect waiting.

**Your mission this round:** round 2 was NOT a clean pass — it found a BLOCKING defect (R2-F1) and two MEDIUMs. That means convergence has not been demonstrated yet; this round exists to find out whether round 2's fixes closed the class of defect they represent, or whether a third independent lens (different provider/model, same "assume nothing" posture) finds another one.
- Re-drive the told-geometry and reap-root scenarios with YOUR OWN fixture choices, different again from both round 1's (3-worktree hub, one subpath anchor) and round 2's (hub-wide deep three-segment anchor, two hubs, prefix-colliding pair, dot-named worktree).
- Specifically hunt for the same SHAPE as R2-F1 (an untold-but-assumed safety property nothing validates), R2-F2 (a snapshot/filter invariant duplicated in two places that can drift), and R2-F6 (a call site quietly bypassing the module's own socket-scoping discipline) — elsewhere in the package.
- Honestly confirm merge-readiness if you find nothing new, or find only trivial issues — a clean-or-near-clean round 3 IS the convergence signal this campaign is waiting for. Say so plainly if that's what you find.

State the **merge bar** so you calibrate: correctness in the NORMAL single-instance flow is the gate; an N×-concurrent stress suite is a diagnostic amplifier, not a merge blocker on its own — though round 2 shows it can surface a real, non-concurrency-specific defect (R2-F6) and should not be skipped.

## Live-substrate cost declaration
`LLM-DRIVING: PARTIAL.` Reed's smoke suite is overwhelmingly real-tmux-only and cheap (a stray pane costs nothing) — `-run Smoke` broadly is fine for those. ONE exception: `internal/reedcli/smoke_resume_test.go`'s `TestSmokeClaudeResumeRecallsCodeword` launches exactly ONE real `claude` subprocess per invocation (it needs a logged-in claude CLI; it will self-skip if none is configured).
- **You MUST pass `--model haiku` (or the cheapest available model) for the real `claude` process this test launches, and for any real `claude` process you spawn yourself in an ad-hoc scenario — this is an explicit operator instruction for this campaign, not a suggestion.** Only deviate if a specific finding genuinely requires testing model-specific behavior, and say so explicitly if you do.
- Do not run this one test as part of a bare, broad `-run Smoke` sweep casually many times — run it deliberately, by its exact name.
- The generic "N× CONCURRENT full smoke suites" gate is fine for reed's tmux-only tests, but exclude `TestSmokeClaudeResumeRecallsCodeword` from any concurrent sweep.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/...`
- `go test -count=5 ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/... ./cmd/lyx/...`

Live smoke (real substrate, behind the `smoke` build tag):
- `go test -tags smoke ./internal/reedcli/... -run Smoke -v -count=1` is fine as written for the tmux-only tests.
- For `TestSmokeClaudeResumeRecallsCodeword` specifically: run it by exact name, deliberately, with the `--model haiku` discipline above.
- tmux is resolved via `PATH` on this host (`/usr/bin/tmux`, 3.6). No pwsh/Windows tooling involved.

Live driving — YOU drive it directly, no launcher (PRIMARY — where the bugs surface):
- Deploy the current source as the dev binary: `./deploy-dev` (POSIX script at repo root). **FOOTGUN:** live driving runs the DEPLOYED snapshot — re-run `./deploy-dev` after EVERY source change or you validate a stale binary.
- Do NOT invoke `sandbox-reed-suite.cmd`. Run the real `lyx reed` CLI commands yourself, directly, foreground, waiting for each to return.
- The High-yield focus list above is a FLOOR — devise and run MANY more adversarial scenarios of your own.
- "Headless" means "no human required", not "no time/token cost to me." A real tmux/claude scenario takes real wall-clock time — that is expected and budgeted for.

TEARDOWN DISCIPLINE (critical): if you start any tmux server/session, tear it down. At the end, confirm ZERO stray tmux processes: `ps aux | grep '[t]mux' || echo "zero tmux"` (bracket trick avoids matching your own grep; this is a Linux host — no `tasklist`/`pgrep -af` self-match risk). Leave no stray state. Be honest about what you could NOT verify and why.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/traced) vs PLAUSIBLE (looks wrong, unverified).
For scope: plan-promised vs shipped; flag deferred-that-should-be-v1 and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings you record get fixed in Job 2 — including every NIT.
The only legitimate reason to leave a finding unfixed is that fixing it genuinely requires something you cannot do alone this round — say so explicitly in the fixer report's deferred section.

## Deferred items from prior rounds — RE-EVALUATE these
None — neither round 1 nor round 2 deferred anything (both fixer reports' "Deferred: Nothing").

## Fixing — after the review
- Fix EVERY finding from your review, all severities including NIT.
- Load the code-quality guidance (`/code-quality` skill) AND `golang:golang-build`/`golang:golang-testing`/`golang:golang-comments` before editing — ALL of them, not code-quality alone.
- For every bug you fix, add or extend a test that would have caught it. For a live-only defect, add a `//go:build smoke` test.
- MAKE SMOKE TESTS DETERMINISTIC — poll on actual state transitions with a deadline, never sleep a fixed amount.
- If `tools/sandbox/SANDBOX-REED-SUITE.md` needs extending because a review surfaces a live/visual behavior it doesn't cover, extend it (keep `sandbox_coverage_test.go` green). Otherwise note the new scenario in your fixer report. (Round 2 named two scenarios worth adding but did not add them — see its fixer report's final section; you may add them now if convenient, or leave the note standing.)
- Keep `go build`/`vet`/`test` green after every change. Then RE-DEPLOY (`./deploy-dev`) and re-run every live scenario yourself, directly.
- Update `docs/overview.md` (reed's bullet) / `internal/reedengine/doc.go` / `CONSTRAINTS.md` IN THE SAME change if invariants or scope move. Do NOT add bugfix/hardening notes to `manifest/roadmap.md`.
- Tear down all tmux state; confirm zero stray processes. COMMIT each fix as you finish it. Do NOT push.
- Report the changed files and how you verified each fix.

## Deliverables
1. Structured review report → `_mill/reed-review-r3.md`, committed incrementally per "Log as you go" above.
2. Fixer report → `_mill/reed-review-r3-fixer-report.md`, committed (folding into a fix commit is fine).
3. Final chat message: concise executive summary + counts by severity + the two report paths + an explicit merge-readiness verdict. Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive the real substrate), produce your independent findings, then implement and verify the fixes.
