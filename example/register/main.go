package main

import (
	"fmt"
	"log"

	"github.com/emove/connpool"
)

func main() {
	p := connpool.NewPool([]string{"tcp://localhost:8080"}, dial, close)

	registrar := connpool.NewRegistrar(p)

	addr := "http://localhost:8888"
	_, err := p.Get(addr)
	if err != nil {
		log.Printf("get error: %v, address: %s", err, addr)
	}

	err = registrar.Register(addr)
	if err != nil {
		_ = fmt.Errorf("register error: %v", err)
	}

	_, err = p.Get(addr)
	if err != nil {
		_ = fmt.Errorf("expected succeeded, but error: %v", err)
	}
}

type Conn struct {
	addr string
}

func dial(addr string) (interface{}, error) {
	log.Printf("dail %s", addr)
	return &Conn{addr: addr}, nil
}

func close(_ interface{}) {
	// ignore
}
