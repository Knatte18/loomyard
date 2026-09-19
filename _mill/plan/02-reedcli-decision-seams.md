# Batch: reedcli pure decision seams

```yaml
task: 'reed: per-hub daemon reaps orphaned sessions'
batch: 'reedcli pure decision seams'
number: 2
cards: 5
verify: go test ./internal/reedcli/
depends-on: []
```

## Batch Scope

This batch delivers every pure, testable seam the daemon's reap needs, plus their untagged tests, without wiring any of them into `runWatchdogLoop` — that wiring is batch 3's whole job.
It is one batch because all five cards are declarations in one file (`internal/reedcli/watchdog.go`) plus their tests in one file, and because the seams are only meaningful together: `planReapCycle` consumes the threshold `watchdogTiming` carries, and the two stat predicates produce the `hubLive`/`gone` inputs it is told.
It has no dependency on batch 1 and can run in parallel with it.

The external interface batch 3 consumes: `watchdogDefaultTiming()`, `validateWatchdogFlags`, `hubIsLiveDir`, `worktreeRootGone` and `planReapCycle`.

Batch-local decision beyond `## Shared Decisions`: every function here is written so it compiles and is fully tested while still unreferenced by `runWatchdogLoop`. Go does not reject an unused package-level function, so the batch is independently verifiable.

## Cards

### Card 4: `watchdogOrphanGoneCycles` and the timing seam

- **Context:**
  - `internal/reedengine/watchloop.go`
- **Edits:**
  - `internal/reedcli/watchdog.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a new package constant `watchdogOrphanGoneCycles = 3` to `internal/reedcli/watchdog.go`, declared immediately after the existing `watchdogHubIdleCycles`, carrying its own doc comment explaining that a session name must be observed gone on three **consecutive affirmative** discovery cycles before it is reaped (~15s at the existing 5s `watchdogHubDiscoveryCycle`), that the reap is destructive and unattended so a directory legitimately absent for a moment — a `git worktree move`, an editor or backup tool swapping a directory, a filesystem remount — must not kill live agent work, and that three is the same figure and reasoning `watchdogHubIdleCycles` already uses so the file carries one cadence idiom rather than two.

  Add a `watchdogTiming` struct with exactly three fields — `DiscoveryCycle time.Duration`, `IdleCycles int`, `OrphanGoneCycles int` — and a `watchdogDefaultTiming() watchdogTiming` constructor returning those three package constants and nothing else. This deliberately mirrors the shape `internal/reedengine/watchloop.go` already uses for its own `watchTiming`/`watchDefaultTiming` pair, so the repo carries one idiom for loop-timing injection.

  Leave the existing `watchdogHubDiscoveryCycle` and `watchdogHubIdleCycles` declarations and their doc comments exactly where they are — only their consumer changes, and that change is batch 3's.
- **Commit:** `feat(reedcli): add watchdog orphan-gone cycle constant and timing seam`

### Card 5: `validateWatchdogFlags`

- **Context:**
  - `internal/reedcli/spawnwatchdog.go`
- **Edits:**
  - `internal/reedcli/watchdog.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func validateWatchdogFlags(hubPath, tmuxPath string) error` to `internal/reedcli/watchdog.go`, lifting the two checks `watchdogCmd`'s `RunE` performs inline today into a pure function. It returns an error whose message is `"--hub-path must be an absolute, non-empty path"` when `hubPath` is empty or `filepath.IsAbs(hubPath)` is false, an error whose message is `"--tmux must not be empty"` when `tmuxPath` is empty, and nil otherwise — the two existing message strings are preserved verbatim so the command's observable output is unchanged.

  The function takes exactly two parameters and has no shell parameter at all. That absence is the structural guarantee behind the discussion's `the-daemon-is-told-its-shell` decision: `--shell` is accepted on every GOOS but never validated, and having no shell parameter to inspect makes that a property of the signature rather than a branch someone can add later. Say so in the doc comment, naming the consequence a hard `--shell` pre-flight would have — `ensureWatchdogSpawned` is best-effort and its child's stderr is discarded, so a rejection there would silently cost the hub its entire watchdog daemon, resize self-heal for every worktree included, over one empty config value.

  `internal/reedcli/spawnwatchdog.go` is this card's `Context:` for exactly that doc comment: the claim it asserts — that `ensureWatchdogSpawned` is best-effort and discards its child's stderr — is a claim about that file's own code, and the allowlist must let the implementer confirm it rather than transcribe it on faith.

  This card only adds the function. Rewiring `watchdogCmd`'s `RunE` to call it is card 11 in batch 3, so the ordering guarantee "pre-flight runs before any side effect" is established in one place at wiring time.
- **Commit:** `refactor(reedcli): extract validateWatchdogFlags as a pure pre-flight`

### Card 6: the two stat predicates

- **Context:**
  - `internal/reedengine/server.go`
- **Edits:**
  - `internal/reedcli/watchdog.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add two small predicates to `internal/reedcli/watchdog.go`.

  `func worktreeRootGone(path string) bool` reports whether `path` is **proven** gone: it returns true when `os.Stat(path)` returns an error satisfying `errors.Is(err, fs.ErrNotExist)`, and true when the stat succeeds but the returned `fs.FileInfo` reports a non-directory. Every other outcome returns false — a successful stat of a directory, and any other stat error (EACCES, EIO, a network-filesystem hiccup) alike. This restates `validateToldWorktreeRootLive`'s own only-proven-gone contract at the one layer that cannot call it: treating an unreadable path as gone would let a momentary permission or I/O blip destroy a session full of live work.

  `func hubIsLiveDir(hub string) bool` reports whether `hub` is **proven** live: it returns true only when `os.Stat(hub)` succeeds and the returned `fs.FileInfo` reports a directory, and false for every error. Write it as its own predicate rather than as the negation of `worktreeRootGone`, per the overview's `two-distinct-stat-predicates-not-one-negated` decision — an EACCES must make both false, and a negation would make one of them true.

  Both doc comments state which of the two verdicts the function proves and that every other outcome is the conservative answer.
