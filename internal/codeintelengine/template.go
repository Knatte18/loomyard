// template.go — servers.yaml seed template accessor. Mirrors modelspec's
// embed pattern: the seed content lives in template.yaml and is embedded
// verbatim at build time, with ConfigTemplate as the sole accessor the CLI
// layer (or an init/reconcile flow) consumes for materialization.

package codeintelengine

import _ "embed"

//go:embed template.yaml
var configTemplate string

// ConfigTemplate returns the seed content for servers.yaml: one YAML block
// per built-in language (go/python/csharp/typescript/rust) at its default
// markers/match/command/install_hint values, plus a header comment
// explaining that the file is operator-editable and that entries whole-
// replace the built-ins.
func ConfigTemplate() string {
	return configTemplate
}
