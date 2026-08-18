# `reed` — independent review + fix (round 4 — final rotation slot)

> Filled instance of `crucible/review-prompt-template.md` for this crucible campaign's first module, rewritten fresh for round 4 (round 1's version is preserved in git history at commit `c0569063`; round 2's at `b38f58c3`; round 3's at `cdd8e107`).
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
Also commit `_mill/reed-review-r4.md` and `_mill/reed-review-r4-fixer-report.md` as you write or update them — they are NOT gitignored scratch, they are the campaign's durable record.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to `_mill/reed-review-r4.md` and committed — before you touch (edit, create, or delete) a single production or test file.
Do not fix findings as you go, even ones that look small and obviously right.
Write it down as a finding, keep reading, finish the review, save the file, THEN start Job 2.
(Round 2 recorded one legitimate exception: a finding surfacing during Job 2's own concurrent-verification sweep, which cannot by definition arrive during Job 1. If the same happens to you, record it with its provenance stated plainly, same as round 2 did — do not hide it in the fixer report.)

## Log as you go during Job 1 (BLOCKING)
Append your observations to `_mill/reed-review-r4.md`'s "What was tested" section immediately after each command/scenario returns.
Jot each finding into the file's findings section provisionally as you spot it.
COMMIT each meaningful append (`reed: review notes — <what you just appended>`).

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first.
Do NOT read any prior review or review-dialogue files before you have your own list.
Specifically do not open anything under `_mill/` matching `reed-review-*` — this is a FILENAME PATTERN, not a content judgment, so it covers rounds 1–3's review reports and fixer reports (`reed-review-r1.md`, `-r1-fixer-report.md`, `reed-review-r2.md`, `-r2-fixer-report.md`, `reed-review-r3.md`, `-r3-fixer-report.md`), AND the orchestrator's own running handoff note (`_mill/reed-shuttle-HANDOFF.md`) even though its name doesn't match the pattern literally — treat it as equally off-limits until your own findings list is complete. Do not open any of these out of curiosity, and do not act on anything you might glimpse in them even by accident.
Reading the design SPEC and the module docs is expected and required (those are not reviews).
AFTER your own findings are written, you MAY consult rounds 1–3's material (all eighteen findings across three rounds CLOSED-AND-VERIFIED — see the "Round context" section below for the exact commit shas) to confirm those previously-fixed behaviors have not regressed, and to re-evaluate anything deferred (nothing was deferred from any round).

## What to read
- Code: `internal/reedengine/**` (especially `geometry.go`, `server.go`'s `validateToldTmuxIdentity`/`firstVisEncodedSessionNameByte`, `strand.go`'s `safeReapRoot`/`sessionReapRoots`, `spawn.go`'s `loadOrInitStateLocked`, `probe.go`, and every file `git show b98ee2ba -- internal/reedengine` touched: `lifecycle.go`, `lock.go`, `header.go`, `strand.go`), `internal/reedcli/**`, `internal/hubgeom/**` (new conversion package), `cmd/lyx` integration.
- The wave-1 commit that changed reed's construction seam: `git show b98ee2ba` — read it in full for `internal/reedengine/*`, `internal/hubgeom/*`, `internal/fabricengine/junctionnames.go`. This is the diff that motivated this campaign; understand it before judging what it might have broken.
- Docs: `docs/overview.md` (reed's bullet, execution-stack section), `manifest/roadmap.md`, `CONSTRAINTS.md` (Cwd Resolution Invariant especially — `hubgeom` is the new told-geometry seam and must not violate it), `README.md`.
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md`.
- Design intent: no dedicated `manifest/designs/reed.md` exists — `docs/overview.md`'s reed bullet (~line 280) plus `internal/reedengine/doc.go`'s package doc are the authoritative scope statement. Read both.
- `tools/sandbox/SANDBOX-REED-SUITE.md` — for SCENARIO IDEAS only (a `**Covers:** reed` tag already satisfies the Sandbox Suite Coverage invariant; round 3 added M20/M21 for the rewrite-refusal and default-socket-purity scenarios — you do not need to extend the launcher machinery, only run scenarios yourself directly).

## Mission (assess on two axes, be adversarial)
1. Scope — is reed's scope right post wave-1? Did the told-geometry refactor (`Engine.New(cfg, Geometry)` replacing `Engine.New(cfg, *lyxcwd.Location)`) silently drop or change any observable behavior?
2. Correctness — bugs, races, error handling, edge cases; concentrate on the areas below. Also assess docs accuracy and operability.

## High-yield focus — where reed's real bugs live post wave-1 (drive these, do not just read them)
This campaign exists because wave-1 (commit `b98ee2ba`) changed reed's constructor signature from `New(cfg, *lyxcwd.Location)` to `New(cfg, Geometry)`, moved `HubLogsDir` out of `reedengine` into `fabricengine`, and introduced `internal/hubgeom` as the one-way told-geometry adapter. Three rounds of adversarial review have progressively hardened the construction seam; a green `go test` proves the mechanical field renames compile and unit-pass, not that the seam is safe under real use — this round's job is to try, one more time with fresh eyes, to find what three rounds of a different flavor of scrutiny each missed.

- **Told-geometry field integrity.** `reedengine.New` still validates nothing about the `Geometry` it receives beyond the identity pre-flight (`validateToldTmuxIdentity`, which as of round 3 refuses both tmux rewrite classes — `.`/`:` substitution and the vis-encode class of control chars/DEL/invalid UTF-8). `hubgeom.ReedGeometry` is the only production populator and builds all seven fields from one already-resolved `*lyxcwd.Location`, which rounds 2 and 3 both confirmed structurally rules out cross-field disagreement. Re-confirm this holds under a fixture the prior three rounds didn't try.
- **Is the identity pre-flight now actually complete?** Round 2 caught the `.`/`:` rewrite; round 3 caught the vis-encode class (control chars, DEL, invalid UTF-8) on the SessionName side and a filesystem-root-yields-`/`  socket key case. Is there a third tmux behavior neither round probed — e.g. tmux's length limits on a session/socket name, a reserved name tmux treats specially, behavior specific to psmux (out of scope to drive, but worth naming if you spot a code-level asymmetry), or an interaction between the two rewrite classes (a name with both a `.` AND a control character)?
- **Reap-root purity elsewhere.** Confirmed twice now (rounds 2 and 3) that `alivePanePIDs` and `sessionReapRoots` are the only two descendant-closure snapshot sites, both routed through `safeReapRoot`. Is there a fourth angle — a snapshot taken at the wrong LOCK phase, a TOCTOU window between snapshot and kill, a path where `safeReapRoot`'s definition of "safe" (not dead, PID > 0) is itself insufter for some substrate state neither round drove?
- **The `TmuxCmd` discipline.** Round 2 found and fixed the one bypass (`probeCapabilityLocked`); round 3's independent sweep confirmed no others exist. Do your own sweep anyway — a new call site, a helper added since, or something both prior sweeps' greps missed by construction (e.g. an indirect call through a function value).
- **`reed.json`'s other persisted fields**, beyond the two round 3 fixed (`Socket`/`Session`). Is anything else in `ReedState` persisted-but-never-read, or read-but-inconsistently-refreshed, in the same shape as R3-F2?
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
**Final rotation slot.** Rounds 1 (`opus-medium-r1`, 10 findings), 2 (`opus-high-r2`, 6 findings), and 3 (`fable-high-r3`, 2 findings) are all CLOSED-AND-VERIFIED — full details in `_mill/reed-review-r1.md`/`-r1-fixer-report.md`, `_mill/reed-review-r2.md`/`-r2-fixer-report.md`, `_mill/reed-review-r3.md`/`-r3-fixer-report.md`, readable only AFTER your own findings list is complete (see "Clean-room review constraint" above).

The orchestrator independently re-verified round 3 from a cold state: reran every hermetic gate, ran the full tmux-only smoke suite (18 PASS, 1 expected SKIP), SABOTAGE-PROVED both of round 3's new regression tests (`TestValidateToldTmuxIdentity_SessionName`'s six new vis-encode rows, and `TestLoadOrInitStateLocked_ExistingFileLoadsStrandsAndRestampsIdentity`) by reverting each fix in turn and confirming each failed at exactly the intended assertion — both did — then restored both files and confirmed an empty diff. The orchestrator also independently live-drove R3-F1's fix on a brand-new fixture built from scratch with a DIFFERENT control byte (0x01) than round 3's own test used (TAB): `reed up` refused in 0.010s with the documented message, zero tmux processes ever started (confirmed via `pgrep -x tmux`/`ps -eo comm`, not the argv-substring-prone `ps aux | grep`). Round 3 itself re-verified rounds 1–2's findings as not regressed (S1–S8 in its own review, re-driving child reap, crash/rebirth, cross-worktree scope, TmuxCmd discipline, fresh-hub first boot).

**CLOSED-AND-VERIFIED from round 1, do NOT re-litigate:**
- F1 (BLOCKING) — pane-child reaping was inert (`os.Process.Wait()` on a non-child pid). Fixed via `proc.IsAlive` polling. Commit `d0bbbc82`.
- F2 (MEDIUM) — strand-pane `split-window` missing `-c AnchorPath`. Fixed. Commit `a6bcb308`.
- F3 (MEDIUM) — `TestSmokeClaudeResumeRecallsCodeword` now pins `--model haiku`. Commit `61f0407d`.
- F4, F8 (LOW/NIT) — commit `0373f7fd`. F6, F7, F9, F10 (NIT) — commit `07883759`. F5 (LOW) — commit `ab6f8fdc`.

**CLOSED-AND-VERIFIED from round 2, do NOT re-litigate:**
- R2-F1 (BLOCKING) — a worktree directory name containing `.`/`:` made `reed up` hang 20s, fail opaquely, and strand an untearable session on the shared hub server. Fixed by `validateToldTmuxIdentity` at the `withOpLock` chokepoint. Commit `ae96397f`.
- R2-F2 (MEDIUM) — `Down`'s reap-root snapshot included dead panes, risking a SIGKILL of an unrelated process tree after pid reuse. Fixed via the shared `safeReapRoot` predicate. Commit `530a1602`.
- R2-F6 (MEDIUM) — the capability probe bypassed `TmuxCmd` and hit the operator's global default socket. Fixed by routing through `e.tmux`. Commit `28c3aa0f`.
- R2-F3 (LOW) — a hub resolving to the filesystem root yields a socket key containing `/`. Fixed. Commit `e9ad525f`.
- R2-F4, R2-F5 (NIT) — stale doc claims / retired `layout` vocabulary. Commits `3a40c8cb` / `eff5eda6`.

**CLOSED-AND-VERIFIED from round 3, do NOT re-litigate:**
- R3-F1 (MEDIUM) — `validateToldTmuxIdentity` only refused the `.`/`:` rewrite class; tmux ALSO vis-encodes every ASCII control character, DEL, and invalid-UTF-8 byte into a multi-character escape — the same failure shape as R2-F1, wider character class, lower reachability (needs a hand-renamed worktree; `fabric add` slugs are git-ref-safe). Fixed by extending the same pre-flight. Commit `12ca3ea5`. Orchestrator sabotage-proof and independent live re-drive (different control byte) both confirmed above.
- R3-F2 (NIT) — `reed.json`'s persisted `Socket`/`Session` diagnostic was stamped once at init and never refreshed, going stale after a worktree rename. Now re-stamped from told geometry on every load. Commit `7af91088`. Orchestrator sabotage-proof confirmed above.

**Attribution note, for context only (not something to re-litigate):** wave-1 is three commits (`2b21ee57` T1, `5b096ebd` T2, `b98ee2ba` T3). T1 touched nothing reed-related. T2 caused one of round 1's F5 rows. `b98ee2ba` caused the rest of round 1's F4–F10. None of round 1's F1/F2/F3, nor any finding from rounds 2 or 3, trace to a wave-1 commit — they are all pre-existing defects the campaign's scrutiny surfaced, not regressions wave-1 introduced. This is not license to narrow scope: three rounds of "clean-looking" module have each still turned up something.

**Your mission this round:** this is the last slot in the operator's planned rotation (Opus/Medium — the same model/effort combination as round 1, giving a natural before/after comparison against round 1's own findings). Round 3 was a near-clean pass (1 MEDIUM — a narrower variant of an already-fixed bug class, not a new one — plus 1 NIT, no BLOCKING), which is the shape of result this campaign has been converging toward. The operator chose to spend this round rather than stop at round 3, specifically to get one more independent check before declaring reed done.
- Re-drive the told-geometry, reap-root, and TmuxCmd-discipline scenarios with YOUR OWN fixture choices — different again from all three prior rounds' fixtures.
- Specifically hunt for the same SHAPE as R2-F1/R3-F1 (an untold-but-assumed tmux safety property nothing yet validates — is the identity pre-flight now actually exhaustive, or is there a fourth rewrite/rejection behavior?), R2-F2 (a snapshot/filter invariant duplicated in two places that can drift), R2-F6 (a call site quietly bypassing the module's own socket-scoping discipline), and R3-F2 (a persisted-but-stale diagnostic field) — elsewhere in the package.
- **This round's outcome directly decides whether the campaign ends here.** If you find nothing new, or only trivial issues, say so plainly and explicitly recommend convergence — that is the valuable, expected outcome of a final safety pass, not a failure to find something. If you find something real, fix it with the same rigor as every prior round, regardless of what that means for the campaign's length.

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

TEARDOWN DISCIPLINE (critical): if you start any tmux server/session, tear it down. At the end, confirm ZERO stray tmux processes: prefer `pgrep -x tmux` or `ps -eo comm | grep -x tmux` over `ps aux | grep tmux` — the latter can self-match text embedded in another process's own argv, a false positive the orchestrator hit and worked around during round 3's independent verification. Leave no stray state. Be honest about what you could NOT verify and why.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/traced) vs PLAUSIBLE (looks wrong, unverified).
For scope: plan-promised vs shipped; flag deferred-that-should-be-v1 and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings you record get fixed in Job 2 — including every NIT.
The only legitimate reason to leave a finding unfixed is that fixing it genuinely requires something you cannot do alone this round — say so explicitly in the fixer report's deferred section.

## Deferred items from prior rounds — RE-EVALUATE these
None — no round 1–3 deferred anything (all three fixer reports' "Deferred: Nothing").

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
1. Structured review report → `_mill/reed-review-r4.md`, committed incrementally per "Log as you go" above.
2. Fixer report → `_mill/reed-review-r4-fixer-report.md`, committed (folding into a fix commit is fine).
3. Final chat message: concise executive summary + counts by severity + the two report paths + an explicit merge-readiness verdict, AND an explicit convergence recommendation (this is the last rotation slot — say clearly whether you believe reed is done or whether you'd want another look). Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive the real substrate), produce your independent findings, then implement and verify the fixes.
