// Package lyxtest holds the shared git-fixture support machinery for Loomyard's
// test suites across internal/warp, internal/weft, and internal/paths.
// It owns the fixture builders and per-test isolation helpers, following the
// template-built-once + per-test filesystem copy pattern to minimize setup overhead
// and maximize parallelism. See MustRun, CopyHostHub, CopyPaired, and CopyWeft.
//
// Leaf Invariant: internal/lyxtest must remain a leaf package importing only the
// standard library and internal/paths. It must not import internal/configreg or any
// feature package (board, warp, weft). Feature packages' internal tests import
// lyxtest; a configreg or feature import would close a test-build cycle. Tests that
// need real configuration seed it via SeedConfig, which takes a configreg-free
// map[string]string (module name to YAML content), converting configreg.Modules()
// or a feature's ConfigTemplate() at the test site instead of inside lyxtest.
package lyxtest
