---
verdict: BLOCKING
findings:
  - id: F1
    severity: "BLOCKING"
    class: "design"
    location: "_lyx/plan/03-gate-call-renders-key.md:37"
    summary: "PATTERN growth fails the per-card gate before card 23 can supply the shortening."
    fix: "Redistribute card 23's exact authorized shortening lines into the four cards that grow PATTERN.md, keeping the final text unchanged: card 3 takes trimpath-safe-root and role-skills-typed; card 7 takes template-overlay, shell-guard-table, command-row and lyxdirs-single-declarer; card 19 takes agent-filed-issues, binary-change-reconcile, shed-recipe-registry, shed-producer-seam and owner-machine; the tier-tags scan card takes the remaining eight lines. These groups remove 69, 123, 139 and 151 bytes against growth of 69, 122, 133 and 137 bytes respectively. Remove the emptied pattern-shorten card with lyx loom remove-card and let it renumber the index. If the scan is folded into the last retag card as F8 requires, keep its assigned shortenings with that scan."
    origin: "handler"
  - id: F2
    severity: "BLOCKING"
    class: "design"
    location: "_lyx/plan/11-package-load-tags.md:13"
    summary: "The tag migration inventories test headers but misses a package loader and the runtime tmux permission switch."
    fix: "Use lyx loom insert-card to add a prerequisite before the first retag card covering both missed tag consumers: widen TestTreeSitterLink_OnlyAllowedTestBinariesLinkIt's go list tags to tier0,tier1,tier2,tier3,integration,tmux,llm, and make tmuxkit/allow_tmux.go compile under tier2 or tier3 as well as tmux or llm during transition. Preserve the socket cleanup policy and update its tag-related guidance. Record both consumers in the overview's migration inventory and in the D13 follow-up body prescribed by the scan card, so removal of legacy spellings leaves the new tier tags active. Keep the existing scanner and cleanup contract tests rather than creating parallel tests for their helpers."
    origin: "lens:affected-set"
  - id: F3
    severity: "BLOCKING"
    class: "design"
    location: "_lyx/plan/24-retag-cmd-contracts-tools.md:31"
    summary: "The only installed-binary migration check accepts a suffix match instead of the tier's actual build tag."
    fix: "Require an exact comma-delimited tier token in the go test step's -tags argument, accepting both -tags value and -tags=value and rejecting absent or malformed values. In particular, tier0 with go test -tags nottier0 ./... must fail while tier0 with go test -tags tier0,integration ./... passes; preserve the no-tiers and covers refusals. A replacement Verify command is withheld because the mandatory baseline and combined validate-plan --overlay runs are forbidden by this round's read-files/run-nothing rule; validate the concrete replacement before adopting it."
    origin: "lens:guard-reach"
  - id: F4
    severity: "BLOCKING"
    class: "consistency"
    location: "_lyx/plan/19-recipe-entry-gate.md:43"
    summary: "The entry-gate recovery command edits the prime worktree instead of the invalid board default."
    fix: "Change card 19's recovery specification so a per-worktree module's invalid board default is edited from the prime with lyx config <module> --layer default; retain the unqualified command only where that module's default dispatch actually selects the hub file. Keep --resolved for locating the source and the separate run_settings recovery. Extend the already planned non-bool rows of TestBuild_EntryGate to assert the applicable recovery text."
    origin: "lens:way-forward"
  - id: F5
    severity: "BLOCKING"
    class: "consistency"
    location: "_lyx/plan/20-batten-switch-entry-gate.md:30"
    summary: "Command-reference errors still tell the operator to add switch after the same card forbids switch."
    fix: "Amend card 20's intended refusal migration to change commandRefWayForward to instruct setting command to a module.key reference, without adding switch, and update the existing MissingCommand and NonStringCommand rows of TestCommandEntry to assert that message. The existing constant needs explicit target coverage; a concrete additional target bullet is withheld pending the overlay validation that the read-only rule prevents in this round."
    origin: "lens:way-forward"
  - id: F6
    severity: "BLOCKING"
    class: "design"
    location: "_lyx/plan/20-batten-switch-entry-gate.md:43"
    summary: "The new hubforge test duplicates the existing public row-lifecycle test's forced-window behavior."
    fix: "Fold the open_window lookup assertions and the seed_ide non-interference assertion into the existing stepCommandRows_FollowTheTasksLocalSwitch case of TestBattenIntegration_Rows, using the actual --open-terminal wiring already required by card 20 line 41. Remove the plan for a separate TestWireSetting_OpenTerminalForcesOpenWindow fixture. The concrete Create-target deletion is withheld pending the overlay validation prohibited in this round."
    origin: "handler"
  - id: F7
    severity: "MEDIUM"
    class: "consistency"
    location: "_lyx/plan/01-gate-key-shape.md:53"
    summary: "The retired-list refusal is intercepted upstream, and its optional reconcile command cannot migrate the hub file."
    fix: "Specify the legacy-list recovery at the LoadConfig/configengine failure boundary rather than relying on decodeConfig to see card: [0]. Preserve configengine's template and layering ownership, and distinguish that retired shape from other invalid configuration errors before choosing the reconcile guidance. Keep TestLoadConfig's real hub-file input and assert the user-visible recovery there. Remove the bare lyx config reconcile alternative or spell it lyx config reconcile --layer default --apply from the prime. Additional target-list changes are withheld pending the prohibited overlay validation."
    origin: "lens:guard-reach"
  - id: F8
    severity: "MEDIUM"
    class: "design"
    location: "_lyx/plan/00-overview.md:62"
    summary: "The Custom rationale contradicts the header-edit contract and is used to postpone the scan beyond D5's last retag card."
    fix: "Replace the Custom retag cards through lyx loom insert-card and lyx loom remove-card with typed retag cards: use Edit member targets for every member of each header-changed file, and Create/Move groups where the prescribed helper splits require them. Do not pair those member targets with their containing file or package self targets. Put the one-tag scan, its PATTERN change and the D13 follow-up on the last retag card, retaining its first-retag prerequisite check on the first card. Remove the redundant standalone scan card and rewrite retag-rules and scan-after-last-retag to match. Preserve F1's shortening allocation when combining the scan."
    origin: "handler"
  - id: F9
    severity: "MEDIUM"
    class: "consistency"
    location: "_lyx/plan/04-verify-gate-site-key.md:19"
    summary: "Two result-signature changes omit the required re-sign arrows."
    fix: "On card 4's existing internal/loomcli#darnTreeScope Edit bullet, add -> func darnTreeScope(l *lyxcwd.Location) func(gateKey string) (string, bool, error). On card 14's existing internal/gatecli#guardRunOf Edit bullet, add -> func guardRunOf(plan []tierRun) []tierRun, and state that it returns the guard runs in tier order. Retain the existing caller targets on those cards."
    origin: "handler"
  - id: F10
    severity: "MEDIUM"
    class: "design"
    location: "_lyx/plan/10-gate-report.md:4"
    summary: "Reusable package APIs are bundled into their consuming feature cards instead of independently reviewable prerequisites."
    fix: "Use lyx loom insert-card to extract ReadRunRecords and the gateslot Run log documentation from gate-report into an earlier API card, leaving the report command and its report assertions in gate-report. Similarly extract CheckSettingKey, SettingLookup's delegation and its existing TestSettingLookup coverage from recipe-entry-gate into an earlier configreg API card; keep entry-gate parsing, evaluation and their tests in recipe-entry-gate. Let the card tools maintain numbering and update the index, without adding one test per extracted symbol."
    origin: "handler"
