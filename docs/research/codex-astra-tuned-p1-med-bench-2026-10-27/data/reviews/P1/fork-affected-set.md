Affected-set list:

- Gate-key/config shape: `Config`, migration, renderers, Webster sites, loom/darn wiring, landing verification, fixtures and docs — cards 1–6, 22.
- Green carry and diff bases: scope planning, slot execution, records and summary readers — cards 7–8, 14.
- Launch/report output: counters, run records and reporting — cards 9–10.
- Tier tags: impactset/planglyph loads, plan/shell guards, timing tool, purity/isolation scans and repository test headers — cards 11–16, 24–33; one package loader remains uncovered.
- Entry gates/skipped history: Shed persistence, goto/relaunch, budgets, activity, coherence, checklist and reporting — cards 17–19; additional history readers remain uncovered.
- Batten switches and darn budget routing — cards 20–21.

Findings:

1. **MEDIUM · design · card 11, with cards 24–32**
   **The tag-load inventory omits the tree-sitter link guard’s package load.**
   `cmd/lyx/treesitterlink_integration_test.go:40` loads test dependencies using only `integration,tmux,llm`.
   Its documented contract is to inspect every tier’s test binaries (`:33–34`), and it checks both missing/stale allowed links and a minimum binary count (`:51–62`).
   Card 11 updates only impactset and planglyph loads, while card 24 only retags this file’s header.
   After retagging, this guard no longer sees imports exclusive to ordinary `tier0`–`tier3` test files, weakening its enforced dependency boundary and potentially producing stale-allowlist failures.
   This is a missing member of D5’s package-load conversion, not a missing authored dependency edge.

2. **MEDIUM · design · cards 17–18**
   **The skipped-history reader inventory misses episode-ending readers and incorrectly assumes a skip cannot name Darn-Write.**
   `internal/shedengine/reroute.go:14–27` resets the reroute episode only at `Done` or `RerouteCapped`; a skipped row therefore leaves its old reroute count active.
   `internal/battenshed/classify.go:190–219` likewise closes failure episodes only when `countDone` increases, so passing a failed row by skipping it does not end that episode.
   `internal/loomcli/darnwiring.go:110–131` selects the latest Darn-Write history entry while excluding only `OutcomeReroute`; it would return `skipped` as the writer’s outcome.
   Card 18 explicitly declares this last reader unaffected because a skip “never names” Darn-Write, but card 19 permits an entry gate on any recipe row.
   Cards 17–18 should cover these readers’ treatment of skipped rows, consistent with D8’s rule that skips are passage through a row rather than producer calls.

Tool-call accounting:

1. Searched gate/config/tag/history references beyond the manifest to enumerate affected consumers.
2. Read the tree-sitter link guard and searched remaining tag/config/switch consumers; the manifest contained none of these definitions.
3. Read tier planning and counters, and searched repository tag guards; these consumers were absent from inherited implementation reads.
4. Read Batten classification and Shed reroute references, plus test imports, to trace skipped-history and tier-load effects absent from the manifest.
5. Read reroute counting, gate-config fixtures, test dependency helpers and scankit behavior to check concrete consequences.
6. Read remaining outcome selection and hand-step readers, fixture declarations and test-economy rules to distinguish affected readers from readers already compatible with skips.
