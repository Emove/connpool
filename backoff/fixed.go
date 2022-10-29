package backoff

import "time"

// NewFixedIntervalPolicy returns a fixed interval policy
func NewFixedIntervalPolicy(interval time.Duration) Policy {
	return &fixedIntervalPolicy{
		interval: interval,
	}
}

var _ Policy = (*fixedIntervalPolicy)(nil)

type fixedIntervalPolicy struct {
	interval time.Duration
}

func (fip *fixedIntervalPolicy) New() IntervalGenerator {
	return &fixedIntervalGenerator{
		interval: fip.interval,
	}
}

var _ IntervalGenerator = (*fixedIntervalGenerator)(nil)

type fixedIntervalGenerator struct {
	interval time.Duration
}

func (fig *fixedIntervalGenerator) Next(_ int) time.Duration {
	return fig.interval
}
