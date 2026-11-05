```yaml
lens: code-tests
outcome: reported
```

Behavior-to-test list:

- Wrapped ETXTBSY followed by success, with 100 ms then 200 ms waits: `TestLoop_ChildStartRetriesABusyExecutable/BusyThenStarted`, `internal/shedverbs/loop_test.go:689`.
- Five unsuccessful starts, with waits of 100/200/400/800 ms and an interrupted stop: `BusyOnEveryTry`, `internal/shedverbs/loop_test.go:696`.
- Another start error receives no retry: `OtherStartErrorNotRetried`, `internal/shedverbs/loop_test.go:706`.
- Required `start-failed after <n> tries: <error>` cause: no correct assertion; the table explicitly expects the older format at lines 702 and 710.
- Each retry emits a Warn: no assertion.
- Interrupted retry sleep ends further start attempts: no test row; every injected retry sleep returns nil.
- Failed start without child trace or stderr uses the supplied loop trace: the new retry table checks both `AfterInterrupt` and envelope paths at line 752.
- Child trace and nonempty stderr retain precedence: existing interruption rows cover those sources, but never with `LoopTraceFile` installed.
- Loom and batten supply the logger trace callback: no test; their existing arming tests exercise process registration.
- Replacement leaves old open readers unchanged, new target installed, and no temporary file: `TestInstallBinary_RenamesTheBuildOverTheTarget/BuildSucceeds`.
- Build failure preserves the target and removes its temporary file: the same test’s `BuildFails` row.
- Rename failure preserves the target and removes its temporary file: no row.
- Retry tests inject waits instead of sleeping; deploy tests contain no waits.
- No duplicate coverage identified; the new cases exercise previously absent behavior.

Findings:

1. **MEDIUM — consistency — `internal/shedverbs/loop_test.go:702` — The new assertions require a cause that contradicts D4.**  
   Evidence: `BusyOnEveryTry` expects `start-failed: ...; ... across 5 tries`, and `OtherStartErrorNotRetried` at line 710 expects no attempt count at all.
   D4 requires `start-failed after <n> tries: <error>`.
   Correct these existing expectations alongside the production change, including the one-attempt non-ETXTBSY case; do not add a redundant test.

2. **MEDIUM — scope — `internal/shedverbs/loop_test.go:677` — Retry cancellation and retry-warning behavior have no coverage.**  
   Evidence: the production branches are `internal/shedverbs/loopchild.go:63–65`; the new table checks starts, waits, stop, cause, and trace, while its injected sleep always returns nil.
   Removing the Warn or ignoring a sleep failure would leave the table green.
   Extend the existing table with a sleep-failure case that stops after the actual attempts, and capture logs to verify the required retry warnings.

3. **MEDIUM — scope — `internal/shedverbs/loop_test.go:731` — Trace fallback tests bypass both production wiring and source precedence.**  
   Evidence: all new interrupted cases inject a nonempty fake `LoopTraceFile` and leave no child trace or stderr.
   The preexisting source-selection cases at lines 360, 417, and 494 leave the callback nil.
   The only production callback assignments are `internal/loomcli/looparm.go:52` and `internal/battencli/arm.go:538`; their existing arming tests only exercise `OnStart`.
   Extend the existing interruption cases with an armed fallback and verify child-trace/nonempty-stderr precedence, empty callback output, and the logger callback supplied by each production armer.

4. **MEDIUM — scope — `tools/deploy/main_test.go:289` — The new deploy table does not exercise rename failure cleanup.**  
   Evidence: its only rows are `BuildSucceeds` and `BuildFails`; neither reaches the failure branch at `tools/deploy/main.go:128–130`.
   Deleting cleanup in that branch leaves all new assertions passing, despite D3 explicitly requiring removal on rename failure.
   Add a rename-refusal row, for example using an existing nonempty directory as the destination, and assert the destination survives, the temporary build is removed, and the error identifies installation failure.

Tool-call record:

- Call 1: searched relevant tests and read previously unseen interruption rows, the file-level serial-test justification, and review severity/format rules.
- Call 2: searched the whole Go tree for the new predicate, callback, installer, and warning; read production arming tests and numbered retry/install branches to establish absent coverage and precise evidence locations.
