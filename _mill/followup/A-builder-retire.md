```yaml
slug: builder-retire
title: "builder: delete internal/builderengine and internal/buildercli, retire builder-contract.md as a reference"
depends_on: []
brief: |
  Delete internal/builderengine and internal/buildercli outright, sweep every reference to them out of the CLI help tree, configreg, sandbox coverage, and docs, and re-status docs/reference/builder-contract.md as a retired-design reference rather than deleting it, because builder is live-registered today — not dormant — so it costs recurring maintenance until it is removed.
```

# builder: delete internal/builderengine and internal/buildercli, retire builder-contract.md as a reference

## Why

Builder is not dormant: `cmd/lyx/main.go:107` registers `buildercli.Command()`,
and it appears in `cmd/lyx/helptree_test.go`'s module list and `cmd/lyx/notransients_test.go`.
Parking it in-tree therefore costs real, recurring maintenance: it stays in the CLI help tree per the **CLI / Cobra Invariant**, keeps a second plan parser alive against the **Planparser Sole-Parser Invariant**, and every future refactor must carry it.

The genuinely reusable asset is the *design*, not the code — the recovery ladder, chain rollback, the `mutate.lock` state-mutation lease, the three fabric-commit points, and crash/resume semantics —
and that already lives in `builder-contract.md`'s 247 lines, rewritable onto the flat card list later.
The implementation itself stays one `git show` away, permanently.

`manifest/roadmap.md:196` already calls `builder` "superseded as an active plan-format consumer" and `:202` says it "becomes obsolete" — nobody had removed it.

**Rejected alternatives:**

- Keeping the code frozen in-tree — pays help-tree, test, and refactor cost indefinitely for a reference binary.
- Moving it to a `sandbox/` or `attic/` directory — invents an excluded-directory convention this repo does not have.

## What needs to happen

1. **Code deletion.**
   - Delete `internal/builderengine` and `internal/buildercli` entirely.
   - `cmd/lyx/main.go` — unregister `buildercli.Command()` (`:107`) and drop `builder` from the module list in the long help text (`:75`).
   - `internal/configreg/configreg.go` — drop the `{Name: "builder", Template: builderengine.ConfigTemplate}` entry (`:44`) and its import (`:10`); update `internal/configreg/configreg_test.go:17`'s expected module list.
   - `internal/configcli/configcli_test.go` — `:311`, `:327–328` assert the config menu prints `builder (default)`,
     and `:455` notes builder is deliberately unseeded.
     Dropping `builder` from `configreg` fails this test;
     it is a second, less obvious consequence of the same one-line registry edit.
   - `cmd/lyx/helptree_test.go` — lines 28 and 106–107.
   - `cmd/lyx/notransients_test.go` — the import (`:21`) and the two `builderengine.Dir`/`ReportsDir` cases (`:57–58`).
   - `cmd/lyx/constructoranchoring_test.go` — the import and its builder assertions.
   - `cmd/lyx/rawgitmutation_test.go` — the `internal/builderengine` half of `TestNoRawGitMutation_WebsterBuilderProductionSource`.
   - `internal/scoutcli/cli_test.go`, `internal/webstercli/cli_test.go`, `internal/webstercli/sync_integration_test.go` — builder references.
   - `internal/webstercli/cli.go:11–12` — a doc comment comparing websterCLI to buildercli.
   - `tools/sandbox` — `suite.go`'s `//go:embed SANDBOX-BUILDER-SUITE.md` (`:47`), the `builderSuite` spec (`:123–128`), the `"builder-suite"` case in `main.go:326`, the doc comments in `suite.go:2` and `main.go:6,12`, and the `SANDBOX-BUILDER-SUITE.md` file itself.
   - `CONSTRAINTS.md` — review `:97` and `:106`, which list `builderengine` among feature packages.

**The `**Covers:** builder` trap.**
`tools/sandbox/SANDBOX-CORE-SUITE.md:224–232`'s scenario S9 "Builder plan validate/status", including its `**Covers:** builder` tag at `:229`, must go.
`cmd/lyx/sandbox_coverage_test.go`'s drift guard hard-fails on a `**Covers:**` token naming a module that is no longer registered, so leaving S9 in place breaks the build even after every other builder site is gone.

