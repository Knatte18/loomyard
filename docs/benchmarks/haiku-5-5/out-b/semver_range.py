"""Resolve the highest version that satisfies a semver range expression."""

import operator
import re
from typing import NamedTuple

_IDENT = r"(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)"
_PRERELEASE_RE = re.compile(rf"{_IDENT}(?:\.{_IDENT})*")
_BUILD_RE = re.compile(r"[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*")
_NUMBER_RE = re.compile(r"0|[1-9][0-9]*")
_WILDCARDS = ("x", "X", "*")
# Ordered so that two-character operators are matched before their prefixes.
_OPERATORS = (">=", "<=", ">", "<", "=")
_COMPARE = {
    ">=": operator.ge,
    ">": operator.gt,
    "<=": operator.le,
    "<": operator.lt,
    "=": operator.eq,
}


class _Version(NamedTuple):
    core: tuple  # (major, minor, patch)
    ids: tuple  # prerelease identifiers, empty for a release

    @property
    def key(self):
        return self.core + (_prerelease_key(self.ids),)


class _Comparator(NamedTuple):
    op: str
    version: _Version


def _prerelease_key(ids):
    # A release sorts above every prerelease of the same core.
    if not ids:
        return (1,)
    return (0, tuple((0, int(i)) if i.isdigit() else (1, i) for i in ids))


def _parse_spec(text):
    """Parse a version or partial version into (parts, prerelease ids).

    Each part is an int, or None for a wildcard or a missing component.
    Build metadata is validated and dropped.
    """
    core, plus, build = text.partition("+")
    if plus and not _BUILD_RE.fullmatch(build):
        raise ValueError(f"invalid build metadata in {text!r}")
    core, dash, prerelease = core.partition("-")
    ids = ()
    if dash:
        if not _PRERELEASE_RE.fullmatch(prerelease):
            raise ValueError(f"invalid prerelease in {text!r}")
        ids = tuple(prerelease.split("."))
    fields = core.split(".")
    if len(fields) > 3:
        raise ValueError(f"too many components in {text!r}")
    parts = []
    for field in fields:
        if field in _WILDCARDS:
            parts.append(None)
        elif _NUMBER_RE.fullmatch(field):
            parts.append(int(field))
        else:
            raise ValueError(f"invalid version component in {text!r}")
    # Everything after a wildcard is a wildcard too.
    leading = next((i for i, p in enumerate(parts) if p is None), len(parts))
    parts = tuple(parts[:leading]) + (None,) * (3 - leading)
    if ids and None in parts:
        raise ValueError(f"a prerelease needs a full version: {text!r}")
    return parts, ids


def _parse_version(text):
    if not isinstance(text, str):
        return None
    try:
        parts, ids = _parse_spec(text)
    except ValueError:
        return None
    if None in parts:
        return None
    return _Version(parts, ids)


def _leading(parts):
    """Number of components before the first wildcard."""
    return next((i for i, p in enumerate(parts) if p is None), 3)


def _floor(parts):
    """The lowest version a partial stands for, with missing components as zero."""
    return tuple(0 if p is None else p for p in parts)


def _bump(core, index):
    """Increment the component at index and zero every component after it."""
    return tuple(
        p + 1 if i == index else (0 if i > index else p) for i, p in enumerate(core)
    )


def _operator_comparators(op, parts, ids):
    leading = _leading(parts)
    if leading == 3:
        return [_Comparator(op, _Version(parts, ids))]
    if leading == 0:
        if op in ("=", ">=", "<="):
            return []
        raise ValueError(f"{op}* is not a valid comparator")
    lower = _Version(_floor(parts), ())
    upper = _Version(_bump(_floor(parts), leading - 1), ())
    if op == "=":
        return [_Comparator(">=", lower), _Comparator("<", upper)]
    if op == ">=":
        return [_Comparator(">=", lower)]
    if op == "<":
        return [_Comparator("<", lower)]
    if op == "<=":
        return [_Comparator("<", upper)]
    return [_Comparator(">=", upper)]


def _caret(parts, ids):
    leading = _leading(parts)
    if leading == 0:
        raise ValueError("^ needs a version")
    floor = _floor(parts)
    # Bump the leftmost non-zero component, or the last given one if all are zero.
    index = next((i for i in range(leading) if floor[i] != 0), leading - 1)
    return [
        _Comparator(">=", _Version(floor, ids)),
        _Comparator("<", _Version(_bump(floor, index), ())),
    ]


def _tilde(parts, ids):
    leading = _leading(parts)
    if leading == 0:
        raise ValueError("~ needs a version")
    floor = _floor(parts)
    index = min(leading, 2) - 1
    return [
        _Comparator(">=", _Version(floor, ids)),
        _Comparator("<", _Version(_bump(floor, index), ())),
    ]


def _hyphen(low_text, high_text):
    low_parts, low_ids = _parse_spec(low_text)
    high_parts, high_ids = _parse_spec(high_text)
    comparators = [_Comparator(">=", _Version(_floor(low_parts), low_ids))]
    leading = _leading(high_parts)
    if leading == 3:
        comparators.append(_Comparator("<=", _Version(high_parts, high_ids)))
    elif leading > 0:
        upper = _Version(_bump(_floor(high_parts), leading - 1), ())
        comparators.append(_Comparator("<", upper))
    return comparators


def _parse_term(term):
    if term.startswith("^"):
        return _caret(*_parse_spec(term[1:]))
    if term.startswith("~"):
        return _tilde(*_parse_spec(term[1:]))
    op = next((o for o in _OPERATORS if term.startswith(o)), "")
    parts, ids = _parse_spec(term[len(op):])
    return _operator_comparators(op or "=", parts, ids)


def _parse_set(text):
    tokens = text.split()
    comparators = []
    i = 0
    while i < len(tokens):
        if i + 1 < len(tokens) and tokens[i + 1] == "-":
            if i + 2 >= len(tokens):
                raise ValueError(f"hyphen range without an upper bound in {text!r}")
            comparators += _hyphen(tokens[i], tokens[i + 2])
            i += 3
        elif tokens[i] in _OPERATORS and i + 1 < len(tokens):
            # Whitespace is allowed between a comparator operator and its version.
            comparators += _parse_term(tokens[i] + tokens[i + 1])
            i += 2
        else:
            comparators += _parse_term(tokens[i])
            i += 1
    return comparators


def _parse_range(range_expr):
    return [_parse_set(text) for text in range_expr.split("||")]


def _satisfies(version, comparators):
    if not all(_COMPARE[c.op](version.key, c.version.key) for c in comparators):
        return False
    if not version.ids:
        return True
    # A prerelease needs a comparator in the same set with the same core and a prerelease.
    return any(c.version.ids and c.version.core == version.core for c in comparators)


def max_satisfying(versions, range_expr):
    """Return the highest entry of versions that satisfies range_expr, or None.

    Entries that are not valid versions are skipped.
    An invalid range_expr raises ValueError, even when versions is empty.
    """
    sets = _parse_range(range_expr)
    best = None
    best_version = None
    for text in versions:
        version = _parse_version(text)
        if version is None:
            continue
        if not any(_satisfies(version, comparators) for comparators in sets):
            continue
        if best_version is None or version.key > best_version.key:
            best, best_version = text, version
    return best