---
### [BLOCKING] The plan cannot reach its late PATTERN shortening

**Location:** _lyx/plan/03-gate-call-renders-key.md:37

**Issue:** PATTERN.md is already 15,651 bytes, above the 15,564-byte headroom threshold in internal/pattern/check.go:24.
Card 3 adds 69 bytes, but the compensating changes do not occur until card 23.
internal/websterengine/cardgate.go:22 renders lyx loom lint-pattern for every card; internal/patternlint/lint.go:26 compares HEAD with the working tree, and internal/pattern/headroom.go:12 refuses growth above the threshold.
The implementer runs this gate before committing or proceeding to the next card (contracts/stencils/webster/webster-body-implementer.md:51).
Thus card 3 fails at 15,720 bytes.
This is an intermediate-card sequencing defect, not the final projected pattern-headroom check that the review must not duplicate.
Moving all shortening earlier is insufficient: the final prescribed overview is still 15,630 bytes, so the final growing card would cross the threshold again.

### [BLOCKING] Two tag consumers remain on the retired spellings

**Location:** _lyx/plan/11-package-load-tags.md:13

**Issue:** Card 11 promises every test-file package load uses the expanded tag set, but cmd/lyx/treesitterlink_integration_test.go:40 still invokes go list with integration,tmux,llm.
Card 24 changes its build header, not that subprocess argument.
After card 26 retags internal/loomshed/gates_test.go and gatefindings_test.go to tier0, the scan omits their planglyph imports.
The allowlist still names loomshed, and treeSitterLinkFindings reports stale entries (cmd/lyx/treesitterlink_test.go:101), producing a false guard failure while also losing coverage of newly tagged imports.
Separately, internal/testkit/tmuxkit/allow_tmux.go:1 is a production support file constrained to tmux || llm; the test-file retag rules never change it.
Its init enables tmuxAllowed, which afterRun passes to checkSockets (tmuxkit.go:105).
Without it, checkSockets rejects even registered sockets (tmuxkit.go:145), so tier2/tier3-only commands break after the prescribed legacy-tag removal.
These are one affected-set enumeration failure: migration must include tag-reading subprocesses and compiled support files, not only test headers and the three loaders named in card 11.
It violates D5's all-loaders requirement and PATTERN-test-speed's tier and link-guard contracts.

