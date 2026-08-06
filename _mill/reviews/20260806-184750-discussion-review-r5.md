MILL_REVIEW_BEGIN
# Review: fabric: close the weft-visibility leak (slice 8)

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-06
```

## Findings

### [GAP] clean-typed-reason has no type shape

**Section:** `### clean-typed-reason`
**Issue:** `Healthy` gets a fully specified `HealthReason{Cause, Detail}` plus a five-value enum, but `Clean` is only told to get "the same treatment" — no type name, no cause values, and no answer for the case `hostclean.go:44-47` actually produces, where *both* sides are dirty and the reasons are joined with `"; "` (a single `Cause` field cannot express two simultaneous causes).
**Fix:** State `Clean`'s return type, its cause values, and how a both-sides-dirty result is represented, or state explicitly that `Clean` keeps a plain `string` and only rewords.

### [GAP] No test named for Clean's reworded reason

**Section:** `## Testing`
**Issue:** The TDD list covers `Open`, `Ready`, `Committed()`, `RefScanner`, and all five `Healthy` causes, but nothing covers `clean-typed-reason`, even though it changes an operator-facing string `loomengine` prints verbatim under `CheckWorktreeClean` (`preflight.go:92-98`) and no existing test is named as covering it.
**Fix:** Add a TDD or regression entry pinning `Clean`'s new reason for the three shapes (code-side only, state-side only, both).

### [GAP] Healthy's in-package reason assertions not inventoried

**Section:** `## Testing` — "Regression coverage that must keep passing"
**Issue:** Changing `Healthy`'s third return from `string` to `HealthReason` breaks compile in five `package fabricengine_test` files the discussion never lists: `junction_pattern_integration_test.go:417-426` pins all three junction reason strings via `reason != wantReason`, and `reconcile_stale_removal_test.go:350` / `reconcile_stale_registration_test.go:487` / `boardjunction_integration_test.go:162` / `config_driven_junctions_integration_test.go:120` read `reason` as a string (`strings.Contains(reason, "unavailable")`).
**Fix:** List these files and state whether their pinned wordings move to the `Detail` string or to the typed `Cause`.

### [GAP] `host` inventory misses the English-verb and machine senses

**Section:** `### fabric-vocabulary-rule`
**Issue:** The rule machine-bans whole-word `host` everywhere outside the owner set and claims the non-warp cases are `websterengine/audit.go:156,159` plus "`internal/shell`'s three occurrences", but a whole-word scan also hits the verb sense in packages with no fabric relation — `builderengine/spawn.go:273` ("cannot host a live strand"), `reedengine/apply.go:88`, `lifecycle.go:211,427`, `doc.go:145`, `proctree.go:6,89`, `headerpane.go:4`, `version.go:5` — plus `internal/shell/posix.go:3` ("host-testable"), a fourth occurrence the count omits. "Reworded, not excepted" then forces rewrites of ordinary English in unrelated modules.
**Fix:** Enumerate the full non-owner `host` set and state the rule for the verb/machine sense (reword, or a narrow exception), rather than implying only two packages are affected.

### [NOTE] Test-file carve-out omits `Write-Host`

**Section:** `## Scope` — "Carve-out"
**Issue:** The hand-cleaning carve-out names only `WEFT_SKIP_GIT`/`WEFT_SKIP_PUSH`, but five `internal/reedcli` test files embed the literal PowerShell cmdlet `Write-Host` in `--cmd` strings (e.g. `smoke_teardown_test.go:36`), which a `host` sweep must not touch.
**Fix:** Add `Write-Host` to the verbatim-retained list.

### [NOTE] Stray rationale block inside clean-typed-reason

**Section:** `### clean-typed-reason`
**Issue:** The section's second "Rationale" bullet argues about `drift.go:58` and `preflight.go:117-130`'s substring classification, which belongs to `healthy-typed-reason`; read in place it implies `Clean`'s reason is substring-matched, when preflight only prints it.
**Fix:** Move that bullet (and the "one behavioural-surface change" note) back under `healthy-typed-reason`.

## Verdict

GAPS_FOUND
Clean's typed reason is under-specified and the `host`/`Healthy`-test inventories are incomplete.
MILL_REVIEW_END
