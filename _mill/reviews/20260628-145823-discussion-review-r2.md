I have verified the discussion's claims against the actual source. The discussion is detailed and well-grounded; line references check out (e.g. `SetPhase` silent no-op at store.go:352-353, `group` rejection at task.go:30/77, `removeOrphanProposals` glob at render.go:41, helptree pin at helptree_test.go:51). Two coverage inconsistencies in the strict-key scope remain.

MILL_REVIEW_BEGIN
# Review: Board fixes from sandbox run — payload keys, help, rerender

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-28
```

## Findings

### [GAP] merge's inner set_status object validation unspecified
**Section:** Scope / Decision reject-unknown-keys + slug-or-id lookup
**Issue:** The reject-unknown-keys enumeration covers single-target payloads only as `get`/`set-status`/`remove`; it never states whether `merge`'s inner `set_status` object also gets the `{slug,id,status}` allowlist, exactly-one-of `slug|id`, and status-required rules — so a `set_status:{"slug":"x","phase":"done"}` inside merge could re-arm the exact rename footgun on the most-renamed command.
**Fix:** State that `merge`'s inner `set_status` object is validated identically to `set-status` (unknown-key rejection, exactly-one-of, status-required) and add a merge-inner test mirroring the set-status cases.

### [GAP] set-deps omitted from unknown-key rejection
**Section:** Scope (line 56 "every board write and lookup payload") vs. Decision reject-unknown-keys enumeration
**Issue:** The general rule says *every* board write payload rejects unknown keys, but the enumeration lists only upsert/upsert-batch/merge/get/set-status/remove — `set-deps` is absent, yet it replaces `depends_on` wholesale, so a typo'd key (`"depends"`) would silently clear a task's deps (a W11-class silent drop).
**Fix:** Decide explicitly whether `set-deps` rejects unknown keys (allowed set `{slug, depends_on}`); if yes, add it to the enumeration and a test, if no, state why it is exempt.

### [NOTE] id=0 is a valid lookup target
**Section:** Testing / Decision slug-or-id lookup
**Issue:** `store.nextID()` returns 0 for the first task (store.go:99-101), so `{"id":0}` is a legitimate lookup; the exactly-one-of and presence detection must key on JSON-key presence (the map decode), not the int zero-value, and the test list mentions int-vs-float64 but not the id=0 boundary.
**Fix:** Add a test that `get '{"id":0}'` resolves the first-created task and that absent `id` is distinguished from `id:0`.

## Verdict

GAPS_FOUND
Strict-key scope is inconsistent on merge's inner set_status and on set-deps.
MILL_REVIEW_END
