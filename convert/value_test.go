package convert_test

import (
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/1set/starlight/convert"
	startime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func TestToValue(t *testing.T) {
	aloha := "aloha!"
	number := 2023
	pi := 3.141592653589793
	yes := true
	bigVal := big.NewInt(1).Mul(big.NewInt(100000000000000), big.NewInt(100000000000000))
	now := time.Now()
	tests := []struct {
		name     string
		v        interface{}
		want     starlark.Value
		wantErr  bool
		strMatch bool
	}{
		{
			name:    "nil to none",
			v:       nil,
			want:    starlark.None,
			wantErr: false,
		},
		{
			name:    "typed nil to none",
			v:       (*string)(nil),
			wantErr: true,
		},
		{
			name:    "starlark typed nil to none",
			v:       (*starlark.Value)(nil),
			wantErr: true,
		},
		{
			name:    "starlark none value",
			v:       starlark.None,
			want:    starlark.None,
			wantErr: false,
		},
		{
			name:    "starlark string value",
			v:       starlark.String("test"),
			want:    starlark.String("test"),
			wantErr: false,
		},
		{
			name:    "starlark int value",
			v:       starlark.MakeInt(123),
			want:    starlark.MakeInt(123),
			wantErr: false,
		},
		{
			name:    "string to value",
			v:       "test",
			want:    starlark.String("test"),
			wantErr: false,
		},
		{
			name:    "int to value",
			v:       123,
			want:    starlark.MakeInt(123),
			wantErr: false,
		},
		{
			name:     "not big int to value",
			v:        big.NewInt(123),
			want:     starlark.MakeInt(123),
			strMatch: true,
		},
		{
			name:     "bigint to value",
			v:        bigVal,
			want:     starlark.MakeBigInt(bigVal),
			strMatch: true,
		},
		{
			name: "bigint to value as go struct",
			v:    bigVal,
			want: convert.NewStruct(bigVal),
		},
		{
			name: "pointer of bigint",
			v:    &bigVal,
			want: convert.MakeGoInterface(&bigVal),
		},
		{
			name:    "bool to value",
			v:       true,
			want:    starlark.Bool(true),
			wantErr: false,
		},
		{
			name:    "float to value",
			v:       123.45,
			want:    starlark.Float(123.45),
			wantErr: false,
		},
		{
			name:     "big float to value",
			v:        big.NewFloat(123.456),
			want:     starlark.Float(123.456),
			wantErr:  false,
			strMatch: true,
		},
		{
			name:     "slice to value",
			v:        []int{1, 2, 3},
			want:     convert.NewGoSlice([]int{1, 2, 3}),
			wantErr:  false,
			strMatch: true,
		},
		{
			name:     "pointer of slice",
			v:        &[]int{1, 2, 3},
			want:     convert.NewGoSlice([]int{1, 2, 3}),
			wantErr:  false,
			strMatch: true,
		},
		{
			name:     "array to value",
			v:        [3]int{1, 2, 3},
			want:     convert.NewGoSlice([]int{1, 2, 3}),
			wantErr:  false,
			strMatch: true,
		},
		{
			name:     "pointer of array",
			v:        &[3]int{1, 2, 3},
			want:     convert.NewGoSlice([]int{1, 2, 3}),
			wantErr:  false,
			strMatch: true,
		},
		{
			name:     "map to value",
			v:        map[string]int{"one": 1, "two": 2},
			want:     convert.NewGoMap(map[string]int{"one": 1, "two": 2}),
			wantErr:  false,
			strMatch: true,
		},
		{
			name:     "pointer of map",
			v:        &map[string]int{"one": 1, "two": 2},
			want:     convert.NewGoMap(map[string]int{"one": 1, "two": 2}),
			wantErr:  false,
			strMatch: true,
		},
		{
			name:     "map slice to value",
			v:        map[string][]int{"one": {1, 2}, "two": {3, 4}},
			want:     convert.NewGoMap(map[string][]int{"one": {1, 2}, "two": {3, 4}}),
			wantErr:  false,
			strMatch: true,
		},
		{
			name:    "empty struct to value",
			v:       struct{}{},
			want:    convert.NewStruct(struct{}{}),
			wantErr: false,
		},
		{
			name:     "custom struct to value",
			v:        struct{ Name string }{Name: "test"},
			want:     convert.NewStruct(struct{ Name string }{Name: "test"}),
			wantErr:  false,
			strMatch: true,
		},
		{
			name:     "lib struct to value",
			v:        big.NewRat(1, 3),
			want:     convert.NewStruct(big.NewRat(1, 3)),
			wantErr:  false,
			strMatch: true,
		},
		{
			name:     "function to value",
			v:        func() string { return "test" },
			want:     convert.MakeStarFn("fn", func() string { return "test" }),
			wantErr:  false,
			strMatch: true,
		},
		{
			name: "string pointer to value",
			v:    &aloha,
			want: starlark.String(aloha),
		},
		{
			name: "int pointer to value",
			v:    &number,
			want: starlark.MakeInt(2023),
		},
		{
			name: "bool pointer to value",
			v:    &yes,
			want: starlark.Bool(true),
		},
		{
			name: "float pointer to value",
			v:    &pi,
			want: starlark.Float(3.141592653589793),
		},
		{
			name: "custom starlark type dereferenced",
			v:    customType{},
			want: convert.NewStruct(customType{}),
		},
		{
			name: "custom starlark type",
			v:    &customType{},
			want: &customType{},
		},
		{
			name: "unknown struct",
			v:    unknownType{},
			want: convert.NewStruct(unknownType{}),
		},
		{
			name: "pointer of unknown struct",
			v:    &unknownType{},
			want: convert.NewStruct(&unknownType{}),
		},
		{
			name: "simple struct",
			v:    simpleType{},
			want: convert.NewStruct(simpleType{}),
		},
		{
			name: "pointer of simple struct",
			v:    &simpleType{},
			want: convert.NewStruct(&simpleType{}),
		},
		{
			name: "naive struct",
			v:    naiveType{},
			want: convert.NewStruct(naiveType{}),
		},
		{
			name:     "pointer of naive struct",
			v:        &naiveType{},
			want:     convert.NewStruct(&naiveType{}),
			strMatch: true,
		},
		{
			name: "starlark time",
			v:    now,
			want: startime.Time(now),
		},
		{
			name:    "unsupported type: channel",
			v:       make(chan int),
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convert.ToValue(tt.v)
			if (err != nil) != tt.wantErr {
				t.Errorf("ToValue(%v) error = %v, wantErr %v", tt.v, err, tt.wantErr)
				return
			}
			if !(reflect.DeepEqual(got, tt.want) || (tt.strMatch && (got.String() == tt.want.String()))) {
				t.Errorf("ToValue(%v) got = %v, want %v", tt.v, got, tt.want)
			}
		})
	}
}

