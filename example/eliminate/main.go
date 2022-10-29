package main

import (
	"errors"
	"github.com/emove/connpool"
	"log"
	"sync"
	"time"
)

func main() {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	p := connpool.NewPool([]string{"tcp://localhost:9090"},
		func(addr string) (interface{}, error) {
			return nil, errors.New("dial error")
		},
		close,
		connpool.EliminateAddr(3, time.Minute, func(addr string) { // 快速连接3次失败然后剔除
			log.Printf("addr: %s was elimiated due to continuous connect error", addr)
			wg.Done()
		}))

	wg.Wait()
	p.Close()

	wg.Add(1)
	now := time.Now()
	p = connpool.NewPool([]string{"tcp://localhost:9090"},
		func(addr string) (interface{}, error) {
			time.Sleep(500 * time.Millisecond)
			return nil, errors.New("dial error")
		},
		close,
		connpool.EliminateAddr(5, time.Second, func(addr string) { // 连续失败时间超过1秒
			log.Printf("addr: %s was elimiated due to continuous connect error", addr)
			log.Printf("total: %s", time.Since(now))
			wg.Done()
		}))

	wg.Wait()
	p.Close()
}

func close(_ interface{}) {
	// ignore
}
