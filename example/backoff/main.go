package main

import (
	"errors"
	"github.com/emove/connpool"
	"github.com/emove/connpool/backoff"
	"time"
)

func main() {
	p := connpool.NewPool([]string{"tcp://localhost:9090"},
		func(addr string) (interface{}, error) {
			return nil, errors.New("dial error")
		},
		func(conn interface{}) {
			// ignore
		},
		connpool.Lazy(),
		// connpool.DialBackoffPolicy(backoff.NewNullPolicy()), // 没有间隔时间，连接失败后一直重试
		// connpool.DialBackoffPolicy(backoff.NewFixedIntervalPolicy(100*time.Millisecond)), // 连接失败后，每100ms重试一次
		connpool.DialBackoffPolicy(backoff.NewExponentialPolicy( // 指数退避重试
			backoff.Multiplier(2),                     // 指数因子
			backoff.MinInterval(100*time.Millisecond), // 最小间隔时间
			backoff.MaxInterval(1*time.Second),        // 最大重试时间 100ms 200ms 400ms 800ms 1s（没有抖动因子）
			backoff.Jitter(0.2)),                      // 间隔时间抖动因子 88.750325ms  207.576404ms 349.527405ms 680.530444ms 1.116610437s （抖动因子等于0.2）
		),
	)
	p.Close()
}