func TestFromValue(t *testing.T) {
	slDict := starlark.NewDict(2)
	slDict.SetKey(starlark.String("a"), starlark.String("b"))

	slSet := starlark.NewSet(2)
	slSet.Insert(starlark.String("a"))
	slSet.Insert(starlark.String("b"))

	testBuiltin := convert.MakeStarFn("fn", func() string { return "test" })
	testFunction := getTestStarlarkFunc()
	testModule := starlarkstruct.Module{Name: "atest"}
	testStruct := starlarkstruct.Struct{}
	now := time.Now()

	bigVal := big.NewInt(1).Mul(big.NewInt(100000000000000), big.NewInt(100000000000000))

	tests := []struct {
		name string
		v    starlark.Value
		want interface{}
	}{
		{
			name: "Bool",
			v:    starlark.Bool(true),
			want: true,
		},
		{
			name: "Int",
			v:    starlark.MakeInt(123),
			want: int64(123),
		},
		{
			name: "Uint",
			v:    starlark.MakeUint64(uint64(18446744073709551615)),
			want: uint64(18446744073709551615),
		},
		{
			name: "BigInt",
			v:    starlark.MakeBigInt(bigVal),
			want: bigVal,
		},
		{
			name: "Float",
			v:    starlark.Float(1.23),
			want: float64(1.23),
		},
		{
			name: "String",
			v:    starlark.String("hello"),
			want: "hello",
		},
		{
			name: "Bytes",
			v:    starlark.Bytes("hello"),
			want: []byte("hello"),
		},
		{
			name: "List",
			v:    starlark.NewList([]starlark.Value{starlark.String("a"), starlark.String("b")}),
			want: []interface{}{"a", "b"},
		},
		{
			name: "Tuple",
			v:    starlark.Tuple([]starlark.Value{starlark.String("a"), starlark.String("b")}),
			want: []interface{}{"a", "b"},
		},
		{
			name: "Dict",
			v:    slDict,
			want: map[interface{}]interface{}{"a": "b"},
		},
		{
			name: "Set",
			v:    slSet,
			want: map[interface{}]bool{"a": true, "b": true},
		},
		{
			name: "None",
			v:    starlark.None,
			want: nil,
		},
		{
			name: "Time",
			v:    startime.Time(now),
			want: now,
		},
		// for GoStruct, GoInterface, GoMap, and GoSlice, we're assuming they just hold an interface{}
		{
			name: "GoStruct",
			v:    convert.NewStruct(customStruct{}),
			want: customStruct{},
		},
		{
			name: "GoInterface",
			v:    convert.MakeGoInterface(123),
			want: 123,
		},
		{
			name: "GoMap",
			v:    convert.NewGoMap(map[string]int{"a": 1}),
			want: map[string]int{"a": 1},
		},
		{
			name: "GoSlice",
			v:    convert.NewGoSlice([]int{1, 2, 3}),
			want: []int{1, 2, 3},
		},
		{
			name: "Default",
			v:    &customType{}, // assuming customType is a starlark.Value
			want: &customType{}, // assuming FromValue returns the original value if it doesn't know how to convert it
		},
		{
			name: "Custom",
			v:    &customType{},
			want: &customType{},
		},
		{
			name: "Builtin",
			v:    testBuiltin,
			want: testBuiltin,
		},
		{
			name: "Function",
			v:    testFunction,
			want: testFunction,
		},
		{
			name: "Module",
			v:    &testModule,
			want: &testModule,
		},
		{
			name: "Struct",
			v:    &testStruct,
			want: &testStruct,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := convert.FromValue(tt.v); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FromValue(%v) = %v, want %v", tt.v, got, tt.want)
			}
		})
	}
}

