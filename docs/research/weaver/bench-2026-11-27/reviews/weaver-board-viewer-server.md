---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: internal/boardcli/opener_linux.go:26
    summary: "The opener is waited on through cmd.Wait with no teardown log, which PATTERN-child-processes requires of every wait"
    fix: "In startOpener of both internal/boardcli/opener_linux.go and opener_windows.go, make the waiting goroutine keep the cmd.Wait error, log it with logger.Info (message such as \"boardcli: the opener exited\", with cmd.Args and the error), and then send it on exited, as internal/creelcli/spawn.go logs its daemon's exit."
    origin: handler
  - id: F2
    severity: MEDIUM
    class: design
    location: internal/boardcli/open.go:170
    summary: "A server that fails to start loses its error: spawnServer logs only stderr, while serve reports every failure on stdout, which goes to the null device"
    fix: "In spawnServer set cmd.Stdout = log beside cmd.Stderr = log, so the JSON error of a failed serve (config load, Register, Listen, NewServer, writeState) lands in the server log, and update the Spawn field and spawnServer comments to say both streams are appended. Pin it with an assertion in a test that reaches spawnServer, or in TestBoardServeScenario, that a serve which fails to start leaves its error in the log."
    origin: lens:code-correctness
  - id: F3
    severity: MEDIUM
    class: design
    location: internal/boardengine/board.html:354
    summary: "A successful release, draft, undraft or promote moves a dirty, outdated edit form onto the new revision, so its next Save overwrites another writer's change without a stale refusal"
    fix: "In write() of board.html, in the success branch for an action, set state.revision = answer.entry.revision only when state.revision === revision (the revision the action sent, so the form was built on the entry the action saw). Otherwise leave state.revision as it is, so the next Save is refused as stale and shows the fresh entry."
    origin: lens:code-correctness
  - id: F4
    severity: MEDIUM
    class: design
    location: internal/boardserve/server_test.go:140
    summary: "Server behaviours with no coverage: the server's own Origin accepted, a stale promote, a listed decision file and project design file, and a long poll with an empty after"
    fix: "Add rows to TestServer: a write carrying Origin http://127.0.0.1:4321 answers 200; a promote with an older revision answers 409 with the fresh entry and leaves board.json unchanged (StaleWritesAnswerAConflict); GET /file/ serves a decision file of a discussed entry with its record on disk, and a project's design file, and the view names that decision_file (FileRouteServesOnlyListedFiles); a GET /api/board?after= on a board with no board.json and a short bound answers unchanged (LongPoll)."
    origin: lens:code-tests
  - id: F5
    severity: MEDIUM
    class: design
    location: internal/boardcli/cli.go:971
    summary: "Verb wiring with no coverage: the keep-warm cap chosen by strand name, and serve registering RoleBoardServer under HubOwner"
    fix: "Move the cap selection of openCmd into one function of the strand name and config dirs that openCmd calls, and test that an empty strand gives zero and an orch strand gives a non-zero cap. In TestBoardServeScenario, after the first open, read the run-process records of runproc.HubOwner under fabricengine.RunProcDir(hub.Path) and assert one with role runproc.RoleBoardServer and the server's pid."
    origin: lens:code-tests
  - id: F6
    severity: LOW
    class: design
    location: internal/boardengine/board.html:565
    summary: "The action buttons have no pending guard, so a double click sends the op twice and the page reports its own successful write as a stale refusal"
    fix: "In board.html keep a set of slugs with an action in flight: the action button's handler returns when its slug is in it, adds the slug before write() and removes it when the promise settles, as the forms do with state.pending."
    origin: lens:code-correctness
  - id: F7
    severity: LOW
    class: consistency
    location: internal/runproc/end.go:99
    summary: "The EndSubject and subjectPassSpared comments say the pass spares the roles that serve the whole hub, which the new hub-wide RoleBoardServer makes false"
    fix: "Reword both comments in internal/runproc/end.go (near lines 87 and 99) so they name what is spared without claiming every hub-wide process is, for example by saying the board server is ended with the subject's own processes."
    origin: lens:code-affected-set
  - id: F8
    severity: LOW
    class: design
    location: internal/fabricengine/hubworkspace_integration_test.go:566
    summary: "No test covers a beam whose anchor cannot be read, whose workspace file must still be written without hook members"
    fix: "Add a row to TestRegenerateHubWorkspace that installs a hook returning a tasks member, makes a beam's anchor unreadable to lymcwd.NamedWorktree, and asserts the regeneration succeeds, the beam file is written without a tasks key and the hub file carries it."
    origin: lens:code-tests
  - id: F9
    severity: LOW
    class: design
    location: internal/boardengine/view_test.go:21
    summary: "The projection's waits_on slug fallback for a dependency the board holds no handle for has no coverage"
    fix: "Give one task of viewFixture, or of a case in TestRenderBoardData, a dependency the board does not hold, and assert its waits_on holds that slug."
    origin: lens:code-tests
  - id: F10
    severity: NIT
    class: design
    location: internal/boardengine/board.html:144
    summary: "A link whose target is a code span renders href=\"0\", because safeTarget trims the placeholder's NULs before the NUL guard looks for them"
    fix: "In inline(), return stash(all) when the raw target holds \\u0000, before calling safeTarget, and drop the check on href."
    origin: lens:code-correctness
