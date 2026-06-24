// Package configtmpl holds the embedded default YAML templates for every
// configuration module (board, worktree, weft).
//
// It is a dependency-free leaf package: it imports nothing from the rest of
// the project. The feature packages (board, worktree, weft) delegate their
// own ConfigTemplate accessors here, and configreg builds its module registry
// from here as well. Centralising the templates in a leaf keeps configreg from
// importing the feature packages, which previously created a test-build import
// cycle for any feature package whose tests import internal/lyxtest
// (lyxtest -> configreg -> feature -> ... back into the feature test binary).
package configtmpl

import _ "embed"

//go:embed board.yaml
var board string

//go:embed worktree.yaml
var worktree string

//go:embed weft.yaml
var weft string

// Board returns the default YAML template for board configuration.
// The template uses ${env:VAR:-default} syntax for environment-based overrides.
func Board() string { return board }

// Worktree returns the default YAML template for worktree configuration.
// The template uses ${env:VAR:-default} syntax for environment-based overrides.
func Worktree() string { return worktree }

// Weft returns the default YAML template for weft configuration.
// The template uses literal values with no environment-variable placeholders.
func Weft() string { return weft }
