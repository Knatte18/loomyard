# Task: semver range resolver

You work alone and independently.
Your output directory is `.scratch/haikubench/out-a/`.
Create it, and write every file you produce there.
Do not read, list or write anything else under `.scratch/haikubench/`, and do not touch any file outside your output directory.
Use Python 3 with the standard library only.

## Timing

Your run is timed.
Your very first tool call, before reading anything or planning, is this Bash command:

```
mkdir -p .scratch/haikubench/out-a && date +%s.%N | tee .scratch/haikubench/out-a/start.txt
```

When everything else is done and `report.md` is final, your very last tool call is a Bash command that reads the current time, computes the duration from `start.txt`, and appends this to `report.md`:

```
## Timing

start: <epoch seconds from start.txt>
stop: <epoch seconds now>
duration: <stop - start, in seconds, one decimal>
```

Do not edit any file after that.

## Deliverables

1. `semver_range.py` exposing `max_satisfying(versions: list[str], range_expr: str) -> str | None`.
2. `test_semver_range.py`, a `unittest` suite of at least 20 cases, covering the edge cases in the spec below.
3. `report.md`: the test command you ran and its real output, a list of every place where the spec was ambiguous and what you chose, and known gaps.

Run your own tests before you finish, and fix failures.
The report must state honestly what passes and what does not.

## Function contract

`max_satisfying` returns the highest version in `versions` that satisfies `range_expr`, or `None` if none does.
The returned string is the element of `versions` exactly as given.
Entries of `versions` that are not valid versions are skipped silently.
An invalid `range_expr` raises `ValueError`.

## Versions

Format: `MAJOR.MINOR.PATCH`, each a non-negative integer without leading zeros, then optional `-PRERELEASE`, then optional `+BUILD`.
`PRERELEASE` is dot-separated identifiers of `[0-9A-Za-z-]`; a numeric identifier has no leading zeros.
`BUILD` is ignored for ordering, and two versions differing only in build compare equal (return the first such maximum in list order).
No leading `v`, no partial versions in the `versions` list.

Ordering follows semver 2.0.0:
- Compare major, minor, patch numerically.
- A version with a prerelease is lower than the same version without one.
- Prerelease identifiers compare left to right; numeric identifiers compare numerically, alphanumeric ones in ASCII order, and a numeric identifier is lower than an alphanumeric one.
- If all shared identifiers are equal, the version with more identifiers is higher.

## Range grammar

A range is one or more comparator sets separated by `||`.
A version satisfies the range if it satisfies at least one set.
A set is whitespace-separated terms, all of which must hold.
An empty or all-whitespace range, or an empty set, means `*`.

A *partial* version is `X`, `X.Y` or `X.Y.Z`, where any component after the first may be missing, and any component may be `x`, `X` or `*` (a wildcard; everything after a wildcard is a wildcard too).
`*` alone is fully wildcard.

Terms:

- **Operator comparators** `>=`, `>`, `<=`, `<`, `=` followed by a partial version.
  A bare partial version means `=`.
  Whitespace between operator and version is allowed (`>= 1.2.3`).
  A partial version is completed as follows:
  - `=1.2` and `1.2` and `1.2.x` mean `>=1.2.0 <1.3.0`; `=1` means `>=1.0.0 <2.0.0`; `*` means any version.
  - `>1.2` means `>=1.3.0`; `>1` means `>=2.0.0`.
  - `>=1.2` means `>=1.2.0`.
  - `<1.2` means `<1.2.0`.
  - `<=1.2` means `<1.3.0`; `<=1` means `<2.0.0`.
  - With a full version the operator has its plain meaning.
- **Caret** `^V`: allow changes that do not modify the leftmost non-zero component.
  `^1.2.3` is `>=1.2.3 <2.0.0`; `^0.2.3` is `>=0.2.3 <0.3.0`; `^0.0.3` is `>=0.0.3 <0.0.4`.
  `^1.2` is `>=1.2.0 <2.0.0`; `^0.0` is `>=0.0.0 <0.1.0`; `^0` is `>=0.0.0 <1.0.0`.
  `^1.2.3-beta.2` is `>=1.2.3-beta.2 <2.0.0`.
- **Tilde** `~V`: `~1.2.3` is `>=1.2.3 <1.3.0`; `~1.2` is `>=1.2.0 <1.3.0`; `~1` is `>=1.0.0 <2.0.0`.
- **Hyphen range** `A - B` (spaces around the hyphen required):
  `A` partial is completed with zeros and used as `>=`.
  `B` full is `<=B`; `B` partial `1.2` is `<1.3.0`, `1` is `<2.0.0`.
  Example: `1.2 - 2.3.4` is `>=1.2.0 <=2.3.4`; `1.2.3 - 2.3` is `>=1.2.3 <2.4.0`.

## Prerelease rule

A version that has a prerelease satisfies a comparator set only if both hold:
1. It satisfies every term in the set by ordering.
2. At least one comparator in the set (after expansion) has a prerelease and the same `MAJOR.MINOR.PATCH` as the version.

So `1.2.3-beta.2` satisfies `>=1.2.3-beta.1 <2.0.0`, but `1.3.0-alpha` does not, and `*` never matches a prerelease.
The rule is evaluated per comparator set; `||` sets do not share comparators.

## Examples

| versions | range | result |
|---|---|---|
| `1.2.3 1.9.0 2.0.0` | `^1.2.3` | `1.9.0` |
| `0.2.3 0.2.9 0.3.0` | `^0.2.3` | `0.2.9` |
| `1.2.3 1.2.4-rc.1` | `>=1.2.3` | `1.2.3` |
| `1.2.4-rc.1 1.2.4-rc.2` | `>=1.2.4-rc.1` | `1.2.4-rc.2` |
| `1.0.0 2.5.0 3.1.0` | `<2.0.0 \|\| >=3.0.0` | `3.1.0` |
| `1.2.5 1.3.0 2.0.0` | `1.2 - 1.2.9` | `1.2.5` |
| `1.0.0` | `>=a.b` | `ValueError` |

## What to optimize for

Correctness against the spec first.
Then clear, small functions and tests that fail when the code is wrong.
Do not add features the spec does not mention.
