# Batch: wiki-publish-and-summary

```yaml
task: "Scope the Shed producer-model rewrite into buildable tasks"
batch: "wiki-publish-and-summary"
number: 3
cards: 2
verify: test -f /home/knatte/Code/loomyard/wiki/proposal-builder-retire.md && test -f /home/knatte/Code/loomyard/wiki/proposal-plan-format-drop-v3-suffix.md && test -f /home/knatte/Code/loomyard/wiki/proposal-format-docs-name-producers.md && test -f /home/knatte/Code/loomyard/wiki/proposal-raddle-finalize-fold-and-link-repair.md && test -f /home/knatte/Code/loomyard/wiki/proposal-shed-model-contradiction-sweep.md && test -f /home/knatte/Code/loomyard/wiki/proposal-batcher-standalone-split.md
depends-on: [1, 2]
```

## Batch Scope

This batch turns the six staged files into wiki tasks and writes the rationale doc the task's Scope section names.
It is one batch and it runs last because both cards depend on all six staged files existing: the publish step reads them, and the summary doc records what was published.
It delivers the task's two externally visible outputs — six entries in the wiki backlog with their `depends_on` wired, and `_mill/followup-tasks.md` committed on this branch.
No batch-local decisions differ from the overview.

## Cards

### Card 7: Publish the six tasks to the wiki in one batched upsert

- **Context:**
  - `_mill/followup/A-builder-retire.md`
  - `_mill/followup/B-plan-format-drop-v3-suffix.md`
  - `_mill/followup/C-format-docs-name-producers.md`
  - `_mill/followup/D-raddle-finalize-fold-and-link-repair.md`
  - `_mill/followup/E-shed-model-contradiction-sweep.md`
  - `_mill/followup/F-batcher-standalone-split.md`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Publish the six staged tasks to the wiki with exactly one call to `wiki._client.upsert_tasks_batch`.
  This card writes no file in this repository and makes no commit;
  its whole effect is on the wiki, which the mill wiki daemon owns.
  Never touch the wiki repository with `git`, `Edit`, `Write`, or `cp` — CLAUDE.md's `## Mill wiki — never touched directly` rule makes the daemon client the only sanctioned path, and the daemon serializes every write.
  Run the publish as a single throwaway Python invocation from the worktree root, using the mill interpreter and script path already exported in the environment:
  ```
  PYTHONPATH="$CLAUDE_PLUGIN_ROOT/scripts" "$MILL_PYTHON" <script>
  ```
  Both `CLAUDE_PLUGIN_ROOT` and `MILL_PYTHON` are exported by the mill plugin;
  if either is unset, stop and report it rather than guessing at a hard-coded plugin-cache path, which carries a version number that changes under you.
  The script must, in order:
  parse each of the six staged files by taking the first fenced yaml block as the header and everything after that block's closing fence as the body;
  `yaml.safe_load` each header and assert it carries exactly the four keys `slug`, `title`, `brief`, `depends_on`, failing loudly with the offending filename if a key is missing, an extra key is present, or the body is empty;
  assert the six parsed slugs are exactly the set `builder-retire`, `plan-format-drop-v3-suffix`, `format-docs-name-producers`, `raddle-finalize-fold-and-link-repair`, `shed-model-contradiction-sweep`, `batcher-standalone-split`;
  assert every slug named in any `depends_on` list is itself one of those six;
  then call `upsert_tasks_batch` once with all six task dicts and a `message` naming this task's slug.
  Resolve the wiki path with `_paths.resolve_wiki_path(_paths.resolve_git_root())` rather than hard-coding it.
  Pass `slug`, `title`, `brief`, `body`, and `depends_on` for each task and nothing else — leave `status`, `isolated`, and `deferred` unset so the six land as ordinary unclaimed backlog entries.
  Use the batched call rather than six `upsert_task` calls: `Store.upsert_tasks_batch` validates against a projected snapshot containing all six incoming tasks at once, so a task may name a sibling in `depends_on` regardless of ordering, whereas six separate calls would fail the dangling-dependency check on the first task that names a not-yet-created sibling.
  After the call returns, verify the result with `wiki._client.list_tasks_brief` and confirm all six slugs are present with the `depends_on` values the staged headers declared — `builder-retire` with none, `plan-format-drop-v3-suffix` and `raddle-finalize-fold-and-link-repair` on `builder-retire`, `format-docs-name-producers` and `batcher-standalone-split` on `plan-format-drop-v3-suffix`, and `shed-model-contradiction-sweep` on both `format-docs-name-producers` and `batcher-standalone-split`.
  Report the verification output;
  a mismatch is a hard failure of this card, not something to patch by a second upsert with different values.
  The operation is idempotent by slug, so re-running the script after a fix is safe and is the intended recovery path.
