<!-- This is the launch prompt for an adopted session: `lyx orch start --adopt` resumes an already-running Claude session as the orch strand, rendered by RenderAdoptPrompt (internal/orchengine/prompt.go).
     It must render to ONE line, so a resume without a launch prompt followed by a Send stays available.
     Its one marker is {{.role_path}}, the role file to read. -->
Read {{.role_path}} in full as your procedure as the hub orchestrator, then continue the work you were doing; the previous process's background shells and Monitors are gone and none is re-armed.
