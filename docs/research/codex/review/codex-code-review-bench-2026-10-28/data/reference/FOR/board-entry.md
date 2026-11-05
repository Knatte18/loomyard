# Board entry `fixer-overcap-review`: A merge fixer change over the caps gets a solo review of its own commits, not a full Code review

## Seen

On 2026-10-28 `creel-hook-post`'s Finalize merged the moved `main` in place, and the landing gate failed on a race between parallel `fabriccli` tests that repoint the process-wide trace sink.
The merge fixer added a `sync.RWMutex` across three test files, ten added lines and none deleted, and the gate passed.
`FixCap.Exceeded` in `internal/landingshed/fixcap.go` judges any change that touches a test file over the caps, whatever its size, so Finalize rerouted the run to Parent-Align and a full Code review segment: a `solid` lens fan reviewed the whole task again for a ten-line test lock.

## Idea (operator, 2026-10-28)

A fixer change over the caps goes to one solo reviewer who reads only the fixer's commits, the range the reroute reason already names, instead of the run's full Code review segment.
The caps stay as they are: what they guard against, a fixer weakening or skipping a test to pass the gate, is still checked by a reviewer before the landing.
A reviewer who finds the change wrong sends it back as today's reroute does.
