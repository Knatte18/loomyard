# Batch: lift-llm-refusals

```yaml
task: 'Seeded driver choice: ly-drive strand as the child''s driver'
batch: lift-llm-refusals
number: 5
cards: 3
verify: go build ./... && go test ./internal/shedrun/... ./internal/shedcli/... ./internal/battencli/... && go test -tags integration ./internal/shedcli/...
depends-on: [4]
```

## Batch Scope

This batch opens the gate the previous four batches built behind: the seed contract stops refusing the llm driver outright, and the two seeding surfaces start deciding by the recipe's own bootstrap-verb capability instead.
It is one batch because the three refusal sites answer one question and must change together — a seed contract that accepts a value no seeding surface will write is dead, and a seeding surface that writes a value the contract rejects cannot round-trip.
It lands last among the behavioural batches by the overview's `mechanism-ships-before-the-gate-is-lifted` decision, so no commit in this task ever accepts a driver value nothing honours.

Three refusals are lifted and one is **rewritten rather than removed**.
The likeliest regression in this batch is lifting all four for symmetry: the batten path's own `--driver` must stay refused, because batten has no bootstrap verb and its driver is the process the operator typed.

Batch-local decision beyond the overview's: every refusal decides by **emptiness of the recipe's bootstrap verb**, never by comparing a recipe name against the literal `loom`.
The refusal text derives from the same fact — a recipe with no bootstrap verb is refused with a message naming that, which is a statement about the recipe rather than about a roadmap item, and stays true as recipes are added.

## Cards

### Card 17: the seed contract accepts the llm driver

- **Context:**
  - `internal/shedrun/paths.go`
  - `internal/shedrun/runid.go`
- **Edits:**
  - `internal/shedrun/seed.go`
  - `internal/shedrun/seed_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `ValidateDriver` in `internal/shedrun/seed.go` to accept both driver constants and refuse anything else as unknown, collapsing today's three-arm switch into a two-arm one.
  The refusal message for an unknown value must now name **both** legal values rather than the go driver alone, since the closed vocabulary is genuinely two values from this commit on.
  Amend the driver constant's own doc comment, which today says the llm value is not yet implemented and that this function refuses it by naming a roadmap item: replace that with what the value now means — an ly-drive session inside the run's own worktree, booted by the recipe's bootstrap verb — and say that whether a given recipe can honour it is decided at the seeding sites by that recipe's bootstrap-verb capability, not here.
  Amend the package doc comment's own sentence about the closed driver vocabulary in the same file to match.
  Nothing else in the file changes: the read function still defaults an absent or empty driver to the go value, and the write function still validates through this same function, so both pick the widened vocabulary up with no edit of their own.
  In `seed_test.go` cover: the llm value validating cleanly; both values round-tripping through write and read; an unknown value refusing with a message naming both legal values; and an absent driver field still reading back as the go value.
  Add the round-trip case for the llm value explicitly — the read function validates after defaulting, so a seed written with the llm value would have failed to read back at all before this change, and that is the property the whole task depends on.
- **Commit:** `feat(shedrun): accept the llm driver in the seed contract`

### Card 18: the shed seed command gates on the bootstrap verb

- **Context:**
  - `internal/shedrun/seed.go`
  - `internal/shedcli/table.go`
  - `internal/shedcli/cli.go`
  - `internal/loomcli/bootstrapverb.go`
- **Edits:**
  - `internal/shedcli/seed.go`
  - `internal/shedcli/seed_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace the driver-flag refusal in `internal/shedcli/seed.go` with the capability check.
  The command keeps validating the flag value through the shedrun validator, which now accepts both values, and gains one further check when the value is the llm driver: look the seeded recipe up in the table and refuse when that entry's bootstrap-verb field is empty.
  Word the refusal as a statement about the recipe — that the named recipe has no bootstrap verb, so it cannot be driven by an LLM — and never as a pointer to a roadmap item.
  Decide by emptiness of the field, never by comparing the recipe name against a literal.
  Order the two checks so an unknown driver value is refused by the shedrun validator first and the capability check is reached only for a legal value: an operator who typed a misspelling should get the vocabulary error, not a capability error about a recipe that would have accepted the value they meant.
  In `seed_test.go` replace the case asserting the llm value refuses with the roadmap pointer.
  Pin the **capability predicate itself** rather than the recipe name: drive the check from a test-local table entry with an empty bootstrap verb and assert it refuses, and from one with a non-empty verb and assert it accepts, so the assertion cannot silently become "is it spelled loom".
  Cover the real table too — seeding the loom recipe with the llm driver succeeds and the seed reads back carrying that value — and keep every existing case in the file green, the unknown-recipe and malformed-parameter refusals among them.
- **Commit:** `feat(shedcli): gate --driver llm on the recipe's bootstrap verb`

### Card 19: batten accepts a child driver and refuses its own

- **Context:**
  - `internal/shedrun/seed.go`
  - `internal/battencli/bootstrapverb.go`
  - `internal/battencli/cli.go`
- **Edits:**
  - `internal/battencli/refusal.go`
  - `internal/battencli/refusal_test.go`
  - `internal/battencli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/battencli/refusal.go`, split the one driver refusal that today covers both flags into the two different answers the two flags now have.
  The child-driver flag's llm value is **accepted**: it names the driver the child worktree's own bootstrap will honour, and batten's own lack of a bootstrap verb says nothing about the child's.
  Validate it through the shedrun validator and nothing further — the child's recipe capability is checked when that child's seed is written, not here.
  Batten's own driver flag stays **refused** for the llm value, and the message is rewritten rather than removed: derive it from this package's own bootstrap-verb constant and word it as a statement about the recipe — batten has no bootstrap verb, so it cannot be driven by an LLM.
  Read the constant directly rather than reaching for the shed CLI's table: this package cannot import that one without an import cycle, which is the reason the capability is declared per module in the first place.
  Keep the refusal reachable through the same call path it uses today so the flag's own error shape does not change.
  In `refusal_test.go` assert the batten driver flag still refuses the llm value, that the message names the missing bootstrap verb, and that it no longer names a roadmap item.
  In `cli_test.go` assert the child-driver flag accepts the llm value and that the accepted value reaches the seed the child is created with.
  Assert both flags still accept the go value and still refuse an unknown one.
  The four-way matrix is the point of this card's tests: the likeliest regression is lifting both refusals for symmetry, and only a case asserting the batten driver flag still refuses catches it.
- **Commit:** `feat(battencli): accept --child-driver llm and rewrite the own-driver refusal`

## Batch Tests

`verify: go build ./... && go test ./internal/shedrun/... ./internal/shedcli/... ./internal/battencli/... && go test -tags integration ./internal/shedcli/...` covers the three packages holding the refusal sites plus a whole-module build.
The integration-tagged shed CLI suite runs because that package's seeding tests write real seeds through a real location, and the round-trip this batch opens — a seed carrying the llm driver being written and read back — is the one property an untagged test with a stubbed filesystem cannot prove.

The load-bearing cases are the negative ones.
Card 19's still-refused assertion is the only thing standing between this batch and the symmetric over-lift the discussion names as a plan writer's likeliest regression, and it is easy to drop precisely because the other three sites in the same batch are being opened.
Card 18's table-driven capability assertions are what keep the predicate from degenerating into a recipe-name comparison — an assertion driven by the real table alone passes against a hard-coded name check today and starts refusing the next recipe to grow a bootstrap verb, with a message that reads as authoritative and is wrong.
Card 17's llm round-trip case is the property the whole task rests on: the read path validates after defaulting, so before this commit a seed carrying that value could be written and then never read back.