---

Review of `git diff 24688edd65fd25d24ec61ff248969ae2b63456d7..HEAD` against the decision record and the nine plan cards.
Nothing was built or run: the verify gate owns build and test results, and the viewer findings come from reading the page's script.

### [BLOCKING] The opener's wait is not logged

**Location:** `internal/boardcli/opener_linux.go:26`, `internal/boardcli/opener_windows.go:26`
**Issue:** `pattern/PATTERN-child-processes.md` says a path that starts a process logs its spawn "and logs its teardown wherever it waits for one".
Only a spawn reaped by `proc.Reap` is excused.
The opener is waited on through `cmd.Wait`, and neither that goroutine nor `openLauncher` logs the exit.
The added entry that chooses `cmd.Wait` settles how the wait happens, not its logging.
An opener left running at the cap leaves no trace of how it ended.

### [MEDIUM] A failed server start leaves no cause

**Location:** `internal/boardcli/open.go:170`
**Issue:** `spawnServer` sets `cmd.Stderr` only.
`serve` and the board pre-run report failures with `outputError` and `output.Err`, which write to stdout.
`open` then waits out its bound and sends the operator to a log holding at most a pid line.
A failure before `Run` takes the lock leaves the former server's pid at the top of the log, and the refusal tells the operator to end that process.

### [MEDIUM] An action silently rebases a dirty edit form

**Location:** `internal/boardengine/board.html:354`
**Issue:** The operator types in the edit form, another writer changes the same field, and the live refresh leaves the dirty form on the old revision.
A press of `release` or `draft` then sends the current revision, succeeds, and sets `state.revision` to the answered one.
Save now passes the check and overwrites the other writer's field with no fresh entry shown, which D8's stale refusal exists to stop.

### [MEDIUM] Server behaviours without a test row

**Location:** `internal/boardserve/server_test.go:140`
**Issue:** No test sends the server's own `Origin`, which a browser puts on every page write.
No test sends a stale promote, asserts a named `decision_file`, or fetches a decision file or project design file.
No test long-polls with an empty `after`, so the busy loop on a board without `board.json` could return unnoticed.

### [MEDIUM] Verb wiring without a test row

**Location:** `internal/boardcli/cli.go:971`
**Issue:** `TestRunOpen` injects `OpenerCap`, so nothing fails if `openCmd` stops setting it for a strand-named caller.
`TestRun` injects `Register` and the runproc test plants its own record, so nothing fails if `serve` registers under another role or owner.

### [LOW] Action buttons can send twice

**Location:** `internal/boardengine/board.html:565`
**Issue:** A second click before the first answer sends the same revision again, and the 409 message then stays beside an action that succeeded.

### [LOW] runproc comments no longer hold

**Location:** `internal/runproc/end.go:99`
**Issue:** Both comments say the subject pass spares the processes that serve the whole hub, and the board server is one that it ends.

### [LOW] Unreadable beam anchor is untested

**Location:** `internal/fabricengine/hubworkspace_integration_test.go:566`
**Issue:** The new Warn-and-continue branch of `regenerateHubWorkspace` has no row.

### [LOW] waits_on slug fallback is untested

**Location:** `internal/boardengine/view_test.go:21`
**Issue:** Every dependency in the fixture has a handle.

### [NIT] A code-span link target renders as a link to "0"

**Location:** `internal/boardengine/board.html:144`
**Issue:** The stashed placeholder is a NUL, an index and a NUL; `safeTarget` trims both NULs as control characters and returns the index.

## Rejected

- The server drops `RecordWarning` on promote and edit (code-correctness F3): the plan's cards 3 and 4 pin the success answer as `ok` and `entry`, and D10's "as the CLI's does" concerns the write.
- A write whose response is lost is retried and answered as stale (code-correctness F4, retry half): D11 asks that a failed page request be retried, and the revision check keeps the resend from writing twice.
- `open` does not spawn when the lock holder vanishes after the first read (code-correctness F5): D5 settles the held-lock path as wait, then refuse, and the refusal's second run starts a server.
- No test for `serve` emptying the strand name (code-tests F7): run agents are refused first, and an orch name and an empty name write alike, so no outcome of the page's operations differs.
- The server's own Origin, stale promote, empty `after` and decision-file gaps as separate findings (code-tests F1 to F4): merged into F4.
- The two verb wiring gaps as separate findings (code-tests F5 and F6): merged into F5.
- Unrequested or unbuilt scope: the code-scope lens found none, and I found none.
