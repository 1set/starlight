package convert_test

import (
	"fmt"
	"testing"

	"github.com/1set/starlight"
	"github.com/1set/starlight/convert"
)

func TestSliceTruth(t *testing.T) {
	empty := []string{}
	full := []bool{false}

	globals := map[string]interface{}{
		"empty": empty,
		"full":  full,
		"fail":  t.Fatal,
	}

	code := []byte(`
def run():
	if empty:
		fail("empty slice should be false")
	if not full:
		fail("non-empty slice should be true")
run()
`)
	_, err := starlight.Eval(code, globals, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSliceIndexing(t *testing.T) {
	abc := []string{
		"a", "b", "c",
	}

	globals := map[string]interface{}{
		"assert": &assert{t: t},
		"abc":    abc,
	}

	code := []byte(`
# indexing, x[i]
assert.Eq(abc[-3], "a")
assert.Eq(abc[-2], "b")
assert.Eq(abc[-1], "c")
assert.Eq(abc[0], "a")
assert.Eq(abc[1], "b")
assert.Eq(abc[2], "c")
`)

	_, err := starlight.Eval(code, globals, nil)
	if err != nil {
		t.Fatal(err)
	}
	tests := []fail{
		{"abc[3]", "starlight_slice<[]string> index 3 out of range [-3:2]"},
		{"abc[-4]", "starlight_slice<[]string> index -4 out of range [-3:2]"},
		{"abc[0.0]", "starlight_slice<[]string> index: got float, want int"},
		{`abc["a"]`, "starlight_slice<[]string> index: got string, want int"},
		{"abc[0, 1]", "starlight_slice<[]string> index: got tuple, want int"},
		{`abc[0] = True`, "index: value of type bool cannot be converted to type string"},
		{`abc[None]`, "starlight_slice<[]string> index: got NoneType, want int"},
	}
	expectFails(t, tests, globals)
}

func intSlice(vals []interface{}) ([]int, error) {
	ret := make([]int, len(vals))
	for i, v := range vals {
		x, ok := v.(int64)
		if !ok {
			return nil, fmt.Errorf("expected int64 but got %v (%T)", v, v)
		}
		ret[i] = int(x)
	}
	return ret, nil
}

func TestSliceIndexAssign(t *testing.T) {
	x3 := []int{0, 1, 2}

	globals := map[string]interface{}{
		"assert":   &assert{t: t},
		"x3":       x3,
		"intSlice": intSlice,
	}

	code := []byte(`
x3[1] = 2
x3[2] += 3
assert.Eq(x3, intSlice([0, 2, 5]))
`)

	_, err := starlight.Eval(code, globals, nil)
	if err != nil {
		t.Fatal(err)
	}

	v, err := convert.ToValue(x3)
	if err != nil {
		t.Fatal(err)
	}
	v.Freeze()

	globals["x3"] = v

	tests := []fail{
		{"x3[3]=4", "starlight_slice<[]int> index 3 out of range [-3:2]"},
		{"x3[0]=0", "cannot assign to frozen slice"},
		{"x3.clear()", "cannot clear frozen slice"},
	}
	expectFails(t, tests, globals)
}

func TestSliceComprehensions(t *testing.T) {
	x3 := []int{1, 2, 3}

	globals := map[string]interface{}{
		"assert":   &assert{t: t},
		"x3":       x3,
		"intSlice": intSlice,
	}

	code := []byte(`
assert.Eq([2 * x for x in x3], [2, 4, 6])
`)
	_, err := starlight.Eval(code, globals, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSliceAppend(t *testing.T) {
	x3 := []int{1, 2, 3}

	globals := map[string]interface{}{
		"assert":   &assert{t: t},
		"x3":       x3,
		"intSlice": intSlice,
	}

	code := []byte(`
x3.append(4)
x3.append(5)
x3.append(6)
assert.Eq(x3, intSlice([1, 2, 3, 4, 5, 6]))
`)
	_, err := starlight.Eval(code, globals, nil)
	if err != nil {
		t.Fatal(err)
	}

	code = []byte(`x3.append("test")`)
	_, err = starlight.Eval(code, globals, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSliceExtend(t *testing.T) {
	x3 := []int{1, 2, 3}
	globals := map[string]interface{}{
		"assert":   &assert{t: t},
		"x3":       x3,
		"intSlice": intSlice,
	}

	code := []byte(`
x3.extend([4,5,6])
assert.Eq(x3, intSlice([1, 2, 3, 4, 5, 6]))
`)
	_, err := starlight.Eval(code, globals, nil)
	if err != nil {
		t.Fatal(err)
	}

	code = []byte(`x3.extend("test")`)
	_, err = starlight.Eval(code, globals, nil)
	expectErr(t, err, "argument is not iterable: test (starlark.String)")

	code = []byte(`x3.extend(["a", "b", "c"])`)
	_, err = starlight.Eval(code, globals, nil)
	expectErr(t, err, "extend: value of type string cannot be converted to type int")
}

func TestSliceIndex(t *testing.T) {
	bananas := []string{"b", "a", "n", "a", "n", "a", "s"}

	globals := map[string]interface{}{
		"assert":  &assert{t: t},
		"bananas": bananas,
	}

	code := []byte(`
assert.Eq(bananas.index('a'), 1) # bAnanas
# start
assert.Eq(bananas.index('a', -1000), 1) # bAnanas
assert.Eq(bananas.index('a', 0), 1)     # bAnanas
assert.Eq(bananas.index('a', 1), 1)     # bAnanas
assert.Eq(bananas.index('a', 2), 3)     # banAnas
assert.Eq(bananas.index('a', 3), 3)     # banAnas
assert.Eq(bananas.index('b', 0), 0)     # Bananas
assert.Eq(bananas.index('n', -3), 4)    # banaNas
assert.Eq(bananas.index('s', -2), 6)    # bananaS
# start, end
assert.Eq(bananas.index('s', -1000, 7), 6) # bananaS
`)
	_, err := starlight.Eval(code, globals, nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := []fail{
		{`bananas.index('b', 1)`, `index: value "b" not in list`},
		{`bananas.index('n', -2)`, `index: value "n" not in list`},
		{`bananas.index('d')`, `index: value "d" not in list`},
		{`bananas.index('s', -1000, 6)`, `index: value "s" not in list`},
		{`bananas.index('d', -1000, 1000)`, `index: value "d" not in list`},
		{`bananas.index([], 0, 0)`, `index: value of type []interface {} cannot be converted to type string`},
		{`bananas.index(None, 0, 0)`, `index: value of type None cannot be converted to non-nullable type string`},
	}
	expectFails(t, tests, globals)
}

func TestSliceFind(t *testing.T) {
	bananas := []string{"b", "a", "n", "a", "n", "a", "s"}

	globals := map[string]interface{}{
		"assert":  &assert{t: t},
		"bananas": bananas,
	}

	code := []byte(`
assert.Eq(bananas.find('a'), 1) # bAnanas
# start
assert.Eq(bananas.find('a', -1000), 1) # bAnanas
assert.Eq(bananas.find('a', 0), 1)     # bAnanas
assert.Eq(bananas.find('a', 1), 1)     # bAnanas
assert.Eq(bananas.find('a', 2), 3)     # banAnas
assert.Eq(bananas.find('a', 3), 3)     # banAnas
assert.Eq(bananas.find('b', 0), 0)     # Bananas
assert.Eq(bananas.find('n', -3), 4)    # banaNas
assert.Eq(bananas.find('s', -2), 6)    # bananaS
# start, end
assert.Eq(bananas.find('s', -1000, 7), 6) # bananaS
# not found
assert.Eq(bananas.find('b', 1), -1)
assert.Eq(bananas.find('n', -2), -1)
assert.Eq(bananas.find('d'), -1)
assert.Eq(bananas.find('s', -1000, 6), -1)
assert.Eq(bananas.find('d', -1000, 1000), -1)
`)
	_, err := starlight.Eval(code, globals, nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := []fail{
		{`bananas.find([], 0, 0)`, `find: value of type []interface {} cannot be converted to type string`},
		{`bananas.find(None, 0, 0)`, `find: value of type None cannot be converted to non-nullable type string`},
	}
	expectFails(t, tests, globals)
}

func TestSliceInsert(t *testing.T) {
	globals := map[string]interface{}{
		"assert":   &assert{t: t},
		"intSlice": intSlice,
	}

	code := []byte(`
def insert_at(index):
	x = intSlice([0,1,2])
	x.insert(index, 42)
	return x
assert.Eq(insert_at(-99), intSlice([42, 0, 1, 2]))
assert.Eq(insert_at(-2), intSlice([0, 42, 1, 2]))
assert.Eq(insert_at(-1), intSlice([0, 1, 42, 2]))
assert.Eq(insert_at( 0), intSlice([42, 0, 1, 2]))
assert.Eq(insert_at( 1), intSlice([0, 42, 1, 2]))
assert.Eq(insert_at( 2), intSlice([0, 1, 42, 2]))
assert.Eq(insert_at( 3), intSlice([0, 1, 2, 42]))
assert.Eq(insert_at( 4), intSlice([0, 1, 2, 42]))
`)
	_, err := starlight.Eval(code, globals, nil)
	if err != nil {
		t.Fatal(err)
	}

	code = []byte(`intSlice([0,1,2]).insert(0, "test")`)
	_, err = starlight.Eval(code, globals, nil)
	expectErr(t, err, `insert: value of type string cannot be converted to type int`)
}

func TestSliceRemove(t *testing.T) {
	globals := map[string]interface{}{
		"assert":   &assert{t: t},
		"intSlice": intSlice,
	}

	code := []byte(`
def remove(v):
  x = intSlice([3, 1, 4, 1])
  x.remove(v)
  return x
assert.Eq(remove(3), intSlice([1, 4, 1]))
assert.Eq(remove(1), intSlice([3, 4, 1]))
assert.Eq(remove(4), intSlice([3, 1, 1]))
`)
	_, err := starlight.Eval(code, globals, nil)
	if err != nil {
		t.Fatal(err)
	}

	code = []byte(`intSlice([3, 1, 4, 1]).remove(42)`)
	_, err = starlight.Eval(code, globals, nil)
	expectErr(t, err, "remove: element 42 not found")

	code = []byte(`intSlice([3, 1, 4, 1]).remove(True)`)
	_, err = starlight.Eval(code, globals, nil)
	expectErr(t, err, "remove: value of type bool cannot be converted to type int")
}

func TestSliceIteratorInvalidation(t *testing.T) {
	globals := map[string]interface{}{
		"intSlice": intSlice,
	}
	code := []byte(`
def iterator1():
	list = intSlice([0, 1, 2])
	for x in list:
		list[x] = 2 * x
	return list
iterator1()
`)
	_, err := starlight.Eval(code, globals, nil)
	expectErr(t, err, "cannot assign to slice during iteration")

	code = []byte(`
def iterator2():
	list = intSlice([0, 1, 2])
	for x in list:
		list.remove(x)
iterator2()
`)
	_, err = starlight.Eval(code, globals, nil)
	expectErr(t, err, "cannot remove from slice during iteration")

	code = []byte(`
def iterator3():
	list = intSlice([0, 1, 2])
	for x in list:
	  list.append(3)
iterator3()
`)
	_, err = starlight.Eval(code, globals, nil)
	expectErr(t, err, "cannot append to slice during iteration")

	code = []byte(`
def iterator4():
	list = intSlice([0, 1, 2])
	for x in list:
		list.extend([3, 4])
iterator4()
`)
	_, err = starlight.Eval(code, globals, nil)
	expectErr(t, err, "cannot extend slice during iteration")

	code = []byte(`
def iterator5():
	def f(x):
	  x.append(4)
	list = intSlice([1, 2, 3])
	_ = [f(list) for x in list]
iterator5()
	`)
	_, err = starlight.Eval(code, globals, nil)
	expectErr(t, err, "cannot append to slice during iteration")
}

func TestSlicePop(t *testing.T) {
	globals := map[string]interface{}{
		"assert":   &assert{t: t},
		"intSlice": intSlice,
	}

	code := []byte(`
# list.pop
x4 = intSlice([1,2,3,4,5])
assert.Eq(x4.pop(), 5)
assert.Eq(x4, intSlice([1,2,3,4]))
assert.Eq(x4.pop(1), 2)
assert.Eq(x4, intSlice([1,3,4]))
assert.Eq(x4.pop(0), 1)
assert.Eq(x4, intSlice([3,4]))
`)
	_, err := starlight.Eval(code, globals, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSliceUnsupportedType(t *testing.T) {
	globals := map[string]interface{}{
		"assert": &assert{t: t},
		"s1":     []chan int{},
		"s2":     []chan int{make(chan int), make(chan int, 1), make(chan int, 2)},
	}

	code := []byte(`s1.append(1)`)
	_, err := starlight.Eval(code, globals, nil)
	expectErr(t, err, "append: value of type int64 cannot be converted to type chan int")

	code = []byte(`val = s2[]`)
	_, err = starlight.Eval(code, globals, nil)
	expectErr(t, err, "eval.sky:1:11: got ']', want primary expression")

	code = []byte(`val = s2["foo"]`)
	_, err = starlight.Eval(code, globals, nil)
	expectErr(t, err, "starlight_slice<[]chan int> index: got string, want int")
}

// func TestSlicePlus(t *testing.T) {
// 	x := []int{1, 2, 3}

// 	globals := map[string]interface{}{
// 		"x":        x,
// 		"intSlice": intSlice,
// 		"assert":   assert{t: t},
// 	}

// 	code := []byte(`
// y = x + [3, 4, 5]
// assert.Eq(y, intSlice([1, 2, 3, 3, 4, 5]))
// `)
// 	_, err := starlight.Eval(code, globals, nil)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// }
