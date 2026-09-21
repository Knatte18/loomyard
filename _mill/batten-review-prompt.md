# batten — independent review + fix (round prompt)

> Filled instance of `crucible/review-prompt-template.md`. Rewritten fresh each round and committed — see that file's header for why. Read `crucible/README.md` for the loop this prompt runs inside.

You are a senior engineer doing a COMPLETE, adversarial, INDEPENDENT review of the **batten** module in the loomyard repo, followed by FIXING what you find.
Work in the worktree at `/home/hanf/Code/loomyard/wts/crucible-batten-end-to-end` (branch `crucible-batten-end-to-end`).
Adjust that path/branch if the task lives elsewhere now.

## Your two jobs, in order
1. REVIEW: form your own independent judgment of batten's scope and correctness.
   Hunt for bugs by reading the code AND by driving the real substrate — this is where the defects hide.
2. FIX: after you have a findings list, implement the fixes one at a time, verify each against the real substrate, keep the whole test suite green, and update the docs in the same change as the fix they document.
   COMMIT after each individual fix lands green (see "Commit per fix" below).
   Do NOT push unless the user explicitly tells you to.

## Commit per fix (BLOCKING — do not batch fixes into one uncommitted diff)
As soon as one finding's fix is implemented, green (`go build`/`vet`/hermetic test, plus the live check if the finding needed one), and its doc update (if any) is included, COMMIT it — on the current branch, no push — before starting the next finding.
Commit message format: `batten: fix <finding-id> — <one-line what/why>`.
Also commit `_mill/batten-review-<yourtag>.md` and `_mill/batten-review-<yourtag>-fixer-report.md` as you write or update them — they are NOT gitignored scratch, they are the campaign's durable record.

## Sequencing rule (BLOCKING — do not skip, do not interleave)
Job 1 must be COMPLETE — and its full review report SAVED to `_mill/batten-review-<yourtag>.md` and committed — before you touch (edit, create, or delete) a single production or test file.
Do not fix findings as you go, even ones that look small and obviously right.
Write it down as a finding, keep reading, finish the review, save the file, THEN start Job 2.

## Log as you go during Job 1 (BLOCKING — crash-resilience, do not batch it all to the end)
Append your observations to `_mill/batten-review-<yourtag>.md`'s "What was tested" section immediately after each command/scenario returns. Jot findings into the file's findings section provisionally as you spot them — the executive summary and final severity ordering can wait, individual findings and test observations cannot.
COMMIT each meaningful append (a finished scenario, a new finding), not just write it to disk.

## Clean-room review constraint (do this part unprimed)
Form your OWN findings first. Do NOT read any prior review or review-dialogue files before you have your own list.
Specifically do not open anything under `_mill/` matching `batten-review-*` — this covers prior review reports, fixer reports, and the orchestrator's own running handoff note (`batten-review-HANDOFF.md`, if one exists by the time you run) — that file is the orchestrator's private state, not a review, but matches the pattern and is exactly as off-limits.
Reading the SPEC and the module docs is expected and required (those are not reviews).
AFTER you have written your own independent findings, you MAY consult prior rounds' `_mill/batten-review-*` material (if any exists by your round) — regardless of which model produced it — EXCEPT your own `-<yourtag>` deliverables — to (a) confirm previously-fixed behaviors have not regressed and (b) re-evaluate the deferred items at the bottom.

