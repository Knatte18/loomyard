# Batch: batten-wiring

```yaml
task: 'Seeded Shed core: run addressing, seed contract, batten'
batch: batten-wiring
number: 6
cards: 6
verify: go build ./... && go test ./internal/battencli/...
depends-on: [1, 2, 3, 5]
```

## Batch Scope

This batch makes batten's own run durable and addressable: its five prime-anchored paths move onto `shedrun`'s run directory, `wire` gains the `Seed-Child` seam group and a non-nil `CommitStatus` with an on-disk no-op-transition skip, `arm` gains the run-id resolution, the `self`-address refusal, the gated auto-seed and the seed-presence refusal, and the module gains a `step` verb with everything that verb's separate `RunE` path needs.
It is one batch because all six changes land in three files — `paths.go`, `wire.go`, `arm.go` — and because each is a precondition of the next: the seam group needs the relocated paths, the auto-seed needs the run-id resolution, and `step` needs both the seam group and the skip to be worth having.

The external interface batch 7 consumes: `battencli.Arm` with today's signature (batch 7 changes it), and a `batten` table entry whose `Verbs` set now includes `step`.

Batch-local decision beyond the overview's: the auto-seed runs **inside `arm`, ahead of `wire`** — never in the `PreRun` hook that seeds the status file.
`battenPreRun` is a `shedverbs.Spec` hook called inside `run`'s `RunE`, which is after `arm` has already called `wire` and built the whole `Env`, so seeding there would leave anything reading the seed at wiring time looking at a seed that does not exist yet on a first `lyx batten run <slug>`.
Belt-and-braces alongside the ordering: every seed-param read stays lazy inside its own closure body, exactly as `wire.go`'s file doc already requires of every path read.

## Cards

### Card 21: relocate batten's paths onto the shed run directory

- **Context:**
  - `internal/shedrun/paths.go`
  - `internal/shedrun/runid.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `internal/battencli/paths.go`
  - `internal/battencli/paths_test.go`
  - `internal/battencli/wire.go`
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Reduce `internal/battencli/paths.go` to a thin forwarding layer over `shedrun`: delete the private `battenDirName` constant and rewrite `BattenDir`, `StatusFile`, `RunLock`, `StatusLock` and `PrimeRunLock` so each returns the corresponding `shedrun` constructor's value for the given run-id — `shedrun.RunDir`, `shedrun.StatusFile`, `shedrun.RunLock`, `shedrun.StatusLock` and `shedrun.PrimeRunLock` respectively.
  `PrimeRunLock` keeps taking no run-id: the lock it names serialises every slug's create and teardown against one another, so it is per-hub rather than per-run, and `battencli` stays its only caller while `shedrun` declares it.
  Rewrite the file's doc comment, whose whole first half asserts this tree is ephemeral and never committed — that is now false of `StatusFile`, which is durable under `_lyx/shed/<run-id>/` while the two locks stay ephemeral under the mirrored `.lyx/shed/<run-id>/`, per the Durable-vs-Ephemeral State Invariant.
  Retarget `wire.go`'s `PrimeRunLock`, `BattenDir` and `StatusFile`/`RunLock`/`StatusLock` call sites, including the `os.MkdirAll(BattenDir(...))` inside the `PrimeLock.Acquire` closure, which must now create the **ephemeral** run directory the lock lives in rather than the durable one — use `shedrun.ScratchDir`.
  In `paths_test.go`, keep every assertion pinning these paths to **prime's** anchor and strengthen none away: this file is one of the Batten Bookend Invariant's two mechanical proxies and must not be weakened while being relocated.
  Add assertions for the new segment and for the durable/ephemeral split.
  Amend `CONSTRAINTS.md`'s `## Batten Bookend Invariant` body, whose "its status file and locks live under prime's own ephemeral tree" clause is the fifth line this task falsifies: the status file is now durable under prime's own anchor and the locks stay ephemeral there, and the invariant's real claim — never under the worktree being managed — is unchanged.
- **Commit:** `refactor(battencli): relocate batten's run paths onto the shed run directory`

### Card 22: the no-op-transition CommitStatus seam

- **Context:**
  - `internal/loomcli/wiring.go`
  - `internal/shedengine/run.go`
  - `internal/shedrun/paths.go`
  - `internal/fabricengine/commitweftpaths.go`
  - `internal/fabricengine/pushanchored.go`
  - `internal/battenrecipe/names.go`
  - `internal/loomcli/wiring_commitstatus_test.go`
