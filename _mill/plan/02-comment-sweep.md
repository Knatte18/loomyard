# Batch: comment-sweep

```yaml
task: 'builder: delete internal/builderengine and internal/buildercli, retire builder-contract.md as a reference'
batch: comment-sweep
number: 2
cards: 3
verify: go build ./... && go vet ./... && go test ./internal/websterengine/... ./internal/webstercli/... ./internal/pattern/... && go test -tags integration ./internal/websterengine/... ./internal/webstercli/...
depends-on: [1]
```

## Batch Scope

Batch 1 removed builder from the build;
this batch removes it from the Go source's prose.
Every remaining `builderengine` / `buildercli` / bare-word `builder` reference in a Go doc-comment is rewritten to stand on its own, so no comment names a package that no longer exists and no comment's reasoning collapses into a fragment.
It is one batch because the three cards share a single rewrite policy and a single completion criterion, and because none of them changes behavior — every card here is comment-only and leaves the tree building and green.

This batch depends on batch 1 because the sweep's completion criterion is a zero-hit grep for the package names, which cannot pass while the packages themselves are still in the tree.

Batch-local decision: this half of the sweep is done **by hand, one sentence at a time**, never by a scripted find/replace.
The provenance comments state webster's mechanisms by reference to builder's;
no regex turns "mirroring builderengine's own runlevel.go naming note" into standalone prose.

## Cards

### Card 5: Rewrite websterengine's provenance comments to stand alone

- **Context:**
  - `internal/websterengine/audit_test.go`
  - `internal/websterengine/integration.go`
  - `internal/websterengine/summary.go`
- **Edits:**
  - `internal/websterengine/archive.go`
  - `internal/websterengine/audit.go`
  - `internal/websterengine/beginbatch.go`
  - `internal/websterengine/beginbatch_test.go`
  - `internal/websterengine/classify.go`
  - `internal/websterengine/config.go`
  - `internal/websterengine/config_test.go`
  - `internal/websterengine/digest.go`
  - `internal/websterengine/doc.go`
  - `internal/websterengine/fingerprint.go`
  - `internal/websterengine/gitwrap.go`
  - `internal/websterengine/outcome.go`
  - `internal/websterengine/pause.go`
  - `internal/websterengine/poll.go`
  - `internal/websterengine/recordbatch.go`
  - `internal/websterengine/recoverbatch.go`
  - `internal/websterengine/recoverbatch_test.go`
  - `internal/websterengine/render.go`
  - `internal/websterengine/report.go`
  - `internal/websterengine/roles.go`
  - `internal/websterengine/roles_test.go`
  - `internal/websterengine/runlevel.go`
  - `internal/websterengine/state.go`
  - `internal/websterengine/state_test.go`
  - `internal/websterengine/strand.go`
  - `internal/websterengine/summary_test.go`
  - `internal/websterengine/template.go`
  - `internal/websterengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite every doc-comment in the listed files that names `builderengine`, `buildercli`, or the bare word `builder` so it states its reason directly, dropping the builder reference entirely.
  Do not delete the clause and leave the remainder — several comments (`fingerprint.go`, `classify.go`, `runlevel.go`'s naming note) exist almost entirely to state the derivation and would become fragments explaining nothing.
  Do not mark the derivation historical ("the since-deleted builderengine") — that keeps a deleted package's name in the tree, which is what this sweep removes.
  Worked example, the required shape: `pause.go`'s "a webster-local copy of builderengine's pause-flag mechanics" becomes "webster's own pause-flag mechanics, deliberately module-local rather than shared".
  Comments citing the Shared Decision name `builder-is-frozen-copy-not-move` (notably `outcome.go`'s header, which explains why webster defines its own `outcome` shape) need their **premise** replaced, not just the word swapped: this task falsifies that decision outright, so restate the reason as webster owning its own contract shape rather than as a copy-vs-move choice against a frozen sibling.
  Leave `internal/websterengine/audit_test.go` untouched — its eleven `master-builder` / `master-builder-weft` occurrences are arbitrary worktree-name fixtures unrelated to the builder module, and are a named exclusion in the acceptance grep.
  Leave `audit.go`'s `"/hub/master-builder/_lyx/webster/outcome.yaml"` example path for the same reason;
  only its `builderengine`-naming clock-seam comment changes.
  This card changes comments only — no identifier, no behavior, no test assertion.
- **Commit:** `docs(webster): rewrite builder-derivation comments to stand alone`

### Card 6: Rewrite webstercli's provenance comments to stand alone

- **Context:**
  - `internal/webstercli/sync_integration_test.go`
- **Edits:**
  - `internal/webstercli/beginbatch.go`
  - `internal/webstercli/cli.go`
  - `internal/webstercli/cli_test.go`
  - `internal/webstercli/pause.go`
  - `internal/webstercli/run.go`
  - `internal/webstercli/status.go`
  - `internal/webstercli/validate.go`
  - `internal/webstercli/verbs_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Apply card 5's rewrite policy to `internal/webstercli`.
  Named sites that need a restated premise rather than a word swap: `validate.go`'s "Unlike builder, webster's own Run pre-flight ALSO refuses a zero-batch plan outright" (state the rule as webster's own, without the contrast), and `status.go`'s "unlike builder (which only learns …)" and "builder-parity in verb shape" (state what webster's `status` verb does and why its shape is what it is).
  `verbs_test.go`'s two comments cite `buildercli`'s own `spawnbatch_test.go` and its package-local injection point as precedent — restate them as descriptions of webster's own starter/injector seam.
  Ignore `verbs_test.go`'s twelve `strings.Builder` occurrences;
  they are the stdlib type and are an enumerated exclusion in the acceptance grep.
  `internal/webstercli/sync_integration_test.go` is card 1's and must not be edited again here.
