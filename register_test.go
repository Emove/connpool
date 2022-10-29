package connpool

import "testing"

func TestRegistrar_Register(t *testing.T) {
	p := NewPool([]string{"tcp://localhost:8080"}, dialFn(t, nil), closeFn(t, nil))

	registrar := NewRegistrar(p)

	addr := "http://localhost:8888"
	_, err := p.Get(addr)
	if err == nil {
		t.Fatalf("expected got error, but nil")
	}

	err = registrar.Register(addr)
	if err != nil {
		t.Fatalf("register error: %v", err)
	}

	_, err = p.Get(addr)
	if err != nil {
		t.Fatalf("expected succeeded, but error: %v", err)
	}
}
