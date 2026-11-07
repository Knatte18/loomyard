<!-- This is the orch role's on-demand file for intake, rendered beside the role by RenderRoleFile (internal/orchengine/prompt.go). It carries no markers. -->
# Intake

`lym board intake` takes in the issues of this hub's own repository, the one its recorded binding names or else its origin remote.
It never takes in selfreport's issues, unless that repository is this hub's own.

When the operator asks:

- Run `lym board intake list` and read only its numbers and titles;
  never run it with `--full`, and never run `lym board intake import` yourself.
- An empty list ends the intake.
- An error envelope is neither empty nor non-empty:
  start no fork, and handle the error as a lym bug under the role's Where a finding goes.
- A non-empty list goes to one one-shot fork;
  when you know an intake fork is open, you start no second one.
  A fork lost to a context cycle leaves its issues open, and you tell the operator, who may ask for intake again.
  A second fork beside the first is harmless, since the CLI settles each issue once:
  a second import prints `recorded`;
  a close of an imported issue is refused as closed;
  an import of a closed issue is refused.
- A `failed` line in the fork's report is handled as a lym bug under Where a finding goes;
  the handling names the issue and, for a failed close, the `close` payload.

The fork follows `ly:board` for the commands and is bound to these outcomes:

- It reads the full list with `--full` and takes each issue to one of three outcomes:
  a new note, a fold or a close as noise.
- A new note carries exactly one type label and the label of every module the issue touches.
- A fold goes into an existing entry only when the issue is essentially the same finding or a correction to its subject.
  The board policy decides which, and the fold adds any module label the issue brings.
- A close as noise is only for an issue that has no actionable content or adds nothing to a recorded duplicate.
  Its reason names which.
- A duplicate that adds anything is folded;
  a fold the board refuses because the entry is run-held becomes a new note.
- When an import writes the board but its close fails, it re-runs the error's `close` payload once.
  A second pointer comment from the re-run is accepted.
  A second failure gives the issue a `failed` line with the error and the `close` payload.
- It writes the board only through `lym board intake` and a `lym board upsert`.
  That upsert writes only the labels of an entry the fork just folded into, keeping every label the entry carries.
- It edits no file, sends no message, and merges, promotes, starts or re-scopes nothing.
- It returns one line per issue:
  the number, then the new slug, the entry folded into, `closed` or `failed`, then the labels, close reason or error.
