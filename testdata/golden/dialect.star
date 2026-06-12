# Golden test that the compiled dialect (set + lambda + float + bitwise +
# nested def) works through starlight's explicit FileOptions.
s = set([3, 1, 2, 1])
out_set_len = len(s)                       # 3 (dedup)
out_lambda = (lambda x: x * 2)(21)         # 42
out_float = 1.5 * 2                         # 3.0
out_bitwise = 6 & 3                         # 2

def outer():
    def inner():
        return 7
    return inner()
out_nested = outer()                       # 7
