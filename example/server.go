package example

import (
	"fmt"
	"io"
	"net"
	"sync"
)

func StartServer(addr string, wg *sync.WaitGroup) {
	listen, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}
	wg.Done()
	conn, err := listen.Accept()
	if err != nil {
		panic(err)
	}

	msg := make([]byte, 11)

	_, err = io.ReadFull(conn, msg)
	if err != nil {
		panic(err)
	}

	fmt.Println(string(msg))
	wg.Done()
}
