## How to read for this review

- Check every claim the board entry, the decision record and the change description make about existing code by reading that code and its callers, and read the code paths the diff reaches beyond the lines it changes.
- For every symbol, type, config key, file, message or command the diff changes, renames or removes, search the whole tree for every reader of it, with `grep -rn`, list the readers, and check that the diff covers each one; a reader the diff leaves wrong is a finding.
- For every function the diff changes, read the functions it calls and the code that calls it, one hop beyond the diff, and check that the change still holds there.
- Read the `pattern/` background file of every PATTERN entry the diff touches, and check that each new command, package or file the diff adds appears where that entry requires.
- Read the tests the diff adds or changes for waits, sleeps and timing that would slow or flake the suite.
- Put several searches into one command where you can.
