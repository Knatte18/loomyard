# Plan: modelspec: bare-effort shorthand and version-to-v rename

```yaml
task: 'modelspec: bare-effort shorthand and version-to-v rename'
slug: 'modelspec-effort-shorthand'
approved: true
started: '20260913-103908'
parent: 'main'
root: ""
verify: go build ./...
discussion_sha: 0113690c7ca65db1d8adc5303da8a16f2081cadf
```

## Batch Index

_The fenced yaml block below is the authoritative DAG mill-go reads to schedule batches.
Every batch lives at `NN-<batch-slug>.md` in this directory and is mirrored as one entry here._

```yaml
batches:
  - number: 1
    name: bracket-grammar
    file: 01-bracket-grammar.md
    depends-on: []
    verify: go test ./internal/modelspec/...
```

## Shared Decisions

### Decision: one batch, six cards

- **Decision:** the whole task is a single batch. Six cards in a fixed order: the vocabulary map, the parser, the parser tests, the vocabulary-sync test, the pinned contract doc, the module overview doc.
- **Rationale:** the blast radius is five existing files plus one new test file, all inside one package plus two markdown docs. Every card shares the same `Context:` and the same mental model of the bracket grammar; splitting would push two adjacent batches over the 80%-shared-context merge rule in the opposite direction.
- **Applies to:** all batches

### Decision: canonical `Params` key stays `version`

- **Decision:** `v` is a bracket spelling only. A parsed bracket never yields a `Params` key named `v` — `bracketKeys` maps the accepted bracket spelling to the canonical key, and the canonical key is what lands in the map.
- **Rationale:** seven production call sites read `resolved.Params["version"]`, and `Registry.Resolve` overlays bracket params onto `Entry.Defaults` key-by-key, so a literal `v` key would sit beside a default `version` key instead of overriding it.
- **Applies to:** all batches

### Decision: closed vocabularies gate keys, never values

- **Decision:** the bare bracket token is validated against the model-id charset and against nothing else. No effort-value vocabulary is introduced anywhere in this task.
- **Rationale:** effort values are engine-validated one layer down; a second closed vocabulary here would need a recompile every time a provider adds a tier, breaking the pinned adopt-a-new-model-without-recompile property.
- **Applies to:** all batches

### Decision: error strings are pinned by the plan, not left to the implementer

- **Decision:** the three new or changed error strings are stated verbatim in the cards that produce them and asserted by substring in the test card. The implementer copies them; it does not invent punctuation.
- **Rationale:** two of the three are asserted across two files written by two different cards, and the test card cannot assert a shape the parser card was free to choose.
- **Applies to:** all batches

### Decision: the bracket grammar line is pinned verbatim, once

- **Decision:** the one-line bracket grammar is this exact text, and both places that state it copy it character-for-character:

  ```
  <alias>[item,item,...]        where item is  key=value  |  <effort>
  ```

  The escape-form line is the same shape with `<provider>:<model-id>` in place of `<alias>`, and states the `item` production by reference rather than repeating it.
- **Rationale:** the same grammar is stated twice — once in the pinned contract doc's Grammar section and once in the package doc of `internal/modelspec/modelspec.go` — by two different cards. Describing it in prose in both places and asking for "identical wording" is exactly the cross-card drift the error-string decision above already refuses to accept; the literal text belongs in one place and gets copied.
- **Applies to:** all batches

### Decision: docs land in the same batch as the code

- **Decision:** the pinned contract doc and the module overview doc are cards 5 and 6 of the same batch as the parser change, not a follow-up batch.
- **Rationale:** the Documentation Lifecycle in `CONSTRAINTS.md` requires observable-behaviour changes to land with their docs. The task branch squash-merges as one commit, so same-batch is same-commit at landing granularity.
- **Applies to:** all batches

### Decision: `manifest/roadmap.md` does not move

- **Decision:** no roadmap edit in this task.
- **Rationale:** this is a grammar change to an already-shipped module, not the completion of a planned item.
- **Applies to:** all batches

### Decision: module-wide `verify:` is a compile check, not a second suite

- **Decision:** the overview-level `verify:` is `go build ./...`. The repo-wide test sweep is already covered by `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`), which mill-go runs once before marking the task done.
- **Rationale:** the one cross-package risk this task carries is a consumer depending on the deleted `no '=' separator` error string, which a compile alone will not catch but the done gate will; `go build ./...` is the cheap batch-boundary half of that pair and does not duplicate the done gate's cost at every round.
- **Applies to:** all batches

## All Files Touched

- `contracts/specs/llm-model-spec.md`
- `docs/overview.md`
- `internal/modelspec/modelspec.go`
- `internal/modelspec/modelspec_test.go`
- `internal/modelspec/parse.go`
- `internal/modelspec/parse_test.go`
