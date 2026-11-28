---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: internal/boardserve/run.go:179
    summary: "The board server's watch loop wakes on every board.json file event with no one-second floor, against PATTERN-daemon-wakeups and the record's constraint that the loop keeps the floor"
    fix: "In watchBoard, keep any two wakes at least watchBaseWait apart, whether an event or the timer raised them. Record the time of the last wake from opts.Now. An event or timer arriving sooner is held and handled once the rest of the second has passed (waited through opts.After), and further events inside that second coalesce into that one wake. An event with no previous wake is handled at once. In pattern/PATTERN-daemon-wakeups.md, add to the board server's list entry that an event never wakes it within a second of the previous wake. Add a TestRun row in internal/boardserve/run_test.go that sends two events through an injected Watch with the clock unmoved and sees one wake. run_test.go's fakeClock.After hands out one shared channel and ignores its duration, so the fake must tell the floor wait from the backoff timer."
    origin: lens:code-correctness
  - id: F2
    severity: BLOCKING
    class: consistency
    location: internal/boardserve/server.go:142
    summary: "Names the diff writes are repeated literals, against PATTERN-code-conventions Named constants: the loopback address format, the runTask command id and the bare lym command"
    fix: "(1) internal/boardserve/server.go:142: replace the fmt.Sprintf of 127.0.0.1:%d with loopbackAddress(s.config.Port), declared in lifecycle.go of the same package, and drop the fmt import only if nothing else uses it. (2) internal/vscode/keybindings.go: declare a constant for workbench.action.tasks.runTask beside terminalSendSequenceCommand and use it in the three keybindingsBlockEntries rows. (3) internal/vscode/config.go: declare one constant for the bare command lym and use it in both WriteConfig (line 72) and WorkspaceTasks (line 197)."
    origin: lens:code-affected-set
  - id: F3
    severity: MEDIUM
    class: design
    location: internal/boardengine/board.html:524
    summary: "A successful action moves an older unsent draft to the new revision, so its next Save overwrites another writer's change with no stale refusal"
    fix: "In send, in the success branch for an op that is neither edit nor create, set draft.revision = answer.entry.revision only when draft.revision === write.revision, that is, when the draft was read at the same state the action was sent from. Otherwise leave it, so Save is refused as stale and the fresh entry is shown. In the same branch set draft.kind = answer.entry.kind, so the draft of a promoted note offers depends_on."
    origin: lens:code-correctness
  - id: F4
    severity: MEDIUM
    class: design
    location: internal/boardcli/open.go#spawnServer
    summary: "A spawned server's startup failure is lost: serve and the pre-run report errors on stdout, which spawnServer discards, while open tells the operator to read the log and end the process"
    fix: "In spawnServer set cmd.Stdout = log beside cmd.Stderr = log, so the JSON refusal of lym board serve and of its pre-run reaches server.log. internal/slackengine/start.go sets both to its log file the same way. Do not truncate the log in spawnServer: a server that loses the lock race would erase the holder's log. Pin it in TestBoardServeScenario (internal/boardcli/serve_linux_integration_test.go) or a case that makes a spawned serve fail and reads the reason from the log."
    origin: lens:code-correctness
  - id: F5
    severity: MEDIUM
    class: scope
    location: internal/boardserve/write.go:80
    summary: "POST /api/write creates a project (create with kind project) and edits one, although D10 allows only tasks and notes and Scope Out excludes project create or edit"
    fix: "In internal/boardserve/write.go refuse, with a way forward, a create whose fields hold a kind other than boardengine.KindTask or boardengine.KindNote (an absent kind keeps today's path: the TestServer row CreateOfExisting sends none; I did not read what the store defaults it to, so confirm it is never a project). Refuse an edit whose slug is a project: view.Projects of Board.View lists the open ones, as taskStatus already reads it; a done project is not listed there, so check the stored kind of it too or state why it need not be. Add a refused row for each to testRefusedWrites in server_test.go; its fixture needs a project."
    origin: lens:code-scope
  - id: F6
    severity: MEDIUM
    class: design
    location: .gitattributes:22
    summary: "The new go:embed target internal/boardengine/board.html is not pinned to LF, so a CRLF checkout builds a binary that rewrites the committed board.html with other bytes"
    fix: "Add the line `internal/boardengine/board.html text eol=lf` to .gitattributes beside the other embedded files."
    origin: lens:code-affected-set
  - id: F7
    severity: MEDIUM
    class: consistency
    location: internal/boardcli/open_test.go#TestRunOpen
    summary: "No test fails when runOpen stops probing the token: no row has a held lock, a running pid and a state that does not answer"
    fix: "Add to the NoServerAnswers map a row recording a state with a running pid (oldPID) and a token the fake Probe refuses, expecting the unanswered refusal and no spawn."
    origin: lens:code-tests
  - id: F8
    severity: MEDIUM
    class: consistency
    location: internal/boardserve/run_test.go#fakeClock.After
    summary: "The watch timer's one-second start, doubling, eight-second ceiling and reset are unpinned, since the fake After ignores its duration"
    fix: "Record the durations passed to fakeClock.After and add a TestRun row that ticks an unchanged board and expects 1s, 2s, 4s, 8s, 8s, then changes board.json, ticks and expects 1s. Use the literal durations, not watchBaseWait or watchMaxWait."
    origin: lens:code-tests
  - id: F9
    severity: MEDIUM
    class: consistency
    location: internal/boardcli/serve_linux_integration_test.go#TestBoardServeScenario
    summary: "The verb wiring is untested: the keep-warm opener cap from the strand name, serve's runproc registration, the server's output reaching server.log and the envelope's launcher key"
    fix: "In TestBoardServeScenario assert that first.Raw[\"launcher\"] equals paths.Launcher, that server.log opens with `lym board serve pid <st.PID>`, and that a record file `<st.PID>-*.json` holding role board-server sits under fabricengine.RunProcDir(h.Path)/runproc.HubOwner. For the cap, move the OpenerCap choice out of openCmd's RunE into an unexported function of the location and strand, and test in internal/boardcli that an empty strand gives zero and an orch strand gives a non-zero cap."
    origin: lens:code-tests
  - id: F10
    severity: MEDIUM
    class: consistency
    location: internal/boardserve/server_test.go#testRefusedWrites
    summary: "No stale-promote row: promoting through the unconditional board would pass every test and let a stale promote write"
    fix: "Add a row {StalePromote, opPromote, \"idea\", \"older\", nil, 409, the `changed since the page read it` message for idea}; the table already checks the fresh entry and an unchanged board.json."
    origin: lens:code-tests
  - id: F11
    severity: LOW
    class: design
    location: internal/boardengine/board.html#removedDrafts
    summary: "An action refused because the entry was removed shows no message when the entry has no draft: the row vanishes and state.messages is never rendered, against D10"
    fix: "Build the #removed boxes from the union of the slugs in state.drafts and in state.messages that the board no longer holds. With no draft, show the message and a Dismiss button that deletes state.messages[slug]; with a draft, keep today's box."
    origin: handler
  - id: F12
    severity: LOW
    class: design
    location: internal/boardengine/board.html#request
    summary: "The retry minute is counted from the request's start, so a long poll that fails late gets about ten seconds of retries, and one failing after a machine sleep gets none"
    fix: "In request, leave deadline unset until the first failure and set it to Date.now() + retryMillis inside the catch when it is unset."
    origin: lens:code-correctness
  - id: F13
    severity: LOW
    class: design
    location: internal/boardserve/write.go#runWrite
    summary: "The page's edit and promote drop the RecordWarning the board returns, so a Sketch whose body changed after its record was stored is released with no warning"
    fix: "Have runWrite return the RecordWarning of UpsertTaskReporting's and PromoteTo's results (the field is exported on both), put it in the success answer as `warning` when non-empty, and have board.html's send show it as text on the entry. The sibling notice is formatted by an unexported boardcli function and can stay out. Add a server_test.go assertion for a promote that warns."
    origin: lens:code-correctness
  - id: F14
    severity: LOW
    class: design
    location: internal/boardcli/open.go#runOpen
    summary: "Whether to spawn is decided once, so a server that exits or dies during the wait leaves open to time out and say to end a process that is gone"
    fix: "In the wait loop, when Live reads false and this call has not spawned yet, resolve Prime and Spawn once and restart the deadline; report Started true when it spawned. Add a TestRunOpen row that releases the held lock mid-wait and expects one spawn."
    origin: lens:code-correctness
  - id: F15
    severity: LOW
    class: consistency
    location: docs/overview.md:464
    summary: "The overview's board module passage says nothing of lym board open, lym board serve, the viewer page, board-data.js or board.html"
    fix: "Add to the board passage under Modules a few lines naming the two verbs, the loopback server behind the viewer page, and that every board write also renders board-data.js and board.html."
    origin: lens:code-affected-set
  - id: F16
    severity: LOW
    class: consistency
    location: internal/runproc/end.go:98
    summary: "The subjectPassSpared comment says the roles that serve the whole hub are spared, which is false now that RoleBoardServer serves the hub and is not"
    fix: "Reword the comment to say which hub-wide roles are spared and that the board server is deliberately ended."
    origin: lens:code-affected-set
  - id: F17
    severity: LOW
    class: consistency
    location: internal/boardserve/server_test.go#TestServer
    summary: "Server behaviours with no row: serving a decision file and a project's design file, CSP hashing of a CRLF page, the body, project and depends_on fields (and brief on create), a project refused by release, draft or undraft, and a field sent to an op that takes none"
    fix: "Extend the existing subtests: fetch a decision file and a project design file in testReadRoutes (the fixture needs both); assert contentSecurityPolicy of a CRLF copy of ViewerPage equals the LF policy; add body, project, depends_on and brief to the testOps create and edit rows; add refused rows for a project and for a field on release."
    origin: lens:code-tests
  - id: F18
    severity: LOW
    class: consistency
    location: internal/boardserve/run_test.go#TestRun
    summary: "Probe rejecting a non-200 answer and the draining server refusing new requests are unpinned"
    fix: "In testRunStartState assert that Probe of the state with a wrong token returns an error. In testRunDrainFinishesWrites, after Live reads false, assert that a new request to the server fails to connect."
    origin: lens:code-tests
  - id: F19
    severity: LOW
    class: consistency
    location: internal/boardengine/view_test.go#TestRenderBoardData
    summary: "Projection behaviour with no row: the Independent and Other sections, entry and project title and brief, project labels, and Board.View dropping a missing record's decision file"
    fix: "Add to viewFixture a task with no dependency tie that lands in Independent and a note with no configured type, set distinct briefs and titles on one entry and one project, and assert those values and the two sections literally. In ViewMigratesWithoutSaving or a sibling row, assert Board.View gives no decision_file for a discussed entry whose file is missing."
    origin: lens:code-tests
  - id: F20
    severity: LOW
    class: consistency
    location: internal/boardcli/serveopen_test.go#TestServeOpenRefusals
    summary: "The D12 criterion that an orch's strand name proceeds has no row, so refusing every non-empty strand would pass"
    fix: "Add a row per verb with an orch strand name expecting the --board-path refusal rather than the run-agent one."
    origin: lens:code-tests
  - id: F21
    severity: LOW
    class: consistency
    location: internal/fabricengine/hubworkspace_integration_test.go#TestRegenerateHubWorkspace
    summary: "The branch that builds a beam's file with no hook members when its anchor cannot be read has no row"
    fix: "Add a row that makes a beam's recorded anchor unreadable to lymcwd.NamedWorktree, installs a content hook, and asserts the regeneration succeeds, the beam file is written without the hook's member and the hub file keeps it. I did not establish how a test best makes that read fail."
    origin: lens:code-tests
  - id: F22
    severity: LOW
    class: consistency
    location: internal/boardcli/open_test.go#TestRunOpen
    summary: "The TheAddressHoldsNoToken subtest duplicates LiveServer, which already asserts the exact address and that the opener got only the launcher path"
    fix: "Delete the TheAddressHoldsNoToken subtest."
    origin: handler
