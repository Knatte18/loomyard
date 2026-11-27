---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: scope
    location: internal/shuttleparity/tables.go:383
    summary: "The seat catalog never builds the unassisted burler reviewer: both SeatTables calls set Assist, so burler-template-reviewer is neither compared nor printable"
    fix: "In burlerSeatTables, also call engine.SeatTables for the solo and the fanned profile with RunOpts that set no Assist and no AssistFan (Round only), under RolePrefix values distinct from the assisted calls so no two catalog seats share a name. Keep the two assisted calls for the assistant seat and the assisted chair."
    origin: lens:code-scope
  - id: F2
    severity: BLOCKING
    class: consistency
    location: internal/shuttleengine/claudeengine/manifest.go:23
    summary: "PATTERN-code-conventions: the diff declares a constant for CLAUDE.local.md while the same package (capabilities.go:35) and heddle's target table (target.go:68) still spell the literal"
    fix: "Declare the file name once as an exported constant in internal/heddleengine (which renders the file and which claudeengine already imports). Use it in the claude-file row's File and ReRender in target.go, in InstructionFiles in claudeengine/capabilities.go and in claudeengine/manifest.go, and delete localInstructionFileName."
    origin: lens:code-affected-set
  - id: F3
    severity: BLOCKING
    class: consistency
    location: internal/heddleengine/target.go:68
    summary: "PATTERN-code-conventions: the diff adds second \"claude\" and \"codex\" literals to the target table beside the ones on the prompt rows"
    fix: "Declare one constant per engine name in internal/heddleengine/target.go and use it as the Engine of both the file row and the prompt row of that engine in targetTable."
    origin: lens:code-affected-set
  - id: F4
    severity: MEDIUM
    class: design
    location: internal/shuttleparity/tables.go:379
    summary: "Burler seats are cataloged only under the stand-in prefixes solo and fanned, so lym shuttle manifest refuses the strand role a real round carries, such as code-burler-review"
    fix: "Make the verb resolve a real burler strand role. Build the unassisted solo seats of F1 with an empty RolePrefix, so burler-review and burler-fix are catalog names. In LookupSeat, when no seat matches exactly, resolve a name of the form <prefix>-burler-review or <prefix>-burler-fix (and the assistant's role) to the catalog seat of that base role. State this in the verb's Long and add a TestReport row for a prefixed burler role."
    origin: lens:code-correctness
  - id: F5
    severity: MEDIUM
    class: design
    location: internal/shuttleengine/claudeengine/manifest.go:206
    summary: "An empty global or above-root CLAUDE.md is listed by Claude and never by Codex, so the parity verdict reports a missing item no delivery can supply"
    fix: "In nativeManifestItems, skip a setup.InstructionFiles entry whose Text is empty. Add a row to claudeengine's TestPrepare_WritesTheManifest with an empty global CLAUDE.md asserting no instruction-source item for it."
    origin: lens:code-correctness
  - id: F6
    severity: MEDIUM
    class: design
    location: internal/shuttleparity/seats_test.go:72
    summary: "The parity fixture exercises neither the refuse_raw guard row nor a pointer-delivered CLAUDE.md, so dropping either Codex delivery passes the parity tests"
    fix: "In newParityFixture write a hub gate config with a non-empty refuse_raw list. Also anchor the fixture below its repository root with a CLAUDE.md and an AGENTS.md at the root, so Codex receives that CLAUDE.md only through the instruction pointers. Assert in TestSeats that the Claude manifest holds the refuse-raw guard item and the root CLAUDE.md item, so the fixture cannot lose either silently."
    origin: lens:code-tests
  - id: F7
    severity: MEDIUM
    class: design
    location: cmd/lym/main.go:180
    summary: "No test runs the installed passage reader, so dropping its FileEntries loop or swapping passages with file entries passes every test"
    fix: "Add a test in cmd/lym that runs root's pre-run, then calls shuttleengine.StandingPassages over a fixture Bolt checkout whose heddle.json holds one agents-prompt and one agents-file entry, and asserts the slug, text and Engine of Passages and of FileEntries."
    origin: lens:code-tests
  - id: F8
    severity: MEDIUM
    class: design
    location: internal/shuttleengine/codexengine/manifest.go:118
    summary: "The read-back filter for text deliveries (passages, instruction files, listing, plugin contexts, notices) has no negative row in either engine, so listing every candidate passes"
    fix: "In each engine's manifest_test.go add a case that calls codexManifest or claudeManifest with candidates whose text is absent from the launch line or appended prompt, and assert that no item is listed for them."
    origin: lens:code-tests
  - id: F9
    severity: MEDIUM
    class: design
    location: internal/shuttleparity/parity_test.go:153
    summary: "Nothing asserts that PrepareEach appends the edit-directive item, so deleting the append passes every test"
    fix: "In TestCompare's PrepareEach rows, add one whose spec Prompt holds each engine's editdirective.Directive text and assert that each returned manifest holds the edit-directive item."
    origin: lens:code-tests
  - id: F10
    severity: MEDIUM
    class: design
    location: internal/shuttlecli/manifest.go:27
    summary: "The verb's production wiring is untested: building the set with shuttleengines.New, which runs plugin hooks and the sandbox probe against D10, fails no test"
    fix: "Add a tier0 test of worktreeManifestReport over a fixture hub with a plugin session-start hook that would leave a marker file, asserting that the report lists the plugin context as omitted with ErrHookUnrun's text and that no marker appears. In TestManifestCmd give one stubbed manifest a non-empty Verdict and assert the output carries no verdict key."
    origin: lens:code-tests
  - id: F11
    severity: LOW
    class: design
    location: internal/shuttleengine/claudeengine/manifest.go:201
    summary: "nativeManifestItems drops setup.Warnings, so a malformed plugins file or unreadable SKILL.md silently shrinks the reference manifest"
    fix: "After source.UserSetup in nativeManifestItems, log each entry of setup.Warnings with logger.Warn."
    origin: lens:code-correctness
  - id: F12
    severity: LOW
    class: design
    location: internal/shuttleparity/parity_test.go:135
    summary: "No row gives Unmet a spec with Requires, so ignoring spec.Requires passes every test"
    fix: "In TestCompare's Unmet subtest add a spec with Requires set to RequirementBackgroundWake and expect exactly that requirement."
    origin: lens:code-tests
  - id: F13
    severity: LOW
    class: design
    location: internal/shuttleengine/codexengine/manifest_test.go:19
    summary: "The record's Role and PromptEngine on either engine, and Codex's hand-written prompt key, are unpinned"
    fix: "Assert manifest.Role and manifest.PromptEngine in both engines' TestPrepare_WritesTheManifest, and add a HandWrittenPrompt row to the Codex table expecting prompt-engine/hand-written."
    origin: lens:code-tests
  - id: F14
    severity: LOW
    class: design
    location: internal/shuttleengine/claudeengine/manifest_test.go:19
    summary: "The Claude manifest's no-repository, anchor-below-root and user-setup-error branches have no row"
    fix: "Add three rows to claudeengine's TestPrepare_WritesTheManifest: an anchor with no .git above it (only instruction-source/CLAUDE.md of the working directory), an anchor below the root (both CLAUDE.md files by root-relative key), and an unreadable global CLAUDE.md (Prepare still succeeds)."
    origin: lens:code-tests
  - id: F15
    severity: LOW
    class: design
    location: internal/shuttleengine/codexengine/codexengine.go:297
    summary: "No test covers a failed manifest write refusing the launch, in either engine"
    fix: "Add a row to each engine's TestPrepare_WritesTheManifest that creates a directory at runDir/manifest.json and expects Prepare to fail naming that path."
    origin: lens:code-tests
  - id: F16
    severity: LOW
    class: design
    location: internal/shuttleengine/manifest_test.go:89
    summary: "Nothing pins that only a launch whose run record was saved keeps a manifest copy"
    fix: "Add a row to TestRunner_Start_KeepsTheManifestCopy in which the run record save fails after Prepare wrote the record, and assert that no manifests directory exists."
    origin: lens:code-tests