// Assuming this is a custom type that implements starlark.Value
type customType struct{}

func (c *customType) String() string        { return "customType" }
func (c *customType) Type() string          { return "customType" }
func (c *customType) Freeze()               {}
func (c *customType) Truth() starlark.Bool  { return starlark.True }
func (c *customType) Hash() (uint32, error) { return 0, nil }

// Assuming this is a custom type that doesn't implement starlark.Callable
type unknownType struct{}

// Assuming this is a custom type that doesn't implement starlark.Callable but its pointer has methods.
type simpleType struct{}

func (s *simpleType) Double(x int) int {
	return x * 2
}

// Assuming this is a custom type that doesn't implement starlark.Callable but has methods.
type naiveType struct {
	Runner string
}

func (s naiveType) Triple(x int) int {
	return x * 3
}

// Generate Starlark Functions

func getTestStarlarkFunc() *starlark.Function {
	code := `
def double(x):
	return x*2
`
	thread := &starlark.Thread{Name: "test"}
	globals, err := starlark.ExecFile(thread, "mock.star", code, nil)
	if err != nil {
		panic(err)
	}
	return globals["double"].(*starlark.Function)
}

func TestMakeDict(t *testing.T) {
	sd1 := starlark.NewDict(1)
	_ = sd1.SetKey(starlark.String("a"), starlark.String("b"))

	sd2 := starlark.NewDict(1)
	_ = sd2.SetKey(starlark.String("a"), starlark.MakeInt(1))

	vf3 := 2
	sd3 := starlark.NewDict(1)
	_ = sd3.SetKey(starlark.String("a"), starlark.Float(vf3))

	sd4 := starlark.NewDict(1)
	_ = sd4.SetKey(starlark.String("a"), convert.NewGoSlice([]string{"b", "c"}))

	sd5 := starlark.NewDict(1)
	_ = sd5.SetKey(starlark.String("a"), starlark.String("b"))

	sd6 := starlark.NewDict(1)
	_ = sd6.SetKey(starlark.MakeInt(10), starlark.Tuple{starlark.String("a")})

	tests := []struct {
		name     string
		v        interface{}
		want     starlark.Value
		wantErr  bool
		strMatch bool
	}{
		{
			name: "map[string]string",
			v:    map[string]string{"a": "b"},
			want: sd1,
		},
		{
			name: "map[string]int",
			v:    map[string]int{"a": 1},
			want: sd2,
		},
		{
			name: "map[string]float32",
			v:    map[string]float32{"a": float32(vf3)},
			want: sd3,
		},
		{
			name:     "map[string][]string",
			v:        map[string][]string{"a": {"b", "c"}},
			want:     sd4,
			strMatch: true,
		},
		{
			name:     "map[string]interface{}",
			v:        map[string]interface{}{"a": "b"},
			want:     sd5,
			strMatch: true,
		},
		{
			name: "map[starlark.String]starlark.String",
			v:    map[starlark.String]starlark.String{"a": starlark.String("b")},
			want: sd1,
		},
		{
			name: "map[starlark.Int]starlark.Tuple",
			v:    map[starlark.Int]starlark.Tuple{starlark.MakeInt(10): {starlark.String("a")}},
			want: sd6,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convert.MakeDict(tt.v)
			if (err != nil) != tt.wantErr {
				t.Errorf("MakeDict(%v) error = %v, wantErr %v", tt.v, err, tt.wantErr)
				return
			}
			if !(reflect.DeepEqual(got, tt.want) || (tt.strMatch && (got.String() == tt.want.String()))) {
				t.Errorf("MakeDict(%v) got = %v, want %v", tt.v, got, tt.want)
			}
		})
	}
}

