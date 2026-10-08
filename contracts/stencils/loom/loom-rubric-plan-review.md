<!-- This is the Plan-Review rubric. It is read by both rows of the Plan-Review perch:
     the Plan-Bouncer row interpolates it as bouncer-template-seed.md's and
     bouncer-template-judge.md's rubric marker value, and the Plan-Burler row interpolates it the
     same way into internal/burlerengine's own round prompt.
     It is a marker VALUE, never a template -- its only markers are the ones ReadRubric fills, and
     internal/stencil's StripLeadingComment removes this leading comment before either consumer ever
     sees it. -->

# Plan-Review rubric

The subject under review is the current plan: `_lyx/plan/00-overview.md` and the card files its Card Index names.
The plan directory may also hold `archive-*/` subdirectories, which are rotations of superseded plans;
they are out of scope, and a finding raised against one is never legitimate.

The format contract, and the Card model it implements, are both `{{.specs_dir}}/loom/loom-plan-spec.md`.
This rubric points at both and restates neither.
The mechanical checks over that contract are already enforced by this round's own gate over this round's own output — every check but `plan-unapproved` and `delete-target-gone` — while `plan-unapproved` is enforced at no row at all, resting on the review segment's own approve seam failing loudly if ever wired nil, and `delete-target-gone` fires only at Webster's dispatch, after this round.

`Plan-Review` is the LLM producer, not the mechanical one — over-flagging is a judgment failure mode a mechanical producer, which has only checks and never judgment, cannot exhibit.
Sitting behind a mechanical gate over this round's own output makes this gate's over-flagging surface larger than that of a gate with no validator ahead of it, not smaller.

**`support-log.md` is outside this review entirely.**
It appears in neither the artifact list nor the answer key, and it must not be read or reasoned from.
`Plan-Write` provably never reads it, so a finding grounded in its content cannot be satisfied except by inventing the missing link.

## Do not flag

Do not flag any of the following as a finding:

- **Anything this round's own gate already checks.**
  Every check ID `{{.specs_dir}}/loom/loom-plan-spec.md`'s own validation-checks section lists is enforced deterministically by this round's own gate over this round's own output, except `plan-unapproved`, which is enforced at no row at all, resting on the review segment's own approve seam failing loudly if ever wired nil, and `delete-target-gone`.
  `delete-target-gone` fires only at Webster's dispatch, after this round, so no plan this round reviews can carry it.
  Re-deriving any of them here is duplicated work whose only possible outcome is disagreement with the gate.
- **A missing `DependsOn`/`Produces` field, or an incomplete dependency list.**
  Dependency edges are derived, never authored — a card's `Uses` intersected against every other card's target list.
  Plan-time completeness of that intersection is explicitly not provable;
  the real gate is the post-merge build and test.
- **A `Rename`, `Move`, `Prosa`, or `Custom` card carrying no `ImpactSummary`.**
  It is required for `Edit` and `Delete` only, per the per-type table in `{{.specs_dir}}/loom/loom-plan-spec.md`.
  For `Rename` the reason is specific: a correctly executed AST-aware rename is binary, with no graded blast radius to summarise.

## Also flag

- **Granularity.**
  One card per independently reviewable unit, not one card per literal symbol.
  A private supporting type, or a constructor inseparable from its type, belongs in the other symbol's card;
  a symbol that is part of a module's public surface, or reused across packages, gets its own card even when one card is its only consumer, and a helper serving one surface is bundled into that surface's card.
- **Test economy.**
  A card that plans a new test function or file for behavior an existing test already covers, or one test per symbol, is a finding; `PATTERN-test-economy` holds the rule.
  The finding names the existing test that covers the behavior, or, for a per-symbol test, the public-surface test that covers the symbol;
  a vague "seems redundant" is not a finding.
- **Member glyphs on an `Edit` card.**
  An `Edit` card whose targets are only Go file paths of non-test files while its `Intent` names specific symbols is a finding; the finding names the member glyphs to list instead.
- **The re-sign arrow.**
  An `Edit` member glyph without the re-sign arrow, while the card's `Intent` changes that member's parameters, results, type parameters or receiver, is a finding; the finding names the arrow to add.
  It never fires for an interface method or a struct field, which take no arrow.
- **`ImpactSummary` carries a real conclusion.**
  A one-line blast-radius conclusion — "3 callers, all local to the billing package, no cross-module effects" — never a restatement of `Intent`.
