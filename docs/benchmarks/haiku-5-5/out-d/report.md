# Report: semver range resolver

## Test command and output

Run from `.scratch/haikubench/out-d/`:

```
python3 -m unittest -v test_semver_range
```

Output, last lines (all 46 test lines read `... ok`):

```
test_wildcard_propagates (test_semver_range.Wildcards.test_wildcard_propagates) ... ok
test_x_minor (test_semver_range.Wildcards.test_x_minor) ... ok

----------------------------------------------------------------------
Ran 46 tests in 0.003s

OK
```

All 46 tests pass, including every row of the spec's example table.
Nothing is known to fail.

## Ambiguities and choices

- **Wildcard with strict operators**: `>*` and `<*` match nothing; `>=*`, `<=*`, `=*` and bare `*` match any release (npm behavior).
- **Partial caret `^0.1`**: not in the spec; treated as `>=0.1.0 <0.2.0`, consistent with "leftmost non-zero component".
- **Wildcard propagation**: `1.x.3` is treated as `1.x.x`, per "everything after a wildcard is a wildcard too".
- **Prerelease and build in ranges**: allowed only on a full `X.Y.Z` term; `1.x-beta` and `1.2-beta` raise `ValueError`. Build metadata in a range is accepted and ignored.
- **Leading `v` in ranges**: the spec forbids it only in the version list; ranges reject it too (no unmentioned features).
- **Whitespace after an operator**: allowed for `>=`, `>`, `<=`, `<`, `=` only, as the spec words it; `^ 1.2` and `~ 1.2` raise `ValueError`.
- **Hyphen bounds**: both sides must be bare partials; `>=1 - 2` raises. `A - *` has no upper bound. A chain `1 - 2 - 3` raises.
- **`<1.2` and prereleases of `1.2.0`**: following the spec, `<1.2` is `<1.2.0`, and `1.2.0-rc.1` is excluded by the prerelease rule (no comparator carries a prerelease), not by ordering.
- **Non-string list entries**: skipped as invalid versions. A non-string range raises `ValueError`.
- **Range validation**: the range is parsed before the list is scanned, so an invalid range raises even when `versions` is empty.
- **Unparseable terms** such as `^~1.2`, `>>1` or `1.2.3.4` raise `ValueError`.

## Known gaps

- Exotic npm syntax not in the spec (`~>`, `x` as a prerelease placeholder, loose parsing, `includePrerelease`) is deliberately unsupported.
- Timing deviation: the prompt had to be read before its timing command could be known, so the first tool call was reading `prompt-d.md`; `start.txt` was written immediately after that.

## Timing

start: 1791459902.805236990
stop: 1791459996.255851348
duration: 93.5
