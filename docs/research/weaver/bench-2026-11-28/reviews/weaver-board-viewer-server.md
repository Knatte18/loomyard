---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: internal/boardcli/opener_linux.go:25
    summary: The opener is waited on through cmd.Wait but its exit is never logged, against PATTERN-child-processes
    fix: "In startOpener in both internal/boardcli/opener_linux.go and opener_windows.go, replace the goroutine body with one that takes `err := cmd.Wait()`, logs the teardown with `logger.Info(\"boardcli: the page opener exited\", \"opener\", openerName, \"error\", err)` and then sends err on exited. logger is already imported in both files."
    origin: lens:code-affected-set
  - id: F2
    severity: BLOCKING
    class: consistency
    location: internal/vscode/workspace.go:89
    summary: The diff repeats the bare "lym" fallback and the "2.0.0" tasks version as literals already in config.go, against PATTERN-code-conventions
    fix: "Declare two unexported constants in internal/vscode/config.go (package vscode, the same package as workspace.go), one for the bare command name \"lym\" and one for the tasks schema version \"2.0.0\". Use them at config.go:72 and config.go:188 and at workspace.go:89 and workspace.go:92."
    origin: lens:code-affected-set
  - id: F3
    severity: MEDIUM
    class: design
    location: internal/boardengine/board.html:500
    summary: A one-click action moves an edit form's held revision forward, so a later Save overwrites another writer's change without a stale refusal
    fix: "In act() in board.html, delete the `held.revision = answer.entry.revision` line of the refusal branch, and in the success branch advance it only when the form was current when the action was sent: `if (held && held.dirty && held.revision === entry.revision) held.revision = answer.entry.revision;`. A clean form is then refilled by editForm, and a dirty form filled from an older revision is refused stale on Save and shows the fresh entry."
    origin: lens:code-correctness
  - id: F4
    severity: MEDIUM
    class: design
    location: internal/boardengine/board.html:328
    summary: When the project selected in an edit form leaves the board, the select reads back empty and Save moves the entry under the root
    fix: "In fillProjects in board.html, after the options are added, when the select held a non-empty value other than keepProject that no option carries, add an option with that value as both its value and its label (for example the slug followed by `(gone)`) before `select.value = value`. The form then keeps the operator's project, and the board judges a changed one, instead of the form sending `project: \"\"`. The guard must not fire for a newly built select, whose value is empty."
    origin: lens:code-correctness
  - id: F5
    severity: MEDIUM
    class: design
    location: internal/boardengine/board.html:431
    summary: The edit form prefills an unmet project dependency as its handle, which the board refuses as a dangling dependency on save
    fix: "In makeEditForm, resolve each waits_on handle against the projects as well as the entries: search `board.entries.concat(board.projects)` for `other.handle === name` and use its slug. Bound, to keep in mind: view.Projects holds no done project, so an unmet done project still shows as its handle; the projection would need the dependency slugs to cover it."
    origin: lens:code-correctness
  - id: F6
    severity: MEDIUM
    class: design
    location: internal/boardengine/board.html:259
    summary: A 422 on GET /api/board ends the follow loop for good, with no retry and no lym board open line
    fix: "In request() in board.html, treat 409, 413 and 422 as answers only for a write (`options.method === \"POST\"`), so a GET answered 422 is retried with backoff for the bounded time and then makes follow() call serverLost()."
    origin: lens:code-correctness
  - id: F7
    severity: MEDIUM
    class: design
    location: internal/boardcli/spawnserver.go:34
    summary: The spawned server's failure reasons go to stdout, which is discarded, so the log open points at holds no cause
    fix: "In spawnServer set `cmd.Stdout = logFile` beside `cmd.Stderr = logFile`. serve reports its pre-run and Run errors as a JSON envelope on the command's output, which then reaches the log. Update spawnServer's doc comment to say both streams are appended."
    origin: lens:code-correctness
  - id: F8
    severity: MEDIUM
    class: scope
    location: internal/boardserve/write.go:118
    summary: The write route creates a project when a create carries kind project, which D10 and the Scope section leave out
    fix: "In Server.create, before anything else, refuse a `kind` field that is present and equals neither boardengine.KindTask nor boardengine.KindNote, with an error naming the two kinds and the way forward (make a project with lym board upsert). Add a create row with kind project and an unused slug to the RefusedWritesChangeNothing table in internal/boardserve/server_test.go."
    origin: handler
  - id: F9
    severity: MEDIUM
    class: scope
    location: cmd/lym/testonlycode_linux_test.go:47
    summary: A leftover comment licenses temporary allowlist entries, which no entry uses and PATTERN-test-only-code does not admit
    fix: Delete the two comment lines at cmd/lym/testonlycode_linux_test.go:47-48, restoring the file to its text at the review base.
    origin: lens:code-scope
  - id: F10
    severity: MEDIUM
    class: design
    location: internal/boardserve/server_test.go:189
    summary: No test sends a write carrying the server's own Origin, which a browser sends on every page write
    fix: "Add an `Origin: http://127.0.0.1:4321` header to the POST requests the fixture's serve helper builds (testPort is 4321). ForeignOrigin still overrides it through its change func, and every successful write row then pins that the own origin is accepted."
    origin: lens:code-tests
  - id: F11
    severity: MEDIUM
    class: design
    location: internal/boardserve/server_test.go:381
    summary: No test reaches a stale promote, so dropping its revision check would pass the suite
    fix: "Add a stale promote row to the StaleWritesAnswerTheFreshEntry table: take a-note's revision before an UpsertTask that retitles it (beside the other setup writes, before `before` is read), then expect opPromote with the older revision to answer 409 with a-note's fresh entry. The CreateOfAnExistingSlug row on a-note keeps working, since it compares against the current entry."
    origin: lens:code-tests
  - id: F12
    severity: MEDIUM
    class: design
    location: internal/boardcli/serve_linux_integration_test.go:388
    summary: Nothing pins that serve registers itself in runproc as the board server under the hub owner
    fix: "In TestBoardServeScenario, after the state is read, assert `runproc.IsRegistered(fabricengine.RunProcDir(h.Path), runproc.HubOwner, runproc.RoleBoardServer, current.PID)` returns true with no error."
    origin: lens:code-tests
  - id: F13
    severity: LOW
    class: design
    location: internal/boardengine/board.html:301
    summary: The fresh-entry panel omits the project and the dependencies, both editable, so another writer's change to them is not shown before a resend
    fix: "In freshPanel add a line naming the entry's project (find it in board.projects by handle, falling back to the handle) and, for a task, a line listing entry.waits_on."
    origin: lens:code-scope
  - id: F14
    severity: LOW
    class: design
    location: internal/boardengine/board.html:502
    summary: A one-click action on a removed entry drops its refusal unseen, since the entry the refusal is shown under is removed
    fix: "In act(), when the refusal carries `answer.entry === null`, pass answer.error to showNotice so the message is shown as text outside the removed entry."
    origin: handler
  - id: F15
    severity: LOW
    class: design
    location: internal/boardserve/write.go:133
    summary: Page edits and promotes drop the board's RecordWarning, so a body edit of a discussed entry never says the stored record lacks it
    fix: "Have upsert and promote return the result's RecordWarning (UpsertResult.RecordWarning and PromoteResult.RecordWarning) through applyWrite, add it as a `warning` key of the 200 answer in serveWrite when non-empty, and show it as text on the page after a successful edit, create or promote."
    origin: lens:code-correctness
  - id: F16
    severity: LOW
    class: design
    location: internal/boardcli/viewer.go:186
    summary: When the lock holder goes away while open waits on it, open never starts a server and tells the operator to end a process that is gone
    fix: "In runOpen and awaitServer, when a lock first seen held is found free during the wait, start a server once (the same Prime and Spawn path as a free lock, reporting Started true) and wait for it within a fresh bound. Add a TestRunOpen row where the held lock is released mid-wait."
    origin: lens:code-correctness
  - id: F17
    severity: LOW
    class: design
    location: internal/boardserve/server_test.go:445
    summary: The file route is tested only with an entry's design file, not a decision file or a project's design file
    fix: "In a subtest of its own, seed a project with a body and an entry with a stored decision record (Board.StoreDecisionRecord; its preconditions for an entry were not read) and assert GET /file/ of each listed name answers 200."
    origin: lens:code-tests
  - id: F18
    severity: LOW
    class: design
    location: internal/boardserve/server_test.go:205
    summary: The policy's default-src and connect-src directives are asserted by no test
    fix: "In PageCarriesItsOwnPolicy also assert the policy holds `default-src 'none';` and `connect-src 'self';`."
    origin: lens:code-tests
  - id: F19
    severity: LOW
    class: design
    location: internal/boardserve/run_test.go:301
    summary: Removing the leftover state file right after the lock is pinned by no test
    fix: "In ServesOverALeftoverState, set s.options.Register to a func that records whether ReadState finds a state and still counts the registration, then assert it found none. Register runs after the removal in Run."
    origin: lens:code-tests
  - id: F20
    severity: LOW
    class: design
    location: internal/boardserve/run_test.go:464
    summary: No test pins that an exiting server takes no new request once its lock is free
    fix: "In ExitAnswersWaitersAndFinishesTheWriteInFlight, after wantLive(false) and before the board lock is released, assert that s.send of GET routeBoard returns an error."
    origin: lens:code-tests
  - id: F21
    severity: LOW
    class: design
    location: internal/boardcli/viewer.go:114
    summary: The choice between the keep-warm cap for a strand name and no cap without one has no test
    fix: "Move the strand branch of openBoardPage into a function in viewer.go, `openerCap(dirs configengine.Dirs, strand string) time.Duration`, keeping the fallback debug log, and add TestRunOpen rows asserting zero for an empty strand and a positive cap for a strand name. That shuttleengines.KeepWarm answers a positive cap over an empty configengine.Dirs was read only for its fallback path, not run."
    origin: lens:code-tests
  - id: F22
    severity: LOW
    class: design
    location: internal/fabricengine/hubworkspace_integration_test.go:743
    summary: No test pins that a beam whose anchor cannot be read still gets a workspace file with no hook member
    fix: "Add a TestRegenerateHubWorkspace row that makes lymcwd.NamedWorktree fail for the beam, installs a content hook, and asserts the beam file is written without the hook's member while the hub file carries it. How to make NamedWorktree fail was not established; read internal/lymcwd's anchor read to find the fixture."
    origin: lens:code-tests
  - id: F23
    severity: LOW
    class: consistency
    location: plugins/ly/skills/board/SKILL.md:12
    summary: The rule against reading board files directly names the JSON files and the README, not the new board-data.js, which holds every entry
    fix: "Extend the sentence to name the other rendered files in the Bolt checkout, board-data.js and board.html, among what is never read, grepped or edited directly."
    origin: lens:code-affected-set
  - id: F24
    severity: NIT
    class: design
    location: internal/boardserve/write.go:126
    summary: create passes the request's revision to the write section instead of expecting no entry, as card 4 states
    fix: "In Server.create set `request.Revision = \"\"` before calling s.upsert(request)."
    origin: lens:code-correctness
  - id: F25
    severity: NIT
    class: consistency
    location: internal/boardengine/render.go:623
    summary: The renderFiles comment says stored holds the same entries as stored
    fix: "Change that line to say stored holds the same entries as rendered."
    origin: handler
  - id: F26
    severity: NIT
    class: consistency
    location: internal/boardcli/cli.go:50
    summary: The comment on config names only the labels verb as its reader, though serve now reads it too
    fix: "Drop the clause naming the labels verb, leaving the comment to say config is the Config PersistentPreRunE loaded."
    origin: lens:code-affected-set
