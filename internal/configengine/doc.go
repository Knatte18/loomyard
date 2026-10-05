// Package configengine loads, resolves and writes a module's YAML configuration under a base directory's `_lyx/config/`.
//
// [Load] is strict: an absent `_lyx/` or config file is an error.
// [LoadOrTemplate] degrades to the caller's embedded template on proven absence of either, and is otherwise identical.
// `PATTERN-config-strictness` decides which of the two a caller adopts.
//
// Both fill, in memory, the template keys a present file lacks and log each fill;
// neither writes the file.
// A key missing inside a list element is the one gap fill cannot close, and is an error.
// The result is resolved through envsource's markers.
//
// [Set] writes dotted key=value pairs into a module's file, scaffolding it from the template when absent, with no editor and no validation loop.
// [Edit] is the interactive counterpart.
//
// # Open maps
//
// [Load], [LoadOrTemplate] and [Set] take a trailing variadic openMaps: the dotted paths of the module's open maps, mappings whose keys the user owns.
// They are passed unchanged to yamlengine, which defines what a declared path changes;
// with none declared, every caller behaves as before.
package configengine
