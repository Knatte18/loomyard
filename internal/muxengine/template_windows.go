//go:build windows

// template_windows.go embeds the Windows variant of the mux.yaml template,
// whose psmux/pwsh defaults point at the machine's pinned psmux.exe/pwsh.exe
// paths. See template_posix.go for the non-Windows counterpart and
// template.go for the shared, untagged ConfigTemplate() accessor.

package muxengine

import _ "embed"

//go:embed template_windows.yaml
var configTemplate string
