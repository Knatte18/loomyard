# `fabric` — independent review + fix (prompt template)

> Filled instance of `crucible/review-prompt-template.md` for the `fabric` module's crucible
> campaign, round 1. Gitignored under `.scratch/` — never checked in. See
> `crucible/README.md` for the loop this prompt runs inside.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `fabric`
module in the loomyard repo, followed by FIXING what you find.
Work in the worktree at `/home/knatte/Code/loomyard/wts/fabric-crucible-hardening` (branch
`fabric-crucible-hardening`).
Adjust that path/branch if the task lives elsewhere now.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of `fabric`'s scope and correctness.
   Hunt for bugs by reading the code AND by driving the real substrate (real `git` — worktrees,
   commits, junctions/symlinks, branches, remotes) — this is where the defects hide.
2. FIX: after you have a findings list, implement the fixes one at a time, verify each against
   the real substrate, keep the whole test suite green, and update the docs in the same change
   as the fix they document.
   COMMIT after each individual fix lands green (see "Commit per fix" below).
   Do NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test, plus the live
integration/CLI check if the finding needed one), and its doc update (if any) is included, COMMIT
it — on the current branch, no push — before starting the next finding.
Commit message format: `fabric: fix <finding-id> — <one-line what/why>` (e.g. `fabric: fix M1 —
propagate *GitError through the migrated reconcile.go call site instead of swallowing stderr`).
Do not commit `.scratch/` (gitignored; your review and fixer reports never belong in a commit
regardless).
This exists because a round agent's session can be killed mid-fix by something entirely outside
the method's control (a corrupted terminal, a lost connection) — see `crucible/README.md`'s "Why
commit per fix" section for the incident this defends against.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to
`.scratch/fabric-review-<yourtag>.md` on disk — before you touch (edit, create, or delete) a
single production or test file.
Do not fix findings as you go, even ones that look small and obviously right.
A review written or finished after code has already changed is no longer an independent judgment
— it is a post-hoc rationalization of edits you already made, and it silently destroys the one
property this whole method depends on.
If you catch yourself wanting to patch something the moment you spot it: don't. Write it down as
a finding, keep reading, finish the review, save the file, THEN start Job 2.

## Log as you go during Job 1 (BLOCKING — crash-resilience, do not batch it all to the end)
As you work through "What to TEST" below — each hermetic command, each live-integration run, each
live-driving scenario — APPEND your observations to `.scratch/fabric-review-<yourtag>.md`'s "What
was tested" section immediately after each command/scenario returns, rather than holding the
results in your own working context to write out in one pass once everything is done.
Do the same for findings as you form them: jot each one into the file's findings section
provisionally as you spot it (the executive summary and final severity ordering can wait until
you have the full picture, but individual findings and test observations should not).
This file lives under `.scratch/` (gitignored), so writing to it during Job 1 does not conflict
with the Sequencing rule above.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first.
Do NOT read any prior review or review-dialogue files before you have your own list.
Specifically do not open anything under `.scratch/` (gitignored; holds prior reviews
`fabric-review-*.md` and `*-fixer-report.md`, and this campaign's private pre-count file
`fabric-precount-r1.md` — do not open the pre-count file at all, this round or any later one; it
exists purely for the orchestrator's own verification and reading it would let you match a number
instead of deriving it).
Reading the design SPEC and the module docs is expected and required (those are not reviews).
AFTER you have written your own independent findings, you MAY consult prior rounds'
`.scratch/fabric-review-*` material (none exist yet this campaign — this is round 1) to (a)
confirm previously-fixed behaviors have not regressed and (b) re-evaluate deferred items.

