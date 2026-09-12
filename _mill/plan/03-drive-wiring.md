# Batch: drive-wiring

```yaml
task: 'self-report Tier 1: Go-detected structural anomalies'
batch: 'drive-wiring'
number: 3
cards: 3
verify: go test ./internal/loomcli/ ./internal/loomengine/ && go test -tags smoke -run 'TestSmokeBootstrap_BringsUpSessionStrandAndDriver|TestSmokeDriveStandalone_AdvancesMachineFromExistingSeed' ./internal/loomcli/ && go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks
depends-on: [1, 2]
```

## Batch Scope

This batch delivers the I/O half and the wiring: the one extracted, told-input `loomcli` function that owns every skip and the whole filing pass, its Tier-1 branch suite, the engine-boundary filing assertions, the `drive` call site, and the documentation that lands with the observable change.
It is one batch because the extracted function, its two test suites, and its single call site cannot be reviewed apart from each other — the whole point of the extraction is that the skip decision and the call site are one design.

It depends on batch 1 for `selfreportengine.DefaultLabels`, `shedadapters.IsLedgerPath`/`ReadLedger`, `loomengine.LoomSelfreportFiled`/`LoomSelfreportFiledLock`, and `loomengine.Config.Selfreport`;
and on batch 2 for `loomengine.EntryObservation`, `LedgerObservation`, `Anomaly`, `DetectAnomalies`, `DetectCrashResume`, and `RenderAnomalyBody`.
This is the batch that creates the two new production import edges — `loomcli` → `selfreportengine` and `loomcli` → `shedadapters` — neither of which needs a `CONSTRAINTS.md` entry, per the overview's `no-new-cross-cutting-invariant` decision.

Batch-local decision differing from the overview's Shared Decisions: none.

## Cards

### Card 7: the extracted detect-and-file function and its Tier-1 branch suite

