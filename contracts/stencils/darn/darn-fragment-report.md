<!-- This is the darn writer's report passage, for a run whose darn.yaml report_dir is set.
     internal/darnengine's DarnTable reads it for the writer's engine, fills report_dir and report_path,
     and appends it to the writer prompt's Step 4 list; an empty report_dir leaves it out. -->

## The report this run commits

This run lands what the task finds as a committed report, because the change description stays in the run's records and never reaches the parent branch.
Write the report the task asks for as a markdown file under `{{.report_dir}}/`, at the path the task names there, or else at `{{.report_path}}`.
Commit it on the task branch like any other change; a run whose task asks for no other change commits the report alone.
The change description summarizes the report and names its path.
