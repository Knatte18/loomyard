# PATTERN-glyph-conversion-chokepoint

loomyard performs no glyph-to-path or path-to-glyph conversion of its own, bar the one external-test unit mapping below.

- `glyph.Self` is the only path-to-glyph call.
- `Glyph.UnitPath` is the only glyph-to-path call.
- `glyph.Parse` plus `Glyph.String` are the only glyph grammar.
- `planparser`'s `goPackageDir` is the single sanctioned adjustment of a Go unit string, for the external-test pseudo-unit only.
  Quarry names an external test package's unit `<dir>_test`, a pseudo-path with no directory (the corner case in quarry's `docs/glyph.md`), and `Glyph.UnitPath` returns it unchanged, so some caller must map it to `<dir>`.
- `planparser.ParseRef` alone picks a plan ref's or handle's alphabet, from the plan's `language:`.
  Nothing outside `internal/planparser` passes a `glyph.Language` to a parse, and `Plan.GlyphLanguage` answers only whether the plan is glyph-enabled.
  An id quarry minted for a C# member is no plan ref, and `planparser.ParseMintedCSharpID` is the one parse of it.
- Path self glyphs use the Go unit alphabet under every glyph-enabled language, since quarry spells a path in that alphabet.
  `planparser.PathEntryPath` is the one Go-alphabet reading of a canonicalized path entry back to its path.
- Forbidden: a `#`-trimming suffix operation over a glyph-typed value, reading `Glyph.Unit` as a disk path, and a local regex over a glyph string.
