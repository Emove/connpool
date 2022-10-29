package connpool

// Registrar used to register new addrs to the pool
type Registrar struct {
	p *pool
}

// NewRegistrar returns a Registrar
func NewRegistrar(cp ConnPool) *Registrar {
	return &Registrar{p: cp.(*pool)}
}

// Register registers addrs to the pool
func (r *Registrar) Register(addrs ...string) error {
	if len(addrs) > 0 {
		return r.p.register(addrs...)
	}
	return nil
}
