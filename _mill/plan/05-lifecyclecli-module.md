# Batch: lifecyclecli module

```yaml
task: "Worktree spawn/teardown as Shed producers"
batch: "lifecyclecli module"
number: 5
cards: 8
verify: go test ./internal/lifecyclecli/... && go test -tags integration ./internal/lifecyclecli/...
depends-on: [3, 4]
```

## Batch Scope

This batch delivers `internal/lifecyclecli`: the tier that resolves geometry, fills every seam the three producers read, and exposes `lyx lifecycle run <slug>` and `lyx lifecycle status <slug>`.
It is the only layer in this task that may import `internal/lyxcwd`, `internal/fabricengine` and `internal/reedengine`, and the only one that knows the managed task worktree's path exists at all.

It is one batch because the path constructors, the wiring closures, the two verbs and the non-prime refusal are a single seam surface: every closure resolves against the same prime `*lyxcwd.Location` the pre-run stores, and neither verb is testable without the wiring or the refusal.

It depends on batch 3 for `lifecyclerecipe.New` and on batch 4 for the `--no-attach` flag the spawn closure passes.
Batch 6 consumes `lifecyclecli.Command()` and the five exported path constructors.

Batch-local decision: every seam that touches the managed task worktree resolves inside its own closure body on `Call`, never at wiring time, because at wiring time that worktree does not exist.
That applies to the status-path pair, the spawn directory, and both teardown halves alike — not only to the one whose laziness is visible in its signature.

## Cards

### Card 21: the path constructors

- **Context:**
  - `internal/loomengine/config.go`
  - `internal/lyxdirs/dirs.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/fabricengine/junction.go`
- **Edits:** none
- **Creates:**
  - `internal/lifecyclecli/paths.go`
  - `internal/lifecyclecli/paths_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Declare five exported path constructors, each a plain `filepath.Join` onto the driving worktree's `AnchorPath()` and none of them calling `os.Getwd` or any git command, per the Cwd Resolution Invariant.
  `LifecycleDir(l *lyxcwd.Location, slug string) string` joins `l.AnchorPath()`, `lyxdirs.DotLyxDirName`, a package-local `lifecycleDirName` constant, and `slug`.
  `StatusFile`, `RunLock` and `StatusLock` each take the same two arguments and join `status.json`, `run.lock` and `status.json.lock` respectively onto `LifecycleDir`.
  `PrimeRunLock(l *lyxcwd.Location) string` takes no slug and joins `run.lock` one level above the per-slug directory, onto `l.AnchorPath()`, `lyxdirs.DotLyxDirName` and `lifecycleDirName`.
  The `.lyx` segment must come from `lyxdirs.DotLyxDirName` rather than a literal, per the Lyxdirs Single-Declarer Invariant, exactly as `loomengine.LoomStatusLock` already does.
  A file comment must record why this whole tree is ephemeral rather than durable, the opposite of loom's own status file: the lifecycle's state is per-machine and per-attempt and is never committed, so the Durable-vs-Ephemeral State Invariant puts it under `.lyx` at the mirrored subpath.
  `paths_test.go` asserts the layout for two synthetic `*lyxcwd.Location` fixtures, one with `AnchorRel` equal to `"."` and one anchored at a subdirectory, and asserts the three per-slug paths are pairwise distinct — `RunLock` differing from `StatusLock` in particular, which `shedengine.Shed`'s own validation rejects and which must not first surface at runtime.
  It also asserts every returned path is under the given `Location`'s own anchor and that none of them contains the managed slug's worktree path, which is this package's half of the Lifecycle Bookend Invariant's mechanical proxy.
- **Commit:** `feat(lifecyclecli): add the prime-anchored lifecycle path constructors`

### Card 22: the non-prime refusal decision

- **Context:**
  - `internal/fabricengine/worktreelist.go`
  - `internal/fabricengine/remove.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:** none
