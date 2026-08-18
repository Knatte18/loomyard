MILL_REVIEW_BEGIN
# Review: lift the orchestrator preflight out of loomengine, plus the shared standalone-CLI foundations

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-5 (self-assessed; Anthropic Opus line)
reviewed_file: _mill/discussion.md
date: 2026-08-18
```

## Findings

### [BLOCKING:consistency] Fabric Vocabulary Invariant stated as the wrong rule
**Section:** § Constraints, "Fabric Vocabulary Invariant / Fabric Write-Side Containment"
**Issue:** The discussion glosses this invariant as "`internal/preflight` is read-only; must not lock, write, or record a mutation" — that is the Write-Side Containment rule, which binds `package fabricengine` only. The actual vocabulary rule is that any production `.go` (identifiers, string literals AND comments) and any `internal/**/*.md` outside the owner set — `internal/preflight` is not in it (`fabricVocabularyOwners`, `enforcement_test.go:599-607`) — may not contain the bare tokens `weft`/`warp` at all (`bareVocabularyToken`, line 649-656). The new `doc.go`, `Wired`'s godoc, and the lifted check comments all describe a warp/weft-pair predicate and will trip `TestEnforcement_FabricVocabulary` unless written in Fabric vocabulary.
**Fix:** Restate the constraint correctly and record that every new `internal/preflight`/`internal/buildinfo`/`internal/standalonestate` comment, literal and `doc.go` must say "Fabric"/"worktree pair", following `loomengine/preflight.go:78-80`'s existing wording.

### [BLOCKING:consistency] docslink_test does not cover CONSTRAINTS.md edits
**Section:** § Testing, "Task-wide verify"
**Issue:** "`internal/lyxcwd/docslink_test.go` covers markdown link integrity for the `CONSTRAINTS.md` and `docs/overview.md` edits" is false for `CONSTRAINTS.md`: the Markdown Link Integrity invariant scopes scan sources to `manifest/` and `docs/` only, and explicitly names `CLAUDE.md`/`README.md`-class root docs as checked by nobody. The guard covers links *pointing at* CONSTRAINTS.md anchors, not links written *inside* the two new invariant sections.
**Fix:** Correct the claim — any link inside the new `Buildinfo`/`Standalonestate` invariant entries is a review obligation, not machine-checked.

### [BLOCKING:design] standalonestate normalisation contradicts "resolves no cwd"
**Section:** § Decisions → standalonestate-is-pure-derivation-with-an-injectable-seam; § Constraints (Cwd Resolution Invariant)
**Issue:** Normalisation is specified as `filepath.Abs` → `EvalSymlinks` → `Clean`, and § Testing requires "a relative and an absolute spelling of the same path agreeing" — but `filepath.Abs` resolves against the process working directory via `os.Getwd`, which the Constraints section of this very discussion forbids ("must not resolve a cwd at all (it is told a target path)"), and which makes the supposedly pure `derive(goos, env…, target)` seam host-cwd-dependent for exactly that test row. `lyxcwd`'s `normalizePath` (`anchor.go:121-129`), named as the semantic model, contains no `Abs` — an unacknowledged second deviation beside the case-fold one. `Abs`'s own error return also has no stated disposition against "returns an error only when the state root cannot be determined".
**Fix:** Decide explicitly — either `Derive` requires an absolute target and rejects a relative one (dropping the relative/absolute test row), or relative input is supported and the cwd it resolves against becomes a seam parameter, with the `Abs` error path classified.

### [NIT:consistency] preflight/export_test.go shim has nothing to export
**Section:** § Decisions → preflight-tests-are-an-external-test-package; § Technical context "Existing helpers to reuse"
**Issue:** The external tests are said to drive `CheckResolved` "through a `preflight/export_test.go` shim", but `CheckResolved(l *lyxcwd.Location)` is exported by the same discussion's entry-point decision — `package preflight_test` can call it directly, so the shim would be dead code.
**Fix:** Drop the `export_test.go` from `internal/preflight`'s artefact list, or name the unexported symbol it actually re-exports.

### [NIT:decision] Which docs/shared-libs/README.md section the three entries land in
**Section:** § Scope (In), last bullet
**Issue:** That file has two sections with different contracts — `## Libraries` entries each carry a dedicated `<name>.md` doc file, `## Implementation-only libraries` entries are one bullet documented in code. "Entries in `docs/shared-libs/README.md`" does not say which, and the Scope list names no new `.md` files.
**Fix:** State that the three land under `## Implementation-only libraries` (no new per-package doc file), or list the new doc files.

## Verdict

REQUEST_CHANGES
Three blockers: mis-stated vocabulary constraint, false docslink coverage claim, contradictory cwd handling in Derive.
MILL_REVIEW_END
