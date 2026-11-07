//go:build !windows

// template_posix.go embeds the POSIX reed.yaml template.
// Its tmux and shell defaults are the PATH-resolved names tmux and bash.
// It carries no _linux filename suffix;
// the explicit !windows build tag is what makes every non-Windows GOOS use these defaults.
// See template_windows.go for the Windows counterpart and template.go for the shared ConfigTemplate accessor.

package reedengine

import _ "embed"

//go:embed template_posix.yaml
var configTemplate string