- **Context:**
  - `internal/loomengine/anomaly.go`
  - `internal/loomengine/anomalybody.go`
  - `internal/loomengine/config.go`
  - `internal/selfreportengine/selfreport.go`
  - `internal/shedadapters/ledgeraccess.go`
  - `internal/shedengine/status.go`
  - `internal/shedengine/errors.go`
  - `internal/state/state.go`
  - `internal/lock/lock.go`
  - `internal/logger/logger.go`
  - `internal/loomcli/wiring_commitstatus_test.go`
  - `internal/loomengine/seed.go`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/selfreport.go`
  - `internal/loomcli/selfreport_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/loomcli/selfreport.go` holding the entire detect-and-file step as told-input functions, with a file doc comment stating that the step lives here rather than inline in `drive`'s cobra closure precisely so every branch is reachable from a direct Tier-1 call with no tmux, no git, and no real run.

  Declare a `selfreportDeps` struct carrying every input told, deriving nothing: the context, the resolved `selfreport` bool, the entry observation, the status path and status lock path, the marker path and its lock path, the error `shed.Run` returned, a ledger-path predicate seam, a ledger-read seam, and a filing seam matching `selfreportengine.CreateIssue`'s `(title string, body *string, labels []string) (string, int, error)` shape.
  The two ledger seams and the filing seam are function fields so the branch suite can count calls without reaching the engine or the filesystem;
  `drive` fills them with `shedadapters.IsLedgerPath`, `shedadapters.ReadLedger`, and `selfreportengine.CreateIssue`.

  Declare `func observeEntry(enabled bool, runLockPath, statusPath, statusLockPath string) loomengine.EntryObservation`, the entry step.
  When `enabled` is false it returns the zero observation immediately, performing no probe and no read — a disabled run must pay for neither.
  Otherwise it probes the run lock first and reads the status file second, and that order is load-bearing and must not be swapped.
  The probe is the same non-blocking shape `run.go`'s liveness probe already uses: `lock.TryAcquireWriteLock` on the run-lock path, releasing immediately when it was free, never holding it across the probe.
  Probing before reading is what makes the residual self-correcting: `Shed` persists its terminal state before `Run` returns and the deferred release follows, so a driver that finishes between the two steps has already written `done`, and the read that follows sees `done` rather than `running`.
  The read follows `VerifySeedOwnership`'s existing two-step pattern in the Context file — `state.ReadJSONStrict` for the shell, then a separate unmarshal of the product payload — and fills `Observed`, `RunLockHeld`, `State`, `CurrentProducer`, `HistoryLength`, `Slug`, and `Parent`.
  Any probe or read failure degrades to a `logger.Warn` and a zero observation with `Observed` false;
  nothing propagates.

  Declare `func detectAndFileAnomalies(deps selfreportDeps)`, returning nothing, and owning all three skips itself, checked before anything is read:
  first, the `selfreport` bool being false returns immediately — before the status file, before any ledger, before the marker;
  second, `errors.Is(deps.RunErr, shedengine.ErrShedBusy)` returns immediately, because that return means this process never ran the machine and never owned the run lock, so anything it observed at entry belongs to a live driver;
  third, a done context returns — but only after one exception, and it is the only one.
  When the context is already done, call `loomengine.DetectCrashResume` against the told entry observation and, if it reports one, file that single anomaly through the ordinary filing pass before returning;
  everything else the pass would have done is skipped.
  The exception exists because the cancellation arm persists the paused state, so the next drive's entry read sees `paused` rather than `running` and the in-memory crash observation is gone for good — the identical permanent loss the error path already refuses to accept.
  For the other four triggers the skip loses nothing: a paused run resolved none of its anomalies, so the next drive detects the same ones.
  The context check is not redundant with the busy check — an operator stop returns a paused outcome with a nil error, and the filing primitive builds its own timeout from a background context, so without this skip an operator stop would be followed by up to thirty seconds of network work per detected anomaly.

  Past the three skips, the pass re-reads the final status from the status file — never from the value `shed.Run` returned, which is documented meaningless whenever the returned error is non-nil, and this step runs on that path too.
  It then discovers ledgers by walking the final status's history entries and offering every non-empty output value to the told predicate, reading each distinct accepted path through the told read seam.
  Three skips apply, each non-fatal and none producing an anomaly: an empty output is skipped before the predicate is consulted, since the Bouncer's seed call publishes an explicitly empty pointer;
  a non-empty output the predicate rejects is skipped and never read, so a fail-loud parser is never handed a producer artifact or one of the Burler's own same-prefix files;
  and an accepted path whose read or parse fails — including a file that no longer exists, which is normal for an ephemeral ledger — is warned and skipped.
  Each parsed ledger entry becomes one `loomengine.LedgerObservation` carrying the round the file claimed and the producer name read from the history entry that published the path, never a recipe row name re-declared here.

  Hand the entry observation, the final status, the decoded product, and the ledger observations to `loomengine.DetectAnomalies`, then run the filing pass in exactly four ordered steps.
  One: collapse by title, reducing the slice to one anomaly per distinct title, keeping the highest `Round` when the duplicates are recurring-finding anomalies and the first occurrence otherwise.
  Mark that `otherwise` branch unreachable-by-construction in a comment, following the in-repo convention `selfreportengine.CreateIssue`'s own `targetRepo` guard already sets — kept as a defensive guard so a future change to the detector's output fails predictably rather than silently, not because it can fire today.
  It cannot fire today because only recurring-finding anomalies carry a non-zero `Round`, and one detector call returns at most one crash-resume and at most one halt-kind anomaly, whose title shapes collide neither with each other nor with a recurring-finding title.
  This is required, not defensive tidying — the judge carries an open entry forward losslessly into every later round's ledger, so from round three onward one recurring finding appears in several ledger files at once and would otherwise produce one identically-titled anomaly per file.
  Keeping the highest-round occurrence is what makes the body carry the fullest rounds list.
  Two: filter against the marker, dropping every title it already holds.
  Three: file, one filing-seam call per surviving anomaly, in the slice's deterministic order, passing `loomengine.RenderAnomalyBody`'s result as a non-nil body pointer and `selfreportengine.DefaultLabels()` as the labels.
  Four: record, adding a title to the marker immediately after that title's own filing call succeeds — never in advance and never in one batch at the end, so a failed call leaves its title unrecorded and therefore retried next run while its already-filed siblings stay recorded.

  Declare the marker as an unexported struct with one exported-tagged string-slice field of titles, read via `state.ReadJSONStrict` and written via `state.WriteJSON` against the told marker and marker-lock paths.
  A read failure is treated as an empty marker;
  a write failure is warned and nothing else.
  The marker itself stays a dumb string set with no parsing of its own — dedupe granularity is whatever the title shape already encodes.

  Create `internal/loomcli/selfreport_test.go` as an untagged Tier-1 suite driving `detectAndFileAnomalies` directly with stub seams, following `wiring_commitstatus_test.go`'s told-seam shape in the Context list.
  Six branch cases are mandatory, one per branch: a nil run error runs the pass and the stubbed filing seam records the expected calls;
  a non-nil run error that is not the busy sentinel still runs the pass with the same expectation, which is the regression test for the reachability defect, without which the natural implementation of appending after the success envelope passes every other case here;
  the busy sentinel reads nothing, detects nothing, and files nothing;
  a false `selfreport` bool gives the same total inaction, asserted on both the filing seam and the ledger seam, which is what distinguishes skipping everything from detecting and then declining to file;
  an already-done context whose entry observation carries no crash-resume gives total inaction;
  and an already-done context whose entry observation does carry a crash-resume gives exactly one filing-seam call, for that anomaly alone, and nothing else.
  Pair the last with its complement — a nil run error and a live context file everything — so the skip is shown to key on the context rather than on the paused outcome.

  Three more named tests, each load-bearing enough to deserve its own name rather than a clause inside another.
  Marker round-trip: write, read back, confirm the set survives, and confirm a second pass over the identical status files nothing — the regression test for the every-resume-re-files failure the marker exists to prevent.
  Its counterpart: a status whose history has advanced to a second, distinct halt files a new issue despite the marker already holding the first halt's title — the over-suppression failure the title discriminator exists to prevent, and a test that only covers the collapse direction would pass on a design that files exactly once per task forever.
  Carry-forward collapse, written before the collapse step: three ledger files for one segment at rounds three, four, and five, each carrying the same open key with a progressively longer rounds list, all reachable from history;
  assert exactly one recurring-finding anomaly survives, that its body carries the round-five entry's rounds list, and that the filing seam is called exactly once.
  Assert that at the filing-pass level rather than by observing the marker absorb the duplicates mid-pass — the collapse must be why there is one issue, not a side effect of filing order.

  One more: ledger discovery against a mixed history containing an empty output, a discussion-write entry whose output is a decision record, a Burler entry whose output is a round review file, a Bouncer entry whose output is a real ledger, and a Bouncer entry whose ledger path no longer exists.
  Assert exactly one ledger is read, that neither the producer artifact nor the Burler review file is ever opened, and that no skip produces an anomaly or an error.
  Alongside it, the generation case: a history entry whose ledger path now holds a different generation's file, with round numbering restarted, is read as ordinary content, contributes its anomaly, and raises no error — the accepted imprecision pinned as behaviour so a later reader does not mistake it for a bug.
- **Commit:** `feat(loomcli): add the told-input detect-and-file step for loom anomalies`

### Card 8: engine-boundary assertions on the real filing primitive

- **Context:**
  - `internal/loomcli/selfreport.go`
  - `internal/loomengine/anomaly.go`
  - `internal/loomengine/anomalybody.go`
  - `internal/selfreportengine/selfreport.go`
  - `internal/selfreportengine/selfreport_test.go`
- **Edits:** none
- **Creates:**
  - `internal/loomcli/selfreport_github_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/loomcli/selfreport_github_test.go`, an untagged Tier-1 suite binding the engine boundary rather than the told-seam boundary, with a file doc comment stating that split outright: the told filing seam is the `loomcli` unit boundary where every branch, filing-pass-order, collapse, marker, and config assertion already binds in `selfreport_test.go`, and this file is the engine boundary, where the actual argument shapes reaching the filing primitive are observable.
  No test binds both.

  Swap `selfreportengine.NewGitHubClient` — the exported package variable that exists for exactly this — pointing it at a real go-github client aimed at an `httptest` server, following the shape `internal/selfreportengine/selfreport_test.go` already uses, and restore it via `t.Cleanup`.
  No test here resolves a real token, reaches the network, spawns a process, or builds a fixture tree.

  Assert, with `selfreportengine.CreateIssue` itself as the filing seam: one issue-create request per detected anomaly;
  each request's title matches the deterministic shape for its kind;
  each request's body contains the slug, and for a halt kind the halt reason, and for a recurring finding the ledger key and every element of its rounds list;
  and each request's label list is exactly the engine's own default.

  Then the posture cases, which are the reason this file exists rather than being folded into the branch suite: a filing call that fails leaves that title out of the marker and does not propagate — assert both halves, the absent marker entry and the unchanged surrounding behaviour;
  an anomaly whose title is already in the marker produces no request at all;
  a marker read failure is treated as an empty marker and filing proceeds;
  and a marker write failure does not propagate.
- **Commit:** `test(loomcli): pin the anomaly filing path at the engine boundary`

### Card 9: wire the call site into `drive`, and land the docs

- **Context:**
  - `internal/loomcli/selfreport.go`
  - `internal/loomcli/cli.go`
  - `internal/loomcli/run.go`
  - `internal/loomengine/config.go`
  - `internal/loomengine/configtemplate.go`
  - `internal/loomrecipe/loomrecipe.go`
  - `internal/selfreportengine/selfreport.go`
  - `internal/shedadapters/ledgeraccess.go`
  - `manifest/designs/loom.md`
  - `manifest/designs/quarry-glyph-plan-alphabet.md`
- **Edits:**
  - `internal/loomcli/drive.go`
  - `internal/loomcli/smoke_test.go`
  - `internal/loomcli/wiring_test.go`
  - `manifest/designs/self-report-tier1.md`
  - `manifest/roadmap.md`
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Wire the two call sites into `internal/loomcli/drive.go`'s cobra closure, changing nothing else about the verb.

  The entry observation goes next to the existing `VerifySeedOwnership` call, where the status file is already being read, and is the one place the closure branches on the knob: call `observeEntry` guarded on `c.cfg.Selfreport`, passing the run-lock path, the status path, and the status lock path off `c.shedPaths`.
  Leaving the entry step unconditional would make a disabled run still pay for a lock probe and an extra status decode on every drive, which is exactly the cost the knob's rationale claims it avoids.

  The detect-and-file call goes on one unconditional line immediately after `shed.Run` returns and **above** the existing `if err != nil` early return, passing that error through.
  It must be above that return, not appended after the success envelope: the hard-error arm persists the failed state and returns a non-nil error, so appending after the success envelope would drop that whole failure class — and, worse, the entry-time crash observation lives only in this process's memory, so a crash-resume followed by a failing producer would be lost permanently, unrecoverable by any later drive.
  Fill the deps' seams with `shedadapters.IsLedgerPath`, `shedadapters.ReadLedger`, and `selfreportengine.CreateIssue`, and its marker paths with the two new `loomengine` accessors against `c.location`.
  Every early return stays inside the extracted function, so the closure gains exactly one unconditional line with no branching to get wrong.

  Neither envelope changes.
  The success envelope's four keys and the error envelope's text stay byte-identical, and the verb's exit code is untouched — a diagnostics side-channel never alters `drive`'s outcome.
  Add a short comment at the call site recording why the line sits above the early return, since that placement is the one thing a later edit would most plausibly undo.

  **Disarm the smoke suite in this same card**, because this is the card that first makes filing possible and therefore the last safe moment to do it.
  `internal/loomcli/smoke_test.go`'s `fastDeadlineLoomConfig` builds its fixture from `loomengine.ConfigTemplate()` with one `strings.Replace`, so it inherits the new `selfreport: true` automatically.
  That suite drives the real compiled `cmd/lyx` binary as a genuine subprocess — never `RunCLI` in-process, as its own header explains, because the bootstrap spawns its driver through `os.Executable()` — so there is no `selfreportengine.NewGitHubClient` seam to swap in it.
  Its runs deliberately bounce Discussion-Write until the bounce budget is spent and then block, which is exactly the bounce-budget-exhausted trigger.
  Combined with the warn-and-continue posture, every `go test -tags smoke ./internal/loomcli/` on a machine with a resolvable GitHub token would file a real issue against the upstream repository and still report a passing test.
  Add a second `strings.Replace` to `fastDeadlineLoomConfig`, turning the template's `selfreport: true` into `selfreport: false`, and state in its doc comment that this override extends the existing providerless/fast-deadline pair into a trio, along with why it is load-bearing: the suite has no filing seam to stub, so the knob is the only thing standing between it and real issues.

  In the same commit, land the documentation.
  Rewrite `manifest/designs/self-report-tier1.md` into the shipped shape the repo's other completed designs use — a status line marking it Done and pointing at the packages' own documentation for as-built detail, then a short description of what shipped: the five triggers and their thresholds, the per-trigger title discriminators, the four-step filing pass, the machine-local marker, the knob and its default, and the failure posture.
  Keep the existing Related links and drop the Open questions section, whose two questions are both now answered.
  Follow `manifest/designs/quarry-glyph-plan-alphabet.md`'s header shape, listed in Context as the model.

  Move the Planned entry for this task in `manifest/roadmap.md` into the Done section, keeping its `See` link line, and renumber nothing — the file uses repeated `1.` markers throughout.
  Rewrite the moved entry's prose into shipped tense to match its Done-section neighbours, which each describe what shipped rather than what is intended: the present-tense framing it carries as a Planned item ("loom's own status file already records …; file these directly via …") becomes a statement of what now happens.
  Drop the Planned entry's two independence clauses in the move — they existed to tell the operator this item could be built in parallel with Tier 2 and `lyx loom step`, which is spent information once it is Done.

  Update the two sibling Planned entries the move leaves stale, in the same edit.
  The `lyx loom step` entry's "Independent of the two self-report items below" is false once only Tier 2 remains below it, so it becomes the singular form naming that one item.
  The Tier 2 entry's "Independent of Tier 1 and of the `lyx loom step` item above" now points at an item that is no longer in the Planned section at all, so rephrase it to name Tier 1 as already shipped rather than as a parallel sibling.
  The repo already keeps such cross-references current rather than letting them rot — the Someday `webster: worktree-per-card parallel execution` entry refers to "The now-Done `Adopt quarry's glyph alphabet as the plan alphabet` item" in exactly this way, and is the phrasing model to follow.

  Update `docs/overview.md`'s selfreport module bullet so it names both triggers: the manual verb it already describes, and the automatic Go-detected structural-anomaly path this task adds, filed from `drive` off loom's own status file.
  Change no other line — this task adds no module and no command, so neither the module table nor the execution stack moves.

  Every markdown edit here is inside the Markdown Link Integrity gate, so every inline link's file part and anchor must still resolve after the edits.
