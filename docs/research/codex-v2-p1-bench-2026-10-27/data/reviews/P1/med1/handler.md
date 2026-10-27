---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: scope
    origin: lens:affected-set
    location: _lyx/plan/01-gate-key-shape.md:3
    summary: The gate-map reshape leaves the shared template fixture at the old Go type
    fix: Require card 1 to migrate the shared templateDefaults initializer to the new gate-map type, eight keys and scopes alongside its consumers. The exact target-list amendment is withheld because the required overlay validation cannot run under this round's read-files-only constraint; it must satisfy redundant-file-target and redundant-package-target.
  - id: F2
    severity: BLOCKING
    class: design
    origin: lens:affected-set
    location: _lyx/plan/11-package-load-tags.md:13
    summary: The tag migration omits the tree-sitter package loader and tmuxkit's build-time permission switch
    fix: Extend card 24's Intent to update TestTreeSitterLink_OnlyAllowedTestBinariesLinkIt's package load to tier0,tier1,tier2,tier3,integration,tmux,llm, and card 29's Intent to include the non-test allow_tmux.go switch with tier2 and tier3 alongside tmux and llm. These packages are already targeted. Include these transitional readers in card 33's follow-up description.
  - id: F3
    severity: BLOCKING
    class: design
    origin: lens:guard-reach
    location: _lyx/plan/24-retag-cmd-contracts-tools.md:31
    summary: The transition check accepts supported tier declarations whose test command lacks its tier tag
    fix: Replace the verification algorithm with validation of every decoded tier command and exact tag tokens, requiring a checked command for every tier and rejecting covers. It must reject flow-style commands without tags and nottier0 for tier0. A replacement Verify command is withheld because the required baseline and combined overlay checks are prohibited by this round's read-files-only constraint.
  - id: F4
    severity: MEDIUM
    class: consistency
    origin: lens:guard-reach
    location: _lyx/plan/01-gate-key-shape.md:53
    summary: The retired-list recovery is intercepted upstream and offers a non-applying reconcile alternative
    fix: Revise card 1's Intent to deliver reconcile guidance for the shape error raised during configengine.Load before decodeConfig, keeping configengine as the layer-loading owner. Prescribe only lyx fabric reconcile from the prime, or the fully specified lyx config reconcile --layer default --apply. Require the planned TestLoadConfig row to traverse the public loader and assert that actionable guidance. Any needed target-list amendment remains subject to the prohibited overlay validation.
  - id: F5
    severity: MEDIUM
    class: consistency
    origin: lens:way-forward
    location: _lyx/plan/19-recipe-entry-gate.md:43
    summary: Repairing board run_settings does not repair the stopped row's effective local configuration
    fix: Change card 19's recovery prose and planned refusal assertions to repair the layer identified by --resolved. For local values use lyx config <module> --layer local in the task worktree and also correct board run_settings for future seeding; for a board default use --layer default from the prime; for a worktree value edit that task's worktree layer. Resume only after the effective value is repaired.
  - id: F6
    severity: MEDIUM
    class: design
    origin: handler
    location: _lyx/plan/00-overview.md:62
    summary: The Custom retag rationale ignores the format's explicit header-only Edit mechanism
    fix: Withdraw the retag-rules claim that header changes have no admissible typed representation and use the documented header-only Edit mechanism, with Create for newly split support where needed. Exact replacement target lists are withheld pending overlay validation, which this round forbids; edit-test-file requires member targets rather than test-file paths, and redundant-package-target forbids retaining package targets beside those members.
  - id: F7
    severity: MEDIUM
    class: consistency
    origin: handler
    location: _lyx/plan/04-verify-gate-site-key.md:19
    summary: Two explicitly re-signed helpers lack the required declaration arrows
    fix: Give card 4's darnTreeScope bullet the declaration arrow func darnTreeScope(l *lyxcwd.Location) func(gateKey string) (string, bool, error). Give card 14's guardRunOf bullet an explicit declaration arrow for its chosen collection result, for example func guardRunOf(plan []tierRun) []tierRun, matching its requirement to return every guard run.
---

### [BLOCKING] Shared gate fixture no longer compiles

**Location:** `_lyx/plan/01-gate-key-shape.md:3`

**Issue:** `internal/gateslot/config_test.go:21` declares `templateDefaults` with `Gates: map[string][]int` and the retired names.
Card 1 changes `Config.Gates` to `map[string]GateKey`, but its detailed fixture migration never includes this initializer.
The initializer therefore stops the package compiling at card 1, before retagging.
`hubWith` at line 32 and the named load/template tests consume the shared value; changing those tests alone does not make the initializer type-correct.
This is a field-type migration omission, not a re-derived caller-uncovered finding for a deleted or re-signed function.

### [BLOCKING] Tag-reader enumeration stops short of two executable readers

**Location:** `_lyx/plan/11-package-load-tags.md:13`

