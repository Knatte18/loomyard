<!-- This is the Hardener rubric.
     The Harden-Bouncer row and the Harden-Burler-Review row read it, in the hardener recipe.
     The Harden-Bouncer row interpolates it as the rubric marker value of harden-template-seed.md and bouncer-template-judge.md,
     and the Harden-Burler-Review row interpolates it the same way into internal/burlerengine's review prompt.
     It is a marker VALUE, never a template: it carries no top-level stencil markers,
     and internal/stencil's StripLeadingComment removes this comment before either consumer sees it. -->

# Hardener rubric

A hardener campaign drives one or more modules of this repository against throwaway sandbox hubs, round after round.
The subject under review is the modules' behaviour on a live substrate, never only their code.
The campaign's charter is round 1's focus file, which the seed pass wrote from the modules, and every later focus file carries it forward.
It names the modules and their focus, what to read, the hub layouts to drive, what is out of scope, the live-substrate cost declaration, the teardown check and the round's assignment.
Each round has a review half, a fix half and Go's own live gate, which runs the campaign's live tests after the fix.
The judge reads the round's facts file, which carries the models, the live report, the strays, the fix commits and the deferred findings,
and the paths of the round's focus file, the charter, and of the decision record, the operator's ask.

## Scope

- **The operator's focus binds.**
  When the decision record's `## Goal` states a focus, the campaign stays inside it, and the charter's out-of-scope section lists the rest of the modules.
  No round reviews outside it, and no focus file steers a round outside it.
- **The existing tests are not the review.**
  The modules' existing tests run in the fix row's gates.
  Rerunning them checks a fix; it never stands in for a scenario driven on a sandbox hub.

## Rules of a round

- **Review before fix.**
  The review half only reviews and writes its review file.
  It changes no source file and commits nothing.
  A fix made before the review is saved turns the review into a defence of edits already made.
- **Clean room.**
  Form your own findings before you open any earlier round's review or fixer report.
  Read the focus file first; it never licenses opening those files.
  Once your findings are written, read them to re-drive earlier fixes and to re-judge the findings an earlier round deferred.
- **Log as you go.**
  Record each probe, its command and what it observed in the review file as you make it, never at the end of the round.
  A probe that is not recorded did not happen.
- **Deploy first.**
  After every source change, build the sandbox's lym through `lym harden sandbox new`, and invoke only the `.dev-bin` lym it prints against a sandbox hub.
  A lym found on the path is the wrong binary.
- **Drive the real substrate, yourself.**
  Run the modules' own verbs against a sandbox hub, in the foreground, waiting for each to return, and read what they did, rather than tracing the code to guess it.
  Handing the scenarios to another agent session is not driving them.
  A finding rests on an observed result.
- **Drive every charted layout.**
  Make a hub of each layout the charter names:
  `lym harden sandbox new` builds a two-repository hub, and `lym harden sandbox new --single-repo` a one-repository hub.
- **The focus list is a floor.**
  Walk every focus entry and record OK, WARN or FAIL for each.
  Then improvise many more adversarial scenarios of your own:
  crash and rebirth, scope across hubs and worktrees, dead-but-present state, orphans of an operation that failed midway, rapid churn,
  and verbs combined in orders nothing has tried.
  Chase anything the code makes you suspicious of.
- **Cost is never a reason not to drive.**
  A real substrate session takes real minutes and tokens; that cost is budgeted, never a reason to skip a scenario or to read code instead of driving it.
  "Could not verify" is legitimate only where a human's eyes are required or the environment lacks something, such as a binary or a login.
  Check for that first, and name what blocked you.
- **Cut waste, never driving.**
  Read large files in spans, batch independent commands, script repetitive fixtures, and never read a verbose command's whole output back.
  None of that cuts a fixture per destructive scenario, a sabotage proof, the live reproduction of a BLOCKING finding or the final gates.
- **Prove the scenario reached the code.**
  A green drive counts only once it is shown to have entered the mechanism it claims to exercise:
  sabotage the mechanism and watch the scenario fail, or instrument the write and see it happen.
  A drive that stays green under sabotage leaves the scenario unproven, not the code proven.
- **Re-drive earlier fixes.**
  Re-drive each prior round's fixes against the sandbox, so an earlier fix that a later change undid is found.
- **Prove a concurrency claim twice.**
  A claim about concurrent behaviour needs a sequential control, and a second hub that runs the same drive beside the first.
- **Hold the merge bar.**
  Correctness in the normal single-instance flow is the gate.
  A timeout under the concurrent copies' CPU peg is a diagnostic, never a defect.
- **Count an enumerated class.**
  When the focus assigns a sweep, list every site of the class in a table, the correct sites with the reason,
  and state the total, the reproducible command that enumerated it and what that command cannot see.
  The focus states no count, so the round never starts from one.
- **Run a real LLM test alone.**
  For a module that drives a real LLM session, run one named tier-3 test at a time, never two live runs at once,
  and never a test the charter's cost declaration bans.
- **Keep the sandbox clean.**
  Make every hub through `lym harden sandbox new`, and make none outside the sandbox root.
  Run the charter's teardown check at the end of the round and record its output.
  Leave zero stray state: no server, no process and no directory the round's reaps would have to remove.
- **Never run `lym selfreport create`.**
  A campaign files no issue of its own.
- **Judge each finding.**
  Give each finding its file and line, a concrete failure scenario, a severity of BLOCKING, MEDIUM, LOW or NIT, and a suggested fix.
  Mark it CONFIRMED when reproduced, or PLAUSIBLE when it looks wrong but is unverified.
- **Record every finding, large or small.**
  Severity decides how a finding is reported, never whether it is fixed.
  A finding whose fix is large, such as a feature, a subsystem or a refactor outside the modules, is recorded like any other.

## The judge

- **Converged.**
  The judge rules CONVERGED only on a round whose review records no BLOCKING or MEDIUM finding, whose live gate is green, and whose reviewer model differs from the previous round's.
  It reads all three from the round's facts.
- **Re-seed from the residue.**
  The focus file derives the next round's assignment from what this round left standing, never "review it again":
  - the residual to close: each open finding with its site and scenario, fixed at the right layer with a regression test;
  - an enumeration sweep when the open findings repeat a shape: every site of that shape counted, before the judge rules CIRCLING;
  - the regions no round has driven, once the countable classes are closed;
  - a safety pass once no residual is known: a clean-room pass to find what every round missed, or to confirm the campaign converged,
    with the closed and verified items listed so they are not re-litigated.
    No new defects is the expected outcome of a safety pass, and the round never invents work.
- **Carry the charter forward.**
  The round's facts name its focus file, the charter to read, and the decision record, the operator's ask.
  These two files are the one exception to the judge's rule against opening the artifacts: read both.
  Each focus file restates the charter's standing sections:
  the modules and focus, what to read, the layouts, what is out of scope, the live-substrate cost declaration and the teardown check.
  It rewrites only the round context, and it never steers a round outside the focus the decision record states.
- **Grade on evidence.**
  A concurrency claim with no sequential control and no second-hub run, or a green drive with no proof that it reached the code, proves nothing;
  the focus file asks the next round for the missing proof.
- **Coverage.**
  A convergence verdict states what the campaign did not drive, and what its method failed to show about itself.
