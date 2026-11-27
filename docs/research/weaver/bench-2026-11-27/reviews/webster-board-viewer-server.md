---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    location: internal/boardengine/board.html:241
    summary: The edit form fills depends_on from the unmet dependencies only, so changing the field silently deletes every met dependency of the task.
    fix: "Give the form the full stored list. Add a depends_on field (the entry's whole depends_on, as slugs) to ViewEntry in internal/boardengine/view.go, set it in viewEntry, name it in the doc.go projection text if it lists fields, and fill the form's depends_on in valuesOf from it instead of entry.waits_on.map(slugOfHandle). Keep waits_on for the facts line. Add a TestRenderBoardData assertion that a task depending on one done and one open entry carries both slugs in the new field."
    origin: lens:code-correctness
  - id: F2
    severity: BLOCKING
    class: design
    location: internal/boardserve/run.go:170
    summary: The watch loop wakes at once on every event of any file in the board directory, breaking the one-second floor of PATTERN-daemon-wakeups (carried-over watch-loop-no-one-second-floor).
    fix: "In watchBoard, never start an iteration within watchBase of the previous one: after an event, wait out the rest of that second through opts.After (still honouring ctx.Done), coalescing the events that arrive meanwhile, then read the revision once. Pass the name filter to the watcher (opts.Watch(opts.BoardPath, <board.json name exported by boardengine>)) as D11 and card 5 say, and word the watchBoard comment, boardserve/doc.go and the pattern/PATTERN-daemon-wakeups.md bullet to match, stating the floor. Drive the TestRun watch row through the injected clock so it pins that a burst of events yields one wake per second."
    origin: lens:code-affected-set
  - id: F3
    severity: BLOCKING
    class: design
    location: internal/boardcli/opener_linux.go:25
    summary: The opener is waited on with cmd.Wait but its exit is never logged, though PATTERN-child-processes requires a teardown log wherever a spawn is waited on.
    fix: "In the waiting goroutine of startOpener, in both opener_linux.go and opener_windows.go (line 25), call err := cmd.Wait(), log the exit with logger.Info (opener name and error), then send err on the channel."
    origin: lens:code-affected-set
  - id: F4
    severity: MEDIUM
    class: design
    location: internal/boardengine/board.html:400
    summary: After a release, draft or undraft with a dirty form, the panel takes the new revision, so the next Save overwrites another writer's change with no stale refusal.
    fix: "In send, do not advance panel.revision of a dirty panel past a change it never showed: take answer.entry.revision only when the action's request revision (body.revision) equals panel.revision. Otherwise leave panel.revision as it is, so the next Save is refused as stale and shows the fresh entry."
    origin: lens:code-correctness
  - id: F5
    severity: MEDIUM
    class: design
    location: internal/boardengine/board.html:293
    summary: When an entry's kind changes (another writer or the page's own promote), panelFor builds a new panel and discards the unsent edit, its message and the fresh block.
    fix: "In panelFor, when the existing panel is dirty and the kind differs, keep the typed values: build the new panel's inputs, copy each dirty control's value, its baseline and the old revision into it, and carry over the message and fresh nodes. Only a clean panel is rebuilt from scratch."
    origin: lens:code-correctness
  - id: F6
    severity: MEDIUM
    class: design
    location: internal/boardengine/board.html:342
    summary: The fresh entry shown on a stale refusal omits project and dependencies, so a conflict in either is invisible beside the unsent edit.
    fix: "In showFresh add a project line (slugOfHandle of entry.project, root for the root handle) and, for a task, a depends_on line from the full dependency list F1 adds."
    origin: handler
  - id: F7
    severity: MEDIUM
    class: design
    location: internal/boardengine/board.html:275
    summary: A body with CRLF, or a title or brief with a newline, differs from its browser-normalised control value, so the panel is dirty from the start, never refreshes and rewrites the field on Save.
    fix: "In fill, after assigning the controls, record as baseline what each control reports back (panel.baseline[name] = control.value for every control just filled), not the raw entry value."
    origin: lens:code-correctness
  - id: F8
    severity: MEDIUM
    class: design
    location: internal/boardserve/write.go:117
    summary: create accepts fields.kind project, so a POST creates a project, which D10 limits to tasks and notes and Scope excludes.
    fix: "In applyWrite, before any write, refuse a create whose fields.kind is boardengine.KindProject (and any kind other than task or note that is given), and refuse an edit whose slug is among view.Projects, each naming the way forward (lym board upsert). Add both as rows of the refused-and-changing-nothing table in server_test.go."
    origin: lens:code-scope
  - id: F9
    severity: MEDIUM
    class: design
    location: internal/boardcli/open.go:137
    summary: The spawned server's stderr alone goes to the log while serve reports its failures on stdout, so why it failed to start is lost and open tells the operator to end a hung process.
    fix: "In the serve RunE (internal/boardcli/cli.go), also write a Run error to os.Stderr; in spawnServer set cmd.Stdout = logFile as well as cmd.Stderr, so the PersistentPreRunE's config and resolve errors reach the server log."
    origin: lens:code-correctness
  - id: F10
    severity: MEDIUM
    class: design
    location: .gitattributes:22
    summary: The new go:embed target board.html is not pinned to LF, so the embedded and committed bytes depend on core.autocrlf (carried-over board-html-not-pinned-to-lf).
    fix: "Add the line `internal/boardengine/board.html text eol=lf` to .gitattributes beside the other go:embed targets."
    origin: lens:code-affected-set
  - id: F11
    severity: MEDIUM
    class: design
    location: internal/boardcli/cli.go:463
    summary: "Verb wiring D5, D11 and D12 rely on has no test: strand name to keep-warm cap, the runproc registration, and an orch name proceeding (carried-over verb-wiring-behaviors-untested)."
    fix: "Add to TestBoardServeScenario a read of the hub's runproc records asserting one RoleBoardServer record under HubOwner. Add to TestServeOpenRefusals an orch-strand row that reaches the --board-path refusal. Pin the cap wiring by moving the strand-to-OpenerCap choice (cli.go:463-465) into a function that runOpen's production deps call and testing it with a strand name set and empty."
    origin: lens:code-tests
  - id: F12
    severity: MEDIUM
    class: design
    location: internal/boardengine/view_test.go:182
    summary: "The projection's displayed values are barely pinned: no row asserts a decoded body, title, brief or handle, or a named decision_file."
    fix: "In TestRenderBoardData assert from literals the decoded body of the </script> entry, an entry's title, brief and handle, a project's title, brief and labels, and decision_file == decision-<slug>.md for a discussed entry on the Render path."
    origin: lens:code-tests
  - id: F13
    severity: MEDIUM
    class: design
    location: internal/boardserve/server_test.go:442
    summary: The file route's decision-file and project design-file branches have no test.
    fix: "Add to TestServer/Files a discussed entry's decision file and a project's design file, each asserted to be served with its bytes."
    origin: lens:code-tests
  - id: F14
    severity: MEDIUM
    class: design
    location: internal/boardserve/run_test.go:53
    summary: The watch timer's doubling from one to eight seconds and its reset on a change are unpinned, since the fake After ignores the duration.
    fix: "Have stepClock.After record each requested duration and add a TestRun row asserting 1s, 2s, 4s, 8s, 8s, then 1s after a board change."
    origin: lens:code-tests
  - id: F15
    severity: LOW
    class: design
    location: internal/boardserve/write.go:140
    summary: The page's promote drops PromoteTo's RecordWarning, so a Sketch whose body outran its record is released with no warning.
    fix: "Return the PromoteResult's RecordWarning from promote through applyWrite, answer it as a warning key on success, and show it as text in the panel's message in board.html send. Add a server_test.go row."
    origin: lens:code-correctness
  - id: F16
    severity: LOW
    class: design
    location: internal/boardserve/run.go:132
    summary: A failure of serving.Serve is swallowed; the log says only that the server exits as stopped.
    fix: "In the Serve goroutine, write the error to opts.Log before calling stopWatching."
    origin: lens:code-correctness
  - id: F17
    severity: LOW
    class: design
    location: internal/boardcli/open.go:75
    summary: When the lock holder exits while open waits, open never spawns, and after the bound tells the operator to end a process that is gone.
    fix: "In runOpen's wait loop, when open spawned nothing yet and the lock now reads free, resolve the prime, spawn once, set Started, and go on waiting. Add a TestRunOpen row."
    origin: lens:code-correctness
  - id: F18
    severity: LOW
    class: design
    location: internal/boardengine/board.html:365
    summary: The retry bound is measured from the first attempt's start, so a long poll that fails late (after a suspend, say) gets little or no retry before the page gives up.
    fix: "In request, set started at the first failure instead of before the first attempt."
    origin: handler
  - id: F19
    severity: LOW
    class: design
    location: internal/boardserve/server_test.go:155
    summary: "Untested server paths: release, draft and undraft of a project, a write targeting a project, a wrong method (405), an empty slug, and refusal of new requests once exit begins."
    fix: "Add the first four as rows of the existing TestServer tables, and in TestRun's exit-with-write-in-flight row assert Probe(server) fails while the write is still blocked."
    origin: lens:code-tests
  - id: F20
    severity: LOW
    class: design
    location: internal/fabricengine/hubworkspace.go:236
    summary: The branch for a beam whose anchor cannot be read (file written without hook members) has no test.
    fix: "Add a TestRegenerateHubWorkspace row with a beam whose anchor lymcwd.NamedWorktree cannot read, asserting the regeneration succeeds and that beam's file carries no hook member."
    origin: lens:code-tests
