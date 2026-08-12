MILL_REVIEW_BEGIN
# Review: lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency

```yaml
duration_s: 105.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-12
```

## Findings

### [BLOCKING:design] SkipDir on a link skips its sibling junctions
**Section:** "Teardown discovers junctions by walking, not by slug"
**Issue:** The decision says teardown removes a link and "returns `filepath.SkipDir`", but `filepath.WalkDir` returns a link as a **non-directory** entry (`fslink.Remove` aside, both `fslink_linux.go:24` and `fslink_windows.go:164` treat junctions/symlinks as links, and Go's `WalkDir` never follows them), and `SkipDir` from a non-directory callback skips *the remaining entries of the containing directory* — so removing `<hub>/_portals/<slug1>` abandons `<slug2>…`, and removing `<worktree>/.lyx` abandons `_board`/`_lyx`.
**Fix:** State the traversal rule in terms Go actually implements — `WalkDir` does not descend into links by construction, so the callback removes the link and returns `nil`, and record that no `SkipDir` is emitted for a link entry.

### [NIT:consistency] fabricengine's in-package CopyWeft count exceeds its total
**Section:** "Measured blast radius of the `fabriccli` dependency set" vs "`Copy*` call sites"
**Issue:** The blast-radius row claims 43× `CopyWeft` among fabricengine's *in-package* files while the call-site table gives 40 for the whole package; measured now, `lyxtest.CopyWeft(` occurs 40× in `internal/fabricengine`, of which 2 sit in the external `weftgit_exclude_test.go`, so the in-package figure is 38.
**Fix:** Re-derive the blast-radius row with the same call-expression method the r1 answer pinned, so the two tables reconcile.

## Verdict

REQUEST_CHANGES
Teardown's stated SkipDir rule would leave sibling junctions un-removed on both platforms.
MILL_REVIEW_END