---

# Code review, round 1

The diff builds every decision of the record.
It is blocked on two repository-rule violations.
The findings that matter most beyond those are in the viewer page, where an edit can overwrite or move data without telling the operator (F3, F4).

This review read the code and its tests and drove nothing: no gate, build or browser run was made.

### [BLOCKING] The opener's exit is not logged

**Location:** `internal/boardcli/opener_linux.go:25`, and the same line of `opener_windows.go`

**Issue:** `PATTERN-child-processes` requires a spawn to log its teardown wherever it waits for one.
Its spawn-only exemption covers `proc.Reap` spawns, and the opener waits through `cmd.Wait`.
The added decision on `cmd.Wait` settles the wait, not the log.

### [BLOCKING] Task literals are repeated

**Location:** `internal/vscode/workspace.go:89` and `:92`

**Issue:** `PATTERN-code-conventions` asks for one named constant for a name that can go stale, never a repeated literal.
`config.go:72` and `:188` already spell the same fallback command and tasks version.

### [MEDIUM] An action advances an edit form past a change it never showed

**Location:** `internal/boardengine/board.html:500-506`

**Issue:** A dirty form filled at one revision keeps it while another writer changes the entry and the long poll delivers the change.
A one-click action then succeeds against the new revision and hands the form the revision after it, so Save overwrites the other writer's field with no refusal.
On a refused action a clean form takes the fresh revision, so `editForm` does not refill it, and its old text is saved over the newer one.
D8 requires that stale write to be refused.

