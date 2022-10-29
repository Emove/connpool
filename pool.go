package connpool

import (
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/emove/connpool/backoff"
)

// ConnPool is an interface
type ConnPool interface {
	Get(addr ...string) (interface{}, error)
	Put(interface{})
	Discard(interface{})
	Close()
}

type (
	// Dial is used to create new connections.
	Dial = func(addr string) (interface{}, error)
	// Close is a connection closed hook.
	Close          = func(interface{})
	WarmUp         = func(waits, idles, actives, max uint32) uint32
	EliminatedHook = func(addr string)
)

var (
	// ErrNotUsableAddr represents all addresses has been eliminated from pool
	ErrNotUsableAddr = errors.New("not usable addr")
	// ErrPoolClosed represents the pool has been closed
	ErrPoolClosed = errors.New("pool closed")
	// ErrGetConnWaitTimeout represents get connection from pool timeout
	ErrGetConnWaitTimeout = errors.New("get connection wait timeout")
	// ErrIllegalAddress represents the specific address does not registered before pool.Get() called
	ErrIllegalAddress = errors.New("unknown address")
	// ErrAddrEliminated represents the address was eliminated
	ErrAddrEliminated = errors.New("address has been eliminated")
)

var (
	// default numbers of the core connection per address
	defaultCoreNums = uint32(1)
	// default max numbers of the connection per address
	defaultMaxNums = uint32(10)
	// default max idle time of the idle connection
	defaultMaxIdleTime = 30 * time.Second
	// default max waiting time to get a connection from pool
	defaultWaitTimeout = 3 * time.Second
)

// NewPool returns a ConnPool
func NewPool(addrs []string, d Dial, closer Close, ops ...Option) ConnPool {
	addrs = filter(addrs)
	if len(addrs) < 1 {
		panic("addrs must contains one address at least")
	}
	p := &pool{
		mu:    sync.RWMutex{},
		idx:   -1,
		peers: make(map[string]*peer),
		reqs:  sync.Map{},
		dr: &dialer{
			dialFn:  d,
			records: sync.Map{},
			done:    make(chan struct{}),
			policy:  backoff.NewNullPolicy(),
		},
		closer: func(conn interface{}) {
			go closer(conn)
		},
		done:        make(chan struct{}),
		wg:          &sync.WaitGroup{},
		inmu:        sync.Mutex{},
		inuse:       make(map[interface{}]*peer),
		coreNums:    defaultCoreNums,
		maxNums:     defaultMaxNums,
		warmUp:      defaultWarmUp,
		maxIdleTime: defaultMaxIdleTime,
		waitTimeout: defaultWaitTimeout,
	}
	p.dr.p = p

	if len(ops) > 0 {
		// apply options
		for _, op := range ops {
			op(p)
		}
	}

	if p.maxNums < p.coreNums {
		p.maxNums = p.coreNums
	}

	if p.maxWaiting == 0 {
		p.maxWaiting = uint64(math.Round(float64(p.maxNums) * 1.75))
	}
	p.freeChan = make(chan *idleConn, int(math.Round(float64(len(addrs)/2)*float64(p.maxNums))))

	p.wg.Add(2)
	go p.dr.handleDialRequest(p.wg)
	go p.process()

	_ = p.register(addrs...)
	return p
}

var _ ConnPool = (*pool)(nil)

const (
	working = iota
	closed
)

