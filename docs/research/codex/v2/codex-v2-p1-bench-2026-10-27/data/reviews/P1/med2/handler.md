---
verdict: BLOCKING
findings:
  - id: F1
    severity: BLOCKING
    class: design
    origin: lens:affected-set
    location: _lyx/plan/01-gate-key-shape.md:3
    summary: The affected-set enumeration misses existing fixtures, a package loader, and an upstream refusal path
    fix: >-
      Correct the affected-set enumeration for cards 1, 11 and 14, including the initializer and assertions described below and the legacy-shape refusal before decoding.
      Each producing card must carry its affected fixtures; the tree-sitter loader must learn the tier tags before retagging begins.
      Concrete target-list additions are withheld because the required validate-plan --overlay baseline could not run (lyx is unavailable).
      Validate the completed target expansion against redundant-package-target, redundant-file-target and caller-uncovered before applying it.
  - id: F2
    severity: BLOCKING
    class: design
    origin: handler
    location: _lyx/plan/23-pattern-shorten.md:7
    summary: PATTERN shortening arrives after the individual cards that need its headroom
    fix: >-
      Distribute card 23's already authorized shortening text among the PATTERN-growing cards so each individual card passes lint-pattern.
      One sufficient distribution is command-row and trimpath-safe-root with card 3; shed-recipe-registry, template-overlay, shell-guard-table and parent-lock with card 7; treadle-runner-seam, binary-change-reconcile, shed-producer-seam, agent-filed-issues and origin-reconcile with card 19; and the remaining eight shortenings with card 33.
      These cards already target PATTERN.md.
      Remove the emptied card 23 through lyx loom remove-card and update the shared prose through that renumbering workflow.
  - id: F3
    severity: BLOCKING
    class: scope
    origin: lens:guard-reach
    location: _lyx/plan/14-guard-step-per-tier.md:58
    summary: Package-wide guard exclusion can omit a guard whose own tier never ran
    fix: >-
      In card 14's Intent, require retaining guard metadata until each guard's own tier scope is known, then exclude only guards covered by that tier's actual run or qualifying green entry.
      Explicitly replace the package-wide exclusions in the already targeted targetsFor and scanGuards.
      Extend the already targeted planning and scenario tests with a tier0-only card gate over a package holding a tier1 guard, and with two tiers whose diff scopes differ.
  - id: F4
    severity: BLOCKING
    class: design
    origin: lens:guard-reach
    location: _lyx/plan/24-retag-cmd-contracts-tools.md:31
    summary: The transition check accepts commands that do not enable the required tier tag
    fix: >-
      Require the first-retag check to inspect every decoded tier command and match tierN as an exact comma-delimited tag token, rejecting a tier whose command was never checked.
      Cover a nottier0 tag and a flow-style tier mapping when validating the check.
      A replacement Verify command is withheld because the required validate-plan --overlay baseline could not run (lyx is unavailable); validate any replacement against the Verify checks before applying it.
  - id: F5
    severity: BLOCKING
    class: consistency
    origin: lens:way-forward
    location: _lyx/plan/19-recipe-entry-gate.md:43
    summary: The entry-gate refusal prescribes config repairs that do not change the running pair's value
    fix: >-
      Replace card 19's repair paragraph with explicit provenance-based layer selection: repair a seeded run-settings override through lyx config <module> --layer local --set in the running pair, a board default through --layer default from the prime, and a worktree override in that worktree's layer, then resume.
      Explain that changing board run_settings preserves the correction for a future Seed-Child but does not repair the current pair.
      Have the already planned refusal coverage assert the actionable layer-specific instructions.
  - id: F6
    severity: BLOCKING
    class: design
    origin: handler
    location: _lyx/plan/20-batten-switch-entry-gate.md:43
    summary: The new hub-backed force-setting test duplicates the existing command-row scenario
    fix: >-
      Consolidate the forced-open-window and unaffected-setting assertions into the already targeted stepCommandRows_FollowTheTasksLocalSwitch scenario instead of adding a second hubforge test for the same behavior.
      The concrete removal of the Create target is withheld pending the required overlay validation, whose baseline could not run because lyx is unavailable.
      Validate that target adjustment before applying it.
  - id: F7
    severity: MEDIUM
    class: design
    origin: handler
    location: _lyx/plan/00-overview.md:59
    summary: The retag Custom justification overlooks the format's explicit header-edit mechanism
    fix: >-
      Replace the retag-rules rationale with the format's supported header-edit representation and express the retag work using conventional card types.
      Restore D5's requirement that the scan land with the final retagging card rather than overriding it through scan-after-last-retag.
      Concrete target redistribution is withheld pending overlay validation against edit-test-file and the redundant-target checks; lyx is unavailable here.
      Any insertion or removal of cards must use the specified insert-card/remove-card workflow.
  - id: F8
    severity: MEDIUM
    class: consistency
    origin: lens:guard-reach
    location: _lyx/plan/15-empty-tier-fails.md:28
    summary: An original caller filter can still exempt a guard run from the empty-tier check
    fix: >-
      In card 15, make run.guardStep take precedence over the caller-argument exemption: guard runs always perform the zero-test check, while ordinary runs inspect testRequest.args for filter exemptions.
      Extend the planned TestEmptyTierFailure cases with a guard run whose original caller arguments also contain -run.
  - id: F9
    severity: MEDIUM
    class: consistency
    origin: handler
    location: _lyx/plan/14-guard-step-per-tier.md:29
    summary: Two planned function signature changes lack their required re-sign arrows
    fix: >-
      On card 14, give the existing guardRunOf target the re-sign head func guardRunOf(plan []tierRun) []tierRun, matching the planned list of guard runs.
      On card 4, give the existing darnTreeScope target the re-sign head func darnTreeScope(l *lyxcwd.Location) func(gateKey string) (string, bool, error), matching the keyed VerifyTreeScope seam.
      These are declaration updates to existing targets, not added targets.
  - id: F10
    severity: LOW
    class: consistency
    origin: lens:way-forward
    location: _lyx/plan/01-gate-key-shape.md:53
    summary: Bare config reconcile does not perform the promised hub migration
    fix: >-
      Remove the bare lyx config reconcile alternative from card 1's legacy-shape refusal and retain lyx fabric reconcile from the prime with the role clause.
      Alternatively spell the full command lyx config reconcile --layer default --apply from the prime.