## What to read
- Code (the module itself): `internal/battenshed/*.go` (`create.go`, `seamchild.go`, `innerrun.go`, `teardown.go`, `ctx.go`, `deps.go`, `stuck.go`, `doc.go`, plus every `_test.go` and `seam_enforcement_test.go`), `internal/battenrecipe/*.go` (`battenrecipe.go`, `names.go`, `recipe_test.go`, `coverage_guard_test.go`, `seam_enforcement_test.go`, `fixture_test.go`, `doc.go`), `internal/battencli/*.go` (`cli.go`, `arm.go`, `bootstrapverb.go`, `commitstatus.go`, `paths.go`, `refusal.go`, `wire.go` + tests, especially `lifecycle_integration_test.go` and `testmain_integration_test.go`), `contracts/recipes/batten-recipe.yaml`.
- Boundary/neighboring code (read for context, not the target of your review): `internal/boardengine/task.go` (the `Type` field, `omitempty`, empty means `loom`), `internal/shedrun` (the `seed.json` contract, run-directory path segments, `ReadSeed`/`WriteSeed`/`List`), `internal/shedengine` (`Status` shape, `Done`/`Stuck` outcome semantics, bounce/`on_stuck` mechanics), `internal/shedcli` (`lyx shed seed/run/step/status/pause`), `plugins/ly/skills/ly-drive/SKILL.md` (how a `driver: llm` seeded run is actually driven forward step by step — this is what runs *inside* the child worktree once Run-Shed hands off), `internal/loomcli/driverlaunch.go` + `driverspec.go` + `driverreport.go` + `internal/loomengine/driver.go` (how a loom run reads and acts on `driver: llm` from its own seed).
- Docs: `docs/overview.md` (the batten module-table entry, ~lines 369–372; the nested-Shed paragraph on the two-status-files design, ~lines 404–408), `CONSTRAINTS.md` (Batten Bookend Invariant; Shed Run-Directory Invariant; Sandbox Suite Coverage; Live-Substrate Spawn Observability; the `battencli`-has-no-engine-package naming deviation), root `README.md`, `CLAUDE.md`.
- Design intent (SPEC, not a review): `git show 8ac857ce1~1:manifest/designs/seeded-shed.md` — the module doc was deleted post-ship per this repo's convention (`manifest: delete every design doc whose work has shipped`), so recover it from git history. Treat it as the authoritative statement of intended v1 scope — including its own two named, consciously-shipped-as-is residuals (see High-yield focus item 10 below). Read it before assuming any gap is unintentional.
- Scenario ideas only (not a review, and not something to treat as complete coverage): `tools/sandbox/SANDBOX-FABRIC-SUITE.md`'s scenario **F22** ("Batten status and refusal surface"), tagged `**Covers:** batten`. Its own fixture note says it deliberately never drives past the create row, because that's exactly where a real driver would spawn. You ARE driving past it this round — that gap is this campaign's whole reason for existing. Run every scenario yourself, directly, with your own tool calls; do NOT invoke any `sandbox-*-suite.cmd` launcher.
- Repo rules you MUST follow: `CLAUDE.md` and `CONSTRAINTS.md` (Batten Bookend, Fabric Git, Shed Run-Directory, Sandbox Suite Coverage, Live-Substrate Spawn Observability, CLI/Cobra, Documentation Lifecycle).

## Mission (assess on two axes, be adversarial)
1. Scope — does the as-built code deliver what the (recovered) design doc intended? Gaps, over-reach, silently-dropped requirements, deferred-that-should-ship-in-v1.
2. Correctness — bugs, races, error handling, edge cases; concentrate on the historically-fragile areas below (this module has NEVER been driven end-to-end with a real driver by any test or prior review). Also assess docs accuracy and operability.

## High-yield focus — where batten's real bugs live (drive these, do not just read them)
Batten has zero `//go:build smoke` tests and its one `integration`-tagged test (`lifecycle_integration_test.go`) stubs `InnerRunDeps.Spawn`/`ReadStatus` at the field level — so nothing has EVER exercised a real `Worktree-Create → Seed-Child → Run-Shed → Worktree-Teardown` pass against a real driver. That is the gap this whole campaign exists to close; treat every item below as genuinely unverified until you drive it live.

