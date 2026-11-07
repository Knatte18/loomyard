<!-- This is the loom-block-plan-format plan-format block: what a card is and the on-disk layout.
     The loom stencils that write plans pull it in with the bare {{template "loom-block-plan-format"}}, with no pipeline.
     The fill hands it the including seat's value map, so it declares one required marker: {{.specs_dir}}.
     It names no step of the stencil around it;
     each including stencil states its own rule for the lines this block leaves out. -->

### What a card is

Each card is the smallest change that:

1. **Builds on its own** — the project builds with the repo's build immediately after the card's commit;
   never reference a symbol that no earlier card creates.
2. **Is independently committable** — a meaningful, revertible git commit on its own.
3. **Carries the coverage for the behavior it introduces** — the card's own targets include the test that covers it.
   `verify:` commands are no substitute for test coverage;
   only pure refactors/renames may rely on existing tests instead.
   A stencil's static prose is not behavior a test covers.
4. **Greps for reworded messages** — a card that changes a user-visible message or error text greps the repository for the old text, and every test asserting it joins that card's targets.
   A stale exact-string assertion then fails on the card that reworded it, under that card's own gate, not at the plan-level gate.

The plan runs strictly in card order, and the Card Index lists the cards in that order: a card never depends on anything a later card does.
`{{.specs_dir}}/loom/loom-plan-spec.md`'s "Plan vs. schedule" section holds the rule.

A card that needs a corpus or a fixture takes it from committed test data in the tree, created by an earlier card where it does not exist yet, never from the live files of other worktrees or the fabric repo.
`card-fabric-reference` in `{{.specs_dir}}/loom/loom-plan-checks-spec.md` names what the plan gate refuses.

The plan gate also refuses a card for `test-tier-unstated`, `seam-test-unlisted` and `doc-go-unlisted`; `{{.specs_dir}}/loom/loom-plan-checks-spec.md` holds their rules.

A card that prescribes an addition to a `PATTERN.md` entry line leaves the line's rule text under the cap, `pattern-entry-line-cap` in the same spec: the gate fails a fenced verbatim entry line whose rule text is over it.
A declared `adds` or `changes` entry is prescribed verbatim as its whole overview line in a fenced block on a card that edits `PATTERN.md`.
A card fences only the new line, never the old one or an example, since the gate reads every fenced line that opens like an entry.
The plan gate measures the overview those lines project against the headroom line, so a line that grows `PATTERN.md` past it is refused.
It also measures each card's own step in plan order, `pattern-card-headroom` in the same spec, so a later card that shortens the overview never covers an earlier card's growth: the shortening goes on the growing card or before it.
A card that removes an entry names the entry's ID in backticks, so the gate credits the removal to that card.

### On-disk layout

`00-overview.md` + one `NN-<card-slug>.md` per card. `NN` is zero-padded and equals the card's flat heading number `N`;
cards run `1..M` with no gaps.