# PATTERN-friction-capture

With Tier 2 on, a loom halt, a loom crash-resume and every webster refusal leave a Go-authored friction note, and no note is archived or deleted before a reflection has covered it.

- A loom halt (`blocked` or `failed`, under `run` or `step`) and every `lyx webster` refusal but `validate`'s write one friction note, through `friction.NotePath`, from `loomcli` and webster code respectively.
  An escalation halt writes its note but spawns no reflection, and the next reflection covers the note.
  A `run` or `step` entry that detects a crash-resume writes a `loom-crash-resume` note the same way.
- A webster refusal cobra raises before the verb's `RunE` (flag parsing, argument count, the persistent pre-run) fires before the friction directory is resolved and is not noted.
- A note-write failure never changes the verb's exit or envelope.
- No code path archives or deletes a friction note before a reflection has covered it: `frictionengine.Reflect` archives only the notes its covered-notes record names, and only after a clean return or on a finished report.
- The reflection files through `lyx selfreport create` only.
- Go-detected anomalies become friction notes: a halt note carries its anomaly kind and history rows, and `lyx loom start` vouches for the resume it spawns, so a deliberate resume never reads as a crash.
  The reflection files an issue for such a note only when the notes, the reason or the trace show a lyx problem behind the event.
  When it misreads a halt, a real lyx fault can stay unfiled, but the evidence stays recoverable: the note names the trace file, the run's status and history stay on disk, and the operator can still file through `lyx selfreport create`.

## Enforcement

The `loomcli`, `webstercli`, `websterengine` and `frictionengine` tests, including the `halt_test.go` and `entryobservation_test.go` cases for the anomaly kind, the history rows and the voucher, and review for the rest.