- **Edits:** none
- **Creates:**
  - `internal/battencli/commitstatus.go`
  - `internal/battencli/commitstatus_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Build batten's `CommitStatus` seam in a new file, modelled on `internal/loomcli/wiring.go`'s `newCommitStatusSeam`/`loomCommitStatusDeps` pair, whose three-case error handling is not trivial and should be followed rather than reinvented.
  The commit is `fabricengine.CommitAnchoredPaths` scoped to `[]string{shedrun.StatusRel(runID)}` through a positive-only pathspec, then `fabricengine.PushAnchored`.
  It is **not** `fabricengine.Bolt`: `Bolt.Commit` stages every change in its repo, which the Fabric Git Invariant forbids a weft-commit caller, and `Bolt` is the Board's own carve-out scoped to the Board directory.
  Batten does **not** take the Board's push lock; a push rejected because a Board write advanced the branch takes the loom seam's existing disposition — warn and let the next transition catch the branch up — which `newCommitStatusSeam` already applies to every push error including `gitrepo.ErrPushRejected`.
  Wrap the whole thing in a **no-op-transition skip**: when the incoming `(producer, state)` pair equals the last pair committed, return nil without committing or pushing.
  The skip's memory lives **on disk**, at `shedrun.LastCommitMarker(location, runID)`, read before deciding and rewritten after a successful commit — not in a closure variable.
  Each `lyx batten step` is a fresh process, so an in-memory variable would start empty every time and the skip would work for `run` mode alone, leaving step mode — the mode the whole re-entrancy decision exists to enable — committing and pushing once per step.
  A missing or corrupt marker falls back to committing once rather than erroring: it is a cache, and losing it costs one redundant commit, never correctness.
  In `commitstatus_test.go`, drive the seam from stub closures with no hub fixture and no git spawn, exactly as `internal/loomcli/wiring_commitstatus_test.go` does.
  Cover: a repeated `(producer, state)` pair committing and pushing **once**, not twice; the skip still holding when the seam is **rebuilt from scratch between calls**, which is the one assertion that actually proves the on-disk marker rather than an in-closure variable — a test exercising only one long-lived seam instance would pass against the broken in-memory design and proves nothing on its own; a changed pair committing again; and a missing and a corrupt marker each falling back to one commit.
- **Commit:** `feat(battencli): add batten's CommitStatus seam with an on-disk no-op-transition skip`

### Card 23: wire the Seed-Child seam group and the CommitStatus seam

