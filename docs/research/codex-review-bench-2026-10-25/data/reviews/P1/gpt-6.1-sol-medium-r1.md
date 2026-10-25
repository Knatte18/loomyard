---
verdict: BLOCKING
findings:
  - id: F1
    severity: MEDIUM
    class: design
    location: _lyx/plan/00-overview.md:62
    summary: The retag cards use Custom even though Edit fits their existing package targets
    fix: In cards 24 through 32, replace Custom with Edit and add a concrete ImpactSummary for each package group. Update the retag-rules shared decision to describe these as Edit cards over the same package self glyphs; no target additions or pairings are needed.
  - id: F2
    severity: BLOCKING
    class: design
    location: _lyx/plan/24-retag-cmd-contracts-tools.md:31
    summary: The transitional hub check can pass without checking any tier command
    fix: The check must examine the command value of every declared tier regardless of YAML block or flow style, require an actual go test step with the exact corresponding tier tag, and reject a missing command as well as covers. A replacement Verify command is withheld because the rubric requires overlay validation before proposing one and this round explicitly permits file reads only.
  - id: F3
    severity: BLOCKING
    class: scope
    location: _lyx/plan/24-retag-cmd-contracts-tools.md:22
    summary: The retag leaves the tree-sitter link guard enumerating only retired tags
    fix: Extend card 24 Intent to update TestTreeSitterLink_OnlyAllowedTestBinariesLinkIt in cmd/lyx/treesitterlink_integration_test.go to enumerate tier0,tier1,tier2,tier3,integration,tmux,llm. Its existing cmd/lyx package target already covers this edit. Include this remaining old-tag enumeration in the follow-up scope stated by card 33.
  - id: F4
    severity: BLOCKING
    class: scope
    location: _lyx/plan/20-batten-switch-entry-gate.md:30
    summary: Removing the Command switch key leaves the registered-engine fixture invalid
    fix: Card 20 must also remove the switch entry from the Command fixture returned by engineMinimalConfig in internal/shedbuild/build_engines_test.go, in the same commit that refuses that key. Keep TestBuild_EveryRegisteredEngineBuilds as the existing coverage. The required target-list expansion is withheld pending the mandatory overlay check, which the round's read-only tool rule forbids running.
  - id: F5
    severity: BLOCKING
    class: design
    location: _lyx/plan/04-verify-gate-site-key.md:69
    summary: Card 4 changes the tree-scope reader signature before its test callers migrate
    fix: In card 4 Intent, retain darnTreeScope's existing no-argument reader result until card 6 and adapt it to Env.VerifyTreeScope with a keyed wrapper that ignores the key and calls the existing reader. Make card 6 the point where the reader itself becomes keyed, alongside its already planned TestDarnTreeScope_ClassifiesAtEachCall updates.
  - id: F6
    severity: MEDIUM
    class: consistency
    location: _lyx/plan/14-guard-step-per-tier.md:24
    summary: Planned result-signature changes lack re-sign annotations
    fix: Mark guardRunOf's transition from a single guard pointer to all guard runs with a re-sign annotation, and mark darnTreeScope's transition to a keyed reader on the card where that change lands. The intended heads are func guardRunOf(plan []tierRun) []tierRun and func darnTreeScope(l *lyxcwd.Location) func(gateKey string) (string, bool, error). Concrete arrow and caller-target edits are withheld until overlay validation is permitted; caller-uncovered must be satisfied on the same card.
  - id: F7
    severity: MEDIUM
    class: design
    location: _lyx/plan/00-overview.md:68
    summary: The scan-after-last-retag decision contradicts D5's required landing point
    fix: After correcting the Custom typing, combine cards 32 and 33 into one final retag-and-scan card, retaining the scan's fixtures, PATTERN updates and D13 board follow-up. Remove the superseded card through lyx loom remove-card and update the shared decision to put the scan on the last retag card. This is a card-removal fix, which the rubric exempts from overlay validation.
---

### [MEDIUM] Retagging does not require Custom

**Location:** _lyx/plan/00-overview.md:62 and cards 24 through 32.

**Issue:** The shared decision justifies Custom by the absence of a member for a header and by edit-test-file. However, the actual targets are existing package self glyphs. The format's edit-test-file check applies to test-file self glyphs, not package self glyphs, so these same targets can carry Edit. Header edits and helper splits are edits to existing packages. The last-resort rule therefore does not admit Custom here.

### [BLOCKING] The only transitional tag check can pass vacuously

**Location:** _lyx/plan/24-retag-cmd-contracts-tools.md:28–31.

**Issue:** The awk program sets seen when it encounters a tier header, but checks commands only on separate lines beginning with eight spaces and command:. A legal flow mapping such as tier0: {command: "go test -json {targets} {args}", all_targets: ./..., timeout_min: 60, counter: go-json} sets seen and never reaches the command check. With covers absent, it succeeds even though tier0 lacks its tag. This shape is supported by yamlengine's node-based merge and annotation, which preserve a supplied flow node; configengine.Provenance does not normalize every tier into block form. Even in block form, a tier without a command never sets bad. D13 and the Constraints require failure for a missing tier tag because the installed binary has no empty-tier safeguard. The current check can therefore authorize retagging while tests silently disappear from the gates. This is a semantic check failure, not a raw-test-command finding.

