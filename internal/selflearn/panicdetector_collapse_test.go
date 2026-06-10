package selflearn

import (
	"reflect"
	"testing"
)

// TestPanicDetectorInterfaceRemoved asserts the PanicDetector marker
// interface is gone (BB-retro M13 collapse). The interface had one
// method (Name) + a single concrete impl in internal/selflearn/rules,
// and every daemon caller immediately type-asserted back to the
// concrete type — the abstraction added a downcast for zero callers.
//
// We assert via reflection on a Resolver instance: the PanicDetectors
// slice field MUST be absent. A compile-time reference to the type
// name would itself fail to compile post-collapse, which would
// satisfy the RED-then-GREEN gate but leave no executable assertion
// for regression. Reflection on the surviving Resolver shape is the
// durable check.
func TestPanicDetectorInterfaceRemoved(t *testing.T) {
	rt := reflect.TypeOf(Resolver{})
	if _, ok := rt.FieldByName("PanicDetectors"); ok {
		t.Fatalf("Resolver.PanicDetectors field MUST be removed (BB-retro M13: interface-collapse)")
	}
}
