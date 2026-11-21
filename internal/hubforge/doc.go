// Package hubforge is the repo-wide real-hub fixture factory:
// it builds every hub fixture through fabriccli.CloneAndWire and never replicates that wiring by hand.
// It asserts nothing about fabric, which is why its name does not end in "test".
//
// It drives fabriccli.CloneAndWire, not fabricengine.CloneHub.
// It does so because CloneHub alone yields a partial hub with no junctions and no repo-wide fabric.yaml.
// A hub built here is therefore real and fully wired, never a hand-assembled stand-in.
//
// The factory follows a copy-the-bares, clone-the-hub model.
// A warp/weft bare-repo pair is built once per test binary and copied fresh into each scenario's own tb.TempDir().
// A hub itself cannot be copied the same way:
// its junctions carry absolute targets that a plain directory copy would leave pointing at the wrong tree.
// The same template-and-copy model extends to the hub through Shape and Registration.
// The first CopyHub, CopyRegistered or SharedHub of a key builds its template once per test binary.
// A build that fails fails its caller and every later caller naming the key, without a rebuild.
// CopyHub and CopyRegistered relocate the template into the test's own tb.TempDir.
//
// A build's process-wide calls are outside its TB's reach.
// They are os.Setenv, os.MkdirTemp, global git config, a package-global seam setter and a once-only registration.
// The TB's Context kept past the build is one too.
// Relocation rewrites only the template root, so any of those results reaches every later copy or test.
// So a build makes none of them, and review holds that line.
// A test that installs a process-global seam takes its copies first.
//
// SharedHub returns a shape's template's own Hub for a test that only reads it.
// A write is anything that may change a file under the hub:
// a fabric mutating verb, SeedConfig, AddPair, any run of the lym binary or any in-process CLI verb.
// Any spawned git subcommand is one too.
// A write to a shared template poisons it, and the test that notices fails naming the fixture;
// the blame may land on a parallel sharer.
//
// Teardown contract: junctions are discovered by walking the hub root with fslink.IsLink, never by slug.
// They are removed in a tb.Cleanup registered before tb.TempDir()'s own cleanup runs,
// so every junction is unwired before Go's os.RemoveAll walks into it.
//
// No package inside internal/fabriccli's dependency set may import hubforge:
// such tests use an external *_test package or gitkit instead.
// See PATTERN-test-isolation for the machine-checked half of this contract.
package hubforge
