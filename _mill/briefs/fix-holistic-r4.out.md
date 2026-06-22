All three findings fixed:

1. [BLOCKING] preserve-names-as-subtests in TestLayoutChecksum - renamed subtest rows in cmd_test.go to original function names.
2. [BLOCKING] Equivalence guardrail name-map incorrect paths - updated all name-map entries to full subtest names across board, weft, ide, muxpoc.
3. [NIT] TestRenderSpecialBucketTask added to board folded-names list.

All verify commands passed.

{"status":"success","commit_sha":"c083bad91af653c36c7e5b24d26172bfd09e229e","session_id":"1fa5df39-aac2-40cb-b094-899e816b95e3"}