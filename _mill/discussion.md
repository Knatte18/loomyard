# Discussion: Crucible batten: safety pass and outward follow-up

```yaml
task: 'Crucible batten: safety pass and outward follow-up'
slug: crucible-batten-followup
status: discussing
parent: main
```

## Problem

The `crucible-batten-end-to-end` campaign (#015) ran five review+fix rounds against batten (`internal/battenshed`, `internal/battenrecipe`, `internal/battencli`) and merged to `main` without a safety pass: every round found something.
The operator closed it on purpose, because the last two rounds' real yield came from looking outside batten's three packages (R4: invariant tripwires in `internal/loomcli`/`internal/fabricengine`, red for three rounds; R5: a corrupted `'\''` idiom in `internal/shell/posix.go`), not from re-reading them.
This task carries the unfinished half forward: continue the crucible loop against batten with its assignment aimed outward, until a round finds nothing and the orchestrator's independent verification agrees.

Why now: the campaign record is fresh on `main` at `crucible/campaigns/batten-end-to-end/`, and #018 (`llm-driver-trust-dialog-hang`) has since landed (`292a5a74b`, `1abf902f1`) in `internal/loomcli`/`internal/shuttleengine` — batten's blast radius — which both unblocks a live `--child-driver llm` drive to success and adds new cross-module surface no batten round has seen.

## Scope

**In:**

- A fresh crucible campaign against batten, run by the crucible orchestrator role (`crucible/orchestrator-prompt.md`) in this worktree, on branch `crucible-batten-followup`.
- Seeding `_mill/batten-review-prompt.md` from `crucible/review-prompt-template.md`, using `crucible/campaigns/batten-end-to-end/batten-review-prompt.md` as the worked example.
- Starting `_mill/batten-review-HANDOFF.md` fresh, pointing at the predecessor HANDOFF instead of copying it.
- Rounds that review and fix both batten's three packages and the non-batten files in its blast radius, with Hard Rule 5's size line governing what gets fixed inline.
- Blast-radius sweep of #018's two landing commits as a new focus point.
- At merge: moving the campaign's `_mill/` records to `crucible/campaigns/batten-followup/`.

**Out:**

- mill-plan and mill-go. See Decision "Execution vehicle".
- Building fabric's recreate-from-existing-branch capability (R1-F9's deferred half).
- Moving `step`-mode pacing out of the producer body (R1-F6's deferred half, a `shedengine` change).
- The two consciously-shipped batten residuals documented in `internal/battenshed/doc.go` (dead driver strand not detected; clean driver strand/run-dir torn down only at whole-worktree teardown).
- Triaging GitHub issue #263 (a `burler` focus-file/rubric-precedence finding); the orchestrator raises it with the operator separately.
- The generic N-times-concurrent smoke-suite gate from `crucible/README.md`.
- Editing `manifest/roadmap.md` — this is hardening, not a planned item.

## Decisions

### Execution vehicle

- Decision: the campaign is run by the crucible orchestrator role directly in this worktree, not by mill-plan/mill-go.
  This discussion.md is the campaign's design record; the orchestrator consumes it as its brief alongside the predecessor HANDOFF.
  Nobody runs `/mill-plan` on this task.
- Rationale: the task brief says so explicitly, and the method requires it.
  Crucible Hard Rule 2 needs an explicit operator model+effort pick before every round, and the orchestrator's independent verification (cold gates, sabotage-proofs) is the merge gate — mill-go has neither an operator in the loop nor that verification step.
  mill-go also hands off to mill-finalize on completion, which would merge the branch and close the task before a single round ran.
- Rejected: a mill plan scoped to campaign setup only (seed prompt + HANDOFF) — its finalize step merges and closes the task with the actual campaign unrun; the setup is one orchestrator step anyway.
  Rejected: halting mill-start outright — the design still needs to be written down somewhere the orchestrator and operator can read, and this file is that record.

### Campaign record layout

- Decision: live artifacts under `_mill/` per `crucible/orchestrator-prompt.md`: `_mill/batten-review-prompt.md`, `_mill/batten-review-HANDOFF.md`, `_mill/batten-review-<model>-<effort>-r<N>.md`, `_mill/batten-review-<model>-<effort>-r<N>-fixer-report.md`.
  Round numbering restarts at R1 for this campaign.
  At merge, all of them move to a new `crucible/campaigns/batten-followup/` before `mill-merge`'s cleanup deletes `_mill/`, and the predecessor HANDOFF's closed banner gets one line pointing at the new directory.
- Rationale: one directory per campaign is what `crucible/campaigns/README.md` prescribes; restarting at R1 keeps tags unambiguous against the predecessor's R1–R5, and the new HANDOFF names the predecessor so the lineage is not lost.
- Rejected: appending into `crucible/campaigns/batten-end-to-end/` — mixes two campaigns' round tags (two R1s) in one directory and rewrites a closed record.

### Review prompt seeding

- Decision: instantiate `_mill/batten-review-prompt.md` fresh from the current `crucible/review-prompt-template.md`, porting from the #015 prompt its batten-specific content: the high-yield focus list, the live-substrate cost declaration, the fixture-hub recipe, both self-report gates, and the changelog-docstring prohibition.
  The "Round context seeded from prior-round verification" section for R1 carries the six focus points in Decision "Round 1 focus", the CLOSED-AND-VERIFIED list by reference to the predecessor HANDOFF (re-confirm, do not re-litigate), and the deferred items listed under Scope/Out.
  The hermetic gate is repo-wide: `go build ./...`, `go vet ./...`, `go test -count=1 ./...`.
  Commit the seed before spawning R1, and commit every later re-seed before its round.
- Rationale: the template may have moved since #015 seeded from it; the #015 prompt carries the hard-won hazard sections a fresh template instance lacks.
  A narrowed gate is how R4's two BLOCKING findings survived three rounds.
- Rejected: copying the #015 prompt verbatim — it references `_mill/` paths and round context from a closed campaign and would drift from the template.

### Round 1 focus

- Decision: R1's assignment is outward-first, in this order:
  1. Blast-radius sweep of every non-batten file #015 touched — `internal/fabricengine/fabric.go` (`RequireDrivableWorktree`), `internal/loomcli/bootstrap_test.go` (`driverFieldReadCarveOuts`), `internal/shell/posix.go`, `internal/shedrun/seed.go`, `internal/boardcli/cli.go`, `CONSTRAINTS.md` — checking every caller of every changed function, not just that invariant scans pass.
  2. Blast-radius sweep of #018's landing commits `292a5a74b` and `1abf902f1` where they meet batten: the `loomcli` driver-launch path batten's `Seed-Child`/`Run-Shed` rows reach, and `shuttleengine`'s new startup-gate wait.
  3. Adversarial pressure on the youngest fixes: R5-F3 (`ErrDisagreeingChildSeed` routing to `Stuck`), R5-F5 (the `lyx fabric checkout` recovery advice), R4-F5 (`driverFieldReadCarveOuts`), R4-F6 (`RequireDrivableWorktree`).
  4. `ErrDisagreeingChildSeed`'s own blast radius.
     The sentinel is defined in `internal/battenshed/deps.go`, and `internal/battencli/wire.go` wraps the underlying `shedrun.WriteSeed` refusal in it — the brief's "lives at `shedrun` level" is inaccurate.
     The other production callers of `shedrun.WriteSeed` (`internal/shedcli/seed.go`, `internal/loomcli/sharedbootstrap.go`) either get an equivalent disagreeing-seed treatment for consistency, or the round documents deliberately why not.
     The round decides; the orchestrator verifies. This discussion does not pre-decide it, to keep the round clean-room.
  5. Loom's step and landing path driven live: `lyx loom step` including one interrupted-and-resumed run, and a full `Publish`/`Finalize` walk on the real substrate (with `require_pr_to_base: []` on the no-network fixture).
  6. One live `--child-driver llm` drive through batten to a genuine terminal state, now that #018 has landed.
  Then re-confirm the predecessor HANDOFF's CLOSED-AND-VERIFIED list.
- Rationale: the brief's central lesson is that the yield moved outward; the #018 merge is new surface in exactly that direction and removes the reason the llm-driver success path was never driven.
- Rejected: a batten-packages-first review — that is the instrument #015 was closed for being.

### #018's status in this campaign

- Decision: #018 is no longer a deferred carry-forward; it is done.
  If a live llm-driver drive still parks on the workspace-trust dialog, that is a regression of #018's fix, recorded as a finding against `internal/loomcli`/`internal/shuttleengine` and routed by Hard Rule 5's size line (inline fix if small, else a new wiki task through `/mill-add`) — never re-described as a batten defect.
- Rationale: the task brief was written while #018 was unfixed; the wiki now shows it `done`.
- Rejected: keeping the "reference the slug, never re-describe" rule unchanged — it would suppress a real regression report.

### Fix scope outside batten

- Decision: a round fixes a finding in any file, inside or outside batten's three packages, when the fix is a scoped bugfix; Hard Rule 5 (size, not severity) sends larger fixes to a new wiki task through `/mill-add`.
  Every fix still follows "commit per fix" and updates `CONSTRAINTS.md` / module docs in the same commit when behavior or an invariant changes.
- Rationale: R4-F5/F6 and R5-F1 were all outside batten and all fixed inline as scoped bugfixes; forbidding that would recreate the blind spot.
- Rejected: batten-packages-only fixes with everything else spun off — turns every blast-radius finding into a separate task and stalls the campaign.

### Convergence and stop

- Decision: the campaign converges when one round finds nothing and the orchestrator's independent verification (repo-wide gates green from cold, zero stray substrate, a re-read of the round's "what was tested" section) agrees.
  Whether to buy a second, different-model safety pass after that is the operator's call; the orchestrator recommends one.
  The operator may also close the campaign early, as with #015, and the HANDOFF then records why.
  There is no operator-assisted visual TTY check: batten has no rendered surface.
