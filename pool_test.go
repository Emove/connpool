package connpool

import (
	"errors"
	"github.com/emove/connpool/backoff"
	"log"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type conn struct {
	addr string
}

func TestNewPool(t *testing.T) {
	addrs := []string{"tcp://localhost:8080", "tcp://localhost:9090"}
	wg := &sync.WaitGroup{}
	wg.Add(2)
	p := NewPool(addrs, dialFn(t, nil), closeFn(t, func() {
		wg.Done()
	}))
	con, err := p.Get()
	if err != nil {
		log.Printf("get error: %v", err)
	}
	p.Put(con)

	con, err = p.Get(addrs[0])
	if err != nil {
		t.Fatalf("get error: %v", err)
	}

	if c, ok := con.(*conn); ok {
		if c.addr != addrs[0] {
			t.Fatalf("get addr connection error, want: %s, got: %s", addrs[0], c.addr)
		}
	}

	p.Discard(con)
	p.Close()
	wg.Wait()
}

func TestCoreNums(t *testing.T) {
	cnt, nums := int64(0), 5
	wg := &sync.WaitGroup{}
	wg.Add(nums)
	p := NewPool([]string{"tcp://localhost:8080"}, dialFn(t, func() {
		atomic.AddInt64(&cnt, 1)
	}), closeFn(t, func() {
		wg.Done()
	}), CoreNums(nums))
	conns := make([]interface{}, nums)
	for i := 0; i < nums; i++ {
		conn, err := p.Get()
		if err != nil {
			t.Errorf("get error: %v", err)
		}
		conns[i] = conn
	}
	for _, conn := range conns {
		p.Put(conn)
	}
	if atomic.LoadInt64(&cnt) != int64(nums) {
		t.Fatalf("core nums sets: %d, but: %d", nums, cnt)
	}
	p.Close()
	wg.Wait()
}

func TestMaxNums(t *testing.T) {
	cnt, nums := int64(0), 5
	wg := &sync.WaitGroup{}
	wg.Add(nums)
	p := NewPool([]string{"tcp://localhost:8080"}, dialFn(t, func() {
		atomic.AddInt64(&cnt, 1)
	}), closeFn(t, func() {
		wg.Done()
	}), MaxNums(nums))
	returnWg := &sync.WaitGroup{}
	for i := 0; i < 10*nums; i++ {
		returnWg.Add(1)
		conn, err := p.Get()
		if err != nil {
			t.Errorf("get error: %v", err)
		}
		time.AfterFunc(time.Millisecond, func() {
			p.Put(conn)
			returnWg.Done()
		})
	}
	if atomic.LoadInt64(&cnt) != int64(nums) {
		t.Fatalf("core nums sets: %d, but: %d", nums, cnt)
	}

	returnWg.Wait()
	p.Close()
	wg.Wait()
}

func TestMaxIdleTime(t *testing.T) {
	nums := 5
	wg := &sync.WaitGroup{}
	wg.Add(nums)
	p := NewPool([]string{"tcp://localhost:8080"},
		dialFn(t, nil),
		closeFn(t, func() {
			wg.Done()
		}),
		MaxIdleTime(5*time.Second), // 设置连接的最大空闲时间
		Lazy(),                     // 以Lazy的方式启动，主动获取5个连接
	)
	returnWg := &sync.WaitGroup{}
	for i := 0; i < nums; i++ {
		returnWg.Add(1)
		conn, err := p.Get()
		if err != nil {
			t.Fatalf("get error: %v", err)
		}
		time.AfterFunc(5*time.Millisecond, func() {
			p.Put(conn)
			returnWg.Done()
		})
	}

	returnWg.Wait()
	wg.Wait()
	p.Close()
}

func TestWaitTimeout(t *testing.T) {
	p := NewPool([]string{"tcp://localhost:8080"},
		dialFn(t, func() {
			time.Sleep(2 * time.Second)
		}),
		closeFn(t, nil),
		Lazy(), // 以Lazy的方式启动，等待主动获取连接
		WaitTimeout(1*time.Second),
	)
	_, err := p.Get()
	if err == nil {
		t.Fatal("expected error, but got nil")
	}
	t.Logf("got err: %v", err)
	p.Close()
}

func TestWithWarmUp(t *testing.T) {
	cnt := 0
	wg := &sync.WaitGroup{}
	once := &sync.Once{}
	p := NewPool([]string{"tcp://localhost:8080"}, dialFn(t, func() {
		wg.Add(1)
		cnt++
	}), closeFn(t, func() {
		wg.Done()
	}), Lazy(),
		WithWarmUp(func(waits, idles, actives, max uint32) uint32 {
			value := 0
			once.Do(func() {
				value = 5
			})
			return uint32(value)
		}),
	)
	conn, err := p.Get()
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	p.Put(conn)
	p.Close()
	wg.Wait()
	if cnt != 6 {
		t.Fatalf("expected 6, but got: %d", cnt)
	}
}

func TestDialBackoffPolicy(t *testing.T) {

	tests := []struct {
		name     string
		policy   backoff.Policy
		interval []time.Duration
	}{
		{
			name:     "null",
			policy:   backoff.NewNullPolicy(),
			interval: []time.Duration{time.Nanosecond},
		},
		{
			name:     "fixed",
			policy:   backoff.NewFixedIntervalPolicy(100 * time.Millisecond),
			interval: []time.Duration{100 * time.Millisecond},
		},
		{
			name:     "exponential",
			policy:   backoff.NewExponentialPolicy(backoff.Multiplier(2), backoff.MaxInterval(100*time.Millisecond), backoff.MaxInterval(time.Second)),
			interval: []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond, 1 * time.Second},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			last, i := time.Time{}, 0
			p := NewPool([]string{"tcp://localhost:8080"},
				func(addr string) (interface{}, error) {
					if last.IsZero() {
						last = time.Now()
					} else {
						since := time.Since(last)
						if since > tt.interval[i] && since-tt.interval[i] < 10*time.Millisecond {
							i++
						}
						if i >= len(tt.interval) {
							return &conn{addr: addr}, nil
						}
					}
					return nil, errors.New("dail error")
				},
				func(i interface{}) {
					// ignore
				},
				DialBackoffPolicy(tt.policy),
				Lazy(),
			)
			_, _ = p.Get()
			p.Close()
			if i != len(tt.interval) {
				t.Fatalf("len(tt.interval): %d, i: %d", len(tt.interval), i)
			}
		})
	}
}

