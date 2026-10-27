Affected sets and plan coverage:

- Machine identity: hostname normalization, production callers and sole-source scan — cards 1, 6, 8, 9, 12–14, 16.
- Task and prime ownership records: Task JSON, store decoding, replay, run locks, payload writes and parent resolution — cards 2–3. `migrate.go` derives its decode type from Task, so it needs no separate field addition.
- Board owner display: brief JSON, get JSON and README Running table — card 4; compact `list --text` rendering is omitted.
- Reconcile owner outcome: classification, movement, summary, push retry and seam construction — cards 5–6; consumers in cards 8–13 and 16.
- Owner pause label: record coverage, tender pause request, InnerRun release, wake, orch and adoption lift — cards 7, 10–11, 13, 16.
- Start/resume refusals: batten arm, pre-run, step, window, claim; loom start/resume and child-spawn envelope — cards 8–11.
- New dependency seams: InnerRunDeps and landingshed.Deps affect the recursive batten wiring guard as well as their own constructors and tests — cards 10–12 do not cover that guard’s nil-exemption inventory.
- Finalize ownership and repair: landing assembly, local-parent checks, halt messages, failed push handling and resume selection — cards 12 and 16.
- Tender owner status: StatusReport, HubDeps, status command and module documentation — card 14.
- Window reopening: reed client reading, batten pre-run and all three existing “resumed past ... opens no window” descriptions — card 15.
- Adopt module: command registration, sandbox exclusion, destruction registries, pair creation, owner setter, pause lifting and resume seams — card 16.
- Orch interpretation and invariant documentation — card 17.

Findings:

1. **MEDIUM — design — `_lyx/plan/04-board-owner-views.md`, Intent and targets.**  
   **The owner-display affected set omits `lyx board list --text`.**  
   D9 requires board list to show the owner, and card 4 enriches `BriefTask`, but the existing text output explicitly selects its columns and will discard the new field. `internal/boardcli/cli.go:694–722` routes `list --text` through `writeListing`; `internal/boardcli/text.go:20–60` (`RenderCompact`) renders kind, priority, dates, slug, title, labels and status only. `internal/boardcli/text_test.go:19` (`TestRenderCompact`) pins this shape. Neither renderer nor its test is carried by any card. Cover the compact output and its existing test along with the list help in card 4.

2. **MEDIUM — design — `_lyx/plan/10-innerrun-owner-check.md` and `_lyx/plan/12-finalize-owner-checks.md`, seam additions.**  
   **The new seam inventory omits batten’s recursive wiring guard.**  
   `internal/battencli/wire_seams_test.go:35–97` declares `intentionallyNil`, and `TestWire_EverySeamFilled` at lines 99–127 rejects every nil seam absent from that inventory. `shedrecipe.Env` embeds landing dependencies (`internal/shedrecipe/recipe.go:188`), although batten intentionally leaves landing seams nil. Card 12 adds four function seams to those dependencies and updates loom’s populated-field test, but carries neither batten’s exemption inventory nor its shared default inventory. Card 10 also introduces nil InnerRun seams until card 11 wires them. This needs the affected nil-seam inventory covered at the introducing cards. **Before retaining, inspect `internal/testkit/envkit.NilSeams`’s default exclusions:** my six-call limit prevented confirming whether its existing exclusion mechanism handles new documented defaults generically.

Tool-call manifest:

1. Read board migration/legacy conversion, Task construction and patching, and searched field/output consumers because the inherited manifest did not include those propagation paths.
2. Read README golden expectations, payload allowlist coverage, additional batten pause wiring, and searched old window wording because those exact assertions and output consumers were absent.
3. Read board brief/full output paths, compact renderer and reconcile/coverage consumers to enumerate alternative readers of the new fields and states.
4. Read compact renderer details, module imports, OpenHub and boardCriticalSection to check output propagation, construction boundaries and write-path effects.
5. Read the actual list/list-full routing, compact-output tests, orch wiring tail and additional guard/test locations to distinguish separate commands and identify omitted consumers.
6. Read batten’s recursive wiring guard, loom’s field-population guard, Env’s landing dependency and tender report definitions to trace the new dependency fields into existing assertions.
