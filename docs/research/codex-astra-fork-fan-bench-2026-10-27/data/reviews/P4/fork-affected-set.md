Affected-set inventory:

- Machine name: hostname readers and leaf-test helpers → card 1. No existing production `os.Hostname(` caller was found.
- Board owner shape: `Task`, embedded decoding aliases, store loading/saving, replay, payload validation and run-lock comparison → cards 2–3. `decodedEntry` embeds a defined `Task` alias, so the new field propagates without another decoder edit.
- Board owner views: brief lists, `Find` through `ListTasksBrief`, get envelopes, README rendering and its golden → card 4. The Running table is constructed inside `renderReadme`, which the card targets.
- Reconcile state and summary: classification, mover, rejected weft push replay and all production reconcile consumers → cards 5–6, 8–9, 11–13. Cleanup’s custom options extend `HubSeamOptions`, so it inherits the reader.
- Pause caller and coverage: pause creation, existing coverage/pause tests, wake’s fake pause writer, InnerRun and adopt’s lift identity → cards 7, 10–11, 13, 16.
- Batten ownership: arming, claim, resume, window launcher, step, status and existing claim-refusal assertions → card 8.
- Shed ownership refusal: both bootstrap verbs, shared refusal-kind constant and child-output mapping → cards 9–11.
- InnerRun owner seams: constructor defaults, wait/spawn paths, lock harness and production wiring → cards 10–11. Existing seam reflection consumers also need accounting.
- Finalize owner seams: producer, loom wiring, landing-deps reflection test and existing landing tests → card 12; batten’s deliberately unused Landing seams are omitted.
- Wake/orch report shape: reconcile mapping, wake notice/pause, orch envelope and launch harness → card 13.
- Tender owners: status report, status reader, hub assembly, status tests and documentation → card 14.
- Reopened windows: reed client read, batten pre-run, help and documentation → card 15. Existing attachment tests assert attachment/window-selection behavior rather than the new boolean client-read contract.
- Adopt: root registration, sandbox exclusion, destruction recorder/result registries, owner setter, pause lift, resumes and clone fixture → card 16.
- Orch/PATTERN prose → card 17.

Finding:

**MEDIUM — design — `_lyx/plan/12-finalize-owner-checks.md`, new `landingshed.Deps` fields.** The plan updates the populated landing-deps consumer but omits the deliberately empty Landing consumer in batten’s seam guard.

Card 12 introduces four new function seams on `landingshed.Deps`. Batten’s `wire` leaves its entire Landing environment unused. `TestWire_EverySeamFilled` walks `c.env` and rejects every unexpected nil function; its companion `intentionallyNil` explicitly enumerates unused `Landing.*` fields. Neither that map nor the shared optional-seam policy is addressed by any card. Thus specifying nil defaults in Finalize does not by itself account for this source-inspection consumer.

Evidence: `internal/battencli/wire_seams_test.go:32` describes the explicit nil policy; lines 50–69 enumerate the existing unused Landing fields; lines 115–125 reject unexpected nils and stale exemptions. Card 12 covers `TestLandingDeps_EveryFieldPopulated`, but no card covers this second seam guard. The handler should inspect `envkit.NilSeams`’s exemption implementation before retaining the finding, because that implementation was outside my six-call evidence budget.

Tool-call record:

1. Searched board field/codec consumers, cross-CLI imports, README golden assertions and write entry points; the manifest lacked those downstream inventories.
2. Read inherited board decode aliases, brief-list consumers and README render plumbing; searched hostname readers and reconcile-state consumers absent from the manifest.
3. Searched aggregate-record test consumers and reconcile callers; read cleanup’s CLI and the landing-deps constructor boundary to identify indirect consumers.
4. Read batten and loom seam reflection tests, cleanup’s extended options and push/Bolt call paths; these consumer contracts were absent from inherited reading.
5. Inspected destruction registry coverage, board test consumers and landing parent-guard references to check consequences beyond the listed targets.
6. Read README table construction and batten’s explicit nil-seam registry; the manifest had not included those definitions.
