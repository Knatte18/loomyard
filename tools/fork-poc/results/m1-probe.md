TOOL_CALLS: 12
1. NONCE=cormorant-58211

2. `func Parse(s string) (Spec, error) {`

3. Spec struct fields:
   - `Alias string`
   - `Engine string`
   - `Model string`
   - `Params map[string]string`

4. When the models.yaml file does not exist, `LoadRegistry` returns the built-in fallback registry (`builtins()`) unchanged, with a nil error.

---
M1 VERDICT: PASS. Post-fork (excluding inherited history): 0 tool calls,
all 4 answers correct from inherited context alone.
Post-fork usage: in=838 cache_cr=37303 cache_rd=27246 out=141 compute=38282.
(The TOOL_CALLS: 12 above counts the explorer's inherited history — fork
transcripts copy the parent's messages; measurement must split at the fork
point. Methodology note for Q3.)
