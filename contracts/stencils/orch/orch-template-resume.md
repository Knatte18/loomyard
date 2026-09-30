<!-- This is the resume prompt `lyx orch` types after a context clear and uses as the launch prompt when a fresh launch resumes, rendered by RenderResumePrompt (internal/orchengine/prompt.go).
     It must render to ONE line: shuttle's Send refuses multi-line text.
     Its one marker is {{.handoff_path}}, the handoff file to resume from. -->
You are the hub orchestrator, resuming after a context cycle: read {{.handoff_path}} and continue from it, and check each watcher it lists before re-arming it.
