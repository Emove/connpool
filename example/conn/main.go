package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/emove/connpool"
	"github.com/emove/connpool/example"
)

// 为了不对使用方现有设计造成入侵，ConnPool 接口用interface{}类型表示链接，内部
// 使用了Mutex和map来存储被取出的链接。用于在Put和Discard时，能够知道该链接连接
// 的是哪个地址，方便做连接数控制。该做法在负载较重时会有性能影响。
// 当有必要的时候，实现connpool.Connection接口，可以分散该部分压力
func main() {
	addr := "localhost:9090"
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go example.StartServer(addr, wg)
	wg.Wait()
	wg.Add(1)
	// 创建一个连接池，传入三个基本参数
	// 连接地址，创建链接的方法，关闭链接的方法
	pool := connpool.NewPool([]string{addr}, dial, close)
	con, err := pool.Get()
	if err != nil {
		fmt.Printf("get connection err: %v\n", err)
		return
	}

	conn := con.(*Conn)
	_, err = conn.raw.Write([]byte("hello world"))
	if err != nil {
		// 使用中发生错误，交由连接池销毁该连接
		pool.Discard(con)
	} else {
		// 将可用链接归还给连接池，等待复用
		pool.Put(con)
	}

	wg.Wait()
	// 关闭连接池
	pool.Close()
	time.Sleep(time.Second)
}

func dial(addr string) (interface{}, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &Conn{ctx: context.Background(), raw: conn}, nil
}

func close(con interface{}) {
	if c, ok := con.(*Conn); ok {
		log.Printf("connection closed, remote: %s", c.raw.RemoteAddr())
		_ = c.raw.Close()
	}
}
