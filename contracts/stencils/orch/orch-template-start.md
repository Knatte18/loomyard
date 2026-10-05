<!-- This is the fresh-launch prompt for the hub orchestrator session `lyx orch start` launches.
     It is read by RenderStartPrompt (internal/orchengine/prompt.go) and handed to shuttle as the launch prompt pointer, so it must render to ONE line.
     Its one marker is {{.role_path}}, the role file rendered from orch-template-role. -->
Read {{.role_path}} in full as your procedure as the hub orchestrator, then read the board and continue.