- **Context:**
  - `internal/battenshed/deps.go`
  - `internal/battenshed/seamchild.go`
  - `internal/shedrun/seed.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/boardengine/board.go`
  - `internal/boardengine/task.go`
  - `internal/boardengine/config.go`
  - `internal/battencli/commitstatus.go`
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `internal/battencli/wire.go`
  - `internal/battencli/wire_test.go`
  - `internal/battencli/run_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `run_test.go`'s `newFakeReceiver` builds its `shedrecipe.Env` by hand, bypassing
  `wire` entirely, and batch 5 already registered the `Seed-Child` row in the batten recipe; without
  a fake `Env.SeedChild` group the registry's own non-nil-field validation refuses every `run_test.go`
  case before the verb body runs, which is what surfaces here rather than in `wire_test.go`. Fill
  `newFakeReceiver`'s `Env.SeedChild` with five trivial no-op fakes (each returning a zero value and
  a nil error) so every existing `run_test.go` case keeps exercising the same run-verb disposition it
  already covers, with no behavioural change to what any of those cases asserts.
  Add a fifth seam group to the `shedrecipe.Env` literal in `wire`, filling `SeedChild` with the five closures `battenshed.SeedChildDeps` declares.
  `ReadBoardType` opens the Board over `fabricengine.BoardDir(location.HubPath)` and returns `task.Type` from `Board.GetTask(slug)`, read fresh on every `Call` inside the closure body rather than captured at wiring time — a `type` corrected after prime was seeded must still be honoured.
  `ChildDriver` reads `child_driver` from prime's own seed via `shedrun.ReadSeed(location, slug)`, defaulting to `shedrun.DriverGo` when the param is absent.
  `WriteSeed` resolves the child worktree's `*lyxcwd.Location` through the existing `taskWorktreeLocation(location, slug)` helper, validates the recipe name with `shedrun.ValidateRecipe` — returning its error, which the producer maps to `Stuck` — and calls `shedrun.WriteSeed(childLocation, shedrun.SelfRunID, …)`.
  This closure is the only place in the batten path that encodes a seed, per the overview's `seed-encoding-stays-behind-a-seam-in-battenshed` Shared Decision.
  `CommitSeed` calls `fabricengine.CommitAnchoredPaths` against the **child's** location with `[]string{shedrun.SeedRel(shedrun.SelfRunID)}`; `PushSeed` calls `fabricengine.PushAnchored` against that same child location.
  Note the two distinct commits this module now performs, so they are not conflated: this seam group's commit is a one-off write of the **child's** seed onto the **child's** pair, while card 22's `CommitStatus` commits **prime's own** batten status onto **prime's** pair on every non-no-op transition — different file, different pair, different frequency.
  Every one of these five closures resolves its paths lazily inside its own body, never at wiring time, exactly as the four existing seam groups do and as this file's doc comment requires.
  Replace `ShedPaths`' `CommitStatus: nil` with the seam card 22 builds, and delete the comment claiming nil is right for this package's per-machine, never-committed state.
  Also retarget `InnerRun.ResolveStatus`, which today returns `loomengine.LoomStatusFile(taskLocation)`/`LoomStatusLock(taskLocation)`, to `shedrun.StatusFile(taskLocation, shedrun.SelfRunID)`/`shedrun.StatusLock(taskLocation, shedrun.SelfRunID)` — batch 4 deleted those two `loomengine` functions, so this call site does not compile otherwise.
  Extend `wire_test.go` to assert the five new closures are non-nil and that `CommitStatus` is non-nil now that the status is durable.
- **Commit:** `feat(battencli): wire the Seed-Child seam group and a non-nil CommitStatus`

### Card 24: run-id resolution, the self refusal, and the gated auto-seed

- **Context:**
  - `internal/shedrun/seed.go`
  - `internal/shedrun/runid.go`
  - `internal/battencli/refusal.go`
  - `internal/battencli/wire.go`
  - `internal/shedverbs/step.go`
- **Edits:**
  - `internal/battencli/arm.go`
- **Creates:**
  - `internal/battencli/arm_seed_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Restructure `battenCLI.arm` so its order is exactly: `lyxcwd.Resolve` → the existing non-prime refusal → run-id resolution from `args` → a `self`-address refusal → the verb branch → `wire`.
  **Batten refuses a `self` address outright**, before the auto-seed gate, naming the missing slug rather than listing run-ids: prime hosts many slug-addressed batten runs, so `self` — "this worktree's own primary run" — has no meaning there, and the operator's mistake is an omitted argument rather than a wrong one.
  Without this refusal an argument-less `lyx batten run` would take the `self` default, fall through the gate, and write prime a `_lyx/shed/self/seed.json` with an empty `params.slug`.
  `shedrun.IsReserved` does not catch it, since that check is scoped to run-ids derived from a Board slug and this one comes from the positional's default; consult `IsReserved` separately where the run-id **is** a Board slug, refusing with a message naming the reservation.
  The verb branch: `run` and `step` auto-seed when no seed is present; `status` and `pause` require one and refuse via `shedrun.MissingSeedMessage`, naming `lyx batten run <slug>` as the remedy.
  Read-only verbs must not create state as a side effect of being asked a question — the same reasoning `EnsureStatusLockDir: false` already encodes for batten's status verb.
  The auto-seeded value is `shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: <--driver>, Params: {"slug": slug, "child_driver": <--child-driver>}}`.
  **Prime's seed carries `recipe: "batten"`, never the Board task's `type`**: the two are different runs' recipes, and a prime seed carrying `loom` would make `lyx shed status <slug>` from prime arm `loomcli` against prime — the wrong recipe against the wrong worktree.
  The seed write happens inside `arm`, ahead of `wire`, per this batch's scope note.
  Keep the seed-presence refusal free of any `kind` field, so the five-value `step` vocabulary stays closed.
  In `arm_seed_test.go` cover: a first `lyx batten run <slug>` and a first `lyx batten step <slug>` each seeding before `wire` builds the `Env`; `status` and `pause` against an unseeded run-id seeding nothing and refusing with the listing; the argument-less `self` address refusing by name before the gate; `IsReserved` firing for a Board slug of `self`; and the two absences staying distinct — no seed giving the listing refusal, seed present with no `status.json` giving batten's `found: false`.
- **Commit:** `feat(battencli): resolve the run-id, refuse a self address, and auto-seed run and step`

### Card 25: batten's step verb

- **Context:**
  - `internal/loomcli/arm.go`
  - `internal/shedverbs/step.go`
  - `internal/shedverbs/spec.go`
  - `internal/shedverbs/verbs.go`
  - `internal/lock/lock.go`
  - `internal/battenrecipe/battenrecipe.go`
  - `internal/state/state.go`
- **Edits:**
  - `internal/battencli/arm.go`
  - `internal/battencli/cli.go`
