<!-- This is the Plan-Review rubric. It is read by the Bouncer row and the review row of the Plan-Review perch:
     the Plan-Bouncer row interpolates it as bouncer-template-seed.md's and
     bouncer-template-judge.md's rubric marker value, and the Plan-Burler-Review row interpolates it the
     same way into internal/burlerengine's own review prompt.
     It is a marker VALUE, never a template -- its only markers are the ones ReadRubric fills, and
     internal/stencil's StripLeadingComment removes this leading comment before either consumer ever
     sees it.
     Among those markers, verify_tiers is filled by ReadRubric from the hub's language.yaml list, as bullets or none. -->

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

## Write surface

{{.webster_record}}

`none` means webster has begun no batch, and no card is frozen.
Otherwise a done card is frozen: a fix never edits its file.
A fix to its work is a follow-up card placed after the last begun batch, with its Card Index line in `00-overview.md`, in the format `{{.specs_dir}}/loom/loom-plan-spec.md` defines.
A fix inserts or removes a card only with `lyx loom insert-card` or `lyx loom remove-card`, which renumber the later cards and whose `--help` says what they do, never by renaming, rewriting under a new name or deleting a card file by hand.

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
  A card that plans a new test function or file for behavior an existing test already covers, or one test per symbol, is a finding; so is a card that plans an assertion on a stencil's static prose; `PATTERN-test-economy` holds the rule.
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
- **CLI shape.**
  A card that adds or reshapes a `lyx` CLI command is checked against the audience bullet of `pattern/PATTERN-cli-cobra.md`; a command shape that bullet rules out is a finding.
- **Fidelity to the answer key.**
  Every Decision and every Constraint in `_lyx/discussion/decision-record.md` is carried by some card, and no card introduces scope the answer key does not license.
  A Decision step no card carries is accepted only when the step changes this task's own board entry's body or scope and the decision record carries an `Added after Discussion` entry recording its drop;
  any other uncarried Decision stays a finding.
  That path is anchor-relative: it resolves from this session's own working directory, and it is deliberately not the absolute form the artifact list uses.
  The answer key is the measuring stick and never the subject — every finding is raised against the plan, never against the decision record or a findings file.
  An entry between `<!-- lyx:carry-over … -->` marker lines in the decision record's `## Open risks` is not part of the answer key, and it licenses no card's scope.
  In a rework generation the live generation's `findings.md` joins the decision record as the answer key:
  every finding in it is covered by some card, and every card's scope is licensed by the decision record or by a finding.
  A finding in `findings.md` is a reviewer's claim the rework plan answers, never a ruling;
  it licenses a card's scope unless it overturns a Decision of the decision record, and a card carrying a finding that overturns a Decision with no `Added after Discussion` entry recording it is a finding.
  The live generation's round is, among `_lyx/loom/rework/round-<N>/` directories, the highest `N` whose `record.json` carries a `class` and whose `first_card` equals the `first_card` in `_lyx/plan/00-overview.md`'s frontmatter (absent means `1`).
  With no such round (generation 0, or a fresh Plan-Write plan after an operator `goto Plan-Write`), the decision record alone is the answer key.
  The round's `prior-generation/` archive is context only, and a finding raised against anything inside it is never legitimate.
- **A card that edits the task's own board entry.**
  A card that changes this task's own board entry's body or scope is a finding, because the run's board lock refuses such a change.
  A card writing another entry, the entry's status or its `run_settings`, or changing board-module code is not flagged by this bullet.
- **Card order.**
  A card that depends on something a later card does breaks the rule in `{{.specs_dir}}/loom/loom-plan-spec.md`'s "Plan vs. schedule" section; the finding names the card and the later card it depends on.