1. **Batten Bookend Invariant.** Every batten producer must run FROM the hub's prime worktree, never from inside the task worktree it manages. Drive `lyx batten run/step/status <slug>` from inside the freshly-created task worktree itself and confirm each refuses, naming both worktree names and telling the operator to re-run from prime (per F22 and `CONSTRAINTS.md`).
2. **Teardown ordering.** `worktreeTeardownProducer.Call` (`teardown.go:53`) calls `Shutdown` strictly before `Remove` and must never call `Remove` at all if `Shutdown` fails (`teardown.go:76-84`). Interrupt a real teardown mid-flight and confirm no state is skipped, and that a worktree whose session shutdown failed is never removed.
3. **Run-Shed never routes to teardown on failure.** `innerRunProducer.Call`'s `StateBlocked`/`StatePaused`/`StateFailed` branch (`innerrun.go:127-128`) returns a hard error, not `Stuck`, specifically so `shedengine` marks Run-Shed itself `StateFailed` rather than bouncing anywhere (`batten-recipe.yaml`'s own header comment says this explicitly, and the destructive row is structurally unreachable from any `on_stuck`). Drive a real child run to a genuinely failed/blocked state and confirm the task worktree is NOT torn down.
4. **`InnerRunDeps.Spawn`'s documented contract says it "blocks until it exits"** (`deps.go:43-48`), yet the row is re-entered every 30s via `on_stuck: Run-Shed` rather than looping internally. Pin down PRECISELY what "exits" refers to by timing a real drive: does the first `InnerRun.Call` block only for the loom bootstrap launch, or could it ever block for the whole nested campaign? Trace it in the real wiring (`battencli`'s `wire.go`), not just the doc comment — a wrong answer here means `lyx batten step`'s very first advancing call could hang for the length of an entire real loom campaign.
5. **Seed-Child's Board-type → recipe → driver plumbing** (`seamchild.go`). Drive it with a real Board task carrying `type: loom` and a prime seed carrying `--driver llm`, and confirm the CHILD's own `seed.json` really ends up with `recipe: loom`/`driver: llm` — not silently defaulted — and that the nested `ly-drive` loop really does spawn a real provider off that value.
6. **`PrimeRunLock` scope.** It is acquired only during Worktree-Create and Worktree-Teardown, never during Run-Shed's long watch. Two DIFFERENT slugs' create/teardown must serialize against each other, while one slug's long-running Run-Shed must NOT block another slug's create or teardown. Drive two slugs interleaved and confirm both properties.
7. **Cold-machine self-heal / mid-operation-failure orphans.** Kill the driving process between Worktree-Create succeeding and Seed-Child running (before a seed commit lands), and again between Seed-Child's commit and its push. Confirm a resumed `lyx batten step`/`run` neither double-creates nor double-seeds, and reports honestly what state it's actually in.
8. **Sabotage — a raw git commit landed directly on weft outside fabric's own seams during a live run.** Does it corrupt batten's own status tracking or the later teardown, and is it detected/reported rather than silently absorbed?
9. **Sabotage — a stray untracked file dropped under `_lyx` mid-run.** Does the next weft commit silently sweep it in (a "stage-all lottery") or silently drop it? Either outcome without an explicit, honest report is the failure mode — determine which actually happens and whether it's reported.
10. **The two residuals the (deleted) design doc shipped consciously "as-is"** — re-verify both are still true against the current code, and judge whether either should now be a recorded finding rather than an accepted gap: (a) a dead driver strand in the child isn't detected or recovered by batten itself; (b) a cleanly finished driver's strand/run-directory isn't torn down automatically by batten (only the whole-worktree teardown row cleans up, at the very end).
11. **`lyx shed seed <slug> --recipe batten` pre-seeding ahead of batten's own auto-seed** (F22's own scenario, extended). Confirm batten's create/seed path treats a hand-written seed as authoritative rather than silently overwriting it, and that a genuinely disagreeing hand-seed is refused rather than silently adopted.

## Explicitly OUT of scope for batten v1
- Batten growing its own bootstrap verb or an `llm` driver option for ITS OWN producer rows (`Worktree-Create`/`Seed-Child`/`Run-Shed`/`Worktree-Teardown`) — deliberately `go`-only by design per the recovered design doc ("`go` stays the only driver batten runs use"). Do not flag this as a gap.
- "Relay-stepping" (Run-Shed subprocess-execing `lyx shed step` inside the child) — an explicitly rejected alternative in the design doc. Do not suggest reintroducing it.
- loom's OWN internal phase-machine correctness (Discussion/Planner/review-round producer bugs) beyond the boundary contract batten actually depends on — `InnerRun` only ever reads the child's persisted `State`/`Error`/`CurrentProducer` fields. A bug purely inside loom's own phase advancement is a different module's scope (see the separate, deferred `crucible-loom-step-selfreport-r4` task) unless it corrupts or mis-reports through that specific status boundary.
- Windows path behaviour anywhere in this stack — unreachable from this Linux host. State plainly that you did not touch it; do not newly flag it as missing.
- fabric's own clone/merge correctness beyond what batten's Worktree-Create/Teardown actually exercise (creating and removing a worktree pair) — fabric has its own separate crucible lineage.

## Round context seeded from prior-round verification
**This is round 1 of a brand-new campaign — there is no prior round and nothing is CLOSED-AND-VERIFIED yet.** This is NOT a safety pass. Do the full independent review + fix described above, starting from the High-yield focus list but not stopping there — the fixture-note gap in F22 (real end-to-end driving has never happened) means you should expect to find real defects.

State the **merge bar** so you calibrate: correctness in the NORMAL single-instance flow (one slug, driven start to finish) is the gate. The generic N×-concurrent-suite gate from `crucible/README.md`/`orchestrator-prompt.md` does NOT apply to batten's real end-to-end live-driving scenario at all (see the cost declaration below) — never attempt it. Item 6 above (two DIFFERENT slugs interleaved) is a real, in-scope concurrency check; it is not the N×-concurrent amplifier gate.

