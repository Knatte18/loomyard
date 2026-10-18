<!-- This is the note request `lyx orch` posts as creel mail to the orchestrator session in the handoff-requested phase, rendered by RenderHandoffInstruction (internal/orchengine/prompt.go).
     Its markers are {{.handoff_path}}, the file the session writes its orch note to, and {{.note_template_path}}, the template the note follows.
     The mail's id is assigned when it is posted, so the text names the id `lyx creel read` prints beside it. -->
Your context is about to be cycled: write the orch note to {{.handoff_path}} following the template {{.note_template_path}}.
Once the note is written, answer this message with `lyx creel post --reply <id> "written {{.handoff_path}}"`, where `<id>` is the id `lyx creel read` prints beside this message, then end your turn.
