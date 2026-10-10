# PATTERN-shuttle-stop

Stopping a shuttle run's strand from Go outside `internal/shuttleengine` goes only through shuttle's stop verb, which settles the run's record first.

- An attach probe or `Wait` reads the run's record, so a strand removed before the record is settled looks like a run that vanished.
- Two removals are exempt: `loomcli`'s removal of the loom driver's strand and `orchcli`'s removal of the orch strand.
  No attach probe or `Wait` reads their records, so there is nothing to settle.