---

### [BLOCKING] The unassisted burler reviewer is in no catalog table

**Location:** `internal/shuttleparity/tables.go:383`

**Issue:** `opts.Assist` names a stand-in model for the solo call and stays set for the fanned one.
`probeTable` therefore always passes an assistant through `withAssistant`, which replaces the chair's stencil with the assisted one.
The reviewer a round with no assistant starts is never rendered, compared or printed.
The acceptance criterion names every seat in `seatengine`'s tables.

### [BLOCKING] CLAUDE.local.md is redeclared beside its literal spellings

**Location:** `internal/shuttleengine/claudeengine/manifest.go:23`

**Issue:** `PATTERN-code-conventions` asks for one constant, declared once and referenced everywhere.
The new `localInstructionFileName` leaves `claudeengine/capabilities.go:35` and `heddleengine/target.go:68` spelling the name themselves.

### [BLOCKING] Engine names are repeated literals in heddle's target table

**Location:** `internal/heddleengine/target.go:68`

**Issue:** The new `Engine` values on the two file rows repeat the literals of the two prompt rows.
`PATTERN-code-conventions` forbids a repeated literal, and no `Pattern-Departure` trailer covers this one.

### [MEDIUM] The verb refuses the burler roles real runs carry

**Location:** `internal/shuttleparity/tables.go:379`

