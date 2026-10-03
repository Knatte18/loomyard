package testkit

import (
	"go/parser"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

const (
	modulePath    = "github.com/Knatte18/loomyard/"
	testkitImport = modulePath + "internal/testkit"
	lyxbinImport  = testkitImport + "/lyxbin"
	tmuxkitImport = testkitImport + "/tmuxkit"
)

// kitDeniedImports are the imports no kit's non-test file may carry.
var kitDeniedImports = []string{
	"os/exec",
	modulePath + "internal/gitexec",
	modulePath + "internal/gitkit",
	modulePath + "internal/hubforge",
	lyxbinImport,
	tmuxkitImport,
}

func isTestkitPath(p string) bool {
	return p == testkitImport || strings.HasPrefix(p, testkitImport+"/")
}

func isCLIImport(p string) bool {
	rest, ok := strings.CutPrefix(p, modulePath+"internal/")
	return ok && !strings.Contains(rest, "/") && strings.HasSuffix(rest, "cli")
}

func TestEnforcement_TestkitInvariant(t *testing.T) {
	var importerFailures, cliFailures, spawnFailures []string

	parsed := scankit.Walk(t, scankit.Options{Roots: []string{"internal", "cmd", "tools"}}, func(f *scankit.File) {
		inKit := strings.HasPrefix(f.Rel, "internal/testkit/")
		kit := ""
		if inKit {
			kit = strings.SplitN(strings.TrimPrefix(f.Rel, "internal/testkit/"), "/", 2)[0]
		}
		for _, imp := range f.AST(t, parser.ImportsOnly).Imports {
			ip := strings.Trim(imp.Path.Value, `"`)
			if !inKit && isTestkitPath(ip) {
				importerFailures = append(importerFailures, f.Rel+": "+ip)
			}
			if !inKit {
				continue
			}
			if isCLIImport(ip) {
				cliFailures = append(cliFailures, f.Rel+": "+ip)
			}
			if slices.Contains(kitDeniedImports, ip) && !(ip == "os/exec" && (kit == "lyxbin" || kit == "tmuxkit")) {
				spawnFailures = append(spawnFailures, f.Rel+": "+ip)
			}
		}
	})

	scankit.RequireFloor(t, parsed, 1, "Testkit Invariant scan")
	if len(importerFailures) > 0 {
		t.Errorf("Testkit Invariant importer rule violated; a non-test file outside internal/testkit imports a kit: %v", importerFailures)
	}
	if len(cliFailures) > 0 {
		t.Errorf("Testkit Invariant *cli rule violated; a kit imports a *cli package: %v", cliFailures)
	}
	if len(spawnFailures) > 0 {
		t.Errorf("Testkit Invariant spawn-import rule violated; a kit imports a spawning package: %v", spawnFailures)
	}
}

func TestEnforcement_ScankitStdlibOnly(t *testing.T) {
	scankit.AssertImportAllowlist(t, "internal/testkit/scankit")
}
