package convert

import (
	"reflect"
	"sync"
	"testing"
)

// TestRecursionDetector tests the functionality of the recursionDetector struct.
func TestRecursionDetector(t *testing.T) {
	// Create a new recursionDetector.
	trd := newRecursionDetector()

	// Create a test object.
	obj := &sync.Mutex{}

	// Check if the object has been visited.
	if trd.hasVisited(obj) {
		t.Errorf("Expected hasVisited to return false for a new object, got true")
	}

	// Set the object as visited.
	trd.setVisited(obj)

	// Check if the object has been visited.
	if !trd.hasVisited(obj) {
		t.Errorf("Expected hasVisited to return true for a visited object, got false")
	}

	// Clear the object from visited.
	trd.clearVisited(obj)

	// Check if the object has been visited.
	if trd.hasVisited(obj) {
		t.Errorf("Expected hasVisited to return false after clearVisited, got true")
	}
}

// TestAddr tests the addr function of the recursionDetector struct.
func TestAddr(t *testing.T) {
	// Create a new recursionDetector.
	trd := newRecursionDetector()

	// Create a test object.
	obj := &sync.Mutex{}

	// Get the address of the object using the addr function.
	addr := trd.addr(obj)

	// Get the address of the object using the reflect package.
	expectedAddr := reflect.ValueOf(obj).Pointer()

	// Check if the addresses match.
	if addr != expectedAddr {
		t.Errorf("Expected addr to return %v, got %v", expectedAddr, addr)
	}

	// Get the address of Go map
	om := map[string]int{"a": 100}
	gm := NewGoMap(om)
	addr1 := trd.addr(gm)
	expectedAddr = reflect.ValueOf(gm).Pointer()
	if addr1 != expectedAddr {
		t.Errorf("Expected GoMap addr1 to return %v, got %v", expectedAddr, addr1)
	}

	gm2 := NewGoMap(om)
	addr2 := trd.addr(gm2)
	expectedAddr = reflect.ValueOf(gm2).Pointer()
	if addr2 != expectedAddr {
		t.Errorf("Expected GoMap addr2 to return %v, got %v", expectedAddr, addr2)
	}

	// Compare the addresses of the two Go maps
	if addr1 == addr2 {
		t.Errorf("Expected GoMap addr1 and addr2 to be different, got same: %v", addr1)
	}
}
