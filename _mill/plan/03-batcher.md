# Batch: batcher

```yaml
task: 'webster: rewrite for flat card list'
batch: batcher
number: 3
cards: 3
verify: go test ./internal/batcher/...
depends-on: [1]
```

## Batch Scope

Stand up the new `internal/batcher` package: a LIBRARY of batchifier implementations behind
a `Batcher` interface (`[]planparser.Card → []Batch`), a name-keyed registry, and a
config-selected active batcher. Ship the **identity** batcher (one card → one batch) as the
first registered entry — one library member, NOT a "v0 version". The interface/registry/config
seam exists from day one so grouping batchifiers drop in later without a webster code change;
grouping batchifiers themselves are OUT of scope. `internal/batcher` imports stdlib +
`internal/planparser` only (leaf; no webster/cli imports). The interface batches 8 and 9
consume is `Batcher`, `Batch`, `Select(name string) (Batcher, error)`, and `DefaultName`.

## Cards

### Card 10: Batcher interface and Batch type

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/websterengine/state.go`
- **Edits:** none
- **Creates:**
  - `internal/batcher/batcher.go`
  - `internal/batcher/doc.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `batcher.go` define `type Batch struct { Cards []planparser.Card }` (an ordered group of ≥1 cards — webster's execution unit; a fork owns one `Batch`) and `type Batcher interface { Batch(cards []planparser.Card) []Batch; Name() string }`. Keep `Batch` minimal — the webster-side `BatchState` (batch 8) carries slug/SHA/status; `batcher.Batch` carries only the card grouping. Write `doc.go` as package godoc: state that batching is 100% webster's own execution-policy decision (never the plan's, never an LLM's), that the active batcher is chosen via config, and that the identity batcher is one library entry among future grouping batchers. No version suffix anywhere.
- **Commit:** `feat(batcher): Batcher interface and Batch type`

### Card 11: registry and identity batcher

- **Context:**
  - `internal/planparser/plan.go`
  - `internal/batcher/batcher.go`
- **Edits:** none
- **Creates:**
  - `internal/batcher/registry.go`
  - `internal/batcher/identity.go`
  - `internal/batcher/registry_test.go`
  - `internal/batcher/identity_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `registry.go` implement a name-keyed registry: an unexported `map[string]Batcher`, a `register(b Batcher)` used by `init()` to self-register library members, and `lookup(name string) (Batcher, bool)`. Define `const DefaultName = "identity"`. In `identity.go` implement `identityBatcher` whose `Batch(cards)` returns one single-card `Batch` per input card preserving order (N cards → N batches), `Name()` returns `"identity"`, and an `init()` that registers it. Tier-1 tests (pure logic, no git, no `TestMain`): `identity_test.go` asserts N cards → N single-card batches in order (including the empty-input and one-card cases); `registry_test.go` asserts register/lookup round-trips and that lookup of an unknown name reports not-found. Keep the identity test general enough that a future grouping batcher slots into the same contract. Follow `golang:golang-testing`.
- **Commit:** `feat(batcher): name-keyed registry and identity batcher`

### Card 12: config-selected active batcher

- **Context:**
  - `internal/batcher/batcher.go`
  - `internal/batcher/registry.go`
  - `internal/batcher/identity.go`
- **Edits:**
  - `internal/batcher/registry.go`
- **Creates:**
  - `internal/batcher/batcher_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `Select(name string) (Batcher, error)` to `registry.go`: an empty `name` resolves to `DefaultName` (`identity`); a registered name returns its batcher; an unregistered name returns a wrapped `batcher:`-prefixed error naming the unknown key (this is the load-time error webstercli surfaces when `webster.yaml`'s `batcher:` key names a nonexistent batcher — batch 8 adds the config key, batch 12 wires `Select` at config-load). In `batcher_test.go` (Tier-1) assert: empty name → identity; `"identity"` → identity; unknown name → error. Follow `golang:golang-testing`.
- **Commit:** `feat(batcher): config-selected active batcher via Select`

## Batch Tests

`verify: go test ./internal/batcher/...` runs Tier-1 tests for the interface contract, the
identity batcher, the registry, and config selection. Pure logic — no git, no `TestMain`,
no fixtures. Grouping batchifiers are out of scope and deliberately untested; the interface
tests stay general enough that a future grouping batcher reuses them.
