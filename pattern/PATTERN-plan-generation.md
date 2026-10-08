# PATTERN-plan-generation

`_lyx/plan/`'s top-level files hold exactly one plan generation.

- A retired generation lives only under the rework round that retired it (`round-<N>/prior-generation/`), except where Plan-Write's own `archive-*/` rotation moves the live plan aside on a re-run.
- Only PR-Rework archives a generation into a round.
- Webster's run record and the Plan-Review and Webster-Review run directories are generation-scoped: they move into the round with the plan, so the next generation starts with none.
- Plan-Write's `archive-*/` rotation moves Webster's run record too, into the archive's `webster` subdirectory and before any plan file moves, so a re-planned run starts a new Webster run instead of looping on `plan_drifted`.
  The Plan-Review and Webster-Review run directories stay out of that rotation.
- `internal/loomshed` declares the round layout once.
  Every path is built from the existing accessors, and webster receives its archive destination as a told path.