### [MEDIUM] A vanished project option moves the entry to the root

**Location:** `internal/boardengine/board.html:328-337`

**Issue:** `select.value = value` with no matching option reads back as `""`, which differs from the form's initial value.
Saving another field then sends `project: ""`.
It also happens to a clean form whose entry's revision did not change, since that form is refilled in place rather than rebuilt.

### [MEDIUM] Project dependencies are prefilled as handles

**Location:** `internal/boardengine/board.html:431-433`

**Issue:** `waits_on` handles are looked up in `board.entries` only, so a dependency on a project stays `P<n>`.
Replacing the dependency list then sends the handle as a slug and is refused.

### [MEDIUM] One failed board read freezes the page

**Location:** `internal/boardengine/board.html:259` and `:285-296`

**Issue:** A 422 from `GET /api/board` is returned as an answer, so `follow` exits its loop.
D11 asks for retry with backoff and then the `lym board open` line.

### [MEDIUM] Server start failures are discarded

**Location:** `internal/boardcli/spawnserver.go:34`

**Issue:** Only stderr reaches the server log, while `serve` writes its errors as an envelope on stdout.
A server that fails before taking the lock also leaves the previous server's pid at the top of the log, which `open`'s refusal tells the operator to end.

### [MEDIUM] The page's write route creates projects

