<!-- This is the soft-trigger handoff request `lyx orch` types into the orchestrator session at a natural break below the hard cap, rendered by RenderSoftHandoffInstruction (internal/orchengine/prompt.go).
     It must render to ONE line: shuttle's Send refuses multi-line text.
     Its one marker is {{.handoff_path}}, the file the session writes its handoff to. -->
A context cycle is due but optional: if work a `/clear` would lose is in flight (a subagent or fork mid-task, an edit half done), reply with the single word DEFER and nothing else, and write no file; otherwise run `/scribe:handoff {{.handoff_path}}`, listing every long-lived watcher (background shell, Monitor) with its id and purpose so the resumed session re-arms it, since watchers are not in-flight work.
