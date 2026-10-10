# PATTERN-shuttle-stop

A shuttle run's strand is stopped from Go only through shuttle's stop verb, which settles the run's record first.
Removing the strand any other way leaves the record saying the run is live, and an attach probe or a Wait then reads a strand that is gone.

- `internal/shuttleengine` owns the stop verb; every other removal of a run's strand goes through it.
- `internal/reedengine` owns the strand removal itself, and `internal/reedcli` exposes it as reed's own remove verb.
- `internal/testkit/shuttlefake` is the test fake of the stop seam.
  Both it and `reedengine` declare `RemoveStrand` and select it nowhere, so the scan carries no entry for them; a selector in either is a finding until an entry with a reason is added.
- `internal/loomcli` removes the loom driver's strand and `internal/orchcli` the orch strand.
  Both are exempt because no attach probe or Wait reads their records.
- Enforced by `cmd/lyx/removestrand_test.go`, which fails on a selector expression selecting `RemoveStrand` in a package outside that list, so a method value handed to a wrapper is caught as well as a call.
- Bound: the scan sees the selector, not whose strand it removes.
  A call inside an allowed package on a run's strand that bypasses the stop verb is caught by review.