- **Commit:** `feat(reedcli): add proven-gone and proven-live stat predicates for the reap`

### Card 7: `planReapCycle`

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `internal/reedcli/watchdog.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func planReapCycle(live []string, hubLive bool, gone map[string]bool, counters map[string]int, inFlight map[string]bool, threshold int) (reap []string, remaining []string)` to `internal/reedcli/watchdog.go`, beside the existing `planSessionDiff`.

  It is the daemon's pure decision seam for one whole cycle's reap bookkeeping. It performs no filesystem access of any kind — gone-ness is an input, produced by `worktreeRootGone` outside it — and it mutates `counters` in place while returning the two name lists the caller needs: `reap` (names to dispatch a reap for this cycle) and `remaining` (names that survive into `planSessionDiff`).

  Behaviour, in this order:

  1. When `hubLive` is false, short-circuit: return a nil `reap` and a `remaining` holding every name in `live` **except** those in the `inFlight` parameter declared in this card's own signature above, no file read needed, with every entry in `counters` untouched — neither advanced, reset, nor pruned. This is the hub probe's refusal to act, and the in-flight exclusion holds on this branch too: a hub outage must not become the one path on which `planSessionDiff` sees a name whose reap goroutine is still running.
  2. Otherwise, prune `counters`: delete every key that is not present in `live`. A name that left the live list starts from zero when it returns.
  3. Then walk `live` in order. A name in the `inFlight` parameter, no file read needed, is skipped entirely — it appears in neither return value. For a name whose `gone` entry is true, increment `counters[name]`; if the incremented value is at or above `threshold`, append it to `reap` and `delete(counters, name)`, otherwise append it to `remaining`. For a name whose `gone` entry is false or absent, `delete(counters, name)` and append it to `remaining`.

  Deleting the counter at dispatch rather than leaving it at `threshold` is the discussion's `counter-resets-after-a-reap` decision: `kill-session` teardown is asynchronous, so a reaped name can still appear in the next cycle's listing, and a still-listed name must re-confirm across three more affirmative cycles before a second kill is issued.

  The doc comment states that the seam is told the cycle's inputs rather than discovering them, that it is the only place the three bookkeeping rules interact, and that "three consecutive cycles" means three consecutive *affirmative* cycles — a cycle whose listing was non-affirmative never reaches this function at all, so it advances, resets and prunes nothing.

  `_mill/discussion.md` is this card's `Context:` because the three rules the function encodes are argued there and nowhere in the code yet: read its `three-consecutive-cycles-before-a-reap`, `counter-resets-after-a-reap` and `the-hub-itself-is-probed-before-the-reap-pass` decisions before writing the body.
- **Commit:** `feat(reedcli): add planReapCycle, the daemon's per-cycle reap decision seam`

### Card 8: untagged tests for the decision seams

- **Context:**
  - `internal/reedcli/watchdog.go`
  - `internal/reedengine/watchloop_test.go`
