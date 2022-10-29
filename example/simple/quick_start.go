package main

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/emove/connpool"
	"github.com/emove/connpool/example"
)

func main() {
	addr := "localhost:9090"
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go example.StartServer(addr, wg)
	wg.Wait()
	wg.Add(1)
	// 创建一个连接池，传入三个基本参数
	// 连接地址，创建链接的方法，关闭链接的方法
	pool := connpool.NewPool([]string{addr}, dial, close,
		connpool.CoreNums(1),                 // 配置单个地址链接的核心连接数
		connpool.MaxNums(10),                 // 配置单个地址链接的最大连接数
		connpool.MaxIdleTime(30*time.Second), // 配置链接的最大空闲时间
		connpool.WaitTimeout(3*time.Second),  // 配置获取链接的超时时间
		connpool.Lazy(),                      // 初始时不会建立任何连接，只在需要时建立连接
	)
	con, err := pool.Get()
	if err != nil {
		fmt.Printf("get connection err: %v\n", err)
		return
	}

	conn := con.(net.Conn)
	_, err = conn.Write([]byte("hello world"))
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
}

func dial(addr string) (interface{}, error) {
	return net.Dial("tcp", addr)
}

func close(con interface{}) {
	if c, ok := con.(net.Conn); ok {
		_ = c.Close()
	}
}
