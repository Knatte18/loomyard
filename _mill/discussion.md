# Discussion: Custom-typed plan cards skip path-missing checks

```yaml
task: Custom-typed plan cards skip path-missing checks
slug: plan-custom-card-skips-path-check
status: discussing
parent: main
```

## Problem

Three shipped rules of the format-4 plan format collide, and the collision makes `path-missing` silently stop checking real targets.
The plan stencil requires a card to bundle its own test with its implementation (`contracts/stencils/loom/loom-template-plan.md:57`);
the stencil also requires **exactly one** bold type label per card (`loom-template-plan.md`, "Each `NN-<card-slug>.md`" section, enforced by `card-type-missing` at `internal/planparser/validate.go:185`);
and `checkPathMissing` skips a `Create` card's and a `Custom` card's own targets entirely (`internal/planparser/validate.go:596`).
A card that edits an existing file AND creates its own new test file is therefore an `Edit` and a `Create` at the same time, and the only label that fits both is `Custom` — which is exactly the label whose targets escape existence checking.

Why now: this was observed unprompted in the very first card of the very first real plan `Plan-Write` has ever produced end to end (crucible round 2, wiki task #120, finding F11 in `_mill/loom-review-opus-high-r1.md`, branch `loom-crucible-hardening-round2`, archive tag `archive/loom-crucible-hardening-round2`).
That card named one real edit target and one real create target, neither existence-checked.
Had the edit target been mistyped, `path-missing` would have stayed silent and the plan would have reached Webster carrying an unresolvable target.
Severity is LOW because the downstream failure is loud — a Webster fork handed a nonexistent target fails its batch visibly — but the check that exists to catch it before Webster is being routed around by the format's own rules, and the routing-around is the normal case rather than the exception.

## Scope

**In:**

- `internal/planparser`: a card may carry **one or more** type labels, each label owning its own target sub-bullets. The parsed model gains a per-label target grouping (`TargetGroups`) alongside the existing flat `Targets` union.
- Every type-conditional validation check in `internal/planparser/validate.go` becomes **group-scoped** rather than card-scoped: `path-missing`, `prosa-symbol-target`, `createTargetsUnion`, `card-field-empty`'s type-label branch, the `ImpactSummary` requirement, and `rename-mechanic-missing`.
- `card-type-missing` relaxes from "exactly one label" to "at least one label".
- One new validation check ID, `card-custom-not-alone`: `Custom` must be a card's sole type label. Total distinct check IDs goes 16 -> 17.
- `contracts/specs/loom-plan-spec.md`: the card-fields grammar, the card-types table, the Validation-checks list (row count, IDs, ordering, entry-point split), and the worked example.
- `contracts/stencils/loom/loom-template-plan.md`: the "exactly one bold type label" rule and new guidance that an implementation card bundling its own new test file is `Edit` + `Create`.
- `contracts/stencils/loom/loom-rubric-plan-review.md` and `manifest/designs/loom.md`: the `Custom` is a last resort bullet gains "a `Custom` card expressible as a multi-label combination is a finding".
- `manifest/designs/plan-card-format.md`: the closed-open-question bullet about `Custom`'s exemptions, restated for the multi-label world.
- `internal/planparser/testdata/goodplan/02-json-flag.md`: gains a `**Create:**` group so the golden plan exercises multi-label.

**Out:**

- `internal/websterengine`, `internal/batcher`, `internal/webstercli`: no change. They consume the flat `Targets`/`Uses` ref lists only (`internal/websterengine/sequence.go:174-177`) and never branch on `CardType`, so retaining the flat union keeps them untouched.
- `Custom`'s exemption from `path-missing` on its own targets and from `prosa-symbol-target` is **kept**, not removed. Multi-label makes `Custom` rare; it does not make it checkable.
- The `Move`-destination false positive documented at `contracts/specs/loom-plan-spec.md:165-166` stays as-is. No third union is added.
- The shape classifier, `root:`/`//` normalization, card numbering, and commit-subject rules are untouched.
- No change to the bundle-your-own-test rule itself — it is a real quality rule and is not what gives.
- No new card type (`EditCreate` or similar) and no per-target inline annotation syntax.
- `Plan-Sweep`, quarry integration, and the three-tier verify model are untouched.

## Decisions

### multi-label-cards

- Decision: a card carries **one or more** bold type labels from the existing seven (`**Create:**`, `**Edit:**`, `**Delete:**`, `**Rename:**`, `**Move:**`, `**Prosa:**`, `**Custom:**`), each with its own indented backtick-wrapped target sub-bullets. A card that edits `foo.go` and creates `foo_new_test.go` writes an `**Edit:**` group and a `**Create:**` group, and each group is checked under its own type's rules.
- Rationale: the collision's root cause is that "one card = one file-operation type" is false once the card-granularity rule forces implementation and its new test into a single card. Multi-label states the truth in vocabulary the format already has, dissolving the collision instead of papering over it. It also shrinks `Custom` back to what it was designed to be — a last resort — rather than the label any edit-plus-create card is forced into.
- Rejected: a new composite type (`EditCreate`) — solves exactly one pair and does not generalize to Edit+Delete or Edit+Prosa, and adds an eighth type whose meaning is a conjunction of two others. A per-target inline marker (`` `path` (new) ``) — new grammar surface that duplicates what a `Create` label already expresses, and it would have to be parsed, validated, and documented as its own sub-format. Removing `Custom`'s exemption without multi-label — every legitimate edit+create card would then emit a false-positive `path-missing` on its new file. Doc-only guidance ("prefer splitting into two cards") — fights the bundle-your-own-test rule, which deliberately wants implementation and test in one commit.

### legal-label-combinations

- Decision: any set of the seven labels is legal on one card, with one restriction — `**Custom:**` must be a card's **sole** type label. A label appearing more than once on a card is legal and its groups simply merge; no check for it.
- Rationale: `Custom` means "none of the other six genuinely fits". A card that can name a typed group has, by definition, found a fit, so `Custom` alongside `Edit` is self-contradictory — and it would also re-open the exact hole this task closes, since the `Custom` group's targets would be exempt again. Beyond that restriction, a whitelist would be guessing at which real combinations plans need; permissive-with-one-rule is the smaller contract. A duplicated label is harmless because two `Edit` groups validate identically to one merged `Edit` group, so a check for it would be pure ceremony (YAGNI).
- Rejected: a hard whitelist of `Edit`+`Create` only — arbitrary, and blocks legitimate `Edit`+`Prosa` (code change plus its doc update) or `Delete`+`Prosa`. Fully unrestricted including `Custom` mixing — reintroduces the exemption hole.

### model-shape-additive

- Decision: `planparser.Card` gains `TargetGroups []TargetGroup`, where `TargetGroup` is `{Type CardType; Refs []string}`, one entry per type label the card body carried, in body order. The existing flat `Targets []string` field is **retained** as the union across all groups (with a `Rename` group's `Pairs` endpoints still projected into it, `Old` then `New`, exactly as today). `Card.Type` keeps its current documented meaning — the **first** recognized type label the card carried — and `TypeLabelCount` stays.
- Rationale: `internal/websterengine/sequence.go:174-177` derives its whole dependency graph from flat `Targets`/`Uses` ref intersection and never reads `CardType`; `checkCardFieldOverlap` (`validate.go:388`), `checkCardPathMalformed` (`validate.go:253`), and `normalizeCard` (`normalize.go:51`) likewise operate on the flat list. Keeping `Targets` as the union means the entire downstream consumer surface — Webster, batcher, webstercli, and four card-generic checks — needs zero change, and the diff is confined to the six type-conditional checks plus the parser. `parseTypeLabelCase` (`internal/planparser/parse.go`) already accumulates into `Targets` and already increments `TypeLabelCount` on every label, so it appends one `TargetGroup` and is otherwise unchanged.
- Rejected: replacing `Targets`/`Type` with groups only — forces every consumer and every card-generic check to re-flatten, for no gain, and breaks the Webster seam this task has no business touching. Adding `Types []CardType` as a separate field — derivable from `TargetGroups`, so it is redundant state that can drift.

### group-scoped-checks

- Decision: all six type-conditional checks in `internal/planparser/validate.go` iterate `TargetGroups` and apply their existing per-type rule to each group's own `Refs`:
  - `checkPathMissing` (`:552`) — per group: an `Edit`/`Delete`/`Move`/`Prosa` group's path-shaped refs are checked; a `Rename` group's `Pairs.Old` entries are checked and its refs skipped; a `Create` or `Custom` group's refs are skipped. `Uses` stays card-level and is always checked, unchanged.
  - `createTargetsUnion` (`:517`) — collects path-shaped refs from every `Create` **group**, not every `Create` card.
  - `checkProsaSymbolTarget` (`:441`) — flags symbol-shaped refs in a `Prosa` **group** only. A symbol in the same card's `Edit` group is legitimate and is not flagged.
  - `checkCardFieldEmpty` (`:346`) — one finding per type-label group carrying zero refs, naming the label, rather than one finding per card.
  - `ImpactSummary` requirement (`checkCardMissingField`, `:327`) — required when **any** group is `Edit` or `Delete`.
  - `rename-mechanic-missing` (`:298`) — fires when any card carries a `Rename` **group**.
- Rationale: these six are exactly the sites reading `Card.Type` today (`validate.go:298`, `:327`, `:444`, `:520`, `:582`, `:589`, `:596`). Leaving any of them card-scoped while the others go group-scoped would produce checks that disagree about what a card is — e.g. a card whose first label is `Create` would escape `prosa-symbol-target` on its `Prosa` group. Converting all six in one pass is the only self-consistent end state.
- Rejected: converting `path-missing` alone — the minimal fix for the reported symptom, but it leaves five checks silently keyed on "first label wins", which is a second, subtler version of the same bug.

### card-type-missing-relaxed-plus-new-check

- Decision: `card-type-missing` keeps its zero-label finding and **drops** its `TypeLabelCount > 1` branch (`validate.go:185-190`). One new check ID, `card-custom-not-alone`, flags a card carrying a `Custom` group alongside any other type label. Distinct check IDs go 16 -> 17; the format-only set `ValidateFormat` runs goes 15 -> 16, and `plan-unapproved` remains the one ID belonging only to the wider `Validate` entry point.
- Rationale: the >1 branch is the literal enforcement of the rule this task removes, so it has to go. The `Custom`-must-be-alone rule is a genuinely distinct defect from "no label at all" and deserves its own ID — `contracts/specs/loom-plan-spec.md` explicitly cleaned up a former 14-row list that bundled two IDs into one row, so bundling a new one now would walk that back.
- Rejected: folding the new rule into `card-type-missing`'s detail text — re-creates the bundled-ID shape the spec's own history rejects. Adding a second new ID for duplicate labels — nothing goes wrong when a label repeats (see `legal-label-combinations`).

### new-check-ordering

- Decision: `card-custom-not-alone` is inserted **after** `card-type-missing` in the fixed check order, becoming row 5; `card-retired-label` through `commit-subject-mismatch` each shift down one, ending at row 17. The corresponding `findings = append(...)` call in `Validate`/`ValidateFormat` (`validate.go:93` region) is inserted at the matching position.
- Rationale: the documented order groups by concern, and the new check is a type-label check — it belongs beside the other one. The order is a doc-and-code contract with no external consumer keyed to row numbers, so renumbering costs nothing beyond the edit itself.
- Rejected: appending as row 17 to minimize doc churn — cheaper diff, but it scatters the two type-label checks to opposite ends of a list whose ordering is otherwise meaningful.

### custom-exemption-retained

- Decision: a `Custom` group stays exempt from `path-missing` on its own targets and from `prosa-symbol-target`, and stays bound by every card-generic check. The rubric text in `contracts/stencils/loom/loom-rubric-plan-review.md:51` and `manifest/designs/loom.md:181` — "a mistyped one silently escapes two checks the rest of the plan is held to" — stays true and stays in place, and gains a sentence making the multi-label alternative explicit.
- Rationale: `manifest/designs/plan-card-format.md:86` closed this question in the affirmative — `Custom` is a principled escape hatch, checked by nothing type-specific by design. The reported defect is not that the exemption exists; it is that the format forced ordinary cards into it. Multi-label removes the forcing. Removing the exemption instead would leave genuinely-custom cards with no way to name a new file without a false positive.
- Rejected: removing the exemption and widening `createTargetsUnion` to include `Custom` targets — self-satisfying, so it would report nothing while looking like a check. Removing the exemption outright — false positives on every legitimate `Custom` card that creates something.

### rename-in-multi-label-cards

- Decision: a `Rename` group may appear alongside other labels. `Card.Pairs` and `Card.RenameRaw` stay flat card-level fields fed by the card's `Rename` group(s), unchanged in shape. Both endpoints of every pair stay projected into the flat `Targets` union in pair order.
- Rationale: `Pairs` is already a flat card-level accumulator in `parseTypeLabelCase`, and only a `Rename` group can contribute to it, so no ambiguity arises from multi-label. Forbidding `Rename` from combining would be a special case with no motivating defect.
- Rejected: `Rename` must be a sole label — arbitrary; a rename that also needs its own new test file hits the identical collision this task exists to fix.

### documentation-and-fixture-updates

- Decision:
  - `contracts/specs/loom-plan-spec.md`: card-fields grammar says one-or-more labels; the card-types table gains a note that a card may declare several rows' worth of groups; the Validation-checks section is rewritten for 17 IDs with the new row 5 and the new counts; the worked example's Card 2 gains a `**Create:**` group.
  - `contracts/stencils/loom/loom-template-plan.md`: "exactly one bold type label" becomes "one or more bold type labels, each with its own target sub-bullets", plus an explicit line that an implementation card bundling its own new test file writes `**Edit:**` for the implementation and `**Create:**` for the new test file, and a tightened `Custom` line.
  - `contracts/stencils/loom/loom-rubric-plan-review.md` and `manifest/designs/loom.md`: the `Custom` is a last resort bullet gains "a `Custom` card whose targets could be expressed as a multi-label combination is a finding".
  - `manifest/designs/plan-card-format.md:86`: the closed-open-question bullet is restated so the exemption is described in multi-label terms.
  - `internal/planparser/testdata/goodplan/02-json-flag.md`: gains a `**Create:**` group naming a new test file, so the golden plan round-trips a multi-label card. `03-json-emission.md` stays `Custom` so that label is still exercised.
- Rationale: this is a plan-format contract change, so the spec, the stencil that generates plans, the rubric that reviews them, and the design doc all move together or they drift. Adding the `Create` group to the existing golden Edit card is additive, keeps all seven labels exercised across the seven-card golden plan, needs no renumbering, and encodes the exact motivating scenario as the reference example. `CLAUDE.md`'s Task-completion rule requires the doc updates in the same commit set; `manifest/roadmap.md` is **not** touched — this is a format correction, not a roadmap item.
- Rejected: an eighth golden card — renumbers nothing but adds a card whose only purpose is one feature, when an existing card is already the natural host. Leaving the golden plan untouched and covering multi-label in inline fixtures only — the golden plan is the format's executable reference; a format feature it does not exercise is a feature the reference denies exists.

## Technical context

- `internal/planparser` is the SOLE parser and SOLE writer of the on-disk plan format (Planparser Sole-Parser Invariant, `CONSTRAINTS.md:519`). Every change to what a card may contain lands here and nowhere else. The package never resolves cwd and never imports `internal/lyxcwd`; the caller supplies the anchor path.
- `internal/planparser/plan.go` holds `CardType` (seven constants plus `CardTypeUnknown`) and the `Card` struct (`Type`, `TypeLabelCount`, `HasType`, `Targets`, `Pairs`, `RenameRaw`, `Uses`, `HasUses`, `Intent`, `HasIntent`, `ImpactSummary`, `HasImpactSummary`, `ImpactSummaryTrailing`, `RetiredLabels`, `Commit`, `Verify`, `HasVerify`). `TargetGroup` and `Card.TargetGroups` are added here.
- `internal/planparser/parse.go`: `parseCardBody`'s switch dispatches each of the seven type labels to `parseTypeLabelCase`, which already increments `TypeLabelCount`, sets `Type` only when still `CardTypeUnknown`, and **appends** to `Targets` — so it already tolerates multiple labels structurally. The change is one `TargetGroups` append per call. `typeLabels` (`parse.go:334-340`) maps label literal to `CardType`. The switch's case order carries no semantics (documented at `parse.go:~402`); do not reorder it.
- `internal/planparser/normalize.go:44-58`: `normalizeCard` rewrites `card.Targets`, `card.Uses`, and both endpoints of every `card.Pairs` entry. With `TargetGroups` sharing the same backing strings by value rather than by slice aliasing, group refs must be normalized too — either normalize groups and rebuild `Targets` from them, or normalize both. Whichever is chosen, `Targets` and the union of `TargetGroups[*].Refs` must stay identical after normalization; a test should pin that.
- `internal/planparser/validate.go`: `Validate`/`ValidateFormat` append findings in a fixed check order starting near `:93`. The type-conditional sites are `:298` (rename mechanic), `:327` (ImpactSummary required), `:444` (prosa), `:520` (create union), `:582`/`:589`/`:596` (path-missing switch). The card-generic sites that must NOT change are `:253` (`card-path-malformed`), `:388` (`card-field-overlap`), and the `Uses` loop inside `checkPathMissing`.
- `checkPathMissing`'s `satisfied` predicate is `pathExistsOnDisk(worktreeRoot, p) || creates[p] || renames[p]`. Its doc comment (`validate.go:545-551`) states the per-type rules explicitly and must be rewritten in group terms.
- `internal/websterengine/sequence.go` `SequenceBatches` derives edges purely from `refsIntersect` over `Targets`/`Uses` — a `Uses` entry matching another card's `Targets` entry orders producer before consumer, and two cards sharing a `Targets` entry settle by card number. It never reads `CardType`. This is why the flat `Targets` union must be preserved.
- The `changes-files`/deviation union (`contracts/specs/loom-plan-spec.md`, Deferred/forward-compat section) is defined as every path-shaped target entry across a batch's cards plus the files holding every symbol-shaped target entry, with `Uses` excluded. Defined over the flat target set, so multi-label does not change it — worth one sentence in the spec confirming that, no code change.
- Golden fixture lives at `internal/planparser/testdata/goodplan/` (`00-overview.md` plus cards 01-07). Card 2 is the `Edit` card, card 3 is the `Custom` card. `internal/loomrecipe/fixture_test.go:244` and `internal/loomshed/planvalidate_test.go:17` and `internal/websterengine/runlevel_test.go:173` each build their own inline plan fixtures that rely on `Create` cards' targets escaping `path-missing`; those comments and fixtures stay valid under group scoping but should be re-read for wording drift.
- Other files carrying `format: 4` plan text that a format change could stale: `internal/loomcli/validate_test.go`, `internal/loomshed/gatefindings_test.go`, `internal/planparser/approve_test.go`, `internal/planparser/sections_test.go`, `internal/webstercli/cli_test.go`, `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`. None should break (multi-label is additive; single-label cards stay valid), but the sandbox suite doc is worth a read for whether it restates the one-label rule.
- `manifest/designs/loom.md:181` and `contracts/stencils/loom/loom-rubric-plan-review.md:51` carry the same `Custom` rubric bullet nearly verbatim; both must move together or they drift.

## Constraints

- **Planparser Sole-Parser Invariant** (`CONSTRAINTS.md:519`) — `internal/planparser` is the sole parser and sole writer of `_lyx/plan/`. No other package may learn to parse type labels. `SetApproved` stays the one write path. The package stays free of `internal/lyxcwd`.
- **Told-Geometry Invariant** (`CONSTRAINTS.md`) — `internal/planparser` is listed under *Review obligation*: it takes absolute paths from its caller and must keep having no direct production import of `internal/lyxcwd`. `checkPathMissing` receives `worktreeRoot` as a told value; that stays.
- **Test Tier Purity Invariant** — no process spawn from tier1. This is why the shape classifier resolves by shape and `path-missing` resolves existence at validation time rather than classification time (`contracts/specs/loom-plan-spec.md:118-123`). The multi-label change must not introduce any lookup that needs a subprocess.
- **Documentation Lifecycle / task-completion rule** (`CLAUDE.md`) — a change to observable behavior or cross-cutting infrastructure updates its module doc, `docs/overview.md` if the module table or execution stack changes, and `CONSTRAINTS.md` for any new cross-cutting invariant, in the same commit. Here: `manifest/designs/plan-card-format.md`, `manifest/designs/loom.md`, `contracts/specs/loom-plan-spec.md`, and the two stencils. No new cross-cutting invariant is introduced, so `CONSTRAINTS.md` needs no new section. `manifest/roadmap.md` is NOT touched — this is a format correction, not a planned-item completion.
- **Markdown: semantic line breaks** (`CLAUDE.md`) — one sentence per line, plus breaks at internal independent-clause boundaries; never a fixed-column hard wrap; plain newlines only, never trailing double-spaces or backslashes. Applies to every `.md` file edited by this task, including the golden fixture cards and the spec.
- **Worktree isolation** — all work stays in `wts/plan-custom-card-skips-path-check`; no push to `main`.
- Go conventions per `golang:golang-build`, `golang:golang-comments`, `golang:golang-testing`: godoc on every exported identifier (`TargetGroup` and its fields need doc comments), table-driven tests, `go build ./... && go test ./...` as the gate.

## Testing

TDD candidates, in order:

1. **`checkPathMissing` group iteration** — the primary TDD target and the defect's own regression test. Write the failing case first: a single card carrying `**Edit:**` naming a nonexistent path plus `**Create:**` naming a new path, against a hermetic `worktreeRoot`. Expect exactly one `path-missing` finding, on the Edit group's path only. Today's code produces zero findings for the `Custom` equivalent, which is the bug.
2. **`card-custom-not-alone`** — a card with `**Custom:**` plus `**Edit:**` yields exactly one `card-custom-not-alone` finding; a `Custom`-only card yields none; a multi-label card with no `Custom` group yields none.
3. **`card-type-missing` relaxation** — a two-label card yields zero `card-type-missing` findings (today it yields one); a zero-label card still yields exactly one.

Scenarios the suite must cover, mostly extending the existing table-driven tests in `internal/planparser/parse_test.go` and `internal/planparser/validate_test.go`:

- **Parse** — a card carrying two labels populates two `TargetGroups` in body order with the right `Type` and `Refs` each; `Targets` equals the concatenation of both groups' refs in body order; `TypeLabelCount` is 2; `Type` is the first label. A repeated label produces two groups that validate identically to one merged group. A single-label card produces exactly one group and is otherwise byte-identical in behaviour to today (regression guard for the whole existing corpus).
- **Rename in a multi-label card** — `Pairs` is populated from the `Rename` group only, both endpoints project into `Targets` in `Old`-then-`New` order, and `path-missing` checks `Pairs.Old` while the `Rename` group's own refs stay skipped.
- **Normalization parity** — after `normalizeCard` with a plan-level `root:` set and a `//`-escaped entry in one group, `Targets` and the union of `TargetGroups[*].Refs` are identical, and symbol-shaped entries in every group pass through verbatim.
- **`createTargetsUnion` per group** — a `Create` group on an otherwise-`Edit` card satisfies a later card's `Edit` target naming the same path (cross-card sequencing, which is legitimate and must not be flagged).
- **`prosa-symbol-target` per group** — a symbol in a `Prosa` group is flagged; the same card's symbol in an `Edit` group is not.
- **`card-field-empty` per group** — a card with a populated `Edit` group and an empty `Create` group yields exactly one `card-field-empty` finding, and its detail names the empty label.
- **`ImpactSummary` requirement** — a `Create`+`Edit` card with no `ImpactSummary` yields a `card-missing-field` finding; a `Create`+`Prosa` card with none yields no finding.
- **`rename-mechanic-missing`** — a plan whose only `Rename` group sits on a multi-label card, with no `## Rename mechanic` section, yields the finding.
- **`card-field-overlap` unchanged** — an entry in a card's `Create` group and its own `Uses` is still one overlap finding (the check reads the flat union, so this is a guard against the union regressing).
- **Check-ID inventory** — whatever existing test pins the check-ID set or count is updated to 17, with `card-custom-not-alone` in row-5 position, and the `ValidateFormat` format-only subset at 16.
- **Golden round-trip** — the updated `testdata/goodplan/` parses clean with zero findings, and card 2's two groups are asserted explicitly.

Verification gate: `go build ./... && go test ./...` from the worktree root.
The `--json`-flag worked example in `contracts/specs/loom-plan-spec.md` is byte-consistent with the golden fixture, so any edit to one must be mirrored in the other and the golden test is what proves it.

## Q&A log

- **Q:** Which fix shape closes the collision — multi-label cards, a new composite type, per-target annotation, or docs only? **A:** [auto-pick] Multi-label cards. **Why:** the root cause is that one-card-one-type is false under the bundle-your-own-test rule; multi-label states that in vocabulary the format already has, and the alternatives either solve one pair only, add parallel grammar, or leave the hole open.
- **Q:** Which label combinations are legal? **A:** [auto-pick] Any set of distinct labels, with `Custom` required to be the sole label; a repeated label merges and needs no check. **Why:** a card that can name a typed group has by definition found a fit, so `Custom` alongside a real label is self-contradictory and would re-open the exemption hole; anything more restrictive is guessing.
- **Q:** Additive model (`TargetGroups` beside a retained flat `Targets`) or groups-only? **A:** [auto-pick] Additive. **Why:** Webster's `SequenceBatches` and four card-generic checks read the flat union and never read `CardType`, so retaining it confines the diff to the parser plus six checks.
- **Q:** Keep `Custom`'s `path-missing` exemption? **A:** [auto-pick] Keep. **Why:** `plan-card-format.md:86` closed this in the affirmative; the defect is the forcing into `Custom`, not the exemption, and removing it would false-positive every legitimate `Custom` card that creates something.
- **Q:** How many new check IDs? **A:** [auto-pick] One, `card-custom-not-alone`, taking the total 16 -> 17. **Why:** the spec's own history rejects bundling two defects under one ID, and a duplicate-label check would guard against nothing.
- **Q:** Convert all six type-conditional checks to group scope, or `path-missing` alone? **A:** [auto-pick] All six. **Why:** leaving five keyed on first-label-wins is the same bug in subtler form and produces checks that disagree about what a card is.
- **Q:** When is `ImpactSummary` required on a multi-label card? **A:** [auto-pick] When any group is `Edit` or `Delete`. **Why:** the requirement exists because those operations have a blast radius over existing callers; that is true whether or not the card also creates something.
- **Q:** How should the golden fixture demonstrate multi-label? **A:** [auto-pick] Add a `**Create:**` group to the existing Edit card, `02-json-flag.md`. **Why:** additive, no renumbering, keeps all seven labels exercised, and encodes the exact motivating scenario as the reference example.
- **Q:** How much stencil guidance? **A:** [auto-pick] One-or-more-labels wording plus an explicit line that implementation-with-its-own-new-test is `Edit` + `Create`, plus a tightened `Custom` line. **Why:** the stencil is what `Plan-Write` reads; the observed defect came from a stencil-conformant card, so the stencil has to name the new correct shape rather than merely permit it.
- **Q:** Update the plan-review rubric? **A:** [auto-pick] Yes — add "a `Custom` card expressible as a multi-label combination is a finding", in both `loom-rubric-plan-review.md` and `loom.md`. **Why:** the mechanical checks cannot detect an over-broad `Custom`; only review can, and the two files carry the same bullet and must move together.
- **Q:** Where does the new check sit in the fixed order? **A:** [auto-pick] Row 5, immediately after `card-type-missing`, shifting the rest. **Why:** the order groups by concern and no consumer is keyed to row numbers.
- **Q:** May a `Rename` group appear on a multi-label card? **A:** [auto-pick] Yes; `Pairs` stays a flat card-level field and `rename-mechanic-missing` fires on any `Rename` group. **Why:** only a `Rename` group can feed `Pairs`, so there is no ambiguity, and forbidding it would recreate the collision for renames that need their own new test.
- **Q:** Testing approach? **A:** [auto-pick] Table-driven parse and validate unit tests extending the existing suites, TDD on `checkPathMissing`'s group iteration, plus the golden round-trip. **Why:** the golden plan is the format's executable reference but is too coarse to pin per-check group semantics on its own.
- **Q:** Does Webster, batcher, or webstercli change? **A:** [auto-pick] No. **Why:** `sequence.go:174-177` intersects flat `Targets`/`Uses` refs and never branches on `CardType`, and the deviation union is likewise defined over the flat target set.