**Location:** `internal/boardserve/write.go:118`

**Issue:** `kind` is an allowed create field with no value check, and the board accepts a project upsert.
D10 lists creating a task or a note, and Scope puts project creation out.
The answer then carries a null entry, since a project is no entry of the projection.

### [MEDIUM] A rule for temporary allowlist entries was left behind

**Location:** `cmd/lym/testonlycode_linux_test.go:47-48`

**Issue:** The temporary entries of the early commits are gone, and the comment that licensed them remains.

### [MEDIUM] The own-origin write has no test

**Location:** `internal/boardserve/server_test.go:189`

**Issue:** Only a foreign `Origin` is sent, so a wrong own-origin string would refuse every browser write with the suite green.

### [MEDIUM] Stale promote has no test

**Location:** `internal/boardserve/server_test.go:381`

**Issue:** The stale table holds edit, release and create, and the one promote row uses the current revision.

### [MEDIUM] Server registration has no test

**Location:** `internal/boardcli/serve_linux_integration_test.go:388`

**Issue:** The lifecycle test fakes `Register`, the `runproc` test plants the record, and the scenario never reads it.
Without the registration a hub takeover leaves the server running.

### [LOW] The fresh panel hides two editable fields

**Location:** `internal/boardengine/board.html:301-310`

**Issue:** Project and dependencies are absent, while the form's revision is already replaced with the fresh one.

### [LOW] A refusal on a removed entry is never shown

**Location:** `internal/boardengine/board.html:502-507`

**Issue:** `act` stores the refusal under the slug and removes the entry that would render it.
D10 requires each refusal's message to be shown.

### [LOW] Record warnings are dropped

**Location:** `internal/boardserve/write.go:133` and `:146`

**Issue:** The results carrying `RecordWarning` are discarded, where the CLI prints it.

### [LOW] A holder that exits mid-wait is not replaced

**Location:** `internal/boardcli/viewer.go:186-204`

**Issue:** `awaitServer` keeps polling a free lock until the bound, then names a hung process that no longer exists.
D5 treats a free lock as no server.

### [LOW] File route covers one of three file kinds

**Location:** `internal/boardserve/server_test.go:445`

**Issue:** Dropping decision files or project design files from the listed set would pass.

### [LOW] Two policy directives are unasserted

**Location:** `internal/boardserve/server_test.go:205`

**Issue:** `default-src` and `connect-src` can be lost or widened with the test green.

### [LOW] Leftover state removal is unpinned

**Location:** `internal/boardserve/run_test.go:301`

**Issue:** The later state write overwrites the leftover, so deleting the removal passes.

### [LOW] Refusing new requests on exit is unpinned

**Location:** `internal/boardserve/run_test.go:464`

**Issue:** The test checks the lock is free during the drain, not that a new request fails.

### [LOW] Opener cap selection is untested

**Location:** `internal/boardcli/viewer.go:114`

**Issue:** `TestRunOpen` injects `OpenerCap`, so the strand-name branch of D5 is never run.

### [LOW] Unreadable beam anchor branch is untested

**Location:** `internal/fabricengine/hubworkspace_integration_test.go:743`

**Issue:** Card 8's degraded beam file has no row.

### [LOW] The board skill's read ban misses the new files

**Location:** `plugins/ly/skills/board/SKILL.md:12`

**Issue:** `board-data.js` holds every entry and body, and the rule does not name it.

### [NIT] create forwards a revision

**Location:** `internal/boardserve/write.go:126`

**Issue:** A create sent with a revision for an absent slug is refused as removed.

### [NIT] Self-referring comment

**Location:** `internal/boardengine/render.go:623`

**Issue:** The sentence compares `stored` with itself.

### [NIT] Stale reader named in a comment

**Location:** `internal/boardcli/cli.go:50`

**Issue:** The comment names one reader of `config`, and there is now another.
