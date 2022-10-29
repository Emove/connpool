package backoff

import (
	"math"
	"math/rand"
	"time"
)

type Option func(policy *exponentialPolicy)

const (
	defaultFactor       = 2
	defaultMinInterval  = 100 * time.Millisecond
	defaultMaxInterval  = 30 * time.Second
	defaultJitterFactor = float64(0.0)
)

// NewExponentialPolicy returns a exponential backoff policy
func NewExponentialPolicy(ops ...Option) Policy {
	p := &exponentialPolicy{
		multiplier:  defaultFactor,
		minInterval: defaultMinInterval,
		maxInterval: defaultMaxInterval,
		jitter:      defaultJitterFactor,
	}

	for _, op := range ops {
		op(p)
	}

	if p.minInterval <= 0 {
		p.minInterval = defaultMinInterval
	}
	if p.maxInterval <= p.minInterval {
		p.maxInterval = defaultMaxInterval
	}
	if p.jitter < 0 {
		p.jitter = defaultJitterFactor
	}
	if p.multiplier <= 1 {
		p.multiplier = defaultFactor
	}
	return p
}

// Multiplier multiplier is the factor with which to multiply backoffs
// after a failed retry. Should be greater than 1.
func Multiplier(multiplier float64) Option {
	return func(policy *exponentialPolicy) {
		policy.multiplier = multiplier
	}
}

// MinInterval sets the amount of time to backoff after the first failure.
func MinInterval(minInterval time.Duration) Option {
	return func(policy *exponentialPolicy) {
		policy.minInterval = minInterval
	}
}

// MaxInterval sets the upper bound of backoff delay.
func MaxInterval(interval time.Duration) Option {
	return func(policy *exponentialPolicy) {
		policy.maxInterval = interval
	}
}

// Jitter sets the factor with which backoffs are randomized.
func Jitter(jitter float64) Option {
	return func(policy *exponentialPolicy) {
		policy.jitter = jitter
	}
}

var _ Policy = (*exponentialPolicy)(nil)

type exponentialPolicy struct {
	multiplier  float64
	minInterval time.Duration
	maxInterval time.Duration
	jitter      float64
}

func (p *exponentialPolicy) New() IntervalGenerator {
	return &exponentialIntervalGenerator{policy: p, rn: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

var _ IntervalGenerator = (*exponentialIntervalGenerator)(nil)

type exponentialIntervalGenerator struct {
	policy *exponentialPolicy
	rn     *rand.Rand
}

func (eb *exponentialIntervalGenerator) Next(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}
	p := eb.policy

	// calculate this duration
	min := float64(p.minInterval)
	dur := min * math.Pow(p.multiplier, float64(attempt-1))

	// check overflow
	if dur > float64(p.maxInterval) {
		dur = float64(p.maxInterval)
	}

	if p.jitter > 0 {
		dur = eb.calculateJitter(dur)
	}

	return time.Duration(dur)
}

func (eb *exponentialIntervalGenerator) calculateJitter(interval float64) float64 {
	delta := eb.policy.jitter * interval
	min := interval - delta
	max := interval + delta

	return min + eb.rn.Float64()*(max-min+1)
}
