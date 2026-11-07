<!-- This is the fresh-launch prompt for the hub orchestrator session `lym beam start` launches.
     RenderStartPrompt (internal/orchengine/prompt.go) reads it and hands it to shuttle as the launch prompt pointer,
     so it must render to ONE line.
     Its one marker is {{.role_path}}, the role file rendered from the orch role. -->
Read {{.role_path}} in full as your procedure as the hub orchestrator, then read the board and continue.
