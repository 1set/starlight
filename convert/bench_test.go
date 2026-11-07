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

func BenchmarkGoMapClear50(b *testing.B) {
	seed := benchMap(50)
	m := make(map[string]int, len(seed))
	g := convert.NewGoMap(m)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for k, v := range seed {
			m[k] = v
		}
		if err := g.Clear(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGoMapPopitem50(b *testing.B) {
	g := convert.NewGoMap(benchMap(50))
	pop, err := g.Attr("popitem")
	if err != nil {
		b.Fatal(err)
	}
	thread := &starlark.Thread{Name: "benchmark"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, err := starlark.Call(thread, pop, nil, nil)
		if err != nil {
			b.Fatal(err)
		}
		pair := v.(starlark.Tuple)
		if err := g.SetKey(pair[0], pair[1]); err != nil {
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

func BenchmarkMakeDict50(b *testing.B) {
	m := benchMap(50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := convert.MakeDict(m); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMapArgumentConversion50(b *testing.B) {
	m, err := convert.ToValue(benchMap(50))
	if err != nil {
		b.Fatal(err)
	}
	fn := convert.MakeStarFn("accept", func(values map[string]int64) {})
	thread := &starlark.Thread{}
	args := starlark.Tuple{m}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := starlark.Call(thread, fn, args, nil); err != nil {
			b.Fatal(err)
		}
	}
}

// Additional merge-gate benchmarks share identical source in both checkouts.
// Fixtures are acyclic, normally representable values; regressions exercise
// the public wrappers rather than a mirror of their implementation.
var foundationValue starlark.Value
var foundationValues []starlark.Value
var foundationItems []starlark.Tuple
var foundationText string

func BenchmarkFoundationMapScale(b *testing.B) {
	for _, n := range []int{1, 1000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			m := benchMap(n)
			g := convert.NewGoMap(m)
			b.Run("Keys", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					foundationValues = g.Keys()
				}
			})
			b.Run("Items", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					foundationItems = g.Items()
				}
			})
			b.Run("Iterate", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					it := g.Iterate()
					var v starlark.Value
					for it.Next(&v) {
						foundationValue = v
					}
					it.Done()
				}
			})
			b.Run("String", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					foundationText = g.String()
				}
			})
			b.Run("MakeDict", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					v, e := convert.MakeDict(m)
					if e != nil {
						b.Fatal(e)
					}
					foundationValue = v
				}
			})
		})
	}
}
func BenchmarkFoundationMapIterate50(b *testing.B) {
	g := convert.NewGoMap(benchMap(50))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		it := g.Iterate()
		var v starlark.Value
		for it.Next(&v) {
			foundationValue = v
		}
		it.Done()
	}
}
func BenchmarkFoundationMapReadWrite(b *testing.B) {
	g := convert.NewGoMap(benchMap(50))
	hit := starlark.String("key0001")
	miss := starlark.String("missing")
	value := starlark.MakeInt(42)
	b.Run("GetHit", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			v, ok, e := g.Get(hit)
			if e != nil || !ok {
				b.Fatal(e)
			}
			foundationValue = v
		}
	})
	b.Run("GetMiss", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, ok, e := g.Get(miss)
			if e != nil || ok {
				b.Fatal(e)
			}
		}
	})
	b.Run("Set", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if e := g.SetKey(hit, value); e != nil {
				b.Fatal(e)
			}
		}
	})
}
func BenchmarkFoundationMapPointerKeys50(b *testing.B) {
	m := make(map[*int]int, 50)
	for i := 0; i < 50; i++ {
		p := new(int)
		*p = i
		m[p] = i
	}
	g := convert.NewGoMap(m)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		foundationValues = g.Keys()
	}
}
func BenchmarkFoundationMapPointerValues50(b *testing.B) {
	m := make(map[string]*int, 50)
	for k, v := range benchMap(50) {
		p := new(int)
		*p = v
		m[k] = p
	}
	g := convert.NewGoMap(m)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		foundationItems = g.Items()
	}
}
func BenchmarkFoundationMapStringReferences50(b *testing.B) {
	m := make(map[string]interface{}, 50)
	for k, v := range benchMap(50) {
		m[k] = []int{v, v + 1}
	}
	g := convert.NewGoMap(m)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		foundationText = g.String()
	}
}
func BenchmarkFoundationSliceMaterialize(b *testing.B) {
	values := make([]int, 50)
	ptrs := make([]*int, 50)
	ifaces := make([]*interface{}, 50)
	for i := range values {
		values[i] = i
		ptrs[i] = &values[i]
		v := interface{}(i)
		ifaces[i] = &v
	}
	for _, fixture := range []struct {
		name  string
		value interface{}
	}{{"Int", values}, {"IntPointer", ptrs}, {"InterfacePointer", ifaces}} {
		b.Run(fixture.name, func(b *testing.B) {
			g := convert.NewGoSlice(fixture.value)
			b.Run("Index", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					foundationValue = g.Index(i % 50)
				}
			})
			b.Run("Iterate50", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					it := g.Iterate()
					var v starlark.Value
					for it.Next(&v) {
						foundationValue = v
					}
					it.Done()
				}
			})
		})
	}
}
func BenchmarkFoundationMapClear1000(b *testing.B) {
	seed := benchMap(1000)
	m := make(map[string]int, len(seed))
	g := convert.NewGoMap(m)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for k, v := range seed {
			m[k] = v
		}
		if e := g.Clear(); e != nil {
			b.Fatal(e)
		}
	}
}