- **Commit:** none

### Card 8: Write the follow-up task summary doc

- **Context:**
  - `_mill/discussion.md`
  - `_mill/followup/A-builder-retire.md`
  - `_mill/followup/B-plan-format-drop-v3-suffix.md`
  - `_mill/followup/C-format-docs-name-producers.md`
  - `_mill/followup/D-raddle-finalize-fold-and-link-repair.md`
  - `_mill/followup/E-shed-model-contradiction-sweep.md`
  - `_mill/followup/F-batcher-standalone-split.md`
- **Edits:** none
- **Creates:**
  - `_mill/followup-tasks.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write the rationale doc the task's Scope section names.
  It opens with a one-paragraph statement of what this scoping task concluded: shed.md, loom.md, and roadmap.md were already rewritten for the flat producer model in the two commits immediately before this worktree spawned, so the remaining work is residue and contradictions rather than three full rewrites, and those three files are the decided state everything else reconciles to.
  Then a table of the six tasks with columns ID, slug, kind, and `depends_on`, matching the table in `_mill/discussion.md`'s Decision `follow-up-task-set` — A `builder-retire` (code, no deps), B `plan-format-drop-v3-suffix` (code, mechanical; A), C `format-docs-name-producers` (docs; B), D `raddle-finalize-fold-and-link-repair` (docs; A), E `shed-model-contradiction-sweep` (docs; C and F), F `batcher-standalone-split` (code and docs; B).
  State the chain in prose: A → B → {C, F} → E, with D branching off A in parallel.
  Then one short paragraph per task saying what it delivers — a summary, not a second copy of the body;
  point at the staged file under `_mill/followup/` and at the wiki page `proposal-<slug>.md` for the full text.
  Then a section on why the ordering is what it is, carrying the reasoning from the `follow-up-task-set` decision: the set is split by file-cluster, with `depends_on` wired wherever two tasks edit the same file and left parallel only where the file sets are genuinely disjoint.
  Explain why E is serialized last rather than parallel — its original parallel scoping rested on a false disjointness claim, since E edits loom.md:15–17 and :75 while C scoped-edits the table rows, and docs/overview.md is edited by A, B and E alike;
  doc edits are cheap, so the parallelism E bought is worth less than the conflict risk, and sequencing it last lets its contradiction sweep read C's finished table rather than guess at it.
  Note that loom.md:75 holds both open questions — the Discussion pre-gate owned by C's decision and the thin-Output carve-out owned by E's — so a single owner beats splitting one line between two tasks.
  Explain why D stays parallel: it owns finalize.md, raddle.md, and self-report.md, and no other task touches any of them.
  Include the loom.md three-owner note — B then C then E, in chain order, never concurrently — with each owner's scope in one line.
  Close with a section recording the four surfaced open questions from `_mill/discussion.md`'s `## Surfaced open questions`, one line each, naming the owner of each: the Webster producer-atomicity tension (E, as a named precondition on the roadmap's Planned Shed item), the thin-Input case for Discussion-Write (E, in shed.md's contract section, no roadmap gate), the overloaded `shed` name (E, as docs/overview.md's last owner), and the deferred Hardener/Tenter Raddle-into-Finalize fold (deferred by the landed design, recorded so a future pass does not read the silence as an oversight).
  Add a closing note that the deferred phase-enum realignment lands with the Shed build task and is deliberately untouched by all six.
  State at the top of the file that it is task state, not part of the durable doc set — it describes work to be done, and it has no place in manifest/ or docs/ once the six tasks land.
- **Commit:** `scoping: record the six follow-up tasks and their ordering`

## Batch Tests

The batch `verify:` asserts that all six `proposal-<slug>.md` pages exist in the wiki clone at `/home/knatte/Code/loomyard/wiki`.
That is the observable proof the batched upsert reached the wiki repository: the daemon renders each task's `body` into `proposal-<slug>.md` and commits it, so a missing file means the publish did not land, whatever the script printed.
The `depends_on` wiring is not covered by this command — card 7 verifies it in-process with `wiki._client.list_tasks_brief` immediately after the upsert, which is the only place the values are readable without re-parsing the daemon's own store.
Card 8 has no test surface;
it writes one markdown file.
