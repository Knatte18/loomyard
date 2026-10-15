<!-- This is the twill critical-judge part: the host-neutral steps that have the writer judge each lens finding as a critical fixer and record each disposition.
     internal/twilldirective renders it into the twill directive for a host that asks for the critical fixer, and the spawning module hands that renderer every host value, so this block names no file, step, verb or section of any one host.
     It declares these required markers:
     {{.governing_input}}, {{.dispute_evidence}} and {{.disposition_place}}. -->

4. Judge every finding: merge duplicates across lenses, then weigh each finding twice: its premise, against the code and {{.governing_input}}, and the change it proposes.
   Give it exactly one of three dispositions, decided in this order; severity decides how a finding is reported, not whether it is weighed.
   - **Dispute**, on one of two grounds.
     `premise false`: the premise is false against what you read.
     `every fix worse`: the premise holds, but every change that addresses it makes the reviewed files worse.
     One sound change means the finding is fixed, not disputed.
     The evidence is cited text opened in this session, {{.dispute_evidence}}, and what it shows against the finding.
     Severity, size, cost, taste, and disagreement with the rubric or `PATTERN` never justify a dispute.
     A finding you cannot refute from what you read is not disputed.
     Keeping a waiver, a deferral or an unplanned test against a finding is a dispute and needs the same evidence.
   - **Fix with your own change**: the premise holds, the change the finding proposes is wrong or harmful, and another change resolves the premise.
     Make that change, and name the proposed change you did not make and why, with the same kinds of evidence.
   - **Fix as proposed**: otherwise.
     A finding that proposes no concrete change leaves the change to you.
5. Record the lens review at `{{.disposition_place}}`: one line per finding, with its lens, its severity and its disposition: `disputed` with its ground and the evidence, `fixed differently` with the proposed change you did not make, why, and what changed instead, or `fixed as proposed` with what changed.
   Name a fork that returned no report.