func TestFromDict(t *testing.T) {
	sd1 := starlark.NewDict(1)
	_ = sd1.SetKey(starlark.String("a"), starlark.String("b"))

	sd2 := starlark.NewDict(1)
	_ = sd2.SetKey(starlark.String("a"), starlark.MakeInt(1))

	vf3 := 2
	sd3 := starlark.NewDict(1)
	_ = sd3.SetKey(starlark.String("a"), starlark.Float(vf3))

	sd4 := starlark.NewDict(1)
	_ = sd4.SetKey(starlark.String("a"), convert.NewGoSlice([]string{"b", "c"}))

	sd5 := starlark.NewDict(1)
	_ = sd5.SetKey(starlark.String("a"), convert.MakeGoInterface("b"))

	sd6 := starlark.NewDict(1)
	_ = sd6.SetKey(starlark.MakeInt(10), starlark.Tuple{starlark.String("a")})

	tests := []struct {
		name string
		v    *starlark.Dict
		want map[interface{}]interface{}
	}{
		{
			name: "map[string]string",
			v:    sd1,
			want: map[interface{}]interface{}{"a": "b"},
		},
		{
			name: "map[string]int64",
			v:    sd2,
			want: map[interface{}]interface{}{"a": int64(1)},
		},
		{
			name: "map[string]float32",
			v:    sd3,
			want: map[interface{}]interface{}{"a": float64(vf3)},
		},
		{
			name: "map[string][]string",
			v:    sd4,
			want: map[interface{}]interface{}{"a": []string{"b", "c"}},
		},
		{
			name: "map[string]interface{}",
			v:    sd5,
			want: map[interface{}]interface{}{"a": "b"},
		},
		{
			name: "map[starlark.String]starlark.String",
			v:    sd1,
			want: map[interface{}]interface{}{"a": "b"},
		},
		{
			name: "map[starlark.Int]starlark.Tuple",
			v:    sd6,
			want: map[interface{}]interface{}{int64(10): []interface{}{"a"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convert.FromDict(tt.v)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FromDict(%v) = %v, want %v", tt.v, got, tt.want)
			}
		})
	}
}

