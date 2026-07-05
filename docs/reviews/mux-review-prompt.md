# mux — independent review + fix

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `mux`
module in the loomyard repo, followed by FIXING what you find. Work in the worktree at
`C:\Code\loomyard\wts\internal-mux` (branch `internal-mux`). Adjust that path/branch if the
task lives elsewhere now.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of mux's scope and correctness. Hunt for bugs by
   reading the code AND by driving real psmux (this is where mux's defects hide).
2. FIX: after you have a findings list, implement the fixes, verify each against real psmux,
   keep the whole test suite green, and update the docs in the same change. Do NOT commit or
   push unless the user explicitly tells you to — leave the changes in the working tree and
   report them.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first. Do NOT read any prior review or review-dialogue files before you
have your own list. Specifically do not open anything under `.scratch/` (it is gitignored and
holds prior reviews: `mux-review-*.md`, `internalmux_*.md`, `orch_*.md`, `*review*.md`) or any
`_mill/reviews/` content. Reading the design SPEC and the module docs is expected and required
(those are not reviews). After you have formed and written your own independent findings, you
MAY consult the prior rounds' review material under `.scratch/` — ANY `mux-review-*.md` and its
`*-fixer-report.md`, regardless of which model produced it (rounds rotate across Opus / Fable /
Sonnet, so do NOT assume a single model's filename prefix — the most recent prior round is
whichever `mux-review-*` file is newest), EXCEPT your own `-<yourtag>` deliverables — to
(a) confirm previously-fixed behaviors have not regressed and (b) re-evaluate the deferred items
listed at the bottom of this prompt — but only AFTER your own pass, so the independent signal is
preserved.

## What to read
- Code: `internal/muxengine/**` (incl. `render/**`), `internal/muxcli/**`, and the `cmd/lyx`
  integration (`main.go`, sandbox/help/registration guard tests).
- Docs: `docs/modules/mux.md`, `docs/research/mux-exploration.md`,
  `docs/research/mux-hooks-exploration.md`, `docs/overview.md`, `docs/roadmap.md`,
  `CONSTRAINTS.md`, `README.md`.
- The dedicated live-driving suite you will RUN: `tools/sandbox/SANDBOX-MUX-SUITE.md`
  (scenarios M0–M11) plus `docs/sandbox-howto.md` for how the sandbox harness works. This
  suite is the maintained, structured vehicle for driving real psmux — see "What to TEST".
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md`
  (Hub Geometry Invariant, CLI/Cobra Invariant, lyxtest Leaf Invariant, Sandbox Suite Coverage,
  Documentation Lifecycle). A change that ships behaviour without updating the module doc /
  invariants in the same change is incomplete.
- Design intent (SPEC, not a review). `_mill/discussion.md` and `_mill/plan/*` were removed from
  this branch by a pre-merge cleanup commit; recover them from git history:
  `git log --oneline -- _mill/discussion.md` to find the last sha that had them (a known-good
  pre-cleanup sha is `a4e0ba8`), then:
    - `git show a4e0ba8:_mill/discussion.md`
    - `git show a4e0ba8:_mill/plan/00-overview.md` and the per-batch cards
      `git show a4e0ba8:_mill/plan/NN-*.md` (01..08)
  Use these as the authoritative source of intended v1 scope/behavior.

## Mission (assess on two axes, be adversarial)
1. Scope / omfang — is the module's scope right? Does the as-built code deliver what the design
   intended? Gaps, over-reach, silently-dropped requirements, deferred-that-should-ship-in-v1.
2. Correctness — bugs, races, error handling, edge cases. Pay special attention to the areas
   that are historically where mux breaks (see "High-yield focus" below).
Also assess docs accuracy (do the docs match the code?) and operability (could a user actually
run this?).

## High-yield focus — where mux's real bugs live (drive these, do not just read them)
The pure/unit-tested parts (render math, checksum, name templating, parsing, the op lock) are
solid and rarely wrong. The defects concentrate in the COMPOSED, LIVE-psmux behavior that the
hermetic tests and the single-strand smoke test never exercise. Treat every one of these as an
INVARIANT you must actively verify by driving real psmux — a green `go test` proves nothing here:

- LIVENESS DEFINITION. "present in `list-panes`" must NOT be conflated with "the strand's
  process is alive". A `pane_dead=1` pane is present-but-not-alive. Verify: `status` reports a
  crashed strand as NOT live; `resume` treats a dead-pane-bound strand as needing relaunch;
  render still counts dead panes (select-layout must enumerate them).
- CRASH / SERVER REBIRTH. After `kill-server`, a reborn session reuses pane ids (initial pane is
  `%1` again). Verify a stale binding is never mistaken for a live strand: `up` after a crash
  must clear stale bindings; `resume` after a crash must rebuild every non-hidden strand exactly
  once (adopt the initial pane, split the rest — no orphans, no double-launch).
- SOLE / ALL-DEAD PANES. psmux refuses to kill a session's last pane. Verify reconcile keeps
  exactly one pane when every pane is dead (so the session survives) and that resume still
  rebuilds all strands in ONE pass (no "resumed:N but only 1 came back", no adopting a dead pane
  and silently swallowing the launch).
- CROSS-WORKTREE SCOPE. The psmux server is per-HUB, shared by sibling worktrees. Verify `down`
  in one worktree does NOT kill sibling worktrees' sessions/agents; verify two worktrees on one
  hub server coexist; watch for duplicate server processes spawned during down/up churn.
- REMOVE / LAYOUT REAPING. psmux (3.3.4) silently DESTROYS any pane not present in an applied
  `select-layout` string. Verify `remove` kills the removed strand's pane deterministically
  (not by accident of layout reaping), and think about what a manual operator-split pane suffers
  when the next mux verb re-applies the layout.
- MID-OP FAILURE. A launched pane must never become an untracked orphan if a later apply/persist
  step fails (i.e. persist the record before the fragile apply).
- SEND-KEYS HYGIENE. Opaque `cmd`/`resumeCmd` strings (shuttle builds arbitrary PowerShell
  chains) must be sent literally so an embedded `;` or a key-name-like token is not reinterpreted
  by psmux.
- REPORTING HONESTY. Result counts (`resumed`, `removed`) and `status.live` must reflect reality,
  not intent.
- ENV HYGIENE. `CleanClaudeEnv` must strip `CLAUDECODE` + `CLAUDE_CODE_*` on the server spawn.

## Hooks are OUT of scope for mux v1
Claude Code hooks (Stop/SessionStart/PreToolUse, marker/idle detection, resume-command
construction) belong to `shuttle`, not mux. Their absence is correct — do not flag it. mux is a
dumb carrier: it runs opaque command strings and its only liveness signal is generic `pane-died`.

## Round context seeded from prior-round verification — SAFETY PASS: confirm merge-readiness or find what all prior rounds missed
There is NO known open residual. Rounds 3→6 CONVERGED and round 6 was INDEPENDENTLY verified CLEAN
by the human operator (not by the round's own self-verdict, which has been wrong before). This round
is a final SAFETY pass before merging `internal-mux` → `main`. Do NOT re-open, re-litigate, or undo
any of the CLOSED-AND-VERIFIED work below; spend your effort looking HARD for anything every prior
round missed.

CLOSED AND VERIFIED (do not re-chase):
- Stray-state teardown race (R3 `down` reap → R4 shared `descendantClosurePIDs`/`reapPaneChildren`
  seam for `down`+`remove` → R5 traced the real holder via PEB cwd and closed the psmux-server leak
  with confirmation-based saturation-tolerant deadlines). Operator-verified: 3× concurrent full smoke
  leaves ZERO stray psmux; serial `-count=5` reap tests 5/5.
- R6 fixed two NEW product defects: **F1** (zero-pane zombie — `up`/apply emitted an empty-cell
  layout when ≥2 panes were live but no strand owned one → psmux destroys every pane; fixed by
  skipping empty-layout apply in `applyLayoutLocked` + healing a zero-pane husk on boot) and **F11**
  (psmux `select-layout` reaping is POSITIONAL and could destroy a TRACKED strand's pane while a
  foreign pane survived; fixed by deterministic untracked-pane reaping in `reconcile.go`). Plus F5
  (`remove` always reaps even when layout repair errors), F6 (`down` tears down an unreachable/zombie
  server), F4 (deadline-based boot), F7 (sibling-boot grace), and harness F2 (kill orphaned
  hub-holding conhosts — they persist for hours) / F3 (scope the claude transcript watch).
- Operator INDEPENDENT verification of R6 (the authoritative sign-off): build/vet; hermetic
  `-count=3`; full serial smoke **11/11**; 3× concurrent full suite ×2 rounds = **3/3 PASS each,
  zero `\hub`/boot/non-conhost markers, zero stray psmux, zero leftover temp dirs**. PLUS a LIVE
  operator-assisted `attach` test on the deployed binary: the M6/M7 layout rendered correctly, and
  **F1/F11 were confirmed fixed live** — the operator split a foreign pane inside the session, then
  `up` reaped it deterministically and all three tracked strands survived (no zombie, no displaced
  strand). Config is honored (`top_band_rows`/`collapsed_strip_rows` scale the layout); an attached
  client correctly resizes the window to its own terminal (expected tmux behavior, not a bug).

MERGE BAR (agreed with the operator): correctness in the NORMAL single-instance flow is the gate.
The 3×-concurrent suite is a DIAGNOSTIC amplifier that already did its job (it drove R3–R6's real
fixes) — it is NOT a merge blocker, and the correctness well it fed is now DRY. Run it as a stress
diagnostic, but a timeout under an artificial 3-suite CPU peg is not a defect.

YOUR JOB this round:
- Do a genuinely INDEPENDENT clean-room pass (form + WRITE your own findings before reading prior
  `.scratch/mux-review-*` reports). Adversarially live-drive psmux for anything every prior round
  missed — new edge cases, races, error paths, resume/crash-rebirth corners, cross-worktree behavior.
- If you find a REAL defect that affects the normal flow, fix it with tests + doc updates in the same
  change. If you do NOT, say so explicitly and CONFIRM merge-readiness — an honest "no new defects,
  ship it" is the expected and valuable outcome of a safety pass. Do not invent work to look busy.
- NON-BLOCKING candidates the operator surfaced — assess and report, implement only if cheap and
  clearly right (do NOT over-engineer, do NOT block merge on these):
  1. mux does not stamp the strand name into the pane title/identity (`pane_title` stays the
     hostname), so an attached operator cannot visually tell strands apart. Acceptable for v1, or a
     cheap ergonomic win (pane title = strand name)?
  2. The reap probe spawns a fresh `pwsh` + full `Get-CimInstance Win32_Process` per poll — costly
     and self-saturating under load; a cheaper probe would speed real single-instance `down` too.
     Worth doing now, or a documented follow-up?
  3. Portability lens (mux targets Linux/tmux too; psmux is meant to be a faithful tmux clone): for
     each Windows-substrate workaround, note whether it is faithful-tmux (portable) or a psmux
     divergence (upstream candidate). The whole `\hub in use` class is Windows-only. Flag
     observations; do not implement a Linux engine here.
- VERIFY with the usual discipline: build/vet; hermetic `-count=5`; full serial smoke; a couple of
  3×-concurrent rounds as a stress diagnostic (zero `\hub` markers, zero stray psmux at teardown);
  live sandbox driving on the freshly re-deployed binary. Report merge-readiness explicitly.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/muxengine/... ./internal/muxcli/...`
- `go test ./internal/muxengine/... ./internal/muxcli/... ./cmd/lyx/...` (stress the
  concurrency/timing tests with `-count=5` to catch flakiness)

Smoke (real psmux, behind a build tag):
- `go test -tags smoke ./internal/muxcli/... -run Smoke -v -count=1`
- psmux is installed at `C:\Code\tools\bin\psmux.exe` (also on PATH as `psmux`); pwsh 7 at
  `C:\Code\tools\powershell7\pwsh.exe`. Launch tools with EXPLICIT absolute paths — a bare
  `pwsh` resolves to a 0-byte WindowsApps ConPTY stub that renders nothing.

Live psmux driving via the MUX SANDBOX SUITE (PRIMARY — this is where the bugs surface).
The repo ships a dedicated, maintained live-psmux suite: `tools/sandbox/SANDBOX-MUX-SUITE.md`,
scenarios M0–M11, driven through the harness. Run it — do not only hand-roll fixtures:
- Deploy the current source as the binary under test: `deploy.cmd` (puts a fresh `lyx.exe`
  on PATH). CRITICAL FOOTGUN: the suite runs the DEPLOYED snapshot, NOT your working tree — it
  has no idea you edited source. You MUST re-run `deploy.cmd` after EVERY source change before
  re-running the suite, or you will validate the STALE binary and wrongly conclude a fix works
  (or fails). Deploy first, always; when in doubt, re-deploy.
- Launch the interactive suite session: `sandbox-mux-suite.cmd` (repo root) — it runs
  `go run ./tools/sandbox -parent C:\Code mux-suite`, materializes the sandbox Hub host repo,
  and copies SANDBOX-MUX-SUITE.md (with a binary-fingerprint header) into it. Follow that
  file's own Pre-conditions + "How to run a scenario" sections as the source of truth.
- After the session, pull the findings back with `sandbox-fetch.cmd` (stamps the binary
  fingerprint into the fetched `sandbox-report.json` `meta`).
- The suite's own scenarios already map onto the "High-yield focus" invariants: M8 (kill one
  pane → resume recreates it), M9 (kill-server → crash-resume rebuilds all), M6 (≥2-top layout
  tiling), M10 (recursive remove), M11 (down leaves no stray psmux). Walk every one and record
  OK/WARN/FAIL per the suite's verdict key.
- NOTE the persona split: SANDBOX-MUX-SUITE.md's black-box rule ("do not read the lyx source
  tree") binds the *agent-under-test* persona, NOT you. As the reviewer you read the source
  AND drive the suite — use the suite's scenarios/harness as your live-driving checklist while
  still reasoning about the code. The `attach` scenario (M7) is operator-assisted (needs a TTY
  in a second terminal); flag it as not-headlessly-verifiable, as before.

Deeper hand-rolled driving (COMPLEMENTARY, and EXPECTED — the suite is a FLOOR, not a ceiling).
Running M0–M11 is the minimum, not the whole job. You are expected to devise and run MANY MORE
adversarial tests of your own beyond the suite — invent scenarios the suite does not cover, push
edge cases, combine verbs in orders the suite never tries, and chase anything the code makes you
suspicious of. In particular drive the paths M0–M11 do not cover: two worktrees on one hub
server, a dead-but-present `pane_dead=1` pane, stale-pane-id reuse after server rebirth,
mid-op-failure orphans, send-keys hygiene with embedded `;`/key-name tokens, rapid down→up→add
churn, non-leaf remove without `--recursive`, unknown-parent and `own-window` rejection paths.
Report the exact commands and observations for these too. Build the binary
(`go build -o <scratch>/lyx.exe ./cmd/lyx`), create throwaway git-repo fixtures with a
`_lyx/config/mux.yaml` (copy `internal/muxengine/template.yaml`), and drive `lyx mux <verb>`
while inspecting real psmux with `psmux -L <socket> list-panes -t <session> -F "#{pane_id}
#{pane_dead} #{pane_top} #{pane_height}"` and `... display-message -p -t <session>
"#{window_layout}"`. Use isolated `-L` sockets. Walk at minimum every scenario in "High-yield
focus" above, including: two worktrees under one hub; a parent+child stack; killing a strand's
process (`send-keys -t <pane> "exit" Enter`, repeat until `pane_dead=1`); `kill-server` to
simulate a crash; `up`/`resume`/`status`/`remove`/`down` in each resulting state; and
`--anchor top|below-parent|hidden` plus rejection paths (`own-window`, unknown parent, non-leaf
remove without `--recursive`). Use `-vv` to trace exact psmux invocations.

TEARDOWN DISCIPLINE (critical): if you start ANY psmux server/session, tear it down
(`psmux -L <socket> kill-server`, or `lyx mux down`). At the end, confirm with `tasklist | grep
-i psmux` that ZERO psmux processes remain. Leave no stray psmux state.

Be honest about what you could NOT verify: interactive `attach` cannot be driven headlessly (no
TTY); real `claude --resume` needs a live agent. Say so explicitly.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong
behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED
(reproduced/traced) vs PLAUSIBLE (looks wrong, unverified). For scope: plan-promised vs shipped;
flag deferred-that-should-be-v1 and shipped-beyond-scope.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
These were consciously deferred last time; decide whether any now warrants fixing:
- Untracked panes destroyed by `select-layout` reaping (mux "owns" the session window — needs a
  documented policy for operator-split panes rather than silent death).
- A rare duplicate psmux server process spawned during rapid down→up→add churn (a boot-path
  race; needs a "server-down vs session-missing" distinction to fix safely).
- psmux normalizing applied layouts (band/strip heights come back off-by-one vs the config knobs
  `collapsed_strip_rows`/`top_band_rows` — cosmetic; maybe a code/doc note).
- `.lyx`/config anchored at `Cwd` not `WorktreeRoot` (running from a worktree SUBDIR gives a
  misleading "not initialized" error; a consistent fix belongs at the `hubgeometry` level).
- Dead/spec-inherited surfaces: `PsmuxCmd.windowSize`, `PsmuxCmd.paneIDsTopToBottom`,
  `Config.Claude`, `MuxState.StrippedEnv` (always serialized `null`) — delete or wire up.

## Fixing — after the review
- Load the code-quality guidance (`/code-quality` skill or `mill:code-quality`) before editing.
- Prefer surgical edits; match existing style and the file-level doc-comment convention.
- For every bug you fix, add or extend a test that would have caught it. In particular, if you
  find a live-only defect, add a `//go:build smoke` test that walks the failing scenario against
  real psmux (the existing `internal/muxcli/smoke_test.go` shows the pattern, incl. a skip when
  psmux is absent). A hermetic unit test for the pure planning helper is good; a smoke test for
  the composed behavior is what actually protects the recovery paths.
- EXTEND THE MUX SANDBOX SUITE when it helps. If the review surfaces a live/visual behavior that
  M0–M11 do not cover — or you find yourself repeatedly hand-driving a scenario the suite should
  own — add it to `tools/sandbox/SANDBOX-MUX-SUITE.md` as a new `M12+` scenario (match the
  existing Goal/Watch/Verdict shape; note any controlled `psmux -L <socket>` exception; keep the
  black-box ethos for the agent-under-test persona). The suite is meant to grow with mux — this
  is encouraged, not scope-creep. If you touch the suite's scenario set, keep the coverage guard
  green (`go test ./cmd/lyx/...` — `sandbox_coverage_test.go` scans `tools/sandbox/*SUITE.md`
  for the `**Covers:** mux` tag) and honor the Documentation Lifecycle / Sandbox Suite Coverage
  invariant in `CONSTRAINTS.md` in the SAME change.
- MAKE SMOKE TESTS DETERMINISTIC. Timing-sensitive psmux operations are asynchronous: `kill-server`
  returns before the socket is released, and a freshly spawned server takes a variable time to
  accept commands (longer on a loaded machine). A smoke test that assumes a CLI verb is synchronous
  will pass on your quiet machine and FAIL intermittently on a loaded orchestrator box. Wait on the
  actual state transition (poll `has-session` until the server is genuinely down / up) with a
  deadline, rather than sleeping a fixed amount or assuming completion. Verify reliability by
  running the new smoke test many times in parallel under load (e.g. several `go test -tags smoke
  -run <name> -count=3` processes at once), not just once — a single PASS is not proof of
  determinism. If a fix touches a boot/reboot poll, prefer a deadline-based loop over a
  fixed-attempt count in the production code too.
- Keep `go build`/`go vet`/`go test` green after every change. Then RE-DEPLOY (`deploy.cmd`)
  and re-run the smoke + live suite scenarios to confirm the fix holds and nothing regressed —
  re-deploying FIRST is mandatory: the MUX SANDBOX SUITE exercises the deployed `lyx.exe`, not
  your edited tree, so skipping the re-deploy re-tests the OLD binary and gives a false PASS/FAIL.
  (The hand-rolled `go build -o <scratch>/lyx.exe` path self-refreshes each build; the suite path
  does NOT — it is your responsibility to `deploy.cmd` before every suite re-run.)
- Update `docs/modules/mux.md` (and `docs/overview.md` / `CONSTRAINTS.md` if invariants or the
  module table move) IN THE SAME change — reconcile any prose the fix makes stale. Do NOT add
  bugfix/hardening notes to `docs/roadmap.md` (roadmap is for planned milestones only, per
  CLAUDE.md).
- Tear down all psmux state; confirm zero psmux processes.
- Do NOT commit or push unless the user explicitly asks. Report the changed files and how you
  verified each fix.

## Deliverables
1. A structured review report (Executive summary with top risks + merge-readiness opinion;
   Scope assessment plan-vs-shipped; Code findings severity-ranked with file:line + scenario +
   fix + CONFIRMED/PLAUSIBLE; Docs & operability findings; What-was-tested with exact commands
   and observed results, including what you could NOT verify and why). Write it to
   `.scratch/mux-review-<yourtag>.md`.
2. A fixer report: what you implemented, what you deliberately deferred (with reasons), the
   exact test commands run + results, and the changed files. Write it to
   `.scratch/mux-review-<yourtag>-fixer-report.md`.
3. In your final chat message: a concise summary (executive summary + counts by severity + the
   two report paths). Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive real psmux), produce
your independent findings, then implement and verify the fixes.