**Issue:** `cmd/lyx/treesitterlink_integration_test.go:40` loads test dependencies with only `integration,tmux,llm`.
Its guard promises all tiers and enforces a minimum of 100 test binaries, so retagging removes tests from its dependency graph and can trip the floor or lose enforcement.
Card 24 covers its package but prescribes only test-file retagging.
Separately, `internal/testkit/tmuxkit/allow_tmux.go:1` is a non-test source file constrained to `tmux || llm`; its `init` sets `tmuxAllowed`.
Card 29 retags only test files, leaving pure `tier2` and `tier3` builds with that switch false.
`tmuxkit.go:105` passes it to `checkSockets`, whose false branch at line 145 rejects even registered surviving sockets.
The transitional commands mask the second omission until old spellings are removed.
These are one enumeration-method gap in implementing D5, rather than separate findings for each missed reader.

### [BLOCKING] The transition check can pass without checking a command

**Location:** `_lyx/plan/24-retag-cmd-contracts-tools.md:31`

**Issue:** The AWK program sets `seen` on a tier header but checks a command only on a separate eight-space-indented `command:` line.
A supported flow mapping such as `tier0: {command: "go test -json {targets} {args}", ...}` sets `seen` without checking its command.
The gate template explicitly illustrates flow mappings at `internal/gateslot/template.yaml:5`, and `yamlengine.AnnotateLeaves` at `internal/yamlengine/layers.go:526` marshals the YAML nodes with their styles.
Independently, the regex's unrestricted prefix before the tier name accepts `-tags nottier0` for `tier0`.
D13 and the Constraints require this check to reject missing tier tags because the installed binary has no D7 protection; these inputs allow untested retagged files through that sole transition check.

### [MEDIUM] Retired-shape refusal does not reach its prescribed recovery

**Location:** `_lyx/plan/01-gate-key-shape.md:53`

**Issue:** `gateslot.LoadConfig` calls `configengine.Load` before `decodeConfig` (`internal/gateslot/config.go:134`).
That loader calls `yamlengine.FillMissing` (`internal/configengine/config.go:234`), whose mapping check rejects an existing sequence where the new template requires a mapping (`internal/yamlengine/fill.go:112`).
Thus the named `card: [0]` input fails before the planned decoding refusal, and follows the existing editor guidance instead.
The offered alternative `lyx config reconcile` also cannot repair the hub file as spelled: `internal/configcli/configcli.go:525` documents that default invocation as a dry run over worktree/local files, requiring both `--layer default` and `--apply` for the hub write.
The latter point consolidates the way-forward fork's finding about this same refusal.

### [MEDIUM] Recovery edits an ineffective configuration source

**Location:** `_lyx/plan/19-recipe-entry-gate.md:43`

**Issue:** Seed-Child copies board run settings into task-local files in `internal/battenshed/seamchild.go:144`.
The stopped row's setting lookup reads those existing files (`internal/battencli/wire.go:1351`); resuming the row does not repeat Seed-Child.
Changing only board `run_settings` therefore leaves the bad value in effect.
Also, for a per-worktree module, bare `lyx config <module>` from the prime selects the prime's worktree layer, not the board default (`internal/configcli/hubwide.go:102`).
The prescribed next step must repair the actual layer the resumed row will read.

### [MEDIUM] Custom is used despite an explicitly supported typed operation

**Location:** `_lyx/plan/00-overview.md:62`

**Issue:** Shared Decision `retag-rules` says header-only changes have no member representation and uses that premise to justify Custom on cards 24–32.
The format authority's `edit-test-file` rule explicitly supplies that representation: for a header-only build-constraint change, name every member of the file under Edit (`.bench/specs/loom/loom-plan-spec.md:569`).
Scale does not make this a last-resort Custom operation under the rubric.
The same premise is used to justify separating the scan from the last retag card, although D5 prescribes landing them together.
This finding concerns the choice of operation, not the deterministic edit-test-file check, which the current Custom groups evade.

### [MEDIUM] Missing re-sign declarations

**Location:** `_lyx/plan/04-verify-gate-site-key.md:19` and `_lyx/plan/14-guard-step-per-tier.md:24`

**Issue:** Card 4 says `darnTreeScope` takes the gate key, changing the returned closure from its current zero-argument signature in `internal/loomcli/darnwiring.go:76`.
Card 14 says `guardRunOf` returns every guard run, changing its current single `*tierRun` result in `internal/gatecli/run.go:202` to a collection.
Both are function result changes, not the exempt struct-field or interface-method cases, and the rubric explicitly requires the declaration arrow.

## Rejected

- The retag Verify lines do not run raw tests: card 24 inspects configuration and card 33 checks a board entry; cards 25–32 have no Verify command.
- The scan in card 33 is not redundant with the existing purity and LLM guards: those constrain spawns and LLM access, rather than requiring exactly one tier per test-bearing file.
- Card 23's nineteen shortened entries are licensed by the Added after Discussion decisions.
- Card 33 files a different board entry and does not edit this task's body or scope.
- No early site renders the new flag before card 2: card 1 expressly retains the old rendering, and card 3 changes it.
- The retag cards follow the preparatory cards 11, 12, 14, 15 and 16.

## Review limits

The instruction to read files and run nothing takes precedence over the requested board command and overlay validation, as step 1's conflict rule requires.
No board command, test, build, or plan validator was run.
Target-list and Verify-command replacement patches that require overlay validation are therefore withheld above; the defects and the relevant constraints are recorded rather than claiming those patches were validated.
Only the review file was written.
