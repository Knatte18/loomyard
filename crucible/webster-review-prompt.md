# webster — independent review + fix

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `webster`
module in the loomyard repo, followed by FIXING what you find. Work in the worktree at
`/home/knatte/Code/loomyard/wts/master-builder` (branch `master-builder`). This is a **Linux**
host — the repo's `.cmd`/`tasklist` tooling is Windows-oriented; the Linux equivalents are given
below. Adjust the path/branch if the task lives elsewhere now.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of webster's scope and correctness. Hunt for bugs
   by reading the code AND by driving the real substrate — `lyx webster run` / `begin-batch` /
   `record-batch` / `recover-batch` / `pause` wired to a REAL shuttle spawn (real tmux, a real
   logged-in `claude`, in-session Agent-tool forks) — this is where webster's defects hide, not
   in the hermetic unit tests.
2. FIX: after you have a findings list, implement the fixes one at a time, verify each against the
   real substrate, keep the whole test suite green, and update the docs in the same change as the
   fix they document. COMMIT after each individual fix lands green (see "Commit per fix" below).
   Do NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test, plus the live
smoke/suite check if the finding needed one), and its doc update (if any) is included, COMMIT it —
on the current branch, no push — before starting the next finding. Commit message format:
`webster: fix <finding-id> — <one-line what/why>` (e.g. `webster: fix W3 — begin-batch injects the
oversized role before, not after, writing the fork prompt`). Do not commit `.scratch/` (gitignored;
your review and fixer reports never belong in a commit regardless). This exists because a round
agent's session can be killed mid-fix by something entirely outside the method's control (a
corrupted terminal, a lost connection). A single monolithic uncommitted diff left behind by a crash
forces the orchestrator to reverse-engineer, finding by finding, which fixes are actually complete;
a trail of small commits turns that same crash into something the orchestrator can just read from
`git log`.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to
`.scratch/webster-review-<yourtag>.md` on disk — before you touch (edit, create, or delete) a
single production or test file. Do not fix findings as you go, even ones that look small and
obviously right. A review written or finished after code has already changed is no longer an
independent judgment — it is a post-hoc rationalization of edits you already made. If you catch
yourself wanting to patch something the moment you spot it: don't. Write it down as a finding, keep
reading, finish the review, save the file, THEN start Job 2.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first. Do NOT read any prior review or review-dialogue files before you have
your own list. Specifically do not open anything under `.scratch/` (gitignored; holds prior reviews
`webster-review-*.md` and `*-fixer-report.md`). Reading the design SPEC and the module docs is
expected and required (those are not reviews). AFTER you have written your own independent findings,
you MAY consult the prior rounds' `.scratch/webster-review-*` material — regardless of which model
produced it (rounds rotate across Fable / Opus; the most recent prior round is whichever
`webster-review-*` file is newest), EXCEPT your own `-<yourtag>` deliverables — to (a) confirm
previously-fixed behaviors have not regressed and (b) re-evaluate the deferred items at the bottom.

## What to read
- Code: `internal/websterengine/**` (audit, beginbatch, recordbatch, recoverbatch, chain, config,
  state, roles, render, runlevel, summary, template + the `master-template.md`/`fork-template.md`
  embeds), `internal/webstercli/**` (the 8 verbs: beginbatch, recordbatch, recoverbatch, run,
  status, validate, pause, weft + cli.go), and the `cmd/lyx` integration (`main.go` line ~129
  `webstercli.Command()`, help/registration guard tests).
- The **authoritative as-built design SPEC** (this is not a review — read it fully):
  - `internal/websterengine/doc.go` — webster's own package documentation: the A/B
    contract-compatibility with builder, the bracket-verb shape, idempotent per-batch model
    assertion, cold-recovery escalation, digest persistence, engine/cli weft-blind split,
    crash/resume, and the builderengine reuse inventory (pause + validate pass-throughs). THIS is
    the primary spec for webster's own mechanism-specific behavior.
  - `docs/reference/builder-contract.md` — the `## Webster: the fork-based sibling` section (pins
    the A/B facts a future `loom` treats as interchangeable) plus the whole doc for the shared
    contract webster imports (digest fields, report schema, outcome schema, chain rollback, pause).
  - `docs/reference/plan-format.md` — webster's pinned input contract (same parser as builder).
