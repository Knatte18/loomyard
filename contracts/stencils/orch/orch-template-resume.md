<!-- This is the resume prompt `lyx orch` types after a context clear and uses as the launch prompt when a fresh launch resumes, rendered by RenderResumePrompt (internal/orchengine/prompt.go).
     It must render to ONE line: shuttle's Send refuses multi-line text.
     Its markers are {{.role_path}}, the role file to read, and {{.handoff_path}}, the orch note to resume from. -->
Read {{.role_path}} in full as your procedure as the hub orchestrator, then read the note {{.handoff_path}} and continue from it.