## What to read
- Code:
  - `internal/fabricengine/**` (the domain kernel — this IS the module doc; read
    `internal/fabricengine/doc.go` FIRST, in full, before anything else — it is dense and
    authoritative about *why* the current shape exists, not just what it does)
  - `internal/fabriccli/**` (the CLI surface)
  - `internal/gitexec/**` (new: the checked/raw git-exec entry point fabric's revision routes
    through — small package, read it in full: `gitexec.go`)
  - `internal/gitrepo/**` (the go-git/gitexec split fabric's `Fabric.Pull`/`Commit`/etc. are
    built on — read at least `gitrepo.go`, `pull.go`, `push.go`, the files the revision touched)
  - `internal/weftname/**`, `internal/gitkit/**` (fabric's paired leaf/fixture dependencies)
  - `cmd/lyx/*guard_test.go` — specifically `checkedcall_test.go`, `gitrepoboundary_test.go`,
    `destructiveguard_test.go`, `cwdmutation_test.go` — these are the machine-enforced half of
    the invariants below; read what they actually assert, not just their names.
- Docs: `docs/overview.md`, `CONSTRAINTS.md` (in full — the invariants section below names which
  ones bind fabric specifically, but read the whole file once), `manifest/designs/fabric-unified-view.md`,
  `manifest/designs/fabric-windows-verification.md`, `README.md`, `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
  (for scenario ideas only — see "Live driving" below).
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md`.
  A change that ships behaviour without updating the module doc / invariants in the SAME change
  is incomplete. Fabric has no separate `manifest/designs/fabric.md` — its module doc IS
  `internal/fabricengine/doc.go`'s package comment; that is what you update for any behavior
  change, alongside `docs/overview.md`/`CONSTRAINTS.md` if invariants or the module table move.
- Design intent (SPEC, not a review): `internal/fabricengine/doc.go`'s package comment is the
  closest thing fabric has to a living spec — it records not just current behavior but the
  rationale chain (Shared Decisions, the destruction-chokepoint rationale, the correspondence
  index's write-path rationale) that decided it. Treat contradictions between the doc comment and
  the code as a finding in either direction (doc describes behavior the code no longer has, or
  code drifted from a documented invariant).

## Mission (assess on two axes, be adversarial)
1. Scope / omfang — is the module's scope right? Does the as-built code deliver what the design
   intended? Gaps, over-reach, silently-dropped requirements, deferred-that-should-ship-in-v1.
2. Correctness — bugs, races, error handling, edge cases; concentrate on the historically-fragile
   areas below. Also assess docs accuracy (do the docs match the code?) and operability.

## High-yield focus — where `fabric`'s real bugs live (drive these, do not just read them)
The pure/unit-tested parts are usually solid; defects concentrate in the COMPOSED, LIVE behavior
the hermetic tests never exercise. Treat each as an INVARIANT you must actively verify by driving
the real substrate — a green `go test` proves nothing here.

**Why this round exists — read this before anything else.** Fabric already went through a full
crucible campaign (6 rounds, 81 findings, 9 BLOCKING — see `crucible/README.md`'s "the fabric
campaign" worked example) that converged, plus four follow-up slices (12-15) that closed out its
remaining residuals — all of that is CLOSED-AND-VERIFIED background, not open work. Since then,
three commits landed that **nobody has crucible-reviewed yet**, the most consequential of which
touched nearly the entire `fabricengine` surface:
- `74e6a6bb` "gitexec: add the checked entry point and migrate the call sites" — introduced
  `internal/gitexec` (a checked-vs-raw git-exec split) and migrated ~19 `fabricengine` files
  (including `destroy.go`, the destruction chokepoint) plus `internal/gitrepo` onto it.
- `f4ce0188` "lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency" — inverted
  which package builds fixture hubs for which.
- `16c0cfcc` "Unblock t.Parallel on hub-fixture tests that currently t.Chdir" — migrated
  integration tests off process-wide `t.Chdir`/`os.Chdir` onto a `RunCLIIn`/`WithCwd` seam so
  hub-fixture tests can run under `t.Parallel`.

**This is NOT a safety pass.** Treat it as a full independent review of freshly landed,
never-independently-reviewed code — the prior campaign's convergence tells you the *pre-migration*
code was clean, not this code. The specific invariants below are new or changed by that
migration and deserve the most adversarial attention:

- **gitexec Checked-Call Invariant correctness** — `gitexec.Run` returns `*GitError` on a
  non-zero git exit (recoverable via `errors.As`), while the exec-level failure (git could not
  run at all) comes back as a raw, unwrapped error. Walk every migrated call site (`destroy.go`,
  `pull.go`, `reconcile.go`, `warpprobe.go`, `checkout.go`, `clone.go`, `add.go`, `weftwiring.go`,
  `index.go`, `prune.go`, `remove.go`, `status.go`, `weftgit.go`, `boardweft.go`, `cleanup.go`,
  `dirtiness.go`, `gitexclude.go`, `hook.go`, `worktreelist.go`) and check: did any site that used
  to string-match stderr, or branch on a specific exit code, silently break because the error
  shape changed? Does any site that should now use `errors.As(err, &gitErr)` to recover the exit
  code/stderr still work correctly? — repro: force a git failure at each of the higher-risk sites
  (e.g. `reconcile` against a repo with a broken ref, `pull` against an unreadable remote) and
  confirm the resulting error message/behavior is still correct, not just "an error came back".
- **The two pinned raw sites** — `weftwiring.go`'s `weftRepoExists`/`weftBranchExists` (marked
  `//gitexec:raw`, justified as bool-returning predicates with no error channel). Confirm they
  still correctly distinguish "git ran and said no" (a real false) from "git could not run at
  all" (which a bool-only predicate has no way to surface — is that silently swallowed as false,
  and is that actually safe at both call sites?) — repro: point one at a path with no git binary
  reachable, or a genuinely corrupt `.git` dir, and see what the predicate reports.
- **The destruction chokepoint under the new gitexec split** — `destroy.go`'s `removeGitWorktree`
  and `deleteBranch` now go through `gitexec.Run` and return `err` unchanged (never a `*GitError`
  is wrapped further) — every call site is documented as recovering exit code/stderr via
  `errors.As`. Confirm that's actually true at every one of those call sites, live: force a
  `git worktree remove` refusal (dirty untracked file it doesn't expect) and a `git branch -D`
  refusal (checked-out branch) and confirm the caller-visible error is still informative, not a
  bare "exit 1" with the useful stderr dropped on the floor.
- **`t.Parallel` safety on hub-fixture tests** — `16c0cfcc` unblocked `t.Parallel` on tests that
  used to serialize on a process-wide `t.Chdir`. Confirm this is actually safe, not just
  compiling: run the migrated fabric integration test files concurrently (`-parallel` high, high
  `-count`) and watch for cross-test interference — shared temp dirs, shared global mutable
  state, or a lingering assumption that cwd is process-global rather than per-call. The one
  documented allowlist exemption is `internal/fabricengine/coalesce_integration_test.go` (its cwd
  mutation IS the assertion under test, not a migration leftover) — confirm that's still the
  ONLY fabric file still doing a raw `t.Chdir`/`os.Chdir`, and that it's actually exempt for the
  stated reason rather than merely unmigrated.
- **The `lyxtest`/fabric fixture-dependency inversion** (`f4ce0188`) — confirm fabric's own
  fixtures (built via `hubforge`/`fabriccli.CloneAndWire` per the hubforge Fabric-Fixture
  Invariant) still produce correct hub geometry after the inversion — junctions wired, `.lyx`
  present, anchor/warp-binding records correct — not just "the test suite is green" but that the
  fixtures a real operator would get from `lyx fabric clone` are unchanged in shape.
- **The already-known, already-accepted residual** — `fabricengine/doc.go`'s "The correspondence
  index's write path" section documents a real, accepted two-phase race between `record()` and
  `RebuildIndex`'s scan-to-write span, graded LOW/self-healing because the weft commit trailers
  remain the sole source of truth. Re-verify this characterization still holds post-migration (did
  the gitexec split change any timing/locking around that window?) — but do NOT re-litigate it as
  a new finding unless something material actually changed; if it's unchanged, say so explicitly
  in your findings as a confirmed-still-accepted item, not silence.
- **Re-verify (do not assume unchanged) the destruction chokepoint's core properties**, since
  `destroy.go` itself was touched: containment before ownership before dirtiness before force,
  in that fixed order, still stops at the first failure; `--force` still answers dirtiness only,
  never containment or ownership; every one of the 28-ish destructive call sites the chokepoint
  consolidates still routes through it (no new raw `os.RemoveAll`/`git worktree remove`/`git
  branch -D`/`fslink.Remove`/`ResetHard` call snuck in elsewhere in the package during the
  migration) — repro at least one BLOCKING-severity scenario per check: a path-traversal
  containment attempt, an unowned-path removal attempt, and a dirty-worktree removal without
  `--force`, each confirmed still refused.
- **gitrepo Client Boundary Invariant pinned counts** — CONSTRAINTS.md states `internal/gitrepo`
  has exactly 3 pinned raw call sites and `internal/fabricengine` exactly 2. Count them yourself
  from the code (not from CONSTRAINTS.md's prose) and confirm the guard tests
  (`cmd/lyx/gitrepoboundary_test.go`, `cmd/lyx/checkedcall_test.go`) actually enforce those exact
  numbers rather than merely existing.

## Explicitly OUT of scope for `fabric` this round
- **Windows path behavior.** The prior campaign named this a permanent, never-executed gap —
  unreachable from a Linux host, reasoned about rather than driven, every round. Still true here;
  do not attempt to fabricate Windows coverage you cannot actually run. State it as a limit in
  your merge-readiness verdict, same as the prior campaign did.
- **The GitHub-remote-backed dedicated sandbox hub** (`lyx-fabric-test`/`lyx-fabric-test-weft`,
  `tools/sandbox/SANDBOX-FABRIC-SUITE.md`'s launcher). You are NOT expected to have GitHub network
  access or `gh` auth for this round — every fabric CLI verb accepts a local filesystem path as a
  git remote/URL (git itself supports this), so drive `lyx fabric clone`/`add`/etc. against
  throwaway local bare-or-plain git repos you create with plain `git init` in a scratch temp dir.
  That is a full, faithful exercise of fabric's own logic — the GitHub-hosted hub in the suite
  file exists for a *human operator's* dogfooding convenience, not because fabric's logic differs
  against a real remote. Read `SANDBOX-FABRIC-SUITE.md`'s scenarios (F0-F13) for IDEAS only, and
  run every one of them yourself against your own local hub, per the "Live driving" rule below —
  never invoke its launcher.
- **The `Snapshot:`/tag mechanism's accumulation cost** (unbounded weft history growth under a
  hypothetical polling consumer) — `doc.go` already documents this as an accepted design
  tradeoff for the actual call pattern fabric serves (one tagged commit per regeneration, not a
  poll). Don't re-flag it as a defect; it is fair game only if you find a REAL polling consumer
  that doesn't exist today.

## Round context seeded from prior-round verification
**This is round 1 of a new campaign segment** — not a residual-closing round and not a safety
pass. The prior fabric crucible campaign (6 rounds + 4 follow-up slices) is CLOSED-AND-VERIFIED
background; nothing in it is open. What changed since (`74e6a6bb`, `f4ce0188`, `16c0cfcc`) has
never been independently reviewed at all. Do a genuinely full, adversarial review of the current
state, weighted toward the "High-yield focus" list above (the gitexec migration's blast radius),
but do not skip the rest of the module on the assumption that only the migration matters — an
unrelated defect is just as real a finding.

State the **merge bar** so you calibrate: correctness in the NORMAL single-instance flow is the
gate; an N×-concurrent suite (if you choose to run one) is a diagnostic amplifier, not a merge
blocker — a timeout under an artificial N-suite CPU peg is not itself a defect, though a
corruption marker under concurrency always is.

## Live-substrate cost declaration
`LLM-DRIVING: no.` Fabric has zero `//go:build smoke` tests (smoke requires a real logged-in
`claude` session — fabric never spawns one). Its live substrate tests are all `//go:build
integration`-tagged: real `git` subprocesses (via `internal/gitexec`), real worktrees, real
filesystem junctions/symlinks (`internal/fslink`) — never a real LLM/provider process. The
generic "N× CONCURRENT full smoke suites" gate and its LLM-driving RAM-exhaustion warning in
`crucible/README.md`/`orchestrator-prompt.md` do NOT apply here in the dangerous sense — you MAY
run concurrent copies of fabric's integration suite freely; the cost is real git subprocesses and
temp directories, not real LLM sessions, and fabric's own locking code (`mutateGitExclude`'s
repo-wide flock, the correspondence index's `state.UpdateJSON`, the push coalescing lock) is
exactly the kind of code concurrency is likely to expose real races in — running N concurrent
copies here is high-value, not a hazard, unlike a burler/perch/loom smoke suite. No EXECUTION BAN
list is needed for this module.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/...`
- `go test ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/... ./cmd/lyx/... -count=5`
  — stress timing/concurrency-sensitive tests specifically with `-count=5`; the whole tree is
  cheap enough to run at that count throughout.

Live integration (real substrate, behind the `integration` build tag — fabric has no `smoke` tag;
do not look for one):
- `go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitexec/... ./internal/gitrepo/... -v -count=1`
  — scan output for FAIL and for any substrate-corruption marker (a leaked temp dir, a dangling
  lock file, an orphaned `.git` directory, "being used by another process").
- `cmd/lyx/hermeticenv_test.go`'s `TestHermeticGitEnv_GitSpawningPackagesHaveTestMain` and
  `cmd/lyx/tierpurity_test.go`'s `TestTierPurity_UntaggedTestsSpawnNothing` must both stay green —
  these are the tier-purity guards; a finding that would only pass by weakening either test is not
  a legitimate fix.
- THE decisive amplifier — N× CONCURRENT full integration suites (compile once, run N copies; safe
  here per the cost declaration above — real git subprocesses, not real LLM sessions):
  ```sh
  go test -c -tags integration -o "$SCRATCH/fabric.integration.test.exe" ./internal/fabricengine/...
  for i in 1 2 3 4; do ( "$SCRATCH/fabric.integration.test.exe" -test.count=1 -test.v \
      -test.parallel=8 > "$SCRATCH/int_$i.txt" 2>&1; echo "run$i rc=$?" ) & done; wait
  grep -hiE 'FAIL|being used by another process|permission denied|dangling|panic:' "$SCRATCH"/int_*.txt \
      || echo "no markers"
  ```
  Run this against `internal/fabricengine` at minimum; repeat for `internal/fabriccli` and
  `internal/gitexec`/`internal/gitrepo` if time allows. Report the exact commands and raw grep
  output, not a summary claim.

Live driving — YOU drive it directly, no launcher (PRIMARY — where the bugs surface):
- Deploy the current source as the dev binary under test: `./deploy-dev` (POSIX) — re-run after
  EVERY source change or you validate a stale binary. Deploy first, always.
- Build your own throwaway local warp+weft pair with plain `git init` in a scratch temp dir (see
  "Explicitly OUT of scope" above — no GitHub network access needed). Then drive every one of
  fabric's 16 verbs (`clone`, `add`, `list`, `remove`, `checkout`, `pairs`, `reconcile`, `prune`,
  `cleanup`, `status`, `commit`, `push`, `pull`, `sync`, `diff`, `unwire`) directly, foreground,
  waiting for each to return — read `SANDBOX-FABRIC-SUITE.md`'s F0-F13 scenarios for ideas
  (never its launcher).
- The suite/list is a FLOOR — devise and run MANY more adversarial scenarios of your own beyond
  it, weighted toward the High-yield focus list above: force every migrated git call site to fail
  and inspect the resulting error; run destructive verbs against uncontained/unowned/dirty
  targets and confirm refusal; run concurrent verb invocations against the same hub and watch the
  locking code.
- **"Headless" means "no human required" — NOT "no time/token cost to me."** A real git
  subprocess doing real work takes real wall-clock seconds to low minutes across many scenarios,
  not zero. That cost is EXPECTED and BUDGETED FOR. You are explicitly forbidden from writing
  "operator-assisted", "cost-bearing", "long-running", "impractical", or "automated context" as a
  reason to skip live driving.
- The only legitimate "cannot verify" cases: (a) a scenario that structurally requires a human's
  physical eyes, or (b) a genuine environment gap (missing binary, no `git` on PATH — check this
  FIRST). Neither applies to a local-filesystem-remote fabric hub. Flag any real instance of (a)
  or (b) explicitly rather than skipping silently.

TEARDOWN DISCIPLINE (critical): tear down every scratch hub/temp dir/lock you create. At the end,
confirm ZERO stray git processes and ZERO leftover `.gitrepo-push.lock`/`fabric.push.lock`/
`weft.write.lock` files outside a torn-down temp dir (`ps aux | grep git` or platform equivalent,
plus a scan of your scratch root for leftover lock files). Leave no stray state. Be honest about
what you could NOT verify and why.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong
behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED
(reproduced/traced) vs PLAUSIBLE (looks wrong, unverified).
For scope: plan-promised vs shipped; flag deferred-that-should-be-v1 and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.**
ALL findings you record get fixed in Job 2 — including every NIT — not just BLOCKING/MEDIUM ones.
A finding you write down but leave unfixed as "low priority" is not actually a reported finding;
it is a dropped one. The only legitimate reason to leave a finding unfixed is that fixing it
genuinely requires something you cannot do alone this round — an operator decision on a real
design tradeoff, or a live capability you don't have. Even then say so explicitly, with the
specific reason, in the fixer report's deferred section.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
None — this is round 1 of this campaign segment.

## Fixing — after the review
- Fix EVERY finding from your review, all severities including NIT — not just BLOCKING/MEDIUM
  ones.
- Load the code-quality guidance (`/code-quality` skill) AND the Go-specific skills
  (`golang:golang-build`, `golang:golang-testing`, `golang:golang-comments`) before editing — ALL
  of the relevant skills, not code-quality alone. Prefer surgical edits; match existing style and
  the file-level doc-comment convention (fabric's doc comments run long and rationale-heavy —
  match that register, don't compress it into terse bullet points that lose the "why").
- For every bug you fix, add or extend a test that would have caught it. For a live-only defect,
  add a `//go:build integration` test that walks the failing scenario against real git (the
  existing integration test files show the pattern).
- MAKE INTEGRATION TESTS DETERMINISTIC. Git/filesystem operations can race under load; a test that
  assumes ordering passes on a quiet machine and FLAKES on a loaded one. Poll on actual state
  (lock released, file written, ref updated) with a deadline, never sleep a fixed amount. Prove
  determinism by running the new test many times, including under the N-concurrent-copies
  pattern above, not once.
- If a review finding surfaces a live/visual behavior `SANDBOX-FABRIC-SUITE.md` doesn't cover,
  extend it (match the existing scenario shape, keep `sandbox_coverage_test.go` green). If the
  finding doesn't warrant a new suite scenario, note it in your fixer report instead.
- Keep `go build`/`vet`/`test` green after every change. Then RE-DEPLOY (`./deploy-dev`) and
  re-run every live scenario yourself, directly — re-deploying FIRST is mandatory.
- Update `internal/fabricengine/doc.go`'s package comment (fabric's module doc) and
  `docs/overview.md`/`CONSTRAINTS.md` if invariants or the module table move, IN THE SAME change
  as the fix. Do NOT add bugfix/hardening notes to `manifest/roadmap.md`.
- Tear down all substrate state; confirm zero stray processes/locks. COMMIT each fix as you
  finish it (see "Commit per fix" above) — do NOT push unless the user explicitly asks. Report
  the changed files and how you verified each fix.

## Deliverables
1. A structured review report (Executive summary with top risks + merge-readiness opinion;
   Scope assessment plan-vs-shipped; Code findings severity-ranked with file:line + scenario +
   fix + CONFIRMED/PLAUSIBLE; Docs & operability findings; What-was-tested with exact commands +
   observed results, including what you could NOT verify and why).
   Write it to `.scratch/fabric-review-<yourtag>.md` — per "Log as you go" above, build the
   What-was-tested section and provisional findings incrementally throughout Job 1, not in one
   pass at the end; only the executive summary and final severity ordering are written last.
2. A fixer report: what you implemented, what you deliberately deferred (with reasons), the exact
   test commands run + results, and the changed files.
   Write it to `.scratch/fabric-review-<yourtag>-fixer-report.md`.
3. In your final chat message: a concise summary (executive summary + counts by severity + the
   two report paths + an explicit merge-readiness verdict). Do not paste the whole reports.

Begin with the clean-room review (read `internal/fabricengine/doc.go` + code + docs, then drive
the real substrate), produce your independent findings, then implement and verify the fixes.
