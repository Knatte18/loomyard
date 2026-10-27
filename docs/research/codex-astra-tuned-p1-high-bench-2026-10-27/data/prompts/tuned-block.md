## How to read for this review

- Check every claim a card makes about existing code by reading that code and its callers, and read the code paths the change reaches beyond the cards.
- Read the `pattern/` background file of every PATTERN entry the plan touches, and check that each new command, package or file the plan adds appears where that entry requires.
- Read the tests the plan adds or changes for waits, sleeps and timing that would slow or flake the suite.
