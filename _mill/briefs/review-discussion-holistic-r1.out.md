MILL_REVIEW_BEGIN
# Review: Board fixes from sandbox run — payload keys, help, rerender

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-06-28
```

## Findings

### [GAP] set-status doesn't reject stale `phase`; silently clears
**Section:** Scope / Decision reject-unknown-keys (C) / Decision slug-or-id
**Issue:** The allowlist is scoped to upsert/upsert-batch/merge-upsert only; `get`/`set-status`/`remove` parse into a typed struct `{Slug,ID,Status}` that silently ignores unknown keys. An agent using the old vocabulary `set-status '{"slug":"T1","phase":"done"}'` leaves `Status` nil → SetStatus *clears* the status and succeeds — re-arming the exact silent-no-op the task exists to kill, on the single most-renamed command.
**Fix:** Decide how strict-key rejection applies to single-target commands (reject unknown keys / hint on `phase`), and resolve the related ambiguity: with `Status *string`, "status key absent" (mistake) and `"status":null` (intentional clear) both collapse to nil — specify whether set-status requires the `status` key present.

### [GAP] merge top-level keys not strictly validated
**Section:** Scope / Decision reject-unknown-keys (C)
**Issue:** Reject-unknown-keys covers merge's `upsert` *object* only. Merge parses top-level keys via a typed struct (`remove_slugs`/`upsert`/`set_status`), so a stale top-level `set_phase` (the most likely migration mistake) is silently dropped and the status step is skipped with no error — same silent-no-op class.
**Fix:** State whether merge's top-level keys are strictly validated too, or accept the gap explicitly.

### [NOTE] Allowlist placement left as either-or
**Section:** Technical context (task.go / store.go)
**Issue:** "The allowlist check goes here (or at the store boundary)" leaves NewTask/ApplyPatch vs store boundary undecided; both paths must cover create *and* patch and the merge-upsert path.
**Fix:** Pick one location and confirm it intercepts upsert, upsert-batch, and merge's upsert before the JSON round-trip.

### [NOTE] Manifest failure modes unspecified
**Section:** Decision manifest-cleanup (Q6/W13)
**Issue:** No behavior named for a missing manifest (existing boards upgraded pre-manifest have Home.md but no sidecar) or a corrupt/unreadable manifest, and whether manifest read/write errors are best-effort like today's `removeOrphanProposals` (which never fails a write) or fatal.
**Fix:** State that absent/corrupt manifest degrades gracefully (best-effort, no write failure) and that first post-upgrade render simply seeds the manifest.

## Verdict

GAPS_FOUND
Strict-key rejection is scoped only to upsert paths, leaving the phase→status rename footgun live on set-status and merge.
MILL_REVIEW_END
