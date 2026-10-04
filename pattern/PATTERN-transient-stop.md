# PATTERN-transient-stop

The transient mark is declared only in `internal/shedengine`, and its class set is closed at the declaration; the set is named there, not restated here.

- A lower-level package exposes its own classification, and `internal/shedtransient` alone translates it into the mark.
- The mark is set only at a producer boundary (the `Shed.Transient` classifier `shedbuild.NewShed` tells) or a step bootstrap boundary (a `PreStep` hook).
- A producer returns a failure `shedtransient.Class` classifies as a hard error rather than a verdict, so it reaches that classifier.
- A gate verdict (`blocked`, `awaiting`, `paused`) never carries the mark.

## Rationale

A mark added at a new site changes what an automated driver re-steps without a human deciding.
