# Batch: weft-pathspec-tolerance

```yaml
task: 'PATTERN wiring: conditional constraint-injection into every agent'
batch: weft-pathspec-tolerance
number: 4
cards: 2
verify: go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/... ./cmd/lyx/...
depends-on: [3]
```

## Batch Scope

This batch widens fabric's default weft pathspec to `_lyx _pattern` so PATTERN content is actually committed and pushed — and, in the card immediately before it, makes `CommitWeft` tolerate a pathspec entry that currently matches nothing. The two must land in this order and in this batch, because the widened default alone is a silent, repo-wide breakage: `git add -- _lyx _pattern` fails **in its entirety** when `_pattern` matches nothing, and `CommitWeft` deliberately swallows git's `did not match any files` into `("", false, nil)` with **no error**. So the moment the default widens, every weft commit in a worktree without PATTERN content stops happening, silently, taking `_lyx` down with it.

Materialisation from batch 3 does not rescue this and must not be mistaken for a fix: git tracks files, not directories, so a materialised-but-empty `weft/_pattern/` still has nothing for a pathspec to match — and an empty `_pattern/` is the **normal, expected state for this entire task**, since content migration is explicitly out of scope. The widened pathspec and the tolerance land together or neither lands.

The filter goes in `internal/fabricengine/weftgit.go` and deliberately **not** in `internal/gitrepo`, whose untouched state is what keeps this task parallel-safe with the concurrent `native-clients` task.

## Cards

### Card 13: filter non-matching entries out of `CommitWeft`'s pathspec

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/buildercli/weft.go`
  - `internal/webstercli/weft.go`
  - `internal/perchcli/run.go`
  - `internal/initengine/undo.go`
  - `internal/fabricengine/testmain_test.go`
- **Edits:**
  - `internal/fabricengine/weftgit.go`
- **Creates:**
  - `internal/fabricengine/weftgit_pathspec_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add an unexported pathspec filter to `internal/fabricengine/weftgit.go` and call it from `CommitWeft` immediately before `f.Weft.StageAndCommit`, inside the already-held weft write lock. Do not touch `internal/gitrepo`. **The predicate, exactly:** an entry is **kept** if either (a) it begins with `:` — a git pathspec-magic entry, always passed through untouched and **never evaluated for matches** — or (b) it matches at least one path in the **worktree or the index**. Each clause exists because a looser reading breaks a real caller, and the implementer must not simplify any of them away. *Untracked must count*: a brand-new `_pattern/PATTERN.md` is untracked at the moment of its first commit, so a `git ls-files`-tracked-only predicate would filter `_pattern` out and drop the very first PATTERN commit — the one case this whole decision exists to enable. *Index-only must count*: `internal/initengine/undo.go` commits a `_lyx` path that `os.RemoveAll` has just deleted from the worktree, surviving only in the index, so a worktree-existence predicate would silently break `lyx init --undo`'s deletion commit. *Exclusion magic must pass through*: `CommitWeft`'s pathspec carries `:(exclude)` entries from `internal/buildercli/weft.go`, `internal/webstercli/weft.go` and `internal/perchcli/run.go`; these never match in the ordinary sense, and filtering one out re-stages machine-local artifacts against the Weft Git Invariant's Cross-module exclusions rule. **Evaluate the match with `git ls-files --cached --others -- <entry>`**, which covers index and untracked-in-worktree in one call, with cwd set to the weft worktree (`f.weftPath`) and entries interpreted relative to the weft root — the same anchor `StageAndCommit`'s own `git add` uses, so the filter can never disagree with the command it is filtering for. **If no positive entry survives, `CommitWeft` returns `("", false, nil)` without calling `StageAndCommit` at all.** This is not tidiness: `buildercli`, `webstercli` and `perchcli` each pass one positive entry plus several `:(exclude)` entries, so in a worktree where the positive entry matches nothing the filter would otherwise hand git a non-empty, **all-negative** pathspec — and git reads exclusions with no positive pathspec as "everything except those", staging the **entire weft worktree**. The early return preserves the no-op semantics `CommitWeft` already promises for "nothing of ours to stage". Update `CommitWeft`'s godoc to describe the filter, the three predicate clauses and the all-negative early return. Create `internal/fabricengine/weftgit_pathspec_integration_test.go` with `//go:build integration` first, covering one case per predicate clause against real git: an untracked new file under a second pathspec entry counts as a match and is committed; an index-only path (in the index, deleted from the worktree) counts as a match and its deletion still commits; a pathspec carrying `:(exclude)` entries passes them through untouched and the excluded artifacts stay unstaged — **this case is mandatory**, being the only guard against a filter that behaves on plain paths yet silently re-stages machine-local files; and a pathspec whose only positive entry matches nothing returns `("", false, nil)` having staged nothing at all, rather than staging the whole worktree.
