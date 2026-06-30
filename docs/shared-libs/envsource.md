# `internal/envsource`

The **single source of truth** for how environment variables enter the system. It reads the `.env` file and overlays the OS environment into a unified map.

**Dependency direction (Go enforces it):** `internal/envsource` imports `internal/hubgeometry` and stdlib only, never domain modules. All modules that need env data call `envsource.Build()`.

## Exported function

### `Build(baseDir string) (map[string]string, error)`

Reads and merges environment variables from `.env` and the OS environment.

**Behavior:**

1. Calls `hubgeometry.DotEnv(baseDir)` to compute the path to the `.env` file.
2. Reads the `.env` file line-by-line, parsing `KEY=VALUE` pairs.
3. Reads the OS environment via `os.Environ()`.
4. Merges the two: OS values **take precedence** over `.env` values for any duplicate key.
5. Returns the merged map.

**`.env` file parsing:**

- Each line is split on the first `=` only; `=` may appear multiple times in the value.
- Lines are taken literally: values are not trimmed, no quote-stripping, no interpretation.
- Blank lines are skipped.
- Lines beginning with `#` (after whitespace-trim) are treated as comments and skipped.
- Lines without an `=` are skipped.
- If the `.env` file does not exist, parsing returns an empty map (no error).

**Precedence:**

OS environment variables always override `.env` variables for the same key. This allows a single `lyx` invocation to override `.env` with a real process environment variable if needed:

```bash
# .env contains: HOME=/old/path
# Running with an override:
export HOME=/new/path
lyx board status          # uses /new/path from OS env, not .env
```

**Eager evaluation:**

The entire map is built once per `Build()` call and returned; there is no lazy evaluation or incremental updates.

**Returns:** A map of environment variable names to values, or an error if the `.env` file cannot be read (other than ENOENT).

## Integration with yamlengine

`envsource.Build()` is designed to be called once at the start of configuration loading:

```go
env, err := envsource.Build(baseDir)
if err != nil {
    return nil, fmt.Errorf("build environment: %w", err)
}

resolved, err := yamlengine.Resolve(configBytes, env)
if err != nil {
    return nil, fmt.Errorf("resolve config: %w", err)
}
```

The pure engine (`yamlengine.Resolve`) has no knowledge of `.env`, OS env, or where variables come from — it only knows how to substitute them.

## Design principles

**Single policy entry point:**

All I/O for environment variable sourcing happens in `Build()`. There is only one place to change or extend env-sourcing behavior (adding support for a different file format, env-var prefixes, secrets stores, etc.).

**OS takes precedence:**

The OS environment always wins over `.env`. This matches common dotenv library behavior and keeps the highest-priority override available to users via standard process env.

**Predictable, eager:**

The map is built completely upfront, not lazily. This makes the behavior predictable and testable: no surprises from side effects during env lookup later.

**Decoupled from the engine:**

`envsource.Build()` does not call `yamlengine.Resolve()` or vice versa. The separation keeps both modules independent and reusable in different contexts.