- **Edits:**
  - `internal/reedcli/watchdog_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Extend `internal/reedcli/watchdog_test.go` with table-driven tests for the four new seams, and update the file's header comment so it names them alongside the `planSessionDiff` and `sessionsAreIdle` seams it already describes. Every test here is pure: no tmux, no spawn, and no sleep — `t.TempDir()` is the only filesystem touched, which is ordinary filesystem work rather than a spawn and stays untagged under CONSTRAINTS.md's Test Tier Purity Invariant.

  `planReapCycle`, table-driven over its inputs, covering:

  - a live directory: never reaps, its counter stays at zero across repeated cycles;
  - missing across fewer than `threshold` cycles: no reap yet, counter advancing;
  - missing across exactly `threshold` cycles: reaps, and the counter entry is deleted rather than left at threshold;
  - missing, then present, then missing: the counter reset on the present cycle means no reap fires;
  - a name that leaves the live list: its counter entry is pruned, so its return starts from zero;
  - several sessions on one hub progressing independently in the same cycle;
  - counter-map hygiene: no unbounded growth across a sequence of cycles whose live set churns.

  The hub-probe case, called out separately because it is the one that fails loudly if the per-name predicate is ever trusted alone: with `hubLive` false and a listing naming several names all marked gone, `planReapCycle` reaps none of them, leaves every counter untouched, and returns every name as remaining except those in the `inFlight` parameter, no file read needed.

  The in-flight case: a name in `inFlight` appears in neither return value — not reaped again, and not passed to `planSessionDiff`, so it can never read as appeared while its reap is still running. Assert this on both the `hubLive` true and false branches.

  `worktreeRootGone` over a `t.TempDir()` fixture: a missing path is gone; a path that exists as a plain file is gone; an existing directory is not gone. `hubIsLiveDir` over the same fixture: an existing directory is live; a missing path is not; a plain file is not.

  The stat-error case both predicates must answer conservatively — a path whose stat fails with neither a not-exist result nor success, the EACCES shape — is asserted too, since it is the one case where treating an unreadable path as gone would destroy live work. Construct it by creating a directory under `t.TempDir()`, placing the target path inside it, and chmod-ing the parent to `0o000` so the stat of the target is denied, with `t.Cleanup` restoring the mode so the temp dir can be removed. Assert `worktreeRootGone` returns false and `hubIsLiveDir` returns false for that path — both conservative, and deliberately not each other's negation.

  Guard this one case rather than let it report a false pass: skip it on Windows, where directory mode bits do not deny traversal this way, and skip it when the test runs as uid 0, where mode bits are not enforced at all. In both cases the stat would succeed and the assertion would pass for the wrong reason. Use `t.Skip` with a message naming which of the two conditions fired, so a skipped run is visible rather than silent. The other cases in this card carry no such guard and run everywhere.

  `validateWatchdogFlags` directly, as a pure function: it rejects an empty `hubPath`, rejects a relative `hubPath`, rejects an empty `tmuxPath`, and accepts an absolute `hubPath` with a non-empty `tmuxPath`. This is the regression guard for the never-validated `--shell` rule, and it is asserted here rather than through the command's `RunE` deliberately — a CLI-level test of the accepting case would fall through the pre-flight into a global logger mutation, a lock acquisition under a scratch directory the command never creates, and then the discovery loop, whose first tick reaches `exec.Command` and is forbidden in an untagged file. An implementer who adds a shell validator cannot make this test compile, which is the point.

  `watchdogDefaultTiming()` returns exactly the three package constants — the guard against a test-only default silently becoming production's cadence, mirroring the coverage `internal/reedengine/watchloop_test.go` already gives its own default-timing constructor.

  `planSessionDiff`'s existing tests are left exactly as they are: unchanged passing tests are the proof the reap did not leak into that seam.
- **Commit:** `test(reedcli): pin planReapCycle, the stat predicates, and the flag pre-flight`

## Batch Tests

`verify: go test ./internal/reedcli/` runs the whole untagged `internal/reedcli` package suite.
The new assertions land in `internal/reedcli/watchdog_test.go`;
the run also re-executes the package's other untagged files (`watchdog_test.go`'s existing `planSessionDiff`/`sessionsAreIdle` cases, `spawnwatchdog_test.go`, `cli_test.go`, `statusline_test.go`), which is the point — the existing `planSessionDiff` cases passing unchanged is itself one of this batch's stated assertions.
The `integration`- and `smoke`-tagged files in the same directory are excluded by their build tags and are not compiled by this command;
this batch edits none of them.
Scope is one Go package, per the overview's `go-native-verify-commands` decision.