func TestEliminateAddr(t *testing.T) {
	type args struct {
		maxErrorTimes  int
		maxErrorPeriod time.Duration
		policy         backoff.Policy
	}

	tests := []struct {
		name         string
		args         args
		hookCalled   bool
		succeedAfter int
	}{
		{
			name:         "maxErrorTime",
			args:         args{maxErrorTimes: 3, maxErrorPeriod: time.Minute, policy: backoff.NewNullPolicy()},
			hookCalled:   true,
			succeedAfter: 3,
		},
		{
			name:         "maxErrorPeriod",
			args:         args{maxErrorTimes: 10, maxErrorPeriod: 100 * time.Millisecond, policy: backoff.NewFixedIntervalPolicy(150 * time.Millisecond)},
			hookCalled:   true,
			succeedAfter: 3,
		},
		{
			name:         "succeedInForthDial",
			args:         args{maxErrorTimes: 4, maxErrorPeriod: time.Second, policy: backoff.NewNullPolicy()},
			hookCalled:   false,
			succeedAfter: 3,
		},
		{
			name:         "succeed",
			args:         args{maxErrorTimes: 4, maxErrorPeriod: time.Second, policy: backoff.NewFixedIntervalPolicy(500 * time.Millisecond)},
			hookCalled:   false,
			succeedAfter: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hookCalled, cnt := false, int64(0)
			p := NewPool([]string{"tcp://localhost:8080"},
				func(addr string) (interface{}, error) {
					if atomic.AddInt64(&cnt, 1) > int64(tt.succeedAfter) {
						return &conn{addr: addr}, nil
					}
					return nil, errors.New("dail error")
				},
				func(i interface{}) {
					// ignore
				},
				EliminateAddr(tt.args.maxErrorTimes, tt.args.maxErrorPeriod, func(addr string) {
					hookCalled = true
				}),
				DialBackoffPolicy(tt.args.policy),
			)
			if tt.hookCalled != hookCalled {
				t.Fatalf("expected: %v, but: %v", tt.hookCalled, hookCalled)
			}

			p.Close()
		})
	}
}

func dialFn(t *testing.T, fn func()) Dial {
	return func(addr string) (interface{}, error) {
		if fn != nil {
			fn()
		}
		t.Logf("dial: %s\n", addr)
		return &conn{addr: addr}, nil
	}
}

func closeFn(t *testing.T, fn func()) Close {
	return func(con interface{}) {
		if c, ok := con.(*conn); ok {
			t.Logf("close: %s\n", c.addr)
		}
		if fn != nil {
			fn()
		}
	}
}
