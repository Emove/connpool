package connpool

import (
	"container/list"
	"sync"
	"time"

	"github.com/emove/connpool/backoff"
)

type dialer struct {
	p              *pool
	dialFn         Dial
	records        sync.Map
	done           chan struct{}
	wg             *sync.WaitGroup
	policy         backoff.Policy
	maxRetryTimes  int           // no longer retry once error times greater than this value.
	maxRetryPeriod time.Duration // no longer retry once the period that since the first error time greater than this value.
}

type record struct {
	mu             sync.Mutex
	dialRequest    *list.List
	ig             backoff.IntervalGenerator
	rate           float64   // control dial new connections rate after server recovery
	batch          int       // control the growth of the rate
	errorTimes     int       // continuous error times.
	firstErrorTime time.Time // record the first dial error time.
	lastErrorTime  time.Time // record the last dial error time and used to check whether starting exponential backoff dial
}

type dialRequest struct {
	addr     string    // target address
	isCore   bool      // whether core connection
	start    time.Time // request time
	dialTime time.Time
	callback func(conn interface{}, err error)
}

func (d *dialer) dial(req *dialRequest) {
	r := d.load(req.addr)

	r.mu.Lock()
	r.pushBack(req)
	r.mu.Unlock()
}

func (d *dialer) handleDialRequest(wg *sync.WaitGroup) {
	wg.Done()
	d.wg = wg
	for {
		select {
		case <-d.done:
			d.records.Range(func(key, value interface{}) bool {
				r := value.(*record)
				for r.dialRequest.Len() > 0 {
					req := r.front(true)
					req.callback(nil, ErrPoolClosed)
				}
				return true
			})
			d.wg.Done()
			return
		default:
			d.records.Range(func(key, value interface{}) bool {
				r := value.(*record)
				r.mu.Lock()
				defer r.mu.Unlock()

				// Check whether dial error before
				if !r.lastErrorTime.IsZero() {

					// Avoid establishing too many unessential connections
					for req := r.front(false); req != nil && time.Since(req.start) > d.p.waitTimeout; {
						r.dialRequest.Remove(r.dialRequest.Front())
						req = r.front(false)
					}

					dur := r.ig.Next(r.errorTimes)
					if time.Since(r.lastErrorTime) < dur {
						return true
					}
				}

				if r.dialRequest.Len() == 0 {
					return true
				}

				if r.rate < 1 && r.batch > 0 {
					return true
				}

				batch := int(float64(r.dialRequest.Len()) * r.rate)
				if batch < 1 {
					batch = 1
				}
				for i := 0; i < batch; i++ {
					req := r.front(true)
					req.dialTime = time.Now()
					go func() {
						d.doDial(req, r)
					}()
				}
				if r.rate < 1 {
					r.batch = batch
				}
				return true
			})
		}
	}
}

func (d *dialer) doDial(req *dialRequest, r *record) {
	conn, err := d.dialFn(req.addr)

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.rate < 1 {
		r.batch--
		if r.batch <= 0 {
			r.rate *= 3
			if r.rate > 1 {
				r.rate = 1
			}
		}
	}

	// Dial succeeded
	if err == nil {
		if r.errorTimes > 0 {
			if !r.lastErrorTime.IsZero() {
				r.rate = 0.2
			}
			r.errorTimes = 0
			r.firstErrorTime = time.Time{}
			r.lastErrorTime = time.Time{}
		}
		req.callback(conn, nil)
		return
	}

	if !r.lastErrorTime.IsZero() && req.dialTime.Before(r.lastErrorTime) {
		if req.isCore || r.dialRequest.Len() == 0 {
			r.pushFront(req)
		} else {
			req.callback(conn, err)
		}
		return
	}

	r.errorTimes++
	if r.errorTimes == 1 {
		r.firstErrorTime = time.Now()
		r.lastErrorTime = time.Now()
	}

	if checkBound(d, r) {
		req.callback(conn, err)
		d.p.eliminateAddr(req.addr)
		// TODO clear the dial request list
		return
	}

	if req.isCore || r.dialRequest.Len() == 0 {
		r.pushFront(req)
	} else {
		req.callback(conn, err)
	}
}

func (d *dialer) close() {
	close(d.done)
}

func (d *dialer) load(addr string) *record {
	load, ok := d.records.Load(addr)
	if ok {
		return load.(*record)
	}
	val, _ := d.records.LoadOrStore(addr, &record{
		mu:             sync.Mutex{},
		dialRequest:    list.New(),
		ig:             d.policy.New(),
		rate:           1,
		firstErrorTime: time.Time{},
		lastErrorTime:  time.Time{}},
	)

	return val.(*record)
}

func (r *record) front(remove bool) *dialRequest {
	front := r.dialRequest.Front()
	if front == nil {
		return nil
	}
	if remove {
		r.dialRequest.Remove(front)
	}
	return front.Value.(*dialRequest)
}

func (r *record) back(remove bool) *dialRequest {
	back := r.dialRequest.Back()
	if back == nil {
		return nil
	}
	if remove {
		r.dialRequest.Remove(back)
	}
	return back.Value.(*dialRequest)
}

func (r *record) pushFront(cb *dialRequest) {
	r.dialRequest.PushFront(cb)
}

func (r *record) pushBack(cb *dialRequest) {
	r.dialRequest.PushBack(cb)
}

// checkBound checks whether error times greater than dialer.maxRetryTimes
// or the period from the first error greater than dialer.maxRetryPeriod
func checkBound(d *dialer, r *record) bool {
	if d.maxRetryTimes > 0 && r.errorTimes >= d.maxRetryTimes {
		return true
	}
	if d.maxRetryPeriod > 0 && time.Since(r.firstErrorTime) >= d.maxRetryPeriod {
		return true
	}
	return false
}