- **Commit:** `docs(webster): rewrite webstercli builder-derivation comments`

### Card 7: Sweep the remaining Go and CONSTRAINTS.md builder references

- **Context:**
  - `internal/websterengine/doc.go`
- **Edits:**
  - `CONSTRAINTS.md`
  - `internal/loomengine/config_test.go`
  - `internal/loomengine/configtemplate.go`
  - `internal/modelspec/modelspec.go`
  - `internal/pattern/doc.go`
  - `internal/pattern/leaf_enforcement_test.go`
  - `internal/planparser/validate.go`
  - `internal/reedengine/headertemplate.go`
  - `internal/scoutengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/planparser/validate.go`, the header comment contrasting planparser with the deleted frozen v2 validator ("Unlike the frozen v2 validator …, findings are keyed by card") loses its comparand — restate it as planparser's own rule (findings are keyed by card) without the contrast.
  In `internal/reedengine/headertemplate.go`, replace the "builderengine `*-template.md` precedent" citation with the general rule it stands for: prompt/text assets ship as embedded `*-template.md` files rendered via `internal/stencil` rather than as Go string literals.
  In `internal/loomengine/configtemplate.go`, drop `builderengine` from "mirroring builderengine's and perchengine's embed-and-accessor" so only `perchengine` is cited.
  In `internal/loomengine/config_test.go`, replace the `builderengine/config_test.go` example of an integration-tagged config fixture with `websterengine/config_test.go`.
  In `internal/scoutengine/doc.go`, change "importable by any future consumer (e.g. builder or webster)" to name webster alone.
  In `internal/modelspec/modelspec.go`, change "builder's roles" to "webster's roles" and "(builder, perch/burler/loom configs)" to "(webster, perch/burler/loom configs)".
  In `internal/pattern/leaf_enforcement_test.go`, `internal/pattern/doc.go`, and `CONSTRAINTS.md` the feature-package lists are three mirrors of one another and must stay consistent: drop `builderengine` from `leaf_enforcement_test.go`'s list and from `CONSTRAINTS.md`'s two lists (the Scout shared-infrastructure sentence and the `internal/pattern` import rule), and change `pattern/doc.go`'s "builder implementer, webster fork, loom plan" to "webster fork/Master, burler review+fix, loom plan".
  In `CONSTRAINTS.md`, also narrow the **Fabric Git Invariant (warp + weft)**'s Enforced-by block: it machine-checks `internal/websterengine` together with the deleted builder engine package via `TestNoRawGitMutation_WebsterBuilderProductionSource` — change it to name `internal/websterengine` alone via `TestNoRawGitMutation_WebsterProductionSource`, matching card 1's rename.
  Locate that block by the **Fabric Git Invariant (warp + weft)** heading, not by line number, and do not edit the Review Round Invariant that follows it.
- **Commit:** `docs: sweep remaining builder references from Go comments and CONSTRAINTS.md`

## Batch Tests

`verify:` runs `go build ./...` and `go vet ./...` because this batch is comment-only: the risk it carries is a malformed comment block or an accidentally-truncated declaration, which is exactly what build and vet catch.
The scoped `go test` covers `./internal/websterengine/...` and `./internal/webstercli/...`, the two packages whose test files this batch edits, plus `./internal/pattern/...`, whose `leaf_enforcement_test.go` parses the feature-package list this batch shortens.
The integration-tagged run over the same two webster packages is required, not optional: three of the test files this batch edits — `internal/websterengine/beginbatch_test.go`, `internal/websterengine/recoverbatch_test.go`, and `internal/webstercli/verbs_test.go` — carry the `integration` build tag, so an untagged run never compiles the comments this batch rewrites in them.
No new tests are written;
the sweep's real completion criterion is the repo-wide zero-hit grep at batch 5.