---

The review range is `932c0b3eb1ae2cc1fa4788cfee22166e3214c2ab..HEAD`.
No change description exists under `_lym/landing/`, and no PATTERN change is declared.
No build or test was run in this round; the findings come from reading the code.

Of the findings the decision record carries over from the earlier round:

- `watch-loop-no-one-second-floor` is still open (F2).
- `board-html-not-pinned-to-lf` is still open (F10).
- `verb-wiring-behaviors-untested` is judged still open in part (F11).
- `page-busy-loops-on-absent-board-json` is fixed: the page now sleeps a second between requests while the revision is empty.

### [BLOCKING] Editing dependencies deletes the met ones

**Location:** `internal/boardengine/board.html:241`
**Issue:** `valuesOf` fills the field from `entry.waits_on`, which `viewEntry` builds from `unmetDependencies`.
A task depending on a done `a` and an open `b` shows only `b`.
Adding `c` sends `["b","c"]`, and the upsert replaces the stored list, so `a` is lost with no message.

### [BLOCKING] Watch loop has no one-second floor

**Location:** `internal/boardserve/run.go:170`
**Issue:** An event falls straight through to a full iteration, and the watcher is opened with no name filter.
One board write renders many files there, so the loop hashes `board.json` and checks the binary many times within milliseconds.
`PATTERN-daemon-wakeups` allows at most one wake a second.
The back-to-back binary checks can also satisfy the two-sighting rule of `selfexec` while a deploy is still copying.