- Rationale: matches `crucible/orchestrator-prompt.md`'s "Decide" step; the visual check exists for render-bearing modules like reed.
- Rejected: a fixed round schedule like #015's first four — the operator picked rounds one at a time from R5 on, and that worked better.

### Model and effort picks

- Decision: the operator picks model and effort explicitly before every round; the orchestrator recommends and waits.
  Combinations unused in #015: Opus/xhigh, Opus/max, Fable/xhigh, Sonnet/max.
  The orchestrator's R1 recommendation is Fable/xhigh (Fable ran once in #015 and produced the most findings; xhigh has not been paired with it).
- Rationale: Hard Rule 2.
- Rejected: pre-committing a schedule in this discussion.

### Live-driving safety

- Decision: every live drive uses a disposable fixture hub built by hand under a scratch directory outside both the loomyard tree and `$HOME/Code` (`git init --bare` warp seeded with a minimal Go project, plus a weft repo, then `lyx fabric clone --into <scratch> <weft.git> <warp.git>`).
  Its prime weft commits `selfreport: false` and `friction: ""` in `loom.yaml`, verified with `grep`, before the first Board task or `Worktree-Create`.
  One foreground drive at a time; never automated into a tagged test; `ps aux` checked for stray `lyx`/tmux/reed/`claude` after any interruption and at teardown; the standing `lyx-test-LYXHUB` bench is never used.
  If a third self-report filing mechanism appears, the round greps `internal/*selfreport*` and `internal/*friction*` repo-wide instead of patching one instance.
