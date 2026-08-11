# Batch: documentation

```yaml
task: 'batcher: split out of webster into a standalone configreg module with its own batcher.yaml'
batch: 'documentation'
number: 3
cards: 4
verify: go build ./... && go test ./internal/websterengine/...
depends-on: [2]
```

## Batch Scope

This batch amends the documentation sites whose claims batches 1–2 falsify, across the twelve files its four cards edit.
It is one batch because every card is the same operation — a prose correction with no behaviour change — and because the sites are only correct to write once the code they describe has landed, which is why the whole batch depends on batch 2 rather than interleaving with it.
Two further corrections are deliberately NOT here: `websterengine/config.go`'s `Config` type doc and `websterengine/template.go`'s `ConfigTemplate` doc are fixed inline by batch 2 card 5, because that card's own edits are what falsify them.

There is no new external interface;
this batch closes the Documentation Lifecycle obligation for the task.

The enumeration method is stated in `## Shared Decisions` → doc-site-ownership and is checkable rather than asserted: grep `batcher.Select`, `batcher:`, and `batcher.yaml` across all production Go and markdown.
Two claims recur across sites and must be reworded consistently everywhere they appear:
the ownership claim ("batching is 100% webster's own execution-policy decision") becomes "a standalone step webster consumes today, and one `Shed` will drive as producer #8 once built";
the config-key pin ("webster.yaml's `batcher:` key") becomes "`batcher.yaml`'s `active:` key".
The clauses "never the plan's decision" and "never an LLM's decision" stay true everywhere they appear and must be preserved.

Batch-local decision beyond `## Shared Decisions`: `manifest/designs/loom.md` row 8 is falsified by this task but is deliberately NOT edited — it is task E's to write, per `manifest/designs/shed-followups.md`'s Scope section.
Do not touch it in any card.

## Cards

### Card 9: internal/batcher's own package and registry docs

- **Context:**
  - `internal/batcher/config.go`
  - `internal/batcher/template.yaml`
  - `manifest/designs/loom.md`
