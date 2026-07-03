# mux — reusable review+fix prompt

This file is a **complete, self-contained prompt**. To run another review+fix cycle on the
`mux` module, paste everything between the `>>> BEGIN PROMPT` and `<<< END PROMPT` markers into
a fresh thread (adjust the worktree path / branch / spec sha if they have changed). Nothing
outside those markers is part of the prompt — this preamble is just documentation.

The prompt is designed around the hard lesson from the first round: **mux's bugs do not show up
in `go test` or the single-strand smoke test — they only surface when you drive real psmux
through the multi-pane, dead-pane, crash, and cross-worktree paths.** The prompt therefore
forces hands-on live psmux driving of specific scenarios, then authorizes fixes.

---

```
>>> BEGIN PROMPT

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
MAY consult `.scratch/mux-review-fable5*.md` (a prior round's review + fixes + fixer-report) to
(a) confirm previously-fixed behaviors have not regressed and (b) re-evaluate the deferred
items listed at the bottom of this prompt — but only AFTER your own pass, so the independent
signal is preserved.

## What to read
- Code: `internal/muxengine/**` (incl. `render/**`), `internal/muxcli/**`, and the `cmd/lyx`
  integration (`main.go`, sandbox/help/registration guard tests).
- Docs: `docs/modules/mux.md`, `docs/research/mux-exploration.md`,
  `docs/research/mux-hooks-exploration.md`, `docs/overview.md`, `docs/roadmap.md`,
  `CONSTRAINTS.md`, `README.md`.
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

Live psmux driving (this is the part that finds the bugs). Build the binary
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
- Keep `go build`/`go vet`/`go test` green after every change; re-run the smoke + live scenarios
  to confirm the fix holds and nothing regressed.
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

<<< END PROMPT
```