// Normal Clear costs for key kinds that can contain NaN use only reflexive
// inputs, so baseline and candidate operations remain comparable. Refill is
// included identically in both legs, as in BenchmarkGoMapClear50.
func BenchmarkFoundationMapClearFloat50(b *testing.B) {
	seed := make(map[float64]int, 50)
	for i := 0; i < 50; i++ {
		seed[float64(i)] = i
	}
	m := make(map[float64]int, len(seed))
	g := convert.NewGoMap(m)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for k, v := range seed {
			m[k] = v
		}
		if err := g.Clear(); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkFoundationMapClearInterface50(b *testing.B) {
	seed := make(map[interface{}]int, 50)
	for k, v := range benchMap(50) {
		seed[k] = v
	}
	m := make(map[interface{}]int, len(seed))
	g := convert.NewGoMap(m)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for k, v := range seed {
			m[k] = v
		}
		if err := g.Clear(); err != nil {
			b.Fatal(err)
		}
	}
}

// Existing public function-call benchmark, copied unchanged from extern_test.go.
type foo struct {
	name string
}

func (f *foo) Name() string {
	return f.name
}

var resultFuncCall starlark.StringDict
var result string

func BenchmarkFuncCall(b *testing.B) {
	fn := func(s string) {
		result = s
	}

	globals := map[string]interface{}{
		"fn":  fn,
		"foo": &foo{name: "bob"},
	}
	dict, err := convert.MakeStringDict(globals)
	if err != nil {
		b.Fatal(err)
	}
	code := []byte(`fn(foo.Name())`)
	// precompile
	_, p, err := starlark.SourceProgram("foo.star", code, dict.Has)
	if err != nil {
		b.Fatal(err)
	}

	for n := 0; n < b.N; n++ {
		dict, err := convert.MakeStringDict(globals)
		if err != nil {
			b.Fatal(err)
		}
		resultFuncCall, err = p.Init(new(starlark.Thread), dict)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Auxiliary composite-key Clear costs are reported separately from the38.
func BenchmarkFoundationMapClearArray50(b *testing.B) {
	seed := make(map[[2]float64]int, 50)
	for i := 0; i < 50; i++ {
		seed[[2]float64{float64(i), float64(i + 1)}] = i
	}
	m := make(map[[2]float64]int, len(seed))
	g := convert.NewGoMap(m)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for k, v := range seed {
			m[k] = v
		}
		if err := g.Clear(); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkFoundationMapClearStruct50(b *testing.B) {
	type key struct {
		F float64
		N int
	}
	seed := make(map[key]int, 50)
	for i := 0; i < 50; i++ {
		seed[key{float64(i), i}] = i
	}
	m := make(map[key]int, len(seed))
	g := convert.NewGoMap(m)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for k, v := range seed {
			m[k] = v
		}
		if err := g.Clear(); err != nil {
			b.Fatal(err)
		}
	}
}