- **Creates:**
  - `internal/lifecyclecli/refusal.go`
  - `internal/lifecyclecli/refusal_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Declare `func refuseNonPrime(worktreeName, primeName string, primeNameErr error) error`, a pure function returning nil only when `primeNameErr` is nil and `worktreeName` equals `primeName`.
  When `primeNameErr` is non-nil it returns a refusal naming that error, never nil and never a hard error: an unresolvable prime name means the hub geometry is already broken, and treating it as "probably fine, proceed" would drive the two bookend rows from an unverified vantage point, which is what this check exists to prevent.
  When the two names differ it returns a refusal naming both, stating the verb runs from the hub's prime worktree only, and telling the operator to re-run it there.
  The doc comment records that the refusal is added rather than inherited: the topology layer's own self-protection compares a named slug against the prime name and does not refuse removing the worktree the caller is standing in, so without this check a run driven from a task worktree would delete its own process working directory.
  It also records, so a reader does not read it as an inconsistency, that `internal/fabricengine`'s own prime-slug refusal deliberately treats the same name-resolution failure as non-fatal because there it weakens one guard among several that still refuse, whereas here it is the whole check.
  `refusal_test.go` is a Tier-1 table covering all three outcomes, including the `primeNameErr` case, and asserting the two refusal messages are distinguishable from one another.
  This split exists so the refusal's decision is unit-testable while only its git-backed inputs need the integration tier.
- **Commit:** `feat(lifecyclecli): add the pure non-prime refusal decision`

### Card 23: the wiring seams

- **Context:**
  - `internal/loomcli/wiring.go`
  - `internal/loomcli/landingdeps.go`
  - `internal/fabricengine/add.go`
  - `internal/fabricengine/remove.go`
  - `internal/fabricengine/topology.go`
  - `internal/fabricengine/junction.go`
  - `internal/fabriccli/fabric.go`
  - `internal/reedcli/cli.go`
  - `internal/reedengine/lifecycle.go`
  - `internal/hubgeom/hubgeom.go`
  - `internal/loomengine/config.go`
  - `internal/lock/lock.go`
  - `internal/state/state.go`
  - `internal/lifecycleshed/deps.go`
  - `internal/lifecyclecli/paths.go`
  - `internal/shedrecipe/recipe.go`
  - `internal/lifecyclerecipe/lifecyclerecipe.go`
- **Edits:** none
- **Creates:**
  - `internal/lifecyclecli/wire.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Declare a `wire(location *lyxcwd.Location, slug string) error` method on the package's CLI receiver that builds and stores the `shedrecipe.Env` and `lifecyclerecipe.ShedPaths` the two verbs need, plus the receiver field the abandoned-session value is recorded into.
  `ShedPaths` is filled from card 21's constructors over the prime `Location`, with `CommitStatus` left nil — nil is the documented absent value meaning "commit nothing", which is exactly right for per-machine state, so this needs no new mechanism.
  `Env.Slug` is the slug, `Env.ScratchDir` is `LifecycleDir`, and `Env.PrimeLock` carries `PrimeRunLock` as its `Path` and an acquire seam that creates the lock's parent directory, calls the non-blocking try-acquire, and returns the release closure, the acquired bool, and the error unchanged.
  `Env.CreateWorktree` loads the topology config the way the topology verbs already do, constructs the topology holder, calls its add operation against the prime `Location` and the slug with a zero-valued options struct, logs the returned result's mutation record at `Info` through `internal/logger`, and returns the error.
  `Env.Teardown.Shutdown` resolves lazily, inside its own body: the task worktree root from the topology package's own sibling-path helper, then that root's `*lyxcwd.Location` through `lyxcwd.ResolveWorktree`, then the reed config through `reedengine.LoadConfig(location.AnchorPath(), "reed")` and the geometry through `hubgeom.ReedGeometry(location)` — both are required, since the reed constructor takes the pair and every shipped caller loads the config this way — then constructs the engine and calls its down operation, returning that result's abandoned-session field and the error.
  `Env.Teardown.Remove` calls the topology holder's remove operation against the prime `Location` and the slug with force false, never true, logs the returned result's mutation record at `Info`, and returns the error.
  `Env.LoomRun.ResolveStatus` resolves the same task-worktree `Location` lazily and returns `loomengine.LoomStatusFile(l)` and `loomengine.LoomStatusLock(l)` — routed through those accessors rather than joined here, which is what keeps this package clear of re-deriving loom's own layout.
  `Env.LoomRun.Spawn` resolves the same `Location` lazily, resolves the binary through `os.Executable()`, builds an `exec.Command` running the `loom run --no-attach` verb with its `Dir` set to that `Location`'s `AnchorPath()` — not its worktree root, since the resolver gates a child's working directory to the anchor and a bare root fails on any subpath-anchored hub — logs the spawn at `Info` and the completed wait at `Info`, and runs it in the foreground, waiting for it.
  `Env.LoomRun.ReadStatus` reads through `state.ReadJSONStrict` over `shedengine.Status`, taking loom's own status lock, and returns its three values unchanged.
  `Env.LoomRun.Now` and `Env.LoomRun.Sleep` are left nil so the production clock and sleep are selected.
  Do not resolve the task worktree at wiring time anywhere in this file: it does not exist until the first row has run.
