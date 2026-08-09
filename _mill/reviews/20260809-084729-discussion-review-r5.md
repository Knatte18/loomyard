MILL_REVIEW_BEGIN
# Review: builder: delete internal/builderengine and internal/buildercli, retire builder-contract.md as a reference

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-09
```

## Findings

### [NIT:consistency] Acceptance grep overrides the loom.md deferral
**Demoted-from:** BLOCKING
**Section:** `loom-md-links-fixed-prose-deferred` vs Testing → Acceptance commands
**Issue:** `manifest/designs/loom.md:91`, `:94`, `:187` literally contain `internal/builderengine`/`internal/buildercli` (`:91`, `:187`) and a bare non-link `builder-contract.md` mention (`:94` — there is no link there to "fix"), and loom.md is not on the grep exclusion list, so the package-name and `builder-contract` zero-hit patterns force exactly the prose rewrite the decision assigns to task E and promises to leave untouched.
**Fix:** State the resolution explicitly — either add `loom.md:91/:94/:187` to the enumerated grep exclusions (with E named as the follow-up owner), or drop the deferral and claim those three sentences for this task.

### [BLOCKING:design] Bare-word exclusion list does not cover the tree
**Section:** Testing → "Ordinary-English and unrelated-fixture false positives are excluded by an enumerated token list, never by judgment"
**Issue:** The load-bearing bare-word gate cannot reach zero hits as enumerated: `internal/shuttleengine/claudeengine/command_test.go:203` ("the same builder produces the"), `internal/gitrepo/parity_test.go:169` ("no fixture builder in this package" — the list has plural "fixture builders" only), and `plugins/prowler/scripts/run.sh:72` ("a builder that died between mkdir and") are all ordinary-English hits outside the six listed tokens; `plugins/` is never mentioned in Scope In or Out at all.
**Fix:** Re-derive the exclusion list from an actual repo-wide bare-word scan (including `plugins/`, `tools/`, `sandbox/`) and state the scan's directory scope, so "mechanical, never by judgment" is true rather than asserted.

### [NIT:decision] No disposition for on-disk `phase: "builder"` status files
**Demoted-from:** BLOCKING
**Section:** `phase-rename-builder-to-webster` / Scope → Out
**Issue:** `internal/loomengine/coherence.go:14–22` gates `validPhases`, and `:58` emits `CheckSeedIncoherent` for an unknown phase, so any in-flight worktree whose durable `_lyx/.../status.json` (`LoomStatusFile`) carries `phase: "builder"` hard-fails preflight after the rename — a strictly harder failure than the inert `builder.yaml` case, which *did* get an explicit carve-out.
**Fix:** State the chosen disposition — accept the break, keep `"builder"` as a transitional accepted value, or migrate on read — in the same decision that owns the rename.

### [NIT:scope] Inventory misses several production builder references
**Section:** Technical context → Go sites (comment-only)
**Issue:** Not listed: `internal/pattern/doc.go:40` ("builder implementer, webster fork, loom plan"), `internal/scoutengine/doc.go:33` ("e.g. builder or webster"), `internal/webstercli/validate.go:7`, `internal/webstercli/status.go:5`, `:9`; `pattern/doc.go:40` is a third mirror of the pair (`CONSTRAINTS.md:106` / `pattern/leaf_enforcement_test.go:3`) the discussion says to keep consistent.
**Fix:** Add them, or restate plainly that the whole comment inventory is illustrative and only the bare-word gate is binding.

### [NIT:consistency] S9 span line numbers off by one
**Section:** Markdown sites → `SANDBOX-CORE-SUITE.md`
**Issue:** The `**Verdict:**` line is `:285`, not `:284` (heading `:224` and closing `---` `:287` are correct); the block also contains two internal `---` fence lines at `:237`/`:240` inside a code block, which a naive separator-balancing edit could trip on.
**Fix:** Cite the span by content (heading through its `**Verdict:**` line) as the technical-context rule already requires, and note the code-fence `---` lines.

## Verdict

REQUEST_CHANGES
Grep gate contradicts the loom.md deferral, exclusion list incomplete, phase-rename data disposition unstated.
MILL_REVIEW_END