- Docs: `docs/overview.md`, `manifest/roadmap.md`, `CONSTRAINTS.md`, `README.md`.
- `tools/sandbox/SANDBOX-WEBSTER-SUITE.md` (scenarios W1 = fork loop, W2 = `/model` injection) —
  for SCENARIO IDEAS only. You run every scenario yourself, directly, with your own tool calls; you
  do NOT invoke any `sandbox-webster-suite.cmd` launcher (that spawns a SEPARATE, context-free
  interactive `claude` session for a human operator's own dogfooding — meaningless for you to spawn
  on top of yourself; see "Live driving" below). W1/W2 are a FLOOR — the "High-yield focus" list
  below is your primary script.
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md`
  (Hub Geometry Invariant, CLI/Cobra Invariant, lyxtest Leaf Invariant, Weft Git Invariant,
  Shuttle Provider-Seam Invariant, Sandbox Suite Coverage, Documentation Lifecycle). A change that
  ships behaviour without updating the module doc / invariants in the SAME change is incomplete.
- Design rationale recovery: the `master-builder` build plan's `_mill/` task state was removed by
  the pre-merge cleanup commit. Recover the original discussion decisions with
  `git show c548e223~1:_mill/discussion.md` (decisions referenced by name in `doc.go`:
  `oversized-model-escalation`, `reuse-by-import`, `bracket-is-discipline-not-gate`), and
  `git log --oneline --all -- 'internal/websterengine/**'` for the per-batch build history. Treat
  `doc.go` + `builder-contract.md#webster` as the authoritative as-built contract.

## Mission (assess on two axes, be adversarial)
1. Scope / omfang — is the module's scope right? Does the as-built code deliver what `doc.go` and
   `builder-contract.md#webster` promise? Gaps, over-reach, silently-dropped requirements,
   deferred-that-should-ship-in-v1. In particular: is "holistic review is perch's job, not
   webster's" honored (webster must never itself perform or fake a terminal review), and does
   webster stay strictly contract-compatible with builder (one plan parser, one report parser, one
   outcome parser, one digest contract — imported, never duplicated; import direction
   websterengine → builderengine only)?
