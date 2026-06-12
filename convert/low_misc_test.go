package convert_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/1set/starlight"
	"github.com/1set/starlight/convert"
	startime "go.starlark.net/lib/time"
)

// TestPointerToTimeTime: a *time.Time reached the struct case still as a
// pointer and was wrapped as a GoStruct instead of the Starlark time type.
func TestPointerToTimeTime(t *testing.T) {
	now := time.Date(2026, 6, 12, 1, 2, 3, 0, time.UTC)
	v, err := convert.ToValue(&now)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := v.(startime.Time); !ok {
		t.Fatalf("expected startime.Time for *time.Time, got %T", v)
	}
	globals := map[string]interface{}{
		"assert": &assert{t: t},
		"t":      &now,
	}
	if _, err := starlight.Eval([]byte(`
assert.Eq(type(t), "time.time")
assert.Eq(t.year, 2026)
`), globals, nil); err != nil {
		t.Fatal(err)
	}
}

// TestNewStructNilMessage: NewStruct/NewStructWithTag formatted their panic
// with val.Interface(), which panics again on a nil (Invalid) arg. The panic
// must carry a clean "<nil>".
func TestNewStructNilMessage(t *testing.T) {
	for _, fn := range []func(){
		func() { convert.NewStruct(nil) },
		func() { convert.NewStructWithTag(nil, "tag") },
	} {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("expected panic for nil arg")
				}
				msg := fmt.Sprint(r)
				if strings.Contains(msg, "zero Value") {
					t.Fatalf("panic-message formatting itself panicked: %v", r)
				}
				if !strings.Contains(msg, "<nil>") {
					t.Fatalf("expected '<nil>' in panic, got %v", r)
				}
			}()
			fn()
		}()
	}
}
