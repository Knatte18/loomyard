# Batch: templates describe one repo

```yaml
task: 'fabric: close the weft-visibility leak (slice 8)'
batch: 'templates describe one repo'
number: 5
cards: 3
verify: go test -tags integration ./internal/websterengine/ ./internal/builderengine/ ./internal/burlerengine/
depends-on: [3]
```

## Batch Scope

Rewrites all seven `go:embed`-ed agent prompt templates so an agent is never told warp, weft, or a "host repo" exist (decision `templates-describe-one-repo`) — 30 occurrences, every line pinned in the discussion's table, none left to implementer judgment.
The governing replacement is the positive rule: *"`_lyx` holds plan and state files — read and write them as ordinary files through `_lyx/...` paths.
You never run git against `_lyx`;
it is committed for you."*
Three occurrences are section headings that pinned template tests assert on, so each heading change is also a test change.
Depends on batch 03 so `"fabric-reference"` wording in `master-template.md:136` matches the already-renamed violation-class value.
Also carries `burlerengine`'s prose (`doc.go`, `prompt.go`) because burlerengine owns one of the templates and its pinned test — it must not be split into the comment sweep.

## Cards

### Card 17: websterengine templates

- **Context:**
  - `internal/websterengine/render.go`
  - `internal/websterengine/audit.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/websterengine/master-template.md`
  - `internal/websterengine/implementer-body.md`
  - `internal/websterengine/fork-prefix.md`
  - `internal/websterengine/integration-template.md`
  - `internal/websterengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Apply the discussion table's websterengine rows verbatim — `master-template.md:20` ("you never run git against the weft" → "you never run git"), `:29-30` (the link/sibling explanation replaced by the positive `_lyx` rule, the `:30` window sentence folded in and deleted so no pronoun dangles), `:136` ("a weft-reference" → "a fabric-reference"), `:140` heading ("## A weft-sync error…" → "## A fabric-sync error ends your run as stuck"), `:142-143` ("weft sync" → "fabric sync", "quoting the weft-sync failure" → "quoting the fabric-sync failure"), `:148-149` ("NEVER run any git command against the weft…" → "NEVER run any git command against `_lyx`, and never reference `_lyx` by any path other than `_lyx/...`. Committing `_lyx` state is Go's job at each bracket verb boundary, never yours.");
  `implementer-body.md:31` ("Commit the card to the HOST repo … never the weft" → "Commit the card to the repo — normal dev git, run from `{{.worktree_root}}` — never any `_lyx` path");
  `fork-prefix.md:21` ("on the HOST repo" → "in your worktree");
  `integration-template.md:21` ("on the HOST repo at `{{.worktree_root}}`" → "at `{{.worktree_root}}`").
  After the rewrite, grep each of the four files for `weft`, `warp`, and fabric-sense `host` phrases — zero hits allowed.
  Update `template_test.go`'s five pinned literals (`:246,257,259,318,412`) to pin the NEW wording (including the new positive `_lyx` rule and the `## A fabric-sync error` heading) — the pins are what prove the rewrite landed in the embedded files, so they must assert the replacement text, not merely drop the old assertions.
- **Commit:** `refactor(websterengine): templates describe one repo`

### Card 18: builderengine templates

- **Context:**
  - `internal/builderengine/template.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/builderengine/implementer-template.md`
  - `internal/builderengine/orchestrator-template.md`
  - `internal/builderengine/template_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Apply the discussion table's builderengine rows verbatim — `orchestrator-template.md:11` ("you never run git against the weft" → "you never run git"), `:88` ("against the weft or any `_lyx` path" → "against any `_lyx` path");
  `implementer-template.md:23` ("the host repo checkout for this task" → "the repo checkout for this task"), `:37` ("Commit the card to the HOST repo — never the weft repo, never any `_lyx` path." → "Commit the card to the repo — never any `_lyx` path."), `:64` heading ("## Never touch the weft" → "## Never touch `_lyx`"), `:66` ("against the weft repo or any `_lyx` path" → "against any `_lyx` path"), `:67` ("you DO commit your own code to the HOST repo" → "you DO commit your own code to the repo").
  Zero-hit grep for `weft`/`warp`/host-phrases in both files afterwards.
  Update `template_test.go`'s pinned literals (including the `## Never touch` heading pin) to assert the new wording.
- **Commit:** `refactor(builderengine): templates describe one repo`

### Card 19: burlerengine template and prose

- **Context:**
  - `internal/burlerengine/template.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/burlerengine/instruction-3-fix-template.md`
  - `internal/burlerengine/template_test.go`
  - `internal/burlerengine/doc.go`
  - `internal/burlerengine/prompt.go`
  - `internal/burlerengine/profile.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Apply the discussion table's burlerengine rows verbatim — `instruction-3-fix-template.md:2` ("the never-push/never-touch-weft rule" → "the never-push/never-touch-`_lyx` rule"), `:26` heading ("## Never push, never touch the weft" → "## Never push, never touch `_lyx`"), `:29` ("against a `_lyx` or weft path" → "against a `_lyx` path"), `:30` ("commit-per-fix on the host repo, stay inside the host working tree" → "commit-per-fix on the repo, stay inside the working tree"), `:31` ("nothing here ever authorizes a weft commit" → "nothing here ever authorizes an `_lyx` commit").
  Update `template_test.go`'s pinned heading/literal assertions to the new wording.
  Reword `doc.go`'s ~12 weft/warp/host-phrase mentions (`:87-88`'s "host repo's own files"/"host working tree", `:111`'s "ordinary host-repo commit, not a weft…") and `prompt.go:124`'s rendered string "Write surface: the host working tree in this task worktree…" → "Write surface: the working tree in this task worktree…" — that string reaches an agent prompt, so it is a leak, not just a comment.
  `profile.go:29`'s trailing comment on `FixScopeSource FixScope = "source"` reads "// host repo, with git commits" and rewords to "// the repo, with git commits" — the `FixScope` *value* `"source"` is unchanged;
  this file belongs to no other card and would otherwise fail batch 07's rule (2) on first activation.
  Zero-hit grep afterwards across the five files.
- **Commit:** `refactor(burlerengine): one-repo template wording and prose`

## Batch Tests

`verify:` runs `go test -tags integration ./internal/websterengine/ ./internal/builderengine/ ./internal/burlerengine/` — the three template test suites re-pin the rewritten wording (builderengine's template test is integration-tagged, hence the tag).
The per-card zero-hit greps are the interim guard until batch 07's `TestEnforcement_FabricVocabulary` polices the embedded `.md` files permanently.
