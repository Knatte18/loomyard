<!-- This is the handoff instruction `lyx orch` types into the orchestrator session in the handoff-requested phase, rendered by RenderHandoffInstruction (internal/orchengine/prompt.go).
     It must render to ONE line: shuttle's Send refuses multi-line text.
     Its one marker is {{.handoff_path}}, the file the session writes its handoff to. -->
/scribe:handoff {{.handoff_path}} Your context is about to be cycled, so make the handoff complete.