- **Creates:**
  - `internal/battencli/step_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Give batten a `step` verb.
  The table entry alone is not enough, because `step`'s `RunE` shares none of `run`'s path — four things are wired with it.
  First, a `battenPreStep` hook on the spec's `Hooks.PreStep`, modelled on `loomPreStep`: a run-lock probe first, then the same work `battenPreRun` performs — decode the status, refuse a `done` slug, resume silently over every other state, seed the status when absent.
  Without it, `stepLocked`'s read gate hits an absent status and hard-errors, which `shedverbs/step.go` reports as `kind: "producer"` — the one kind `ly-drive` retries, so the failure would loop.
  Second, its refusal-kind mapping, staying inside the closed five: run lock already held is `shedverbs.KindBusy`; a status decode failure or a failed status seed is `shedverbs.KindUnseeded`; any other pre-producer failure is `shedverbs.KindBootstrap`.
  `KindOwnership` is not used — it exists for loom's seeded-status-belongs-to-another-slug check, which has no batten analogue — and `KindProducer` is `shed.Step`'s to emit, never the hook's.
  Third, a `BuildShed` arm for `verb == "step"` in `specFor`, which today fills it for `"run"` only: batten's step arm is plain `battenrecipe.New(c.env, c.shedPaths)`, the same constructor `run` uses, with no `buildLoomShed`-style split, because that split exists in loom solely to avoid opening the fabric and reading origin twice and batten's `wire` does no such work.
  Fourth, a `Step` entry in `battenVerbTexts` with a non-empty `Short` as the CLI/Cobra Invariant requires, plus the told `StepBusyMessage` that travels beside `StepBusyKind`, both of which are empty strings today.
  Register the `step` verb on the cobra tree in `cli.go` alongside the other three.
  Add `--driver` and `--child-driver` flags to `run` and `step`, each defaulting to `go` and each refusing `llm` through `shedrun.ValidateDriver`, supplying card 24's auto-seed with both values; the flags exist now so the next roadmap item changes a default rather than a surface.
  In `step_test.go` cover the whole new path end to end, since none of `run`'s coverage touches it: `battenPreStep` seeding an absent status before `shed.Step` reads it, each of the three refusal-kind mappings, and `BuildShed` being non-nil for `verb == "step"`.
  Assert explicitly that a `step` against a fresh slug never surfaces `kind: "producer"` — conflating the unseeded and producer cases is the likeliest regression here, since both read as "nothing here yet".
- **Commit:** `feat(battencli): add the step verb with its PreStep hook, kind mapping and BuildShed arm`

### Card 26: relax batten's verb arity to an optional run-id

- **Context:**
  - `internal/battencli/arm.go`
  - `internal/loomcli/cli.go`
- **Edits:**
  - `internal/battencli/cli.go`
  - `internal/battencli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Relax `run`, `status` and `pause` from `cobra.ExactArgs(1)` to `cobra.MaximumNArgs(1)` in `internal/battencli/cli.go`, and set the same on the `step` verb card 25 registered.
  All three surfaces — `lyx shed`, `lyx batten`, `lyx loom` — land on this one arity contract, so there is no rule to remember about which subtree accepts a run-id.
  The visible consequence is deliberate: `lyx batten run` with no argument stops being an arity error and becomes a `self` address, which card 24's refusal rejects by name with a better message than cobra's count error.
  Update each verb's `Use` string from `<slug>` to `[<run-id>]` and its `Long` text accordingly, keeping every `Short` non-empty.
  In `cli_test.go` assert each of the four verbs carries `MaximumNArgs(1)` independently — not by reading a shared value — refusing two positionals and accepting both zero and one, with the zero case reaching card 24's named refusal rather than an arity error.
- **Commit:** `refactor(battencli): relax the verb arity to an optional run-id positional`

## Batch Tests

`verify: go build ./... && go test ./internal/battencli/...` runs the whole `battencli` package against a whole-module build.
The build half catches the consumers card 23's `Env` field and card 25's spec changes reach outside this package, and it is the only thing that would surface a `loomengine.LoomStatusFile` call site batch 4 deleted and card 23 missed.

The untagged tests here are Tier 1 and must stay so: `specFor` is called directly against a hand-populated receiver, and card 22's seam is driven from stub closures with no hub fixture and no git spawn, following `internal/loomcli/wiring_commitstatus_test.go`'s own shape.
`internal/battencli/lifecycle_integration_test.go` is `integration`-tagged and does not run under this verify; batch 8 exercises it against the finished shape.

Three assertions in this batch carry disproportionate weight and are named so a reviewer can find them.
In `commitstatus_test.go`, the skip surviving a seam **rebuilt from scratch between calls** is the only test that distinguishes the on-disk marker from the broken in-closure design.
In `arm_seed_test.go`, `status` and `pause` writing nothing while `run` and `step` seed is the read-only-verbs-create-no-state rule, easy to lose in a refactor of `arm`.
In `step_test.go`, a fresh-slug `step` never surfacing `kind: "producer"` is what keeps `ly-drive` from retrying an unseeded status in a loop.

`paths_test.go` continues to serve as the Batten Bookend Invariant's mechanical proxy and is in scope on every run of this verify.