type pool struct {
	mu             sync.RWMutex          // guard the addrs, peers, idx, closed.
	addrs          []string              // usable addresses.
	peers          map[string]*peer      // mapping address to peer.
	idx            int64                 // used to select a peer in nearly polling.
	reqs           sync.Map              // the mapping of peers and respective request channel.
	freeChan       chan *idleConn        // the connections waiting for close.
	closed         int32                 // 0 - working, 1 - closed.
	done           chan struct{}         // shutdown all background goroutine.
	wg             *sync.WaitGroup       // used to wait process goroutine starts or exits.
	dr             *dialer               // used to create new connection.
	closer         Close                 // used to close connection.
	inmu           sync.Mutex            // guard the inuse.
	inuse          map[interface{}]*peer // mapping the inuse connection to peer.
	coreNums       uint32                // core connection nums of per addr, default value is one.
	maxNums        uint32                // max connection nums of per addr, default value is 1024.
	maxWaiting     uint64                // the max request size of per peer
	maxIdleTime    time.Duration         // the max idle time to remaining the connection.
	waitTimeout    time.Duration         // the max time to wait when get connection from pool.
	warmUp         WarmUp                // warm up connection func.
	lazy           bool                  // do dial when Get called.
	eliminateConn  bool                  // whether eliminate connection when dial error times out of dialer.maxRetryTimes or dialer.maxRetryPeriod, default value is false.
	eliminatedHook EliminatedHook        // called when address was eliminated.
}

// Get pick or generate a connection, if addr parameter is not empty, returns the given address connection.
func (p *pool) Get(addr ...string) (conn interface{}, err error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed == closed {
		return nil, ErrPoolClosed
	}
	var pr *peer
	if len(addr) > 0 {
		pr = p.peers[addr[0]]
		if pr == nil {
			return nil, ErrIllegalAddress
		}
	} else {
		pr, err = p.selectPeer()
		if err != nil {
			return nil, err
		}
	}

	req := newRequest(pr, p.waitTimeout)
	v, _ := p.reqs.Load(pr)
	ch := v.(chan *request)

	ch <- req

	select {
	case conn = <-req.conn:
	case err = <-req.err:
	}

	if err != nil {
		if err != ErrGetConnWaitTimeout {
			// Although request return due to timeout, it still
			// in the peer's waiting channel, so it should be
			// reused by the peer's process
			reuseRequest(req)
		}
		return
	}

	if _, ok := conn.(Connection); ok {
		return
	}

	p.inmu.Lock()
	p.inuse[conn] = pr
	p.inmu.Unlock()
	return
}

