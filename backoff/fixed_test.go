package backoff

import (
	"testing"
	"time"
)

func Test_fixedIntervalGenerator_Next(t *testing.T) {
	policy := NewFixedIntervalPolicy(100 * time.Millisecond)
	generator := policy.New()
	for i := 0; i < 100; i++ {
		interval := generator.Next(i)
		if interval != 100*time.Millisecond {
			t.Fatalf("expected: %d, but: %d", 100*time.Millisecond, interval)
		}
	}
}
