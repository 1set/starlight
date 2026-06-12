# Golden end-to-end test of starlight's Go<->Starlark conversion, run through
# the real interpreter. Globals supplied by the Go runner:
#   nums  : Go map[string]int
#   mixed : Go map[string]interface{}  (JSON-shaped)
#   pairs : Go map[interface{}]interface{} with tuple + big-int keys
#
# Each `out_*` global is asserted by the runner.

# --- SLT-01 deterministic map order: keys() is sorted, stable, == iteration ---
ks = nums.keys()
out_keys_sorted = (ks == sorted(ks))
out_keys_match_iter = (ks == [k for k in nums])
out_items_match = ([nums[k] for k in ks] == [i[1] for i in nums.items()])

# --- SLT-08 empty-interface unwrap: JSON-shaped values are native, usable ---
out_sum = mixed["a"] + mixed["b"]          # ints unwrap -> arithmetic works
out_a_type = type(mixed["a"])              # "int", not a wrapper
out_eq = (mixed["a"] == 1)                 # comparison works
out_nested = mixed["inner"]["x"] + 0       # nested dict usable

# --- SLT-02 tuple key + SLT-17 big-int key round-trip through a wrapped map ---
out_tuple_key = pairs[(1, "a")]
out_bigint_key = pairs[1 << 70]

# --- SLT-03 safety: str() of a (large) wrapped map never crashes the host ---
out_str_ok = (len(str(nums)) > 0)