### [BLOCKING] The tree-sitter guard loses the retagged test binaries

**Location:** _lyx/plan/24-retag-cmd-contracts-tools.md:22–25; _lyx/plan/11-package-load-tags.md:15–21.

**Issue:** Card 11 updates the production package loads, but card 24 only changes test headers and helper placement. The existing TestTreeSitterLink_OnlyAllowedTestBinariesLinkIt still runs go list -test -deps with integration,tmux,llm in cmd/lyx/treesitterlink_integration_test.go:40. After retagging, that enumeration omits tier0, tier2 and tier3 tests and non-guard tier1 tests. The guard retains its 100-test-binary floor and stale-allowlist checks, so it can fail as packages disappear from its view, and it no longer enforces the background's requirement to inspect binaries from every tier. D5's all-package-loads rule and PATTERN-test-speed's Tree-sitter links section require this load to migrate too. Retagging the guard's own header does not change the tags it passes to go list.

### [BLOCKING] The Command fixture still supplies the removed key

**Location:** _lyx/plan/20-batten-switch-entry-gate.md:30 and 39–43.

**Issue:** The card makes switch an unknown Command config key, but no card carries the corresponding fixture update. internal/shedbuild/build_engines_test.go:48–51 supplies switch: batten.open_window in engineMinimalConfig. TestBuild_EveryRegisteredEngineBuilds feeds that config through Build and requires success for every registered engine. Executing card 20 as scoped makes its Command case fail. The later shedbuild retag in card 29 changes headers, not this config value. This is a missing migration of test data, rather than an incomplete Uses dependency list.

### [BLOCKING] The keyed reader lands before the caller update

**Location:** _lyx/plan/04-verify-gate-site-key.md:57–69; _lyx/plan/06-darn-tree-scope-key.md:51–55.

**Issue:** Card 4 says darnTreeScope takes the key so its result can fill the keyed Env.VerifyTreeScope. Its current result is func() (string, bool, error). TestDarnTreeScope_ClassifiesAtEachCall explicitly declares locate and its assertion helpers with that old function type and returns darnTreeScope directly at internal/loomcli/darntreescope_integration_test.go:53–55. That test is absent from card 4. Card 5 updates its landing VerifyCommand call, while card 6 finally updates its tree-scope calls and per-key assertions. Consequently card 4 cannot compile the integration test while waiting for card 6. The plan's interim adapter must cover this caller too; merely ignoring the new key inside a re-signed reader does not preserve the old function type.

### [MEDIUM] Two result changes are not declared as re-signs

**Location:** _lyx/plan/14-guard-step-per-tier.md:24 and 61; _lyx/plan/04-verify-gate-site-key.md:19 and 69.

**Issue:** guardRunOf currently returns *tierRun (internal/gatecli/run.go:202), whereas card 14 says it returns every guard run. The target has no re-sign arrow. Likewise darnTreeScope currently returns a no-argument closure, while the planned keyed closure changes that result type without an arrow in card 4 or card 6. Both are function result changes, not the struct-field changes that the rubric exempts. The missing annotations also conceal the same-card caller obligations; in particular the tree-scope test is scheduled after card 4.

### [MEDIUM] The final retag commit lacks its required scan

**Location:** _lyx/plan/00-overview.md:66–70; cards 32 and 33.

**Issue:** D5 expressly requires the one-tag scan test to land with the last retag card. The shared decision instead places it in the following commit and justifies this by Custom's prohibition on mixed labels. No Added after Discussion entry authorizes that departure. A plan-level shared decision cannot override the answer key, and the reason disappears when the retag cards use the applicable Edit type. Immediate adjacency still leaves the last retag commit without the coverage D5 required on that commit.

## Rejected

- Cards 24–32 prescribe no raw test invocation. Card 24's Verify command inspects configuration; its defect is its incomplete inspection, recorded above. The remaining retag cards inherit the gates without adding a raw command.
- Cards 1–3 explicitly retain the old rendering until the --gate parser lands. The first retag follows cards 11, 12, 14, 15 and 16. No contrary ordering finding is raised for those transitions.
- Card 23's nineteen shortenings are licensed by the Added after Discussion entries. Card 33 files a different board entry, with the required dependency, and does not edit this task's own body or scope.
- Cards 2, 7, 15, 17 and 19 state their skip or bypass bounds and name reachable refusal inputs and tests. The skip-cycle and nil-wiring branches are explicitly wiring guards. No additional missing-bound or unreachable-refusal finding is raised for them.
- Card 33's one-tag scan covers behavior that the existing spawn-purity, LLM and tmux-isolation guards do not enforce. No duplicate-test finding is raised against that scan.

## Review limits

The round allows file reads and forbids running analysis programs. No tests, quarry commands or plan-validator commands were executed. For findings requiring a replacement Verify command or target/arrow changes, the mandatory validate-plan --overlay check could not be performed; those concrete edits are withheld rather than represented as validated proposals. The other fixes change intent or type labels over existing targets.
