<!-- This is the soft-trigger note request `lyx orch` types into the orchestrator session at a natural break below the hard cap, rendered by RenderSoftHandoffInstruction (internal/orchengine/prompt.go).
     It must render to ONE line: shuttle's Send refuses multi-line text.
     Its markers are {{.handoff_path}}, the file the session writes its orch note to, and {{.note_template_path}}, the template the note follows. -->
A context cycle is due but optional: if work a cycle would lose is in flight (a subagent or fork mid-task, an edit half done), reply with the single word DEFER and nothing else, and write no file; otherwise write the orch note to {{.handoff_path}} following the template {{.note_template_path}}, then end your turn.
