package convert_test

import (
	"testing"

	"github.com/1set/starlight/convert"
)

type methodInt int

func (m methodInt) Marker() {}

// TestGoInterfaceNilPointerConversions: ToInt/ToBool/ToUint dereferenced a
// nil pointer to a zero reflect.Value, then formatted the error with
// v.Interface() — which panics on a zero Value. They must return a clean
// error (like ToString/ToFloat, which use the reflect.Value directly).
func TestGoInterfaceNilPointerConversions(t *testing.T) {
	g := convert.MakeGoInterface((*methodInt)(nil))
	if _, err := g.ToInt(); err == nil {
		t.Fatal("ToInt on a nil typed pointer should error, not panic")
	}
	if _, err := g.ToBool(); err == nil {
		t.Fatal("ToBool on a nil typed pointer should error, not panic")
	}
	if _, err := g.ToUint(); err == nil {
		t.Fatal("ToUint on a nil typed pointer should error, not panic")
	}
}
