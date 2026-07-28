All 6 cards in batch `pattern-junction-flip` are committed (6 of 6), matching each card's `Commit:` message exactly. The batch verify command passed cleanly across all six packages.

Summary of work:
- **Card 15** (`3bf96a9a`): `internal/hubgeometry/hubgeometry.go` — `HostJunctions`/`HostJunctionsHere` now return `_lyx` then `_pattern`; updated `internal/hubgeometry/hubgeometry_test.go` and `internal/hubgeometry/weft_test.go`.
- **Card 16** (`dab1f102`): `internal/initengine/init.go` — added `PatternDir`, restructured to observe each **host** junction path pre-wiring (not the weft-side target, which can inherit `_lyx` content from branch history and would give a false "exists"); `internal/initcli/initcli.go` adds `pattern_dir`; tests in `internal/initengine/init_test.go`, `internal/initcli/initcli_test.go`.
- **Card 17** (`d106d21e`): `internal/initengine/undo.go`/`undo_test.go` — pin the deliberate `_lyx`-only clearing (weft `_pattern` preserved); `internal/initcli/initcli.go` help text updated.
- **Card 18** (`4df683dc`): `internal/fabricengine/status.go` — `detectHostPollution` now treats `_pattern` like `_lyx` (restorable), never like `_raddle`; new test in `junction_pattern_integration_test.go`.
- **Card 19** (`9c0f956d`): extended `junction_pattern_integration_test.go`, `junction_repoint_test.go`, `remove_junctions_integration_test.go`, `preflight_integration_test.go` to cover the genuine two-junction world (reconcile/status/drift per-site, per-junction repoint/refusal, nested-Remove, legacy-upgrade path).
- **Card 20** (`92255e89`): `docs/overview.md`, `internal/fabricengine/doc.go`, `internal/fabriccli/fabric.go` — documented the second junction, fixed the pre-existing `_raddle`-junction doc error, and documented the upgrade consequence + updated cobra help text.

One deliberate deviation from the plan's literal wording, called out in the Card 16 commit message: the plan said to observe the *weft-side* target pre-wiring, but that signal is unreliable (a weft branch forks from its parent's weft branch and can already carry `_lyx` content unrelated to whether this host worktree ran `Init`) — verified empirically against the `CopyPairedLocal` fixture. I used the **host** junction path instead, which satisfies the same documented requirement (created/exists on first/second run) without touching `internal/lyxtest` (out of scope).

Final JSON status line:

{"status":"success","commit_sha":"92255e8927d4c86e2a94456ba896437b1c8f318","session_id":"345dfd54-bcc9-47ae-8f82-f39532f70afa","cards_done":[15,16,17,18,19,20]}
