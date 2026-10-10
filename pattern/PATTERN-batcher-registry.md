# PATTERN-batcher-registry

Webster's execution unit is the batch the active profile's batchifier derives, and `internal/batcher`'s registry is the one place a batchifier kind is chosen.

- The profile is the one `batcher.yaml`'s `active:` names; the kind that profile selects is looked up in `internal/batcher`'s registry.
- The partition is recorded in `state.json` at first init.
  A later run reads the recorded partition and does not derive it again; only a rebaseline replaces it.