## Live-substrate cost declaration (BLOCKING)
**No automated smoke tests exist to classify** — there are zero `//go:build smoke` files under `internal/battenshed`, `internal/battencli`, `internal/battenrecipe`, so no `-tags smoke` command spawns anything, and batten's own producer rows are structurally `go`-only (mechanical, no LLM/Bouncer/shuttle row of batten's own).

**However — read this before driving live.** This campaign's whole mission (per the mill-wiki task) requires you to seed a child task worktree with `--driver llm` and drive Run-Shed to watch the nested loom campaign run for real, which DOES spawn a real Claude provider subprocess inside the child's own reed session once the campaign reaches loom's Discussion/Planner/review rows. Treat that one deliberate end-to-end drive with the same discipline an LLM-driving module's live smoke test gets:
- Foreground, one at a time, never concurrently, never backgrounded-and-abandoned.
- Do the full real end-to-end drive (create → seed → run to a terminal state → teardown) ONCE this round as the primary live-driving scenario. Re-run it again only if a fix genuinely requires re-proving the whole path — do not repeat it casually.
- NEVER write an automated tagged test (`smoke`, `integration`, or otherwise) that itself seeds `--driver llm` and spawns a real provider inside `go test`. Every existing batten test deliberately stubs `InnerRunDeps.Spawn`/`ReadStatus` — keep that pattern for any new regression test you add. A real-provider spawn belongs only in your own manual, narrated CLI driving.
- Use the smallest fixture task you can — a trivial one-file change on a minimal seeded Go project (see "Fixture hub" below) — so the nested loom campaign's own real discussion→plan→build→review chain finishes in the shortest real time that still genuinely exercises it. This is a real agent doing real work end to end and will take real wall-clock minutes-to-hours; that cost is expected and budgeted, never a reason to skip it or substitute code-tracing.
- The generic "N× CONCURRENT full smoke suites" gate does not apply here — never run it against this scenario.
- MACHINE NOTE (this host): `GOPROXY` is set to `direct` machine-locally, because the route to `proxy.golang.org` stalls partway through any large object from this network — `github.com` serves the same bytes fine. If a build inside the fixture hub fails fetching `github.com/Knatte18/quarry@v0.2.0`, that is this environment, NOT a batten defect — do not log it as a finding.

