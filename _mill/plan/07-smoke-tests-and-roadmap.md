# Batch: smoke-tests-and-roadmap

```yaml
task: 'loom: session bootstrap'
batch: smoke-tests-and-roadmap
number: 7
cards: 2
verify: go vet -tags smoke ./internal/loomcli/ && go test ./internal/loomcli/
depends-on: [6]
```

## Batch Scope

This batch adds the tagged smoke suite that covers the one thing nothing else can catch — the real tmux, real detached-process, real-lock interaction the bootstrap verb is made of — and moves the roadmap item to Done now that the whole task has landed.
It is one batch because the smoke suite is the last verification surface and the roadmap move is the last thing that becomes true.

The smoke suite is the regression home for the two bugs this task's own design rounds found before any code existed: the cleanliness-ordering blocker (loom's own seed dirtying the weft and failing loom's own first precondition row) and the double-spawn window (the run lock being taken by the child long after the spawn call returns).

Batch-local decisions beyond `## Shared Decisions` in the overview:

- The smoke file is verified by type-checking under its tag rather than by running it, because a real tmux server and a real detached child are not available in every environment this batch's verify command runs in.
  Running the suite is an operator act against a live hub, exactly as the reed smoke suite already is.

## Cards

### Card 32: the smoke suite for the bootstrap

- **Context:**
  - `internal/loomcli/bootstrap.go`
  - `internal/loomcli/drive.go`
  - `internal/loomcli/cli.go`
  - `internal/loomcli/testmain_test.go`
  - `internal/loomengine/config.go`
  - `internal/loomshed/seed.go`
  - `internal/fabricengine/origin.go`
  - `internal/fabricengine/warpclean.go`
  - `internal/reedcli/smoke_lifecycle_test.go`
  - `internal/reedcli/smoke_test.go`
  - `internal/reedcli/smoke_procalive_linux_test.go`
  - `internal/hubforge/hub.go`
  - `internal/proc/proc_linux.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/loomcli/run.go`
- **Creates:**
  - `internal/loomcli/smoke_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** The file carries the `smoke` build constraint as its first non-empty line, and follows the shape and fixture conventions the reed smoke suite already uses — a real wired hub, a real tmux server, and the same skip-when-the-multiplexer-is-absent posture that suite takes.
  While writing the smoke suite, this card's own live-hub runs surfaced a real defect in the spawn step `run.go` already carries: the spawned driver's `exec.Cmd` is never `Wait()`-ed, so a driver that finishes before the parent bootstrap process exits becomes a zombie, and `proc.IsAlive`'s `kill(pid, 0)` probe reports a zombie as alive indefinitely — `awaitRunLock`'s handshake then spins its full deadline and falsely refuses a bootstrap whose driver actually completed cleanly, on every fresh task (the common case: Discussion-Validate has nothing to validate yet and bounces to its budget, finishing in milliseconds). `run.go` moves from this card's `Context:` to its `Edits:` to fix this: reap the spawned process via a detached `go func() { _ = driveCmd.Wait() }()` immediately after a successful `Start()`, so a fast-finishing driver is reaped promptly and `proc.IsAlive` reports its death accurately.
  Cover, each as its own test function:
  (a) the bootstrap verb in a real wired hub brings the tmux session up, leaves exactly one status strand present under its fixed name, leaves a detached driver process alive, and leaves the status file seeded;
  (b) a second bootstrap invocation while the first driver holds the run lock leaves still exactly one strand and still exactly one driver process, spawns nothing new, and does not surface the seed-exists refusal as a failure;
  (c) the driver verb standalone with no tmux at all, on a pair the fixture seeded by running the bootstrap once and then killing the driver, advances the machine and records that advance in the status file;
  (d) the driver verb on a never-seeded pair refuses on the envelope with a message naming the bootstrap verb, writes no driver log, and leaves the weft clean;
  (e) a driver rigged to fail before its first persist leaves a non-empty driver log naming the failure;
  (f) the run launcher exists in the per-slug hub launcher directory after a pair is added and is gone after the matching remove;
  (g) the cleanliness ordering — in a freshly added, never-run pair the bootstrap reaches past the first precondition row rather than blocking on it, the fabric cleanliness check reports clean immediately after the seed commit, and the weft carries exactly one new commit touching only the status file;
  (h) the spawn handshake — two bootstrap invocations started concurrently produce exactly one driver process and no already-running refusal in the driver log;
  (i) the handshake's failure exit — a driver rigged to die immediately makes the bootstrap refuse on the envelope naming the driver log, hold no lock afterwards, and not attach.
  Cases (g) and (h) are the two named regression guards and must assert the mechanism, not merely a happy outcome.
  Where a case needs a driver that fails or dies, rig it through an injected executable path or an environment-gated failure the production code already honours rather than by editing production code for the test's benefit.
  Every case that starts a real process must tear it down, including on a failed assertion.
- **Commit:** `test(loomcli): add the smoke suite for the session bootstrap`

### Card 33: move the roadmap item to Done

- **Context:**
  - `internal/loomcli/cli.go`
  - `internal/loomcli/run.go`
  - `internal/fabricengine/origin.go`
  - `manifest/designs/loom.md`
  - `docs/overview.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Remove the session-bootstrap item from the Planned section and add a corresponding entry to the Done section, in the voice and shape the entries already there use: a bold title, a sentence or two of what shipped, and a see-reference.
  The Done entry must name what actually landed rather than what was planned: the four verbs, the root alias, the seed-and-commit-before-spawn ordering, the spawn handshake, the recorded parent-branch provenance record written and committed by the pair-creating fabric verb, and the third launcher script.
  Do not carry the stale worktree-local launcher description forward from the Planned item; that mechanism was corrected during this task.
  Leave the numbering alone — both sections restart at one and are automatic, per that file's own maintenance note.
  Every link in the entry must resolve, both its file part and its anchor, since the markdown link guard scans this file.
  Write in this repo's markdown style: one sentence per line, no fixed-column hard wrapping.
- **Commit:** `docs(roadmap): move the loom session-bootstrap item to Done`

## Batch Tests

`verify: go vet -tags smoke ./internal/loomcli/ && go test ./internal/loomcli/` type-checks the new tagged file and then runs the package's untagged suite.
The vet pass under the tag is the verification card 32 can actually get in an arbitrary environment: it compiles the smoke file against the real production signatures, so a drifted call site or a stale identifier fails here, while running the suite needs a live tmux server and real detached children that the batch-verify environment does not guarantee.
The untagged run is the regression check that adding a file to the package did not break the tier-1 suite.
Card 33 touches only a doc and has no runnable surface; the markdown link guard that covers it lives in another package and is reached by the repo-wide done gate at Handoff.
