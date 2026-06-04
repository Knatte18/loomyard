# Review: wiki-go-port

```yaml
verdict: GAPS_FOUND
reviewer_model: sonnetmax_tool
reviewed_file: _mill/discussion.md
date: 2026-06-04
```

## Findings

### [GAP] `list` output schema omits computed fields
**Section:** § CLI surface / Technical context → `_server.py`
**Issue:** Python `_handle_list_tasks_brief` injects two computed fields into every row — `layer` (from `compute_layers`) and `has_proposal` (from store) — that are absent from the Go `Task` struct and unmentioned in the discussion. Mill-skill callers depend on `layer` being present. The plan writer cannot determine what the `list` subcommand must return.
**Fix:** Specify the full output schema for `list`: whether it returns the Task struct verbatim, or enriches it with `layer` and `has_proposal`, and whether those fields live on a wrapper type or are injected ad-hoc.

### [GAP] `upsert-batch` subcommand absent despite "full parity" claim
**Section:** § CLI surface → Q&A log ("Full CLI parity or reduced v1? A: Full parity.")
**Issue:** The listed subcommands (`upsert`, `set-phase`, `remove`, `get`, `list`, `list-full`, `merge`, `set-deps`, `rerender`) map to all Python server ops except `OP_UPSERT_TASKS_BATCH`, which is a non-daemon store operation. It is not listed under "Out" and not mentioned in the Q&A.
**Fix:** Either add `upsert-batch` to the subcommand list or explicitly exclude it in § Scope → Out with a reason.

### [NOTE] `remove` not-found behavior undefined at CLI surface
**Section:** § Testing → `store.go` / `_server.py` line 217–224
**Issue:** The discussion says "`remove_task`: no-op if slug not found" (store layer), but the Python server returns `{"ok": false, "error": "task not found: ..."}` for not-found slugs. The CLI-level behavior is unspecified.
**Fix:** Clarify whether the `remove` subcommand exits 0 with `{"ok":true}` or exits 1 with an error when the slug does not exist.

### [NOTE] `merge` subcommand `set_phase` JSON shape unspecified
**Section:** § CLI surface / Technical context → `_server.py` line 317, `_store.py` merge_tasks signature
**Issue:** Python `merge_tasks` takes `set_phase` as a Python tuple `(identifier, phase_value) | None`, which serialises as a JSON array `[identifier, phase]`. The discussion doesn't document the JSON schema for the `merge` payload, leaving the `set_phase` field's type ambiguous (array vs. object vs. omitted).
**Fix:** Document the JSON payload shape for `merge`, specifically `set_phase: [id_or_slug, phase | null] | null`.

## Verdict

GAPS_FOUND
Two blocking gaps: `list` output schema and missing `upsert-batch` subcommand require resolution before planning.