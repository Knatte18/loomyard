# Batch: sandbox-suite-doc

```yaml
task: "Expand the sandbox suite: subfolder init, weft, warp, config reconcile + coverage invariant"
batch: "sandbox-suite-doc"
number: 1
cards: 3
verify: go test ./tools/sandbox/...
depends-on: []
```

## Batch Scope

This batch delivers every edit to `tools/sandbox/SANDBOX-SUITE.md`: the
forward-only renumber (current S6 → S5), the three new scenarios (S6 subfolder
init, S7 weft lifecycle, S8 warp introspection), the S4 config round-trip
extension, the rewrite of the "Operating model" note to carve out the S6
exception, the session-log-format and Capturing-findings updates for the new
scenario range, and the `**Covers:**` tag lines the coverage test in batch 2
parses. It is one batch because it is one file and one cohesive doc rewrite;
the external interface batch 2 consumes is the set of `**Covers:**` lines
produced by Card 3. `SANDBOX-SUITE.md` is `//go:embed`-ed into
`tools/sandbox/suite.go`, so the batch verify (`go test ./tools/sandbox/...`)
confirms the embed still compiles and `TestRenderScheme_ContainsHeaderAndBody`
(which only asserts the `"SANDBOX-SUITE"` heading survives) still passes.

Batch-local decision: the three cards all edit the same file in sequence
(structural renumber → scenario content → coverage tags); this ordering keeps
each commit's diff legible.

## Cards

### Card 1: Renumber current S6 → S5 and fix the scenario-range references

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `tools/sandbox/SANDBOX-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rename the existing `### S6 -- Wrong-directory and error
  ergonomics` scenario heading to `### S5 -- Wrong-directory and error
  ergonomics`, leaving its Goal/Watch/Verdict body text unchanged. In the
  "Capturing findings" section, change the `ref` documentation line that reads
  "`ref` is the scenario id (`S0`-`S6`)." to span `S0`-`S8`, and update the JSON
  example object's `"ref": "S6"` to a still-valid id (use `"ref": "S5"` so the
  example points at the renamed error-ergonomics scenario). In the "Session log
  format" section, replace the fixed `S0:`/`S1:`/`S2:`/`S3:`/`S4:`/`S6:` line
  list with the contiguous `S0:` through `S8:` (add `S5:`, `S7:`, `S8:`; the gap
  at S5 is now closed). Do NOT touch the "Notes" section's historical mention of
  scenario growth. This card only renumbers and fixes range references — new
  scenario bodies and Covers tags come in Cards 2 and 3.
- **Commit:** `docs(sandbox): renumber S6 to S5 and close the scenario-range gap`

### Card 2: Add scenarios S6/S7/S8, extend S4, and rewrite the Operating-model note

- **Context:**
  - `_mill/discussion.md`
  - `internal/weftcli/cli.go`
  - `internal/warpcli/warp.go`
  - `internal/configcli/configcli.go`
  - `internal/initcli/initcli.go`
- **Edits:**
  - `tools/sandbox/SANDBOX-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add three new scenarios after the renamed S5, in this order,
  each following the existing scenario shape (`### SN -- <title>`, then
  `**Goal:**`, `**Watch:**`, `**Verdict:** OK / WARN / FAIL`, then a `---`
  separator):
  (1) `### S6 -- Subfolder init` — Goal: from a non-root subdirectory of the host
  repo, run `lyx init` there, then run `config` and `board` from that subdir, then
  reverse it with `lyx init --undo`. Watch: does `lyx init` scaffold a subdir-scoped
  `_lyx`; does `lyx config --print`/`--set` from the subdir resolve against the
  subdir's own `_lyx/config` (the subfolder-scoping demonstrator); does `lyx board`
  still run from the subdir (a "still works from any subfolder" smoke check — note
  in the scenario prose that board's data is hub-level, so it does not itself prove
  subdir-scoping); does `lyx init --undo` cleanly reverse the scaffolding. Include a
  durability note (same style as S3): S6 must run `lyx init --undo` from the subdir
  at session end to restore the "not yet initialized" state; note that `init --undo`
  commits and pushes the weft-side deletion to the shared `lyx-test-weft` remote
  (so each run leaves an init-then-undo commit pair) and is a clean no-op on a
  never-initialized directory.
  (2) `### S7 -- Weft lifecycle` — Goal: make a small change inside the weft-tracked
  scope and run it through `weft status`/`commit`/`push`/`pull`/`sync`. Watch: does
  status report the change, does commit/push mirror it, does sync's detached push
  behave. Include the technical notes from discussion.md: the commit message is
  always the fixed string `"weft sync"` (no `-m` flag), staging is scoped to the
  weft config's dirs (default `_lyx`), and `weft sync` pushes via a detached child
  so status may lag. Include a durability note (same style as S3): make a small,
  clearly-marked test change and do not leave the shared weft/host remotes diverged
  or broken.
  (3) `### S8 -- Warp introspection` — Goal: exercise valid `warp list`, `warp pairs`,
  `warp reconcile`, and `warp checkout`. Watch: do list/pairs report sane geometry;
  is `warp reconcile` a safe no-op on a healthy pair (note it has no `--apply`/dry-run
  flag — it acts directly); does `warp checkout` do a coordinated host+weft switch.
  Include a checkout-and-restore durability note (same style as S3): record the
  starting branch, `warp checkout <other-branch>` to prove it works, then
  `warp checkout <original-branch>` to restore. Note that a *bad* `warp checkout`
  now yields a clean wrapped error (not raw git stderr), so a legible error there
  is the OK/expected outcome.
  Then extend the existing `### S4 -- Config round-trip` scenario: update its Watch
  to describe the now-fully-sandbox-native round-trip — write a value with
  `lyx config <module> --set key=value` (non-interactive, bypasses the editor;
  mutually exclusive with `--print`; requires a module arg), read it back with
  `lyx config <module> --print`, then run `lyx config reconcile`. Do not remove
  S4's existing goal; augment it.
  Finally, rewrite the "Operating model" note (the paragraph forbidding nested
  `_lyx/` scaffolding during a session) to name S6 as the explicit, sole exception:
  the no-nested-`_lyx` rule still stands everywhere except the controlled S6
  scenario, which deliberately runs `lyx init` in a subdir and reverses it with
  `lyx init --undo` at session end.
