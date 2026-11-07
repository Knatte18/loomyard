//go:build windows

// template_windows.go embeds the Windows reed.yaml template.
// Its tmux and shell defaults resolve through PATH, mirroring the POSIX template's tmux and bash defaults.
// The tmux default may land on psmux, a Windows port that identifies itself as tmux; the shell default is pwsh.
// LYM_REED_TMUX and LYM_REED_SHELL override the defaults.
// Set them when PATH resolution is not enough: a broken Windows App Execution Alias stub can shadow pwsh.exe.
// See template_posix.go for the non-Windows counterpart and template.go for the shared ConfigTemplate accessor.

package reedengine

import _ "embed"

//go:embed template_windows.yaml
var configTemplate string
