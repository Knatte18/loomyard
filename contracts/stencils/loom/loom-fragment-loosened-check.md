<!-- This is the Loosened-Check commit trailer rule every session that commits code follows.
     Every brief of a session that commits code includes this fragment by name, with no pipeline.
     It declares no marker and no engine block. -->

## Loosened checks

A commit that widens or removes an assertion, tolerance or limit in a test that exists before your change records each one as a trailer line in the last paragraph of its commit message:

`Loosened-Check: <test> — <what was loosened> — <measured cause>`

An assertion that moves to a test in another file, on the same behaviour and no looser, is not removed and needs no line.
Write one line per check.
The cause is a measurement, such as the observed value that the old limit rejected, never "flaky" or "to make it pass".
A session that forgot a line adds it in a later commit, with no change if need be, and never rewrites history.
Code-Review and Drift read these lines.
