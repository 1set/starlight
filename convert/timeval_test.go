package convert_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/1set/starlight"
	"github.com/1set/starlight/convert"
	startime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
)

// TestDurationSymmetry verifies time.Duration maps bidirectionally to the
// standard Starlark time.duration. It used to convert one way only: into
// an opaque interface wrapper that scripts could not use as a duration,
// while time.duration values coming back were never unwrapped.
func TestDurationSymmetry(t *testing.T) {
	d := 90 * time.Second

	v, err := convert.ToValue(d)
	if err != nil {
		t.Fatal(err)
	}
	sd, ok := v.(startime.Duration)
	if !ok {
		t.Fatalf("expected startime.Duration, got %T", v)
	}
	if time.Duration(sd) != d {
		t.Fatalf("expected %v, got %v", d, time.Duration(sd))
	}

	back := convert.FromValue(v)
	gd, ok := back.(time.Duration)
	if !ok || gd != d {
		t.Fatalf("expected round-trip %v, got %v (%T)", d, back, back)
	}

	// script-visible: it is a real time.duration, with duration semantics
	globals := map[string]interface{}{"d": d}
	res, err := starlight.Eval([]byte(`
t = type(d)
double = d + d
seconds = d.seconds
`), globals, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res["t"] != "time.duration" {
		t.Fatalf("expected time.duration, got %v", res["t"])
	}
	if dd, ok := res["double"].(time.Duration); !ok || dd != 3*time.Minute {
		t.Fatalf("expected 3m, got %v (%T)", res["double"], res["double"])
	}
	if res["seconds"] != float64(90) {
		t.Fatalf("expected 90 seconds, got %v", res["seconds"])
	}

	// duration arguments convert back for Go functions
	var got time.Duration
	globals2 := map[string]interface{}{
		"d":   d,
		"fnD": func(in time.Duration) { got = in },
	}
	if _, err := starlight.Eval([]byte(`fnD(d)`), globals2, nil); err != nil {
		t.Fatal(err)
	}
	if got != d {
		t.Fatalf("expected %v, got %v", d, got)
	}
}

// TestIntLadderContract pins the documented FromValue integer ladder:
// int64 if it fits, else uint64, else *big.Int.
func TestIntLadderContract(t *testing.T) {
	small := convert.FromValue(starlark.MakeInt(42))
	if v, ok := small.(int64); !ok || v != 42 {
		t.Fatalf("expected int64 42, got %v (%T)", small, small)
	}

	negative := convert.FromValue(starlark.MakeInt(-42))
	if v, ok := negative.(int64); !ok || v != -42 {
		t.Fatalf("expected int64 -42, got %v (%T)", negative, negative)
	}

	wide := convert.FromValue(starlark.MakeUint64(10000000000000000000)) // > MaxInt64
	if v, ok := wide.(uint64); !ok || v != 10000000000000000000 {
		t.Fatalf("expected uint64 1e19, got %v (%T)", wide, wide)
	}

	huge, ok := new(big.Int).SetString("36893488147419103232", 10) // 2^65
	if !ok {
		t.Fatal("bad big literal")
	}
	b := convert.FromValue(starlark.MakeBigInt(huge))
	if v, ok := b.(*big.Int); !ok || v.Cmp(huge) != 0 {
		t.Fatalf("expected *big.Int 2^65, got %v (%T)", b, b)
	}
}