- **Commit:** `feat(lifecyclecli): wire every lifecycle seam, resolving the task worktree lazily`

### Card 24: the cobra seam

- **Context:**
  - `internal/loomcli/cli.go`
  - `internal/reedcli/cli.go`
  - `internal/clihelp/exec.go`
  - `internal/output/output.go`
  - `internal/lifecyclecli/wire.go`
  - `internal/lifecyclecli/refusal.go`
  - `internal/lifecyclecli/paths.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/lifecyclecli/cli.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Declare `package lifecyclecli` with a package doc comment stating: it is the module that drives one task worktree's whole lifecycle as a single Shed run from the hub's prime worktree; it imports `internal/lifecycleshed` and `internal/lifecyclerecipe`, neither of which imports cobra; and it is outside the Fabric Vocabulary Invariant's owner set, so no identifier, literal or comment here may name either side of the pair.
  Declare an unexported `lifecycleCLI` receiver carrying the resolved prime `*lyxcwd.Location`, the assembled `shedrecipe.Env`, the `lifecyclerecipe.ShedPaths`, the slug, and the recorded abandoned-session string.
  Declare `Command() *cobra.Command` building a `lifecycle` parent with a non-empty `Short`, a `Long` naming both verbs and the prime-worktree requirement, `RunE` set to `clihelp.GroupRunE` so a bare invocation lists subcommands and an unknown subcommand emits a JSON envelope, and a `PersistentPreRunE` that short-circuits when the group command itself is invoked, then reads cwd through `lyxcwd.CwdFrom`, resolves it through `lyxcwd.Resolve`, resolves the prime name through `fabricengine.PrimeName`, applies card 22's refusal, and on a refusal writes it through `internal/output` and aborts through `clihelp.Abort` without resolving anything further.
  It then reads the slug from the command's own arguments and calls `wire`.
  Declare `RunCLI(out io.Writer, args []string) int` and `RunCLIIn(cwd string, out io.Writer, args []string) int` in exactly the shape `internal/loomcli/cli.go` already uses, including the empty-cwd branch, since the resolver's context helper panics on an empty directory.
  `RunCLIIn` is carried rather than skipped for a concrete reason to state in its doc comment: the path-derivation tests serving as the Lifecycle Bookend Invariant's mechanical proxy and the non-prime refusal test both need an injectable cwd, and neither is reachable through `RunCLI` alone.
  Register both verb commands on the parent.
- **Commit:** `feat(lifecyclecli): add the cobra seam and the prime-worktree pre-run refusal`

### Card 25: the run verb

- **Context:**
  - `internal/lifecyclecli/cli.go`
  - `internal/lifecyclecli/wire.go`
  - `internal/lifecyclecli/paths.go`
  - `internal/lifecyclerecipe/lifecyclerecipe.go`
  - `internal/shedengine/shed.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/run.go`
  - `internal/state/state.go`
  - `internal/lock/lock.go`
  - `internal/loomcli/drive.go`
  - `internal/output/output.go`
  - `internal/clihelp/exec.go`
- **Edits:** none
- **Creates:**
  - `internal/lifecyclecli/run.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Declare a `runCmd()` method returning the `run <slug>` subcommand with a non-empty `Short`, exactly one positional argument, and a `RunE` that checks `clihelp.ShouldAbort` first.
  The verb reads the persisted lifecycle status through `state.ReadJSONStrict` over `shedengine.Status` and branches per `shedengine.State` value rather than on the word "terminal", which means something different in the poll producer's own vocabulary.
  `StateRunning`, `StateBlocked`, `StateFailed` and `StatePaused` each resume silently from the persisted current producer with no re-seed, no prompt and no flag — `StateBlocked` is the everyday path, since every operator-fixable refusal in this task lands there, and the engine itself already resumes from blocked and failed, so this is the verb matching engine behaviour rather than adding any.
  `StateDone` refuses on the envelope, naming the slug as already completed and naming the per-slug directory `LifecycleDir` returns as the directory to delete to run the slug again; it must not silently re-run, because the per-slug state lives on prime and survives the teardown that removed the worktree, so a completed slug's status file is the normal steady state.
  An absent status file is a fresh start.
  The verb then builds the Shed through `lifecyclerecipe.New` over the receiver's stored `Env` and `ShedPaths`, creates the status file's parent directory, and calls the Shed's own `Run` in the foreground.
  A run-lock contention refusal is reported on the envelope naming the lock path and never waits and never spawns a second driver: the engine already takes that lock non-blocking for the whole of one run, so refusing is its native behaviour and the verb's only job is to report it legibly rather than surfacing a raw lock error.
  There is no bootstrap handshake here and no detached child, so the loom bootstrap's own await machinery has no counterpart and must not be copied.
  On return the verb emits a success envelope carrying the run's outcome, the halted producer and the reason, and — when the teardown closure recorded a non-empty value — the key `abandonedSession`, spelled exactly as the reed module's own up verb spells it.
  The envelope deliberately carries neither a `mutations` array nor a `partial` bool: a run may perform zero, one or two topology mutations at arbitrary points hours apart, so there is no coherent single array at run scope and `partial` has no referent there; the records are logged at `Info` by the wiring closures instead of discarded.
  State that reasoning in a comment on the envelope construction, since it resolves a conditional the Mutation Record Invariant otherwise leaves open for this verb.
