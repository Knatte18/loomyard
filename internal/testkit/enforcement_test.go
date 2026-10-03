package testkit

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	modulePath    = "github.com/Knatte18/loomyard/"
	testkitImport = modulePath + "internal/testkit"
	lyxbinImport  = testkitImport + "/lyxbin"
)

// kitDeniedImports are the imports no kit's non-test file may carry.
var kitDeniedImports = map[string]bool{
	"os/exec":                        true,
	modulePath + "internal/gitexec":  true,
	modulePath + "internal/gitkit":   true,
	modulePath + "internal/hubforge": true,
	lyxbinImport:                     true,
}

func isTestkitPath(p string) bool {
	return p == testkitImport || strings.HasPrefix(p, testkitImport+"/")
}

func isCLIImport(p string) bool {
	rest, ok := strings.CutPrefix(p, modulePath+"internal/")
	return ok && !strings.Contains(rest, "/") && strings.HasSuffix(rest, "cli")
}

func TestEnforcement_TestkitInvariant(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine testkit source directory location")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(file)))

	var importerFailures, cliFailures, spawnFailures []string
	parsed := 0

	for _, top := range []string{"internal", "cmd", "tools"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			rel = filepath.ToSlash(rel)

			f, err := parser.ParseFile(token.NewFileSet(), p, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s: %v", rel, err)
			}
			parsed++

			inKit := strings.HasPrefix(rel, "internal/testkit/")
			kit := ""
			if inKit {
				kit = strings.SplitN(strings.TrimPrefix(rel, "internal/testkit/"), "/", 2)[0]
			}
			for _, imp := range f.Imports {
				ip := strings.Trim(imp.Path.Value, `"`)
				if !inKit && isTestkitPath(ip) {
					importerFailures = append(importerFailures, rel+": "+ip)
				}
				if !inKit {
					continue
				}
				if isCLIImport(ip) {
					cliFailures = append(cliFailures, rel+": "+ip)
				}
				if kitDeniedImports[ip] && !(kit == "lyxbin" && ip == "os/exec") {
					spawnFailures = append(spawnFailures, rel+": "+ip)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", top, err)
		}
	}

	if parsed == 0 {
		t.Fatal("vacuous scan: no non-test file was parsed")
	}
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
