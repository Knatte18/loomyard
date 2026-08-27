# Batch: loom-transition-commit

```yaml
task: "Add a local-only file category to weft"
batch: "loom-transition-commit"
number: 7
cards: 4
verify: go build ./cmd/lyx && go test ./internal/loomcli/... ./internal/lyxcwd/...
depends-on: [5, 6]
```

## Batch Scope

This is the batch that makes cross-machine resume real: `internal/loomcli` fills `loomrecipe.ShedPaths.CommitStatus` with a closure that commits loom's own status file and pushes it, once per producer transition.
It consumes both of batch 5's new seams — `fabricengine.MergeStateActive` and `fabricengine.PushAnchored` — and batch 6's `CommitStatus` field, hence the two `depends-on` edges.
`manifest/designs/loom.md`'s resume paragraph currently asserts outright that cross-machine resume does not work and that fixing it "is a `Shed` persistence-policy decision with a real per-transition git cost";
this task makes that decision, so those lines are rewritten here rather than left asserting removed behaviour.

Batch-local decisions beyond `## Shared Decisions`:

- The seam is built by a named constructor over an injected `commitStatusDeps` struct rather than as an anonymous literal inside `wire`.
  Both fill sites then share one definition, and the branching this batch must prove — commit hard-errors, push warns, mid-merge skips — becomes Tier 1 testable without a hub fixture or a git spawn, which the Test Tier Purity Invariant otherwise makes impossible for an inline closure over three real fabric calls.
- Disposition, per `commit-hard-errors-push-warns`: a commit failure returns an error from the seam and therefore halts the run — a git fault on the run's own bookkeeping is infrastructure breakage, and it is the disposition `landingshed` already gives a failing `CommitStatus`.
  A push failure logs a warning and returns nil — an offline laptop must not kill an autonomous run, and the next transition's push catches the branch up.
- Disposition, per `skip-while-mid-merge`: `MergeStateActive` reporting true means no commit, no push, no error, logged at warn.
  A non-nil error from the probe is treated **exactly like true** — an unreadable probe is the same "git state cannot be trusted right now" category the skip exists for, and probe I/O failures cluster precisely when foreign merge machinery is touching the repo.
- `landingshed.Deps.CommitStatus` and its calls in both landing producers stay exactly as they are, per `landing-checkpoint-stays`.
  With the per-transition hook wired it is a no-op on the ordinary path, and it is the only protection if a product wires `Shed.CommitStatus` as nil.
  Nothing in `internal/landingshed` is edited by this task.

## Cards

### Card 29: build the per-transition status seam

- **Context:**
  - `internal/fabricengine/pushanchored.go`
  - `internal/fabricengine/mergestateactive.go`
  - `internal/fabricengine/commitweftpaths.go`
  - `internal/fabricengine/fabric.go`
  - `internal/loomcli/landingdeps.go`
  - `internal/loomengine/config.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/gitrepo/push.go`
  - `internal/logger/logger.go`
  - `internal/lyxcwd/lyxcwd.go`
