package clock

import (
	"sync"
	"time"
)

// MockClock is a Clock implementation for deterministic testing.
type MockClock struct {
	mu          sync.Mutex
	currentTime time.Time
}

// NewMockClock creates a new MockClock starting at the given time.
func NewMockClock(start time.Time) *MockClock {
	return &MockClock{
		currentTime: start,
	}
}

// Now returns the mock's current time. It does not advance on its own.
func (m *MockClock) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.currentTime
}

// Since returns the mock's current time minus t.
func (m *MockClock) Since(t time.Time) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.currentTime.Sub(t)
}

// SetTime explicitly sets the current time of the mock clock.
func (m *MockClock) SetTime(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentTime = t
}

// Add advances the mock clock by the given duration.
func (m *MockClock) Add(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentTime = m.currentTime.Add(d)
}
