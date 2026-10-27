## Cluster rules

This is a cluster round: after exploring the target fully, spawn ALL of the fork reviewers below in a SINGLE message by calling your `spawn_agent` tool once per lens listed here, so the forks run in parallel, each with `fork_turns` set to `"all"` so it inherits your context; never pass a name, a model or an effort for a fork, so each fork keeps your own.

Lenses for this round:

- affected-set: Own what each card's change forces outside the card. For each output, key,
string, file shape or behaviour a card changes, find everything that reads
or asserts it, tests, callers, docs and fixtures, from the pre-read and a
search for what the pre-read lacks. For each change, find the cards that
cover the set. A card that covers part of a set is one finding about the
method, naming the members it missed. Your list: per change, the set you
found and the cards that cover it.
- guard-reach: Own whether each guard can be reached. For each guard, refusal, branch or
test row a card adds, trace the sequence of states a real entry point
passes through to reach it. Say whether the state it guards can occur at
that point, and whether the guarded site recurs elsewhere on a path the
card does not guard. A guard whose state cannot occur, and a guarded site
that recurs unguarded, are findings. Your list: per guard, reachable or
not.
- way-forward: Own the failure and recovery ends. For each refusal, stop, reset or
recovery step a card adds, find the next step it names and check whether
that step can succeed from the state the refusal leaves. A refusal that
names no next step, and a next step that cannot succeed from that state,
are findings. Your list: per refusal, the next step, progresses or not.

### Fork rules for this round

Spawn the forks as soon as step 1's reading manifest is written, in the same turn.
Each fork's prompt is the boilerplate below, then the line `Your lens: <name>` and that lens's text, and nothing else: never add your own direction, hint, suspicion or reading list to a fork's prompt.

Boilerplate:

> You are one fork reviewer of this round, and you own one lens; the other forks own the other lenses, and the handler reviews holistically. Your inherited context already holds the target, the rules and the code listed in the reading manifest; review from it. Reading for this review: Check every claim a card makes about existing code by reading that code and its callers, and read the code paths the change reaches beyond the cards. Read the `pattern/` background file of every PATTERN entry the plan touches, and check that each new command, package or file the plan adds appears where that entry requires. Read the tests the plan adds or changes for waits, sleeps and timing that would slow or flake the suite. Work your lens's procedure, and start your final message with the list your lens asks for. Report every finding your lens's procedure produces and nothing outside it, because the other lenses and the handler cover the rest. You may make at most 6 tool calls, each only for something your procedure needs that the manifest lacks, such as a caller or a spawned process one hop beyond it, and never to re-read what your context already shows; end your final message with one line per tool call: what you read and why your context lacked it. You are READ-ONLY: never Write, Edit or delete any file, run only commands that read, never touch the two round output files, and never spawn another agent. Return your findings ONLY as your final message, each with a severity, a class, a location, a one-line summary and the file and line of the evidence you read.

While the forks run, YOU (the handler) do your own HOLISTIC review: what no lens owns, such as the target against its fasit and its board entry, the rubric's leanness and relocation rules, architecture and PATTERN-fit, reading further only where your review needs it.
Then prepare the ground truths and the severity rubric you will judge every finding against.
Write nothing to the review file while any fork is outstanding, whatever an early report says: consolidate only once the last fork has returned, with every finding in view.

Consolidation: judge every fork finding and your own holistic findings with EQUAL skepticism, dedup findings that describe the same defect across lenses, tag every kept finding's frontmatter with an `origin:` key (`lens:<name>` or `handler`), move false positives to a `## Rejected` prose section below the frontmatter with a one-line reason each (a rejected item never appears in `findings:`), and order kept findings by severity.
Wait for every fork with your wait tool before you consolidate.
The consolidated review is the ONE review file, and the review is complete once it is fully written to disk — consolidation is part of the review, not a separate step after it.

## The review file

Write the review to `/home/knatte/Code/loomyard-LYMHUB/codex-astra-tuned-p4-med-bench/.scratch/bench/inputs/P4/work/.bench/out/review.md`, in the format `/home/knatte/Code/loomyard-LYMHUB/codex-astra-tuned-p4-med-bench/.scratch/bench/inputs/P4/work/.bench/instr/review-format.md` describes.
Read that file before you write.
The review is complete once it is fully written to disk and passes the format's strict rules.

## Source-grounding rule

Never fabricate file contents.
Read the actual files before you describe or judge them — every claim in your review must be grounded in something you actually read.

## Prior rounds

This is the first round — no prior round files exist.
