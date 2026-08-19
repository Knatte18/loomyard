# Batch: loomcli-run-bootstrap

```yaml
task: 'loom: session bootstrap'
batch: loomcli-run-bootstrap
number: 5
cards: 5
verify: go test ./internal/loomcli/
depends-on: [1, 2, 3, 4]
```

## Batch Scope

This batch delivers the session bootstrap itself: the `run` verb's four steps, the seed-input resolution and legacy-worktree refusal that precede them, the spawn handshake that makes a concurrent second invocation safe, and the root alias constructor.
It is one batch because the verb is a single composition and every pure piece it composes is testable from the same package suite; splitting it would leave a half-wired verb behind.

Every fallible decision in the verb is factored into a pure function first — the parent-branch resolution, the spawn-or-skip predicate, the handshake poll, the strand lookup, and the two command-string builders — so the verb body is assembly and the judgment is under test without a real lock, a real process, or a real clock.

It depends on batch 4 for the receiver and the wired dependency stacks the verb assembles over, on batch 1 for the provenance record's read and write functions, its anchor-relative form, and the weft-commit helper the seed commit goes through, and on batch 3 for the status file's anchor-relative form, the driver-log and bootstrap-lock accessors, and the seed-exists sentinel the re-entrant seed call keys on.
Every one of those is first reached here — batch 4 touches none of them.
It also depends on batch 2, for one reason only: both batches edit the same design doc's bootstrap section, batch 2 its run-launcher paragraph and this batch its step diagram, so they are serialised rather than left to collide in the same file.

The external interface batch 6 consumes: `loomcli.RunAliasCommand`.

Batch-local decisions beyond `## Shared Decisions` in the overview:

- The bootstrap lock is held across the probe and the spawn AND across the handshake, and released only once the run lock is observed held.
  Holding it merely across the spawn call is not enough: the run lock is taken by the child at the top of the phase machine's run loop, long after the spawn call returns.
- The handshake's two failure exits refuse on the envelope and do not attach, so the operator is left with no driver, no held lock, and a message naming the log — never a silent half-start.

## Cards

### Card 21: seed-input resolution and the legacy-worktree refusal

- **Context:**
  - `internal/fabricengine/origin.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `contracts/specs/loom-status-spec.md`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/seedinput.go`
  - `internal/loomcli/seedinput_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `seedinput.go` add a pure `func resolveParentBranch(recorded fabricengine.Origin, found bool, parentFlag string) (parent string, write bool, err error)` implementing exactly this table, with each row's reason in the doc comment:
  a record present with a non-empty recorded value and no flag returns that value with no write;
  a record present whose recorded value equals the flag returns that value with no write, so re-passing the flag is a no-op;
  a record present whose recorded value differs from a non-empty flag returns an error naming both values, because changing a recorded parent would silently re-target the eventual merge-back and there is deliberately no override flag;
  no record, or a record whose recorded value is empty, plus a non-empty flag returns the flag with a write requested;
  no record, or a record whose recorded value is empty, and no flag returns an error naming the missing record and telling the operator to pass the flag once.
  State in the doc comment that a present-but-empty recorded value is treated exactly as an absent record because the status contract makes the parent mandatory and counts an empty string as absent.
  Add a second pure helper `func seedSlug(worktreeName string) string` returning its argument unchanged, documented as the single place the slug's source is stated — the worktree's own name from the resolved location — so the choice is named rather than inlined at the call site.
  `seedinput_test.go` is an untagged table over `resolveParentBranch` covering all five rows plus the present-but-empty variant of the last two, asserting the returned parent, the write flag, and, for the two error rows, that the message names the values or the flag the operator needs.
- **Commit:** `feat(loomcli): resolve the seed's parent branch from the recorded provenance only`

### Card 22: the bootstrap's pure decisions — spawn, handshake, strand, and the two command strings

