# `gitrepo` — independent review + fix

> Instantiated from [`review-prompt-template.md`](review-prompt-template.md) for the `internal/gitrepo`
> module. See [README.md](README.md) for the loop this prompt runs inside.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the `gitrepo`
module (`internal/gitrepo`) in the loomyard repo, followed by FIXING what you find. Work in the
worktree at `/home/knatte/Code/loomyard/wts/gitrepo` (branch `gitrepo`). Adjust that path/branch
if the task lives elsewhere now.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of `gitrepo`'s scope and correctness. Hunt for bugs
   by reading the code AND by driving the real substrate — real `git` subprocesses against real
   repositories (temp checkouts, bare remotes, real cross-process file locks via `gofrs/flock`).
   This is a library, not a CLI module: there is no `lyx gitrepo <verb>` command and no
   `deploy.cmd` step. "Driving the real substrate" here means writing and running your own Go test
   programs / ad hoc `go run` harnesses that call the `gitrepo.Repo` API directly against real git
   state you construct — the existing `integration`-tagged tests already do exactly this pattern
   (see `newRepo`/`newBareRemote`/`newRepoWithRemote`/`cloneFromBare` helpers) and are your
   starting point, not your ceiling.
2. FIX: after you have a findings list, implement the fixes one at a time, verify each against the
   real substrate, keep the whole test suite green, and update the docs in the same change as the
   fix they document. COMMIT after each individual fix lands green (see "Commit per fix" below). Do
   NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic + integration test),
and its doc update (if any) is included, COMMIT it — on the current branch, no push — before
starting the next finding. Commit message format: `gitrepo: fix <finding-id> — <one-line
what/why>` (e.g. `gitrepo: fix M1 — reject SHA-like arguments starting with '-' before passing to
git`). Do not commit `.scratch/` (gitignored; your review and fixer reports never belong in a
commit regardless). This exists because a round agent's session can be killed mid-fix by something
entirely outside the method's control (a corrupted terminal, a lost connection). A single
monolithic uncommitted diff left behind by a crash forces the orchestrator to reverse-engineer,
finding by finding, which fixes are actually complete versus half-done, from the diff alone. A
trail of small commits turns that same crash into something the orchestrator can just read: `git
log` shows exactly which findings landed clean, and anything with no commit is unambiguously not
done yet — no guesswork.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to
`.scratch/gitrepo-review-<yourtag>.md` on disk — before you touch (edit, create, or delete) a
single production or test file. Do not fix findings as you go, even ones that look small and
obviously right. A review written or finished after code has already changed is no longer an
independent judgment — it is a post-hoc rationalization of edits you already made. If you catch
yourself wanting to patch something the moment you spot it: don't. Write it down as a finding,
keep reading, finish the review, save the file, THEN start Job 2.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first. Do NOT read any prior review or review-dialogue files before you
have your own list. Specifically do not open anything under `.scratch/` (gitignored; holds prior
reviews `gitrepo-review-*.md` and `*-fixer-report.md`). Reading the module doc and the module docs
is expected and required (those are not reviews). AFTER you have written your own independent
findings, you MAY consult the prior rounds' `.scratch/gitrepo-review-*` material — regardless of
which model produced it (rounds rotate across Fable / Opus; the most recent prior round is
whichever `gitrepo-review-*` file is newest), EXCEPT your own `-<yourtag>` deliverables — to (a)
confirm previously-fixed behaviors have not regressed and (b) re-evaluate the deferred items at the
bottom.

## What to read
- Code: `internal/gitrepo/**` (all of `gitrepo.go`, `push.go`, `snapshot.go`, `doc.go`, and every
  `*_test.go`), `internal/lock/**` (the `gofrs/flock`-backed file lock `PushCoalesced` builds on —
  read it, do not just trust it), `internal/gitexec/**` (the single exec choke-point `gitrepo.run`
  delegates to — read-only awareness, it is a leaf dependency shared by ~80 call-sites, out of
  scope to modify unless a `gitrepo`-caused defect genuinely traces into it).