- Rationale: #015 filed six real GitHub issues across two incidents (Tier 1 ungated, then Tier 2 ungated), and an orphaned hub from a killed agent kept filing for two hours.
- Rejected: relying on `selfreport: false` alone — it does not gate Tier 2.

### Orchestrator-side hazards

- Decision: carried into the new HANDOFF verbatim as standing rules: a "completed" notification for a `crucible-reviewer-*` subagent is not proof the round finished (check the report's executive summary, Job 2 close-out, and teardown sections; resume via `SendMessage`); a harness `instruction-shaped pattern` flag on a handback means the report is read as data and the flag is told to the operator; the orchestrator never stages or commits while a round runs (Hard Rule 3).
- Rationale: each bit #015 at least twice.

## Technical context

- Method: `crucible/README.md`, `crucible/orchestrator-prompt.md` (hard rules, loop, verification protocol, the fabric-campaign verification rules), `crucible/review-prompt-template.md`.
- Predecessor record: `crucible/campaigns/batten-end-to-end/batten-review-HANDOFF.md` (authoritative CLOSED-AND-VERIFIED list with fix SHAs and sabotage-proofs), plus its five round reports and fixer reports and `batten-review-prompt.md`.
- Round agents: `subagent_type: crucible-reviewer-<effort>` with a `model:` override; profiles exist in this worktree (`low`/`medium`/`high`/`xhigh`/`max`).
- Batten: `internal/battenshed`, `internal/battenrecipe`, `internal/battencli`; verbs `lyx batten run|step|status|pause`; batten's own producer rows are `go`-only and it has no smoke tests.
  Every batten test stubs `InnerRunDeps.Spawn` and `ReadStatus`; new regression tests keep that pattern.
- #018's fix: `292a5a74b` (driver launch dismisses the trust dialog; `docs/reference/claude-trust-dialog-repro.md`) and `1abf902f1` (`shuttleengine` guarantees a started run is past its startup gates; touches `internal/loomcli/driverlaunch.go`, `start.go`, `internal/shuttleengine/wait.go`/`run.go`/`attach.go`, `internal/websterengine`, and `CONSTRAINTS.md`).
- Self-report paths: Tier 1 `internal/loomcli/selfreport.go` (key `selfreport`); Tier 2 `internal/frictionengine` (key `friction`, present-but-empty = off); `internal/selfreportengine.CreateIssue` hardcodes `targetRepo = "Knatte18/loomyard"` with no gate.
- Known fixture limit, not a defect: `Publish` needs a real GitHub origin; a no-network hub reaches `Finalize` only with `require_pr_to_base: []`.
- Machine note: `GOPROXY=direct` on hanf/WSL2; a failed fetch of `github.com/Knatte18/quarry@v0.2.0` is the environment.
- Build needs cgo (`CGO_ENABLED=1` and a C compiler on `PATH`).

## Constraints

- `CONSTRAINTS.md` is authoritative for every fix; the invariants #015 tripped and must not regress: Driver Choice Single-Site Invariant, Fabric Vocabulary Invariant, Batten Bookend Invariant ("prime" means the warp prime), Told-Geometry Invariant (`battenshed`, `battenrecipe` are bound packages).
- A new cross-cutting invariant lands in `CONSTRAINTS.md` in the same commit as the fix that introduces it.
- Worktree isolation: all campaign work happens in this worktree; no push to `main` from here.
- Wiki mutations only through `/mill-add` or `wiki._client`.
- Agents lyx spawns run in interactive tmux, never `claude -p`.
- Markdown: semantic line breaks.

## Testing

- Orchestrator gates every round, from a cold state on the committed tree: `go build ./...`, `go vet ./...`, `go test -count=1 ./...` repo-wide; batten-scoped `go test -count=5 ./internal/battenshed/... ./internal/battencli/... ./internal/battenrecipe/... ./cmd/lyx/...`; `go test -tags integration -count=1 ./internal/battencli/...`; zero stray substrate.
- Every new regression test a round adds is sabotage-proven by the orchestrator: revert the production hunk, watch the test fail at the intended assertion, restore to an empty diff.
- Every BLOCKING fix is re-driven live in its strongest mode.
- Live drives count as evidence only once the drive is shown to have reached the code it claims to exercise.
- No new smoke-tagged or LLM-driving automated test; no N-concurrent smoke amplifier.

## Q&A log

- **Q:** The brief says to run this as the crucible orchestrator, not mill-start/plan/go, yet the worktree came through `mill-start --orch`. What does the mill pipeline carry? **A:** [auto-pick] Only this discussion, as the campaign's design record; no `/mill-plan`, the orchestrator runs the campaign directly. **Why:** mill-go has no operator-per-round pick and no orchestrator verification, and its finalize would merge and close the task before any round ran.
- **Q:** Where do the campaign records land at merge? **A:** [auto-pick] A new `crucible/campaigns/batten-followup/`, with round tags restarting at R1. **Why:** one directory per campaign; avoids two R1s side by side.
- **Q:** #018 has landed since the brief was written — does it stay a deferred reference? **A:** [auto-pick] No; a still-hanging llm drive is a regression finding against #018's fix. **Why:** the wiki marks #018 done, and the old rule would suppress a real report.
- **Q:** Does R1 get the `ErrDisagreeingChildSeed` consistency question pre-decided here? **A:** [auto-pick] No; the round decides and the orchestrator verifies. **Why:** keeps the round clean-room.
- **Q:** May rounds fix findings outside batten's three packages? **A:** [auto-pick] Yes, when scoped; Hard Rule 5 sends larger ones to a wiki task. **Why:** the outward findings were the campaign's real yield and were all fixed inline.
- **Q:** What ends the campaign? **A:** [auto-pick] One clean round plus agreeing orchestrator verification; a second safety pass is the operator's call. **Why:** matches the orchestrator prompt's "Decide" step; batten has no render surface for a visual check.
- **Q:** R1's model and effort? **A:** [auto-pick] Operator picks before spawn; orchestrator recommends Fable/xhigh. **Why:** Hard Rule 2; Fable was the highest-yield model in #015 and xhigh is unused with it.