---

The diff builds each decision of the record.
Two violations of listed constraints block it, the first of them the watch loop's missing floor.
I did not build or run tests; the verify gate covers that, and every finding here comes from reading the code.

### [BLOCKING] Watch loop has no one-second floor

**Location:** `internal/boardserve/run.go:179`
**Issue:** `watchBoard` handles each fswatch event at once.
`PATTERN-daemon-wakeups` requires any periodic wake of an unbounded wait to be at least one second apart, and the record says this loop keeps that floor.
A burst of writes or a replay wakes it back to back, which also defeats the two-consecutive-checks debounce of the replaced-binary test.
The record's carry-over names `watch-loop-no-one-second-floor`, and the code still has no floor.

### [BLOCKING] Repeated literals

**Location:** `internal/boardserve/server.go:142`
**Issue:** The Host check formats the loopback address itself, although `loopbackAddress` declares it in the same package.
`internal/vscode/keybindings.go` now spells `workbench.action.tasks.runTask` three times, and `internal/vscode/config.go` spells the bare `lym` fallback twice.

### [MEDIUM] An action carries a stale draft past the revision check

**Location:** `internal/boardengine/board.html:524`
**Issue:** A draft read at revision r1 stays open while another writer produces r2.
An action sent at r2 succeeds as r3, and the draft is moved to r3.
Save then overwrites the other writer's field without a stale refusal, which D8 exists to prevent.

