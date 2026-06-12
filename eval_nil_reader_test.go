package starlight

import (
	"bytes"
	"testing"
)

// TestEvalTypedNilReader: a typed-nil io.Reader passed as src (a host
// mistake) reached the interpreter's source reader and panicked. It must
// return a clean error instead.
func TestEvalTypedNilReader(t *testing.T) {
	var r *bytes.Buffer // typed-nil, but satisfies io.Reader
	_, err := Eval(r, nil, nil)
	if err == nil {
		t.Fatal("expected an error for a typed-nil reader, not a panic")
	}
}
