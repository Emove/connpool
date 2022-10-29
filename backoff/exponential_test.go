package backoff

import (
	"fmt"
	"testing"
	"time"
)

func Test_exponentialIntervalGenerator_Next(t *testing.T) {
	//p := NewExponentialPolicy(Multiplier(2), MinInterval(100*time.Millisecond), MaxInterval(time.Second), Jitter(0.2))
	p := NewExponentialPolicy(Multiplier(2), MinInterval(100*time.Millisecond), MaxInterval(time.Second))
	generator := p.New()
	for i := 1; i <= 11; i++ {
		fmt.Println(generator.Next(i))
	}
}