### [MEDIUM] A failed server start leaves no reason anywhere

**Location:** `internal/boardcli/open.go` (`spawnServer`)
**Issue:** Only stderr goes to the log, while `serve` and its pre-run print refusals on stdout.
`open` then waits out its bound and tells the operator the log opens with the pid of a process to end.
A failure before the log is truncated leaves a previous server's pid in it.

### [MEDIUM] The write route creates and edits projects

**Location:** `internal/boardserve/write.go:80`
**Issue:** Field names are checked, the value of `kind` is not, and the board's upsert makes a project from `kind: project`.
The page offers only note and task, so the route is reached by a hand-built request.

### [MEDIUM] board.html is not pinned to LF

**Location:** `.gitattributes`
**Issue:** The file's own rule pins `go:embed` targets, since their bytes must not depend on a checkout's `core.autocrlf`.
Two machines on one lym version would otherwise rewrite `board.html` on each other's writes, against D2.

### [MEDIUM] Token probe unpinned

**Location:** `internal/boardcli/open_test.go`
**Issue:** Deleting the `deps.Probe` condition from `runOpen` fails no row, so D5's "answers a request carrying its token" has no test.

### [MEDIUM] Watch timer schedule unpinned

**Location:** `internal/boardserve/run_test.go` (`fakeClock.After`)
**Issue:** A loop with no doubling, no ceiling, no reset or a millisecond timer passes `TestRun`.

