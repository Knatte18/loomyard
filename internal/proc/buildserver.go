// buildserver.go declares the environment that keeps a .NET build's build servers from outliving it.

package proc

// EnvVar is one environment variable as a name and its value.
type EnvVar struct {
	Name  string
	Value string
}

// BuildServerOffEnv is the environment that stops a .NET build leaving MSBuild reuse nodes and its server running.
// Lingering nodes of the builds parallel agents run hold enough threads to exhaust a user's task limit,
// so every process lym starts for an agent or a gate run gets these values.
var BuildServerOffEnv = []EnvVar{
	{Name: "MSBUILDDISABLENODEREUSE", Value: "1"},
	{Name: "DOTNET_CLI_USE_MSBUILD_SERVER", Value: "0"},
}