### [BLOCKING] Opener's exit is not logged

**Location:** `internal/boardcli/opener_linux.go:25`
**Issue:** Only the spawn is logged.
`PATTERN-child-processes` requires a teardown log wherever the starter waits, and only a `proc.Reap` spawn is excused.
`opener_windows.go` has the same line.

### [MEDIUM] Action on a dirty form hides another writer's change

**Location:** `internal/boardengine/board.html:400`
**Issue:** The form holds a typed title at revision R1 while another writer moves the entry to R2.
The `draft` button sends R2 and succeeds, the dirty panel takes the new revision, and Save then overwrites the other writer's title unrefused.

### [MEDIUM] Kind change discards the unsent edit

**Location:** `internal/boardengine/board.html:293`
**Issue:** A panel is reused only while the kind matches.
D10 requires the unsent edit to stay in its form.

### [MEDIUM] Fresh entry omits project and dependencies

**Location:** `internal/boardengine/board.html:342`
**Issue:** D10 shows the fresh entry beside the unsent edit, but two of the editable fields are missing from it.

### [MEDIUM] Normalised field values read as unsent edits

**Location:** `internal/boardengine/board.html:275`
**Issue:** A textarea turns CRLF into LF and a text input strips newlines, while the baseline keeps the raw value.

### [MEDIUM] The server's create makes projects

**Location:** `internal/boardserve/write.go:117`
**Issue:** `fields.kind` is passed to the upsert unchecked.
D10 lists create of "a task or a note", and Scope puts project create or edit out.

### [MEDIUM] Server start failures are lost

**Location:** `internal/boardcli/open.go:137`
**Issue:** `cmd.Stdout` is nil, and `serve` and its pre-run report errors on stdout.
A broken `board.yaml` leaves no cause in the log, and `open` refuses after its bound with advice about a hung process.

### [MEDIUM] board.html is not pinned to LF

**Location:** `.gitattributes:22`
**Issue:** The file's own header pins `go:embed` targets, and `git check-attr eol` answers `unspecified` for `board.html`.

### [MEDIUM] Verb wiring is untested

**Location:** `internal/boardcli/cli.go:463`
**Issue:** Removing the keep-warm cap wiring or the `runproc` registration fails no test, and no row shows an orch strand name passing.

### [MEDIUM] Projection values are barely asserted

**Location:** `internal/boardengine/view_test.go:182`
**Issue:** The escaping row asserts only absence, so it passes with the body dropped.
Only the empty `decision_file` case is asserted.

### [MEDIUM] File route branches untested

**Location:** `internal/boardserve/server_test.go:442`
**Issue:** Only an entry's design file is served by a test.

### [MEDIUM] Watch backoff untested

**Location:** `internal/boardserve/run_test.go:53`
**Issue:** A fixed or never-reset interval passes every test.

### [LOW] Promote warning dropped

**Location:** `internal/boardserve/write.go:140`
**Issue:** The command line's promote reports that a Sketch's body changed after its record was stored; the page reports plain success.

### [LOW] Serve error swallowed

**Location:** `internal/boardserve/run.go:132`
**Issue:** An accept error only cancels the watch, and `Run` returns nil.

### [LOW] open does not recover from a server exiting mid-wait

**Location:** `internal/boardcli/open.go:75`
**Issue:** A server closing its listener just before releasing the lock reads as held on the first check, and the loop never spawns once the lock is free.

### [LOW] Retry bound starts too early

**Location:** `internal/boardengine/board.html:365`
**Issue:** `started` is set before the first attempt, and a long poll attempt alone can last most of the bound.

### [LOW] Untested server paths

**Location:** `internal/boardserve/server_test.go:155`
**Issue:** The project-target, wrong-method, empty-slug and closed-listener behaviours have no row.

### [LOW] Unreadable beam anchor untested

**Location:** `internal/fabricengine/hubworkspace.go:236`
**Issue:** The degrade path has no row.

## Rejected

- code-affected-set F4, `docs/overview.md` board section silent on the new verbs: no card or repository rule asks for that text.
- code-correctness F9, a hand-run `serve` leaves the log without its pid: the plan sets stderr as the log, so the finding is against the answer key.
