package convert

import (
	"reflect"
	"testing"
)

// TestGoInterfaceTruthNilable: Truth returned a blanket true for kinds it
// didn't enumerate, so a nil chan/map/func/slice wrapped in a GoInterface
// reported truthy. Nilable kinds should reflect their nil-ness.
func TestGoInterfaceTruthNilable(t *testing.T) {
	cases := []struct {
		v    interface{}
		want bool
	}{
		{(chan int)(nil), false},
		{make(chan int), true},
		{map[string]int(nil), false},
		{map[string]int{"a": 1}, true},
		{([]int)(nil), false},
		{[]int{1}, true},
		{(func())(nil), false},
	}
	for _, c := range cases {
		gi := &GoInterface{v: reflect.ValueOf(c.v)}
		if got := bool(gi.Truth()); got != c.want {
			t.Errorf("Truth(%T nil=%v) = %v, want %v", c.v, reflect.ValueOf(c.v).IsNil(), got, c.want)
		}
	}
}
