Affected-set inventory:

- Task and prime owner JSON: `Task`, its reflection-based key sets, `decodedEntry`, `storedEntryFrom`, store load/save, replay, scope locking and payload writers; cards 2–3 cover the changes, and the decoder helpers inherit `Task`’s new field automatically.
- Board views: brief listing, get envelope and README Running table; card 4 covers their JSON/rendering changes and existing tests. `list --text` still uses `RenderCompact`, which the plan does not change.
- Reconcile outcome: classification, movement, summary, weft retry and production seam assembly; cards 5–6 cover them. All existing production reconcile callers ultimately use `HubSeamOptions`, including cleanup’s extended options.
- Owner pauses: pause request, coverage, InnerRun lock handling, wake and orch startup; cards 7 and 10–13 cover the affected consumers.
- Resume refusals: batten arming/preflight/window, loom start/resume and InnerRun spawn-envelope decoding; cards 8–11 cover these paths.
- Landing: Finalize, shared reconcile mapping, production dependency assembly and existing outcome tests; card 12 covers them while preserving Merge-In’s distinct disposition.
- Status owners: tender’s report, assembly and status tests; card 14 covers them.
- Window reopening: client attachment query, batten pre-run, command runner and existing help/docs; card 15 covers the changed resume behavior.
- Adopt module: root registration, sandbox exclusion, destruction registration, fixture support and overview/package documentation; card 16 covers these, but omits the required CLI package-naming deviation below.

Finding:

**BLOCKING — consistency — `_lyx/plan/16-adopt-module.md:49`**

**The new engine-less CLI is absent from `PATTERN-cli-cobra`’s package-naming deviations.**

Card 16 explicitly creates `internal/adoptcli` “with no engine of its own,” matching the decision record’s design, but no card updates `pattern/PATTERN-cli-cobra.md`. That background requires `<module>cli` to import `<module>engine` and enumerates exceptions, including cleanup’s equivalent engine-less composition. Adopt is not an exception in that list. Preserve the decided architecture and explicitly register its deviation in the authoritative background.

Evidence: `pattern/PATTERN-cli-cobra.md:27–35`; `_lyx/plan/16-adopt-module.md:49,67–68`; card 17’s targets cover only the orch stencil, PATTERN overview and new owner-machine background.

Tool-call reading log:

1. Located owner decoder helpers, every production reconcile seam call and Open-Window consumers; these downstream consumers were absent from the manifest.
2. Read decoder forwarding, cleanup’s seam extension and Bolt method locations; the inherited excerpts did not show whether these needed separate edits.
3. Checked README golden storage, refusal-kind consumers, Task key reflection and Bolt write/push contracts; these determine affected files and existing coverage.
4. Read board-view consumers, parent-work predicate, network pause installation, window execution, InnerRun lock handling and shared landing verdict mapping; these were downstream paths beyond the listed definitions.
5. Checked hostname readers, compact board rendering, status/pause tests and relevant imports; these identify remaining output consumers and seam coverage.
6. Read compact-list dispatch, tender status assertions, module-registration intent and CLI package-naming rules; these establish the missing documented architectural deviation.