// Put puts the connection to pool
func (p *pool) Put(conn interface{}) {
	if conn == nil {
		return
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed == closed {
		return
	}

	pr, ok := (*peer)(nil), false
	if con, ok := conn.(Connection); ok {
		v := con.Value(&ctxPeerKey{})
		if v != nil {
			pr = v.(*peer)
		}
	}
	if pr == nil {
		p.inmu.Lock()
		if pr, ok = p.inuse[conn]; !ok {
			p.inmu.Unlock()
			return
		}
		delete(p.inuse, conn)
		p.inmu.Unlock()
	}
	pr.idles <- newIdleConn(pr, conn)
}

// Discard close the connection
func (p *pool) Discard(conn interface{}) {
	if conn == nil {
		return
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	pr, ok := (*peer)(nil), false
	if con, ok := conn.(Connection); ok {
		v := con.Value(&ctxPeerKey{})
		if v != nil {
			pr = v.(*peer)
		}
	}
	if pr == nil {
		p.inmu.Lock()
		if pr, ok = p.inuse[conn]; !ok {
			p.inmu.Unlock()
			return
		}
		delete(p.inuse, conn)
		p.inmu.Unlock()
	}

	ic := newIdleConn(pr, conn)
	if p.closed == closed {
		p.free(ic)
	} else {
		p.freeChan <- ic
	}
}

// Close releases all the resources of the pool
func (p *pool) Close() {
	p.mu.Lock()
	p.closed = closed
	p.mu.Unlock()

	p.wg.Add(2)
	p.dr.close()
	// Stop the process loop
	close(p.done)
	p.wg.Wait()
	p.clear()
}

func (p *pool) process() {
	p.wg.Done()
	for {
		select {
		case <-p.done:
			p.wg.Done()
			return
		case ic := <-p.freeChan:
			p.free(ic)
		default:
			// Ensure FCFS
			p.foreachPeerRequest()
		}
	}
}

// foreachPeerRequest assigns idle connections for requests or dial new connections
func (p *pool) foreachPeerRequest() {
	p.reqs.Range(func(key, value interface{}) bool {
		pr, ch := key.(*peer), value.(chan *request)
		loops := 0
	requestLoop:
		for len(ch) > 0 {
			loops++
			if loops > 60 {
				// Handle others request channel
				return true
			}
			ic := (*idleConn)(nil)
			for len(pr.idles) > 0 {
				// Select a usable idle connection
				ic = <-pr.idles
				if p.maxIdleTime > 0 && time.Since(ic.idleTime) > p.maxIdleTime {
					p.freeChan <- ic
					continue
				}
				break
			}

			if ic == nil {
				if atomic.LoadInt32(&pr.dialing) > 0 {
					return true
				}
				p.dial(pr, nil)
				attemptWarmUp(p, pr, ch)
				return true
			}
			attemptWarmUp(p, pr, ch)

			for len(ch) > 0 {
				req := <-ch
				if !atomic.CompareAndSwapInt32(&req.state, waiting, finished) {
					// The request timeout
					reuseRequest(req)
					continue
				}

				req.conn <- ic.conn
				reuseIdleConn(ic)
				goto requestLoop
			}

			// All request timeout, return the idle connection to channel
			pr.idles <- ic
		}
		// shrink idle connections
		if len(pr.idles) > 0 {
			ic := <-pr.idles
			if p.maxIdleTime > 0 && time.Since(ic.idleTime) > p.maxIdleTime {
				//p.freeChan <- ic
				p.free(ic)
			} else {
				pr.idles <- ic
			}
		}
		return true
	})
}

func (p *pool) free(ic *idleConn) {
	if ic == nil {
		return
	}
	p.closer(ic.conn)
	pr := ic.pr
	reuseIdleConn(ic)
	pr.am.Lock()
	pr.actives--
	if !p.lazy && pr.actives < p.coreNums {
		pr.am.Unlock()
		p.dial(pr, nil)
		return
	}
	pr.am.Unlock()
}

func (p *pool) dial(pr *peer, wg *sync.WaitGroup) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed == closed {
		return
	}
	pr.am.Lock()
	isCore := false
	if pr.actives < p.coreNums {
		isCore = true
	}
	pr.actives++

	if pr.actives > p.maxNums {
		pr.actives--
		pr.am.Unlock()
		return
	}
	pr.am.Unlock()
	dialReq := &dialRequest{
		addr:   pr.addr,
		isCore: isCore,
		start:  time.Now(),
		callback: func(conn interface{}, err error) {
			defer func() {
				if wg != nil {
					wg.Done()
				}
				atomic.AddInt32(&pr.dialing, -1)
			}()
			if err != nil {
				pr.am.Lock()
				pr.actives--
				pr.am.Unlock()
				return
			}
			if con, ok := conn.(Connection); ok {
				con.WithValue(&ctxPeerKey{}, pr)
			}

			ic := newIdleConn(pr, conn)
			p.mu.RLock()
			if p.closed == closed {
				p.mu.RUnlock()
				p.free(ic)
				return
			} else {
				pr.idles <- ic
			}
			p.mu.RUnlock()
		},
	}
	p.dr.dial(dialReq)
	atomic.AddInt32(&pr.dialing, 1)
}

func (p *pool) clear() {
	// Reject all of the waiting requests
	p.clearReqs()

	// Clear free channel
	for len(p.freeChan) > 0 {
		p.free(<-p.freeChan)
	}
	close(p.freeChan)

	// Clear all peers
	for _, pr := range p.peers {
		for len(pr.idles) > 0 {
			p.free(<-pr.idles)
		}
	}
}

func (p *pool) clearReqs() {
	p.reqs.Range(func(key, value interface{}) bool {
		ch := value.(chan *request)
		p.clearChannel(ch, ErrPoolClosed)
		close(ch)
		return true
	})
}

func (p *pool) clearChannel(ch <-chan *request, err error) {
	for len(ch) > 0 {
		req := <-ch
		if !atomic.CompareAndSwapInt32(&req.state, waiting, finished) {
			// The request timeout
			reuseRequest(req)
			continue
		}
		req.err <- err
	}
}

func (p *pool) selectPeer() (*peer, error) {
	if len(p.addrs) < 1 {
		p.mu.RLock()
		defer p.mu.RUnlock()
		if p.closed == closed {
			return nil, ErrPoolClosed
		}
		return nil, ErrNotUsableAddr
	}

	idx := atomic.AddInt64(&p.idx, 1)
	p.mu.RLock()
	defer p.mu.RUnlock()
	addr := p.addrs[idx%int64(len(p.addrs))]
	if idx == math.MaxInt32 {
		atomic.StoreInt64(&p.idx, -1)
	}
	return p.peers[addr], nil
}

// register adds new address
func (p *pool) register(addrs ...string) error {
	p.mu.Lock()
	if p.closed == closed {
		return ErrPoolClosed
	}
	wg := &sync.WaitGroup{}
	for _, addr := range addrs {
		p.addrs = append(p.addrs, addr)

		pr := &peer{
			addr:  addr,
			idles: make(chan *idleConn, p.maxNums),
			am:    sync.Mutex{},
		}

		p.peers[addr] = pr
		p.reqs.Store(pr, make(chan *request, p.maxWaiting))

		if p.lazy {
			continue
		}
		wg.Add(int(p.coreNums))
		for i := 0; i < int(p.coreNums); i++ {
			go p.dial(pr, wg)
		}

	}
	p.mu.Unlock()
	wg.Wait()
	return nil
}

// eliminateAddr eliminates the addr and clears the peer
func (p *pool) eliminateAddr(addr string) bool {
	if !p.eliminateConn {
		return false
	}
	p.mu.Lock()
	for i, ad := range p.addrs {
		if ad == addr {
			p.addrs = append(p.addrs[:i], p.addrs[i+1:]...)
		}
	}
	var pr *peer
	pr = p.peers[addr]
	delete(p.peers, addr)
	p.mu.Unlock()

	// Clear peer's request channel
	v, _ := p.reqs.LoadAndDelete(pr)
	ch := v.(chan *request)
	p.clearChannel(ch, ErrAddrEliminated)

	// Clear all idle connections
	for len(pr.idles) > 0 {
		p.freeChan <- <-pr.idles
	}

	// Call the address eliminated hook
	p.eliminatedHook(addr)
	return true
}

// filter deletes the repeated addresses
func filter(addrs []string) (ans []string) {
	m := make(map[string]struct{})
	for _, addr := range addrs {
		if _, ok := m[addr]; !ok {
			ans = append(ans, addr)
			m[addr] = struct{}{}
		}
	}
	return ans
}

func defaultWarmUp(waits, idles, actives, max uint32) uint32 {
	hungry := func(waits, idles, actives uint32) bool {
		if waits < actives {
			return false
		}
		if idles == 0 && waits > 0 {
			return true
		}
		return idles > 0 && float64(waits)/float64(idles) >= 2
	}
	if hungry(waits, idles, actives) && float64(actives)/float64(max) < 0.85 {
		return uint32(math.Round(math.Min(float64(waits-idles)*0.4, float64(max-actives))))
	}
	return 0
}

func attemptWarmUp(p *pool, pr *peer, ch chan *request) {
	pr.am.Lock()
	nums := uint32(0)
	if pr.actives < p.maxNums && len(ch) > len(pr.idles) && p.warmUp != nil {
		nums = p.warmUp(uint32(len(ch)), uint32(len(pr.idles)), pr.actives, p.maxNums)
		if nums > p.maxNums-pr.actives {
			nums = p.maxNums - pr.actives
		}
	}
	pr.am.Unlock()
	for i := uint32(0); i < nums; i++ {
		p.dial(pr, nil)
	}
}
