MILL_REVIEW_BEGIN
# Review: landing: Publish + Finalize producers

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-19
```

## Findings

### [BLOCKING:design] Resolved conflicts are never staged
**Section:** `verify-before-conclude` / Constraints (Fabric Git Invariant)
**Issue:** `MergeContinue` hard-refuses while `gitrepo.ConflictedFiles` (`git diff --name-only --diff-filter=U`, an *index* probe) is non-empty; editing a file's content does not clear its unmerged index entry, so after the LLM session every path is still unmerged — and no actor may `git add`: the agent is forbidden git, `mergeresolve` is bound by the Fabric Git Invariant, and `internal/fabricengine`'s merge surface exposes no staging verb.
**Fix:** Decide who stages the resolution (a new `fabricengine` verb, or a `MergeContinue` that stages resolved paths itself) and record it; the marker-scan verification also needs restating against the index check that actually gates.

### [BLOCKING:design] No decided way to obtain a `*Fabric` handle
**Section:** `finalize-merge-geometry`, `told-values-via-landingshed-deps`
**Issue:** `fabricengine.Open(l *lyxcwd.Location)` is the only exported constructor (`newPaired` is unexported), so "open a second `Fabric` handle on the told parent worktree path" requires a direct `internal/lyxcwd` import — exactly what both new packages' `seam_enforcement_test.go` will forbid; the existing precedent (`webstercli`/`perchcli`'s injected `openFabric func() (*fabricengine.Fabric, error)` closure) is never named, and nothing says who resolves the *parent* worktree's Location.
**Fix:** State the seam explicitly — injected open-closures (task and parent) in `landingshed.Deps`, filled by the CLI layer — or a path-based `fabricengine` constructor.

### [BLOCKING:design] `origin` → owner/repo has no mechanism
**Section:** `publish-repo-resolution`
**Issue:** No package in the repo reads a remote URL today (`internal/gitrepo` has no such method; `selfreportengine` uses a hardcoded constant), so this needs new code the discussion never places — and adding a `gitexec` call inside `gitrepo` engages the gitrepo Client Boundary and gitexec Checked-Call Invariants; URL-form parsing (SSH vs HTTPS, `.git` suffix) and the missing/non-GitHub-`origin` failure mode are undecided.
**Fix:** Name the owning package and API for the read plus the parse, and state the behaviour when `origin` is absent or not a GitHub URL.

### [BLOCKING:scope] Registration artifacts missing from the inventory
**Section:** Scope / In
**Issue:** Three registration sites that make the listed artifacts live are absent: `contracts/stencils/stencils.go` needs the embed var and `entries` row (`registry_test.go` fails both directions on an unregistered `.md`); `internal/configreg`'s `Modules()` needs a `"landing"` entry plus its `configreg_test.go` `want` list, or `lyx config reconcile` never sees `landing.yaml`; `internal/loomshed/seam_enforcement_test.go`'s `loomshedAllowedImports` must gain `internal/landingshed`.
**Fix:** Add all three to the In-scope list.

### [BLOCKING:scope] Inbound-link inventory is incomplete and mis-cited
**Section:** Constraints (Markdown Link Integrity)
**Issue:** Two live inbound links to `manifest/designs/landing.md` are unlisted — `manifest/designs/raddle.md:55` (anchored: `landing.md#raddle-regeneration--part-of-the-merge-not-a-step-before-it`) and `manifest/designs/fabric-unified-view.md:227` — and the cited line numbers are wrong (loom.md links are at 40, 41, 48, 59; shed.md at 3, 62, 298, 309), so the enumeration method itself is unreliable.
**Fix:** Re-enumerate by grep over `manifest/` and `docs/` and cite files rather than line numbers; also repoint `internal/loomshed/loomshed.go:19`'s prose reference.

### [NIT:consistency] Roadmap corrections misstate the current text
**Section:** `docs-lifecycle-landing-md-deletes`
**Issue:** The Someday item (`roadmap.md:129-130`) already reads "Only the ordinary-git-conflict shape shipped; the document shape is not built", so no correction is needed there; meanwhile item 1's own body — "conflict-shape detection", "`_lyx` teardown", "Returns `Done` once the PR exists" — contradicts this discussion, and "move from Planned to Done" does not say to rewrite it.
**Fix:** Drop the Someday-item claim and state that item 1's body is rewritten to match what ships.

## Verdict

REQUEST_CHANGES
Two merge-mechanics gaps, an unplaced git read, and two incomplete inventories.
MILL_REVIEW_END
