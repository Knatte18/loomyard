<!-- This is the job body of the cold fixer for a Webster card gate's red `lym gate test --gate card` call.
     internal/gatefixer's RenderPrompt joins it behind the shared cold frame gatefix-frame-cold and fills it via internal/stencil.
     Its markers are required top-level {{.X}} substitutions ({{.worktree_root}}, {{.card_ids}}, {{.gate_call}}),
     and stencil.FillOptional requires each to be non-empty.
     There are no {{if}}/{{range}} conditionals in this file. -->

## The failure

An implementer fork's card gate call failed, and the report names each failure.
The fork is implementing the card or cards `{{.card_ids}}` of the plan, and it has not committed them yet: their uncommitted change is in the worktree.
You fix the code the failures point at.

## The rules

Your session's working directory already is the worktree root, `{{.worktree_root}}`: run every Bash command from there, give the paths in Bash commands worktree-relative, and follow the gate rule in {{template "engine-standing-instructions"}} for `cd` and for quick checks.
{{template "engine-absolute-paths"}}
You never run a Bash call in the background.

- Read the card file or files under `_lym/plan/` that the ids name, and the worktree's uncommitted diff, before you change anything: the card says what the change is for, and the diff says what the fork has done so far.
- Read the source behind each failure before you change it, and fix every failure you can in one pass.
- Change source only, never a path under `_lym`.
- Never delete, skip or weaken a test to make it pass: no `t.Skip`, no loosened assertion, no removed case.
  A test that is wrong is fixed to assert what its behavior intends, never to assert less.
- Commit nothing: the fork commits the card, and your uncommitted change stays in the worktree for it to read.
- Leave every failure whose fix would add, remove or change a decision, an acceptance criterion, a PATTERN change, a card or a card's scope, or a test's intended assertion, or that needs a choice the plan or the card does not make.
  Name each one you leave in your reply, with why.
- Re-run only each check that failed, as a foreground Bash call under the gate rule in {{template "engine-standing-instructions"}}, and fix what it still reports.
  The report heads each failed run with its label, such as `tier0`, `vet tier1`, `build` or `lint comments`, and the label decides the re-run:
  - A failed C# test of a `tier<N>` run: `lym gate test --tier <N> <its test project> -- --filter 'FullyQualifiedName~<its name without arguments>'`.
    A filtered run passes even when it matched no test, so count the test as checked only when the re-run's summary line counts it as passed.
  - A failed Go test of a `tier<N>` or `guard tier<N>` run: `lym gate test --tier <N> <its package> -- -run '<pattern>'`.
    The pattern anchors each level of the test's path apart: split the reported name at each `/`, and anchor each part as its own `^<part>$`.
    `TestX/case` takes `-run '^TestX$/^case$'`, and a top-level test takes `-run '^TestX$'`.
    Escape each regular expression metacharacter in a part, and write each `'` as `.`.
    A filtered run passes even when it matched no test, and a parent test passes when none of its subtests matched.
    Read the log that the re-run's summary line names, and count the test as checked only when it holds a `--- PASS:` line with the test's full path.
  - A failure of a `tier<N>` run outside any test: `lym gate test --tier <N> <its package>`, with no `--` filter.
    A run that names no failing package is run the same way over the card's packages.
  - A failure of a `vet tier<N>` run: `go vet -tags tier<N> <the packages its errors name>`.
  - A `build` failure: `lym gate test --tier <N> <the package or test project its errors name>`, which runs the build first.
    The report's `build before tier<N>` line gives the `<N>`.
  - A `lint comments` failure: `lym loom lint-comments`; a `lint pattern` failure: `lym loom lint-pattern`.
  - A package of a nested module, one whose `go.mod` lies below the worktree root, is given relative to that module.
    Its call takes `-C <the module's worktree-relative directory>` right after `lym gate test` or `go vet`.
- Never run the whole card gate call `{{.gate_call}}`: it runs again over your tree after you end, and that run judges the fix.

{{template "loom-fragment-tooling-names"}}