- **Commit:** `feat(lifecyclecli): add the run verb with its resume and refusal dispositions`

### Card 26: the status verb

- **Context:**
  - `internal/lifecyclecli/cli.go`
  - `internal/lifecyclecli/paths.go`
  - `internal/loomcli/status.go`
  - `internal/shedengine/status.go`
  - `internal/state/state.go`
  - `internal/output/output.go`
  - `internal/clihelp/exec.go`
- **Edits:** none
- **Creates:**
  - `internal/lifecyclecli/status.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Declare a `statusCmd()` method returning the `status <slug>` subcommand with a non-empty `Short`, exactly one positional argument, and a `RunE` that checks `clihelp.ShouldAbort` first.
  It reads the same status file through `state.ReadJSONStrict` over `shedengine.Status` and emits the current producer, the state, the error field, the activity and the history on a success envelope, plus the resolved status path so an operator can find the file.
  An absent status file is reported as a determined answer on the success envelope — the slug has not been run on this machine — not as an error, since nothing failed.
  A decode failure or a status-lock failure is an error envelope.
  The verb runs under the same prime-worktree refusal the parent's pre-run applies, so it is refused from a task worktree exactly as `run` is; state in a comment why a read-only verb is refused too: the path it would read is derived from whichever worktree it is invoked in, so answering from a task worktree would report on a different file and quietly mislead.
- **Commit:** `feat(lifecyclecli): add the status verb`

### Card 27: Tier-1 CLI tests

- **Context:**
  - `internal/lifecyclecli/cli.go`
  - `internal/lifecyclecli/run.go`
  - `internal/lifecyclecli/status.go`
  - `internal/lifecyclecli/wire.go`
  - `internal/lifecyclecli/paths.go`
  - `internal/lifecyclecli/refusal.go`
  - `internal/loomcli/cli_test.go`
  - `internal/shedengine/status.go`
  - `internal/lifecycleshed/deps.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/lifecyclecli/cli_test.go`
  - `internal/lifecyclecli/run_test.go`
  - `internal/lifecyclecli/wire_test.go`
  - `internal/lifecyclecli/testmain_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** `cli_test.go` follows `internal/loomcli/cli_test.go`'s shape: every command in the built tree carries a non-empty `Short`; the registered verb set is exactly `run` and `status`; each verb rejects zero arguments and two arguments; a bare group invocation needs no git repository; and an unknown subcommand emits a JSON error envelope.
  `run_test.go` drives the four run dispositions against a hand-populated receiver and a hand-written status file under `t.TempDir()`, bypassing the wiring entirely: `StateRunning`, `StateBlocked`, `StateFailed` and `StatePaused` each resume from the persisted current producer — `StateBlocked` especially, since it is the state every escalation produces; `StateDone` refuses on the envelope with the per-slug directory named in the message; an absent status file starts fresh; and a held per-slug run lock refuses on the envelope naming the lock path without waiting, asserted by taking the lock in the test before invoking the verb.
  It also asserts the `abandonedSession` key reaches the envelope on a `Done` teardown when the recorded value is non-empty, and is absent when it is empty — nothing else would catch that half being silently dropped.
  `wire_test.go` proves no seam is evaluated at wiring time: building the wiring for a slug whose worktree does not exist must succeed.
  Cover all four lazy seams individually — the status-path resolver, the spawn directory, and both teardown halves — not just the first, because eager evaluation is exactly the failure laziness exists to avoid and a test covering only one would let the other three regress silently.
  `testmain_test.go` declares a `TestMain` calling `gitkit.HermeticGitEnv()` before `m.Run()`, per the Hermetic Git Test Environment Invariant, because this package's `integration`-tagged sibling spawns git.
  Every test in these three files stays untagged and Tier 1: none calls the resolver, none runs git, and none spawns a process.
