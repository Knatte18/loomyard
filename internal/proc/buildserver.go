// buildserver.go declares the environment that keeps a .NET build from leaving build-server processes running after it exits.

package proc

// EnvVar is one environment variable as a name and its value.
type EnvVar struct {
	Name  string
	Value string
}

// BuildServerOffEnv is the environment that stops a .NET build from leaving MSBuild reuse nodes and the MSBuild server running once it exits.
// Lingering nodes of the builds parallel agents run hold enough threads to exhaust a user's task limit, so every process lym starts for an agent or a gate run gets these values.
var BuildServerOffEnv = []EnvVar{
	{Name: "MSBUILDDISABLENODEREUSE", Value: "1"},
	{Name: "DOTNET_CLI_USE_MSBUILD_SERVER", Value: "0"},
}
