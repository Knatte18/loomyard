<!-- This is the handoff instruction `lyx orch` types into the orchestrator session in the handoff-requested phase, rendered by RenderHandoffInstruction (internal/orchengine/prompt.go).
     It must render to ONE line: shuttle's Send refuses multi-line text.
     Its one marker is {{.handoff_path}}, the file the session writes its handoff to. -->
/scribe:handoff {{.handoff_path}} Your context is about to be cleared, so make the handoff complete: list every live background watcher (Monitor, background Bash, subagent) with its id and purpose, so the resumed session can check each one before re-arming it.