- **Commit:** `fabricengine: skip pathspec entries that match nothing when committing weft`

### Card 14: widen the default weft pathspec to `_lyx _pattern`

- **Context:**
  - `internal/fabricengine/weftgit.go`
  - `internal/fabricengine/config.go`
  - `internal/fabricengine/weftgit_pathspec_integration_test.go`
- **Edits:**
  - `internal/fabricengine/template.yaml`
  - `internal/fabricengine/template_test.go`
  - `internal/fabriccli/weft_verbs.go`
  - `internal/fabricengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change `internal/fabricengine/template.yaml`'s `pathspec:` default from `_lyx` to `_lyx _pattern`, and correct its inline comment, which currently ends `_lyx is the default`. This is the single place the default weft-staging pathspec is declared; without it nothing ever stages `_pattern`, so a `PATTERN.md` written through the junction would never be committed or pushed and would never reach another machine or another worktree's weft pull — the mechanism would be inert in exactly the way that matters. Extend `internal/fabricengine/template_test.go` to assert the default value parses into **two** whitespace-separated paths: the value is whitespace-split, so a splitting bug here is silent and would simply drop `_pattern`. Update `internal/fabriccli/weft_verbs.go`, whose `Long` for `lyx fabric commit`/`push`/`sync` reads `Staging is scoped to the directories listed in the fabric config (default: _lyx)` — help accuracy is a review-blocking obligation under the CLI/Cobra Invariant. Then document two consequences in `internal/fabricengine/doc.go`, both confirmed rather than speculative. First, **existing worktrees never pick this up**: `configsync.ReconcileAll` → `yamlengine.Reconcile` keeps a `pathspec:` key that is already present and adds no key when one exists, so every already-initialised worktree stays on `pathspec: _lyx` forever and never persists PATTERN content, no matter how many times `lyx init` is re-run — operators must widen it by hand. Second, **no detection or warning surface is in scope**: nothing, not `lyx fabric status` and not `lyx init`, reports a narrow pathspec, so an existing worktree stays silently inert. That is accepted here rather than papered over — a "your pathspec predates PATTERN" warning means a new diagnostic class in `fabric status`, and PATTERN has no content to persist in this repo yet — and documenting it is what makes the next operator meet it in writing rather than by surprise. Note the deliberate asymmetry with the junction side, which batch 5 documents: junctions self-heal on the next `init`/`reconcile` and report loudly until they do, because `WireJunctions` owns junction state outright, whereas `pathspec` is an operator-editable config value `configsync` must not overwrite. Write both markdown-facing prose edits as one unwrapped line per paragraph.
- **Commit:** `fabricengine: add _pattern to the default weft pathspec`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine/... ./internal/fabriccli/... ./cmd/lyx/...` covers the new `internal/fabricengine/weftgit_pathspec_integration_test.go` (integration-tagged, so `-tags integration` is required for it to compile at all), the existing `internal/fabricengine/template_test.go`, and `internal/fabriccli`'s own suite for the `Long`-string change. The single most important assertion in this batch is the one that proves the two cards belong together: with the widened default and **no files under `_pattern`** — tested in both shapes, an existing-but-empty directory and a wholly absent one — a `_lyx` change still commits. Without that assertion the failure is invisible, because `CommitWeft` returns no error either way; only asserting that the commit actually happened catches it. One premise behind that test is asserted rather than verified in the discussion and must be settled empirically here rather than pinned from prose: whether `git add` dies for an *existing-but-empty* directory entry as opposed to only for a wholly absent one depends on git's unmatched-pathspec guard. Derive the expected value from what real git does in these two tests. The filter is required for the wholly-absent case regardless, so the decision does not depend on the answer — only the regression test's expected value does. `./cmd/lyx/...` is in scope for the same reason as batch 3: the new integration-tagged file carries tokens `cmd/lyx/tierpurity_test.go` matches as raw substrings, and `cmd/lyx/hermeticenv_test.go` scans it regardless of build tag.