- **Edits:**
  - `internal/loomcli/wiring.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomcli/wiring.go`, add an unexported struct `commitStatusDeps` carrying exactly three function fields — `MergeActive func() (bool, error)`, `Commit func(msg string) error`, and `Push func() error` — with a doc comment stating it exists so the seam's branching is drivable without a hub fixture.
  Add `func loomCommitStatusDeps(location *lyxcwd.Location) commitStatusDeps`, filling the three fields from fabric:
  `MergeActive` calls `fabricengine.MergeStateActive(location)`;
  `Commit` calls `fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{loomengine.LoomStatusRel()}, msg, fabricengine.EnvSyncOptions())` and returns the error alone, discarding the `(sha, committed)` pair exactly as `landingdeps.go`'s own `CommitStatus` closure does — which is what makes a second call over an already-clean tracked path a no-op rather than a failure;
  `Push` calls `fabricengine.PushAnchored(location, fabricengine.EnvSyncOptions())` and returns the error alone.
  The pathspec is `loomengine.LoomStatusRel()`, never a hand-built join naming the `_lyx` literal, which the Lyxdirs Single-Declarer Invariant forbids.
  Add `func commitStatusMessage(producer, state string) string` returning `fmt.Sprintf("loom: %s -> %s", producer, state)`, with a doc comment stating why it is not a bare constant: the seam fires once per transition rather than once per landing, and an unreadable stream of identical messages is the log a resuming operator has to read.
  Add `func newCommitStatusSeam(deps commitStatusDeps) func(producer, state string) error` implementing the disposition in this exact order:
  call `deps.MergeActive()`;
  on a non-nil error, `logger.Warn` and return nil;
  on a true result, `logger.Warn` naming the producer and state, and return nil;
  otherwise call `deps.Commit(commitStatusMessage(producer, state))` and return its error unchanged when non-nil;
  otherwise call `deps.Push()`, and on a non-nil error `logger.Warn` and return nil.
  Its doc comment must state the three dispositions and why each is what it is, and must state that `gitrepo.ErrPushRejected` is an ordinary push failure here — warn and continue, never retry, never rebase — because a rejection means another machine advanced the branch, which is a human decision rather than something a background persist may rewrite history over.
  Import `internal/logger` in this file if it is not already imported.
- **Commit:** `feat(loomcli): build the per-transition status commit-and-push seam`

### Card 30: fill both ShedPaths sites with the seam

- **Context:**
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/shedengine/shed.go`
  - `internal/loomcli/cli.go`
- **Edits:**
  - `internal/loomcli/wiring.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomcli/wiring.go`, set `CommitStatus: newCommitStatusSeam(loomCommitStatusDeps(location))` in both `loomrecipe.ShedPaths` literals — the one in `wireStatusPathsOnly` and the one at the end of `wire`.
  Fill both, not only `wire`'s: `wireStatusPathsOnly` serves the read-only status and pause verbs, which never call `Run`, so the seam is never invoked from there — filling it anyway keeps the two literals structurally identical, so a future verb promoted from one path to the other cannot silently lose the hook.
  Add a comment at the `wireStatusPathsOnly` site saying exactly that, so the fill does not read as an accident.
  Leave `wireStatusPathsOnly`'s own doc comment's claim that it "loads no module config, constructs no engine, and can fail only if loomengine's own path accessors do" true: `loomCommitStatusDeps` builds three closures and performs no I/O at build time, so it neither loads config nor opens a fabric.
  Leave `MaxBounces` unset in both literals, with its existing comment.
- **Commit:** `feat(loomcli): fill ShedPaths.CommitStatus at both wiring sites`

### Card 31: rewrite loom.md's resume and landing-checkpoint paragraphs

- **Context:**
  - `internal/loomcli/wiring.go`
  - `internal/loomcli/landingdeps.go`
  - `internal/shedengine/run.go`
  - `manifest/designs/shed.md`
  - `manifest/roadmap.md`
  - `_mill/discussion.md`
- **Edits:**
  - `manifest/designs/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `manifest/designs/loom.md`, rewrite the resume paragraph around `manifest/designs/loom.md:273-277`.
  It currently says the status file "is not continuously committed there, so **resume across machines does not work today**", that `lyx loom run` commits the seed once with the landing checkpoint as the only other commit, and that making it cross-machine "is a `Shed` persistence-policy decision with a real per-transition git cost, not a property this doc can assert into existence".
  Replace those lines with what now holds: `Shed` commits and pushes the status file on every producer transition through an injected seam `internal/loomcli` fills, so a second machine that pulls the branch sees the run's live FSM state;
  the push respects `SkipPush`;
  a commit failure halts the run while a push failure warns and self-heals on the next transition;
  and a status commit is skipped outright while the weft is mid-merge.
  Name the operator's own half explicitly, because loom does not do it: nothing in `internal/loomcli` pulls, so a second machine resumes by pulling the branch itself with `lyx fabric pull`.
  Rewrite the landing-checkpoint paragraph around `manifest/designs/loom.md:279-283`.
  It currently explains the checkpoint as load-bearing against `fabricengine`'s merge guard refusing a tracked modification on either side of the pair.
  It must now say the weft is no longer a merge participant at all, that the checkpoint is therefore a no-op safety net on the ordinary path rather than the last row's only protection, and that it is retained because it is the sole protection if a product wires `Shed.CommitStatus` as nil.
  Keep the `#crash-recovery--resume-on-output-files-not-live-processes` heading text exactly as written: `manifest/roadmap.md` and this same file both link it, and the Markdown Link Integrity invariant binds.
  Add no roadmap entry: this is a reopened bug, not a completed or newly added planned item, so `manifest/roadmap.md` does not move.
  Follow the repo's semantic-line-break rule.