2. Correctness — bugs, races, error handling, edge cases; concentrate on the structurally-fragile
   seams below (this is round 1 — read "historically fragile" as "the seams most likely to hide a
   live-only bug by design"). Also assess docs accuracy (do the docs match the code?) and operability.

## High-yield focus — where webster's real bugs live (drive these, do not just read them)
The pure/unit-tested parts (fingerprint hashing, config parsing, outcome YAML decode, plan
validation — most of it imported from builderengine and already hardened) are usually solid;
defects concentrate in webster's OWN mechanism-specific, COMPOSED, live behavior the hermetic tests
never exercise: the fork loop, the in-pane `/model` injection, digest carry, cold recovery, and
crash/resume. Treat each as an INVARIANT you must actively verify by driving the real substrate — a
green `go test` proves nothing here:

- **Bracket discipline is two-layer enforcement, NOT a gate around the fork.** The fork is Master's
  own un-gateable act (it happens inside Master's turn, Go never sees it). Enforcement = template
  discipline (the master template pins begin → fork → record, property-tested) PLUS fail-loud
  detection after the fact. Verify: `record-batch N` with NO prior `begin-batch N` record must
  hard-error (not silently distil a report); the run-exit audit must cross-check fork-transcript
  count against begun-batch count and fail loud on a mismatch. Try to slip a fork past the audit
  (begin without record, record without begin, a batch that forks twice, a batch that never forks).
- **Fork audit / attribution (the `subagents/<id>.jsonl` transcript check).** At the exact moment
  the Agent fork tool call returns control to Master, the fork's transcript file must already exist
  on disk (under `~/.claude/projects/<encoded-cwd>/<sessionID>/subagents/`). The incremental
  per-batch audit's transcript-count-before-report-presence check depends on this flush having
  already happened by call-return time, not merely by session end (SANDBOX W2c). Verify the
  attribution counting is robust: exactly one new transcript per batch (zero = fork never ran; two =
  double-fork), any settle/retry the audit does to tolerate a slow flush is bounded and deterministic
  — never an unbounded or flaky wait, never a false "no fork" on a machine under load.
- **`/model` pane injection for oversized batches — the escalation-vs-fallback decider (SANDBOX W2).**
  `begin-batch` for an `oversized: true` batch synchronously injects `/model <target>` into Master's
  own pane via `shuttleengine`'s `Runner.Inject` before returning its envelope — while the
  `begin-batch` subprocess is itself the foreground tool call inside Master's pane. This is the ONE
  load-bearing, previously-UNVALIDATED production timing. Verify three separately: (a) the model
  actually switches for subsequent calls in the same turn — a MISS here is the BENIGN documented
  fallback (`oversized:` keeps plan-format compatibility but has no spawn effect), not a defect to
  invent a fix for; (b) the injected keystrokes do NOT leak into / corrupt the running foreground
  subprocess's stdin/result — a HIT here (corruption) is the DANGEROUS mode that unconditionally
  forces the fallback; (c) the injection races the still-running subprocess correctly. Also verify
  the assertion is IDEMPOTENT per batch: `begin-batch` injects `master` or `master_oversized`
  (whichever THIS batch declares) fresh every time; there is no de-escalation step and nothing to
  forget on a failure path that skipped `record-batch` — the next batch's `begin-batch` re-asserts
  regardless of what the prior batch left behind. (Webster carries NO implementer/implementer_oversized
  fork roles — forks always inherit Master's current model; confirm no dead per-fork-override path.)
- **Digest persistence carries batch context forward.** Unlike builder (which never persisted its
  Digest), webster MUST: `record-batch` persists the digest into `BatchState.Digest` at terminal
  classification; `begin-batch(N+1)` renders the immediately-preceding batch's digest into the next
  fork's prompt, and a crash-resumed Master reconstructs `{{.progress}}` from the persisted digests.
  Nothing downstream ever re-`Distill`s a report (its originating HEAD may have moved). Verify: after
  `record-batch N`, batch N+1's fork prompt carries batch N's digest read from `state.json`, NOT
  re-distilled; a crash-resumed Master reconstructs progress from persisted digests, not by
  re-reading reports against a moved HEAD.
- **`recover-batch` — bounded, re-entrant long-poll; spawn-or-attach, never double-spawn.** The one
  place webster spawns a genuinely separate process: when a fork reports stuck or writes no report,
  `recover-batch` spawns a fresh implementer as its OWN shuttle/mux strand at the `recovery` role
  (reusing `builderengine.SpawnBatch` by import). EVERY call (including the first) blocks for at most
  `poll_wait_s` and returns either a terminal digest or a running snapshot; a re-entrant call must
  find the strand already recorded in state and skip STRAIGHT to the bounded wait — it must NOT
  spawn a second recovery strand. Verify live: first `recover-batch` spawns one strand; a second
  `recover-batch` for the same batch attaches to the SAME strand (confirm exactly one recovery
  strand in `lyx mux status`), waits bounded, returns. Confirm each call's Bash-tool duration is
  bounded by `poll_wait_s`, never open for the whole recovery timeout.
- **Crash/resume: fresh Master re-drives the first unreported batch.** Forks die WITH Master (same
  process) — so, unlike builder, there is never an orphaned in-flight NORMAL-batch implementer; only
  Master's own strand and a possible recovery strand ever need reclaiming. Resuming = re-running
  `lyx webster run`: entry-time reclaim stops any live recorded strand, then a FRESH Master (never a
  provider `--resume`) is spawned, hydrated from the on-disk register (reports dir + `state.json` →
  `{{.progress}}`), and re-drives the first batch with no terminal record. Verify live: kill Master
  mid-run; re-run; confirm NO double-spawn of Master (the old strand is reclaimed first), the resume
  lands on the first unreported batch, and every card an implementer already committed survives
  independently (only reports + state are weft-committed per batch).
- **Weft scoping — engine is weft-blind; every commit lives in `webstercli` at deterministic
  boundaries.** `websterengine` takes already-resolved directory strings and touches NO weft/git;
  all `_lyx/webster` path construction is in `internal/hubgeometry`
  (`WebsterDir`/`WebsterReportsDir`/`WebsterPromptsDir`), per the Hub Geometry Invariant. Every weft
  commit (state.json, batch report, outcome.yaml, summary.md) happens in `internal/webstercli` at
  exactly: begin-batch, record-batch, recover-batch (spawn AND terminal), and run's exit backstop.
  Neither Master nor its forks ever drive weft/git for webster's bookkeeping (Weft Git Invariant).
  Verify (SANDBOX W1): `state.json` committed at each `begin-batch` (start-SHA + batch entry durable
  BEFORE the fork), and batch report + `state.json` committed at each `record-batch`; inspect the
  weft's own commit log; confirm no engine-level git call exists.
- **Pause / fingerprint / validate / zero-batch gates (imported builderengine against webster dirs).**
  `BeginBatch` checks `PauseRequested` at the begin-batch boundary — the ONLY place a pause gate can
  fire (Master's fork call is un-gateable) — and refuses with webster's own `ErrPaused`. `Run` clears
  the pause flag once it is committed to spawning (a resumed run must never instantly re-pause on the
  very flag it is resuming from) AND at every non-paused terminal outcome (a paused terminal
  deliberately leaves the flag as the operator's record — see `mapMasterDone`). `Run` refuses a plan
  that parses to zero batches (Validate itself carries no such check — "nothing to build" must never
  resolve to a vacuous `done`). Fingerprint mismatch + `--fresh` is archive-never-refuse (imported).
  Verify live: pause mid-run → the in-flight batch finishes normally, the NEXT `begin-batch` refuses
  with `ErrPaused`; a paused terminal leaves the flag; a fresh `run` clears it and does not
  instantly re-pause; a zero-batch plan is refused loud.
- **`summary.md` gate (webster-only artifact).** Alongside `outcome.yaml`, webster writes
  `_lyx/webster/summary.md` — first line `# <title>`, then a non-empty narrative of what shipped
  (incl. deviations). REQUIRED (presence + non-empty + title line, fail-loud) ONLY when
  `outcome: done`; a stale pre-existing summary follows archive-never-refuse. Verify: an
  `outcome: done` run with a missing / empty / title-less summary fails LOUD; a non-done terminal
  does not require it.
- **Co-versioning: `master-template.md` / `fork-template.md` ↔ Go parsers.** Both are `//go:embed`'d
  and filled via `internal/stencil`; each is half of a Go-parsed contract (the fields the fork emits
  into its batch-report that `Distill` parses; the `{{.progress}}`/role/digest fields the master
  template consumes). Drift is silent, not a compile error. Deliberately hand-edit one side (rename a
  field the template tells the fork to emit) and confirm `template_test.go`'s property tests actually
  catch it — a drift here breaks silently in production.

## Explicitly OUT of scope for webster v1
- **Holistic / terminal review of the plan's output.** That is perch's job
  (`internal/perchengine`), driven separately by `loom` or an operator running `lyx perch run`
  AFTER `webster run` returns `done`. Its absence from webster's own loop is correct — do NOT flag
  it as a gap. DO flag it if you find webster's code secretly performing or faking any part of it.
- **`loom`'s phase-machine wiring.** `loom` (not yet built) will drive `webster run` (or
  `builder run`) as one phase, gated by `perch`. webster must not contain loom-specific orchestration.
- **The `websterv2.md` redesign (DAG-based intra-batch parallelism, worktree-isolated parallel
  cards, atomic-card dependency lists).** `manifest/designs/websterv2.md` is a DRAFT FUTURE-REDESIGN
  CONCEPT — it is NOT the v1 spec. webster v1 is deliberately strictly sequential (one in-session
  fork per batch, batches in DAG order, same as builder), and that is CORRECT, not a missing feature.
  Do NOT flag v1 for failing to match websterv2's parallel-card design. Judge v1 solely against
  `doc.go` + `builder-contract.md#webster`.
- **Non-Claude engines.** Per `CLAUDE.md`, non-Claude LLM support is not a current priority — don't
  flag the absence of a Gemini/other-provider path. (webster's `/model` injection and Agent-tool
  fork are legitimately Claude-Code-specific by design.)
- **Changing shuttle's outcome contract.** Do NOT modify `internal/shuttleengine/wait.go`'s
  Stop-without-output⇒`OutcomeAsking` classification, the `Outcome` enum in `engine.go`, or the
  run-loop. That done-or-asking model is correct for shuttle's other clients (builder's single-
  deliverable implementers) and is a deliberately-accepted limitation webster works around in its own
  package (`await-batch`). A true async/parallel Master would need a shuttle "still-working" third
  state — that is a SEPARATE future shuttle task, explicitly not this campaign's. Your R2-a mechanism
  guard belongs in the **claudeengine hook seam** (`settings.go`, like `steerAgentNonForkDeny`), NOT
  in shuttle's outcome classification. Keep v1 strictly serial — no fan-out / await-many / parallel
  fork orchestration (that is websterv2).

## Round context seeded from prior-round verification
You are round tag `opus-r4` — **round 4**, the **LAST authorized round** of an initial cap of **up to
4 rounds**, model rotation **Fable → Opus → Fable → Opus**. **This is a SAFETY PASS.** Rounds 1, 2,
and 3 have all been independently orchestrator-verified. Crucially, **round 3 (the previous safety
pass) did NOT come back clean** — a genuinely independent Fable pass found a **BLOCKING happy-path
run-killer** (FR3-1: the run-exit audit false-failed a fully-successful `done` run whenever Master
spelled its contract-file Write path relatively) plus two more confirmed-live MEDIUMs, all in seams
the earlier fork-loop deadlock had blocked from being reached. That is exactly why a fresh-model
safety pass has value: **the last two rounds each found real bugs the prior cold verification missed.**
Your job is the belt-and-suspenders check the method requires before merge: a genuinely independent
clean-room pass — with an OPUS lens this time — to find anything the three prior rounds missed, OR to
honestly confirm merge-readiness. **"No new defects, ship it" is the EXPECTED, valuable outcome of a
safety pass — do not invent work to justify the round.** Equally, do not rubber-stamp: round 3 proves
these now-reachable seams still hide bugs — genuinely exercise them, especially the ones round 3 just
touched.

**CLOSED AND ORCHESTRATOR-VERIFIED — do NOT re-litigate (confirm no regression only):**
- Round 1 (`fable-r1`, commits `aea58060..51b65cf2`): F1–F18, 18 findings. Headline: on Claude Code
  2.1.205 the Agent-tool fork is **BACKGROUNDED, not synchronous**; F1's fix is the current
  authoritative design — `begin-batch` → spawn the (async) fork → **`lyx webster await-batch <NN>`**
  polled in a loop by the master template until the report lands → `record-batch`, strictly serial.
  Also: F2 (fork audit miscounted the parent's spawning `Agent` call), F3 (weft-path read wedged
  record-batch), F18 (`/model` injection's leading **Escape** interrupted the foreground begin-batch
  — dropped), F4–F17.
- Round 2 (`opus-r2`, commits `10ffa16b..9cddf9d9`): F-R2a — the **deterministic fork-context guard**
  (`steerWebsterForkDeny`, a `PreToolUse(Bash)` hook in `claudeengine/settings.go`, gated on
  `forkSubagents`, keyed on the fork payload's top-level `agent_id`/`agent_type:"fork"`) that refuses
  a fork's `lyx webster` call while Master's own + a fork's git/verify Bash pass — this is what makes
  the deadlock mechanically impossible; recovery strands run `ForkSubagents:false` so they are
  unaffected. F-R2b — the `//go:build smoke` suite (`internal/webstercli/smoke_test.go`). Plus two
  LOWs.
- Round 3 (`fable-r3`, commits `804c6b83..9d9b4642`): 8 findings, 3 CONFIRMED live. **FR3-1
  (BLOCKING):** `CheckParent`/`CheckFork` now canonicalize each transcript-recorded write path via
  `resolveWritePath(workdir, path)` (relative → resolved against the pane cwd) before comparing to the
  absolute contract paths — killed a false `parent-write` violation that failed healthy `done` runs
  on a path-spelling coin-flip. **FR3-2 (MEDIUM):** `RecordBatch` now audits the bracket-OPENING
  session (`bs.SessionID`), not the current Master session, so a resume after a crash between a fork's
  report landing and `record-batch` can still consume that report (the crashed session's fork
  transcript persists on disk); + a Kind guard refusing a recovery-kind batch loud, a template resume
  rung, and a live smoke `TestSmoke_RecordBatchConsumesCrashedSessionReport`. **FR3-3 (MEDIUM):**
  `shuttleengine.ForkReport.WritePaths` + a `fork-contract-write` hard-violation class flag a fork
  writing Master's `outcome.yaml`/`summary.md`. **FR3-4 (MEDIUM):** master template hardened against a
  fork reasoning "I am the Master" past the disambiguation banner. FR3-5/6 (LOW: progress-trail
  status-awareness; model-injection-is-last-fallible-act ordering), FR3-7 (doc pin), FR3-8 (NIT flag
  help). **W2 `/model` was closed live in round 3:** on Claude Code 2.1.205 the injected `/model` is
  QUEUED for Master's whole single turn and never executes mid-run — the oversized escalation degrades
  to its documented benign fallback (forks inherit the launch model); (b) no foreground corruption
  holds post-F18; (c) exactly one transcript attributed per batch. This is now pinned in `doc.go`
  (FR3-7) — do NOT re-litigate whether oversized "should" switch the model mid-run; it structurally
  cannot on 2.1.205, and that is documented, not a defect.

**Your mandate this round:**
1. **Independent safety pass (Opus lens).** Read the SPEC + current code yourself; drive the real
   substrate against every "High-yield focus" invariant. Form your own findings before consulting any
   prior `.scratch/webster-review-*` material.
2. **Regression-check round 3's fresh fixes specifically** (these are the newest, least-weathered
   code): the relative-path write resolution (`resolveWritePath` — probe odd spellings: `../`
   escaping the workdir, a symlinked cwd, an absolute path that differs only by a trailing slash or
   `//`, a Windows-style path if the parser ever sees one; does the canonicalization ever FALSE-flag a
   legitimate Master write, or MISS a genuine rogue one?); the FR3-2 bracket-opening-session keying
   (does any OTHER resume path — recover-batch, --fresh, a double crash — now key on the wrong session,
   or does record-batch's new Kind guard reject a legitimate call?); FR3-3's fork-contract-write class
   (can a fork evade it, e.g. via a relative path FR3-1's resolution then normalizes AWAY from the
   contract path?); FR3-4's template wording (does the new banner rung ever mis-fire and confuse a
   legitimate Master?). Also the seams round 3 exercised but you should re-drive independently:
   `await-batch` under a slow/never-arriving report; the R2-a guard vs `recover-batch`; crash/resume
   THROUGH the async await loop.

If your safety pass finds real defects, fix them (all severities incl. NIT, per the discipline below)
— finding real bugs is itself the signal to keep hardening. If it finds none, say so plainly with the
transcript of what you drove as evidence. **This is the last authorized round: an honest, evidenced
"clean — ship it" is the outcome that lets the campaign converge.**

State the **merge bar** so you calibrate: correctness in the NORMAL single-instance flow (one
`lyx webster run` at a time, no artificial concurrency stress) is the gate. If you run N× concurrent
runs against the same worktree as a diagnostic amplifier, a timeout or lock-contention under that
artificial peg is NOT itself a defect — but any state corruption, double-spawn (a second Master, a
second recovery strand), or silent data loss IS, regardless of how much concurrency it took to
surface it.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

**Environment check FIRST.** This is a Linux host. Confirm up front: `tmux` on PATH, a logged-in
`claude` on PATH, `lyx` on PATH, `go` on PATH. (At authoring time all four were present:
`/usr/bin/tmux`, `/home/knatte/.local/bin/claude`, `/home/knatte/.local/bin/lyx`, `/usr/bin/go`.)
If any is genuinely missing, that is a real environment gap — flag it specifically and say what it
blocked; it is the ONLY legitimate "cannot verify headlessly" reason besides a scenario that
structurally needs a human's physical eyes.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/websterengine/... ./internal/webstercli/... ./internal/shuttleengine/...`
- `go test -count=5 ./internal/websterengine/... ./internal/webstercli/... ./cmd/lyx/...`
- **`go test -tags integration -count=2 ./internal/websterengine/... ./internal/webstercli/...`** —
  DO NOT skip this. Webster's `beginbatch/recordbatch/recoverbatch/runlevel/chain _test.go` are ALL
  `//go:build integration` (Tier-2, real scratch git repo + call-scripted fakes). A plain
  `go test -count=5` compiles and runs NONE of them — the FR3-2 keying test, the FR3-1/FR3-3 audit
  tests, and the template-drift guards all live behind this tag. Rounds 1–2's gate missed it.

Live smoke (real substrate, behind the `smoke` build tag):
- `go test -tags smoke ./internal/webstercli/... -run Smoke -v -count=1` — webster now has **5**
  `//go:build smoke` tests (`internal/webstercli/smoke_test.go`), the newest being
  `TestSmoke_RecordBatchConsumesCrashedSessionReport` (FR3-2, spawns a real fork under one session and
  drives RecordBatch under a different session). Keep accreting: for every NEW live-only defect you
  fix, add a deterministic smoke test that walks the failing scenario against the real substrate
  (follow the existing ones' pattern, incl. the substrate-absent skip).

Live driving — YOU drive it directly, no launcher (PRIMARY — where the bugs surface):
- **Deploy on Linux:** `go run ./tools/deploy -dest /home/knatte/.local/bin` (this is the Linux
  equivalent of the Windows `deploy.cmd`; it builds the current source and installs `lyx` onto the
  PATH). **FOOTGUN:** live driving runs the DEPLOYED snapshot, not your working tree — re-run this
  deploy after EVERY source change or you validate a stale binary. Deploy first, always.
- **Materialize a throwaway test hub yourself** (there is no `sandbox-build.cmd` on this host): make
  a temp dir OUTSIDE this worktree (e.g. under `/tmp` or the session scratchpad — never inside the
  repo, never a second git worktree of loomyard), `git init` it, `lyx init` it (webster needs
  `_lyx/config/webster.yaml` + `shuttle.yaml`/`mux.yaml`), `lyx mux up`, then write a tiny plan
  under `_lyx/plan/`. Keep plan cards TRIVIAL — e.g. "create `resultN.md` containing the single line
  `OK`" — so a real fork finishes each batch in one card, one commit, fast.
- **Do NOT invoke any `sandbox-webster-suite.cmd` launcher.** It spawns a SEPARATE, context-free
  interactive `claude` session for a human operator's own dogfooding — meaningless for you to spawn
  on top of yourself. Run the real CLI commands yourself, directly, foreground, waiting for each to
  return: `lyx webster validate` / `run` / `begin-batch` / `record-batch` / `recover-batch` /
  `pause` / `status`. This spawns real tmux panes and a real interactive `claude` Master session
  underneath (webster's own substrate via shuttle) — that is expected and required. None of it needs
  an attached TTY of its own: a tmux pane is a real pty regardless of whether anyone is watching it.
- Walk the "High-yield focus" list (and W1/W2's scenarios for extra ideas) and record OK/WARN/FAIL
  for each. The list is a FLOOR — devise and run MANY more adversarial scenarios of your own
  (record-batch without begin-batch; a double-fork batch; a crash between begin-batch and
  record-batch then resume; recover-batch re-entrancy; pause racing the last batch settling; an
  oversized batch's `/model` injection timing).
- **"Headless" means "no human required" — NOT "no time/token cost to me."** A real Master/fork
  session doing real work takes real wall-clock MINUTES, not seconds. That cost is EXPECTED and
  BUDGETED FOR, never a reason to skip a scenario. **You are explicitly forbidden from writing
  "operator-assisted", "cost-bearing", "long-running", "impractical", or "automated context" as a
  reason to skip live driving** — those words describe a cost to YOU, never a reason a human is
  required. (Builder's first hardening round skipped its entire live suite citing exactly those
  words; it was a rationalization, not a real blocker.)
- **Before writing "could not verify", ask literally: "would a human's physical eyes be required
  here, or am I just avoiding spending my own time/turns?"** Only the first is a real reason. If a
  scenario just takes several minutes of you waiting on a real command to return, wait for it, and
  report the actual output (with the commands you ran) as evidence — not a summary claim.
- **W2's `/model` injection (a/b/c) is the highest-value live scenario** — it decides whether the
  oversized-batch escalation feature stays enabled or degrades to its documented fallback. Drive it
  for real and record all three assertions separately; a benign miss on (a) is fine, a corruption
  hit on (b) is a real defect.

TEARDOWN DISCIPLINE (critical): if you start any substrate (Master's strand, a recovery strand,
`lyx mux up`), tear it down (`lyx mux down`). At the end confirm ZERO stray substrate: `lyx mux
status` lists nothing, and `pgrep -a tmux` shows no leftover server for your test hub. Leave no
stray state. Be honest about what you could NOT verify and why.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong
behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/traced)
vs PLAUSIBLE (looks wrong, unverified). For scope: doc-promised vs shipped; flag
deferred-that-should-be-v1 and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings you record get
fixed in Job 2 — including every NIT — not just BLOCKING/MEDIUM ones. A finding you write down but
leave unfixed as "low priority" is not actually a reported finding; it is a dropped one that will
either silently vanish or re-surface and loop across future rounds instead of closing. The only
legitimate reason to leave a finding unfixed is that fixing it genuinely requires something you
cannot do alone this round — an operator decision on a real design tradeoff, or a live capability
you don't have. Even then say so explicitly in the fixer report's deferred section — never bucket
something as "deferred, low priority" just because it felt small.

## Deferred items from the prior rounds — RE-EVALUATE these (after your own pass)
Nothing outstanding to FIX — R1's residuals (R2-a fork-loop deadlock guard, R2-b smoke tests) closed
in R2; R3 fixed all 8 of its own findings (FR3-1..FR3-8), nothing deferred. The only consciously-
carried item is an **accepted limitation, not a defect to fix**: `await-batch` has no Go-side upper
bound / fork-liveness signal — a mis-looping Master only stops at `master_timeout_min`. This is
inherent to the backgrounded-fork + shuttle done-or-asking model; a "still-working" third state in
shuttle is explicitly OUT OF SCOPE for this campaign (future shuttle task). The R2-a guard already
forces a mis-driving fork to abandon the loop, which is the in-scope mitigation. Do NOT re-open it as
a finding; note it only if you discover it is worse than characterized. If your own pass surfaces
something new you cannot resolve alone this round, defer it here explicitly with the reason; do not
silently drop it.

## Fixing — after the review
- Fix EVERY finding from your review, all severities including NIT.
- Load the code-quality guidance (`/code-quality` skill) AND `mill:golang-build` /
  `mill:golang-testing` / `mill:golang-comments` before editing — ALL of the relevant skills, not
  code-quality alone. Prefer surgical edits; match existing style and the file-level doc-comment
  convention (see `doc.go`).
- For every bug you fix, add or extend a test that would have caught it. For a live-only defect, add
  a `//go:build smoke` test that walks the failing scenario against the real substrate (webster has
  none yet — you are starting this file; follow another module's smoke-test pattern incl. a skip
  when the substrate is absent). A hermetic unit test for the pure helper is good; a smoke test for
  the composed behavior is what protects the recovery paths.
- MAKE SMOKE TESTS DETERMINISTIC. Substrate operations are asynchronous; wait on the actual state
  transition (poll with a deadline), never sleep a fixed amount. Prove determinism by running the
  new test many times in parallel under load, not once.
- If your review surfaces a live/visual behavior `SANDBOX-WEBSTER-SUITE.md` doesn't cover, EXTEND it
  (match the W1/W2 scenario shape; keep `sandbox_coverage_test.go`'s `**Covers:** webster` guard
  green in the SAME change). Creating a brand-new suite file/launcher is NOT required.
- Keep `go build`/`vet`/`test` green after every change. Then RE-DEPLOY
  (`go run ./tools/deploy -dest /home/knatte/.local/bin`) and re-run every live scenario yourself —
  re-deploying FIRST is mandatory.
- Update `internal/websterengine/doc.go` and/or `docs/reference/builder-contract.md` (and
  `docs/overview.md` / `CONSTRAINTS.md` if invariants or the module table move) IN THE SAME change
  as the fix. Do NOT add bugfix/hardening notes to `manifest/roadmap.md` (roadmap is planned milestones
  only, per CLAUDE.md).
- Tear down all substrate state; confirm zero stray processes. COMMIT each fix as you finish it — do
  NOT push unless the user explicitly asks. Report the changed files and how you verified each fix.

## Deliverables
1. A structured review report (Executive summary with top risks + merge-readiness opinion; Scope
   assessment doc-vs-shipped; Code findings severity-ranked with file:line + scenario + fix +
   CONFIRMED/PLAUSIBLE; Docs & operability findings; What-was-tested with exact commands + observed
   results, including what you could NOT verify and why). Write it to
   `.scratch/webster-review-<yourtag>.md`.
2. A fixer report: what you implemented, what you deliberately deferred (with reasons), the exact
   test commands run + results, and the changed files. Write it to
   `.scratch/webster-review-<yourtag>-fixer-report.md`.
3. In your final chat message: a concise summary (executive summary + counts by severity + the two
   report paths + an explicit merge-readiness verdict). Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive the real substrate),
produce your independent findings, then implement and verify the fixes.
