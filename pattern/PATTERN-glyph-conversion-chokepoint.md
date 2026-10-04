# PATTERN-glyph-conversion-chokepoint

loomyard performs no glyph-to-path or path-to-glyph conversion of its own.

- `glyph.Self` is the only path-to-glyph call.
- `Glyph.UnitPath` is the only glyph-to-path call.
- `glyph.Parse` plus `Glyph.String` are the only glyph grammar.
- Forbidden: a `#`-trimming suffix operation over a glyph-typed value, reading `Glyph.Unit` as a disk path, and a local regex over a glyph string.