**The dangling-anchor trap.**
The four deep links into `builder-contract.md`'s "Webster: the fork-based sibling" section — `manifest/designs/finalize.md:36`, `finalize.md:50` (Related), `docs/overview.md:268`, and `docs/reference/plan-format-v3.md:343` — must all be re-pointed by this task **before** task B runs.
B's zero-hit grep for `plan-format-v3` cannot catch a dangling `builder-contract.md#…` anchor, so nothing downstream would find it.

**The inert-builder.yaml trap.**
Removing `builder` from `configreg`'s module list means `lyx config reconcile` stops emitting `builder.yaml`.
Existing `builder.yaml` files in already-created worktrees are left in place — they are inert once no module reads them,
and reconcile does not delete files it no longer owns.
This task states this so nobody files it as a leak.

2. **Doc retirement.**
   - Extract `builder-contract.md`'s `## Webster: the fork-based sibling` section (`:222`) into `internal/websterengine`'s package doc.
   - Re-point all four deep links into that section, per the dangling-anchor trap above.
   - Re-status `builder-contract.md` as a retired-design reference.
   - Delete `docs/reference/plan-format.md` (v2).
   - `discussion-format.md:30` — its justification for `plan-format`'s `approved:` field reads "because `lyx builder run` can be invoked standalone, outside loom", which is false once this task lands;
     `discussion-format.md:3` links `plan-format.md`.
     Both belong to this task, since this task is what falsifies them.
   - `docs/overview.md` — all builder and plan-format references, not the module table alone: `:92` (lists both `plan-format.md` and `plan-format-v3.md` as kept reference docs — only one survives), `:227` (the `internal/pattern` tree comment naming builder as a consumer), `:264` (the `builder` module-table entry), `:265` (the webster entry, defined as "fork-based sibling of builder"), `:268` (the deep link, above), `:292` (names "builder implementer" among `internal/pattern`'s prompt consumers — the same phrase this task also owns at `roadmap.md:42`), and `:375` (the `builder-contract.md` see-also).
   - `README.md` — `:25` lists `lyx builder` in the subcommand tree, `:86` is the `builder` module bullet, `:87` defines webster as "a fork-based sibling of `builder`", `:94` asserts builder "stays frozen in-tree as the plan-format-v2 consumer" (directly falsified by this task),
     and `:115` describes builder's place in the module topology.
   - `docs/sandbox-howto.md` — `:8`'s launcher list, `:141–147`'s "Run the builder suite" section, and `:190`'s `SANDBOX-BUILDER-SUITE.md` see-also.
   - `sandbox/builder-suite.cmd` — delete it.
     It invokes the `"builder-suite"` case this task removes from `tools/sandbox/main.go:326`, so it is an orphan by construction, not an independent decision.
   - `.gitattributes:7–9` — the three `internal/builderengine/*` `text eol=lf` pins (`implementer-template.md`, `orchestrator-template.md`, `template.yaml`), all pointing at files this task deletes.
   - **Comment-only residue, swept opportunistically** (none of it breaks the build, all of it reads as stale once builder is gone): `internal/perchengine/doc.go:13` ("builder-review"), `internal/modelspec/modelspec.go:7,35` ("builder's roles", "builder, perch/burler/loom configs"), `internal/loomengine/configtemplate.go:4` ("mirroring builderengine's ... embed-and-accessor").
   - `manifest/roadmap.md` — the Done `builder` item (`:196`, `:202`) and `:42`'s "builder implementer" template mention.
   - `docs/reference/status-schema.md` — its builder-specific prose (`:16`, `:53`, `:69`, `:73`, `:81`) and the `builder-contract.md` link at `:3`.
     **The `phase` enum itself is deliberately NOT this task's** — see the standalone note below on the deferred phase enum.

3. **Comment-only residue and v2-coexistence prose class.**
   Sweep the comment-only residue listed above opportunistically.

**The v2-coexistence prose class.**
Once this task deletes v2 and task B reuses the filename, every surviving "plan-format v2" link silently re-targets v3 content — a worse failure than a dangling link, because nothing breaks.
This task owns every site whose claim it itself falsifies: `docs/reference/builder-contract.md:3`, `:7`, `:224` ("until then it stays frozen and fully functional in-tree"), `:243`; `docs/reference/model-spec.md:3` ("Pinned alongside [plan-format v2] and the emerging [v3]"); `docs/reference/status-schema.md:3`; `manifest/roadmap.md:207` ("Coexists with the still-live plan-format v2 — still used by the frozen `builder`"); and `manifest/designs/review-finding-classification.md:7`, `:47` — where task B's sweep would otherwise turn a v2/v3 pair into "plan-format.md / plan-format.md".

## What this task does not own

The `phase` enum in `internal/loomengine/coherence.go:14–22`'s `validPhases` map and its twin in `docs/reference/status-schema.md` — both currently `preflight | discussion | plan | builder | raddle | finalize | done` — are deliberately left alone by this task and the rest of the follow-up set.
Realigning them lands with the `Shed` build task.
The flat producer list replaces the phase enum rather than editing it;
rewriting the enum now would mean inventing an interim phase set that `Shed` would immediately discard — churn on a pinned contract and live validation code, to no end.
The enum is not wrong today: it describes the machine that exists.

**What is not deferred:** `status-schema.md`'s builder-specific prose and its `builder-contract.md` link go stale the moment this task lands, so those are this task's, per the "Doc retirement" list above.
Only the enum itself waits.

## Scope

This task is one task producing one compiling commit, because a package deletion is atomic by nature and splitting it guarantees an intermediate state that does not build.

This task's ownership rule for the v2-coexistence prose class is: it owns every site whose claim it itself falsifies.
Two exclusions from that rule: `plan-format-v3.md:5`'s own "Coexistence, not replacement" section belongs to task C,
and `loom.md:29` belongs to task E.

`manifest/roadmap.md` has two owners, this task then task E, in chain order rather than concurrently.

## Sequencing

`depends_on:` nothing — nothing blocks this task.

Two tasks depend on this task:

- Task B, because the `plan-format.md` filename is not free until this task deletes v2's doc.
- Task D, because `finalize.md:36` and `:50`'s link targets move in this task.

## Acceptance

The existing suite is the test.
`go build ./...` and `go test ./...` must pass with `builderengine` and `buildercli` gone.
No new tests are written.

Four guards fail loudly on a half-removal,
and this task should expect to be driven by them:

- `cmd/lyx/helptree_test.go`
- `cmd/lyx/notransients_test.go`
- `internal/configreg/configreg_test.go:17`'s module-list assertion
- `cmd/lyx/sandbox_coverage_test.go`'s `TestSandboxCoverage_AllModulesCoveredOrExcluded`

This task must satisfy four `CONSTRAINTS.md` invariants:

- **CLI / Cobra Invariant** — remove `builder` from the help tree cleanly, not orphan it.
- **Planparser Sole-Parser Invariant** — this task's deletion of `builderengine` is what finally makes this literally true.
- **Sandbox Suite Coverage** — this task trips it by removing a registered module, and must delete both `SANDBOX-BUILDER-SUITE.md` and `SANDBOX-CORE-SUITE.md`'s S9 `**Covers:** builder` tag.
- **Fabric Git Invariant (warp + weft)** — its Enforced-by block at `CONSTRAINTS.md:205` machine-checks module ownership for `internal/websterengine`/`internal/builderengine` via `cmd/lyx/rawgitmutation_test.go`'s `TestNoRawGitMutation_WebsterBuilderProductionSource`; narrow that clause to webster alone.
  `:205` sits inside this invariant, which begins at `:173` — not inside the Review Round Invariant, which begins at `:209`.

This task must also satisfy the **Documentation Lifecycle**, which governs the extraction of the Webster section into `internal/websterengine`'s package doc before `builder-contract.md` is retired.