- **Context:**
  - `internal/lock/lock.go`
  - `internal/proc/proc_linux.go`
  - `internal/proc/proc_windows.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/headerpane.go`
  - `internal/reedcli/attach.go`
  - `internal/shell/shell.go`
  - `internal/shedengine/run.go`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/bootstrap.go`
  - `internal/loomcli/bootstrap_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `bootstrap.go` declare the fixed status-strand display name as an unexported constant, documented as the strand's stable identity because the reed engine's add operation has no upsert semantics and a second add would append a second pane.
  Add `func mustSpawnDriver(runLockHeld bool) bool` returning the negation, documented as the whole re-entrancy decision: the run lock being held means a driver is alive, so the bootstrap ensures substrate and attaches instead of spawning a second one, and the lock is only ever probed non-blockingly and released immediately, never held by the probe.
  Add a named result type with three values — ready, child-died, and deadline — and `func awaitRunLock(lockHeld func() (bool, error), alive func() bool, wait func(), attempts int) (<result>, error)`, looping at most `attempts` times and, each iteration, first consulting `lockHeld` and returning ready when it reports held, then consulting `alive` and returning child-died when it reports gone, then calling `wait`; returning deadline after the loop.
  The doc comment must state why the order is load-bearing (a child that took the lock and is about to exit must still be reported ready) and why the whole poll takes injected seams (so a test drives it with no real process, no real lock, and no wall-clock sleep, which the Test Tier Purity Invariant's long-sleep guard would otherwise flag).
  Add `func findStatusStrand(strands []reedengine.StrandStatus, name string) (reedengine.StrandStatus, bool)` returning the first exact name match.
  Add `func statusStrandCmd(sh shell.Shell, exe string) string` composing the pane's command line through the shell seam exactly as the reed header pane's own builder does, invoking the executable with the two-word status verb and the watch flag, and `func attachArgv(socket, session string) []string` returning the tmux argv for an in-place attach in the same shape `internal/reedcli` uses.
  Both builders are pure and take the executable path and socket as arguments so the real lookup happens only at the call site.
  `bootstrap_test.go` is untagged and covers: the spawn predicate both ways; the poll returning ready when the lock seam flips held on a later iteration, child-died when the alive seam goes false first, deadline when neither happens, and the error path when the lock seam itself errors; the strand lookup hitting, missing, and picking the exact name over a prefix; and both command builders against a fake executable path for both shell flavours.
  Use a counting wait seam rather than a real sleep.
- **Commit:** `feat(loomcli): add the bootstrap's pure spawn, handshake, and strand decisions`

### Card 23: the run verb composes the four bootstrap steps

- **Context:**
  - `internal/loomcli/cli.go`
  - `internal/loomcli/wiring.go`
  - `internal/loomcli/bootstrap.go`
  - `internal/loomcli/seedinput.go`
  - `internal/loomshed/seed.go`
  - `internal/loomengine/config.go`
  - `internal/fabricengine/origin.go`
  - `internal/fabricengine/commitweftpaths.go`
  - `internal/fabricengine/fabric.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/mutation.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/reedengine/strand.go`
  - `internal/reedengine/render/types.go`
  - `internal/lock/lock.go`
  - `internal/proc/proc_linux.go`
  - `internal/shell/shell.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/loomcli/cli.go`
  - `manifest/designs/loom.md`
- **Creates:**
  - `internal/loomcli/run.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace the placeholder `runCmd` from batch 4 with a real `func (c *loomCLI) runCmd() *cobra.Command` in the new file, removing the placeholder from `cli.go` so exactly one definition survives.
  Give it a non-empty `Short` and a `Long` describing the four steps and stating that the terminal is handed to tmux at the end, that a second invocation while a driver is alive ensures substrate and attaches rather than spawning a second driver, and that the detached driver's output goes to the log the ephemeral-tree accessor names.
  Register a `--parent` string flag whose help text says it writes the pair's provenance record once for a worktree created before that record existed and is refused when it disagrees with an already-recorded value.
  The `RunE` checks `clihelp.ShouldAbort` first and then runs this sequence, refusing on one `output.Err` envelope at the first failure of any step and performing no attach in that case:
  (1) read the recorded provenance through `fabricengine.ReadOrigin`, feed it and the flag to `resolveParentBranch`, and when a write is requested call `fabricengine.WriteOrigin` with a throwaway recorder that is then discarded — add a comment stating loom's envelope deliberately gains no mutation keys, because the Mutation Record Invariant binds fabric verb outcomes and loom's result is not one;
  (2) call `loomshed.Seed` with the two `loomengine` path accessors, the slug from `seedSlug` over the location's worktree name, and the resolved parent, treating exactly the seed-exists sentinel as success via `errors.Is` and propagating every other error — add a comment that the sentinel check is what makes a re-run work and that a stat-then-seed probe would reintroduce the race the seeder's single lock exists to close;
  (3) commit the seed, and the provenance record when step 1 wrote one, weft-side through `fabricengine.CommitWeftPaths` against the weft sibling path accessor with the location's anchor-relative field, a path list built from `loomengine.LoomStatusRel` plus `fabricengine.OriginRecordRel`, a message naming the slug, and the environment-derived sync options — add a comment that this commit must precede the driver spawn because the cleanliness check scans the weft including untracked files and the status file is not on the never-tracked exclude list, so an uncommitted seed would make the phase machine's very first precondition row fail on loom's own write;
  (4) create the bootstrap lock's parent directory and take that lock blockingly, then bring the reed substrate up, then look the status strand up by its fixed name in the reed status result and add it only when absent, with the below-parent anchor and the shrink-when-waiting-on-child display flag, and its command line from `statusStrandCmd` over the resolved executable;
  (5) probe the run lock non-blockingly, releasing it immediately when it was free, feed the observation to `mustSpawnDriver`, and when a spawn is required start a detached child running the two-word drive verb with its standard output and standard error both pointed at an appended, created-on-demand file at the driver-log accessor's path, never inheriting the parent's handles;
  (6) still holding the bootstrap lock, run `awaitRunLock` with a real lock probe, a real liveness check on the child's process id, a short bounded wait, and an attempt count that gives a generous but finite deadline, and on either failure result release the bootstrap lock, refuse on the envelope naming the driver log as where the reason is, and return without attaching;
  (7) release the bootstrap lock, then pre-flight the reed status once on the envelope, then hand stdio to a tmux attach child built from `attachArgv` over the engine's socket and session, propagating the child's exit code and writing no further JSON.
  Add a comment at step 7 recording that this tail is the CLI/Cobra Invariant's interactive-handoff exception, and that steps 1 through 6 are pre-flight precisely so every fallible thing is reported before stdio is gone.
  Every step that starts a real process must log its spawn through the shared logger, per the Live-Substrate Spawn Observability invariant.
  Do not create the driver log through any path other than the accessor's, and construct no path inline.
  In the same commit, rewrite the fenced step diagram in the "Entry point — the session bootstrap" section of the design doc so it matches the sequence this card ships.
  That diagram today lists exactly four steps and mentions neither seeding, nor the weft-side commit, nor the recorded provenance;
  it must gain the three pre-steps ahead of them — resolve the recorded parent branch, seed the status file when absent, and commit that seed weft-side before the spawn — and its spawn step must gain the handshake, so a reader is not left believing the spawner returns the moment the child starts.
  Keep the diagram's existing four steps and their parenthetical annotations intact, and keep the prose beneath it about loom going to the background and the terminal belonging to tmux, which this card does not change.
  Do not touch the run-launcher paragraph in that same section; batch 2 owns it.
  Write in this repo's markdown style: one sentence per line, no fixed-column hard wrapping.
- **Commit:** `feat(loomcli): add the run verb, the session bootstrap`

### Card 24: the root run alias

- **Context:**
  - `internal/loomcli/cli.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/loomcli/run.go`
  - `internal/loomcli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func RunAliasCommand() *cobra.Command` to the run verb's own file, building a fresh receiver, taking its `runCmd` unchanged, attaching that receiver's `resolvePersistentPreRun` as the returned command's own `PersistentPreRunE`, and returning it.
  Its doc comment must state that this is the alias registered as a second root child, that it deliberately carries no seam functions of its own because it delegates into the subtree's verb, and that the pre-run is attached here because a root child gets no parent group's pre-run to inherit — while the group short-circuit inside that pre-run keys on the group's own name and therefore does not fire for this command.
  The alias must not be registered inside `Command()`; the root does that in batch 6.
  Add an assertion in the package's existing tree test that the alias's `Short` is non-empty, that its `Use` is the bare verb, and that it exposes the same flag the subtree's verb does — the guard that the two stay one command rather than drifting into two.
- **Commit:** `feat(loomcli): add the root run alias constructor`

### Card 25: pin the status contract's spawn-time seed role to this command

- **Context:**
  - `internal/loomshed/seed.go`
  - `internal/loomcli/run.go`
- **Edits:**
  - `contracts/specs/loom-status-spec.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** This spec is a pinned Contract doc that today defers which command the spawn-time seeding role binds to, saying it is pinned when that command lands.
  This task is that landing, so bind it: state that the bootstrap verb seeds the file itself when it is absent, and correct every clause that currently says the seed is written before that verb has ever executed — the seed is now written by that verb's own first invocation, and the file is committed weft-side by it before the driver is spawned.
  Update the worked example's introductory sentence that describes the seed as written before the verb ran, and the sentence in the seed section that names the role without a binding, keeping the JSON examples themselves byte-identical since the schema does not change.
  Update the single-writer sentence so its parenthetical names the same seeder and pause verb this task ships rather than an unbound role.
  Correct the doc's opening sentence about which command rewrites the status shell on every step: that loop belongs to the driver verb, which the bootstrap verb spawns detached, not to the bootstrap verb itself — leaving it attributed to the bootstrap verb would ship a false statement about which command owns the phase-machine loop.
  Fix that attribution wherever else in the file the same claim appears, not only at its first occurrence.
  Leave the validation checklist, the parse discipline, and the per-field notes untouched — none of them changes.
  Write in this repo's markdown style: one sentence per line, no fixed-column hard wrapping.
- **Commit:** `docs(loom): pin the status spec's spawn-time seed role to the bootstrap verb`

## Batch Tests

`verify: go test ./internal/loomcli/` runs the one package this batch touches.
Cards 21 and 22 are covered by their own new untagged test files, which are where the batch's real judgment lives — the parent-branch table and the handshake's three exits.
Card 24's alias assertions extend the tree test batch 4 created in the same package.
Card 23's verb body is assembly over those tested pieces and is not unit-testable end to end without a live tmux server and a real detached process; it is covered by batch 7's tagged smoke suite, which is the only place that interaction can be caught.
Card 25 touches only a doc and has no runnable surface.
