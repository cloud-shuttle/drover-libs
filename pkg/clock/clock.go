package clock

import "time"

// Clock provides an interface for time-related operations
// to allow for deterministic testing.
type Clock interface {
	Now() time.Time
	Since(t time.Time) time.Duration
}

// RealClock is the standard implementation that uses the real system time.
type RealClock struct{}

// Now returns time.Now().
func (RealClock) Now() time.Time {
	return time.Now()
}

// Since returns time.Since(t).
func (RealClock) Since(t time.Time) time.Duration {
	return time.Since(t)
}