- Docs: `internal/gitrepo/doc.go`'s package doc IS the module doc — the standalone
  `manifest/designs/gitrepo.md` was folded into it and deleted per the Documentation Lifecycle
  (see `docs/overview.md#documentation-lifecycle`); there is no separate design file to read.
  Also read `docs/overview.md`, `manifest/roadmap.md`, `CONSTRAINTS.md` (in particular: Test Tier
  Purity Invariant, Hermetic Git Test Environment Invariant, Documentation Lifecycle — `gitrepo`
  has no CLI surface so the CLI/Cobra Invariant and Sandbox Suite Coverage do not apply to it), and
  `manifest/designs/fabric.md` + `manifest/designs/board-use-gitrepo.md` (the two documented future
  consumers — useful for understanding what API shape they expect `gitrepo` to already support).
- No `tools/sandbox/SANDBOX-*-SUITE.md` scenario covers `gitrepo` (it is a pure library with no
  `lyx` verb) — skip this source, there is nothing to read there.
- Repo rules you MUST follow: `CLAUDE.md` (root + `~/.claude/CLAUDE.md`) and `CONSTRAINTS.md`. A
  change that ships behaviour without updating the module doc / invariants in the SAME change is
  incomplete.
- Design intent (SPEC, not a review): `internal/gitrepo/doc.go`'s package doc comment is the
  authoritative source of intended v1 scope/behavior (it explicitly states the Scope boundaries —
  see "Explicitly OUT of scope" below). If you want the original design rationale in more detail,
  `git log --all --oneline -- manifest/designs/gitrepo.md` finds the pre-fold-in commit.

## Mission (assess on two axes, be adversarial)
1. Scope / omfang — is the module's scope right? Does the as-built code deliver what `doc.go`
   promises? Gaps, over-reach, silently-dropped requirements, deferred-that-should-ship-in-v1.