---

### [BLOCKING] Incomplete affected-set enumeration

**Location:** `_lyx/plan/01-gate-key-shape.md:3`, `11-package-load-tags.md:3`, `14-guard-step-per-tier.md:8`.
**Issue:** Card 1 changes `Config.Gates` but leaves the package-level `templateDefaults` initializer in `internal/gateslot/config_test.go:21` using `map[string][]int`, causing a compile error.
Card 14 changes `Targets.Guards`, but `TestDerive_OverARepository` still compares it against `map[string][]string` at `internal/impactset/derive_integration_test.go:102` and `:162`.
Card 11's earlier import-edge extension does not cover that later representation change.
The tree-sitter guard's `go list` at `cmd/lyx/treesitterlink_integration_test.go:40` still selects only `integration,tmux,llm`; header retagging does not update this loader, so it loses the ordinary retagged test files despite PATTERN-test-speed requiring all-tier coverage.
The guard-reach lens found another omitted path: card 1's `card: [0]` input cannot reach its prescribed decoder refusal after the template changes to a mapping.
`gateslot.LoadConfig` calls `configengine.Load` first (`internal/gateslot/config.go:134`), whose `FillMissing` rejects the shape (`internal/configengine/config.go:234`, `internal/yamlengine/fill.go:112`), and the unchanged wrapper recommends `lyx config gate` (`internal/gateslot/config.go:160`).
This is one enumeration-method finding; none of these omissions is the mechanical deleted/re-signed-member caller check.

### [BLOCKING] Headroom must hold at each card

**Location:** `_lyx/plan/23-pattern-shorten.md:7`.
**Issue:** The current PATTERN file is 15,651 bytes, already above the 15,564-byte headroom line.
Card 3's replacement alone grows it to 15,720 bytes; cards 7 and 19 add further text before card 23 supplies any cuts.
Every card runs `lyx loom lint-pattern` (`internal/websterengine/cardgate.go:23`), and `Headroom` rejects growth above that line (`internal/pattern/headroom.go:11`).
Simply moving all shortening earlier also leaves card 33 growing the overview from below the line to above it.
The plan gate checks the combined final projection (`internal/loomshed/patternplan.go:77`); this finding concerns intermediate card gates, not a duplicate of that mechanical check.

### [BLOCKING] Guard coverage must be tier-specific

**Location:** `_lyx/plan/14-guard-step-per-tier.md:58`.
**Issue:** D6 requires guards outside their own tier's scope.
The plan instead retains the existing outside-the-package-scope formulation while changing the guard payload.
`targetsFor` discards guards in any impacted package (`internal/impactset/targets.go:126`), and `scanGuards` does the same for `sc.modules` (`internal/gatecli/scope.go:138`).
A tier0-only card gate over a package containing a tier1 guard therefore compiles no such guard and also excludes it from the guard step.
The proposed two-group tests do not name this case.

### [BLOCKING] The transition check has false passes

**Location:** `_lyx/plan/24-retag-cmd-contracts-tools.md:31`.
**Issue:** The regex permits arbitrary non-space text immediately before the expected tier name, so `go test -tags nottier0` satisfies its tier0 match.
It also sets `seen` on a tier declaration without requiring that a corresponding command line was checked; a flow-style mapping can bypass the eight-space command-line pattern.
Resolved output is YAML remarshal output, not a guaranteed block-only format (`internal/configcli/configcli.go:70`, `internal/yamlengine/resolve.go:31`, `internal/yamlengine/layers.go:526`).
This violates D13's explicit pre-retag constraint at the point where the installed binary cannot supply the D7 backstop.
The Verify line is not a raw test command; the defect is what its inspection accepts.

