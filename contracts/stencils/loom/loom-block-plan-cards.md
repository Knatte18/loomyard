<!-- This is the loom-block-plan-cards plan-format block: the per-card file format, the rename mechanic and the skeleton.
     The loom stencils that write plans pull it in with the bare {{template "loom-block-plan-cards"}}, with no pipeline.
     The fill hands it the including seat's value map,
     so it declares two required markers: {{.specs_dir}} and {{.gate_tiers}}.
     It names no step of the stencil around it;
     each including stencil states its own rule for the lines this block leaves out. -->

A symbol `Rename` needs the hub's `rename_tools` in `language.yaml`; the plan gate refuses one when the list is empty.

### Each `NN-<card-slug>.md`

In this exact order: `# Card N — <name>`;
one or more bold type labels from `**Create:**`, `**Edit:**`, `**Extends:**`, `**Delete:**`, `**Rename:**`, `**Move:**`, `**Prosa:**`, `**Custom:**`, each label's own indented backtick-wrapped sub-bullets are the card's targets for that label;
optionally `**Uses:**`, in the same bullet shape, for what the card reads but does not change;
a required, multi-line `**Intent:**` (prose — what, and why);
`**ImpactSummary:**` on `Edit`/`Extends`/`Delete` cards only, taking its value inline on the label line;
optionally `**Commit:**` (must start `N: `) and `**Verify:**`.

An implementation card lists the implementation's member glyphs under `**Edit:**`, beside the test members it changes or the `**Create:**` handles of the tests it adds.
A new test file is a `**Create:**` path, written only for a surface no existing test file covers.

`**Edit:**` lists the member glyphs the card changes, and a file path only for a file with no symbol to name: a non-Go file, a file `lym quarry toc` lists with no symbol, or a whole file the card creates or deletes.
A `_test.go` path with a symbol to name under `**Edit:**` is refused by `edit-test-file` in `{{.specs_dir}}/loom/loom-plan-checks-spec.md`, which holds the way forward.
A card names an existing test function, fake or helper it changes as a member glyph, for example `internal/x#TestFoo` or `internal/x#fakeClock`.
A new test function is a `**Create:**` declaration with a `plan:` handle whose unit is the package directory, or `<dir>_test` for an external test package, never a file, so the plan does not pin the file a new test goes in.
A card whose new tests reuse an existing test's setup, or call existing test helpers or fakes, names those members as glyphs under `**Uses:**`.
Listing a file or a package beside a member of it on one card is refused: `redundant-package-target` and `redundant-file-target` in `{{.specs_dir}}/loom/loom-plan-checks-spec.md` say what counts and the way forward.

A card that changes a member's parameters, results, type parameters or receiver writes that `**Edit:**` bullet with the re-sign arrow, `` `<glyph>` -> `<new declaration head>` ``, the head written like a `Create` head.
An interface method or a struct field takes no arrow, and the card's `**Intent:**` states its change.

A card that deletes or re-signs a member lists every caller of it: the file, or the member glyph whose body holds the call, on the same card for a re-sign and on the same or an earlier card for a delete.
For a caller in a `_test.go` file the card lists only the member glyph whose body holds the call.
`caller-uncovered` in `{{.specs_dir}}/loom/loom-plan-checks-spec.md` says what counts.

`**Extends:**` lists a ref that already exists and that the card only adds to, without changing anything already there: a flag, a list line, a doc paragraph, a spec entry, a verb in a wire function.
A ref whose existing content the card rewrites, reorders or removes goes under `**Edit:**`, and a re-sign arrow never goes under `**Extends:**`.
Two cards that extend one ref make no dependency on each other, unless both cards fence a verbatim line for one PATTERN entry ID.
The card that writes the code writes its docs too, naming the doc files under `**Extends:**`, so no separate closing docs card is planned.

`**Custom:**` is a last resort, used only where none of the other seven genuinely fits.
A card whose targets can be expressed as a multi-label combination of the other seven is not `Custom`.
A `**Custom:**` group may not be combined with a group of a different type.

A card that introduces a new verb, flag, escape hatch or routing edge, or removes, downgrades or makes bypassable a guard, states in its `**Intent:**` what the change can now skip or let through and what bounds it.
Plan-Review flags one left unbounded.

A card that prescribes a refusal or guard branch names in its `**Intent:**` the input that reaches it after every upstream check passes, and the test that reaches it.
A propagated raw I/O error, which `refusal-spec.md` exempts as transient, is exempt.
A branch that upstream validation already rules out is not prescribed: drop it, or, when it guards an invariant, write it as a fail-loud wiring guard in `refusal-spec.md`'s sense, which takes no reaching test.
A test that calls only the refusal's error constructor does not reach it.

A field with no content is omitted entirely — never write a `none` sentinel on any field.

**`Uses:` names what the card reads but does not change — never a target.**
An entry appearing in both a card's own target list and its own `Uses:` is a contradiction: is it being changed, or only read?
That is the `card-field-overlap` finding — see `{{.specs_dir}}/loom/loom-plan-spec.md`'s own Card fields section for the full grammar, and `{{.specs_dir}}/loom/loom-plan-checks-spec.md` for the validation checks.

The gates run the hub's tier lists over scopes lym computes, so the plan carries no plan-level verify section and asks for none.
The hub's tiers and gate lists follow:
{{.gate_tiers}}
A card's `**Verify:**` is the optional escape hatch for what no gate expresses, written only for what the gates cannot catch on their own.
A `**Verify:**` line that tests is one `lym gate test --wait` call with a tier list and a scope form, never a raw test command or a target list.
See `{{.specs_dir}}/loom/loom-plan-spec.md`'s verify model section for the gates and scope forms themselves — this file does not restate them.

### `## Rename mechanic` — reproduce verbatim when any card is type `Rename`

A `Rename` card's bullets are `` `old` -> `new` `` pairs.
A genuinely new file with no predecessor belongs in a separate `Create` card, never folded into a `Rename` pair.

```markdown
## Rename mechanic

1. For a file pair (both sides file self glyphs or paths), run `git mv <old> <new>` FIRST, before any other change to the moved file.
2. Then make ONLY surgical edits (package declaration, imports, identifier
   retargeting) — no unrelated rewrites.
3. A genuinely new file with no predecessor belongs in a separate `Create` card, never folded
   into the `Rename` pair.
4. Never write the relocated file from scratch and delete the original — that loses
   git history exactly as an unstructured create+delete pair would.
5. For a symbol pair, run the hub's structure-aware rename tool the implementer prompt names (`language.yaml` `rename_tools`), never a text or regex replace, then grep for zero remaining old-name references and confirm the symbol's doc comment opening names the new name.
```

### Minimal skeleton

`00-overview.md`:

```markdown
---
format: 5
approved: false
---

# Plan: <task title>

<task-framing paragraph>

## Card Index

1 — <card-slug> — <one-line intent>
```

`01-<card-slug>.md`:

```markdown
# Card 1 — <name>

**Edit:**
- `internal/boardcli#newListCmd`
- `internal/boardcli#Row.Render` -> `func (r Row) Render(width int) string`
- `internal/boardcli#TestList`

**Intent:** <the change to make, concretely>

**ImpactSummary:** <one line: the blast radius across this card's Edit/Extends/Delete targets>
```

`**ImpactSummary:**` is in the skeleton because the skeleton's own label is `**Edit:**`, and
`card-missing-field` blocks an `Edit` or `Delete` card that omits it.
A card whose only labels are `Create`/`Rename`/`Move`/`Prosa`/`Custom` omits the field entirely.