# batten follow-up — orchestrator pre-count (round agents: do NOT open this file)

Counted at `b9ebb1ba9` (branch point `d7ab9eca0` on `main`), before R1 spawned.
Production (non-`_test.go`) sites only unless stated.

## Non-batten files #015 changed (`git show --stat 36900c6f5`)

Production/doc: `CONSTRAINTS.md`, `docs/overview.md`, `internal/boardcli/cli.go` (help text only), `internal/fabricengine/doc.go`, `internal/fabricengine/fabric.go`, `internal/shedrecipe/entries_batten.go` (comment only), `internal/shedrun/seed.go`, `internal/shell/posix.go` (comment only), `tools/sandbox/SANDBOX-FABRIC-SUITE.md`, `crucible/review-prompt-template.md`.
Test: `internal/loomcli/bootstrap_test.go`, `internal/shedrecipe/entries_batten_test.go`, `internal/shedrun/seed_test.go`.
Behavioral changes outside batten: `shedrun.ErrDisagreeingSeed` (new sentinel, `WriteSeed`'s refusal message reworded) and `fabricengine.RequireDrivableWorktree` (new wrapper). Everything else is docs/comments/tests.

## Call-site counts

| Symbol | Production sites | Where |
|---|---|---|
| `shedrun.WriteSeed(` | 4 | `shedcli/seed.go:76`, `loomcli/sharedbootstrap.go:104`, `battencli/wire.go:338`, `battencli/arm.go:222` |
| `shedrun.ErrDisagreeingSeed` consumers | 1 file | `battencli/wire.go` only (`:312` comment, `:339` `errors.Is`) |
| `battenshed.ErrDisagreeingChildSeed` | defined `battenshed/deps.go:34`; matched `battenshed/seamchild.go:130`; wrapped `battencli/wire.go:340` | |
| `RequireWarpWorktree(` callers | 2 | `fabriccli/fabric.go:471,478` (owner set) plus the wrapper itself `fabricengine/fabric.go:160` |
| `RequireDrivableWorktree(` callers | 1 | `battencli/arm.go:286` |
| `WriteSeed` refusal-text matchers ("already seeded with" / "refusing to overwrite") | 0 outside `shedrun` production | `battencli/arm.go:119,125,131` are doc comments; `SANDBOX-FABRIC-SUITE.md:547-548` describes the scenario, not the text |

Blind spots: grep cannot see a call routed through a function value or interface (e.g. a `WriteSeed` closure in `SeedChildDeps`), and counts comment mentions on the same lines as code.
`internal/shell` has no `Quote(` caller spelled `shell.Quote(` — its users call `shell.ForGOOS`/`shell.Shell` methods (7/6 production mentions), so a `Quote` change reaches callers only through the `Shell` interface; a round's count here should be via the interface, not the free function.

## Expected answer to focus point 4 (not binding — for checking the round's reasoning)

`shedcli/seed.go` (`lyx shed seed`) and `loomcli/sharedbootstrap.go` return the `WriteSeed` error straight to a CLI caller, where a hard error is the right outcome — there is no Shed row to route to `Stuck`.
A round concluding "no change needed, document why" is plausible; a round inventing a `Stuck` path in a non-Shed CLI verb is over-reach.

## #018 surface batten reaches

`battencli/wire.go:262` Spawn execs `lyx loom start --no-attach` in the child → `internal/loomcli/start.go` / `driverlaunch.go`, both changed by `292a5a74b` and `1abf902f1`.
`InnerRunDeps.Spawn` "blocks until it exits": after #018, `loom start` for an llm-driven child now waits for the driver's provider to pass its startup gates (trust dialog dismissed) before returning.
Production files #018 changed: `loomcli/{driverlaunch,driverspec,start}.go`, `shuttleengine/{attach,doc,rundir,run,wait}.go`, `shuttleengine/claudeengine/startup.go`, `webstercli/recoverbatch.go`, `websterengine/{awaitbatch,doc,recoverbatch,runlevel,state,strand}.go`.

## Baseline gates

`go build ./...`, `go vet ./...`, `go test -count=1 ./...` green at `b9ebb1ba9` (92 `ok` packages). Zero stray `lyx`/tmux/reed processes before R1.

## R2 pre-count (at `09544378f`, before R2 spawned)

- `contracts/recipes/loom-recipe.yaml`: 14 rows — Preflight, Loom-Preflight, Discussion-Write, Discussion-Bouncer, Discussion-Burler, Plan-Write, Plan-Bouncer, Plan-Burler, Batchifier, Webster, Webster-Bouncer, Webster-Burler, Publish, Finalize. A focus-1 table with fewer than 14 loom rows is truncated.
- `fabricengine.CommitAnchoredPaths(` production call sites: 10, across `fabriccli/clone.go`, `fabricengine/commitweftpaths.go`, `loomcli/wiring.go`, `loomcli/landingdeps.go`, `loomcli/sharedbootstrap.go`, `battencli/commitstatus.go`, `battencli/wire.go`. Blind spot: commits routed through other fabric helpers (`CommitWeftPaths`, `PushAnchored`, a Bouncer/Burler adapter's own commit decorator) are not counted here.
- Batten rows for focus 2: 4 (`Worktree-Create`, `Seed-Child`, `Run-Shed`, `Worktree-Teardown`).

## R3 pre-count (at `0ffa16b08`, before R3 spawned)

- **Orchestrator-proven wiring gap (focus 3 expected answer, at minimum):** replacing `add.go`'s `return AddResult{}, &ErrBranchExists{Branch: warpBranch}` with an untyped `errors.New` of the same text leaves `go test -count=1` over fabricengine/battencli/battenshed AND `-tags integration` over battencli/fabricengine green. `TestCreateRefusal_LeftoverBranchRemedyNeverNamesCheckout` hand-builds the typed error; `add_branch_exists_test.go` pins only the text. The call site `return createRefusal(err)` in the `CreateWorktree` closure is likewise unpinned by reading (not sabotaged). A round that reports focus 3 clean has not done it.
- **Candidate (PLAUSIBLE, not driven by the orchestrator) for focus 1:** `taskWorktreePresent` stats only `fabricengine.WorktreePath(prime, slug)` (the warp). `Topology.Add`'s rollback is in-process, so a SIGKILL after the warp worktree exists but before the weft worktree / junctions / provenance commit / pushes leaves a half-created pair that the re-entered `Worktree-Create` reports Done over. R2's focus-2 table claimed "kill in (1) → fabric rolls back", which does not hold under SIGKILL. Expected: the create-side mirror of R2-F1 — Seed-Child or Run-Shed then fails against a missing weft/junction, or loom's Publish against an unpushed branch.
- Error pass-through sites for focus 2: the `return err` / `return "", err` lines in `internal/battencli/wire.go` (about 30 by `grep -n 'return .*err'`, many are plain propagation of I/O errors with no remedy text). Remedy-bearing texts known before R3: `taskWorktreeLocation` absent-worktree text, `createRefusal`, the half-torn-pair refusal, `doneSlugRefusal`, `refuseNonPrime`, fabric's `RequireWarpWorktree` refusals, shedrun's `ErrDisagreeingSeed`. Blind spot: remedies worded inside `lyx loom start`'s child output, folded into the spawn error (capped at `maxChildOutputInError`).
- R2 fixes to be sabotage-checked on the production side: `37e4b4788` (F1), `85bf53198` (F2), `eefd0dc1f` (F3, table entry `RefuseSeedAt: battencli.RefuseUnlessPrime` — `TestRecipes_SeedLocationRuleMatchesTheRecipesOwnVerbs` claims to pin it), `d79829eb6` (F4).