**Issue:** Production rounds lead their burler roles with a recipe's `role_prefix`, or with none.
The catalog holds only `solo-…` and `fanned-…` names, so a role copied from a run's record is refused.
D10 takes a seat, and the verb's help says a seat is named by its strand role.

### [MEDIUM] An empty instruction file reads as a Codex gap

**Location:** `internal/shuttleengine/claudeengine/manifest.go:206`

**Issue:** `instructionFiles` returns a 0-byte file as found.
Claude lists it, while Codex's `inSetting("")` is false.
`lym shuttle manifest` then exits non-zero for every seat on that machine, with a way forward nobody can follow.

### [MEDIUM] The parity fixture leaves two deliveries unexercised

**Location:** `internal/shuttleparity/seats_test.go:72`

**Issue:** With no gate config, `shellguard.Rows` yields no refuse-raw row.
With the anchor at the repository root under a marked `AGENTS.override.md`, no `CLAUDE.md` reaches Codex through the pointers.
The acceptance criterion says dropping any one Codex delivery fails the test.

### [MEDIUM] The production passage reader is never run by a test

**Location:** `cmd/lym/main.go:180`

**Issue:** The existing pre-run test checks only that the reader is installed.
The engine and parity tests install readers of their own.

### [MEDIUM] The read-back filter for text deliveries is unpinned

**Location:** `internal/shuttleengine/codexengine/manifest.go:118`

**Issue:** The rows show items present, and absent only for guards, the nudge, a skill load and an unmarked file.
Replacing `inSetting` or `appended` with a non-empty check passes, which would hide a dropped delivery from the parity test.

### [MEDIUM] PrepareEach's edit-directive append is unpinned

**Location:** `internal/shuttleparity/parity_test.go:153`

**Issue:** Removing the append drops the item from both engines' manifests, so the comparison still passes and no row notices.

### [MEDIUM] The verb's no-process wiring is unpinned

**Location:** `internal/shuttlecli/manifest.go:27`

**Issue:** `TestManifestCmd` stubs the whole report.
D10's bounds (no plugin hook, no sandbox probe, no stored verdict in the output) rest on code no test runs or asserts.

### [LOW] Claude's user-setup warnings are discarded

**Location:** `internal/shuttleengine/claudeengine/manifest.go:201`

**Issue:** `UserSetup` reports these causes as warnings with a nil error, and only the error is logged.

### [LOW] Unmet is tested for forks alone

**Location:** `internal/shuttleparity/parity_test.go:135`

**Issue:** The driver's and the master's background-wake requirement could stop being reported unnoticed.

### [LOW] Record header fields are unpinned

**Location:** `internal/shuttleengine/codexengine/manifest_test.go:19`

**Issue:** The verb prints `role` and `promptEngine`, and no `Prepare` test reads either.

### [LOW] Three Claude manifest branches are untested

**Location:** `internal/shuttleengine/claudeengine/manifest_test.go:19`

**Issue:** Every row makes the anchor its own repository root with a readable user setup.

### [LOW] The manifest write failure path is untested

**Location:** `internal/shuttleengine/codexengine/codexengine.go:297`

**Issue:** Cards 3 and 4 require the refusal, and swallowing the error passes every test.

### [LOW] The copy's placement is untested

**Location:** `internal/shuttleengine/manifest_test.go:89`

**Issue:** Every row starts successfully, so moving `copyManifest` above `saveRunState` passes.

## Rejected

- code-tests F12 (guard keys expected through `shellguard.Rows`): the behaviour under test is that the manifest lists every row of the table, and the table's own contents belong to `shellguard`.
- code-tests F5, code-correctness F3's second half: the same defect as F1, merged into it.
