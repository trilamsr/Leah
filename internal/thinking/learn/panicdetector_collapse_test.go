package learn

import (
	"reflect"
	"testing"
)

// TestPanicDetectorInterfaceRemoved asserts Resolver.PanicDetectors field is gone post-collapse (BB-retro M13).
func TestPanicDetectorInterfaceRemoved(t *testing.T) {
	rt := reflect.TypeOf(Resolver{})
	if _, ok := rt.FieldByName("PanicDetectors"); ok {
		t.Fatalf("Resolver.PanicDetectors field MUST be removed (BB-retro M13: interface-collapse)")
	}
}
