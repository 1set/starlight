package convert

import (
	"reflect"
	"strings"
	"testing"
)

// TestStableKeyString covers every kind branch of the address-free key
// renderer and asserts equal values render equally (no addresses leak).
func TestStableKeyString(t *testing.T) {
	type inner struct {
		B bool
		F float64
	}
	type composite struct {
		N int
		U uint8
		S string
		P *int
		C complex128
		A [2]int
		I inner
	}
	mk := func() composite {
		x := 7
		return composite{N: -3, U: 200, S: "k", P: &x, C: 1 + 2i, A: [2]int{4, 5}, I: inner{B: true, F: 1.5}}
	}
	a, b := mk(), mk() // equal values, different *int addresses
	sa := stableKeyString(reflect.ValueOf(a))
	sb := stableKeyString(reflect.ValueOf(b))
	if sa != sb {
		t.Fatalf("equal values must render equally:\n a=%s\n b=%s", sa, sb)
	}
	for _, want := range []string{"-3", "200", "k", "7", "true", "1.5", "[4 5]"} {
		if !strings.Contains(sa, want) {
			t.Errorf("rendered key %q missing %q", sa, want)
		}
	}
	// nil pointer and channel branches
	if got := stableKeyString(reflect.ValueOf((*int)(nil))); got != "<nil>" {
		t.Errorf("nil ptr -> %q, want <nil>", got)
	}
	if got := stableKeyString(reflect.ValueOf(make(chan int))); got != "<chan>" {
		t.Errorf("chan -> %q, want <chan>", got)
	}
}
