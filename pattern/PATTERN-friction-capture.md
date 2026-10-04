# PATTERN-friction-capture

With Tier 2 on, a loom halt and every webster refusal leave a Go-authored friction note, and no note is archived or deleted before a reflection has covered it.

- A loom halt (`blocked` or `failed`, under `run` or `step`) and every `lyx webster` refusal but `validate`'s write one friction note, through `friction.NotePath`, from `loomcli` and webster code respectively.
- A webster refusal cobra raises before the verb's `RunE` (flag parsing, argument count, the persistent pre-run) fires before the friction directory is resolved and is not noted.
- A note-write failure never changes the verb's exit or envelope.
- No code path archives or deletes a friction note before a reflection has covered it: `frictionengine.Reflect` archives only the notes its covered-notes record names, and only after a clean return or on a finished report.
- The reflection files through `lyx selfreport create` only.
- With the `selfreport` knob on, Tier 1 anomaly filing files a halt event under `run` and `step` alike, retrying a failed filing on every later pass, and the halt note says so.
  The reflection files only the lyx problems behind the halt.

## Enforcement

The `loomcli`, `webstercli`, `websterengine` and `frictionengine` tests, including the `selfreport_test.go` and `halt_test.go` cases for the pending retry and the `step` hook, and review for the rest.
