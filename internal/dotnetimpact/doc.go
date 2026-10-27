// Package dotnetimpact derives the test projects and test classes a `lym gate test` run over a C# tier has to cover, for the `dotnet` scoper.
// It is told absolute paths and resolves no cwd.
//
// It reads C# source itself, with no compiler or language server.
// The lexer turns a file into identifier and punctuation tokens and drops everything that is not code:
// `//` and `/* */` comments, `#` preprocessor lines, character literals, and the literal text of every string.
// That covers regular, verbatim `@"…"` with its doubled quotes, and raw `"""…"""` strings with any quote count.
// Inside an interpolated string, regular `$"…"`, verbatim `$@"…"` or `@$"…"`, and raw `$"""…"""` with any count of `$`, the code of every hole is kept as tokens, nested strings and braces included, and the literal text and the `{{` and `}}` escapes are dropped.
//
// Each source file reads into a set of its identifiers and its type declarations.
// A declaration holds the type's name without type parameters, its namespace from a block or file-scoped `namespace`, its enclosing types, its kind, whether it is abstract, the first name of its base list, and whether it carries a test attribute or a member does.
// An attribute is matched in its plain, `Attribute`-suffixed and namespace-qualified spelling, with arguments, and anywhere inside a multi-attribute list;
// an attribute with a target prefix such as `[assembly: …]` sets nothing.
//
// A seed is a set of changed or targeted `.cs` paths and a set of type names.
// Its type-name closure starts from the seeded types and the types declared in the seed files, and adds, until nothing changes, every type declared in a file whose identifiers name a type already in the set.
// A test class is a class carrying `TestFixture` or `TestClass` or a test method, or a non-abstract class whose base, followed through the scanned sources by name, reaches such a class.
// A test class is selected when its file is a seed file or its file's identifiers name a type of the closure, and a selected abstract test class selects every class deriving from it, transitively.
// Each selected non-abstract class gets the filter term `FullyQualifiedName~` and its fully qualified name, a nested class joined to its outer class by `+`; an abstract class gets none.
// A selected class belongs to the `.csproj` in the nearest ancestor directory of its file, up to the tier's `dir`.
// The Scope that results lists one Project per test project holding a selected class, with its sorted terms.
// A non-empty seed that selects no class sets NoTestClass, and an empty seed is the empty scope.
// A seed file or selected class with no `.csproj` above it, and a nearest directory holding more than one, make the Scope Whole with the reason.
//
// DeriveDiff and DeriveCards are the entry points.
// Both are told the worktree, the tier's `dir` relative to it and a predicate over worktree-relative paths that says which are inert, and neither resolves a cwd.
// Git runs through `gitexec.Run` only, and the package spawns nothing else.
// scanTree reads every `.cs` file and lists every `.csproj` under the `dir`, skipping `bin`, `obj`, `.scratch`, `.git`, `_lym` and `.lym` directories.
//
// DeriveDiff diffs a base to the working tree, untracked files included, and drops the inert paths first.
// A diff left with no path, empty or inert-only, is the empty scope, read before the scan.
// Otherwise the seed is the changed `.cs` files and the types both their base and working-tree versions declare, so a renamed or deleted type still seeds.
// It returns the Whole scope, with the reason, when:
// the base is empty or is not an ancestor of HEAD;
// a changed path, by its new or old name, is neither inert nor a `.cs` file, so a `.csproj`, `.props`, `.targets`, `.sln`, `global.json`, `nuget.config` or `.resx` change runs whole;
// a changed `.cs` file lies outside the `dir`;
// a changed `.cs` file declares no type in either version;
// or a git read, the scan or a source read fails.
//
// DeriveCards seeds from the targets of a plan's cards.
// It drops a seed whose file is inert, reads no git, and returns the Whole scope when a seed names a non-inert file that is not `.cs`, a `.cs` file outside the `dir` or absent from the working tree with no types, or neither a file nor a type, or when the scan fails.
// An empty seed list, and seeds all dropped as inert, are the empty scope.
//
// The empty scope, which has no project and does not set NoTestClass, runs nothing: nothing testable changed.
// NoTestClass says something changed but no test class names it.
// A cancelled ctx is the one error, checked before each git read.
//
// Bound: the closure misses only references that never spell a type's name, such as a reflection lookup by a computed string, and the whole gates catch those.
package dotnetimpact
