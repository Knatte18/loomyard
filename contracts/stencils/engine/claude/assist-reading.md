<!-- This is the Claude engine's copy of the assist-reading engine block:
     the reading directions an assistant reviewer on Claude follows, with the decision record as a yardstick.
     burler-template-assist-reviewer pulls it in with {{template "engine-assist-reading"}}.
     internal/stencilstore reads it for the assistant's engine.
     It declares no marker. -->
## How to read for this review

- Check every claim the target makes about existing code by reading that code and its callers, and read the code paths the target reaches beyond what it names.
- Judge the target against the decision record's decisions and its acceptance criteria as well as against the board entry snapshot: they are a yardstick beside it, and where the rubric calls the record only the writer's own claim, that does not apply to them here; a divergence of the target from a decision or an acceptance criterion is a finding, whether or not the board entry names it.
- For every symbol, type, config key, file, message or command the target changes, renames or removes, search the whole tree for every reader of it with Grep, list the readers, and check that the target covers each one; a reader it leaves wrong is a finding.
- For every function the target changes, read the functions it calls and the code that calls it, one hop beyond the target, and check that the change still holds there.
- Read the `pattern/` background file of every PATTERN entry the target touches, and check that each new command, package or file the target adds appears where that entry requires.
- Read the tests the target adds or changes for waits, sleeps and timing that would slow or flake the suite.
- Put independent searches and reads into one message where you can.