func TestMakeSet(t *testing.T) {
	s1 := starlark.NewSet(1)
	_ = s1.Insert(starlark.String("a"))

	s2 := starlark.NewSet(1)
	_ = s2.Insert(starlark.MakeInt(1))

	s3 := starlark.NewSet(2)
	_ = s3.Insert(starlark.String("a"))
	_ = s3.Insert(starlark.String("b"))

	tests := []struct {
		name    string
		s       map[interface{}]bool
		want    *starlark.Set
		wantErr bool
	}{
		{
			name: "set[string]",
			s:    map[interface{}]bool{"a": true},
			want: s1,
		},
		{
			name: "set[int]",
			s:    map[interface{}]bool{1: true},
			want: s2,
		},
		{
			name: "set[string,string]",
			s:    map[interface{}]bool{"a": true, "b": true},
			want: s3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convert.MakeSet(tt.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("MakeSet(%v) error = %v, wantErr %v", tt.s, err, tt.wantErr)
				return
			}
			if eq, err := starlark.Equal(got, tt.want); !eq || err != nil {
				t.Errorf("MakeSet(%v) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestFromSet(t *testing.T) {
	s1 := starlark.NewSet(1)
	_ = s1.Insert(starlark.String("a"))

	s2 := starlark.NewSet(1)
	_ = s2.Insert(starlark.MakeInt(200))

	s3 := starlark.NewSet(2)
	_ = s3.Insert(starlark.String("a"))
	_ = s3.Insert(starlark.String("b"))

	tests := []struct {
		name string
		s    *starlark.Set
		want map[interface{}]bool
	}{
		{
			name: "set[string]",
			s:    s1,
			want: map[interface{}]bool{"a": true},
		},
		{
			name: "set[int]",
			s:    s2,
			want: map[interface{}]bool{int64(200): true},
		},
		{
			name: "set[string,string]",
			s:    s3,
			want: map[interface{}]bool{"a": true, "b": true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convert.FromSet(tt.s)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FromSet(%v) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestFromTuple(t *testing.T) {
	tuple1 := starlark.Tuple{starlark.String("a")}
	tuple2 := starlark.Tuple{starlark.MakeInt(100)}
	tuple3 := starlark.Tuple{starlark.String("a"), starlark.String("b")}
	tests := []struct {
		name string
		v    starlark.Tuple
		want []interface{}
	}{
		{
			name: "tuple[string]",
			v:    tuple1,
			want: []interface{}{"a"},
		},
		{
			name: "tuple[int]",
			v:    tuple2,
			want: []interface{}{int64(100)},
		},
		{
			name: "tuple[string, string]",
			v:    tuple3,
			want: []interface{}{"a", "b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convert.FromTuple(tt.v)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FromTuple(%v) = %v, want %v", tt.v, got, tt.want)
			}
		})
	}
}

func TestFromList(t *testing.T) {
	l1 := starlark.NewList([]starlark.Value{starlark.String("a")})
	l2 := starlark.NewList([]starlark.Value{starlark.MakeInt(200)})
	l3 := starlark.NewList([]starlark.Value{starlark.String("a"), starlark.String("b")})
	tests := []struct {
		name string
		l    *starlark.List
		want []interface{}
	}{
		{
			name: "list[string]",
			l:    l1,
			want: []interface{}{"a"},
		},
		{
			name: "list[int]",
			l:    l2,
			want: []interface{}{int64(200)},
		},
		{
			name: "list[string, string]",
			l:    l3,
			want: []interface{}{"a", "b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convert.FromList(tt.l)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FromList(%v) = %v, want %v", tt.l, got, tt.want)
			}
		})
	}
}

func TestGoTypeWrapperValue(t *testing.T) {
	type HasValue interface {
		Value() reflect.Value
	}
	tests := []struct {
		name  string
		input starlark.Value
		want  interface{}
	}{
		{
			name:  "Map",
			input: convert.NewGoMap(map[string]int{"a": 1}),
			want:  map[string]int{"a": 1},
		},
		{
			name:  "Slice",
			input: convert.NewGoSlice([]int{1, 2, 3}),
			want:  []int{1, 2, 3},
		},
		{
			name:  "Struct",
			input: convert.NewStruct(struct{ A int }{A: 1}),
			want:  struct{ A int }{A: 1},
		},
		{
			name:  "Interface",
			input: convert.MakeGoInterface(100),
			want:  100,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := tt.input.(HasValue)
			if !ok {
				t.Errorf("input(%v) doesn't have Value(): %v", tt.name, tt.input)
				return
			}
			got := v.Value().Interface()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GoTypeWrapper(%v) got = %v, want = %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestMakeDictWithTag(t *testing.T) {
	type contact struct {
		Name   string `sl:"name"`
		Street string `sl:"address,omitempty"`
	}
	type testCase struct {
		name        string
		data        interface{}
		codeSnippet string
		customTag   string
		wantErrConv bool
		wantErrExec bool
	}
	testCases := []testCase{
		{
			name: "1map[string]int",
			data: map[string]int{
				"a": 1,
				"b": 2,
			},
			codeSnippet: `
assert.Eq(type(data), "dict")
assert.Eq(type(data.keys()[0]), "string")
a = data["a"]
b = data["b"]
assert.Eq(type(a), "int")
assert.Eq(type(b), "int")
assert.Eq(a, 1)
assert.Eq(b, 2)
`,
		},
		{
			name: "1map[string]interface{}",
			data: map[string]interface{}{
				"a": 1,
				"b": 2,
			},
			codeSnippet: `
assert.Eq(type(data), "dict")
assert.Eq(type(data.keys()[0]), "string")
a = data["a"]
b = data["b"]
assert.Eq(type(a), "int")
assert.Eq(type(b), "int")
assert.Eq(a, 1)
assert.Eq(b, 2)
`,
		},
		{
			name: "1map[interface{}]int",
			data: map[interface{}]int{
				"a": 1,
				"b": 2,
			},
			codeSnippet: `
assert.Eq(type(data), "dict")
assert.Eq(type(data.keys()[0]), "string")
a = data["a"]
b = data["b"]
assert.Eq(type(a), "int")
assert.Eq(type(b), "int")
assert.Eq(a, 1)
assert.Eq(b, 2)
`,
		},
		{
			name: "1map[interface{}]interface{}",
			data: map[interface{}]interface{}{
				"a": 1,
				"b": 2,
			},
			codeSnippet: `
assert.Eq(type(data), "dict")
assert.Eq(type(data.keys()[0]), "string")
a = data["a"]
b = data["b"]
assert.Eq(type(a), "int")
assert.Eq(type(b), "int")
assert.Eq(a, 1)
assert.Eq(b, 2)
`,
		},
		{
			name: "2map[int]int",
			data: map[int]int{
				100: 1,
				200: 2,
			},
			codeSnippet: `
assert.Eq(type(data), "dict")
assert.Eq(type(data.keys()[0]), "int")
a = data[100]
b = data[200]
assert.Eq(type(a), "int")
assert.Eq(type(b), "int")
assert.Eq(a, 1)
assert.Eq(b, 2)
`,
		},
		{
			name: "2map[int]interface{}",
			data: map[int]interface{}{
				100: 1,
				200: 2,
				300: 3.5,
			},
			codeSnippet: `
assert.Eq(type(data), "dict")
assert.Eq(type(data.keys()[0]), "int")
a = data[100]
b = data[200]
c = data[300]
assert.Eq(type(a), "int")
assert.Eq(type(b), "int")
assert.Eq(type(c), "float")
assert.Eq(a, 1)
assert.Eq(b, 2)
assert.Eq(c, 3.5)
`,
		},
		{
			name: "2map[interface{}]interface{}",
			data: map[interface{}]interface{}{
				100: 1,
				200: 2,
				300: 3.5,
			},
			codeSnippet: `
assert.Eq(type(data), "dict")
assert.Eq(type(data.keys()[0]), "int")
a = data[100]
b = data[200]
c = data[300]
assert.Eq(type(a), "int")
assert.Eq(type(b), "int")
assert.Eq(type(c), "float")
assert.Eq(a, 1)
assert.Eq(b, 2)
assert.Eq(c, 3.5)
`,
		},
		{
			name: "3map[interface{}]interface{}",
			data: map[interface{}]interface{}{
				true:  1,
				false: 3.5,
			},
			codeSnippet: `
assert.Eq(type(data), "dict")
ks = data.keys()
assert.Eq(type(ks[0]), "bool")
assert.Eq(type(ks[1]), "bool")
a = data[True]
b = data[False]
assert.Eq(type(a), "int")
assert.Eq(type(b), "float")
assert.Eq(a, 1)
assert.Eq(b, 3.5)
`,
		},
		{
			name: "4map[interface{}]interface{}",
			data: map[interface{}]interface{}{
				"A": &contact{Name: "bob", Street: "oak"},
			},
			codeSnippet: `
assert.Eq(type(data), "dict")
a = data["A"]
assert.Eq(a.Name, "bob")
assert.Eq(a.Street, "oak")
assert.Eq(type(a), "starlight_struct<*convert_test.contact>")
assert.Eq(dir(a), ["Name", "Street"])
`,
		},
		{
			name: "5map[interface{}]interface{}",
			data: map[interface{}]interface{}{
				"A": &contact{Name: "bob", Street: "oak"},
			},
			customTag: `sl`,
			codeSnippet: `
assert.Eq(type(data), "dict")
a = data["A"]
assert.Eq(a.name, "bob")
assert.Eq(a.address, "oak")
assert.Eq(type(a), "starlight_struct<*convert_test.contact>")
assert.Eq(dir(a), ["address", "name"])
`,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gs := map[string]interface{}{
				"assert": &assert{t: t},
				"i1":     int64(1),
			}
			envs, err := convert.MakeStringDict(gs)
			if err != nil {
				t.Fatalf(`expected no error while converting globals, but got %v`, err)
			}

			// convert go values to Starlark values as predefined globals
			var (
				cv      starlark.Value
				errConv error
			)
			if tc.customTag != "" {
				cv, errConv = convert.MakeDictWithTag(tc.data, tc.customTag)
			} else {
				cv, errConv = convert.MakeDict(tc.data)
			}
			if errConv != nil == !tc.wantErrConv {
				t.Fatalf(`expected no error while converting dict, but got %v`, errConv)
			} else if errConv == nil && tc.wantErrConv {
				t.Fatalf(`expected an error while converting dict, but got none`)
			}
			if errConv != nil {
				return
			}

			// run the Starlark code to test the converted globals
			envs["data"] = cv
			_, errExec := execStarlark(tc.codeSnippet, envs)
			if errExec != nil && !tc.wantErrExec {
				t.Fatalf(`expected no error while executing code snippet, but got %v`, errExec)
			} else if errExec == nil && tc.wantErrExec {
				t.Fatalf(`expected an error while executing code snippet, but got none`)
			}
		})
	}
}
