<!-- This is burler's fork-reading part: the pre-read a fanned round's handler does in the explore step, so every fork inherits the reading instead of repeating it.
     internal/burlerengine's forkReadingBlock (prompt.go) reads and fills it, and composeReviewPrompt injects it as burler-step-1-explore's optional fork_reading marker value in a fanned round only.
     It declares no marker. -->
## Reading for the fork reviewers

This round fans out: in step 2 you spawn fork reviewers, and each fork inherits your context as it stands at the spawn.
Your reading in this step is the forks' reading, so read here what the review will need, once, instead of leaving each fork to read it again.

Before you leave this step, read:

- every file the target names, at the definitions of the symbols it cites;
- for each symbol the target changes, guards or removes: its direct callers and callees, and every path that reaches it from another process, such as a child the code spawns or a command that calls the guarded verb in-process;
- for each state, setting or file a decision introduces or changes: the existing code paths that also write, reset or reapply it, such as other verbs, hooks, resize or apply steps, and restart paths, and the code that reads it;
- for each output, key, string, file shape or behaviour the target changes: every test, caller, doc and fixture that reads or asserts it;
- the board entry of the task the target belongs to, through `lyx board get`;
- the `pattern/` background file of every PATTERN entry the target names or touches;
- the failure, restart and resume paths of each change the target makes.

Read definitions and call sites, not whole files: a fork re-reads its inherited context on every turn.
Judge nothing while you read: write no finding, verdict or assessment, not even in a status line or a note to yourself.
A judgement in your context before the spawn colours every fork; your own review starts after the forks are spawned.

Close this step with a reading manifest: one line per file and symbol you read, grouped by the part of the target it serves.
The manifest names what you read and nothing about what you found.
Write it as text in the same turn and go straight on to step 2: the manifest never ends your turn.
