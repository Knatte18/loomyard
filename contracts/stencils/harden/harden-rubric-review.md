<!-- This is the Hardener rubric.
     The Harden-Bouncer row and the Harden-Burler-Review row read it, in the hardener recipe.
     The Harden-Bouncer row interpolates it as the rubric marker value of bouncer-template-seed.md and bouncer-template-judge.md,
     and the Harden-Burler-Review row interpolates it the same way into internal/burlerengine's review prompt.
     It is a marker VALUE, never a template: it carries no top-level stencil markers,
     and internal/stencil's StripLeadingComment removes this comment before either consumer sees it. -->

# Hardener rubric

A hardener campaign drives one module of this repository against throwaway sandbox hubs, round after round.
The subject under review is the module's behaviour on a live substrate, never only its code.
Each round has a review half, a fix half and Go's own live gate, which runs the campaign's live tests after the fix.
The judge reads the round's facts file, which carries the models, the live report, the strays, the fix commits and the deferred findings.

## Rules of a round

- **Review before fix.**
  The review half only reviews and writes its review file.
  It changes no source file and commits nothing.
- **Log as you go.**
  Record each probe, its command and what it observed in the review file as you make it, never at the end of the round.
  A probe that is not recorded did not happen.
- **Deploy first.**
  After every source change, build the sandbox's lym through `lym harden sandbox new`, and invoke only the `.dev-bin` lym it prints against a sandbox hub.
  A lym found on the path is the wrong binary.
- **Drive the real substrate.**
  Run the module against a sandbox hub and read what it did, rather than tracing the code to guess it.
  A finding rests on an observed result.
- **Sabotage-prove every new regression test.**
  Break the fix, see the new test fail, and restore the fix.
  Re-drive each prior round's fixes against the sandbox, so an earlier fix that a later change undid is found.
- **Prove a concurrency claim twice.**
  A claim about concurrent behaviour needs a sequential control, and a second hub that runs the same drive beside the first.
- **Run a real LLM test alone.**
  For a module that drives a real LLM session, run one named tier-3 test at a time.
- **Keep the sandbox clean.**
  Make every hub through `lym harden sandbox new`, and make none outside the sandbox root.
  Leave zero stray state: no server, no process and no directory the round's reaps would have to remove.
- **Never run `lym selfreport create`.**
  A campaign files no issue of its own.
- **Defer what is not the module's.**
  A finding whose fix is a feature, or a refactor outside the module, is deferred by the fixer and never fixed in the round.

## The judge

- **Converged.**
  The judge rules CONVERGED only on a round whose review records no BLOCKING or MEDIUM finding, whose live gate is green, and whose reviewer model differs from the previous round's.
  It reads all three from the round's facts.
- **Circling.**
  When the open findings repeat a shape, the focus file assigns an enumeration sweep of that shape before the judge rules CIRCLING.
- **Focus.**
  The focus file derives the next round's assignment from what the last round left standing.
- **Coverage.**
  A convergence verdict states what the campaign did not drive.
