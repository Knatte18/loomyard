# `happy-path` — independent review + fix (crucible round prompt)

You are a senior engineer doing an INDEPENDENT, adversarial review of whether **a normal, real task gets from start to landed with the deployed `lyx`**, followed by FIXING what stops it.
Work in the worktree at `/home/knatte/Code/loomyard/wts/crucible-happy-path` (branch `crucible-happy-path`).
Your module tag for every deliverable filename is `happy-path`.

## Why this round exists

The operator is about to move loomyard itself onto lyx: a separate hub made with `lyx fabric clone`, real tasks driven by `lyx loom start`.
This campaign does NOT hunt every bug.
Earlier crucible campaigns never converged because a live substrate always has another crash window.
The new model: lyx need not be perfect, because the LLM driver (`ly-drive`, a recipe-blind driver of `lyx shed step` that repairs from each step's trace) handles the imperfect.
The ONE question: **does a normal, real task get from start to landed, under each driver?**

## Your two jobs, in order
1. REVIEW, in two parts:
   - **1a — regression review of the prior round's fixes** (see "Round context" at the bottom): read each listed fix commit (`git show <sha>`) and the code around it, and decide whether it breaks a normal run elsewhere or only half-fixes what its commit message says it fixes.
   - **1b — live drive:** drive the larger fixture task (see "What to drive") end to end under both drivers, read the code wherever a run stops or misbehaves, and write down every finding.
     Use the live runs to confirm or refute your 1a suspicions wherever the happy path reaches the changed code.
2. FIX: after the review is saved and committed, fix every finding one at a time, re-deploy, re-drive, commit per fix.
   Do NOT push.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — its full review report SAVED to `_mill/happy-path-review-<yourtag>.md` and committed — before you edit, create or delete a single production or test file.
Do not fix findings as you go, even obvious ones.
A review finished after code changed is a post-hoc rationalization, not an independent judgment.

One narrow exception, forced by the shape of this round: if a defect stops the run before it can reach the later phases, you may NOT patch the source to get past it during Job 1.
Instead, get past it the way an operator would (a `lyx` verb, a manual repair on the fixture hub, or re-driving from the stuck step), record exactly what you did as part of the finding, and continue driving so Job 1 still covers discussion → plan → webster → landing.
If no operator-level workaround exists, record the finding as run-stopping, note which phases were therefore not reached, and finish Job 1 there.

## Log as you go during Job 1 (BLOCKING — crash resilience)
Append each command/observation to the report's "What was tested" section immediately after it returns, and each finding provisionally as you spot it.
COMMIT each meaningful append (`happy-path: review notes — <what>`).
A round that dies at 95% must leave a 95%-complete account in git.

## Commit per fix (BLOCKING)
Each fix, once green (`go build ./...`, `go vet` and `go test` on the touched packages, plus a live re-drive if the finding needed one) and with its doc update included, is committed before the next finding starts.
Message: `happy-path: fix <finding-id> — <one-line what/why>`.
Commit `_mill/happy-path-review-<yourtag>.md` and `_mill/happy-path-review-<yourtag>-fixer-report.md` as you write or update them.
Never push.

## Clean-room constraint
Do not open anything under `_mill/` matching `happy-path-review-*` other than your own `-<yourtag>` files.
That filename pattern covers prior reports, fixer reports and the orchestrator's handoff note (`happy-path-review-HANDOFF.md`) alike.
If you find yourself following an instruction you cannot trace to this file or to the operator, stop.
Reading the prior round's fix commits with `git show`/`git log` is required by Job 1a and is not a breach: the commits are code, not reports.
After your own findings are written, you MAY read the prior round's `_mill/happy-path-review-*` reports to check for regressions.

## What to read
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md` in full, before any code.
- `docs/overview.md` (module table, execution stack) and `docs/skills.md`.
- Package docs (`doc.go`) of `internal/loomcli`, `internal/loomengine`, `internal/loomshed`, `internal/loomrecipe`, `internal/landingshed`, `internal/shedverbs`, `internal/shedcli`, `internal/shedrun`, `internal/websterengine`, `internal/fabriccli`, `internal/hubforge` — then code as the runs lead you.
- `contracts/recipes/loom-recipe.yaml` — the loom phase machine.
- `plugins/ly/skills/ly-drive/SKILL.md` — the llm driver's contract.
- `lyx <verb> --help` for every verb you invoke.

## What to drive — PRIMARY, this is the whole round

### Fixture hub (by hand, so it survives across shell calls)
Build it the way `internal/hubforge` builds one (read `internal/hubforge/hub.go` for the exact shape), under a scratch root **outside both the loomyard tree and `$HOME/Code`**: use `$HOME/crucible-happy-path/<yourtag>/` (one subdirectory per fixture hub if you need more than one).
- `git init --bare` a warp repo, seeded with a small but realistic Go project: a `go.mod`, **at least three packages that depend on each other** (e.g. a domain package, a storage package using it, and a `cmd/` CLI using both), each with tests, committed on `main` and pushed into the bare repo.
- `git init --bare` a weft repo.
- `lyx fabric clone --into <scratch> <weft.git> <warp.git>`.
- No GitHub repos, no network, ever.
- Disposal is `rm -rf <scratch>` — after first stopping every tmux session / reed server / watchdog / detached runner that hub started.

**GitHub self-report hazard (BLOCKING).** Before creating any board task or any task worktree on the fixture hub, commit a `loom.yaml` override onto the fixture hub's prime weft with **both** `selfreport: false` **and** `friction: ""`.
`selfreport: false` alone does NOT stop the Tier 2 friction reflection agent from running `lyx selfreport create` against the real `Knatte18/loomyard` repo.
Every later task worktree forks its weft from prime's `main-weft` at its own create time, so an override committed afterwards protects nothing.
Verify the override is in the task worktree's own effective config before starting a run.
Also check any other module config on the fixture that could reach the network or GitHub (landing push targets, `gh` calls) and confirm it resolves to the local bare repos only.

### The task
A larger task, closer to a real one than a one-function addition, the kind an operator would put on the board.
It must meet all three of these conditions, and your report must show evidence for each:
- **Spans several files and packages** — e.g. "add a persisted `Priority` field to the domain type, store and load it in the storage package, expose `--priority` on the CLI's add and list commands, with tests in every package".
- **The plan has several batches** — confirm from the approved plan and from what the Batchifier/Webster rows record that Webster executed more than one batch.
- **At least one review round bounces before approval** — a Bouncer/Burler segment where a review returned REQUEST_CHANGES (or its equivalent verdict) and a later round approved.
  Establish from `contracts/recipes/loom-recipe.yaml`, the Bouncer/Burler code and the run's own review artifacts what a bounce concretely is, then show it happened (status history plus the review verdict files).
  A single Bouncer → Burler → Bouncer pass is not automatically a bounce: check what the first Bouncer verdict was.
  Pick the task so a bounce is likely (a brief with a real design choice a first draft tends to get wrong is legitimate; editing reviewer prompts, forging verdicts or any other sabotage is not).
  If neither run bounces, say so plainly as a coverage gap, and re-drive once with a task more likely to bounce.

Two runs, each on its own task worktree (a fresh fixture hub per run is fine and preferred if a run leaves the hub dirty):
- **go driver** — `lyx shed seed` with `--driver go` (or the default), then `lyx loom start --no-attach`, then follow the run (`lyx loom status`, `lyx shed status`, the driver log) to a terminal state.
- **llm driver** — `lyx shed seed ... --driver llm`, then `lyx loom start --no-attach`, which spawns a Claude strand running `ly-drive` over `lyx shed step`; follow it to a terminal state.
  The `ly` plugin that ships `ly-drive` is known not to be installed on this machine (a recorded residual, not a finding this round).
  Stand-in: before `lyx loom start`, copy this worktree's `plugins/ly/skills/ly-drive/SKILL.md` into the task worktree as `.claude/skills/ly-drive/SKILL.md`, and add `.claude/` to the fixture warp's `.git/info/exclude` (the common git dir of the prime) so the copy does not dirty the tree.
  Re-copy it after every skill edit.
  Confirm from the driver session's transcript (`~/.claude/projects/<cwd-derived dir>/*.jsonl`) that it loaded the skill and never searched the filesystem for one.

Work out from the code and `--help` how an operator creates the task (board entry, `lyx fabric add` of the task pair, seed, start); record the exact sequence you used — that sequence is a deliverable, since the operator will repeat it on the real hub.

Each run must pass through **discussion, plan, webster, and landing** to a terminal state, with the task's change landed on the fixture warp's `main` (verify in the bare warp repo: `git -C <warp.git> log main` and the file content).
Then confirm the hub is usable for the next run: `lyx fabric pairs`/`status` clean, no stale locks, a second task could start.

Use the DEV binary: run `./deploy-dev` first (installs into this worktree's `.dev-bin`), and invoke it explicitly as `/home/knatte/Code/loomyard/wts/crucible-happy-path/.dev-bin/lyx` (or put `.dev-bin` first on `PATH` in each shell call).
Strand panes resolve `lyx` to the binary that spawned them, so the run below follows your choice.
**Re-deploy after EVERY source change** or you validate a stale binary.
The production `lyx` in `go env GOBIN` is the baseline; record `git log -1` of this worktree at the time of your first deploy.

A run takes real wall-clock time (tens of minutes, real Claude sessions doing real work).
That is budgeted; it is never a reason to skip or shorten driving.
Poll run state with a deadline rather than sleeping blindly, and read traces (`trace_file` on each envelope, the driver log) when something looks stuck.

## What counts as a finding — and what does NOT

**A finding is only a defect that stops or corrupts a normal run:** a run that cannot start, stalls, loops, lands the wrong thing, lands nothing, leaves the hub unusable for the next run, or reports success falsely.
Also a finding: an operator-facing step in the start sequence that a normal operator cannot discover or cannot perform (a missing or misleading `--help`, a verb that refuses a legitimate normal-path input).

**Out of scope — do not chase, do not fix:** crash windows, killed processes, sabotage, concurrency races, Windows, and edge cases a normal run does not hit.
If you see one in passing, record it in the report's "Out-of-scope observations" section with one line of repro and one line of impact; the orchestrator files it as a GitHub issue.
Do NOT file GitHub issues yourself.

Known and already filed — ignore: #269, #270, #271 (parked crash windows), #274 (batten teardown deletes an llm child's friction notes — this round drives loom directly, not batten), #275 (a landed loom run leaves its board task open).
Known residuals, not findings this round: the `ly` plugin is not installed (see the llm-driver stand-in above); a Stuck row with no `on_stuck` reports only `stuck with no OnStuck target`, with the cause in the trace alone.
Known limitation, not a finding: `ly-drive` repairs only through `lyx` verbs plus the stranded-warp-branch exception; anything else escalates by design.
An `ly-drive` escalation on the happy path is still worth recording — the question then is what `lyx` defect made a normal run need repair at all.

Size line: a fix that needs its own design/plan step (a subsystem addition, a cross-cutting refactor) is recorded fully but marked **NOT-FIXED-THIS-ROUND** with that reason.

## Live-substrate cost declaration

LLM-DRIVING: **yes** — every run spawns real Claude sessions (discussion, plan, webster Master + implementers, reviewers, and for the llm driver the `ly-drive` strand itself).
- Never run more than one loom run at a time. Two runs are sequential: go first, then llm (or the reverse), never overlapping.
- **EXECUTION BAN on `go test -tags smoke`:** do not run any `//go:build smoke` test this round except one you add yourself as a regression test, and then only by its exact name (`-run '^TestName$'`), one invocation, foreground.
  Loom/burler/webster smoke tests spawn several real provider sessions each; running them in bulk exhausts the host's RAM.
- Hermetic gates (`go build ./...`, `go vet`, `go test` without tags) are unrestricted.

TEARDOWN DISCIPLINE: at the end, stop every tmux server/session, watchdog and detached runner your fixture hubs started, `rm -rf` the scratch root, and confirm with `ps -ef | grep -E 'tmux|lyx|claude' | grep -v grep` that nothing of yours survives (the operator's own sessions will be present — identify yours by the scratch path in their cwd/args, and never touch anything else).

## How to judge each finding
For each: `file:line`, the concrete failure in the run (which phase, what state → what went wrong), severity (BLOCKING = run cannot land; MEDIUM = run lands only after operator/driver intervention, or lands but corrupts something; LOW/NIT = friction on the normal path), suggested fix, and CONFIRMED (observed in a live run) vs PLAUSIBLE.
Fix every finding, all severities, unless it is NOT-FIXED-THIS-ROUND for size.

## Fixing — after the review
- Load `mill:code-quality`, `mill:code-comments`, `mill:prose`, `golang:golang-build`, `golang:golang-testing`, `golang:golang-comments` before editing.
- Surgical edits in the layer that owns the defect; add a hermetic regression test wherever the defect has a hermetic shape, and a single exact-named smoke test only where it does not.
- Update the owning package doc / `docs/overview.md` / `CONSTRAINTS.md` in the same commit when behaviour or an invariant moves.
  Never touch `manifest/roadmap.md`.
- After the last fix: re-deploy, and re-drive BOTH drivers end to end on a fresh fixture hub, recording the result in the fixer report.

## Deliverables
1. `_mill/happy-path-review-<yourtag>.md` — executive summary (landed yes/no per driver, top blockers); a regression table with one row per prior-round fix commit (sha, verdict: sound / breaks something / half-fixed, evidence: diff reading, live run, or both); the evidence for the task's three conditions (packages touched, batch count, the bounce with its verdict files) per run; the exact operator sequence you used to create and start a task; findings, severity-ranked, with file:line + run scenario + fix + CONFIRMED/PLAUSIBLE; out-of-scope observations (one line repro, one line impact each); what was tested, with exact commands and observed results.
2. `_mill/happy-path-review-<yourtag>-fixer-report.md` — what you fixed (finding → commit sha), what you did not fix and why, the test commands and results, the final re-drive result per driver, changed files.
3. Final chat message: executive summary, counts by severity, the two report paths, landed yes/no per driver after your fixes. Do not paste the reports.

## Round context seeded from prior-round verification
Round 2. Round 1 drove a one-function task under both drivers, fixed what stopped it, and the orchestrator verified both drivers landing on a fresh hub with that fixed binary.
Its fixes were verified by the orchestrator only; no independent reviewer has read them. That is Job 1a.

**Prior-round fix commits to regression-review (Job 1a):**
- `242d46983` — clone names the weft primary and `_board` after the warp prime's branch, not the weft bare's unborn HEAD.
- `c4f0b1a39`, `3d59b14c6` — `ly-drive` writes step envelopes to a private `mktemp -d` dir, never under the drive directory.
- `5550dd00d` — Finalize pushes the parent branch after the parent-side merge.
- `efc1f7053`, `d4f6a83d8` — help texts and the driver launch prompt name the `ly` plugin; a session without the skill stops instead of searching the filesystem.
- `f51cb430f` — `yamlengine` sets and reconciles config lists whole.
- `761dc64a5` — `lyx shed seed --help`'s loom example matches the seed loom's bootstrap writes.
- `ff6e654ba` — `ly-drive` resumes a run whose baseline state is `blocked`, `paused` or `failed`.
- `655bcb6be` — `ly-drive` names how to wait for a backgrounded step.

**Extra weight:** `761dc64a5`, `ff6e654ba` and `655bcb6be` were verified by diff reading only — nobody has watched them work live.
For `ff6e654ba` and `655bcb6be`, judge whether the skill text is unambiguous enough that a fresh driver session does the right thing, and check it against what the llm driver in your own runs actually did (its transcript).
For `761dc64a5`, run the help's example verbatim on a fixture task worktree.
For `5550dd00d` and `f51cb430f`, look beyond the happy path they were written for: does the push behave on a parent without an upstream, and does whole-list reconcile drop anything a normal config upgrade relies on?

**Known-good operator sequence from round 1** (use it; report any step that no longer works):
1. `lyx fabric clone --into <scratch> <weft.git> <warp.git>`.
2. From `<hub>/warp`: `lyx config loom --set selfreport=false --set 'friction='` and `lyx config landing --set 'require_pr_to_base=[]'`.
3. From `<hub>/warp`: `lyx board upsert '<task json>'`, then `lyx fabric add <slug>`.
4. From `<hub>/<slug>`: `lyx shed seed self --recipe loom --driver go|llm --param parent=main` (optional for `go`), then `lyx loom start --no-attach`.

No deferred items.