- **Edits:**
  - `internal/batcher/doc.go`
  - `internal/batcher/registry.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/batcher/doc.go`, rewrite two things.
  First, the package summary sentence naming `Select` as what "resolves the config-chosen active batcher by name" must also name `Active` as the config entry point, since `Select` is now a name-level primitive `Active` builds on rather than the thing config callers reach for.
  Second, the sentence "Batching — how many cards land in one fork, and in what grouping — is 100% webster's own execution-policy decision" must be replaced per the ownership rewording in `## Batch Scope`, keeping the two following clauses ("It is never the plan's decision …" and "never an LLM's decision …") intact.
  Third, the paragraph beginning "The active batcher is chosen via webster.yaml's batcher: config key (see docs/reference/plan-format.md and websterengine's config loading), which Select resolves against the registry at config-load time" must name `batcher.yaml`'s `active:` key and `Active` instead, and its cross-reference must drop "websterengine's config loading" — webster no longer loads it.
  Keep the "An empty key resolves to DefaultName, the identity batcher" sentence, which stays true.
  Leave the whole identity-batcher paragraph and the no-version-suffix sentence untouched.
  In `internal/batcher/registry.go`, rewrite the file-header comment clause "and webster resolves the config-chosen active batcher back out by name via the exported Select" so it names `Active` (this package's own config entry point, config.go) as what resolves the config-chosen batcher, with `Select` described as the name-level lookup `Active` and tests call directly.
  Do not name webster as the caller — after batch 2 no webster code calls `Select` at all.
  Change no code in either file.
- **Commit:** `docs(batcher): reword package and registry docs for the config split`

### Card 10: websterengine's package doc and the two bracket-verb Deps comments

- **Context:**
  - `internal/batcher/doc.go`
  - `internal/websterengine/runlevel.go`
- **Edits:**
  - `internal/websterengine/doc.go`
  - `internal/websterengine/recordbatch.go`
  - `internal/websterengine/beginbatch.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/websterengine/doc.go`, rewrite the whole batcher paragraph, not just its config-key clause — fixing one of its two stale claims and not the other would leave webster's and batcher's package docs contradicting each other.
  The clause "selected once at config-load time via webster.yaml's `batcher:` key (default: the identity batcher — one card, one batch)" must name `batcher.yaml`'s `active:` key, and should also record that the selection is resolved by `internal/webstercli` and handed to `Run` via `RunDeps.Batcher` — this package loads no batcher config itself.
  The sentence "Batching is 100% webster's own execution-policy decision" must be reworded exactly as card 9 rewords the identical sentence in `internal/batcher/doc.go`.
  Keep the "never the plan's" and "never an LLM's" parentheticals and the following "In v0 the identity batcher is the only registered entry, so batch ≡ card everywhere this package numbers or persists a batch" sentence intact.
  In both `internal/websterengine/recordbatch.go` and `internal/websterengine/beginbatch.go`, the `RecordDeps`/`BeginDeps` doc comments each say "Batches is the batchifier-derived execution batches (see internal/batcher.Select) `run` computed once at entry".
  Repoint the parenthetical from `internal/batcher.Select` to `RunDeps.Batcher`, since after batch 2 `run` computes the batches from the injected batchifier and calls `Select` nowhere.
  Keep the rest of each comment, including `beginbatch.go`'s trailing "and threads through every bracket verb call", intact.
  Change no code in any of the three files.
- **Commit:** `docs(webster): repoint batcher references at batcher.yaml and RunDeps.Batcher`

### Card 11: CONSTRAINTS.md and the reference docs

- **Context:**
  - `internal/batcher/template.yaml`
  - `internal/batcher/config.go`
- **Edits:**
  - `CONSTRAINTS.md`
  - `docs/overview.md`
  - `docs/reference/plan-format.md`
  - `docs/reference/webster-contract.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `CONSTRAINTS.md`'s **Batcher Registry+Config Invariant**, both the ownership claim in the section's one-line summary ("webster's execution unit is the batchifier-derived batch, not the raw plan card") and the config-key pin in the bullet ("plus the `batcher:` webster.yaml config key (default `identity`)") must be updated.
  The invariant's substance survives intact and must not be weakened: batching is still selected by `internal/batcher`'s name-keyed registry plus a config key defaulting to `identity`, still with no plan-supplied batching and no batch grouping in the plan format.
  Only the file the key lives in changes — to `batcher.yaml`'s `active:` key, owned by `internal/batcher` rather than by webster.
  Leave the "**Enforced by** review obligation" line as-is.
  In `docs/overview.md`, fix two entries.
  The **batcher** module-table entry pins the key to `webster.yaml`'s `batcher:` config key — repoint it to `batcher.yaml`'s `active:` key and state that batcher is its own configreg module.
  The **webster** entry describes batcher as "its own config-selected `internal/batcher` registry" — the possessive is the same ownership claim this task removes, so reword it to name `internal/batcher` as a separate config-selected module webster consumes.
  In `docs/reference/plan-format.md`'s "## Batch is gone / the card is the unit" section, the card stays the plan's unit and that framing must be preserved;
  what goes is the "entirely internal to webster" framing — the heading-following sentence "Batching is a webster-internal execution-policy optimization, not a plan-schema concept" and the clause "a later, measured, entirely internal decision" both need rewording so batching reads as a step outside the plan schema rather than as webster-internal policy.
  Leave the "no batch-level declared ownership `## Scope` concept" paragraph untouched.
  In `docs/reference/webster-contract.md`'s "## Plan input" section, the sentence "webster groups a plan's cards into execution batches via a config-selected batcher" is ambiguous today and reads as actively wrong once the config is not webster's;
  a one-clause fix naming `batcher.yaml` as the config source is enough.
- **Commit:** `docs: repoint the batcher config key at batcher.yaml across constraints and reference docs`

### Card 12: the embedded prompt, the sandbox suite, and the planparser doc

- **Context:**
  - `internal/batcher/template.yaml`
  - `internal/configreg/configreg.go`
  - `internal/configcli/configcli_test.go`
- **Edits:**
  - `internal/websterengine/master-template.md`
  - `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
  - `internal/planparser/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/websterengine/master-template.md`, the line "It is your navigation source, not the execution unit: `lyx webster` groups this flat list into execution batches via the plan's configured batchifier (one card per batch under the default identity batchifier)" is wrong *today* — the batchifier is not the plan's, it is config's — and this task makes it wrong a second way by moving the config owner.
  Correct "the plan's configured batchifier" to name `batcher.yaml`'s configured batchifier.
  Leave the parenthetical about the default identity batchifier and the whole "you drive the loop below by BATCH number, not by reasoning about grouping yourself" instruction untouched;
  this is an embedded agent prompt and its operational instruction must not shift.
  In `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`, item 4's "**Wired worktree required.**" bullet enumerates exactly which config files a wired worktree materializes ("`_lyx/config/webster.yaml`, plus `shuttle.yaml`/`reed.yaml` since webster branches off shuttle directly").
  That enumeration is complete today and incomplete the moment `batcher.yaml` becomes required, so add `batcher.yaml` to the list.
  The suite still runs green with no other change — each scenario drives a fresh `lyx fabric clone`, which reconciles against the then-current registry and materializes the new file automatically — so change nothing else in this file, and add no `**Covers:** batcher` tag.
  In `internal/planparser/doc.go`, the phrase "every consumer (webster's batcher, master, and fork prompt rendering) goes through planparser.ParsePlan" carries the same ownership claim this task removes everywhere else.
  Reword it to "the batcher, webster's master, and fork prompt rendering".
  A one-word fix, listed rather than swept silently because the site list claims completeness.
  Do NOT edit `internal/configcli/configcli_test.go`.
  Its comment "The other nine modules are absent; their sections must each say # (not configured)" looks like a count this task falsifies, and it is not: that test seeds `board` and leaves the rest absent, so "other" means total minus one.
  With today's nine registered modules "other" is eight and the word "nine" is already a pre-existing off-by-one;
  batch 1's registration makes the total ten and "other" nine, so the existing word becomes correct untouched.
  Bumping it to "ten" would introduce the error this card would be claiming to fix.
  Change no code in any of these three files.
- **Commit:** `docs: fix the remaining batcher-ownership claims across prompt, sandbox, and comments`

## Batch Tests

This is a prose-only batch — no card changes a line of executable code — so `verify:` proves two things rather than exercising new behaviour.

`go build ./...` catches a malformed Go comment (an accidentally unterminated block comment or a stray character in a package doc) across the four `.go` files cards 9, 10, and 12 touch.

`go test ./internal/websterengine/...` is the real gate: `internal/websterengine/master-template.md` is `//go:embed`-ed, and `internal/websterengine/template_test.go` pins its rendered content by substring, so an edit that removes more of the "navigation source" line than intended fails there rather than silently degrading a live agent prompt.

There is no runnable surface for `CONSTRAINTS.md`, `docs/overview.md`, `docs/reference/plan-format.md`, `docs/reference/webster-contract.md`, or `tools/sandbox/SANDBOX-WEBSTER-SUITE.md` — those five are review-obligation documents, verified by reading rather than by a command.
The task's repo-wide done gate (`go test ./... && go test -tags integration ./...`) is the backstop for anything this batch's scoped verify does not reach.
