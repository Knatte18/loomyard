MILL_REVIEW_BEGIN
# Review: Relocate producer prompt files into a stencils/ directory

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: claude-opus-4-x (Claude Opus, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-14
```

## Findings

### [BLOCKING:design] Stamp leaks into webster's composed prompts
**Section:** `stamp-format-and-edit-detection` + Technical context ("Webster composes rather than reads directly")
**Issue:** `render.go:60-77` concatenates prefix + body and `stencil.stripLeadingComment` (`stencil.go:67`) drops only the FIRST banner, so once `implementer-body.md` carries `<!-- lyx-stencil: sha256=... -->` that stamp line is delivered verbatim into the fork and recovery prompts — the discussion notes the existing double-banner behaviour and says "must not make it worse" without deciding anything, and this change makes it worse.
**Fix:** Decide the mechanism — strip every leading banner in `joinTemplateAssets`, or strip the second file's comment at compose time — and record it as a decision rather than a caution.

### [BLOCKING:design] Hash stability across checkouts / line endings undecided
**Section:** `stamp-format-and-edit-detection`, `no-automatic-merge` (base recovery)
**Issue:** The board copy is a git checkout, so on a machine with `core.autocrlf=true` the seeded LF file returns as CRLF, `hash(body) != stamp` and `!= hash(default)`, and every stencil is classified human-edited forever and never refreshed; `.gitattributes` pins loomyard's own copies by exact path (rows 7-10, 17-20) but the generated board repo has no `.gitattributes` at all, and `gitrepo/doc.go:218` records that go-git does no CRLF conversion, so the `diff <name>` blob-hash base lookup diverges from the on-disk hash on the same platform.
**Fix:** State the normalisation rule (e.g. hash over LF-normalised body) or the seeded board-side `.gitattributes`, and add the 15 new `stencils/**` paths plus removal of the 8 stale `internal/*` rows in `.gitattributes` to Scope.

### [BLOCKING:design] Name→default registry has no stated owner
**Section:** `stencils-is-a-go-package`, `stencilstore-ownership`
**Issue:** `stencilstore.Read(baseDir, "loom-template-discussion")` is name-keyed and `list`/`sync` must enumerate all 15, yet the decision exports one typed var per stencil and explicitly rejects `embed.FS`; nothing says who holds the name→default map, and a hand-maintained map reintroduces exactly the silent-omission failure the typed-var rationale claims to prevent.
**Fix:** Decide where the registry lives and how it is guarded against a `.md` present but unregistered; if any engine imports the top-level `stencils` package directly, say so — `seam_enforcement_test.go:24-29` would then need a second treadle allowlist entry beyond `internal/stencilstore`.

### [BLOCKING:consistency] Test tier / hermetic-env disposition contradicts the test list
**Section:** Constraints ("Test Tier Purity Invariant — satisfied"), Testing
**Issue:** The claim holds only for `stencilstore`; the promote round-trip, diff-base-recovery, seeding-commit-pathspec and mutation-record tests all require a real board repo, so they must carry an `integration` build tag and their packages need a `TestMain` calling `gitkit.HermeticGitEnv` — the Hermetic Git Test Environment Invariant is not mentioned anywhere in the document.
**Fix:** Scope the Test Tier Purity claim to `stencilstore`, and state the tag + `TestMain` obligation for the git-spawning tests and the new `stencilcli`/`fabricengine` test files.

### [BLOCKING:decision] Sandbox coverage choice left open
**Section:** Constraints ("Sandbox Suite Coverage")
**Issue:** The document restates the either/or from the invariant and never picks; `excludedModules` (`sandbox_coverage_test.go:28-32`) carries three entries today, and a `**Covers:** stencil` scenario is materially more work than an allowlist row.
**Fix:** Choose one — a named `*SUITE.md` scenario, or an `excludedModules` entry with its reason text — and record it in Scope.

## Verdict

REQUEST_CHANGES
Five unresolved design/decision gaps: stamp leakage, hash stability, registry ownership, test tiers, sandbox coverage.
MILL_REVIEW_END
