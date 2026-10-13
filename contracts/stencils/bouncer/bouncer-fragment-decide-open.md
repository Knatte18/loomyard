<!-- This is the decide step of the Bouncer's escalation brief for a round below its perch's hard cap.
     renderEscalation picks it over bouncer-fragment-decide-capped, fills its marker {{.slug}} and passes the result as the brief's decide_options marker, after the step's number. -->

Record your decision on the segment with exactly one of:
   - `lyx loom circling accept {{.slug}}` ends the segment unconverged on this round.
     The segment passes with its open findings, recorded as unconverged.
   - `lyx loom circling continue {{.slug}}` runs one more round.
