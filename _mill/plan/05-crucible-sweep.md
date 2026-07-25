# Batch: crucible-sweep

```yaml
task: dev/test lyx.exe separated from production deploy
batch: crucible-sweep
number: 5
cards: 2
verify: null
depends-on: [4]
```

## Batch Scope

Retargets every deploy instruction in the `crucible/` review prompts from the production deploy
to the dev flow. Two sub-cases: prompts whose reviewers **live-drive `lyx` by hand** (board,
webster) must, after `deploy-dev`, put the derived `.dev-bin` on the driving **session** PATH so
bare `lyx` exercises the dev build (prod stays untouched, off the default PATH per Q7); prompts
that drive **through the sandbox suite** (builder) or speak generically (review-template,
README, orchestrator) only swap `deploy.cmd` → `deploy-dev` (the suite resolves `.dev-bin`
itself). Pure prose edits — `verify: null`. Depends on batch 4 so `deploy-dev` exists.

`crucible/gitrepo-review-prompt.md` is intentionally untouched (it explicitly has "no deploy
step").

## Cards

### Card 17: Retarget manual-live-driving prompts (board, webster)

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `crucible/board-review-prompt.md`
  - `crucible/webster-review-prompt.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In both prompts, replace the production deploy command used "as the binary
  under test before live driving" with the dev deploy: `go run ./tools/deploy -dev` (Linux) —
  no `-dest`, no `$(dirname "$(which lyx)")` / `/home/knatte/.local/bin` target. In
  `board-review-prompt.md` this is the `go run ./tools/deploy -dest $(dirname "$(which lyx)")`
  instruction (~217–221) and the re-deploy note (~283); in `webster-review-prompt.md` it is the
  `go run ./tools/deploy -dest /home/knatte/.local/bin` instruction (~332–335) and the re-deploy
  note (~420). Because the dev binary lands in `.dev-bin` (deliberately NOT on the operator's
  default PATH), add an instruction that for the hands-on driving session the reviewer prepends
  it: `export PATH="$PWD/.dev-bin:$PATH"` (run from the repo root) so bare `lyx` resolves to the
  dev build; state that this keeps the production `lyx` untouched. Preserve each FOOTGUN warning
  but reword it to "re-run `go run ./tools/deploy -dev` after EVERY source change" (the driving
  still tests the deployed dev snapshot, not the working tree). Do not alter the environment-
  check lines that merely assert tmux/claude/go presence.
- **Commit:** `docs(crucible): retarget board+webster prompts to dev deploy`

### Card 18: Retarget suite-driven and generic prompts

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `crucible/builder-review-prompt.md`
  - `crucible/review-prompt-template.md`
  - `crucible/README.md`
  - `crucible/orchestrator-prompt.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace `deploy.cmd` with `deploy-dev.cmd` (Windows) / `deploy-dev` (POSIX)
  in each deploy instruction and its re-deploy/footgun restatement:
  `builder-review-prompt.md` (~353, 450), `review-prompt-template.md` (~123, 206),
  `README.md` (~191–193), and the "deploys the binary" reference in
  `orchestrator-prompt.md` (~34). For the suite-driven builder prompt, note that the sandbox
  suite fingerprints and runs the `.dev-bin` binary automatically (the operator only runs
  `deploy-dev` first; the fingerprint header's `Source: dev` confirms the dev build). Keep every
  FOOTGUN warning intact, reworded to point at the dev deploy. Do not change the black-box
  "exactly as a real user with only the binary on PATH" contract language.
- **Commit:** `docs(crucible): retarget builder/template/README/orchestrator to dev deploy`

## Batch Tests

`verify: null` — pure Markdown prose edits with no runnable surface. Correctness is by review:
every `crucible/` deploy instruction (except the deploy-less `gitrepo` prompt) targets the dev
flow, and the manual-driving prompts explain the session-PATH prepend so bare `lyx` exercises
the dev build.
