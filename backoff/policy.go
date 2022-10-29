package backoff

import "time"

// Policy is an interface for the dial policies.
type Policy interface {
	New() IntervalGenerator
}

// IntervalGenerator used to generate next interval.
type IntervalGenerator interface {
	Next(attempt int) time.Duration
}

// NewNullPolicy does not do any backoff. It always
// allows the caller to execute dial.
func NewNullPolicy() Policy {
	return &nullPolicy{}
}

var _ Policy = (*nullPolicy)(nil)

type nullPolicy struct{}

func (nullPolicy) New() IntervalGenerator {
	return &nullIntervalGenerator{}
}

var _ IntervalGenerator = (*nullIntervalGenerator)(nil)

type nullIntervalGenerator struct{}

func (nullIntervalGenerator) Next(attempt int) time.Duration {
	return time.Duration(0)
}
