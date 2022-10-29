package connpool

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

var requestPool sync.Pool

const (
	waiting = iota
	finished
)

type request struct {
	reuseCnt uint32
	state    int32 // 0 - waiting, 1 - finished
	pr       *peer
	conn     chan interface{}
	err      chan error
}

func init() {
	requestPool = sync.Pool{New: func() interface{} {
		return &request{
			conn: make(chan interface{}),
			err:  make(chan error),
		}
	}}
}

func newRequest(pr *peer, waitTimeout time.Duration) *request {
	req := requestPool.Get().(*request)
	req.pr = pr
	if waitTimeout > 0 {
		time.AfterFunc(waitTimeout, func() {
			// Ensure that time.AfterFunc only works in current reuse lifecycle
			cur := req.reuseCnt
			if atomic.LoadUint32(&req.reuseCnt) == cur && atomic.CompareAndSwapInt32(&req.state, waiting, finished) {
				req.err <- ErrGetConnWaitTimeout
			}
		})
	}
	return req
}

func reuseRequest(req *request) {
	if req == nil {
		return
	}
	req.pr = nil
	atomic.AddUint32(&req.reuseCnt, 1)
	if req.reuseCnt == math.MaxUint32 {
		req.reuseCnt = 0
	}
	req.state = waiting
	requestPool.Put(req)
}
