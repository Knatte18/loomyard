## Cluster rules

This is a cluster round. The fork reviewers have already run, one per lens listed here, and each returned its findings as its final message.

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

The forks' final messages are these files, which you read once your reading in step 1 is done:

- `/home/knatte/Code/loomyard-LYMHUB/codex-astra-fan-bench/.scratch/bench/inputs/P4/work/.bench/lens/affected-set.md`: the final message of the `affected-set` fork
- `/home/knatte/Code/loomyard-LYMHUB/codex-astra-fan-bench/.scratch/bench/inputs/P4/work/.bench/lens/guard-reach.md`: the final message of the `guard-reach` fork
- `/home/knatte/Code/loomyard-LYMHUB/codex-astra-fan-bench/.scratch/bench/inputs/P4/work/.bench/lens/way-forward.md`: the final message of the `way-forward` fork

You do not spawn any fork: the results are in those files.

Do your own HOLISTIC review: what no lens owns, such as the target against its fasit and its board entry, the rubric's leanness and relocation rules, architecture and PATTERN-fit, reading further only where your review needs it.
Then prepare the ground truths and the severity rubric you will judge every finding against.
Write nothing to the review file until you have read all three fork reports and finished your own holistic review: consolidate with every finding in view.

Consolidation: judge every fork finding and your own holistic findings with EQUAL skepticism, dedup findings that describe the same defect across lenses, tag every kept finding's frontmatter with an `origin:` key (`lens:<name>` or `handler`), move false positives to a `## Rejected` prose section below the frontmatter with a one-line reason each (a rejected item never appears in `findings:`), and order kept findings by severity.
The consolidated review is the ONE review file, and the review is complete once it is fully written to disk — consolidation is part of the review, not a separate step after it.

## The review file

Write the review to `/home/knatte/Code/loomyard-LYMHUB/codex-astra-fan-bench/.scratch/bench/inputs/P4/work/.bench/out/review.md`, in the format `/home/knatte/Code/loomyard-LYMHUB/codex-astra-fan-bench/.scratch/bench/inputs/P4/work/.bench/instr/review-format.md` describes.
Read that file before you write.
The review is complete once it is fully written to disk and passes the format's strict rules.

## Source-grounding rule

Never fabricate file contents.
Read the actual files before you describe or judge them — every claim in your review must be grounded in something you actually read.

## Prior rounds

This is the first round — no prior round files exist.
