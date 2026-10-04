# PATTERN-config-strictness

`internal/configengine` offers `Load` (strict) and `LoadOrTemplate` (degrades to the embedded template); a caller adopts exactly one.

- Degrading callers: `{shuttleengine, reedengine, websterengine, batcher, orchengine, loggerconfig}`.
- Strict callers: `{fabricengine, boardengine, loomengine, landingshed}`.
- A template list is a default, not a minimum length.
- A key missing from a present config file resolves to its template default and is logged.
- `Load` and `LoadOrTemplate` differ only on an absent `_lyx/` or file.
