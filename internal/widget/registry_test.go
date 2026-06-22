package widget

import (
	"sync"
	"testing"
)

func TestRegistry_RegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	a := NewPureAdapter("stat")
	if err := r.Register(a); err != nil {
		t.Fatalf("register: %v", err)
	}
	got, ok := r.Lookup("stat")
	if !ok {
		t.Fatalf("lookup miss")
	}
	if got.Type() != "stat" {
		t.Fatalf("lookup wrong adapter: %q", got.Type())
	}
}

func TestRegistry_DuplicateRegisterErrors(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(NewPureAdapter("stat")); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := r.Register(NewPureAdapter("stat")); err == nil {
		t.Fatalf("duplicate must error")
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := NewRegistry()
	kinds := []string{"stat", "table", "list", "code", "diff"}
	for _, k := range kinds {
		if err := r.Register(NewPureAdapter(k)); err != nil {
			t.Fatalf("register %s: %v", k, err)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			kind := kinds[i%len(kinds)]
			if _, ok := r.Lookup(kind); !ok {
				t.Errorf("lookup %s: miss", kind)
			}
		}(i)
	}
	wg.Wait()
}
