<!-- This is the guard-scan-failure line of webster's card-gate list in a fork or recovery prompt.
     guardNotice (guardstep.go) reads it through stencilstore, fills its one marker, strips this banner and trailing newline,
     and appends the text to the card gate list when the scan for `//lyx:guard` tests failed on the content of a test file.
     guard_error is the scan error's text. -->

The guard scan failed with `{{.guard_error}}`, so no card gate above runs the repository's `//lyx:guard` tests.
The plan-level verify still runs them.