## Fixture hub (build this yourself before live driving)
Build a DISPOSABLE hub — never the operator's standing `lyx-test-LYXHUB` bench:
1. `git init --bare` a warp repo (seeded with a minimal Go project — a `go.mod` and a trivial `main.go` is enough) and a weft repo, by hand, in a scratch directory OUTSIDE both the loomyard tree and `$HOME/Code` (so it can never be mistaken for the standing bench).
2. `lyx fabric clone --into <scratch> <weft.git> <warp.git>` to wire the pair.
3. Materialize `_board` as a second weft worktree on `weft:main` inside that same hub — your campaign's own Board task (the one carrying `type: loom` for a real batten run) lives only there.
4. Disposal at teardown: `rm -rf <scratch>`. No GitHub repos, no network involved.
Build it so it survives across your own separate shell tool calls (i.e. actually persist it to disk in that scratch directory, don't rely on shell state) — you'll return to it across many separate commands while driving the live scenarios above.

## What to TEST — do not just read, EXERCISE it
Report the exact commands you ran and what you observed.

Hermetic (must stay green throughout):
- `go build ./...`
- `go vet ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/...`
- `go test -count=5 ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/... ./cmd/lyx/...`

Live integration (existing stubbed-spawn coverage — must stay green throughout, and does NOT satisfy the live-driving requirement above):
- `go test -tags integration -count=1 ./internal/battencli/...`

Live driving — YOU drive it directly, no launcher (PRIMARY — where the bugs surface):
- Deploy the current source as the dev binary under test: `./deploy-dev` (POSIX script on this host, not `.cmd`). **FOOTGUN:** live driving runs the DEPLOYED snapshot, not your working tree — re-run `./deploy-dev` after EVERY source change.
- Build the disposable fixture hub above. Seed a Board task with `type: loom` (or leave it empty to prove the `loom`-default path works too, on a separate drive) and a prime seed with `--driver llm`.
- Do NOT invoke any `sandbox-*-suite.cmd` launcher — `tools/sandbox/SANDBOX-FABRIC-SUITE.md`'s F22 is for scenario ideas only.
- Walk the High-yield focus list above with your own `lyx batten`/`lyx shed` tool calls, foreground, waiting for each to return. Devise MANY more adversarial scenarios beyond that list — it is a floor, not a ceiling.
- "Headless" means "no human required," never "no time/token cost to me." A real driven loom campaign takes real wall-clock minutes-to-hours; that is expected and budgeted, never a reason to skip a scenario, fall back to code-tracing, or write "operator-assisted"/"cost-bearing"/"long-running" as a reason to skip. The only legitimate "cannot verify" cases are a genuine environment gap (check for this FIRST) or a scenario structurally requiring a human's physical eyes.

TEARDOWN DISCIPLINE (critical): tear down every session/server you start. At the end, confirm ZERO stray `tmux`/`claude` processes (`tasklist | grep -i tmux` / `pgrep -fl claude` as applicable), confirm the disposable fixture hub is fully `rm -rf`'d, and confirm the operator's standing `lyx-test-LYXHUB` bench was never touched. Be honest about what you could NOT verify and why.

## How to judge each finding
For each code finding give: `file:line`, a concrete failure scenario (inputs/state → wrong behavior), severity (BLOCKING / MEDIUM / LOW / NIT), suggested fix, and CONFIRMED (reproduced/traced) vs PLAUSIBLE (looks wrong, unverified).
For scope: plan-promised vs shipped; flag deferred-that-should-be-v1 and shipped-beyond-scope.

**Severity affects how you REPORT a finding, not whether you fix it.** ALL findings you record get fixed in Job 2 — including every NIT. The only legitimate reason to leave a finding unfixed is that fixing it genuinely requires an operator decision on a real design tradeoff, or a capability you don't have — say so explicitly in the fixer report's deferred section, with the specific reason, never bucketed as "low priority."

## Deferred items from the prior round — RE-EVALUATE these (after your own pass)
None — round 1, nothing carried forward from a prior crucible round. (High-yield focus items 10 and 11 above carry forward the design doc's own consciously-accepted gaps — those are a different thing from a deferred crucible finding, and are already folded into the focus list.)

## Fixing — after the review
- Fix EVERY finding from your review, all severities including NIT.
- Load `/code-quality` AND `/golang:golang-build`/`/golang:golang-testing`/`/golang:golang-comments` before editing — all of them, not code-quality alone.
- For every bug you fix, add or extend a test that would have caught it. For a live-only defect, add a fixture-based test matching the existing stubbed-`Spawn`/`ReadStatus` pattern (never a real-provider spawn inside `go test` — see the cost declaration above).
- MAKE ANY NEW TIMING-SENSITIVE TEST DETERMINISTIC — poll with a deadline, never sleep a fixed amount; prove it under `-count=5`.
- Extend `tools/sandbox/SANDBOX-FABRIC-SUITE.md`'s F22 if a review surfaces a live/visual behavior it doesn't cover (keep `sandbox_coverage_test.go` green); otherwise note the new scenario in your fixer report.
- Keep `go build`/`vet`/`test` green after every change. Then RE-DEPLOY (`./deploy-dev`) and re-run every live scenario yourself, directly.
- Update `docs/overview.md`'s batten entry (and `CONSTRAINTS.md` if any invariant moves) IN THE SAME change as any behavior change. Do NOT add bugfix/hardening notes to `manifest/roadmap.md`.
- Tear down all substrate state; confirm zero stray processes and a fully disposed fixture hub. COMMIT each fix as you finish it — do NOT push unless explicitly asked.

## Deliverables
1. A structured review report (Executive summary with top risks + merge-readiness opinion; Scope assessment plan-vs-shipped; Code findings severity-ranked with file:line + scenario + fix + CONFIRMED/PLAUSIBLE; Docs & operability findings; What-was-tested with exact commands + observed results, including what you could NOT verify and why). Write it to `_mill/batten-review-<yourtag>.md` and commit it — built incrementally per "Log as you go" above.
2. A fixer report: what you implemented, what you deliberately deferred (with reasons), the exact test commands run + results, and the changed files. Write it to `_mill/batten-review-<yourtag>-fixer-report.md` and commit it (folding into a fix commit is fine).
3. In your final chat message: a concise summary (executive summary + counts by severity + the two report paths + an explicit merge-readiness verdict). Do not paste the whole reports.

Begin with the clean-room review (read the SPEC + code + docs, then drive the real substrate), produce your independent findings, then implement and verify the fixes.