### [BLOCKING] The retag prerequisite accepts the wrong tag

**Location:** _lyx/plan/24-retag-cmd-contracts-tools.md:31

**Issue:** The awk expression permits arbitrary non-space text immediately before the tier name: [^ ]* followed by tier.
For a tier0 command, go test -tags nottier0 ./... therefore matches, although Go enables nottier0 and does not enable tier0.
The first retag card then passes its stated prerequisite while the installed binary can miss the retagged tests.
D13 and the Constraints explicitly rely on this Verify line because the installed lyx lacks D7's check.
This is a false acceptance in the proposed check, not a raw-test-command formatting objection.

### [BLOCKING] The default-layer way forward edits the wrong file

**Location:** _lyx/plan/19-recipe-entry-gate.md:43

**Issue:** Card 19 tells the operator to fix either a hub or default layer with lyx config <module> from the prime.
Batten is a per-worktree module, not HubWide (internal/configreg/configreg.go:120).
Without --layer default, configcli selects its worktree layer; editOne handles the board default only through the explicit LayerDefault branch (internal/configcli/configcli.go:126).
For an invalid batten.open_window inherited from the board default, following the prescribed command edits the prime's own configuration and leaves the task's failing value intact.
The refusal therefore lacks a progressing way forward under PATTERN-refusal-way-forward.

### [BLOCKING] A Command refusal sends the operator into another refusal

**Location:** _lyx/plan/20-batten-switch-entry-gate.md:30

**Issue:** Card 20 updates commandKeyWayForward and rejects switch as an unknown key, but omits commandRefWayForward.
internal/shedrecipe/entries_command.go:19 still says to set switch and command to module.key references.
commandSettingRef uses that message for absent or non-string command values at lines 58–67, so those paths remain reachable after the switch lookup is removed.
TestCommandEntry already pins the old wording in its MissingCommand and NonStringCommand rows (entries_command_test.go:60–70).
Following that recovery adds a key the newly prescribed commandEntry rejects.
Both the message and the existing reaching assertions need migration to satisfy PATTERN-refusal-way-forward.

### [BLOCKING] Reuse the existing forced-window behavior test

**Location:** _lyx/plan/20-batten-switch-entry-gate.md:43

**Issue:** internal/battencli/lifecycle_integration_test.go:1254 already tests that open_window false skips Open-Window and that forcing it runs its command; TestBattenIntegration_Rows includes that case at line 339.
Card 20 line 41 expressly migrates this existing test from the removed Forced field to --open-terminal, covering the new wiring through the public row behavior.
Line 43 then creates another hubforge fixture to test the same false/forced-true behavior through Env.Setting.
Its additional seed_ide assertion belongs in the existing case, and the already targeted wire-seam test also covers the forced lookup.
A second integration test for this behavior violates PATTERN-test-economy; the plan should extend the existing coverage.

### [MEDIUM] The planned legacy-config recovery is not reachable as described

**Location:** _lyx/plan/01-gate-key-shape.md:53

**Issue:** LoadConfig calls configengine.Load before decodeConfig (internal/gateslot/config.go:134–141).
configengine.Load fills the effective template through yamlengine.FillMissing (internal/configengine/config.go:234).
Because gates remains a per-name map rather than an open map, the template's new mapping at gates.card encounters the existing sequence [0] and fillMapping returns shapeMismatch before decodeConfig runs (internal/yamlengine/fill.go:112–116).
LoadConfig wraps this earlier error with the existing lyx config gate advice, so the named TestLoadConfig input cannot reach the planned decoder refusal.
The way-forward lens also confirmed that the alternative lyx config reconcile is a dry run over the worktree's own/local layers, not an applied migration of the board default (internal/configcli/configcli.go:525–563 and internal/configcli/hubwide.go:185).
These two defects affect the same legacy-config recovery path.