2. Correctness — bugs, races, error handling, edge cases; concentrate on the historically-fragile
   areas below. Also assess docs accuracy (does `doc.go`'s package doc match the code?) and
   operability.

## High-yield focus — where `gitrepo`'s real bugs live (drive these, do not just read them)
The pure/unit-tested parts are usually solid; defects concentrate in the COMPOSED, LIVE behavior
against real git processes and a real cross-process file lock that the existing tests only
partially exercise. Treat each as an INVARIANT you must actively verify by driving the real
substrate — a green `go test` proves nothing here on its own.

- **Cross-process lock serialization, with REAL separate OS processes, not just goroutines.** The
  existing `TestPushCoalesced_LockBlocking_Serializes` test proves serialization across two
  goroutines in one process, each acquiring its own `flock.Flock` handle — a reasonable proxy, but
  not proof that the lock actually holds across genuinely separate OS processes (the scenario
  `PushCoalesced` exists for: multiple `lyx board sync` invocations racing against the same
  worktree). Build a small `go run` harness (or a helper `TestMain`-spawned subprocess) that
  launches several real child processes, each calling `PushCoalesced` against the *same* clone
  concurrently, and confirm the bare remote ends up with every commit and no corruption — same
  spirit as the mux campaign's "compile once, run N copies" concurrent smoke gate.
- **Crash/rebirth under the lock.** Kill (SIGKILL, not a graceful stop) a process while it holds
  `.gitrepo-push.lock` mid-push (e.g. after acquiring the lock but before/during the `git push`
  subprocess). Confirm a subsequent `PushCoalesced` call from a fresh process is not wedged
  (`gofrs/flock` locks are supposed to release on process death — verify this holds in practice,
  not just per the comment in `internal/lock/lock_test.go`) and correctly detects/finishes whatever
  the killed process left behind (partially pushed vs. not-pushed-at-all).
- **Rebase-retry's single-attempt boundary.** `pushWithRebaseRetry` retries exactly once. Drive a
  scenario where the remote advances a SECOND time during the rebase-then-retry window (e.g. a
  third clone pushes between your rebase and your retry push) so the retry push is itself
  rejected — confirm the resulting error is honest and non-corrupting (no partial rebase left
  in-progress, no silently-lost local commits), not just that it "works when nobody else moves".
  Also drive an actual rebase CONFLICT (not just the documented dirty-tracked-file precondition
  failure) during the retry — does `rebase --abort`'s own (currently unchecked, see `push.go:77`)
  return value matter, and does the repo end up in a clean, non-rebasing state either way?
- **Argument/flag injection via SHA-like strings.** `SHAExists`, `ChangedFilesSince`, and
  `SetSnapshotSHA` all interpolate a caller-supplied string directly into git subcommand arguments
  (`sha+"^{commit}"`, `sha+"..HEAD"`, `update-ref <ref> <sha>`). None of these go through a shell,
  but git itself treats a leading `-` as an option. Drive `ChangedFilesSince("--upload-pack=foo")`-
  or `SHAExists("--help")`-shaped inputs and confirm the actual observed git behavior — is this a
  real, reachable defect (a consumer passing an untrusted/derived string) or provably inert given
  today's callers? Say which, with the actual command output as evidence, not a guess.
- **`SetSnapshotSHA`'s adopt-on-conflict race, with real concurrent writers.** Two (or three) real
  processes/goroutines calling `SetSnapshotSHA` for the SAME key with DIFFERENT SHAs at
  (approximately) the same time, against a shared bare remote. Confirm: (a) exactly one SHA wins
  and every other caller's local ref converges to the same winning value with no error, (b) no
  caller is left believing its OWN `sha` argument won when it actually didn't — re-read
  `SnapshotSHA(key)` immediately after each caller's `SetSnapshotSHA` returns nil and confirm it
  matches what that caller actually expects, not just that no error was returned. Also test the
  every-existing-test-only-does-two-clones-serially gap: does the fetch-then-check-then-push
  window admit a THIRD write landing in between adopt and the caller's next read?
- **`remoteName()`'s "origin" fallback under a non-"origin"-named or multi-remote repo.** The
  fallback is a documented assumption (single conventional "origin" remote), but confirm what
  actually happens when it's violated live: a repo with its only remote named something else, or a
  detached HEAD with no `branch.<name>.remote` configured. Does `SnapshotSHA`/`SetSnapshotSHA`
  silently no-op against a nonexistent "origin" (misleadingly reporting "no snapshot found" or
  successfully "pushing" nowhere) rather than surfacing a clear error? Decide whether that silence
  is acceptable given the stated assumption, or a real correctness gap worth a fix/doc update.
- **`StageAndCommit` with an empty/nil `files` argument.** Confirmed to return `("", false, nil)`
  by doc comment — verify live that `git add --` with zero paths actually behaves this way across
  the git version installed here (some git versions emit a warning/behave subtly differently for a
  bare `add --` with nothing following), and that it never falls through to a real commit.

## Explicitly OUT of scope for `gitrepo` v1
Per `doc.go`'s own "Scope boundaries" section: rebase (beyond the one automatic retry
`pushWithRebaseRetry` performs internally), interactive staging, cherry-pick, and general conflict
resolution are deliberately NOT supported — do not flag their absence as a gap. Repo creation,
cloning, and worktree topology (`fabric`'s job, per `manifest/designs/fabric.md`) are out of scope.
`gitrepo` is also explicitly not goroutine-safe for concurrent writes to the SAME checkout from
CALLER-side (non-`PushCoalesced`) methods — flag it only if a realistic single-caller-discipline
scenario silently corrupts data rather than failing visibly; do not flag the documented
not-goroutine-safe contract itself as a bug.

## Round context seeded from prior-round verification
**Safety pass.** Rounds 1 (`fable-r1`, 9 findings), 2 (`opus-r2`, 1 finding), and 3 (`fable-r3`, 5
findings) are all CLOSED-AND-VERIFIED by the orchestrator — build/vet/hermetic (whole-repo
`go test ./...`, all CONSTRAINTS guards)/`-tags integration -count=5`/`-race -count=2` green after
every round; `golangci-lint` clean (remaining notes match pre-existing repo patterns); every
MEDIUM-or-above finding's regression test was independently falsified by the orchestrator
(production fix reverted, confirmed the test fails at the intended assertion — including a
`-count=20` flake-rate check for round 2's probabilistic bug — then cleanly restored to an empty
diff), including round 3's MEDIUM finding (F-R3-1, `StageAndCommit`) which was falsified in two
separate mutation steps (the empty-list guard alone, and then the commit-scoping alone) to confirm
each half of the fix is independently load-bearing.

**Rounds 2 and 3 were BOTH seeded as safety passes and BOTH found real defects** (round 2: 1 LOW;
round 3: 1 MEDIUM + 3 LOW + 1 NIT). Do not assume convergence just because a round is labeled
"safety pass" — that label describes what was seeded, not a prediction of the outcome. Round 3's
own reviewer flagged that its MEDIUM fix (`StageAndCommit`'s pathspec-scoping) touches the module's
single most-consumed primitive and explicitly asked for fresh-eyes re-verification in a further
round — this round IS that fresh look, on top of also being a genuinely independent full pass, not
a narrow recheck of just that one fix.

**CLOSED-AND-VERIFIED — do NOT re-litigate these** (commits `e06daf6a`..`ee462c04` on branch
`gitrepo`):
- F1 — SHA-argument injection (`validSHA` hex-only gate on `SHAExists`/`ChangedFilesSince`/`SetSnapshotSHA`)
- F2 — `ChangedFilesSince` non-ASCII path mangling (`-z` + NUL split)
- F3 — `SetSnapshotSHA` adopt-on-conflict dropping a strictly-newer value under a 2-way creation race (superseded by R1 below, which generalizes this to N-way)
- F4 — `validSnapshotKey` admitting keys git itself refuses (trailing `.`, `.lock` suffix)
- F5 — `ChangedFilesSince` dropping a rename's old path (`--no-renames`)
- F6 — non-`origin`-remote silent read-path degradation (documented, no behavior change)
- F7 — `rebase --abort`'s result previously discarded (now checked, honest mid-rebase error)
- F8 — `StageAndCommit` file entries being pathspecs, not literal paths (doc-only)
- F9 — missing real-process lock-serialization and crash-recovery test coverage (added)
- R1 — `SetSnapshotSHA`'s F3 fix only tolerated 2-way transient contention; fixed with a bounded
  retry loop (`snapshotPushMaxAttempts = 8`) that keeps retrying while the caller remains strictly
  ahead of the value it just adopted. Independently falsified by the orchestrator (cap reverted to
  2, new 3-writer test failed repeatedly under `-count=20`, restored clean).
- F-R3-1 — MEDIUM — `StageAndCommit` committed pre-staged, unlisted index entries, and an *empty*
  files list could produce a real commit (contradicting the documented `("", false, nil)`
  contract). Fixed with an empty-list early return (no git spawn) plus pathspec-scoping both the
  staged-change check and the commit itself to the listed files. Independently falsified by the
  orchestrator in two steps: (1) removing only the empty-list guard reproduced the empty-list
  false-commit; (2) additionally unscoping the commit's own pathspec reproduced the
  pre-staged-unlisted-file-swept-into-commit bug too — both failed at the intended assertions,
  both restored clean.
- F-R3-2 — LOW — `SnapshotSHA` folded hard git failures (not-a-repo, exit 128) into the "no
  snapshot" `("", nil)` state; now only a verified-absent ref (exit 1) reads as absent. Independently
  falsified by the orchestrator (folded every non-zero exit back to absent, confirmed
  `TestSnapshotSHA_NotARepo_SurfacesError` fails at the intended assertion, restored clean).
- F-R3-3 — LOW (doc) — `Push`'s rebase-retry can rewrite local SHAs out from under a caller
  (`SHAExists` can't catch it — the old object survives via reflog); documented on `Push`/
  `PushCoalesced`/the package doc. No behavior change.
- F-R3-4 — LOW (doc) — stderr-text outcome classification assumes untranslated git messages;
  documented as an assumption. The real fix (pinning `LC_ALL=C` in `gitexec`) is DELIBERATELY
  DEFERRED as an operator decision (repo-wide blast radius across ~80 `gitexec.RunGit` call-sites,
  beyond this module's scope, risk unreachable on this machine and always degrades loudly never
  silently) — do not re-open this as an in-scope fix; it is a conscious, recorded deferral, not an
  oversight.
- F-R3-5 — NIT — `isStrictDescendant` compared SHA spellings, not resolved commits, for an
  abbreviated `sha`; `SetSnapshotSHA` now canonicalizes to the full spelling at entry.

This is round 4 (the last round in this loop's budget) — do a genuinely independent clean-room
pass to find anything rounds 1-3 missed, OR honestly confirm merge-readiness ("no new defects, ship
it" is the expected, valuable outcome of a safety pass — do not invent work to justify the round).
Given the pattern so far, both `StageAndCommit`/`ChangedFilesSince`'s git-argument construction and
the snapshot-ref concurrency/adopt-on-conflict machinery have each yielded real defects across
different rounds — those deserve a particularly skeptical look, but review the whole module
clean-room, don't confine yourself to them. The one deliberately-deferred item (F-R3-4's `LC_ALL=C`
pinning) is an operator decision, not something to re-litigate or fix yourself. Nothing else is
currently deferred.

State the **merge bar** so you calibrate: correctness in the NORMAL single-instance,
single-or-few-concurrent-caller flow is the gate; an artificial many-way concurrency stress beyond
what a real `board sync`-style caller would ever produce is a diagnostic amplifier, not a merge
blocker, per the same principle the mux campaign used (see README.md's "Reading the result").

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout — untagged, no git spawn, per the Test Tier Purity Invariant):
- `go build ./...`
- `go vet ./internal/gitrepo/... ./internal/lock/...`
- `go test ./internal/gitrepo/... ./internal/lock/...` (this alone runs only the untagged
  `keyvalidation_test.go` file for `gitrepo` — the rest require the tag below)

Integration (real substrate — real git subprocesses, real temp repos and bare remotes, per the
Hermetic Git Test Environment Invariant's `TestMain`):
- `go test -tags integration -count=5 ./internal/gitrepo/...` — stress timing/concurrency.
- `go test -tags integration ./internal/lock/...`

Live driving — YOU drive it directly, no launcher (PRIMARY — where the bugs surface):
- There is no deploy step and no `lyx` CLI verb for this module — you call the `gitrepo.Repo` Go
  API directly. Build small, throwaway Go harnesses (either as new `t.Run` scenarios inside a
  `_test.go` file under the `integration` tag, or standalone `go run` scratch files under
  `.scratch/` if a scenario needs real separate OS processes rather than in-process goroutines) and
  run them yourself, foreground, waiting for each to return.
- Walk the "High-yield focus" list above. Devise MANY more adversarial scenarios beyond it — this
  list is a FLOOR, not a ceiling (combine operations in orders nothing has tried; chase anything
  the code makes you suspicious of).
- **"Headless" means "no human required" — NOT "no time/token cost to me."** A real multi-process
  concurrency scenario takes real wall-clock time, not seconds — that cost is EXPECTED and
  BUDGETED FOR, never a reason to skip a scenario. You are explicitly forbidden from writing
  "operator-assisted", "cost-bearing", "long-running", "impractical", or "automated context" as a
  reason to skip live driving.
- **Before writing "could not verify", ask yourself literally: "would a human's physical eyes be
  required here, or am I just trying to avoid spending my own time/turns?"** Only the first is a
  real reason for `gitrepo` — there is no visual/TTY-only surface in this module at all, so a
  legitimate "cannot verify headlessly" here should be rare; if you reach for it, say exactly what
  concrete environment gap (missing binary, no network) blocked you.

TEARDOWN DISCIPLINE (critical): if you spawn any subprocess or background process for a
multi-process scenario, confirm it is not left running afterward (`ps`/`tasklist` grep for any
harness binary you launched) and that any temp directories you created outside `t.TempDir()` (i.e.
anything under `.scratch/`) are cleaned up or clearly left as intentional artifacts. Leave no stray
state. Be honest about what you could NOT verify and why.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong
behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED
(reproduced/traced) vs PLAUSIBLE (looks wrong, unverified). For scope: plan-promised vs shipped;
flag deferred-that-should-be-v1 and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings you record get
fixed in Job 2 — including every NIT — not just BLOCKING/MEDIUM ones. A finding you write down but
leave unfixed as "low priority" is not actually a reported finding; it is a dropped one. The only
legitimate reason to leave a finding unfixed is that fixing it genuinely requires something you
cannot do alone this round — an operator decision on a real design tradeoff. Even then you must say
so explicitly, with the specific reason, in the fixer report's deferred section.

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
None — round 1 fixed every finding it recorded; nothing was deferred.

## Fixing — after the review
- Fix EVERY finding from your review, all severities including NIT — not just BLOCKING/MEDIUM ones.
- Load the code-quality guidance (`/code-quality` skill) AND the Go-specific skills
  (`mill:golang-build`, `mill:golang-testing`, `mill:golang-comments`) before editing — all of them,
  not code-quality alone. Prefer surgical edits; match existing style and the file-level doc-comment
  convention already used across `gitrepo.go`/`push.go`/`snapshot.go`.
- For every bug you fix, add or extend a test that would have caught it. For a real-substrate-only
  defect (a concurrency race, a crash-recovery gap), add it under the `integration` build tag,
  following the existing fixture helpers (`newRepo`, `newBareRemote`, `newRepoWithRemote`,
  `cloneFromBare`) rather than inventing a parallel style. A pure hermetic unit test is right for
  logic that doesn't need real git (extend `keyvalidation_test.go`'s style if applicable); an
  integration test is right for anything that needs the real subprocess/lock behavior.
- MAKE CONCURRENCY TESTS DETERMINISTIC. A test that assumes a fixed timing window passes on a quiet
  machine and FLAKES on a loaded one. Wait on actual state transitions (poll with a deadline,
  synchronize goroutines/processes with explicit signals), never a fixed `time.Sleep` alone as the
  sole synchronization. Prove determinism by running the new test many times (`-count=5` or more)
  under load, not once.
- Keep `go build`/`vet`/`test` (both hermetic and `-tags integration`) green after every change.
- Update `internal/gitrepo/doc.go`'s package doc (and `docs/overview.md` / `CONSTRAINTS.md` if
  invariants or the module table move) IN THE SAME change as any behavior fix. Do NOT add
  bugfix/hardening notes to `manifest/roadmap.md` (roadmap is planned milestones only, per
  `CLAUDE.md`).
- Tear down all substrate state (kill any harness subprocess you spawned; confirm zero stray
  processes). COMMIT each fix as you finish it (see "Commit per fix" above) — do NOT push unless
  the user explicitly asks. Report the changed files and how you verified each fix.

## Deliverables
1. A structured review report (Executive summary with top risks + merge-readiness opinion; Scope
   assessment plan-vs-shipped; Code findings severity-ranked with file:line + scenario + fix +
   CONFIRMED/PLAUSIBLE; Docs & operability findings; What-was-tested with exact commands + observed
   results, including what you could NOT verify and why). Write it to
   `.scratch/gitrepo-review-<yourtag>.md`.
2. A fixer report: what you implemented, what you deliberately deferred (with reasons), the exact
   test commands run + results, and the changed files. Write it to
   `.scratch/gitrepo-review-<yourtag>-fixer-report.md`.
3. In your final chat message: a concise summary (executive summary + counts by severity + the two
   report paths + an explicit merge-readiness verdict). Do not paste the whole reports.

Begin with the clean-room review (read `doc.go` + code + docs, then drive the real substrate),
produce your independent findings, then implement and verify the fixes.
