MILL_REVIEW_BEGIN
# Review: loom: interactive Discussion-Write

```yaml
duration_s: 236.0
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude Opus-class model (self-reported id claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-08-25
```

## Findings

### [NIT:consistency] Migration remedy is a dry-run verb
**Demoted-from:** BLOCKING
**Section:** Technical context, "Migration obligation for existing worktrees"
**Issue:** The stated remedy `lyx config reconcile` writes nothing — `internal/configcli/configcli.go:338-343` documents it as a dry-run and gates writes behind `--apply`, so an operator following the quoted instruction still fails the next `lyx loom run`.
**Fix:** State the remedy as `lyx config reconcile --apply` (noting the bare form only reports), and say so in the change-note/commit-message obligation.

### [BLOCKING:design] Ladder-step-1 rejection rests on an incomplete premise
**Section:** `resume-discriminates-on-live-agent-evidence-only` / `ladder-step-1-survives-only-inside-attach`
**Issue:** "a run whose output files are complete has already reached `Done` and cleaned itself up" omits that `shedengine` persists history only after `Call` returns (`internal/shedengine/run.go`), so a crash between `Run.finalize` and that persist leaves `current_producer: Discussion-Write` with both files present and nothing live — `Attach` reports `found == false` and the producer archives a *completed* interview and re-interviews from scratch. `leftover-run-dir-from-a-completed-run` routes the run-dir-survived variant to the same respawn without naming this cost.
**Fix:** Record this window as an explicitly accepted residual (as `bounce-still-re-interviews-from-scratch` is), and restate the step-1 rejection as "it would buy the completed-crash case at the price of the bounce ping-pong; we accept losing it" rather than "it buys nothing".

### [NIT:decision] Bouncer/judge rows keep the duplicate-agent path
**Section:** `attach-is-unconditional-not-interactive-only`; "On widening the shared seam"
**Issue:** The rationale is that respawning over a live agent is a correctness bug for every row, yet `shedadapters`' Bouncer judge run drives the same `Shuttle` seam (`shedrecipe/entries_bouncer.go:141`) and is left respawning, with no stated disposition in Scope or Out.
**Fix:** Add one line placing the Bouncer/Burler rows' own attach behaviour explicitly out of scope, with the reason.

### [NIT:scope] `docs/overview.md`'s loom.yaml key list not in the docs set
**Section:** Scope → Docs list
**Issue:** `docs/overview.md:319` enumerates loom.yaml's keys ("`discussion_timeout_min`/`plan_timeout_min`") and describes the Discussion producer; the new `discussion_interactive` key and the interactive mode are an observable behaviour change the list does not mention.
**Fix:** Either add `docs/overview.md:319` to the docs-in-the-same-commit list or state why that line is deliberately left as-is.

## Verdict

REQUEST_CHANGES
Two blocking items: a wrong migration command and an incomplete resume premise.
_Note: 1 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
