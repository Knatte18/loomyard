//go:build windows

// template_windows.go embeds the Windows variant of the mux.yaml template,
// whose psmux/pwsh defaults resolve via PATH (psmux/pwsh), mirroring the
// POSIX template's tmux/bash defaults — no machine-specific install path is
// baked in. LYX_MUX_PSMUX/LYX_MUX_PWSH override the default when PATH
// resolution is not enough (e.g. a broken Windows App Execution Alias stub
// shadowing the real pwsh.exe). See template_posix.go for the non-Windows
// counterpart and template.go for the shared, untagged ConfigTemplate()
// accessor.

package muxengine

import _ "embed"

//go:embed template_windows.yaml
var configTemplate string
