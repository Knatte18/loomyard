## Cluster rules

This is a cluster round: after exploring the target fully, spawn ALL of the fork reviewers below in a SINGLE message by calling your `spawn_agent` tool once per lens listed here, so the forks run in parallel, each with `fork_turns` set to `"all"` so it inherits your context; never pass a name, a model or an effort for a fork, so each fork keeps your own.

Lenses for this round:

- code-correctness: Own the correctness of the code the diff changes. For each function,
branch and error path the diff adds or changes, take one concrete input,
including an empty, nil, zero or boundary value and a failure of each call
it makes, and run the code step by step with the data each step is handed.
A wrong result, a swallowed or misreported error and a state left
inconsistent on a failure path are findings, as the rubric's "Finding
class" defines `design`. Your list: each function or branch, the case you
ran, correct or not.
- code-affected-set: Own what the diff's changes force outside the lines it touches. For each
signature, output, key, message, file shape or behaviour the diff changes,
find everything that calls, reads or asserts it, callers, tests, docs,
stencils, fixtures and the repository's rule documents that describe it,
the PATTERN.md entries and pattern/ background files where the repository
has a PATTERN.md, otherwise the rules its CLAUDE.md files state, from the
definitions your context holds and a search for what it lacks. A member of
the set the diff leaves wrong is a finding. Apply the rubric's "Stencil
wording", "The repository's own rules", "Declared PATTERN changes landed"
and "Commit messages and departures" where this round's rubric carries
them. Your list: per change, the set you found, covered or not.
- code-tests: Own the tests of the diff. For each behaviour the diff adds or changes,
find the test row that would fail if it were broken; a behaviour with no
such row is a finding, a stencil's static prose exempt. Your list: per
behaviour, the test that pins it, or none.

### Fork rules for this round

Spawn the forks as soon as step 1's reading manifest is written, in the same turn.
Each fork's prompt is the boilerplate below, then the line `Your lens: <name>` and that lens's text, and nothing else: never add your own direction, hint, suspicion or reading list to a fork's prompt.

Boilerplate:

> You are one fork reviewer of this round, and you own one lens; the other forks own the other lenses, and the handler reviews holistically. Your inherited context already holds the target, the rules and the code listed in the reading manifest; review from it. Reading for this review: Check every claim the board entry, the decision record and the change description make about existing code by reading that code and its callers, and read the code paths the diff reaches beyond the lines it changes. For every symbol, type, config key, file, message or command the diff changes, renames or removes, search the whole tree for every reader of it, with `grep -rn`, list the readers, and check that the diff covers each one; a reader the diff leaves wrong is a finding. For every function the diff changes, read the functions it calls and the code that calls it, one hop beyond the diff, and check that the change still holds there. Read the `pattern/` background file of every PATTERN entry the diff touches, and check that each new command, package or file the diff adds appears where that entry requires. Read the tests the diff adds or changes for waits, sleeps and timing that would slow or flake the suite. Put several searches into one command where you can. Work your lens's procedure, and start your final message with the list your lens asks for. Report every finding your lens's procedure produces and nothing outside it, because the other lenses and the handler cover the rest. You may make at most 20 tool calls, each only for something your procedure needs that the manifest lacks, such as a caller or a spawned process one hop beyond it, and never to re-read what your context already shows; end your final message with one line per tool call: what you read and why your context lacked it. You are READ-ONLY: never Write, Edit or delete any file, run only commands that read, such as `git diff`, `git log` or `git show`, and never one that changes the repository, its index, its refs or its working tree, never touch the two round output files, and never spawn another agent. Return your findings ONLY as your final message, each with a severity, a class, a location, a one-line summary and the file and line of the evidence you read.

Wait for every fork with your wait tool; while the forks run, YOU (the handler) do your own HOLISTIC review: what no lens owns, such as the target against its fasit and its board entry, the rubric's leanness and relocation rules, architecture and PATTERN-fit, reading further only where your review needs it.
Then prepare the ground truths and the severity rubric you will judge every finding against.
Write nothing to the review file while any fork is outstanding, whatever an early report says: consolidate only once the last fork has returned, with every finding in view.

Once the last fork has returned, and before you consolidate, save each fork's final message verbatim to `/home/knatte/Code/loomyard-LYMHUB/codex-code-review-bench/.scratch/bench/inputs/STB/work/.bench/out/review.lens-<lens>.md`, with its lens name for `<lens>`, in the format `/home/knatte/Code/loomyard-LYMHUB/codex-code-review-bench/.scratch/bench/inputs/STB/work/.bench/specs/twill-lens-report-spec.md` defines; a fork that returned no report gets its file too.
These lens reports are part of your write surface, beside the review file, and are the one exception to the "nothing else" and "no other file" of the write-surface rules in your prompt.

Consolidation: judge every fork finding and your own holistic findings with EQUAL skepticism, dedup findings that describe the same defect across lenses, tag every kept finding's frontmatter with an `origin:` key (`lens:<name>` or `handler`), move false positives to a `## Rejected` prose section below the frontmatter with a one-line reason each (a rejected item never appears in `findings:`), and order kept findings by severity.
The consolidated review is the ONE review file, and the review is complete once it is fully written to disk — consolidation is part of the review, not a separate step after it.

## The review file

Write the review to `/home/knatte/Code/loomyard-LYMHUB/codex-code-review-bench/.scratch/bench/inputs/STB/work/.bench/out/review.md`, in the format `/home/knatte/Code/loomyard-LYMHUB/codex-code-review-bench/.scratch/bench/inputs/STB/work/.bench/instr/review-format.md` describes.
Read that file before you write.
The review is complete once it is fully written to disk and passes the format's strict rules.

## Source-grounding rule

Never fabricate file contents.
Read the actual files before you describe or judge them — every claim in your review must be grounded in something you actually read.

## Prior rounds

This is the first round — no prior round files exist.
