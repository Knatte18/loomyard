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

- It reads the full list with `--full` and takes each issue to one of four outcomes:
  a new note, a fold, a close as noise or a close as already fixed.
- A new note carries exactly one type label and the label of every module the issue touches.
- A fold goes into an existing entry only when the issue is essentially the same finding or a correction to its subject.
  The board policy decides which, and the fold adds any module label the issue brings.
- A close as noise is only for an issue that has no actionable content or adds nothing to a recorded duplicate.
  Its reason names which.
- A close as already fixed is only for an issue whose fault the code on the hub's main branch no longer has.
  The fork judges it from that branch and its history, read with git and never checked out or changed:
  the code path the issue names, and the commits that touch that path since the issue was filed or since the build it reports.
  It closes the issue with `"completed":true`,
  its reason naming the commit on the main branch that fixed the fault and what the fork saw on the path.
  When the fork cannot tell, or cannot name the fixing commit, the issue becomes a note or a fold like any other.
- A duplicate that adds anything is folded;
  a fold the board refuses because the entry is run-held becomes a new note.
- The full list carries each issue's comments, and the import carries them into the entry's body.
  A comment that is a finding apart from its issue also becomes a note of its own,
  written with `lym board upsert` and `"kind":"note"`, its body quoting the comment and naming the issue.
  The split happens only when the fork's own import of that issue wrote the board,
  which is a success or an error carrying a `close` payload;
  it is skipped when the import prints `recorded`.
- When an import writes the board but its close fails, it re-runs the error's `close` payload once.
  A second pointer comment from the re-run is accepted.
  A second failure gives the issue a `failed` line with the error and the `close` payload.
- It writes the board only through `lym board intake` and a `lym board upsert`.
  An upsert writes only a note split from a comment,
  or the labels of an entry the fork just folded into, keeping every label the entry carries.
- It edits no file, sends no message, and merges, promotes, starts or re-scopes nothing.
- It returns one line per issue:
  the number, then the new slug, the entry folded into, `closed` or `failed`, then the labels, close reason or error,
  then the slug of each note split from a comment.