- **Commit:** `docs(loom): per-transition status commit makes cross-machine resume real`

### Card 32: Tier 1 tests for the seam and both fill sites

- **Context:**
  - `internal/loomcli/wiring.go`
  - `internal/loomcli/wiring_test.go`
  - `internal/loomcli/landingdeps_test.go`
  - `internal/loomcli/testmain_test.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/lyxcwd/lyxcwd.go`
  - `internal/gitrepo/push.go`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/wiring_commitstatus_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/loomcli/wiring_commitstatus_test.go` in `package loomcli`, untagged, spawning no git and no process — `newCommitStatusSeam` takes injected function values, so every branch is drivable from stub closures and the file stays Tier 1.
  Follow `wiring_test.go`'s own fixture approach for the two fill-site tests, which need a hand-built `*lyxcwd.Location` over a temporary directory rather than a hub.
  Cover seven properties, one test function each:
  (1) the ordinary path — `MergeActive` false, `Commit` nil, `Push` nil — calls `Commit` exactly once and `Push` exactly once, in that order, and returns nil;
  (2) `commitStatusMessage` renders exactly `loom: <producer> -> <state>` for a table of producer/state pairs, so a regression to a bare constant fails here;
  (3) a `Commit` error propagates out of the seam unchanged, and `Push` is never called;
  (4) a `Push` error returns nil from the seam, so a failed push never halts a run, while `Commit` still ran;
  (5) `gitrepo.ErrPushRejected` specifically returns nil from the seam, since a rejection is the routine multi-machine case this feature creates;
  (6) `MergeActive` reporting true skips both `Commit` and `Push` and returns nil, and `MergeActive` returning a non-nil error does exactly the same — assert both in this one test, since the equal-disposition property is the point;
  (7) both `wireStatusPathsOnly` and `wire` leave `c.shedPaths.CommitStatus` non-nil.
  For property 7, drive `wire` the way `wiring_test.go` already does rather than building a second fixture idiom.
- **Commit:** `test(loomcli): pin the per-transition status seam's three dispositions`

## Batch Tests

`verify:` runs `go build ./cmd/lyx`, then `./internal/loomcli/...` and `./internal/lyxcwd/...`.

- No `-tags integration` invocation is chained, and no `-tags smoke` one either.
  `internal/loomcli` carries no `integration`-tagged test file at all — its heavy tests are `smoke`-tagged, they spawn real tmux sessions and LLM-shaped processes, and card 32 adds no tagged file.
  Running the smoke tier per batch would cost minutes and exercise nothing this batch changed.
- `./internal/lyxcwd/...` is included because card 31 edits `manifest/designs/loom.md`, whose heading anchors are linked from `manifest/roadmap.md`;
  `docslink_test.go` in that package enforces the Markdown Link Integrity invariant and is what catches a renamed heading.
- `./internal/loomcli/...` also runs `landingdeps_test.go`'s reflection-based drift guard over `landingshed.Deps`, which is what catches an accidental edit to the landing checkpoint this batch is required to leave alone.
- Card 32's properties 3, 4 and 6 are the batch's primary proof: they are the three dispositions `commit-hard-errors-push-warns` and `skip-while-mid-merge` decide, and an implementation that got any one of them backwards passes properties 1, 2 and 7.
- `pipeline.done_gate` — `go test ./... && go test -tags integration ./...` — is what sweeps the whole repo, including every `fabricengine` suite batches 1–5 touched, before the task is marked done.