- **Verify coverage.**
  A package a card targets whose tests the plan's `## verify:` section does not run is a finding against the plan, its hermetic build-tagged tests included.
  The hub's verify tiers, in the order and form the plan writer was given:
{{.verify_tiers}}
  A tier the list says is compiled rather than run is not a finding, and a section out of the list's order is a finding.
  With `none`, only the coverage rule applies.
  An entry that calls a tier compiled rather than run lets a plan leave that tier unrun, and this review accepts it.
  Only the hub operator writes the list, because its key is Guard-classified in a board-only file.
  Inside webster the card and batch gates still run the untagged and `-tags integration` tests over the batch's packages whatever an entry says,
  and at landing the skip is bounded only by landing config's `publish_verify`;
  the shell guard and the plan gate refuse what they refuse whatever an entry says.
- **The attack-surface question.**
  For every new verb, flag, escape hatch or routing edge, and every guard that is removed, downgraded or bypassable, the card that introduces the change states in its `**Intent:**` what it can now skip or let through and what bounds it.
  A card that introduces one with no stated bound is a finding.
- **Reachability of a prescribed refusal.**
  A card that prescribes a refusal or guard branch with no named input that reaches it, or no named test that reaches it, is a finding.
  So is one whose named input an upstream check rejects; the finding names that upstream check.
  A propagated raw I/O error that joins its section's grouped transient row in `refusal-spec.md` is exempt, and so is a fail-loud wiring guard in that file's sense, which takes no per-row test.
  A test that calls only the refusal's error constructor does not reach the refusal.
- **The writer/reviewer symmetry note.**
  The plan writer's own stencil is `{{.stencils_dir}}/loom/loom-template-plan.md`.
  Whatever it says not to write, this rubric must not flag as missing.

## Proposed fixes

A finding whose fix adds, drops, moves or pairs targets, or changes a verify command, proposes only a fix that every check in the validation-checks section of `{{.specs_dir}}/loom/loom-plan-spec.md` admits.
A fix that adds or pairs targets must pass `redundant-package-target` and `redundant-file-target` there, since a file or a package listed beside a member of it on one card is refused, and the spec says what counts and the way forward.
When the reviewer sees no admissible fix, it still reports the defect and names the check that constrains the fix.
A finding's `fix` key is a proposal: the fixer may apply any other fix the checks admit, and when none exists it defers the finding with the constraining check as the reason.
The rule narrows only which fix shapes a reviewer proposes; no defect becomes unreportable.

Before writing a finding whose `fix` adds, drops, moves or pairs targets, or changes a verify command, the agent writing the review file checks the fixes against the plan gate with `lyx loom validate-plan --overlay`, which checks proposed plan file contents in memory and writes no file.
`lyx loom validate-plan --help` shows the stdin format; this rubric does not restate it.
It first runs `validate-plan --overlay` with `{}` as a baseline, never the flag-less run, so that no run writes a file.
It then runs it once with every such fix applied together, with its Bash timeout raised to cover a type load, and proposes no fix whose run reports a blocking finding the baseline does not.
When the combined run reports a new blocking finding, it re-runs with each fix overlaid alone.
A fix whose solo run reports a new blocking finding is withheld, and its defect is reported with the constraining check named, as above for a defect with no admissible fix.
A new blocking finding of the combined run that no solo run reproduces is a joint conflict: the fixes that finding names by card are withheld, and their defects are reported together the same way.
A fix that inserts, removes or renumbers cards is not overlaid, since it goes through `insert-card` and `remove-card`.
The directive binds writing a finding's `fix`, not a Bouncer row's focus file.

A finding whose proposed glyph stands for a comment or a line it cites names the function or method whose body encloses that line, confirmed with `lyx quarry resolve` that the line lies inside the glyph's span, never the member after or near it.
When no member span holds the line, as for a free-standing comment between declarations, it names the file's self glyph.

## Finding class

Every finding carries one class: `design`, `scope`, `decision` or `consistency`.
Their generic meanings are in burler's review step; this section does not restate them.
In this segment, `design` means the plan's structure is wrong: batching, sequencing, sizing or `verify:` correctness.
A recurring enumeration gap is filed once as a `design` finding about the method, not once per missing item.
Class never changes whether a finding is fixed.
