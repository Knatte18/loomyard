# Discussion review — cap-round override (r6)

Round 6 was the configured cap (`roles.discussion-review.holistic.rounds`, raised from 5 to 6 in `.millhouse/config.local.yaml` during this task).
It returned `GAPS_FOUND` with 3 gaps and 2 NOTEs.

Per an explicit operator instruction given before the round ran — "if you reach the review cap: just approve and hand off" — all five findings were verified against the code, fixed in `discussion.md`, and the task was advanced to `phase: discussed` without a confirming re-review.

## Fixed in the cap round (no confirming re-review)

- **[GAP] Bare-literal `_pattern` inventory not exhaustive** — verified. `junctionnames_test.go:44-45,54-55,172` and `structuraldirs_test.go:32,37` were absent from every list. The enumeration is now labelled indicative, with a `grep` sweep named as the authoritative closing step.
- **[GAP] `junctionnames_test.go:172` fixture unresolved** — verified. Resolved to an empty `[]string{}`, with each surviving name attributed to its real reservation source. Safe because the injected-junction-name sub-test carries its own local `{"_custom"}` fixture.
- **[GAP] `TestDeployedLyxPathspec_YieldsNoDuplicateLyx` intent undecided** — verified. Resolved to **keep** `_pattern`, since per the "No migration" decision that value persists in deployed repos and this is the only test exercising a real deployed pathspec.
- **[NOTE] Closing grep hits `raddle_guard_test.go`'s nine deliberate occurrences** — verified. An "expected residue" list now names that file, `structuraldirs_test.go`, and `loomengine/coherence.go`.
- **[NOTE] `raddle_guard_test.go`'s doc comment describes deleted geometry** — verified. The exemption is now stated to cover the file's prose too, with the reason: the guard asserts that `lyxcwd` never scans to enumerate directories, which holds independently of where raddle content lands.

## Pushed Back

None.
All five findings were factually accurate, and no fix conflicted with an operator decision.

## Consequence for `/mill-plan`

The five items above are the only changes in `discussion.md` that no reviewer has confirmed.
Everything from rounds 1-5 was re-reviewed at least once.