- **`Custom` is a last resort.**
  Used only where none of `Create`, `Edit`, `Delete`, `Rename`, `Move`, or `Prosa` genuinely fits, never as a shortcut around correct typing.
  A `Custom` card is exempt from `path-missing` on its own targets and from `prosa-symbol-target` — which under the glyph alphabet means a `Prosa` group may only target file and unit self glyphs, with a member glyph (or anything else that fails to parse as a self glyph) the finding — so a mistyped `Custom` card silently escapes two checks the rest of the plan is held to.
  It escapes only those two: `bare-symbol-target` and `directory-target` bind a card's flat `Targets`/`Uses` with no group scoping at all, so this round's own gate already blocks a `Custom` card carrying either — do not hunt for one here.
  A `Custom` card whose targets could instead be expressed as a multi-label combination of the other six is a finding — the format's one-or-more-labels grammar means `Custom` is never the only way to name a mixed target list.
- **Fidelity to the answer key.**
  Every Decision and every Constraint in `_lyx/discussion/decision-record.md` is carried by some card, and no card introduces scope the answer key does not license.
  That path is anchor-relative: it resolves from this session's own working directory, and it is deliberately not the absolute form the artifact list uses.
  The answer key is the measuring stick and never the subject — every finding is raised against the plan, never against the decision record or a findings file.
  An entry between `<!-- lyx:carry-over … -->` marker lines in the decision record's `## Open risks` is not part of the answer key, and it licenses no card's scope.
  In a rework generation the live generation's `findings.md` joins the decision record as the answer key:
  every finding in it is covered by some card, and every card's scope is licensed by the decision record or by a finding.
  The live generation's round is, among `_lyx/loom/rework/round-<N>/` directories, the highest `N` whose `record.json` carries a `class` and whose `first_card` equals the `first_card` in `_lyx/plan/00-overview.md`'s frontmatter (absent means `1`).
  With no such round (generation 0, or a fresh Plan-Write plan after an operator `goto Plan-Write`), the decision record alone is the answer key.
  The round's `prior-generation/` archive is context only, and a finding raised against anything inside it is never legitimate.
- **Card order.**
  A card that depends on something a later card does breaks the rule in `{{.specs_dir}}/loom/loom-plan-spec.md`'s "Plan vs. schedule" section; the finding names the card and the later card it depends on.
- **Verify coverage.**
  A package a card targets whose tests the plan's `## verify:` section does not run is a finding against the plan, hermetic build-tagged tests (for example `-tags integration`) included.
  The `llm` tag that the section compiles rather than runs (for example `go vet -tags llm <packages>`) is not a finding.
  A section that compiles the `tmux` tier (for example `go vet -tags tmux <packages>`) rather than running it is not a finding either; Publish runs that tier.
  A section out of the order the plan template states is a finding.
  A nested module's packages count as covered by a command run inside that module (`go -C <module>`).
- **The attack-surface question.**
  For every new verb, flag, escape hatch or routing edge, and every guard that is removed, downgraded or bypassable, the card that introduces the change states in its `**Intent:**` what it can now skip or let through and what bounds it.
  A card that introduces one with no stated bound is a finding.
- **The writer/reviewer symmetry note.**
  The plan writer's own stencil is `{{.stencils_dir}}/loom/loom-template-plan.md`.
  Whatever it says not to write, this rubric must not flag as missing.

## Proposed fixes

A finding whose fix adds, drops, moves or pairs targets, or changes a verify command, proposes only a fix that every check in the validation-checks section of `{{.specs_dir}}/loom/loom-plan-spec.md` admits.
When the reviewer sees no admissible fix, it still reports the defect and names the check that constrains the fix.
The `**Fix:**` line is a proposal: the fixer may apply any other fix the checks admit, and when none exists it defers the finding with the constraining check as the reason.
The rule narrows only which fix shapes a reviewer proposes; no defect becomes unreportable.

## Finding class

Every finding carries one class: `design`, `scope`, `decision` or `consistency`.
Their generic meanings are in burler's review step; this section does not restate them.
In this segment, `design` means the plan's structure is wrong: batching, sequencing, sizing or `verify:` correctness.
A recurring enumeration gap is filed once as a `design` finding about the method, not once per missing item.
Class never changes whether a finding is fixed.