- **Commit:** `docs(sandbox): add S6/S7/S8 scenarios and extend S4 config round-trip`

### Card 3: Add `**Covers:**` tag lines to module scenarios

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `tools/sandbox/SANDBOX-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `**Covers:** <module>` line to each module-driving
  scenario, placed alongside that scenario's existing bold-label lines (e.g.
  immediately after `**Goal:**` or before `**Verdict:**`, matching the surrounding
  style). Exact mapping: S3 → `**Covers:** board`; S4 → `**Covers:** config`;
  S6 → `**Covers:** init`; S7 → `**Covers:** weft`; S8 → `**Covers:** warp`. Do
  NOT add a `**Covers:**` line to S0 (discovery), S1 (hub orientation), or S5
  (error ergonomics) — these drive no single module and must carry no `Covers:`
  line at all (no `(discovery)` sentinel; a literal parenthesized token would trip
  the coverage test's drift-guard assertion). S2 (first real work) also gets no
  `Covers:` line (host git housekeeping is not a lyx module). The resulting covered
  set is exactly `{board, config, init, weft, warp}`.
- **Commit:** `docs(sandbox): tag scenarios with Covers lines for the coverage invariant`

## Batch Tests

`verify: go test ./tools/sandbox/...` — `SANDBOX-SUITE.md` is `//go:embed`-ed
into `tools/sandbox/suite.go`, so this confirms the edited doc still embeds and
compiles, and that `TestRenderScheme_ContainsHeaderAndBody` (asserts the
`"SANDBOX-SUITE"` heading and fingerprint fields survive) and the other
`tools/sandbox` tests still pass. The `.md` content itself is not otherwise
machine-checked in this batch — the coverage invariant that enforces the
`**Covers:**` tags lands in batch 2.