### [BLOCKING] Repair the layer the live run reads

**Location:** `_lyx/plan/19-recipe-entry-gate.md:43`.
**Issue:** The proposed instruction to change task `run_settings` and resume leaves the seeded local override unchanged.
`internal/boardcli/runsettings.go:77` explicitly states that such an update applies at the next Seed-Child and directs live repairs to `--layer local` in the pair.
Seed-Child writes those overrides before the Command rows (`internal/battenshed/seamchild.go:144`); resuming a gated Command row does not revisit it.
Also, bare `lyx config batten` from the prime selects that worktree's layer, not the board default (`internal/configcli/hubwide.go:102`, `internal/battenshed/config.go:49`).
The prescribed recovery fails PATTERN-refusal-way-forward for these reachable cases.

### [BLOCKING] Reuse the existing force-setting scenario

**Location:** `_lyx/plan/20-batten-switch-entry-gate.md:43`.
**Issue:** `stepCommandRows_FollowTheTasksLocalSwitch` already creates the real task config, sets `open_window: false`, checks that nothing runs, forces Open-Window, and checks its command marker (`internal/battencli/lifecycle_integration_test.go:1254`).
Card 20 already targets that scenario and explicitly changes its force mechanism to `--open-terminal`.
The additional hubforge test repeats the same behavior through the lower-level Setting seam; its unaffected-setting assertion can extend the existing scenario.
PATTERN-test-economy requires extending existing coverage rather than adding duplicate fixtures and test functions.

### [MEDIUM] Custom is not justified by header edits

**Location:** `_lyx/plan/00-overview.md:59` and `:66`.
**Issue:** The shared decision says header edits have no member to name and therefore require Custom.
The format explicitly addresses this case: the `edit-test-file` way forward names every member of a file whose header changes (`.bench/specs/loom/loom-plan-spec.md:573`).
Volume does not make Custom the last resort, and helper splitting can use the conventional types.
The resulting claim that Custom forces a separate scan card also overrides D5's explicit requirement to land the scan with the final retag card without a corresponding decision-record amendment.

### [MEDIUM] Caller filters must not exempt guards

**Location:** `_lyx/plan/15-empty-tier-fails.md:28`.
**Issue:** With `--guards -- -run Something`, the original arguments still contain a filter even though the guard runner appends its own matching filter last (`internal/gatecli/tierrun.go:173`).
Using `testRequest.args` alone therefore exempts the guard run, contrary to D7 and the preceding sentence of the card.
The planned test only covers empty original arguments, so it does not catch this path.

### [MEDIUM] Declare the changed signatures

**Location:** `_lyx/plan/14-guard-step-per-tier.md:29` and `_lyx/plan/04-verify-gate-site-key.md:18`.
**Issue:** Card 14 says `guardRunOf` returns every guard run, replacing the current `*tierRun` result (`internal/gatecli/run.go:202`), but its target has no re-sign arrow.
Card 4 changes `darnTreeScope` to supply the keyed callback, replacing its current no-argument callback result (`internal/loomcli/darnwiring.go:75`), likewise without an arrow.
These are function result changes, not the exempt interface-method or struct-field edits.

### [LOW] Name a reconcile command that writes the hub

**Location:** `_lyx/plan/01-gate-key-shape.md:53`.
**Issue:** Bare `lyx config reconcile` defaults to dry-run and does not reconcile hub-wide files in that mode.
Its help requires `--layer default --apply` for that operation (`internal/configcli/configcli.go:707`), and the normal path only reports/retires worktree copies (`internal/configsync/configsync.go:405`).
The primary `lyx fabric reconcile` recommendation remains valid.

## Rejected

- No early use of `--gate` was found in cards 1–3: card 1 explicitly preserves tier rendering, card 2 adds the flag, and card 3 changes rendering.
- Retagging starts after the package-load, guard, empty-tier and purity changes; that prerequisite ordering is sound.
- Card 23's nineteen wording changes are licensed by the Added after Discussion entries; their scheduling, rather than their scope, is defective.
- Card 33 writes a different board entry, not this task's own body or scope.
- Card 33's one-tag scan covers a distinct contract; the existing tier-purity and LLM scans do not make it redundant.
- The plan names bounds for the new gate flag, green carry, entry-gate skip and goto bypass; their mere existence is not a finding.

## Review limitations

The required overlay baseline was attempted and failed with `lyx: command not found`.
No target-list expansion or replacement Verify command is presented as validated, and no target file was changed.
The board entry could not be read through its required lyx interface for the same reason; fidelity judgments use the supplied decision record.