- **Commit:** `test(lifecyclecli): cover the cobra seam, the run dispositions and seam laziness`

### Card 28: the integration end-to-end

- **Context:**
  - `internal/lifecyclecli/cli.go`
  - `internal/lifecyclecli/run.go`
  - `internal/lifecyclecli/wire.go`
  - `internal/lifecyclecli/paths.go`
  - `internal/lifecyclecli/refusal.go`
  - `internal/hubforge/hub.go`
  - `internal/landingshed/finalize_integration_test.go`
  - `internal/landingshed/testmain_integration_test.go`
  - `internal/lifecycleshed/deps.go`
  - `internal/shedengine/status.go`
  - `CONSTRAINTS.md`
- **Edits:** none
- **Creates:**
  - `internal/lifecyclecli/lifecycle_integration_test.go`
  - `internal/lifecyclecli/testmain_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write an `integration`-tagged end-to-end over a real hub built by `internal/hubforge` through its fabric fixture entry point, never hand-assembled, per the hubforge Fabric-Fixture Invariant.
  Stub at the `LoomRunDeps` field level rather than replacing the producer — a no-op spawn and a read-status answering a chosen state — so the real poll logic is exercised rather than bypassed.
  Three cases: a spawn stub plus a read-status answering `StateDone`, asserting the pair exists on disk after the create row and is gone after the teardown row; a read-status answering `StateBlocked`, asserting the run halts blocked with the task worktree still present, which is the safety property the whole design turns on; and a deliberately dirty prime, asserting the create row halts blocked before anything is created, since that refusal fires on every lifecycle run and is invisible to the unit tests' fakes.
  Add a fourth case proving mid-list resume: crash after the create row and resume into the poll row without re-running the create row, driven by writing the persisted current producer and re-invoking the verb.
  Add a fifth and sixth case for the non-prime refusal, one per verb, driven through `RunCLIIn` with an injected cwd pointing at a real task worktree, asserting the envelope refuses and naming both worktree names — this is the runtime check standing in for the Bookend invariant's missing enforcing test, so it is not an afterthought.
  It lives at the integration tier rather than Tier 1 because the prime-name lookup reaches a real git worktree listing and getting there at all needs the resolver, both barred from untagged files by the Test Tier Purity Invariant.
  `testmain_integration_test.go` declares the tagged `TestMain` calling `gitkit.HermeticGitEnv()` before `m.Run()`, in the shape `internal/landingshed/testmain_integration_test.go` already uses.
- **Commit:** `test(lifecyclecli): add the integration end-to-end over a real hub`

## Batch Tests

`verify:` runs the untagged suite and then the `integration`-tagged one over the same package: `go test ./internal/lifecyclecli/... && go test -tags integration ./internal/lifecyclecli/...`.
Both halves are required at this boundary because card 28 creates `integration`-tagged files that the untagged run does not compile, and the safety property card 28's second case asserts — a blocked run leaves the task worktree intact — has no Tier-1 equivalent.
Untagged files covered: `paths_test.go`, `refusal_test.go`, `cli_test.go`, `run_test.go`, `wire_test.go`.
Tagged: `lifecycle_integration_test.go`.
The scope stays on the one new package; nothing outside it is edited in this batch.