### [MEDIUM] Verb wiring untested

**Location:** `internal/boardcli/serve_linux_integration_test.go`
**Issue:** `TestRunOpen` sets `OpenerCap` by hand and the tier1 scenario asserts neither the registration, the log nor the launcher key.

### [MEDIUM] Stale promote unpinned

**Location:** `internal/boardserve/server_test.go`
**Issue:** `testOps` sends only current revisions and no refused row sends a stale promote.

### [LOW] A removed entry's refusal is not shown

**Location:** `internal/boardengine/board.html` (`removedDrafts`)
**Issue:** `send` stores the message and removes the entry, but only slugs with a draft are rendered under `#removed`.

### [LOW] Retry window starts at the request

**Location:** `internal/boardengine/board.html` (`request`)
**Issue:** The long poll's own wait of up to fifty seconds counts against the retry minute, and a page that gives up stops polling for good.

### [LOW] Record warning dropped

**Location:** `internal/boardserve/write.go` (`runWrite`)
**Issue:** The CLI prints the warning that a Sketch's record may not cover its changed body; the page answers plain success.

### [LOW] Spawn decided once

**Location:** `internal/boardcli/open.go` (`runOpen`)
**Issue:** A lock that falls free during the wait never leads to a spawn, although D11 says an `open` during an exit starts a fresh server.

### [LOW] Overview omits the viewer

**Location:** `docs/overview.md:464`
**Issue:** The diff adds only a package-tree line and the VS Code key text, not the board module's new verbs and outputs.

### [LOW] Stale spared-roles comment

**Location:** `internal/runproc/end.go:98`
**Issue:** The comment claims every hub-wide role is spared.

### [LOW] Server route and field gaps

**Location:** `internal/boardserve/server_test.go`
**Issue:** Each behaviour in the summary can be removed with every test passing.

### [LOW] Probe and drain gaps

**Location:** `internal/boardserve/run_test.go`
**Issue:** `Probe` is only asserted to succeed, and removing the listener close before the lock release still passes.

### [LOW] Projection gaps

**Location:** `internal/boardengine/view_test.go`
**Issue:** The fixture never produces two of the sections and never asserts titles, briefs or project labels.

### [LOW] Orch strand row missing

**Location:** `internal/boardcli/serveopen_test.go`
**Issue:** Only the empty strand name is shown to pass the run-agent refusal.

### [LOW] Unreadable beam anchor untested

**Location:** `internal/fabricengine/hubworkspace_integration_test.go`
**Issue:** The degrade branch card 8 asks for has no row.

### [LOW] Redundant subtest

**Location:** `internal/boardcli/open_test.go`
**Issue:** `TheAddressHoldsNoToken` asserts nothing `LiveServer` does not.

## Rejected

- code-correctness F7 (the token sits at its final path before `hardenFile` runs): the window is inside the user's own hub directory, D9 asks for the mode and DACL on the written file, which it gets, and closing the window would need a change to the shared state writer for no shown exposure.
- code-correctness F8 (a draft keeps `kind: note` after a promote): kept, folded into F3's fix rather than listed on its own.
