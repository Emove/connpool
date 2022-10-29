package connpool

import (
	"time"

	"github.com/emove/connpool/backoff"
)

// Option pool option
type Option func(p *pool)

// CoreNums sets how many connections should be initial of per address
func CoreNums(nums int) Option {
	return func(p *pool) {
		if nums > 0 {
			p.coreNums = uint32(nums)
		}
	}
}

// MaxNums sets max connection number of per address
func MaxNums(nums int) Option {
	return func(p *pool) {
		if nums > 0 {
			p.maxNums = uint32(nums)
		}
	}
}

// MaxWaitingCapacity sets waiting request channel capacity
func MaxWaitingCapacity(nums int) Option {
	return func(p *pool) {
		if nums > 0 {
			p.maxWaiting = uint64(nums)
		}
	}
}

// MaxIdleTime sets a duration before an idle connection would be closed
func MaxIdleTime(d time.Duration) Option {
	return func(p *pool) {
		if d > 0 {
			p.maxIdleTime = d
		}
	}
}

// WaitTimeout sets max waiting time when get connection from pool
func WaitTimeout(d time.Duration) Option {
	return func(p *pool) {
		if d > 0 {
			p.waitTimeout = d
		}
	}
}

// Lazy dials only when in need
func Lazy() Option {
	return func(p *pool) {
		p.lazy = true
	}
}

// WithWarmUp sets warm up func to control the warming up connection pool
func WithWarmUp(wu WarmUp) Option {
	return func(p *pool) {
		p.warmUp = wu
	}
}

// EliminateAddr eliminates the address from pool when an address dial error times greater than maxErrorTimes or continuous error duration greater than maxErrorPeriod
func EliminateAddr(maxErrorTimes int, maxErrorPeriod time.Duration, hook EliminatedHook) Option {
	return func(p *pool) {
		if maxErrorTimes > 0 {
			p.dr.maxRetryTimes = maxErrorTimes
		}
		if maxErrorPeriod > 0 {
			p.dr.maxRetryPeriod = maxErrorPeriod
		}
		p.eliminateConn = true
		p.eliminatedHook = hook
	}
}

// DialBackoffPolicy sets the backoff policy for dial error
func DialBackoffPolicy(policy backoff.Policy) Option {
	return func(p *pool) {
		p.dr.policy = policy
	}
}