### [MEDIUM] Custom is not necessary for header edits

**Location:** _lyx/plan/00-overview.md:62

**Issue:** The overview claims a header change has no member to name and that edit-test-file forces Custom.
The authoritative plan spec explicitly supplies the opposite way forward: for a build-constraint header change, name every member of the file under Edit (.bench/specs/loom/loom-plan-spec.md:569–574).
Helper creation and relocation are also expressible with the ordinary labels.
The repeated Custom choice therefore fails the rubric's last-resort rule.
Its stated consequence is a second departure: the overview moves the scan to card 33 because Custom cannot combine with Create, whereas D5 explicitly requires the scan to land with the last retag card (_lyx/discussion/decision-record.md:107).
No amendment licenses replacing 'with' by 'after'.
Correctly typed cards remove the claimed obstacle and allow the last retag and its enforcement to land together.

### [MEDIUM] Declare both changed result signatures

**Location:** _lyx/plan/04-verify-gate-site-key.md:19

**Issue:** Card 4 line 57 changes Env.VerifyTreeScope to a keyed function and line 69 requires darnTreeScope to supply it, but the existing darnTreeScope result is func() (string, bool, error) (internal/loomcli/darnwiring.go:76).
Its Edit bullet lacks an arrow for that changed result type.
Likewise card 14 line 61 changes guardRunOf from one guard run to every guard run, but its line 24 bullet omits an arrow; the current declaration returns *tierRun (internal/gatecli/run.go:202).
These are function result changes, not the exempt Env struct-field changes.
The rubric expressly requires the declaration heads even where the named functions and their callers are already targeted.

### [MEDIUM] Separate the reusable API contracts from their first consumers

**Location:** _lyx/plan/10-gate-report.md:4

**Issue:** Card 10 creates exported gateslot.ReadRunRecords alongside the gatecli report command and implementation.
Card 19 similarly creates exported configreg.CheckSettingKey, refactors SettingLookup and introduces shedbuild entry-gate parsing and evaluation in one card.
The record-directory reader and known-setting validation are independently reviewable module contracts reused across package boundaries, rather than private helpers inseparable from the CLI or recipe feature.
The rubric's Granularity rule explicitly gives such public or cross-package symbols their own card even when the current feature is their only new consumer.
The same extraction rule should be applied consistently to these two boundaries, while leaving private supporting helpers bundled.

## Rejected

- The first retag Verify is not itself a raw test invocation: go test appears inside an awk pattern, while the command inspects resolved configuration.
- Cards 1–3 deliberately keep the old rendering until the --gate flag exists, so the alleged early flag-use defect is absent.
- Card 23's nineteen shortenings are authorized amendments, and card 33 files a different board entry expressly required by D13; neither is unauthorized task-body editing.
- The new one-tag scan covers a file-tag contract that the existing purity and LLM-import guards do not enforce.
- The green-record trust boundary and goto bypass are explicit decisions with stated bounds, not unbounded escapes inferred by this review.
- Existing waits in the affected scenario infrastructure are not new production-duration waits introduced by this plan.

## Focus departures

- The instruction to run a baseline and combined `lyx loom validate-plan --overlay` conflicts with the same step's tool-use rule: “Read-only analysis in the explore and review steps: read files, run nothing.”
  I followed the read-only rule and ran no validator, tests or quarry command.
  Findings F3, F5, F6 and F7 therefore state the required semantic repair while withholding the concrete Verify or target-list patch whose admissibility requires that validator.
  The card insertion/removal fixes in F1, F2, F8 and F10 use the rubric's explicit overlay exemption; F4 changes prose/test requirements and F9 adds arrows to already listed targets.
- The exploration instruction to read “the board entry of the task the target belongs to, through `lyx board get`” conflicts with the same run-nothing rule.
  The board entry was not fetched; board-related conclusions above are limited to the prescribed commands, the decision record and the plan's stated task identity.
  No claim is made about the live board's contents.

The overview and all 33 indexed cards were reviewed against D1–D14, their amendments, the Constraints, the relevant PATTERN backgrounds, and the existing code and test paths.
All three fork reviewers returned before consolidation.
No target file was changed.

