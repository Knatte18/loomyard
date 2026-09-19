<!-- status-line.md is the default status-line text template, rendered via
     tokenvocab.Render (internal/tokenvocab) into the tmux status-line.
     This leading banner comment is stripped by stencil.Fill before parsing, so it documents the template for a human reader only.
     Available top-level tokens (see internal/tokenvocab's registry): {{.repo}} (the repo name), {{.worktree}} (the worktree name), and {{.hub}} (the hub's absolute directory path).
     A Config.StatusLine.Template override replaces this whole asset;
     it is not merged with it. --> {{.repo}}/{{.worktree}} · {{.hub}}
