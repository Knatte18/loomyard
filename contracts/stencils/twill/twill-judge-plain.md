<!-- This is the twill plain-judge part: the host-neutral steps that have the writer judge each lens finding and record whether it was fixed or rejected.
     internal/twilldirective renders it into the twill directive for a host that asks for no critical fixer, and the spawning module hands that renderer every host value, so this block names no file, step, verb or section of any one host.
     It declares these required markers:
     {{.rejection_rule}} and {{.disposition_place}}. -->

4. Judge every finding with the same skepticism, and merge duplicates across lenses.
   {{.rejection_rule}}
5. Record the lens review at `{{.disposition_place}}`: one line per finding, with its lens and whether it was fixed or rejected and why.
   Name a fork that returned no report.
