## How to read for this review

- Check every claim a card makes about existing code by reading that code and its callers, and read the code paths the change reaches beyond the cards.
- For every symbol, type, config key, file or command a card changes, renames or removes, search the whole tree for every reader of it, with `grep -rn` or `lym quarry`, list the readers, and check that a card covers each one; a reader no card covers is a finding.
- For every function a card changes, read the functions it calls and the code that calls it, one hop beyond the cards, and check that the change still holds there.
- for each guard, refusal or seam a card adds: the callers of the guarded function, the order in which the root constructs its commands and runs its pre-run, and the definition of every environment switch that turns the guarded step off;
- Where several cards change the same file or limit, check what each card's gate sees given the cards before it.
- Read the `pattern/` background file of every PATTERN entry the plan touches, and check that each new command, package or file the plan adds appears where that entry requires.
- Read the tests the plan adds or changes for waits, sleeps and timing that would slow or flake the suite.
- Put several searches into one command where you can.
