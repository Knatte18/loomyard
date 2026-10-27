Recovery audit:

- Card 1, empty/unreadable hostname → give the machine a hostname or fix the reported OS error; progresses.
- Card 3, changed owner payload → `lyx adopt`; progresses through the dedicated setter.
- Card 3, owner-write fetch/push failure or deferred push → restore origin, `lyx board sync`, rerun; progresses.
- Card 8, start/resume owned elsewhere → adopt from the prime; progresses.
- Card 8, offline resume with a local foreign owner → rerun once origin answers; progresses.
- Card 9, online foreign owner → adopt; progresses.
- Card 9, offline local foreign owner → adopt alone; does not provide D3’s prescribed offline recovery.
- Cards 10–11, child owner refusal or wake detection → owner pause and adopt; progresses through the owner-label lift and resumed owner check.
- Card 12, run/parent/both ownership halt → the corresponding adopt forms; progresses, including `--keep-local` where local records or a live pair require it.
- Card 12, unreachable origin or owner-read error → rerun loom resume after restoring the prerequisite; progresses.
- Card 12, adopter-first parent-push failure → D6’s reset/rebase instructions, then fabric removal; the plan carries the decision record’s prescribed repair.
- Card 13, failed owner pause → next wake or orch start; recovery is named.
- Card 16, invalid selection → board list/promote or the completed-run resume; appropriate remedies.
- Card 16, local divergence/live pair → `--keep-local` or eligible `--drop-local`; progresses.
- Card 16, live-pair drop refusal → `--keep-local`; progresses.
- Card 16, missing-pair add refusal → remove the leftover branch and rerun adopt; progresses.
- Card 16, failed/lost owner push → rerun adopt; resumes nothing prematurely.
- Card 16, surviving ordinary pause → `lyx resume <slug>` or hub `lyx resume`; progresses through the existing lift semantics.

Finding:

**MEDIUM · decision · `_lyx/plan/09-loom-owner-check.md:31–38` — Preserve the unreachable-origin distinction in the shed owner refusal.**  
`runOwnerVerdict` falls back to the local Bolt when origin is unreachable, but its planned result contains only `Owner` and `Elsewhere`.
Consequently `refuseOwnedElsewhere` gives the same unconditional `lyx adopt <slug>` instruction for both online ownership and potentially stale offline ownership.
Adopt itself refuses unreachable origin, whereas D3 explicitly requires this offline refusal to name the owner and a rerun once origin answers (`_lyx/discussion/decision-record.md:69–71`).
Retain the unreachable/local provenance in the verdict, prescribe the offline rerun message, and extend the planned start/resume table to assert it.
The existing verbs have a suitable pre-bootstrap refusal point (`internal/loomcli/start.go:503`, `internal/loomcli/resume.go:263`).

Tool-call reading log:

1. Read `fabricengine/replay.go`’s parent-work detection, D6/D7 repair wording and numbered card 12 repair lines; the inherited reading lacked the full repair contract and detection implementation.
2. Read `landingshed/stuck.go`, shed halt-persistence references and `battenshed/innerrun_pause.go`; the inherited reading lacked how owner-halt reasons persist and how pause recovery suppresses spawns.
3. Read network-gate installation, `runInWindow`, tender’s exact-entry lift and InnerRun’s parent-lock release; the inherited reading lacked the recovery operations invoked by adopt and pause lifting.
4. Read parent-tip/HEAD accessors and located the exact card 9 and D3 offline-refusal lines; the inherited reading lacked the accessor semantics and numbered evidence for the recovery distinction.