- **Commit:** `feat(loomcli): file Go-detected loom anomalies from drive, and land the docs`

## Batch Tests

`verify:` runs `internal/loomcli`, the one package whose files this batch's cards create or edit, plus `internal/loomengine`, which this batch consumes by import rather than touching — it is in the list so a mismatch between the seams batch 2 declared and the way batch 3 calls them surfaces here rather than at the repo-wide done gate.
Because card 9 edits the `smoke`-tagged `internal/loomcli/smoke_test.go`, `verify:` also carries a `-tags smoke` invocation — an untagged run would not compile that file at all, so the fixture change would go unexercised until the repo-wide done gate.
It is scoped by `-run` to the two tests that drive the machine to a halt through `fastDeadlineLoomConfig`, which is where a filing attempt would fire, rather than to the whole thirteen-test suite: the rest build the same fixture but assert bootstrap and launcher behaviour that the `selfreport` override cannot affect, and the full suite builds the `cmd/lyx` binary and spawns real tmux sessions on every implementer and fixer round.
Both scoped smoke tests skip outright when tmux is absent from `PATH`, so this adds a real check where the substrate exists and costs nothing where it does not.
All of the above is followed by the markdown link-integrity gate, scoped by `-run` to the one test that enforces it, since card 9 edits three files inside that gate.
Only the untagged suites run: the package's `smoke`- and `integration`-tagged files stay out, which is the point of the extraction.

Files covered: `internal/loomcli/selfreport_test.go` (card 7's six branch cases, the two marker tests, the carry-forward collapse, and the mixed-history discovery cases), `internal/loomcli/selfreport_github_test.go` (card 8's engine-boundary argument-shape and posture cases), the package's existing untagged suites including `cli_test.go` and `wiring_commitstatus_test.go`, which must keep passing since card 9 changes `drive`'s envelopes not at all, and `internal/lyxcwd/docslink_test.go`'s markdown enforcement test.

No assertion in this batch needs control over what `shed.Run` returns, and none needs `reed.Up()` or a fabric open — the extracted function takes the run error as a told input, which is what keeps all six branch cases Tier 1.
The one assertion the discussion permits to fall back to the existing `smoke`-tagged suite is the thin call-site placement check, and it is deliberately not planned as a card here: `drive`'s envelope shape is already pinned by the package's existing tests, and card 9 changes neither envelope, so a dedicated placement assertion would restate existing coverage.
