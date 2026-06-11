package convert_test

import (
	"fmt"
	"testing"

	"github.com/1set/starlight/convert"
	"go.starlark.net/starlark"
)

// Benchmarks for the conversion hot paths touched by the panic/determinism
// fixes: key materialization (sorted snapshots), String() (cycle pre-scan),
// map get/set (key conversion pre-checks), and collection wrapping (static
// element type checks).

func benchMap(n int) map[string]int {
	m := make(map[string]int, n)
	for i := 0; i < n; i++ {
		m[fmt.Sprintf("key%04d", i)] = i
	}
	return m
}

func BenchmarkGoMapKeys50(b *testing.B) {
	g := convert.NewGoMap(benchMap(50))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = g.Keys()
	}
}

func BenchmarkGoMapItems50(b *testing.B) {
	g := convert.NewGoMap(benchMap(50))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = g.Items()
	}
}

func BenchmarkGoMapString50(b *testing.B) {
	g := convert.NewGoMap(benchMap(50))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = g.String()
	}
}

func BenchmarkGoSliceString1k(b *testing.B) {
	s := make([]int, 1000)
	for i := range s {
		s[i] = i
	}
	g := convert.NewGoSlice(s)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = g.String()
	}
}

func BenchmarkGoMapSetGet(b *testing.B) {
	g := convert.NewGoMap(map[string]int{})
	k := starlark.String("key")
	v := starlark.MakeInt(42)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := g.SetKey(k, v); err != nil {
			b.Fatal(err)
		}
		if _, _, err := g.Get(k); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkToValueMap50(b *testing.B) {
	m := benchMap(50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := convert.ToValue(m); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFromDict50(b *testing.B) {
	d := starlark.NewDict(50)
	for i := 0; i < 50; i++ {
		if err := d.SetKey(starlark.String(fmt.Sprintf("key%04d", i)), starlark.MakeInt(i)); err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = convert.FromDict(d)
	}
}
