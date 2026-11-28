<!-- This is the hand-back variant of the parent directive, for a role whose verdict file is its only channel.
     internal/parentdirective.HandBack renders it when it is told a non-empty parent name;
     an empty one renders parent-directive-none instead.
     Its one marker is the parent name: the orch that parents the run. -->

## Your parent

Your run's parent is `{{.parent_name}}`.
When you are blocked, write your report and stop; the run hands the stop back.
